package prowgen

import (
	"context"
	"fmt"

	"github.com/openshift-knative/hack/pkg/konfluxgen"
	"github.com/openshift-knative/hack/pkg/sobranch"
)

const (
	KonfluxBranchPrefix = "sync-konflux/"
)

func GenerateKonflux(ctx context.Context, openshiftRelease Repository, config *Config) error {

	for _, r := range config.Repositories {
		for branchName, b := range config.Config.Branches {
			if b.Konflux != nil && b.Konflux.Enabled {

				if err := GitMirror(ctx, r); err != nil {
					return err
				}

				if err := GitCheckout(ctx, r, branchName); err != nil {
					return err
				}

				// Special case "release-next"
				targetBranch := branchName
				upstreamVersion := "release-next"
				if branchName == "release-next" {
					targetBranch = "main"
				} else {
					upstreamVersion = sobranch.FromUpstreamVersion(branchName)
				}

				cfg := konfluxgen.Config{
					OpenShiftReleasePath: openshiftRelease.RepositoryDirectory(),
					ApplicationName:      fmt.Sprintf("serverless-operator %s", upstreamVersion),
					Includes: []string{
						fmt.Sprintf("ci-operator/config/%s/.*%s.*.yaml", r.RepositoryDirectory(), branchName),
					},
					Excludes: nil,
					ExcludesImages: []string{
						".*source.*",
						".*test.*",
					},
					ResourcesOutputPath: fmt.Sprintf("%s/.konflux", r.RepositoryDirectory()),
					PipelinesOutputPath: fmt.Sprintf("%s/.tekton", r.RepositoryDirectory()),
					Nudges:              b.Konflux.Nudges,
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

	return nil
}
