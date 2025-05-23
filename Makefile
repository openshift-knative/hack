# Includes test environment variables to test that the generate command support
# replacing reference images via env variable.
include pkg/project/testdata/env

generate-ci: clean generate-ci-no-clean
.PHONY: generate-ci

generate-ci-no-clean:
	go run github.com/openshift-knative/hack/cmd/prowgen --config config/
.PHONY: generate-ci-no-clean

generate-client-ci:
	go run github.com/openshift-knative/hack/cmd/prowgen --config config/client.yaml $(ARGS)
.PHONY: generate-client-ci

generate-eventing-ci:
	go run github.com/openshift-knative/hack/cmd/prowgen --config config/eventing.yaml $(ARGS)
	go run github.com/openshift-knative/hack/cmd/prowgen --config config/backstage-plugins.yaml $(ARGS)
	go run github.com/openshift-knative/hack/cmd/prowgen --config config/eventing-istio.yaml $(ARGS)
	go run github.com/openshift-knative/hack/cmd/prowgen --config config/eventing-kafka-broker.yaml $(ARGS)
	go run github.com/openshift-knative/hack/cmd/prowgen --config config/eventing-hyperfoil-benchmark.yaml $(ARGS)
	go run github.com/openshift-knative/hack/cmd/prowgen --config config/eventing-integrations.yaml $(ARGS)
.PHONY: generate-eventing-ci

generate-serving-ci:
	go run github.com/openshift-knative/hack/cmd/prowgen --config config/serving.yaml $(ARGS)
	go run github.com/openshift-knative/hack/cmd/prowgen --config config/serving-net-istio.yaml $(ARGS)
	go run github.com/openshift-knative/hack/cmd/prowgen --config config/serving-net-kourier.yaml $(ARGS)
.PHONY: generate-serving-ci

generate-serverless-operator-ci:
	go run github.com/openshift-knative/hack/cmd/prowgen --config config/serverless-operator.yaml $(ARGS)
.PHONY: generate-serverless-operator-ci

generate-functions-ci:
	go run github.com/openshift-knative/hack/cmd/prowgen --config config/kn-plugin-func.yaml $(ARGS)
.PHONY: generate-functions-ci

generate-plugin-event-ci:
	go run github.com/openshift-knative/hack/cmd/prowgen --config config/kn-plugin-event.yaml $(ARGS)
.PHONY: generate-plugin-event-ci

discover-branches: clean
	go run github.com/openshift-knative/hack/cmd/discover $(ARGS)
.PHONY: discover-branches

generate-konflux-release: clean
	go run github.com/openshift-knative/hack/cmd/konflux-release-gen $(ARGS)
.PHONY: discover-branches

generate-ci-action:
	go run github.com/openshift-knative/hack/cmd/generate-ci-action
.PHONY: generate-ci-action

konflux-apply: clean konflux-apply-no-clean
.PHONY: konflux-apply

konflux-apply-no-clean:
	go run github.com/openshift-knative/hack/cmd/konflux-apply
.PHONY: konflux-apply-no-clean

konflux-update-pipelines:
	tkn bundle list quay.io/konflux-ci/tekton-catalog/pipeline-docker-build-multi-platform-oci-ta:devel -o=yaml > pkg/konfluxgen/kustomize/docker-build.yaml
	tkn bundle list quay.io/konflux-ci/tekton-catalog/pipeline-fbc-builder:devel -o=yaml > pkg/konfluxgen/kustomize/fbc-builder.yaml
	kustomize build pkg/konfluxgen/kustomize/kustomize-docker-build/ --output pkg/konfluxgen/docker-build.yaml --load-restrictor LoadRestrictionsNone
	kustomize build pkg/konfluxgen/kustomize/kustomize-java-docker-build/ --output pkg/konfluxgen/docker-java-build.yaml --load-restrictor LoadRestrictionsNone
	kustomize build pkg/konfluxgen/kustomize/kustomize-fbc-builder/ --output pkg/konfluxgen/fbc-builder.yaml --load-restrictor LoadRestrictionsNone
	kustomize build pkg/konfluxgen/kustomize/kustomize-bundle-build/ --output pkg/konfluxgen/bundle-build.yaml --load-restrictor LoadRestrictionsNone

gotest:
	go run gotest.tools/gotestsum@latest \
    		--format testname \
    		./...

unit-tests: gotest
	rm -rf openshift/project/testoutput
	rm -rf openshift/project/.github

	mkdir -p openshift
	go run ./cmd/generate-ci-action --input ".github/workflows/release-generate-ci-template.yaml" --config "config/" --output "openshift/release-generate-ci.yaml"
	# If the following fails, please run 'make generate-ci-action'
	diff -u -r "openshift/release-generate-ci.yaml" ".github/workflows/release-generate-ci.yaml"
.PHONY: unit-tests

test-select:
	go run github.com/openshift-knative/hack/cmd/testselect --testsuites $(TESTSUITES) --clonerefs $(CLONEREFS) --output=tests.txt
.PHONY: test-select

clean:
	rm -rf openshift openshift-knative
.PHONY: clean
