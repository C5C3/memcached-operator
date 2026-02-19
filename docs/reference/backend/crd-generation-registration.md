# CRD Generation Pipeline and Scheme Registration

Reference documentation for how CRD manifests are generated, how Go types are
registered with the controller-runtime scheme, and how to install/uninstall CRDs
on a Kubernetes cluster.

## Overview

The memcached-operator uses `controller-gen` (from the controller-tools project)
to generate two categories of artifacts from the Go type definitions in
`api/v1alpha1/`:

1. **CRD YAML manifests** — OpenAPI v3 validation schemas, printer columns,
   subresources, and resource metadata derived from kubebuilder markers.
2. **DeepCopy methods** — `DeepCopy`, `DeepCopyInto`, and `DeepCopyObject`
   implementations required by the `runtime.Object` interface.

Both artifacts are checked into the repository. CI verifies they stay in sync
with the Go types via `make verify-manifests`.

## Generation Pipeline

### `make manifests` — CRD and RBAC Generation

```
make manifests
```

Invokes `controller-gen` with:

```
controller-gen rbac:roleName=manager-role crd webhook \
  paths="./..." \
  output:crd:artifacts:config=config/crd/bases
```

**What happens:**

1. `controller-gen` scans all Go packages (`./...`) for types annotated with
   kubebuilder markers (`+kubebuilder:object:root=true`,
   `+kubebuilder:validation:*`, `+kubebuilder:printcolumn:*`, etc.).
2. For each root object type (`Memcached`), it generates a
   `CustomResourceDefinition` YAML manifest.
3. RBAC markers (`+kubebuilder:rbac:*`) on the reconciler produce ClusterRole
   rules in `config/rbac/role.yaml`.

**Output:**

| File | Content |
|------|---------|
| `config/crd/bases/memcached.c5c3.io_memcacheds.yaml` | CRD with OpenAPI schema, printer columns, status subresource |
| `config/rbac/role.yaml` | ClusterRole with RBAC rules from reconciler markers |

**Key markers that drive CRD generation** (defined in `api/v1alpha1/memcached_types.go`):

| Marker | Location | Effect in CRD |
|--------|----------|---------------|
| `+kubebuilder:object:root=true` | `Memcached`, `MemcachedList` | Marks types as CRD root objects |
| `+kubebuilder:subresource:status` | `Memcached` | Enables `/status` subresource |
| `+kubebuilder:printcolumn:*` | `Memcached` | Adds `additionalPrinterColumns` for `kubectl get` |
| `+kubebuilder:validation:Minimum=N` | Spec fields | Sets `minimum` in OpenAPI schema |
| `+kubebuilder:validation:Maximum=N` | Spec fields | Sets `maximum` in OpenAPI schema |
| `+kubebuilder:validation:Pattern=...` | `MaxItemSize` | Sets `pattern` in OpenAPI schema |
| `+kubebuilder:validation:Enum=...` | `AntiAffinityPreset` | Sets `enum` in OpenAPI schema |
| `+kubebuilder:default=...` | Various fields | Sets `default` in OpenAPI schema |

### `make generate` — DeepCopy Code Generation

```
make generate
```

Invokes `controller-gen` with:

```
controller-gen object:headerFile="hack/boilerplate.go.txt" paths="./..."
```

**What happens:**

1. `controller-gen` scans for types annotated with
   `+kubebuilder:object:generate=true` (set as a package-level directive in
   `api/v1alpha1/groupversion_info.go`).
2. For each struct in the package, it generates `DeepCopyInto` and `DeepCopy`
   methods. Root objects additionally get `DeepCopyObject`.

**Output:**

| File | Content |
|------|---------|
| `api/v1alpha1/zz_generated.deepcopy.go` | `DeepCopy*` methods for all types in the package |

The generated file includes:
- A `DO NOT EDIT` header with the license from `hack/boilerplate.go.txt`.
- Methods for every struct: `Memcached`, `MemcachedList`, `MemcachedSpec`,
  `MemcachedStatus`, `MemcachedConfig`, `HighAvailabilitySpec`, `PDBSpec`,
  `MonitoringSpec`, `ServiceMonitorSpec`, `SecuritySpec`, `SASLSpec`, `TLSSpec`.

### Idempotency

Both commands are idempotent. Running `make manifests` or `make generate` twice
in succession produces identical output. This property is verified by the
`verify-manifests` target and in tests.

## Scheme Registration

The controller-runtime scheme maps Go types to Kubernetes GroupVersionKind (GVK)
tuples. Three files form the registration chain:

### 1. `api/v1alpha1/groupversion_info.go`

Declares the API group identity and creates the scheme builder:

```go
var (
    GroupVersion  = schema.GroupVersion{Group: "memcached.c5c3.io", Version: "v1alpha1"}
    SchemeBuilder = &scheme.Builder{GroupVersion: GroupVersion}
    AddToScheme   = SchemeBuilder.AddToScheme
)
```

- `GroupVersion` — the GV tuple used for all types in this package.
- `SchemeBuilder` — collects type registrations via `Register()`.
- `AddToScheme` — a function variable that callers invoke to register all types
  with a given `runtime.Scheme`.

### 2. `api/v1alpha1/memcached_types.go`

Registers the CRD root types in `init()`:

```go
func init() {
    SchemeBuilder.Register(&Memcached{}, &MemcachedList{})
}
```

This makes both types discoverable by GVK:
- `memcached.c5c3.io/v1alpha1, Kind=Memcached` resolves to `*v1alpha1.Memcached`
- `memcached.c5c3.io/v1alpha1, Kind=MemcachedList` resolves to `*v1alpha1.MemcachedList`

