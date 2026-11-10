# StatefulSet Reconciliation

Reference documentation for the StatefulSet reconciliation logic that creates and
updates an `apps/v1` StatefulSet for each Memcached CR, and that removes the
Deployment earlier operator releases ran the pods in.

**Source**: `internal/controller/deployment.go`, `internal/controller/memcached_controller.go`

The page and the source file keep their earlier names; their content describes
the StatefulSet.

## Overview

When a Memcached CR is created or updated, the reconciler ensures a matching
StatefulSet exists in the same namespace. The StatefulSet is named
`statefulSetName(<cr-name>)`, which is the CR name unless the CR name has more
than 52 characters (see [StatefulSet Name](#statefulset-name)). The headless
Service named like the CR governs it through `spec.serviceName`, so every pod
gets a DNS name that survives a restart:
`<statefulset>-<i>.<cr-name>.<namespace>.svc`.

The StatefulSet is constructed from the CR spec using pure builder functions,
then applied via `controllerutil.CreateOrUpdate` for idempotent create/update
semantics. A controller owner reference on the StatefulSet enables automatic
garbage collection when the Memcached CR is deleted. Once the StatefulSet is
reconciled, the reconciler deletes the Deployment that the CR controls, if one
is left from an earlier release (see
[Legacy Deployment Migration](#legacy-deployment-migration)).

---

## Labels

`labelsForMemcached(name string)` returns the standard Kubernetes recommended
labels applied to both the StatefulSet and its pod template:

```go
func labelsForMemcached(name string) map[string]string {
    return map[string]string{
        "app.kubernetes.io/name":       "memcached",
        "app.kubernetes.io/instance":   name,
        "app.kubernetes.io/managed-by": "memcached-operator",
    }
}
```

| Label Key                      | Value                | Purpose                                         |
|--------------------------------|----------------------|-------------------------------------------------|
| `app.kubernetes.io/name`       | `memcached`          | Identifies the application                      |
| `app.kubernetes.io/instance`   | `<cr-name>`          | Distinguishes instances of the same application |
| `app.kubernetes.io/managed-by` | `memcached-operator` | Identifies the managing controller              |

These labels are used as the StatefulSet's `spec.selector.matchLabels` and on
the pod template `metadata.labels`, ensuring the StatefulSet manages the correct
pods. The `app.kubernetes.io/instance` value is always the full CR name, also
when the StatefulSet name is shortened.

The StatefulSet metadata and the pod template also carry
`app.kubernetes.io/version`, set to the image tag (for example `1.6.29` for
`memcached:1.6.29`). The selector leaves it out, so an image change does not
change the selector.

---

## Memcached CLI Arguments

`buildMemcachedArgs(config *MemcachedConfig, sasl *SASLSpec)` translates the
CRD's `spec.memcached` fields into memcached command-line flags. When `sasl` is
non-nil and enabled, the `-Y` flag is appended pointing to the SASL password
file mount path.

### Flag Mapping

| CRD Field        | Flag | Default | Example Output                                                                        |
|------------------|------|---------|---------------------------------------------------------------------------------------|
| `maxMemoryMB`    | `-m` | `64`    | `["-m", "128"]`                                                                       |
| `maxConnections` | `-c` | `1024`  | `["-c", "2048"]`                                                                      |
| `threads`        | `-t` | `4`     | `["-t", "8"]`                                                                         |
| `maxItemSize`    | `-I` | `"1m"`  | `["-I", "2m"]`                                                                        |
| `verbosity`      | `-v` | `0`     | `0`: none, `1`: `-v`, `2`: `-vv`                                                      |
| SASL enabled     | `-Y` | —       | `/etc/memcached/sasl/password-file` (see [SASL Authentication](#sasl-authentication)) |
| `extraArgs`      | —    | `[]`    | Appended after all flags                                                              |

### Default Arguments

When `spec.memcached` is `nil` or all fields are zero-valued, the produced
argument list is:

```text
["-m", "64", "-c", "1024", "-t", "4", "-I", "1m"]
```

### Verbosity Handling

| `spec.memcached.verbosity` | Flags Appended |
|----------------------------|----------------|
| `0` (default)              | (none)         |
| `1`                        | `"-v"`         |
| `2`                        | `"-vv"`        |

### Argument Ordering

Arguments are appended in a fixed order:

1. Standard flags (`-m`, `-c`, `-t`, `-I`)
2. Verbosity (`-v` or `-vv`)
3. SASL flag (`-Y /etc/memcached/sasl/password-file`) — only when SASL is enabled
4. Extra arguments (`spec.memcached.extraArgs`)

### Extra Arguments

`spec.memcached.extraArgs` are appended **after** all other flags, preserving
order. This allows passing arbitrary memcached flags not covered by typed fields:

```yaml
spec:
  memcached:
    maxMemoryMB: 128
    extraArgs: ["-o", "modern", "-B", "auto"]
```

Produces: `["-m", "128", "-c", "1024", "-t", "4", "-I", "1m", "-o", "modern", "-B", "auto"]`

---

## StatefulSet Name

`statefulSetName(name string) string` returns the StatefulSet name for a
Memcached CR named `name`:

```go
const maxStatefulSetNameLength = 52

const statefulSetNameHashLength = 8

func statefulSetName(name string) string {
    if len(name) <= maxStatefulSetNameLength {
        return name
    }
    sum := sha256.Sum256([]byte(name))
    prefixLength := maxStatefulSetNameLength - 1 - statefulSetNameHashLength
    return name[:prefixLength] + "-" + hex.EncodeToString(sum[:statefulSetNameHashLength/2])
}
```

| CR name length | StatefulSet name                                                                       |
|----------------|----------------------------------------------------------------------------------------|
| 52 or fewer    | The CR name, unchanged                                                                 |
| 53 to 63       | First 43 characters, `-`, and the first 8 hex characters of the SHA-256 of the CR name |

A shortened name is always exactly 52 characters. Two long names that share
their first 43 characters still get different StatefulSet names, because the
hash covers the full name. For example, the 60-character CR name
`token-cache-for-the-identity-service-in-the-region-eu-de-one` yields the
StatefulSet `token-cache-for-the-identity-service-in-the-4390109e`.

### Why 52 Characters

The StatefulSet controller labels every pod it creates with
`controller-revision-hash: <statefulset>-<hash>`, where the hash has up to 10
characters. Label values are limited to 63 characters, so a StatefulSet name
longer than 52 characters (52 + 1 + 10 = 63) can never create pods. Memcached
names of 53 to 63 characters are valid (the Service name caps them at 63), so
the operator shortens the StatefulSet name instead of rejecting the CR.

### Where Each Name Is Used

| Object or field                                | Name                             |
|------------------------------------------------|----------------------------------|
| StatefulSet `metadata.name`                    | `statefulSetName(<cr-name>)`     |
| Pod names and the host prefix in `serverList`  | `statefulSetName(<cr-name>)-<i>` |
| HPA `scaleTargetRef.name`                      | `statefulSetName(<cr-name>)`     |
| StatefulSet `spec.serviceName`                 | `<cr-name>`                      |
| Headless Service, PDB, and HPA `metadata.name` | `<cr-name>`                      |
| `app.kubernetes.io/instance` label             | `<cr-name>`                      |

---

## Replica Count

`statefulSetReplicas(mc *Memcached, current *int32) *int32` returns the replica
count for the StatefulSet. It never returns nil.

```go
func statefulSetReplicas(mc *memcachedv1beta1.Memcached, current *int32) *int32 {
    replicas := memcachedv1beta1.DefaultReplicas
    switch {
    case !mc.IsAutoscalingEnabled():
        if mc.Spec.Replicas != nil {
            replicas = *mc.Spec.Replicas
        }
    case current != nil:
        replicas = *current
    case mc.Spec.Autoscaling.MinReplicas != nil:
        replicas = *mc.Spec.Autoscaling.MinReplicas
    }
    return &replicas
}
```

| Autoscaling | `current` | `spec.autoscaling.minReplicas` | Result                                             |
|-------------|-----------|--------------------------------|----------------------------------------------------|
| Off         | (ignored) | (ignored)                      | `spec.replicas`, or `1` (`DefaultReplicas`) if nil |
| On          | Set       | (ignored)                      | `current`, the count the HPA last set              |
| On          | nil       | Set                            | `minReplicas`                                      |
| On          | nil       | nil                            | `1` (`DefaultReplicas`)                            |

`constructStatefulSet` reads `sts.Spec.Replicas` as `current` before it replaces
the spec. Inside `controllerutil.CreateOrUpdate`, `sts` holds the stored
object, so with autoscaling on the reconciler writes the HPA's count back
unchanged. A new StatefulSet has no stored count and starts at `minReplicas`,
or 1.

Earlier releases left `replicas` nil while autoscaling was on. The API server
defaults a nil `replicas` to 1, so the desired spec differed from the stored
count on every reconcile, and each update undid the HPA's scale-up. On a
StatefulSet that would delete every pod above ordinal 0 and shrink
`status.serverList`.

---

## StatefulSet Construction

`constructStatefulSet(mc *Memcached, sts *StatefulSet, secretHash, restartTrigger string)`
sets the desired state of the StatefulSet in-place. It is called within the
`controllerutil.CreateOrUpdate` mutate function so that both creation and updates
use identical logic. The `secretHash` and `restartTrigger` parameters are written
as pod template annotations (`memcached.c5c3.io/secret-hash` and
`memcached.c5c3.io/restart-trigger` respectively), causing Kubernetes to roll
pods when these values change.

### StatefulSet Spec

| Field                  | Value                                                                    |
|------------------------|--------------------------------------------------------------------------|
| `replicas`             | `statefulSetReplicas(mc, current)` (see [Replica Count](#replica-count)) |
| `selector.matchLabels` | `labelsForMemcached(<cr-name>)`                                          |
| `serviceName`          | `<cr-name>`, the headless Service                                        |
| `podManagementPolicy`  | `Parallel`                                                               |
| `updateStrategy.type`  | `RollingUpdate`                                                          |
| `template`             | The pod template described below                                         |
| `volumeClaimTemplates` | Not set; memcached keeps its data in memory                              |

`podManagementPolicy: Parallel` lets the StatefulSet controller create and
delete pods without waiting for each pod in turn. Memcached pods share no
state, so there is no reason to wait, and `OrderedReady` would let one stuck
pod block every higher ordinal. The field is immutable after creation, so
`constructStatefulSet` sets the same value on create and on every update.

### Spec Defaults

| Field       | Source           | Default           |
|-------------|------------------|-------------------|
| `replicas`  | `spec.Replicas`  | `1`               |
| `image`     | `spec.Image`     | `"memcached:1.6"` |
| `args`      | `spec.Memcached` | See default args  |
| `resources` | `spec.Resources` | (empty)           |

### Container Specification

The pod template contains the `memcached` container:

| Property       | Value                                                                                  |
|----------------|----------------------------------------------------------------------------------------|
| `name`         | `memcached`                                                                            |
| `image`        | From `spec.Image` (default `memcached:1.6`)                                            |
| `args`         | Built by `buildMemcachedArgs`                                                          |
| `resources`    | From `spec.Resources` (empty if nil)                                                   |
| `ports`        | `memcached`: 11211/TCP                                                                 |
| `volumeMounts` | SASL credentials mount (when enabled, see [SASL Authentication](#sasl-authentication)) |

The pod template is the same one earlier releases put into the Deployment:
args, ports, probes, volumes, the exporter sidecar, affinity, topology spread
constraints, security contexts, and pod annotations do not change.

### Container Port

```go
corev1.ContainerPort{
    Name:          "memcached",
    ContainerPort: 11211,
    Protocol:      corev1.ProtocolTCP,
}
```

The named port `memcached` is referenced by health probes using
`intstr.FromString("memcached")`.

### Health Probes

Both probes use TCP socket checks on the named port `memcached` (11211):

| Probe            | Type       | Port        | InitialDelay | Period |
|------------------|------------|-------------|--------------|--------|
| `livenessProbe`  | TCP socket | `memcached` | 10s          | 10s    |
| `readinessProbe` | TCP socket | `memcached` | 5s           | 5s     |

The readiness probe gates traffic to the pod. The liveness probe restarts
the container if memcached becomes unresponsive.

### Update Strategy

```go
appsv1.StatefulSetUpdateStrategy{
    Type: appsv1.RollingUpdateStatefulSetStrategyType,
}
```

On a pod template change, the StatefulSet controller replaces the pods one at a
time, from the highest ordinal down. It deletes each pod, creates its
replacement under the same name, and waits until the replacement is Ready
before it moves on. `podManagementPolicy: Parallel` applies to scaling only and
does not change this order.

| Property          | Effect                                                                            |
|-------------------|-----------------------------------------------------------------------------------|
| No surge          | No extra pod is created; the StatefulSet has no `maxSurge` counterpart            |
| One pod at a time | Each pod is unreachable for the seconds its replacement needs to become Ready     |
| Stable pod names  | A replaced pod keeps its name and DNS name, so `status.serverList` stays the same |
| Single replica    | An instance with one replica has no ready pod while that pod is replaced          |

Each replaced pod starts with an empty cache. That was also true of the new
pods the Deployment created.

---

## SASL Authentication

When `spec.security.sasl.enabled` is `true`, the operator configures memcached
for SASL authentication by mounting a credentials Secret and adding the `-Y`
flag to the container arguments.

### Configuration

```yaml
spec:
  security:
    sasl:
      enabled: true
      credentialsSecretRef:
        name: memcached-sasl-credentials
```

The referenced Secret must contain a `password-file` key with the SASL password
file content (username:password pairs in memcached's expected format).

### Helper Functions

**`buildSASLVolume(mc *Memcached) *corev1.Volume`** — Returns a Volume named
`sasl-credentials` that references the Secret from
`spec.security.sasl.credentialsSecretRef.name`, or `nil` when SASL is not
enabled (security is nil, SASL is nil, or `enabled` is `false`).

**`buildSASLVolumeMount(mc *Memcached) *corev1.VolumeMount`** — Returns a
read-only VolumeMount named `sasl-credentials` at `/etc/memcached/sasl/`, or
`nil` when SASL is not enabled.

### Volume and Mount Details

| Property      | Value                                  |
|---------------|----------------------------------------|
| Volume name   | `sasl-credentials`                     |
| Volume source | `Secret` (from `credentialsSecretRef`) |
| Mount path    | `/etc/memcached/sasl/`                 |
| Read-only     | `true`                                 |
| Secret key    | `password-file`                        |

### Container Args

When SASL is enabled, `buildMemcachedArgs` appends `-Y /etc/memcached/sasl/password-file`
after verbosity flags and before `extraArgs`:

```text
["-m", "64", "-c", "1024", "-t", "4", "-I", "1m", "-Y", "/etc/memcached/sasl/password-file"]
```

### Integration with constructStatefulSet

The SASL volume mount is added **only** to the `memcached` container. When
monitoring is enabled, the `exporter` sidecar does **not** receive the SASL
volume mount. SASL coexists with all other features:

| Feature                    | Interaction                                                    |
|----------------------------|----------------------------------------------------------------|
| Pod security context       | SASL volume/mount added alongside pod-level security settings  |
| Container security context | SASL mount present on the same container with security context |
| Monitoring sidecar         | Exporter container does **not** get the SASL volume mount      |
| Graceful shutdown          | Lifecycle preStop hook coexists with SASL volume mount         |
| Extra args                 | `-Y` flag appears before `extraArgs` in argument list          |

### Disabled Behavior

When SASL is not enabled (`spec.security` is nil, `spec.security.sasl` is nil,
or `spec.security.sasl.enabled` is `false`):

- No `-Y` flag in container args
- No `sasl-credentials` volume on the pod
- No SASL volume mount on any container
- Existing Memcached instances continue to work unchanged

### RBAC

The operator's ClusterRole includes `get`, `list`, `watch` permissions for
`core/v1` Secrets to support reading the SASL credentials Secret. This is
generated from the RBAC marker on the controller:

```go
//+kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch
```

---

## Reconciliation Method

`reconcileStatefulSet(ctx, mc *Memcached) ([]string, error)` on
`MemcachedReconciler` ensures the StatefulSet matches the desired state. It
fetches referenced Secrets, computes a hash for rolling-update annotations, reads
the restart-trigger annotation from the CR, and passes everything to
`constructStatefulSet`. Once the StatefulSet is reconciled, it calls
`deleteLegacyDeployment`. It returns the names of any missing Secrets for use
by status reconciliation.

```go
func (r *MemcachedReconciler) reconcileStatefulSet(ctx context.Context, mc *memcachedv1beta1.Memcached) ([]string, error) {
    found, missing := fetchReferencedSecrets(ctx, r.Client, mc)
    secretHash := computeSecretHash(found...)
    restartTrigger := mc.Annotations[AnnotationRestartTrigger]

    sts := &appsv1.StatefulSet{
        ObjectMeta: metav1.ObjectMeta{
            Name:      statefulSetName(mc.Name),
            Namespace: mc.Namespace,
        },
    }

    if _, err := r.reconcileResource(ctx, mc, sts, func() error {
        constructStatefulSet(mc, sts, secretHash, restartTrigger)
        return nil
    }, "StatefulSet"); err != nil {
        return missing, err
    }
    return missing, r.deleteLegacyDeployment(ctx, mc)
}
```

### CreateOrUpdate Behavior

`controllerutil.CreateOrUpdate` performs a server-side get-or-create:

| Scenario                         | Mutate Function Called | API Operation | `result` Value                          |
|----------------------------------|------------------------|---------------|-----------------------------------------|
| StatefulSet does not exist       | Yes                    | Create        | `controllerutil.OperationResultCreated` |
| StatefulSet exists, spec differs | Yes                    | Update        | `controllerutil.OperationResultUpdated` |
| StatefulSet exists, spec matches | Yes                    | (no-op)       | `controllerutil.OperationResultNone`    |

The mutate function runs **before** every create or update, ensuring the
StatefulSet always reflects the current CR spec. External drift (manual edits)
is corrected on the next reconciliation cycle. With autoscaling off, a
`spec.replicas` edited by hand on the StatefulSet is restored to the CR's count;
with autoscaling on, the stored count is kept (see [Replica Count](#replica-count)).

### Owner Reference

`controllerutil.SetControllerReference` adds an owner reference to the
StatefulSet's metadata:

| Field                | Value                       |
|----------------------|-----------------------------|
| `apiVersion`         | `memcached.c5c3.io/v1beta1` |
| `kind`               | `Memcached`                 |
| `name`               | `<cr-name>`                 |
| `uid`                | `<cr-uid>`                  |
| `controller`         | `true`                      |
| `blockOwnerDeletion` | `true`                      |

This enables:
- **Garbage collection**: Deleting the Memcached CR automatically deletes the
  owned StatefulSet via Kubernetes' owner reference cascade.
- **Watch filtering**: The `Owns(&appsv1.StatefulSet{})` watch on the controller
  maps StatefulSet events back to the owning Memcached CR for reconciliation.

### Error Handling

| Error Scenario                  | Behavior                                                  |
|---------------------------------|-----------------------------------------------------------|
| API server unreachable          | Error returned, controller-runtime requeues with backoff  |
| StatefulSet create/update fails | Error wrapped with context, returned for requeue          |
| Owner reference conflict        | Error from `SetControllerReference`, returned for requeue |
| Legacy Deployment fetch fails   | `fetching legacy Deployment: %w`, returned for requeue    |
| Legacy Deployment delete fails  | `deleting legacy Deployment: %w`, returned for requeue    |

StatefulSet errors are wrapped with `fmt.Errorf("reconciling StatefulSet: %w", err)`
to provide context in logs while preserving the original error for
`apierrors.IsXxx()` checks upstream. When the StatefulSet reconcile fails,
`deleteLegacyDeployment` is not called, so a legacy Deployment keeps serving
until the StatefulSet exists.

---

## Legacy Deployment Migration

Operator releases before the StatefulSet ran the memcached pods in a Deployment
named like the CR. `deleteLegacyDeployment(ctx, mc *Memcached) error` removes
that Deployment on the first reconcile after an upgrade, with no manual step:

```go
func (r *MemcachedReconciler) deleteLegacyDeployment(ctx context.Context, mc *memcachedv1beta1.Memcached) error {
    logger := log.FromContext(ctx)

    dep := &appsv1.Deployment{}
    if err := r.Get(ctx, types.NamespacedName{Name: mc.Name, Namespace: mc.Namespace}, dep); err != nil {
        if apierrors.IsNotFound(err) {
            return nil
        }
        return fmt.Errorf("fetching legacy Deployment: %w", err)
    }

    if !metav1.IsControlledBy(dep, mc) {
        logger.V(1).Info("Skipping Deployment not controlled by this Memcached", "name", dep.Name)
        return nil
    }

    if err := r.Delete(ctx, dep, client.PropagationPolicy(metav1.DeletePropagationBackground)); err != nil {
        if apierrors.IsNotFound(err) {
            return nil
        }
        return fmt.Errorf("deleting legacy Deployment: %w", err)
    }

    logger.Info("Deleted legacy Deployment after migrating to StatefulSet", "name", dep.Name)
    if r.Recorder != nil {
        r.Recorder.Eventf(mc, nil, corev1.EventTypeNormal, "Deleted", "Reconcile", "Deleted legacy Deployment %s", dep.Name)
    }
    return nil
}
```

| Situation                            | Result                                                            |
|--------------------------------------|-------------------------------------------------------------------|
| No Deployment named `<cr-name>`      | Returns `nil`                                                     |
| `Get` fails with another error       | Returns `fetching legacy Deployment: <err>`                       |
| Deployment not controlled by this CR | Left in place, logged at `V(1)`, returns `nil`                    |
| Deployment controlled by this CR     | Deleted with background propagation                               |
| `Delete` returns NotFound            | Returns `nil`                                                     |
| `Delete` fails with another error    | Returns `deleting legacy Deployment: <err>`; the request requeues |
| Delete succeeds                      | Info log and a `Normal` event, reason `Deleted`                   |

### Ownership Check

`metav1.IsControlledBy(dep, mc)` compares the Deployment's controller owner
reference with the CR's UID. A Deployment of the same name that has no owner
reference, or that another object controls, is never deleted.

### Background Propagation

`client.PropagationPolicy(metav1.DeletePropagationBackground)` makes the delete
call return at once while the garbage collector removes the ReplicaSet and its
pods. Foreground propagation would block the delete on a finalizer, and in
envtest, which runs no garbage collector, it would never finish.

### Timing

The Deployment is deleted in the same reconcile that creates or updates the
StatefulSet, not once the StatefulSet is Ready. Waiting would stall the move
whenever the cluster cannot fit a second copy, for example with the `hard`
anti-affinity preset and no spare node, or a tight ResourceQuota. The cost is
that for a few seconds the Service name has no ready pod, until the new pods
pass their readiness probe. The cached entries are lost during the move.

When the delete fails, `Reconcile` returns the error and the request requeues
with backoff. The StatefulSet already exists, so a retry repeats only the
delete.

### Event

A successful delete emits one event on the Memcached CR:

| Type     | Reason    | Note                                  |
|----------|-----------|---------------------------------------|
| `Normal` | `Deleted` | `Deleted legacy Deployment <cr-name>` |

### RBAC

The `Get` goes through the manager's cached client, which starts a Deployment
informer on first use and so needs `list` and `watch` as well as `get`. The
operator keeps `get`, `list`, `watch`, and `delete` on `apps/deployments`:

```go
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;delete
```

---

## Reconcile Integration

The `Reconcile` method calls `reconcileStatefulSet` first after fetching the
Memcached CR:

```go
func (r *MemcachedReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
    memcached := &memcachedv1beta1.Memcached{}
    if err := r.Get(ctx, req.NamespacedName, memcached); err != nil {
        if apierrors.IsNotFound(err) {
            return ctrl.Result{}, nil
        }
        return ctrl.Result{}, err
    }

    // ... reconcile metrics ...

    var missingSecrets []string
    missingSecrets, reconcileErr = r.reconcileStatefulSet(ctx, memcached)
    if reconcileErr != nil {
        return ctrl.Result{}, reconcileErr
    }

    // ... reconcileHPA, reconcileService, reconcilePDB, reconcileServiceMonitor, reconcileNetworkPolicy ...

    if reconcileErr = r.reconcileStatus(ctx, memcached, missingSecrets); reconcileErr != nil {
        return ctrl.Result{}, reconcileErr
    }

    return ctrl.Result{}, nil
}
```

| Scenario                                | Return Value         | Effect                                                    |
|-----------------------------------------|----------------------|-----------------------------------------------------------|
| CR not found (deleted)                  | `ctrl.Result{}, nil` | No requeue; owner ref cascade handles StatefulSet cleanup |
| CR fetch fails                          | `ctrl.Result{}, err` | Requeue with exponential backoff                          |
| StatefulSet reconcile succeeds          | `ctrl.Result{}, nil` | No requeue; missing Secrets forwarded to status           |
| StatefulSet reconcile fails             | `ctrl.Result{}, err` | Requeue with exponential backoff                          |
| Legacy Deployment fetch or delete fails | `ctrl.Result{}, err` | Requeue with exponential backoff                          |

---

## Reconciliation Flow

```text
  Memcached CR created/updated
            │
            ▼
  ┌──────────────────────────────────────┐
  │  Reconcile                           │
  │  1. Fetch Memcached CR               │
  │  2. If NotFound → return             │
  │  3. If error → requeue               │
  └──────────────────┬───────────────────┘
                     │
                     ▼
  ┌──────────────────────────────────────┐
  │  reconcileStatefulSet                │
  │  → returns missingSecrets            │
  │                                      │
  │  1. fetchReferencedSecrets           │
  │  2. computeSecretHash                │
  │  3. Read restart-trigger             │
  │                                      │
  │  CreateOrUpdate:                     │
  │    ┌───────────────────────────────┐ │
  │    │ Mutate function               │ │
  │    │  constructStatefulSet         │ │
  │    │  (secretHash, restartTrigger) │ │
  │    │  SetControllerRef             │ │
  │    └───────────────────────────────┘ │
  │                                      │
  │  StatefulSet                         │
  │  ├─ Name: statefulSetName(cr-name)   │
  │  ├─ Namespace: <cr-ns>               │
  │  ├─ Replicas: statefulSetReplicas    │
  │  ├─ ServiceName: <cr-name>           │
  │  ├─ PodManagementPolicy: Parallel    │
  │  ├─ UpdateStrategy: RollingUpdate    │
  │  ├─ Image: spec/default              │
  │  ├─ Args: buildMemcachedArgs         │
  │  ├─ Port: 11211/TCP                  │
  │  ├─ Probes: TCP socket               │
  │  ├─ Volumes: SASL (if on)            │
  │  ├─ VolumeMounts: SASL               │
  │  ├─ PodAnnotations:                  │
  │  │  ├─ secret-hash                   │
  │  │  └─ restart-trigger               │
  │  └─ OwnerRef → Memcached CR          │
  └──────────────────┬───────────────────┘
                     │ success
                     ▼
  ┌──────────────────────────────────────┐
  │  deleteLegacyDeployment              │
  │  1. Get Deployment <cr-name>         │
  │  2. NotFound → done                  │
  │  3. Not controlled by CR → skip      │
  │  4. Delete (background propagation)  │
  │  5. Log + Normal "Deleted" event     │
  └──────────────────────────────────────┘
```

---

## StatefulSet Manifest Example

A Memcached CR with custom settings:

```yaml
apiVersion: memcached.c5c3.io/v1beta1
kind: Memcached
metadata:
  name: my-cache
  namespace: default
spec:
  replicas: 3
  image: "memcached:1.6.29"
  resources:
    requests:
      cpu: "100m"
      memory: "128Mi"
    limits:
      cpu: "500m"
      memory: "256Mi"
  memcached:
    maxMemoryMB: 128
    maxConnections: 2048
    threads: 8
    maxItemSize: "2m"
    verbosity: 1
    extraArgs: ["-o", "modern"]
```

Produces a StatefulSet with:

```yaml
apiVersion: apps/v1
kind: StatefulSet
metadata:
  name: my-cache
  namespace: default
  labels:
    app.kubernetes.io/name: memcached
    app.kubernetes.io/instance: my-cache
    app.kubernetes.io/managed-by: memcached-operator
    app.kubernetes.io/version: "1.6.29"
  ownerReferences:
    - apiVersion: memcached.c5c3.io/v1beta1
      kind: Memcached
      name: my-cache
      controller: true
      blockOwnerDeletion: true
spec:
  replicas: 3
  serviceName: my-cache
  podManagementPolicy: Parallel
  updateStrategy:
    type: RollingUpdate
  selector:
    matchLabels:
      app.kubernetes.io/name: memcached
      app.kubernetes.io/instance: my-cache
      app.kubernetes.io/managed-by: memcached-operator
  template:
    metadata:
      labels:
        app.kubernetes.io/name: memcached
        app.kubernetes.io/instance: my-cache
        app.kubernetes.io/managed-by: memcached-operator
        app.kubernetes.io/version: "1.6.29"
    spec:
      containers:
        - name: memcached
          image: "memcached:1.6.29"
          args:
            - "-m"
            - "128"
            - "-c"
            - "2048"
            - "-t"
            - "8"
            - "-I"
            - "2m"
            - "-v"
            - "-o"
            - "modern"
          resources:
            requests:
              cpu: "100m"
              memory: "128Mi"
            limits:
              cpu: "500m"
              memory: "256Mi"
          ports:
            - name: memcached
              containerPort: 11211
              protocol: TCP
          livenessProbe:
            tcpSocket:
              port: memcached
            initialDelaySeconds: 10
            periodSeconds: 10
          readinessProbe:
            tcpSocket:
              port: memcached
            initialDelaySeconds: 5
            periodSeconds: 5
```

The pods are named `my-cache-0`, `my-cache-1`, and `my-cache-2`, and resolve as
`my-cache-0.my-cache.default.svc` and so on through the headless Service.

### SASL-Enabled CR Example

```yaml
apiVersion: memcached.c5c3.io/v1beta1
kind: Memcached
metadata:
  name: my-cache
  namespace: default
spec:
  replicas: 3
  image: "memcached:1.6.29"
  security:
    sasl:
      enabled: true
      credentialsSecretRef:
        name: memcached-sasl-credentials
```

Produces a StatefulSet with SASL volume and mount on the memcached container:

```yaml
spec:
  template:
    spec:
      volumes:
        - name: sasl-credentials
          secret:
            secretName: memcached-sasl-credentials
      containers:
        - name: memcached
          image: "memcached:1.6.29"
          args:
            - "-m"
            - "64"
            - "-c"
            - "1024"
            - "-t"
            - "4"
            - "-I"
            - "1m"
            - "-Y"
            - "/etc/memcached/sasl/password-file"
          volumeMounts:
            - name: sasl-credentials
              mountPath: /etc/memcached/sasl
              readOnly: true
```

### Minimal CR Example

```yaml
apiVersion: memcached.c5c3.io/v1beta1
kind: Memcached
metadata:
  name: my-cache
spec: {}
```

Produces a StatefulSet named `my-cache` with 1 replica, image `memcached:1.6`,
args `["-m", "64", "-c", "1024", "-t", "4", "-I", "1m"]`, no resource limits,
`serviceName: my-cache`, `podManagementPolicy: Parallel`,
`updateStrategy.type: RollingUpdate`, and the same labels, probes, and owner
reference.
