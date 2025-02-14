package main

import (
	"fmt"
	"log"
	"os"

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
	defs := dockerfilegen.DefaultParams(wd)

	pflag.StringVar(&params.RootDir, "root-dir", defs.RootDir, "Root directory to start scanning, default to current working directory")
	pflag.StringArrayVar(&params.Includes, "includes", defs.Includes, "File or directory regex to include")
	pflag.StringArrayVar(&params.Excludes, "excludes", defs.Excludes, "File or directory regex to exclude")
	pflag.StringVar(&params.Generators, "generators", defs.Generators, fmt.Sprintf("Generate something supported: %q", []string{dockerfilegen.GenerateDockerfileOption, dockerfilegen.GenerateMustGatherDockerfileOption}))
	pflag.StringVar(&params.DockerfilesDir, "dockerfile-dir", defs.DockerfilesDir, "Dockerfiles output directory for project images relative to output flag")
	pflag.StringVar(&params.DockerfilesBuildDir, "dockerfile-build-dir", defs.DockerfilesBuildDir, "Dockerfiles output directory for build image relative to output flag")
	pflag.StringVar(&params.DockerfilesSourceDir, "dockerfile-source-dir", defs.DockerfilesSourceDir, "Dockerfiles output directory for source image relative to output flag")
	pflag.StringVar(&params.DockerfilesTestDir, "dockerfile-test-dir", defs.DockerfilesTestDir, "Dockerfiles output directory for test images relative to output flag")
	pflag.StringVar(&params.Output, "output", defs.Output, "Output directory")
	pflag.StringVar(&params.ProjectFilePath, "project-file", defs.ProjectFilePath, "Project metadata file path")
	pflag.StringVar(&params.DockerfileImageBuilderFmt, "dockerfile-image-builder-fmt", defs.DockerfileImageBuilderFmt, "Dockerfile image builder format")
	pflag.StringVar(&params.AppFileFmt, "app-file-fmt", defs.AppFileFmt, "Target application binary path format")
	pflag.StringVar(&params.RegistryImageFmt, "registry-image-fmt", defs.RegistryImageFmt, "Container registry image format")
	pflag.StringArrayVar(&params.ImagesFromRepositories, "images-from", defs.ImagesFromRepositories, "Additional image references to be pulled from other midstream repositories matching the tag in project.yaml")
	pflag.StringVar(&params.ImagesFromRepositoriesURLFmt, "images-from-url-format", defs.ImagesFromRepositoriesURLFmt, "Additional images to be pulled from other midstream repositories matching the tag in project.yaml")
	pflag.StringArrayVar(&params.AdditionalPackages, "additional-packages", defs.AdditionalPackages, "Additional packages to be installed in the image")
	pflag.StringArrayVar(&params.AdditionalBuildEnvVars, "additional-build-env", defs.AdditionalBuildEnvVars, "Additional env vars to be added to builder in the image")
	pflag.StringVar(&params.TemplateName, "template-name", defs.TemplateName, fmt.Sprintf("Dockerfile template name to use. Supported values are [%s, %s]", dockerfilegen.DefaultDockerfileTemplateName, dockerfilegen.FuncUtilDockerfileTemplateName))
	pflag.BoolVar(&params.RpmsLockFileEnabled, "generate-rpms-lock-file", defs.RpmsLockFileEnabled, "Enable the creation of the rpms.lock.yaml file")
	pflag.Parse()

	if err = dockerfilegen.GenerateDockerfiles(params); err != nil {
		log.Fatalf("🔥 Error: %+v\n", errors.Rewrap(err))
	}
}
