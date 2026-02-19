# Topology Spread Constraints

Reference documentation for the topology spread constraints feature that controls
how Memcached pods are distributed across topology domains (e.g. availability zones).

**Source**: `internal/controller/deployment.go`

## Overview

The `spec.highAvailability.topologySpreadConstraints` field configures topology
spread constraints on the Deployment's pod template. This allows operators to
control how Memcached pods are distributed across failure domains such as zones,
regions, or nodes.

Unlike `antiAffinityPreset` which provides opinionated presets, topology spread
constraints are a direct passthrough: the constraints defined in the CR are
applied verbatim to the Deployment pod template without transformation.

Each constraint supports all standard Kubernetes `TopologySpreadConstraint`
fields:

| Field | Type | Description |
|-------|------|-------------|
| `maxSkew` | int32 | Maximum allowed difference in pod count between topology domains |
| `topologyKey` | string | Node label key used to define topology domains (e.g. `topology.kubernetes.io/zone`) |
| `whenUnsatisfiable` | string | `DoNotSchedule` or `ScheduleAnyway` |
| `labelSelector` | LabelSelector | Selector to identify the set of pods to spread |
| `matchLabelKeys` | []string | Pod label keys used to calculate spreading |
| `minDomains` | int32 | Minimum number of eligible domains |
| `nodeAffinityPolicy` | string | How node affinity/selector is treated (`Honor` or `Ignore`) |
| `nodeTaintsPolicy` | string | How node taints are treated (`Honor` or `Ignore`) |

When `spec.highAvailability` is nil or `topologySpreadConstraints` is nil or
empty, no topology spread constraints are set on the Deployment.

---

## CRD Field Path

```
spec.highAvailability.topologySpreadConstraints[]
```

Defined in `api/v1alpha1/memcached_types.go` on the `HighAvailabilitySpec`
struct:

```go
TopologySpreadConstraints []corev1.TopologySpreadConstraint `json:"topologySpreadConstraints,omitempty,omitzero"`
```

---

## Zone-Aware Spreading

The most common use case is spreading pods across availability zones to survive
zone failures.

### CR Example

```yaml
apiVersion: memcached.c5c3.io/v1alpha1
kind: Memcached
metadata:
  name: my-cache
spec:
  replicas: 3
  highAvailability:
    topologySpreadConstraints:
      - maxSkew: 1
        topologyKey: topology.kubernetes.io/zone
        whenUnsatisfiable: DoNotSchedule
        labelSelector:
          matchLabels:
            app.kubernetes.io/name: memcached
            app.kubernetes.io/instance: my-cache
```

### Generated Deployment Structure

```yaml
spec:
  template:
    spec:
      topologySpreadConstraints:
        - maxSkew: 1
          topologyKey: topology.kubernetes.io/zone
          whenUnsatisfiable: DoNotSchedule
          labelSelector:
            matchLabels:
              app.kubernetes.io/name: memcached
              app.kubernetes.io/instance: my-cache
```

---

## Multiple Constraints

Multiple constraints can be specified to spread pods across different topology
dimensions simultaneously.

### CR Example

```yaml
apiVersion: memcached.c5c3.io/v1alpha1
kind: Memcached
metadata:
  name: my-cache
spec:
  replicas: 6
  highAvailability:
    topologySpreadConstraints:
      - maxSkew: 1
        topologyKey: topology.kubernetes.io/zone
        whenUnsatisfiable: DoNotSchedule
        labelSelector:
          matchLabels:
            app.kubernetes.io/name: memcached
            app.kubernetes.io/instance: my-cache
      - maxSkew: 1
        topologyKey: kubernetes.io/hostname
        whenUnsatisfiable: ScheduleAnyway
        labelSelector:
          matchLabels:
            app.kubernetes.io/name: memcached
            app.kubernetes.io/instance: my-cache
```

This configuration first ensures even distribution across zones (hard
requirement), then tries to spread across nodes within each zone (best-effort).

---

## Combined with Anti-Affinity

Topology spread constraints and `antiAffinityPreset` can be used together. They
are independent — each sets a different field on the pod template:

- `antiAffinityPreset` sets `spec.template.spec.affinity`
- `topologySpreadConstraints` sets `spec.template.spec.topologySpreadConstraints`

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
    topologySpreadConstraints:
      - maxSkew: 1
        topologyKey: topology.kubernetes.io/zone
        whenUnsatisfiable: DoNotSchedule
        labelSelector:
          matchLabels:
            app.kubernetes.io/name: memcached
            app.kubernetes.io/instance: my-cache
```

Changing or removing one does not affect the other.

---

## No Constraints (Default)

When `spec.highAvailability` is omitted, or `topologySpreadConstraints` is nil
or an empty list, the Deployment has no topology spread constraints. Pods are
scheduled freely by the default scheduler.

```yaml
apiVersion: memcached.c5c3.io/v1alpha1
kind: Memcached
metadata:
  name: my-cache
spec:
  replicas: 3
```

The Deployment's `spec.template.spec.topologySpreadConstraints` will be nil.

---

## Runtime Behavior

| Action | Result |
|--------|--------|
| Add `topologySpreadConstraints` | Deployment updated with constraints on next reconcile |
| Modify a constraint (e.g. change `maxSkew`) | Deployment updated on next reconcile |
| Remove `topologySpreadConstraints` | Deployment constraints cleared to nil |
| Remove `highAvailability` section | Deployment constraints cleared to nil |
| Reconcile twice with same spec | No Deployment update (idempotent) |
| Add/remove `antiAffinityPreset` | No effect on topology spread constraints |

---

## Implementation

The `buildTopologySpreadConstraints` function in
`internal/controller/deployment.go` is a pure function that extracts constraints
from the CR spec:

```go
func buildTopologySpreadConstraints(mc *Memcached) []corev1.TopologySpreadConstraint
```

- Returns `nil` when `spec.highAvailability` is nil
- Returns `nil` when `topologySpreadConstraints` is nil or empty
- Returns the constraints slice as-is when populated (direct passthrough)
- Called from `constructDeployment`, which sets the result on the pod template spec

The function follows the same pattern as `buildAntiAffinity`: a pure builder
function with no side effects, tested independently via unit tests.
