generate-ci:
	rm -rf openshift openshift-knative
	go run github.com/openshift-knative/hack/cmd/prowgen --config config/eventing.yaml --remote $(REMOTE)
	go run github.com/openshift-knative/hack/cmd/prowgen --config config/eventing-kafka-broker.yaml --remote $(REMOTE)
	go run github.com/openshift-knative/hack/cmd/prowgen --config config/eventing-hyperfoil-benchmark.yaml --remote $(REMOTE)
.PHONY: generate-ci

unit-tests:
	go test ./pkg/...
.PHONY: unit-tests

# TESTSUITES=/home/mgencur/go/src/github.com/openshift-knative/hack/config/testsuites.yaml CLONEREFS=/home/mgencur/go/src/github.com/openshift-knative/hack/config/clonerefs.json make test-select
test-select:
	go run github.com/openshift-knative/hack/cmd/testselect --testsuites $(TESTSUITES) --clonerefs $(CLONEREFS) --output=tests.txt
.PHONY: test-select
