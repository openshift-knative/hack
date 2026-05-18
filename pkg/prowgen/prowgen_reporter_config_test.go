package prowgen

import (
	"math/rand"
	"testing"

	"github.com/google/go-cmp/cmp"
	cioperatorapi "github.com/openshift/ci-tools/pkg/api"
)

func TestDiscoverTestsSetsSlackReporterConfig(t *testing.T) {
	r := Repository{
		Org:          "testdata",
		Repo:         "serving",
		ImagePrefix:  "knative-serving",
		SlackChannel: "#test-channel",
		Dockerfiles: Dockerfiles{
			Matches: []string{
				"knative-images/.*",
			},
		},
		E2ETests: []E2ETest{
			{Match: "test-e2e$"},
		},
	}

	random := rand.New(rand.NewSource(1))
	cfg := &cioperatorapi.ReleaseBuildConfiguration{}
	opt := DiscoverTests(r, OpenShift{Version: "4.12"}, "knative-serving-source-image", nil, random)
	if err := opt(cfg); err != nil {
		t.Fatalf("DiscoverTests failed: %v", err)
	}

	var presubmit, cron *cioperatorapi.TestStepConfiguration
	for i := range cfg.Tests {
		if cfg.Tests[i].Cron != nil {
			cron = &cfg.Tests[i]
		} else {
			presubmit = &cfg.Tests[i]
		}
	}

	if presubmit == nil {
		t.Fatal("expected a presubmit test")
	}
	if presubmit.SlackReporterConfig != nil {
		t.Error("presubmit test should not have SlackReporterConfig")
	}

	if cron == nil {
		t.Fatal("expected a cron test")
	}
	if cron.SlackReporterConfig == nil {
		t.Fatal("cron test should have SlackReporterConfig")
	}
	if diff := cmp.Diff("#test-channel", cron.SlackReporterConfig.Channel); diff != "" {
		t.Errorf("channel mismatch (-want +got):\n%s", diff)
	}
	if diff := cmp.Diff(cioperatorapi.DefaultSlackReporterJobStatesToReport, cron.SlackReporterConfig.JobStatesToReport); diff != "" {
		t.Errorf("job_states_to_report mismatch (-want +got):\n%s", diff)
	}
	if cron.SlackReporterConfig.ReportTemplate != slackReportTemplate {
		t.Errorf("report_template mismatch: got %q", cron.SlackReporterConfig.ReportTemplate)
	}
}

func TestDiscoverTestsNoSlackReporterWhenNoChannel(t *testing.T) {
	r := Repository{
		Org:         "testdata",
		Repo:        "serving",
		ImagePrefix: "knative-serving",
		Dockerfiles: Dockerfiles{
			Matches: []string{
				"knative-images/.*",
			},
		},
		E2ETests: []E2ETest{
			{Match: "test-e2e$"},
		},
	}

	random := rand.New(rand.NewSource(1))
	cfg := &cioperatorapi.ReleaseBuildConfiguration{}
	opt := DiscoverTests(r, OpenShift{Version: "4.12"}, "knative-serving-source-image", nil, random)
	if err := opt(cfg); err != nil {
		t.Fatalf("DiscoverTests failed: %v", err)
	}

	for _, test := range cfg.Tests {
		if test.SlackReporterConfig != nil {
			t.Errorf("test %q should not have SlackReporterConfig when no SlackChannel is set", test.As)
		}
	}
}

func TestDiscoverTestsNoSlackReporterWhenSkipCron(t *testing.T) {
	r := Repository{
		Org:          "testdata",
		Repo:         "serving",
		ImagePrefix:  "knative-serving",
		SlackChannel: "#test-channel",
		Dockerfiles: Dockerfiles{
			Matches: []string{
				"knative-images/.*",
			},
		},
		E2ETests: []E2ETest{
			{Match: "test-e2e$", SkipCron: true},
		},
	}

	random := rand.New(rand.NewSource(1))
	cfg := &cioperatorapi.ReleaseBuildConfiguration{}
	opt := DiscoverTests(r, OpenShift{Version: "4.12"}, "knative-serving-source-image", nil, random)
	if err := opt(cfg); err != nil {
		t.Fatalf("DiscoverTests failed: %v", err)
	}

	for _, test := range cfg.Tests {
		if test.SlackReporterConfig != nil {
			t.Errorf("test %q should not have SlackReporterConfig when SkipCron is true", test.As)
		}
	}
}

