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
	projectID string
	zone      string
	instance  string

	slackSigningSecretSecretID string

	notifyTextTemplate         string
	notifyChannelWebhookRawurl string

	secretManagerClient *secretmanager.Client
	instancesService    *compute.InstancesService
)

func init() {
	var err error

	zone = os.Getenv("TARGET_ZONE")
	instance = os.Getenv("TARGET_INSTANCE_NAME")
	slackSigningSecretSecretID = os.Getenv("SLACK_SIGNING_SECRET_SECRET_ID")
	notifyTextTemplate = os.Getenv("NOTIFY_TEXT_TEMPLATE")
	notifyChannelWebhookRawurl = os.Getenv("NOTIFY_CHANNEL_WEBHOOK_URL")

	projectID, err = metadata.ProjectID()
	if err != nil {
		log.Fatalf("Get project ID: %v.", err)
	}

	ctx := context.Background()

	secretManagerClient, err = secretmanager.NewClient(ctx)
	if err != nil {
		log.Fatalf("New secret manager client: %v.", err)
	}

	s, err := compute.NewService(ctx)
	if err != nil {
		log.Fatalf("New compute service: %v.", err)
	}
	instancesService = s.Instances
}

type payload struct {
	ChannelName string
	UserName    string
	ProjectID   string
	Zone        string
	Instance    string
}

func ResetInstance(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	body, err := ioutil.ReadAll(r.Body)
	if err != nil {
		errorHandler(w, fmt.Errorf("read body: %w", err))
		return
	}

	if err := verifyRequest(ctx, r.Header, body); err != nil {
		errorHandler(w, fmt.Errorf("verify request: %w", err))
		return
	}

	if _, err := instancesService.Reset(projectID, zone, instance).Context(ctx).Do(); err != nil {
		errorHandler(w, fmt.Errorf("reset instance: %w", err))
		return
	}

	if _, err := w.Write([]byte("OK")); err != nil {
		errorHandler(w, fmt.Errorf("write body: %w", err))
		return
	}

	if err := notifyWebhook(ctx, body); err != nil {
		errorHandler(w, fmt.Errorf("notify webhook: %w", err))
		return
	}
}

func verifyRequest(ctx context.Context, header http.Header, body []byte) error {
	resp, err := secretManagerClient.AccessSecretVersion(ctx,
		&secretmanagerpb.AccessSecretVersionRequest{
			Name: fmt.Sprintf(
				"projects/%s/secrets/%s/versions/latest",
				projectID,
				slackSigningSecretSecretID,
			),
		},
	)
	if err != nil {
		return fmt.Errorf("access secret version: %w", err)
	}

	sv, err := slack.NewSecretsVerifier(header, string(resp.Payload.Data))
	if err != nil {
		return fmt.Errorf("new secrets verifier: %w", err)
	}

	if _, err := sv.Write(body); err != nil {
		return fmt.Errorf("write body: %w", err)
	}

	if err := sv.Ensure(); err != nil {
		return fmt.Errorf("ensure: %w", err)
	}

	return nil
}

func notifyWebhook(ctx context.Context, body []byte) error {
	if notifyTextTemplate == "" || notifyChannelWebhookRawurl == "" {
		return nil
	}

	vs, err := url.ParseQuery(string(body))
	if err != nil {
		return fmt.Errorf("parse body: %w", err)
	}

	p := payload{
		ChannelName: vs.Get("channel_name"),
		UserName:    vs.Get("user_name"),
		ProjectID:   projectID,
		Zone:        zone,
		Instance:    instance,
	}

	b := &strings.Builder{}
	t := template.Must(template.New("").Parse(notifyTextTemplate))
	if err := t.Execute(b, p); err != nil {
		return fmt.Errorf("execute template: %w", err)
	}

	msg := &slack.WebhookMessage{Text: b.String()}
	if err := slack.PostWebhookContext(ctx, notifyChannelWebhookRawurl, msg); err != nil {
		return fmt.Errorf("post message to webhook: %w", err)
	}

	return nil
}

func errorHandler(w http.ResponseWriter, err error) {
	if _, e := w.Write([]byte(err.Error())); e != nil {
		log.Printf("Write error: %v.", e)
	}
	log.Printf("Error : %v.", err)
}
