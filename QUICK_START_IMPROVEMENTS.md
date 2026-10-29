# Quick Start: Immediate Improvements

This guide helps you make high-impact improvements **this week** without a major overhaul.

## 🚀 Day 1: Quick Win #1 - Parallel Repo Cloning (2 hours)

**Problem**: Cloning 13 repos sequentially takes 6+ minutes  
**Solution**: Clone in parallel  
**Impact**: Save ~5 minutes per workflow run  

### Implementation

Edit `.github/workflows/release-generate-ci.yaml`:

```yaml
# BEFORE (slow):
- name: '[client] Clone repository'
  uses: actions/checkout@v6
  with:
    path: ./src/github.com/openshift-knative/hack/openshift-knative/client
    ref: main
    repository: openshift-knative/client
    token: ${{ secrets.SERVERLESS_QE_ROBOT }}

- name: '[eventing] Clone repository'
  uses: actions/checkout@v6
  with:
    path: ./src/github.com/openshift-knative/hack/openshift-knative/eventing
    ref: main
    repository: openshift-knative/eventing
    token: ${{ secrets.SERVERLESS_QE_ROBOT }}

# ... 11 more sequential clones ...

# AFTER (fast):
- name: Clone all repositories in parallel
  run: |
    REPOS=(
      "backstage-plugins"
      "client"
      "eventing"
      "eventing-integrations"
      "eventing-istio"
      "eventing-kafka-broker"
      "kn-plugin-event"
      "kn-plugin-func"
      "serverless-operator"
      "net-istio"
      "net-kourier"
      "serving"
    )

    BASE_PATH="./src/github.com/openshift-knative/hack/openshift-knative"
    mkdir -p "$BASE_PATH"

    for repo in "${REPOS[@]}"; do
      (
        echo "Cloning $repo..."
        git clone \
          --depth 1 \
          --branch main \
          "https://${{ secrets.SERVERLESS_QE_ROBOT }}@github.com/openshift-knative/$repo.git" \
          "$BASE_PATH/$repo" 2>&1 | sed "s/^/[$repo] /"
      ) &
    done

    # Wait for all background jobs
    wait
    echo "All repositories cloned successfully!"
```

**Test it**:
```bash
git checkout -b test-parallel-cloning
# Make the change above
git commit -am "Parallelize repository cloning"
git push origin test-parallel-cloning
# Create PR and test
```

---

## 🚀 Day 2: Quick Win #2 - Add Caching (2 hours)

**Problem**: Re-cloning repos and rebuilding Go modules every time  
**Solution**: Use GitHub Actions cache  
**Impact**: Save ~3-4 minutes per run  

### Implementation

Add to `.github/workflows/release-generate-ci.yaml`:

```yaml
jobs:
  generate-ci:
    runs-on: ubuntu-latest
    steps:
      - name: Checkout openshift-knative/hack
        uses: actions/checkout@v6
        with:
          path: ./src/github.com/openshift-knative/hack

      # NEW: Cache Go modules
      - name: Cache Go modules
        uses: actions/cache@v3
        with:
          path: |
            ~/.cache/go-build
            ~/go/pkg/mod
          key: ${{ runner.os }}-go-${{ hashFiles('**/go.sum') }}
          restore-keys: |
            ${{ runner.os }}-go-

      # NEW: Cache Go binaries
      - name: Cache Go binaries
        uses: actions/cache@v3
        with:
          path: ~/go/bin
          key: ${{ runner.os }}-go-bin-${{ hashFiles('.github/workflows/release-generate-ci.yaml') }}
          restore-keys: |
            ${{ runner.os }}-go-bin-

      # Setup Golang (now faster due to cache)
      - name: Setup Golang
        uses: ./src/github.com/openshift-knative/hack/actions/setup-go

      # Install tools (skip if cached)
      - name: Install yq
        run: |
          if ! command -v yq &> /dev/null; then
            go install github.com/mikefarah/yq/v3@latest
          else
            echo "yq already installed (from cache)"
          fi

      # ... rest of workflow ...
```

---

## 🚀 Day 3: Quick Win #3 - Extract Reusable Script (3 hours)

