# Includes test environment variables to test that the generate command support
# replacing reference images via env variable.
include pkg/project/testdata/env

generate-ci:
	rm -rf openshift openshift-knative
	go run github.com/openshift-knative/hack/cmd/prowgen --config config/eventing.yaml --remote $(REMOTE)
	go run github.com/openshift-knative/hack/cmd/prowgen --config config/eventing-istio.yaml --remote $(REMOTE)
	go run github.com/openshift-knative/hack/cmd/prowgen --config config/eventing-kafka-broker.yaml --remote $(REMOTE)
	go run github.com/openshift-knative/hack/cmd/prowgen --config config/eventing-hyperfoil-benchmark.yaml --remote $(REMOTE)
	go run github.com/openshift-knative/hack/cmd/prowgen --config config/serving.yaml --remote $(REMOTE)
	go run github.com/openshift-knative/hack/cmd/prowgen --config config/serving-net-istio.yaml --remote $(REMOTE)
	go run github.com/openshift-knative/hack/cmd/prowgen --config config/serving-net-kourier.yaml --remote $(REMOTE)
.PHONY: generate-ci

generate-serving-ci:
	rm -rf openshift openshift-knative
	go run github.com/openshift-knative/hack/cmd/prowgen --config config/serving.yaml --remote $(REMOTE)
	go run github.com/openshift-knative/hack/cmd/prowgen --config config/serving-net-istio.yaml --remote $(REMOTE)
	go run github.com/openshift-knative/hack/cmd/prowgen --config config/serving-net-kourier.yaml --remote $(REMOTE)
.PHONY: generate-ci

generate-eventing-ci:
	rm -rf openshift openshift-knative
	go run github.com/openshift-knative/hack/cmd/prowgen --config config/eventing.yaml --remote $(REMOTE)
	go run github.com/openshift-knative/hack/cmd/prowgen --config config/eventing-istio.yaml --remote $(REMOTE)
	go run github.com/openshift-knative/hack/cmd/prowgen --config config/eventing-kafka-broker.yaml --remote $(REMOTE)
	go run github.com/openshift-knative/hack/cmd/prowgen --config config/eventing-hyperfoil-benchmark.yaml --remote $(REMOTE)
.PHONY: generate-ci

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