### 3. `cmd/main.go`

Wires the scheme into the controller manager:

```go
func init() {
    utilruntime.Must(clientgoscheme.AddToScheme(scheme))
    utilruntime.Must(memcachedv1alpha1.AddToScheme(scheme))
}
```

The `init()` function registers:
1. Core Kubernetes types (`clientgoscheme`) — Pods, Services, Deployments, etc.
2. Memcached types (`memcachedv1alpha1.AddToScheme`) — Memcached and MemcachedList.

The resulting `scheme` is passed to the controller-runtime `Manager`, which uses
it for serialization, deserialization, and watch configuration.

### Registration Flow Diagram

```
groupversion_info.go              memcached_types.go              cmd/main.go
─────────────────────             ─────────────────               ──────────
GroupVersion{                     func init() {                   func init() {
  "memcached.c5c3.io",             SchemeBuilder.Register(          utilruntime.Must(
  "v1alpha1",                         &Memcached{},                   memcachedv1alpha1
}                                     &MemcachedList{},                 .AddToScheme(scheme))
                                    )                               }
SchemeBuilder = &scheme.Builder{  }
  GroupVersion: GroupVersion,                                     ctrl.NewManager(cfg, ctrl.Options{
}                                                                   Scheme: scheme,
                                                                  })
AddToScheme = SchemeBuilder
                .AddToScheme
```

## CRD Installation and Removal

### Install CRDs

```
make install
```

Runs:

```
kustomize build config/crd | kubectl apply -f -
```

This:
1. Builds the CRD manifest using `config/crd/kustomization.yaml`, which
   references `bases/memcached.c5c3.io_memcacheds.yaml`.
2. Applies the CRD to the cluster in `~/.kube/config`.
3. After installation, the API server accepts Memcached CR CRUD operations under
   `memcached.c5c3.io/v1alpha1`.

### Uninstall CRDs

```
make uninstall
```

Runs:

```
kustomize build config/crd | kubectl delete --ignore-not-found=false -f -
```

Removes the CRD and all Memcached custom resources from the cluster.

### Verify Generated Artifacts

```
make verify-manifests
```

Re-runs both `make manifests` and `make generate`, then checks for uncommitted
changes:

```
git diff --exit-code -- config/crd/ api/v1alpha1/zz_generated.deepcopy.go
```

If the working tree is dirty, the target fails with an error message instructing
the developer to run `make manifests generate` and commit the result. This
target is intended for CI pipelines to detect stale generated artifacts.

## Generated CRD Structure

The generated CRD at `config/crd/bases/memcached.c5c3.io_memcacheds.yaml`
contains:

| Section | Value |
|---------|-------|
| `apiVersion` | `apiextensions.k8s.io/v1` |
| `kind` | `CustomResourceDefinition` |
| `metadata.name` | `memcacheds.memcached.c5c3.io` |
| `spec.group` | `memcached.c5c3.io` |
| `spec.scope` | `Namespaced` |
| `spec.names.kind` | `Memcached` |
| `spec.names.listKind` | `MemcachedList` |
| `spec.names.plural` | `memcacheds` |
| `spec.names.singular` | `memcached` |
| `spec.versions[0].name` | `v1alpha1` |

### Printer Columns

| Column | Type | JSONPath |
|--------|------|----------|
| Replicas | integer | `.spec.replicas` |
| Ready | integer | `.status.readyReplicas` |
| Age | date | `.metadata.creationTimestamp` |

### Validation Constraints

| Field | Constraint |
|-------|-----------|
| `spec.replicas` | minimum: 0, maximum: 64, default: 1 |
| `spec.image` | default: `memcached:1.6` |
| `spec.memcached.maxMemoryMB` | minimum: 16, maximum: 65536, default: 64 |
| `spec.memcached.maxConnections` | minimum: 1, maximum: 65536, default: 1024 |
| `spec.memcached.threads` | minimum: 1, maximum: 128, default: 4 |
| `spec.memcached.maxItemSize` | pattern: `^[0-9]+(k\|m)$`, default: `1m` |
| `spec.highAvailability.antiAffinityPreset` | enum: [soft, hard], default: `soft` |
| `spec.monitoring.exporterImage` | default: `prom/memcached-exporter:v0.15.4` |

### Status Subresource

The CRD enables the `/status` subresource (`spec.versions[0].subresources.status`),
which means:
- Status updates via the `/status` subresource do not increment
  `metadata.generation`.
- Spec updates via the main resource endpoint do increment `metadata.generation`.
- The reconciler can update status independently of spec changes.

## Common Tasks

### Adding a new field to the CRD

1. Add the field to the appropriate struct in `api/v1alpha1/memcached_types.go`
   with JSON tags and kubebuilder markers.
2. Run `make manifests generate` to regenerate the CRD YAML and DeepCopy methods.
3. Verify the generated output: `make verify-manifests` should pass.
4. Commit the Go type changes and the regenerated files together.

### Adding a new validation constraint

Add a kubebuilder marker above the field:

```go
// +kubebuilder:validation:Minimum=1
// +kubebuilder:validation:Maximum=100
// +kubebuilder:default=10
MyField int32 `json:"myField,omitempty"`
```

Then run `make manifests` to regenerate the CRD.

### Adding a new printer column

Add a `+kubebuilder:printcolumn` marker to the root type (`Memcached`):

```go
// +kubebuilder:printcolumn:name="MyCol",type="string",JSONPath=".spec.myField"
```

Then run `make manifests` to regenerate the CRD.
