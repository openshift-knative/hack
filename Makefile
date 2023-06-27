# Includes test environment variables to test that the generate command support
# replacing reference images via env variable.
include pkg/project/testdata/env

generate-ci: clean generate-eventing-ci generate-serving-ci
.PHONY: generate-ci

generate-ci-no-clean: generate-eventing-ci generate-serving-ci
.PHONY: generate-ci-no-clean

generate-eventing-ci: clean
	go run github.com/openshift-knative/hack/cmd/prowgen --config config/eventing.yaml $(REMOTE)
	go run github.com/openshift-knative/hack/cmd/prowgen --config config/eventing-istio.yaml $(REMOTE)
	go run github.com/openshift-knative/hack/cmd/prowgen --config config/eventing-kafka-broker.yaml $(REMOTE)
	go run github.com/openshift-knative/hack/cmd/prowgen --config config/eventing-hyperfoil-benchmark.yaml $(REMOTE)
.PHONY: generate-eventing-ci

generate-serving-ci: clean
	go run github.com/openshift-knative/hack/cmd/prowgen --config config/serving.yaml $(REMOTE)
	go run github.com/openshift-knative/hack/cmd/prowgen --config config/serving-net-istio.yaml $(REMOTE)
	go run github.com/openshift-knative/hack/cmd/prowgen --config config/serving-net-kourier.yaml $(REMOTE)
.PHONY: generate-serving-ci

unit-tests:
	go test ./pkg/...

	rm -rf openshift/project/testoutput
	go run ./cmd/generate/ --generators dockerfile \
		--project-file pkg/project/testdata/project.yaml \
		--excludes ".*vendor.*" \
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
