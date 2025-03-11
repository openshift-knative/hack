package prowgen

import (
	"context"
	"fmt"
	"log"
	"math/rand"
	"os"
	"path/filepath"
	"slices"
	"strings"

	cioperatorapi "github.com/openshift/ci-tools/pkg/api"
	prowapi "sigs.k8s.io/prow/pkg/apis/prowjobs/v1"

	"github.com/openshift-knative/hack/pkg/util"
)

type Repository struct {
	Org                   string                                                      `json:"org,omitempty" yaml:"org,omitempty"`
	Repo                  string                                                      `json:"repo,omitempty" yaml:"repo,omitempty"`
	Promotion             Promotion                                                   `json:"promotion,omitempty" yaml:"promotion,omitempty"`
	ImagePrefix           string                                                      `json:"imagePrefix,omitempty" yaml:"imagePrefix,omitempty"`
	ImageNameOverrides    map[string]string                                           `json:"imageNameOverrides,omitempty" yaml:"imageNameOverrides,omitempty"`
	SlackChannel          string                                                      `json:"slackChannel,omitempty" yaml:"slackChannel,omitempty"`
	CanonicalGoRepository *string                                                     `json:"canonicalGoRepository,omitempty" yaml:"canonicalGoRepository,omitempty"`
	E2ETests              []E2ETest                                                   `json:"e2e,omitempty" yaml:"e2e,omitempty"`
	Dockerfiles           Dockerfiles                                                 `json:"dockerfiles,omitempty" yaml:"dockerfiles,omitempty"`
	IgnoreConfigs         IgnoreConfigs                                               `json:"ignoreConfigs,omitempty" yaml:"ignoreConfigs,omitempty"`
	CustomConfigs         []CustomConfigs                                             `json:"customConfigs,omitempty" yaml:"customConfigs,omitempty"`
	Images                []cioperatorapi.ProjectDirectoryImageBuildStepConfiguration `json:"images,omitempty" yaml:"images,omitempty"`
	Tests                 []cioperatorapi.TestStepConfiguration                       `json:"tests,omitempty" yaml:"tests,omitempty"`
	Resources             cioperatorapi.ResourceConfiguration                         `json:"resources,omitempty" yaml:"resources,omitempty"`
}

type E2ETest struct {
	Match        string `json:"match,omitempty" yaml:"match,omitempty"`
	OnDemand     bool   `json:"onDemand,omitempty" yaml:"onDemand,omitempty"`
	IgnoreError  bool   `json:"ignoreError,omitempty" yaml:"ignoreError,omitempty"`
	RunIfChanged string `json:"runIfChanged,omitempty" yaml:"runIfChanged,omitempty"`
	// SkipCron ensures that no periodic job will be generated for the given test.
	SkipCron   bool              `json:"skipCron,omitempty" yaml:"skipCron,omitempty"`
	SkipImages []string          `json:"skipImages,omitempty" yaml:"skipImages,omitempty"`
	Timeout    *prowapi.Duration `json:"timeout,omitempty" yaml:"timeout,omitempty"`
	JobTimeout *prowapi.Duration `json:"jobTimeout,omitempty" yaml:"jobTimeout,omitempty"`
}

type Dockerfiles struct {
	Matches  []string `json:"matches,omitempty" yaml:"matches,omitempty"`
	Excludes []string `json:"excludes,omitempty" yaml:"excludes,omitempty"`
}

type IgnoreConfigs struct {
	Matches []string `json:"matches,omitempty" yaml:"matches,omitempty"`
}

type Promotion struct {
	Namespace string `json:"namespace,omitempty" yaml:"namespace,omitempty"`
	Template  string `json:"template,omitempty" yaml:"template,omitempty"`
}

type CustomConfigs struct {
	// Name will be used to generate a specific variant.
	Name string `json:"name,omitempty" yaml:"name,omitempty"`
	// ReleaseBuildConfiguration allows defining configuration manually. The final configuration
	// is extended with images and test steps with dependencies.
	ReleaseBuildConfiguration cioperatorapi.ReleaseBuildConfiguration `json:"releaseBuildConfiguration,omitempty" yaml:"releaseBuildConfiguration,omitempty"`
}

