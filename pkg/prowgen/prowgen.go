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
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/openshift-knative/hack/pkg/util"
	"github.com/openshift/ci-tools/pkg/api/shardprowconfig"
	ciconfig "github.com/openshift/ci-tools/pkg/config"
	"sigs.k8s.io/yaml"

	"github.com/coreos/go-semver/semver"
	"golang.org/x/sync/errgroup"
	prowapi "sigs.k8s.io/prow/pkg/apis/prowjobs/v1"
)

// Config is the prowgen configuration file struct.
type Config struct {
	Repositories []Repository `json:"repositories,omitempty" yaml:"repositories,omitempty"`

	Config CommonConfig `json:"config,omitempty" yaml:"config,omitempty"`
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
	build := flag.Bool("build", true, "Run the openshift/release generator")
	push := flag.Bool("push", true, "Whether to commit and push the changes")
	konflux := flag.Bool("konflux", true, "Whether to generate Konflux config")
	owners := flag.Bool("owners", true, "Whether to generate OWNERS files")
	flag.Parse()

	log.Println(*inputConfig, *outConfig)

	var inConfigs []*Config

	fi, err := os.Lstat(*inputConfig)
	if err != nil {
		log.Fatalln(err)
	}
	if fi.IsDir() {
		err := filepath.WalkDir(*inputConfig, func(path string, d fs.DirEntry, err error) error {
			if d.IsDir() {
				return nil
			}

			if !strings.HasSuffix(path, ".yaml") {
				return nil
			}

			inConfig, err := LoadConfig(path)
			if err != nil {
				return fmt.Errorf("failed to load config file %q: %v", path, err)
			}
			inConfigs = append(inConfigs, inConfig)
			return nil
		})
		if err != nil {
			log.Fatalln("Failed to load configs", *inputConfig, err)
		}
	} else {
		inConfig, err := LoadConfig(*inputConfig)
		if err != nil {
			log.Fatalln("Failed to load config", err)
		}
		inConfigs = append(inConfigs, inConfig)
	}

	for _, inConfig := range inConfigs {
		for _, v := range inConfig.Config.Branches {
			sort.Slice(v.OpenShiftVersions, func(i, j int) bool {
				return semver.New(v.OpenShiftVersions[i].Version + ".0").LessThan(*semver.New(v.OpenShiftVersions[j].Version + ".0"))
			})
		}
	}

	// Clone openshift/release and clean up existing jobs for the configured branches
	openshiftReleaseInitialization, openshiftReleaseInitCtx := errgroup.WithContext(ctx)
	openshiftReleaseInitialization.Go(func() error {
		return InitializeOpenShiftReleaseRepository(openshiftReleaseInitCtx, openShiftRelease, inConfigs, outConfig)
	})

	// For each repository and branch generate openshift/release configuration, and write it to the output file.
	repositoriesGenerateConfigs, generatorsCtx := errgroup.WithContext(ctx)
	for _, inConfig := range inConfigs {
		inConfig := inConfig

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
				for branch, b := range inConfig.Config.Branches {
					if b.Prowgen != nil && b.Prowgen.Disabled {
						continue
					}
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

				branchProtectionAndTideConfig := NewProwConfig(repository)
				if err := SaveProwConfig(openShiftRelease, repository, branchProtectionAndTideConfig); err != nil {
					return err
				}

				prowgenCfg := NewProwgenConfig(repository, inConfig.Config, cfgs)
				if err := SaveProwgenConfig(outConfig, repository, prowgenCfg); err != nil {
					return err
				}

				return nil
			})
		}
	}
	// Wait for the openshift/release initialization goroutine and repositories generators goroutines.
	if err := openshiftReleaseInitialization.Wait(); err != nil {
		log.Fatalln("Failed waiting for", openShiftRelease.RepositoryDirectory(), "initialization", err)
	}

	if err := repositoriesGenerateConfigs.Wait(); err != nil {
		log.Fatalln("Failed waiting for repositories generator", err)
	}
	if *build {
		if err := RunOpenShiftReleaseGenerator(ctx, openShiftRelease); err != nil {
			log.Fatalln("Failed to run openshift/release generator:", err)
		}
	}
	if *push {
		if err := PushBranch(ctx, openShiftRelease, remote, *branch, "Sync Serverless CI "+*inputConfig); err != nil {
			log.Fatalln("Failed to push branch to openshift/release fork", *remote, err)
		}
	}
	if *konflux {
		if err := GenerateKonflux(ctx, openShiftRelease, inConfigs); err != nil {
			log.Fatalln("Failed to generate Konflux configurations: %w", err)
		}
	}
	if *owners {
		if err := GenerateOwners(ctx, inConfigs); err != nil {
			log.Fatalln("Failed to generate OWNERS files: %w", err)
		}
	}
}

