# Helm Chart Scaffold

Reference documentation for the `charts/memcached-operator/` Helm chart structure,
`values.yaml` tunables, template helpers, and CRD management strategy.

**Source**: `charts/memcached-operator/`

## Overview

The Helm chart provides an alternative deployment method to Kustomize for
installing the memcached-operator. The chart uses Helm v2 API (`apiVersion: v2`)
and is structured as an `application` chart.

This scaffold contains the foundational chart files — `Chart.yaml`,
`values.yaml`, `_helpers.tpl`, and CRD handling — but does not yet include
resource templates for Deployment, RBAC, webhooks, or Services. Those are added
by subsequent features that build on this scaffold.

---

## Directory Structure

```text
charts/memcached-operator/
├── Chart.yaml                      # Chart metadata (name, version, appVersion)
├── .helmignore                     # Files excluded from chart packaging
├── values.yaml                     # Default configuration values
├── crds/
│   └── memcached.c5c3.io_memcacheds.yaml  # CRD for Helm-native install
├── templates/
│   ├── _helpers.tpl                # Shared template helpers (names, labels)
│   ├── crd-template.yaml           # Optional CRD template (gated by toggle)
│   └── NOTES.txt                   # Post-install instructions
└── tests/
    ├── chart_metadata_test.yaml    # helm-unittest: Chart.yaml and defaults
    ├── crd_template_test.yaml      # helm-unittest: CRD toggle behavior
    └── helpers_test.yaml           # helm-unittest: label and name helpers
```

---

## Chart.yaml

| Field         | Value                                                                  |
|---------------|------------------------------------------------------------------------|
| `apiVersion`  | `v2`                                                                   |
| `name`        | `memcached-operator`                                                   |
| `type`        | `application`                                                          |
| `version`     | `0.1.0` (chart version)                                                |
| `appVersion`  | `0.1.0` (operator version, tracks Makefile/Dockerfile)                 |
| `description` | A Helm chart for the Memcached Kubernetes Operator (memcached.c5c3.io) |

---

## values.yaml Reference

### General

| Key                | Type     | Default | Description                                      |
|--------------------|----------|---------|--------------------------------------------------|
| `replicaCount`     | `int`    | `1`     | Number of operator replicas to deploy            |
| `nameOverride`     | `string` | `""`    | Partially override `memcached-operator.fullname` |
| `fullnameOverride` | `string` | `""`    | Fully override `memcached-operator.fullname`     |

### Image

| Key                | Type     | Default        | Description                                                 |
|--------------------|----------|----------------|-------------------------------------------------------------|
| `image.repository` | `string` | `controller`   | Container image repository                                  |
| `image.tag`        | `string` | `""`           | Container image tag (defaults to chart appVersion if empty) |
| `image.pullPolicy` | `string` | `IfNotPresent` | Image pull policy                                           |
| `imagePullSecrets` | `list`   | `[]`           | Secrets for pulling images from private registries          |

### Resources

| Key                         | Type     | Default | Description                                          |
|-----------------------------|----------|---------|------------------------------------------------------|
| `resources.limits.cpu`      | `string` | `500m`  | CPU limit (matches `config/manager/manager.yaml`)    |
| `resources.limits.memory`   | `string` | `128Mi` | Memory limit (matches `config/manager/manager.yaml`) |
| `resources.requests.cpu`    | `string` | `10m`   | CPU request                                          |
| `resources.requests.memory` | `string` | `64Mi`  | Memory request                                       |

### Service Account

| Key                          | Type     | Default | Description                                                         |
|------------------------------|----------|---------|---------------------------------------------------------------------|
| `serviceAccount.create`      | `bool`   | `true`  | Whether to create a service account                                 |
| `serviceAccount.annotations` | `map`    | `{}`    | Annotations to add to the service account                           |
| `serviceAccount.name`        | `string` | `""`    | Name to use; if empty and `create` is true, generated from fullname |

### RBAC

| Key           | Type   | Default | Description                      |
|---------------|--------|---------|----------------------------------|
| `rbac.create` | `bool` | `true`  | Whether to create RBAC resources |

### Leader Election

| Key                      | Type   | Default | Description                                   |
|--------------------------|--------|---------|-----------------------------------------------|
| `leaderElection.enabled` | `bool` | `true`  | Enable leader election for controller manager |

### Feature Toggles

| Key                      | Type   | Default | Description                                       |
|--------------------------|--------|---------|---------------------------------------------------|
| `webhook.enabled`        | `bool` | `true`  | Enable admission webhooks (defaulting/validation) |
| `certmanager.enabled`    | `bool` | `true`  | Enable cert-manager integration for webhook TLS   |
| `serviceMonitor.enabled` | `bool` | `false` | Enable Prometheus ServiceMonitor resource         |
| `networkPolicy.enabled`  | `bool` | `false` | Enable NetworkPolicy resource                     |

### Namespace Watching

| Key               | Type   | Default | Description                                   |
|-------------------|--------|---------|-----------------------------------------------|
| `watchNamespaces` | `list` | `[]`    | List of namespaces to watch (empty means all) |

### CRD Management

| Key                  | Type   | Default | Description                                                           |
|----------------------|--------|---------|-----------------------------------------------------------------------|
| `crds.managedByHelm` | `bool` | `false` | If true, CRD is rendered as a Helm template for helm-managed upgrades |

