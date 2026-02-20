# Operator Metrics Server

Reference documentation for the operator's Prometheus metrics endpoint, including
custom metrics for reconciliation monitoring and Memcached instance observability.

**Source**: `internal/metrics/metrics.go`, `internal/controller/memcached_controller.go`, `internal/controller/reconcile_resource.go`, `cmd/main.go`

## Overview

The memcached-operator exposes a Prometheus-compatible metrics endpoint at
`:8443/metrics` served by the controller-runtime metrics server. The endpoint
provides both standard controller-runtime metrics and custom operator metrics
defined in the `internal/metrics` package.

Custom metrics are registered with the controller-runtime metrics registry
(`sigs.k8s.io/controller-runtime/pkg/metrics.Registry`), not the global default
Prometheus registerer. The controller-runtime metrics server automatically serves
all metrics from this registry.

---

## Metrics Endpoint Configuration

### Flags

| Flag                     | Default | Description                                                                                              |
|--------------------------|---------|----------------------------------------------------------------------------------------------------------|
| `--metrics-bind-address` | `0`     | Address the metrics endpoint binds to. Use `:8443` for HTTPS or `:8080` for HTTP. Set to `0` to disable. |
| `--metrics-secure`       | `true`  | Serve the metrics endpoint via HTTPS with authentication and authorization.                              |
| `--enable-http2`         | `false` | Enable HTTP/2 for the metrics server. When disabled, TLS is restricted to HTTP/1.1.                      |

### Production Deployment

The default kustomize patch (`config/default/manager_metrics_patch.yaml`) sets:

```yaml
args:
  - --metrics-bind-address=:8443
  - --metrics-secure
```

### Disabling the Metrics Server

When `--metrics-bind-address=0`, no HTTP server listens on any metrics port.
Custom metrics are still registered (no panic) but not served. The operator
starts and reconciles normally without metrics serving.

### Authentication and Authorization

When `--metrics-secure` is set, the metrics endpoint uses controller-runtime's
`filters.WithAuthenticationAndAuthorization` filter, which delegates to the
Kubernetes API server for token review and subject access review.

Required RBAC resources:

| Resource                                      | File                                               | Purpose                                                       |
|-----------------------------------------------|----------------------------------------------------|---------------------------------------------------------------|
| `ClusterRole/metrics-auth-role`               | `config/rbac/metrics_auth_role.yaml`               | Grants `create` on `tokenreviews` and `subjectaccessreviews`  |
| `ClusterRoleBinding/metrics-auth-rolebinding` | `config/rbac/metrics_auth_role_binding.yaml`       | Binds the auth role to the controller-manager ServiceAccount  |
| `ClusterRole/metrics-reader`                  | `config/rbac/metrics_reader_role.yaml`             | Grants `get` on the `/metrics` non-resource URL               |
| `NetworkPolicy/allow-metrics-traffic`         | `config/network-policy/allow-metrics-traffic.yaml` | Allows ingress TCP traffic on port 8443 to the controller pod |

### Operator ServiceMonitor

The operator itself is scraped via a ServiceMonitor defined in
`config/prometheus/monitor.yaml`:

```yaml
apiVersion: monitoring.coreos.com/v1
kind: ServiceMonitor
metadata:
  name: controller-manager-metrics-monitor
spec:
  endpoints:
    - path: /metrics
      port: https
      scheme: https
      bearerTokenFile: /var/run/secrets/kubernetes.io/serviceaccount/token
      tlsConfig:
        insecureSkipVerify: true
  selector:
    matchLabels:
      control-plane: controller-manager
```

---

## Custom Metrics

All custom metrics are defined in `internal/metrics/metrics.go` and use a
consistent `memcached_operator_` prefix. They are registered in an `init()` function
via `ctrlmetrics.Registry.MustRegister`.

### memcached_operator_reconcile_resource_total

Per-resource-kind reconciliation outcome counter.

| Property | Value                                                     |
|----------|-----------------------------------------------------------|
| Type     | Counter                                                   |
| Help     | `Total number of per-resource reconciliation operations.` |
| Labels   | `resource_kind`, `result`                                 |

**Labels:**

| Label           | Values                                                           | Description                                   |
|-----------------|------------------------------------------------------------------|-----------------------------------------------|
| `resource_kind` | `Deployment`, `Service`, `PodDisruptionBudget`, `ServiceMonitor` | The Kubernetes resource kind being reconciled |
| `result`        | `created`, `updated`, `unchanged`                                | The outcome of the `CreateOrUpdate` call      |

