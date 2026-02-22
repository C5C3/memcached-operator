# Helm Core Templates

Reference documentation for the core Helm chart templates: Deployment,
ServiceAccount, and RBAC resources under `charts/memcached-operator/templates/`.

**Source**: `charts/memcached-operator/templates/`

## Overview

The core templates translate the operator's Kustomize manifests
(`config/manager/manager.yaml`, `config/rbac/`) into parameterized Helm
templates. Together they produce the minimum resources required for a functional
operator deployment:

| Template File                              | Kind                         | Scope     | Gate                                       |
|--------------------------------------------|------------------------------|-----------|--------------------------------------------|
| `templates/serviceaccount.yaml`            | ServiceAccount               | Namespace | `serviceAccount.create`                    |
| `templates/deployment.yaml`                | Deployment                   | Namespace | Always rendered                            |
| `templates/rbac/clusterrole.yaml`          | ClusterRole                  | Cluster   | `rbac.create`                              |
| `templates/rbac/clusterrole.yaml`          | ClusterRoleBinding           | Cluster   | `rbac.create`                              |
| `templates/rbac/leader-election-role.yaml` | Role                         | Namespace | `rbac.create` AND `leaderElection.enabled` |
| `templates/rbac/leader-election-role.yaml` | RoleBinding                  | Namespace | `rbac.create` AND `leaderElection.enabled` |
| `templates/rbac/metrics-role.yaml`         | ClusterRole                  | Cluster   | `rbac.create`                              |
| `templates/rbac/metrics-role.yaml`         | ClusterRoleBinding           | Cluster   | `rbac.create`                              |
| `templates/rbac/metrics-role.yaml`         | ClusterRole (metrics-reader) | Cluster   | `rbac.create`                              |

All resources use the `memcached-operator.labels` helper for standard Helm
labels and `memcached-operator.fullname` for consistent naming. See
[Helm Chart Scaffold](helm-chart-scaffold.md) for helper definitions.

---

## Deployment

**Template**: `templates/deployment.yaml`
**Source**: `config/manager/manager.yaml` merged with `config/default/manager_metrics_patch.yaml`

The Deployment template renders the operator pod with a single `manager`
container. It is always rendered (no conditional gate).

### Resource Naming

```yaml
metadata:
  name: {{ include "memcached-operator.fullname" . }}
  namespace: {{ .Release.Namespace }}
```

### Labels

- **metadata.labels**: Full `memcached-operator.labels` + `control-plane: controller-manager`
- **spec.selector.matchLabels**: `memcached-operator.selectorLabels` + `control-plane: controller-manager`
- **pod template labels**: Full `memcached-operator.labels` + `control-plane: controller-manager`

The `control-plane: controller-manager` label preserves backward compatibility
with existing monitoring dashboards and log selectors from the Kustomize
deployment.

### Container Image

```yaml
image: "{{ .Values.image.repository }}:{{ .Values.image.tag | default .Chart.AppVersion }}"
imagePullPolicy: {{ .Values.image.pullPolicy }}
```

When `image.tag` is empty (default), `Chart.AppVersion` (`0.1.0`) is used.

### Container Args

The manager container runs `/manager` with the following arguments:

| Arg                                 | Condition                        | Default |
|-------------------------------------|----------------------------------|---------|
| `--leader-elect`                    | `leaderElection.enabled == true` | Present |
| `--health-probe-bind-address=:8081` | Always                           | Present |
| `--metrics-bind-address=:8443`      | Always                           | Present |
| `--metrics-secure`                  | Always                           | Present |
| `--watch-namespaces=<ns1,ns2>`      | `watchNamespaces` is non-empty   | Absent  |

When `watchNamespaces` contains entries, they are comma-joined:

```yaml
# values.yaml
watchNamespaces:
  - production
  - staging

# Renders as:
args:
  - --watch-namespaces=production,staging
```

### Ports

| Name    | Port | Protocol | Purpose          |
|---------|------|----------|------------------|
| health  | 8081 | TCP      | Health probes    |
| metrics | 8443 | TCP      | Metrics endpoint |

### Probes

| Probe     | Path       | Port | Initial Delay | Period |
|-----------|------------|------|---------------|--------|
| Liveness  | `/healthz` | 8081 | 15s           | 20s    |
| Readiness | `/readyz`  | 8081 | 5s            | 10s    |

### Security Contexts

**Pod-level** (`podSecurityContext`):

| Field                 | Default          |
|-----------------------|------------------|
| `runAsNonRoot`        | `true`           |
| `runAsUser`           | `65532`          |
| `seccompProfile.type` | `RuntimeDefault` |

**Container-level** (`securityContext`):

| Field                      | Default |
|----------------------------|---------|
| `allowPrivilegeEscalation` | `false` |
| `readOnlyRootFilesystem`   | `true`  |
| `capabilities.drop`        | `[ALL]` |

These defaults match `config/manager/manager.yaml` exactly.

### Scheduling

All scheduling fields are optional and only rendered when non-empty:

