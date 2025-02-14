package dockerfilegen

import (
	"path"
)

type Params struct {
	RootDir                      string
	Includes                     []string
	Excludes                     []string
	Generators                   string
	Output                       string
	DockerfilesDir               string
	DockerfilesTestDir           string
	DockerfilesBuildDir          string
	DockerfilesSourceDir         string
	ProjectFilePath              string
	DockerfileImageBuilderFmt    string
	AppFileFmt                   string
	RegistryImageFmt             string
	ImagesFromRepositories       []string
	ImagesFromRepositoriesURLFmt string
	AdditionalPackages           []string
	AdditionalBuildEnvVars       []string
	TemplateName                 string
	RpmsLockFileEnabled          bool
	ScanImports                  bool
	ScanImportsSubPackages       []string
	ScanImportsTags              []string
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
		ScanImportsSubPackages:       []string{"hack"},
		ScanImportsTags:              []string{"tools"},
	}
}

// DefaultBuildEnvVars is default set of FIPS flags to be used per builds
func DefaultBuildEnvVars() []string {
	return []string{
		"ENV CGO_ENABLED=1",
		"ENV GOEXPERIMENT=strictfipsruntime",
	}
}