**Instrumentation point**: Incremented in `reconcileResource()` after a
successful `controllerutil.CreateOrUpdate` call. Not incremented on error.
The `OperationResultNone` value is mapped to `"unchanged"`.

**Recording function**:

```go
metrics.RecordReconcileResource(resourceKind, result string)
```

### memcached_operator_reconcile_total

Per-instance reconciliation counter.

| Property | Value                                        |
|----------|----------------------------------------------|
| Type     | Counter                                      |
| Help     | `Total number of Memcached reconciliations.` |
| Labels   | `name`, `namespace`, `result`                |

**Labels:**

| Label       | Description                                      |
|-------------|--------------------------------------------------|
| `name`      | Memcached CR name                                |
| `namespace` | Memcached CR namespace                           |
| `result`    | Reconciliation outcome (e.g. `success`, `error`) |

**Instrumentation point**: Incremented in `Reconcile()` at the end of a
successful reconciliation (`"success"`) or on any sub-reconciler error
(`"error"`).

### memcached_operator_reconcile_duration_seconds

Per-instance reconciliation duration histogram.

| Property | Value                                                                           |
|----------|---------------------------------------------------------------------------------|
| Type     | Histogram                                                                       |
| Help     | `Duration of Memcached reconciliation in seconds.`                              |
| Labels   | `name`, `namespace`                                                             |
| Buckets  | Prometheus default buckets (`.005, .01, .025, .05, .1, .25, .5, 1, 2.5, 5, 10`) |

**Labels:**

| Label       | Description            |
|-------------|------------------------|
| `name`      | Memcached CR name      |
| `namespace` | Memcached CR namespace |

**Instrumentation point**: Observed in `Reconcile()` alongside
`memcached_operator_reconcile_total`. Duration is measured from after the CR
fetch until the reconciliation completes (success or error).

**Recording function** (records both counter and histogram):

```go
metrics.RecordReconciliation(name, namespace, result string, duration time.Duration)
```

### memcached_operator_instance_info

Info-style gauge for active Memcached instances.

| Property | Value                                     |
|----------|-------------------------------------------|
| Type     | Gauge                                     |
| Help     | `Information about a Memcached instance.` |
| Labels   | `name`, `namespace`, `image`              |
| Value    | Always `1` when the instance exists       |

**Labels:**

| Label       | Description                                   |
|-------------|-----------------------------------------------|
| `name`      | Memcached CR name                             |
| `namespace` | Memcached CR namespace                        |
| `image`     | Container image in use (e.g. `memcached:1.6`) |

**Instrumentation point**: Set in `Reconcile()` after successfully fetching the
Memcached CR. When the image changes, the old series is cleaned up via
`DeletePartialMatch` before setting the new label combination.

**Recording function**:

```go
metrics.RecordInstanceInfo(name, namespace, image string, replicas int32)
```

This also sets `memcached_operator_instance_replicas_desired`.

### memcached_operator_instance_replicas_desired

Desired replica count per Memcached instance.

| Property | Value                                                  |
|----------|--------------------------------------------------------|
| Type     | Gauge                                                  |
| Help     | `Desired number of replicas for a Memcached instance.` |
| Labels   | `name`, `namespace`                                    |

**Labels:**

| Label       | Description            |
|-------------|------------------------|
| `name`      | Memcached CR name      |
| `namespace` | Memcached CR namespace |

**Instrumentation point**: Set in `Reconcile()` alongside `memcached_operator_instance_info`
via `RecordInstanceInfo`.

### memcached_operator_instance_replicas_ready

Ready replica count per Memcached instance.

| Property | Value                                                |
|----------|------------------------------------------------------|
| Type     | Gauge                                                |
| Help     | `Number of ready replicas for a Memcached instance.` |
| Labels   | `name`, `namespace`                                  |

**Labels:**

| Label       | Description            |
|-------------|------------------------|
| `name`      | Memcached CR name      |
| `namespace` | Memcached CR namespace |

**Instrumentation point**: Set in `Reconcile()` after `reconcileStatus` succeeds,
using the CR's `status.readyReplicas` field.

**Recording function**:

```go
metrics.RecordReadyReplicas(name, namespace string, ready int32)
```

---

## Metric Cleanup on CR Deletion

When a Memcached CR is deleted (the `Get` call returns `IsNotFound`), all metric
series associated with that instance are removed:

```go
metrics.ResetInstanceMetrics(req.Name, req.Namespace)
```