func (r Repository) RepositoryDirectory() string {
	if r.Org == "" && r.Repo == "" {
		return ""
	}
	return filepath.Join(r.Org, r.Repo)
}

type Branch struct {
	Prowgen                *Prowgen    `json:"prowgen,omitempty" yaml:"prowgen,omitempty"`
	OpenShiftVersions      []OpenShift `json:"openShiftVersions,omitempty" yaml:"openShiftVersions,omitempty"`
	SkipE2EMatches         []string    `json:"skipE2EMatches,omitempty" yaml:"skipE2EMatches,omitempty"`
	SkipDockerFilesMatches []string    `json:"skipDockerFilesMatches,omitempty" yaml:"skipDockerFilesMatches,omitempty"`
	Konflux                *Konflux    `json:"konflux,omitempty" yaml:"konflux,omitempty"`

	// DependabotEnabled enabled if `nil`.
	DependabotEnabled *bool `json:"dependabotEnabled,omitempty" yaml:"dependabotEnabled,omitempty"`
}

type Konflux struct {
	Enabled bool `json:"enabled,omitempty" yaml:"enabled,omitempty"`

	Nudges []string `json:"nudges,omitempty" yaml:"nudges,omitempty"`

	Excludes       []string `json:"excludes,omitempty" yaml:"excludes,omitempty"`
	ExcludesImages []string `json:"excludesImages,omitempty" yaml:"excludesImages,omitempty"`

	JavaImages []string `json:"javaImages,omitempty" yaml:"javaImages,omitempty"`

	ImageOverrides []Image `json:"imageOverrides,omitempty" yaml:"imageOverrides,omitempty"`
}

type Prowgen struct {
	Disabled bool `json:"disabled,omitempty" yaml:"disabled,omitempty"`
}

type OpenShift struct {
	Version        string `json:"version,omitempty" yaml:"version,omitempty"`
	UseClusterPool bool   `json:"useClusterPool,omitempty" yaml:"useClusterPool,omitempty"`
	Cron           string `json:"cron,omitempty" yaml:"cron,omitempty"`
	// SkipCron ensures that no periodic jobs are generated for tests running on the given OpenShift version.
	SkipCron         bool                     `json:"skipCron,omitempty" yaml:"skipCron,omitempty"`
	OnDemand         bool                     `json:"onDemand,omitempty" yaml:"onDemand,omitempty"`
	CustomConfigs    *CustomConfigsEnablement `json:"customConfigs,omitempty" yaml:"customConfigs,omitempty"`
	CandidateRelease bool                     `json:"candidateRelease,omitempty" yaml:"candidateRelease,omitempty"`
}

type CustomConfigsEnablement struct {
	Enabled  bool     `json:"enabled,omitempty" yaml:"enabled,omitempty"`
	Includes []string `json:"includes,omitempty" yaml:"includes,omitempty"`
	Excludes []string `json:"excludes,omitempty" yaml:"excludes,omitempty"`
}

type Image struct {
	Name     string `json:"name" yaml:"name"`
	PullSpec string `json:"pullSpec" yaml:"pullSpec"`
}

type CommonConfig struct {
	Branches map[string]Branch `json:"branches,omitempty" yaml:"branches,omitempty"`
}

type ReleaseBuildConfigurationOption func(cfg *cioperatorapi.ReleaseBuildConfiguration) error

type ProjectDirectoryImageBuildStepConfigurationFunc func() (cioperatorapi.ProjectDirectoryImageBuildStepConfiguration, error)

type ReleaseBuildConfiguration struct {
	cioperatorapi.ReleaseBuildConfiguration

	Path   string
	Branch string
}

