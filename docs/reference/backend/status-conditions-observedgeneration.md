# Status Conditions and ObservedGeneration

Reference documentation for the status reconciliation logic that updates
conditions, `readyReplicas`, and `observedGeneration` on the Memcached CR after
every reconciliation cycle.

**Source**: `internal/controller/status.go`, `internal/controller/memcached_controller.go`

## Overview

After reconciling the Deployment and Service, the reconciler updates the
Memcached CR's status subresource with three pieces of information:

1. **`observedGeneration`** — the CR's `metadata.generation` at the time of
   reconciliation, so operators can detect stale status.
2. **`readyReplicas`** — the number of ready pods from the owned Deployment,
   surfaced at the CR level for convenience.
3. **Conditions** — three standard conditions (`Available`, `Progressing`,
   `Degraded`) derived from the Deployment's status.

Status is always updated via the status subresource (`r.Status().Update`) to
avoid conflicts with spec writes.

---

## ObservedGeneration

`status.observedGeneration` is set to `metadata.generation` on every successful
reconciliation. This follows the Kubernetes convention for detecting whether the
controller has processed the latest spec.

| Scenario                                    | Meaning                                                 |
|---------------------------------------------|---------------------------------------------------------|
| `observedGeneration == metadata.generation` | Controller has acted on the latest spec                 |
| `observedGeneration < metadata.generation`  | Controller has not yet processed the latest spec change |

The field is updated unconditionally on every reconcile — not just when the spec
changes — so it always reflects the most recent reconciliation pass.

---

## ReadyReplicas

`status.readyReplicas` mirrors the owned Deployment's `status.readyReplicas`.

| Deployment State     | `readyReplicas` Value      |
|----------------------|----------------------------|
| Deployment exists    | `dep.Status.ReadyReplicas` |
| Deployment not found | `0`                        |

This field is also exposed as the `Ready` printer column via the
`+kubebuilder:printcolumn` marker on the Memcached type, so `kubectl get memcached`
displays it directly.

---

## Conditions

Three conditions are computed on every reconciliation and applied using
`apimachinery/pkg/api/meta.SetStatusCondition`, which correctly handles
`lastTransitionTime` — the timestamp only changes when the condition's `Status`
value changes.

Each condition includes `ObservedGeneration` matching the CR's
`metadata.generation` at the time of computation (set in `computeConditions`).

### Available

Indicates whether the Memcached instance has minimum availability.

| Status  | Reason        | When                                                             |
|---------|---------------|------------------------------------------------------------------|
| `True`  | `Available`   | Deployment has `readyReplicas >= 1` **or** desired replicas is 0 |
| `False` | `Unavailable` | Deployment has `readyReplicas == 0` and desired > 0              |
| `False` | `Unavailable` | Deployment does not exist yet                                    |

**Message format**: `"<ready>/<desired> replicas are ready"`

### Progressing

Indicates whether a rollout or scaling operation is in progress.

| Status  | Reason                | When                                                            |
|---------|-----------------------|-----------------------------------------------------------------|
| `True`  | `Progressing`         | Deployment does not exist yet                                   |
| `True`  | `Progressing`         | `updatedReplicas < desired` (rollout in progress)               |
| `True`  | `Progressing`         | `totalReplicas != desired` (scaling in/out)                     |
| `False` | `ProgressingComplete` | `updatedReplicas == desired` **and** `totalReplicas == desired` |

**Message format**:
- When Deployment is nil: `"Waiting for deployment to be created"`
- When progressing: `"Rollout in progress: <updated>/<desired> replicas updated"`
- When complete: `"All <desired> replicas are updated"`

### Degraded

Indicates whether the instance has fewer ready replicas than desired.

| Status  | Reason        | When                                          |
|---------|---------------|-----------------------------------------------|
| `True`  | `Degraded`    | `readyReplicas < desired` and `desired > 0`   |
| `True`  | `Degraded`    | Deployment does not exist and `desired > 0`   |
| `False` | `NotDegraded` | `readyReplicas == desired`                    |
| `False` | `NotDegraded` | `desired == 0` (intentionally scaled to zero) |

**Message format**:
- When Deployment is nil: `"Waiting for deployment to be created"`
- When degraded: `"Only <ready>/<desired> replicas are ready"`
- When not degraded: `"All <desired> desired replicas are ready"`

---

## Edge Cases

### Scaled to Zero

When `spec.replicas` is `0`, the cluster is intentionally empty:

| Condition   | Status  | Reason                | Rationale                   |
|-------------|---------|-----------------------|-----------------------------|
| Available   | `True`  | `Available`           | No availability requirement |
| Progressing | `False` | `ProgressingComplete` | Nothing to progress         |
| Degraded    | `False` | `NotDegraded`         | Desired state is met        |

### Nil Deployment

If the Deployment does not exist when status is computed (e.g., first
reconciliation before `reconcileDeployment` creates it, or a race condition):

