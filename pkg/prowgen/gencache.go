package prowgen

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/openshift-knative/hack/pkg/dependabotgen"
	"github.com/openshift-knative/hack/pkg/ownersfilegen"
)

// Generation cache: prowgen's output for a repository is a deterministic
// function of (a) the hack-side generation inputs (configs, templates and
// generator sources), (b) the repository's git refs, and (c) the
// serverless-operator git refs (release versions feed image build args).
// When none of these inputs changed since the last run that generated a
// repository, its previously generated outputs are restored from the cache
// instead of being regenerated.
//
// Cached outputs per repository:
//   - the local sync branches (konflux-*, owners-*, sync-dependabot-*) as
//     git format-patch bundles, replayed with git am
//   - the repository's generated files in the openshift/release tree:
//     ci-operator/config/<org>/<repo>/** and
//     core-services/prow/02_config/<org>/<repo>/**
//
// Not cached (always regenerated):
//   - the openshift/release `make ci-operator-config jobs prow-config` run
//   - the hack repository's own konflux branch
//
// The cache fails safe: any input change is a cache miss and triggers full
// regeneration. Refs are read from the local clones (what generation actually
// consumes), so runs against stale clones record stale keys and simply miss on
// the next fresh run. Delete the cache directory to force full regeneration.

const (
	// DefaultGenCacheDir is the default generation cache directory, relative
	// to the working directory prowgen runs from (the hack repository root).
	DefaultGenCacheDir = ".prowgen-cache"

	genCacheStateFile  = "state.json"
	genCachePatchesDir = "patches"

	// soRepoOrg/soRepoRepo identify the serverless-operator repository whose
	// refs are part of every cache key (its release versions feed build args).
	soRepoOrg  = "openshift-knative"
	soRepoRepo = "serverless-operator"
)

// genCacheHashPaths are the paths (relative to the working directory) whose
// contents are hashed into the generation hash: repository configs, the
// generator sources and everything they embed (templates live under pkg/).
var genCacheHashPaths = []string{"config", "pkg", "cmd", "go.mod", "go.sum"}

// GenCache caches prowgen generation outputs per repository. All methods are
// safe for concurrent use and nil-receiver-safe (a nil or dir-less cache is a
// disabled no-op).
type GenCache struct {
	dir     string
	genHash string

	soOnce  sync.Once
	soHeads map[string]string

	mu        sync.Mutex
	entries   map[string]*genCacheEntry    // key: RepositoryDirectory()
	decisions map[string]bool              // key: RepositoryDirectory()
	branches  map[string]map[string]string // key: repo, value: sync branch -> target branch
	restored  map[string]bool              // key: RepositoryDirectory()
	dirty     bool
}

type genCacheEntry struct {
	GenHash     string            `json:"genHash"`
	SOHeads     map[string]string `json:"soHeads"`
	RepoHeads   map[string]string `json:"repoHeads"`
	Branches    map[string]string `json:"branches,omitempty"`       // sync branch -> target branch
	OSRelFiles  map[string]string `json:"osReleaseFiles,omitempty"` // path -> content
	OSRelRecord bool              `json:"osReleaseRecorded"`
}

// NewGenCache loads (or initializes) the generation cache in dir. An empty dir
// returns a disabled cache. Extra strings are mixed into the generation hash
// (used for the input config path and generator flags, which change what is
// generated).
func NewGenCache(ctx context.Context, dir string, extra ...string) (*GenCache, error) {
	c := &GenCache{
		dir:       dir,
		entries:   map[string]*genCacheEntry{},
		decisions: map[string]bool{},
		branches:  map[string]map[string]string{},
		restored:  map[string]bool{},
	}
	if dir == "" {
		return c, nil
	}

	if data, err := os.ReadFile(filepath.Join(dir, genCacheStateFile)); err == nil {
		if err := json.Unmarshal(data, &c.entries); err != nil {
			return nil, fmt.Errorf("failed to parse generation cache state %q (delete the directory to reset): %w", dir, err)
		}
	} else if !os.IsNotExist(err) {
		return nil, err
	}

	hash, err := genCacheHash(".", extra...)
	if err != nil {
		return nil, fmt.Errorf("failed to compute generation hash: %w", err)
	}
	c.genHash = hash

	return c, nil
}

