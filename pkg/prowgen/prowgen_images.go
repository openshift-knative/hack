package prowgen

import (
	"path/filepath"
	"strings"

	cioperatorapi "github.com/openshift/ci-tools/pkg/api"
)

type imageContext string

var (
	ProductionContext imageContext = ""
	TestContext       imageContext = "test"
)

type ImageInput struct {
	Context        imageContext
	DockerfilePath string
	Inputs         map[string]cioperatorapi.ImageBuildInputs
}

func ProjectDirectoryImageBuildStepConfigurationFuncFromImageInput(r Repository, input ImageInput) ProjectDirectoryImageBuildStepConfigurationFunc {
	return func() (cioperatorapi.ProjectDirectoryImageBuildStepConfiguration, error) {
		return cioperatorapi.ProjectDirectoryImageBuildStepConfiguration{
			To: cioperatorapi.PipelineImageStreamTagReference(toImage(r, input)),
			ProjectDirectoryImageBuildInputs: cioperatorapi.ProjectDirectoryImageBuildInputs{
				DockerfilePath: input.DockerfilePath,
				Inputs:         input.Inputs,
			},
		}, nil
	}
}

func toImage(r Repository, input ImageInput) string {
	context := ""
	if input.Context != "" {
		context = "-" + string(input.Context)
	}

	folderName := filepath.Base(filepath.Dir(input.DockerfilePath))
	if override, ok := r.ImageNameOverrides[folderName]; ok {
		folderName = override
	}

	to := r.ImagePrefix + context + "-" + folderName
	return strings.ReplaceAll(to, "_", "-")
}

func WithImage(ibcFunc ProjectDirectoryImageBuildStepConfigurationFunc) ReleaseBuildConfigurationOption {
	return func(cfg *cioperatorapi.ReleaseBuildConfiguration) error {
		ibc, err := ibcFunc()
		if err != nil {
			return err
		}
		cfg.Images = append(cfg.Images, ibc)
		return nil
	}
}

func WithBaseImages(baseImages map[string]cioperatorapi.ImageStreamTagReference) ReleaseBuildConfigurationOption {
	return func(cfg *cioperatorapi.ReleaseBuildConfiguration) error {
		if cfg.InputConfiguration.BaseImages == nil {
			cfg.InputConfiguration.BaseImages = make(map[string]cioperatorapi.ImageStreamTagReference)
		}

		for key, img := range baseImages {
			cfg.InputConfiguration.BaseImages[key] = img
		}

		return nil
	}
}
