package testselect

import "testing"

type test struct {
	name string
	upstream bool
	paths []string
	selectedTests []string
}

func TestSelectTestsuites(t *testing.T) {
	ts := TestSuites{
		List: []TestSuite {
			{
				Name: "Always",
				//RunIfChanged: not defined
				Tests: []Test{
					{
						Name: "serverless_operator_e2e_tests",
						Upstream: false,
					},
				},
			},
			{
				Name: "Eventing Kafka",
				RunIfChanged: []string{
					"^knative-operator/pkg/controller/knativekafka/",
					"^knative-operator/pkg/webhook/knativekafka/",
				},
				Tests: []Test{
					{
						Name: "serverless_operator_kafka_e2e_tests",
						Upstream: false,
					},
					{
						Name: "downstream_knative_kafka_e2e_tests",
						Upstream: false,
					},
				},
			},
			{
				Name: "Eventing",
				RunIfChanged: []string{
					"^knative-operator/pkg/controller/knativeeventing/",
					"^knative-operator/pkg/webhook/knativeeventing/",
				},
				Tests: []Test{
					{
						Name: "downstream_eventing_e2e_tests",
						Upstream: false,
					},
					{
						Name: "upstream_knative_eventing_e2e",
						Upstream: true,
					},
				},
			},
			{
				Name: "NoTests",
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
			upstream: false,
			paths: []string{
				"knative-operator/pkg/webhook/knativeeventing/webhook_mutating.go",
				"knative-operator/pkg/webhook/knativekafka/webhook_validating.go",
			},
			selectedTests: []string{
				"downstream_eventing_e2e_tests",
				"serverless_operator_kafka_e2e_tests",
				"downstream_knative_kafka_e2e_tests",
				"serverless_operator_e2e_tests",
			},
		},
		{
			name: "Choose All",
			upstream: false,
			paths: []string{
				"hack/generate/csv.sh",
				"docs/mesh.md",
				"hack/lib/serverless.bash",
			},
			selectedTests: []string{
				"All",
			},
		},
		{
			name: "Choose None",
			upstream: false,
			paths: []string{
				"hack/generate/csv.sh",
				"docs/mesh.md",
			},
			selectedTests: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := filterTests(ts, tt.paths, tt.upstream)
			if err != nil {
				t.Error(err)
			}
			if len(result) != len(tt.selectedTests) {
				t.Fatalf("Selected tests don't match, got: %v, want: %v", result, tt.selectedTests)
			}
			for i, tst := range result {
				if tst != tt.selectedTests[i] {
					t.Fatalf("Unexpected test. Got: %s, want: %s", tst, tt.selectedTests[i])
				}
			}
		})
	}
}