// Skip reports whether generation for r can be skipped because its inputs are
// unchanged since the last run that generated it. The decision is computed
// once per repository and memoized. A false return marks r for re-recording.
func (c *GenCache) Skip(ctx context.Context, r Repository) bool {
	if c == nil || c.dir == "" {
		return false
	}
	key := r.RepositoryDirectory()

	c.mu.Lock()
	if d, ok := c.decisions[key]; ok {
		c.mu.Unlock()
		return d
	}
	c.mu.Unlock()

	// Clone r if needed so its refs can be read; this mirrors what the
	// generators themselves do and is a no-op for existing clones.
	if err := genCacheMirror(ctx, r); err != nil {
		log.Printf("[%s] generation cache: failed to mirror repository, not skipping: %v", key, err)
		soHeads := c.loadSOHeads(ctx)
		return c.decide(key, false, nil, soHeads)
	}
	heads, err := genCacheLocalHeads(ctx, r)
	if err != nil {
		log.Printf("[%s] generation cache: failed to read refs, not skipping: %v", key, err)
		soHeads := c.loadSOHeads(ctx)
		return c.decide(key, false, nil, soHeads)
	}

	// Compute the serverless-operator heads (which may clone it) before
	// taking the cache lock: holding the lock around git operations that
	// other cache lookups wait on can deadlock.
	soHeads := c.loadSOHeads(ctx)

	c.mu.Lock()
	entry := c.entries[key]
	c.mu.Unlock()
	skip := entry != nil &&
		entry.GenHash == c.genHash &&
		genCacheHeadsEqual(entry.RepoHeads, heads) &&
		genCacheHeadsEqual(entry.SOHeads, soHeads)

	return c.decide(key, skip, heads, soHeads)
}

// decide stores the memoized decision; on a regeneration decision it replaces
// the entry with a fresh one that will be filled in by the Record* calls.
func (c *GenCache) decide(key string, skip bool, heads, soHeads map[string]string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.decisions[key] = skip
	if !skip {
		c.entries[key] = &genCacheEntry{
			GenHash:   c.genHash,
			RepoHeads: heads,
			SOHeads:   soHeads,
		}
		c.dirty = true
	}
	return skip
}

// loadSOHeads lazily reads the serverless-operator refs (cloning it if
// needed), once per cache instance.
func (c *GenCache) loadSOHeads(ctx context.Context) map[string]string {
	c.soOnce.Do(func() {
		soRepo := Repository{Org: soRepoOrg, Repo: soRepoRepo}
		if err := genCacheMirror(ctx, soRepo); err != nil {
			log.Printf("[%s] generation cache: failed to mirror serverless-operator, treating its refs as unknown: %v", soRepo.RepositoryDirectory(), err)
			return
		}
		heads, err := genCacheLocalHeads(ctx, soRepo)
		if err != nil {
			log.Printf("[%s] generation cache: failed to read serverless-operator refs, treating them as unknown: %v", soRepo.RepositoryDirectory(), err)
			return
		}
		c.soHeads = heads
	})
	return c.soHeads
}

