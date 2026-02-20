# Multi-Stage Dockerfile

Reference documentation for the operator's multi-stage container build, including
OCI image labels, version embedding via ldflags, cross-compilation, and build
metadata automation.

**Source**: `Dockerfile`, `Makefile`, `internal/version/version.go`, `.dockerignore`

## Overview

The operator uses a two-stage Docker build to produce a minimal, secure container
image. The builder stage compiles a statically-linked Go binary with embedded
version information. The runtime stage copies only the binary into a distroless
base image running as a non-root user.

Build metadata (version, git commit, timestamp) flows from `make` variables through
Docker `--build-arg` flags into Go `ldflags`, producing a self-describing binary
and OCI-labeled image with no manual intervention.

---

## Dockerfile Stages

### Builder Stage

| Property | Value |
|----------|-------|
| Base image | `golang:1.25` |
| `CGO_ENABLED` | `0` (static binary, no libc dependency) |
| `GOOS` | `${TARGETOS:-linux}` |
| `GOARCH` | `${TARGETARCH}` |
| ldflags | `-s -w` (strip debug info and DWARF tables) |

Build args declared in the builder stage:

| ARG | Purpose |
|-----|---------|
| `TARGETOS` | Target OS for cross-compilation (default: `linux`) |
| `TARGETARCH` | Target architecture (`amd64`, `arm64`) |
| `VERSION` | Semantic version injected into the binary |
| `GIT_COMMIT` | Git SHA injected into the binary |
| `BUILD_DATE` | RFC 3339 timestamp injected into the binary |

The build command:

```dockerfile
RUN CGO_ENABLED=0 GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH} go build -a \
    -ldflags "-s -w \
      -X github.com/c5c3/memcached-operator/internal/version.Version=${VERSION} \
      -X github.com/c5c3/memcached-operator/internal/version.GitCommit=${GIT_COMMIT} \
      -X github.com/c5c3/memcached-operator/internal/version.BuildDate=${BUILD_DATE}" \
    -o manager cmd/main.go
```

### Runtime Stage

| Property | Value |
|----------|-------|
| Base image | `gcr.io/distroless/static:nonroot` |
| User | `65532:65532` (nonroot) |
| Entrypoint | `/manager` |
| Shell | None (distroless has no shell) |
| Package manager | None |

The runtime stage contains only the compiled `manager` binary. No build tools,
Go runtime, source code, or system utilities are present.

---

## OCI Image Labels

Labels follow the [OCI Image Spec annotations](https://github.com/opencontainers/image-spec/blob/main/annotations.md).
Static labels are hardcoded; dynamic labels use build ARGs that must be re-declared
after the `FROM` instruction (Docker scoping rule).

| Label | Value | Type |
|-------|-------|------|
| `org.opencontainers.image.title` | `memcached-operator` | Static |
| `org.opencontainers.image.description` | `Kubernetes operator for managing Memcached clusters` | Static |
| `org.opencontainers.image.url` | `https://github.com/c5c3/memcached-operator` | Static |
| `org.opencontainers.image.source` | `https://github.com/c5c3/memcached-operator` | Static |
| `org.opencontainers.image.licenses` | `Apache-2.0` | Static |
| `org.opencontainers.image.version` | `${VERSION}` | Dynamic |
| `org.opencontainers.image.revision` | `${GIT_COMMIT}` | Dynamic |
| `org.opencontainers.image.created` | `${BUILD_DATE}` | Dynamic |

Inspect labels with:

```bash
docker inspect --format='{{json .Config.Labels}}' controller:latest | jq .
```

---

## Version Package

**Path**: `internal/version/version.go`

Three package-level variables are set at build time via `-X` ldflags:

| Variable | Default | Description |
|----------|---------|-------------|
| `Version` | `dev` | Semantic version or git describe output |
| `GitCommit` | `unknown` | Full git commit SHA |
| `BuildDate` | `unknown` | UTC build timestamp in RFC 3339 format |

### Functions

**`String() string`** — Returns a formatted version string:

```
v1.2.3 (commit: abc1234, built: 2026-02-20T12:00:00Z)
```

**`Info() VersionInfo`** — Returns a `VersionInfo` struct with all fields accessible:

```go
type VersionInfo struct {
    Version   string
    GitCommit string
    BuildDate string
}
```

---

## Makefile Integration

### Build Metadata Variables

Defined at the top of the Makefile with `?=` for override-friendly defaults:

| Variable | Default | Description |
|----------|---------|-------------|
| `VERSION` | `$(shell git describe --tags --always --dirty)` | Version from the latest git tag, falls back to short SHA |
| `GIT_COMMIT` | `$(shell git rev-parse HEAD)` | Full 40-character commit SHA |
| `BUILD_DATE` | `$(shell date -u +"%Y-%m-%dT%H:%M:%SZ")` | Current UTC time in RFC 3339 |

All three commands include `2>/dev/null || echo <fallback>` guards for CI
environments without a git history or tags.

### docker-build

```bash
make docker-build IMG=myregistry/memcached-operator:v1.0.0
```

Passes `VERSION`, `GIT_COMMIT`, and `BUILD_DATE` as `--build-arg` flags to the
container tool (Docker by default, configurable via `CONTAINER_TOOL`).

### docker-buildx

```bash
make docker-buildx IMG=myregistry/memcached-operator:v1.0.0
```

Same build args as `docker-build`, plus multi-platform support via
`--platform=linux/arm64,linux/amd64`. Builds and pushes in a single step using a
dedicated buildx builder instance (`memcached-operator-builder`).

### Overriding Variables

```bash
make docker-build VERSION=v2.0.0 GIT_COMMIT=abc1234 BUILD_DATE=2026-01-01T00:00:00Z
```

---

## Cross-Compilation

The Dockerfile declares `TARGETOS` and `TARGETARCH` build args that Docker
BuildKit automatically populates during multi-platform builds. The `go build`
command uses these to set `GOOS` and `GOARCH`:

| Platform | `TARGETOS` | `TARGETARCH` |
|----------|------------|--------------|
| Linux AMD64 | `linux` | `amd64` |
| Linux ARM64 | `linux` | `arm64` |

`CGO_ENABLED=0` ensures a fully static binary with no libc dependency, which is
required for the distroless runtime image (no libc available).

---

## .dockerignore

The `.dockerignore` file excludes non-essential files from the build context to
reduce image build time and prevent leaking sensitive or unnecessary files:

| Pattern | Reason |
|---------|--------|
| `.git` | Git history not needed in build |
| `bin/` | Local build artifacts |
| `testbin/` | Test binaries |
| `vendor/` | Vendored dependencies (using module download) |
| `docs/` | Documentation |
| `cover.out` | Coverage reports |
| `*.md` | Markdown files |
| `.gitignore` | Git configuration |
| `.dockerignore` | Docker configuration |
| `.golangci.yml` | Linter configuration |
| `.serena/` | IDE configuration |
| `.planwerk/` | Planning files |
| `LICENSES/` | License files |
| `hack/` | Build scripts |
| `config/` | Kustomize manifests |
| `test/` | E2E test fixtures |

Only `go.mod`, `go.sum`, `cmd/`, `api/`, and `internal/` are included in the
build context.

---

## Security Properties

| Property | Implementation |
|----------|---------------|
| Non-root execution | `USER 65532:65532` in runtime stage |
| No shell access | distroless image contains no shell binary |
| No package manager | distroless image contains no apt/apk |
| Static binary | `CGO_ENABLED=0` eliminates runtime library dependencies |
| Minimal attack surface | Only the `manager` binary is present in the final image |
| Stripped binary | `-s -w` ldflags remove symbol tables and debug information |
