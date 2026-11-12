package prowgen

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// gencacheTestEnv chdirs into a fresh temp directory (so that relative
// RepositoryDirectory() paths and the generation hash root point at test
// fixtures) and seeds the minimal files the generation hash covers.
func gencacheTestEnv(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chdir(oldWD) })

	for dir, content := range map[string]string{
		"config/repositories.yaml": "repositories: []\n",
		"go.mod":                   "module github.com/openshift-knative/hack\n",
	} {
		if err := os.MkdirAll(filepath.Dir(filepath.Join(root, dir)), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, dir), []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func gitTestCmd(t *testing.T, r Repository, args ...string) {
	t.Helper()
	if _, err := Run(context.Background(), r, args[0], args[1:]...); err != nil {
		t.Fatalf("git %v: %v", args, err)
	}
}

func initTestRepo(t *testing.T, r Repository) {
	t.Helper()
	dir := r.RepositoryDirectory()
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	gitTestCmd(t, r, "git", "init", "-b", "main")
	commitTestFile(t, r, "README.md", "readme\n", "base")
}

func commitTestFile(t *testing.T, r Repository, path, content, msg string) {
	t.Helper()
	full := filepath.Join(r.RepositoryDirectory(), path)
	if err := os.MkdirAll(filepath.Dir(full), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	gitTestCmd(t, r, "git", "add", ".")
	gitTestCmd(t, r, "git", "-c", "user.name=test", "-c", "user.email=test@test", "commit", "-m", msg)
}

func checkoutTestBranch(t *testing.T, r Repository, branch, startPoint string) {
	t.Helper()
	args := []string{"git", "checkout"}
	if startPoint != "" {
		args = append(args, "-B", branch, startPoint)
	} else {
		args = append(args, branch)
	}
	gitTestCmd(t, r, args...)
}

func readRepoFile(t *testing.T, r Repository, path string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(r.RepositoryDirectory(), path))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestGenCacheHashStableAndSensitive(t *testing.T) {
	root := gencacheTestEnv(t)

	h1, err := genCacheHash(".")
	if err != nil {
		t.Fatal(err)
	}
	h2, err := genCacheHash(".")
	if err != nil {
		t.Fatal(err)
	}
	if h1 != h2 {
		t.Fatal("generation hash must be deterministic")
	}

	// Changes in hashed paths must change the hash.
	if err := os.WriteFile(filepath.Join(root, "config", "repositories.yaml"), []byte("repositories: []\n# changed\n"), 0644); err != nil {
		t.Fatal(err)
	}
	h3, err := genCacheHash(".")
	if err != nil {
		t.Fatal(err)
	}
	if h3 == h1 {
		t.Fatal("hash must change when a hashed input changes")
	}

	// Changes outside hashed paths must not change the hash.
	if err := os.WriteFile(filepath.Join(root, "README.md"), []byte("docs\n"), 0644); err != nil {
		t.Fatal(err)
	}
	h4, err := genCacheHash(".")
	if err != nil {
		t.Fatal(err)
	}
	if h4 != h3 {
		t.Fatal("hash must ignore files outside the hashed paths")
	}
}

func TestGenCacheDisabledIsNoOp(t *testing.T) {
	gencacheTestEnv(t)
	r := Repository{Org: "openshift-knative", Repo: "testrepo"}
	initTestRepo(t, r)

	c, err := NewGenCache(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	if c.Skip(context.Background(), r) {
		t.Fatal("disabled cache must never skip")
	}
	if err := c.Save(); err != nil {
		t.Fatal(err)
	}
}

func TestGenCacheSkipAndRestoreLifecycle(t *testing.T) {
	root := gencacheTestEnv(t)
	ctx := context.Background()
	cacheDir := ".prowgen-cache"

	r := Repository{Org: "openshift-knative", Repo: "testrepo"}
	initTestRepo(t, r)
	// A stub serverless-operator repo so that soHeads resolution stays offline.
	soRepo := Repository{Org: "openshift-knative", Repo: "serverless-operator"}
	initTestRepo(t, soRepo)

	// A target branch with content.
	checkoutTestBranch(t, r, "release-v1.17", "main")
	commitTestFile(t, r, "code.txt", "target branch content\n", "target")
	checkoutTestBranch(t, r, "main", "")

	// First run: no entry, must not skip.
	c, err := NewGenCache(ctx, cacheDir)
	if err != nil {
		t.Fatal(err)
	}
	if c.Skip(ctx, r) {
		t.Fatal("first run must not skip")
	}

	// Simulate generation: sync branch with commits + o/release files.
	checkoutTestBranch(t, r, "sync-konflux-release-v1.17", "release-v1.17")
	commitTestFile(t, r, ".konflux/pipeline.yaml", "pipeline: generated\n", "konflux")
	c.RecordBranch(r, "sync-konflux-release-v1.17", "release-v1.17")
	if err := c.SnapshotBranches(ctx, r); err != nil {
		t.Fatal(err)
	}

	soRelPath := filepath.Join("openshift", "release", "ci-operator", "config", r.RepositoryDirectory(), "testrepo-main.yaml")
	if err := os.MkdirAll(filepath.Dir(soRelPath), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(soRelPath, []byte("ci-operator config\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := c.RecordOSReleaseFiles(r); err != nil {
		t.Fatal(err)
	}
	if err := c.Save(); err != nil {
		t.Fatal(err)
	}

	// Second run with unchanged inputs: must skip and restore.
	c2, err := NewGenCache(ctx, cacheDir)
	if err != nil {
		t.Fatal(err)
	}
	if !c2.Skip(ctx, r) {
		t.Fatal("unchanged inputs must skip")
	}

	// Simulate a fresh workspace: sync branch gone, o/release files deleted.
	gitTestCmd(t, r, "git", "checkout", "main")
	gitTestCmd(t, r, "git", "branch", "-D", "sync-konflux-release-v1.17")
	if err := os.RemoveAll(filepath.Dir(soRelPath)); err != nil {
		t.Fatal(err)
	}

	if err := c2.RestoreBranches(ctx, r); err != nil {
		t.Fatal(err)
	}
	if err := c2.RestoreOSReleaseFiles(r); err != nil {
		t.Fatal(err)
	}

	got, err := os.ReadFile(soRelPath)
	if err != nil {
		t.Fatalf("o/release file not restored: %v", err)
	}
	if string(got) != "ci-operator config\n" {
		t.Fatalf("unexpected o/release content: %q", got)
	}
	checkoutTestBranch(t, r, "sync-konflux-release-v1.17", "")
	if readRepoFile(t, r, ".konflux/pipeline.yaml") != "pipeline: generated\n" {
		t.Fatal("restored branch is missing generated content")
	}

	// Changed repository heads must invalidate the skip.
	checkoutTestBranch(t, r, "release-v1.17", "")
	commitTestFile(t, r, "code.txt", "changed\n", "invalidate heads")
	checkoutTestBranch(t, r, "main", "")
	c3, err := NewGenCache(ctx, cacheDir)
	if err != nil {
		t.Fatal(err)
	}
	if c3.Skip(ctx, r) {
		t.Fatal("changed repository heads must not skip")
	}

	// Changed hack-side inputs must invalidate the skip.
	checkoutTestBranch(t, r, "release-v1.17", "")
	gitTestCmd(t, r, "git", "reset", "--hard", "HEAD~1")
	checkoutTestBranch(t, r, "main", "")
	if err := os.WriteFile(filepath.Join(root, "config", "repositories.yaml"), []byte("repositories: []\n# changed\n"), 0644); err != nil {
		t.Fatal(err)
	}
	c4, err := NewGenCache(ctx, cacheDir)
	if err != nil {
		t.Fatal(err)
	}
	if c4.Skip(ctx, r) {
		t.Fatal("changed generation hash must not skip")
	}
}

func TestGenCacheRestoreEmptyBranch(t *testing.T) {
	gencacheTestEnv(t)
	ctx := context.Background()
	r := Repository{Org: "openshift-knative", Repo: "testrepo"}
	initTestRepo(t, r)
	soRepo := Repository{Org: "openshift-knative", Repo: "serverless-operator"}
	initTestRepo(t, soRepo)

	c, err := NewGenCache(ctx, ".prowgen-cache")
	if err != nil {
		t.Fatal(err)
	}
	if c.Skip(ctx, r) {
		t.Fatal("first run must not skip")
	}

	// A sync branch with no commits (nothing to commit case).
	checkoutTestBranch(t, r, "sync-owners-main", "main")
	c.RecordBranch(r, "sync-owners-main", "main")
	if err := c.SnapshotBranches(ctx, r); err != nil {
		t.Fatal(err)
	}
	if err := c.Save(); err != nil {
		t.Fatal(err)
	}

	c2, err := NewGenCache(ctx, ".prowgen-cache")
	if err != nil {
		t.Fatal(err)
	}
	if !c2.Skip(ctx, r) {
		t.Fatal("unchanged inputs must skip")
	}
	gitTestCmd(t, r, "git", "checkout", "main")
	gitTestCmd(t, r, "git", "branch", "-D", "sync-owners-main")
	if err := c2.RestoreBranches(ctx, r); err != nil {
		t.Fatal(err)
	}
	// Branch must exist again and point at the same tree as the target.
	out, err := Run(ctx, r, "git", "rev-parse", "sync-owners-main", "main")
	if err != nil {
		t.Fatal(err)
	}
	refs := strings.Fields(string(out))
	if len(refs) != 2 || refs[0] != refs[1] {
		t.Fatalf("empty sync branch must be restored at the target tip, got %v", refs)
	}
}

func TestGenCacheLocalHeadsFiltersIrrelevantBranches(t *testing.T) {
	gencacheTestEnv(t)
	ctx := context.Background()
	r := Repository{Org: "openshift-knative", Repo: "testrepo"}
	initTestRepo(t, r)

	// Branches generation actually reads.
	gitTestCmd(t, r, "git", "branch", "release-v1.17")
	gitTestCmd(t, r, "git", "branch", "release-1.36")
	gitTestCmd(t, r, "git", "branch", "release-next")
	// Branches generation never reads: automation, konflux and personal
	// branches that move constantly and must not invalidate the cache key.
	gitTestCmd(t, r, "git", "branch", "auto/update-generated-files-release-1.37")
	gitTestCmd(t, r, "git", "branch", "konflux-purge-kn-test-116")
	gitTestCmd(t, r, "git", "branch", "konflux/references/release-v1.17")
	gitTestCmd(t, r, "git", "branch", "dependabot/go_modules/main-x")
	gitTestCmd(t, r, "git", "branch", "somebody-patch-1")
	gitTestCmd(t, r, "git", "branch", "appstudio-kn-test-115")

	heads, err := genCacheLocalHeads(ctx, r)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]bool{"main": true, "release-v1.17": true, "release-1.36": true, "release-next": true}
	for branch := range heads {
		if !want[branch] {
			t.Errorf("irrelevant branch %q leaked into cache heads", branch)
		}
	}
	for branch := range want {
		if _, ok := heads[branch]; !ok {
			t.Errorf("relevant branch %q missing from cache heads", branch)
		}
	}
}
