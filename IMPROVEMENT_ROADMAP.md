# Automation Infrastructure Improvement Roadmap

## Executive Summary

This document outlines a phased approach to modernize the openshift-knative/hack automation infrastructure, making it **faster, more maintainable, and easier to extend**.

**Current State**: 2,679-line monolithic workflow with extensive repetition
**Goal**: Modular, scalable automation that's 50%+ faster and easy to understand

---

## Phase 1: Quick Wins (1-2 weeks)

### 1.1 Use GitHub Actions Matrix Strategy

**Problem**: 100+ repetitive workflow steps for each repo/branch combination

**Solution**: Replace with matrix strategy

```yaml
# Instead of 100+ steps like:
# - [client - release-v1.17] Create Konflux PR
# - [client - release-v1.21] Create Konflux PR
# - [client - release-v1.22] Create Konflux PR
# ...

# Use:
jobs:
  sync-configs:
    strategy:
      matrix:
        repo:
          - backstage-plugins
          - client
          - eventing
          - eventing-integrations
          # ... etc
        config:
          - type: konflux
            branch: release-v1.17
          - type: konflux  
            branch: release-v1.21
          # ... etc
    steps:
      - uses: ./.github/actions/sync-repo-config  # Reusable action
        with:
          repo: ${{ matrix.repo }}
          config-type: ${{ matrix.config.type }}
          branch: ${{ matrix.config.branch }}
```

**Benefits**:
- Reduce workflow from 2,679 → ~200 lines
- Add new repo = 1 line, not 50+ lines
- Easier to test and debug
- Parallel execution by default

**Effort**: Medium (2-3 days)
**Impact**: High

---

### 1.2 Create Reusable Composite Actions

**Problem**: Same bash scripts duplicated 100+ times

**Solution**: Extract to `.github/actions/` directory

```
.github/actions/
├── sync-repo-config/
│   ├── action.yml
│   └── sync.sh
├── create-pr/
│   ├── action.yml
│   └── create-pr.sh
└── clone-repo/
    ├── action.yml
    └── clone.sh
```

**Example**: `.github/actions/sync-repo-config/action.yml`
```yaml
name: 'Sync Repo Config'
description: 'Sync configuration files to a repository'
inputs:
  repo:
    description: 'Repository name'
    required: true
  config-type:
    description: 'Type of config (konflux, owners, dependabot)'
    required: true
  branch:
    description: 'Target branch'
    required: true
  token:
    description: 'GitHub token'
    required: true

runs:
  using: 'composite'
  steps:
    - name: Sync config
      shell: bash
      run: |
        # Single implementation used everywhere
        .github/actions/sync-repo-config/sync.sh \
          "${{ inputs.repo }}" \
          "${{ inputs.config-type }}" \
          "${{ inputs.branch }}"
```

**Benefits**:
- DRY principle - one implementation
- Easy to fix bugs (change once, affects all)
- Testable in isolation

**Effort**: Low (1-2 days)
**Impact**: High

---

### 1.3 Add Workflow Concurrency and Caching

**Problem**: Sequential cloning of 13+ repos, no caching

**Solution**:

```yaml
jobs:
  generate-ci:
    steps:
      # Cache Go modules
      - uses: actions/cache@v3
        with:
          path: |
            ~/.cache/go-build
            ~/go/pkg/mod
          key: ${{ runner.os }}-go-${{ hashFiles('**/go.sum') }}
          
      # Cache cloned repos
      - uses: actions/cache@v3
        with:
          path: ./src/github.com/openshift-knative/hack/openshift-knative/
          key: repos-${{ github.run_id }}
          restore-keys: repos-
          
      # Parallel repo cloning
      - name: Clone repositories in parallel
        run: |
          repos=(
            "backstage-plugins"
            "client"
            "eventing"
            # ... etc
          )
          for repo in "${repos[@]}"; do
            git clone "git@github.com:openshift-knative/$repo.git" \
              "./src/github.com/openshift-knative/hack/openshift-knative/$repo" &
          done
          wait  # Wait for all background jobs
```

**Benefits**:
- 50%+ faster execution
- Reduced API calls to GitHub

**Effort**: Low (1 day)
**Impact**: Medium

