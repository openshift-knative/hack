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

	"github.com/coreos/go-semver/semver"
	cioperatorapi "github.com/openshift/ci-tools/pkg/api"
	prowapi "k8s.io/test-infra/prow/apis/prowjobs/v1"
)

type Repository struct {
	Org                   string                                                      `json:"org,omitempty" yaml:"org,omitempty"`
	Repo                  string                                                      `json:"repo,omitempty" yaml:"repo,omitempty"`
	Promotion             Promotion                                                   `json:"promotion,omitempty" yaml:"promotion,omitempty"`
	BinaryBuildCommands   string                                                      `json:"binaryBuildCommands,omitempty" yaml:"binaryBuildCommands,omitempty"`
	ImagePrefix           string                                                      `json:"imagePrefix,omitempty" yaml:"imagePrefix,omitempty"`
	ImageNameOverrides    map[string]string                                           `json:"imageNameOverrides,omitempty" yaml:"imageNameOverrides,omitempty"`
	SlackChannel          string                                                      `json:"slackChannel,omitempty" yaml:"slackChannel,omitempty"`
	CanonicalGoRepository *string                                                     `json:"canonicalGoRepository,omitempty" yaml:"canonicalGoRepository,omitempty"`
	E2ETests              []E2ETest                                                   `json:"e2e,omitempty" yaml:"e2e,omitempty"`
	Dockerfiles           Dockerfiles                                                 `json:"dockerfiles,omitempty" yaml:"dockerfiles,omitempty"`
	SharedInputs          map[string]cioperatorapi.ImageBuildInputs                   `json:"sharedInputs,omitempty" yaml:"sharedInputs,omitempty"`
	IgnoreConfigs         IgnoreConfigs                                               `json:"ignoreConfigs,omitempty" yaml:"ignoreConfigs,omitempty"`
	CustomConfigs         []CustomConfigs                                             `json:"customConfigs,omitempty" yaml:"customConfigs,omitempty"`
	Images                []cioperatorapi.ProjectDirectoryImageBuildStepConfiguration `json:"images,omitempty" yaml:"images,omitempty"`
	Tests                 []cioperatorapi.TestStepConfiguration                       `json:"tests,omitempty" yaml:"tests,omitempty"`
	Resources             cioperatorapi.ResourceConfiguration                         `json:"resources,omitempty" yaml:"resources,omitempty"`
}

type E2ETest struct {
	// Name is an optional field that can be used to identify the test.
	Name string `json:"name,omitempty" yaml:"name,omitempty"`
	// Command or Match is required to define the test. If both are defined, the config will fail.
	Command string `json:"command,omitempty" yaml:"command,omitempty"`
	// Match will look for a given Makefile target to run the test.
	Match        string                        `json:"match,omitempty" yaml:"match,omitempty"`
	Environment  cioperatorapi.TestEnvironment `json:"env,omitempty" yaml:"env,omitempty"`
	OnDemand     bool                          `json:"onDemand,omitempty" yaml:"onDemand,omitempty"`
	IgnoreError  bool                          `json:"ignoreError,omitempty" yaml:"ignoreError,omitempty"`
	RunIfChanged string                        `json:"runIfChanged,omitempty" yaml:"runIfChanged,omitempty"`
	// SkipCron ensures that no periodic job will be generated for the given test.
	SkipCron   bool              `json:"skipCron,omitempty" yaml:"skipCron,omitempty"`
	SkipImages []string          `json:"skipImages,omitempty" yaml:"skipImages,omitempty"`
	Timeout    *prowapi.Duration `json:"timeout,omitempty" yaml:"timeout,omitempty"`
}

type Dockerfiles struct {
	Matches  []string `json:"matches,omitempty" yaml:"matches,omitempty"`
	Excludes []string `json:"excludes,omitempty" yaml:"excludes,omitempty"`
}

type IgnoreConfigs struct {
	Matches []string `json:"matches,omitempty" yaml:"matches,omitempty"`
}

type Promotion struct {
	Namespace  string `json:"namespace,omitempty" yaml:"namespace,omitempty"`
	Template   string `json:"template,omitempty" yaml:"template,omitempty"`
	OmitSource bool   `json:"omitSource,omitempty" yaml:"omitSource,omitempty"`
}

type CustomConfigs struct {
	// Name will be used together with OpenShift version to generate a specific variant.
	Name string `json:"name,omitempty" yaml:"name,omitempty"`
	// ReleaseBuildConfiguration allows defining configuration manually. The final configuration
	// is extended with images and test steps with dependencies.
	ReleaseBuildConfiguration cioperatorapi.ReleaseBuildConfiguration `json:"releaseBuildConfiguration,omitempty" yaml:"releaseBuildConfiguration,omitempty"`
}

