# hack

CI tooling and hacks to improve CI

## Tools

| Tool | Description                                                                                                                                                                       |
|------|-----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| [prowgen](cmd/prowgen) | Generates OpenShift CI (Prow) configurations from repository config YAML files. Clones repos, parses Makefiles, and creates presubmit/periodic test jobs for `openshift/release`. Used by the [release-generate-ci](#ci-workflows) workflow. |
| [prowcopy](cmd/prowcopy) | Copies and adapts Prow CI configuration from one branch to another in `openshift/release`. Useful when cutting new release branches. |
| [discover](cmd/discover) | Discovers new release branches across repositories and updates config files automatically. Also manages Konflux resources for unsupported versions. Used by the [release-discover-branches](#ci-workflows) workflow. |
| [generate](cmd/generate) | Generates Dockerfiles and build artifacts from project metadata. Supports regular, must-gather, test, and source image Dockerfiles. |
| [generate-ci-action](cmd/generate-ci-action) | Generates GitHub Actions CI workflow files by populating a template with steps for each configured repository. |
| [konflux-apply](cmd/konflux-apply) | Applies Konflux manifests (applications, components, ...) to a Konflux instance. See its [README](cmd/konflux-apply/README.md) for service account setup. Used by the [apply-konflux-manifests](#ci-workflows) workflow. |
| [konflux-gen](cmd/konflux-gen) | Generates Konflux application and component manifests from `openshift/release` CI configs. See its [README](cmd/konflux-gen/README.md) for usage. |
| [konflux-release-gen](cmd/konflux-release-gen) | Generates Konflux release CRs (ReleasePlans, ReleasePlanAdmissions) for Serverless Operator releases. Used by the [generate-release-crs](#ci-workflows) workflow. |
| [sobranch](cmd/sobranch) | Maps upstream Knative version numbers to Serverless Operator release branch names (e.g. `1.11` → `release-1.32`). |
| [sorhel](cmd/sorhel) | Maps Serverless Operator versions to compatible RHEL versions. |
| [testselect](cmd/testselect) | Determines which test suites to run based on changed files in a PR, using regex patterns from a testsuites YAML config. |

## CI Workflows

GitHub Actions workflows in [`.github/workflows/`](.github/workflows/):

| Workflow | Trigger | Description                                                                                                                                                                                                                                                                                                                                                                                                                                    |
|----------|---------|------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| [apply-konflux-manifests](.github/workflows/apply-konflux-manifests.yaml) | Daily (06:00 UTC), push to `.konflux/**`, manual | Applies Konflux manifests from all repos (configured in [./config](./config) to the Konflux cluster using the `konflux-apply` tool.                                                                                                                                                                                                                                                                                                            |
| [generate-release-crs](.github/workflows/generate-release-crs.yaml) | Manual | Generates Konflux release CRs using `konflux-release-gen`, applies override snapshots, and creates PRs in hack repos containing the component and FBC release CRs. Merging those PRs will trigger a Konflux release pipeline. <br /> ⚠️ It is important to merge the component PR first and let the release pipeline succeed before merging the FBC release PR, as the FBC references the components and thus needs them to be released first. |
| [release-discover-branches](.github/workflows/release-discover-branches.yaml) | Daily (05:00 UTC), push to main, PRs, manual | Runs `discover` to find new release branches and creates a PR to update the [configs](./config) with the new branches.                                                                                                                                                                                                                                                                                                                         |
| [release-generate-ci](.github/workflows/release-generate-ci.yaml) | Weekly (Monday 06:00 UTC), push to main, PRs, manual | Generates CI configurations for all tracked repositories using `prowgen`, updates dependabot and OWNERS files, updates the Konflux pipelines and creates sync PRs.                                                                                                                                                                                                                                                                             |

## Generate openshift/release config

- Add configuration for your repository in `config/<file.yaml>`
    - If you're adding a new file in `config/` directory, add the new file to the `make generate-ci`
      command
- Run `make generate-ci ARGS=--remote=<your_remote>`
    - For example, `make generate-ci ARGS=--remote=git@github.com:pierDipi/release.git`
    - If you are using `podman`, make sure to have `export CONTAINER_ENGINE=podman` set
- Create a PR to [https://github.com/openshift/release](https://github.com/openshift/release) (to be
  automated)

To generate openshift/release config for a single repository, run the specific task in
the `Makefile` or run individual commands in the `Makefile`, such as:

```shell
go run github.com/openshift-knative/hack/cmd/prowgen --config config/eventing-hyperfoil-benchmark.yaml --remote <your_remote>
# go run github.com/openshift-knative/hack/cmd/prowgen --config config/eventing-hyperfoil-benchmark.yaml --remote git@github.com:aliok/release.git 
```

This generation works this way:

- `openshift/relase` is cloned
- The target repository is cloned
- The makefile of the target repository is parsed to find the make targets that match the regex in
  the `config/<file.yaml>` files
- For any matches, 2 `test`s are generated.
    - One for the presubmit (that runs on PRs on the target repository)
    - One for the periodics (that runs regularly)
- There are also CI job config generated, which use the tests above.
- If the matching regex is specified in `onDemand` field, then the presubmit is marked as
  optional (`always_run: false`).
- Individual OpenShift versions can enable custom configurations via `customConfigs`:
  ```yaml
  customConfigs:
    enabled: true
    includes:
    - ".*ocp-4.22-lp-interop.*"
    excludes:
    - ".*some-pattern.*"
  ```
  The repository configuration should then list custom configurations under `customConfigs`.
  The `releaseBuildConfiguration` key should include at least `tests` key
  with the list of tests to be run. For custom configurations, tests are not generated from Makefile
  targets but rather taken directly from the configuration. The resulting build configuration is
  then enriched with images, base images, and dependencies for test steps.

- Periodic jobs can be disabled per test or per OpenShift version using `skipCron: true`:
  ```yaml
  # Per test:
  e2eTests:
  - match: "^test-e2e$"
    skipCron: true

  # Per OpenShift version:
  openShiftVersions:
  - version: "4.22"
    skipCron: true
  ```

### Additional Makefile targets

- `make discover-branches` — Run `discover` to detect new release branches and update configs automatically.
- `make generate-ci-action` — Regenerate the `.github/workflows/release-generate-ci.yaml` workflow from the template.
- `make generate-konflux-release` — Generate Konflux release CRs using `konflux-release-gen`.
- `make konflux-update-pipelines` — Pull latest Konflux Tekton pipeline bundles and rebuild local pipeline YAMLs via kustomize.
- `make test-select` — Run `testselect` to determine which tests to run. Requires `TESTSUITES` and `CLONEREFS` variables.

## Apply Konflux configurations

1. Follow the instructions to access the Konflux instance
   at [GitLab (VPN required)](https://gitlab.cee.redhat.com/konflux/docs/users/-/blob/main/topics/getting-started/getting-access.md#accessing-konflux-via-cli)
2. Set the `kubectl` context to use the `ocp-serverless` workspace
    ```shell
    kubectl config use-context konflux-ocp-serverless
    ```
3. Run `make konflux-apply`

## Run unit tests

```shell
make unit-tests
```

## Updating OpenShift versions

CI configs use specific OpenShift versions. To change the version, you need to update the YAML files
in the `config/` directory.

When a new OpenShift version is released, wait until the cluster pool for OpenShift CI is available
in
[https://docs.ci.openshift.org/docs/how-tos/cluster-claim/#existing-cluster-pools](https://docs.ci.openshift.org/docs/how-tos/cluster-claim/#existing-cluster-pools).

## Getting SO branch associated with upstream branch

SO branch follows the product versioning, while midstream branches follows the upstream versioning.

To make the "clone associated SO branch" easier, you can run the `sobranch` tool as follows:

```shell
go install github.com/openshift-knative/hack/cmd/sobranch@latest

so_branch=$(sobranch --upstream-version "release-1.11") # or "release-v1.11" or "release-1.11" or "v1.11" or "1.11"

git clone --branch $so_branch git@github.com:openshift-knative/serverless-operator.git
```

## Troubleshooting

#### Git not configured to use GPG signing

```
error: gpg failed to sign the data
fatal: failed to write commit object
```

You need to configure GPG signing in your git config.
See [https://docs.github.com/en/authentication/managing-commit-signature-verification/telling-git-about-your-signing-key#telling-git-about-your-ssh-key](https://docs.github.com/en/authentication/managing-commit-signature-verification/telling-git-about-your-signing-key#telling-git-about-your-ssh-key).

If you see this error, you need to update your Git to version 2.34 or later.

```
error: unsupported value for gpg.format: ssh
fatal: bad config variable 'gpg.format' in file '/Users/<you>/.gitconfig'
```


