package prowgen

import (
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"io/ioutil"
	"log"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	cioperatorapi "github.com/openshift/ci-tools/pkg/api"
	"k8s.io/apimachinery/pkg/util/sets"
	prowapi "k8s.io/test-infra/prow/apis/prowjobs/v1"
	"k8s.io/utils/pointer"
)

func DiscoverTests(r Repository, openShiftVersion string) ReleaseBuildConfigurationOption {
	return func(cfg *cioperatorapi.ReleaseBuildConfiguration) error {
		tests, err := discoverE2ETests(r)
		if err != nil {
			return err
		}

		for i := range tests {
			test := &tests[i]
			as := ToName(r, test, openShiftVersion)
			testConfiguration := cioperatorapi.TestStepConfiguration{
				As: as,
				ClusterClaim: &cioperatorapi.ClusterClaim{
					Product:      cioperatorapi.ReleaseProductOCP,
					Version:      openShiftVersion,
					Architecture: cioperatorapi.ReleaseArchitectureAMD64,
					Cloud:        cioperatorapi.CloudAWS,
					Owner:        "openshift-ci",
					Timeout:      &prowapi.Duration{Duration: time.Hour},
				},
				MultiStageTestConfiguration: &cioperatorapi.MultiStageTestConfiguration{
					AllowBestEffortPostSteps: pointer.Bool(true),
					Test: []cioperatorapi.TestStep{
						{
							LiteralTestStep: &cioperatorapi.LiteralTestStep{
								As:       "test",
								From:     "src",
								Commands: fmt.Sprintf("make %s", test.Command),
								Resources: cioperatorapi.ResourceRequirements{
									Requests: cioperatorapi.ResourceList{
										"cpu": "100m",
									},
								},
								Timeout:      &prowapi.Duration{Duration: 4 * time.Hour},
								Dependencies: dependenciesFromImages(cfg.Images),
								Cli:          "latest",
							},
						},
					},
					Post: []cioperatorapi.TestStep{
						{
							LiteralTestStep: &cioperatorapi.LiteralTestStep{
								As:       "knative-must-gather",
								From:     "src",
								Commands: `oc adm must-gather --image=quay.io/openshift-knative/must-gather --dest-dir "${ARTIFACT_DIR}/gather-knative"`,
								Resources: cioperatorapi.ResourceRequirements{
									Requests: cioperatorapi.ResourceList{
										"cpu": "100m",
									},
								},
								Timeout:    &prowapi.Duration{Duration: 20 * time.Minute},
								BestEffort: pointer.Bool(true),
								Cli:        "latest",
							},
						},
					},
					Workflow: pointer.String("generic-claim"),
				},
			}
			cfg.Tests = append(cfg.Tests, testConfiguration)

			cronTestConfiguration := testConfiguration.DeepCopy()
			cronTestConfiguration.As += "-continuous"
			cronTestConfiguration.Cron = pointer.String("0 5 * * 2,6")

			cfg.Tests = append(cfg.Tests, *cronTestConfiguration)
		}

		return nil
	}
}

const (
	shaLength = 7
)

type Test struct {
	Command string
}

func (t *Test) HexSha() string {
	return hex.EncodeToString(sha1.New().Sum([]byte(t.Command)))[:shaLength]
}

func discoverE2ETests(r Repository) ([]Test, error) {
	mc, err := ioutil.ReadFile(filepath.Join(r.RepositoryDirectory(), "Makefile"))
	if err != nil {
		return nil, fmt.Errorf("[%s] failed to read file %s: %w", r.RepositoryDirectory(), "Makefile", err)
	}

	mcStr := string(mc)
	lines := strings.Split(mcStr, "\n")
	targets := make([]Test, 0, len(lines)/2)
	commands := sets.NewString()
	for _, l := range lines {
		l := strings.TrimSpace(l)
		for _, match := range r.E2ETests.Matches {
			if err := createTest(r, l, match, &targets, commands); err != nil {
				return nil, err
			}
		}
	}

	sort.Slice(targets, func(i, j int) bool {
		return targets[i].Command < targets[j].Command
	})

	return targets, nil
}

func createTest(r Repository, line string, shouldMatch string, tests *[]Test, commands sets.String) error {
	if strings.HasSuffix(line, ":") {
		line := strings.TrimSuffix(line, ":")

		log.Println(r.RepositoryDirectory(), "Comparing", line, "to match", shouldMatch)

		matches, err := regexp.Match(shouldMatch, []byte(line))
		if err != nil {
			return fmt.Errorf("[%s] failed to match test %s: %w", r.RepositoryDirectory(), shouldMatch, err)
		}
		if matches && !commands.Has(line) {
			*tests = append(*tests, Test{Command: line})
			commands.Insert(line)
		}
	}

	return nil
}

func dependenciesFromImages(images []cioperatorapi.ProjectDirectoryImageBuildStepConfiguration) []cioperatorapi.StepDependency {
	deps := make([]cioperatorapi.StepDependency, 0, len(images))
	for _, image := range images {
		dep := cioperatorapi.StepDependency{
			Name: strings.ReplaceAll(string(image.To), "_", "-"),
			Env:  strings.ToUpper(strings.ReplaceAll(string(image.To), "-", "_")),
		}
		deps = append(deps, dep)
	}
	return deps
}
