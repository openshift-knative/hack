package prowcopy

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	gyaml "github.com/ghodss/yaml"
	cioperatorapi "github.com/openshift/ci-tools/pkg/api"
	"golang.org/x/sync/errgroup"
	"k8s.io/utils/pointer"

	"github.com/openshift-knative/hack/pkg/prowgen"
)

type Config struct {
	Org            string
	Repo           string
	FromBranch     string
	Branch         string
	Tag            string
	RemovePeriodic bool
	Remote         string
	Config         string
}

// Main is the main function for prowcopy.
//
// prowcopy allows to create Prow configuration in openshift/release starting from the configuration
// of another branch. For example, if you have configuration for CI of the main branch, and then you
// cut a new branch, the main branch configuration can be used to define CI of the new branch.
func Main() error {
	ctx := context.Background()

	c := Config{}

	flag.StringVar(&c.Org, "org", "openshift-knative", "GH organization name")
	flag.StringVar(&c.Repo, "repo", "serverless-operator", "GH repository name")
	flag.StringVar(&c.FromBranch, "from-branch", "main", "Branch name to copy prow configs from")
	flag.StringVar(&c.Branch, "branch", "", "Target branch name")
	flag.StringVar(&c.Tag, "tag", "", "Target promotion name or tag")
	flag.BoolVar(&c.RemovePeriodic, "remove-periodic-tests", true, "Remove periodic tests")
	flag.StringVar(&c.Remote, "remote", "", "Git remote URL")
	flag.StringVar(&c.Config, "config", filepath.Join("config", "repositories.yaml"), "Specify repositories config")
	flag.Parse()

	prowgenConfig, err := prowgen.LoadConfig(c.Config)
	if err != nil {
		log.Fatalln("Failed to load config", err)
	}

	// Clone openshift/release and clean up existing jobs for the configured branches
	openShiftRelease := prowgen.Repository{
		Org:  "openshift",
		Repo: "release",
	}
	if err := prowgen.InitializeOpenShiftReleaseRepository(ctx, openShiftRelease, &prowgen.Config{}, pointer.String("")); err != nil {
		return err
	}

	if c.FromBranch != c.Branch {
		outConfig := filepath.Join(openShiftRelease.Org, openShiftRelease.Repo, "ci-operator", "config")
		if err := prowgen.DeleteExistingReleaseBuildConfigurationForBranch(&outConfig, prowgen.Repository{Org: c.Org, Repo: c.Repo}, c.Branch); err != nil {
			return err
		}
	}

	files, err := discoverJobConfigs(openShiftRelease, c)
	if err != nil {
		return err
	}
	log.Println("Matching job configs for branch", c.FromBranch, "files", files)

	jobs, err := getJobConfigs(files, c)
	if err != nil {
		return err
	}
	log.Println("Got", len(jobs), "jobs config")

	for _, j := range jobs {
		if err := prowgen.SaveReleaseBuildConfiguration(pointer.String(""), j); err != nil {
			return err
		}
	}

	if err := prowgen.RunOpenShiftReleaseGenerator(ctx, openShiftRelease); err != nil {
		log.Fatalln("Failed to run openshift/release generator:", err)
	}

	if err := mirrorRepositories(ctx, prowgenConfig); err != nil {
		log.Fatalln("Failed to mirror repositories", err)
	}

	if c.FromBranch != c.Branch {
		if err := runProwCopyInjectors(&c, prowgenConfig, openShiftRelease); err != nil {
			log.Fatalln("Failed to run Prow job injectors", err)
		}
	}

	return nil
}

func mirrorRepositories(ctx context.Context, inConfig *prowgen.Config) error {
	repositoryMirrors, generatorsCtx := errgroup.WithContext(ctx)
	for _, r := range inConfig.Repositories {
		r := r
		repositoryMirrors.Go(func() error {
			if err := prowgen.GitMirror(generatorsCtx, r); err != nil {
				return err
			}
			return nil
		})
	}
	if err := repositoryMirrors.Wait(); err != nil {
		return err
	}
	return nil
}

func getJobConfigs(files []string, c Config) ([]prowgen.ReleaseBuildConfiguration, error) {
	jobs := make([]prowgen.ReleaseBuildConfiguration, 0, len(files))
	for _, match := range files {
		jc, err := getJobConfig(match, c)
		if err != nil {
			return nil, fmt.Errorf("failed to get job config for %s: %w", match, err)
		}
		jobs = append(jobs, *jc)
	}

	return transform(jobs, c), nil
}

