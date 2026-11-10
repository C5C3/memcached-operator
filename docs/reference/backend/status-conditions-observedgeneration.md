# Status Conditions and ObservedGeneration

Reference documentation for the status reconciliation logic that updates
conditions, `readyReplicas`, `serverList`, and `observedGeneration` on the
Memcached CR after every reconciliation cycle.

**Source**: `internal/controller/status.go`, `internal/controller/memcached_controller.go`

## Overview

After reconciling the StatefulSet, the Service, and the optional resources, the
reconciler updates the Memcached CR's status subresource with four pieces of
information:

1. **`observedGeneration`**: the CR's `metadata.generation` at the time of
   reconciliation, so operators can detect stale status.
2. **`readyReplicas`**: the number of ready pods from the owned StatefulSet,
   surfaced at the CR level for convenience.
3. **`serverList`**: one `host:port` entry per pod, built from the
   StatefulSet's `spec.replicas` (see [serverList](#serverlist)).
4. **Conditions**: four standard conditions (`Available`, `Progressing`,
   `Degraded`, `Ready`) derived from the StatefulSet's status.

The status reads the StatefulSet named `statefulSetName(<cr-name>)`, which is
the CR name unless the CR name has more than 52 characters (see
[StatefulSet Reconciliation](deployment-reconciliation.md#statefulset-name)).

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

`status.readyReplicas` mirrors the owned StatefulSet's `status.readyReplicas`.

| StatefulSet State     | `readyReplicas` Value      |
|-----------------------|----------------------------|
| StatefulSet exists    | `sts.Status.ReadyReplicas` |
| StatefulSet not found | `0`                        |

This field is also exposed as the `Ready` printer column via the
`+kubebuilder:printcolumn` marker on the Memcached type, so `kubectl get memcached`
displays it directly.

---

## serverList

`status.serverList` holds one `host:port` entry per pod of the StatefulSet,
ordered by pod index `i` from 0:

```text
<statefulset>-<i>.<cr-name>.<namespace>.svc:11211
```

`<statefulset>-<i>` is the pod name and `<cr-name>` is the headless Service
that governs the StatefulSet, so each entry is the pod's stable DNS name (see
[Per-Pod DNS Names](headless-service-reconciliation.md#per-pod-dns-names)). The
port is `PortMemcached` (11211), also when TLS is enabled.

`buildServerList` in `internal/controller/status.go` builds the list:

```go
func buildServerList(workloadName, serviceName, namespace string, replicas int32) []string {
    if replicas <= 0 {
        return nil
    }
    servers := make([]string, 0, replicas)
    for i := range replicas {
        servers = append(servers, fmt.Sprintf("%s-%d.%s.%s.svc:%d", workloadName, i, serviceName, namespace, PortMemcached))
    }
    return servers
}
```

`reconcileStatus` calls it with the StatefulSet's name, the CR name, the CR
namespace, and the StatefulSet's `spec.replicas`:

| StatefulSet State     | `serverList` Value                                                                          |
|-----------------------|---------------------------------------------------------------------------------------------|
| StatefulSet not found | `nil` (field absent)                                                                        |
| `spec.replicas: 0`    | `nil` (field absent)                                                                        |
| `spec.replicas` nil   | One entry, `<statefulset>-0.<cr-name>.<namespace>.svc:11211` (Kubernetes defaults nil to 1) |
| `spec.replicas: N`    | N entries, for `i` from 0 to N-1                                                            |

The list follows the StatefulSet's `spec.replicas`, never pod readiness. It
changes only when the replica count changes, including a change the HPA makes,
and stays the same while pods restart or a rolling update replaces them. A
client library that hashes keys over the list therefore keeps each key on the
same server while pods come and go. An entry for a pod that is not ready yet
stays in the list; its DNS name resolves once the pod is ready.

For a CR named `my-cache` in namespace `default` with `replicas: 3`:

```yaml
status:
  serverList:
    - my-cache-0.my-cache.default.svc:11211
    - my-cache-1.my-cache.default.svc:11211
    - my-cache-2.my-cache.default.svc:11211
```

For a CR name longer than 52 characters, the host starts with the shortened
StatefulSet name, `statefulSetName(<cr-name>)`, while the Service part keeps the
full CR name.

---

## Conditions

Four conditions are computed on every reconciliation and applied using
`apimachinery/pkg/api/meta.SetStatusCondition`, which correctly handles
`lastTransitionTime` — the timestamp only changes when the condition's `Status`
value changes.

Each condition includes `ObservedGeneration` matching the CR's
`metadata.generation` at the time of computation (set in `computeConditions`).

### Available

Indicates whether the Memcached instance has minimum availability.

| Status  | Reason        | When                                                         |
|---------|---------------|--------------------------------------------------------------|
| `True`  | `Available`   | StatefulSet has `readyReplicas >= 1`                         |
| `False` | `Unavailable` | StatefulSet has `readyReplicas == 0`, also when desired is 0 |
| `False` | `Unavailable` | StatefulSet does not exist yet                               |

**Message format**: `"<ready>/<desired> replicas are ready"`

### Progressing

Indicates whether a rollout or scaling operation is in progress.

| Status  | Reason                | When                                                            |
|---------|-----------------------|-----------------------------------------------------------------|
| `True`  | `Progressing`         | StatefulSet does not exist yet                                  |
| `True`  | `Progressing`         | `updatedReplicas < desired` (rollout in progress)               |
| `True`  | `Progressing`         | `totalReplicas != desired` (scaling in/out)                     |
| `False` | `ProgressingComplete` | `updatedReplicas == desired` **and** `totalReplicas == desired` |

**Message format**:
- When StatefulSet is nil: `"Waiting for StatefulSet to be created"`
- When progressing: `"Rollout in progress: <updated>/<desired> replicas updated"`
- When complete: `"All <desired> replicas are updated"`

### Degraded

Indicates whether the instance has fewer ready replicas than desired, or whether
referenced Secrets are missing.

| Status  | Reason           | When                                              |
|---------|------------------|---------------------------------------------------|
| `True`  | `SecretNotFound` | One or more referenced Secrets are missing        |
| `True`  | `Degraded`       | `readyReplicas < desired` and `desired > 0`       |
| `True`  | `Degraded`       | StatefulSet does not exist and `desired > 0`      |
| `False` | `NotDegraded`    | `readyReplicas == desired` and no missing Secrets |
| `False` | `NotDegraded`    | `desired == 0` (intentionally scaled to zero)     |

`SecretNotFound` takes precedence over replica-based degraded status. When any
referenced Secret (SASL credentials or TLS certificate) cannot be fetched, the
Degraded condition is set to `SecretNotFound` regardless of replica counts.

**Message format**:
- When Secrets missing: `"Referenced Secrets not found: <name1>, <name2>"`
- When StatefulSet is nil: `"Waiting for StatefulSet to be created"`
- When degraded: `"Only <ready>/<desired> replicas are ready"`
- When not degraded: `"All <desired> desired replicas are ready"`

The `"Waiting for StatefulSet to be created"` text is the constant
`msgWaitingForStatefulSet`, shared by the Progressing, Degraded, and Ready
conditions.

### Ready

Indicates whether every desired replica is ready.

| Status  | Reason              | When                                             |
|---------|---------------------|--------------------------------------------------|
| `True`  | `MemcachedReady`    | `desired > 0` **and** `readyReplicas == desired` |
| `False` | `MemcachedNotReady` | `desired == 0`                                   |
| `False` | `MemcachedNotReady` | StatefulSet does not exist yet                   |
| `False` | `MemcachedNotReady` | `readyReplicas < desired`                        |

**Message format**:
- When ready: `"All <desired> replicas are ready"`
- When desired is 0: `"Instance has zero desired replicas"`
- When StatefulSet is nil: `"Waiting for StatefulSet to be created"`
- Otherwise: `"<ready>/<desired> replicas are ready"`

`Ready` does not affect `serverList`; the list is published whether `Ready` is
`True` or `False`.

---

## Edge Cases

### Scaled to Zero

When `spec.replicas` is `0`, the cluster is intentionally empty:

| Condition   | Status  | Reason                | Rationale                 |
|-------------|---------|-----------------------|---------------------------|
| Available   | `False` | `Unavailable`         | No replica serves traffic |
| Progressing | `False` | `ProgressingComplete` | Nothing to progress       |
| Degraded    | `False` | `NotDegraded`         | Desired state is met      |
| Ready       | `False` | `MemcachedNotReady`   | No replica to be ready    |

`serverList` is absent, because the StatefulSet has `spec.replicas: 0`.

### Nil StatefulSet

If the StatefulSet does not exist when status is computed (e.g., first
reconciliation before `reconcileStatefulSet` creates it, or a race condition):

| Condition   | Status  | Reason              | Rationale                                     |
|-------------|---------|---------------------|-----------------------------------------------|
| Available   | `False` | `Unavailable`       | No replicas serving traffic                   |
| Progressing | `True`  | `Progressing`       | Creation is an in-progress operation          |
| Degraded    | `True`  | `Degraded`          | 0 ready replicas with desired > 0 (default 1) |
| Ready       | `False` | `MemcachedNotReady` | 0 ready replicas with desired > 0 (default 1) |

`readyReplicas` is `0` and `serverList` is absent.

---

## Condition Constants

Defined in `internal/controller/status.go`:

### Type Constants

| Constant                   | Value           |
|----------------------------|-----------------|
| `ConditionTypeAvailable`   | `"Available"`   |
| `ConditionTypeProgressing` | `"Progressing"` |
| `ConditionTypeDegraded`    | `"Degraded"`    |
| `ConditionTypeReady`       | `"Ready"`       |

### Reason Constants

| Constant                             | Value                   |
|--------------------------------------|-------------------------|
| `ConditionReasonAvailable`           | `"Available"`           |
| `ConditionReasonUnavailable`         | `"Unavailable"`         |
| `ConditionReasonProgressing`         | `"Progressing"`         |
| `ConditionReasonProgressingComplete` | `"ProgressingComplete"` |
| `ConditionReasonDegraded`            | `"Degraded"`            |
| `ConditionReasonNotDegraded`         | `"NotDegraded"`         |
| `ConditionReasonSecretNotFound`      | `"SecretNotFound"`      |
| `ConditionReasonReady`               | `"MemcachedReady"`      |
| `ConditionReasonNotReady`            | `"MemcachedNotReady"`   |

---

## Reconciliation Method

`reconcileStatus(ctx, mc *Memcached, missingSecrets []string)` on
`MemcachedReconciler` performs the status update. The `missingSecrets` parameter
is the list of Secret names that could not be fetched during StatefulSet
reconciliation (returned by `reconcileStatefulSet`).

```go
func (r *MemcachedReconciler) reconcileStatus(ctx context.Context, mc *memcachedv1beta1.Memcached, missingSecrets []string) error {
    // 1. Fetch the StatefulSet named statefulSetName(mc.Name) (nil if not found)
    // 2. Compute conditions from StatefulSet status and missingSecrets
    // 3. Apply conditions via meta.SetStatusCondition
    // 4. Set serverList via buildServerList from the StatefulSet's spec.replicas (nil if not found)
    // 5. Set readyReplicas from StatefulSet (0 if nil)
    // 6. Set observedGeneration from mc.Generation
    // 7. Update via r.Status().Update(ctx, mc)
}
```

### Error Handling

| Error Scenario                         | Behavior                                                          |
|----------------------------------------|-------------------------------------------------------------------|
| StatefulSet fetch fails (not NotFound) | `fetching StatefulSet for status: %w`, returned without an update |
| Status update conflict                 | Error returned, controller-runtime requeues with backoff          |
| Status update transient failure        | Error returned, controller-runtime requeues with backoff          |

Status update errors do not affect resource reconciliation. The StatefulSet and
Service are reconciled before status, so they converge independently.

---

## Reconcile Integration

The `Reconcile` method calls `reconcileStatus` after all resource
reconciliation:

```go
func (r *MemcachedReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
    // 1. Fetch Memcached CR
    // 2. missingSecrets, err := reconcileStatefulSet  ← resource convergence + secret resolution
    // 3. reconcileHPA, reconcileService, ...           ← resource convergence
    // 4. reconcileStatus(ctx, mc, missingSecrets)      ← status update (last)
}
```

`reconcileStatefulSet` returns the list of missing Secret names alongside the
error, so they can be forwarded to `reconcileStatus` for the `SecretNotFound`
condition.

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
  │  2. reconcileStatefulSet    │
  │  3. reconcileHPA,           │
  │     reconcileService, ...   │
  └────────────┬────────────────┘
               │
               ▼
  ┌─────────────────────────────┐
  │  reconcileStatus            │
  │                             │
  │  1. Fetch StatefulSet       │
  │     ├─ Found → use status   │
  │     └─ NotFound → nil       │
  │                             │
  │  2. computeConditions       │
  │     (with missingSecrets)   │
  │     ├─ Available            │
  │     ├─ Progressing          │
  │     ├─ Degraded /           │
  │     │  SecretNotFound       │
  │     └─ Ready                │
  │                             │
  │  3. meta.SetStatusCondition │
  │     (preserves transition   │
  │      timestamps)            │
  │                             │
  │  4. Set serverList          │
  │     (buildServerList from   │
  │      spec.replicas)         │
  │  5. Set readyReplicas       │
  │  6. Set observedGeneration  │
  │  7. r.Status().Update()     │
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
  serverList:
    - my-cache-0.my-cache.default.svc:11211
    - my-cache-1.my-cache.default.svc:11211
    - my-cache-2.my-cache.default.svc:11211
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
    - type: Ready
      status: "False"
      reason: MemcachedNotReady
      message: "2/3 replicas are ready"
      lastTransitionTime: "2024-01-15T10:29:00Z"
      observedGeneration: 2
```

`serverList` already holds all three entries, although only two pods are ready.

### Fully Ready State

```yaml
status:
  observedGeneration: 2
  readyReplicas: 3
  serverList:
    - my-cache-0.my-cache.default.svc:11211
    - my-cache-1.my-cache.default.svc:11211
    - my-cache-2.my-cache.default.svc:11211
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
    - type: Ready
      status: "True"
      reason: MemcachedReady
      message: "All 3 replicas are ready"
```
