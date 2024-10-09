package konfluxapply

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/openshift-knative/hack/pkg/prowgen"
)

type ApplyConfig struct {
	InputConfigPath string
	ExcludePatterns []*regexp.Regexp
	KonfluxDir      string // default: `.konflux`
}

func Apply(ctx context.Context, cfg ApplyConfig) error {
	hack := &prowgen.Config{
		Repositories: []prowgen.Repository{
			{
				Org:  "openshift-knative",
				Repo: "hack",
			},
		},
		Config: prowgen.CommonConfig{
			Branches: map[string]prowgen.Branch{
				"main": {
					Konflux: &prowgen.Konflux{Enabled: true},
				},
			},
		},
	}
	if err := apply(ctx, cfg, hack); err != nil {
		return fmt.Errorf("failed to apply konflux for hack repo: %w", err)
	}

	err := filepath.Walk(cfg.InputConfigPath, func(path string, info fs.FileInfo, err error) error {
		if info.IsDir() {
			return nil
		}

		inConfig, err := prowgen.LoadConfig(path)
		if err != nil {
			return err
		}

		if err := apply(ctx, cfg, inConfig); err != nil {
			return fmt.Errorf("failed to apply config: %v", err)
		}

		return nil
	})
	if err != nil {
		return fmt.Errorf("failed to walk filesystem path %q: %w", cfg.InputConfigPath, err)
	}

	return nil
}

func apply(ctx context.Context, cfg ApplyConfig, config *prowgen.Config) error {
	for _, r := range config.Repositories {
		if err := prowgen.GitMirror(ctx, r); err != nil {
			return fmt.Errorf("failed to mirror repository %q: %w", r.RepositoryDirectory(), err)
		}

		for bn, b := range config.Config.Branches {
			if b.Konflux == nil || !b.Konflux.Enabled {
				continue
			}

			if err := prowgen.GitCheckout(ctx, r, bn); err != nil {
				if !strings.Contains(err.Error(), "failed to run git [checkout") {
					return fmt.Errorf("[%s] failed to checkout branch %q: %w", r.RepositoryDirectory(), bn, err)
				}

				// Skip non-existing branches
				log.Println(r.RepositoryDirectory(), "Skipping non existing branch", bn)
				continue
			}

			repoKonfluxDir := filepath.Join(r.RepositoryDirectory(), cfg.KonfluxDir)
			if _, err := os.Stat(repoKonfluxDir); err != nil {
				if errors.Is(err, os.ErrNotExist) {
					continue // Skip repositories without Konflux components directory
				}
				return fmt.Errorf("[%s] failed to stat Konflux directory %q for branch %q: %w", r.RepositoryDirectory(), cfg.KonfluxDir, bn, err)
			}

			err := filepath.WalkDir(repoKonfluxDir, func(path string, d fs.DirEntry, err error) error {
				if err != nil {
					return fmt.Errorf("failed to walk directory %q: %w", path, err)
				}

				if d.IsDir() {
					return nil
				}

				for _, exclude := range cfg.ExcludePatterns {
					if exclude.MatchString(path) {
						log.Printf("skipping excluded file %q\n", path)
						return nil
					}
				}

				inRepoPath := strings.TrimPrefix(path, r.RepositoryDirectory()+"/")
				if _, err := prowgen.Run(ctx, r, "oc", "apply", "-f", inRepoPath); err != nil {
					return fmt.Errorf("failed to apply konflux manifest %q: %w", path, err)
				}

				return nil
			})

			if err != nil {
				return fmt.Errorf("[%s] failed to apply branch %q: %w", r.RepositoryDirectory(), bn, err)
			}
		}
	}
	return nil
}