func NewGenerateConfigs(ctx context.Context, r Repository, cc CommonConfig, opts ...ReleaseBuildConfigurationOption) ([]ReleaseBuildConfiguration, error) {
	// Use the same seed to always get the same sequence of random
	// numbers for tests within the given repository. It means the cron schedules
	// for jobs will change less often when generating jobs.
	random := rand.New(rand.NewSource(seed))

	cfgs := make([]ReleaseBuildConfiguration, 0, len(cc.Branches)*2)

	if err := GitMirror(ctx, r); err != nil {
		return nil, err
	}

	branches := make([]string, 0, len(cc.Branches))
	for k := range cc.Branches {
		branches = append(branches, k)
	}
	// Make sure to iterate every time in the same order to keep
	// cron times consistent between runs.
	slices.Sort(branches)

	for _, branchName := range branches {

		// This is a special GH log format: https://docs.github.com/en/actions/writing-workflows/choosing-what-your-workflow-does/workflow-commands-for-github-actions#example-grouping-log-lines
		log.Printf("::group::prowgen %s %s\n", r.RepositoryDirectory(), branchName)

		branch := cc.Branches[branchName]
		if branch.Prowgen != nil && branch.Prowgen.Disabled {
			continue
		}

		if err := GitCheckout(ctx, r, branchName); err != nil {
			return nil, fmt.Errorf("[%s] failed to checkout branch %s", r.RepositoryDirectory(), branchName)
		}

		openshiftVersions := branch.OpenShiftVersions

		for i, ov := range openshiftVersions {
			log.Println(r.RepositoryDirectory(), "Generating config", branchName, "OpenShiftVersion", ov)

			variant := strings.ReplaceAll(ov.Version, ".", "")

			images := make([]cioperatorapi.ProjectDirectoryImageBuildStepConfiguration, 0, len(r.Images))
			for _, img := range r.Images {
				images = append(images, *img.DeepCopy())
			}

			tests := make([]cioperatorapi.TestStepConfiguration, 0, len(r.Tests))
			for _, test := range r.Tests {
				tests = append(tests, *test.DeepCopy())
			}

			resources := make(cioperatorapi.ResourceConfiguration, 1)
			resources["*"] = cioperatorapi.ResourceRequirements{
				Requests: map[string]string{
					"cpu":    "500m",
					"memory": "1Gi",
				},
			}
			for k, v := range r.Resources {
				resources[k] = v
			}

			metadata := cioperatorapi.Metadata{
				Org:     r.Org,
				Repo:    r.Repo,
				Branch:  branchName,
				Variant: variant,
			}
			buildRootImage := &cioperatorapi.BuildRootImageConfiguration{
				ProjectImageBuild: &cioperatorapi.ProjectDirectoryImageBuildInputs{
					DockerfilePath: "openshift/ci-operator/build-image/Dockerfile",
				},
			}
			// Include releases as it's required by clusters that start from scratch (vs. cluster-pools).
			releases := map[string]cioperatorapi.UnresolvedRelease{
				"latest": {
					Release: &cioperatorapi.Release{
						Version: ov.Version,
						Channel: cioperatorapi.ReleaseChannelFast},
				},
			}
			if ov.CandidateRelease {
				releases = map[string]cioperatorapi.UnresolvedRelease{
					"latest": {
						Candidate: &cioperatorapi.Candidate{
							Version: ov.Version,
							Stream:  "nightly",
							ReleaseDescriptor: cioperatorapi.ReleaseDescriptor{
								Product: "ocp",
							},
						}},
				}
			}

			cfg := cioperatorapi.ReleaseBuildConfiguration{
				Metadata: metadata,
				InputConfiguration: cioperatorapi.InputConfiguration{
					BuildRootImage: buildRootImage,
					Releases:       releases,
				},
				CanonicalGoRepository: r.CanonicalGoRepository,
				Images:                images,
				Tests:                 tests,
				Resources:             resources,
			}

			options := make([]ReleaseBuildConfigurationOption, 0, len(opts))
			copy(options, opts)
			if i == 0 {
				options = append(options, withNamePromotion(r, branchName))
			} else if i == 1 {
				options = append(options, withTagPromotion(r, branchName))
			}

			fromImage := srcImage
			srcImageDockerfile, err := discoverSourceImageDockerfile(r)
			if err != nil {
				return nil, err
			}
			if srcImageDockerfile != "" {
				fromImage = toImage(r, ImageInput{
					Context:        discoverImageContext(srcImageDockerfile),
					DockerfilePath: strings.Join(strings.Split(srcImageDockerfile, string(os.PathSeparator))[2:], string(os.PathSeparator)),
				})
			}

			options = append(
				options,
				DiscoverImages(r, branch.SkipDockerFilesMatches),
				DiscoverTests(r, ov, fromImage, branch.SkipE2EMatches, random),
			)

			log.Println(r.RepositoryDirectory(), "Apply input options", len(options))

			if err := applyOptions(&cfg, options...); err != nil {
				return nil, fmt.Errorf("[%s] failed to apply option: %w", r.RepositoryDirectory(), err)
			}

			log.Println("numTests", len(cfg.Tests), "numImages", len(cfg.Images))

			// openshift-knative/eventing-kafka-broker/openshift-knative-eventing-kafka-broker-release-next__411.yaml
			buildConfigPath := filepath.Join(
				r.RepositoryDirectory(),
				r.Org+"-"+r.Repo+"-"+branchName+"__"+variant+".yaml",
			)

			cfgs = append(cfgs, ReleaseBuildConfiguration{
				ReleaseBuildConfiguration: cfg,
				Path:                      buildConfigPath,
				Branch:                    branchName,
			})

			if ov.CustomConfigs == nil || !ov.CustomConfigs.Enabled {
				continue
			}

			// Generate custom configs.
			for _, customCfg := range r.CustomConfigs {
				shouldInclude, err := shouldIncludeCustomConfig(ov, customCfg.Name)
				if err != nil {
					return nil, err
				}
				if !shouldInclude {
					continue
				}
				customBuildCfg := customCfg.ReleaseBuildConfiguration.DeepCopy()
				customBuildCfg.Metadata = metadata
				if customBuildCfg.BuildRootImage == nil {
					customBuildCfg.BuildRootImage = buildRootImage
				}
				if customBuildCfg.CanonicalGoRepository == nil {
					customBuildCfg.CanonicalGoRepository = r.CanonicalGoRepository
				}
				if len(customBuildCfg.Resources) == 0 {
					customBuildCfg.Resources = resources
				}
				if len(customBuildCfg.Releases) == 0 {
					customBuildCfg.Releases = releases
				}

				customBuildOptions := append(
					opts,
					DiscoverImages(r, branch.SkipDockerFilesMatches),
					DependenciesForTestSteps(),
				)

				log.Println(r.RepositoryDirectory(), "Apply input options", len(customBuildOptions))

				if err := applyOptions(customBuildCfg, customBuildOptions...); err != nil {
					return nil, fmt.Errorf("[%s] failed to apply option: %w", r.RepositoryDirectory(), err)
				}

				log.Println("numTests", len(customBuildCfg.Tests), "numImages", len(customBuildCfg.Images))

				buildConfigPath = filepath.Join(
					r.RepositoryDirectory(),
					r.Org+"-"+r.Repo+"-"+branchName+"__"+customCfg.Name+".yaml",
				)

				cfgs = append(cfgs, ReleaseBuildConfiguration{
					ReleaseBuildConfiguration: *customBuildCfg,
					Path:                      buildConfigPath,
					Branch:                    branchName,
				})
			}
		}

		// This is a special GH log format: https://docs.github.com/en/actions/writing-workflows/choosing-what-your-workflow-does/workflow-commands-for-github-actions#example-grouping-log-lines
		log.Printf("::endgroup::\n\n")
	}

	return cfgs, nil
}

