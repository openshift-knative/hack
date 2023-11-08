// prowgen generates openshift/release configurations based on the OpenShift serverless
// teams conventions.
//
// For example, it extracts image builds Dockerfile from the common
// directory `openshift/ci-operator/**/Dockerfile.
//
// To onboard a new repository, update the configuration in config/repositories.yaml
// and run the program, or alternatively, you can provide your own configuration file
// using the -config <path> argument.

package prowgen

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/coreos/go-semver/semver"
	gyaml "github.com/ghodss/yaml"
	"golang.org/x/sync/errgroup"
	"gopkg.in/yaml.v2"
	prowapi "k8s.io/test-infra/prow/apis/prowjobs/v1"
	prowconfig "k8s.io/test-infra/prow/config"
)

// Config is the prowgen configuration file struct.
type Config struct {
	Repositories []Repository `json:"repositories" yaml:"repositories"`

	Config CommonConfig `json:"config" yaml:"config"`
}

func Main() {
	ctx := context.TODO()

	openShiftRelease := Repository{
		Org:  "openshift",
		Repo: "release",
	}

	inputConfig := flag.String("config", filepath.Join("config", "repositories.yaml"), "Specify repositories config")
	outConfig := flag.String("output", filepath.Join(openShiftRelease.Org, openShiftRelease.Repo, "ci-operator", "config"), "Specify repositories config")
	remote := flag.String("remote", "", "openshift/release remote fork (example: git@github.com:pierDipi/release.git)")
	branch := flag.String("branch", "sync-serverless-ci", "Branch for remote fork")
	flag.Parse()

	log.Println(*inputConfig, *outConfig)

	in, err := os.ReadFile(*inputConfig)
	if err != nil {
		log.Fatalln(err)
	}

	inConfig := &Config{}
	if err := yaml.UnmarshalStrict(in, inConfig); err != nil {
		log.Fatalln("Unmarshal input config", err)
	}

	for _, v := range inConfig.Config.Branches {
		sort.Slice(v.OpenShiftVersions, func(i, j int) bool {
			return semver.New(v.OpenShiftVersions[i] + ".0").LessThan(*semver.New(v.OpenShiftVersions[j] + ".0"))
		})
	}

	// Clone openshift/release and clean up existing jobs for the configured branches
	openshiftReleaseInitialization, openshiftReleaseInitCtx := errgroup.WithContext(ctx)
	openshiftReleaseInitialization.Go(func() error {
		return InitializeOpenShiftReleaseRepository(openshiftReleaseInitCtx, openShiftRelease, inConfig, outConfig)
	})

	// For each repository and branch generate openshift/release configuration, and write it to the output file.
	repositoriesGenerateConfigs, generatorsCtx := errgroup.WithContext(ctx)
	for _, repository := range inConfig.Repositories {
		repository := repository

		repositoriesGenerateConfigs.Go(func() error {

			cfgs, err := NewGenerateConfigs(generatorsCtx, repository, inConfig.Config)
			if err != nil {
				return err
			}

			// Wait for the openshift/release initialization goroutine.
			if err := openshiftReleaseInitialization.Wait(); err != nil {
				return fmt.Errorf("failed waiting for %s initialization: %w", openShiftRelease.RepositoryDirectory(), err)
			}

			// Delete existing configuration for each configured branch.
			// TODO. Have this optional?
			for branch := range inConfig.Config.Branches {
				if err := DeleteExistingReleaseBuildConfigurationForBranch(outConfig, repository, branch); err != nil {
					return err
				}
			}

			// Write generated configurations.
			for _, cfg := range cfgs {
				if err := SaveReleaseBuildConfiguration(outConfig, cfg); err != nil {
					return err
				}
			}

			// Generate and write image mirroring configurations.
			for _, imageMirroring := range GenerateImageMirroringConfigs(openShiftRelease, cfgs) {
				if err := ReconcileImageMirroringConfig(imageMirroring); err != nil {
					return err
				}
			}
			return nil
		})
	}

	// Wait for the openshift/release initialization goroutine and repositories generators goroutines.
	if err := openshiftReleaseInitialization.Wait(); err != nil {
		log.Fatalln("Failed waiting for", openShiftRelease.RepositoryDirectory(), "initialization", err)
	}
	if err := repositoriesGenerateConfigs.Wait(); err != nil {
		log.Fatalln("Failed waiting for repositories generator", err)
	}

	if err := RunOpenShiftReleaseGenerator(ctx, openShiftRelease); err != nil {
		log.Fatalln("Failed to run openshift/release generator:", err)
	}
	if err := runJobConfigInjectors(inConfig, openShiftRelease); err != nil {
		log.Fatalln("Failed to inject Slack reporter", err)
	}
	if err := RunOpenShiftReleaseGenerator(ctx, openShiftRelease); err != nil {
		log.Fatalln("Failed to run openshift/release generator after injecting Slack reporter", err)
	}
	if err := PushBranch(ctx, openShiftRelease, remote, *branch, *inputConfig); err != nil {
		log.Fatalln("Failed to push branch to openshift/release fork", *remote, err)
	}
}

