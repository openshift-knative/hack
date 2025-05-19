package konfluxgen

import (
	"bytes"
	"context"
	"crypto/md5"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"text/template"

	"github.com/blang/semver/v4"
	gosemver "github.com/coreos/go-semver/semver"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/tools/clientcmd"

	"github.com/openshift-knative/hack/pkg/k8sresource"
	"github.com/openshift-knative/hack/pkg/soversion"
	"github.com/openshift-knative/hack/pkg/util"

	gyaml "github.com/ghodss/yaml"
	cioperatorapi "github.com/openshift/ci-tools/pkg/api"
	operatorsv1alpha1 "github.com/operator-framework/api/pkg/operators/v1alpha1"
)

const (
	ApplicationsDirectoryName          = "applications"
	ReleasePlanAdmissionsDirectoryName = "releaseplanadmissions"
	ReleasePlansDirName                = "releaseplans"
	ReleasesDirName                    = "releases"

	StageEnv = "stage"
	ProdEnv  = "prod"

	prodRegistryHost  = "registry.redhat.io"
	stageRegistryHost = "registry.stage.redhat.io"
	registryRepoName  = "openshift-serverless-1"
	prodRegistry      = prodRegistryHost + "/" + registryRepoName
	stageRegistry     = stageRegistryHost + "/" + registryRepoName
)

//go:embed application.template.yaml
var ApplicationTemplate embed.FS

//go:embed dockerfile-component.template.yaml
var DockerfileComponentTemplate embed.FS

//go:embed imagerepository.template.yaml
var ImageRepositoryTemplate embed.FS

//go:embed pipeline-run.template.yaml
var PipelineRunTemplate embed.FS

//go:embed docker-build.yaml
var PipelineDockerBuildTemplate embed.FS

//go:embed docker-java-build.yaml
var PipelineDockerJavaBuildTemplate embed.FS

//go:embed fbc-builder.yaml
var PipelineFBCBuildTemplate embed.FS

//go:embed bundle-build.yaml
var PipelineBundleBuildTemplate embed.FS

//go:embed integration-test-scenario.template.yaml
var EnterpriseContractTestScenarioTemplate embed.FS

//go:embed releaseplanadmission-component.template.yaml
var ComponentReleasePlanAdmissionsTemplate embed.FS

//go:embed releaseplanadmission-fbc.template.yaml
var FBCReleasePlanAdmissionsTemplate embed.FS

//go:embed releaseplan.template.yaml
var ReleasePlanTemplate embed.FS

//go:embed release.template.yaml
var ReleaseTemplate embed.FS

type Config struct {
	OpenShiftReleasePath string
	ApplicationName      string
	BuildArgs            []string
	ComponentNameFunc    func(cfg cioperatorapi.ReleaseBuildConfiguration, ib cioperatorapi.ProjectDirectoryImageBuildStepConfiguration) string

	Includes       []string
	Excludes       []string
	ExcludesImages []string

	FBCImages   []string
	JavaImages  []string
	BundleImage string

	ResourcesOutputPathSkipRemove bool
	ResourcesOutputPath           string
	RepositoryRootPath            string
	GlobalResourcesOutputPath     string

	PipelinesOutputPathSkipRemove bool
	PipelinesOutputPath           string

	AdditionalTektonCELExpressionFunc func(cfg cioperatorapi.ReleaseBuildConfiguration, ib cioperatorapi.ProjectDirectoryImageBuildStepConfiguration) string
	NudgesFunc                        func(cfg cioperatorapi.ReleaseBuildConfiguration, ib cioperatorapi.ProjectDirectoryImageBuildStepConfiguration) []string
	Nudges                            []string

	PipelineRunAnnotationsFunc func(cfg cioperatorapi.ReleaseBuildConfiguration, ib cioperatorapi.ProjectDirectoryImageBuildStepConfiguration) map[string]string

	// Preserve the version tag as first tag in any instance since SO, when bumping the patch version
	// will change it before merging the PR.
	// See `openshift-knative/serverless-operator/hack/generate/update-pipelines.sh` for more details.
	Tags []string

	PrefetchDeps PrefetchDeps

	IsHermetic func(cfg cioperatorapi.ReleaseBuildConfiguration, ib cioperatorapi.ProjectDirectoryImageBuildStepConfiguration) bool

	ComponentReleasePlanConfig *ComponentReleasePlanConfig
	AdditionalComponentConfigs []TemplateConfig
}

type PrefetchDeps struct {
	DevPackageManagers string
	/*
		Example: '[{ "type": "rpm" }, {"type": "gomod", "path": "."}]'
	*/
	PrefetchInput string
}

type ComponentReleasePlanConfig struct {
	FirstRelease              *gosemver.Version
	ClusterServiceVersionPath string
	BundleComponentName       string
	BundleImageRepoName       string
}

func (pd *PrefetchDeps) WithRPMs() {
	types := make([]map[string]interface{}, 0)
	if pd.PrefetchInput != "" {
		if err := json.Unmarshal([]byte(pd.PrefetchInput), &types); err != nil {
			log.Fatal("Failed to unmarshal prefetch input: ", err)
		}
	}
	types = append(types, map[string]interface{}{
		"type": "rpm",
	})
	b, err := json.Marshal(types)
	if err != nil {
		log.Fatal("Failed to marshal prefetch input: ", err)
	}
	pd.PrefetchInput = string(b)
}

func (pd *PrefetchDeps) WithUnvendoredGo(path string) {
	types := make([]map[string]interface{}, 0)
	if pd.PrefetchInput != "" {
		if err := json.Unmarshal([]byte(pd.PrefetchInput), &types); err != nil {
			log.Fatal("Failed to unmarshal prefetch input: ", err)
		}
	}
	types = append(types, map[string]interface{}{
		"type": "gomod",
		"path": path,
	})
	b, err := json.Marshal(types)
	if err != nil {
		log.Fatal("Failed to marshal prefetch input: ", err)
	}
	pd.PrefetchInput = string(b)
}

func (pd *PrefetchDeps) WithNPM(path string) {
	types := make([]map[string]interface{}, 0)
	if pd.PrefetchInput != "" {
		if err := json.Unmarshal([]byte(pd.PrefetchInput), &types); err != nil {
			log.Fatal("Failed to unmarshal prefetch input: ", err)
		}
	}
	types = append(types, map[string]interface{}{
		"type": "npm",
		"path": path,
	})
	b, err := json.Marshal(types)
	if err != nil {
		log.Fatal("Failed to marshal prefetch input: ", err)
	}
	pd.PrefetchInput = string(b)
}

