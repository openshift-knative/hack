package dockerfilegen

import (
	"os"
	"path"

	"github.com/octago/sflags"
	"github.com/octago/sflags/gen/gpflag"
	"github.com/spf13/pflag"
)

type Params struct {
	RootDir                      string   `json:"root-dir" desc:"Root directory to start scanning, default to current working directory"`
	Includes                     []string `json:"includes" desc:"File or directory regex to include"`
	Excludes                     []string `json:"excludes" desc:"File or directory regex to exclude"`
	Generators                   string   `json:"generators" desc:"Generate something supported: [dockerfile, must-gather-dockerfile]"`
	Output                       string   `json:"output" desc:"Output directory"`
	DockerfilesDir               string   `json:"dockerfiles-dir" desc:"Dockerfiles output directory for project images relative to output flag"`
	DockerfilesTestDir           string   `json:"dockerfiles-test-dir" desc:"Dockerfiles output directory for test images relative to output flag"`
	DockerfilesBuildDir          string   `json:"dockerfiles-build-dir" desc:"Dockerfiles output directory for build image relative to output flag"`
	DockerfilesSourceDir         string   `json:"dockerfiles-source-dir" desc:"Dockerfiles output directory for source image relative to output flag"`
	ProjectFilePath              string   `json:"project-file" desc:"Project metadata file path"`
	DockerfileImageBuilderFmt    string   `json:"dockerfile-image-builder-fmt" desc:"Dockerfile image builder format"`
	AppFileFmt                   string   `json:"app-file-fmt" desc:"Target application binary path format"`
	RegistryImageFmt             string   `json:"registry-image-fmt" desc:"Container registry image format"`
	ImagesFromRepositories       []string `json:"images-from" desc:"Additional image references to be pulled from other midstream repositories matching the tag in project.yaml"`
	ImagesFromRepositoriesURLFmt string   `json:"images-from-url-format" desc:"Additional images to be pulled from other midstream repositories matching the tag in project.yaml"`
	AdditionalPackages           []string `json:"additional-packages" desc:"Additional packages to be installed in the image"`
	AdditionalBuildEnvVars       []string `json:"additional-build-env" desc:"Additional env vars to be added to builder in the image"`
	TemplateName                 string   `json:"template-name" desc:"Dockerfile template name to use. Supported values are [default, func-util]"`
	RpmsLockFileEnabled          bool     `json:"generate-rpms-lock-file" desc:"Enable the creation of the rpms.lock.yaml file"`
}

func (p *Params) ConfigureFlags() (*pflag.FlagSet, error) {
	fs := pflag.NewFlagSet(os.Args[0], pflag.ExitOnError)
	fs.SortFlags = false
	if err := gpflag.ParseTo(p, fs, sflags.FlagTag("json")); err != nil {
		return nil, err
	}
	return fs, nil
}

func DefaultParams(wd string) Params {
	return Params{
		RootDir: wd,
		Includes: []string{
			"test/test_images.*",
			"cmd.*",
		},
		Excludes: []string{
			".*k8s\\.io.*",
			".*knative.dev/pkg/codegen.*",
		},
		Generators:                   GenerateDockerfileOption,
		Output:                       "openshift",
		DockerfilesDir:               path.Join("ci-operator", "knative-images"),
		DockerfilesTestDir:           path.Join("ci-operator", "knative-test-images"),
		DockerfilesBuildDir:          path.Join("ci-operator", "build-image"),
		DockerfilesSourceDir:         path.Join("ci-operator", "source-image"),
		ProjectFilePath:              path.Join("openshift", "project.yaml"),
		DockerfileImageBuilderFmt:    BuilderImageFmt,
		AppFileFmt:                   "/usr/bin/%s",
		RegistryImageFmt:             "registry.ci.openshift.org/openshift/%s:%s",
		ImagesFromRepositories:       nil,
		ImagesFromRepositoriesURLFmt: "https://raw.githubusercontent.com/openshift-knative/%s/%s/openshift/images.yaml",
		AdditionalPackages:           nil,
		AdditionalBuildEnvVars:       nil,
		TemplateName:                 DefaultDockerfileTemplateName,
		RpmsLockFileEnabled:          false,
	}
}

// DefaultBuildEnvVars is default set of FIPS flags to be used per builds
func DefaultBuildEnvVars() []string {
	return []string{
		"ENV CGO_ENABLED=1",
		"ENV GOEXPERIMENT=strictfipsruntime",
	}
}
