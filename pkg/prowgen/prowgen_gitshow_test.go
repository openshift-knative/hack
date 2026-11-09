package prowgen

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// newTestRepo builds a throwaway git repository at <org>/<repo> relative to a
// temp working directory, so Repository.RepositoryDirectory resolves to it.
func newTestRepo(t *testing.T) Repository {
	t.Helper()

	t.Chdir(t.TempDir())

	r := Repository{Org: "testorg", Repo: "testrepo"}
	dir := r.RepositoryDirectory()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}

	git := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v: %s", args, err, out)
		}
	}

	git("init", "-q", "-b", "main")
	git("config", "user.email", "test@example.com")
	git("config", "user.name", "test")

	write := func(content string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, "project.yaml"), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	write("version: main\n")
	git("add", "project.yaml")
	git("commit", "-q", "-m", "main")

	git("checkout", "-q", "-b", "release-1.2")
	write("version: release\n")
	git("commit", "-q", "-am", "release")

	git("checkout", "-q", "main")

	return r
}

func TestGitShowReadsRefWithoutTouchingWorktree(t *testing.T) {
	ctx := context.Background()
	r := newTestRepo(t)
	worktreeFile := filepath.Join(r.RepositoryDirectory(), "project.yaml")

	before, err := os.ReadFile(worktreeFile)
	if err != nil {
		t.Fatal(err)
	}

	for _, tc := range []struct{ ref, want string }{
		{"main", "version: main\n"},
		{"release-1.2", "version: release\n"},
	} {
		got, err := GitShow(ctx, r, tc.ref, "project.yaml")
		if err != nil {
			t.Fatalf("GitShow(%q): %v", tc.ref, err)
		}
		if string(got) != tc.want {
			t.Errorf("GitShow(%q) = %q, want %q", tc.ref, got, tc.want)
		}
	}

	// The point of GitShow over GitCheckout: reading another ref must not
	// mutate the shared working tree, which is what allows callers to drop
	// the mutex and run concurrently.
	after, err := os.ReadFile(worktreeFile)
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Errorf("GitShow modified the working tree: %q -> %q", before, after)
	}
}

func TestGitShowMissingRefIsErrBranchNotFound(t *testing.T) {
	ctx := context.Background()
	r := newTestRepo(t)

	_, err := GitShow(ctx, r, "release-9.9", "project.yaml")
	if !errors.Is(err, ErrBranchNotFound) {
		t.Fatalf("GitShow(missing ref) error = %v, want ErrBranchNotFound", err)
	}
}

// A missing file on an existing ref must NOT be reported as a missing branch:
// callers fall back to a default version for a missing branch, but a missing
// file is a real error that should surface.
func TestGitShowMissingPathIsNotErrBranchNotFound(t *testing.T) {
	ctx := context.Background()
	r := newTestRepo(t)

	_, err := GitShow(ctx, r, "main", "does/not/exist.yaml")
	if err == nil {
		t.Fatal("GitShow(missing path) succeeded, want error")
	}
	if errors.Is(err, ErrBranchNotFound) {
		t.Fatalf("GitShow(missing path) returned ErrBranchNotFound, want a distinct error: %v", err)
	}
}

func TestGitCheckoutMissingBranchIsErrBranchNotFound(t *testing.T) {
	ctx := context.Background()
	r := newTestRepo(t)

	err := GitCheckout(ctx, r, "release-9.9")
	if !errors.Is(err, ErrBranchNotFound) {
		t.Fatalf("GitCheckout(missing branch) error = %v, want ErrBranchNotFound", err)
	}
}