---

## Phase 2: Structural Improvements (2-4 weeks)

### 2.1 Config-Driven Repo Management

**Problem**: Repo list hardcoded in workflow

**Solution**: Single source of truth in YAML

Create `repos.yaml`:
```yaml
repositories:
  - name: backstage-plugins
    branches:
      - main
      - release-v1.17
      - release-v1.21
      - release-v1.22
      - release-v1.23
    configs:
      - konflux
      - owners
      - dependabot
      
  - name: client
    branches:
      - main
      - release-v1.17
      - release-v1.21
      - release-v1.22
      - release-v1.23
    configs:
      - konflux
      - owners
      - dependabot
      
  # ... etc

config_types:
  konflux:
    pr_title: "[$branch] Update Konflux configurations"
    pr_body: "Update Konflux configurations"
  owners:
    pr_title: "[$branch] Update OWNERS file"
    pr_body: "Update OWNERS file"
  dependabot:
    pr_title: "[$branch] Update dependabot configurations"
    pr_body: "Update dependabot configurations"
```

Then workflow reads this:
```yaml
jobs:
  load-config:
    runs-on: ubuntu-latest
    outputs:
      matrix: ${{ steps.set-matrix.outputs.matrix }}
    steps:
      - id: set-matrix
        run: |
          matrix=$(yq eval -o=json repos.yaml | jq -c)
          echo "matrix=$matrix" >> $GITHUB_OUTPUT
          
  sync:
    needs: load-config
    strategy:
      matrix: ${{ fromJson(needs.load-config.outputs.matrix) }}
    # ... rest of workflow
```

**Benefits**:
- Add new repo = edit one YAML file
- Generate workflow from config automatically
- Version-controlled repo metadata

**Effort**: Medium (3-4 days)
**Impact**: High

---

### 2.2 Workflow Orchestration Tool

**Problem**: Complex logic embedded in bash scripts

**Solution**: Use a proper orchestration tool

**Option A: Dagger** (Recommended)
```go
// ci/main.go
package main

import (
    "dagger.io/dagger"
)

type CI struct{}

func (m *CI) GenerateCI(ctx context.Context, repos []string) error {
    // Clone repos in parallel
    var eg errgroup.Group
    for _, repo := range repos {
        repo := repo
        eg.Go(func() error {
            return m.cloneRepo(ctx, repo)
        })
    }
    if err := eg.Wait(); err != nil {
        return err
    }
    
    // Generate configs
    return m.generateConfigs(ctx)
}
```

**Option B: Earthly**
```earthfile
# Earthfile
VERSION 0.7

generate-ci:
    FROM golang:1.26
    COPY . .
    RUN go run cmd/prowgen/main.go --config config/
    SAVE ARTIFACT openshift/release AS LOCAL ./output/release

sync-repo:
    ARG repo
    ARG branch
    FROM alpine/git
    RUN git clone https://github.com/openshift-knative/${repo}
    # ... sync logic
```

**Benefits**:
- Reproducible locally and in CI
- Better error handling
- Language-native tools (Go)
- Easier debugging

**Effort**: High (1-2 weeks)
**Impact**: Very High

---

### 2.3 Modularize Go Tools

**Problem**: Tools tightly coupled, hard to test

**Solution**: Refactor with clean architecture

```
pkg/
├── generator/
│   ├── prow.go         # Prow config generation
│   ├── konflux.go      # Konflux manifest generation  
│   └── dockerfile.go   # Dockerfile generation
├── discovery/
│   └── branches.go     # Branch discovery logic
├── sync/
│   ├── pr.go           # PR creation logic
│   └── git.go          # Git operations
└── config/
    └── loader.go       # Config file parsing

cmd/
├── prowgen/            # CLI wrapper for generator.Prow
├── discover/           # CLI wrapper for discovery
└── sync/               # CLI wrapper for sync
```

**Benefits**:
- Unit testable
- Reusable across tools
- Clear separation of concerns

**Effort**: High (2 weeks)
**Impact**: Medium-High

---

## Phase 3: Advanced Optimizations (1-2 months)

### 3.1 Incremental Updates

**Problem**: Regenerates ALL configs even if nothing changed

