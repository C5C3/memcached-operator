# Namespace-Scoped Mode

Reference documentation for restricting the operator to watch specific
namespaces instead of the entire cluster, using the `--watch-namespaces` CLI
flag and the `config/namespace-scoped/` Kustomize overlay for RBAC scoping.

**Source**: `cmd/main.go`, `config/namespace-scoped/kustomization.yaml`

## Overview

By default the operator watches all namespaces (cluster-scoped). Passing the
`--watch-namespaces` flag restricts the controller-runtime cache to only the
listed namespaces. This follows the principle of least privilege: the operator
creates informers only for the namespaces it manages.

When `--watch-namespaces` is set, the operator populates
`ctrl.Options.Cache.DefaultNamespaces` with a `map[string]cache.Config` entry
for each namespace. When the flag is omitted or empty, `DefaultNamespaces`
remains `nil` and controller-runtime falls back to cluster-wide watches.

CRDs are cluster-scoped resources and must still be installed cluster-wide
regardless of namespace restriction.

---

## CLI Flag

```text
--watch-namespaces=<ns1>,<ns2>,...
```

| Property    | Value                                                             |
|-------------|-------------------------------------------------------------------|
| Flag name   | `--watch-namespaces`                                              |
| Type        | `string`                                                          |
| Default     | `""` (empty — watch all namespaces)                               |
| Delimiter   | Comma-separated                                                   |
| Whitespace  | Leading/trailing whitespace around each namespace name is trimmed |
| Empty items | Trailing commas and empty segments are ignored                    |

### Examples

Watch two namespaces:

```bash
manager --watch-namespaces=staging,production
```

Watch a single namespace:

```bash
manager --watch-namespaces=production
```

Watch all namespaces (default behavior):

```bash
manager
```

Whitespace is trimmed automatically:

```bash
manager --watch-namespaces=" ns1 , ns2 "
# Equivalent to --watch-namespaces=ns1,ns2
```

### Startup Logging

When namespaces are specified, the operator logs:

```text
INFO  setup  watching namespaces  {"namespaces": ["staging", "production"]}
```

When no namespaces are specified:

```text
INFO  setup  watching all namespaces
```

---

## Parse Logic

The `parseWatchNamespaces` function in `cmd/main.go` converts the flag value
into the map consumed by controller-runtime:

```go
func parseWatchNamespaces(namespaces string) map[string]cache.Config
```

| Input           | Output                                                                   |
|-----------------|--------------------------------------------------------------------------|
| `""`            | `nil` (cluster-scoped)                                                   |
| `"   "`         | `nil` (whitespace-only treated as empty)                                 |
| `"production"`  | `map[string]cache.Config{"production": {}}`                              |
| `" ns1 , ns2 "` | `map[string]cache.Config{"ns1": {}, "ns2": {}}`                          |
| `"ns1,ns2,"`    | `map[string]cache.Config{"ns1": {}, "ns2": {}}` (trailing comma ignored) |

Each map value is an empty `cache.Config{}`. The resulting map is assigned to
`ctrl.Options.Cache.DefaultNamespaces`, which tells controller-runtime to scope
its informer cache to only the listed namespaces.

---

## Kustomize Overlay: Namespace-Scoped RBAC

The `config/namespace-scoped/` overlay converts the operator's ClusterRole and
ClusterRoleBinding to namespace-scoped Role and RoleBinding resources. Use this
overlay when deploying the operator with `--watch-namespaces` to match RBAC
scope to cache scope.

**Path**: `config/namespace-scoped/kustomization.yaml`

### What It Patches

| Original Resource                        | Patched To                        |
|------------------------------------------|-----------------------------------|
| `ClusterRole/manager-role`               | `Role/manager-role`               |
| `ClusterRoleBinding/manager-rolebinding` | `RoleBinding/manager-rolebinding` |
| `roleRef.kind: ClusterRole`              | `roleRef.kind: Role`              |

The overlay uses JSON patches (`op: replace`) to change the `kind` fields. It
does **not** modify `leader-election-role` (already a Role) or the metrics RBAC
resources (must remain cluster-scoped because they reference cluster-scoped
resources like `tokenreviews` and `subjectaccessreviews`).

### Building the Overlay

```bash
kustomize build config/namespace-scoped/
```

### Using with a Full Deployment

Reference the overlay from your main kustomization:

```yaml
apiVersion: kustomize.config.k8s.io/v1beta1
kind: Kustomization
resources:
  - ../default
  - ../namespace-scoped
```

---

## Deployment Considerations

### CRDs Are Cluster-Scoped

Custom Resource Definitions (CRDs) are always cluster-scoped in Kubernetes.
Even when deploying the operator in namespace-scoped mode, the Memcached CRD
must be installed cluster-wide:

```bash
kubectl apply -f config/crd/bases/
```

### Multi-Tenant Deployments

To run separate operator instances per namespace (or set of namespaces),
deploy each instance with its own `--watch-namespaces` value and
namespace-scoped RBAC. Each instance manages only its assigned namespaces.

### Backward Compatibility

Omitting `--watch-namespaces` preserves the default cluster-wide behavior.
Existing deployments require no configuration changes.
