package prowgen

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/coreos/go-semver/semver"
	cioperatorapi "github.com/openshift/ci-tools/pkg/api"

	"github.com/openshift-knative/hack/pkg/project"

	"github.com/openshift-knative/hack/pkg/konfluxgen"
	"github.com/openshift-knative/hack/pkg/sobranch"
)

const (
	KonfluxBranchPrefix = "sync-konflux/"
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

					// Special case "release-next"
					targetBranch := branchName
					downstreamVersion := "release-next"
					if branchName == "release-next" {
						targetBranch = "main"
					} else {
						downstreamVersion = sobranch.FromUpstreamVersion(branchName)
					}

					// Checkout s-o to get the right release version from project.yaml (e.g. 1.34.1)
					soRepo := Repository{Org: "openshift-knative", Repo: "serverless-operator"}
					if err := GitMirror(ctx, soRepo); err != nil {
						return err
					}

					versionLabel := downstreamVersion
					var buildArgs []string
					if err := GitCheckout(ctx, soRepo, downstreamVersion); err != nil {
						// For non-existent branches we keep going and use downstreamVersion for versionLabel.
						if !strings.Contains(err.Error(), "failed to run git [checkout") {
							return err
						}
					} else {
						soProjectYamlPath := filepath.Join(soRepo.RepositoryDirectory(),
							"olm-catalog", "serverless-operator", "project.yaml")
						soMetadata, err := project.ReadMetadataFile(soProjectYamlPath)
						if err != nil {
							return err
						}
						versionLabel = soMetadata.Project.Version
						for _, img := range soMetadata.ImageOverrides {
							buildArgs = append(buildArgs, fmt.Sprintf("%s=%s", img.Name, img.PullSpec))
						}
					}
					log.Println("Version label:", versionLabel)
					buildArgs = append(buildArgs, fmt.Sprintf("VERSION=%s", versionLabel))

					if err := GitMirror(ctx, r); err != nil {
						return err
					}

					if err := GitCheckout(ctx, r, targetBranch); err != nil {
						return err
					}

					nudges := b.Konflux.Nudges
					if downstreamVersion != "release-next" {
						_, ok := operatorVersions[downstreamVersion]
						if ok {
							nudges = append(nudges, serverlessBundleNudge(downstreamVersion))
						}
						log.Printf("[%s] created nudges (%v) - operatorVersions: %#v - downstreamVersion: %v): %#v", r.RepositoryDirectory(), ok, operatorVersions, downstreamVersion, nudges)
					}

					cfg := konfluxgen.Config{
						OpenShiftReleasePath: openshiftRelease.RepositoryDirectory(),
						ApplicationName:      fmt.Sprintf("serverless-operator %s", downstreamVersion),
						BuildArgs:            buildArgs,
						Includes: []string{
							fmt.Sprintf("ci-operator/config/%s/.*%s.*.yaml", r.RepositoryDirectory(), branchName),
						},
						Excludes:            b.Konflux.Excludes,
						ExcludesImages:      b.Konflux.ExcludesImages,
						FBCImages:           b.Konflux.FBCImages,
						ResourcesOutputPath: fmt.Sprintf("%s/.konflux", r.RepositoryDirectory()),
						PipelinesOutputPath: fmt.Sprintf("%s/.tekton", r.RepositoryDirectory()),
						Nudges:              nudges,
						Tags:                []string{versionLabel},
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
		for _, img := range soMetadata.ImageOverrides {
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
				if string(ib.To) == "serverless-openshift-knative-operator" {
					return "&& files.all.exists(x, !x.matches('^olm-catalog/|^knative-operator/'))"
				}
				if string(ib.To) == "serverless-knative-operator" {
					return "&& files.all.exists(x, !x.matches('^olm-catalog/|^openshift-knative-operator/'))"
				}
				return ""
			},
			Includes: []string{
				fmt.Sprintf("ci-operator/config/%s/.*%s.*.yaml", r.RepositoryDirectory(), branch),
			},
			Excludes:       b.Konflux.Excludes,
			ExcludesImages: b.Konflux.ExcludesImages,
			FBCImages:      b.Konflux.FBCImages,
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
					return []string{serverlessIndexNudge(release)}
				}
				return []string{serverlessBundleNudge(release)}
			},
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

func serverlessBundleNudge(downstreamVersion string) string {
	return konfluxgen.Truncate(konfluxgen.Sanitize(fmt.Sprintf("%s-%s", "serverless-bundle", downstreamVersion)))
}

func serverlessIndexNudge(downstreamVersion string) string {
	return konfluxgen.Truncate(konfluxgen.Sanitize(fmt.Sprintf("%s-%s", "serverless-index", downstreamVersion)))
}
