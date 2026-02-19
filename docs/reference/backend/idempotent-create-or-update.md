# Idempotent Create-or-Update Pattern

Reference documentation for the generic `reconcileResource` helper that provides
idempotent create-or-update semantics with conflict retries and event emission
for all managed Kubernetes resources.

**Source**: `internal/controller/reconcile_resource.go`

## Overview

Every managed resource (Deployment, Service, and future resource types) is
reconciled through a single `reconcileResource` method on `MemcachedReconciler`.
This method wraps `controllerutil.CreateOrUpdate` with:

- **Controller owner reference** set automatically on every resource
- **Conflict retry loop** for HTTP 409 errors (up to 5 attempts)
- **Structured logging** distinguishing Created, Updated, and Unchanged operations
- **Kubernetes event emission** for Created and Updated operations

This ensures consistent, idempotent behavior across all resource types and
eliminates duplicated create-or-update boilerplate.

---

## Function Signature

```go
func (r *MemcachedReconciler) reconcileResource(
    ctx          context.Context,
    mc           *memcachedv1alpha1.Memcached,
    obj          client.Object,
    mutate       func() error,
    resourceKind string,
) (controllerutil.OperationResult, error)
```

| Parameter      | Type                              | Description                                                                 |
|----------------|-----------------------------------|-----------------------------------------------------------------------------|
| `ctx`          | `context.Context`                 | Request context with logger                                                 |
| `mc`           | `*Memcached`                      | The owning Memcached CR (used for owner reference and event recording)      |
| `obj`          | `client.Object`                   | Target resource with Name and Namespace set; populated in-place on success  |
| `mutate`       | `func() error`                    | Sets the desired spec on `obj`; called before every create/update attempt   |
| `resourceKind` | `string`                          | Human-readable kind for logs and errors (e.g. `"Deployment"`, `"Service"`)  |

### Return Values

| Value                                     | Meaning                                                    |
|-------------------------------------------|------------------------------------------------------------|
| `controllerutil.OperationResultCreated`   | Resource did not exist and was created                     |
| `controllerutil.OperationResultUpdated`   | Resource existed with different spec and was updated        |
| `controllerutil.OperationResultNone`      | Resource existed with matching spec; no API call made       |
| `error`                                   | Non-nil on failure; wrapped with resource kind and name     |

---

## Conflict Retry Mechanism

When `controllerutil.CreateOrUpdate` returns an HTTP 409 Conflict error
(typically caused by a stale `resourceVersion` from concurrent modifications),
`reconcileResource` retries the entire CreateOrUpdate operation. On retry,
CreateOrUpdate re-fetches the resource with a fresh `resourceVersion` before
calling the mutate function again.

```
const maxConflictRetries = 5
```

| Attempt | Action                                                                 |
|---------|------------------------------------------------------------------------|
| 1       | Call CreateOrUpdate; if 409 Conflict, log and retry                    |
| 2       | Re-enter CreateOrUpdate (re-Get with fresh resourceVersion); if 409, retry |
| ...     | Continue retrying                                                      |
| 5       | Final attempt; if still 409, return conflict error                     |

Non-conflict errors (e.g. 500 Internal Server Error, validation failures) are
returned immediately without retrying.

### Sequence Diagram

```
Reconciler              CreateOrUpdate         API Server
    │                        │                      │
    ├──── attempt 1 ────────►│                      │
    │                        ├─── Get obj ─────────►│
    │                        │◄── obj (rv=100) ─────┤
    │                        │                      │
    │                        │  mutate() sets spec  │
    │                        │                      │
    │                        ├─── Update (rv=100) ──►│
    │                        │◄── 409 Conflict ─────┤  (rv changed to 101 by
    │◄── conflict error ─────┤                      │   another controller)
    │                        │                      │
    │  log: "Conflict retrying, attempt=1"          │
    │                        │                      │
    ├──── attempt 2 ────────►│                      │
    │                        ├─── Get obj ─────────►│
    │                        │◄── obj (rv=101) ─────┤
    │                        │                      │
    │                        │  mutate() sets spec  │
    │                        │                      │
    │                        ├─── Update (rv=101) ──►│
    │                        │◄── 200 OK ───────────┤
    │◄── OperationResultUpdated                     │
    │                        │                      │
    │  log: "Deployment reconciled, operation=updated"
    │  event: "Normal Updated Updated Deployment my-cache"
```

---

## Idempotent Behavior

The mutate function is called on **every** reconciliation, but
`controllerutil.CreateOrUpdate` only issues an API Update when the object
has actually changed. This means:

1. **First reconciliation**: Creates the resource (`OperationResultCreated`)
2. **Subsequent reconciliations with same CR spec**: Mutate runs but produces
   identical state, so no API update occurs (`OperationResultNone`)
3. **After CR spec change**: Mutate produces different state, triggering an
   update (`OperationResultUpdated`)
4. **After external drift** (manual edit): Mutate restores the desired state,
   triggering an update (`OperationResultUpdated`)

This is **level-triggered** reconciliation: the desired state is computed
purely from the current CR spec, not from a sequence of events. Missed,
duplicate, or out-of-order events all converge to the same correct state.

---

## Event Emission

`reconcileResource` emits Kubernetes events on the Memcached CR for create and
update operations via `emitEventForResult`:

