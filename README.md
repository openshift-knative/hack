# hack

CI tooling and hacks to improve CI

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
- Individual OpenShift versions can specify `generateCustomConfigs: true`.
  The repository configuration should then list custom configurations under `customConfigs`.
  The `releaseBuildConfiguration` key should include at least `tests` key
  with the list of tests to be run. For custom configurations, tests are not generated from Makefile
  targets but rather taken directly from the configuration. The resulting build configuration is
  then
  enriched with images, base images, and dependencies for test steps.

Limitations:

- It is not currently possible to disable periodics per job

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
GO111MODULE=off go get -u github.com/openshift-knative/hack/cmd/sobranch

so_branch=$( $(go env GOPATH)/bin/sobranch --upstream-version "release-1.11") # or "release-v1.11" or "release-1.11" or "v1.11" or "1.11"

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


