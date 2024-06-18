# Includes test environment variables to test that the generate command support
# replacing reference images via env variable.
include pkg/project/testdata/env

all: eventing serving operator client ## Generate all the prow jobs
.PHONY: all

eventing: ## Generate the eventing prow jobs
	go run github.com/openshift-knative/hack/cmd/prowgen --config config/backstage-plugins.yaml $(ARGS)
	go run github.com/openshift-knative/hack/cmd/prowgen --config config/eventing.yaml $(ARGS)
	go run github.com/openshift-knative/hack/cmd/prowgen --config config/eventing-istio.yaml $(ARGS)
	go run github.com/openshift-knative/hack/cmd/prowgen --config config/eventing-kafka-broker.yaml $(ARGS)
	go run github.com/openshift-knative/hack/cmd/prowgen --config config/eventing-hyperfoil-benchmark.yaml $(ARGS)
.PHONY: eventing

serving: ## Generate the serving prow jobs
	go run github.com/openshift-knative/hack/cmd/prowgen --config config/serving.yaml $(ARGS)
	go run github.com/openshift-knative/hack/cmd/prowgen --config config/serving-net-istio.yaml $(ARGS)
	go run github.com/openshift-knative/hack/cmd/prowgen --config config/serving-net-kourier.yaml $(ARGS)
.PHONY: serving

operator: ## Generate the serverless-operator prow jobs
	go run github.com/openshift-knative/hack/cmd/prowgen --config config/serverless-operator.yaml $(ARGS)
.PHONY: operator

client: kn-event ## Generate the client and its plugins prow jobs
.PHONY: client

kn-event: ## Generate the kn-event prow jobs
	go run github.com/openshift-knative/hack/cmd/prowgen --config config/kn-event.yaml $(ARGS)
.PHONY: kn-event

# FIXME: Remove this target
deprecated:
	@go run github.com/charmbracelet/gum@latest style \
		--foreground "#DD0" \
		--padding '1 2' \
		--border double \
		--border-foreground "#DD0" \
		"WARNING: This Makefile target is deprecated and will be removed in the future."
.PHONY: deprecated

# FIXME: Remove this target
generate-ci: deprecated clean all
.PHONY: generate-ci

# FIXME: Remove this target
generate-ci-no-clean: deprecated all
.PHONY: generate-ci-no-clean

# FIXME: Remove this target
generate-eventing-ci: deprecated eventing
.PHONY: generate-eventing-ci

# FIXME: Remove this target
generate-serving-ci: deprecated serving
.PHONY: generate-serving-ci

# FIXME: Remove this target
generate-serverless-operator-ci: deprecated operator
.PHONY: generate-serverless-operator-ci

discover-branches: ## Discover branches
	go run github.com/openshift-knative/hack/cmd/discover $(ARGS)
.PHONY: discover-branches

generate-action: ## Update the konflux-gen action
	go run github.com/openshift-knative/hack/cmd/update-konflux-gen-action
.PHONY: generate-action

test: unit-tests ## Run all tests
.PHONY: test

unit-tests: ## Run unit tests
	go run gotest.tools/gotestsum@latest \
		--format testname -- \
		-count=1 -race -timeout=10m \
		./pkg/...

	rm -rf openshift/project/testoutput
	rm -rf openshift/project/.github

	mkdir -p openshift
	go run ./cmd/update-konflux-gen-action --input ".github/workflows/release-generate-ci-template.yaml" --config "config/" --output "openshift/release-generate-ci.yaml"
	# If the following fails, please run 'make generate-action'
	diff -ur "openshift/release-generate-ci.yaml" ".github/workflows/release-generate-ci.yaml"

	go run ./cmd/generate/ --generators dockerfile \
		--project-file pkg/project/testdata/project.yaml \
		--excludes ".*vendor.*" \
		--excludes ".*konflux-gen.*" \
		--excludes "openshift.*" \
		--images-from "hack" \
		--images-from-url-format "https://raw.githubusercontent.com/openshift-knative/%s/%s/pkg/project/testdata/additional-images.yaml" \
		--output "openshift/project/testoutput/openshift"
	diff -ur "pkg/project/testoutput" "openshift/project/testoutput"

.PHONY: unit-tests

test-select: ## Run the testselect tool
	go run github.com/openshift-knative/hack/cmd/testselect --testsuites $(TESTSUITES) --clonerefs $(CLONEREFS) --output=tests.txt
.PHONY: test-select

clean: ## Cleans the project temporary files
	rm -rf openshift openshift-knative
.PHONY: clean

# See: https://gist.github.com/prwhite/8168133?permalink_comment_id=3455873#gistcomment-3455873
help:  ## Display this help
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m\033[0m\n"} /^[a-zA-Z_-]+:.*?##/ { printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)
.PHONY: help
