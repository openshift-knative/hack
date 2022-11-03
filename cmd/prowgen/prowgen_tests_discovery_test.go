package main

import (
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	cioperatorapi "github.com/openshift/ci-tools/pkg/api"
	"k8s.io/apimachinery/pkg/api/equality"
	prowapi "k8s.io/test-infra/prow/apis/prowjobs/v1"
	"k8s.io/utils/pointer"
)

func TestDiscoverTests(t *testing.T) {

	r := Repository{
		Org:                   "testdata",
		Repo:                  "eventing",
		ImagePrefix:           "knative-eventing",
		CanonicalGoRepository: pointer.String("knative.dev/eventing"),
		E2ETests: E2ETests{
			Matches: []string{
				".*-conformance$",
				"test-e2e$",
				"test-reconcile.*",
				"test-conformance.*",
			},
		},
	}

	options := []ReleaseBuildConfigurationOption{
		DiscoverImages(r),
		DiscoverTests(r, "4.12"),
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
	}

	expectedTests := []cioperatorapi.TestStepConfiguration{
		{
			As: "test-conformance-aws-ocp-412",
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
							From:     "src",
							Commands: "make test-conformance",
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
			As:   "test-conformance-aws-ocp-412-continuous",
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
							From:     "src",
							Commands: "make test-conformance",
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
			As: "test-confor-7465737-aws-ocp-412",
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
							From:     "src",
							Commands: "make test-conformance-long-command",
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
			As:   "test-confor-7465737-aws-ocp-412-continuous",
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
							From:     "src",
							Commands: "make test-conformance-long-command",
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
			As: "test-e2e-aws-ocp-412",
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
							From:     "src",
							Commands: "make test-e2e",
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
			As:   "test-e2e-aws-ocp-412-continuous",
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
							From:     "src",
							Commands: "make test-e2e",
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
			As: "test-reconciler-aws-ocp-412",
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
							From:     "src",
							Commands: "make test-reconciler",
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
			As:   "test-reconciler-aws-ocp-412-continuous",
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
							From:     "src",
							Commands: "make test-reconciler",
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
		expectedTests[i].MultiStageTestConfiguration.AllowBestEffortPostSteps = pointer.Bool(true)
		expectedTests[i].MultiStageTestConfiguration.Post = append(
			expectedTests[i].MultiStageTestConfiguration.Post,
			cioperatorapi.TestStep{
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
