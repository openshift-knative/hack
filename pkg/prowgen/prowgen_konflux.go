package prowgen

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"

	"github.com/openshift-knative/hack/pkg/dependabotgen"
	"github.com/openshift-knative/hack/pkg/soversion"
	"golang.org/x/sync/errgroup"

	"github.com/coreos/go-semver/semver"
	cioperatorapi "github.com/openshift/ci-tools/pkg/api"

	"github.com/openshift-knative/hack/pkg/project"

	"github.com/openshift-knative/hack/pkg/konfluxgen"
)

const (
	KonfluxBranchPrefix = "sync-konflux-"
)

var hackRepo = Repository{Org: "openshift-knative", Repo: "hack"}

func GenerateKonflux(ctx context.Context, openshiftRelease Repository, configs []*Config) error {

	if err := GitMirror(ctx, hackRepo); err != nil {
		return err
	}
	if err := GitCheckout(ctx, hackRepo, "main"); err != nil {
		return err
	}

	eg := &errgroup.Group{}
	soMutex := &sync.Mutex{}

	for _, config := range configs {
		config := config
		eg.Go(func() error {

			for _, r := range config.Repositories {

				// Special case serverless-operator
				if r.IsServerlessOperator() {
					soMutex.Lock()
					if err := GenerateKonfluxServerlessOperator(ctx, openshiftRelease, r, config); err != nil {
						soMutex.Unlock()
						return fmt.Errorf("failed to generate konflux for %q: %w", r.RepositoryDirectory(), err)
					}
					soMutex.Unlock()
					continue
				}

				dependabotConfig := dependabotgen.NewDependabotConfig()

				for branchName, b := range config.Config.Branches {
					if b.Konflux != nil && b.Konflux.Enabled {

						// This is a special GH log format: https://docs.github.com/en/actions/writing-workflows/choosing-what-your-workflow-does/workflow-commands-for-github-actions#example-grouping-log-lines
						log.Printf("::group::konfluxgen %s %s\n", r.RepositoryDirectory(), branchName)

						var soVersion *semver.Version
						// Special case "release-next"
						targetBranch := branchName
						soBranchName := "main"
						if branchName == "release-next" {
							targetBranch = "main"
						} else {
							soVersion = soversion.FromUpstreamVersion(branchName)
							soBranchName = soversion.BranchName(soVersion)

							if b.DependabotEnabled == nil || *b.DependabotEnabled {
								dependabotConfig.WithGo(branchName)
								if r.IsEKB() {
									dependabotConfig.WithMaven([]string{"/data-plane"}, branchName)
								}
								if r.IsEventingIntegrations() {
									dependabotConfig.WithNPM([]string{
										"/transform-jsonata",
									}, branchName)
									dependabotConfig.WithMaven([]string{"/"}, branchName)
								}
								if r.IsBackstagePlugins() {
									dependabotConfig.WithNPM([]string{
										"/backstage",
										"/backstage/plugins/knative-event-mesh-backend",
										"/backstage/packages/app",
										"/backstage/packages/backend",
										"/backstage/plugins/knative-event-mesh-backend/dist-dynamic",
									}, branchName)
								}
								if r.IsFunc() {
									dependabotConfig.WithMaven([]string{
										"/templates/quarkus/http",
										"/templates/quarkus/cloudevents",
										"/templates/springboot/http",
										"/templates/springboot/cloudevents",
									}, branchName)
								}
							}
						}

						log.Printf("targetBranch: %s, soBranchName: %s, soVersion: %s\n", targetBranch, soBranchName, soVersion)

						soMutex.Lock()
						// Checkout s-o to get the right release version from project.yaml (e.g. 1.34.1)
						soRepo := Repository{Org: "openshift-knative", Repo: "serverless-operator"}
						if err := GitMirror(ctx, soRepo); err != nil {
							return err
						}

						versionLabel := soBranchName
						var buildArgs []string
						if err := GitCheckout(ctx, soRepo, soBranchName); err != nil {
							if !strings.Contains(err.Error(), "failed to run git [checkout") {
								soMutex.Unlock()
								return err
							}
							// For non-existent branches we use the `.0` patch version if soVersion is set.
							if soVersion != nil {
								versionLabel = soVersion.String()
							}
							// For non-existent branches we keep going and use downstreamVersion for versionLabel.
						} else {
							soProjectYamlPath := filepath.Join(soRepo.RepositoryDirectory(),
								"olm-catalog", "serverless-operator", "project.yaml")
							soMetadata, err := project.ReadMetadataFile(soProjectYamlPath)
							if err != nil {
								soMutex.Unlock()
								return err
							}

							versionLabel = soMetadata.Project.Version
						}
						soMutex.Unlock()
						log.Println("Version label:", versionLabel)
						buildArgs = append(buildArgs, fmt.Sprintf("VERSION=%s", versionLabel))

						soConfig, loadErr := LoadConfig("config/serverless-operator.yaml")
						if loadErr != nil {
							return fmt.Errorf("failed to load config for serverless-operator: %w", loadErr)
						}
						br, ok := soConfig.Config.Branches[soBranchName]
						if !ok {
							br, ok = soConfig.Config.Branches["main"]
							if !ok {
								return fmt.Errorf("main or %s branch configuration not found for serverless-operator", soBranchName)
							}
						}

						overrides := make(map[string]string)
						// add overrides from SO config
						for _, img := range br.Konflux.ImageOverrides {
							if img.Name == "" || img.PullSpec == "" {
								return fmt.Errorf("image override missing name or pull spec: %#v", img)
							}
							overrides[img.Name] = img.PullSpec
						}

						// add overrides from this branch config and let them override the ones from SO
						for _, img := range b.Konflux.ImageOverrides {
							if img.Name == "" || img.PullSpec == "" {
								return fmt.Errorf("image override missing name or pull spec: %#v", img)
							}
							overrides[img.Name] = img.PullSpec
						}

						for name, pullSpec := range overrides {
							buildArgs = append(buildArgs, fmt.Sprintf("%s=%s", name, pullSpec))
						}
						slices.Sort(buildArgs)

						if err := GitMirror(ctx, r); err != nil {
							return err
						}

						if err := GitCheckout(ctx, r, targetBranch); err != nil {
							return err
						}

						pushBranch := fmt.Sprintf("%s%s", KonfluxBranchPrefix, branchName)

						if run := r.RunDockefileGenCommand(); run != "" {
							commitMsg := fmt.Sprintf("Generate dockerfiles with %q", run)
							commands := strings.Split(run, " ")
							var args []string
							if len(commands) > 1 {
								args = commands[1:]
							}
							if out, err := Run(ctx, r, commands[0], args...); err != nil {
								return fmt.Errorf("failed to %s for %q [%s]: %w - %s", commitMsg, r.RepositoryDirectory(), targetBranch, err, string(out))
							}
							if err := PushBranch(ctx, r, nil, pushBranch, commitMsg); err != nil {
								return err
							}
						}

						nudges := b.Konflux.Nudges

						prefetchDeps, err := getPrefetchDeps(r, targetBranch)
						if err != nil {
							return fmt.Errorf("could not get prefetchDeps: %w", err)
						}

						cfg := konfluxgen.Config{
							OpenShiftReleasePath: openshiftRelease.RepositoryDirectory(),
							ApplicationName:      konfluxgen.AppName(soBranchName),
							BuildArgs:            buildArgs,
							Includes: []string{
								fmt.Sprintf("ci-operator/config/%s/.*%s.*.yaml", r.RepositoryDirectory(), branchName),
							},
							Excludes:                  b.Konflux.Excludes,
							ExcludesImages:            b.Konflux.ExcludesImages,
							JavaImages:                b.Konflux.JavaImages,
							ResourcesOutputPath:       fmt.Sprintf("%s/.konflux", r.RepositoryDirectory()),
							RepositoryRootPath:        r.RepositoryDirectory(),
							GlobalResourcesOutputPath: fmt.Sprintf("%s/.konflux", hackRepo.RepositoryDirectory()),
							PipelinesOutputPath:       fmt.Sprintf("%s/.tekton", r.RepositoryDirectory()),
							Nudges:                    nudges,
							// Preserve the version tag as first tag in any instance since SO, when bumping the patch version
							// will change it before merging the PR.
							// See `openshift-knative/serverless-operator/hack/generate/update-pipelines.sh` for more details.
							Tags:         []string{versionLabel},
							PrefetchDeps: *prefetchDeps,
						}
						if len(cfg.ExcludesImages) == 0 {
							cfg.ExcludesImages = []string{
								".*-source-.*",
							}
						}

						if err := konfluxgen.Generate(cfg); err != nil {
							return fmt.Errorf("failed to generate Konflux configurations for %s (%s): %w", r.RepositoryDirectory(), branchName, err)
						}

						if err := dependabotgen.WriteDependabotWorkflow(r.RepositoryDirectory(), r.RunCodegenCommand()); err != nil {
							return fmt.Errorf("[%s][%s] failed to write dependabot workflow: %w", r.RepositoryDirectory(), branchName, err)
						}

						commitMsg := fmt.Sprintf("[%s] Sync Konflux configurations", targetBranch)
						if err := PushBranch(ctx, r, nil, pushBranch, commitMsg); err != nil {
							return err
						}

						// This is a special GH log format: https://docs.github.com/en/actions/writing-workflows/choosing-what-your-workflow-does/workflow-commands-for-github-actions#example-grouping-log-lines
						log.Printf("::endgroup::\n\n")
					}
				}

				if err := writeDependabotConfig(ctx, dependabotConfig, r); err != nil {
					return err
				}

			}
			return nil
		})
	}

	if err := eg.Wait(); err != nil {
		return fmt.Errorf("eg.Wait(): %w", err)
	}

	commitMsg := fmt.Sprintf("Sync Konflux configurations for serverless operator")
	if err := PushBranch(ctx, hackRepo, nil, fmt.Sprintf("%s%s", KonfluxBranchPrefix, "main"), commitMsg); err != nil {
		return err
	}

	return nil
}