```yaml
# values.yaml
nodeSelector:
  kubernetes.io/os: linux
tolerations:
  - key: "dedicated"
    operator: "Equal"
    value: "memcached"
    effect: "NoSchedule"
affinity:
  nodeAffinity:
    requiredDuringSchedulingIgnoredDuringExecution:
      nodeSelectorTerms:
        - matchExpressions:
            - key: kubernetes.io/arch
              operator: In
              values: [amd64, arm64]
```

### Other Pod Spec Fields

| Field                           | Source                                         |
|---------------------------------|------------------------------------------------|
| `serviceAccountName`            | `memcached-operator.serviceAccountName` helper |
| `terminationGracePeriodSeconds` | Hardcoded `10`                                 |
| `imagePullSecrets`              | `imagePullSecrets` value (list of secrets)     |

---

## ServiceAccount

**Template**: `templates/serviceaccount.yaml`
**Gate**: `serviceAccount.create` (default: `true`)

When `serviceAccount.create` is `true`, the chart renders a ServiceAccount with:

```yaml
apiVersion: v1
kind: ServiceAccount
metadata:
  name: {{ include "memcached-operator.serviceAccountName" . }}
  namespace: {{ .Release.Namespace }}
  labels:
    # Standard Helm labels
  annotations:
    # From serviceAccount.annotations
```

### Name Resolution

The `serviceAccountName` helper determines the SA name used by the Deployment
and all RBAC bindings:

| `serviceAccount.create` | `serviceAccount.name` | Result          |
|-------------------------|-----------------------|-----------------|
| `true`                  | `""`                  | Fullname helper |
| `true`                  | `"my-sa"`             | `my-sa`         |
| `false`                 | `""`                  | `default`       |
| `false`                 | `"my-sa"`             | `my-sa`         |

When `create` is `false`, no ServiceAccount resource is rendered, but the
Deployment and RBAC bindings still reference the resolved name.

---

## RBAC Templates

**Gate**: `rbac.create` (default: `true`)

All RBAC resources are suppressed when `rbac.create` is `false`. When disabled,
the cluster administrator is responsible for creating the necessary RBAC
resources manually. The chart displays a warning in NOTES.txt when RBAC is
disabled.

### Resource Naming Convention

All RBAC resources use the `fullname-<suffix>` pattern to avoid name collisions
across multiple releases:

| Resource           | Name Pattern                                 |
|--------------------|----------------------------------------------|
| ClusterRole        | `{{ fullname }}-manager-role`                |
| ClusterRoleBinding | `{{ fullname }}-manager-rolebinding`         |
| Role               | `{{ fullname }}-leader-election-role`        |
| RoleBinding        | `{{ fullname }}-leader-election-rolebinding` |
| ClusterRole        | `{{ fullname }}-metrics-auth-role`           |
| ClusterRoleBinding | `{{ fullname }}-metrics-auth-rolebinding`    |
| ClusterRole        | `{{ fullname }}-metrics-reader`              |

### Manager ClusterRole

**Template**: `templates/rbac/clusterrole.yaml`
**Source**: `config/rbac/role.yaml`

Contains 11 rule blocks granting the operator least-privilege access to manage
Memcached CRs and their dependent resources:

| API Group               | Resource                   | Verbs                                           |
|-------------------------|----------------------------|-------------------------------------------------|
| `""` (core)             | `secrets`                  | get, list, watch                                |
| `""` (core)             | `services`                 | create, delete, get, list, patch, update, watch |
| `""`, `events.k8s.io`   | `events`                   | create, patch                                   |
| `apps`                  | `deployments`              | create, delete, get, list, patch, update, watch |
| `autoscaling`           | `horizontalpodautoscalers` | create, delete, get, list, patch, update, watch |
| `memcached.c5c3.io`     | `memcacheds`               | create, delete, get, list, patch, update, watch |
| `memcached.c5c3.io`     | `memcacheds/finalizers`    | update                                          |
| `memcached.c5c3.io`     | `memcacheds/status`        | get, patch, update                              |
| `monitoring.coreos.com` | `servicemonitors`          | create, delete, get, list, patch, update, watch |
| `networking.k8s.io`     | `networkpolicies`          | create, delete, get, list, patch, update, watch |
| `policy`                | `poddisruptionbudgets`     | create, delete, get, list, patch, update, watch |

The ClusterRoleBinding binds this role to the operator's ServiceAccount in the
release namespace.

### Leader Election Role

**Template**: `templates/rbac/leader-election-role.yaml`
**Gate**: `rbac.create` AND `leaderElection.enabled`
**Kind**: Role (namespace-scoped)

Grants permissions for lease-based leader election in the release namespace:

| API Group             | Resource     | Verbs                                           |
|-----------------------|--------------|-------------------------------------------------|
| `""` (core)           | `configmaps` | get, list, watch, create, update, patch, delete |
| `coordination.k8s.io` | `leases`     | get, list, watch, create, update, patch, delete |
| `""` (core)           | `events`     | create, patch                                   |

