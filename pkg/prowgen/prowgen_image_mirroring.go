package prowgen

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	cioperatorapi "github.com/openshift/ci-tools/pkg/api"
	"k8s.io/apimachinery/pkg/util/sets"
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
			lines := sets.NewString()
			release := ""
			for _, img := range cfg.Images {
				if cfg.PromotionConfiguration.Name != "" {
					from := fmt.Sprintf("%s/%s/%s", CIRegistry, cfg.PromotionConfiguration.Namespace, cfg.PromotionConfiguration.Name)
					to := fmt.Sprintf("%s/%s", QuayRegistry, img.To)
					lines.Insert(fmt.Sprintf("%s %s", from, to))
					release = cfg.PromotionConfiguration.Name
				} else if cfg.PromotionConfiguration.Tag != "" {
					from := fmt.Sprintf("%s/%s/%s", CIRegistry, cfg.PromotionConfiguration.Namespace, img.To)
					to := fmt.Sprintf("%s/%s", QuayRegistry, img.To)
					lines.Insert(fmt.Sprintf("%s %s", from, to))
					release = cfg.PromotionConfiguration.Tag
				}
			}

			if lines.Len() == 0 {
				continue
			}

			fileName := fmt.Sprintf("%s_%s_%s_quay", ImageMirroringConfigFilePrefix, release, cfg.Metadata.Repo)
			mirroringConfigs = append(mirroringConfigs, ImageMirroringConfig{
				Path:     filepath.Join(openshiftRelease.RepositoryDirectory(), ImageMirroringConfigPath, fileName),
				Content:  strings.Join(lines.List(), "\n") + "\n",
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

	if err := os.WriteFile(mirroring.Path, []byte(mirroring.Content), os.ModePerm); err != nil {
		return fmt.Errorf("failed to write file %s: %w", mirroring.Path, err)
	}
	return nil
}
