# DO NOT EDIT! Generated Dockerfile for cmd/discover.
ARG GO_BUILDER=registry.ci.openshift.org/openshift/release:rhel-8-release-golang-1.21-openshift-4.17
ARG GO_RUNTIME=registry.access.redhat.com/ubi8/ubi-minimal

FROM $GO_BUILDER as builder

COPY . .

ENV CGO_ENABLED=1
ENV GOEXPERIMENT=strictfipsruntime

RUN go build -tags strictfipsruntime -o /usr/bin/main ./cmd/discover

FROM $GO_RUNTIME

ARG VERSION=main

# Install zoneinfo.
RUN microdnf install tzdata

USER 65532

COPY --from=builder /usr/bin/main /usr/bin/main

LABEL \
      com.redhat.component="openshift-serverless-1-knative-discover-rhel8-container" \
      name="openshift-serverless-1/knative-discover-rhel8" \
      version=$VERSION \
      summary="Red Hat OpenShift Serverless 1 Knative Discover" \
      maintainer="serverless-support@redhat.com" \
      description="Red Hat OpenShift Serverless 1 Knative Discover" \
      io.k8s.display-name="Red Hat OpenShift Serverless 1 Knative Discover"

ENTRYPOINT ["/usr/bin/main"]
