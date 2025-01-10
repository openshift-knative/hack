package dependabotgen

import (
	"fmt"
	"log"
	"os"
	"path/filepath"

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

func (cfg *DependabotConfig) Write(repoDir string) error {
	log.Printf("Writing dependabot config %#v\n", *cfg)

	out, err := yaml.Marshal(*cfg)
	if err != nil {
		return fmt.Errorf("failed to marshal dependabot config: %w", err)
	}

	const ghDir = ".github"

	if err := os.MkdirAll(filepath.Join(repoDir, ghDir), 0755); err != nil {
		return fmt.Errorf("failed to create .github directory: %w", err)
	}
	if err := os.WriteFile(filepath.Join(repoDir, ghDir, "dependabot.yml"), out, 0644); err != nil {
		return fmt.Errorf("failed to write dependabot config file: %w", err)
	}

	return nil
}