// RecordBranch registers a sync branch (and the target branch it was created
// from) for snapshotting by SnapshotBranches.
func (c *GenCache) RecordBranch(r Repository, branch, target string) {
	if c == nil || c.dir == "" {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	key := r.RepositoryDirectory()
	if c.branches[key] == nil {
		c.branches[key] = map[string]string{}
	}
	c.branches[key][branch] = target
}

// SnapshotBranches writes format-patch bundles for all branches recorded via
// RecordBranch for r into the cache directory.
func (c *GenCache) SnapshotBranches(ctx context.Context, r Repository) error {
	if c == nil || c.dir == "" {
		return nil
	}
	c.mu.Lock()
	key := r.RepositoryDirectory()
	entry := c.entries[key]
	branches := c.branches[key]
	c.mu.Unlock()
	if entry == nil || len(branches) == 0 {
		return nil
	}

	if entry.Branches == nil {
		entry.Branches = map[string]string{}
	}
	for branch, target := range branches {
		ref, err := genCacheResolveRef(ctx, r, target)
		if err != nil {
			return fmt.Errorf("[%s] failed to resolve target branch %q for cache snapshot: %w", key, target, err)
		}
		// git runs with the repository as working directory, so the patch
		// directory must be resolved against the process working directory.
		patchDir, err := filepath.Abs(c.patchDir(key, branch))
		if err != nil {
			return err
		}
		if err := os.RemoveAll(patchDir); err != nil {
			return err
		}
		if err := os.MkdirAll(patchDir, 0755); err != nil {
			return err
		}
		if _, err := Run(ctx, r, "git", "format-patch", "--binary", "--zero-commit", "-o", patchDir, ref+".."+branch); err != nil {
			return fmt.Errorf("[%s] failed to snapshot branch %q: %w", key, branch, err)
		}
		entry.Branches[branch] = target
	}
	c.mu.Lock()
	c.dirty = true
	c.mu.Unlock()
	return nil
}

// RestoreBranches recreates r's cached sync branches in the repository
// workspace by replaying their cached patches. Runs once per repository.
func (c *GenCache) RestoreBranches(ctx context.Context, r Repository) error {
	if c == nil || c.dir == "" {
		return nil
	}
	c.mu.Lock()
	key := r.RepositoryDirectory()
	if c.restored[key] {
		c.mu.Unlock()
		return nil
	}
	c.restored[key] = true
	entry := c.entries[key]
	c.mu.Unlock()
	if entry == nil {
		return nil
	}

	for branch, target := range entry.Branches {
		ref, err := genCacheResolveRef(ctx, r, target)
		if err != nil {
			return fmt.Errorf("[%s] failed to resolve target branch %q for cache restore: %w", key, target, err)
		}
		if _, err := Run(ctx, r, "git", "checkout", "-B", branch, ref); err != nil {
			return fmt.Errorf("[%s] failed to restore branch %q: %w", key, branch, err)
		}
		patchDir, err := filepath.Abs(c.patchDir(key, branch))
		if err != nil {
			return err
		}
		patches, err := filepath.Glob(filepath.Join(patchDir, "*.patch"))
		if err != nil {
			return err
		}
		if len(patches) == 0 {
			// Branch had no commits relative to its target; creating it at
			// the target tip is the equivalent state.
			continue
		}
		sort.Strings(patches)
		args := append([]string{"-c", "user.name=prowgen", "-c", "user.email=serverless-support@redhat.com", "am"}, patches...)
		if _, err := Run(ctx, r, "git", args...); err != nil {
			return fmt.Errorf("[%s] failed to replay cached patches for branch %q: %w", key, branch, err)
		}
		log.Printf("[%s] restored cached branch %q (%d patches)", key, branch, len(patches))
	}
	return nil
}

// RecordOSReleaseFiles snapshots the files generated for r in the
// openshift/release tree.
func (c *GenCache) RecordOSReleaseFiles(r Repository) error {
	if c == nil || c.dir == "" {
		return nil
	}
	c.mu.Lock()
	key := r.RepositoryDirectory()
	entry := c.entries[key]
	c.mu.Unlock()
	if entry == nil {
		return nil
	}

	files := map[string]string{}
	for _, dir := range genCacheOSReleaseDirs(r) {
		if err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return err
			}
			content, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			files[path] = string(content)
			return nil
		}); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	entry.OSRelFiles = files
	entry.OSRelRecord = true
	c.mu.Lock()
	c.dirty = true
	c.mu.Unlock()
	return nil
}

// RestoreOSReleaseFiles writes r's cached openshift/release files back into
// the tree (InitializeOpenShiftReleaseRepository deletes them on every run).
func (c *GenCache) RestoreOSReleaseFiles(r Repository) error {
	if c == nil || c.dir == "" {
		return nil
	}
	c.mu.Lock()
	entry := c.entries[r.RepositoryDirectory()]
	c.mu.Unlock()
	if entry == nil {
		return nil
	}
	for path, content := range entry.OSRelFiles {
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			return err
		}
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			return err
		}
	}
	log.Printf("[%s] restored %d openshift/release files from cache", r.RepositoryDirectory(), len(entry.OSRelFiles))
	return nil
}

