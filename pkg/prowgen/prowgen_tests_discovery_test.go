package prowgen

import (
	"fmt"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	cioperatorapi "github.com/openshift/ci-tools/pkg/api"
	"k8s.io/apimachinery/pkg/api/equality"
	prowapi "k8s.io/test-infra/prow/apis/prowjobs/v1"
	"k8s.io/utils/pointer"
)

func TestDiscoverTestsServing(t *testing.T) {
	r := Repository{
		Org:                   "testdata",
		Repo:                  "serving",
		ImagePrefix:           "knative-serving",
		ImageNameOverrides:    map[string]string{"migrate": "storage-version-migration"},
		CanonicalGoRepository: pointer.String("knative.dev/serving"),

		Dockerfiles: Dockerfiles{
			Matches: []string{
				"knative-perf-images/.*",
				"knative-images/.*",
				"knative-test-images/.*",
				"skip-images/.*",
			},
		},

		E2ETests: []E2ETest{
			{
				Match:       "test-e2e$",
				IgnoreError: true,
				SkipImages: []string{
					"knative-serving-scale-from-zero",
				},
			},
			{
				Match: "test-e2e-tls$",
				SkipImages: []string{
					"knative-serving-scale-from-zero",
				},
			},
			{
				Match:    "perf-tests$",
				SkipCron: true, // The "-continuous" variant should not be generated.
			},
			{
				Match: "skip-e2e$",
			},
			{
				Match: "ui-e2e$",
				SkipImages: []string{
					"knative-serving-scale-from-zero",
				},
				RunIfChanged: "test/ui",
				SkipCron:     true,
			}},
	}

	cron := pointer.String("0 8 * * 1-5")

	servingSourceImage := "knative-serving-source-image"
	options := []ReleaseBuildConfigurationOption{
		DiscoverImages(r, []string{"skip-images/.*"}),
		DiscoverTests(r, OpenShift{Version: "4.12", Cron: *cron}, servingSourceImage, []string{"skip-e2e$"}),
	}

	dependencies := []cioperatorapi.StepDependency{
		{
			Name: "knative-serving-autoscaler",
			Env:  "KNATIVE_SERVING_AUTOSCALER",
		},
		{
			Name: "knative-serving-storage-version-migration",
			Env:  "KNATIVE_SERVING_STORAGE_VERSION_MIGRATION",
		},
		{
			Name: "knative-serving-test-webhook",
			Env:  "KNATIVE_SERVING_TEST_WEBHOOK",
		},
		{
			Name: servingSourceImage,
			Env:  "KNATIVE_SERVING_SOURCE_IMAGE",
		},
	}

	perfDependencies := append(dependencies, cioperatorapi.StepDependency{})
	copy(perfDependencies[3:], perfDependencies[2:])
	perfDependencies[2] = cioperatorapi.StepDependency{
		Name: "knative-serving-scale-from-zero",
		Env:  "KNATIVE_SERVING_SCALE_FROM_ZERO",
	}

	expectedTests := []cioperatorapi.TestStepConfiguration{
		{
			As: "perf-tests-aws-412",
			ClusterClaim: &cioperatorapi.ClusterClaim{
				Product:      cioperatorapi.ReleaseProductOCP,
				Version:      "4.12",
				Architecture: cioperatorapi.ReleaseArchitectureAMD64,
				Cloud:        cioperatorapi.CloudAWS,
				Owner:        "openshift-ci",
				Timeout:      &prowapi.Duration{Duration: time.Hour},
			},
			MultiStageTestConfiguration: &cioperatorapi.MultiStageTestConfiguration{
				Test: []cioperatorapi.TestStep{
					{
						LiteralTestStep: &cioperatorapi.LiteralTestStep{
							As:       "test",
							From:     servingSourceImage,
							Commands: formatCommand("make perf-tests"),
							Resources: cioperatorapi.ResourceRequirements{
								Requests: cioperatorapi.ResourceList{
									"cpu": "100m",
								},
							},
							Timeout:      &prowapi.Duration{Duration: 4 * time.Hour},
							Dependencies: perfDependencies,
							Cli:          "latest",
						},
					},
				},
				Workflow: pointer.String("generic-claim"),
			},
		},
		{
			As: "test-e2e-aws-412",
			ClusterClaim: &cioperatorapi.ClusterClaim{
				Product:      cioperatorapi.ReleaseProductOCP,
				Version:      "4.12",
				Architecture: cioperatorapi.ReleaseArchitectureAMD64,
				Cloud:        cioperatorapi.CloudAWS,
				Owner:        "openshift-ci",
				Timeout:      &prowapi.Duration{Duration: time.Hour},
			},
			Optional: true,
			MultiStageTestConfiguration: &cioperatorapi.MultiStageTestConfiguration{
				Test: []cioperatorapi.TestStep{
					{
						LiteralTestStep: &cioperatorapi.LiteralTestStep{
							As:       "test",
							From:     servingSourceImage,
							Commands: formatCommand("make test-e2e"),
							Resources: cioperatorapi.ResourceRequirements{
								Requests: cioperatorapi.ResourceList{
									"cpu": "100m",
								},
							},
							Timeout:      &prowapi.Duration{Duration: 4 * time.Hour},
							Dependencies: dependencies,
							Cli:          "latest",
						},
					},
				},
				Workflow: pointer.String("generic-claim"),
			},
		},
		{
			As:   "test-e2e-aws-412-c",
			Cron: cron,
			ClusterClaim: &cioperatorapi.ClusterClaim{
				Product:      cioperatorapi.ReleaseProductOCP,
				Version:      "4.12",
				Architecture: cioperatorapi.ReleaseArchitectureAMD64,
				Cloud:        cioperatorapi.CloudAWS,
				Owner:        "openshift-ci",
				Timeout:      &prowapi.Duration{Duration: time.Hour},
			},
			MultiStageTestConfiguration: &cioperatorapi.MultiStageTestConfiguration{
				Test: []cioperatorapi.TestStep{
					{
						LiteralTestStep: &cioperatorapi.LiteralTestStep{
							As:       "test",
							From:     servingSourceImage,
							Commands: formatCommand("make test-e2e"),
							Resources: cioperatorapi.ResourceRequirements{
								Requests: cioperatorapi.ResourceList{
									"cpu": "100m",
								},
							},
							Timeout:      &prowapi.Duration{Duration: 4 * time.Hour},
							Dependencies: dependencies,
							Cli:          "latest",
						},
					},
				},
				Workflow: pointer.String("generic-claim"),
			},
		},
		{
			As: "test-e2e-tls-aws-412",
			ClusterClaim: &cioperatorapi.ClusterClaim{
				Product:      cioperatorapi.ReleaseProductOCP,
				Version:      "4.12",
				Architecture: cioperatorapi.ReleaseArchitectureAMD64,
				Cloud:        cioperatorapi.CloudAWS,
				Owner:        "openshift-ci",
				Timeout:      &prowapi.Duration{Duration: time.Hour},
			},
			MultiStageTestConfiguration: &cioperatorapi.MultiStageTestConfiguration{
				Test: []cioperatorapi.TestStep{
					{
						LiteralTestStep: &cioperatorapi.LiteralTestStep{
							As:       "test",
							From:     servingSourceImage,
							Commands: formatCommand("make test-e2e-tls"),
							Resources: cioperatorapi.ResourceRequirements{
								Requests: cioperatorapi.ResourceList{
									"cpu": "100m",
								},
							},
							Timeout:      &prowapi.Duration{Duration: 4 * time.Hour},
							Dependencies: dependencies,
							Cli:          "latest",
						},
					},
				},
				Workflow: pointer.String("generic-claim"),
			},
		},
		{
			As:   "test-e2e-tls-aws-412-c",
			Cron: cron,
			ClusterClaim: &cioperatorapi.ClusterClaim{
				Product:      cioperatorapi.ReleaseProductOCP,
				Version:      "4.12",
				Architecture: cioperatorapi.ReleaseArchitectureAMD64,
				Cloud:        cioperatorapi.CloudAWS,
				Owner:        "openshift-ci",
				Timeout:      &prowapi.Duration{Duration: time.Hour},
			},
			MultiStageTestConfiguration: &cioperatorapi.MultiStageTestConfiguration{
				Test: []cioperatorapi.TestStep{
					{
						LiteralTestStep: &cioperatorapi.LiteralTestStep{
							As:       "test",
							From:     servingSourceImage,
							Commands: formatCommand("make test-e2e-tls"),
							Resources: cioperatorapi.ResourceRequirements{
								Requests: cioperatorapi.ResourceList{
									"cpu": "100m",
								},
							},
							Timeout:      &prowapi.Duration{Duration: 4 * time.Hour},
							Dependencies: dependencies,
							Cli:          "latest",
						},
					},
				},
				Workflow: pointer.String("generic-claim"),
			},
		},
		{
			As: "ui-e2e-aws-412",
			ClusterClaim: &cioperatorapi.ClusterClaim{
				Product:      cioperatorapi.ReleaseProductOCP,
				Version:      "4.12",
				Architecture: cioperatorapi.ReleaseArchitectureAMD64,
				Cloud:        cioperatorapi.CloudAWS,
				Owner:        "openshift-ci",
				Timeout:      &prowapi.Duration{Duration: time.Hour},
			},
			RunIfChanged: "test/ui",
			MultiStageTestConfiguration: &cioperatorapi.MultiStageTestConfiguration{
				Test: []cioperatorapi.TestStep{
					{
						LiteralTestStep: &cioperatorapi.LiteralTestStep{
							As:       "test",
							From:     servingSourceImage,
							Commands: formatCommand("make ui-e2e"),
							Resources: cioperatorapi.ResourceRequirements{
								Requests: cioperatorapi.ResourceList{
									"cpu": "100m",
								},
							},
							Timeout:      &prowapi.Duration{Duration: 4 * time.Hour},
							Dependencies: dependencies,
							Cli:          "latest",
						},
					},
				},
				Workflow: pointer.String("generic-claim"),
			},
		},
	}

	// Add must-gather step to each test as post step
	for i := range expectedTests {
		optionalOnSuccess := true
		if expectedTests[i].Cron != nil {
			optionalOnSuccess = false
		}
		expectedTests[i].MultiStageTestConfiguration.AllowBestEffortPostSteps = pointer.Bool(true)
		expectedTests[i].MultiStageTestConfiguration.AllowSkipOnSuccess = pointer.Bool(true)
		expectedTests[i].MultiStageTestConfiguration.Post = append(
			expectedTests[i].MultiStageTestConfiguration.Post,
			cioperatorapi.TestStep{
				LiteralTestStep: &cioperatorapi.LiteralTestStep{
					As:       "knative-must-gather",
					From:     servingSourceImage,
					Commands: `oc adm must-gather --image=quay.io/openshift-knative/must-gather --dest-dir "${ARTIFACT_DIR}/gather-knative"`,
					Resources: cioperatorapi.ResourceRequirements{
						Requests: cioperatorapi.ResourceList{
							"cpu": "100m",
						},
					},
					Timeout:           &prowapi.Duration{Duration: 20 * time.Minute},
					BestEffort:        pointer.Bool(true),
					OptionalOnSuccess: &optionalOnSuccess,
					Cli:               "latest",
				},
			},
			cioperatorapi.TestStep{
				LiteralTestStep: &cioperatorapi.LiteralTestStep{
					As:       "openshift-must-gather",
					From:     servingSourceImage,
					Commands: `oc adm must-gather --dest-dir "${ARTIFACT_DIR}/gather-openshift"`,
					Resources: cioperatorapi.ResourceRequirements{
						Requests: cioperatorapi.ResourceList{
							"cpu": "100m",
						},
					},
					Timeout:           &prowapi.Duration{Duration: 20 * time.Minute},
					BestEffort:        pointer.Bool(true),
					OptionalOnSuccess: &optionalOnSuccess,
					Cli:               "latest",
				},
			},
			cioperatorapi.TestStep{
				LiteralTestStep: &cioperatorapi.LiteralTestStep{
					As:          "openshift-gather-extra",
					From:        servingSourceImage,
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
					OptionalOnSuccess: &optionalOnSuccess,
					Cli:               "latest",
				},
			},
		)
	}

	cfg := cioperatorapi.ReleaseBuildConfiguration{}

	if err := applyOptions(&cfg, options...); err != nil {
		t.Fatal(err)
	}

	if !equality.Semantic.DeepEqual(expectedTests, cfg.Tests) {
		diff := cmp.Diff(expectedTests, cfg.Tests)
		t.Errorf("Unexpected tests (-want, +got): \n%s", diff)
	}
}

