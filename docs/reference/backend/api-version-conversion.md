# API Version Conversion (Hub/Spoke)

Reference documentation for the multi-version CRD conversion architecture used to
support both `v1alpha1` and `v1beta1` of the `memcached.c5c3.io` API group.

## Overview

The memcached-operator serves two API versions for the `Memcached` custom resource:

| Version    | Role  | Storage | Status     |
|------------|-------|---------|------------|
| `v1beta1`  | Hub   | Yes     | Active     |
| `v1alpha1` | Spoke | No      | Deprecated |

The Kubernetes API server automatically converts between versions using the
**hub/spoke pattern** from controller-runtime. When a client sends a `v1alpha1`
object, the API server converts it to `v1beta1` (the storage version) before
persisting it to etcd. When a client requests `v1alpha1`, the stored `v1beta1`
object is converted back on the fly.

## Hub/Spoke Pattern

The hub/spoke pattern designates one version as the canonical representation
(the **hub**) and all other versions as **spokes**. Spokes implement conversion
to and from the hub. This avoids an O(n^2) explosion of conversion functions
when adding new versions — each new version only needs two functions (to/from
the hub).

```text
v1alpha1 (spoke)  ──ConvertTo──►  v1beta1 (hub / storage version)
v1alpha1 (spoke)  ◄─ConvertFrom─  v1beta1 (hub / storage version)
```

## Conversion Interfaces

### Hub — `v1beta1.Memcached`

**Source**: `api/v1beta1/memcached_conversion.go`

The hub type implements the `conversion.Hub` interface from controller-runtime.
This is a marker interface with a single empty method:

```go
// Hub marks v1beta1.Memcached as the hub type for conversion.
func (*Memcached) Hub() {}
```

The `+kubebuilder:storageversion` marker on the `Memcached` type in
`api/v1beta1/memcached_types.go` tells controller-gen to set `storage: true`
for `v1beta1` in the generated CRD manifest.

### Spoke — `v1alpha1.Memcached`

**Source**: `api/v1alpha1/memcached_conversion.go`

The spoke type implements the `conversion.Convertible` interface:

```go
type Convertible interface {
    ConvertTo(hub Hub) error
    ConvertFrom(hub Hub) error
}
```

- **`ConvertTo`** — copies all fields from a `v1alpha1.Memcached` into a
  `v1beta1.Memcached`.
- **`ConvertFrom`** — copies all fields from a `v1beta1.Memcached` into a
  `v1alpha1.Memcached`.

Both methods type-assert the `conversion.Hub` argument to `*v1beta1.Memcached`
and return a descriptive error if the assertion fails.

## Field Mapping

Since `v1alpha1` and `v1beta1` have identical schemas, the conversion is a
direct field-by-field copy. No fields are added, removed, or renamed between
versions.

### Conversion Rules

| Category                | Fields                                                      | Strategy                                                      |
|-------------------------|-------------------------------------------------------------|---------------------------------------------------------------|
| `ObjectMeta`            | Name, Namespace, Labels, Annotations, ResourceVersion, etc. | Direct assignment (`dst.ObjectMeta = src.ObjectMeta`)         |
| Scalar spec fields      | `Replicas`, `Image`                                         | Direct assignment                                             |
| Kubernetes types        | `Resources` (`corev1.ResourceRequirements`)                 | Direct assignment (same package, same type)                   |
| Simple optional structs | `MemcachedConfig`, `AutoscalingSpec`, `ServiceSpec`         | Type conversion via `v1beta1.Type(*src.Field)` with nil guard |
| Nested optional structs | `HighAvailabilitySpec`, `MonitoringSpec`, `SecuritySpec`    | Helper functions that recursively convert pointer sub-fields  |
| Status fields           | `Conditions`, `ReadyReplicas`, `ObservedGeneration`         | Direct assignment                                             |
| `TypeMeta`              | `APIVersion`, `Kind`                                        | **Not copied** — set by the conversion framework              |

