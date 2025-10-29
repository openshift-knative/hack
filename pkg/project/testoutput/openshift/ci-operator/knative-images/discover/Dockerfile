# DO NOT EDIT! Generated Dockerfile for cmd/discover.
ARG GO_BUILDER=registry.ci.openshift.org/openshift/release:rhel-9-release-golang-1.22-openshift-4.17
ARG GO_RUNTIME=registry.access.redhat.com/ubi9/ubi-minimal

FROM $GO_BUILDER as builder

WORKDIR /workspace
COPY . .

ENV CGO_ENABLED=1
ENV GOEXPERIMENT=strictfipsruntime
ENV GOFLAGS='-mod=mod'

RUN go build -tags strictfipsruntime -o /usr/bin/main ./cmd/discover

FROM $GO_RUNTIME

ARG VERSION=main
RUN microdnf update tzdata -y && microdnf reinstall tzdata -y
RUN microdnf install rsync

COPY --from=builder /usr/bin/main /usr/bin/discover
COPY LICENSE /licenses/

USER 65532

LABEL \
      com.redhat.component="openshift-serverless-1-knative-discover-rhel9-container" \
      name="openshift-serverless-1/knative-discover-rhel9" \
      version=$VERSION \
      summary="Red Hat OpenShift Serverless 1 Knative Discover" \
      maintainer="serverless-support@redhat.com" \
      description="Red Hat OpenShift Serverless 1 Knative Discover" \
      io.k8s.display-name="Red Hat OpenShift Serverless 1 Knative Discover" \
      io.k8s.description="Red Hat OpenShift Serverless Knative Discover" \
      io.openshift.tags="discover" \
      vendor="Red Hat, Inc." \
      release=$VERSION \
      cpe="cpe:/a:redhat:openshift_serverless:1.37::el9"

ENTRYPOINT ["/usr/bin/discover"]
