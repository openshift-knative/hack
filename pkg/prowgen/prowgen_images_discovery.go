package prowgen

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"

	cioperatorapi "github.com/openshift/ci-tools/pkg/api"
)

func DiscoverImages(r Repository) ReleaseBuildConfigurationOption {
	return func(cfg *cioperatorapi.ReleaseBuildConfiguration) error {
		log.Println(r.RepositoryDirectory(), "Discovering images")
		opts, err := discoverImages(r)
		if err != nil {
			return err
		}

		return applyOptions(cfg, opts...)
	}
}

func discoverImages(r Repository) ([]ReleaseBuildConfigurationOption, error) {
	dockerfiles, err := discoverDockerfiles(r)
	if err != nil {
		return nil, err
	}
	sort.Strings(dockerfiles)

	log.Println(r.RepositoryDirectory(), "Discovered Dockerfiles", dockerfiles)

	options := make([]ReleaseBuildConfigurationOption, 0, len(dockerfiles))

	for _, dockerfile := range dockerfiles {
		options = append(options,
			WithImage(ProjectDirectoryImageBuildStepConfigurationFuncFromImageInput(r, ImageInput{
				Context:        discoverImageContext(dockerfile),
				DockerfilePath: strings.Join(strings.Split(dockerfile, string(os.PathSeparator))[2:], string(os.PathSeparator)),
			})),
		)
	}

	return options, nil
}

func discoverImageContext(dockerfile string) imageContext {
	context := ProductionContext
	if strings.Contains(dockerfile, "test-images") {
		context = TestContext
	}
	return context
}

func discoverDockerfiles(r Repository) ([]string, error) {
	dir := filepath.Join(r.RepositoryDirectory(), "openshift", "ci-operator")
	dockerfiles, err := filepath.Glob(filepath.Join(dir, "**", "**", "Dockerfile"))
	if err != nil {
		return nil, fmt.Errorf("failed while discovering container images in %s: %w", dir, err)
	}
	return dockerfiles, nil
}