func Generate(cfg Config) error {
	fbcBuildPipelinePath := filepath.Join(cfg.PipelinesOutputPath, "fbc-builder.yaml")
	bundleBuildPipelinePath := filepath.Join(cfg.PipelinesOutputPath, "bundle-build.yaml")
	containerBuildPipelinePath := filepath.Join(cfg.PipelinesOutputPath, "docker-build.yaml")
	containerJavaBuildPipelinePath := filepath.Join(cfg.PipelinesOutputPath, "docker-java-build.yaml")

	if cfg.ComponentNameFunc == nil {
		cfg.ComponentNameFunc = func(cfg cioperatorapi.ReleaseBuildConfiguration, ib cioperatorapi.ProjectDirectoryImageBuildStepConfiguration) string {
			return fmt.Sprintf("%s-%s", ib.To, cfg.Metadata.Branch)
		}
	}
	if cfg.AdditionalTektonCELExpressionFunc == nil {
		cfg.AdditionalTektonCELExpressionFunc = func(cfg cioperatorapi.ReleaseBuildConfiguration, ib cioperatorapi.ProjectDirectoryImageBuildStepConfiguration) string {
			return ""
		}
	}
	if cfg.PipelineRunAnnotationsFunc == nil {
		cfg.PipelineRunAnnotationsFunc = func(cfg cioperatorapi.ReleaseBuildConfiguration, ib cioperatorapi.ProjectDirectoryImageBuildStepConfiguration) map[string]string {
			return nil
		}
	}

	if cfg.NudgesFunc == nil {
		cfg.NudgesFunc = func(cfg cioperatorapi.ReleaseBuildConfiguration, ib cioperatorapi.ProjectDirectoryImageBuildStepConfiguration) []string {
			return []string{}
		}
	}
	if cfg.IsHermetic == nil {
		cfg.IsHermetic = defaultIsHermetic
	}

	if !cfg.ResourcesOutputPathSkipRemove {
		if err := removeAllExcept(cfg.ResourcesOutputPath, filepath.Join(cfg.ResourcesOutputPath, ReleasesDirName)); err != nil {
			return fmt.Errorf("failed to clean %q directory: %w", cfg.ResourcesOutputPath, err)
		}
	}

	if !cfg.PipelinesOutputPathSkipRemove {
		imageDigestMirrorSetPath := filepath.Join(cfg.PipelinesOutputPath, "images-mirror-set.yaml")
		if err := removeAllExcept(cfg.PipelinesOutputPath, fbcBuildPipelinePath, bundleBuildPipelinePath, containerBuildPipelinePath, containerJavaBuildPipelinePath, imageDigestMirrorSetPath); err != nil {
			return fmt.Errorf("failed to clean %q directory: %w", cfg.PipelinesOutputPath, err)
		}
	}

	includes, err := util.ToRegexp(cfg.Includes)
	if err != nil {
		return fmt.Errorf("failed to create regular expressions for %+v: %w", cfg.Includes, err)
	}
	excludes, err := util.ToRegexp(cfg.Excludes)
	if err != nil {
		return fmt.Errorf("failed to create regular expressions for %+v: %w", cfg.Excludes, err)
	}
	excludeImages, err := util.ToRegexp(cfg.ExcludesImages)
	if err != nil {
		return fmt.Errorf("failed to create regular expressions for %+v: %w", cfg.ExcludesImages, err)
	}
	fbcImages, err := util.ToRegexp(cfg.FBCImages)
	if err != nil {
		return fmt.Errorf("failed to create regular expressions for %+v: %w", cfg.FBCImages, err)
	}
	javaImages, err := util.ToRegexp(cfg.JavaImages)
	if err != nil {
		return fmt.Errorf("failed to create regular expressions for %+v: %w", cfg.JavaImages, err)
	}

	var bundleImage *regexp.Regexp
	if cfg.BundleImage != "" {
		bundleImage, err = regexp.Compile(cfg.BundleImage)
		if err != nil {
			return fmt.Errorf("failed to create regular expressions for %+v: %w", cfg.BundleImage, err)
		}
	}

	configs, err := collectConfigurations(cfg.OpenShiftReleasePath, includes, excludes, cfg.AdditionalComponentConfigs)
	if err != nil {
		return err
	}

	log.Printf("Found %d configs", len(configs))

	funcs := template.FuncMap{
		"sanitize": Sanitize,
		"truncate": Truncate,
		"replace":  replace,
	}

	applicationTemplate, err := template.New("application.template.yaml").Funcs(funcs).ParseFS(ApplicationTemplate, "*.yaml")
	if err != nil {
		return fmt.Errorf("failed to parse application template: %w", err)
	}
	dockerfileComponentTemplate, err := template.New("dockerfile-component.template.yaml").Funcs(funcs).ParseFS(DockerfileComponentTemplate, "*.yaml")
	if err != nil {
		return fmt.Errorf("failed to parse dockerfile component template: %w", err)
	}
	imageRepositoryTemplate, err := template.New("imagerepository.template.yaml").Funcs(funcs).ParseFS(ImageRepositoryTemplate, "*.yaml")
	if err != nil {
		return fmt.Errorf("failed to parse dockerfile component template: %w", err)
	}
	pipelineRunTemplate, err := template.New("pipeline-run.template.yaml").Delims("{{{", "}}}").Funcs(funcs).ParseFS(PipelineRunTemplate, "*.yaml")
	if err != nil {
		return fmt.Errorf("failed to parse pipeline run push template: %w", err)
	}
	pipelineDockerBuildTemplate, err := template.New("docker-build.yaml").Delims("{{{", "}}}").Funcs(funcs).ParseFS(PipelineDockerBuildTemplate, "*.yaml")
	if err != nil {
		return fmt.Errorf("failed to parse pipeline run push template: %w", err)
	}
	pipelineDockerJavaBuildTemplate, err := template.New("docker-java-build.yaml").Delims("{{{", "}}}").Funcs(funcs).ParseFS(PipelineDockerJavaBuildTemplate, "*.yaml")
	if err != nil {
		return fmt.Errorf("failed to parse pipeline run push template: %w", err)
	}
	pipelineFBCBuildTemplate, err := template.New("fbc-builder.yaml").Delims("{{{", "}}}").Funcs(funcs).ParseFS(PipelineFBCBuildTemplate, "*.yaml")
	if err != nil {
		return fmt.Errorf("failed to parse pipeline run push template: %w", err)
	}
	pipelineBundleBuildTemplate, err := template.New("bundle-build.yaml").Delims("{{{", "}}}").Funcs(funcs).ParseFS(PipelineBundleBuildTemplate, "*.yaml")
	if err != nil {
		return fmt.Errorf("failed to parse pipeline run bundle push template: %w", err)
	}
	enterpriseContractTestScenarioTemplate, err := template.New("integration-test-scenario.template.yaml").Delims("{{{", "}}}").Funcs(funcs).ParseFS(EnterpriseContractTestScenarioTemplate, "*.yaml")
	if err != nil {
		return fmt.Errorf("failed to parse integration test scenario template: %w", err)
	}
	applications := make(map[string]map[string]DockerfileApplicationConfig, 8)
	for _, c := range configs {
		appKey := Truncate(Sanitize(cfg.ApplicationName))
		if _, ok := applications[appKey]; !ok {
			applications[appKey] = make(map[string]DockerfileApplicationConfig, 8)
		}
		if (c.PromotionConfiguration == nil || len(c.PromotionConfiguration.Targets) == 0) && !c.IsContained(cfg.AdditionalComponentConfigs) {
			continue
		}
		for _, ib := range c.Images {

			ignore := false
			for _, r := range excludeImages {
				if r.MatchString(string(ib.To)) {
					ignore = true
					break
				}
			}
			if ignore {
				continue
			}
			pipeline := DockerBuild
			for _, r := range fbcImages {
				if r.MatchString(string(ib.To)) {
					pipeline = FBCBuild
					break
				}
			}

			if bundleImage != nil && bundleImage.MatchString(string(ib.To)) {
				pipeline = BundleBuild
			}

			dockerfilePath := ib.ProjectDirectoryImageBuildInputs.DockerfilePath
			for _, r := range javaImages {
				if r.MatchString(string(ib.To)) {
					pipeline = DockerJavaBuild
					dockerfilePath = filepath.Dir(ib.ProjectDirectoryImageBuildInputs.DockerfilePath)
					dockerfileName := filepath.Base(ib.ProjectDirectoryImageBuildInputs.DockerfilePath)
					dockerfilePath = filepath.Join(dockerfilePath, "hermetic", dockerfileName)
					break
				}
			}

			r := DockerfileApplicationConfig{
				ApplicationName:           cfg.ApplicationName,
				ComponentName:             Truncate(Sanitize(cfg.ComponentNameFunc(c.ReleaseBuildConfiguration, ib))),
				ReleaseBuildConfiguration: c.ReleaseBuildConfiguration,
				Path:                      c.Path,
				ProjectDirectoryImageBuildStepConfiguration: ib,
				Nudges:                        append(cfg.Nudges, cfg.NudgesFunc(c.ReleaseBuildConfiguration, ib)...),
				Pipeline:                      pipeline,
				AdditionalTektonCELExpression: cfg.AdditionalTektonCELExpressionFunc(c.ReleaseBuildConfiguration, ib),
				// Do not "prepend" tags as SO relies on the order.
				Tags:           append(cfg.Tags, "latest"),
				BuildArgs:      cfg.BuildArgs,
				PrefetchDeps:   cfg.PrefetchDeps,
				DockerfilePath: dockerfilePath,

				PipelineRunAnnotations: cfg.PipelineRunAnnotationsFunc(c.ReleaseBuildConfiguration, ib),
			}

			if cfg.IsHermetic(c.ReleaseBuildConfiguration, ib) {
				r.Hermetic = "true"
			} else {
				r.Hermetic = "false"
			}

			applications[appKey][Truncate(Sanitize(cfg.ComponentNameFunc(c.ReleaseBuildConfiguration, ib)))] = r
		}
	}

	if err := os.MkdirAll(filepath.Dir(containerBuildPipelinePath), 0777); err != nil {
		return fmt.Errorf("failed to create directory for %q: %w", containerBuildPipelinePath, err)
	}

	for appKey, components := range applications {

		for componentKey, config := range components {
			buf := &bytes.Buffer{}

			appPath := filepath.Join(cfg.GlobalResourcesOutputPath, ApplicationsDirectoryName, appKey, fmt.Sprintf("%s.yaml", appKey))
			if err := os.MkdirAll(filepath.Dir(appPath), 0777); err != nil {
				return fmt.Errorf("failed to create directory for %q: %w", appPath, err)
			}

			if err := applicationTemplate.Execute(buf, config); err != nil {
				return fmt.Errorf("failed to execute template for application %q: %w", appKey, err)
			}
			if err := WriteFileReplacingNewerTaskImages(appPath, buf.Bytes(), 0777); err != nil {
				return fmt.Errorf("failed to write application file %q: %w", appPath, err)
			}

			buf.Reset()

			componentPath := filepath.Join(cfg.ResourcesOutputPath, ApplicationsDirectoryName, appKey, "components", fmt.Sprintf("%s.yaml", componentKey))
			if err := os.MkdirAll(filepath.Dir(componentPath), 0777); err != nil {
				return fmt.Errorf("failed to create directory for %q: %w", componentPath, err)
			}

			if err := dockerfileComponentTemplate.Execute(buf, config); err != nil {
				return fmt.Errorf("failed to execute template for component %q: %w", componentKey, err)
			}
			if err := WriteFileReplacingNewerTaskImages(componentPath, buf.Bytes(), 0777); err != nil {
				return fmt.Errorf("failed to write component file %q: %w", componentPath, err)
			}

			buf.Reset()

			imageRepositoryPath := filepath.Join(cfg.ResourcesOutputPath, ApplicationsDirectoryName, appKey, "components", "imagerepositories", fmt.Sprintf("%s.yaml", componentKey))
			if err := os.MkdirAll(filepath.Dir(imageRepositoryPath), 0777); err != nil {
				return fmt.Errorf("failed to create directory for %q: %w", imageRepositoryPath, err)
			}

			if err := imageRepositoryTemplate.Execute(buf, config); err != nil {
				return fmt.Errorf("failed to execute template for imagerepository for %q: %w", componentKey, err)
			}
			if err := WriteFileReplacingNewerTaskImages(imageRepositoryPath, buf.Bytes(), 0777); err != nil {
				return fmt.Errorf("failed to write imagerepository file %q: %w", imageRepositoryPath, err)
			}

			buf.Reset()

			pipelineRunPRPath := filepath.Join(cfg.PipelinesOutputPath, fmt.Sprintf("%s-pull-request.yaml", componentKey))
			pipelineRunPushPath := filepath.Join(cfg.PipelinesOutputPath, fmt.Sprintf("%s-push.yaml", componentKey))

			config.Event = PullRequestEvent
			if err := pipelineRunTemplate.Execute(buf, config); err != nil {
				return fmt.Errorf("failed to execute template for pipeline run PR %q: %w", pipelineRunPRPath, err)
			}
			if err := WriteFileReplacingNewerTaskImages(pipelineRunPRPath, buf.Bytes(), 0777); err != nil {
				return fmt.Errorf("failed to write component file %q: %w", pipelineRunPRPath, err)
			}

			buf.Reset()

			config.Event = PushEvent
			if err := pipelineRunTemplate.Execute(buf, config); err != nil {
				return fmt.Errorf("failed to execute template for pipeline run PR %q: %w", pipelineRunPushPath, err)
			}
			if err := WriteFileReplacingNewerTaskImages(pipelineRunPushPath, buf.Bytes(), 0777); err != nil {
				return fmt.Errorf("failed to write component file %q: %w", pipelineRunPushPath, err)
			}

			buf.Reset()

			if config.Pipeline == FBCBuild {

				if err := pipelineFBCBuildTemplate.Execute(buf, nil); err != nil {
					return fmt.Errorf("failed to execute template for pipeline %q: %w", fbcBuildPipelinePath, err)
				}
				if err := WriteFileReplacingNewerTaskImages(fbcBuildPipelinePath, buf.Bytes(), 0777); err != nil {
					return fmt.Errorf("failed to write FBC build pipeline file %q: %w", fbcBuildPipelinePath, err)
				}

			}

			buf.Reset()

			if config.Pipeline == BundleBuild {

				if err := pipelineBundleBuildTemplate.Execute(buf, nil); err != nil {
					return fmt.Errorf("failed to execute template for pipeline %q: %w", bundleBuildPipelinePath, err)
				}
				if err := WriteFileReplacingNewerTaskImages(bundleBuildPipelinePath, buf.Bytes(), 0777); err != nil {
					return fmt.Errorf("failed to write bundle build pipeline file %q: %w", bundleBuildPipelinePath, err)
				}

			}

			buf.Reset()

			if config.Pipeline == DockerJavaBuild {

				if err := pipelineDockerJavaBuildTemplate.Execute(buf, nil); err != nil {
					return fmt.Errorf("failed to execute template for pipeline %q: %w", containerJavaBuildPipelinePath, err)
				}
				if err := WriteFileReplacingNewerTaskImages(containerJavaBuildPipelinePath, buf.Bytes(), 0777); err != nil {
					return fmt.Errorf("failed to write FBC build pipeline file %q: %w", containerJavaBuildPipelinePath, err)
				}

			}
		}

		buf := &bytes.Buffer{}

		// add default integration test scenario with the stage policies
		config := IntegrationTestConfig{
			Name:            fmt.Sprintf("%s-ec", appKey),
			ApplicationName: cfg.ApplicationName,
			Contexts: []IntegrationTestContext{{
				Name:        "push",
				Description: "Application testing",
			}},
		}

		if len(cfg.FBCImages) > 0 {
			config.ECPolicyConfiguration = "rhtap-releng-tenant/fbc-ocp-serverless-stage"
		} else {
			config.ECPolicyConfiguration = "rhtap-releng-tenant/registry-standard-stage"
		}

		ecTestDir := filepath.Join(cfg.GlobalResourcesOutputPath, ApplicationsDirectoryName, appKey, "tests")
		if err := os.MkdirAll(ecTestDir, 0777); err != nil {
			return fmt.Errorf("failed to create directory for %q: %w", appKey, err)
		}

		if err := enterpriseContractTestScenarioTemplate.Execute(buf, config); err != nil {
			return fmt.Errorf("failed to execute template for EC test: %w", err)
		}
		if err := WriteFileReplacingNewerTaskImages(filepath.Join(ecTestDir, "ec-test.yaml"), buf.Bytes(), 0777); err != nil {
			return fmt.Errorf("failed to write application file: %w", err)
		}

		buf.Reset()

		// add integration test scenario for override snapshots with prod policies
		config = IntegrationTestConfig{
			Name:            fmt.Sprintf("%s-ec-override-snapshot", appKey),
			ApplicationName: cfg.ApplicationName,
			Contexts: []IntegrationTestContext{{
				Name:        "override",
				Description: "Override Snapshot testing",
			}},
		}

		if len(cfg.FBCImages) > 0 {
			config.ECPolicyConfiguration = "rhtap-releng-tenant/fbc-ocp-serverless-prod"
		} else {
			config.ECPolicyConfiguration = "rhtap-releng-tenant/registry-ocp-serverless-prod"
		}

		if err := os.MkdirAll(ecTestDir, 0777); err != nil {
			return fmt.Errorf("failed to create directory for %q: %w", appKey, err)
		}

		if err := enterpriseContractTestScenarioTemplate.Execute(buf, config); err != nil {
			return fmt.Errorf("failed to execute template for EC test: %w", err)
		}
		if err := WriteFileReplacingNewerTaskImages(filepath.Join(ecTestDir, "override-snapshot-ec-test.yaml"), buf.Bytes(), 0777); err != nil {
			return fmt.Errorf("failed to write application file: %w", err)
		}

		buf.Reset()
	}

	buf := &bytes.Buffer{}
	if err := pipelineDockerBuildTemplate.Execute(buf, cfg); err != nil {
		return fmt.Errorf("failed to execute template for pipeline run PR %q: %w", containerBuildPipelinePath, err)
	}
	if err := WriteFileReplacingNewerTaskImages(containerBuildPipelinePath, buf.Bytes(), 0777); err != nil {
		return fmt.Errorf("failed to write component file %q: %w", containerBuildPipelinePath, err)
	}

	buf.Reset()

	if cfg.ComponentReleasePlanConfig != nil {
		csv, err := loadClusterServiceVerion(cfg.ComponentReleasePlanConfig.ClusterServiceVersionPath)
		if err != nil {
			return fmt.Errorf("failed to load ClusterServiceVersion: %w", err)
		}

		if err := GenerateComponentReleasePlanAdmission(cfg, csv); err != nil {
			return fmt.Errorf("failed to generate ReleasePlanAdmission: %w", err)
		}

		if err = GenerateComponentsReleasePlans(
			cfg.ResourcesOutputPath,
			cfg.ApplicationName,
			consistentVersion(cfg, csv).String(),
			/* In this case RPA.name == RP.name */ ReleasePlanAdmissionName,
			ReleasePlanAdmissionName,
		); err != nil {
			return fmt.Errorf("failed to generate ReleasePlan: %w", err)
		}
	}

	return nil
}