func writeDependabotConfig(ctx context.Context, dependabotConfig *dependabotgen.DependabotConfig, r Repository) error {
	if dependabotConfig.Updates != nil && len(*dependabotConfig.Updates) > 0 {
		if err := GitMirror(ctx, r); err != nil {
			return err
		}

		if err := GitCheckout(ctx, r, dependabotgen.DefaultTargetBranch); err != nil {
			return err
		}
		if err := dependabotConfig.Write(r.RepositoryDirectory(), r.RunCodegenCommand()); err != nil {
			return fmt.Errorf("[%s] %w", r.RepositoryDirectory(), err)
		}

		pushBranch := fmt.Sprintf("%s%s", dependabotgen.SyncBranchPrefix, dependabotgen.DefaultTargetBranch)
		commitMsg := fmt.Sprintf("Update dependabot configurations")

		if err := PushBranch(ctx, r, nil, pushBranch, commitMsg); err != nil {
			return err
		}
	} else {
		log.Println("No dependabot configurations")
	}
	return nil
}

func ServerlessOperatorKonfluxVersions(ctx context.Context) (map[string]string, error) {
	r := Repository{Org: "openshift-knative", Repo: "serverless-operator"}
	sortedBranches, err := Branches(ctx, r)
	if err != nil {
		return nil, fmt.Errorf("failed to list branches for %q: %w", r.RepositoryDirectory(), err)
	}

	config, err := LoadConfig("config/serverless-operator.yaml")
	if err != nil {
		return nil, fmt.Errorf("failed to load config for %q: %w", r.RepositoryDirectory(), err)
	}

	konfluxVersions := make(map[string]string, len(sortedBranches))
	for i, branch := range sortedBranches {
		// First branch we will build the operator with konflux
		if v := os.Getenv("FIRST_KONFLUX_BRANCH"); (v != "" && v == branch) || branch == "release-1.35" {
			for j := i; j < len(sortedBranches); j++ {
				konfluxVersions[sortedBranches[j]] = sortedBranches[j]
			}
		}

		if b, ok := config.Config.Branches["main"]; ok && b.Konflux.Enabled {
			last := sortedBranches[len(sortedBranches)-1]
			last = strings.ReplaceAll(last, "release-v", "")
			last = strings.ReplaceAll(last, "release-", "")
			last += ".0"
			v := semver.New(last)
			v.BumpMinor()
			konfluxVersions[fmt.Sprintf("release-%d.%d", v.Major, v.Minor)] = "main"
		}
	}

	log.Println("serverless operator - konflux versions", konfluxVersions)

	return konfluxVersions, nil
}

