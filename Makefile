generate-ci:
	rm -rf openshift openshift-knative
	go run github.com/openshift-knative/hack/cmd/prowgen --config config/eventing.yaml --remote $(REMOTE)
	go run github.com/openshift-knative/hack/cmd/prowgen --config config/eventing-hyperfoil-benchmark.yaml --remote $(REMOTE)
.PHONY: generate-ci

unit-tests:
	go test ./cmd/...
.PHONY: unit-tests