func collectConfigurations(openshiftReleasePath string, includes []*regexp.Regexp, excludes []*regexp.Regexp, additionalConfigs []TemplateConfig) ([]TemplateConfig, error) {
	configs := make([]TemplateConfig, 0, 8)
	err := filepath.WalkDir(openshiftReleasePath, func(path string, info fs.DirEntry, err error) error {
		if info.IsDir() {
			return nil
		}

		matchablePath, err := filepath.Rel(openshiftReleasePath, path)
		if err != nil {
			return fmt.Errorf("failed to get relative path for %q (base path %q): %w", matchablePath, openshiftReleasePath, err)
		}

		shouldInclude := false
		for _, i := range includes {
			if i.MatchString(matchablePath) {
				shouldInclude = true
			}
			for _, x := range excludes {
				if x.MatchString(matchablePath) {
					shouldInclude = false
				}
			}
		}
		if !shouldInclude {
			return nil
		}

		log.Printf("Parsing file %q\n", path)

		c, err := parseConfig(path)
		if err != nil {
			return fmt.Errorf("failed to parse CI config in %q: %w", path, err)
		}

		configs = append(configs, TemplateConfig{
			ReleaseBuildConfiguration: *c,
			Path:                      path,
		})

		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("failed while walking directory %q: %w\n", openshiftReleasePath, err)
	}

	return append(configs, additionalConfigs...), nil
}

type TemplateConfig struct {
	cioperatorapi.ReleaseBuildConfiguration
	Path string
}

func (c TemplateConfig) IsContained(configs []TemplateConfig) bool {
	for _, config := range configs {
		if reflect.DeepEqual(config, c) {
			return true
		}
	}

	return false
}

type DockerfileApplicationConfig struct {
	ApplicationName                             string
	ComponentName                               string
	ReleaseBuildConfiguration                   cioperatorapi.ReleaseBuildConfiguration
	Path                                        string
	ProjectDirectoryImageBuildStepConfiguration cioperatorapi.ProjectDirectoryImageBuildStepConfiguration
	Nudges                                      []string

	AdditionalTektonCELExpression string
	Event                         PipelineEvent
	Pipeline                      Pipeline

	Tags      []string
	BuildArgs []string

	PrefetchDeps PrefetchDeps

	Hermetic string

	DockerfilePath string

	PipelineRunAnnotations map[string]string
}

type IntegrationTestConfig struct {
	Name                  string
	ApplicationName       string
	ECPolicyConfiguration string
	Contexts              []IntegrationTestContext
}

type IntegrationTestContext struct {
	Name        string
	Description string
}

type PipelineEvent string

const (
	PushEvent        PipelineEvent = "push"
	PullRequestEvent PipelineEvent = "pull_request"
)

type Pipeline string

const (
	DockerBuild     Pipeline = "docker-build"
	DockerJavaBuild Pipeline = "docker-java-build"
	FBCBuild        Pipeline = "fbc-builder"
	BundleBuild     Pipeline = "bundle-build"
)

func parseConfig(path string) (*cioperatorapi.ReleaseBuildConfiguration, error) {
	// Going directly from YAML raw input produces unexpected configs (due to missing YAML tags),
	// so we convert YAML to JSON and unmarshal the struct from the JSON object.
	y, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read file %q: %w", path, err)
	}
	j, err := gyaml.YAMLToJSON(y)
	if err != nil {
		return nil, fmt.Errorf("failed to convert YAML to JSON: %w", err)
	}

	jobConfig := &cioperatorapi.ReleaseBuildConfiguration{}
	if err := json.Unmarshal(j, jobConfig); err != nil {
		return nil, fmt.Errorf("failed to unmarshal file %q into %T: %w", path, jobConfig, err)
	}

	return jobConfig, err
}

