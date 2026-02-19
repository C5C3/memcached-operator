# ServiceMonitor Reconciliation

Reference documentation for the Prometheus ServiceMonitor reconciliation logic
that enables automatic metrics discovery for Memcached pods.

**Source**: `internal/controller/servicemonitor.go`, `internal/controller/memcached_controller.go`

## Overview

When `spec.monitoring.enabled` is `true` and `spec.monitoring.serviceMonitor` is
set, the reconciler ensures a matching ServiceMonitor exists in the same
namespace with the same name as the Memcached CR. The ServiceMonitor is
constructed from the CR spec using a pure builder function, then applied via
`controllerutil.CreateOrUpdate` for idempotent create/update semantics. A
controller owner reference on the ServiceMonitor enables automatic garbage
collection when the Memcached CR is deleted.

The ServiceMonitor is opt-in — it requires both `monitoring.enabled: true` and
the `serviceMonitor` sub-section to be present.

---

## CRD Field Path

```
spec.monitoring.serviceMonitor
```

Defined in `api/v1alpha1/memcached_types.go` on the `ServiceMonitorSpec` struct:

```go
type ServiceMonitorSpec struct {
    AdditionalLabels map[string]string `json:"additionalLabels,omitempty,omitzero"`
    Interval         string            `json:"interval,omitempty"`
    ScrapeTimeout    string            `json:"scrapeTimeout,omitempty"`
}
```

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| `additionalLabels` | `map[string]string` | No | — | Extra labels merged into ServiceMonitor metadata |
| `interval` | `string` | No | `"30s"` | Prometheus scrape interval |
| `scrapeTimeout` | `string` | No | `"10s"` | Prometheus scrape timeout |

---

## ServiceMonitor Construction

`constructServiceMonitor(mc *Memcached, sm *ServiceMonitor)` sets the desired
state of the ServiceMonitor in-place. It is called within the
`controllerutil.CreateOrUpdate` mutate function so that both creation and updates
use identical logic.

### Default Values

The controller applies default values when the CR does not specify them:

| Field | Default |
|-------|---------|
| `interval` | `"30s"` |
| `scrapeTimeout` | `"10s"` |

### Labels

The ServiceMonitor labels are built by first copying `additionalLabels` from the
CR, then overlaying the standard Kubernetes recommended labels from
`labelsForMemcached(name)`. This merge strategy ensures standard labels always
take precedence and cannot be overridden by user-specified additional labels.

| Label Key | Value | Purpose |
|-----------|-------|---------|
| `app.kubernetes.io/name` | `memcached` | Identifies the application |
| `app.kubernetes.io/instance` | `<cr-name>` | Distinguishes instances of the same application |
| `app.kubernetes.io/managed-by` | `memcached-operator` | Identifies the managing controller |

Any labels in `additionalLabels` are merged alongside these standard labels.
If an additional label has the same key as a standard label, the standard label
wins.

### Selector

The ServiceMonitor selector uses the same label set as the Deployment's
`spec.selector.matchLabels`, ensuring Prometheus discovers the correct Service
endpoints:

```go
sm.Spec.Selector = metav1.LabelSelector{
    MatchLabels: labelsForMemcached(mc.Name),
}
```

A `namespaceSelector` restricts scraping to the CR's namespace:

```go
sm.Spec.NamespaceSelector = monitoringv1.NamespaceSelector{
    MatchNames: []string{mc.Namespace},
}
```

### Endpoint

The ServiceMonitor defines a single endpoint targeting the named port `metrics`
(port 9150 on the headless Service, exposed by the memcached-exporter sidecar):

```go
sm.Spec.Endpoints = []monitoringv1.Endpoint{
    {
        Port:          "metrics",
        Interval:      interval,
        ScrapeTimeout: scrapeTimeout,
    },
}
```

---

## Reconciliation Method

`reconcileServiceMonitor(ctx, mc *Memcached)` on `MemcachedReconciler` ensures
the ServiceMonitor matches the desired state:

```go
func (r *MemcachedReconciler) reconcileServiceMonitor(ctx context.Context, mc *memcachedv1alpha1.Memcached) error {
    if !serviceMonitorEnabled(mc) {
        return nil
    }

    sm := &monitoringv1.ServiceMonitor{
        ObjectMeta: metav1.ObjectMeta{
            Name:      mc.Name,
            Namespace: mc.Namespace,
        },
    }

    _, err := r.reconcileResource(ctx, mc, sm, func() error {
        constructServiceMonitor(mc, sm)
        return nil
    }, "ServiceMonitor")
    return err
}
```

### Skip Logic

The `serviceMonitorEnabled` guard returns `false` (skipping ServiceMonitor
reconciliation) when:

- `spec.monitoring` is nil
- `spec.monitoring.enabled` is `false`
- `spec.monitoring.serviceMonitor` is nil

All three conditions must be satisfied for reconciliation to proceed:
`monitoring != nil && monitoring.enabled == true && monitoring.serviceMonitor != nil`.

When ServiceMonitor is not enabled, `reconcileServiceMonitor` returns nil
immediately without error.

### Owner Reference

The `reconcileResource` helper calls `controllerutil.SetControllerReference`,
adding an owner reference to the ServiceMonitor's metadata:

| Field | Value |
|-------|-------|
| `apiVersion` | `memcached.c5c3.io/v1alpha1` |
| `kind` | `Memcached` |
| `name` | `<cr-name>` |
| `uid` | `<cr-uid>` |
| `controller` | `true` |
| `blockOwnerDeletion` | `true` |