func shouldIncludeCustomConfig(ov OpenShift, customCfgName string) (bool, error) {
	includes, err := util.ToRegexp(ov.CustomConfigs.Includes)
	if err != nil {
		return false, fmt.Errorf("failed to create regular expressions for %+v: %w", ov.CustomConfigs.Includes, err)
	}
	excludes, err := util.ToRegexp(ov.CustomConfigs.Excludes)
	if err != nil {
		return false, fmt.Errorf("failed to create regular expressions for %+v: %w", ov.CustomConfigs.Excludes, err)
	}
	// Empty includes means we want everything. Configs can still be excluded later.
	// If both "includes" and "excludes" match the config name then excludes take precedence.
	shouldInclude := len(includes) == 0
	for _, i := range includes {
		if i.MatchString(customCfgName) {
			shouldInclude = true
		}
	}
	for _, x := range excludes {
		if x.MatchString(customCfgName) {
			shouldInclude = false
		}
	}
	return shouldInclude, nil
}

// TODO: In 2023 we need to move forward to use the new `eventing` or `serving`, for _new_ repos,
// The tool should only generate desired updates, always all
func transformLegacyKnativeSourceImageName(r Repository) string {
	// The old repository is called knative-eventing or knative-serving,
	// and we need to keep this coordinate on the build
	// For now: we can not use the new `eventing` or `serving` name
	srcImage := r.Repo + "-src"
	if r.Repo == "eventing" || r.Repo == "serving" {
		srcImage = "knative-" + r.Repo + "-src"
	}
	return srcImage
}