### Nested Struct Helpers

Three pairs of helper functions handle structs with pointer sub-fields that
cannot use a simple type conversion:

| Helper                           | Nested pointer fields handled                                   |
|----------------------------------|-----------------------------------------------------------------|
| `convertHighAvailabilityTo/From` | `AntiAffinityPreset`, `PodDisruptionBudget`, `GracefulShutdown` |
| `convertMonitoringTo/From`       | `ServiceMonitor`                                                |
| `convertSecurityTo/From`         | `SASL`, `TLS`, `NetworkPolicy`                                  |

Each helper nil-checks pointer fields before converting them individually.

## Key Source Files

| File                                        | Purpose                                                     |
|---------------------------------------------|-------------------------------------------------------------|
| `api/v1beta1/groupversion_info.go`          | Registers `memcached.c5c3.io/v1beta1` with SchemeBuilder    |
| `api/v1beta1/memcached_types.go`            | v1beta1 type definitions with `+kubebuilder:storageversion` |
| `api/v1beta1/memcached_conversion.go`       | `Hub()` marker method                                       |
| `api/v1beta1/zz_generated.deepcopy.go`      | Generated DeepCopy methods for v1beta1 types                |
| `api/v1alpha1/memcached_conversion.go`      | `ConvertTo` / `ConvertFrom` with helper functions           |
| `api/v1alpha1/memcached_conversion_test.go` | Round-trip and edge-case conversion tests                   |

## Testing Strategy

The conversion test suite in `api/v1alpha1/memcached_conversion_test.go` covers:

1. **Interface satisfaction** — compile-time assertions that `v1beta1.Memcached`
   satisfies `conversion.Hub` and `v1alpha1.Memcached` satisfies
   `conversion.Convertible`.
2. **Round-trip (fully populated)** — creates a `v1alpha1.Memcached` with every
   field set, converts to `v1beta1` and back, and asserts `Spec` and `Status`
   are deeply equal via `reflect.DeepEqual`.
3. **Round-trip (minimal object)** — verifies that an object with only
   `Name` and `Namespace` survives the round trip with nil optional fields
   preserved.
4. **ObjectMeta preservation** — confirms Labels, Annotations, Name, Namespace,
   and ResourceVersion survive the round trip.
5. **Wrong hub type** — asserts that passing a non-`v1beta1` hub to `ConvertTo`
   or `ConvertFrom` returns a descriptive error.
6. **Minimal ConvertTo** — verifies that converting an object with nil spec
   fields succeeds without panics.

## v1alpha1 Deprecation Timeline

`v1alpha1` is deprecated as of this release. The `Memcached` type in
`api/v1alpha1/memcached_types.go` carries the comment:

```go
// Deprecated: Use v1beta1.Memcached instead.
```

Planned deprecation timeline:

| Phase                | Action                                                                                                                                       |
|----------------------|----------------------------------------------------------------------------------------------------------------------------------------------|
| Current              | `v1beta1` is the storage version. `v1alpha1` remains served with automatic conversion.                                                       |
| Next minor release   | `v1alpha1` is marked as `deprecated: true` in the CRD served-versions list (Kubernetes 1.28+). Clients receive a deprecation warning header. |
| Future major release | `v1alpha1` is removed from served versions. Existing stored objects are already in `v1beta1` format and require no migration.                |

## Adding a New API Version

To add a future `v1` version following this pattern:

1. Create `api/v1/` with types, `groupversion_info.go`, and a `Hub()` method.
2. Move the `+kubebuilder:storageversion` marker from `v1beta1` to `v1`.
3. Add `ConvertTo`/`ConvertFrom` methods to `v1beta1` (it becomes a spoke).
4. Run `make generate manifests` to regenerate deepcopy and CRD manifests.
5. Write round-trip tests for `v1beta1 <-> v1` conversion.
6. `v1alpha1` continues to convert through `v1beta1` — no changes needed as
   long as `v1beta1` converts to the new hub.
