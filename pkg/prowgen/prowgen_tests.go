package prowgen

import (
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"log"
	"math/rand"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	cioperatorapi "github.com/openshift/ci-tools/pkg/api"
	"k8s.io/apimachinery/pkg/util/sets"
	prowapi "k8s.io/test-infra/prow/apis/prowjobs/v1"
	"k8s.io/utils/pointer"
	"k8s.io/utils/strings/slices"
)

const (
	cronTemplate         = "%d %d * * 2,6"
	seed                 = 12345
	devclusterBaseDomain = "serverless.devcluster.openshift.com"
	// Holds version of the cluster pool dedicated to OpenShift Serverless in CI.
	// See https://docs.ci.openshift.org/docs/how-tos/cluster-claim/#existing-cluster-pools
	clusterPoolVersion = "4.15"
)

func DiscoverTests(r Repository, openShift OpenShift, sourceImageName string, skipE2ETestMatch []string, random *rand.Rand) ReleaseBuildConfigurationOption {
	return func(cfg *cioperatorapi.ReleaseBuildConfiguration) error {
		tests, err := discoverE2ETests(r, skipE2ETestMatch)
		if err != nil {
			return err
		}

		for i := range tests {
			test := &tests[i]
			as := ToName(r, test, openShift.Version)

			var testTimeout *prowapi.Duration
			var jobTimeout *prowapi.Duration

			if test.Timeout != nil {
				testTimeout = test.Timeout
				jobTimeout = &prowapi.Duration{Duration: test.Timeout.Duration + time.Hour} // test time + 3 * 20m must-gathers
			} else {
				// Use 4h test timeout by default
				testTimeout = &prowapi.Duration{Duration: 4 * time.Hour}
			}

			var (
				clusterClaim   *cioperatorapi.ClusterClaim
				clusterProfile cioperatorapi.ClusterProfile
				workflow       *string
				env            cioperatorapi.TestEnvironment
			)

			useClusterPool := openShift.Version == clusterPoolVersion
			// Make sure to use the existing cluster pool if available for the given OpenShift version.
			if useClusterPool {
				clusterClaim = &cioperatorapi.ClusterClaim{
					Product:      cioperatorapi.ReleaseProductOCP,
					Version:      openShift.Version,
					Architecture: cioperatorapi.ReleaseArchitectureAMD64,
					Cloud:        cioperatorapi.CloudAWS,
					Owner:        "serverless-ci",
					Timeout:      &prowapi.Duration{Duration: time.Hour},
				}
				workflow = pointer.String("generic-claim")
			} else {
				clusterProfile = "aws-serverless"
				env = map[string]string{
					"BASE_DOMAIN": devclusterBaseDomain,
				}
				workflow = pointer.String("ipi-aws")
			}
			testConfiguration := cioperatorapi.TestStepConfiguration{
				As:           as,
				ClusterClaim: clusterClaim,
				Timeout:      jobTimeout,
				MultiStageTestConfiguration: &cioperatorapi.MultiStageTestConfiguration{
					ClusterProfile:           clusterProfile,
					AllowBestEffortPostSteps: pointer.Bool(true),
					AllowSkipOnSuccess:       pointer.Bool(true),
					Environment:              env,
					Test: []cioperatorapi.TestStep{
						{
							LiteralTestStep: &cioperatorapi.LiteralTestStep{
								As:       "test",
								From:     sourceImageName,
								Commands: fmt.Sprintf("SKIP_MESH_AUTH_POLICY_GENERATION=true make %s", test.Command),
								Resources: cioperatorapi.ResourceRequirements{
									Requests: cioperatorapi.ResourceList{
										"cpu": "100m",
									},
								},
								Timeout:      testTimeout,
								Dependencies: dependenciesFromImages(cfg.Images, test.SkipImages),
								Cli:          "latest",
							},
						},
					},
					Post: []cioperatorapi.TestStep{
						{
							LiteralTestStep: &cioperatorapi.LiteralTestStep{
								As:       "knative-must-gather",
								From:     sourceImageName,
								Commands: `oc adm must-gather --image=quay.io/openshift-knative/must-gather --dest-dir "${ARTIFACT_DIR}/gather-knative"`,
								Resources: cioperatorapi.ResourceRequirements{
									Requests: cioperatorapi.ResourceList{
										"cpu": "100m",
									},
								},
								Timeout:           &prowapi.Duration{Duration: 20 * time.Minute},
								BestEffort:        pointer.Bool(true),
								OptionalOnSuccess: pointer.Bool(true),
								Cli:               "latest",
							},
						},
						{
							LiteralTestStep: &cioperatorapi.LiteralTestStep{
								As:       "openshift-must-gather",
								From:     sourceImageName,
								Commands: `oc adm must-gather --dest-dir "${ARTIFACT_DIR}/gather-openshift"`,
								Resources: cioperatorapi.ResourceRequirements{
									Requests: cioperatorapi.ResourceList{
										"cpu": "100m",
									},
								},
								Timeout:           &prowapi.Duration{Duration: 20 * time.Minute},
								BestEffort:        pointer.Bool(true),
								OptionalOnSuccess: pointer.Bool(true),
								Cli:               "latest",
							},
						},
						{
							LiteralTestStep: &cioperatorapi.LiteralTestStep{
								As:          "openshift-gather-extra",
								From:        sourceImageName,
								Commands:    `curl -skSL https://raw.githubusercontent.com/openshift/release/master/ci-operator/step-registry/gather/extra/gather-extra-commands.sh | /bin/bash -s`,
								GracePeriod: &prowapi.Duration{Duration: 60 * time.Second},
								Resources: cioperatorapi.ResourceRequirements{
									Requests: cioperatorapi.ResourceList{
										"cpu":    "300m",
										"memory": "300Mi",
									},
								},
								Timeout:           &prowapi.Duration{Duration: 20 * time.Minute},
								BestEffort:        pointer.Bool(true),
								OptionalOnSuccess: pointer.Bool(true),
								Cli:               "latest",
							},
						},
					},
					Workflow: workflow,
				},
			}

			if !useClusterPool {
				testConfiguration.MultiStageTestConfiguration.Post =
					append(testConfiguration.MultiStageTestConfiguration.Post,
						cioperatorapi.TestStep{
							Reference: pointer.String("ipi-deprovision-deprovision"),
						},
					)
			}

			preSubmitConfiguration := testConfiguration.DeepCopy()
			preSubmitConfiguration.Optional = test.IgnoreError
			preSubmitConfiguration.RunIfChanged = test.RunIfChanged
			cfg.Tests = append(cfg.Tests, *preSubmitConfiguration)

			// This condition allows skipping generation of periodic jobs either
			// for individual tests or, for all tests running on the given
			// OpenShift version. Periodic tests are also not generated for candidate
			// versions.
			if !test.SkipCron && !openShift.SkipCron && !openShift.CandidateRelease {
				cronTestConfiguration := testConfiguration.DeepCopy()
				cronTestConfiguration.As += "-c"
				if openShift.Cron == "" {
					// Make sure jobs start between 00:00 and 06:00 UTC by default.
					r := random.Intn(360)
					hour := r / 60
					minute := r % 60
					nightlyCron := fmt.Sprintf(cronTemplate, minute, hour)
					cronTestConfiguration.Cron = pointer.String(nightlyCron)
				} else {
					cronTestConfiguration.Cron = &openShift.Cron
				}
				// Periodic jobs gather artifacts on both failure/success.
				for _, postStep := range cronTestConfiguration.MultiStageTestConfiguration.Post {
					if postStep.LiteralTestStep != nil && strings.Contains(postStep.LiteralTestStep.As, "gather") {
						postStep.OptionalOnSuccess = pointer.Bool(false)
					}
				}
				cfg.Tests = append(cfg.Tests, *cronTestConfiguration)
			}
		}

		return nil
	}
}

