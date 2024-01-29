# DO NOT EDIT! Generated Dockerfile for cmd/prowcopy.
FROM registry.ci.openshift.org/openshift/release:golang-1.21 as builder

COPY . .

RUN mkdir -p /var/run/ko && \
    mkdir -p cmd/prowcopy/kodata && \
    go build -o /usr/bin/main ./cmd/prowcopy && \
    cp -r cmd/prowcopy/kodata /var/run/ko

FROM registry.access.redhat.com/ubi8/ubi-minimal

# install the missing zoneinfo to ubi-minimal
RUN microdnf install tzdata

USER 65532

COPY --from=builder /usr/bin/main /usr/bin/main
COPY --from=builder /var/run/ko /var/run/ko
ENTRYPOINT ["/usr/bin/main"]
