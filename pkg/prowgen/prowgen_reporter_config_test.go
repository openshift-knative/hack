package prowgen

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	"sigs.k8s.io/yaml"
)

func TestAddReporterConfigToTests(t *testing.T) {
	tests := []struct {
		name         string
		inputYAML    string
		slackChannel string
		wantErr      bool
		check        func(t *testing.T, out []byte)
	}{
		{
			name:         "adds reporter_config to cron tests only",
			slackChannel: "#knative-eventing-ci",
			inputYAML: `tests:
- as: test-e2e
- as: test-e2e-c
  cron: 23 1 * * 2,6
`,
			check: func(t *testing.T, out []byte) {
				var result map[string]interface{}
				if err := yaml.Unmarshal(out, &result); err != nil {
					t.Fatalf("failed to unmarshal output: %v", err)
				}
				tests := result["tests"].([]interface{})
				if len(tests) != 2 {
					t.Fatalf("expected 2 tests, got %d", len(tests))
				}

				// Non-cron test should not have reporter_config
				presubmit := tests[0].(map[string]interface{})
				if _, ok := presubmit["reporter_config"]; ok {
					t.Error("non-cron test should not have reporter_config")
				}

				// Cron test should have reporter_config
				cronTest := tests[1].(map[string]interface{})
				rc, ok := cronTest["reporter_config"].(map[string]interface{})
				if !ok {
					t.Fatal("cron test missing reporter_config")
				}
				if diff := cmp.Diff("#knative-eventing-ci", rc["channel"]); diff != "" {
					t.Errorf("channel mismatch (-want +got):\n%s", diff)
				}
				states := rc["job_states_to_report"].([]interface{})
				if diff := cmp.Diff([]interface{}{"success", "failure", "error"}, states); diff != "" {
					t.Errorf("job_states_to_report mismatch (-want +got):\n%s", diff)
				}
				if rc["report_template"] != slackReportTemplate {
					t.Errorf("report_template mismatch: got %q", rc["report_template"])
				}
			},
		},
		{
			name:         "no cron tests returns unchanged yaml",
			slackChannel: "#knative-eventing-ci",
			inputYAML: `tests:
- as: test-e2e
`,
			check: func(t *testing.T, out []byte) {
				var result map[string]interface{}
				if err := yaml.Unmarshal(out, &result); err != nil {
					t.Fatalf("failed to unmarshal output: %v", err)
				}
				test := result["tests"].([]interface{})[0].(map[string]interface{})
				if _, ok := test["reporter_config"]; ok {
					t.Error("expected no reporter_config when no cron tests")
				}
			},
		},
		{
			name:         "no tests key returns unchanged yaml",
			slackChannel: "#knative-eventing-ci",
			inputYAML:    "metadata: {}\n",
			check: func(t *testing.T, out []byte) {
				if string(out) != "metadata: {}\n" {
					t.Errorf("expected unchanged yaml, got %q", string(out))
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out, err := addReporterConfigToTests([]byte(tt.inputYAML), tt.slackChannel)
			if (err != nil) != tt.wantErr {
				t.Fatalf("addReporterConfigToTests() error = %v, wantErr %v", err, tt.wantErr)
			}
			if err == nil && tt.check != nil {
				tt.check(t, out)
			}
		})
	}
}