// Save persists the cache state, if anything was regenerated.
func (c *GenCache) Save() error {
	if c == nil || c.dir == "" || !c.dirty {
		return nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()

	data, err := json.MarshalIndent(c.entries, "", "  ")
	if err != nil {
		return err
	}
	state := filepath.Join(c.dir, genCacheStateFile)
	tmp := state + ".tmp"
	if err := os.MkdirAll(c.dir, 0755); err != nil {
		return err
	}
	if err := os.WriteFile(tmp, data, 0600); err != nil {
		return err
	}
	return os.Rename(tmp, state)
}

func (c *GenCache) patchDir(repoKey, branch string) string {
	sanitize := func(s string) string {
		return strings.ReplaceAll(filepath.ToSlash(s), "/", "_")
	}
	return filepath.Join(c.dir, genCachePatchesDir, sanitize(repoKey), sanitize(branch))
}

// genCacheOSReleaseDirs returns the openshift/release directories holding the
// files generated for r.
func genCacheOSReleaseDirs(r Repository) []string {
	return []string{
		filepath.Join("openshift", "release", "ci-operator", "config", r.RepositoryDirectory()),
		filepath.Join("openshift", "release", "core-services", "prow", "02_config", r.Org, r.Repo),
	}
}

// genCacheMirror clones r if needed. serverless-operator mirrors are
// serialized because several repositories' cache lookups read its refs
// concurrently with the serverless-operator generator goroutine.
var soRepoMirrorMu sync.Mutex

func genCacheMirror(ctx context.Context, r Repository) error {
	if r.Org == soRepoOrg && r.Repo == soRepoRepo {
		soRepoMirrorMu.Lock()
		defer soRepoMirrorMu.Unlock()
	}
	return GitMirror(ctx, r)
}

// genCacheResolveRef resolves a branch name to a concrete ref, preferring
// local branches over remote-tracking ones.
func genCacheResolveRef(ctx context.Context, r Repository, branch string) (string, error) {
	for _, ref := range []string{"refs/heads/" + branch, "refs/remotes/origin/" + branch} {
		if _, err := Run(ctx, r, "git", "show-ref", "--verify", "--quiet", ref); err == nil {
			return ref, nil
		}
	}
	return "", fmt.Errorf("ref for branch %q not found", branch)
}

// genCacheLocalHeads returns the branch -> sha map of a repository workspace,
// combining local and remote-tracking refs and excluding prowgen's own sync
// branches (which would make the key self-referential across runs).
func genCacheLocalHeads(ctx context.Context, r Repository) (map[string]string, error) {
	out, err := Run(ctx, r, "git", "for-each-ref", "--format=%(refname) %(objectname)", "refs/heads", "refs/remotes/origin")
	if err != nil {
		return nil, err
	}
	heads := map[string]string{}
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		fields := strings.Fields(line)
		if len(fields) != 2 {
			continue
		}
		ref, sha := fields[0], fields[1]
		branch := strings.TrimPrefix(strings.TrimPrefix(ref, "refs/remotes/origin/"), "refs/heads/")
		if branch == "HEAD" || branch == ref {
			continue
		}
		if strings.HasPrefix(branch, KonfluxBranchPrefix) ||
			strings.HasPrefix(branch, ownersfilegen.SyncBranchPrefix) ||
			strings.HasPrefix(branch, dependabotgen.SyncBranchPrefix) {
			continue
		}
		heads[branch] = sha
	}
	return heads, nil
}

func genCacheHeadsEqual(a, b map[string]string) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if b[k] != v {
			return false
		}
	}
	return true
}

// genCacheHash hashes the contents of genCacheHashPaths under root, mixed
// with any extra strings.
func genCacheHash(root string, extra ...string) (string, error) {
	hasher := sha256.New()
	var paths []string
	for _, p := range genCacheHashPaths {
		full := filepath.Join(root, p)
		info, err := os.Lstat(full)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return "", err
		}
		if !info.IsDir() {
			paths = append(paths, p)
			continue
		}
		err = filepath.WalkDir(full, func(path string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return err
			}
			rel, err := filepath.Rel(root, path)
			if err != nil {
				return err
			}
			paths = append(paths, rel)
			return nil
		})
		if err != nil {
			return "", err
		}
	}
	sort.Strings(paths)

	for _, p := range paths {
		content, err := os.ReadFile(filepath.Join(root, p))
		if err != nil {
			return "", err
		}
		hasher.Write([]byte(p))
		hasher.Write([]byte{0})
		hasher.Write(content)
		hasher.Write([]byte{0})
	}
	for _, e := range extra {
		hasher.Write([]byte(e))
		hasher.Write([]byte{0})
	}
	return hex.EncodeToString(hasher.Sum(nil)), nil
}
