package konfluxgen

import (
	"os"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
)

func Test_replaceTaskImagesFromExisting(t *testing.T) {
	t.Parallel()

	tt := []struct {
		name     string
		existing string
		template string
		expected string
	}{
		{
			name:     "simple value",
			template: "        value: quay.io/konflux-ci/tekton-catalog/task-prefetch-dependencies-oci-ta:0.1@sha256:34a2a8b700bfdfddc4a3e6328f0f8ba29eb2de89a921e24d05c39cc6c5d05351",
			existing: "        value: quay.io/konflux-ci/tekton-catalog/task-prefetch-dependencies-oci-ta:0.1@sha256:f13f6783f73971e4d1fbe8fd7fde3ea6cc080943c3fe2a4338ce6373c43f26a7",
			expected: "        value: quay.io/konflux-ci/tekton-catalog/task-prefetch-dependencies-oci-ta:0.1@sha256:f13f6783f73971e4d1fbe8fd7fde3ea6cc080943c3fe2a4338ce6373c43f26a7",
		},
		{
			name:     "simple value, different task",
			template: "        value: quay.io/konflux-ci/tekton-catalog/task-push-dockerfile:0.1@sha256:81312124d27361cfa2d7ff09fb38a177b27b0e9b43426aa4ea9cec9f640ec42a",
			existing: "        value: quay.io/konflux-ci/tekton-catalog/task-push-dockerfile:0.1@sha256:e4abc7c7671e4455465e48f96831cfafdb4de368cbcb9f27a8e5b9b0553ac35e",
			expected: "        value: quay.io/konflux-ci/tekton-catalog/task-push-dockerfile:0.1@sha256:e4abc7c7671e4455465e48f96831cfafdb4de368cbcb9f27a8e5b9b0553ac35e",
		},
		{
			name:     "simple value, trailing whitespace",
			template: "        value: quay.io/konflux-ci/tekton-catalog/task-prefetch-dependencies-oci-ta:0.1@sha256:34a2a8b700bfdfddc4a3e6328f0f8ba29eb2de89a921e24d05c39cc6c5d05351   ",
			existing: "        value: quay.io/konflux-ci/tekton-catalog/task-prefetch-dependencies-oci-ta:0.1@sha256:f13f6783f73971e4d1fbe8fd7fde3ea6cc080943c3fe2a4338ce6373c43f26a7   ",
			expected: "        value: quay.io/konflux-ci/tekton-catalog/task-prefetch-dependencies-oci-ta:0.1@sha256:f13f6783f73971e4d1fbe8fd7fde3ea6cc080943c3fe2a4338ce6373c43f26a7   ",
		},
		{
			name: "full YAML",
			existing: `
apiVersion: tekton.dev/v1
kind: Pipeline
metadata:
  creationTimestamp: null
  labels:
    pipelines.openshift.io/runtime: generic
    pipelines.openshift.io/strategy: docker
    pipelines.openshift.io/used-by: build-cloud
  name: docker-build-oci-ta
spec:
  finally:
  - name: show-sbom
    params:
    - name: IMAGE_URL
      value: $(tasks.build-container.results.IMAGE_URL)
    taskRef:
      params:
      - name: name
        value: show-sbom
      - name: bundle
        value: quay.io/konflux-ci/tekton-catalog/task-show-sbom:0.1@sha256:9bfc6b99ef038800fe131d7b45ff3cd4da3a415dd536f7c657b3527b01c4a13b
      - name: kind
        value: task
      resolver: bundles
  params:
  - description: Source Repository URL
    name: git-url
    type: string
  - default: ""
    description: Revision of the Source Repository
    name: revision
    type: string
  - description: Fully Qualified Output Image
    name: output-image
    type: string
  - default: .
    description: Path to the source code of an application's component from where
      to build image.
    name: path-context
    type: string
  - default: Dockerfile
    description: Path to the Dockerfile inside the context specified by parameter
      path-context
    name: dockerfile
    type: string
  - default: "false"
    description: Force rebuild image
    name: rebuild
    type: string
  - default: "false"
    description: Skip checks against built image
    name: skip-checks
    type: string
  - default: "false"
    description: Execute the build with network isolation
    name: hermetic
    type: string
  - default: ""
    description: Build dependencies to be prefetched by Cachi2
    name: prefetch-input
    type: string
  - default: "false"
    description: Java build
    name: java
    type: string
  - default: ""
    description: Image tag expiration time, time values could be something like 1h,
      2d, 3w for hours, days, and weeks, respectively.
    name: image-expires-after
  - default: "false"
    description: Build a source image.
    name: build-source-image
    type: string
  - default: []
    description: Array of --build-arg values ("arg=value" strings) for buildah
    name: build-args
    type: array
  - default: ""
    description: Path to a file with build arguments for buildah, see https://www.mankier.com/1/buildah-build#--build-arg-file
    name: build-args-file
    type: string
  results:
  - description: ""
    name: IMAGE_URL
    value: $(tasks.build-container.results.IMAGE_URL)
  - description: ""
    name: IMAGE_DIGEST
    value: $(tasks.build-container.results.IMAGE_DIGEST)
  - description: ""
    name: CHAINS-GIT_URL
    value: $(tasks.clone-repository.results.url)
  - description: ""
    name: CHAINS-GIT_COMMIT
    value: $(tasks.clone-repository.results.commit)
  - description: ""
    name: JAVA_COMMUNITY_DEPENDENCIES
    value: $(tasks.build-container.results.JAVA_COMMUNITY_DEPENDENCIES)
  tasks:
  - name: init
    params:
    - name: image-url
      value: $(params.output-image)
    - name: rebuild
      value: $(params.rebuild)
    - name: skip-checks
      value: $(params.skip-checks)
    taskRef:
      params:
      - name: name
        value: init
      - name: bundle
        value: quay.io/konflux-ci/tekton-catalog/task-init:0.2@sha256:092c113b614f6551113f17605ae9cb7e822aa704d07f0e37ed209da23ce392cc
      - name: kind
        value: task
      resolver: bundles
  - name: clone-repository
    params:
    - name: url
      value: $(params.git-url)
    - name: revision
      value: $(params.revision)
    - name: ociStorage
      value: $(params.output-image).git
    - name: ociArtifactExpiresAfter
      value: $(params.image-expires-after)
    runAfter:
    - init
    taskRef:
      params:
      - name: name
        value: git-clone-oci-ta
      - name: bundle
        value: quay.io/konflux-ci/tekton-catalog/task-git-clone-oci-ta:0.1@sha256:0f4360ce144d46171ebd2e8f4d4575539a0600e02208ba5fc9beeb2c27ddfd4c
      - name: kind
        value: task
      resolver: bundles
    when:
    - input: $(tasks.init.results.build)
      operator: in
      values:
      - "true"
    workspaces:
    - name: basic-auth
      workspace: git-auth
  - name: prefetch-dependencies
    params:
    - name: input
      value: $(params.prefetch-input)
    - name: SOURCE_ARTIFACT
      value: $(tasks.clone-repository.results.SOURCE_ARTIFACT)
    - name: ociStorage
      value: $(params.output-image).prefetch
    - name: ociArtifactExpiresAfter
      value: $(params.image-expires-after)
    runAfter:
    - clone-repository
    taskRef:
      params:
      - name: name
        value: prefetch-dependencies-oci-ta
      - name: bundle
        value: quay.io/konflux-ci/tekton-catalog/task-prefetch-dependencies-oci-ta:0.1@sha256:f13f6783f73971e4d1fbe8fd7fde3ea6cc080943c3fe2a4338ce6373c43f26a7
      - name: kind
        value: task
      resolver: bundles
    when:
    - input: $(params.prefetch-input)
      operator: notin
      values:
      - ""
    workspaces:
    - name: git-basic-auth
      workspace: git-auth
    - name: netrc
      workspace: netrc
  - name: build-container
    params:
    - name: IMAGE
      value: $(params.output-image)
    - name: DOCKERFILE
      value: $(params.dockerfile)
    - name: CONTEXT
      value: $(params.path-context)
    - name: HERMETIC
      value: $(params.hermetic)
    - name: PREFETCH_INPUT
      value: $(params.prefetch-input)
    - name: IMAGE_EXPIRES_AFTER
      value: $(params.image-expires-after)
    - name: COMMIT_SHA
      value: $(tasks.clone-repository.results.commit)
    - name: BUILD_ARGS
      value:
      - $(params.build-args[*])
    - name: BUILD_ARGS_FILE
      value: $(params.build-args-file)
    - name: SOURCE_ARTIFACT
      value: $(tasks.prefetch-dependencies.results.SOURCE_ARTIFACT)
    - name: CACHI2_ARTIFACT
      value: $(tasks.prefetch-dependencies.results.CACHI2_ARTIFACT)
    runAfter:
    - prefetch-dependencies
    taskRef:
      params:
      - name: name
        value: buildah-oci-ta
      - name: bundle
        value: quay.io/konflux-ci/tekton-catalog/task-buildah-oci-ta:0.2@sha256:725e781e74463d0bb111002eec85918e53df69a5eb55a7ccd6bd260e6542a9a3
      - name: kind
        value: task
      resolver: bundles
    when:
    - input: $(tasks.init.results.build)
      operator: in
      values:
      - "true"
  - name: build-source-image
    params:
    - name: BINARY_IMAGE
      value: $(params.output-image)
    - name: SOURCE_ARTIFACT
      value: $(tasks.prefetch-dependencies.results.SOURCE_ARTIFACT)
    - name: CACHI2_ARTIFACT
      value: $(tasks.prefetch-dependencies.results.CACHI2_ARTIFACT)
    runAfter:
    - build-container
    taskRef:
      params:
      - name: name
        value: source-build-oci-ta
      - name: bundle
        value: quay.io/konflux-ci/tekton-catalog/task-source-build-oci-ta:0.1@sha256:99ee22c5e8e8a66da3d68ec5f3334e7cc59f8b8907e9d2a78f7338aa37d952eb
      - name: kind
        value: task
      resolver: bundles
    when:
    - input: $(tasks.init.results.build)
      operator: in
      values:
      - "true"
    - input: $(params.build-source-image)
      operator: in
      values:
      - "true"
  - name: deprecated-base-image-check
    params:
    - name: IMAGE_URL
      value: $(tasks.build-container.results.IMAGE_URL)
    - name: IMAGE_DIGEST
      value: $(tasks.build-container.results.IMAGE_DIGEST)
    runAfter:
    - build-container
    taskRef:
      params:
      - name: name
        value: deprecated-image-check
      - name: bundle
        value: quay.io/konflux-ci/tekton-catalog/task-deprecated-image-check:0.4@sha256:4ec00cf88c55a64eebaac0fe661cdb8dc8d3586d3a288f54400fbf2b5f06b408
      - name: kind
        value: task
      resolver: bundles
    when:
    - input: $(params.skip-checks)
      operator: in
      values:
      - "false"
  - name: clair-scan
    params:
    - name: image-digest
      value: $(tasks.build-container.results.IMAGE_DIGEST)
    - name: image-url
      value: $(tasks.build-container.results.IMAGE_URL)
    runAfter:
    - build-container
    taskRef:
      params:
      - name: name
        value: clair-scan
      - name: bundle
        value: quay.io/konflux-ci/tekton-catalog/task-clair-scan:0.1@sha256:3ac359dfc034948d59b9e8d507a8cf505a19b205c543e4c861a332c6f82a0307
      - name: kind
        value: task
      resolver: bundles
    when:
    - input: $(params.skip-checks)
      operator: in
      values:
      - "false"
  - name: ecosystem-cert-preflight-checks
    params:
    - name: image-url
      value: $(tasks.build-container.results.IMAGE_URL)
    runAfter:
    - build-container
    taskRef:
      params:
      - name: name
        value: ecosystem-cert-preflight-checks
      - name: bundle
        value: quay.io/konflux-ci/tekton-catalog/task-ecosystem-cert-preflight-checks:0.1@sha256:a4dc853e50a31272d45aef8e8bdc7dac0f0f92212fc7bf8bab67ba5917c03405
      - name: kind
        value: task
      resolver: bundles
    when:
    - input: $(params.skip-checks)
      operator: in
      values:
      - "false"
  - name: sast-snyk-check
    params:
    - name: SOURCE_ARTIFACT
      value: $(tasks.prefetch-dependencies.results.SOURCE_ARTIFACT)
    runAfter:
    - build-container
    taskRef:
      params:
      - name: name
        value: sast-snyk-check-oci-ta
      - name: bundle
        value: quay.io/konflux-ci/tekton-catalog/task-sast-snyk-check-oci-ta:0.1@sha256:a5b18d0949240e2fc919277970fc1a9c4a0b13c4dec2bdf3ef579bc502a6f3d6
      - name: kind
        value: task
      resolver: bundles
    when:
    - input: $(params.skip-checks)
      operator: in
      values:
      - "false"
  - name: clamav-scan
    params:
    - name: image-digest
      value: $(tasks.build-container.results.IMAGE_DIGEST)
    - name: image-url
      value: $(tasks.build-container.results.IMAGE_URL)
    runAfter:
    - build-container
    taskRef:
      params:
      - name: name
        value: clamav-scan
      - name: bundle
        value: quay.io/konflux-ci/tekton-catalog/task-clamav-scan:0.1@sha256:4cb5750b01759a4f3d02bb8c6869e80dcde7bd4c7f5c0a68dd18e57ea2ac676f
      - name: kind
        value: task
      resolver: bundles
    when:
    - input: $(params.skip-checks)
      operator: in
      values:
      - "false"
  - name: apply-tags
    params:
    - name: IMAGE
      value: $(tasks.build-container.results.IMAGE_URL)
    runAfter:
    - build-container
    taskRef:
      params:
      - name: name
        value: apply-tags
      - name: bundle
        value: quay.io/konflux-ci/tekton-catalog/task-apply-tags:0.1@sha256:8a8c1342bfa0b263361cbbc28036750ebc3d7c2460230b14d641170b812dddcc
      - name: kind
        value: task
      resolver: bundles
  - name: push-dockerfile
    params:
    - name: IMAGE
      value: $(tasks.build-container.results.IMAGE_URL)
    - name: IMAGE_DIGEST
      value: $(tasks.build-container.results.IMAGE_DIGEST)
    - name: DOCKERFILE
      value: $(params.dockerfile)
    - name: CONTEXT
      value: $(params.path-context)
    - name: SOURCE_ARTIFACT
      value: $(tasks.prefetch-dependencies.results.SOURCE_ARTIFACT)
    runAfter:
    - build-container
    taskRef:
      params:
      - name: name
        value: push-dockerfile-oci-ta
      - name: bundle
        value: quay.io/konflux-ci/tekton-catalog/task-push-dockerfile-oci-ta:0.1@sha256:5dbf1dc5bc060c7d501623bac63dc0b0572c099f1c432846dbd801d82b66f581
      - name: kind
        value: task
      resolver: bundles
  workspaces:
  - name: git-auth
    optional: true
  - name: netrc
    optional: true
`,
			template: `
apiVersion: tekton.dev/v1
kind: Pipeline
metadata:
  creationTimestamp: null
  labels:
    pipelines.openshift.io/runtime: generic
    pipelines.openshift.io/strategy: docker
    pipelines.openshift.io/used-by: build-cloud
  name: docker-build-oci-ta
spec:
  finally:
  - name: show-sbom
    params:
    - name: IMAGE_URL
      value: $(tasks.build-container.results.IMAGE_URL)
    taskRef:
      params:
      - name: name
        value: show-sbom
      - name: bundle
        value: quay.io/konflux-ci/tekton-catalog/task-show-sbom:0.1@sha256:9bfc6b99ef038800fe131d7b45ff3cd4da3a415dd536f7c657b3527b01c4a13b
      - name: kind
        value: task
      resolver: bundles
  params:
  - description: Source Repository URL
    name: git-url
    type: string
  - default: ""
    description: Revision of the Source Repository
    name: revision
    type: string
  - description: Fully Qualified Output Image
    name: output-image
    type: string
  - default: .
    description: Path to the source code of an application's component from where
      to build image.
    name: path-context
    type: string
  - default: Dockerfile
    description: Path to the Dockerfile inside the context specified by parameter
      path-context
    name: dockerfile
    type: string
  - default: "false"
    description: Force rebuild image
    name: rebuild
    type: string
  - default: "false"
    description: Skip checks against built image
    name: skip-checks
    type: string
  - default: "false"
    description: Execute the build with network isolation
    name: hermetic
    type: string
  - default: ""
    description: Build dependencies to be prefetched by Cachi2
    name: prefetch-input
    type: string
  - default: "false"
    description: Java build
    name: java
    type: string
  - default: ""
    description: Image tag expiration time, time values could be something like 1h,
      2d, 3w for hours, days, and weeks, respectively.
    name: image-expires-after
  - default: "false"
    description: Build a source image.
    name: build-source-image
    type: string
  - default: []
    description: Array of --build-arg values ("arg=value" strings) for buildah
    name: build-args
    type: array
  - default: ""
    description: Path to a file with build arguments for buildah, see https://www.mankier.com/1/buildah-build#--build-arg-file
    name: build-args-file
    type: string
  results:
  - description: ""
    name: IMAGE_URL
    value: $(tasks.build-container.results.IMAGE_URL)
  - description: ""
    name: IMAGE_DIGEST
    value: $(tasks.build-container.results.IMAGE_DIGEST)
  - description: ""
    name: CHAINS-GIT_URL
    value: $(tasks.clone-repository.results.url)
  - description: ""
    name: CHAINS-GIT_COMMIT
    value: $(tasks.clone-repository.results.commit)
  - description: ""
    name: JAVA_COMMUNITY_DEPENDENCIES
    value: $(tasks.build-container.results.JAVA_COMMUNITY_DEPENDENCIES)
  tasks:
  - name: init
    params:
    - name: image-url
      value: $(params.output-image)
    - name: rebuild
      value: $(params.rebuild)
    - name: skip-checks
      value: $(params.skip-checks)
    taskRef:
      params:
      - name: name
        value: init
      - name: bundle
        value: quay.io/konflux-ci/tekton-catalog/task-init:0.2@sha256:092c113b614f6551113f17605ae9cb7e822aa704d07f0e37ed209da23ce392cc
      - name: kind
        value: task
      resolver: bundles
  - name: clone-repository
    params:
    - name: url
      value: $(params.git-url)
    - name: revision
      value: $(params.revision)
    - name: ociStorage
      value: $(params.output-image).git
    - name: ociArtifactExpiresAfter
      value: $(params.image-expires-after)
    runAfter:
    - init
    taskRef:
      params:
      - name: name
        value: git-clone-oci-ta
      - name: bundle
        value: quay.io/konflux-ci/tekton-catalog/task-git-clone-oci-ta:0.1@sha256:0f4360ce144d46171ebd2e8f4d4575539a0600e02208ba5fc9beeb2c27ddfd4c
      - name: kind
        value: task
      resolver: bundles
    when:
    - input: $(tasks.init.results.build)
      operator: in
      values:
      - "true"
    workspaces:
    - name: basic-auth
      workspace: git-auth
  - name: prefetch-dependencies
    params:
    - name: input
      value: $(params.prefetch-input)
    - name: SOURCE_ARTIFACT
      value: $(tasks.clone-repository.results.SOURCE_ARTIFACT)
    - name: ociStorage
      value: $(params.output-image).prefetch
    - name: ociArtifactExpiresAfter
      value: $(params.image-expires-after)
    runAfter:
    - clone-repository
    taskRef:
      params:
      - name: name
        value: prefetch-dependencies-oci-ta
      - name: bundle
        value: quay.io/konflux-ci/tekton-catalog/task-prefetch-dependencies-oci-ta:0.1@sha256:34a2a8b700bfdfddc4a3e6328f0f8ba29eb2de89a921e24d05c39cc6c5d05351
      - name: kind
        value: task
      resolver: bundles
    when:
    - input: $(params.prefetch-input)
      operator: notin
      values:
      - ""
    workspaces:
    - name: git-basic-auth
      workspace: git-auth
    - name: netrc
      workspace: netrc
  - name: build-container
    params:
    - name: IMAGE
      value: $(params.output-image)
    - name: DOCKERFILE
      value: $(params.dockerfile)
    - name: CONTEXT
      value: $(params.path-context)
    - name: HERMETIC
      value: $(params.hermetic)
    - name: PREFETCH_INPUT
      value: $(params.prefetch-input)
    - name: IMAGE_EXPIRES_AFTER
      value: $(params.image-expires-after)
    - name: COMMIT_SHA
      value: $(tasks.clone-repository.results.commit)
    - name: BUILD_ARGS
      value:
      - $(params.build-args[*])
    - name: BUILD_ARGS_FILE
      value: $(params.build-args-file)
    - name: SOURCE_ARTIFACT
      value: $(tasks.prefetch-dependencies.results.SOURCE_ARTIFACT)
    - name: CACHI2_ARTIFACT
      value: $(tasks.prefetch-dependencies.results.CACHI2_ARTIFACT)
    runAfter:
    - prefetch-dependencies
    taskRef:
      params:
      - name: name
        value: buildah-oci-ta
      - name: bundle
        value: quay.io/konflux-ci/tekton-catalog/task-buildah-oci-ta:0.2@sha256:3ccc7122935a0f24f02276b216bf9dc02dc2da4ef9cdb9263f05c34c003c7532
      - name: kind
        value: task
      resolver: bundles
    when:
    - input: $(tasks.init.results.build)
      operator: in
      values:
      - "true"
  - name: build-source-image
    params:
    - name: BINARY_IMAGE
      value: $(params.output-image)
    - name: SOURCE_ARTIFACT
      value: $(tasks.prefetch-dependencies.results.SOURCE_ARTIFACT)
    - name: CACHI2_ARTIFACT
      value: $(tasks.prefetch-dependencies.results.CACHI2_ARTIFACT)
    runAfter:
    - build-container
    taskRef:
      params:
      - name: name
        value: source-build-oci-ta
      - name: bundle
        value: quay.io/konflux-ci/tekton-catalog/task-source-build-oci-ta:0.1@sha256:99ee22c5e8e8a66da3d68ec5f3334e7cc59f8b8907e9d2a78f7338aa37d952eb
      - name: kind
        value: task
      resolver: bundles
    when:
    - input: $(tasks.init.results.build)
      operator: in
      values:
      - "true"
    - input: $(params.build-source-image)
      operator: in
      values:
      - "true"
  - name: deprecated-base-image-check
    params:
    - name: IMAGE_URL
      value: $(tasks.build-container.results.IMAGE_URL)
    - name: IMAGE_DIGEST
      value: $(tasks.build-container.results.IMAGE_DIGEST)
    runAfter:
    - build-container
    taskRef:
      params:
      - name: name
        value: deprecated-image-check
      - name: bundle
        value: quay.io/konflux-ci/tekton-catalog/task-deprecated-image-check:0.4@sha256:4ec00cf88c55a64eebaac0fe661cdb8dc8d3586d3a288f54400fbf2b5f06b408
      - name: kind
        value: task
      resolver: bundles
    when:
    - input: $(params.skip-checks)
      operator: in
      values:
      - "false"
  - name: clair-scan
    params:
    - name: image-digest
      value: $(tasks.build-container.results.IMAGE_DIGEST)
    - name: image-url
      value: $(tasks.build-container.results.IMAGE_URL)
    runAfter:
    - build-container
    taskRef:
      params:
      - name: name
        value: clair-scan
      - name: bundle
        value: quay.io/konflux-ci/tekton-catalog/task-clair-scan:0.1@sha256:3ac359dfc034948d59b9e8d507a8cf505a19b205c543e4c861a332c6f82a0307
      - name: kind
        value: task
      resolver: bundles
    when:
    - input: $(params.skip-checks)
      operator: in
      values:
      - "false"
  - name: ecosystem-cert-preflight-checks
    params:
    - name: image-url
      value: $(tasks.build-container.results.IMAGE_URL)
    runAfter:
    - build-container
    taskRef:
      params:
      - name: name
        value: ecosystem-cert-preflight-checks
      - name: bundle
        value: quay.io/konflux-ci/tekton-catalog/task-ecosystem-cert-preflight-checks:0.1@sha256:a4dc853e50a31272d45aef8e8bdc7dac0f0f92212fc7bf8bab67ba5917c03405
      - name: kind
        value: task
      resolver: bundles
    when:
    - input: $(params.skip-checks)
      operator: in
      values:
      - "false"
  - name: sast-snyk-check
    params:
    - name: SOURCE_ARTIFACT
      value: $(tasks.prefetch-dependencies.results.SOURCE_ARTIFACT)
    runAfter:
    - build-container
    taskRef:
      params:
      - name: name
        value: sast-snyk-check-oci-ta
      - name: bundle
        value: quay.io/konflux-ci/tekton-catalog/task-sast-snyk-check-oci-ta:0.1@sha256:ab70a8249b7cbcf21ee5e5054f6cd26368d4270014ebd584c1bf31a382e9ed4f
      - name: kind
        value: task
      resolver: bundles
    when:
    - input: $(params.skip-checks)
      operator: in
      values:
      - "false"
  - name: clamav-scan
    params:
    - name: image-digest
      value: $(tasks.build-container.results.IMAGE_DIGEST)
    - name: image-url
      value: $(tasks.build-container.results.IMAGE_URL)
    runAfter:
    - build-container
    taskRef:
      params:
      - name: name
        value: clamav-scan
      - name: bundle
        value: quay.io/konflux-ci/tekton-catalog/task-clamav-scan:0.1@sha256:b494d21c755f5142f74441df3dc204a1d357a0fc339ac5a8000b80dc983182f9
      - name: kind
        value: task
      resolver: bundles
    when:
    - input: $(params.skip-checks)
      operator: in
      values:
      - "false"
  - name: apply-tags
    params:
    - name: IMAGE
      value: $(tasks.build-container.results.IMAGE_URL)
    runAfter:
    - build-container
    taskRef:
      params:
      - name: name
        value: apply-tags
      - name: bundle
        value: quay.io/konflux-ci/tekton-catalog/task-apply-tags:0.1@sha256:8a8c1342bfa0b263361cbbc28036750ebc3d7c2460230b14d641170b812dddcc
      - name: kind
        value: task
      resolver: bundles
  - name: push-dockerfile
    params:
    - name: IMAGE
      value: $(tasks.build-container.results.IMAGE_URL)
    - name: IMAGE_DIGEST
      value: $(tasks.build-container.results.IMAGE_DIGEST)
    - name: DOCKERFILE
      value: $(params.dockerfile)
    - name: CONTEXT
      value: $(params.path-context)
    - name: SOURCE_ARTIFACT
      value: $(tasks.prefetch-dependencies.results.SOURCE_ARTIFACT)
    runAfter:
    - build-container
    taskRef:
      params:
      - name: name
        value: push-dockerfile-oci-ta
      - name: bundle
        value: quay.io/konflux-ci/tekton-catalog/task-push-dockerfile-oci-ta:0.1@sha256:117ca1f960e8d003f9a5e144e90cacf1ec9d8b4dbdb3113516658d8020db72f2
      - name: kind
        value: task
      resolver: bundles
  workspaces:
  - name: git-auth
    optional: true
  - name: netrc
    optional: true
`,
			expected: `
apiVersion: tekton.dev/v1
kind: Pipeline
metadata:
  creationTimestamp: null
  labels:
    pipelines.openshift.io/runtime: generic
    pipelines.openshift.io/strategy: docker
    pipelines.openshift.io/used-by: build-cloud
  name: docker-build-oci-ta
spec:
  finally:
  - name: show-sbom
    params:
    - name: IMAGE_URL
      value: $(tasks.build-container.results.IMAGE_URL)
    taskRef:
      params:
      - name: name
        value: show-sbom
      - name: bundle
        value: quay.io/konflux-ci/tekton-catalog/task-show-sbom:0.1@sha256:9bfc6b99ef038800fe131d7b45ff3cd4da3a415dd536f7c657b3527b01c4a13b
      - name: kind
        value: task
      resolver: bundles
  params:
  - description: Source Repository URL
    name: git-url
    type: string
  - default: ""
    description: Revision of the Source Repository
    name: revision
    type: string
  - description: Fully Qualified Output Image
    name: output-image
    type: string
  - default: .
    description: Path to the source code of an application's component from where
      to build image.
    name: path-context
    type: string
  - default: Dockerfile
    description: Path to the Dockerfile inside the context specified by parameter
      path-context
    name: dockerfile
    type: string
  - default: "false"
    description: Force rebuild image
    name: rebuild
    type: string
  - default: "false"
    description: Skip checks against built image
    name: skip-checks
    type: string
  - default: "false"
    description: Execute the build with network isolation
    name: hermetic
    type: string
  - default: ""
    description: Build dependencies to be prefetched by Cachi2
    name: prefetch-input
    type: string
  - default: "false"
    description: Java build
    name: java
    type: string
  - default: ""
    description: Image tag expiration time, time values could be something like 1h,
      2d, 3w for hours, days, and weeks, respectively.
    name: image-expires-after
  - default: "false"
    description: Build a source image.
    name: build-source-image
    type: string
  - default: []
    description: Array of --build-arg values ("arg=value" strings) for buildah
    name: build-args
    type: array
  - default: ""
    description: Path to a file with build arguments for buildah, see https://www.mankier.com/1/buildah-build#--build-arg-file
    name: build-args-file
    type: string
  results:
  - description: ""
    name: IMAGE_URL
    value: $(tasks.build-container.results.IMAGE_URL)
  - description: ""
    name: IMAGE_DIGEST
    value: $(tasks.build-container.results.IMAGE_DIGEST)
  - description: ""
    name: CHAINS-GIT_URL
    value: $(tasks.clone-repository.results.url)
  - description: ""
    name: CHAINS-GIT_COMMIT
    value: $(tasks.clone-repository.results.commit)
  - description: ""
    name: JAVA_COMMUNITY_DEPENDENCIES
    value: $(tasks.build-container.results.JAVA_COMMUNITY_DEPENDENCIES)
  tasks:
  - name: init
    params:
    - name: image-url
      value: $(params.output-image)
    - name: rebuild
      value: $(params.rebuild)
    - name: skip-checks
      value: $(params.skip-checks)
    taskRef:
      params:
      - name: name
        value: init
      - name: bundle
        value: quay.io/konflux-ci/tekton-catalog/task-init:0.2@sha256:092c113b614f6551113f17605ae9cb7e822aa704d07f0e37ed209da23ce392cc
      - name: kind
        value: task
      resolver: bundles
  - name: clone-repository
    params:
    - name: url
      value: $(params.git-url)
    - name: revision
      value: $(params.revision)
    - name: ociStorage
      value: $(params.output-image).git
    - name: ociArtifactExpiresAfter
      value: $(params.image-expires-after)
    runAfter:
    - init
    taskRef:
      params:
      - name: name
        value: git-clone-oci-ta
      - name: bundle
        value: quay.io/konflux-ci/tekton-catalog/task-git-clone-oci-ta:0.1@sha256:0f4360ce144d46171ebd2e8f4d4575539a0600e02208ba5fc9beeb2c27ddfd4c
      - name: kind
        value: task
      resolver: bundles
    when:
    - input: $(tasks.init.results.build)
      operator: in
      values:
      - "true"
    workspaces:
    - name: basic-auth
      workspace: git-auth
  - name: prefetch-dependencies
    params:
    - name: input
      value: $(params.prefetch-input)
    - name: SOURCE_ARTIFACT
      value: $(tasks.clone-repository.results.SOURCE_ARTIFACT)
    - name: ociStorage
      value: $(params.output-image).prefetch
    - name: ociArtifactExpiresAfter
      value: $(params.image-expires-after)
    runAfter:
    - clone-repository
    taskRef:
      params:
      - name: name
        value: prefetch-dependencies-oci-ta
      - name: bundle
        value: quay.io/konflux-ci/tekton-catalog/task-prefetch-dependencies-oci-ta:0.1@sha256:f13f6783f73971e4d1fbe8fd7fde3ea6cc080943c3fe2a4338ce6373c43f26a7
      - name: kind
        value: task
      resolver: bundles
    when:
    - input: $(params.prefetch-input)
      operator: notin
      values:
      - ""
    workspaces:
    - name: git-basic-auth
      workspace: git-auth
    - name: netrc
      workspace: netrc
  - name: build-container
    params:
    - name: IMAGE
      value: $(params.output-image)
    - name: DOCKERFILE
      value: $(params.dockerfile)
    - name: CONTEXT
      value: $(params.path-context)
    - name: HERMETIC
      value: $(params.hermetic)
    - name: PREFETCH_INPUT
      value: $(params.prefetch-input)
    - name: IMAGE_EXPIRES_AFTER
      value: $(params.image-expires-after)
    - name: COMMIT_SHA
      value: $(tasks.clone-repository.results.commit)
    - name: BUILD_ARGS
      value:
      - $(params.build-args[*])
    - name: BUILD_ARGS_FILE
      value: $(params.build-args-file)
    - name: SOURCE_ARTIFACT
      value: $(tasks.prefetch-dependencies.results.SOURCE_ARTIFACT)
    - name: CACHI2_ARTIFACT
      value: $(tasks.prefetch-dependencies.results.CACHI2_ARTIFACT)
    runAfter:
    - prefetch-dependencies
    taskRef:
      params:
      - name: name
        value: buildah-oci-ta
      - name: bundle
        value: quay.io/konflux-ci/tekton-catalog/task-buildah-oci-ta:0.2@sha256:725e781e74463d0bb111002eec85918e53df69a5eb55a7ccd6bd260e6542a9a3
      - name: kind
        value: task
      resolver: bundles
    when:
    - input: $(tasks.init.results.build)
      operator: in
      values:
      - "true"
  - name: build-source-image
    params:
    - name: BINARY_IMAGE
      value: $(params.output-image)
    - name: SOURCE_ARTIFACT
      value: $(tasks.prefetch-dependencies.results.SOURCE_ARTIFACT)
    - name: CACHI2_ARTIFACT
      value: $(tasks.prefetch-dependencies.results.CACHI2_ARTIFACT)
    runAfter:
    - build-container
    taskRef:
      params:
      - name: name
        value: source-build-oci-ta
      - name: bundle
        value: quay.io/konflux-ci/tekton-catalog/task-source-build-oci-ta:0.1@sha256:99ee22c5e8e8a66da3d68ec5f3334e7cc59f8b8907e9d2a78f7338aa37d952eb
      - name: kind
        value: task
      resolver: bundles
    when:
    - input: $(tasks.init.results.build)
      operator: in
      values:
      - "true"
    - input: $(params.build-source-image)
      operator: in
      values:
      - "true"
  - name: deprecated-base-image-check
    params:
    - name: IMAGE_URL
      value: $(tasks.build-container.results.IMAGE_URL)
    - name: IMAGE_DIGEST
      value: $(tasks.build-container.results.IMAGE_DIGEST)
    runAfter:
    - build-container
    taskRef:
      params:
      - name: name
        value: deprecated-image-check
      - name: bundle
        value: quay.io/konflux-ci/tekton-catalog/task-deprecated-image-check:0.4@sha256:4ec00cf88c55a64eebaac0fe661cdb8dc8d3586d3a288f54400fbf2b5f06b408
      - name: kind
        value: task
      resolver: bundles
    when:
    - input: $(params.skip-checks)
      operator: in
      values:
      - "false"
  - name: clair-scan
    params:
    - name: image-digest
      value: $(tasks.build-container.results.IMAGE_DIGEST)
    - name: image-url
      value: $(tasks.build-container.results.IMAGE_URL)
    runAfter:
    - build-container
    taskRef:
      params:
      - name: name
        value: clair-scan
      - name: bundle
        value: quay.io/konflux-ci/tekton-catalog/task-clair-scan:0.1@sha256:3ac359dfc034948d59b9e8d507a8cf505a19b205c543e4c861a332c6f82a0307
      - name: kind
        value: task
      resolver: bundles
    when:
    - input: $(params.skip-checks)
      operator: in
      values:
      - "false"
  - name: ecosystem-cert-preflight-checks
    params:
    - name: image-url
      value: $(tasks.build-container.results.IMAGE_URL)
    runAfter:
    - build-container
    taskRef:
      params:
      - name: name
        value: ecosystem-cert-preflight-checks
      - name: bundle
        value: quay.io/konflux-ci/tekton-catalog/task-ecosystem-cert-preflight-checks:0.1@sha256:a4dc853e50a31272d45aef8e8bdc7dac0f0f92212fc7bf8bab67ba5917c03405
      - name: kind
        value: task
      resolver: bundles
    when:
    - input: $(params.skip-checks)
      operator: in
      values:
      - "false"
  - name: sast-snyk-check
    params:
    - name: SOURCE_ARTIFACT
      value: $(tasks.prefetch-dependencies.results.SOURCE_ARTIFACT)
    runAfter:
    - build-container
    taskRef:
      params:
      - name: name
        value: sast-snyk-check-oci-ta
      - name: bundle
        value: quay.io/konflux-ci/tekton-catalog/task-sast-snyk-check-oci-ta:0.1@sha256:a5b18d0949240e2fc919277970fc1a9c4a0b13c4dec2bdf3ef579bc502a6f3d6
      - name: kind
        value: task
      resolver: bundles
    when:
    - input: $(params.skip-checks)
      operator: in
      values:
      - "false"
  - name: clamav-scan
    params:
    - name: image-digest
      value: $(tasks.build-container.results.IMAGE_DIGEST)
    - name: image-url
      value: $(tasks.build-container.results.IMAGE_URL)
    runAfter:
    - build-container
    taskRef:
      params:
      - name: name
        value: clamav-scan
      - name: bundle
        value: quay.io/konflux-ci/tekton-catalog/task-clamav-scan:0.1@sha256:4cb5750b01759a4f3d02bb8c6869e80dcde7bd4c7f5c0a68dd18e57ea2ac676f
      - name: kind
        value: task
      resolver: bundles
    when:
    - input: $(params.skip-checks)
      operator: in
      values:
      - "false"
  - name: apply-tags
    params:
    - name: IMAGE
      value: $(tasks.build-container.results.IMAGE_URL)
    runAfter:
    - build-container
    taskRef:
      params:
      - name: name
        value: apply-tags
      - name: bundle
        value: quay.io/konflux-ci/tekton-catalog/task-apply-tags:0.1@sha256:8a8c1342bfa0b263361cbbc28036750ebc3d7c2460230b14d641170b812dddcc
      - name: kind
        value: task
      resolver: bundles
  - name: push-dockerfile
    params:
    - name: IMAGE
      value: $(tasks.build-container.results.IMAGE_URL)
    - name: IMAGE_DIGEST
      value: $(tasks.build-container.results.IMAGE_DIGEST)
    - name: DOCKERFILE
      value: $(params.dockerfile)
    - name: CONTEXT
      value: $(params.path-context)
    - name: SOURCE_ARTIFACT
      value: $(tasks.prefetch-dependencies.results.SOURCE_ARTIFACT)
    runAfter:
    - build-container
    taskRef:
      params:
      - name: name
        value: push-dockerfile-oci-ta
      - name: bundle
        value: quay.io/konflux-ci/tekton-catalog/task-push-dockerfile-oci-ta:0.1@sha256:5dbf1dc5bc060c7d501623bac63dc0b0572c099f1c432846dbd801d82b66f581
      - name: kind
        value: task
      resolver: bundles
  workspaces:
  - name: git-auth
    optional: true
  - name: netrc
    optional: true
`,
		},
	}

	for _, tc := range tt {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := replaceTaskImagesFromExisting([]byte(tc.existing), []byte(tc.template))

			if diff := cmp.Diff(tc.expected, string(got)); diff != "" {
				t.Errorf("(-want, +got)\n%s", diff)
			}
		})
	}
}

func TestWriteFileReplacingNewerTaskImages(t *testing.T) {

	newBytes, err := os.ReadFile("testdata/docker-build.yaml")

	expected, err := os.ReadFile("testdata/docker-build-expected.yaml")
	if err != nil {
		t.Fatal(err)
	}

	if err := WriteFileReplacingNewerTaskImages("testdata/docker-build-expected.yaml", newBytes, 0777); err != nil {
		t.Fatal(err)
	}

	got, err := os.ReadFile("testdata/docker-build-expected.yaml")
	if err != nil {
		t.Fatal(err)
	}

	if strings.Contains(string(got), "xyz") {
		t.Fatal("expected new image to be replaced")
	}
	if strings.Contains(string(expected), "xyz") {
		t.Fatal("expected new image to be replaced")
	}

	if diff := cmp.Diff(string(expected), string(got)); diff != "" {
		t.Fatalf("(-want, +got)\n%s", diff)
	}
}