See [CRD Handling Strategy](#crd-handling-strategy) for details.

### Security Contexts

Values mirror `config/manager/manager.yaml` exactly.

| Key                                        | Type     | Default          | Description                  |
|--------------------------------------------|----------|------------------|------------------------------|
| `podSecurityContext.runAsNonRoot`          | `bool`   | `true`           | Require non-root user        |
| `podSecurityContext.runAsUser`             | `int`    | `65532`          | UID to run as                |
| `podSecurityContext.seccompProfile.type`   | `string` | `RuntimeDefault` | Seccomp profile type         |
| `securityContext.allowPrivilegeEscalation` | `bool`   | `false`          | Prevent privilege escalation |
| `securityContext.readOnlyRootFilesystem`   | `bool`   | `true`           | Read-only root filesystem    |
| `securityContext.capabilities.drop`        | `list`   | `[ALL]`          | Drop all Linux capabilities  |

### Scheduling

| Key            | Type   | Default | Description                       |
|----------------|--------|---------|-----------------------------------|
| `nodeSelector` | `map`  | `{}`    | Node selector for pod assignment  |
| `tolerations`  | `list` | `[]`    | Tolerations for pod assignment    |
| `affinity`     | `map`  | `{}`    | Affinity rules for pod assignment |

---

## Template Helpers (`_helpers.tpl`)

The `templates/_helpers.tpl` file defines reusable named templates for consistent
naming and labeling across all chart templates.

### `memcached-operator.name`

Returns the chart name, optionally overridden by `nameOverride`, truncated to
63 characters (DNS name limit).

```yaml
# Default output:
memcached-operator

# With nameOverride: "custom-name":
custom-name
```

### `memcached-operator.fullname`

Returns a release-prefixed name, truncated to 63 characters. Logic:

1. If `fullnameOverride` is set, use it directly.
2. Otherwise, if the release name already contains the chart name, use the
   release name alone.
3. Otherwise, use `<release-name>-<chart-name>`.

```yaml
# Release "my-release", no overrides:
my-release-memcached-operator

# Release "memcached-operator", no overrides (contains chart name):
memcached-operator

# With fullnameOverride: "my-operator":
my-operator
```

### `memcached-operator.chart`

Returns `<chart-name>-<chart-version>` with `+` replaced by `_`, truncated to
63 characters. Used in the `helm.sh/chart` label.

```yaml
# Output:
memcached-operator-0.1.0
```

### `memcached-operator.labels`

Returns the full set of common labels for all resources:

```yaml
helm.sh/chart: memcached-operator-0.1.0
app.kubernetes.io/name: memcached-operator
app.kubernetes.io/instance: RELEASE-NAME
app.kubernetes.io/version: "0.1.0"
app.kubernetes.io/managed-by: Helm
```

Note: `app.kubernetes.io/managed-by` is `Helm` in the chart (vs. `kustomize`
in the Kustomize manifests under `config/`).

### `memcached-operator.selectorLabels`

Returns the subset of labels used for pod selectors:

```yaml
app.kubernetes.io/name: memcached-operator
app.kubernetes.io/instance: RELEASE-NAME
```

### `memcached-operator.serviceAccountName`

Returns the service account name:

- If `serviceAccount.create` is true: uses `serviceAccount.name` if set,
  otherwise falls back to the fullname.
- If `serviceAccount.create` is false: uses `serviceAccount.name` if set,
  otherwise `default`.

---

## CRD Handling Strategy

The chart provides two CRD management strategies controlled by
`crds.managedByHelm`:

### Default: `crds/` Directory (`crds.managedByHelm: false`)

The CRD file at `crds/memcached.c5c3.io_memcacheds.yaml` uses Helm's native
`crds/` directory behavior:

- **Install**: Helm installs the CRD automatically before any template resources
  on `helm install`.
- **Upgrade**: Helm does **not** update CRDs in the `crds/` directory on
  `helm upgrade`. CRD updates must be applied manually.
- **Uninstall**: Helm does **not** delete CRDs on `helm uninstall` (safety
  measure to prevent data loss).

This is the recommended default for production use, where CRD updates should be
a deliberate operation.

### Helm-Managed: Template (`crds.managedByHelm: true`)

When enabled, `templates/crd-template.yaml` renders the CRD as a regular Helm
template resource:

- **Install**: CRD is created as part of the release (in addition to the
  `crds/` directory copy).
- **Upgrade**: CRD is updated on `helm upgrade`, enabling automated CRD
  lifecycle management.
- **Uninstall**: CRD is deleted on `helm uninstall` (which also deletes all
  Memcached custom resources).

The template includes standard chart labels via `memcached-operator.labels`.

```bash
# Enable Helm-managed CRD upgrades:
helm install my-release ./charts/memcached-operator --set crds.managedByHelm=true
```

---

## Testing

The chart includes [helm-unittest](https://github.com/helm-unittest/helm-unittest)
test suites under `tests/`:

| Test File                  | Coverage                                          |
|----------------------------|---------------------------------------------------|
| `chart_metadata_test.yaml` | Chart.yaml metadata, default rendering, NOTES.txt |
| `crd_template_test.yaml`   | CRD template toggle (`crds.managedByHelm`)        |
| `helpers_test.yaml`        | Label helpers, name/fullname overrides            |

Run tests:

```bash
helm unittest charts/memcached-operator
```