func GenerateKonfluxServerlessOperator(ctx context.Context, openshiftRelease Repository, r Repository, config *Config) error {

	dependabotConfig := dependabotgen.NewDependabotConfig()

	konfluxVersions, err := ServerlessOperatorKonfluxVersions(ctx)
	if err != nil {
		return fmt.Errorf("failed to get Konflux versions for serverless operator: %w", err)
	}
	log.Println("Recreating konflux configurations for serverless operator")

	resourceOutputPath := fmt.Sprintf("%s/.konflux", hackRepo.RepositoryDirectory())

	for release, branch := range konfluxVersions {

		// This is a special GH log format: https://docs.github.com/en/actions/writing-workflows/choosing-what-your-workflow-does/workflow-commands-for-github-actions#example-grouping-log-lines
		log.Printf("::group::konfluxgen %s %s %s\n", r.RepositoryDirectory(), branch, release)
		log.Println("Creating Konflux configuration for serverless operator", branch, release)

		dependabotConfig.WithGo(branch)

		if err := GitMirror(ctx, r); err != nil {
			return err
		}

		if err := GitCheckout(ctx, r, branch); err != nil {
			return err
		}

		// Use configuration for main branch if branch-specific configuration is not present.
		b, ok := config.Config.Branches[branch]
		if !ok {
			b, ok = config.Config.Branches["main"]
			if !ok {
				return fmt.Errorf("main or %s branch configuration not found for %q", branch, r.RepositoryDirectory())
			}
			log.Printf("Using configuration for branch main")
		}

		soProjectYamlPath := filepath.Join(r.RepositoryDirectory(),
			"olm-catalog", "serverless-operator", "project.yaml")
		soMetadata, err := project.ReadMetadataFile(soProjectYamlPath)
		if err != nil {
			return err
		}
		buildArgs := []string{fmt.Sprintf("VERSION=%s", soMetadata.Project.Version)}

		cliImage, err := getCLIArtifactsImage(soMetadata.Requirements.OcpVersion.Min)
		if err != nil {
			return fmt.Errorf("failed to get cli artifacts image for OCP %s: %w", soMetadata.Requirements.OcpVersion.Min, err)
		}

		buildArgs = append(buildArgs, fmt.Sprintf("CLI_ARTIFACTS=%s", cliImage))

		for _, img := range b.Konflux.ImageOverrides {
			if img.Name == "" || img.PullSpec == "" {
				return fmt.Errorf("image override missing name or pull spec: %#v", img)
			}
			buildArgs = append(buildArgs, fmt.Sprintf("%s=%s", img.Name, img.PullSpec))
		}

		prefetchDeps, err := getPrefetchDeps(r, branch)
		if err != nil {
			return fmt.Errorf("could not get prefetchDeps: %w", err)
		}

		semverRelease, err := SemverFromReleaseBranch(release)
		if err != nil {
			return fmt.Errorf("Failed to get semver from release branch %q: %w", release, err)
		}

		cfg := konfluxgen.Config{
			OpenShiftReleasePath: openshiftRelease.RepositoryDirectory(),
			ApplicationName:      konfluxgen.AppName(release),
			BuildArgs:            buildArgs,
			ComponentNameFunc: func(cfg cioperatorapi.ReleaseBuildConfiguration, ib cioperatorapi.ProjectDirectoryImageBuildStepConfiguration) string {
				return fmt.Sprintf("%s-%s", ib.To, release)
			},
			AdditionalTektonCELExpressionFunc: func(cfg cioperatorapi.ReleaseBuildConfiguration, ib cioperatorapi.ProjectDirectoryImageBuildStepConfiguration) string {
				if string(ib.To) == "serverless-bundle" {
					return "&& (" +
						" files.all.exists(x, x.matches('^olm-catalog/serverless-operator/')) ||" +
						" files.all.exists(x, x.matches('^.tekton/'))" +
						" )"
				}
				return "&& files.all.exists(x, !x.matches('^olm-catalog/') && !x.matches('^.konflux-release/'))"
			},
			Includes: []string{
				fmt.Sprintf("ci-operator/config/%s/.*%s.*.yaml", r.RepositoryDirectory(), branch),
			},
			Excludes:       b.Konflux.Excludes,
			ExcludesImages: b.Konflux.ExcludesImages,
			JavaImages:     b.Konflux.JavaImages,
			// Use hack repo to store configurations for Serverless operator since when we cut
			// the branch we could have conflicting components for a new release branch and
			// main with the same name but different "revision" (branch).
			ResourcesOutputPathSkipRemove: true,
			ResourcesOutputPath:           resourceOutputPath,
			RepositoryRootPath:            r.RepositoryDirectory(),
			GlobalResourcesOutputPath:     resourceOutputPath,
			PipelinesOutputPath:           fmt.Sprintf("%s/.tekton", r.RepositoryDirectory()),
			Nudges:                        b.Konflux.Nudges,
			ComponentReleasePlanConfig: &konfluxgen.ComponentReleasePlanConfig{
				FirstRelease:              semverRelease,
				ClusterServiceVersionPath: filepath.Join(r.RepositoryDirectory(), "olm-catalog", "serverless-operator", "manifests", "serverless-operator.clusterserviceversion.yaml"),
				BundleComponentName:       "serverless-bundle",
				BundleImageRepoName:       "serverless-operator-bundle",
			},
			// Preserve the version tag as first tag in any instance since SO, when bumping the patch version
			// will change it before merging the PR.
			// See `openshift-knative/serverless-operator/hack/generate/update-pipelines.sh` for more details.
			Tags:         []string{soMetadata.Project.Version},
			PrefetchDeps: *prefetchDeps,
		}
		if len(cfg.ExcludesImages) == 0 {
			cfg.ExcludesImages = []string{
				".*operator-src.*",
				".*-source-.*",
				".*serverless-index.*",
			}
		}

		if err := konfluxgen.Generate(cfg); err != nil {
			return fmt.Errorf("failed to generate Konflux configurations for %s (%s): %w", r.RepositoryDirectory(), branch, err)
		}

		if err := generateFBCApplications(soMetadata, openshiftRelease, r, branch, release, resourceOutputPath, buildArgs); err != nil {
			return fmt.Errorf("failed to generate FBC applications for %s (%s): %w", r.RepositoryDirectory(), branch, err)
		}

		if err := dependabotgen.WriteDependabotWorkflow(r.RepositoryDirectory(), r.RunCodegenCommand()); err != nil {
			return fmt.Errorf("[%s][%s] failed to write dependabot workflow: %w", r.RepositoryDirectory(), branch, err)
		}

		pushBranch := fmt.Sprintf("%s%s", KonfluxBranchPrefix, branch)
		commitMsg := fmt.Sprintf("[%s] Sync Konflux configurations", release)

		if err := PushBranch(ctx, r, nil, pushBranch, commitMsg); err != nil {
			return err
		}

		// This is a special GH log format: https://docs.github.com/en/actions/writing-workflows/choosing-what-your-workflow-does/workflow-commands-for-github-actions#example-grouping-log-lines
		log.Printf("::endgroup::\n\n")
	}

	if err := writeDependabotConfig(ctx, dependabotConfig, r); err != nil {
		return err
	}

	return nil
}

