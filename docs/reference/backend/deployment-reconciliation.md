# Deployment Reconciliation

Reference documentation for the Deployment reconciliation logic that creates and
updates an `apps/v1` Deployment for each Memcached CR.

**Source**: `internal/controller/deployment.go`, `internal/controller/memcached_controller.go`

## Overview

When a Memcached CR is created or updated, the reconciler ensures a matching
Deployment exists in the same namespace with the same name. The Deployment is
constructed from the CR spec using pure builder functions, then applied via
`controllerutil.CreateOrUpdate` for idempotent create/update semantics. A
controller owner reference on the Deployment enables automatic garbage
collection when the Memcached CR is deleted.

---

## Labels

`labelsForMemcached(name string)` returns the standard Kubernetes recommended
labels applied to both the Deployment and its pod template:

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

These labels are used as the Deployment's `spec.selector.matchLabels` and on the
pod template `metadata.labels`, ensuring the Deployment manages the correct pods.

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

## Deployment Construction

`constructDeployment(mc *Memcached, dep *Deployment)` sets the desired state of
the Deployment in-place. It is called within the `controllerutil.CreateOrUpdate`
mutate function so that both creation and updates use identical logic.

### Spec Defaults

| Field       | Source           | Default           |
|-------------|------------------|-------------------|
| `replicas`  | `spec.Replicas`  | `1`               |
| `image`     | `spec.Image`     | `"memcached:1.6"` |
| `args`      | `spec.Memcached` | See default args  |
| `resources` | `spec.Resources` | (empty)           |

### Container Specification

The Deployment contains a single container:

