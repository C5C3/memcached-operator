# Pod Security Contexts

Reference documentation for the Memcached operator's pod and container security
context implementation that applies user-provided security settings to the
Deployment pod template and all containers.

**Source**: `internal/controller/deployment.go`, `api/v1alpha1/memcached_types.go`

## Overview

The operator passes through user-provided security contexts from the Memcached
CR to the Deployment pod template. This enables cluster operators to configure
Kubernetes Pod Security Standards (e.g., the restricted profile) without the
operator imposing defaults.

Two security context levels are supported:

1. **Pod security context** (`spec.security.podSecurityContext`) — applied to
   `PodSpec.SecurityContext`, controlling pod-level settings like `runAsNonRoot`,
   `fsGroup`, and `seccompProfile`.
2. **Container security context** (`spec.security.containerSecurityContext`) —
   applied to every container in the pod (both `memcached` and `exporter`),
   controlling container-level settings like `readOnlyRootFilesystem`,
   `capabilities`, and `runAsUser`.

The operator does **not** set restrictive defaults. It applies exactly what the
user provides. Defaulting to a restricted profile is the responsibility of the
future defaulting webhook (S021).

---

## CRD Field Path

```
spec.security.podSecurityContext
spec.security.containerSecurityContext
```

Defined in `api/v1alpha1/memcached_types.go` on the `SecuritySpec` struct:

```go
type SecuritySpec struct {
    PodSecurityContext       *corev1.PodSecurityContext `json:"podSecurityContext,omitempty,omitzero"`
    ContainerSecurityContext *corev1.SecurityContext     `json:"containerSecurityContext,omitempty,omitzero"`
    SASL                    *SASLSpec                   `json:"sasl,omitempty,omitzero"`
    TLS                     *TLSSpec                    `json:"tls,omitempty,omitzero"`
}
```

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| `podSecurityContext` | `*PodSecurityContext` | No | nil | Pod-level security context applied to `PodSpec.SecurityContext` |
| `containerSecurityContext` | `*SecurityContext` | No | nil | Container-level security context applied to all containers |

---

## Helper Functions

### `buildPodSecurityContext`

```go
func buildPodSecurityContext(mc *Memcached) *corev1.PodSecurityContext
```

Returns `spec.security.podSecurityContext` from the CR, or `nil` when:

- `spec.security` is nil
- `spec.security.podSecurityContext` is nil

### `buildContainerSecurityContext`

```go
func buildContainerSecurityContext(mc *Memcached) *corev1.SecurityContext
```

Returns `spec.security.containerSecurityContext` from the CR, or `nil` when:

- `spec.security` is nil
- `spec.security.containerSecurityContext` is nil

Both functions follow the same nil-guard pattern as `buildAntiAffinity`,
`buildTopologySpreadConstraints`, and `buildGracefulShutdown`.

---

## Deployment Mapping

In `constructDeployment`, the security contexts are applied as follows:

| CR Field | Deployment Field |
|----------|-----------------|
| `spec.security.podSecurityContext` | `spec.template.spec.securityContext` |
| `spec.security.containerSecurityContext` | `spec.template.spec.containers[*].securityContext` |

The container security context is applied to **all** containers in the pod:

- `memcached` container (always present)
- `exporter` container (when `spec.monitoring.enabled` is `true`)

This ensures consistent security settings across all containers, which is
required for Pod Security Standards admission.

---

## CR Examples

### Kubernetes Restricted Profile

```yaml
apiVersion: memcached.c5c3.io/v1alpha1
kind: Memcached
metadata:
  name: my-cache
  namespace: default
spec:
  replicas: 3
  security:
    podSecurityContext:
      runAsNonRoot: true
      fsGroup: 1000
      seccompProfile:
        type: RuntimeDefault
    containerSecurityContext:
      runAsUser: 1000
      runAsNonRoot: true
      readOnlyRootFilesystem: true
      allowPrivilegeEscalation: false
      capabilities:
        drop:
          - ALL
```

Produces a Deployment with:

```yaml
spec:
  template:
    spec:
      securityContext:
        runAsNonRoot: true
        fsGroup: 1000
        seccompProfile:
          type: RuntimeDefault
      containers:
        - name: memcached
          securityContext:
            runAsUser: 1000
            runAsNonRoot: true
            readOnlyRootFilesystem: true
            allowPrivilegeEscalation: false
            capabilities:
              drop:
                - ALL
```

### Pod Security Context Only

```yaml
spec:
  replicas: 3
  security:
    podSecurityContext:
      runAsNonRoot: true
      fsGroup: 1000
```

Only the pod-level security context is set. Container security contexts are nil.

### Container Security Context Only

```yaml
spec:
  replicas: 3
  security:
    containerSecurityContext:
      readOnlyRootFilesystem: true
      capabilities:
        drop:
          - ALL
```

Only the container-level security context is set. Pod security context is nil.

### With Monitoring (Exporter Sidecar)

```yaml
spec:
  replicas: 3
  monitoring:
    enabled: true
  security:
    containerSecurityContext:
      runAsUser: 1000
      readOnlyRootFilesystem: true
```

Both the `memcached` and `exporter` containers receive the same container
security context.

### No Security Contexts (Default)

```yaml
apiVersion: memcached.c5c3.io/v1alpha1
kind: Memcached
metadata:
  name: my-cache
spec:
  replicas: 3
```

No security contexts are set on the Deployment. The pod runs with Kubernetes
defaults.

---

## Runtime Behavior

| Action | Result |
|--------|--------|
| Set `spec.security.podSecurityContext` | Pod template `securityContext` is set |
| Set `spec.security.containerSecurityContext` | All containers get `securityContext` |
| Change security context values | Deployment updated with new values |
| Remove `spec.security` | Both pod and container security contexts cleared |
| Set `podSecurityContext` to nil | Pod security context cleared, container security context preserved |
| Set `containerSecurityContext` to nil | Container security contexts cleared, pod security context preserved |
| Reconcile twice with same spec | No Deployment update (idempotent) |
| External drift (manual removal) | Corrected on next reconciliation cycle |

---

## Implementation

The implementation adds two pure helper functions to
`internal/controller/deployment.go` and integrates them into
`constructDeployment`:

1. `buildPodSecurityContext` — returns `*corev1.PodSecurityContext` or nil
2. `buildContainerSecurityContext` — returns `*corev1.SecurityContext` or nil

In `constructDeployment`:
- `buildPodSecurityContext` result is assigned to `PodSpec.SecurityContext`
- `buildContainerSecurityContext` result is assigned to the memcached container's
  `SecurityContext` field
- The same container security context is applied to the exporter container after
  it is built by `buildExporterContainer`

No changes to the controller (`memcached_controller.go`) are needed — the
existing `reconcileDeployment` calls `constructDeployment` which now includes
security context logic.

No changes to the CRD types are needed — `SecuritySpec` already defines
`PodSecurityContext` and `ContainerSecurityContext` fields.