func getCLIArtifactsImage(ocpVersion string) (string, error) {
	parts := strings.SplitN(ocpVersion, ".", 2)
	if len(parts) != 2 {
		return "", fmt.Errorf("invalid OCP version: %s", ocpVersion)
	}

	minor, err := strconv.Atoi(parts[1])
	if err != nil {
		return "", fmt.Errorf("could not convert OCP minor to int (%q): %w", ocpVersion, err)
	}

	if minor <= 15 {
		return fmt.Sprintf("registry.redhat.io/openshift4/ose-cli-artifacts:v4.%d", minor), nil
	} else {
		// use RHEL9 variant for OCP version > 4.15
		return fmt.Sprintf("registry.redhat.io/openshift4/ose-cli-artifacts-rhel9:v4.%d", minor), nil
	}
}

func getPrefetchDeps(repo Repository, branch string) (*konfluxgen.PrefetchDeps, error) {
	prefetchDeps := konfluxgen.PrefetchDeps{}
	if _, err := os.Stat(filepath.Join(repo.RepositoryDirectory(), "rpms.lock.yaml")); err == nil {
		// If rpms.lock.yaml is present enable dev-package-managers and RPM caching
		prefetchDeps.DevPackageManagers = "true"
		prefetchDeps.WithRPMs()
	}

	_, err := os.Stat(filepath.Join(repo.RepositoryDirectory(), "vendor"))
	if err != nil {
		if !errors.Is(err, fs.ErrNotExist) {
			return nil, fmt.Errorf("[%s - %s] failed to verify if the project uses Go vendoring: %w", repo.RepositoryDirectory(), branch, err)
		}
		if _, err := os.Stat(filepath.Join(repo.RepositoryDirectory(), "go.mod")); err == nil {
			// If it's a Go project and no vendor dir is present enable Go caching
			prefetchDeps.WithUnvendoredGo("." /* root of the repository */)
		}
	}

	if repo.IsEventingIntegrations() {
		prefetchDeps.WithNPM("transform-jsonata")
	}

	return &prefetchDeps, nil
}

