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
	Project Project `json:"project" yaml:"project"`
}

type Project struct {
	Tag         string `json:"tag" yaml:"tag"`
	ImagePrefix string `json:"imagePrefix" yaml:"imagePrefix"`
}

func ReadMetadataFile(path string) (*Metadata, error) {
	bytes, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read file: %w", err)
	}
	m := &Metadata{}
	return m, yaml.Unmarshal(bytes, m)
}
