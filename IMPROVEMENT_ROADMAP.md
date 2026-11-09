# Improvement Roadmap

Evidence-based plan derived from a full audit of `pkg/`, `cmd/`, and `.github/workflows/`.
Every claim below cites `file:line` and was verified by reading or executing the code.

> **Supersedes the previous revision of this file.** The earlier version was written without
> reading the Go source and contained estimates that measurement later contradicted. See
> [Corrections](#corrections-to-the-previous-revision) at the bottom — two recommendations in
> `QUICK_START_IMPROVEMENTS.md` are actively harmful and should not be implemented.

---

## Measured baseline

From CI run `40897223955` (2026-10-29) and static analysis of the generated workflow:

| Measurement | Value |
|---|---|
| `release-generate-ci` total job time | ~35 min |
| └─ `Generate CI` step alone | **23m35s (67% of job)** |
| └─ `Freeup disk space` step | 88s |
| `go: downloading` lines in one run | **2,668** |
| Repos cloned by prowgen itself | 14 (~106s) |
| `gh pr create` calls, sequential, one job | **114** |
| `git rebase` calls, unguarded | **112** |
| `continue-on-error` occurrences | **0** |
| Generated workflow size | 2,679 lines / 138 steps |
| └─ mechanical repetition | **127 of 140 steps** |
| `go vet` / linter in CI | **none** |

**The generated workflow is a build artifact.** `.github/workflows/release-generate-ci.yaml` is
produced by `pkg/action/` (282 lines) from a 126-line template, and `make unit-tests` diffs the two
to enforce they match. Hand-editing the YAML will fail CI. All workflow-shape changes belong in
`pkg/action/update_action.go`.

---

## 1. Fix two silent-failure bugs that make the system quietly stop working

**Impact: critical · Effort: low (~1 day) · Risk: low**

Both fail silently and both auto-commit their wrong output into PRs that then get merged.

### 1a. Transient git error silently drops a branch from management

`pkg/action/update_action.go:174`

```go
if err := prowgen.GitCheckout(ctx, r, targetBranch); err != nil {
    if !strings.Contains(err.Error(), "failed to run git [checkout") {
        return nil, nil, err
    }
    // Skip non-existing branches
    log.Println(r.RepositoryDirectory(), "Skipping non existing branch", branchName)
    continue
}
```

Branch existence is inferred by substring-matching an error message, but *every* checkout failure
produces that same string — network blip, expired auth, disk full, `index.lock` contention. Any of
them is indistinguishable from "branch doesn't exist", so the code `continue`s and **omits that
branch's Konflux PR step from the regenerated workflow**. The regenerated file is auto-committed and
merged; that branch then stops receiving Konflux config updates indefinitely. The only trace is a
`Skipping non existing branch` log line.

Same pattern: `pkg/prowgen/prowgen_konflux.go:125`.

**It is worse than it looks.** `prowgen_config.go:194` rewraps the same `GitCheckout` failure
**without `%w`**, discarding the cause. So one site matches on an error string while another
destroys it — the two defects are the same root cause from opposite ends, and fixing either alone
leaves the behaviour unpredictable.

**Fix:** return a typed sentinel error (e.g. `ErrBranchNotFound`) from the git layer, wrap with
`%w` everywhere, and test with `errors.Is`. Never branch on error text. This belongs in the
`pkg/git` extraction described in §7b.

### 1b. `CmpBranches` violates the comparator contract

`pkg/prowgen/prowgen_git.go:88-120`

The `ignoreBranches` loop returns `-1` when `a` matches, and both parse-failure paths also return
`-1` on `a`. So `cmp(x,y)` and `cmp(y,x)` can **both** return `-1`:

```
cmp("main","main")           = -1   (not 0)        VIOLATION
cmp("main","release-next")   = -1
cmp("release-next","main")   = -1                  VIOLATION
cmp("garbage","alsogarbage") = -1 both directions  VIOLATION
```

`slices.SortFunc` uses pdqsort and gives **unspecified order** for an inconsistent comparator.
`pkg/discover/discover.go:102` takes `configuredBranches[len-1]` as `latestConfigured`. When `main`
lands last, it never matches any `release-*` in `availableBranches`, so **branch discovery silently
finds nothing** (gated at `discover.go:121`), and `inConfig.Config.Branches[latest]` yields a
zero-value `Branch` that gets copied into new branch configs. Because it depends on map iteration
order, behaviour can differ run to run.

Found independently by two auditors and confirmed by direct source read.

**Fix:** return `0` for equal/both-ignored, make ignore handling symmetric, return a stable total
order for unparseable inputs. Add a property test asserting antisymmetry and transitivity.

**Why this pair ranks first:** everything else in this document is slow, noisy, or fragile. These
two are *wrong without telling you*.

---

## 2. Make the daily production Konflux apply safe

**Impact: critical · Effort: low–medium (~2 days) · Risk: low**

`.github/workflows/apply-konflux-manifests.yaml` runs **daily, unattended, against the production
cluster** `api.kflux-prd-rh02.0fk9.p1.openshiftapps.com`, applying whatever is on `main`.

- `pkg/konfluxapply/konflux_apply.go:99` — `kubectl apply -Rf` with **no dry-run anywhere in the
  toolchain**. No `--dry-run` flag exists in any command.
- It returns on first error, so a failure mid-walk leaves the cluster **half-updated**: earlier
  repos applied, later ones not, no rollback, no summary of what landed.
- `pkg/konfluxgen/konfluxgen.go:814` — `Sanitize` only lowercases, strips `.`, and maps space to
  `-`. Underscores and other invalid characters survive into Component names the API server rejects
  under DNS-1123, so the failure surfaces at *apply* time rather than generation time, triggering
  the partial-state problem above.
- Stripping dots also collides silently: `my.component` and `mycomponent` both become
  `mycomponent`, and since that string is the map key at `konfluxgen.go:462`, one component is
  overwritten and never generated.

**Fix:**
1. Server-side dry-run pass over every manifest before any mutation; abort the whole run if any
   fail.
2. Accumulate per-resource errors and report a summary instead of fail-fast.
3. Make `Sanitize` emit valid DNS-1123 and detect key collisions at generation time.

**Related, lower priority:** component-before-FBC release ordering is unenforced
(`cmd/konflux-release-gen/main.go:91-136`). The generated PRs do carry `--label
"do-not-merge/hold"`, which is a merge gate but not an *ordering* gate — a human can still unhold
them in the wrong order. The generator already has cluster access via `loadReleaseFromCluster` and
could refuse to emit FBC CRs until the component Release reports `Released=True`.

---

## 3. Cut CI time, in the right place

**Impact: high · Effort: medium (~1 week) · Risk: low**

Ordered by measured leverage, not by intuition.

### 3a. Attack the 23m35s `Generate CI` step — two angles, cheaper one first

The 16-minute tail is target-repo build tooling (`deepcopy-gen`, codegen) re-running per repo ×
branch, producing **2,668 `go: downloading` lines**. `actions/setup-go` caches only *hack's own*
modules; target repos build under `/tmp/knative.*/go` with a cold cache every time.

**Angle 1 — question what we run at all (dsimansk, PR #1051).** From prior investigation: the
knative generators take non-trivial time, and **knative/eventing's license generation is
significantly slower than any other repository**, for output unrelated to what hack needs. Auditing
what `make generate-release` actually needs per repository, and skipping the rest, is likely a
larger win than caching — and it removes work instead of making work faster. Do this first.

**Angle 2 — warm a shared `GOMODCACHE`** for whatever generators survive angle 1.

Note the parallelism picture is more nuanced than the old roadmap claimed: generation is **already
parallel per repository** via the `errgroup` at `prowgen.go:113`. The serialization is one level
down — `NewGenerateConfigs` (`prowgen_config.go:160`) loops branches **sequentially**, re-reading
the target `Makefile` per branch (`prowgen_tests.go:321`).

### 3b. Delete the 13 dead clone steps

`.github/workflows/release-generate-ci.yaml:24-126` clone repos into
`./src/.../hack/openshift-knative/<repo>`, then line 134 runs `actions/checkout` on the **parent**
`./src/.../hack`. That path has no `.git`, so checkout wipes the lot. The CI log confirms it
verbatim:

```
Checkout openshift-knative/hack  Deleting the contents of '.../src/github.com/openshift-knative/hack'
```

prowgen then re-clones all 14 repos itself. The steps cost ~21s and create the disk pressure that
the 88-second `Freeup disk space` step exists to relieve. Delete them (or move them after line 134).
Emitted from `pkg/action/`.

### 3c. Collapse the 114 PR steps into a matrix — and move the logic into Go

`pkg/action/update_action.go:94-111` emits 114 structurally identical
`reusableGitPushAndCreatePR` bodies varying only in `(repo, branch, target_branch, msg)` — the
generator already computes exactly that 4-tuple. **127 of 140 steps are mechanical repetition.**

**The justification is runtime, not duplication.** As dsimansk notes on PR #1051, the repetition is
generated from a template, so it is already fixable in one place and costs little maintenance. The
real cost is that all 114 `gh pr create` and 112 `git rebase` calls run in a **single job with zero
`continue-on-error`**. The `|| true` guards cover only `gh pr create`; the rebases are unguarded, so
the first rebase conflict aborts the job and every remaining repo goes unsynced with no
partial-success signal. A matrix with `fail-fast: false` buys **parallelism and failure
isolation** — that is the whole case for it.

**Design constraint (dsimansk, PR #1051): the sync logic belongs in Go, not in a bash composite
action, so it can be unit-tested.** The earlier `examples/workflow-matrix-example.yaml` sketch put
it in shell; that was the wrong shape. The matrix step should invoke a small `cmd/sync-pr` that
takes the 4-tuple as flags, with the git/PR logic covered by tests. This also dovetails with the
`pkg/git` extraction in §7.

**Caveat:** `UpdateAction` currently needs a full `GitMirror` plus per-branch `GitCheckout` of all
13 repos (`update_action.go:118,174`) purely to decide which steps to emit. That network dependency,
not the YAML shape, is the real cost — and it is the same code path as bug 1a.

**Do not cache the clones.** `pkg/prowgen/prowgen_git.go:139-142` — `gitClone` returns `nil` without
fetching if the directory already exists. It is correct only because `make generate-ci` runs `clean`
first. Caching clones would generate CI config from stale branch content.

---

## 4. Eliminate the panic class, then add a linter so it stays gone

**Impact: high · Effort: low–medium (~3 days) · Risk: low**

Crashes on malformed or missing input, in every package audited:

| Site | Trigger |
|---|---|
| `pkg/dockerfilegen/generator.go:211,535,551,627` | `semver.New` is `Must(NewVersion(...))` — it panics and can never return nil, so **every `if semVer != nil` guard is dead code**. A two-part version like `1.37` panics at all four sites. Line 627 has no empty-string check and is reached whenever OCP minor ≥ 15, which is true for all current configs. |
| `cmd/konflux-release-gen/main.go:74` | Same `semver.New` pattern. |
| `pkg/dockerfilegen/generator.go:532,610` | `Metadata.Requirements` is a pointer (`project.go:15`) that `ReadMetadataFile` never initializes — only `DefaultMetadata()` does. Any `project.yaml` lacking a `requirements:` block panics. |
| `pkg/konfluxapply/konflux_apply.go:43`, `pkg/konfluxgen/konfluxgen.go:677` | `WalkDir`/`Walk` callbacks ignore the incoming `err` and immediately dereference the entry. A missing or unreadable root path panics instead of erroring cleanly. |
| `pkg/soversion/version.go:86-93` | `IncrementBranchName` panics on `""` and `"main"`, both reachable from `discover.go:172`. Line 89 also has `err != nil && n != 2` where `||` is intended. |
| `pkg/rhel/rhel-version.go:35` | `br.Konflux` dereferenced with no nil check; a branch without a `konflux:` block panics. |
| `pkg/discover/discover.go:52` | `info.IsDir()` dereferenced before the incoming `err` is checked — same `Walk` nil-deref class as the two above. |

**There is no `go vet` and no linter anywhere in CI** — `make unit-tests` runs `gotestsum ./...`
plus the generated-workflow diff, nothing else. `serverless-operator/.golangci.yaml` already exists
and can be copied as a starting point. `errcheck` and `nilness` would have caught several of these.

**Also in this cleanup:**
- `Makefile:8-9` — `generate-ci-no-clean` omits `$(ARGS)` while every sibling target includes it.
  Both workflows call `make generate-ci-no-clean ARGS=--branch=main`, so that flag is **silently a
  no-op**. Harmless today, a live footgun.
- `pkg/dockerfilegen/generator.go:344-354` and `583-593` — duplicate `ParseFS` + `Execute` into a
  buffer that is never read; `saveDockerfile` re-parses and re-executes the same template. Editing
  one block silently has no effect on output.
- `pkg/dockerfilegen/generator.go:694,700` — `http.Get` with the default client (**no timeout**, so
  a hung `raw.githubusercontent` stalls CI indefinitely) and `if response.StatusCode > 400`, which
  lets `400` itself through to unmarshal into an empty map, silently dropping additional images.
  Should be `>= 400`.
- Generated artifacts written `0777` (`generator.go:414,732,737`, `testselect.go:95`,
  `ownersfilegen.go:51`) — world-writable files committed into repos.
- `Makefile:3` unconditionally includes the test fixture `pkg/project/testdata/env`
  (`export KNATIVE_GENERATE=hello`) into every build. The image-override mechanism at
  `generator.go:382` rewrites refs from any env var matching an uppercased image name, silently and
  unlogged.

---

## 5. Close the two workflow script-injection holes

**Impact: medium–high · Effort: trivial (~2 hours) · Risk: none**

- `.github/workflows/run-command-repository.yaml:56` — `run: make ${{ inputs.command }}`. Raw
  free-text `workflow_dispatch` input interpolated directly into a shell.
- `.github/workflows/generate-release-crs.yaml:78,80,108,110` — `inputs.revision` interpolated into
  four shell commands, in a job that holds **both** `KONFLUX_SA_TOKEN` (production cluster) and
  `SERVERLESS_QE_ROBOT`.

Exploitation requires `workflow_dispatch` permission, so this is privilege **escalation** —
repo-write becomes production-Konflux-credential access — rather than anonymous RCE. That still
matters: the blast radius of a compromised or careless contributor account is currently the prod
build system.

**Fix:** pass inputs through `env:` and reference `"$VAR"` inside `run:`; never inline `${{ }}` in a
shell body. Validate `revision` against `^release-[0-9.]+$` before use.

Same file `wget`s a yq binary to `/usr/bin/yq` with no checksum verification
(`run-command-repository.yaml`), and both Konflux workflows duplicate a hardcoded cluster URL in
their kubeconfig setup block.

---

## 6. GitHub Actions hardening

**Impact: high (cumulative) · Effort: low · Risk: none**

Reviewed all 7 workflows. Findings are individually small and collectively significant; most are
one-line fixes. Items already covered above are cross-referenced rather than repeated.

### 6a. No `permissions:` block in any workflow

Zero of the 7 declare `permissions:`, so every job receives the repository-default `GITHUB_TOKEN`
scope — on older repositories that is read/write across contents, packages, issues, and pull
requests. Several of these jobs additionally carry `SERVERLESS_QE_ROBOT` and `KONFLUX_SA_TOKEN`.

**Fix:** add `permissions: contents: read` at the top level of every workflow and widen per-job only
where needed (`go-unit-tests.yaml` needs nothing more; the PR-creating jobs need
`pull-requests: write`).

### 6b. No `timeout-minutes` in any workflow

Zero of the 7 set a timeout, so every job inherits the 360-minute default. This compounds directly
with the unbounded `http.Get` at `pkg/dockerfilegen/generator.go:694` (see §4): a hung
`raw.githubusercontent` request burns **six hours** of runner time before the job is killed.

**Fix:** `timeout-minutes: 45` on `release-generate-ci`, lower elsewhere. Independently, give that
HTTP client a timeout.

### 6c. Third-party action pinned to a mutable branch

`.github/workflows/release-discover-branches.yaml:30`

```yaml
- uses: tektoncd/actions/setup-tektoncd-cli@main
```

A third-party action tracked at `@main`, in a job that holds `SERVERLESS_QE_ROBOT`. Anyone able to
push to that repository's default branch gets arbitrary code execution with your robot token. The
other third-party action, `peter-evans/create-pull-request@v8` (2 workflows, both privileged), is on
a mutable major tag that can be repointed.

**Fix:** pin both to full commit SHAs with a version comment. Consider SHA-pinning `actions/*` in the
privileged workflows too (27 `actions/checkout@v6` references).

### 6d. Four dead steps in the CI-generation workflow

Verified: the Go code contains exactly two `exec.Command` sites (`pkg/prowgen/prowgen_cmd.go:21,42`)
and every caller passes `git` or `make`. Nothing shells out to `skopeo`, `yq`, or `yaml2json`, and
`skopeo` is not a Go dependency either. Nothing reads the generated `config/*.yaml.json`.

| Step (template lines 42–53) | Cost | Status |
|---|---|---|
| `go install github.com/mikefarah/yq/v3@latest` | compiles | unused — only occurrence of `yq` in the repo is a template string written *into* a target repo (`dependabotgen.go:336`) |
| `go install ... skopeo@v1.16.1` | compiles from source with build tags, the slowest of the four | unused |
| `go install github.com/bronze1man/yaml2json@latest` | compiles | unused |
| `find config/*.yaml \| xargs ... yaml2json ... > {}.json` | fast | output consumed by nothing; also writes non-gitignored junk into `config/` |

Roughly 2–3 minutes per run, plus disk pressure. Same category as the 13 dead clone steps in §3b.
**Confirm with the team before deleting** — it is possible a human or an `openshift/release`-side
process consumes the JSON, though nothing in this repository does.

### 6e. Unpinned build tooling

`yq/v3@latest`, `yaml2json@latest` (template:43,49) and `gotestsum@latest` (`Makefile:71`) resolve at
run time. That is non-reproducible and means a compromised upstream executes inside a job holding
the robot token. Note `yq` **v3** has been end-of-life for years; v4 is current.

**Fix:** pin exact versions. Moot for the first two if §6d removes them.

### 6f. Missing concurrency control on the production-apply workflow

Only `release-generate-ci` declares `concurrency`. `apply-konflux-manifests.yaml` triggers on both a
daily cron *and* pushes to `.konflux/**`, with no guard — so two runs can **apply to the production
cluster simultaneously**. Given that the apply is already non-atomic and fail-fast (§2), overlapping
runs can interleave partial writes. `release-discover-branches` likewise has none and can race with
itself on PR creation.

**Fix:** add `concurrency: { group: <workflow>, cancel-in-progress: false }` — *not* `true`, since
cancelling mid-apply is exactly the partial-state failure to avoid.

### 6g. Duplicated, unpinned cluster access

`apply-konflux-manifests.yaml` and `generate-release-crs.yaml` each hardcode the same four-line
kubeconfig block and the same cluster URL
(`api.kflux-prd-rh02.0fk9.p1.openshiftapps.com`). Both rely on the runner image's preinstalled
`kubectl`, whose version drifts silently — against a production cluster.

**Fix:** extract a `./actions/setup-konflux` composite action (`./actions/setup-go` is the existing
precedent) that pins the `kubectl` version and holds the cluster URL once.

### 6h. Go version is pinned in two places and can drift

`actions/setup-go/action.yaml` hardcodes `go-version: '1.26'` while `go.mod` declares `go 1.26.4`.
Meanwhile `run-command-repository.yaml:50` uses `actions/setup-go@v6` with
`go-version-file: "go.mod"` — the better pattern. Bumping `go.mod` today does not move CI.

**Fix:** switch the composite to `go-version-file` and delete the hardcoded version.

### Already covered elsewhere

| Finding | See |
|---|---|
| `run: make ${{ inputs.command }}` and `inputs.revision` script injection | §5 |
| `wget` of the yq binary with no checksum | §5 |
| 138 steps in a single job, no `continue-on-error`, no job parallelism | §3c |
| 13 clone steps deleted by a later `actions/checkout` | §3b |

### Worth knowing

Commit `2229cf25` ("Remove unused gh actions, switch to preinstalled tools", #1040) already deleted
the two `multiarch-*` workflows and left no dangling references — the cleanup in §6d is a
continuation of work the team has started, not a new direction.

Low priority: `.gitignore:16` ignores `rpms.lock.yaml` under a `# TODO: Remove` comment while a
68 KB `rpms.lock.yaml` is tracked at the repo root. Harmless today (gitignore does not affect
tracked files) but confusing, and a hazard if the file is ever untracked while
`release-discover-branches` regenerates lockfiles.

---

## 7. Two decomposition seams, and a documentation bug that silently breaks configs

**Impact: medium–high · Effort: low–medium · Risk: low**

### 7a. `README.md:81` documents a config key that does not exist

The struct tag is `json:"e2e,omitempty"` (`pkg/prowgen/prowgen_config.go:31`). Zero of the 13
configs use `e2eTests`; ten use `e2e`. Configs are parsed YAML → JSON → `json.Unmarshal`
(`prowgen.go:189`), and `encoding/json` **silently discards unknown keys** — so anyone following the
README's example gets no tests generated and no error.

Fix the README, and consider `DisallowUnknownFields` on the decoder so a typo in any config fails
loudly instead of silently. That single decoder change would have caught this class permanently.

Related dead surface: `nudges` and `excludesImages` are parsed but referenced by no config file;
`imageNameOverrides` and `skipPromotion` have one non-test reference each.

### 7b. Extract `pkg/config` and `pkg/git` from the `prowgen` god package

`pkg/prowgen` is 4,096 lines with **9 importers**, 7 of them other `pkg/` packages. There is also a
dependency inversion: `prowgen → konfluxgen`, but `konfluxapply → prowgen`.

Two seams, established by auditing what each importer actually consumes:

| Extract | Contents | Effect |
|---|---|---|
| `pkg/config` | `Config`, `CommonConfig`, `Branch`, `Repository`, `Konflux`, `LoadConfig`, `UnmarshalConfig` | `konfluxapply`, `rhel`, `dockerfilegen` import `prowgen` for *only* these. Drops `rhel` to **zero** prowgen deps. |
| `pkg/git` | `prowgen_git.go`, 188 lines — `GitCheckout/Clone/Fetch/Merge/DiffNameOnly`, `Repository`, `GitMirror`, `Run` | `testselect` uses only these. Drops `testselect` to **zero** prowgen deps. |

Together they decouple **4 of 7** internal importers. Both are type-and-function moves with no
logic change, and they are a prerequisite for testing the packages that currently have none
(§4). `pkg/git` is also where the sentinel-error fix from §1a belongs, and where `cmd/sync-pr`
from §3c would draw its git operations.

### 7c. `konflux-apply` never applies `releaseplanadmissions`

`cmd/konflux-apply/main.go:22-25` defaults `additionalDirs` to `releaseplans` and `releases` only.
The 22 RPA files under `.konflux/releaseplanadmissions/` are generated (`konfluxgen.go:1037,1097`)
and committed, but never pushed to the cluster. This is plausibly deliberate — RPAs are normally
owned by the Konflux tenant admin — but nothing says so. **Confirm intent and document it**, or the
next person to touch this will "fix" it and push 22 unexpected resources to production.

---

## Suggested sequence

| When | Items | Rationale |
|---|---|---|
| Week 1 | **5**, **6a–6c**, **7a**, then **1**, then **4**'s lint gate | #5, #6a–6c and #7a are hours of work each; #5/#6 close credential-exposure paths and #7a stops configs failing silently. #1 stops silent data loss. The lint gate lands before later work so everything after is checked. |
| Week 2 | **2** and **3a** in parallel; **6d–6f**, **7c** alongside | Independent. #2 is safety, #3a is the biggest perf lever, #6d is free time back, #7c is a question to answer before someone guesses wrong. |
| Week 3+ | **7b**, then **3b**, **3c**, rest of **4**, **6g–6h** | #7b (`pkg/git`) is a prerequisite for both the #1 sentinel-error fix and `cmd/sync-pr` in #3c, so it comes before the matrix work. |

---

## Not problems (checked, no action needed)

Recording these so they don't get re-investigated:

- **`testselect` is fail-safe.** An unmatched path falls back to running the full suite, so there is
  no false-green risk today. Two real-but-minor defects: `prowgen_git.go:187` —
  `strings.Split("", "\n")` returns `[""]`, producing a phantom empty path that causes a spurious
  full-suite run (safe direction); and `testselect.go:122` recompiles every regex for every path,
  roughly 21k compilations per run. Worth noting that
  `serverless-operator/test/testsuites.yaml:67` has an unescaped dot (`^.github/` should be
  `^\.github/`) — the one route to a false green would be an unanchored pattern in the `NoTests`
  suite, so pattern anchoring deserves validation.
- **Generated output is deterministic.** `pkg/action` uses `sortedKeys` plus a lexical
  `filepath.Walk`, which is why the `make unit-tests` diff is stable. Earlier speculation about
  unsorted-map YAML churn was unfounded.
- **`yaml.Unmarshal` into the non-pointer map** at `dockerfilegen/generator.go:709` works correctly.
- **63-character Konflux name truncation** in `konfluxgen` is correct.
- **`go vet` is clean** across `dockerfilegen`, `testselect`, `ownersfilegen`, `k8sresource`,
  `project`, `prowcopy`.

## Known blocker for test coverage

`pkg/dockerfilegen` is 952 lines with zero tests for a structural reason: `GenerateDockerfiles`
calls `os.Chdir` (`generator.go:70`), a process-global side effect that makes the package
non-reentrant and untestable in parallel. Removing that is a prerequisite for any meaningful
coverage there. Same situation, smaller scale, in `pkg/dependabotgen` (359 lines, 0 tests),
`pkg/discover` (314, 0) and `pkg/action` (282, 0).

A latent nondeterminism also lives here: `generator.go:356,360,363` derive the output path from
`filepath.Base(p)`, so two main packages sharing a leaf name (`cmd/a/server`, `cmd/b/server`) map to
one directory which `saveDockerfile` `RemoveAll`s first. The loop iterates
`mainPackagesPaths.UnsortedList()` (`:283`, random order), so **which package wins differs per
run**. Checked serverless-operator and client: no collisions today, so this is latent, not firing.

---

## Corrections to the previous revision

The earlier version of this file, `ARCHITECTURE.md`, and `QUICK_START_IMPROVEMENTS.md` were written
before the Go source was read. Corrections:

| Previous claim | Reality |
|---|---|
| "Parallelize the 13 repo clones to save ~5 min" | Those steps are **dead work** — `actions/checkout` at line 134 deletes them before use. Parallelizing optimizes work that is thrown away. **Do not implement.** |
| "Cache cloned repos" | `gitClone` (`prowgen_git.go:139`) skips fetching when the directory exists. Caching would generate config from **stale branch content**. **Do not implement.** |
| "Workflow takes 45–60 min" | Measured ~35 min. |
| "PR creation takes 30–45 min" | ~11 min. The real bottleneck is `Generate CI` at 23m35s (67%). |
| "~500+ API calls, rate-limit risk" | Not measured; no evidence gathered. Disregard. |
| "Refactor the 2,679-line workflow into a matrix" | Right idea, wrong target — the file is generated, and `make unit-tests` enforces it. Change `pkg/action/update_action.go`. |
| "Generated YAML may be unstable from unsorted maps" | Output is deterministic. Unfounded. |
| "Introduce a central `repos.yaml` as single source of truth" | **Drop this.** As dsimansk notes on PR #1051, `config/*.yaml` already drives test discovery and CI-job creation; the proposed schema was a strict subset. A second config file would duplicate and then diverge from the real one. |
| "Extract the duplicated bash into a reusable script" | Misdiagnosed. The duplication is generated from a template and already fixable in one place (dsimansk, PR #1051). The cost is runtime and failure isolation, not maintenance — see §3c. |
| "Cache cloned repos" (second reason) | Beyond the staleness bug in §3, dsimansk flags likely GitHub Actions cache-storage limits for repos of this size. Two independent reasons not to. |
| Config example using `e2eTests:` | The key is `e2e`; `e2eTests` is silently discarded. See §7a — the README itself is wrong. |

**`QUICK_START_IMPROVEMENTS.md` should be deleted or gutted** — two of its five recommendations are
in the "do not implement" rows above. `ARCHITECTURE.md` contains the fabricated performance figures
in this table; its structural description of the system is broadly accurate but its numbers are not.