func TestDiscoverTestsEventing(t *testing.T) {

	r := Repository{
		Org:                   "testdata",
		Repo:                  "eventing",
		ImagePrefix:           "knative-eventing",
		CanonicalGoRepository: pointer.String("knative.dev/eventing"),
		E2ETests: []E2ETest{
			{
				Match: ".*-conformance$",
			},
			{
				Match: "test-e2e$",
			},
			{
				Match: "test-reconcile.*",
			},
			{
				Match: "test-conformance.*",
			},
		},
	}

	eventingSourceImage := "knative-eventing-source-image"
	options := []ReleaseBuildConfigurationOption{
		DiscoverImages(r, nil),
		DiscoverTests(r, OpenShift{Version: "4.12"}, eventingSourceImage, nil),
	}

	dependencies := []cioperatorapi.StepDependency{
		{
			Name: "knative-eventing-dispatcher",
			Env:  "KNATIVE_EVENTING_DISPATCHER",
		},
		{
			Name: "knative-eventing-test-webhook",
			Env:  "KNATIVE_EVENTING_TEST_WEBHOOK",
		},
		{
			Name: eventingSourceImage,
			Env:  "KNATIVE_EVENTING_SOURCE_IMAGE",
		},
	}

	expectedTests := []cioperatorapi.TestStepConfiguration{
		{
			As: "test-conformance-aws-412",
			ClusterClaim: &cioperatorapi.ClusterClaim{
				Product:      cioperatorapi.ReleaseProductOCP,
				Version:      "4.12",
				Architecture: cioperatorapi.ReleaseArchitectureAMD64,
				Cloud:        cioperatorapi.CloudAWS,
				Owner:        "openshift-ci",
				Timeout:      &prowapi.Duration{Duration: time.Hour},
			},
			MultiStageTestConfiguration: &cioperatorapi.MultiStageTestConfiguration{
				Test: []cioperatorapi.TestStep{
					{
						LiteralTestStep: &cioperatorapi.LiteralTestStep{
							As:       "test",
							From:     eventingSourceImage,
							Commands: formatCommand("make test-conformance"),
							Resources: cioperatorapi.ResourceRequirements{
								Requests: cioperatorapi.ResourceList{
									"cpu": "100m",
								},
							},
							Timeout:      &prowapi.Duration{Duration: 4 * time.Hour},
							Dependencies: dependencies,
							Cli:          "latest",
						},
					},
				},
				Workflow: pointer.String("generic-claim"),
			},
		},
		{
			As:   "test-conformance-aws-412-c",
			Cron: pointer.String("0 5 * * 2,6"),
			ClusterClaim: &cioperatorapi.ClusterClaim{
				Product:      cioperatorapi.ReleaseProductOCP,
				Version:      "4.12",
				Architecture: cioperatorapi.ReleaseArchitectureAMD64,
				Cloud:        cioperatorapi.CloudAWS,
				Owner:        "openshift-ci",
				Timeout:      &prowapi.Duration{Duration: time.Hour},
			},
			MultiStageTestConfiguration: &cioperatorapi.MultiStageTestConfiguration{
				Test: []cioperatorapi.TestStep{
					{
						LiteralTestStep: &cioperatorapi.LiteralTestStep{
							As:       "test",
							From:     eventingSourceImage,
							Commands: formatCommand("make test-conformance"),
							Resources: cioperatorapi.ResourceRequirements{
								Requests: cioperatorapi.ResourceList{
									"cpu": "100m",
								},
							},
							Timeout:      &prowapi.Duration{Duration: 4 * time.Hour},
							Dependencies: dependencies,
							Cli:          "latest",
						},
					},
				},
				Workflow: pointer.String("generic-claim"),
			},
		},
		{
			As: "test-conformance-long-lo-510e96a-aws-412",
			ClusterClaim: &cioperatorapi.ClusterClaim{
				Product:      cioperatorapi.ReleaseProductOCP,
				Version:      "4.12",
				Architecture: cioperatorapi.ReleaseArchitectureAMD64,
				Cloud:        cioperatorapi.CloudAWS,
				Owner:        "openshift-ci",
				Timeout:      &prowapi.Duration{Duration: time.Hour},
			},
			MultiStageTestConfiguration: &cioperatorapi.MultiStageTestConfiguration{
				Test: []cioperatorapi.TestStep{
					{
						LiteralTestStep: &cioperatorapi.LiteralTestStep{
							As:       "test",
							From:     eventingSourceImage,
							Commands: formatCommand("make test-conformance-long-long-long-command"),
							Resources: cioperatorapi.ResourceRequirements{
								Requests: cioperatorapi.ResourceList{
									"cpu": "100m",
								},
							},
							Timeout:      &prowapi.Duration{Duration: 4 * time.Hour},
							Dependencies: dependencies,
							Cli:          "latest",
						},
					},
				},
				Workflow: pointer.String("generic-claim"),
			},
		},
		{
			As:   "test-conformance-long-lo-510e96a-aws-412-c",
			Cron: pointer.String("0 5 * * 2,6"),
			ClusterClaim: &cioperatorapi.ClusterClaim{
				Product:      cioperatorapi.ReleaseProductOCP,
				Version:      "4.12",
				Architecture: cioperatorapi.ReleaseArchitectureAMD64,
				Cloud:        cioperatorapi.CloudAWS,
				Owner:        "openshift-ci",
				Timeout:      &prowapi.Duration{Duration: time.Hour},
			},
			MultiStageTestConfiguration: &cioperatorapi.MultiStageTestConfiguration{
				Test: []cioperatorapi.TestStep{
					{
						LiteralTestStep: &cioperatorapi.LiteralTestStep{
							As:       "test",
							From:     eventingSourceImage,
							Commands: formatCommand("make test-conformance-long-long-long-command"),
							Resources: cioperatorapi.ResourceRequirements{
								Requests: cioperatorapi.ResourceList{
									"cpu": "100m",
								},
							},
							Timeout:      &prowapi.Duration{Duration: 4 * time.Hour},
							Dependencies: dependencies,
							Cli:          "latest",
						},
					},
				},
				Workflow: pointer.String("generic-claim"),
			},
		},
		{
			As: "test-e2e-aws-412",
			ClusterClaim: &cioperatorapi.ClusterClaim{
				Product:      cioperatorapi.ReleaseProductOCP,
				Version:      "4.12",
				Architecture: cioperatorapi.ReleaseArchitectureAMD64,
				Cloud:        cioperatorapi.CloudAWS,
				Owner:        "openshift-ci",
				Timeout:      &prowapi.Duration{Duration: time.Hour},
			},
			MultiStageTestConfiguration: &cioperatorapi.MultiStageTestConfiguration{
				Test: []cioperatorapi.TestStep{
					{
						LiteralTestStep: &cioperatorapi.LiteralTestStep{
							As:       "test",
							From:     eventingSourceImage,
							Commands: formatCommand("make test-e2e"),
							Resources: cioperatorapi.ResourceRequirements{
								Requests: cioperatorapi.ResourceList{
									"cpu": "100m",
								},
							},
							Timeout:      &prowapi.Duration{Duration: 4 * time.Hour},
							Dependencies: dependencies,
							Cli:          "latest",
						},
					},
				},
				Workflow: pointer.String("generic-claim"),
			},
		},
		{
			As:   "test-e2e-aws-412-c",
			Cron: pointer.String("0 5 * * 2,6"),
			ClusterClaim: &cioperatorapi.ClusterClaim{
				Product:      cioperatorapi.ReleaseProductOCP,
				Version:      "4.12",
				Architecture: cioperatorapi.ReleaseArchitectureAMD64,
				Cloud:        cioperatorapi.CloudAWS,
				Owner:        "openshift-ci",
				Timeout:      &prowapi.Duration{Duration: time.Hour},
			},
			MultiStageTestConfiguration: &cioperatorapi.MultiStageTestConfiguration{
				Test: []cioperatorapi.TestStep{
					{
						LiteralTestStep: &cioperatorapi.LiteralTestStep{
							As:       "test",
							From:     eventingSourceImage,
							Commands: formatCommand("make test-e2e"),
							Resources: cioperatorapi.ResourceRequirements{
								Requests: cioperatorapi.ResourceList{
									"cpu": "100m",
								},
							},
							Timeout:      &prowapi.Duration{Duration: 4 * time.Hour},
							Dependencies: dependencies,
							Cli:          "latest",
						},
					},
				},
				Workflow: pointer.String("generic-claim"),
			},
		},
		{
			As: "test-reconciler-aws-412",
			ClusterClaim: &cioperatorapi.ClusterClaim{
				Product:      cioperatorapi.ReleaseProductOCP,
				Version:      "4.12",
				Architecture: cioperatorapi.ReleaseArchitectureAMD64,
				Cloud:        cioperatorapi.CloudAWS,
				Owner:        "openshift-ci",
				Timeout:      &prowapi.Duration{Duration: time.Hour},
			},
			MultiStageTestConfiguration: &cioperatorapi.MultiStageTestConfiguration{
				Test: []cioperatorapi.TestStep{
					{
						LiteralTestStep: &cioperatorapi.LiteralTestStep{
							As:       "test",
							From:     eventingSourceImage,
							Commands: formatCommand("make test-reconciler"),
							Resources: cioperatorapi.ResourceRequirements{
								Requests: cioperatorapi.ResourceList{
									"cpu": "100m",
								},
							},
							Timeout:      &prowapi.Duration{Duration: 4 * time.Hour},
							Dependencies: dependencies,
							Cli:          "latest",
						},
					},
				},
				Workflow: pointer.String("generic-claim"),
			},
		},
		{
			As:   "test-reconciler-aws-412-c",
			Cron: pointer.String("0 5 * * 2,6"),
			ClusterClaim: &cioperatorapi.ClusterClaim{
				Product:      cioperatorapi.ReleaseProductOCP,
				Version:      "4.12",
				Architecture: cioperatorapi.ReleaseArchitectureAMD64,
				Cloud:        cioperatorapi.CloudAWS,
				Owner:        "openshift-ci",
				Timeout:      &prowapi.Duration{Duration: time.Hour},
			},
			MultiStageTestConfiguration: &cioperatorapi.MultiStageTestConfiguration{
				Test: []cioperatorapi.TestStep{
					{
						LiteralTestStep: &cioperatorapi.LiteralTestStep{
							As:       "test",
							From:     eventingSourceImage,
							Commands: formatCommand("make test-reconciler"),
							Resources: cioperatorapi.ResourceRequirements{
								Requests: cioperatorapi.ResourceList{
									"cpu": "100m",
								},
							},
							Timeout:      &prowapi.Duration{Duration: 4 * time.Hour},
							Dependencies: dependencies,
							Cli:          "latest",
						},
					},
				},
				Workflow: pointer.String("generic-claim"),
			},
		},
	}

	// Add must-gather step to each test as post step
	for i := range expectedTests {
		optionalOnSuccess := true
		if expectedTests[i].Cron != nil {
			optionalOnSuccess = false
		}
		expectedTests[i].MultiStageTestConfiguration.AllowBestEffortPostSteps = pointer.Bool(true)
		expectedTests[i].MultiStageTestConfiguration.AllowSkipOnSuccess = pointer.Bool(true)
		expectedTests[i].MultiStageTestConfiguration.Post = append(
			expectedTests[i].MultiStageTestConfiguration.Post,
			cioperatorapi.TestStep{
				LiteralTestStep: &cioperatorapi.LiteralTestStep{
					As:       "knative-must-gather",
					From:     eventingSourceImage,
					Commands: `oc adm must-gather --image=quay.io/openshift-knative/must-gather --dest-dir "${ARTIFACT_DIR}/gather-knative"`,
					Resources: cioperatorapi.ResourceRequirements{
						Requests: cioperatorapi.ResourceList{
							"cpu": "100m",
						},
					},
					Timeout:           &prowapi.Duration{Duration: 20 * time.Minute},
					BestEffort:        pointer.Bool(true),
					OptionalOnSuccess: &optionalOnSuccess,
					Cli:               "latest",
				},
			},
			cioperatorapi.TestStep{
				LiteralTestStep: &cioperatorapi.LiteralTestStep{
					As:       "openshift-must-gather",
					From:     eventingSourceImage,
					Commands: `oc adm must-gather --dest-dir "${ARTIFACT_DIR}/gather-openshift"`,
					Resources: cioperatorapi.ResourceRequirements{
						Requests: cioperatorapi.ResourceList{
							"cpu": "100m",
						},
					},
					Timeout:           &prowapi.Duration{Duration: 20 * time.Minute},
					BestEffort:        pointer.Bool(true),
					OptionalOnSuccess: &optionalOnSuccess,
					Cli:               "latest",
				},
			},
			cioperatorapi.TestStep{
				LiteralTestStep: &cioperatorapi.LiteralTestStep{
					As:          "openshift-gather-extra",
					From:        eventingSourceImage,
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
					OptionalOnSuccess: &optionalOnSuccess,
					Cli:               "latest",
				},
			},
		)
	}

	cfg := cioperatorapi.ReleaseBuildConfiguration{}

	if err := applyOptions(&cfg, options...); err != nil {
		t.Fatal(err)
	}

	if !equality.Semantic.DeepEqual(expectedTests, cfg.Tests) {
		diff := cmp.Diff(expectedTests, cfg.Tests)
		t.Errorf("Unexpected tests (-want, +got): \n%s", diff)
	}
}

func formatCommand(cmd string) string {
	return fmt.Sprintf("SKIP_MESH_AUTH_POLICY_GENERATION=true %s", cmd)
}
