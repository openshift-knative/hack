package prowgen

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/openshift-knative/hack/pkg/soversion"

	"github.com/coreos/go-semver/semver"
	cioperatorapi "github.com/openshift/ci-tools/pkg/api"

	"github.com/openshift-knative/hack/pkg/project"

	"github.com/openshift-knative/hack/pkg/konfluxgen"
)

const (
	KonfluxBranchPrefix = "sync-konflux-"
)

func GenerateKonflux(ctx context.Context, openshiftRelease Repository, configs []*Config) error {

	operatorVersions, err := ServerlessOperatorKonfluxVersions(ctx)
	if err != nil {
		return fmt.Errorf("failed to get konflux versions for serverless-operator: %w", err)
	}

	for _, config := range configs {
		for _, r := range config.Repositories {

			// Special case serverless-operator
			if r.Org == "openshift-knative" && r.Repo == "serverless-operator" {
				if err := GenerateKonfluxServerlessOperator(ctx, openshiftRelease, r, config); err != nil {
					return fmt.Errorf("failed to generate konflux for %q: %w", r.RepositoryDirectory(), err)
				}
				continue
			}

			for branchName, b := range config.Config.Branches {
				if b.Konflux != nil && b.Konflux.Enabled {

					var soVersion *semver.Version
					// Special case "release-next"
					targetBranch := branchName
					soBranchName := "main"
					if branchName == "release-next" {
						targetBranch = "main"
					} else {
						soVersion = soversion.FromUpstreamVersion(branchName)
						soBranchName = soversion.BranchName(soVersion)
					}

					// Checkout s-o to get the right release version from project.yaml (e.g. 1.34.1)
					soRepo := Repository{Org: "openshift-knative", Repo: "serverless-operator"}
					if err := GitMirror(ctx, soRepo); err != nil {
						return err
					}

					versionLabel := soBranchName
					var buildArgs []string
					if err := GitCheckout(ctx, soRepo, soBranchName); err != nil {
						if !strings.Contains(err.Error(), "failed to run git [checkout") {
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
							return err
						}
						versionLabel = soMetadata.Project.Version
					}
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
					for _, img := range br.Konflux.ImageOverrides {
						buildArgs = append(buildArgs, fmt.Sprintf("%s=%s", img.Name, img.PullSpec))
					}

					if err := GitMirror(ctx, r); err != nil {
						return err
					}

					if err := GitCheckout(ctx, r, targetBranch); err != nil {
						return err
					}

					nudges := b.Konflux.Nudges
					if soBranchName != "release-next" {
						_, ok := operatorVersions[soBranchName]
						if ok {
							nudges = append(nudges, serverlessBundleNudge(soBranchName))
						}
						log.Printf("[%s] created nudges (%v) - operatorVersions: %#v - downstreamVersion: %v): %#v", r.RepositoryDirectory(), ok, operatorVersions, soBranchName, nudges)
					}

					prefetchDeps := konfluxgen.PrefetchDeps{}
					if _, err := os.Stat(filepath.Join(r.RepositoryDirectory(), "rpms.lock.yaml")); err == nil {
						// If rpms.lock.yaml is present enable dev-package-managers and RPM caching
						prefetchDeps.DevPackageManagers = "true"
						prefetchDeps.WithRPMs()
					}
					_, err := os.Stat(filepath.Join(r.RepositoryDirectory(), "vendor"))
					if err != nil {
						if !errors.Is(err, fs.ErrNotExist) {
							return fmt.Errorf("[%s - %s] failed to verify if the project uses Go vendoring: %w", r.RepositoryDirectory(), targetBranch, err)
						}
						if _, err := os.Stat(filepath.Join(r.RepositoryDirectory(), "go.mod")); err == nil {
							// If it's a Go project and no vendor dir is present enable Go caching
							prefetchDeps.WithUnvendoredGo("." /* root of the repository */)
						}
					}

					cfg := konfluxgen.Config{
						OpenShiftReleasePath: openshiftRelease.RepositoryDirectory(),
						ApplicationName:      fmt.Sprintf("serverless-operator %s", soBranchName),
						BuildArgs:            buildArgs,
						Includes: []string{
							fmt.Sprintf("ci-operator/config/%s/.*%s.*.yaml", r.RepositoryDirectory(), branchName),
						},
						Excludes:            b.Konflux.Excludes,
						ExcludesImages:      b.Konflux.ExcludesImages,
						FBCImages:           b.Konflux.FBCImages,
						JavaImages:          b.Konflux.JavaImages,
						ResourcesOutputPath: fmt.Sprintf("%s/.konflux", r.RepositoryDirectory()),
						PipelinesOutputPath: fmt.Sprintf("%s/.tekton", r.RepositoryDirectory()),
						Nudges:              nudges,
						Tags:                []string{versionLabel},
						PrefetchDeps:        prefetchDeps,
					}
					if len(cfg.ExcludesImages) == 0 {
						cfg.ExcludesImages = []string{
							".*source.*",
						}
					}

					if err := konfluxgen.Generate(cfg); err != nil {
						return fmt.Errorf("failed to generate Konflux configurations for %s (%s): %w", r.RepositoryDirectory(), branchName, err)
					}

					pushBranch := fmt.Sprintf("%s%s", KonfluxBranchPrefix, branchName)
					commitMsg := fmt.Sprintf("[%s] Sync Konflux configurations", targetBranch)

					if err := PushBranch(ctx, r, nil, pushBranch, commitMsg); err != nil {
						return err
					}
				}
			}
		}
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

	konfluxVersions, err := ServerlessOperatorKonfluxVersions(ctx)
	if err != nil {
		return fmt.Errorf("failed to get Konflux versions for serverless operator: %w", err)
	}

	hackRepo := Repository{Org: "openshift-knative", Repo: "hack"}
	if err := GitMirror(ctx, hackRepo); err != nil {
		return err
	}
	if err := GitCheckout(ctx, hackRepo, "main"); err != nil {
		return err
	}
	log.Println("Recreating konflux configurations for serverless operator")

	resourceOutputPath := fmt.Sprintf("%s/.konflux", hackRepo.RepositoryDirectory())
	if err := os.RemoveAll(resourceOutputPath); err != nil {
		return fmt.Errorf("failed to remove %q directory: %w", resourceOutputPath, err)
	}

	for release, branch := range konfluxVersions {

		log.Println("Creating Konflux configuration for serverless operator", release)

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
		}

		soProjectYamlPath := filepath.Join(r.RepositoryDirectory(),
			"olm-catalog", "serverless-operator", "project.yaml")
		soMetadata, err := project.ReadMetadataFile(soProjectYamlPath)
		if err != nil {
			return err
		}
		buildArgs := []string{fmt.Sprintf("VERSION=%s", soMetadata.Project.Version)}

		for _, img := range b.Konflux.ImageOverrides {
			buildArgs = append(buildArgs, fmt.Sprintf("%s=%s", img.Name, img.PullSpec))
		}

		cfg := konfluxgen.Config{
			OpenShiftReleasePath: openshiftRelease.RepositoryDirectory(),
			ApplicationName:      fmt.Sprintf("serverless-operator %s", release),
			BuildArgs:            buildArgs,
			ComponentNameFunc: func(cfg cioperatorapi.ReleaseBuildConfiguration, ib cioperatorapi.ProjectDirectoryImageBuildStepConfiguration) string {
				return fmt.Sprintf("%s-%s", ib.To, release)
			},
			AdditionalTektonCELExpressionFunc: func(cfg cioperatorapi.ReleaseBuildConfiguration, ib cioperatorapi.ProjectDirectoryImageBuildStepConfiguration) string {
				if string(ib.To) == "serverless-index" {
					return "&& (" +
						" files.all.exists(x, x.matches('^olm-catalog/serverless-operator-index/')) ||" +
						" files.all.exists(x, x.matches('^.tekton/'))" +
						" )"
				}
				if string(ib.To) == "serverless-bundle" {
					return "&& (" +
						" files.all.exists(x, x.matches('^olm-catalog/serverless-operator/')) ||" +
						" files.all.exists(x, x.matches('^.tekton/'))" +
						" )"
				}
				nonCatalog := " && files.all.exists(x, !x.matches('^olm-catalog/'))"
				return nonCatalog
			},
			Includes: []string{
				fmt.Sprintf("ci-operator/config/%s/.*%s.*.yaml", r.RepositoryDirectory(), branch),
			},
			Excludes:       b.Konflux.Excludes,
			ExcludesImages: b.Konflux.ExcludesImages,
			FBCImages:      b.Konflux.FBCImages,
			JavaImages:     b.Konflux.JavaImages,
			// Use hack repo to store configurations for Serverless operator since when we cut
			// the branch we could have conflicting components for a new release branch and
			// main with the same name but different "revision" (branch).
			ResourcesOutputPathSkipRemove: true,
			ResourcesOutputPath:           resourceOutputPath,
			PipelinesOutputPath:           fmt.Sprintf("%s/.tekton", r.RepositoryDirectory()),
			Nudges:                        b.Konflux.Nudges,
			NudgesFunc: func(cfg cioperatorapi.ReleaseBuildConfiguration, ib cioperatorapi.ProjectDirectoryImageBuildStepConfiguration) []string {
				if strings.Contains(string(ib.To), "serverless-index") {
					return nil
				}
				if strings.Contains(string(ib.To), "serverless-bundle") {
					return serverlessIndexNudges(release, soMetadata.Requirements.OcpVersion.List)
				}
				return []string{serverlessBundleNudge(release)}
			},
			ClusterServiceVersionPath: filepath.Join(r.RepositoryDirectory(), "olm-catalog", "serverless-operator", "manifests", "serverless-operator.clusterserviceversion.yaml"),
			Tags:                      []string{soMetadata.Project.Version},
		}
		if len(cfg.ExcludesImages) == 0 {
			cfg.ExcludesImages = []string{
				".*operator-src.*",
				".*source.*",
			}
		}

		if err := konfluxgen.Generate(cfg); err != nil {
			return fmt.Errorf("failed to generate Konflux configurations for %s (%s): %w", r.RepositoryDirectory(), branch, err)
		}

		if err := generateFBCApplications(soMetadata, openshiftRelease, r, branch, release, resourceOutputPath, buildArgs); err != nil {
			return fmt.Errorf("failed to generate FBC applications for %s (%s): %w", r.RepositoryDirectory(), branch, err)
		}

		pushBranch := fmt.Sprintf("%s%s", KonfluxBranchPrefix, branch)
		commitMsg := fmt.Sprintf("[%s] Sync Konflux configurations", release)

		if err := PushBranch(ctx, r, nil, pushBranch, commitMsg); err != nil {
			return err
		}
	}

	commitMsg := fmt.Sprintf("Sync Konflux configurations for serverless operator")
	if err := PushBranch(ctx, hackRepo, nil, fmt.Sprintf("%s%s", KonfluxBranchPrefix, "main"), commitMsg); err != nil {
		return err
	}

	return nil
}