func generateFBCApplications(soMetadata *project.Metadata, openshiftRelease Repository, r Repository, branch string, release string, resourceOutputPath string, buildArgs []string) error {
	fbcApps := make([]string, 0, len(soMetadata.Requirements.OcpVersion.List))

	for _, ocpVersion := range soMetadata.Requirements.OcpVersion.List {

		opmImage, err := getOPMImage(ocpVersion)
		if err != nil {
			return fmt.Errorf("failed to get OPM image ref for OCP %q: %w", ocpVersion, err)
		}
		buildArgs := append(buildArgs, fmt.Sprintf("OPM_IMAGE=%s", opmImage))

		fbcAppName := konfluxgen.FBCAppName(release, ocpVersion)

		c := konfluxgen.Config{
			OpenShiftReleasePath: openshiftRelease.RepositoryDirectory(),
			ApplicationName:      fbcAppName,
			BuildArgs:            buildArgs,
			ResourcesOutputPath:  resourceOutputPath,
			RepositoryRootPath:   r.RepositoryDirectory(),
			PipelinesOutputPath:  fmt.Sprintf("%s/.tekton", r.RepositoryDirectory()),
			AdditionalTektonCELExpressionFunc: func(cfg cioperatorapi.ReleaseBuildConfiguration, ib cioperatorapi.ProjectDirectoryImageBuildStepConfiguration) string {
				return fmt.Sprintf("&& ("+
					" files.all.exists(x, x.matches('^olm-catalog/serverless-operator-index/v%s/')) ||"+
					" files.all.exists(x, x.matches('^.tekton/'))"+
					" )", ocpVersion)
			},
			AdditionalComponentConfigs: []konfluxgen.TemplateConfig{
				{
					ReleaseBuildConfiguration: cioperatorapi.ReleaseBuildConfiguration{
						Metadata: cioperatorapi.Metadata{
							Org:    r.Org,
							Repo:   r.Repo,
							Branch: branch,
						},
						Images: []cioperatorapi.ProjectDirectoryImageBuildStepConfiguration{
							{
								To: cioperatorapi.PipelineImageStreamTagReference(fmt.Sprintf("serverless-index-%s-fbc-%s", release, ocpVersion)),
								ProjectDirectoryImageBuildInputs: cioperatorapi.ProjectDirectoryImageBuildInputs{
									DockerfilePath: "Dockerfile",
									ContextDir:     fmt.Sprintf("./olm-catalog/serverless-operator-index/v%s", ocpVersion),
								},
							},
						},
					},
				},
			},
			ComponentNameFunc: func(cfg cioperatorapi.ReleaseBuildConfiguration, ib cioperatorapi.ProjectDirectoryImageBuildStepConfiguration) string {
				return string(ib.To)
			},
			FBCImages: []string{
				fmt.Sprintf("serverless-index-%s-fbc-%s", release, ocpVersion),
			},
			ResourcesOutputPathSkipRemove: true,
			PipelinesOutputPathSkipRemove: true,
			IsHermetic: func(_ cioperatorapi.ReleaseBuildConfiguration, _ cioperatorapi.ProjectDirectoryImageBuildStepConfiguration) bool {
				return true
			},
			// Preserve the version tag as first tag in any instance since SO, when bumping the patch version
			// will change it before merging the PR.
			// See `openshift-knative/serverless-operator/hack/generate/update-pipelines.sh` for more details.
			Tags: []string{soMetadata.Project.Version},
		}

		if err := konfluxgen.Generate(c); err != nil {
			return fmt.Errorf("failed to generate Konflux FBC configurations for %s (%s): %w", r.RepositoryDirectory(), branch, err)
		}

		fbcApps = append(fbcApps, fbcAppName)
	}

	appName := fmt.Sprintf("serverless-operator %s", release)
	if err := konfluxgen.GenerateFBCReleasePlanAdmission(fbcApps, resourceOutputPath, appName, soMetadata.Project.Version); err != nil {
		return fmt.Errorf("failed to generate ReleasePlanAdmissions for FBC of %s (%s): %w", r.RepositoryDirectory(), branch, err)
	}
	if err := konfluxgen.GenerateReleasePlans(fbcApps, resourceOutputPath, appName, soMetadata.Project.Version); err != nil {
		return fmt.Errorf("failed to generate ReleasePlan for FBC of %s (%s): %w", r.RepositoryDirectory(), branch, err)
	}

	return nil
}

func getOPMImage(v string) (string, error) {
	parts := strings.SplitN(v, ".", 2)
	if len(parts) != 2 {
		return "", fmt.Errorf("invalid OCP version: %s", v)
	}

	minor, err := strconv.Atoi(parts[1])
	if err != nil {
		return "", fmt.Errorf("could not convert OCP minor to int (%q): %w", v, err)
	}

	if minor <= 14 {
		return fmt.Sprintf("registry.redhat.io/openshift4/ose-operator-registry:v4.%d", minor), nil
	} else {
		// use RHEL9 variant for OCP version >= 4.15
		return fmt.Sprintf("brew.registry.redhat.io/rh-osbs/openshift-ose-operator-registry-rhel9:v4.%d", minor), nil
	}
}