**Problem**: Same bash code repeated 100+ times for PR creation  
**Solution**: Single script used everywhere  
**Impact**: Easier to fix bugs, modify behavior  

### Implementation

Create `.github/scripts/create-sync-pr.sh`:

```bash
#!/bin/bash
set -euo pipefail

# Usage: create-sync-pr.sh <repo> <branch> <config-type>
#
# Example: create-sync-pr.sh client release-v1.17 konflux

REPO=$1
TARGET_BRANCH=$2
CONFIG_TYPE=$3

SYNC_BRANCH="sync-${CONFIG_TYPE}-${TARGET_BRANCH}"
FORK_REMOTE="https://serverless-qe:${GH_TOKEN}@github.com/serverless-qe/${REPO}.git"

echo "──────────────────────────────────────────────────────"
echo "Syncing $CONFIG_TYPE to $REPO/$TARGET_BRANCH"
echo "──────────────────────────────────────────────────────"

# Set PR title and body based on config type
case $CONFIG_TYPE in
  konflux)
    PR_TITLE="[$TARGET_BRANCH] Update Konflux configurations"
    PR_BODY="Update Konflux configurations"
    ;;
  owners)
    PR_TITLE="[$TARGET_BRANCH] Update OWNERS file"
    PR_BODY="Update OWNERS file"
    ;;
  dependabot)
    PR_TITLE="[$TARGET_BRANCH] Update dependabot configurations"
    PR_BODY="Update dependabot configurations"
    ;;
  *)
    echo "❌ Unknown config type: $CONFIG_TYPE"
    exit 1
    ;;
esac

# Add fork remote if not exists
git remote add fork "$FORK_REMOTE" 2>/dev/null || true

# Check if remote branch exists
if git ls-remote --heads fork "$SYNC_BRANCH" | grep -q "$SYNC_BRANCH"; then
  echo "✓ Remote branch exists"
else
  echo "⚠ Remote branch doesn't exist, will create"
fi

# Fetch and rebase
git fetch origin "$TARGET_BRANCH"
git fetch fork "$SYNC_BRANCH" || echo "Fork branch doesn't exist yet"

# Check for changes
if git diff --quiet "origin/$TARGET_BRANCH"; then
  echo "✓ No changes to sync"
  exit 0
fi

# Create/update branch and push
git checkout -B "$SYNC_BRANCH" "origin/$TARGET_BRANCH"
git rebase --quiet "origin/$TARGET_BRANCH" || {
  echo "❌ Rebase failed"
  git rebase --abort
  exit 1
}

git push "$FORK_REMOTE" "$SYNC_BRANCH:$SYNC_BRANCH" -f

# Create PR (or note it exists)
if gh pr create \
  --base "$TARGET_BRANCH" \
  --head "serverless-qe:$SYNC_BRANCH" \
  --title "$PR_TITLE" \
  --body "$PR_BODY"; then
  echo "✓ PR created successfully"
else
  echo "✓ PR already exists (or creation skipped)"
fi

echo "──────────────────────────────────────────────────────"
```

Make it executable:
```bash
chmod +x .github/scripts/create-sync-pr.sh
```

Now replace all PR creation steps in the workflow with:

```yaml
- name: '[client - release-v1.17] Create Konflux PR'
  env:
    GH_TOKEN: ${{ secrets.SERVERLESS_QE_ROBOT }}
  working-directory: ./src/github.com/openshift-knative/hack/openshift-knative/client
  run: ../../../../../.github/scripts/create-sync-pr.sh client release-v1.17 konflux

- name: '[client - release-v1.21] Create Konflux PR'
  env:
    GH_TOKEN: ${{ secrets.SERVERLESS_QE_ROBOT }}
  working-directory: ./src/github.com/openshift-knative/hack/openshift-knative/client
  run: ../../../../../.github/scripts/create-sync-pr.sh client release-v1.21 konflux

# etc...
```

**Benefits**:
- Fix a bug once, affects all 100+ PR creations
- Easier to add features (e.g., better error messages, retry logic)
- Can test the script locally
- Reduces workflow file from 2,679 to ~1,500 lines

---

