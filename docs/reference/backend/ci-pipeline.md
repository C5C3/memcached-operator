# CI Pipeline

Reference documentation for the GitHub Actions CI workflow that runs lint,
test, Docker build, and CRD manifest validation on every push and pull request.

**Source**: `.github/workflows/ci.yml`

## Overview

The CI pipeline runs on pushes to `main` and pull requests targeting `main`. It
enforces code quality, correctness, build integrity, and manifest freshness
through four jobs:

| Job                  | Purpose                                        | Dependencies |
| -------------------- | ---------------------------------------------- | ------------ |
| `lint`               | `go vet` + golangci-lint v2.10.1               | none         |
| `test`               | envtest integration tests with race + coverage | none         |
| `build`              | Multi-stage Docker image build via Buildx       | none         |
| `validate-manifests` | Verify generated CRDs/deepcopy are up-to-date  | none         |

All four jobs run in parallel with no inter-job dependencies, minimising total
pipeline time.

---

## Triggers

```yaml
on:
  push:
    branches: [main]
  pull_request:
    branches: [main]
```

The workflow fires on direct pushes to `main` and on PRs targeting `main`.

---

## Concurrency

```yaml
concurrency:
  group: ${{ github.workflow }}-${{ github.ref }}
  cancel-in-progress: ${{ github.ref != format('refs/heads/{0}', github.event.repository.default_branch) }}
```

Concurrent runs for the same branch are cancelled to save runner minutes. The
`cancel-in-progress` expression evaluates to `true` for all branches except the
default branch (`main`), ensuring `main` always gets a full CI pass.

---

## Permissions

The workflow uses least-privilege permissions:

- **Top-level**: `contents: read` — minimum needed for checkout.
- **Per-job**: Each job explicitly declares `contents: read`.

No write permissions are granted. Docker images are built but not pushed.

---

## Jobs

### lint

Runs `go vet` followed by golangci-lint using the official
`golangci/golangci-lint-action@v7` action. The golangci-lint version is pinned
to `v2.10.1` to match the project's `Makefile` and `.golangci.yml`.

Go is set up using `go-version-file: go.mod` so the CI Go version always
matches the project's `go.mod` directive.

### test

Installs `setup-envtest` and downloads Kubernetes API server binaries (v1.32.0),
then runs all non-e2e tests with:

- `-race` — data race detection
- `-covermode=atomic` — thread-safe coverage for race-enabled tests
- `-coverprofile=cover.out` — coverage output

The coverage report is uploaded as a GitHub Actions artifact retained for 7 days.

### build

Uses `docker/build-push-action@v6` with Buildx and GitHub Actions cache
(`cache-from: type=gha`). The image is built and loaded locally (`push: false`,
`load: true`) to verify the Dockerfile compiles successfully. Build args inject
the version from `git describe --tags --always --dirty`, git SHA, and build
timestamp.

### validate-manifests

Runs `make manifests generate` to regenerate CRD YAMLs and deepcopy code, then
`make verify-manifests` to assert no diff exists. This catches stale generated
files that were not committed after API type changes.

---

## Makefile Integration

The CI workflow reuses the project's Makefile targets where possible:

| CI Step               | Makefile Target      |
| --------------------- | -------------------- |
| Generate manifests    | `make manifests`     |
| Generate deepcopy     | `make generate`      |
| Verify no drift       | `make verify-manifests` |

The lint and test jobs use direct `go` commands / actions rather than Makefile
targets to avoid installing tool binaries that the actions already provide.

---

## Local Reproduction

Each CI check can be reproduced locally using Makefile targets:

```bash
# Lint — runs go vet and golangci-lint
make lint

# Unit and integration tests with envtest and coverage
make test

# Docker image build (build-only, no push)
make docker-build

# Verify generated CRD manifests and deepcopy are up-to-date
make verify-manifests
```

---

## Adding New Jobs

To add a new CI job:

1. Add the job definition under `jobs:` in `.github/workflows/ci.yml`.
2. Use `go-version-file: go.mod` for Go setup.
3. Declare explicit `permissions: contents: read`.
4. Add `needs:` only if the job must wait for another to finish; all current
   jobs run in parallel.
