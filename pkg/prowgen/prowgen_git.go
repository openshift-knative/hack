package prowgen

import (
	"cmp"
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

// ErrBranchNotFound is returned by GitCheckout when the requested branch does
// not exist in the repository. Callers must use errors.Is to detect it rather
// than matching on error text, which cannot distinguish a missing branch from a
// transient failure such as a network blip, expired auth or index.lock contention.
var ErrBranchNotFound = errors.New("branch not found")

// gitRefExists reports whether ref resolves to a commit in r. It is a local,
// read-only lookup against the object database and does not touch the working
// tree, so it is safe to call concurrently.
func gitRefExists(ctx context.Context, r Repository, ref string) bool {
	_, err := runQuiet(ctx, r, "git", "rev-parse", "--verify", "--quiet", ref+"^{commit}")
	return err == nil
}

func GitCheckout(ctx context.Context, r Repository, branch string) error {
	// Resolve the ref first: a failure here means the branch genuinely does not
	// exist, whereas a bare `git checkout` failure would conflate that with
	// transient errors.
	if !gitRefExists(ctx, r, branch) {
		return fmt.Errorf("%w: %q in %s", ErrBranchNotFound, branch, r.RepositoryDirectory())
	}
	_, err := Run(ctx, r, "git", "checkout", branch)
	return err
}

// GitShow returns the contents of path at ref without modifying the working
// tree. Prefer it over GitCheckout when only a handful of files are needed: a
// checkout rewrites the entire worktree (~1.2s on the larger repositories) and
// forces every caller sharing a clone to serialize. Reads go to the object
// database, so GitShow is safe to call concurrently, including alongside a
// checkout of the same repository.
//
// Returns ErrBranchNotFound if ref does not exist.
func GitShow(ctx context.Context, r Repository, ref string, path string) ([]byte, error) {
	if !gitRefExists(ctx, r, ref) {
		return nil, fmt.Errorf("%w: %q in %s", ErrBranchNotFound, ref, r.RepositoryDirectory())
	}
	// git object paths are always slash-separated, independent of the host OS.
	out, err := runQuiet(ctx, r, "git", "show", ref+":"+path)
	if err != nil {
		return nil, fmt.Errorf("failed to read %q at %q: %w", path, ref, err)
	}
	return out, nil
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

func isIgnoredBranch(branch string) bool {
	for _, ignored := range ignoreBranches {
		if strings.Contains(branch, ignored) {
			return true
		}
	}
	return false
}

// branchVersion resolves a branch name to its upstream semver, or returns an
// error when the name is not parseable (for example "main").
func branchVersion(branch string) (*semver.Version, error) {
	if strings.HasPrefix(branch, "serverless-") {
		return soversion.ToUpstreamVersion(branch), nil
	}
	return SemverFromReleaseBranch(branch)
}

// CmpBranches orders branches by their upstream version, sorting ignored and
// unparseable names to the front.
//
// It must be a valid comparator: slices.SortFunc uses pdqsort, whose output is
// unspecified if cmp(a,b) and cmp(b,a) ever agree in sign. The previous
// implementation returned -1 for both when a and b were both ignored or both
// unparseable, which made the sort order depend on input order and, via
// discover.go taking the last element, could silently disable branch discovery.
// Every branch below therefore falls through to a lexical tiebreak so that
// equal-ranked names still produce a stable total order.
func CmpBranches(a string, b string) int {
	if a == b {
		return 0
	}

	aIgnored, bIgnored := isIgnoredBranch(a), isIgnoredBranch(b)
	switch {
	case aIgnored && bIgnored:
		return cmp.Compare(a, b)
	case aIgnored:
		return -1
	case bIgnored:
		return 1
	}

	av, aErr := branchVersion(a)
	bv, bErr := branchVersion(b)
	switch {
	case aErr != nil && bErr != nil:
		return cmp.Compare(a, b)
	case aErr != nil:
		return -1
	case bErr != nil:
		return 1
	}

	if c := av.Compare(*bv); c != 0 {
		return c
	}
	return cmp.Compare(a, b)
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
