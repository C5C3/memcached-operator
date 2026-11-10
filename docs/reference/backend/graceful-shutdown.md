# Graceful Shutdown

Reference documentation for the graceful shutdown feature that configures
preStop lifecycle hooks and terminationGracePeriodSeconds on Memcached pods
to allow in-flight connections to drain before pod termination.

**Source**: `internal/controller/deployment.go`, `api/v1alpha1/memcached_types.go`

## Overview

When `spec.highAvailability.gracefulShutdown.enabled` is `true`, the reconciler
configures the StatefulSet pod template with:

1. A **preStop lifecycle hook** on the memcached container that executes
   `sleep <preStopDelaySeconds>` to allow in-flight connections to drain
   before SIGTERM is sent.
2. A **terminationGracePeriodSeconds** on the pod spec that exceeds the
   preStop sleep duration, giving Kubernetes enough time to complete the
   hook before sending SIGKILL.

This prevents cache stampedes during rolling updates by ensuring clients
experience no abrupt connection resets. The StatefulSet replaces pods one at a
time, so the hook runs on one pod at a time (see
[Rolling Updates](#rolling-updates)).

The feature is opt-in — no preStop hook or custom terminationGracePeriodSeconds
is set unless explicitly enabled.

---

## CRD Field Path

```text
spec.highAvailability.gracefulShutdown
```

Defined in `api/v1alpha1/memcached_types.go` on the `GracefulShutdownSpec` struct:

```go
type GracefulShutdownSpec struct {
    Enabled                       bool  `json:"enabled,omitempty"`
    PreStopDelaySeconds           int32 `json:"preStopDelaySeconds,omitempty"`
    TerminationGracePeriodSeconds int64 `json:"terminationGracePeriodSeconds,omitempty"`
}
```

| Field                           | Type    | Required | Default | Validation       | Description                                                     |
|---------------------------------|---------|----------|---------|------------------|-----------------------------------------------------------------|
| `enabled`                       | `bool`  | No       | `false` | —                | Controls whether graceful shutdown is configured                |
| `preStopDelaySeconds`           | `int32` | No       | `10`    | Min: 1, Max: 300 | Seconds the preStop hook sleeps for connection draining         |
| `terminationGracePeriodSeconds` | `int64` | No       | `30`    | Min: 1, Max: 600 | Pod termination grace period; must exceed `preStopDelaySeconds` |

---

## StatefulSet Construction

`buildGracefulShutdown(mc *Memcached)` returns the lifecycle hook and
terminationGracePeriodSeconds, or `(nil, nil)` if graceful shutdown is not
enabled. These values are applied in `constructStatefulSet`:

- `lifecycle` is set on `containers[0].Lifecycle`
- `terminationGracePeriodSeconds` is set on `spec.template.spec.terminationGracePeriodSeconds`

### Skip Logic

`buildGracefulShutdown` returns `(nil, nil)` when:

- `spec.highAvailability` is nil
- `spec.highAvailability.gracefulShutdown` is nil
- `spec.highAvailability.gracefulShutdown.enabled` is `false`

When graceful shutdown is disabled, the controller sets both `lifecycle` and
`terminationGracePeriodSeconds` to nil on the StatefulSet pod template. The Kubernetes
API server applies its own default of 30s for `terminationGracePeriodSeconds`.

### PreStop Hook

The preStop hook uses an exec handler with the `sleep` command:

```yaml
lifecycle:
  preStop:
    exec:
      command: ["sleep", "<preStopDelaySeconds>"]
```

This is the standard Kubernetes pattern for connection draining. During the
sleep period:

1. The pod is removed from Service endpoints (kube-proxy/iptables update)
2. Existing connections continue to be served
3. After the sleep completes, SIGTERM is sent to the memcached process
4. The process has `terminationGracePeriodSeconds - preStopDelaySeconds`
   remaining to shut down before SIGKILL

### Rolling Updates

The StatefulSet uses `updateStrategy.type: RollingUpdate` with no surge. On a
pod template change it replaces the pods one at a time, from the highest
ordinal down:

1. The StatefulSet controller deletes one pod. The pod leaves the Service
   endpoints, and its per-pod DNS name
   (`<statefulset>-<i>.<cr-name>.<namespace>.svc`) stops resolving.
2. The preStop hook sleeps for `preStopDelaySeconds` while existing
   connections are still served, then memcached receives SIGTERM.
3. Once the pod has terminated, the controller creates its replacement under
   the same name.
4. The controller waits until the replacement is Ready before it deletes the
   next pod.

Each pod is unreachable from the moment it is deleted until its replacement is
Ready, and an instance with one replica has no ready pod during that window.
The replacement keeps the pod's name, so `status.serverList` does not change
during a rolling update. A rollout takes roughly one termination plus one
startup per replica, so a longer `preStopDelaySeconds` makes it slower.

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
  highAvailability:
    gracefulShutdown:
      enabled: true
```

Produces a StatefulSet with:

```yaml
spec:
  template:
    spec:
      terminationGracePeriodSeconds: 30
      containers:
        - name: memcached
          lifecycle:
            preStop:
              exec:
                command: ["sleep", "10"]
```

### Custom Values

```yaml
spec:
  replicas: 5
  highAvailability:
    gracefulShutdown:
      enabled: true
      preStopDelaySeconds: 15
      terminationGracePeriodSeconds: 45
```

Produces `command: ["sleep", "15"]` and `terminationGracePeriodSeconds: 45`.

### Disabled (Default)

```yaml
apiVersion: memcached.c5c3.io/v1alpha1
kind: Memcached
metadata:
  name: my-cache
spec:
  replicas: 3
```

No preStop hook or custom terminationGracePeriodSeconds is set. The Kubernetes
default of 30s applies for terminationGracePeriodSeconds.

### Combined with Other HA Features

```yaml
spec:
  replicas: 3
  highAvailability:
    antiAffinityPreset: soft
    topologySpreadConstraints:
      - maxSkew: 1
        topologyKey: topology.kubernetes.io/zone
        whenUnsatisfiable: DoNotSchedule
    podDisruptionBudget:
      enabled: true
      minAvailable: 1
    gracefulShutdown:
      enabled: true
      preStopDelaySeconds: 10
      terminationGracePeriodSeconds: 30
```

All HA features are applied independently and coexist without conflicts.

---

## Runtime Behavior

| Action                                       | Result                                                                           |
|----------------------------------------------|----------------------------------------------------------------------------------|
| Enable graceful shutdown (`enabled: true`)   | PreStop hook and terminationGracePeriodSeconds set on next reconcile             |
| Change `preStopDelaySeconds`                 | StatefulSet updated with new sleep duration; pods replaced one at a time         |
| Change `terminationGracePeriodSeconds`       | StatefulSet updated with new grace period; pods replaced one at a time           |
| Disable graceful shutdown (`enabled: false`) | PreStop hook removed; terminationGracePeriodSeconds reverts to K8s default (30s) |
| Remove `gracefulShutdown` section            | Same as disabled — hook removed                                                  |
| Remove `highAvailability` section            | Hook removed; other HA features also cleared                                     |
| Reconcile twice with same spec               | No StatefulSet update (idempotent)                                               |
| External drift (manual StatefulSet edit)     | Corrected on next reconciliation cycle                                           |

---

## Implementation

The `buildGracefulShutdown` function in `internal/controller/deployment.go` is
a pure function that returns the lifecycle and terminationGracePeriodSeconds:

```go
func buildGracefulShutdown(mc *Memcached) (*corev1.Lifecycle, *int64)
```

- Returns `(nil, nil)` when graceful shutdown is not enabled
- Returns a Lifecycle with PreStop exec `["sleep", "<seconds>"]` when enabled
- Returns a pointer to the terminationGracePeriodSeconds value

The returned values are applied directly in `constructStatefulSet` on the
container and pod spec respectively.