func discoverJobConfigs(openShiftRelease prowgen.Repository, c Config) ([]string, error) {
	ciConfigDir := filepath.Join(openShiftRelease.RepositoryDirectory(), "ci-operator", "config", c.Org, c.Repo)

	glob := filepath.Join(ciConfigDir, fmt.Sprintf("%s-%s-%s__*.yaml", c.Org, c.Repo, c.FromBranch))
	log.Println(glob)
	return filepath.Glob(glob)
}

func transform(jobs []prowgen.ReleaseBuildConfiguration, c Config) []prowgen.ReleaseBuildConfiguration {
	r := make([]prowgen.ReleaseBuildConfiguration, 0, len(jobs))
	for _, j := range jobs {
		j = removePeriodicTests(j, c)
		r = append(r, j)
	}

	return r
}

func removePeriodicTests(job prowgen.ReleaseBuildConfiguration, c Config) prowgen.ReleaseBuildConfiguration {
	if !c.RemovePeriodic {
		return job
	}

	tests := make([]cioperatorapi.TestStepConfiguration, 0, len(job.Tests))
	for _, t := range job.Tests {
		if t.Cron != nil && *t.Cron != "" {
			continue
		}
		tests = append(tests, *t.DeepCopy())
	}

	r := prowgen.ReleaseBuildConfiguration{
		ReleaseBuildConfiguration: *job.DeepCopy(),
		Path:                      job.Path,
		Branch:                    job.Branch,
	}
	r.Tests = tests

	return r
}

func getJobConfig(match string, c Config) (*prowgen.ReleaseBuildConfiguration, error) {
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

	jobConfig := &prowgen.ReleaseBuildConfiguration{}
	if err := json.Unmarshal(j, jobConfig); err != nil {
		return nil, err
	}

	initialConfig, _ := json.MarshalIndent(jobConfig, "", "  ")
	log.Println("Initial configuration\n", string(initialConfig))

	jobConfig.Path = strings.Replace(match, c.FromBranch, c.Branch, 1)
	jobConfig.Branch = c.Branch
	jobConfig.Metadata.Branch = c.Branch

	if jobConfig.PromotionConfiguration != nil && c.Tag != "" {
		for i := range jobConfig.PromotionConfiguration.Targets {
			if jobConfig.PromotionConfiguration.Targets[i].Name != "" {
				jobConfig.PromotionConfiguration.Targets[i].Name = c.Tag
			}
			if jobConfig.PromotionConfiguration.Targets[i].Tag != "" {
				jobConfig.PromotionConfiguration.Targets[i].Tag = c.Tag
			}
		}
	}

	return jobConfig, nil
}

// JobConfigCopiedInjectors are configured from the base branch. They're applied to
// generated Prow jobs for the target branch.
type JobConfigCopiedInjectors []prowgen.JobConfigInjector

func (jcis JobConfigCopiedInjectors) Inject(prowcopyCfg *Config, prowgenCfg *prowgen.Config, openShiftRelease prowgen.Repository) error {
	for _, jci := range jcis {
		sourceBranchName, targetBranch := prowcopyCfg.FromBranch, prowcopyCfg.Branch
		var sourceBranch *prowgen.Branch
		// Injectors need to be applied to the new branch in the same way as they were applied
		// to the source branch when its config was generated.
		sb, ok := prowgenCfg.Config.Branches[sourceBranchName]
		if !ok {
			return fmt.Errorf("unable to find source branch in config")
		}
		sourceBranch = &sb

		for _, r := range prowgenCfg.Repositories {
			generatedOutputDir := "ci-operator/jobs"
			glob := filepath.Join(openShiftRelease.RepositoryDirectory(), generatedOutputDir, r.RepositoryDirectory(), "*"+targetBranch+"*"+string(jci.Type)+"*")
			matches, err := filepath.Glob(glob)
			if err != nil {
				return err
			}
			for _, match := range matches {
				jobConfig, err := prowgen.GetJobConfig(match)
				if err != nil {
					return err
				}
				if err := jci.Update(&r, sourceBranch, sourceBranchName, jobConfig); err != nil {
					return err
				}
				if err := prowgen.SaveJobConfig(match, jobConfig); err != nil {
					return err
				}
			}
		}
	}

	return nil
}

func runProwCopyInjectors(config *Config, inConfig *prowgen.Config, openShiftRelease prowgen.Repository) error {
	injectors := JobConfigCopiedInjectors{
		prowgen.AlwaysRunInjector(),
	}
	return injectors.Inject(config, inConfig, openShiftRelease)
}
