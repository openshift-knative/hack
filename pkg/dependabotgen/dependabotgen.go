package dependabotgen

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"

	"gopkg.in/yaml.v3"
)

const (
	DependabotConfigVersion = "2"
	SyncBranchPrefix        = "sync-dependabot-"
	DefaultTargetBranch     = "main"
)

type DependabotConfig struct {
	Version string             `yaml:"version,omitempty"`
	Updates *DependabotUpdates `yaml:"updates,omitempty"`
}

func NewDependabotConfig() *DependabotConfig {
	return &DependabotConfig{
		Version: DependabotConfigVersion,
		Updates: &DependabotUpdates{},
	}
}

type DependabotUpdate struct {
	PackageEcosystem string         `yaml:"package-ecosystem,omitempty"`
	Directories      []string       `yaml:"directories,omitempty"`
	Schedule         ScheduleUpdate `yaml:"schedule,omitempty"`
	Ignore           []IgnoreUpdate `yaml:"ignore,omitempty"`

	TargetBranch string `yaml:"target-branch,omitempty"`

	CommitMessage CommitMessageUpdate `yaml:"commit-message,omitempty"`

	OpenPullRequestLimit int `yaml:"open-pull-requests-limit"`
}

type DependabotUpdates []DependabotUpdate

type ScheduleUpdate struct {
	Interval string `yaml:"interval,omitempty"`
}

type IgnoreUpdate struct {
	DependencyName string   `yaml:"dependency-name,omitempty"`
	Versions       []string `yaml:"versions,omitempty"`
	UpdateTypes    []string `yaml:"update-types,omitempty"`
}

type CommitMessageUpdate struct {
	Prefix string `yaml:"prefix,omitempty"`
}

func (cfg *DependabotConfig) WithGo(branch string) {
	u := DependabotUpdate{
		PackageEcosystem: "gomod",
		Directories:      []string{"/"},
		Schedule: ScheduleUpdate{
			Interval: "weekly",
		},
		TargetBranch: branch,
		CommitMessage: CommitMessageUpdate{
			Prefix: fmt.Sprintf("[%s][%s]", branch, "gomod"),
		},
		OpenPullRequestLimit: 10,
		Ignore: []IgnoreUpdate{
			{
				DependencyName: "knative.dev/*",
			},
			{
				DependencyName: "k8s.io/*",
				UpdateTypes:    []string{"version-update:semver-major", "version-update:semver-minor"},
			},
			{
				DependencyName: "github.com/openshift/*",
				UpdateTypes:    []string{"version-update:semver-major", "version-update:semver-minor"},
			},
			{
				DependencyName: "sigs.k8s.io/controller-runtime",
				UpdateTypes:    []string{"version-update:semver-major", "version-update:semver-minor"},
			},
		},
	}

	*cfg.Updates = append(*cfg.Updates, u)
}

func (cfg *DependabotConfig) WithMaven(dirs []string, branch string) {
	if len(dirs) == 0 {
		dirs = []string{"/"}
	}

	u := DependabotUpdate{
		PackageEcosystem: "maven",
		Directories:      dirs,
		Schedule: ScheduleUpdate{
			Interval: "weekly",
		},
		TargetBranch: branch,
		CommitMessage: CommitMessageUpdate{
			Prefix: fmt.Sprintf("[%s][%s]", branch, "maven"),
		},
		OpenPullRequestLimit: 10,
		Ignore: []IgnoreUpdate{
			{
				DependencyName: "io.quarkus*",
				UpdateTypes:    []string{"version-update:semver-major", "version-update:semver-minor"},
			},
			{
				DependencyName: "com.redhat.quarkus.platform*",
				UpdateTypes:    []string{"version-update:semver-major", "version-update:semver-minor"},
			},
			{
				DependencyName: "io.vertx*",
				UpdateTypes:    []string{"version-update:semver-major", "version-update:semver-minor"},
			},
		},
	}

	*cfg.Updates = append(*cfg.Updates, u)
}

func (cfg *DependabotConfig) Write(repoDir string, run string) error {
	log.Printf("Writing dependabot config %#v\n", *cfg)

	sort.SliceStable(*cfg.Updates, func(i, j int) bool {
		a := (*cfg.Updates)[i].TargetBranch
		b := (*cfg.Updates)[j].TargetBranch
		if a == b {
			return (*cfg.Updates)[i].PackageEcosystem < (*cfg.Updates)[j].PackageEcosystem
		}
		return a < b
	})

	out, err := yaml.Marshal(*cfg)
	if err != nil {
		return fmt.Errorf("failed to marshal dependabot config: %w", err)
	}

	const ghDir = ".github"
	const workflowsDir = "workflows"

	if err := os.MkdirAll(filepath.Join(repoDir, ghDir, workflowsDir), 0755); err != nil {
		return fmt.Errorf("failed to create .github directory: %w", err)
	}
	if err := os.WriteFile(filepath.Join(repoDir, ghDir, "dependabot.yml"), out, 0644); err != nil {
		return fmt.Errorf("failed to write dependabot config file: %w", err)
	}

	if run == "" {
		return nil
	}

	workflow := []byte(fmt.Sprintf(`
name: Dependabot

on:
  pull_request:

permissions:
  contents: write

jobs:
  update-deps:
    name: Update deps
    runs-on: ubuntu-latest
    if: ${{ github.actor == 'dependabot[bot]' }}
    steps:
      - name: Checkout
        uses: actions/checkout@v4
        with:
          ref: ${{ github.head_ref }}
          path: ./src/github.com/${{ github.repository }}
          fetch-depth: 0

      - name: Setup Golang
        uses: openshift-knative/hack/actions/setup-go@main

      - name: Install yq
        run: |
          go install github.com/mikefarah/yq/v3@latest

      - name: Generate files
        working-directory: ./src/github.com/${{ github.repository }}
        run: %s

      - name: git push
        working-directory: ./src/github.com/${{ github.repository }}
        run: |
          if ! git diff --exit-code --quiet
          then
            git config --local user.email "41898282+github-actions[bot]@users.noreply.github.com"
            git config --local user.name "github-actions[bot]"
            git add .
            git commit -m "Run generate release"
            git push
          fi
`, run))
	if err := os.WriteFile(filepath.Join(repoDir, ghDir, workflowsDir, "dependabot-deps.yaml"), workflow, 0644); err != nil {
		return fmt.Errorf("failed to write dependabot workflow file: %w", err)
	}

	return nil
}
