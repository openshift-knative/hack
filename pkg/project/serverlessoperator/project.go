package serverlessoperator

import (
	"fmt"
	"os"

	"k8s.io/apimachinery/pkg/util/yaml"
)

// Metadata is the structure for project metadata
// of SOs project.yaml
type Metadata struct {
	Project      Project      `yaml:"project"`
	Olm          Olm          `yaml:"olm"`
	Requirements Requirements `yaml:"requirements"`
}

type Project struct {
	Name    string `yaml:"name"`
	Version string `yaml:"version"`
}

type Olm struct {
	Replaces  string   `yaml:"replaces"`
	SkipRange string   `yaml:"skipRange"`
	Channels  Channels `yaml:"channels"`
}

type Requirements struct {
	Kube       Kube       `yaml:"kube"`
	Golang     string     `yaml:"golang"`
	Nodejs     string     `yaml:"nodejs"`
	OcpVersion OcpVersion `yaml:"ocpVersion"`
}

type Channels struct {
	Default string   `yaml:"default"`
	List    []string `yaml:"list"`
}

type Kube struct {
	MinVersion string `yaml:"minVersion"`
}

type OcpVersion struct {
	Min   string `yaml:"min"`
	Max   string `yaml:"max"`
	Label string `yaml:"label"`
}

func ReadMetadataFile(path string) (*Metadata, error) {
	bytes, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read file: %w", err)
	}
	m := &Metadata{}
	return m, yaml.Unmarshal(bytes, m)
}