This enables:
- **Garbage collection**: Deleting the Memcached CR automatically deletes the
  owned ServiceMonitor via Kubernetes' owner reference cascade.
- **Watch filtering**: The `Owns(&monitoringv1.ServiceMonitor{})` watch on the
  controller maps ServiceMonitor events back to the owning Memcached CR for
  reconciliation.

### Reconciliation Order

`reconcileServiceMonitor` is called between `reconcilePDB` and
`reconcileStatus` in the main `Reconcile` function. This ensures the Service
(with metrics port) exists before the ServiceMonitor is created.

---

## CR Examples

### ServiceMonitor with Defaults

```yaml
apiVersion: memcached.c5c3.io/v1alpha1
kind: Memcached
metadata:
  name: my-cache
  namespace: default
spec:
  replicas: 3
  monitoring:
    enabled: true
    serviceMonitor: {}
```

Produces:

```yaml
apiVersion: monitoring.coreos.com/v1
kind: ServiceMonitor
metadata:
  name: my-cache
  namespace: default
  labels:
    app.kubernetes.io/name: memcached
    app.kubernetes.io/instance: my-cache
    app.kubernetes.io/managed-by: memcached-operator
  ownerReferences:
    - apiVersion: memcached.c5c3.io/v1alpha1
      kind: Memcached
      name: my-cache
      controller: true
      blockOwnerDeletion: true
spec:
  selector:
    matchLabels:
      app.kubernetes.io/name: memcached
      app.kubernetes.io/instance: my-cache
      app.kubernetes.io/managed-by: memcached-operator
  namespaceSelector:
    matchNames:
      - default
  endpoints:
    - port: metrics
      interval: 30s
      scrapeTimeout: 10s
```

### ServiceMonitor with Custom Interval and Timeout

```yaml
spec:
  replicas: 3
  monitoring:
    enabled: true
    serviceMonitor:
      interval: "15s"
      scrapeTimeout: "5s"
```

Produces a ServiceMonitor with `endpoints[0].interval: 15s` and
`endpoints[0].scrapeTimeout: 5s`.

### ServiceMonitor with Additional Labels

```yaml
spec:
  replicas: 3
  monitoring:
    enabled: true
    serviceMonitor:
      additionalLabels:
        release: prometheus
        team: platform
```

Produces a ServiceMonitor with labels:

```yaml
labels:
  app.kubernetes.io/name: memcached
  app.kubernetes.io/instance: my-cache
  app.kubernetes.io/managed-by: memcached-operator
  release: prometheus
  team: platform
```

### Full Monitoring with All Features

```yaml
apiVersion: memcached.c5c3.io/v1alpha1
kind: Memcached
metadata:
  name: my-cache
  namespace: default
spec:
  replicas: 3
  monitoring:
    enabled: true
    serviceMonitor:
      interval: "15s"
      scrapeTimeout: "5s"
      additionalLabels:
        release: prometheus
  highAvailability:
    podDisruptionBudget:
      enabled: true
    antiAffinity:
      type: preferred
```

Produces all expected resources: Deployment with exporter sidecar, Service with
metrics port, PDB, and ServiceMonitor.

### Monitoring Disabled (Default)

```yaml
apiVersion: memcached.c5c3.io/v1alpha1
kind: Memcached
metadata:
  name: my-cache
spec:
  replicas: 3
```

No ServiceMonitor is created. The `reconcileServiceMonitor` method returns
immediately.

---

## Runtime Behavior

| Action | Result |
|--------|--------|
| Enable monitoring with `serviceMonitor: {}` | ServiceMonitor created with defaults (`interval: 30s`, `scrapeTimeout: 10s`) on next reconcile |
| Set `interval: "15s"` | ServiceMonitor endpoint updated on next reconcile |
| Set `scrapeTimeout: "5s"` | ServiceMonitor endpoint updated on next reconcile |
| Add `additionalLabels` | Labels merged into ServiceMonitor metadata on next reconcile |
| Override standard label in `additionalLabels` | Standard label preserved (takes precedence) |
| Disable monitoring (`enabled: false`) | ServiceMonitor reconciliation skipped; existing ServiceMonitor persists until CR is deleted |
| Remove `monitoring` section | ServiceMonitor reconciliation skipped; existing ServiceMonitor persists until CR is deleted |
| Remove `serviceMonitor` sub-section | ServiceMonitor reconciliation skipped; existing ServiceMonitor persists until CR is deleted |
| Delete Memcached CR | ServiceMonitor deleted via garbage collection (owner reference) |
| Reconcile twice with same spec | No ServiceMonitor update (idempotent) |
| External drift (manual ServiceMonitor edit) | Corrected on next reconciliation cycle |

---

## Implementation

The `constructServiceMonitor` function in `internal/controller/servicemonitor.go`
is a pure function that sets ServiceMonitor desired state in-place:

```go
func constructServiceMonitor(mc *Memcached, sm *ServiceMonitor)
```

- Builds labels by copying `additionalLabels` first, then overlaying standard
  labels from `labelsForMemcached` (standard labels always win)
- Sets `spec.selector.matchLabels` using `labelsForMemcached`
- Sets `spec.namespaceSelector` to the CR's namespace
- Configures a single endpoint targeting port `metrics` with the configured
  or default `interval` and `scrapeTimeout`

The `serviceMonitorEnabled` function is a pure guard:

```go
func serviceMonitorEnabled(mc *Memcached) bool
```

- Returns `false` when `spec.monitoring` is nil
- Returns `false` when `spec.monitoring.enabled` is `false`
- Returns `false` when `spec.monitoring.serviceMonitor` is nil
- Returns `true` only when all three conditions are satisfied
