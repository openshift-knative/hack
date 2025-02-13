package dockerfilegen

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
}

var (
	DefaultIncludes = []string{
		"test/test_images.*",
		"cmd.*",
	}
	DefaultExcludes = []string{
		".*k8s\\.io.*",
		".*knative.dev/pkg/codegen.*",
	}
	// DefaultBuildEnvVar is default set of FIPS flags to be used per builds
	DefaultBuildEnvVar = []string{
		"ENV CGO_ENABLED=1",
		"ENV GOEXPERIMENT=strictfipsruntime",
	}
)