func Sanitize(input interface{}) string {
	in := fmt.Sprintf("%s", input)
	// TODO very basic name sanitizer
	return strings.ToLower(strings.ReplaceAll(strings.ReplaceAll(in, ".", ""), " ", "-"))
}

func Truncate(input interface{}) string {
	in := fmt.Sprintf("%s", input)
	in = strings.ReplaceAll(in, "release-v", "")
	in = strings.ReplaceAll(in, "release-", "")
	in = strings.ReplaceAll(in, "knative-", "kn-")
	in = strings.ReplaceAll(in, "eventing-kafka-broker-", "ekb-")
	if strings.HasPrefix(in, "kn-kn-") {
		// avoid kn-kn-plugin-xyz names
		in = strings.TrimPrefix(in, "kn-")
	}
	return Name(in, "")
}

func replace(input interface{}, old, new string) string {
	in := fmt.Sprintf("%s", input)
	return strings.ReplaceAll(in, old, new)
}

func removeAllExcept(dir string, excludedFiles ...string) error {
	if !fileExists(dir) {
		return nil
	}

	return filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if path == dir && len(excludedFiles) > 0 {
			return nil
		}

		for _, excludedFile := range excludedFiles {
			if path == excludedFile {
				// skip deletion
				if d.IsDir() {
					// skip whole directory, so no need to check on child files too
					return filepath.SkipDir
				}
				return nil
			}
		}

		if err := os.RemoveAll(path); err != nil {
			return err
		}

		if d.IsDir() {
			// we deleted the directory, so no need to check on child files too
			return filepath.SkipDir
		}

		return nil
	})
}