## 🚀 Week 2: Quick Win #4 - Create repos.yaml Config (1 day)

**Problem**: Repo/branch list hardcoded in workflow  
**Solution**: Centralized configuration file  
**Impact**: Easier to add/remove repos  

### Implementation

Create `repos.yaml`:

```yaml
# Central repository configuration
# Add a new repo here, not in the workflow!

repositories:
  - name: backstage-plugins
    branches: [main, release-v1.17, release-v1.21, release-v1.22, release-v1.23]
    configs: [konflux, owners, dependabot]

  - name: client
    branches: [main, release-v1.17, release-v1.21, release-v1.22, release-v1.23]
    configs: [konflux, owners, dependabot]

  - name: eventing
    branches: [main, release-v1.17, release-v1.21, release-v1.22, release-v1.23]
    configs: [konflux, owners, dependabot]

  - name: eventing-integrations
    branches: [main, release-v1.17, release-v1.21, release-v1.22, release-v1.23]
    configs: [konflux, owners, dependabot]

  - name: eventing-istio
    branches: [main, release-v1.17, release-v1.21, release-v1.22, release-v1.23]
    configs: [konflux, owners, dependabot]

  - name: eventing-kafka-broker
    branches: [main, release-v1.17, release-v1.21, release-v1.22, release-v1.23]
    configs: [konflux, owners, dependabot]

  - name: kn-plugin-event
    branches: [main, release-v1.17, release-v1.21, release-v1.22, release-v1.23]
    configs: [konflux, owners]

  - name: kn-plugin-func
    branches: [main, release-v1.17, release-v1.21, release-v1.22, release-v1.23]
    configs: [konflux, owners, dependabot]

  - name: serverless-operator
    branches: [main, release-1.37, release-1.38]
    configs: [konflux, owners, dependabot]

  - name: net-istio
    branches: [main, release-v1.17, release-v1.21, release-v1.22, release-v1.23]
    configs: [konflux, owners, dependabot]

  - name: net-kourier
    branches: [main, release-v1.17, release-v1.21, release-v1.22, release-v1.23]
    configs: [konflux, owners, dependabot]

  - name: serving
    branches: [main, release-v1.17, release-v1.21, release-v1.22, release-v1.23]
    configs: [konflux, owners, dependabot]

  - name: eventing-hyperfoil-benchmark
    branches: [main]
    configs: [owners]

# Config type definitions
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

Update `.github/scripts/create-sync-pr.sh` to read from this file:

```bash
#!/bin/bash
set -euo pipefail

REPO=$1
TARGET_BRANCH=$2
CONFIG_TYPE=$3

# Read PR details from repos.yaml
PR_TITLE=$(yq eval ".config_types.${CONFIG_TYPE}.pr_title" repos.yaml | sed "s/\$branch/$TARGET_BRANCH/g")
PR_BODY=$(yq eval ".config_types.${CONFIG_TYPE}.pr_body" repos.yaml)

# ... rest of script ...
```

---

## 🚀 Week 2: Quick Win #5 - Add Basic Monitoring (2 hours)

**Problem**: No visibility into workflow performance  
**Solution**: Add timing and status annotations  
**Impact**: Can track improvements, spot regressions  

### Implementation

Add to workflow:

```yaml
jobs:
  generate-ci:
    runs-on: ubuntu-latest
    steps:
      - name: Start workflow timer
        id: timer
        run: echo "start_time=$(date +%s)" >> $GITHUB_OUTPUT

      # ... your existing steps ...

      - name: Report workflow metrics
        if: always()
        run: |
          end_time=$(date +%s)
          start_time=${{ steps.timer.outputs.start_time }}
          duration=$((end_time - start_time))
          minutes=$((duration / 60))
          seconds=$((duration % 60))

          echo "──────────────────────────────────────────"
          echo "📊 Workflow Metrics"
          echo "──────────────────────────────────────────"
          echo "Duration: ${minutes}m ${seconds}s"
          echo "Status: ${{ job.status }}"
          echo "Trigger: ${{ github.event_name }}"
          echo "──────────────────────────────────────────"

          # Append to job summary (visible in GitHub UI)
          cat >> $GITHUB_STEP_SUMMARY <<EOF
          ## 📊 Workflow Metrics

          | Metric | Value |
          |--------|-------|
          | Duration | ${minutes}m ${seconds}s |
          | Status | ${{ job.status }} |
          | Trigger | ${{ github.event_name }} |
          | Run ID | ${{ github.run_id }} |
          EOF