func LoadConfig(path string) (*Config, error) {
	// Going directly from YAML raw input produces unexpected configs (due to missing YAML tags),
	// so we convert YAML to JSON and unmarshal the struct from the JSON object.
	y, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	return UnmarshalConfig(y)
}

func UnmarshalConfig(rawYaml []byte) (*Config, error) {
	j, err := yaml.YAMLToJSON(rawYaml)
	if err != nil {
		return nil, err
	}
	inConfig := &Config{}
	if err := json.Unmarshal(j, inConfig); err != nil {
		return nil, fmt.Errorf("failed to unmarshall config: %w", err)
	}
	return inConfig, nil
}

func PushBranch(ctx context.Context, release Repository, remote *string, branch string, commitMsg string) error {

	// Ignore error since remote and branch might be already there
	_, _ = Run(ctx, release, "git", "checkout", "-b", branch)
	_, _ = Run(ctx, release, "git", "checkout", branch)

	if _, err := Run(ctx, release, "git", "add", "."); err != nil {
		return err
	}
	if _, err := Run(ctx, release, "git", "commit", "-m", commitMsg); err != nil {
		// Ignore error since we could have nothing to commit
		log.Println("Ignored error", err)
	}

	if remote == nil || *remote == "" {
		return nil
	}

	log.Println("Pushing branch", branch, "to", *remote)

	_, _ = Run(ctx, release, "git", "remote", "add", "fork", *remote)
	if _, err := Run(ctx, release, "git", "push", "fork", branch, "-f"); err != nil {
		return err
	}

	return nil
}

func DeleteExistingReleaseBuildConfigurationForBranch(outConfig *string, r Repository, branch string) error {
	dir := filepath.Join(*outConfig, r.RepositoryDirectory())
	configPaths, err := filepath.Glob(filepath.Join(dir, "*"+branch+"*"))
	if err != nil {
		return err
	}
	if err := deleteConfigsIfNeeded(r.IgnoreConfigs.Matches, configPaths, branch); err != nil {
		return err
	}
	return nil
}

func deleteConfigsIfNeeded(ignoreConfigs []string, paths []string, branch string) error {
	excludeFilePattern, err := util.ToRegexp(ignoreConfigs)
	if err != nil {
		return fmt.Errorf("failed to parse ignore configs regex: %w", err)
	}

	for _, path := range paths {
		include := true
		for _, r := range excludeFilePattern {
			if r.MatchString(path) {
				include = false
				break
			}
		}
		if include {
			if branch != "" {
				log.Println("Detected a config for branch", branch, "removing file", path)
			} else {
				log.Println("Detected a config, removing file", path)
			}

			if err := os.Remove(path); err != nil {
				return err
			}
		}
	}
	return nil
}

func SaveProwConfig(openShiftRelease Repository, repository Repository, config shardprowconfig.ProwConfigWithPointers) error {
	outPath := filepath.Join(openShiftRelease.Org, openShiftRelease.Repo, "core-services", "prow", "02_config", repository.Org, repository.Repo, "_prowconfig.yaml")

	dir := filepath.Dir(outPath)

	if err := os.MkdirAll(dir, os.ModePerm); err != nil {
		return err
	}

	out, err := yaml.Marshal(config)
	if err != nil {
		return err
	}

	return os.WriteFile(outPath, out, os.ModePerm)
}

const slackReportTemplate = `{{if eq .Status.State "success"}} :rainbow: Job *{{.Spec.Job}}* ended with *{{.Status.State}}*. <{{.Status.URL}}|View logs> :rainbow: {{else}} :volcano: Job *{{.Spec.Job}}* ended with *{{.Status.State}}*. <{{.Status.URL}}|View logs> :volcano: {{end}}`

