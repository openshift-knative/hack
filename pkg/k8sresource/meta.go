package k8sresource

import (
	"encoding/json"
	"fmt"
	"os"

	gyaml "github.com/ghodss/yaml"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func Metadata(yamlFilePath string) (*metav1.PartialObjectMetadata, error) {
	metadata := metav1.PartialObjectMetadata{}
	if err := unmarshalYaml(yamlFilePath, &metadata); err != nil {
		return nil, fmt.Errorf("failed to unmarshal yaml file: %w", err)
	}

	return &metadata, nil
}

func unmarshalYaml[T interface{}](yamlFilePath string, target *T) error {
	y, err := os.ReadFile(yamlFilePath)
	if err != nil {
		return fmt.Errorf("failed to read file: %w", err)
	}

	j, err := gyaml.YAMLToJSON(y)
	if err != nil {
		return fmt.Errorf("failed to convert yaml file to json: %w", err)
	}

	if err := json.Unmarshal(j, target); err != nil {
		return fmt.Errorf("failed to unmarshal json file: %w", err)
	}

	return nil
}