func PushBranch(ctx context.Context, release Repository, remote *string, branch string, config string) error {

	// Ignore error since remote and branch might be already there
	_, _ = run(ctx, release, "git", "checkout", "-b", branch)
	_, _ = run(ctx, release, "git", "checkout", branch)

	if _, err := run(ctx, release, "git", "add", "."); err != nil {
		return err
	}
	if _, err := run(ctx, release, "git", "commit", "-m", "Sync Serverless CI "+config); err != nil {
		// Ignore error since we could have nothing to commit
		log.Println("Ignored error", err)
	}

	if remote == nil || *remote == "" {
		return nil
	}

	log.Println("Pushing branch", branch, "to", *remote)

	_, _ = run(ctx, release, "git", "remote", "add", "fork", *remote)
	if _, err := run(ctx, release, "git", "push", "fork", branch, "-f"); err != nil {
		return err
	}

	return nil
}

func DeleteExistingReleaseBuildConfigurationForBranch(outConfig *string, r Repository, branch string) error {
	dir := filepath.Join(*outConfig, r.RepositoryDirectory())
	matches, err := filepath.Glob(filepath.Join(dir, "*"+branch+"*"))
	if err != nil {
		return err
	}

	for _, match := range matches {
		log.Println("Detected a new config for branch", branch, "removing file", match)
		if err := os.Remove(match); err != nil {
			return err
		}
	}

	return nil
}

func SaveReleaseBuildConfiguration(outConfig *string, cfg ReleaseBuildConfiguration) error {
	dir := filepath.Join(*outConfig, filepath.Dir(cfg.Path))

	if err := os.MkdirAll(dir, os.ModePerm); err != nil {
		return err
	}
	// Going directly from struct to YAML produces unexpected configs (due to missing YAML tags),
	// so we produce JSON and then convert it to YAML.
	out, err := json.Marshal(cfg.ReleaseBuildConfiguration)
	if err != nil {
		return err
	}
	out, err = gyaml.JSONToYAML(out)
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(*outConfig, cfg.Path), out, os.ModePerm); err != nil {
		return err
	}
	return nil
}

func RunOpenShiftReleaseGenerator(ctx context.Context, openShiftRelease Repository) error {
	if _, err := run(ctx, openShiftRelease, "make", "ci-operator-config", "jobs"); err != nil {
		return err
	}
	return nil
}

func runJobConfigInjectors(inConfig *Config, openShiftRelease Repository) error {
	injectors := JobConfigInjectors{
		alwaysRunInjector(),
		slackInjector(),
	}
	return injectors.Inject(inConfig, openShiftRelease)
}

func slackInjector() JobConfigInjector {
	return JobConfigInjector{
		Type: Periodic,
		Update: func(r *Repository, jobConfig *prowconfig.JobConfig, _ string) error {
			for i := range jobConfig.Periodics {
				jobConfig.Periodics[i].ReporterConfig = &prowapi.ReporterConfig{
					Slack: &prowapi.SlackReporterConfig{
						Channel: r.SlackChannel,
						JobStatesToReport: []prowapi.ProwJobState{
							prowapi.SuccessState,
							prowapi.FailureState,
							prowapi.ErrorState,
						},
						ReportTemplate: `{{if eq .Status.State "success"}} :rainbow: Job *{{.Spec.Job}}* ended with *{{.Status.State}}*. <{{.Status.URL}}|View logs> :rainbow: {{else}} :volcano: Job *{{.Spec.Job}}* ended with *{{.Status.State}}*. <{{.Status.URL}}|View logs> :volcano: {{end}}`,
					},
				}
			}
			return nil
		},
	}
}

