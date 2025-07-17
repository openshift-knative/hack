package prowgen

import (
	"context"
	"fmt"
	"log"

	"github.com/openshift-knative/hack/pkg/ownersfilegen"
	"golang.org/x/sync/errgroup"
	"k8s.io/utils/strings/slices"
)

func GenerateOwners(ctx context.Context, configs []*Config) error {
	if err := GitMirror(ctx, hackRepo); err != nil {
		return err
	}
	if err := GitCheckout(ctx, hackRepo, "main"); err != nil {
		return err
	}

	eg := &errgroup.Group{}

	for _, config := range configs {
		config := config
		eg.Go(func() error {
			for _, r := range config.Repositories {
				branchesInGit, err := Branches(ctx, r, "*")
				if err != nil {
					return fmt.Errorf("could not get branches for %q: %w", r.Repo, err)
				}

				for branchName := range config.Config.Branches {
					if branchName == "release-next" {
						// skip updates on release-next
						continue
					}

					if !slices.Contains(branchesInGit, branchName) {
						// some repos have branch configs, but the branches are not cut yet
						// (e.g. SO with the latest release branch). So we skip those.
						log.Printf("Skipping branch %q for %q, because banch does not exist in Git yet", branchName, r.Repo)
						continue
					}

					if err := createOwnersFile(ctx, r, branchName); err != nil {
						return fmt.Errorf("failed to create ownersfile for branch %s: %w", branchName, err)
					}
				}

				if _, ok := config.Config.Branches["main"]; !ok {
					// no main branch in config list. Create it out of the loop for main
					if err := createOwnersFile(ctx, r, "main"); err != nil {
						return fmt.Errorf("failed to create ownersfile for branch main: %w", err)
					}
				}
			}

			return nil
		})
	}

	if err := eg.Wait(); err != nil {
		return fmt.Errorf("eg.Wait(): %w", err)
	}

	return nil
}

func createOwnersFile(ctx context.Context, r Repository, branchName string) error {
	// This is a special GH log format: https://docs.github.com/en/actions/writing-workflows/choosing-what-your-workflow-does/workflow-commands-for-github-actions#example-grouping-log-lines
	log.Printf("::group::ownersfilegen %s %s\n", r.RepositoryDirectory(), branchName)
	log.Printf("branchName: %s\n", branchName)

	if err := GitMirror(ctx, r); err != nil {
		return err
	}
	if err := GitCheckout(ctx, r, branchName); err != nil {
		return err
	}

	pushBranch := fmt.Sprintf("%s%s", ownersfilegen.SyncBranchPrefix, branchName)

	if err := ownersfilegen.WriteOwnersFile(r.RepositoryDirectory(), r.Owners.Reviewers, r.Owners.Approvers); err != nil {
		return fmt.Errorf("[%s][%s] failed to write OWNERS file: %w", r.RepositoryDirectory(), branchName, err)
	}

	commitMsg := fmt.Sprintf("[%s] Update OWNERS file", branchName)
	if err := PushBranch(ctx, r, nil, pushBranch, commitMsg); err != nil {
		return err
	}

	// This is a special GH log format: https://docs.github.com/en/actions/writing-workflows/choosing-what-your-workflow-does/workflow-commands-for-github-actions#example-grouping-log-lines
	log.Printf("::endgroup::\n\n")

	return nil
}