The RoleBinding binds this role to the operator's ServiceAccount.

### Metrics Auth ClusterRole

**Template**: `templates/rbac/metrics-role.yaml`
**Source**: `config/rbac/metrics_auth_role.yaml`

Grants permissions for the metrics endpoint authentication proxy:

| API Group               | Resource               | Verbs  |
|-------------------------|------------------------|--------|
| `authentication.k8s.io` | `tokenreviews`         | create |
| `authorization.k8s.io`  | `subjectaccessreviews` | create |

The same template also renders a `metrics-reader` ClusterRole granting read
access to the `/metrics` non-resource URL:

| Non-Resource URL | Verbs |
|------------------|-------|
| `/metrics`       | get   |

---

## values.yaml Reference

All values controlling core templates, with types, defaults, and descriptions.

### Deployment Parameters

| Key                                        | Type     | Default          | Description                                           |
|--------------------------------------------|----------|------------------|-------------------------------------------------------|
| `replicaCount`                             | `int`    | `1`              | Number of operator replicas                           |
| `image.repository`                         | `string` | `controller`     | Container image repository                            |
| `image.tag`                                | `string` | `""`             | Image tag (defaults to `Chart.AppVersion` when empty) |
| `image.pullPolicy`                         | `string` | `Always`         | Image pull policy                                     |
| `imagePullSecrets`                         | `list`   | `[]`             | Image pull secrets for private registries             |
| `resources.limits.cpu`                     | `string` | `500m`           | CPU limit                                             |
| `resources.limits.memory`                  | `string` | `128Mi`          | Memory limit                                          |
| `resources.requests.cpu`                   | `string` | `10m`            | CPU request                                           |
| `resources.requests.memory`                | `string` | `64Mi`           | Memory request                                        |
| `podSecurityContext.runAsNonRoot`          | `bool`   | `true`           | Require non-root user                                 |
| `podSecurityContext.runAsUser`             | `int`    | `65532`          | UID for the pod                                       |
| `podSecurityContext.seccompProfile.type`   | `string` | `RuntimeDefault` | Seccomp profile type                                  |
| `securityContext.allowPrivilegeEscalation` | `bool`   | `false`          | Prevent privilege escalation                          |
| `securityContext.readOnlyRootFilesystem`   | `bool`   | `true`           | Read-only root filesystem                             |
| `securityContext.capabilities.drop`        | `list`   | `[ALL]`          | Linux capabilities to drop                            |
| `nodeSelector`                             | `map`    | `{}`             | Node selector for pod scheduling                      |
| `tolerations`                              | `list`   | `[]`             | Tolerations for pod scheduling                        |
| `affinity`                                 | `map`    | `{}`             | Affinity rules for pod scheduling                     |

### ServiceAccount Parameters

| Key                          | Type     | Default | Description                                                        |
|------------------------------|----------|---------|--------------------------------------------------------------------|
| `serviceAccount.create`      | `bool`   | `true`  | Create a ServiceAccount                                            |
| `serviceAccount.annotations` | `map`    | `{}`    | Annotations for the ServiceAccount                                 |
| `serviceAccount.name`        | `string` | `""`    | Name override; if empty and `create` is true, uses fullname helper |

### RBAC Parameters

| Key                      | Type   | Default | Description                                                     |
|--------------------------|--------|---------|-----------------------------------------------------------------|
| `rbac.create`            | `bool` | `true`  | Create RBAC resources (ClusterRoles, Roles, and their bindings) |
| `leaderElection.enabled` | `bool` | `true`  | Enable leader election and its RBAC Role                        |

### Namespace Watching

| Key               | Type   | Default | Description                                     |
|-------------------|--------|---------|-------------------------------------------------|
| `watchNamespaces` | `list` | `[]`    | Namespaces to watch; empty means all namespaces |

### Name Overrides

| Key                | Type     | Default | Description                                      |
|--------------------|----------|---------|--------------------------------------------------|
| `nameOverride`     | `string` | `""`    | Partially override `memcached-operator.fullname` |
| `fullnameOverride` | `string` | `""`    | Fully override `memcached-operator.fullname`     |

---

## Testing

The core templates are tested with [helm-unittest](https://github.com/helm-unittest/helm-unittest):

| Test File                              | Coverage                                                  |
|----------------------------------------|-----------------------------------------------------------|
| `tests/serviceaccount_test.yaml`       | Create toggle, custom name, custom annotations, labels    |
| `tests/rbac_clusterrole_test.yaml`     | Permission rules, create toggle, binding subjects, labels |
| `tests/rbac_leader_election_test.yaml` | Permission rules, create toggle, namespace scope, binding |
| `tests/rbac_metrics_test.yaml`         | Permission rules, create toggle, binding subjects         |
| `tests/deployment_test.yaml`           | Image, args, security contexts, scheduling, labels        |
| `tests/integration_test.yaml`          | Cross-template: toggle combinations, name propagation     |

Run all chart tests:

```bash
helm unittest charts/memcached-operator
```