func DependenciesForTestSteps() ReleaseBuildConfigurationOption {
	return func(cfg *cioperatorapi.ReleaseBuildConfiguration) error {
		for _, testConfig := range cfg.Tests {
			if testConfig.MultiStageTestConfiguration != nil {
				for _, testStep := range testConfig.MultiStageTestConfiguration.Test {
					testStep.Dependencies = dependenciesFromImages(cfg.Images, nil)
				}
			}
		}
		return nil
	}
}

const (
	shaLength = 7
)

type Test struct {
	Command      string
	OnDemand     bool
	IgnoreError  bool
	RunIfChanged string
	SkipCron     bool
	SkipImages   []string
	Timeout      *prowapi.Duration
}

func (t *Test) HexSha() string {
	h := sha1.New()
	h.Write([]byte(t.Command))
	return hex.EncodeToString(h.Sum(nil))[:shaLength]
}

func discoverE2ETests(r Repository, skipE2ETestMatch []string) ([]Test, error) {
	makefilePath := filepath.Join(r.RepositoryDirectory(), "Makefile")
	if _, err := os.Stat(makefilePath); err != nil && os.IsNotExist(err) {
		return nil, nil
	}

	mc, err := os.ReadFile(makefilePath)
	if err != nil {
		return nil, fmt.Errorf("[%s] failed to read file %s: %w", r.RepositoryDirectory(), "Makefile", err)
	}

	mcStr := string(mc)
	lines := strings.Split(mcStr, "\n")
	targets := make([]Test, 0, len(lines)/2)
	commands := sets.NewString()

	for _, l := range lines {
		l := strings.TrimSpace(l)
		for _, e2e := range r.E2ETests {
			if slices.Contains(skipE2ETestMatch, e2e.Match) {
				continue
			}
			if err := createTest(r, l, e2e, &targets, commands); err != nil {
				return nil, err
			}
		}
	}

	sort.Slice(targets, func(i, j int) bool {
		return targets[i].Command < targets[j].Command
	})

	return targets, nil
}

