# Build the manager binary
FROM golang:1.25 AS builder
ARG TARGETOS
ARG TARGETARCH
ARG VERSION
ARG GIT_COMMIT
ARG BUILD_DATE

WORKDIR /workspace
# Copy the Go Modules manifests
COPY go.mod go.mod
COPY go.sum go.sum
# Cache deps before building and copying source so that we don't need to re-download as much
# and so that source changes don't invalidate our downloaded layer
RUN go mod download

# Copy the go source
COPY cmd/main.go cmd/main.go
COPY api/ api/
COPY internal/ internal/

# Build with ldflags to embed version information and strip debug symbols
RUN CGO_ENABLED=0 GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH} go build -a \
    -ldflags "-s -w \
      -X github.com/c5c3/memcached-operator/internal/version.Version=${VERSION} \
      -X github.com/c5c3/memcached-operator/internal/version.GitCommit=${GIT_COMMIT} \
      -X github.com/c5c3/memcached-operator/internal/version.BuildDate=${BUILD_DATE}" \
    -o manager cmd/main.go

# Use distroless as minimal base image to package the manager binary
# https://github.com/GoogleContainerTools/distroless
FROM gcr.io/distroless/static:nonroot@sha256:01e550fdb7ab79ee7be5ff440a563a58f1fd000ad9e0c532e65c3d23f917f1c5
WORKDIR /

# OCI image spec labels (https://github.com/opencontainers/image-spec/blob/main/annotations.md)
ARG VERSION
ARG GIT_COMMIT
ARG BUILD_DATE
LABEL org.opencontainers.image.title="memcached-operator" \
      org.opencontainers.image.description="Kubernetes operator for managing Memcached clusters" \
      org.opencontainers.image.url="https://github.com/c5c3/memcached-operator" \
      org.opencontainers.image.source="https://github.com/c5c3/memcached-operator" \
      org.opencontainers.image.version="${VERSION}" \
      org.opencontainers.image.revision="${GIT_COMMIT}" \
      org.opencontainers.image.created="${BUILD_DATE}" \
      org.opencontainers.image.licenses="Apache-2.0"

COPY --from=builder /workspace/manager .
USER 65532:65532

ENTRYPOINT ["/manager"]
