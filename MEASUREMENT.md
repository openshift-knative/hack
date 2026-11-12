# release-generate-ci cache measurement

Scratch notes for measuring the generation cache introduced in PR #1086.
This file lives outside the generation hash paths (`config/`, `pkg/`, `cmd/`,
`go.mod`, `go.sum`), so commits touching only this file do not invalidate
the cache.

## Run 1 — cold (cache miss)

- Run: https://github.com/openshift-knative/hack/actions/runs/43277443698
- Generate CI: 23m 27s
- Total run: 27m 17s
- Cache: restored nothing (miss), saved successfully in the post step

## Run 2 — warm (docs-only commit, generation hash unchanged)

- Pending — recorded on the PR after completion.

- Run 2 note: warm run at 18:41 did not skip (under investigation); re-recorded state from full regeneration.