func createTest(r Repository, line string, e2e E2ETest, tests *[]Test, commands sets.String) error {
	if strings.HasSuffix(line, ":") {
		line := strings.TrimSuffix(line, ":")

		log.Println(r.RepositoryDirectory(), "Comparing", line, "to match", e2e.Match)

		matches, err := regexp.Match(e2e.Match, []byte(line))
		if err != nil {
			return fmt.Errorf("[%s] failed to match test %s: %w", r.RepositoryDirectory(), e2e.Match, err)
		}
		if matches && !commands.Has(line) {
			*tests = append(*tests, Test{Command: line, OnDemand: e2e.OnDemand, IgnoreError: e2e.IgnoreError, RunIfChanged: e2e.RunIfChanged, SkipCron: e2e.SkipCron, SkipImages: e2e.SkipImages, Timeout: e2e.Timeout})
			commands.Insert(line)
		}
	}

	return nil
}

func dependenciesFromImages(images []cioperatorapi.ProjectDirectoryImageBuildStepConfiguration, skipImages []string) []cioperatorapi.StepDependency {
	deps := make([]cioperatorapi.StepDependency, 0, len(images))
	for _, image := range images {
		imageFinal := strings.ReplaceAll(string(image.To), "_", "-")
		if shouldAcceptImage(skipImages, imageFinal) {
			dep := cioperatorapi.StepDependency{
				Name: imageFinal,
				Env:  strings.ToUpper(strings.ReplaceAll(string(image.To), "-", "_")),
			}
			deps = append(deps, dep)
		}
	}
	return deps
}

// Accept an image if it is not the skip image list
func shouldAcceptImage(skipImages []string, image string) bool {
	return slices.Index(skipImages, image) < 0
}