const (
	longest = 63
	md5Len  = 32
	head    = longest - md5Len // How much to truncate to fit the hash.
)

// Name generates a name for the resource based upon the parent resource and suffix.
// If the concatenated name is longer than K8s permits the name is hashed and truncated to permit
// construction of the resource, but still keeps it unique.
// If the suffix itself is longer than 31 characters, then the whole string will be hashed
// and `parent|hash|suffix` will be returned, where parent and suffix will be trimmed to
// fit (prefix of parent at most of length 31, and prefix of suffix at most length 30).
func Name(parent, suffix string) string {
	n := parent
	if len(parent) > (longest - len(suffix)) {
		// If the suffix is longer than the longest allowed suffix, then
		// we hash the whole combined string and use that as the suffix.
		if head-len(suffix) <= 0 {
			//nolint:gosec // No strong cryptography needed.
			h := md5.Sum([]byte(parent + suffix))
			// 1. trim parent, if needed
			if head < len(parent) {
				parent = parent[:head]
			}
			// Format the return string, if it's shorter than longest: pad with
			// beginning of the suffix. This happens, for example, when parent is
			// short, but the suffix is very long.
			ret := parent + fmt.Sprintf("%x", h)
			if d := longest - len(ret); d > 0 {
				ret += suffix[:d]
			}
			return makeValidName(ret)
		}
		//nolint:gosec // No strong cryptography needed.
		n = fmt.Sprintf("%s%x", parent[:head-len(suffix)], md5.Sum([]byte(parent)))
	}
	return n + suffix
}

var isAlphanumeric = regexp.MustCompile(`^[a-zA-Z0-9]*$`)

// If due to trimming above we're terminating the string with a non-alphanumeric
// character, remove it.
func makeValidName(n string) string {
	for i := len(n) - 1; !isAlphanumeric.MatchString(string(n[i])); i-- {
		n = n[:len(n)-1]
	}
	return n
}

func WriteFileReplacingNewerTaskImages(name string, data []byte, perm os.FileMode) error {
	if !fileExists(name) {
		return os.WriteFile(name, data, perm)
	}

	existingBytes, err := os.ReadFile(name)
	if err != nil {
		return fmt.Errorf("failed to read file %q: %w", name, err)
	}

	data = replaceTaskImagesFromExisting(existingBytes, data)
	return os.WriteFile(name, data, perm)
}

func replaceTaskImagesFromExisting(existingBytes, newBytes []byte) []byte {

	dataStr := string(newBytes)

	const pattern = "%s:%s@%s:%s"

	existingRegex := regexp.MustCompile(`.*(quay.io/konflux-ci.*):(.*)@(.*):(.*)`)
	existingMatches := existingRegex.FindAllStringSubmatch(string(existingBytes), -1)
	for _, existingMatch := range existingMatches {
		existingImg := existingMatch[1]
		existingVersion := existingMatch[2]
		existingS := existingMatch[3]
		existingSha := existingMatch[4]

		log.Printf("Found new image "+pattern, existingImg, existingVersion, existingS, existingSha)

		newRegex := regexp.MustCompile(fmt.Sprintf(`.*(%s):(.*)@(.*):(.*)`, existingImg))
		newMatches := newRegex.FindAllStringSubmatch(dataStr, -1)
		for _, match := range newMatches {
			img := match[1]
			v := match[2]
			s := match[3]
			sha := match[4]

			log.Printf("Replacing "+pattern+" with "+pattern, img, v, s, sha, existingImg, existingVersion, existingS, existingSha)

			dataStr = strings.ReplaceAll(dataStr, fmt.Sprintf(pattern, img, v, s, sha), fmt.Sprintf(pattern, img, existingVersion, existingS, existingSha))
		}
	}

	return []byte(dataStr)
}

func defaultIsHermetic(_ cioperatorapi.ReleaseBuildConfiguration, _ cioperatorapi.ProjectDirectoryImageBuildStepConfiguration) bool {
	return true
}

type rpaBaseData struct {
	Name string

	SOVersion      semver.Version
	Policy         string
	PipelineSA     string
	SignCMName     string
	SignSecretName string
	Intention      string
}

type rpaFBCData struct {
	rpaBaseData

	Applications          []string
	FromIndex             string
	TargetIndex           string
	PublishingCredentials string
	StagedIndex           bool
}

type rpaComponentData struct {
	rpaBaseData

	ApplicationName string
	Components      []ComponentImageRepoRef
	PyxisSecret     string
	PyxisServer     string
	AtlasServer     string
}

