package k8sresource

import (
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// KonfluxRelease is similar to
// https://github.com/konflux-ci/release-service/blob/main/api/v1alpha1/release_types.go
// except having only the fields we need for it.
// We're creating our own struct for the release type, so we don't need to add
// the dependency to github.com/konflux-ci/release-service only for this type.
type KonfluxRelease struct {
	metav1.PartialObjectMetadata `json:",inline"`

	Spec struct {
		ReleasePlan string `json:"releasePlan"`
		Snapshot    string `json:"snapshot"`
	} `json:"spec"`

	Status struct {
		Conditions []metav1.Condition `json:"conditions"`
	} `json:"status"`
}

var KonfluxReleaseGVR = schema.GroupVersionResource{
	Group:    "appstudio.redhat.com",
	Version:  "v1alpha1",
	Resource: "releases",
}

func KonfluxReleaseFromFile(yamlFilePath string) (*KonfluxRelease, error) {
	release := KonfluxRelease{}
	if err := unmarshalYaml(yamlFilePath, &release); err != nil {
		return nil, fmt.Errorf("failed to unmarshal yaml file: %w", err)
	}

	return &release, nil
}
