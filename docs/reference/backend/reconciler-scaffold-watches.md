# MemcachedReconciler Scaffold and Watch Setup

Reference documentation for the `MemcachedReconciler` struct, its watch
configuration via `SetupWithManager`, and the RBAC markers that generate the
operator's `ClusterRole`.

**Source**: `internal/controller/memcached_controller.go`

## MemcachedReconciler Struct

The reconciler struct embeds the controller-runtime client and carries the
runtime scheme and an event recorder:

```go
type MemcachedReconciler struct {
    client.Client
    Scheme   *runtime.Scheme
    Recorder events.EventRecorder
}
```

| Field      | Type                    | Description                                                                 |
|------------|-------------------------|-----------------------------------------------------------------------------|
| `Client`   | `client.Client`         | Embedded controller-runtime client for reading/writing Kubernetes objects    |
| `Scheme`   | `*runtime.Scheme`       | Runtime scheme for GVK resolution (serialization, deserialization, watches)  |
| `Recorder` | `events.EventRecorder`  | Kubernetes event recorder for emitting events on Memcached CRs              |

The struct satisfies the `reconcile.Reconciler` interface by implementing the
`Reconcile(ctx, ctrl.Request) (ctrl.Result, error)` method.

### Initialization in `cmd/main.go`

The reconciler is created and registered in the operator's main function:

```go
if err = (&controller.MemcachedReconciler{
    Client:   mgr.GetClient(),
    Scheme:   mgr.GetScheme(),
    Recorder: mgr.GetEventRecorder("memcached-controller"),
}).SetupWithManager(mgr); err != nil {
    setupLog.Error(err, "unable to create controller", "controller", "Memcached")
    os.Exit(1)
}
```

- `mgr.GetClient()` returns a cached client that reads from the informer cache
  and writes directly to the API server.
- `mgr.GetScheme()` returns the scheme with both core Kubernetes types and
  `memcached.c5c3.io/v1alpha1` types registered.
- `mgr.GetEventRecorder("memcached-controller")` creates an event recorder
  that tags events with source component `memcached-controller`.

---

## Reconcile Method

The `Reconcile` method is the entry point for each reconciliation cycle. In the
current scaffold, it fetches the Memcached CR and handles the not-found case:

```go
func (r *MemcachedReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
    logger := log.FromContext(ctx)

    memcached := &memcachedv1alpha1.Memcached{}
    if err := r.Get(ctx, req.NamespacedName, memcached); err != nil {
        if apierrors.IsNotFound(err) {
            logger.Info("Memcached resource not found; ignoring since it must have been deleted")
            return ctrl.Result{}, nil
        }
        logger.Error(err, "Failed to get Memcached resource")
        return ctrl.Result{}, err
    }

    logger.Info("Reconciling Memcached", "name", memcached.Name, "namespace", memcached.Namespace)
    return ctrl.Result{}, nil
}
```

### Behavior

| Scenario                           | Return Value              | Effect                          |
|------------------------------------|---------------------------|---------------------------------|
| CR not found (deleted)             | `ctrl.Result{}, nil`     | No requeue, no error logged     |
| CR fetch fails (non-NotFound)      | `ctrl.Result{}, err`     | Requeue with exponential backoff|
| CR fetched successfully            | `ctrl.Result{}, nil`     | Log reconciliation, no requeue  |

The scaffold returns an empty result after a successful fetch. Subsequent
features (MO-0005 through MO-0014) add reconciliation logic for Deployments,
Services, PDBs, NetworkPolicies, status updates, and ServiceMonitors.

---

## Watch Configuration

`SetupWithManager` registers the controller with the manager and configures
which Kubernetes resources trigger reconciliation:

```go
func (r *MemcachedReconciler) SetupWithManager(mgr ctrl.Manager) error {
    return ctrl.NewControllerManagedBy(mgr).
        For(&memcachedv1alpha1.Memcached{}).
        Owns(&appsv1.Deployment{}).
        Owns(&corev1.Service{}).
        Owns(&policyv1.PodDisruptionBudget{}).
        Owns(&networkingv1.NetworkPolicy{}).
        Named("memcached").
        Complete(r)
}
```

### Watch Types

| Method | Resource Type                   | API Group            | Trigger Condition                                                         |
|--------|---------------------------------|----------------------|---------------------------------------------------------------------------|
| `For`  | `Memcached`                     | `memcached.c5c3.io`  | Any create, update, or delete of a Memcached CR                           |
| `Owns` | `Deployment`                    | `apps`               | Changes to Deployments with an owner reference pointing to a Memcached CR |
| `Owns` | `Service`                       | (core)               | Changes to Services with an owner reference pointing to a Memcached CR    |
| `Owns` | `PodDisruptionBudget`           | `policy`             | Changes to PDBs with an owner reference pointing to a Memcached CR        |
| `Owns` | `NetworkPolicy`                 | `networking.k8s.io`  | Changes to NetworkPolicies with an owner reference pointing to a Memcached CR |

### How `For` Works

`For(&memcachedv1alpha1.Memcached{})` registers the Memcached CRD as the
primary watched resource. Any create, update, or delete event on a Memcached CR
enqueues a `ctrl.Request` with the CR's `NamespacedName` for reconciliation.

