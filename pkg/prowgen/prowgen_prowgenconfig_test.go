package prowgen

import (
	"os"
	"testing"

	cioperatorapi "github.com/openshift/ci-tools/pkg/api"
	ciconfig "github.com/openshift/ci-tools/pkg/config"
	"sigs.k8s.io/yaml"

	"github.com/google/go-cmp/cmp"
	"k8s.io/utils/pointer"
	prowv1 "sigs.k8s.io/prow/pkg/apis/prowjobs/v1"
)

func TestNewProwgenConfig(t *testing.T) {
	tests := []struct {
		name     string
		repo     Repository
		cc       CommonConfig
		cfgs     []ReleaseBuildConfiguration
		expected *ciconfig.Prowgen
	}{
		{
			name: "generates config with periodic tests and excluded variants",
			repo: Repository{
				SlackChannel: "#knative-eventing-ci",
			},
			cc: CommonConfig{
				Branches: map[string]Branch{
					"release-next": {
						OpenShiftVersions: []OpenShift{
							{Version: "4.21", CandidateRelease: true, SkipCron: true},
							{Version: "4.20"},
							{Version: "4.14", OnDemand: true},
						},
					},
				},
			},
			cfgs: []ReleaseBuildConfiguration{
				{
					ReleaseBuildConfiguration: cioperatorapi.ReleaseBuildConfiguration{
						Tests: []cioperatorapi.TestStepConfiguration{
							{As: "test-e2e"},
							{As: "test-e2e-c", Cron: pointer.String("0 2 * * 2,6")},
							{As: "test-conformance-c", Cron: pointer.String("0 3 * * 2,6")},
						},
					},
				},
			},
			expected: &ciconfig.Prowgen{
				SlackReporterConfigs: []ciconfig.SlackReporterConfig{
					{
						Channel:           "#knative-eventing-ci",
						JobStatesToReport: []prowv1.ProwJobState{prowv1.SuccessState, prowv1.FailureState, prowv1.ErrorState},
						ReportTemplate:    slackReportTemplate,
						JobNames:          []string{"test-conformance-c", "test-e2e-c"},
						ExcludedVariants:  []string{"421"},
					},
				},
			},
		},
		{
			name: "returns nil when SlackChannel is empty",
			repo: Repository{},
			cc:   CommonConfig{},
			cfgs: []ReleaseBuildConfiguration{
				{
					ReleaseBuildConfiguration: cioperatorapi.ReleaseBuildConfiguration{
						Tests: []cioperatorapi.TestStepConfiguration{
							{As: "test-e2e-c", Cron: pointer.String("0 2 * * 2,6")},
						},
					},
				},
			},
			expected: nil,
		},
		{
			name: "returns nil when no periodic tests exist",
			repo: Repository{
				SlackChannel: "#test-channel",
			},
			cc: CommonConfig{},
			cfgs: []ReleaseBuildConfiguration{
				{
					ReleaseBuildConfiguration: cioperatorapi.ReleaseBuildConfiguration{
						Tests: []cioperatorapi.TestStepConfiguration{
							{As: "test-e2e"},
						},
					},
				},
			},
			expected: nil,
		},
		{
			name: "deduplicates job names across configs",
			repo: Repository{
				SlackChannel: "#test-channel",
			},
			cc: CommonConfig{
				Branches: map[string]Branch{
					"release-next": {
						OpenShiftVersions: []OpenShift{
							{Version: "4.20"},
						},
					},
				},
			},
			cfgs: []ReleaseBuildConfiguration{
				{
					ReleaseBuildConfiguration: cioperatorapi.ReleaseBuildConfiguration{
						Tests: []cioperatorapi.TestStepConfiguration{
							{As: "test-e2e-c", Cron: pointer.String("0 2 * * 2,6")},
						},
					},
				},
				{
					ReleaseBuildConfiguration: cioperatorapi.ReleaseBuildConfiguration{
						Tests: []cioperatorapi.TestStepConfiguration{
							{As: "test-e2e-c", Cron: pointer.String("0 3 * * 2,6")},
						},
					},
				},
			},
			expected: &ciconfig.Prowgen{
				SlackReporterConfigs: []ciconfig.SlackReporterConfig{
					{
						Channel:           "#test-channel",
						JobStatesToReport: []prowv1.ProwJobState{prowv1.SuccessState, prowv1.FailureState, prowv1.ErrorState},
						ReportTemplate:    slackReportTemplate,
						JobNames:          []string{"test-e2e-c"},
					},
				},
			},
		},
		{
			name: "skips disabled prowgen branches for excluded_variants",
			repo: Repository{
				SlackChannel: "#test-channel",
			},
			cc: CommonConfig{
				Branches: map[string]Branch{
					"release-next": {
						OpenShiftVersions: []OpenShift{
							{Version: "4.20"},
						},
					},
					"disabled-branch": {
						Prowgen: &Prowgen{Disabled: true},
						OpenShiftVersions: []OpenShift{
							{Version: "4.19", SkipCron: true},
						},
					},
				},
			},
			cfgs: []ReleaseBuildConfiguration{
				{
					ReleaseBuildConfiguration: cioperatorapi.ReleaseBuildConfiguration{
						Tests: []cioperatorapi.TestStepConfiguration{
							{As: "test-e2e-c", Cron: pointer.String("0 2 * * 2,6")},
						},
					},
				},
			},
			expected: &ciconfig.Prowgen{
				SlackReporterConfigs: []ciconfig.SlackReporterConfig{
					{
						Channel:           "#test-channel",
						JobStatesToReport: []prowv1.ProwJobState{prowv1.SuccessState, prowv1.FailureState, prowv1.ErrorState},
						ReportTemplate:    slackReportTemplate,
						JobNames:          []string{"test-e2e-c"},
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := NewProwgenConfig(tt.repo, tt.cc, tt.cfgs)
			if diff := cmp.Diff(tt.expected, got); diff != "" {
				t.Errorf("NewProwgenConfig() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestSaveProwgenConfig(t *testing.T) {
	t.Run("writes correct YAML", func(t *testing.T) {
		dir := t.TempDir()
		outConfig := dir
		r := Repository{
			Org:  "openshift-knative",
			Repo: "eventing",
		}
		cfg := &ciconfig.Prowgen{
			SlackReporterConfigs: []ciconfig.SlackReporterConfig{
				{
					Channel:           "#knative-eventing-ci",
					JobStatesToReport: []prowv1.ProwJobState{prowv1.SuccessState, prowv1.FailureState, prowv1.ErrorState},
					ReportTemplate:    slackReportTemplate,
					JobNames:          []string{"test-conformance-c", "test-e2e-c"},
					ExcludedVariants:  []string{"421"},
				},
			},
		}

		if err := SaveProwgenConfig(&outConfig, r, cfg); err != nil {
			t.Fatalf("SaveProwgenConfig() error: %v", err)
		}

		filePath := dir + "/openshift-knative/eventing/.config.prowgen"
		data, err := os.ReadFile(filePath)
		if err != nil {
			t.Fatalf("failed to read .config.prowgen: %v", err)
		}

		// Unmarshal back and verify structurally
		var got ciconfig.Prowgen
		if err := yaml.Unmarshal(data, &got); err != nil {
			t.Fatalf("failed to unmarshal .config.prowgen: %v", err)
		}

		if diff := cmp.Diff(*cfg, got); diff != "" {
			t.Errorf("SaveProwgenConfig() roundtrip mismatch (-want +got):\n%s", diff)
		}
	})

	t.Run("skips writing when config is nil", func(t *testing.T) {
		dir := t.TempDir()
		outConfig := dir
		r := Repository{
			Org:  "openshift-knative",
			Repo: "eventing",
		}

		if err := SaveProwgenConfig(&outConfig, r, nil); err != nil {
			t.Fatalf("SaveProwgenConfig() error: %v", err)
		}

		filePath := dir + "/openshift-knative/eventing/.config.prowgen"
		if _, err := os.Stat(filePath); !os.IsNotExist(err) {
			t.Errorf("expected .config.prowgen to not exist, but it does")
		}
	})
}
