package prowgen

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	cioperatorapi "github.com/openshift/ci-tools/pkg/api"
)

type ImageMirroringConfig struct {
	Path    string
	Content string
	Release string

	Metadata cioperatorapi.Metadata
}

const (
	CIRegistry   = "registry.ci.openshift.org"
	QuayRegistry = "quay.io/openshift-knative"

	ImageMirroringConfigPath       = "core-services/image-mirroring/knative"
	ImageMirroringConfigFilePrefix = "mapping_knative"
)

func GenerateImageMirroringConfigs(openshiftRelease Repository, cfgs []ReleaseBuildConfiguration) []ImageMirroringConfig {
	mirroringConfigs := make([]ImageMirroringConfig, 0, 8)
	for _, cfg := range cfgs {
		if cfg.PromotionConfiguration != nil {
			lines := make([]string, 0, 8)
			release := ""
			for _, img := range cfg.Images {
				if cfg.PromotionConfiguration.Name != "" {
					from := fmt.Sprintf("%s/%s/%s:%s", CIRegistry, cfg.PromotionConfiguration.Namespace, cfg.PromotionConfiguration.Name, img.To)
					to := fmt.Sprintf("%s/%s:%s", QuayRegistry, img.To, cfg.PromotionConfiguration.Name)
					lines = append(lines, fmt.Sprintf("%s %s", from, to))
					release = cfg.PromotionConfiguration.Name
				} else if cfg.PromotionConfiguration.Tag != "" {
					from := fmt.Sprintf("%s/%s/%s:%s", CIRegistry, cfg.PromotionConfiguration.Namespace, img.To, cfg.PromotionConfiguration.Tag)
					to := fmt.Sprintf("%s/%s:%s", QuayRegistry, img.To, cfg.PromotionConfiguration.Tag)
					lines = append(lines, fmt.Sprintf("%s %s", from, to))
					release = cfg.PromotionConfiguration.Tag
				}
			}

			if len(lines) == 0 {
				continue
			}

			fileName := fmt.Sprintf("%s_%s_%s_quay", ImageMirroringConfigFilePrefix, release, cfg.Metadata.Repo)
			mirroringConfigs = append(mirroringConfigs, ImageMirroringConfig{
				Path:     filepath.Join(openshiftRelease.RepositoryDirectory(), ImageMirroringConfigPath, fileName),
				Content:  strings.Join(lines, "\n") + "\n",
				Release:  release,
				Metadata: cfg.Metadata,
			})
		}
	}
	return mirroringConfigs
}

func ReconcileImageMirroringConfig(mirroring ImageMirroringConfig) error {
	matching := filepath.Join(filepath.Dir(mirroring.Path), "*"+mirroring.Release+"_"+mirroring.Metadata.Repo+"*")
	existing, err := filepath.Glob(matching)
	if err != nil {
		return fmt.Errorf("failed to find files matching %s: %w", matching, err)
	}

	for _, f := range existing {
		if err := os.Remove(f); err != nil {
			return fmt.Errorf("failed to delete file %s: %w", f, err)
		}
	}

	if err := ioutil.WriteFile(mirroring.Path, []byte(mirroring.Content), os.ModePerm); err != nil {
		return fmt.Errorf("failed to write file %s: %w", mirroring.Path, err)
	}
	return nil
}
