# Konflux Gen

A CLI to get started
using [Konflux](https://redhat-appstudio.github.io/docs.appstudio.io/Documentation/main/).

## Usage

### Generate applications and components

```shell
# Clone openshift/release repository in openshift-release
git clone git@github.com:openshift/release.git openshift-release
konflux-gen --openshift-release-path openshift-release --includes "ci-operator/config/<org>/.*.yaml" --output "$(pwd)/<org>"
```

Example command:

```shell
go run ./cmd/konflux-gen/main.go --openshift-release-path openshift-release \
  --application-name "serverless-operator release-1.32" \
  --includes "ci-operator/config/openshift-knative/.*v1.11.*.yaml" \
  --includes "ci-operator/config/openshift-knative/serverless-operator/.*1.32.*.yaml" \
  --exclude-images ".*source.*" \
  --exclude-images ".*test.*" \
  --output konflux-gen/out
```