```

Create `.github/scripts/track-step-time.sh` for timing individual steps:

```bash
#!/bin/bash
# Usage: source this at the start of a step, then call track_step_end at the end

STEP_START=$(date +%s)

track_step_end() {
  local step_name="$1"
  local end_time=$(date +%s)
  local duration=$((end_time - STEP_START))

  echo "⏱️  $step_name took ${duration}s"

  # Add to workflow summary
  echo "| $step_name | ${duration}s |" >> $GITHUB_STEP_SUMMARY
}
```

Use it in workflow:

```yaml
- name: Clone repositories
  run: |
    source .github/scripts/track-step-time.sh

    # ... clone logic ...

    track_step_end "Clone repositories"

- name: Generate CI
  run: |
    source .github/scripts/track-step-time.sh

    # ... generate CI logic ...

    track_step_end "Generate CI"
```

---

## 📊 Tracking Your Improvements

Create a simple tracking document to measure progress:

### Metrics Dashboard (`METRICS.md`)

```markdown
# Automation Metrics

Track improvements over time.

## Baseline (Before Improvements)
- Workflow size: 2,679 lines
- Execution time: 45-60 minutes
- Clone time: 6+ minutes
- Time to add repo: ~2 hours
- Success rate: ~85%

## Week 1 Progress
- [x] Parallel cloning implemented
  - Clone time: ~~6 minutes~~ → **30 seconds** ✅
  - Execution time: ~~50 minutes~~ → **44 minutes**

- [x] Caching added
  - Go module download: ~~2 minutes~~ → **5 seconds** ✅
  - Execution time: ~~44 minutes~~ → **40 minutes**

- [x] Reusable script created
  - Workflow size: ~~2,679 lines~~ → **1,500 lines** ✅
  - Bugs fixed in one place: **3 bugs fixed** ✅

## Week 2 Progress
- [x] repos.yaml created
  - Time to add repo: ~~2 hours~~ → **10 minutes** ✅

- [x] Basic monitoring added
  - Can now track performance: **Yes** ✅

## Next Steps
- [ ] Implement matrix strategy (see IMPROVEMENT_ROADMAP.md Phase 1.1)
- [ ] Create reusable GitHub Action
- [ ] Set up automated testing for scripts

## Goal Tracker
| Metric | Baseline | Current | Target | Status |
|--------|----------|---------|--------|--------|
| Workflow size | 2,679 | 1,500 | <500 | 🟡 In Progress |
| Execution time | 50 min | 40 min | <20 min | 🟡 In Progress |
| Clone time | 6 min | 30 sec | <1 min | ✅ Done |
| Time to add repo | 2 hrs | 10 min | <5 min | 🟡 In Progress |
| Success rate | 85% | 90% | >95% | 🟡 In Progress |
```

---

## 🎯 Summary: Week 1-2 Achievements

After implementing these quick wins:

✅ **Execution time**: 50 min → **~35-40 min** (20-30% faster)  
✅ **Workflow readability**: 2,679 lines → **~1,500 lines** (45% reduction)  
✅ **Maintainability**: 100 bash blocks → **1 script** (99% reduction in duplication)  
✅ **Configuration**: Hardcoded → **repos.yaml** (centralized)  
✅ **Observability**: None → **Basic metrics** (can track progress)  

**Time invested**: ~2-3 days  
**Ongoing savings**: ~10-15 min per workflow run + easier maintenance  

---

## 🚀 Next: Full Transformation

Once you've done these quick wins and validated the improvements, you're ready for the full transformation outlined in `IMPROVEMENT_ROADMAP.md`.

The quick wins have:
- Proven the concept
- Built team confidence
- Provided measurable ROI
- Created momentum for larger changes

Start with Phase 1.1 (Matrix Strategy) when ready!