### How `Owns` Works

`Owns` registers a watch on a secondary resource type and automatically:

1. Filters events to only those resources that have an
   [`ownerReference`][owner-ref] pointing to a Memcached CR.
2. Maps the event to the owning Memcached CR's `NamespacedName`, so
   `Reconcile` receives the owner's name — not the owned resource's name.

This means:
- A Deployment created by the reconciler with `controllerutil.SetControllerReference`
  will trigger reconciliation of the owning Memcached CR if modified externally.
- A Deployment without a Memcached owner reference is ignored entirely.

### ServiceMonitor Watch

The `ServiceMonitor` type is intentionally **not** included in `Owns()` in this
scaffold. The ServiceMonitor CRD (`monitoring.coreos.com`) may not be installed
on all clusters, and creating an `Owns` watch for a non-existent CRD would
cause `SetupWithManager` to fail. The ServiceMonitor watch is added in MO-0014
with conditional CRD detection.

---

## RBAC Markers

Kubebuilder RBAC markers on the reconciler generate the `ClusterRole` rules in
`config/rbac/role.yaml` when `make manifests` is run.

### Marker Definitions

```go
// +kubebuilder:rbac:groups=memcached.c5c3.io,resources=memcacheds,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=memcached.c5c3.io,resources=memcacheds/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=memcached.c5c3.io,resources=memcacheds/finalizers,verbs=update
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=services,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=policy,resources=poddisruptionbudgets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=networking.k8s.io,resources=networkpolicies,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=monitoring.coreos.com,resources=servicemonitors,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=events,verbs=create;patch
```

### Generated ClusterRole Rules

The markers produce the following rules in `config/rbac/role.yaml`:

| API Group                | Resource                | Verbs                                              | Purpose                                  |
|--------------------------|-------------------------|----------------------------------------------------|------------------------------------------|
| `memcached.c5c3.io`      | `memcacheds`           | get, list, watch, create, update, patch, delete    | Full CRUD on Memcached CRs               |
| `memcached.c5c3.io`      | `memcacheds/status`    | get, update, patch                                 | Status subresource updates               |
| `memcached.c5c3.io`      | `memcacheds/finalizers`| update                                             | Finalizer management                     |
| `apps`                   | `deployments`          | get, list, watch, create, update, patch, delete    | Manage Memcached Deployments             |
| (core)                   | `services`             | get, list, watch, create, update, patch, delete    | Manage headless Services                 |
| `policy`                 | `poddisruptionbudgets` | get, list, watch, create, update, patch, delete    | Manage PodDisruptionBudgets              |
| `networking.k8s.io`      | `networkpolicies`      | get, list, watch, create, update, patch, delete    | Manage NetworkPolicies                   |
| `monitoring.coreos.com`  | `servicemonitors`      | get, list, watch, create, update, patch, delete    | Manage Prometheus ServiceMonitors        |
| (core)                   | `events`               | create, patch                                      | Emit Kubernetes events                   |

### Why RBAC Markers Are Added Now

RBAC markers for all resource types (including ServiceMonitors, which are
reconciled in MO-0014) are declared in this scaffold because:

1. The `ClusterRole` must be complete before the operator is deployed. Adding
   permissions incrementally would require redeploying the operator after each
   feature.
2. Missing RBAC permissions cause runtime `Forbidden` errors that are difficult
   to debug.
3. The markers serve as documentation of the full set of resources the operator
   will manage.

---

## Reconciliation Flow Diagram

```
                     ┌──────────────────────────────┐
                     │     Kubernetes API Server     │
                     └──────┬───────────┬───────────┘
                            │           │
              Watch events  │           │  Watch events
              (Memcached)   │           │  (Owned resources)
                            ▼           ▼
                     ┌──────────────────────────────┐
                     │    controller-runtime cache   │
                     │    (informer per GVK)         │
                     └──────────────┬───────────────┘
                                    │
                                    │  Enqueue ctrl.Request
                                    │  {Namespace, Name}
                                    ▼
                     ┌──────────────────────────────┐
                     │        Work Queue            │
                     └──────────────┬───────────────┘
                                    │
                                    │  Dequeue
                                    ▼
                     ┌──────────────────────────────┐
                     │  MemcachedReconciler.Reconcile│
                     │                              │
                     │  1. Fetch Memcached CR        │
                     │  2. If NotFound → return nil  │
                     │  3. If error → return error   │
                     │  4. (future: reconcile owned  │
                     │      resources)               │
                     └──────────────────────────────┘
```

**Event-to-reconcile mapping:**

| Event Source            | Event Type       | Reconciled Object         |
|-------------------------|------------------|---------------------------|
| Memcached CR            | Create/Update/Delete | The Memcached CR itself |
| Owned Deployment        | Create/Update/Delete | The owning Memcached CR |
| Owned Service           | Create/Update/Delete | The owning Memcached CR |
| Owned PDB               | Create/Update/Delete | The owning Memcached CR |
| Owned NetworkPolicy     | Create/Update/Delete | The owning Memcached CR |
| Unowned Deployment      | Any              | (not reconciled)          |

[owner-ref]: https://kubernetes.io/docs/concepts/overview/working-with-objects/owners-dependents/