func (r Repository) RepositoryDirectory() string {
	return filepath.Join(r.Org, r.Repo)
}

type Branch struct {
	OpenShiftVersions      []OpenShift `json:"openShiftVersions,omitempty" yaml:"openShiftVersions,omitempty"`
	SkipE2EMatches         []string    `json:"skipE2EMatches,omitempty" yaml:"skipE2EMatches,omitempty"`
	SkipDockerFilesMatches []string    `json:"skipDockerFilesMatches,omitempty" yaml:"skipDockerFilesMatches,omitempty"`
}

type OpenShift struct {
	Version string `json:"version,omitempty" yaml:"version,omitempty"`
	Cron    string `json:"cron,omitempty" yaml:"cron,omitempty"`
	// SkipCron ensures that no periodic jobs are generated for tests running on the given OpenShift version.
	SkipCron              bool `json:"skipCron,omitempty" yaml:"skipCron,omitempty"`
	OnDemand              bool `json:"onDemand,omitempty" yaml:"onDemand,omitempty"`
	GenerateCustomConfigs bool `json:"generateCustomConfigs,omitempty" yaml:"generateCustomConfigs,omitempty"`
	CandidateRelease      bool `json:"candidateRelease,omitempty" yaml:"candidateRelease,omitempty"`
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
		branch := cc.Branches[branchName]

		if err := GitCheckout(ctx, r, branchName); err != nil {
			return nil, fmt.Errorf("[%s] failed to checkout branch %s", r.RepositoryDirectory(), branchName)
		}

		openshiftVersions, err := addCandidateRelease(branch.OpenShiftVersions)
		if err != nil {
			return nil, err
		}

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
				BinaryBuildCommands: r.BinaryBuildCommands,
				Metadata:            metadata,
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

			if !ov.GenerateCustomConfigs {
				continue
			}

			// Generate custom configs.
			for _, customCfg := range r.CustomConfigs {
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
					r.Org+"-"+r.Repo+"-"+branchName+"__"+variant+"-"+customCfg.Name+".yaml",
				)

				cfgs = append(cfgs, ReleaseBuildConfiguration{
					ReleaseBuildConfiguration: *customBuildCfg,
					Path:                      buildConfigPath,
					Branch:                    branchName,
				})
			}
		}
	}

	return cfgs, nil
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
			Targets: []cioperatorapi.PromotionTarget{{
				Namespace:        ns,
				Name:             createPropotionName(r.Promotion, branchName),
				AdditionalImages: createPromotionAdditionalImages(r),
			}},
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
			Targets: []cioperatorapi.PromotionTarget{{
				Namespace:        ns,
				Tag:              createPropotionName(r.Promotion, branchName),
				TagByCommit:      false, // TODO: revisit this later
				AdditionalImages: createPromotionAdditionalImages(r),
			}},
		}
		return nil
	}
}

func createPromotionAdditionalImages(r Repository) map[string]string {
	if r.Promotion.OmitSource {
		return nil
	}
	return map[string]string{
		// Add source image
		transformLegacyKnativeSourceImageName(r): "src",
	}
}

func createPropotionName(p Promotion, branchName string) string {
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

func addCandidateRelease(openshiftVersions []OpenShift) ([]OpenShift, error) {
	semVersions := make([]*semver.Version, 0, len(openshiftVersions))
	for _, ov := range openshiftVersions {
		v := ov.Version
		// Make sure version strings are in the format MAJOR.MINOR.MICRO
		if len(strings.SplitN(v, ".", 3)) != 3 {
			v = v + ".0"
		}
		ovSemVer, err := semver.NewVersion(v)
		if err != nil {
			return nil, err
		}
		semVersions = append(semVersions, ovSemVer)
	}
	semver.Sort(semVersions)

	latest := *semVersions[len(semVersions)-1]
	latest.BumpMinor()

	extendedVersions := append(openshiftVersions, OpenShift{
		Version:          fmt.Sprintf("%d.%d", latest.Major, latest.Minor),
		OnDemand:         true,
		CandidateRelease: true},
	)

	return extendedVersions, nil
}

func applyOptions(cfg *cioperatorapi.ReleaseBuildConfiguration, opts ...ReleaseBuildConfigurationOption) error {
	for _, opt := range opts {
		if err := opt(cfg); err != nil {
			return err
		}
	}
	return nil
}
