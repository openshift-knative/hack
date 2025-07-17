package prowgen

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/openshift-knative/hack/pkg/soversion"

	"github.com/coreos/go-semver/semver"
)

func GitCheckout(ctx context.Context, r Repository, branch string) error {
	_, err := Run(ctx, r, "git", "checkout", branch)
	return err
}

func GitMirror(ctx context.Context, r Repository) error {
	return gitClone(ctx, r, true)
}

func GitClone(ctx context.Context, r Repository) error {
	return gitClone(ctx, r, false)
}

var (

	/* Example outputs:
	$ git --no-pager branch --list "release-*"
	  release-1.33
	  release-1.34
	* release-1.35

	$ git --no-pager branch --list "release-1.33*"
	  release-1.33
	*/
	branchParsingRegexes = []*regexp.Regexp{
		regexp.MustCompile("([ \t]+|^)(release-[0-9]+.[0-9]+)"),
		regexp.MustCompile("([ \t]+|^)(release-v[0-9]+.[0-9]+)"),
		regexp.MustCompile("([ \t]+|^)(main)"),
	}
)

func Branches(ctx context.Context, r Repository, pattern string) ([]string, error) {
	if err := GitMirror(ctx, r); err != nil {
		return nil, err
	}

	// git --no-pager branch --list "release-v*"
	branchesBytes, err := Run(ctx, r, "git", "--no-pager", "branch", "--list", pattern)
	if err != nil {
		return nil, err
	}

	branchesList := string(branchesBytes)

	var sortedBranches []string
	for _, branch := range strings.Split(branchesList, "\n") {
		for _, regex := range branchParsingRegexes {
			match := regex.FindStringSubmatch(branch)
			if len(match) == 3 {
				sortedBranches = append(sortedBranches, match[2])
				break
			}
		}
	}
	slices.SortFunc(sortedBranches, CmpBranches)

	log.Println("Branches for", r.RepositoryDirectory(), sortedBranches)

	return sortedBranches, nil
}

func ReleaseBranches(ctx context.Context, r Repository) ([]string, error) {
	return Branches(ctx, r, "release-*")
}

var (
	ignoreBranches = []string{"1.13"}
)

func CmpBranches(a string, b string) int {
	for _, branch := range ignoreBranches {
		if strings.Contains(a, branch) {
			return -1 // this is equivalent to ignoring the branch
		}
		if strings.Contains(b, branch) {
			return 1 // this is equivalent to ignoring the branch
		}
	}

	var av, bv *semver.Version
	var err error

	if strings.HasPrefix(a, "serverless-") {
		av = soversion.ToUpstreamVersion(a)
	} else {
		av, err = SemverFromReleaseBranch(a)
		if err != nil {
			return -1 // this is equivalent to ignoring branches that aren't parseable
		}
	}

	if strings.HasPrefix(b, "serverless-") {
		bv = soversion.ToUpstreamVersion(b)
	} else {
		bv, err = SemverFromReleaseBranch(b)
		if err != nil {
			return 1 // this is equivalent to ignoring branches that aren't parseable
		}
	}

	return av.Compare(*bv)
}

func SemverFromReleaseBranch(b string) (*semver.Version, error) {
	b = strings.ReplaceAll(b, "release-v", "")
	b = strings.ReplaceAll(b, "release-", "")
	if strings.Count(b, ".") == 1 {
		b += ".0"
	}

	return semver.NewVersion(b)
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
		if _, err := Run(ctx, r, "git", "config", "--bool", "core.bare", "false"); err != nil {
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
	_, err := Run(ctx, r, "git", "merge", sha, "--no-ff", "-m", "Merge "+sha)
	return err
}

func GitFetch(ctx context.Context, r Repository, sha string) error {
	remoteRepo := fmt.Sprintf("https://github.com/%s/%s.git", r.Org, r.Repo)
	_, err := Run(ctx, r, "git", "fetch", remoteRepo, sha)
	return err
}

func GitDiffNameOnly(ctx context.Context, r Repository, sha string) ([]string, error) {
	out, err := Run(ctx, r, "git", "diff", "--name-only", sha)
	if err != nil {
		return nil, err
	}
	return strings.Split(strings.TrimSpace(string(out)), "\n"), nil
}
