package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/openshift-knative/hack/pkg/dockerfilegen"
	"github.com/openshift-knative/hack/pkg/util/errors"
	"github.com/spf13/pflag"
)

func main() {
	wd, err := os.Getwd()
	if err != nil {
		log.Fatal(err)
	}

	params := dockerfilegen.Params{}

	pflag.StringVar(&params.RootDir, "root-dir", wd, "Root directory to start scanning, default to current working directory")
	pflag.StringArrayVar(&params.Includes, "includes", dockerfilegen.DefaultIncludes, "File or directory regex to include")
	pflag.StringArrayVar(&params.Excludes, "excludes", dockerfilegen.DefaultExcludes, "File or directory regex to exclude")
	pflag.StringVar(&params.Generators, "generators", dockerfilegen.GenerateDockerfileOption, fmt.Sprintf("Generate something supported: %q", []string{dockerfilegen.GenerateDockerfileOption, dockerfilegen.GenerateMustGatherDockerfileOption}))
	pflag.StringVar(&params.DockerfilesDir, "dockerfile-dir", "ci-operator/knative-images", "Dockerfiles output directory for project images relative to output flag")
	pflag.StringVar(&params.DockerfilesBuildDir, "dockerfile-build-dir", "ci-operator/build-image", "Dockerfiles output directory for build image relative to output flag")
	pflag.StringVar(&params.DockerfilesSourceDir, "dockerfile-source-dir", "ci-operator/source-image", "Dockerfiles output directory for source image relative to output flag")
	pflag.StringVar(&params.DockerfilesTestDir, "dockerfile-test-dir", "ci-operator/knative-test-images", "Dockerfiles output directory for test images relative to output flag")
	pflag.StringVar(&params.Output, "output", "openshift", "Output directory")
	pflag.StringVar(&params.ProjectFilePath, "project-file", filepath.Join("openshift", "project.yaml"), "Project metadata file path")
	pflag.StringVar(&params.DockerfileImageBuilderFmt, "dockerfile-image-builder-fmt", dockerfilegen.BuilderImageFmt, "Dockerfile image builder format")
	pflag.StringVar(&params.AppFileFmt, "app-file-fmt", "/usr/bin/%s", "Target application binary path format")
	pflag.StringVar(&params.RegistryImageFmt, "registry-image-fmt", "registry.ci.openshift.org/openshift/%s:%s", "Container registry image format")
	pflag.StringArrayVar(&params.ImagesFromRepositories, "images-from", nil, "Additional image references to be pulled from other midstream repositories matching the tag in project.yaml")
	pflag.StringVar(&params.ImagesFromRepositoriesURLFmt, "images-from-url-format", "https://raw.githubusercontent.com/openshift-knative/%s/%s/openshift/images.yaml", "Additional images to be pulled from other midstream repositories matching the tag in project.yaml")
	pflag.StringArrayVar(&params.AdditionalPackages, "additional-packages", nil, "Additional packages to be installed in the image")
	pflag.StringArrayVar(&params.AdditionalBuildEnvVars, "additional-build-env", nil, "Additional env vars to be added to builder in the image")
	pflag.StringVar(&params.TemplateName, "template-name", dockerfilegen.DefaultDockerfileTemplateName, fmt.Sprintf("Dockerfile template name to use. Supported values are [%s, %s]", dockerfilegen.DefaultDockerfileTemplateName, dockerfilegen.FuncUtilDockerfileTemplateName))
	pflag.BoolVar(&params.RpmsLockFileEnabled, "generate-rpms-lock-file", false, "Enable the creation of the rpms.lock.yaml file")
	pflag.Parse()

	if err = dockerfilegen.GenerateDockerfiles(params); err != nil {
		log.Fatalf("🔥 Error: %+v\n", errors.Rewrap(err))
	}
}
