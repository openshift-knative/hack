# Current Architecture Overview

## System Diagram

```
┌──────────────────────────────────────────────────────────────────────────┐
│                     openshift-knative/hack (This Repo)                    │
├──────────────────────────────────────────────────────────────────────────┤
│                                                                           │
│  ┌─────────────────────┐      ┌─────────────────────────────────────┐   │
│  │   Configuration     │      │        Go Tools (cmd/)              │   │
│  │   (config/)         │──────▶                                      │   │
│  │                     │      │  • prowgen    - CI config generation│   │
│  │  • client.yaml      │      │  • discover   - Branch discovery    │   │
│  │  • eventing.yaml    │      │  • konflux-*  - Konflux management  │   │
│  │  • serving.yaml     │      │  • generate   - Dockerfile gen      │   │
│  │  • serverless-op... │      │  • sobranch   - Version mapping     │   │
│  │  • 10+ more repos   │      │  • testselect - Test selection      │   │
│  └─────────────────────┘      └─────────────────────────────────────┘   │
│           │                                     │                         │
│           │                                     │                         │
│           ▼                                     ▼                         │
│  ┌─────────────────────────────────────────────────────────────────┐    │
│  │          GitHub Actions Workflows (.github/workflows/)          │    │
│  │                                                                  │    │
│  │  1. release-generate-ci.yaml (2,679 lines!)                     │    │
│  │     ├─ Clones 13 repos                                          │    │
│  │     ├─ Runs prowgen for each                                    │    │
│  │     ├─ Generates CI configs                                     │    │
│  │     └─ Creates 100+ PRs                                         │    │
│  │                                                                  │    │
│  │  2. release-discover-branches.yaml                              │    │
│  │     ├─ Discovers new release branches                           │    │
│  │     ├─ Updates config files                                     │    │
│  │     └─ Updates Konflux pipelines                                │    │
│  │                                                                  │    │
│  │  3. apply-konflux-manifests.yaml                                │    │
│  │     └─ Applies manifests to Konflux cluster                     │    │
│  │                                                                  │    │
│  │  4. generate-release-crs.yaml                                   │    │
│  │     └─ Generates release CRs for new releases                   │    │
│  └─────────────────────────────────────────────────────────────────┘    │
│                                                                           │
└───────────────────────────────────┬───────────────────────────────────────┘
                                    │
                    ┌───────────────┴───────────────┐
                    │                               │
                    ▼                               ▼
    ┌───────────────────────────┐   ┌──────────────────────────────┐
    │  openshift/release        │   │  Component Repos (13+)       │
    │                           │   │                              │
    │  CI Configuration PRs     │   │  • client                    │
    │  • Prow jobs              │   │  • eventing                  │
    │  • Build configs          │   │  • serving                   │
    │  • Test definitions       │   │  • serverless-operator       │
    └───────────────────────────┘   │  • eventing-kafka-broker     │
                                    │  • ... 8 more                │
                                    │                              │
                                    │  Each gets PRs for:          │
                                    │  ├─ Konflux configs          │
                                    │  ├─ OWNERS files             │
                                    │  └─ Dependabot configs       │
                                    └──────────────────────────────┘
```

## Data Flow

### 1. Branch Discovery Flow (Daily)
```
┌─────────────┐
│ Cron: 05:00 │
└──────┬──────┘
       │
       ▼
┌────────────────────────────────────────┐
│ Workflow: release-discover-branches    │
│                                        │
│ 1. For each repo in config/            │
│    ├─ Query GitHub API for branches   │
│    ├─ Match pattern: release-*        │
│    └─ Compare with config/*.yaml      │
│                                        │
│ 2. If new branches found:             │
│    ├─ Update config/*.yaml            │
│    ├─ Update Konflux pipelines        │
│    ├─ Update RPM lock files           │
│    └─ Create PR to hack repo          │
└────────────────────────────────────────┘
       │
       ▼
┌──────────────────────┐
│ PR: "Add new         │
│ midstream branches   │
│ and update pipelines"│
└──────────────────────┘
```

