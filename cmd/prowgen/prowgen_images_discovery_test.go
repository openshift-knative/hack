package main

import (
	"testing"

	cioperatorapi "github.com/openshift/ci-tools/pkg/api"
	"k8s.io/utils/pointer"
)

func TestDiscoverImages(t *testing.T) {

	r := Repository{
		Org:                   "testdata",
		Repo:                  "eventing",
		ImagePrefix:           "knative-eventing",
		CanonicalGoRepository: pointer.String("knative.dev/eventing"),
	}

	options := DiscoverImages(r)

	expectedImages := []cioperatorapi.ProjectDirectoryImageBuildStepConfiguration{
		{
			To: "knative-eventing-dispatcher",
			ProjectDirectoryImageBuildInputs: cioperatorapi.ProjectDirectoryImageBuildInputs{
				DockerfilePath: "openshift/ci-operator/knative-images/dispatcher/Dockerfile",
			},
		},
		{
			To: "knative-eventing-test-webhook",
			ProjectDirectoryImageBuildInputs: cioperatorapi.ProjectDirectoryImageBuildInputs{
				DockerfilePath: "openshift/ci-operator/knative-test-images/webhook/Dockerfile",
			},
		},
	}

	cfg := cioperatorapi.ReleaseBuildConfiguration{}

	if err := applyOptions(&cfg, options); err != nil {
		t.Fatal(err)
	}

	if len(cfg.Images) != len(expectedImages) {
		t.Errorf("expected %d images, got %d images", len(expectedImages), len(cfg.Images))
	}

	for i, expectedImage := range expectedImages {
		got := cfg.Images[i]
		if got.To != expectedImage.To {
			t.Errorf("Want 'to' %s, got %s", expectedImage.To, got.To)
		}
		if got.DockerfilePath != expectedImage.DockerfilePath {
			t.Errorf("Want dockerfile_path %s, got %s", expectedImage.DockerfilePath, got.DockerfilePath)
		}
	}
}
