package instance

import (
	"context"
	"log"
	"net/http"
	"os"

	"cloud.google.com/go/compute/metadata"
	"google.golang.org/api/compute/v1"
)

var projectID, zone string
var instancesService *compute.InstancesService

func init() {
	var err error

	projectID, err = metadata.ProjectID()
	if err != nil {
		log.Fatal(err)
	}

	s, err := compute.NewService(context.Background())
	if err != nil {
		log.Fatal(err)
	}
	instancesService = s.Instances
}

func ResetInstance(w http.ResponseWriter, r *http.Request) {
	if _, err := instancesService.Reset(projectID, zone, os.Getenv("TARGET_INSTANCE_NAME")).Context(r.Context()).Do(); err != nil {
		log.Fatal(err)
	}
}