func GenerateFBCReleasePlanAdmission(applications []string, resourceOutputPath string, appName string, soVersion string) error {
	outputDir := filepath.Join(resourceOutputPath, ReleasePlanAdmissionsDirectoryName)
	if err := os.MkdirAll(outputDir, 0777); err != nil {
		return fmt.Errorf("failed to create release plan admissions directory: %w", err)
	}

	semv, err := semver.New(soVersion)
	if err != nil {
		return fmt.Errorf("failed to parse SO version %q: %w", soVersion, err)
	}

	rpaName := FBCReleasePlanAdmissionName(appName, soVersion, ProdEnv)
	fbcData := rpaFBCData{
		rpaBaseData: rpaBaseData{
			Name:           rpaName,
			SOVersion:      *semv,
			Policy:         "fbc-ocp-serverless-prod",
			PipelineSA:     "release-index-image-prod",
			SignCMName:     "hacbs-signing-pipeline-config-redhatrelease2",
			SignSecretName: "konflux-cosign-signing-production",
			Intention:      "production",
		},
		Applications:          applications,
		FromIndex:             "registry-proxy.engineering.redhat.com/rh-osbs/iib-pub:{{ OCP_VERSION }}",
		TargetIndex:           "quay.io/redhat-prod/redhat----redhat-operator-index:{{ OCP_VERSION }}",
		PublishingCredentials: "fbc-production-publishing-credentials-redhat-prod",
	}
	outputFilePath := filepath.Join(outputDir, fmt.Sprintf("%s.yaml", rpaName))
	if err := executeFBCReleasePlanAdmissionTemplate(fbcData, outputFilePath); err != nil {
		return fmt.Errorf("failed to execute release plan admission template: %w", err)
	}

	// generate RPA for stage
	rpaName = FBCReleasePlanAdmissionName(appName, soVersion, StageEnv)
	fbcData = rpaFBCData{
		rpaBaseData: rpaBaseData{
			Name:           rpaName,
			SOVersion:      *semv,
			Policy:         "fbc-ocp-serverless-stage",
			PipelineSA:     "release-index-image-staging",
			SignCMName:     "hacbs-signing-pipeline-config-staging-redhatrelease2",
			SignSecretName: "konflux-cosign-signing-stage",
			Intention:      "staging",
		},
		Applications:          applications,
		FromIndex:             "registry-proxy.engineering.redhat.com/rh-osbs/iib-pub-pending:{{ OCP_VERSION }}",
		TargetIndex:           "",
		PublishingCredentials: "staged-index-fbc-publishing-credentials",
		StagedIndex:           true,
	}
	outputFilePath = filepath.Join(outputDir, fmt.Sprintf("%s.yaml", rpaName))
	if err := executeFBCReleasePlanAdmissionTemplate(fbcData, outputFilePath); err != nil {
		return fmt.Errorf("failed to execute release plan admission template: %w", err)
	}

	return nil
}

func GenerateComponentReleasePlanAdmission(cfg Config, csv *operatorsv1alpha1.ClusterServiceVersion) error {
	soVersion := consistentVersion(cfg, csv)

	outputDir := filepath.Join(cfg.ResourcesOutputPath, ReleasePlanAdmissionsDirectoryName)
	if err := os.MkdirAll(outputDir, 0777); err != nil {
		return fmt.Errorf("failed to create release plan admissions directory: %w", err)
	}

	components, err := getComponentImageRefs(csv)
	if err != nil {
		return fmt.Errorf("failed to get component image refs: %w", err)
	}

	// append bundle component, as this is not part of the CSV
	components = append(components, ComponentImageRepoRef{
		ComponentName:   fmt.Sprintf("%s-%d%d", cfg.ComponentReleasePlanConfig.BundleComponentName, soVersion.Major, soVersion.Minor),
		ImageRepository: fmt.Sprintf("%s/%s", prodRegistry, cfg.ComponentReleasePlanConfig.BundleImageRepoName),
	})

	rpaName := ReleasePlanAdmissionName(cfg.ApplicationName, soVersion.String(), ProdEnv)
	rpaData := rpaComponentData{
		rpaBaseData: rpaBaseData{
			Name:           rpaName,
			SOVersion:      soVersion,
			PipelineSA:     "release-registry-prod",
			SignCMName:     "hacbs-signing-pipeline-config-redhatrelease2",
			SignSecretName: "konflux-cosign-signing-production",
			Policy:         "registry-ocp-serverless-prod",
			Intention:      "production",
		},
		ApplicationName: cfg.ApplicationName,
		Components:      components,
		PyxisSecret:     "pyxis-prod-secret",
		PyxisServer:     "production",
		AtlasServer:     "production",
	}
	outputFilePath := filepath.Join(outputDir, fmt.Sprintf("%s.yaml", rpaName))
	if err := executeComponentReleasePlanAdmissionTemplate(rpaData, outputFilePath); err != nil {
		return fmt.Errorf("failed to execute release plan admission template: %w", err)
	}

	// create RPA for stage ENV
	componentWithStageRepoRef := make([]ComponentImageRepoRef, 0, len(components))
	for _, component := range components {
		componentWithStageRepoRef = append(componentWithStageRepoRef, ComponentImageRepoRef{
			ComponentName:   component.ComponentName,
			ImageRepository: strings.ReplaceAll(component.ImageRepository, prodRegistryHost, stageRegistryHost),
		})
	}

	rpaName = ReleasePlanAdmissionName(cfg.ApplicationName, soVersion.String(), StageEnv)
	rpaData = rpaComponentData{
		rpaBaseData: rpaBaseData{
			Name:           rpaName,
			SOVersion:      soVersion,
			PipelineSA:     "release-registry-staging",
			SignCMName:     "hacbs-signing-pipeline-config-staging-redhatrelease2",
			SignSecretName: "konflux-cosign-signing-stage",
			Policy:         "registry-ocp-serverless-stage",
			Intention:      "staging",
		},
		ApplicationName: cfg.ApplicationName,
		Components:      componentWithStageRepoRef,
		PyxisSecret:     "pyxis-staging-secret",
		PyxisServer:     "stage",
		AtlasServer:     "stage",
	}
	outputFilePath = filepath.Join(outputDir, fmt.Sprintf("%s.yaml", rpaName))
	if err := executeComponentReleasePlanAdmissionTemplate(rpaData, outputFilePath); err != nil {
		return fmt.Errorf("failed to execute release plan admission template: %w", err)
	}

	return nil
}

func executeFBCReleasePlanAdmissionTemplate(data rpaFBCData, outputFilePath string) error {
	funcs := template.FuncMap{
		"sanitize": Sanitize,
		"truncate": Truncate,
		"replace":  replace,
	}

	rpaTemplate, err := template.New("releaseplanadmission-fbc.template.yaml").Delims("{{{", "}}}").Funcs(funcs).ParseFS(FBCReleasePlanAdmissionsTemplate, "*.yaml")
	if err != nil {
		return fmt.Errorf("failed to parse FBC RPA template: %w", err)
	}

	buf := &bytes.Buffer{}
	if err := rpaTemplate.Execute(buf, data); err != nil {
		return fmt.Errorf("failed to execute template for ReleasePlanAdmission: %w", err)
	}
	if err := os.WriteFile(outputFilePath, buf.Bytes(), 0777); err != nil {
		return fmt.Errorf("failed to write ReleasPlanAdmission file %q: %w", outputFilePath, err)
	}

	return nil
}

func executeComponentReleasePlanAdmissionTemplate(data rpaComponentData, outputFilePath string) error {
	funcs := template.FuncMap{
		"sanitize": Sanitize,
		"truncate": Truncate,
		"replace":  replace,
	}

	rpaTemplate, err := template.New("releaseplanadmission-component.template.yaml").Delims("{{{", "}}}").Funcs(funcs).ParseFS(ComponentReleasePlanAdmissionsTemplate, "*.yaml")
	if err != nil {
		return fmt.Errorf("failed to parse component RPA template: %w", err)
	}

	buf := &bytes.Buffer{}
	if err := rpaTemplate.Execute(buf, data); err != nil {
		return fmt.Errorf("failed to execute template for ReleasePlanAdmission: %w", err)
	}
	if err := os.WriteFile(outputFilePath, buf.Bytes(), 0777); err != nil {
		return fmt.Errorf("failed to write ReleasPlanAdmission file %q: %w", outputFilePath, err)
	}

	return nil
}

