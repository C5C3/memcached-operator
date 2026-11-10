# Pod Anti-Affinity Presets

Reference documentation for the pod anti-affinity preset feature that controls
how Memcached pods are spread across nodes.

**Source**: `internal/controller/deployment.go`

## Overview

The `spec.highAvailability.antiAffinityPreset` field configures pod anti-affinity
rules on the StatefulSet's pod template. This encourages or enforces scheduling
Memcached pods from the same CR instance onto different nodes, improving fault
isolation.

Two presets are available:

| Preset | Kubernetes API Field                              | Scheduling Behavior                                          |
|--------|---------------------------------------------------|--------------------------------------------------------------|
| `soft` | `preferredDuringSchedulingIgnoredDuringExecution` | Best-effort spreading; pods can co-locate if necessary       |
| `hard` | `requiredDuringSchedulingIgnoredDuringExecution`  | Strict spreading; scheduling fails if nodes are insufficient |

When `spec.highAvailability` is nil or `antiAffinityPreset` is nil, no affinity
rules are set on the StatefulSet (the `spec.template.spec.affinity` field is nil).

---

## Anti-Affinity Label Selector

The anti-affinity rule uses an **instance-scoped** label selector matching pods
from the same Memcached CR:

```yaml
labelSelector:
  matchLabels:
    app.kubernetes.io/name: memcached
    app.kubernetes.io/instance: <cr-name>
```

This ensures that anti-affinity rules for one Memcached CR do not affect pods
from a different Memcached CR in the same namespace.

The topology key is `kubernetes.io/hostname`, which spreads pods across
individual nodes.

---

## Soft Preset

The `soft` preset uses `preferredDuringSchedulingIgnoredDuringExecution` with
weight 100 (maximum). The scheduler will prefer placing pods on different nodes
but will not block scheduling if no suitable node is available.

### Generated Affinity Structure

```yaml
spec:
  template:
    spec:
      affinity:
        podAntiAffinity:
          preferredDuringSchedulingIgnoredDuringExecution:
            - weight: 100
              podAffinityTerm:
                topologyKey: kubernetes.io/hostname
                labelSelector:
                  matchLabels:
                    app.kubernetes.io/name: memcached
                    app.kubernetes.io/instance: <cr-name>
```

### CR Example

```yaml
apiVersion: memcached.c5c3.io/v1alpha1
kind: Memcached
metadata:
  name: my-cache
spec:
  replicas: 3
  highAvailability:
    antiAffinityPreset: soft
```

---

## Hard Preset

The `hard` preset uses `requiredDuringSchedulingIgnoredDuringExecution`. The
scheduler will not place two pods from the same Memcached instance on the same
node. If there are fewer available nodes than replicas, pods will remain in
Pending state.

### Generated Affinity Structure

```yaml
spec:
  template:
    spec:
      affinity:
        podAntiAffinity:
          requiredDuringSchedulingIgnoredDuringExecution:
            - topologyKey: kubernetes.io/hostname
              labelSelector:
                matchLabels:
                  app.kubernetes.io/name: memcached
                  app.kubernetes.io/instance: <cr-name>
```

### CR Example

```yaml
apiVersion: memcached.c5c3.io/v1alpha1
kind: Memcached
metadata:
  name: my-cache
spec:
  replicas: 3
  highAvailability:
    antiAffinityPreset: hard
```

---

## No Anti-Affinity (Default)

When `spec.highAvailability` is omitted or `antiAffinityPreset` is nil, the
StatefulSet has no affinity rules. Pods are scheduled freely by the default
scheduler.

```yaml
apiVersion: memcached.c5c3.io/v1alpha1
kind: Memcached
metadata:
  name: my-cache
spec:
  replicas: 3
```

The StatefulSet's `spec.template.spec.affinity` will be nil.

---

## Runtime Behavior

| Action                            | Result                                           |
|-----------------------------------|--------------------------------------------------|
| Set `antiAffinityPreset: soft`    | StatefulSet updated with preferred anti-affinity |
| Set `antiAffinityPreset: hard`    | StatefulSet updated with required anti-affinity  |
| Change `soft` to `hard`           | StatefulSet affinity updated on next reconcile   |
| Change `hard` to `soft`           | StatefulSet affinity updated on next reconcile   |
| Remove `highAvailability` section | StatefulSet affinity cleared to nil              |
| Reconcile twice with same spec    | No StatefulSet update (idempotent)               |

A change to the affinity rules updates the pod template, so the StatefulSet
replaces its pods one at a time: it deletes a pod and then creates the
replacement under the same name. While the `hard` preset is in effect, a
rolling update needs no spare node, because each replacement can use the node
its predecessor freed.

---

## Implementation

The `buildAntiAffinity` function in `internal/controller/deployment.go` is a
pure function that translates the CR spec into a `*corev1.Affinity`:

```go
func buildAntiAffinity(mc *Memcached) *corev1.Affinity
```

- Returns `nil` when `spec.highAvailability` is nil or `antiAffinityPreset` is nil
- Returns an `Affinity` with `PodAntiAffinity` for `soft` or `hard` presets
- Called from `constructStatefulSet`, which sets the result on the pod template spec

The function follows the same pattern as `buildMemcachedArgs` and
`labelsForMemcached`: a pure builder function with no side effects, tested
independently via unit tests.
