package konfluxapply

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/openshift-knative/hack/pkg/prowgen"
)

type ApplyConfig struct {
	InputConfigPath string

	KonfluxDir string // default: `.konflux`
}

func Apply(ctx context.Context, cfg ApplyConfig) error {
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
				return fmt.Errorf("[%s] failed to checkout branch %q: %w", r.RepositoryDirectory(), bn, err)
			}

			if _, err := os.Stat(filepath.Join(r.RepositoryDirectory(), cfg.KonfluxDir)); err != nil {
				if errors.Is(err, os.ErrNotExist) {
					continue // Skip repositories without Konflux components directory
				}
				return fmt.Errorf("[%s] failed to stat Konflux directory %q for branch %q: %w", r.RepositoryDirectory(), cfg.KonfluxDir, bn, err)
			}

			if _, err := prowgen.Run(ctx, r, "oc", "apply", "-Rf", cfg.KonfluxDir); err != nil {
				return fmt.Errorf("[%s] failed to apply branch %q: %w", r.RepositoryDirectory(), bn, err)
			}
		}
	}
	return nil
}