### 2. CI Config Generation Flow (Weekly + on Push)
```
┌─────────────────┐
│ Cron: Mon 06:00 │
│ or Push to main │
└────────┬────────┘
         │
         ▼
┌──────────────────────────────────────────────────────────┐
│ Workflow: release-generate-ci (THE BIG ONE)              │
│                                                          │
│ Step 1: Clone ALL repos (sequential!)                   │
│  ├─ backstage-plugins/main                              │
│  ├─ client/main                                         │
│  ├─ eventing/main                                       │
│  ├─ ... (13 repos × ~4 branches each = 50+ clones)     │
│  └─ openshift/release/main                              │
│                                                          │
│ Step 2: Generate CI configs                             │
│  └─ Run prowgen for each config/*.yaml                  │
│      ├─ Parse repo Makefiles                            │
│      ├─ Generate Prow job configs                       │
│      ├─ Generate build configs                          │
│      └─ Output to openshift/release repo                │
│                                                          │
│ Step 3: Create PRs (100+ repetitive steps!)             │
│  For each (repo, branch, config_type):                  │
│    ├─ Check if remote branch exists                     │
│    ├─ Create/update branch                              │
│    ├─ Push changes                                      │
│    └─ Create PR (or update existing)                    │
│                                                          │
│  Example matrix (incomplete!):                          │
│    backstage-plugins × (main, release-v1.17, ...)       │
│                      × (konflux, owners, dependabot)    │
│    = 15 PRs                                             │
│                                                          │
│    × 13 repos = 195 PRs! 😱                             │
└──────────────────────────────────────────────────────────┘
         │
         ▼
┌─────────────────────────────────────────────┐
│ Results:                                    │
│  • 1 PR to openshift/release (CI configs)   │
│  • 1 PR to hack (Konflux configs)           │
│  • ~194 PRs to component repos              │
└─────────────────────────────────────────────┘
```

### 3. Release Flow (Manual)
```
┌────────────────────────┐
│ Manual Trigger         │
│ (workflow_dispatch)    │
└───────────┬────────────┘
            │
            ▼
┌─────────────────────────────────────────┐
│ Workflow: generate-release-crs          │
│                                         │
│ 1. Generate release CRs                 │
│    ├─ Component release CRs             │
│    └─ FBC (catalog) release CRs         │
│                                         │
│ 2. Apply snapshot overrides             │
│                                         │
│ 3. Create PRs                           │
│    ├─ Component release CR → hack repo  │
│    └─ FBC release CR → hack repo        │
└─────────────────────────────────────────┘
            │
            ▼
┌─────────────────────────────────────────┐
│ Manual Process:                         │
│  1. Merge component PR                  │
│  2. Wait for release pipeline success   │
│  3. Then merge FBC PR                   │
│     (FBC references released components)│
└─────────────────────────────────────────┘
```

## Key Problems Visualized

### Problem 1: Massive Repetition
```yaml
# This pattern is repeated 100+ times:

- name: '[client - release-v1.17] Create Konflux PR'
  run: |
    set -x
    repo="client"
    branch="sync-konflux-release-v1.17"
    target_branch="release-v1.17"
    git remote add fork "https://github.com/serverless-qe/$repo.git" || true
    remote_exists=$(git ls-remote --heads fork "$branch")
    if [ -z "$remote_exists" ]; then
      git push "https://serverless-qe:${GH_TOKEN}@github.com/serverless-qe/$repo.git" "$branch:$branch" -f
    fi
    git fetch origin "$target_branch"
    git fetch fork "$branch"
    git rebase --quiet "$target_branch" "$branch"
    git push "https://serverless-qe:${GH_TOKEN}@github.com/serverless-qe/$repo.git" "$branch:$branch" -f
    gh pr create --base "$target_branch" --head "serverless-qe:$branch" \
      --title "[$target_branch] Update Konflux configurations" \
      --body "Update Konflux configurations" || true

- name: '[client - release-v1.21] Create Konflux PR'
  run: |
    set -x
    repo="client"
    branch="sync-konflux-release-v1.21"  # ← Only difference!
    target_branch="release-v1.21"        # ← Only difference!
    # ... EXACT SAME CODE AGAIN ...

# Repeated for:
# - 13 repos
# - ~4 branches each
# - 3 config types each
# = 156 nearly identical steps! 😱
```

