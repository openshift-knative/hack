package project

import (
	"fmt"
	"os"

	"k8s.io/apimachinery/pkg/util/yaml"
)

// Metadata is the structure for project metadata
// Every project should have a file, usually called
// project.yaml that contains such metadata.
type Metadata struct {
	Project      Project       `json:"project" yaml:"project"`
	Requirements *Requirements `json:"requirements,omitempty" yaml:"requirements,omitempty"`
}

type Project struct {
	Tag         string `json:"tag" yaml:"tag"`
	ImagePrefix string `json:"imagePrefix" yaml:"imagePrefix"`
	Version     string `json:"version" yaml:"version"`
}

type Requirements struct {
	OcpVersion OcpVersion `json:"ocpVersion" yaml:"ocpVersion"`
}

type OcpVersion struct {
	Min   string   `json:"min" yaml:"min"`
	Max   string   `json:"max" yaml:"max"`
	Label string   `json:"label" yaml:"label"`
	List  []string `json:"list" yaml:"list"`
}

func DefaultMetadata() *Metadata {
	return &Metadata{
		Requirements: &Requirements{},
	}
}

func ReadMetadataFile(path string) (*Metadata, error) {
	bytes, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read file: %w", err)
	}
	m := &Metadata{}
	return m, yaml.Unmarshal(bytes, m)
}