**Solution**: Track changes and only update what's needed

```go
// pkg/generator/incremental.go
type ChangeDetector struct {
    previousState map[string]string  // hash of previous configs
}

func (cd *ChangeDetector) HasChanged(repo, branch string, newConfig []byte) bool {
    key := fmt.Sprintf("%s/%s", repo, branch)
    newHash := sha256.Sum256(newConfig)
    
    if previousHash, exists := cd.previousState[key]; exists {
        return previousHash != hex.EncodeToString(newHash[:])
    }
    return true
}
```

**Benefits**:
- Fewer unnecessary PRs
- Faster CI runs
- Less noise for reviewers

**Effort**: Medium (4-5 days)
**Impact**: Medium

---

### 3.2 GitHub App for Better Integration

**Problem**: Using personal access tokens, rate limits

**Solution**: Create GitHub App

```yaml
# .github/app-manifest.yml
name: OpenShift Serverless CI Automation
url: https://github.com/openshift-knative/hack
default_permissions:
  contents: write
  pull_requests: write
  workflows: write
default_events:
  - push
  - pull_request
```

**Benefits**:
- Higher rate limits
- Better security (scoped permissions)
- Audit trail
- No token rotation issues

**Effort**: Medium (3-4 days)
**Impact**: Medium

---

### 3.3 Observability and Monitoring

**Problem**: Hard to know when automation fails or is slow

**Solution**: Add metrics and dashboards

```yaml
# .github/workflows/release-generate-ci.yaml
- name: Report metrics
  if: always()
  run: |
    # Send to monitoring system
    curl -X POST https://metrics.example.com/ci \
      -d "workflow=generate-ci" \
      -d "duration=${{ steps.generate.outputs.duration }}" \
      -d "status=${{ job.status }}"
```

Create dashboard showing:
- Workflow execution times
- Success/failure rates
- PR merge times
- Config drift detection

**Benefits**:
- Proactive issue detection
- Performance tracking
- Better incident response

**Effort**: Medium (1 week)
**Impact**: Medium

---

### 3.4 Web UI for Configuration Management

**Problem**: Editing YAML files manually is error-prone

**Solution**: Simple web UI for managing configurations

```
┌─────────────────────────────────────────┐
│  OpenShift Serverless CI Dashboard      │
├─────────────────────────────────────────┤
│  Repositories                           │
│  ├── [+] Add Repository                 │
│  ├── client                             │
│  │   ├── Branches: main, release-v1.17  │
│  │   └── Configs: konflux, owners       │
│  ├── eventing                           │
│  │   └── ...                            │
│  └── ...                                │
│                                         │
│  Recent Workflow Runs                   │
│  ├── ✓ generate-ci (2 min ago)         │
│  ├── ✓ discover-branches (1 day ago)   │
│  └── ✗ apply-konflux (2 days ago)      │
│                                         │
│  Pending PRs                            │
│  ├── #1234 [main] Update Konflux...    │
│  └── #1235 [release-1.37] Update...    │
└─────────────────────────────────────────┘
```

Tech stack:
- Backend: Simple Go HTTP server reading YAML configs
- Frontend: React or Vue.js
- Hosted: GitHub Pages or simple container

**Benefits**:
- Visual overview of automation state
- Easier for non-experts to make changes
- Validation before committing

**Effort**: High (2-3 weeks)
**Impact**: Medium

---

## Phase 4: Next-Generation Architecture (3-6 months)

### 4.1 Event-Driven Architecture

**Problem**: Scheduled workflows waste resources

**Solution**: Trigger on actual events

```yaml
# .github/workflows/on-new-branch.yaml
name: On New Branch Created
on:
  push:
    branches:
      - 'release-*'

jobs:
  auto-configure:
    runs-on: ubuntu-latest
    steps:
      - name: Detect new branch
        run: |
          # Check if branch is new
          # Auto-generate configs
          # Create PR to hack repo
```

Use webhooks to listen for:
- New branch creation → auto-discover and configure
- PR merge → trigger downstream updates
- Release published → trigger release automation

**Benefits**:
- Faster response times
- Lower resource usage
- Self-healing automation

**Effort**: High (2-3 weeks)
**Impact**: High