### Problem 2: Sequential Cloning
```
Time ──────────────────────────────────────────────────▶

Clone backstage-plugins  ████████ (30s)
                                  Clone client  ████████ (30s)
                                                          Clone eventing  ████████ (30s)
                                                                                   ...
Total: ~13 repos × 30s = 6.5 minutes just for cloning! ⏱️

With parallel cloning:
Clone all repos in parallel  ████████ (30s)
Total: 30 seconds! 🚀
```

### Problem 3: Hard to Change
```
To add a new repository:
  ❌ Edit workflow template: +50 lines
  ❌ Edit generated workflow: +50 lines  
  ❌ Test locally: impossible (GitHub Actions only)
  ❌ Debug failures: parse 2,679 lines
  ⏱️  Time: ~2 hours

With improved architecture:
  ✅ Add to repos.yaml: 1 line
  ✅ Config auto-validated
  ✅ Test locally with Dagger
  ✅ Clear error messages
  ⏱️  Time: ~5 minutes
```

## Technology Stack

### Current Stack
```
┌─────────────────────────────────────────────┐
│ Languages                                   │
│  • Go (tooling)                             │
│  • Bash (workflow scripts)                  │
│  • YAML (configs + workflows)               │
└─────────────────────────────────────────────┘
┌─────────────────────────────────────────────┐
│ Tools                                       │
│  • GitHub Actions (orchestration)           │
│  • prowgen (custom, Prow config gen)        │
│  • Kustomize (Konflux manifests)            │
│  • yq (YAML processing)                     │
│  • skopeo (image operations)                │
└─────────────────────────────────────────────┘
┌─────────────────────────────────────────────┐
│ External Systems                            │
│  • GitHub API                               │
│  • OpenShift CI (Prow)                      │
│  • Konflux (build/release)                  │
│  • Tekton (pipelines)                       │
└─────────────────────────────────────────────┘
```

### Recommended Additions
```
┌─────────────────────────────────────────────┐
│ Orchestration                               │
│  • Dagger (local + CI, containerized)       │
│  OR                                         │
│  • Earthly (reproducible builds)            │
└─────────────────────────────────────────────┘
┌─────────────────────────────────────────────┐
│ Configuration Management                    │
│  • JSON Schema (validation)                 │
│  • CUE (config validation + generation)     │
└─────────────────────────────────────────────┘
┌─────────────────────────────────────────────┐
│ Observability                               │
│  • OpenTelemetry (tracing)                  │
│  • Prometheus (metrics)                     │
│  • Grafana (dashboards)                     │
└─────────────────────────────────────────────┘
```

## File Structure

```
/Users/kpande/midstream/hack/
├── .github/
│   ├── workflows/
│   │   ├── release-generate-ci.yaml        (2,679 lines) 😱
│   │   ├── release-discover-branches.yaml  (98 lines)
│   │   ├── apply-konflux-manifests.yaml    (~100 lines)
│   │   └── generate-release-crs.yaml       (~100 lines)
│   └── actions/                            ← Would add reusable actions here
│
├── cmd/                                    # CLI tools
│   ├── prowgen/                           # Most important tool
│   ├── discover/
│   ├── konflux-apply/
│   ├── konflux-gen/
│   ├── konflux-release-gen/
│   └── ... (5 more)
│
├── config/                                 # Repo configurations
│   ├── client.yaml
│   ├── eventing.yaml
│   ├── serving.yaml
│   ├── serverless-operator.yaml           # Largest config
│   └── ... (10 more)
│
├── pkg/                                    # Shared Go packages
│   └── ... (17 packages)
│
├── openshift/                              # Generated output (gitignored)
│   └── release/                           # openshift/release clone
│
└── openshift-knative/                      # Cloned repos (gitignored)
    ├── client/
    ├── eventing/
    └── ... (13+ repos)
```

