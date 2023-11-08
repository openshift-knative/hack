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
		E2ETests: []E2ETest{
			{
				Regexp: "test-e2e$",
			},
			{
				Regexp: "test-e2e-tls$",
			},
			{
				Regexp: "perf-tests$",
			},
		},
	}

	cron := pointer.String("0 8 * * 1-5")

	servingSourceImage := "knative-serving-source-image"
	options := []ReleaseBuildConfigurationOption{
		DiscoverImages(r),
		DiscoverTests(r, "4.12", cron, servingSourceImage),
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

	expectedTests := []cioperatorapi.TestStepConfiguration{
		{
			As: "perf-tests-aws-ocp-412",
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
							Dependencies: dependencies,
							Cli:          "latest",
						},
					},
				},
				Workflow: pointer.String("generic-claim"),
			},
		},
		{
			As:   "perf-tests-aws-ocp-412-continuous",
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
							Commands: formatCommand("make perf-tests"),
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
			As:   "test-e2e-aws-ocp-412-continuous",
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
			As: "test-e2e-tls-aws-ocp-412",
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
			As:   "test-e2e-tls-aws-ocp-412-continuous",
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
				Regexp: ".*-conformance$",
			},
			{
				Regexp: "test-e2e$",
			},
			{
				Regexp: "test-reconcile.*",
			},
			{
				Regexp: "test-conformance.*",
			},
		},
	}

	eventingSourceImage := "knative-eventing-source-image"
	options := []ReleaseBuildConfigurationOption{
		DiscoverImages(r),
		DiscoverTests(r, "4.12", nil, eventingSourceImage),
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
			As: "test-confor-2627121-aws-ocp-412",
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
							Commands: formatCommand("make test-conformance-long-command"),
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
			As:   "test-confor-2627121-aws-ocp-412-continuous",
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
							Commands: formatCommand("make test-conformance-long-command"),
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
