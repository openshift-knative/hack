package prowgen

import (
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"log"
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
)

const defaultCron = "0 5 * * 2,6"

func DiscoverTests(r Repository, openShift OpenShift, sourceImageName string) ReleaseBuildConfigurationOption {
	return func(cfg *cioperatorapi.ReleaseBuildConfiguration) error {
		tests, err := discoverE2ETests(r)
		if err != nil {
			return err
		}

		for i := range tests {
			test := &tests[i]
			as := ToName(r, test, openShift.Version)
			testConfiguration := cioperatorapi.TestStepConfiguration{
				As: as,
				ClusterClaim: &cioperatorapi.ClusterClaim{
					Product:      cioperatorapi.ReleaseProductOCP,
					Version:      openShift.Version,
					Architecture: cioperatorapi.ReleaseArchitectureAMD64,
					Cloud:        cioperatorapi.CloudAWS,
					Owner:        "openshift-ci",
					Timeout:      &prowapi.Duration{Duration: time.Hour},
				},
				MultiStageTestConfiguration: &cioperatorapi.MultiStageTestConfiguration{
					AllowBestEffortPostSteps: pointer.Bool(true),
					AllowSkipOnSuccess:       pointer.Bool(true),
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
					Workflow: pointer.String("generic-claim"),
				},
			}

			preSubmitConfiguration := testConfiguration.DeepCopy()
			preSubmitConfiguration.Optional = test.IgnoreError
			preSubmitConfiguration.RunIfChanged = test.RunIfChanged
			cfg.Tests = append(cfg.Tests, *preSubmitConfiguration)

			if !test.SkipCron {
				cronTestConfiguration := testConfiguration.DeepCopy()
				cronTestConfiguration.As += "-continuous"
				if openShift.Cron == "" {
					cronTestConfiguration.Cron = pointer.String(defaultCron)
				} else {
					cronTestConfiguration.Cron = &openShift.Cron
				}
				// Periodic jobs gather artifacts on both failure/success.
				for _, postStep := range cronTestConfiguration.MultiStageTestConfiguration.Post {
					postStep.OptionalOnSuccess = pointer.Bool(false)
				}
				cfg.Tests = append(cfg.Tests, *cronTestConfiguration)
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
}

func (t *Test) HexSha() string {
	h := sha1.New()
	h.Write([]byte(t.Command))
	return hex.EncodeToString(h.Sum(nil))[:shaLength]
}

func discoverE2ETests(r Repository) ([]Test, error) {
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
			*tests = append(*tests, Test{Command: line, OnDemand: e2e.OnDemand, IgnoreError: e2e.IgnoreError, RunIfChanged: e2e.RunIfChanged, SkipCron: e2e.SkipCron})
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
