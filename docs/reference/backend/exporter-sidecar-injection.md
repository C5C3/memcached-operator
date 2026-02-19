# Exporter Sidecar Injection

Reference documentation for the Memcached exporter sidecar injection feature
that automatically adds a `prom/memcached-exporter` container to the Deployment
and a metrics port to the headless Service when monitoring is enabled.

**Source**: `internal/controller/deployment.go`, `internal/controller/service.go`, `api/v1alpha1/memcached_types.go`

## Overview

When `spec.monitoring.enabled` is `true`, the reconciler injects a
[memcached-exporter](https://github.com/prometheus/memcached_exporter) sidecar
container into the Deployment pod template. The exporter connects to the
co-located memcached process on `localhost:11211` and exposes Prometheus metrics
on port 9150.

The feature configures two resources:

1. **Deployment** — a second container named `exporter` is appended to the pod
   template alongside the `memcached` container.
2. **Service** — a `metrics` port (9150/TCP) is added to the headless Service,
   enabling Prometheus ServiceMonitor discovery.

The feature is opt-in — no sidecar or metrics port is added unless
`spec.monitoring.enabled` is explicitly set to `true`.

---

## CRD Field Path

```
spec.monitoring
```

Defined in `api/v1alpha1/memcached_types.go` on the `MonitoringSpec` struct:

```go
type MonitoringSpec struct {
    Enabled           bool                         `json:"enabled,omitempty"`
    ExporterImage     *string                      `json:"exporterImage,omitempty,omitzero"`
    ExporterResources *corev1.ResourceRequirements `json:"exporterResources,omitempty,omitzero"`
    ServiceMonitor    *ServiceMonitorSpec           `json:"serviceMonitor,omitempty,omitzero"`
}
```

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| `enabled` | `bool` | No | `false` | Controls whether the exporter sidecar is injected |
| `exporterImage` | `*string` | No | `prom/memcached-exporter:v0.15.4` | Container image for the exporter sidecar |
| `exporterResources` | `*ResourceRequirements` | No | empty (no limits) | Resource requests and limits for the exporter container |
| `serviceMonitor` | `*ServiceMonitorSpec` | No | nil | Prometheus ServiceMonitor configuration (separate feature) |

---

## Exporter Container Construction

`buildExporterContainer(mc *Memcached)` returns a fully configured exporter
container, or `nil` if monitoring is not enabled.

### Skip Logic

`buildExporterContainer` returns `nil` when:

- `spec.monitoring` is nil
- `spec.monitoring.enabled` is `false`

### Container Specification

When enabled, the exporter container has the following configuration:

| Property | Value |
|----------|-------|
| Name | `exporter` |
| Image | `spec.monitoring.exporterImage` or `prom/memcached-exporter:v0.15.4` |
| Port | `9150/TCP` named `metrics` |
| Resources | `spec.monitoring.exporterResources` or empty |
| Memcached address | `localhost:11211` (exporter default, no explicit args) |
| Lifecycle hooks | None |
| Probes | None |

The exporter connects to memcached via `localhost:11211` using the exporter's
built-in default `--memcached.address` flag. Since both containers share the
same pod network namespace, no explicit argument is needed.

No liveness or readiness probes are configured on the exporter container. The
memcached container's probes cover the primary process, and the exporter is a
lightweight HTTP server that starts immediately.

---

## Service Port

When monitoring is enabled, `constructService` appends a second port to the
headless Service:

```yaml
- name: metrics
  port: 9150
  targetPort: metrics
  protocol: TCP
```

The `targetPort` references the container port by name (`metrics`), ensuring
correct routing regardless of port number changes.

When monitoring is disabled or nil, the Service has only the `memcached` port
(11211/TCP).

---

## CR Examples

### Enabled with Defaults

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
```

Produces a Deployment with two containers:

```yaml
spec:
  template:
    spec:
      containers:
        - name: memcached
          image: memcached:1.6
          ports:
            - name: memcached
              containerPort: 11211
              protocol: TCP
        - name: exporter
          image: prom/memcached-exporter:v0.15.4
          ports:
            - name: metrics
              containerPort: 9150
              protocol: TCP
```

And a Service with two ports:

```yaml
spec:
  ports:
    - name: memcached
      port: 11211
      targetPort: memcached
      protocol: TCP
    - name: metrics
      port: 9150
      targetPort: metrics
      protocol: TCP
```

### Custom Exporter Image

```yaml
spec:
  replicas: 3
  monitoring:
    enabled: true
    exporterImage: my-registry.example.com/memcached-exporter:v0.16.0
```

The exporter container uses the specified image instead of the default.

### With Resource Limits

```yaml
spec:
  replicas: 3
  monitoring:
    enabled: true
    exporterResources:
      requests:
        cpu: 50m
        memory: 32Mi
      limits:
        cpu: 100m
        memory: 64Mi
```

Produces an exporter container with the specified resource constraints.

### Disabled (Default)

```yaml
apiVersion: memcached.c5c3.io/v1alpha1
kind: Memcached
metadata:
  name: my-cache
spec:
  replicas: 3
```

No exporter sidecar is injected. The Deployment has a single `memcached`
container. The Service has only the `memcached` port (11211/TCP).

### Combined with Other Features

```yaml
spec:
  replicas: 3
  monitoring:
    enabled: true
    exporterResources:
      requests:
        cpu: 50m
        memory: 32Mi
  highAvailability:
    antiAffinityPreset: soft
    gracefulShutdown:
      enabled: true
      preStopDelaySeconds: 10
      terminationGracePeriodSeconds: 30
    podDisruptionBudget:
      enabled: true
      minAvailable: 1
```

All features are applied independently and coexist without conflicts. The
exporter sidecar does not have lifecycle hooks — only the memcached container
receives the preStop hook from graceful shutdown.

---

## Runtime Behavior

| Action | Result |
|--------|--------|
| Enable monitoring (`enabled: true`) | Exporter sidecar added to Deployment; metrics port added to Service |
| Set `exporterImage` | Exporter container uses the specified image |
| Change `exporterImage` | Deployment updated with new exporter image |
| Set `exporterResources` | Exporter container uses the specified resource requests/limits |
| Change `exporterResources` | Deployment updated with new resource configuration |
| Disable monitoring (`enabled: false`) | Exporter container removed from Deployment; metrics port removed from Service |
| Remove `monitoring` section | Same as disabled — sidecar and metrics port removed |
| Reconcile twice with same spec | No Deployment or Service update (idempotent) |
| External drift (manual container removal) | Corrected on next reconciliation cycle |

---

## Implementation

The `buildExporterContainer` function in `internal/controller/deployment.go` is
a pure function that returns a container definition:

```go
func buildExporterContainer(mc *Memcached) *corev1.Container
```

- Returns `nil` when monitoring is not enabled or `spec.monitoring` is nil
- Returns a `*corev1.Container` with name `exporter`, the resolved image, port
  9150/TCP, and optional resource requirements
- Called from `constructDeployment`, which appends the container to the
  containers slice when non-nil

The function follows the same pattern as `buildAntiAffinity` and
`buildGracefulShutdown`: a pure builder function with no side effects, tested
independently via unit tests.

The Service metrics port is added inline in `constructService` using the same
nil-guard check (`mc.Spec.Monitoring != nil && mc.Spec.Monitoring.Enabled`).
