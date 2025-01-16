package action

import (
	"bytes"
	"cmp"
	"context"
	"fmt"
	"path/filepath"
	"sort"

	"gopkg.in/yaml.v3"
)

type Config struct {
	InputAction     string
	InputConfigPath string
	OutputAction    string
}

func Generate(ctx context.Context, inputConfig string, outputFolder string) error {
	if err := UpdateAction(ctx, Config{
		InputConfigPath: inputConfig,
		InputAction:     ".github/workflows/release-generate-ci-template.yaml",
		OutputAction:    filepath.Join(outputFolder, "release-generate-ci.yaml"),
	}); err != nil {
		return fmt.Errorf("could not generate update action: %w", err)
	}

	if err := GoModuleBumpAction(ctx, Config{
		InputConfigPath: inputConfig,
		InputAction:     ".github/workflows/go-module-bump-template.yaml",
		OutputAction:    filepath.Join(outputFolder, "go-module-bump.yaml"),
	}); err != nil {
		return fmt.Errorf("could not generate Go module bump action: %w", err)
	}

	return nil
}

func AddNestedField(node *yaml.Node, value interface{}, prepend bool, fields ...string) error {

	for i, n := range node.Content {

		if i > 0 && node.Content[i-1].Value == fields[0] {

			// Base case for scalar nodes
			if len(fields) == 1 && n.Kind == yaml.ScalarNode {
				n.SetString(fmt.Sprintf("%s", value))
				break
			}
			// base case for sequence node
			if len(fields) == 1 && n.Kind == yaml.SequenceNode {

				if v, ok := value.([]interface{}); ok {
					var s yaml.Node

					b, err := yaml.Marshal(v)
					if err != nil {
						return err
					}
					if err := yaml.NewDecoder(bytes.NewBuffer(b)).Decode(&s); err != nil {
						return err
					}

					if prepend {
						n.Content = append(s.Content[0].Content, n.Content...)
					} else {
						n.Content = append(n.Content, s.Content[0].Content...)
					}

					// print list entries in a single line each
					n.Style = yaml.LiteralStyle
				}
				break
			}

			// Continue to the next level
			return AddNestedField(n, value, prepend, fields[1:]...)
		}

		if node.Kind == yaml.DocumentNode {
			return AddNestedField(n, value, prepend, fields...)
		}
	}

	return nil
}

func sortedKeys[K cmp.Ordered, V any](m map[K]V) []K {
	keys := make([]K, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool { return keys[i] < keys[j] })
	return keys
}
