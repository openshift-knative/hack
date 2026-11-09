# Architecture

How `openshift-knative/hack` actually works, traced through the source. Every claim carries a
`file:line` anchor. Items that could not be confirmed are marked **[unverified]**.

> **Supersedes the previous revision**, whose performance figures were estimated rather than
> measured and whose description of the workflow layer was wrong in a way that mattered. See
> [Corrections](#corrections) at the bottom.

---

## What this repository is

A control plane. It holds no product code. It reads configuration describing 13 midstream
repositories, clones them, inspects them, and writes generated configuration **into** them and into
`openshift/release`. Nearly everything it produces lands in another repository as a pull request.

The unit of configuration is `config/<repo>.yaml` — 13 files, each with a `repositories:` list and a
`config.branches:` map. Two further config files live outside that directory:
`pkg/discover/unsupported.yaml` and `pkg/discover/ignored.yaml` (`discover.go:28-29`).

---

## Entry points

Four scheduled or dispatched workflows. Everything else is a library.

| Workflow | Trigger | Make target | Entry function |
|---|---|---|---|
| `release-generate-ci` | weekly Mon 06:00 + push to main | `generate-ci-no-clean` | `prowgen.Main()` — `prowgen.go:39` |
| `release-discover-branches` | daily 05:00 | `discover-branches`, `konflux-update-pipelines` | `discover.Main()` — `discover.go:32` |
| `apply-konflux-manifests` | daily 06:00 + push to `.konflux/**` | `konflux-apply` | `konfluxapply.Apply()` ×3 |
| `generate-release-crs` | manual dispatch | `generate-konflux-release` | `cmd/konflux-release-gen/main.go` |

### `prowgen.Main()` — the main pipeline

`prowgen.go:39-181`, in order:

1. Load all configs
2. Sort `OpenShiftVersions` by semver (`:97`)
3. **`errgroup`**: `InitializeOpenShiftReleaseRepository` concurrently with per-repository
   `NewGenerateConfigs` (`:113`)
4. `DeleteExistingReleaseBuildConfigurationForBranch`
5. `SaveReleaseBuildConfiguration` → `SaveProwConfig` → `RunOpenShiftReleaseGenerator`
6. `PushBranch` → `GenerateKonflux` → `GenerateOwners`

Flags `-build`, `-push`, `-konflux`, `-owners` all default **true**.

### Where the time actually goes

Generation is **already parallel per repository** via the `errgroup` at `:113`. The serialization is
one level down: `NewGenerateConfigs` (`prowgen_config.go:160`) calls `GitMirror` once, then loops
branches **sequentially**, `GitCheckout`-ing each (`:193`) and re-reading that repo's `Makefile`
(`prowgen_tests.go:321`) once per branch.

The dominant cost is not hack's own logic. It is the target repositories' build tooling —
`deepcopy-gen` and friends — invoked downstream with a cold module cache.

**Measured**, CI run `40897223955` (2026-10-29):

| | |
|---|---|
| `release-generate-ci` total | ~35 min |
| └─ `Generate CI` step | **23m35s — 67% of the job** |
| └─ `Freeup disk space` | 88s |
| `go: downloading` lines, one run | **2,668** |
| Repos cloned by prowgen itself | 14 (~106s) |

---

## Configuration schema

Parsed YAML → JSON → `json.Unmarshal` (`prowgen.go:189`). **Only `json:` tags bind**, and
`encoding/json` silently ignores unknown keys — so a misspelled field is discarded without error.

**`Repository`** (`prowgen_config.go:23`) — `org`, `repo`, `promotion`, `imagePrefix`,
`imageNameOverrides`, `slackChannel`, `canonicalGoRepository`, **`e2e`**, `dockerfiles`,
`ignoreConfigs`, `customConfigs`, `images`, `tests`, `resources`, `owners`.

**`Branch`** (`:85`) — `prowgen.disabled`, `promotion`, `openShiftVersions`, `skipE2EMatches`,
`skipDockerFilesMatches`, `konflux`, `golangVersion`, `dependabotEnabled`.

**`OpenShift`** (`:117`) — `version`, `useClusterPool`, `cron`, `skipCron`, `onDemand`,
`customConfigs{enabled,includes,excludes}`, `candidateRelease`, `skipPromotion`, `skipE2EMatches`,
`includeE2EMatches`.

**`Konflux`** (`:101`) — `enabled`, `nudges`, `excludes`, `excludesImages`, `javaImages`,
`imageOverrides[]{name,pullSpec}`.

### Semantics worth knowing

- **E2E tests are discovered, not declared.** `E2ETest.Match` is regex-matched against target lines
  in the cloned repository's `Makefile` (`prowgen_tests.go:320-361`). Changing a target name
  upstream changes the generated jobs.
- `onDemand` → `always_run: false` on the presubmit.
- `skipCron` suppresses periodics at **two independent levels**: per-test (`E2ETest.SkipCron`) and
  per-OpenShift-version (`OpenShift.SkipCron`).
- `includeE2EMatches` whitelists and overrides the branch-level set; `skipE2EMatches` blacklists.
  Both compare **exactly** against `E2ETest.Match`, not against the resolved target name.
- `customConfigs` bypasses Makefile discovery entirely: `releaseBuildConfiguration` is taken
  verbatim, then enriched with images, base images, and step dependencies.

### Schema defects

- **`README.md:81` documents `e2eTests:`. The real key is `e2e`** (`prowgen_config.go:31`,
  `json:"e2e,omitempty"`). Zero of 13 configs use `e2eTests`; 10 use `e2e`. Because unknown JSON
  keys are dropped silently, following the README produces **no tests and no error**.
- `nudges` and `excludesImages` are parsed but used by no config file — dead surface.
- `imageNameOverrides` and `skipPromotion` have exactly one non-test code reference each.

---

## Artifact flow

`konfluxgen.Config` writes to three distinct roots (`prowgen_konflux.go:225-228`):

| Output root | Path | Contents |
|---|---|---|
| `PipelinesOutputPath` | `<target-repo>/.tekton` | Tekton PipelineRuns |
| `ResourcesOutputPath` | `<target-repo>/.konflux` | `applications/<app>/components/`, `imagerepositories/` |
| `GlobalResourcesOutputPath` | `hack/.konflux` | `applications/<app>/<app>.yaml`, `tests/` |

**For serverless-operator, both resource roots collapse into hack's own `.konflux`**
(`prowgen_konflux.go:353,457-460`). Confirmed: serverless-operator has no `.konflux` directory at
all, and all 388 files under `hack/.konflux` are `serverless-operator-*`.

`hack/.konflux` contents: `releases/` 65, `releaseplans/` 118, `releaseplanadmissions/` 22,
`applications/` 37 (one per `<so-version>`, plus one per `<so-version>-fbc-<ocp>`).

**`konflux-apply` never applies `releaseplanadmissions`.** `cmd/konflux-apply/main.go:22-25`
defaults `additionalDirs` to `releaseplans` and `releases` only. The 22 RPA files are generated
(`konfluxgen.go:1037,1097`) and committed, but never pushed to the cluster. This is plausibly
deliberate — RPAs are normally owned by the Konflux tenant admin — but it is not documented.
**[unverified]**

Also written into every managed repository: `OWNERS` (`ownersfilegen.go:50`),
`.github/dependabot.yml` (`dependabotgen.go:274`), `renovate.json` (`:293`),
`.github/workflows/dependabot-deps.yaml` (`:354`). Prow CI configs go to
`openshift/release/ci-operator/config/<org>/<repo>/`.

---

## Package coupling

```
action        -> dependabotgen ownersfilegen prowgen
discover      -> action konfluxgen prowgen soversion
dockerfilegen -> project prowgen rhel soversion util
konfluxapply  -> prowgen
konfluxgen    -> k8sresource soversion util
prowgen       -> dependabotgen konfluxgen ownersfilegen project soversion util
prowcopy, rhel, testselect -> prowgen
```

`pkg/prowgen` is the god package: **4,096 lines, 9 importers**, 7 of them other `pkg/` packages.
Note the inversion — `prowgen → konfluxgen`, but `konfluxapply → prowgen`.

### Two decomposition seams that are actually feasible

Established by auditing what each importer genuinely consumes, not by inspection:

1. **`pkg/config`** — the schema and loader. `konfluxapply`, `rhel`, and `dockerfilegen` import
   `prowgen` *solely* for `Config`, `CommonConfig`, `Branch`, `Repository`, `Konflux`, `LoadConfig`,
   `UnmarshalConfig`. Extracting these drops `rhel` to **zero** prowgen dependencies.
2. **`pkg/git`** — `prowgen_git.go`, 188 lines. `testselect` uses *only*
   `GitCheckout`/`GitClone`/`GitFetch`/`GitMerge`/`GitDiffNameOnly`/`Repository`; `konfluxapply`
   adds `GitMirror` and `Run`. Extracting drops `testselect` to **zero** prowgen dependencies.

Together these decouple **4 of 7** internal importers and leave the `prowgen`/`konfluxgen`
entanglement as the only remaining knot. Both are type-and-function moves with no logic change —
low risk, and they are a prerequisite for testing the packages that currently have none.

---

## Relationship with serverless-operator

**hack reads from SO:**

| Path | Used for | Anchor |
|---|---|---|
| `olm-catalog/serverless-operator/project.yaml` | the master version manifest | `prowgen_konflux.go:135,384` |
| `Makefile` | e2e target discovery | `prowgen_tests.go:321` |
| `olm-catalog/.../serverless-operator.clusterserviceversion.yaml` | CSV metadata | `prowgen_konflux.go:465` |
| `openshift/ci-operator/**images**` | Dockerfile discovery | `prowgen_images_discovery.go:40-46` |
| `rpms.lock.yaml`, `vendor/`, `go.mod` | presence probes | `prowgen_konflux.go:534-545` |

`project.yaml` is the keystone: `project.version` (currently 1.37.3), `olm.replaces` /
`skipRange` / `previousBundles`, `requirements.ocpVersion.{list,min,build,label}`,
`requirements.golang`, and `dependencies.*_artifacts_branch` mapping each midstream component to its
branch.

**hack writes into SO:** `.tekton/` (38 files), `OWNERS`, `.github/dependabot.yml`,
`renovate.json`, `.github/workflows/dependabot-deps.yaml`. **Not** `.konflux/` — that is centralized
here.

**SO-owned, hand-maintained:** `.konflux-release/override-snapshot-fbc-4*.yaml`, consumed by SO's
own `make apply-override-snapshot` during `generate-release-crs`.

---

## Lifecycle of a release branch

| # | Stage | Automation | Human step |
|---|---|---|---|
| 1 | Branch cut in a midstream repo | none | **manual**, upstream-driven |
| 2 | `release-discover-branches` lists branches (`discover.go:109`), compares against `configuredBranches[len-1]`, adds config entries, regenerates the workflow via `action.UpdateAction`, refreshes Tekton bundles and RPM lockfiles | daily 05:00 | **merge the PR** |
| 3 | `release-generate-ci` emits Prow configs → PR to `openshift/release`; Konflux manifests → PRs to each repo's `.tekton/` and hack's `.konflux/` | weekly + push | **merge ~114 PRs** |
| 4 | `apply-konflux-manifests` pushes `.konflux/` to the production cluster | daily 06:00, unattended | none |
| 5 | Version bump in `project.yaml` | none | **manual edit** |
| 6 | `generate-release-crs` applies override snapshots, emits component then FBC Release CRs, opens two PRs labelled `do-not-merge/hold` | manual dispatch | **unhold component, await pipeline, then unhold FBC** |
| 7 | `removeUnsupportedBranches` (`discover.go:224`) prunes config and `rm -rf`s `.konflux/**/serverless-operator-<Sanitize(v)>*` (`:269`) | daily, inside stage 2 | none |

**Five of seven stages require human action.** Stage 6's ordering constraint — component Release
must succeed before FBC — is enforced only by a label and the README;
`cmd/konflux-release-gen/main.go` treats `--type component` and `--type fbc` as fully independent.

Stage 4 is the only stage that mutates production infrastructure, and it is the only fully
unattended one.

---

## Corrections

The previous revision of this file was written without reading the Go source.

| Previous claim | Reality |
|---|---|
| "45–60 min workflow, PR creation 30–45 min" | ~35 min; PR steps ~11 min. The real cost is `Generate CI` at 23m35s (67%). |
| "~500+ API calls, rate-limit risk" | Never measured. No evidence. Disregard. |
| "Sequential cloning is the bottleneck — parallelize it" | The 13 clone steps are **deleted** by `actions/checkout` at `release-generate-ci.yaml:134` before anything reads them. Also, generation is already parallel per repository via `errgroup`. |
| "Nothing runs in parallel" | Per-repository generation is parallel; per-*branch* iteration inside `NewGenerateConfigs` is sequential. |
| "The 2,679-line workflow should be refactored" | It is a generated artifact. `make unit-tests` diffs it. Change `pkg/action/update_action.go`. |
| Config example using `e2eTests:` | The key is `e2e`. `e2eTests` is silently ignored. |

See `IMPROVEMENT_ROADMAP.md` for the prioritized plan derived from this analysis.