type releasePlanNameFunc func(appName string, soVersion string, env string) string

func (f releasePlanNameFunc) forceAppName(appName string) releasePlanNameFunc {
	return func(_ string, soVersion string, env string) string {
		return f(appName, soVersion, env)
	}
}

func ReleasePlanAdmissionName(appName, soVersion, env string) string {
	return Truncate(Sanitize(fmt.Sprintf("%s-%s-%s", appName, soVersion, env)))
}

func FBCReleasePlanAdmissionName(appName, soVersion, env string) string {
	return Truncate(Sanitize(fmt.Sprintf("%s-%s-fbc-%s", appName, soVersion, env)))
}

type ReleasePlan struct {
	Name                     string
	ApplicationName          string
	AutoRelease              string
	ReleasePlanAdmissionName string
	SOVersion                string
}

func GenerateComponentsReleasePlans(resourceOutputPath string, appName string, soVersion string, planNameFunc releasePlanNameFunc, rpaNameFunc releasePlanNameFunc) error {
	outputDir := filepath.Join(resourceOutputPath, ReleasePlansDirName)
	if err := os.MkdirAll(outputDir, 0777); err != nil {
		return fmt.Errorf("failed to create release plan directory: %w", err)
	}

	prod := ReleasePlan{
		Name:                     planNameFunc(appName, soVersion, ProdEnv),
		ApplicationName:          appName,
		AutoRelease:              "false", // Never ever flip this to true for prod.
		ReleasePlanAdmissionName: rpaNameFunc(appName, soVersion, ProdEnv),
		SOVersion:                soVersion,
	}
	if err := executeReleasePlanTemplate(prod, filepath.Join(outputDir, fmt.Sprintf("%s.yaml", prod.Name))); err != nil {
		return fmt.Errorf("failed to execute release plan template: %w", err)
	}

	stage := ReleasePlan{
		Name:                     planNameFunc(appName, soVersion, StageEnv),
		ApplicationName:          appName,
		AutoRelease:              "true", // Auto release for stage
		ReleasePlanAdmissionName: rpaNameFunc(appName, soVersion, StageEnv),
		SOVersion:                soVersion,
	}
	if err := executeReleasePlanTemplate(stage, filepath.Join(outputDir, fmt.Sprintf("%s.yaml", stage.Name))); err != nil {
		return fmt.Errorf("failed to execute release plan template: %w", err)
	}

	return nil
}

func GenerateReleasePlans(applications []string, resourceOutputPath string, appName string, soVersion string) error {
	for _, app := range applications {

		// There is only a single RPA for all FBC applications and the name uses `appName` vs the FBC-specific application name.
		var rpaNameFunc releasePlanNameFunc = FBCReleasePlanAdmissionName
		rpaNameFunc = rpaNameFunc.forceAppName(appName)

		if err := GenerateComponentsReleasePlans(resourceOutputPath, app, soVersion, ReleasePlanAdmissionName, rpaNameFunc); err != nil {
			return fmt.Errorf("failed to generate release plan for %q: %w", app, err)
		}
	}
	return nil
}

func executeReleasePlanTemplate(data ReleasePlan, outputFilePath string) error {
	funcs := template.FuncMap{
		"sanitize": Sanitize,
		"truncate": Truncate,
	}

	tpl, err := template.New("releaseplan.template.yaml").Delims("{{{", "}}}").Funcs(funcs).ParseFS(ReleasePlanTemplate, "*.yaml")
	if err != nil {
		return fmt.Errorf("failed to parse ReleasePlan template: %w", err)
	}

	buf := &bytes.Buffer{}
	if err := tpl.Execute(buf, data); err != nil {
		return fmt.Errorf("failed to execute template for ReleasePlan: %w", err)
	}
	if err := os.WriteFile(outputFilePath, buf.Bytes(), 0777); err != nil {
		return fmt.Errorf("failed to write ReleasPlan file %q: %w", outputFilePath, err)
	}
	return nil
}

type ComponentImageRepoRef struct {
	ComponentName   string
	ImageRepository string
}

func getComponentImageRefs(csv *operatorsv1alpha1.ClusterServiceVersion) ([]ComponentImageRepoRef, error) {
	var refs []ComponentImageRepoRef

	soVersion := csv.Spec.Version.Version
	var rhelRe = regexp.MustCompile(`(.*)-rhel\d+(.*)`)

	componentVersion := soversion.ToUpstreamVersion(soVersion.String())
	addedComponents := make(map[string]interface{})
	for _, relatedImage := range csv.Spec.RelatedImages {
		if !strings.HasPrefix(relatedImage.Image, prodRegistry) {
			continue
		}

		repoRef, _, _ := strings.Cut(relatedImage.Image, "@sha")
		componentName := strings.TrimPrefix(repoRef, prodRegistry+"/")
		// remove -rhelXYZ from component name
		componentName = rhelRe.ReplaceAllString(componentName, "${1}${2}")

		if strings.HasPrefix(componentName, "serverless-") {
			// SO component image
			componentName = fmt.Sprintf("%s-%d%d", componentName, soVersion.Major, soVersion.Minor)
		} else {
			// upstream component image
			componentName = fmt.Sprintf("%s-%d%d", componentName, componentVersion.Major, componentVersion.Minor)
		}

		if _, ok := addedComponents[componentName]; !ok {
			refs = append(refs, ComponentImageRepoRef{
				ComponentName:   componentName,
				ImageRepository: repoRef,
			})

			addedComponents[componentName] = nil
		}
	}

	return refs, nil
}

func loadClusterServiceVerion(path string) (*operatorsv1alpha1.ClusterServiceVersion, error) {
	// Going directly from YAML raw input produces unexpected configs (due to missing YAML tags),
	// so we convert YAML to JSON and unmarshal the struct from the JSON object.
	y, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	j, err := gyaml.YAMLToJSON(y)
	if err != nil {
		return nil, err
	}
	csv := &operatorsv1alpha1.ClusterServiceVersion{}
	if err := json.Unmarshal(j, csv); err != nil {
		return nil, fmt.Errorf("failed to unmarshall CSV: %w", err)
	}
	return csv, nil
}

func consistentVersion(cfg Config, csv *operatorsv1alpha1.ClusterServiceVersion) semver.Version {
	version := csv.Spec.Version.Version
	if csv.Spec.Version.Major != uint64(cfg.ComponentReleasePlanConfig.FirstRelease.Major) || csv.Spec.Version.Minor != uint64(cfg.ComponentReleasePlanConfig.FirstRelease.Minor) {
		// Use the first release if the csv doesn't match major + minor
		version = semver.Version{
			Major: uint64(cfg.ComponentReleasePlanConfig.FirstRelease.Major),
			Minor: uint64(cfg.ComponentReleasePlanConfig.FirstRelease.Minor),
			Patch: uint64(cfg.ComponentReleasePlanConfig.FirstRelease.Patch),
		}
	}
	return version
}

