package prowgen

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	cioperatorapi "github.com/openshift/ci-tools/pkg/api"
	prowapi "k8s.io/test-infra/prow/apis/prowjobs/v1"
)

type Repository struct {
	Org                   string                                                      `json:"org" yaml:"org"`
	Repo                  string                                                      `json:"repo" yaml:"repo"`
	Promotion             Promotion                                                   `json:"promotion" yaml:"promotion"`
	ImagePrefix           string                                                      `json:"imagePrefix" yaml:"imagePrefix"`
	ImageNameOverrides    map[string]string                                           `json:"imageNameOverrides" yaml:"imageNameOverrides"`
	SlackChannel          string                                                      `json:"slackChannel" yaml:"slackChannel"`
	CanonicalGoRepository *string                                                     `json:"canonicalGoRepository" yaml:"canonicalGoRepository"`
	E2ETests              []E2ETest                                                   `json:"e2e" yaml:"e2e"`
	Dockerfiles           Dockerfiles                                                 `json:"dockerfiles" yaml:"dockerfiles"`
	IgnoreConfigs         IgnoreConfigs                                               `json:"ignoreConfigs" yaml:"ignoreConfigs"`
	CustomConfigs         []CustomConfigs                                             `json:"customConfigs" yaml:"customConfigs"`
	Images                []cioperatorapi.ProjectDirectoryImageBuildStepConfiguration `json:"images" yaml:"images"`
	Tests                 []cioperatorapi.TestStepConfiguration                       `json:"tests" yaml:"tests"`
	Resources             cioperatorapi.ResourceConfiguration                         `json:"resources" yaml:"resources"`
}

type E2ETest struct {
	Match        string            `json:"match" yaml:"match"`
	OnDemand     bool              `json:"onDemand" yaml:"onDemand"`
	IgnoreError  bool              `json:"ignoreError" yaml:"ignoreError"`
	RunIfChanged string            `json:"runIfChanged" yaml:"runIfChanged"`
	SkipCron     bool              `json:"skipCron" yaml:"skipCron"`
	SkipImages   []string          `json:"skipImages" yaml:"skipImages"`
	Timeout      *prowapi.Duration `json:"timeout" yaml:"timeout"`
}

type Dockerfiles struct {
	Matches []string `json:"matches" yaml:"matches"`
}

type IgnoreConfigs struct {
	Matches []string `json:"matches" yaml:"matches"`
}

type Promotion struct {
	Namespace string
}

type CustomConfigs struct {
	// Name will be used together with OpenShift version to generate a specific variant.
	Name string `json:"name" yaml:"name"`
	// ReleaseBuildConfiguration allows defining configuration manually. The final configuration
	// is extended with images and test steps with dependencies.
	ReleaseBuildConfiguration cioperatorapi.ReleaseBuildConfiguration `json:"releaseBuildConfiguration" yaml:"releaseBuildConfiguration"`
}

func (r Repository) RepositoryDirectory() string {
	return filepath.Join(r.Org, r.Repo)
}

type Branch struct {
	OpenShiftVersions      []OpenShift `json:"openShiftVersions" yaml:"openShiftVersions"`
	SkipE2EMatches         []string    `json:"skipE2EMatches" yaml:"skipE2EMatches"`
	SkipDockerFilesMatches []string    `json:"skipDockerFilesMatches" yaml:"skipDockerFilesMatches"`
}

type OpenShift struct {
	Version               string `json:"version" yaml:"version"`
	Cron                  string `json:"cron" yaml:"cron"`
	OnDemand              bool   `json:"onDemand" yaml:"onDemand"`
	GenerateCustomConfigs bool   `json:"generateCustomConfigs" yaml:"generateCustomConfigs"`
}

type CommonConfig struct {
	Branches map[string]Branch `json:"branches" yaml:"branches"`
}

type ReleaseBuildConfigurationOption func(cfg *cioperatorapi.ReleaseBuildConfiguration) error

type ProjectDirectoryImageBuildStepConfigurationFunc func() (cioperatorapi.ProjectDirectoryImageBuildStepConfiguration, error)

type ReleaseBuildConfiguration struct {
	cioperatorapi.ReleaseBuildConfiguration

	Path   string
	Branch string
}

func NewGenerateConfigs(ctx context.Context, r Repository, cc CommonConfig, opts ...ReleaseBuildConfigurationOption) ([]ReleaseBuildConfiguration, error) {

	cfgs := make([]ReleaseBuildConfiguration, 0, len(cc.Branches)*2)

	if err := GitMirror(ctx, r); err != nil {
		return nil, err
	}

	for branchName, branch := range cc.Branches {

		if err := GitCheckout(ctx, r, branchName); err != nil {
			return nil, fmt.Errorf("[%s] failed to checkout branch %s", r.RepositoryDirectory(), branchName)
		}

		isFirstVersion := true
		for _, ov := range branch.OpenShiftVersions {

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
				"latest": {Release: &cioperatorapi.Release{
					Version: ov.Version,
					Channel: cioperatorapi.ReleaseChannelFast},
				},
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
			if isFirstVersion {
				isFirstVersion = false
				options = append(options, withNamePromotion(r, branchName))
			} else {
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
				DiscoverTests(r, ov, fromImage, branch.SkipE2EMatches),
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
			Namespace: ns,
			Name:      strings.ReplaceAll(strings.ReplaceAll(branchName, "release", "knative"), "next", "nightly"),
			AdditionalImages: map[string]string{
				// Add source image
				transformLegacyKnativeSourceImageName(r): "src",
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
			Namespace:   ns,
			Tag:         strings.ReplaceAll(strings.ReplaceAll(branchName, "release", "knative"), "next", "nightly"),
			TagByCommit: false, // TODO: revisit this later
			AdditionalImages: map[string]string{
				// Add source image
				transformLegacyKnativeSourceImageName(r): "src",
			},
		}
		return nil
	}
}

func applyOptions(cfg *cioperatorapi.ReleaseBuildConfiguration, opts ...ReleaseBuildConfigurationOption) error {
	for _, opt := range opts {
		if err := opt(cfg); err != nil {
			return err
		}
	}
	return nil
}
