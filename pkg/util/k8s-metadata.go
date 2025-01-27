package util

import (
	"encoding/json"
	"fmt"
	"os"

	gyaml "github.com/ghodss/yaml"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func K8sMetadata(yamlFilePath string) (*metav1.PartialObjectMetadata, error) {
	metadata := metav1.PartialObjectMetadata{}

	y, err := os.ReadFile(yamlFilePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read file: %w", err)
	}

	j, err := gyaml.YAMLToJSON(y)
	if err != nil {
		return nil, fmt.Errorf("failed to convert yaml file to json: %w", err)
	}

	if err := json.Unmarshal(j, &metadata); err != nil {
		return nil, fmt.Errorf("failed to unmarshal json file: %w", err)
	}

	return &metadata, nil
}