type ReleaseConfig struct {
	Snapshot            string
	ReleasePlan         string
	Environment         string
	ResourcesOutputPath string
}

type Release struct {
	Name        string
	Snapshot    string
	ReleasePlan string
}

func GenerateRelease(ctx context.Context, cfg ReleaseConfig) error {
	fullOutputPath := filepath.Join(cfg.ResourcesOutputPath, ReleasesDirName)
	// make sure output path exists
	if err := os.MkdirAll(fullOutputPath, os.ModePerm); err != nil {
		return fmt.Errorf("failed to create directory %q: %w", fullOutputPath, err)
	}

	releaseFile := filepath.Join(fullOutputPath, fmt.Sprintf("%s.yaml", cfg.ReleasePlan))

	relName, err := releaseName(ctx, releaseFile, cfg)
	if err != nil {
		return fmt.Errorf("failed to get release name for %q: %w", cfg.ReleasePlan, err)
	}

	data := Release{
		Name:        relName,
		Snapshot:    cfg.Snapshot,
		ReleasePlan: cfg.ReleasePlan,
	}

	if err := executeReleaseTemplate(data, releaseFile); err != nil {
		return fmt.Errorf("failed to execute release template: %w", err)
	}

	return nil
}

func releaseName(ctx context.Context, releaseFile string, cfg ReleaseConfig) (string, error) {
	/*
		if [ no existing release file ]: return RPA name
		else
			get release name from file
			if [ release with name exists in cluster ]
				if [ release is for same snapshot & same releaseplan]
					if [ release failed ]
						return unused release name to give another try
					else
						// release either succeeded or still running
						return same release name
				else
					// release was for another/older snapshot
					return unused release name
			else
				// release not applied yet
				return release name from file
	*/

	if !fileExists(releaseFile) {
		// we don't have a release file yet -> simply use the default (RPA) name
		return cfg.ReleasePlan, nil
	}

	releaseFromFile, err := k8sresource.KonfluxReleaseFromFile(releaseFile)
	if err != nil {
		return "", fmt.Errorf("failed to get release from file %q: %w", releaseFile, err)
	}

	// check state of release in konflux cluster
	releaseInCluster, err := loadReleaseFromCluster(ctx, releaseFromFile.Name)
	if err != nil {
		if apierrors.IsNotFound(err) {
			// release from file is not applied yet in cluster -> use same name
			return releaseFromFile.Name, nil
		} else {
			return "", fmt.Errorf("failed to get release from cluster %q: %w", releaseFromFile.Name, err)
		}
	}

	// release exists in cluster
	if releaseInCluster.Spec.Snapshot == releaseFromFile.Spec.Snapshot &&
		releaseInCluster.Spec.ReleasePlan == releaseFromFile.Spec.ReleasePlan {

		// release in cluster is for same snapshot & RP -> check status
		for _, condition := range releaseInCluster.Status.Conditions {
			if condition.Type == "Released" {
				if condition.Reason == "Failed" {
					// release failed -> use another name
					nextRN, err := nextReleaseName(releaseFromFile.Name, cfg.ReleasePlan)
					if err != nil {
						return "", fmt.Errorf("failed to get next release name from release %q: %w", releaseFromFile.Name, err)
					}
					return nextRN, nil
				} else {
					// release succeeded or is still running -> no need to update name (we could also skip CR generation/update)
					return releaseFromFile.Name, nil
				}
			}
		}

		return "", fmt.Errorf("could not find 'Released' condition from release %s", releaseFromFile.Name)
	}

	// release in cluster is for another snapshot & RP -> use another name
	nextRN, err := nextReleaseName(releaseFromFile.Name, cfg.ReleasePlan)
	if err != nil {
		return "", fmt.Errorf("failed to get next release name from release %q: %w", releaseFromFile.Name, err)
	}
	return nextRN, nil
}

func nextReleaseName(currentReleaseName string, releasePlanName string) (string, error) {
	/*
		release name is either:
		<releasePlanName> or <releasePlanName>-<counter>
	*/
	if currentReleaseName == releasePlanName {
		return fmt.Sprintf("%s-2", releasePlanName), nil
	} else if strings.HasPrefix(currentReleaseName, fmt.Sprintf("%s-", releasePlanName)) {
		strCounter := strings.TrimPrefix(currentReleaseName, fmt.Sprintf("%s-", releasePlanName))
		counter, err := strconv.Atoi(strCounter)
		if err != nil {
			return "", fmt.Errorf("could not parse suffix %q of %q to number", strCounter, currentReleaseName)
		}

		return fmt.Sprintf("%s-%d", releasePlanName, counter+1), nil
	} else {
		return "", fmt.Errorf("last release name %q does not match pattern '%s-<counter>'", currentReleaseName, releasePlanName)
	}
}

func loadReleaseFromCluster(ctx context.Context, name string) (*k8sresource.KonfluxRelease, error) {
	kubeconfigPath, found := os.LookupEnv("KUBECONFIG")
	if !found || kubeconfigPath == "" {
		return nil, fmt.Errorf("KUBECONFIG environment variable not set")
	}

	restConfig, err := clientcmd.BuildConfigFromFlags("", kubeconfigPath)
	if err != nil {
		return nil, fmt.Errorf("failed to build config from KUBECONFIG: %w", err)
	}

	clientConfig, err := clientcmd.LoadFromFile(kubeconfigPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load config from KUBECONFIG: %w", err)
	}

	currentContext, ok := clientConfig.Contexts[clientConfig.CurrentContext]
	if !ok {
		return nil, fmt.Errorf("failed to find current context in KUBECONFIG")
	}
	namespace := currentContext.Namespace

	dynamicClient, err := dynamic.NewForConfig(restConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to build dynamic client: %w", err)
	}

	releaseObj := k8sresource.KonfluxRelease{}
	us, err := dynamicClient.Resource(k8sresource.KonfluxReleaseGVR).Namespace(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		if apierrors.IsNotFound(err) {
			return nil, fmt.Errorf("release %s not found: %w", name, err)
		}
		return nil, fmt.Errorf("failed to get release %s: %w", name, err)
	}

	if err = runtime.DefaultUnstructuredConverter.FromUnstructured(us.Object, &releaseObj); err != nil {
		return nil, fmt.Errorf("failed to decode release %s: %w", name, err)
	}

	return &releaseObj, nil
}

func executeReleaseTemplate(data Release, outputFilePath string) error {
	tpl, err := template.New("release.template.yaml").Delims("{{{", "}}}").ParseFS(ReleaseTemplate, "*.yaml")
	if err != nil {
		return fmt.Errorf("failed to parse Release template: %w", err)
	}

	buf := &bytes.Buffer{}
	if err := tpl.Execute(buf, data); err != nil {
		return fmt.Errorf("failed to execute template for Release: %w", err)
	}
	if err := os.WriteFile(outputFilePath, buf.Bytes(), 0777); err != nil {
		return fmt.Errorf("failed to write Release file %q: %w", outputFilePath, err)
	}

	return nil
}

func fileExists(path string) bool {
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		return false
	}
	return true
}

func AppName(soBranch string) string {
	return fmt.Sprintf("serverless-operator %s", soBranch)
}

func FBCAppName(soBranch, ocpVersion string) string {
	return fmt.Sprintf("serverless-operator %s FBC %s", soBranch, ocpVersion)
}
