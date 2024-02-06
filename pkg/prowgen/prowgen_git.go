package prowgen

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/coreos/go-semver/semver"
)

func GitCheckout(ctx context.Context, r Repository, branch string) error {
	_, err := run(ctx, r, "git", "checkout", branch)
	return err
}

func GitMirror(ctx context.Context, r Repository) error {
	return gitClone(ctx, r, true)
}

func GitClone(ctx context.Context, r Repository) error {
	return gitClone(ctx, r, false)
}

func Branches(ctx context.Context, r Repository) ([]string, error) {
	if err := GitMirror(ctx, r); err != nil {
		return nil, err
	}

	// git --no-pager branch --list "release-v*"
	branchesBytes, err := run(ctx, r, "git", "--no-pager", "branch", "--list", "release-v*")
	if err != nil {
		return nil, err
	}

	branchesList := string(branchesBytes)

	sortedBranches := strings.Split(branchesList, "\n")
	for i, b := range sortedBranches {
		b = strings.TrimSpace(b)
		sortedBranches[i] = b
	}
	slices.SortFunc(sortedBranches, CmpBranches)

	log.Println("Branches for", r.RepositoryDirectory(), sortedBranches)

	return sortedBranches, nil
}

func CmpBranches(a string, b string) int {
	a = strings.ReplaceAll(a, "release-v", "")
	b = strings.ReplaceAll(b, "release-v", "")
	if strings.Count(a, ".") == 1 {
		a += ".0"
	}
	if strings.Count(b, ".") == 1 {
		b += ".0"
	}
	log.Printf("%q %q\n", a, b)
	av, err := semver.NewVersion(a)
	if err != nil {
		return -1 // this is equivalent to ignoring branches that aren't parseable
	}
	bv, err := semver.NewVersion(b)
	if err != nil {
		return 1 // this is equivalent to ignoring branches that aren't parseable
	}

	return av.Compare(*bv)
}

func gitClone(ctx context.Context, r Repository, mirror bool) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	if _, err := os.Stat(r.RepositoryDirectory()); !errors.Is(err, os.ErrNotExist) {
		log.Println("Repository", r.RepositoryDirectory(), "already cloned")
		return nil
	}

	if err := os.RemoveAll(r.RepositoryDirectory()); err != nil {
		return fmt.Errorf("[%s] failed to delete directory: %w", r.RepositoryDirectory(), err)
	}

	if err := os.MkdirAll(filepath.Dir(r.RepositoryDirectory()), os.ModePerm); err != nil {
		return fmt.Errorf("[%s] failed to create directory: %w", r.RepositoryDirectory(), err)
	}

	remoteRepo := fmt.Sprintf("https://github.com/%s/%s.git", r.Org, r.Repo)
	if mirror {
		log.Println("Mirroring repository", r.RepositoryDirectory())
		if _, err := runNoRepo(ctx, "git", "clone", "--mirror", remoteRepo, filepath.Join(r.RepositoryDirectory(), ".git")); err != nil {
			return fmt.Errorf("[%s] failed to clone repository: %w", r.RepositoryDirectory(), err)
		}
		if _, err := run(ctx, r, "git", "config", "--bool", "core.bare", "false"); err != nil {
			return fmt.Errorf("[%s] failed to set config for repository: %w", r.RepositoryDirectory(), err)
		}
	} else {
		log.Println("Cloning repository", r.RepositoryDirectory())
		if _, err := runNoRepo(ctx, "git", "clone", remoteRepo, r.RepositoryDirectory()); err != nil {
			return fmt.Errorf("[%s] failed to clone repository: %w", r.RepositoryDirectory(), err)
		}
	}

	return nil
}

func GitMerge(ctx context.Context, r Repository, sha string) error {
	_, err := run(ctx, r, "git", "merge", sha, "--no-ff", "-m", "Merge "+sha)
	return err
}

func GitFetch(ctx context.Context, r Repository, sha string) error {
	remoteRepo := fmt.Sprintf("https://github.com/%s/%s.git", r.Org, r.Repo)
	_, err := run(ctx, r, "git", "fetch", remoteRepo, sha)
	return err
}

func GitDiffNameOnly(ctx context.Context, r Repository, sha string) ([]string, error) {
	out, err := run(ctx, r, "git", "diff", "--name-only", sha)
	if err != nil {
		return nil, err
	}
	return strings.Split(strings.TrimSpace(string(out)), "\n"), nil
}
