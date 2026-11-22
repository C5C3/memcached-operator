# Manager Cache

Reference documentation for the informer cache of the operator's manager: which
objects it holds, in which form, how much memory each one costs, and how to size
the operator's memory limit from that.

**Source**: `internal/controller/cache.go`, `internal/controller/memcached_controller.go`,
`internal/controller/reconcile_resource.go`, `internal/controller/cache_footprint_test.go`

## What Is Cached

`cmd/main.go` builds the manager's cache from `controller.CacheOptions`. Every
kind is cached only for objects labeled
`app.kubernetes.io/managed-by: memcached-operator`, the label that every object
the operator creates carries. Two kinds are exempt: Memcached resources are
cached whatever their labels, and Secrets are cached as stripped metadata.
`--watch-namespaces` limits every kind to the listed namespaces (see
[Namespace-Scoped Mode](namespace-scoped-mode.md)).

| Kind                      | Cached form                    | Label selector | Number of cached objects                              | Bytes per object |
|---------------------------|--------------------------------|----------------|-------------------------------------------------------|------------------|
| `Memcached`               | Full object                    | None           | Memcached instances in the watched namespaces         | 4.7 KiB          |
| `StatefulSet`             | Full object                    | managed-by     | One per Memcached instance                            | 10.2 KiB         |
| `Service`                 | Full object                    | managed-by     | One per Memcached instance                            | 3.2 KiB          |
| `HorizontalPodAutoscaler` | Full object                    | managed-by     | One per instance with autoscaling enabled             | 2.7 KiB          |
| `PodDisruptionBudget`     | Full object                    | managed-by     | One per instance with the PodDisruptionBudget enabled | 2.4 KiB          |
| `NetworkPolicy`           | Full object                    | managed-by     | One per instance with the NetworkPolicy enabled       | 2.5 KiB          |
| `ServiceMonitor`          | Full object                    | managed-by     | One per instance with the ServiceMonitor enabled      | 3.0 KiB          |
| `Secret`                  | Metadata, five fields stripped | None           | Secrets in the watched namespaces                     | 0.6 KiB          |
| `Deployment`              | Full object                    | managed-by     | Legacy Deployments not yet migrated                   | Not measured     |

"managed-by" stands for the selector
`app.kubernetes.io/managed-by=memcached-operator`, which every list and watch
request for that kind carries. The selector is the cache-wide default, not one
entry per kind, so a kind that is read through the cache later is bounded
without a code change. The bytes per object come from the footprint spec (see
[How the Figures Were Measured](#how-the-figures-were-measured)).

The cache size follows the number of Memcached instances and the number of
Secrets in the watched namespaces. Other Deployments, Services, and the other
owned kinds in those namespaces are not cached.

---

## Secrets

The Secret watch in `SetupWithManager` uses `builder.OnlyMetadata`, so the
manager lists and watches Secrets as `metav1.PartialObjectMetadata` and never
receives a payload through the cache. The cache transform
`stripSecretMetadata` sets these five fields to `nil` before an entry is
stored:

- `labels`
- `annotations`
- `managedFields`
- `ownerReferences`
- `finalizers`

The name, namespace, UID, `resourceVersion`, and timestamps stay. That is
enough for `mapSecretToMemcached`, which maps a Secret event to the Memcached
instances that reference the Secret by name.

The payload is read live: `reconcileStatefulSet` reads each referenced Secret
from the API server through the reconciler's `APIReader`. An instance with SASL
or TLS enabled sends one GET per distinct referenced Secret, at most two, per
reconcile; an instance without either sends none. The client's rate limiter
bounds the total. A read that fails with anything other than NotFound fails
the reconcile before the StatefulSet is touched (see
[Secret Hash Computation and References](secret-hash-and-references.md#fetchreferencedsecrets)).

---

## The managed-by Label

An object without the managed-by label is invisible to the cache, while it
still exists on the API server. The reconciler handles the three cases this
creates:

- **An owned object lost the label.** The watch reports the object as deleted,
  which enqueues the Memcached that owns it. The create in `reconcileResource`
  then fails with AlreadyExists, and `updateLive` reads the object from the API
  server, logs `Resource exists but is not in the cache, reading it from the API server`,
  and updates it because the Memcached controls it. The update restores the
  label, so the object re-enters the cache.
- **A same-named object that the Memcached does not control.** `updateLive`
  leaves it alone. The reconcile fails with
  `<Kind> <namespace>/<name> exists but is not controlled by Memcached <name>: ...`,
  a `Warning` event with reason `AlreadyExists` is recorded on the Memcached,
  and the request is retried with backoff until the object is removed or
  renamed (see
  [Idempotent Create-or-Update](idempotent-create-or-update.md#alreadyexists-objects-the-cache-cannot-see)).
- **A legacy Deployment without the label.** The Deployment informer holds
  only labeled Deployments. Every Deployment an earlier release created
  carries the label, but one whose label was removed is not found and so not
  deleted (see
  [Legacy Deployment Migration](deployment-reconciliation.md#legacy-deployment-migration)).

---

## Sizing the Memory Limit

The expected cache size is the sum, over the kinds in the table, of the number
of cached objects times the bytes per object. An instance with every optional
object enabled costs 28.7 KiB for the Memcached resource and
its six owned objects. For example, 100 such instances and 1,000 Secrets in the
watched namespaces come to 100 × 28.7 KiB + 1,000 × 0.6 KiB = 3,470 KiB, about
3.4 MiB. The figures cover the cached objects only, not the rest of the
manager's memory, which the footprint spec does not measure.

The limit is `resources.limits.memory` of the manager container, 128Mi by
default on both install paths:

- Helm: `resources.limits.memory` in `charts/memcached-operator/values.yaml`
- Kustomize: `resources.limits.memory` of the `manager` container in
  `config/manager/manager.yaml`

---

## How the Figures Were Measured

The figures come from one run of the footprint spec
(`internal/controller/cache_footprint_test.go`, Ginkgo label `footprint`) with
Go 1.27.4 and envtest Kubernetes 1.32.0, each rounded up to the next 0.1 KiB:

```bash
make kustomize envtest
KUBEBUILDER_ASSETS="$(bin/setup-envtest use 1.32.0 --bin-dir "$PWD/bin" -p path)" \
  go test ./internal/controller/ -run TestControllers -count=1 -v \
  -ginkgo.label-filter=footprint -ginkgo.v
```

The spec creates 100 Memcached instances in a fresh namespace, each with its
own SASL Secret holding 16384 random bytes and with autoscaling, the
PodDisruptionBudget, the ServiceMonitor, and the NetworkPolicy enabled, so each
of the eight measured kinds has 100 objects. For each kind it builds a cache
from `controller.CacheOptions` scoped to that namespace, syncs the kind's
informer, and reads `runtime.MemStats.HeapAlloc` after two garbage collections
before and after. It repeats the measurement against an empty namespace, which
gives the fixed cost of a cache and an informer, and reports
`cache-footprint kind=<Kind> objects=100 bytes_per_object=<n>` with
`n = (populated - empty) / 100`. The spec fails when a cached Secret takes 4096
bytes or more.

Under envtest no Kubernetes controller runs, so the StatefulSet,
HorizontalPodAutoscaler, and PodDisruptionBudget carry no status written by
their controllers and no `managedFields` entry for that status. On a cluster
they carry both, and their cached objects are larger than the table states.