| Condition   | Status  | Reason        | Rationale                                     |
|-------------|---------|---------------|-----------------------------------------------|
| Available   | `False` | `Unavailable` | No replicas serving traffic                   |
| Progressing | `True`  | `Progressing` | Creation is an in-progress operation          |
| Degraded    | `True`  | `Degraded`    | 0 ready replicas with desired > 0 (default 1) |

---

## Condition Constants

Defined in `internal/controller/status.go`:

### Type Constants

| Constant                   | Value           |
|----------------------------|-----------------|
| `ConditionTypeAvailable`   | `"Available"`   |
| `ConditionTypeProgressing` | `"Progressing"` |
| `ConditionTypeDegraded`    | `"Degraded"`    |

### Reason Constants

| Constant                             | Value                   |
|--------------------------------------|-------------------------|
| `ConditionReasonAvailable`           | `"Available"`           |
| `ConditionReasonUnavailable`         | `"Unavailable"`         |
| `ConditionReasonProgressing`         | `"Progressing"`         |
| `ConditionReasonProgressingComplete` | `"ProgressingComplete"` |
| `ConditionReasonDegraded`            | `"Degraded"`            |
| `ConditionReasonNotDegraded`         | `"NotDegraded"`         |

---

## Reconciliation Method

`reconcileStatus(ctx, mc *Memcached)` on `MemcachedReconciler` performs the
status update:

```go
func (r *MemcachedReconciler) reconcileStatus(ctx context.Context, mc *memcachedv1alpha1.Memcached) error {
    // 1. Fetch the current Deployment (nil if not found)
    // 2. Compute conditions from Deployment status
    // 3. Apply conditions via meta.SetStatusCondition
    // 4. Set readyReplicas from Deployment (0 if nil)
    // 5. Set observedGeneration from mc.Generation
    // 6. Update via r.Status().Update(ctx, mc)
}
```

### Error Handling

| Error Scenario                        | Behavior                                                 |
|---------------------------------------|----------------------------------------------------------|
| Deployment fetch fails (not NotFound) | Error wrapped and returned for requeue                   |
| Status update conflict                | Error returned, controller-runtime requeues with backoff |
| Status update transient failure       | Error returned, controller-runtime requeues with backoff |

Status update errors do not affect resource reconciliation — the Deployment and
Service are reconciled before status, so they converge independently.

---

## Reconcile Integration

The `Reconcile` method calls `reconcileStatus` after all resource
reconciliation:

```go
func (r *MemcachedReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
    // 1. Fetch Memcached CR
    // 2. reconcileDeployment  ← resource convergence
    // 3. reconcileService     ← resource convergence
    // 4. reconcileStatus      ← status update (last)
}
```

This ordering ensures:
- Resources converge even if status update fails on a subsequent requeue.
- Status reflects the state after the latest resource reconciliation.
- A failed status update returns an error, triggering a requeue to retry.

---

## Reconciliation Flow

```text
  Memcached CR created/updated
            │
            ▼
  ┌─────────────────────────────┐
  │  Reconcile                  │
  │  1. Fetch Memcached CR      │
  │  2. reconcileDeployment     │
  │  3. reconcileService        │
  └────────────┬────────────────┘
               │
               ▼
  ┌─────────────────────────────┐
  │  reconcileStatus            │
  │                             │
  │  1. Fetch Deployment        │
  │     ├─ Found → use status   │
  │     └─ NotFound → nil       │
  │                             │
  │  2. computeConditions       │
  │     ├─ Available            │
  │     ├─ Progressing          │
  │     └─ Degraded             │
  │                             │
  │  3. meta.SetStatusCondition │
  │     (preserves transition   │
  │      timestamps)            │
  │                             │
  │  4. Set readyReplicas       │
  │  5. Set observedGeneration  │
  │  6. r.Status().Update()     │
  └─────────────────────────────┘
```

---

## Status Example

A Memcached CR with 3 replicas where 2 are ready:

```yaml
apiVersion: memcached.c5c3.io/v1alpha1
kind: Memcached
metadata:
  name: my-cache
  namespace: default
  generation: 2
spec:
  replicas: 3
status:
  observedGeneration: 2
  readyReplicas: 2
  conditions:
    - type: Available
      status: "True"
      reason: Available
      message: "2/3 replicas are ready"
      lastTransitionTime: "2024-01-15T10:30:00Z"
      observedGeneration: 2
    - type: Progressing
      status: "True"
      reason: Progressing
      message: "Rollout in progress: 3/3 replicas updated"
      lastTransitionTime: "2024-01-15T10:29:00Z"
      observedGeneration: 2
    - type: Degraded
      status: "True"
      reason: Degraded
      message: "Only 2/3 replicas are ready"
      lastTransitionTime: "2024-01-15T10:29:00Z"
      observedGeneration: 2
```

### Fully Ready State

```yaml
status:
  observedGeneration: 2
  readyReplicas: 3
  conditions:
    - type: Available
      status: "True"
      reason: Available
      message: "3/3 replicas are ready"
    - type: Progressing
      status: "False"
      reason: ProgressingComplete
      message: "All 3 replicas are updated"
    - type: Degraded
      status: "False"
      reason: NotDegraded
      message: "All 3 desired replicas are ready"
```
