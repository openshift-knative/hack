# DO NOT EDIT! Generated Dockerfile for cmd/prowgen.
ARG GO_BUILDER=registry.ci.openshift.org/openshift/release:rhel-8-release-golang-1.21-openshift-4.17
ARG UBI8_MINIMAL=registry.access.redhat.com/ubi8/ubi-minimal
ARG VERSION=main

FROM $GO_BUILDER as builder

COPY . .

ENV CGO_ENABLED=1
ENV GOEXPERIMENT=strictfipsruntime

RUN go build -tags strictfipsruntime -o /usr/bin/main ./cmd/prowgen

FROM $UBI8_MINIMAL

# Install the missing zoneinfo to ubi-minimal
RUN microdnf install tzdata

USER 65532

COPY --from=builder /usr/bin/main /usr/bin/main

LABEL \
      com.redhat.component="openshift-serverless-1-knative-prowgen-rhel8-container" \
      name="openshift-serverless-1/knative-prowgen-rhel8" \
      version="$VERSION" \
      summary="Red Hat OpenShift Serverless 1 Knative Prowgen" \
      maintainer="serverless-support@redhat.com" \
      description="Red Hat OpenShift Serverless 1 Knative Prowgen" \
      io.k8s.display-name="Red Hat OpenShift Serverless 1 Knative Prowgen"

ENTRYPOINT ["/usr/bin/main"]
