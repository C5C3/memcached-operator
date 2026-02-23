# Helm Chart Release Workflow

Reference documentation for the GitHub Actions workflow that validates, packages,
and publishes the Helm chart to the GHCR OCI registry on version tags.

**Source**: `.github/workflows/helm-release.yml`

## Overview

The Helm release workflow is triggered when a tag matching `v*` is pushed
(e.g., `v0.2.0`). It validates the chart version against the tag, runs lint and
unit tests, then packages and pushes the chart to the OCI registry at
`oci://ghcr.io/c5c3/charts`. This means a single version tag releases both the
container image (via `release.yml`) and the Helm chart.

| Job        | Purpose                                            | Dependencies |
|------------|----------------------------------------------------|--------------|
| `validate` | Extract version from tag, lint, and run unit tests | none         |
| `release`  | Package chart and push to OCI registry             | `validate`   |

---

## Trigger

```yaml
on:
  push:
    tags:
      - "v*"
```

The workflow fires when a tag starting with `v` is pushed. The tag suffix
(without `v`) must match the `version` field in
`charts/memcached-operator/Chart.yaml` (e.g., tag `v0.2.0` requires
`version: 0.2.0` in `Chart.yaml`).

---

## Concurrency

```yaml
concurrency:
  group: helm-release
  cancel-in-progress: false
```

Only one Helm release workflow runs at a time. In-progress runs are **not**
cancelled to prevent partial releases.

---

## Permissions

| Permission        | Scope      | Purpose                |
|-------------------|------------|------------------------|
| `contents: read`  | `validate` | Checkout code          |
| `packages: write` | `release`  | Push chart to GHCR OCI |

---

## Jobs

### validate

Extracts the chart version from the tag, verifies it matches `Chart.yaml`, and
runs the full chart validation suite.

**Version extraction**: Strips the `v` prefix from the tag name to derive the
expected chart version.

**Version validation**: Reads the `version` field from
`charts/memcached-operator/Chart.yaml` and compares it to the tag-derived
version. The job fails with an annotated error if they do not match.

**Lint**: Runs `ct lint --config ct.yaml --charts charts/memcached-operator`
using the chart-testing tool to validate chart structure, `Chart.yaml` metadata,
and Helm template rendering.

**Unit tests**: Installs the helm-unittest plugin and runs
`helm unittest charts/memcached-operator` to verify template correctness.

Tools installed:

| Tool          | Version | Source                         |
|---------------|---------|--------------------------------|
| Helm          | v3.17.3 | `azure/setup-helm@v4`          |
| Python        | 3.12    | `actions/setup-python@v5`      |
| chart-testing | latest  | `helm/chart-testing-action@v2` |
| helm-unittest | v1.0.3  | helm plugin from upstream repo |

The validated chart version is passed to the `release` job via the
`chart-version` output.

### release

Packages the chart and pushes it to the OCI registry.

**Steps:**

1. **Login** — Authenticates to `ghcr.io` using `helm registry login` with the
   `GITHUB_TOKEN` secret.

2. **Package** — Runs `helm package charts/memcached-operator` to produce
   `memcached-operator-<version>.tgz`.

3. **Push** — Pushes the packaged chart to `oci://ghcr.io/c5c3/charts` using
   `helm push`. The chart is available at:
   ```text
   oci://ghcr.io/c5c3/charts/memcached-operator
   ```

4. **Logout** — Runs `helm registry logout ghcr.io` in an `always()` step to
   clean up credentials regardless of push success.

---

## OCI Registry

Helm charts are published to the GitHub Container Registry (GHCR) as OCI
artifacts at:

```text
oci://ghcr.io/c5c3/charts/memcached-operator
```

### Installing from the OCI Registry

```bash
helm install memcached-operator oci://ghcr.io/c5c3/charts/memcached-operator --version 0.2.0
```

To install into a specific namespace:

```bash
helm install memcached-operator oci://ghcr.io/c5c3/charts/memcached-operator \
  --version 0.2.0 \
  --namespace memcached-system \
  --create-namespace
```

To download the chart locally:

```bash
helm pull oci://ghcr.io/c5c3/charts/memcached-operator --version 0.2.0
```

---

## Creating a Helm Chart Release

To release a new version:

1. **Bump the chart version** in `charts/memcached-operator/Chart.yaml`:
   ```yaml
   version: 0.3.0
   appVersion: "0.3.0"
   ```

2. **Commit and push** the version change to `main`.

3. **Create and push the release tag**:
   ```bash
   git tag v0.3.0
   git push origin v0.3.0
   ```

This single tag triggers both the container image release (`release.yml`) and
the Helm chart release (`helm-release.yml`). Verify the chart release:

```bash
helm show chart oci://ghcr.io/c5c3/charts/memcached-operator --version 0.3.0
```

---

## Relationship to Other Workflows

| Workflow           | Scope                            | Trigger           |
|--------------------|----------------------------------|-------------------|
| `ci.yml`           | Code lint, test, E2E             | PR + push to main |
| `release.yml`      | Container image + GitHub Release | Tag `v*`          |
| `helm-release.yml` | Helm chart OCI package           | Tag `v*`          |

Both release workflows are triggered by the same `v*` tag. The chart's
`appVersion` in `Chart.yaml` should match the operator image version that the
chart deploys.
