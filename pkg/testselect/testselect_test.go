package testselect

import (
	"testing"

	"github.com/google/go-cmp/cmp"
)

type test struct {
	name string
	paths []string
	selectedTests []string
}

func TestSelectTestsuites(t *testing.T) {
	ts := TestSuites{
		List: []TestSuite {
			{
				Name: "Run Always",
				// RunIfChanged: not defined
				Tests: []string{
					"serverless_operator_e2e_tests",
				},
			},
			{
				Name: "Run Eventing Kafka",
				RunIfChanged: []string{
					"^knative-operator/pkg/controller/knativekafka/",
					"^knative-operator/pkg/webhook/knativekafka/",
				},
				Tests: []string{
					"serverless_operator_kafka_e2e_tests",
					"downstream_knative_kafka_e2e_tests",
				},
			},
			{
				Name: "Run Eventing",
				RunIfChanged: []string{
					"^knative-operator/pkg/controller/knativeeventing/",
					"^knative-operator/pkg/webhook/knativeeventing/",
				},
				Tests: []string{
					"downstream_eventing_e2e_tests",
					"upstream_knative_eventing_e2e",
				},
			},
			{
				Name: "Run nothing",
				RunIfChanged: []string{
					"^hack/generate/",
					"^docs/",
				},
				// Tests: not defined
			},
		},
	}

	tests := []test{
		{
			name: "Choose Eventing, Eventing Kafka, Common",
			paths: []string{
				"knative-operator/pkg/webhook/knativeeventing/webhook_mutating.go",
				"knative-operator/pkg/webhook/knativekafka/webhook_validating.go",
			},
			selectedTests: []string{
				"downstream_eventing_e2e_tests",
				"downstream_knative_kafka_e2e_tests",
				"serverless_operator_e2e_tests",
				"serverless_operator_kafka_e2e_tests",
				"upstream_knative_eventing_e2e",
			},
		},
		{
			name: "Choose All",
			paths: []string{
				"hack/generate/csv.sh",
				"docs/mesh.md",
				"hack/lib/serverless.bash", // Entry that is not covered anywhere.
				"knative-operator/pkg/webhook/knativekafka/webhook_validating.go",
			},
			selectedTests: []string{
				"All",
			},
		},
		{
			name: "Choose None",
			paths: []string{
				"hack/generate/csv.sh",
				"docs/mesh.md",
			},
			selectedTests: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := filterTests(ts, tt.paths)
			if err != nil {
				t.Error(err)
			}
			t.Logf("Expected: %+v", tt.selectedTests)
			t.Logf("Result: %+v", result)
			diff := cmp.Diff(tt.selectedTests, result)
			if diff != "" {
				t.Errorf("Unexpected tests (-want, +got): \n%s", diff)
			}
		})
	}
}
