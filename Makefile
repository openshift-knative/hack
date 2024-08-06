# Includes test environment variables to test that the generate command support
# replacing reference images via env variable.
include pkg/project/testdata/env

generate-ci: clean generate-ci-no-clean
.PHONY: generate-ci

generate-ci-no-clean:
	go run github.com/openshift-knative/hack/cmd/prowgen --config config/
.PHONY: generate-ci-no-clean

generate-eventing-ci:
	go run github.com/openshift-knative/hack/cmd/prowgen --config config/eventing.yaml $(ARGS)
	go run github.com/openshift-knative/hack/cmd/prowgen --config config/backstage-plugins.yaml $(ARGS)
	go run github.com/openshift-knative/hack/cmd/prowgen --config config/eventing-istio.yaml $(ARGS)
	go run github.com/openshift-knative/hack/cmd/prowgen --config config/eventing-kafka-broker.yaml $(ARGS)
	go run github.com/openshift-knative/hack/cmd/prowgen --config config/eventing-hyperfoil-benchmark.yaml $(ARGS)
.PHONY: generate-eventing-ci

generate-serving-ci:
	go run github.com/openshift-knative/hack/cmd/prowgen --config config/serving.yaml $(ARGS)
	go run github.com/openshift-knative/hack/cmd/prowgen --config config/serving-net-istio.yaml $(ARGS)
	go run github.com/openshift-knative/hack/cmd/prowgen --config config/serving-net-kourier.yaml $(ARGS)
.PHONY: generate-serving-ci

generate-serverless-operator-ci:
	go run github.com/openshift-knative/hack/cmd/prowgen --config config/serverless-operator.yaml $(ARGS)
.PHONY: generate-serverless-operator-ci

discover-branches:
	go run github.com/openshift-knative/hack/cmd/discover $(ARGS)
.PHONY: discover-branches

generate-action:
	go run github.com/openshift-knative/hack/cmd/update-konflux-gen-action
.PHONY: generate-action

konflux-apply: clean konflux-apply-no-clean
.PHONY: konflux-apply

konflux-apply-no-clean:
	go run github.com/openshift-knative/hack/cmd/konflux-apply
.PHONY: konflux-apply-no-clean

unit-tests:
	go test ./pkg/...

	rm -rf openshift/project/testoutput
	rm -rf openshift/project/.github

	mkdir -p openshift
	go run ./cmd/update-konflux-gen-action --input ".github/workflows/release-generate-ci-template.yaml" --config "config/" --output "openshift/release-generate-ci.yaml"
	# If the following fails, please run 'make generate-action'
	diff -r "openshift/release-generate-ci.yaml" ".github/workflows/release-generate-ci.yaml"

	go run ./cmd/generate/ --generators dockerfile \
		--project-file pkg/project/testdata/project.yaml \
		--excludes ".*vendor.*" \
		--excludes ".*konflux-gen.*" \
		--excludes "openshift.*" \
		--images-from "hack" \
		--images-from-url-format "https://raw.githubusercontent.com/openshift-knative/%s/%s/pkg/project/testdata/additional-images.yaml" \
		--output "openshift/project/testoutput/openshift"
	diff -r "pkg/project/testoutput" "openshift/project/testoutput"

.PHONY: unit-tests

test-select:
	go run github.com/openshift-knative/hack/cmd/testselect --testsuites $(TESTSUITES) --clonerefs $(CLONEREFS) --output=tests.txt
.PHONY: test-select

clean:
	rm -rf openshift openshift-knative
.PHONY: clean