func alwaysRunInjector() JobConfigInjector {
	return JobConfigInjector{
		Type: PreSubmit,
		Update: func(r *Repository, jobConfig *prowconfig.JobConfig, branchName string) error {
			if err := GitCheckout(context.TODO(), *r, branchName); err != nil {
				return fmt.Errorf("[%s] failed to checkout branch %s", r.RepositoryDirectory(), branchName)
			}
			tests, err := discoverE2ETests(*r)
			if err != nil {
				return fmt.Errorf("failed to discover tests: %w", err)
			}

			for k := range jobConfig.PresubmitsStatic {
				for i := range jobConfig.PresubmitsStatic[k] {
					if err != nil {
						return err
					}

					variant := jobConfig.PresubmitsStatic[k][i].Labels["ci-operator.openshift.io/variant"]
					ocpVersion := strings.SplitN(variant, "-", 2)[0]

					for _, t := range tests {
						name := ToName(*r, &t, ocpVersion)
						if (t.OnDemand || t.RunIfChanged != "") && strings.Contains(jobConfig.PresubmitsStatic[k][i].Name, name) {
							jobConfig.PresubmitsStatic[k][i].AlwaysRun = false
						}
					}
				}
			}

			return nil
		},
	}
}

type JobConfigType string

const (
	Periodic   JobConfigType = "periodics"
	PreSubmit  JobConfigType = "presubmits"
	PostSubmit JobConfigType = "postsubmits"
)

type JobConfigInjectors []JobConfigInjector

func (jcis JobConfigInjectors) Inject(inConfig *Config, openShiftRelease Repository) error {
	for _, jci := range jcis {
		if err := jci.Inject(inConfig, openShiftRelease); err != nil {
			return err
		}
	}
	return nil
}

type JobConfigInjector struct {
	Type   JobConfigType
	Update func(r *Repository, jobConfig *prowconfig.JobConfig, branchName string) error
}

func (jci *JobConfigInjector) Inject(inConfig *Config, openShiftRelease Repository) error {
	for branch := range inConfig.Config.Branches {
		for _, r := range inConfig.Repositories {

			generatedOutputDir := "ci-operator/jobs"
			glob := filepath.Join(openShiftRelease.RepositoryDirectory(), generatedOutputDir, r.RepositoryDirectory(), "*"+branch+"*"+string(jci.Type)+"*")
			matches, err := filepath.Glob(glob)
			if err != nil {
				return err
			}
			for _, match := range matches {
				jobConfig, err := getJobConfig(match)
				if err != nil {
					return err
				}

				if err := jci.Update(&r, jobConfig, branch); err != nil {
					return err
				}

				if err := saveJobConfig(match, jobConfig); err != nil {
					return err
				}
			}
		}
	}

	return nil
}

func saveJobConfig(match string, jobConfig *prowconfig.JobConfig) error {
	// Going directly from struct to YAML produces unexpected configs (due to missing YAML tags),
	// so we produce JSON and then convert it to YAML.
	out, err := json.Marshal(jobConfig)
	if err != nil {
		return err
	}
	y, err := gyaml.JSONToYAML(out)
	if err != nil {
		return err
	}
	if err := os.WriteFile(match, y, os.ModePerm); err != nil {
		return err
	}

	return nil
}

func getJobConfig(match string) (*prowconfig.JobConfig, error) {
	// Going directly from YAML raw input produces unexpected configs (due to missing YAML tags),
	// so we convert YAML to JSON and unmarshal the struct from the JSON object.
	y, err := os.ReadFile(match)
	if err != nil {
		return nil, err
	}
	j, err := gyaml.YAMLToJSON(y)
	if err != nil {
		return nil, err
	}

	jobConfig := &prowconfig.JobConfig{}
	if err := json.Unmarshal(j, jobConfig); err != nil {
		return nil, err
	}
	return jobConfig, nil
}

// InitializeOpenShiftReleaseRepository clones openshift/release and clean up existing jobs
// for the configured branches
func InitializeOpenShiftReleaseRepository(ctx context.Context, openShiftRelease Repository, inConfig *Config, outputConfig *string) error {
	if err := GitMirror(ctx, openShiftRelease); err != nil {
		return err
	}
	if err := GitCheckout(ctx, openShiftRelease, "master"); err != nil {
		return err
	}
	for branch := range inConfig.Config.Branches {
		for _, r := range inConfig.Repositories {
			matches, err := filepath.Glob(filepath.Join(*outputConfig, r.RepositoryDirectory(), "*"+branch+"*"))
			if err != nil {
				return err
			}
			for _, match := range matches {
				if err := os.Remove(match); err != nil {
					return err
				}
			}
		}
	}
	return nil
}