## Interaction with serverless-operator

```
┌──────────────────────────────────────────────────────────────────────┐
│                    serverless-operator repo                          │
├──────────────────────────────────────────────────────────────────────┤
│                                                                       │
│  • Main codebase for OpenShift Serverless Operator                   │
│  • Contains actual operator implementation                           │
│  • Has Makefile with build/test targets                              │
│  • Multiple branches: main, release-1.37, release-1.38, etc.         │
│                                                                       │
│  Receives from hack repo:                                            │
│  ├─ .tekton/ (Konflux pipeline configs)                              │
│  ├─ OWNERS (maintainer list)                                         │
│  └─ .github/dependabot.yml                                           │
│                                                                       │
│  Consumed by hack repo:                                              │
│  └─ Makefile (parsed by prowgen to generate CI tests)                │
│                                                                       │
└───────────────────────────────┬──────────────────────────────────────┘
                                │
                                │ Managed by
                                │
                                ▼
┌──────────────────────────────────────────────────────────────────────┐
│                         hack repo                                     │
│                                                                       │
│  config/serverless-operator.yaml (largest config file)               │
│  ├─ Defines branches: main, release-1.37, release-1.38              │
│  ├─ Defines OpenShift versions: 4.16, 4.20, 4.21, 4.22              │
│  ├─ Defines Konflux settings                                        │
│  └─ Defines promotion settings                                      │
│                                                                       │
│  Generates:                                                          │
│  ├─ Prow CI jobs → openshift/release                                │
│  ├─ Konflux manifests → serverless-operator/.tekton/                │
│  └─ OWNERS, dependabot → serverless-operator/                       │
└──────────────────────────────────────────────────────────────────────┘
```

## Performance Characteristics (Current)

```
Workflow: release-generate-ci
├─ Trigger: Weekly + on push to main
├─ Duration: ~45-60 minutes 😴
│   ├─ Setup (checkout, install tools): ~2 min
│   ├─ Clone repos: ~6.5 min (sequential)
│   ├─ Generate CI: ~3 min
│   ├─ Create PRs: ~30-45 min (API rate limits)
│   └─ Overhead: ~3-5 min
│
├─ Resources:
│   ├─ GitHub Actions runners: 1 ubuntu-latest
│   ├─ API calls: ~500+ (potential rate limits)
│   └─ Storage: ~5 GB (all cloned repos)
│
└─ Pain points:
    ├─ Long feedback loop (commit → see results: ~1 hour)
    ├─ Can't test locally
    ├─ Hard to debug failures
    └─ Wastes resources (re-clones everything)

Workflow: release-discover-branches
├─ Trigger: Daily at 05:00
├─ Duration: ~10-15 minutes
├─ Usually no changes (wasted runs)
└─ Could be event-driven instead
```

## Success Metrics Baseline (Current State)

To track improvement, here are current baseline metrics:

| Metric | Current | Target (Post-Improvement) |
|--------|---------|---------------------------|
| Workflow file size | 2,679 lines | < 500 lines |
| CI generation time | 45-60 min | < 20 min |
| Time to add new repo | ~2 hours | < 5 min |
| Number of API calls | ~500+ | < 100 |
| Can test locally? | ❌ No | ✅ Yes |
| Clear error messages? | ❌ No | ✅ Yes |
| Parallel execution? | ❌ No | ✅ Yes |
| Incremental updates? | ❌ No | ✅ Yes |

---

## Next: See IMPROVEMENT_ROADMAP.md

This document explains the current architecture. For detailed improvement recommendations and implementation plan, see `IMPROVEMENT_ROADMAP.md`.