`ResetInstanceMetrics` uses `DeletePartialMatch` to remove all series matching
the `name` and `namespace` labels from:

- `memcached_operator_instance_info`
- `memcached_operator_instance_replicas_desired`
- `memcached_operator_instance_replicas_ready`
- `memcached_operator_reconcile_total`
- `memcached_operator_reconcile_duration_seconds`

The `memcached_operator_reconcile_resource_total` counter is not cleaned up on deletion
because it tracks per-resource-kind totals, not per-CR state.

---

## Standard Controller-Runtime Metrics

The controller-runtime framework automatically registers and serves the following
metrics (non-exhaustive):

| Metric                                      | Type      | Description                                                                                 |
|---------------------------------------------|-----------|---------------------------------------------------------------------------------------------|
| `controller_runtime_reconcile_total`        | Counter   | Total number of reconciliations per controller, with `result` label (success/error/requeue) |
| `controller_runtime_reconcile_errors_total` | Counter   | Total number of reconciliation errors per controller                                        |
| `controller_runtime_reconcile_time_seconds` | Histogram | Duration of reconciliation per controller                                                   |
| `workqueue_depth`                           | Gauge     | Current depth of the work queue                                                             |
| `workqueue_adds_total`                      | Counter   | Total number of adds to the work queue                                                      |
| `workqueue_queue_duration_seconds`          | Histogram | Time an item stays in the work queue                                                        |
| `workqueue_work_duration_seconds`           | Histogram | Time spent processing an item from the work queue                                           |
| `workqueue_retries_total`                   | Counter   | Total number of retries handled by the work queue                                           |

These metrics are served alongside custom metrics from the same registry and
endpoint.

---

## Metrics Package Architecture

The `internal/metrics` package is structured for separation of concerns:

- **Metric definitions** — package-level `var` block with `prometheus.NewCounterVec`,
  `prometheus.NewGaugeVec`, and `prometheus.NewHistogramVec` constructors
- **Registration** — `init()` function registers all metrics with the
  controller-runtime registry
- **Recording functions** — exported functions (`RecordReconcileResource`,
  `RecordReconciliation`, `RecordInstanceInfo`, `RecordReadyReplicas`,
  `ResetInstanceMetrics`) that accept typed parameters instead of raw label
  strings
- **Test access** — `registry()` exposes the metrics registry as a
  `prometheus.Gatherer` for test assertions

Adding a new custom metric requires:

1. Define the metric variable in the `var` block
2. Add it to the `MustRegister` call in `init()`
3. Create a recording function with typed parameters
4. Instrument the appropriate controller method

---

## Cardinality

| Metric                                          | Cardinality Bound           | Notes                                     |
|-------------------------------------------------|-----------------------------|-------------------------------------------|
| `memcached_operator_reconcile_resource_total`   | O(resource_kinds × results) | Fixed at ~12 series (4 kinds × 3 results) |
| `memcached_operator_reconcile_total`            | O(CRs × results)            | Scales with number of Memcached CRs       |
| `memcached_operator_reconcile_duration_seconds` | O(CRs)                      | Scales with number of Memcached CRs       |
| `memcached_operator_instance_info`              | O(CRs)                      | One series per active CR                  |
| `memcached_operator_instance_replicas_desired`  | O(CRs)                      | One series per active CR                  |
| `memcached_operator_instance_replicas_ready`    | O(CRs)                      | One series per active CR                  |

All per-CR metrics scale linearly with the number of Memcached CRs. For an
operator managing tens-to-hundreds of CRs, this is well within acceptable bounds.

---

## PromQL Examples

### Reconciliation rate by resource kind

```promql
rate(memcached_operator_reconcile_resource_total[5m])
```

### Creation storm detection (>10 creations/min)

```promql
rate(memcached_operator_reconcile_resource_total{result="created"}[5m]) * 60 > 10
```

### Desired vs ready replica mismatch

```promql
memcached_operator_instance_replicas_desired - memcached_operator_instance_replicas_ready > 0
```

### Fleet overview — all active instances

```promql
memcached_operator_instance_info == 1
```

### P99 reconciliation latency

```promql
histogram_quantile(0.99, rate(memcached_operator_reconcile_duration_seconds_bucket[5m]))
```

### Under-replicated instances alert

```promql
(memcached_operator_instance_replicas_desired - memcached_operator_instance_replicas_ready) > 0
  and on(name, namespace) memcached_operator_instance_info == 1
```
