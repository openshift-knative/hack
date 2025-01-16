package action

import (
	"bytes"
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/openshift-knative/hack/pkg/prowgen"
)

type BumpRepoConfig struct {
	Repo          string `yaml:"repo"`
	Branch        string `yaml:"branch"`
	PostUpdateCmd string `yaml:"postUpdateCmd,omitempty"`
}

func GoModuleBumpAction(ctx context.Context, cfg Config) error {
	y, err := os.ReadFile(cfg.InputAction)
	if err != nil {
		return err
	}
	var node yaml.Node
	if err := yaml.NewDecoder(bytes.NewBuffer(y)).Decode(&node); err != nil {
		return fmt.Errorf("failed to decode file into node: %w", err)
	}

	if err := AddNestedField(&node, "Go Module bump", false, "name"); err != nil {
		return fmt.Errorf("failed to rename workflow: %w", err)
	}

	if err := AddNestedField(&node, "Go Module bump", false, "jobs", "go-mod-bump", "name"); err != nil {
		return fmt.Errorf("failed to rename workflow: %w", err)
	}

	var repoConfigs []interface{}
	err = filepath.Walk(cfg.InputConfigPath, func(path string, info fs.FileInfo, err error) error {
		if info.IsDir() || !strings.HasSuffix(path, ".yaml") {
			return nil
		}

		inConfig, err := prowgen.LoadConfig(path)
		if err != nil {
			return err
		}

		for _, repo := range inConfig.Repositories {
			for branchName := range inConfig.Config.Branches {

				if branchName == "release-next" {
					continue
				}

				repoConfig := BumpRepoConfig{
					Repo:          repo.Repo,
					Branch:        branchName,
					PostUpdateCmd: "make generated-files",
				}

				if repo.IsServerlessOperator() {
					repoConfig.PostUpdateCmd = "make generate-release"
				} else if repo.IsFunc() || repo.IsEventPlugin() {
					repoConfig.PostUpdateCmd = ""
				}

				repoConfigs = append(repoConfigs, repoConfig)
			}
		}

		return nil
	})
	if err != nil {
		return fmt.Errorf("failed to walk filesystem path %q: %w", cfg.InputConfigPath, err)
	}

	if err := AddNestedField(&node, repoConfigs, false, "jobs", "go-mod-bump", "strategy", "matrix", "include"); err != nil {
		return fmt.Errorf("failed to add repo config as matrix entry: %w", err)
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