func withNamePromotion(r Repository, branchName string) ReleaseBuildConfigurationOption {
	return func(cfg *cioperatorapi.ReleaseBuildConfiguration) error {
		ns := "openshift"
		if r.Promotion.Namespace != "" {
			ns = r.Promotion.Namespace
		}
		cfg.PromotionConfiguration = &cioperatorapi.PromotionConfiguration{
			Targets: []cioperatorapi.PromotionTarget{
				{
					Namespace: ns,
					Name:      createPromotionName(r.Promotion, branchName),
					AdditionalImages: map[string]string{
						// Add source image
						transformLegacyKnativeSourceImageName(r): "src",
					},
				},
			},
		}
		return nil
	}
}

func withTagPromotion(r Repository, branchName string) ReleaseBuildConfigurationOption {
	return func(cfg *cioperatorapi.ReleaseBuildConfiguration) error {
		ns := "openshift"
		if r.Promotion.Namespace != "" {
			ns = r.Promotion.Namespace
		}
		cfg.PromotionConfiguration = &cioperatorapi.PromotionConfiguration{
			Targets: []cioperatorapi.PromotionTarget{
				{
					Namespace:   ns,
					Tag:         createPromotionName(r.Promotion, branchName),
					TagByCommit: false, // TODO: revisit this later
					AdditionalImages: map[string]string{
						// Add source image
						transformLegacyKnativeSourceImageName(r): "src",
					},
				},
			},
		}
		return nil
	}
}

func createPromotionName(p Promotion, branchName string) string {
	tpl := "knative-${version}"
	if p.Template != "" {
		tpl = p.Template
	}
	version := strings.Replace(branchName, "release-", "", 1)
	if version == "next" {
		version = "nightly"
	}
	return strings.ReplaceAll(tpl, "${version}", version)
}

func applyOptions(cfg *cioperatorapi.ReleaseBuildConfiguration, opts ...ReleaseBuildConfigurationOption) error {
	for _, opt := range opts {
		if err := opt(cfg); err != nil {
			return err
		}
	}
	return nil
}

func (r Repository) IsServerlessOperator() bool {
	return r.Org == "openshift-knative" && r.Repo == "serverless-operator"
}

func (r Repository) IsEventingIntegrations() bool {
	return r.Org == "openshift-knative" && r.Repo == "eventing-integrations"
}

func (r Repository) IsEKB() bool {
	return r.Org == "openshift-knative" && r.Repo == "eventing-kafka-broker"
}

func (r Repository) IsFunc() bool {
	return r.Org == "openshift-knative" && r.Repo == "kn-plugin-func"
}

func (r Repository) IsEventPlugin() bool {
	return r.Org == "openshift-knative" && r.Repo == "kn-plugin-event"
}

func (r Repository) RunCodegenCommand() string {
	run := "make generate-release"
	if r.IsFunc() || r.IsEventPlugin() {
		// These repos don't use vendor, so they don't patch dependencies.
		run = ""
	}
	if r.IsServerlessOperator() {
		run = "make generated-files"
	}
	return run
}

func (r Repository) RunDockefileGenCommand() string {
	run := r.RunCodegenCommand()
	if r.IsFunc() {
		run = "./openshift/scripts/generate-dockerfiles.sh"
	}
	if r.IsServerlessOperator() {
		// SO has its own scheduled workflow (Validate).
		run = ""
	}
	return run
}