func NewProwgenConfig(r Repository, cc CommonConfig, cfgs []ReleaseBuildConfiguration) *ciconfig.Prowgen {
	if r.SlackChannel == "" {
		return nil
	}

	jobNameSet := make(map[string]struct{})
	for _, cfg := range cfgs {
		for _, test := range cfg.Tests {
			if test.Cron != nil {
				jobNameSet[test.As] = struct{}{}
			}
		}
	}

	if len(jobNameSet) == 0 {
		return nil
	}

	jobNames := make([]string, 0, len(jobNameSet))
	for name := range jobNameSet {
		jobNames = append(jobNames, name)
	}
	sort.Strings(jobNames)

	variantSet := make(map[string]struct{})
	for _, branch := range cc.Branches {
		if branch.Prowgen != nil && branch.Prowgen.Disabled {
			continue
		}
		for _, ov := range branch.OpenShiftVersions {
			if ov.SkipCron || ov.CandidateRelease {
				variant := strings.ReplaceAll(ov.Version, ".", "")
				variantSet[variant] = struct{}{}
			}
		}
	}

	var excludedVariants []string
	if len(variantSet) > 0 {
		excludedVariants = make([]string, 0, len(variantSet))
		for v := range variantSet {
			excludedVariants = append(excludedVariants, v)
		}
		sort.Strings(excludedVariants)
	}

	return &ciconfig.Prowgen{
		SlackReporterConfigs: []ciconfig.SlackReporterConfig{
			{
				Channel:           r.SlackChannel,
				JobStatesToReport: []prowapi.ProwJobState{prowapi.SuccessState, prowapi.FailureState, prowapi.ErrorState},
				ReportTemplate:    slackReportTemplate,
				JobNames:          jobNames,
				ExcludedVariants:  excludedVariants,
			},
		},
	}
}

func SaveProwgenConfig(outConfig *string, r Repository, prowgenCfg *ciconfig.Prowgen) error {
	if prowgenCfg == nil {
		return nil
	}

	dir := filepath.Join(*outConfig, r.RepositoryDirectory())
	if err := os.MkdirAll(dir, os.ModePerm); err != nil {
		return err
	}

	out, err := yaml.Marshal(prowgenCfg)
	if err != nil {
		return err
	}

	return os.WriteFile(filepath.Join(dir, ciconfig.ProwgenFile), out, os.ModePerm)
}

func SaveReleaseBuildConfiguration(outConfig *string, cfg ReleaseBuildConfiguration) error {
	dir := filepath.Join(*outConfig, filepath.Dir(cfg.Path))

	if err := os.MkdirAll(dir, os.ModePerm); err != nil {
		return err
	}

	out, err := yaml.Marshal(cfg.ReleaseBuildConfiguration)
	if err != nil {
		return err
	}

	if err := os.WriteFile(filepath.Join(*outConfig, cfg.Path), out, os.ModePerm); err != nil {
		return err
	}

	return copyOwnersFileIfNotPresent(dir)
}

func copyOwnersFileIfNotPresent(dir string) error {
	if _, err := os.Stat(filepath.Join(dir, "OWNERS")); err == nil {
		// skip if file already exists, openshift-ci bot will keep it up to date
		return nil
	}
	owners, err := os.ReadFile("OWNERS")
	if err != nil {
		// Log just a warning
		log.Printf("failed to read file: %v", err)
		return nil
	}
	if err := os.WriteFile(filepath.Join(dir, "OWNERS"), owners, os.ModePerm); err != nil {
		return fmt.Errorf("failed to write OWNERS file in %q: %w", dir, err)
	}
	return nil
}

func RunOpenShiftReleaseGenerator(ctx context.Context, openShiftRelease Repository) error {
	if _, err := Run(ctx, openShiftRelease, "make", "ci-operator-config", "jobs", "prow-config"); err != nil {
		return err
	}
	return nil
}

// InitializeOpenShiftReleaseRepository clones openshift/release and clean up existing jobs
// for the configured branches
func InitializeOpenShiftReleaseRepository(ctx context.Context, openShiftRelease Repository, inConfigs []*Config, outputConfig *string) error {
	if err := GitMirror(ctx, openShiftRelease); err != nil {
		return err
	}
	if err := GitCheckout(ctx, openShiftRelease, "main"); err != nil {
		return err
	}

	// Remove all config files except the ones explicitly excluded
	for _, inConfig := range inConfigs {
		for _, r := range inConfig.Repositories {
			// TODO: skip automatic deletion for S-O for now
			if strings.Contains(r.RepositoryDirectory(), "serverless-operator") {
				for branch, branchConfig := range inConfig.Config.Branches {
					if branchConfig.Prowgen != nil && branchConfig.Prowgen.Disabled {
						continue
					}

					matches, err := filepath.Glob(filepath.Join(*outputConfig, r.RepositoryDirectory(), "*"+branch+"*"))
					if err != nil {
						return err
					}
					if err := deleteConfigsIfNeeded(r.IgnoreConfigs.Matches, matches, branch); err != nil {
						return err
					}
				}
				continue
			}
			// Remove all config files except the ones explicitly excluded
			matchesForDeletion, err := filepath.Glob(filepath.Join(*outputConfig, r.RepositoryDirectory(), "*.*"))
			if err != nil {
				return err
			}
			if err := deleteConfigsIfNeeded(r.IgnoreConfigs.Matches, matchesForDeletion, ""); err != nil {
				return err
			}
		}
	}
	return nil
}
