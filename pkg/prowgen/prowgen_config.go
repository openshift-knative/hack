package prowgen

import (
	"context"
	"fmt"
	"log"
	"path/filepath"
	"strings"

	cioperatorapi "github.com/openshift/ci-tools/pkg/api"
)

type Repository struct {
	Org                   string                                                      `json:"org" yaml:"org"`
	Repo                  string                                                      `json:"repo" yaml:"repo"`
	ImagePrefix           string                                                      `json:"imagePrefix" yaml:"imagePrefix"`
	SlackChannel          string                                                      `json:"slackChannel" yaml:"slackChannel"`
	CanonicalGoRepository *string                                                     `json:"canonicalGoRepository" yaml:"canonicalGoRepository"`
	E2ETests              E2ETests                                                    `json:"e2e" yaml:"e2e"`
	Images                []cioperatorapi.ProjectDirectoryImageBuildStepConfiguration `json:"images" yaml:"images"`
	Tests                 []cioperatorapi.TestStepConfiguration                       `json:"tests" yaml:"tests"`
	Resources             cioperatorapi.ResourceConfiguration                         `json:"resources" yaml:"resources"`
}

type E2ETests struct {
	Matches []string `json:"matches" yaml:"matches"`
}

func (r Repository) RepositoryDirectory() string {
	return filepath.Join(r.Org, r.Repo)
}

type Branch struct {
	OpenShiftVersions []string `json:"openShiftVersions" yaml:"openShiftVersions"`
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

	if err := GitClone(ctx, r); err != nil {
		return nil, err
	}

	for branchName, branch := range cc.Branches {

		if err := GitCheckout(ctx, r, branchName); err != nil {
			return nil, fmt.Errorf("[%s] failed to checkout branch %s", r.RepositoryDirectory(), branchName)
		}

		isFirstVersion := true
		for _, ov := range branch.OpenShiftVersions {

			log.Println(r.RepositoryDirectory(), "Generating config", branchName, "OpenShiftVersion", ov)

			variant := strings.ReplaceAll(ov, ".", "")

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

			cfg := cioperatorapi.ReleaseBuildConfiguration{
				Metadata: cioperatorapi.Metadata{
					Org:     r.Org,
					Repo:    r.Repo,
					Branch:  branchName,
					Variant: variant,
				},
				InputConfiguration: cioperatorapi.InputConfiguration{
					BuildRootImage: &cioperatorapi.BuildRootImageConfiguration{
						ProjectImageBuild: &cioperatorapi.ProjectDirectoryImageBuildInputs{
							DockerfilePath: "openshift/ci-operator/build-image/Dockerfile",
						},
					},
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

			options = append(
				options,
				DiscoverImages(r),
				DiscoverTests(r, ov),
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
		}
	}

	return cfgs, nil
}

// TODO: In 2023 we need to move forward to use the new `eventing`, for _new_ repos,
// The tool should only generate desired updates, always all
func transformLegacyKnativeEventingSourceImageName(r Repository) string {
	// The old repository is called knative-eventing, and we need to keep this coordinate on the build
	// For now: we can not use the new `eventing` name
	srcImage := r.Repo + "-src"
	if r.Repo == "eventing" {
		srcImage = "knative-" + r.Repo + "-src"
	}
	return srcImage
}

func withNamePromotion(r Repository, branchName string) ReleaseBuildConfigurationOption {
	return func(cfg *cioperatorapi.ReleaseBuildConfiguration) error {
		cfg.PromotionConfiguration = &cioperatorapi.PromotionConfiguration{
			Namespace: "openshift",
			Name:      strings.ReplaceAll(strings.ReplaceAll(branchName, "release", "knative"), "next", "nightly"),
			AdditionalImages: map[string]string{
				// Add source image
				transformLegacyKnativeEventingSourceImageName(r): "src",
			},
		}
		return nil
	}
}

func withTagPromotion(r Repository, branchName string) ReleaseBuildConfigurationOption {
	return func(cfg *cioperatorapi.ReleaseBuildConfiguration) error {
		cfg.PromotionConfiguration = &cioperatorapi.PromotionConfiguration{
			Namespace:   "openshift",
			Tag:         strings.ReplaceAll(strings.ReplaceAll(branchName, "release", "knative"), "next", "nightly"),
			TagByCommit: true,
			AdditionalImages: map[string]string{
				// Add source image
				transformLegacyKnativeEventingSourceImageName(r): "src",
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