---

### 4.2 Multi-Repo Monorepo Management

**Problem**: Managing 13+ separate repos is complex

**Solution**: Consider migration strategy to monorepo or better tooling

**Option A**: Keep separate repos but use better tooling
- [Turborepo](https://turbo.build/) for task orchestration
- Bazel for incremental builds
- Better dependency management

**Option B**: Gradual monorepo migration
- Start with shared libraries
- Migrate related components
- Keep release process separate

**Benefits**:
- Atomic changes across components
- Easier dependency management
- Better CI/CD efficiency

**Effort**: Very High (3+ months)
**Impact**: Very High
**Risk**: High (major architectural change)

---

### 4.3 AI-Powered Configuration Generation

**Problem**: Manual configuration is tedious

**Solution**: Use AI to suggest or generate configs

```go
// Example: AI suggests test patterns based on code changes
func (ai *ConfigAI) SuggestTests(repo string, changes []FileDiff) []TestConfig {
    prompt := fmt.Sprintf(`
        Repository: %s
        Changed files: %v
        
        Suggest appropriate CI test configurations.
    `, repo, changes)
    
    return ai.llm.Generate(prompt)
}
```

**Benefits**:
- Smarter automation
- Reduced manual work
- Better test coverage

**Effort**: High (1-2 months)
**Impact**: Medium
**Maturity**: Experimental

---

## Implementation Priority Matrix

```
High Impact    │
              │  1.1 Matrix     2.1 Config    4.1 Event
              │  Strategy       Driven        Driven
              │                               
              │  1.2 Reusable   2.2 Dagger   
              │  Actions        Orchestration
              │                 
              │  1.3 Caching    2.3 Modular
              │                 Tools
              │                                3.2 GitHub
Medium Impact │                                App
              │  
              │                 3.1 Incremental  3.3 Observability
              │                                   3.4 Web UI
              │
Low Impact    │
              │
              └─────────────────────────────────────────
                Low Effort      Medium Effort    High Effort
```

**Start with**: 1.1, 1.2, 1.3 (Quick wins, high impact)
**Then move to**: 2.1, 2.2 (Structural improvements)
**Consider later**: Phase 3 and 4 items based on team bandwidth

---

## Success Metrics

Track these to measure improvement:

1. **Speed**
   - Workflow execution time: Target 50% reduction
   - Time to add new repo: Target < 5 minutes

2. **Reliability**
   - Success rate: Target > 95%
   - Mean time to recovery: Target < 1 hour

3. **Maintainability**
   - Lines of workflow YAML: Target < 500
   - Time to onboard new team member: Target < 1 day

4. **Developer Experience**
   - PR creation time: Target < 2 minutes
   - Config update time: Target < 5 minutes

---

## Risks and Mitigations

| Risk | Impact | Mitigation |
|------|--------|------------|
| Breaking existing workflows | High | Extensive testing, gradual rollout, feature flags |
| Team learning curve | Medium | Documentation, training, pair programming |
| Migration takes too long | Medium | Incremental approach, parallel systems during transition |
| New tools have bugs | Medium | Thorough testing, rollback plan |
| Costs increase | Low | Monitor resource usage, optimize caching |

---

## Next Steps

1. **Week 1**: Review this roadmap with team, prioritize phases
2. **Week 2-3**: Implement Phase 1.1 (Matrix Strategy) in a test workflow
3. **Week 4**: Validate improvements, measure metrics
4. **Month 2**: Roll out to production, implement 1.2 and 1.3
5. **Month 3+**: Start Phase 2

---

## Questions to Discuss

1. What are the biggest pain points you experience today?
2. Which improvements would have the most immediate impact?
3. What's the team's bandwidth for this work?
4. Are there any constraints (security, compliance, etc.) I should know about?
5. What's the preferred timeline - quick fixes or comprehensive overhaul?

---

## Resources

- [GitHub Actions Best Practices](https://docs.github.com/en/actions/learn-github-actions/usage-limits-billing-and-administration)
- [Dagger.io Documentation](https://docs.dagger.io)
- [Earthly Documentation](https://docs.earthly.dev)
- [Monorepo Tools Comparison](https://monorepo.tools)
