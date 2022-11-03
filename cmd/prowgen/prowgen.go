// prowgen generates openshift/release configurations based on the OpenShift serverless
// teams conventions.
//
// For example, it extracts image builds Dockerfile from the common
// directory `openshift/ci-operator/**/Dockerfile.
//
// To onboard a new repository, update the configuration in config/repositories.yaml
// and run the program, or alternatively, you can provide your own configuration file
// using the -config <path> argument.

package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"sort"

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

func main() {
	ctx := context.TODO()

	openShiftRelease := Repository{
		Org:  "openshift",
		Repo: "release",
	}

	inputConfig := flag.String("config", filepath.Join("config", "repositories.yaml"), "Specify repositories config")
	outConfig := flag.String("output", filepath.Join(openShiftRelease.Org, openShiftRelease.Repo, "ci-operator", "config"), "Specify repositories config")
	remote := flag.String("remote", "", "openshift/release remote fork (example: git@github.com:pierDipi/release.git)")
	flag.Parse()

	log.Println(*inputConfig, *outConfig)

	in, err := ioutil.ReadFile(*inputConfig)
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
		return initializeOpenShiftReleaseRepository(openshiftReleaseInitCtx, openShiftRelease, inConfig, outConfig)
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
			for branch := range inConfig.Config.Branches {
				if err := deleteExistingReleaseBuildConfigurationForBranch(outConfig, repository, branch); err != nil {
					return err
				}
			}

			// Write generated configurations.
			for _, cfg := range cfgs {
				if err := saveReleaseBuildConfiguration(outConfig, cfg); err != nil {
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

	if err := runOpenShiftReleaseGenerator(ctx, openShiftRelease); err != nil {
		log.Fatalln("Failed to run openshift/release generator:", err)
	}
	if err := injectSlackReporterConfig(inConfig, openShiftRelease); err != nil {
		log.Fatalln("Failed to inject Slack reporter", err)
	}
	if err := runOpenShiftReleaseGenerator(ctx, openShiftRelease); err != nil {
		log.Fatalln("Failed to run openshift/release generator after injecting Slack reporter", err)
	}
	if err := pushBranch(ctx, openShiftRelease, remote, "sync-serverless-ci"); err != nil {
		log.Fatalln("Failed to push branch to openshift/release fork", *remote, err)
	}
}

func pushBranch(ctx context.Context, release Repository, remote *string, branch string) error {
	if remote == nil || *remote == "" {
		return nil
	}

	log.Println("Pushing branch", branch, "to", *remote)

	// Ignore error since remote and branch might be already there
	_ = run(ctx, release, "git", "checkout", "-b", branch)
	_ = run(ctx, release, "git", "remote", "add", "fork", *remote)

	if err := run(ctx, release, "git", "add", "."); err != nil {
		return err
	}
	if err := run(ctx, release, "git", "commit", "-s", "-S", "-m", "Sync Serverless CI"); err != nil {
		return err
	}
	if err := run(ctx, release, "git", "push", "fork", branch, "-f"); err != nil {
		return err
	}

	return nil
}

func deleteExistingReleaseBuildConfigurationForBranch(outConfig *string, r Repository, branch string) error {
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

func saveReleaseBuildConfiguration(outConfig *string, cfg ReleaseBuildConfiguration) error {
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
	if err := ioutil.WriteFile(filepath.Join(*outConfig, cfg.Path), out, os.ModePerm); err != nil {
		return err
	}
	return nil
}

func runOpenShiftReleaseGenerator(ctx context.Context, openShiftRelease Repository) error {
	if err := run(ctx, openShiftRelease, "make", "ci-operator-config", "jobs"); err != nil {
		return err
	}
	return nil
}

func injectSlackReporterConfig(inConfig *Config, openShiftRelease Repository) error {
	// Inject Slack reporter for each repository and branch

	log.Println("Injecting Slack reporter configs")

	for branch := range inConfig.Config.Branches {
		for _, r := range inConfig.Repositories {
			generatedOutputDir := "ci-operator/jobs"
			glob := filepath.Join(openShiftRelease.RepositoryDirectory(), generatedOutputDir, r.RepositoryDirectory(), "*"+branch+"*periodics*")
			matches, err := filepath.Glob(glob)
			if err != nil {
				return err
			}
			for _, match := range matches {
				// Going directly from YAML raw input produces unexpected configs (due to missing YAML tags),
				// so we convert YAML to JSON and unmarshal the struct from the JSON object.
				y, err := ioutil.ReadFile(match)
				if err != nil {
					return err
				}
				j, err := gyaml.YAMLToJSON(y)
				if err != nil {
					return err
				}

				jobConfig := &prowconfig.JobConfig{}
				if err := json.Unmarshal(j, jobConfig); err != nil {
					return err
				}

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

				// Going directly from struct to YAML produces unexpected configs (due to missing YAML tags),
				// so we produce JSON and then convert it to YAML.
				out, err := json.Marshal(jobConfig)
				if err != nil {
					return err
				}
				y, err = gyaml.JSONToYAML(out)
				if err != nil {
					return err
				}
				if err := ioutil.WriteFile(match, y, os.ModePerm); err != nil {
					return err
				}
			}
		}
	}

	return nil
}

// initializeOpenShiftReleaseRepository clones openshift/release and clean up existing jobs
// for the configured branches
func initializeOpenShiftReleaseRepository(ctx context.Context, openShiftRelease Repository, inConfig *Config, outputConfig *string) error {
	if err := GitClone(ctx, openShiftRelease); err != nil {
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
