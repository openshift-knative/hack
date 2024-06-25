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
	var cloneSteps []interface{}

	y, err := os.ReadFile(cfg.InputAction)
	if err != nil {
		return err
	}
	var node yaml.Node
	if err := yaml.NewDecoder(bytes.NewBuffer(y)).Decode(&node); err != nil {
		return fmt.Errorf("failed to decode file into node: %w", err)
	}

	if err := AddNestedField(&node, "Generate CI config", false, "name"); err != nil {
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

			cloneSteps = append(cloneSteps,
				map[string]interface{}{
					"name": fmt.Sprintf("[%s] Clone repository", r.Repo),
					"if":   "${{ (github.event_name == 'push' || github.event_name == 'workflow_dispatch' || github.event_name == 'schedule') && github.ref_name == 'main' }}",
					"uses": "actions/checkout@v4",
					"with": map[string]interface{}{
						"repository": r.RepositoryDirectory(),
						"token":      "${{ secrets.SERVERLESS_QE_ROBOT }}",
						"path":       fmt.Sprintf("./src/github.com/openshift-knative/hack/%s", r.RepositoryDirectory()),
					},
				})

			for branchName, b := range inConfig.Config.Branches {
				if b.Konflux != nil && b.Konflux.Enabled {

					// Special case "release-next"
					targetBranch := branchName
					if branchName == "release-next" {
						targetBranch = "main"
					}

					localBranch := fmt.Sprintf("%s%s", prowgen.KonfluxBranchPrefix, branchName)
					steps = append(steps, map[string]interface{}{
						"name": fmt.Sprintf("[%s - %s] Create Konflux PR", r.Repo, branchName),
						"if":   "${{ (github.event_name == 'push' || github.event_name == 'workflow_dispatch' || github.event_name == 'schedule') && github.ref_name == 'main' }}",
						"env": map[string]string{
							"GH_TOKEN":     "${{ secrets.SERVERLESS_QE_ROBOT }}",
							"GITHUB_TOKEN": "${{ secrets.SERVERLESS_QE_ROBOT }}",
						},
						"working-directory": fmt.Sprintf("./src/github.com/openshift-knative/hack/%s", r.RepositoryDirectory()),
						"run": fmt.Sprintf(`set -x
git remote add fork "https://github.com/serverless-qe/%s.git"
git push "https://serverless-qe:${GH_TOKEN}@github.com/serverless-qe/%s.git" %s:%s -f
gh pr create --base %s --head %s --fill-verbose
`,
							r.Repo,
							r.Repo,
							localBranch,
							localBranch,
							targetBranch,
							fmt.Sprintf("serverless-qe:%s", localBranch),
						),
					})
				}
			}
		}

		return nil
	})
	if err != nil {
		return fmt.Errorf("failed to walk filesystem path %q: %w", cfg.InputConfigPath, err)
	}

	if err := AddNestedField(&node, cloneSteps, true, "jobs", "generate-ci", "steps"); err != nil {
		return fmt.Errorf("failed to add cloned steps: %w", err)
	}

	if err := AddNestedField(&node, steps, false, "jobs", "generate-ci", "steps"); err != nil {
		return fmt.Errorf("failed to add steps: %w", err)
	}

	buf := bytes.NewBuffer(nil)
	enc := yaml.NewEncoder(buf)
	enc.SetIndent(2)
	if err := enc.Encode(&node); err != nil {
		return fmt.Errorf("failed to encode node into buf: %w", err)
	}
	defer enc.Close()

	if err := os.WriteFile(cfg.OutputAction, buf.Bytes(), 0600); err != nil {
		return fmt.Errorf("failed to write updates: %w", err)
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
