# Includes test environment variables to test that the generate command support
# replacing reference images via env variable.
include pkg/project/testdata/env

all: eventing serving operator client
.PHONY: all

eventing:
	go run github.com/openshift-knative/hack/cmd/prowgen --config config/backstage-plugins.yaml $(ARGS)
	go run github.com/openshift-knative/hack/cmd/prowgen --config config/eventing.yaml $(ARGS)
	go run github.com/openshift-knative/hack/cmd/prowgen --config config/eventing-istio.yaml $(ARGS)
	go run github.com/openshift-knative/hack/cmd/prowgen --config config/eventing-kafka-broker.yaml $(ARGS)
	go run github.com/openshift-knative/hack/cmd/prowgen --config config/eventing-hyperfoil-benchmark.yaml $(ARGS)
.PHONY: eventing

serving:
	go run github.com/openshift-knative/hack/cmd/prowgen --config config/serving.yaml $(ARGS)
	go run github.com/openshift-knative/hack/cmd/prowgen --config config/serving-net-istio.yaml $(ARGS)
	go run github.com/openshift-knative/hack/cmd/prowgen --config config/serving-net-kourier.yaml $(ARGS)
.PHONY: serving

operator:
	go run github.com/openshift-knative/hack/cmd/prowgen --config config/serverless-operator.yaml $(ARGS)
.PHONY: operator

client: kn-event
.PHONY: client

kn-event:
	go run github.com/openshift-knative/hack/cmd/prowgen --config config/kn-event.yaml $(ARGS)
.PHONY: kn-event

discover-branches:
	go run github.com/openshift-knative/hack/cmd/discover $(ARGS)
.PHONY: discover-branches

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
