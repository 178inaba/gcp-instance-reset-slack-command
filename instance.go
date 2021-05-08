package instance

import (
	"context"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"
	"text/template"

	"cloud.google.com/go/compute/metadata"
	secretmanager "cloud.google.com/go/secretmanager/apiv1"
	"github.com/slack-go/slack"
	"google.golang.org/api/compute/v1"
	secretmanagerpb "google.golang.org/genproto/googleapis/cloud/secretmanager/v1"
)

var (
	projectID           string
	zone                string
	instancesService    *compute.InstancesService
	secretManagerClient *secretmanager.Client
)

func init() {
	var err error

	projectID, err = metadata.ProjectID()
	if err != nil {
		log.Fatal(err)
	}

	ctx := context.Background()

	secretManagerClient, err = secretmanager.NewClient(ctx)
	if err != nil {
		log.Fatal(err)
	}

	s, err := compute.NewService(ctx)
	if err != nil {
		log.Fatal(err)
	}
	instancesService = s.Instances
}

type payload struct {
	ChannelName string
	UserName    string
}

func ResetInstance(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	body, err := ioutil.ReadAll(r.Body)
	if err != nil {
		errorHandler(w, err)
		return
	}

	if err := verifyRequest(ctx, r.Header, body); err != nil {
		errorHandler(w, err)
		return
	}

	p, err := newPayload(body)
	if err != nil {
		errorHandler(w, err)
		return
	}

	if _, err := instancesService.Reset(projectID, os.Getenv("TARGET_ZONE"), os.Getenv("TARGET_INSTANCE_NAME")).Context(ctx).Do(); err != nil {
		errorHandler(w, err)
		return
	}

	if _, err := w.Write([]byte("OK")); err != nil {
		errorHandler(w, err)
		return
	}

	if err := notifyWebhook(ctx, p); err != nil {
		errorHandler(w, err)
		return
	}
}

func newPayload(body []byte) (*payload, error) {
	vs, err := url.ParseQuery(string(body))
	if err != nil {
		return nil, err
	}

	return &payload{
		ChannelName: vs.Get("channel_name"),
		UserName:    vs.Get("user_name"),
	}, nil
}

func verifyRequest(ctx context.Context, header http.Header, body []byte) error {
	resp, err := secretManagerClient.AccessSecretVersion(ctx,
		&secretmanagerpb.AccessSecretVersionRequest{
			Name: fmt.Sprintf(
				"projects/%s/secrets/%s/versions/latest",
				projectID,
				os.Getenv("SLACK_SIGNING_SECRET_SECRET_ID"),
			),
		},
	)
	if err != nil {
		return err
	}

	sv, err := slack.NewSecretsVerifier(header, string(resp.Payload.Data))
	if err != nil {
		return err
	}

	if _, err := sv.Write(body); err != nil {
		return err
	}

	if err := sv.Ensure(); err != nil {
		return err
	}

	return nil
}

func notifyWebhook(ctx context.Context, payload *payload) error {
	webhookRawurl := os.Getenv("NOTIFY_CHANNEL_WEBHOOK_URL")
	textTmpl := os.Getenv("NOTIFY_TEXT_TEMPLATE")
	if webhookRawurl == "" || textTmpl == "" {
		return nil
	}

	b := &strings.Builder{}
	t := template.Must(template.New("").Parse(textTmpl))
	if err := t.Execute(b, payload); err != nil {
		return err
	}

	msg := &slack.WebhookMessage{Text: b.String()}
	if err := slack.PostWebhookContext(ctx, webhookRawurl, msg); err != nil {
		return err
	}

	return nil
}

func errorHandler(w http.ResponseWriter, err error) {
	if _, e := w.Write([]byte(err.Error())); err != nil {
		log.Print(e)
	}
	log.Print(err)
}
