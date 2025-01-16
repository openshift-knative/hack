package action

import (
	"bytes"
	"context"
	"fmt"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"gopkg.in/yaml.v3"

	"github.com/openshift-knative/hack/pkg/dependabotgen"
	"github.com/openshift-knative/hack/pkg/prowgen"
)

func UpdateAction(ctx context.Context, cfg Config) error {
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
		if info.IsDir() || !strings.HasSuffix(path, ".yaml") {
			return nil
		}

		inConfig, err := prowgen.LoadConfig(path)
		if err != nil {
			return err
		}

		cs, s, err := updateAction(ctx, inConfig)
		if err != nil {
			return fmt.Errorf("failed to update action: %w", err)
		}
		cloneSteps = append(cloneSteps, cs...)
		steps = append(steps, s...)

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

func updateAction(ctx context.Context, inConfig *prowgen.Config) ([]interface{}, []interface{}, error) {
	var cloneSteps []interface{}
	var steps []interface{}
	for _, r := range inConfig.Repositories {

		if err := prowgen.GitMirror(ctx, r); err != nil {
			return nil, nil, err
		}

		log.Println(r.RepositoryDirectory(), "update action")

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

		addDependabotStepOnce := sync.OnceFunc(func() {

			steps = append(steps, map[string]interface{}{
				"name": fmt.Sprintf("[%s - %s] Update dependabot configurations", r.Repo, dependabotgen.DefaultTargetBranch),
				"if":   "${{ (github.event_name == 'push' || github.event_name == 'workflow_dispatch' || github.event_name == 'schedule') && github.ref_name == 'main' }}",
				"env": map[string]string{
					"GH_TOKEN":     "${{ secrets.SERVERLESS_QE_ROBOT }}",
					"GITHUB_TOKEN": "${{ secrets.SERVERLESS_QE_ROBOT }}",
				},
				"working-directory": fmt.Sprintf("./src/github.com/openshift-knative/hack/%s", r.RepositoryDirectory()),
				"run": fmt.Sprintf(`set -x
repo="%s"
branch="%s"
target_branch="%s"
git remote add fork "https://github.com/serverless-qe/$repo.git" || true # ignore: already exists errors
remote_exists=$(git ls-remote --heads fork "$branch")
if [ -z "$remote_exists" ]; then
  # remote doesn't exist.
  git push "https://serverless-qe:${GH_TOKEN}@github.com/serverless-qe/$repo.git" "$branch:$branch" -f || exit 1
fi
git fetch fork "$branch"
if git diff --quiet "fork/$branch" "$branch"; then
  echo "Branches are identical. No need to force push."
else
  git push "https://serverless-qe:${GH_TOKEN}@github.com/serverless-qe/$repo.git" "$branch:$branch" -f
fi
gh pr create --base "$target_branch" --head "serverless-qe:$branch" --title "[$target_branch] Update dependabot configurations" --body "Update dependabot configurations" --label needs-ok-to-test || true
`,
					r.Repo,
					fmt.Sprintf("%s%s", dependabotgen.SyncBranchPrefix, dependabotgen.DefaultTargetBranch),
					dependabotgen.DefaultTargetBranch,
				),
			})

		})

		sortedBranches := sortedKeys(inConfig.Config.Branches)

		for _, branchName := range sortedBranches {
			b := inConfig.Config.Branches[branchName]

			if b.DependabotEnabled == nil || *b.DependabotEnabled {
				addDependabotStepOnce()
			}

			if b.Konflux != nil && b.Konflux.Enabled {

				log.Println(r.RepositoryDirectory(), "adding branch", branchName)

				// Special case "release-next"
				targetBranch := branchName
				if branchName == "release-next" {
					targetBranch = "main"
				}

				if err := prowgen.GitCheckout(ctx, r, targetBranch); err != nil {
					if !strings.Contains(err.Error(), "failed to run git [checkout") {
						return nil, nil, err
					}
					// Skip non-existing branches
					log.Println(r.RepositoryDirectory(), "Skipping non existing branch", branchName)
					continue
				}

				localBranch := fmt.Sprintf("%s%s", prowgen.KonfluxBranchPrefix, branchName)

				if run := r.RunDockefileGenCommand(); run != "" && !r.IsServerlessOperator() {
					steps = append(steps, map[string]interface{}{
						"name":              fmt.Sprintf("[%s - %s] Update codegen", r.Repo, branchName),
						"working-directory": fmt.Sprintf("./src/github.com/openshift-knative/hack/%s", r.RepositoryDirectory()),
						"run": fmt.Sprintf(`set -euox
git checkout %s
%s
git add .
git commit -m "Run %s" || true # ignore: nothing to commit
`, localBranch, run, run),
					})
				}

				steps = append(steps, map[string]interface{}{
					"name": fmt.Sprintf("[%s - %s] Create Konflux PR", r.Repo, branchName),
					"if":   "${{ (github.event_name == 'push' || github.event_name == 'workflow_dispatch' || github.event_name == 'schedule') && github.ref_name == 'main' }}",
					"env": map[string]string{
						"GH_TOKEN":     "${{ secrets.SERVERLESS_QE_ROBOT }}",
						"GITHUB_TOKEN": "${{ secrets.SERVERLESS_QE_ROBOT }}",
					},
					"working-directory": fmt.Sprintf("./src/github.com/openshift-knative/hack/%s", r.RepositoryDirectory()),
					"run": fmt.Sprintf(`set -x
repo="%s"
branch="%s"
target_branch="%s"
git remote add fork "https://github.com/serverless-qe/$repo.git" || true # ignore: already exists errors
remote_exists=$(git ls-remote --heads fork "$branch")
if [ -z "$remote_exists" ]; then
  # remote doesn't exist.
  git push "https://serverless-qe:${GH_TOKEN}@github.com/serverless-qe/$repo.git" "$branch:$branch" -f || exit 1
fi
git fetch fork "$branch"
if git diff --quiet "fork/$branch" "$branch"; then
  echo "Branches are identical. No need to force push."
else
  git push "https://serverless-qe:${GH_TOKEN}@github.com/serverless-qe/$repo.git" "$branch:$branch" -f
fi
gh pr create --base "$target_branch" --head "serverless-qe:$branch" --title "[$target_branch] Update Konflux configurations" --body "Update Konflux components and pipelines" --label needs-ok-to-test || true
`,
						r.Repo,
						localBranch,
						targetBranch,
					),
				})
			}
		}
	}
	return cloneSteps, steps, nil
}
