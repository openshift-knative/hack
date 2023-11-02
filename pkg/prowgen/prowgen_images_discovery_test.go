package prowgen

import (
	"testing"

	cioperatorapi "github.com/openshift/ci-tools/pkg/api"
	"k8s.io/utils/pointer"
	"k8s.io/utils/strings/slices"
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
				Inputs: map[string]cioperatorapi.ImageBuildInputs{
					"openshift_release_golang-1.18": {
						As: []string{
							"registry.ci.openshift.org/openshift/release:golang-1.18",
						},
					},
				},
			},
		},
		{
			To: "knative-eventing-test-webhook",
			ProjectDirectoryImageBuildInputs: cioperatorapi.ProjectDirectoryImageBuildInputs{
				DockerfilePath: "openshift/ci-operator/knative-test-images/webhook/Dockerfile",
			},
		},
		{
			To: "knative-eventing-source-image",
			ProjectDirectoryImageBuildInputs: cioperatorapi.ProjectDirectoryImageBuildInputs{
				DockerfilePath: "openshift/ci-operator/source-image/Dockerfile",
			},
		},
	}

	expectedBaseImages := map[string]cioperatorapi.ImageStreamTagReference{
		"openshift_release_golang-1.18": {
			Namespace: "openshift",
			Name:      "release",
			Tag:       "golang-1.18",
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

		for key := range expectedImage.Inputs {
			if _, found := got.Inputs[key]; !found {
				t.Errorf("Key %s doesn't exist in got.Inputs", key)
			} else {
				if !slices.Equal(got.Inputs[key].As, expectedImage.Inputs[key].As) {
					t.Errorf("Want inputs[%s].As %v, got %v", key, expectedImage.Inputs[key].As, got.Inputs[key].As)
				}
			}

			if len(expectedImage.Inputs[key].As) != len(got.Inputs[key].As) {
				t.Errorf("expected %d image inputs.As for 'to' %s, got %d", len(expectedImage.Inputs[key].As), expectedImage.To, len(got.Inputs[key].As))
			}
		}
	}

	for name, imgref := range expectedBaseImages {
		if _, found := cfg.BaseImages[name]; !found {
			t.Errorf("%s doesn't exist in got.BaseImages", name)
		} else {
			if cfg.BaseImages[name] != imgref {
				t.Errorf("Want base image %v, got %v", expectedBaseImages[name], cfg.BaseImages[name])
			}
		}
	}
}
