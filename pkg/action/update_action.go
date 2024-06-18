package action

import (
	"bytes"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"

	"github.com/openshift-knative/hack/pkg/prowgen"
)

type Config struct {
	InputAction     string
	InputConfigPath string
	OutputAction    string
}

func UpdateAction(cfg Config) error {
	var steps []interface{}

	y, err := os.ReadFile(cfg.InputAction)
	if err != nil {
		return err
	}
	var node yaml.Node
	if err := yaml.NewDecoder(bytes.NewBuffer(y)).Decode(&node); err != nil {
		return fmt.Errorf("failed to decode file into node: %w", err)
	}

	if err := AddNestedField(&node, "Generate CI config", "name"); err != nil {
		return fmt.Errorf("failed to add steps: %w", err)
	}

	err = filepath.Walk(cfg.InputConfigPath, func(path string, info fs.FileInfo, err error) error {
		if info.IsDir() {
			return nil
		}

		inConfig, err := prowgen.LoadConfig(path)
		if err != nil {
			return err
		}

		for _, r := range inConfig.Repositories {
			for branchName, b := range inConfig.Config.Branches {
				if b.Konflux != nil && b.Konflux.Enabled {

					// Special case "release-next"
					targetBranch := branchName
					if branchName == "release-next" {
						targetBranch = "main"
					}

					commit := fmt.Sprintf("[%s] Sync Konflux configurations", targetBranch)

					steps = append(steps, map[string]interface{}{
						"name": fmt.Sprintf("[%s - %s] Create Konflux PR", r.Repo, branchName),
						"if":   "(github.event_name == 'push' || github.event_name == 'workflow_dispatch') && github.ref_name == 'main'",
						"uses": "peter-evans/create-pull-request@v5",
						"with": map[string]interface{}{
							"token":          "${{ secrets.SERVERLESS_QE_ROBOT }}",
							"path":           fmt.Sprintf("/src/github.com/openshift-knative/hack/%s", r.RepositoryDirectory()),
							"base":           targetBranch,
							"branch":         fmt.Sprintf("%s%s", prowgen.KonfluxBranchPrefix, branchName),
							"title":          commit,
							"commit-message": commit,
							"push-to-fork":   fmt.Sprintf("serverless-qe/%s", r.Repo),
							"delete-branch":  true,
							"body":           commit,
						},
					})
				}
			}
		}

		return nil
	})
	if err != nil {
		return fmt.Errorf("failed to walk filesystem path %q: %w", cfg.InputConfigPath, err)
	}

	if err := AddNestedField(&node, steps, "jobs", "generate-ci", "steps"); err != nil {
		return fmt.Errorf("failed to add steps: %w", err)
	}

	buf := bytes.NewBuffer(nil)
	if err := yaml.NewEncoder(buf).Encode(&node); err != nil {
		return fmt.Errorf("failed to encode node into buf: %w", err)
	}

	if err := os.WriteFile(cfg.OutputAction, buf.Bytes(), 0600); err != nil {
		return fmt.Errorf("failed to write updates: %w", err)
	}

	return nil
}

func AddNestedField(node *yaml.Node, value interface{}, fields ...string) error {

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

					n.Content = append(n.Content, s.Content[0].Content...)
				}
				break
			}

			// Continue to the next level
			return AddNestedField(n, value, fields[1:]...)
		}

		if node.Kind == yaml.DocumentNode {
			return AddNestedField(n, value, fields...)
		}
	}

	return nil
}
