# Architecture Overview

This document explains the architecture of the Memcached Operator, its design principles, how the reconciliation loop works, and its role within the CobaltCore ecosystem.

---

## CobaltCore Context

The Memcached Operator is part of [CobaltCore (C5C3)](https://github.com/c5c3/c5c3), a Kubernetes-native OpenStack distribution for operating Hosted Control Planes on bare-metal infrastructure.

CobaltCore follows a strict **operator-per-service architecture**: every component gets its own dedicated Kubernetes operator. The central `c5c3-operator` orchestrates the overall lifecycle by creating Custom Resources that the individual operators reconcile.

Memcached is one of four infrastructure services in the Control Plane Cluster, deployed in **Phase 1** of the dependency graph -- before any OpenStack service can start:

```text
Phase 1: Infrastructure    --> MariaDB, RabbitMQ, Valkey, Memcached
Phase 2: Identity          --> Keystone (depends: MariaDB, Memcached)
Phase 3: Service Catalog   --> K-ORC
Phase 4: Core              --> Glance, Placement
Phase 5: Compute           --> Nova, Neutron, Cinder
```

The primary consumer of Memcached is **Keystone**, which uses it for token caching via the `dogpile.cache.pymemcache` backend. Every Keystone instance connects to the individual Memcached pods by address (e.g., `memcached-0.memcached:11211, memcached-1.memcached:11211, ...`), which is why the operator provisions a **headless Service** (`clusterIP: None`) for direct pod discovery.

---

## Operator Architecture

```text
+---------------------------------------------------------------------+
|                        Kubernetes Cluster                           |
|                                                                     |
|  +--------------------------------------------------------------+   |
|  |                    Operator Manager                          |   |
|  |                                                              |   |
|  |  +---------------------+    +-----------------------------+  |   |
|  |  |  MemcachedReconciler|    |  Webhook Server             |  |   |
|  |  |                     |    |  - Defaulting               |  |   |
|  |  |  Watches:           |    |  - Validation               |  |   |
|  |  |  - Memcached CR     |    +-----------------------------+  |   |
|  |  |  - Deployments      |                                     |   |
|  |  |  - Services         |    +-----------------------------+  |   |
|  |  |  - PDBs             |    |  Metrics Server             |  |   |
|  |  |  - ServiceMonitors  |    |  :8443 /metrics             |  |   |
|  |  |  - NetworkPolicies  |    +-----------------------------+  |   |
|  |  +--------+------------+                                     |   |
|  +-----------|-------------------------------------------------+    |
|              | creates / updates / deletes                          |
|              v                                                      |
|  +--------------------------------------------------------------+   |
|  |                    Managed Resources                         |   |
|  |                                                              |   |
|  |  +--------------+  +----------+  +------+  +---------------+  |   |
|  |  | Deployment   |  | Service  |  | PDB  |  | ServiceMonitor|  |   |
|  |  |              |  | (headless|  |      |  |               |  |   |
|  |  | +----------+ |  | clusterIP|  |min:1 |  | prometheus    |  |   |
|  |  | |memcached | |  | :11211   |  |      |  | scrape config |  |   |
|  |  | |:11211    | |  +----------+  +------+  +---------------+  |   |
|  |  | +----------+ |                                              |   |
|  |  | |exporter  | |  +---------------+                           |   |
|  |  | |:9150     | |  | NetworkPolicy |                           |   |
|  |  | +----------+ |  |               |                           |   |
|  |  +--------------+  +---------------+                           |   |
|  +--------------------------------------------------------------+   |
+---------------------------------------------------------------------+
```

The operator runs as a single manager process in the cluster containing:

- **MemcachedReconciler** -- the core controller that watches the `Memcached` Custom Resource and all owned child resources, driving them toward the declared desired state.
- **Webhook Server** -- handles admission requests from the Kubernetes API server, providing defaulting (setting sensible defaults for omitted fields) and validation (enforcing complex cross-field constraints).
- **Metrics Server** -- exposes controller-runtime metrics on `:8443/metrics` for Prometheus scraping.

Leader election (ID: `d4f3c8a2.c5c3.io`) ensures only one instance of the operator is active at a time when running multiple replicas.

---

## Design Principles

### Declarative

Users describe the desired state through a `Memcached` Custom Resource. The operator continuously drives reality toward that state. Users never issue imperative commands -- they declare what they want and the operator figures out how to get there.

### Idempotent

Every reconciliation produces the same outcome regardless of how many times it runs. The operator uses the `controllerutil.CreateOrUpdate` pattern: it builds the desired state for each managed resource and applies it, creating the resource if it does not exist or updating it if it differs.

### Kubernetes-native

The operator follows standard Kubernetes API conventions:
- **Owner references** for automatic garbage collection of managed resources
- **Status conditions** (`Available`, `Progressing`, `Degraded`) following the standard condition format
- **ObservedGeneration** tracking to distinguish stale status from up-to-date status
- Standard labels (`app.kubernetes.io/name`, `app.kubernetes.io/instance`, `app.kubernetes.io/managed-by`)

### Level-triggered

Reconciliation reacts to the **current state**, not to a sequence of events. If the operator misses an event or is restarted, the next reconciliation will still converge to the correct state by comparing the declared desired state against the observed actual state. This makes the operator resilient to missed notifications, restarts, and race conditions.

---

## Watched Resources

The controller sets up watches for the following resources. All child resource watches are filtered to resources owned by a `Memcached` CR via owner references.

| Resource         | API Group               | Watch Reason                                         |
|------------------|-------------------------|------------------------------------------------------|
| `Memcached`      | `memcached.c5c3.io`     | Primary resource; triggers reconciliation on changes |
| `Deployment`     | `apps`                  | Detect drift, observe rollout status                 |
| `Service`        | core (`""`)             | Detect drift                                         |
| `PDB`            | `policy`                | Detect drift                                         |
| `ServiceMonitor` | `monitoring.coreos.com` | Detect drift (if monitoring enabled)                 |
| `NetworkPolicy`  | `networking.k8s.io`     | Detect drift (if network policy enabled)             |

When any watched resource changes, the controller enqueues the owning `Memcached` CR for reconciliation. This ensures that if someone manually edits a managed Deployment or Service, the operator will detect the drift and restore the desired state.

---

## Reconciliation Flow

The `MemcachedReconciler` follows a structured, sequential reconciliation flow that runs every time the state of a watched resource changes.

```text
Reconcile(ctx, req)
|
+-- 1. Fetch the Memcached CR
|   +-- Not found? --> return (deleted, nothing to do)
|   +-- Found --> continue
|
+-- 2. Set status condition: Progressing=True
|
+-- 3. Reconcile Deployment
|   +-- Build desired Deployment spec
|   +-- CreateOrUpdate (controllerutil)
|   |   +-- Create if not exists
|   |   +-- Update if spec differs
|   +-- Set owner reference on Deployment
|
+-- 4. Reconcile Service (headless, clusterIP: None)
|   +-- Build desired Service spec
|   +-- CreateOrUpdate
|   +-- Set owner reference
|
+-- 5. Reconcile PodDisruptionBudget (if HA PDB enabled)
|   +-- Build desired PDB spec
|   +-- CreateOrUpdate
|   +-- Set owner reference
|
+-- 6. Reconcile ServiceMonitor (if monitoring enabled)
|   +-- Check if ServiceMonitor CRD exists
|   +-- Build desired ServiceMonitor spec
|   +-- CreateOrUpdate
|   +-- Set owner reference
|
+-- 7. Reconcile NetworkPolicy (if security.networkPolicy enabled)
|   +-- Build desired NetworkPolicy spec
|   +-- CreateOrUpdate
|   +-- Set owner reference
|
+-- 8. Update Status
|   +-- Read Deployment status (readyReplicas)
|   +-- Set conditions: Available, Progressing, Degraded
|   +-- Set observedGeneration
|   +-- Patch status sub-resource
|
+-- 9. Return result
    +-- Requeue after 10s if Deployment not yet ready
```

### Create-or-Update Pattern

All managed resources follow the **create-or-update** pattern using `controllerutil.CreateOrUpdate`:

```go
dep := &appsv1.Deployment{
    ObjectMeta: metav1.ObjectMeta{
        Name:      mc.Name,
        Namespace: mc.Namespace,
    },
}

op, err := controllerutil.CreateOrUpdate(ctx, r.Client, dep, func() error {
    // Set the desired state on the resource
    r.constructDeployment(mc, dep)
    // Set owner reference for garbage collection
    return controllerutil.SetControllerReference(mc, dep, r.Scheme)
})
```

This pattern is idempotent: if the resource already exists and matches the desired state, the update is a no-op. If it differs, the resource is updated in place. If it does not exist, it is created.

---

## Managed Resources Detail

### Deployment

The operator creates a Deployment for each `Memcached` CR with the following characteristics:

- **Replicas**: Configured via `spec.replicas` (default: 1, range: 0-64)
- **Strategy**: `RollingUpdate` with `maxSurge=1` and `maxUnavailable=0` for zero-downtime updates
- **Memcached container**: Runs the Memcached server with command-line arguments derived from `spec.memcached` fields
- **Exporter sidecar** (optional): When `spec.monitoring.enabled` is `true`, a `prom/memcached-exporter` sidecar is injected, exposing metrics on port 9150
- **Health probes**:
  - Liveness: TCP socket on port 11211, `initialDelaySeconds=10`, `periodSeconds=10`
  - Readiness: TCP socket on port 11211, `initialDelaySeconds=5`, `periodSeconds=5`
- **Labels**: `app.kubernetes.io/name=memcached`, `app.kubernetes.io/instance=<name>`, `app.kubernetes.io/managed-by=memcached-operator`

### Headless Service

A headless Service (`clusterIP: None`) is created for each `Memcached` CR, exposing:

- Port 11211 (memcached) for client connections
- Port 9150 (metrics) for Prometheus scraping (when monitoring is enabled)

The headless Service enables direct pod discovery by DNS, which is critical for clients like Keystone's `pymemcache` that connect to individual pod addresses.

### PodDisruptionBudget

Created when `spec.highAvailability.podDisruptionBudget.enabled` is `true`. Supports both `minAvailable` and `maxUnavailable` configurations. The controller defaults `minAvailable` to 1 when neither field is set.

### ServiceMonitor

Created when `spec.monitoring.enabled` is `true` and the ServiceMonitor CRD exists in the cluster. The controller checks for CRD availability at reconciliation time and gracefully skips ServiceMonitor creation if the Prometheus Operator CRDs are not installed.

### NetworkPolicy

Created when `spec.security.networkPolicy.enabled` is `true`. Restricts ingress traffic to the Memcached port (11211) from only the specified `allowedSources`. When `allowedSources` is empty, all sources are allowed.

---

## Status Conditions

The operator uses standard Kubernetes conditions to communicate the state of a `Memcached` instance:

| Condition     | Meaning                                                 |
|---------------|---------------------------------------------------------|
| `Available`   | `True` when the Deployment has minimum availability     |
| `Progressing` | `True` when a rollout or scale operation is in progress |
| `Degraded`    | `True` when fewer replicas than desired are ready       |

Condition updates use `meta.SetStatusCondition` to handle `lastTransitionTime` correctly -- the transition time is only updated when the condition's status value actually changes.

The `status.observedGeneration` field is set to the CR's `metadata.generation` after each successful reconciliation, allowing clients to determine whether the status reflects the latest spec changes.

---

## Owner References and Garbage Collection

Every managed resource (Deployment, Service, PDB, ServiceMonitor, NetworkPolicy) gets a controller owner reference pointing back to the `Memcached` CR:

```yaml
ownerReferences:
  - apiVersion: memcached.c5c3.io/v1alpha1
    kind: Memcached
    name: my-cache
    controller: true
    blockOwnerDeletion: true
```

This enables Kubernetes garbage collection: when the `Memcached` CR is deleted, the garbage collector automatically deletes all owned resources. No finalizer is required for this basic cleanup.

---

## Requeue Strategy

The reconciler requeues to track rollout progress:

| Condition                | Requeue Interval | Reason                    |
|--------------------------|------------------|---------------------------|
| Deployment not yet ready | 10 seconds       | Poll for rollout progress |

---

## Labels

All managed resources are labeled with standard Kubernetes recommended labels:

| Label                          | Value                 |
|--------------------------------|-----------------------|
| `app.kubernetes.io/name`       | `memcached`           |
| `app.kubernetes.io/instance`   | `<Memcached CR name>` |
| `app.kubernetes.io/managed-by` | `memcached-operator`  |

These labels are used for:
- Deployment selector matching
- Service selector matching
- PDB selector matching
- Pod anti-affinity label selectors
- ServiceMonitor selector matching
- NetworkPolicy pod selectors
