package konfluxgen

import (
	"bytes"
	"crypto/md5"
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"text/template"

	"github.com/operator-framework/api/pkg/lib/version"

	"github.com/openshift-knative/hack/pkg/soversion"
	"github.com/openshift-knative/hack/pkg/util"

	gyaml "github.com/ghodss/yaml"
	cioperatorapi "github.com/openshift/ci-tools/pkg/api"
	operatorsv1alpha1 "github.com/operator-framework/api/pkg/operators/v1alpha1"
)

const (
	ApplicationsDirectoryName = "applications"
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

//go:embed integration-test-scenario.template.yaml
var EnterpriseContractTestScenarioTemplate embed.FS

//go:embed releaseplanadmission.template.yaml
var ReleasePlanAdmissionsTemplate embed.FS

type Config struct {
	OpenShiftReleasePath string
	ApplicationName      string
	BuildArgs            []string
	ComponentNameFunc    func(cfg cioperatorapi.ReleaseBuildConfiguration, ib cioperatorapi.ProjectDirectoryImageBuildStepConfiguration) string

	Includes       []string
	Excludes       []string
	ExcludesImages []string

	FBCImages  []string
	JavaImages []string

	ResourcesOutputPathSkipRemove bool
	ResourcesOutputPath           string

	PipelinesOutputPathSkipRemove bool
	PipelinesOutputPath           string

	AdditionalTektonCELExpressionFunc func(cfg cioperatorapi.ReleaseBuildConfiguration, ib cioperatorapi.ProjectDirectoryImageBuildStepConfiguration) string
	NudgesFunc                        func(cfg cioperatorapi.ReleaseBuildConfiguration, ib cioperatorapi.ProjectDirectoryImageBuildStepConfiguration) []string

	Nudges []string

	Tags []string

	PrefetchDeps PrefetchDeps

	IsHermetic func(cfg cioperatorapi.ReleaseBuildConfiguration, ib cioperatorapi.ProjectDirectoryImageBuildStepConfiguration) bool

	ClusterServiceVersionPath  string
	AdditionalComponentConfigs []TemplateConfig
}

type PrefetchDeps struct {
	DevPackageManagers string
	/*
		Example: '[{ "type": "rpm" }, {"type": "gomod", "path": "."}]'
	*/
	PrefetchInput string
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

func Generate(cfg Config) error {
	fbcBuildPipelinePath := filepath.Join(cfg.PipelinesOutputPath, "fbc-builder.yaml")
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

	if cfg.NudgesFunc == nil {
		cfg.NudgesFunc = func(cfg cioperatorapi.ReleaseBuildConfiguration, ib cioperatorapi.ProjectDirectoryImageBuildStepConfiguration) []string {
			return []string{}
		}
	}
	if cfg.IsHermetic == nil {
		cfg.IsHermetic = defaultIsHermetic
	}

	if !cfg.ResourcesOutputPathSkipRemove {
		if err := os.RemoveAll(cfg.ResourcesOutputPath); err != nil {
			return fmt.Errorf("failed to remove %q directory: %w", cfg.ResourcesOutputPath, err)
		}
	}

	if !cfg.PipelinesOutputPathSkipRemove {
		if err := removeAllExcept(cfg.PipelinesOutputPath, fbcBuildPipelinePath, containerBuildPipelinePath); err != nil {
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
				Tags:                          append(cfg.Tags, "latest"),
				BuildArgs:                     cfg.BuildArgs,
				PrefetchDeps:                  cfg.PrefetchDeps,
				DockerfilePath:                dockerfilePath,
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

			appPath := filepath.Join(cfg.ResourcesOutputPath, ApplicationsDirectoryName, appKey, fmt.Sprintf("%s.yaml", appKey))
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

			ecTestPath := filepath.Join(cfg.ResourcesOutputPath, ApplicationsDirectoryName, appKey, "tests", "ec-test.yaml")
			if err := os.MkdirAll(filepath.Dir(ecTestPath), 0777); err != nil {
				return fmt.Errorf("failed to create directory for %q: %w", appPath, err)
			}

			if err := enterpriseContractTestScenarioTemplate.Execute(buf, config); err != nil {
				return fmt.Errorf("failed to execute template for EC test: %w", err)
			}
			if err := WriteFileReplacingNewerTaskImages(ecTestPath, buf.Bytes(), 0777); err != nil {
				return fmt.Errorf("failed to write application file %q: %w", ecTestPath, err)
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

			if config.Pipeline == DockerJavaBuild {

				if err := pipelineDockerJavaBuildTemplate.Execute(buf, nil); err != nil {
					return fmt.Errorf("failed to execute template for pipeline %q: %w", containerJavaBuildPipelinePath, err)
				}
				if err := WriteFileReplacingNewerTaskImages(containerJavaBuildPipelinePath, buf.Bytes(), 0777); err != nil {
					return fmt.Errorf("failed to write FBC build pipeline file %q: %w", containerJavaBuildPipelinePath, err)
				}

			}
		}
	}

	buf := &bytes.Buffer{}
	if err := pipelineDockerBuildTemplate.Execute(buf, cfg); err != nil {
		return fmt.Errorf("failed to execute template for pipeline run PR %q: %w", containerBuildPipelinePath, err)
	}
	if err := WriteFileReplacingNewerTaskImages(containerBuildPipelinePath, buf.Bytes(), 0777); err != nil {
		return fmt.Errorf("failed to write component file %q: %w", containerBuildPipelinePath, err)
	}

	buf.Reset()

	if cfg.ClusterServiceVersionPath != "" {
		if err := GenerateReleasePlanAdmission(cfg.ClusterServiceVersionPath, cfg.ResourcesOutputPath, cfg.ApplicationName); err != nil {
			return fmt.Errorf("failed to generate ReleasePlanAdmission: %w", err)
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
	if _, err := os.Stat(dir); err != nil {
		// directory does not exist anyhow
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
				return nil
			}
		}

		return os.RemoveAll(path)
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
	if _, err := os.Stat(name); err != nil {
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

func defaultIsHermetic(_ cioperatorapi.ReleaseBuildConfiguration, ib cioperatorapi.ProjectDirectoryImageBuildStepConfiguration) bool {
	return !isIndex(ib)
}

func isIndex(ib cioperatorapi.ProjectDirectoryImageBuildStepConfiguration) bool {
	return string(ib.To) == "serverless-index"
}

func GenerateReleasePlanAdmission(csvPath string, resourceOutputPath string, appName string) error {
	csv, err := loadClusterServiceVerion(csvPath)
	if err != nil {
		return fmt.Errorf("failed to load ClusterServiceVersion: %w", err)
	}

	outputDir := filepath.Join(resourceOutputPath, "releaseplanadmissions")
	if err := os.MkdirAll(outputDir, 0777); err != nil {
		return fmt.Errorf("failed to create release plan admissions directory: %w", err)
	}
	outputFilePath := filepath.Join(outputDir, fmt.Sprintf("%s-%s-prod.yaml", Truncate(Sanitize(appName)), Truncate(Sanitize(csv.Spec.Version.String()))))

	components, err := getComponentImageRefs(csv)
	if err != nil {
		return fmt.Errorf("failed to get component image refs: %w", err)
	}

	soVersion := csv.Spec.Version
	if err := executeReleasePlanAdmissionTemplate(components, outputFilePath, appName, soVersion); err != nil {
		return fmt.Errorf("failed to execute release plan admission template: %w", err)
	}

	// create RPA for stage ENV
	componentWithStageRepoRef := make([]ComponentImageRepoRef, 0, len(components))
	for _, component := range components {
		componentWithStageRepoRef = append(componentWithStageRepoRef, ComponentImageRepoRef{
			ComponentName:   component.ComponentName,
			ImageRepository: strings.ReplaceAll(component.ImageRepository, "registry.redhat.io", "registry.stage.redhat.io"),
		})
	}

	outputFilePath = filepath.Join(outputDir, fmt.Sprintf("%s-%s-stage.yaml", Truncate(Sanitize(appName)), Truncate(Sanitize(csv.Spec.Version.String()))))
	if err := executeReleasePlanAdmissionTemplate(componentWithStageRepoRef, outputFilePath, appName, soVersion); err != nil {
		return fmt.Errorf("failed to execute release plan admission template: %w", err)
	}

	return nil
}

func executeReleasePlanAdmissionTemplate(components []ComponentImageRepoRef, outputFilePath string, appName string, soVersion version.OperatorVersion) error {
	funcs := template.FuncMap{
		"sanitize": Sanitize,
		"truncate": Truncate,
		"replace":  replace,
	}

	rpaTemplate, err := template.New("releaseplanadmission.template.yaml").Delims("{{{", "}}}").Funcs(funcs).ParseFS(ReleasePlanAdmissionsTemplate, "*.yaml")
	if err != nil {
		return fmt.Errorf("failed to parse pipeline run push template: %w", err)
	}

	data := map[string]interface{}{
		"ApplicationName": appName,
		"Components":      components,
		"Version":         soVersion.String(),
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

type ComponentImageRepoRef struct {
	ComponentName   string
	ImageRepository string
}

func getComponentImageRefs(csv *operatorsv1alpha1.ClusterServiceVersion) ([]ComponentImageRepoRef, error) {
	var refs []ComponentImageRepoRef

	soVersion := csv.Spec.Version.Version
	componentVersion := soversion.ToUpstreamVersion(soVersion.String())
	for _, relatedImage := range csv.Spec.RelatedImages {
		if !strings.HasPrefix(relatedImage.Image, "registry.redhat.io/openshift-serverless-1") {
			continue
		}

		repoRef, _, _ := strings.Cut(relatedImage.Image, "@sha")
		componentName := strings.TrimPrefix(repoRef, "registry.redhat.io/openshift-serverless-1/")

		if strings.HasPrefix(componentName, "serverless-") {
			// SO component image
			componentName = fmt.Sprintf("%s-%d%d", componentName, soVersion.Major, soVersion.Minor)
		} else {
			// upstream component image
			componentName = fmt.Sprintf("%s-%d%d", componentName, componentVersion.Major, componentVersion.Minor)
		}

		refs = append(refs, ComponentImageRepoRef{
			ComponentName:   componentName,
			ImageRepository: repoRef,
		})
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