| Property       | Value                                                                                  |
|----------------|----------------------------------------------------------------------------------------|
| `name`         | `memcached`                                                                            |
| `image`        | From `spec.Image` (default `memcached:1.6`)                                            |
| `args`         | Built by `buildMemcachedArgs`                                                          |
| `resources`    | From `spec.Resources` (empty if nil)                                                   |
| `ports`        | `memcached`: 11211/TCP                                                                 |
| `volumeMounts` | SASL credentials mount (when enabled, see [SASL Authentication](#sasl-authentication)) |

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

### Deployment Strategy

```go
appsv1.DeploymentStrategy{
    Type: appsv1.RollingUpdateDeploymentStrategyType,
    RollingUpdate: &appsv1.RollingUpdateDeployment{
        MaxSurge:       intstr.FromInt32(1),
        MaxUnavailable: intstr.FromInt32(0),
    },
}
```

| Parameter        | Value | Effect                                               |
|------------------|-------|------------------------------------------------------|
| `maxSurge`       | `1`   | One extra pod is created before terminating old pods |
| `maxUnavailable` | `0`   | No existing pods are terminated until new pods ready |

This ensures zero-downtime rolling updates for cache availability.

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

### Integration with constructDeployment

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

`reconcileDeployment(ctx, mc *Memcached)` on `MemcachedReconciler` ensures the
Deployment matches the desired state:

```go
func (r *MemcachedReconciler) reconcileDeployment(ctx context.Context, mc *memcachedv1alpha1.Memcached) error {
    dep := &appsv1.Deployment{
        ObjectMeta: metav1.ObjectMeta{
            Name:      mc.Name,
            Namespace: mc.Namespace,
        },
    }

    result, err := controllerutil.CreateOrUpdate(ctx, r.Client, dep, func() error {
        constructDeployment(mc, dep)
        return controllerutil.SetControllerReference(mc, dep, r.Scheme)
    })
    if err != nil {
        return fmt.Errorf("reconciling Deployment: %w", err)
    }

    logger.Info("Deployment reconciled", "name", dep.Name, "operation", result)
    return nil
}
```

### CreateOrUpdate Behavior

`controllerutil.CreateOrUpdate` performs a server-side get-or-create:

| Scenario                        | Mutate Function Called | API Operation | `result` Value                          |
|---------------------------------|------------------------|---------------|-----------------------------------------|
| Deployment does not exist       | Yes                    | Create        | `controllerutil.OperationResultCreated` |
| Deployment exists, spec differs | Yes                    | Update        | `controllerutil.OperationResultUpdated` |
| Deployment exists, spec matches | Yes                    | (no-op)       | `controllerutil.OperationResultNone`    |

The mutate function runs **before** every create or update, ensuring the
Deployment always reflects the current CR spec. External drift (manual edits)
is corrected on the next reconciliation cycle.

### Owner Reference

`controllerutil.SetControllerReference` adds an owner reference to the
Deployment's metadata:

| Field                | Value                        |
|----------------------|------------------------------|
| `apiVersion`         | `memcached.c5c3.io/v1alpha1` |
| `kind`               | `Memcached`                  |
| `name`               | `<cr-name>`                  |
| `uid`                | `<cr-uid>`                   |
| `controller`         | `true`                       |
| `blockOwnerDeletion` | `true`                       |

This enables:
- **Garbage collection**: Deleting the Memcached CR automatically deletes the
  owned Deployment via Kubernetes' owner reference cascade.
- **Watch filtering**: The `Owns(&appsv1.Deployment{})` watch on the controller
  maps Deployment events back to the owning Memcached CR for reconciliation.

### Error Handling

| Error Scenario                 | Behavior                                                  |
|--------------------------------|-----------------------------------------------------------|
| API server unreachable         | Error returned, controller-runtime requeues with backoff  |
| Deployment create/update fails | Error wrapped with context, returned for requeue          |
| Owner reference conflict       | Error from `SetControllerReference`, returned for requeue |

Errors are wrapped with `fmt.Errorf("reconciling Deployment: %w", err)` to
provide context in logs while preserving the original error for
`apierrors.IsXxx()` checks upstream.

---

## Reconcile Integration

The `Reconcile` method calls `reconcileDeployment` after fetching the
Memcached CR:

```go
func (r *MemcachedReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
    memcached := &memcachedv1alpha1.Memcached{}
    if err := r.Get(ctx, req.NamespacedName, memcached); err != nil {
        if apierrors.IsNotFound(err) {
            return ctrl.Result{}, nil
        }
        return ctrl.Result{}, err
    }

    if err := r.reconcileDeployment(ctx, memcached); err != nil {
        return ctrl.Result{}, err
    }

    return ctrl.Result{}, nil
}
```

| Scenario                      | Return Value         | Effect                                                   |
|-------------------------------|----------------------|----------------------------------------------------------|
| CR not found (deleted)        | `ctrl.Result{}, nil` | No requeue; owner ref cascade handles Deployment cleanup |
| CR fetch fails                | `ctrl.Result{}, err` | Requeue with exponential backoff                         |
| Deployment reconcile succeeds | `ctrl.Result{}, nil` | No requeue                                               |
| Deployment reconcile fails    | `ctrl.Result{}, err` | Requeue with exponential backoff                         |

---

## Reconciliation Flow

```text
  Memcached CR created/updated
            │
            ▼
  ┌─────────────────────────────┐
  │  Reconcile                  │
  │  1. Fetch Memcached CR      │
  │  2. If NotFound → return    │
  │  3. If error → requeue      │
  └────────────┬────────────────┘
               │
               ▼
  ┌─────────────────────────────┐
  │  reconcileDeployment        │
  │                             │
  │  CreateOrUpdate:            │
  │    ┌──────────────────────┐ │
  │    │ Mutate function      │ │
  │    │  constructDeployment │ │
  │    │  SetControllerRef    │ │
  │    └──────────────────────┘ │
  │                             │
  │  Deployment                 │
  │  ├─ Name: <cr-name>        │
  │  ├─ Namespace: <cr-ns>     │
  │  ├─ Replicas: spec/default │
  │  ├─ Image: spec/default    │
  │  ├─ Args: buildMemcachedArgs│
  │  ├─ Port: 11211/TCP        │
  │  ├─ Probes: TCP socket     │
  │  ├─ Strategy: RollingUpdate│
  │  ├─ Volumes: SASL (if on)  │
  │  ├─ VolumeMounts: SASL     │
  │  └─ OwnerRef → Memcached CR│
  └─────────────────────────────┘
```

---

## Deployment Manifest Example

A Memcached CR with custom settings:

```yaml
apiVersion: memcached.c5c3.io/v1alpha1
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

Produces a Deployment with:

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: my-cache
  namespace: default
  labels:
    app.kubernetes.io/name: memcached
    app.kubernetes.io/instance: my-cache
    app.kubernetes.io/managed-by: memcached-operator
  ownerReferences:
    - apiVersion: memcached.c5c3.io/v1alpha1
      kind: Memcached
      name: my-cache
      controller: true
      blockOwnerDeletion: true
spec:
  replicas: 3
  selector:
    matchLabels:
      app.kubernetes.io/name: memcached
      app.kubernetes.io/instance: my-cache
      app.kubernetes.io/managed-by: memcached-operator
  strategy:
    type: RollingUpdate
    rollingUpdate:
      maxSurge: 1
      maxUnavailable: 0
  template:
    metadata:
      labels:
        app.kubernetes.io/name: memcached
        app.kubernetes.io/instance: my-cache
        app.kubernetes.io/managed-by: memcached-operator
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

### SASL-Enabled CR Example

```yaml
apiVersion: memcached.c5c3.io/v1alpha1
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

Produces a Deployment with SASL volume and mount on the memcached container:

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
apiVersion: memcached.c5c3.io/v1alpha1
kind: Memcached
metadata:
  name: my-cache
spec: {}
```

Produces a Deployment with 1 replica, image `memcached:1.6`, args
`["-m", "64", "-c", "1024", "-t", "4", "-I", "1m"]`, no resource limits,
and the same labels, probes, strategy, and owner reference.