| Operation Result | Event Type | Reason    | Message Format                        |
|------------------|------------|-----------|---------------------------------------|
| Created          | Normal     | Created   | `Created <Kind> <name>`              |
| Updated          | Normal     | Updated   | `Updated <Kind> <name>`              |
| Unchanged        | —          | —         | No event emitted                      |

Events are visible via `kubectl describe memcached <name>` and provide an audit
trail of operator actions.

---

## Logging

| Operation   | Log Level | Message                                          | Fields                        |
|-------------|-----------|--------------------------------------------------|-------------------------------|
| Created     | Info      | `<Kind> reconciled`                              | `name`, `operation=created`   |
| Updated     | Info      | `<Kind> reconciled`                              | `name`, `operation=updated`   |
| Unchanged   | Info      | `<Kind> reconciled`                              | `name`, `operation=unchanged` |
| Conflict    | Info      | `Conflict retrying <Kind> reconciliation`        | `name`, `attempt`, `maxRetries` |

---

## Owner References

`reconcileResource` automatically sets a controller owner reference on every
managed resource via `controllerutil.SetControllerReference`. This is called
inside the mutate wrapper, after the caller's mutate function, ensuring it
applies to both creates and updates.

| Field                | Value                           |
|----------------------|---------------------------------|
| `apiVersion`         | `memcached.c5c3.io/v1alpha1`   |
| `kind`               | `Memcached`                     |
| `name`               | `<cr-name>`                     |
| `uid`                | `<cr-uid>`                      |
| `controller`         | `true`                          |
| `blockOwnerDeletion` | `true`                          |

This enables automatic garbage collection when the Memcached CR is deleted.

---

## Current Usage

Both `reconcileDeployment` and `reconcileService` delegate to
`reconcileResource`, keeping them as thin wrappers that construct the initial
`ObjectMeta` and pass a mutate function:

```go
func (r *MemcachedReconciler) reconcileDeployment(ctx context.Context, mc *memcachedv1alpha1.Memcached) error {
    dep := &appsv1.Deployment{
        ObjectMeta: metav1.ObjectMeta{
            Name:      mc.Name,
            Namespace: mc.Namespace,
        },
    }

    _, err := r.reconcileResource(ctx, mc, dep, func() error {
        constructDeployment(mc, dep)
        return nil
    }, "Deployment")
    return err
}
```

The builder functions (`constructDeployment`, `constructService`) are pure
functions of the CR spec — they take the Memcached CR and the target object,
and set all desired fields in-place. They have no side effects and no hidden
state.

---

## Adding a New Resource Type

To add a new managed resource (e.g. PodDisruptionBudget), follow these steps:

### 1. Create the builder function

Add a pure builder function in a new file (e.g. `internal/controller/pdb.go`):

```go
func constructPDB(mc *memcachedv1alpha1.Memcached, pdb *policyv1.PodDisruptionBudget) {
    labels := labelsForMemcached(mc.Name)
    pdb.Labels = labels
    pdb.Spec = policyv1.PodDisruptionBudgetSpec{
        MinAvailable: &intstr.IntOrString{Type: intstr.Int, IntVal: 1},
        Selector: &metav1.LabelSelector{
            MatchLabels: labels,
        },
    }
}
```

### 2. Create the reconcile wrapper

Add a thin wrapper in `memcached_controller.go`:

```go
func (r *MemcachedReconciler) reconcilePDB(ctx context.Context, mc *memcachedv1alpha1.Memcached) error {
    pdb := &policyv1.PodDisruptionBudget{
        ObjectMeta: metav1.ObjectMeta{
            Name:      mc.Name,
            Namespace: mc.Namespace,
        },
    }

    _, err := r.reconcileResource(ctx, mc, pdb, func() error {
        constructPDB(mc, pdb)
        return nil
    }, "PodDisruptionBudget")
    return err
}
```

### 3. Wire into Reconcile

Add the call in the `Reconcile` method:

```go
if err := r.reconcilePDB(ctx, memcached); err != nil {
    return ctrl.Result{}, err
}
```

### 4. Register the watch

Add `Owns` in `SetupWithManager`:

```go
Owns(&policyv1.PodDisruptionBudget{}).
```

### 5. Add RBAC markers

Add the kubebuilder RBAC marker above `Reconcile`:

```go
// +kubebuilder:rbac:groups=policy,resources=poddisruptionbudgets,verbs=get;list;watch;create;update;patch;delete
```

### 6. Write tests

Write unit tests for the builder function (table-driven, testing field mapping)
and integration tests verifying idempotent behavior through `reconcilePDB`.

The new resource automatically inherits conflict retries, owner references,
structured logging, and event emission from `reconcileResource`.

---

## Error Handling Summary

| Error Scenario                     | Behavior                                                    |
|------------------------------------|-------------------------------------------------------------|
| Mutate function returns error      | Error wrapped with `"reconciling <Kind>: ..."` and returned |
| API server returns 409 Conflict    | Retry up to 5 times, then return conflict error             |
| API server returns other error     | Error wrapped and returned immediately (no retry)           |
| Owner reference conflict           | Error from `SetControllerReference`, returned via mutate    |
| All retries exhausted              | Conflict error returned to controller-runtime for requeue   |

All errors are returned to controller-runtime, which applies its standard
exponential backoff requeue strategy.