func generateFBCApplications(soMetadata *project.Metadata, openshiftRelease Repository, r Repository, branch string, release string, resourceOutputPath string, buildArgs []string) error {
	for _, v := range soMetadata.Requirements.OcpVersion.List {
		cfg := konfluxgen.FBCConfig{
			ApplicationName:     fmt.Sprintf("serverless-operator %s FBC %s", release, v),
			BuildArgs:           buildArgs,
			ResourcesOutputPath: resourceOutputPath,
			PipelinesOutputPath: fmt.Sprintf("%s/.tekton", r.RepositoryDirectory()),
			OCPVersion:          v,
			Metadata: cioperatorapi.Metadata{
				Org:    r.Org,
				Repo:   r.Repo,
				Branch: branch,
			},
			AdditionalTektonCELExpressions: fmt.Sprintf(" && ("+
				" files.all.exists(x, x.matches('^olm-catalog/serverless-operator-index/v%s/')) ||"+
				" files.all.exists(x, x.matches('^.tekton/'))"+
				" )", v),
			DockerfilePath: "Dockerfile",
			ContextDirPath: fmt.Sprintf("./olm-catalog/serverless-operator-index/v%s", v),
		}

		if err := konfluxgen.GenerateFBCApp(cfg); err != nil {
			return fmt.Errorf("failed to generate Konflux FBC configurations for %s (%s): %w", r.RepositoryDirectory(), branch, err)
		}
	}

	return nil
}

func serverlessBundleNudge(downstreamVersion string) string {
	return konfluxgen.Truncate(konfluxgen.Sanitize(fmt.Sprintf("%s-%s", "serverless-bundle", downstreamVersion)))
}

func serverlessIndexNudges(downstreamVersion string, ocpVersions []string) []string {
	indexes := []string{
		konfluxgen.Truncate(konfluxgen.Sanitize(fmt.Sprintf("%s-%s", "serverless-index", downstreamVersion))),
	}

	for _, v := range ocpVersions {
		indexes = append(indexes, konfluxgen.Truncate(konfluxgen.Sanitize(fmt.Sprintf("serverless-operator-%s-fbc-%s-index", downstreamVersion, v))))
	}

	return indexes
}
