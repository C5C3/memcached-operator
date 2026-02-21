# Validation Webhook

Reference documentation for the validating admission webhook that rejects
Memcached custom resources with invalid field combinations.

**Source**: `api/v1alpha1/memcached_validation_webhook.go`

## Overview

The Memcached operator includes a validating admission webhook that intercepts
`CREATE` and `UPDATE` requests for Memcached resources and rejects those with
invalid field values. The webhook collects all validation errors into a single
response so that operators can fix every issue in one pass rather than
discovering errors one at a time.

The webhook uses the controller-runtime `Validator` interface
(`admission.Validator[*Memcached]`). It is registered alongside the defaulting
webhook in `SetupMemcachedWebhookWithManager` and runs **after** the mutating
webhook in the admission chain, so validation sees the fully-defaulted CR.

### Webhook Path

```text
/validate-memcached-c5c3-io-v1alpha1-memcached
```

Admission type: **Validating** (`mutating=false`)
Failure policy: **Fail** — rejects the request if the webhook is unavailable.
Side effects: **None**
Operations: `create`, `update`

---

## Validation Rules

The webhook enforces six categories of validation rules. All rules are
evaluated on every request, and errors are aggregated into a single
`StatusError` response.

### Memory Limit Sufficiency (REQ-001)

Ensures the container memory limit can accommodate the configured Memcached
cache size plus 32Mi of operational overhead for connections, threads, and
internal data structures.

| Field                          | Constraint                                                |
|--------------------------------|-----------------------------------------------------------|
| `spec.resources.limits.memory` | Must be >= `spec.memcached.maxMemoryMB` (in bytes) + 32Mi |

**Skip condition**: Validation is skipped when `spec.resources` is nil or
`spec.resources.limits.memory` is not set.

**Error example**:
```text
spec.resources.limits.memory: Invalid value: "64Mi":
  memory limit must be at least 96Mi (maxMemoryMB=64Mi + 32Mi overhead)
```

### PDB Constraints (REQ-002, REQ-003)

Validates PodDisruptionBudget configuration to prevent impossible disruption
constraints.

| Field                                                    | Constraint                                                                         |
|----------------------------------------------------------|------------------------------------------------------------------------------------|
| `spec.highAvailability.podDisruptionBudget`              | `minAvailable` and `maxUnavailable` are mutually exclusive                         |
| `spec.highAvailability.podDisruptionBudget`              | At least one of `minAvailable` or `maxUnavailable` must be set when PDB is enabled |
| `spec.highAvailability.podDisruptionBudget.minAvailable` | Integer value must be strictly less than `spec.replicas`                           |

**Skip condition**: Validation is skipped when `spec.highAvailability` is nil,
`spec.highAvailability.podDisruptionBudget` is nil, or PDB is not enabled.
Percentage values for `minAvailable` are not validated against replicas because
they cannot be compared statically.

**Error examples**:
```text
spec.highAvailability.podDisruptionBudget: Invalid value: "":
  minAvailable and maxUnavailable are mutually exclusive, specify only one

spec.highAvailability.podDisruptionBudget: Required value:
  one of minAvailable or maxUnavailable must be set when PDB is enabled

spec.highAvailability.podDisruptionBudget.minAvailable: Invalid value: 3:
  minAvailable (3) must be less than replicas (3)
```

### Security Secret References (REQ-004, REQ-005)

Validates that secret references are provided when security features are
enabled, preventing runtime failures from missing secrets.

| Field                                          | Constraint                                           |
|------------------------------------------------|------------------------------------------------------|
| `spec.security.sasl.credentialsSecretRef.name` | Required when `spec.security.sasl.enabled` is `true` |
| `spec.security.tls.certificateSecretRef.name`  | Required when `spec.security.tls.enabled` is `true`  |

**Skip condition**: Validation is skipped when `spec.security` is nil, or when
the respective SASL/TLS section is nil or not enabled.

**Error examples**:
```text
spec.security.sasl.credentialsSecretRef.name: Required value:
  credentialsSecretRef.name is required when SASL is enabled

spec.security.tls.certificateSecretRef.name: Required value:
  certificateSecretRef.name is required when TLS is enabled
```

### Graceful Shutdown Timing (REQ-006)

Validates that the termination grace period exceeds the pre-stop delay to
ensure the pre-stop hook completes before the pod receives SIGKILL.

| Field                                                                  | Constraint                                                                                 |
|------------------------------------------------------------------------|--------------------------------------------------------------------------------------------|
| `spec.highAvailability.gracefulShutdown.terminationGracePeriodSeconds` | Must be strictly greater than `spec.highAvailability.gracefulShutdown.preStopDelaySeconds` |

**Skip condition**: Validation is skipped when `spec.highAvailability` is nil,
`spec.highAvailability.gracefulShutdown` is nil, or graceful shutdown is not
enabled.

**Error example**:
```text
spec.highAvailability.gracefulShutdown.terminationGracePeriodSeconds: Invalid value: 10:
  terminationGracePeriodSeconds (10) must exceed preStopDelaySeconds (10)
```

### Autoscaling Constraints (REQ-005, REQ-006, REQ-007)

Validates autoscaling configuration to prevent conflicting settings and
ensure the HPA can function correctly.

| Field                          | Constraint                                                                     |
|--------------------------------|--------------------------------------------------------------------------------|
| `spec.replicas`                | Must not be set when `spec.autoscaling.enabled` is `true` (mutually exclusive) |
| `spec.autoscaling.minReplicas` | Must not exceed `spec.autoscaling.maxReplicas`                                 |
| `spec.resources.requests.cpu`  | Required when autoscaling uses CPU utilization metrics                         |

**Skip condition**: Validation is skipped when `spec.autoscaling` is nil or
`spec.autoscaling.enabled` is `false`.

**Error examples**:
```text
spec.replicas: Invalid value: 3:
  spec.replicas and spec.autoscaling.enabled are mutually exclusive

spec.autoscaling.minReplicas: Invalid value: 10:
  minReplicas (10) must not exceed maxReplicas (5)

spec.resources.requests.cpu: Required value:
  resources.requests.cpu is required when using CPU utilization metrics
```

### Delete Operations (REQ-010)

`DELETE` operations are always allowed. `ValidateDelete` returns nil without
performing any checks.

---

## CR Examples

### Rejected: Insufficient Memory Limit

```yaml
apiVersion: memcached.c5c3.io/v1alpha1
kind: Memcached
metadata:
  name: my-cache
spec:
  memcached:
    maxMemoryMB: 64
  resources:
    limits:
      memory: "64Mi"   # Too low: needs at least 96Mi (64Mi + 32Mi overhead)
```

**Error**: `spec.resources.limits.memory: Invalid value: "64Mi": memory limit must be at least 96Mi (maxMemoryMB=64Mi + 32Mi overhead)`

### Rejected: PDB minAvailable >= Replicas

```yaml
apiVersion: memcached.c5c3.io/v1alpha1
kind: Memcached
metadata:
  name: my-cache
spec:
  replicas: 3
  highAvailability:
    podDisruptionBudget:
      enabled: true
      minAvailable: 3   # Must be < replicas (3)
```

**Error**: `spec.highAvailability.podDisruptionBudget.minAvailable: Invalid value: 3: minAvailable (3) must be less than replicas (3)`

### Rejected: SASL Enabled Without Secret

```yaml
apiVersion: memcached.c5c3.io/v1alpha1
kind: Memcached
metadata:
  name: my-cache
spec:
  security:
    sasl:
      enabled: true
      # Missing credentialsSecretRef.name
```

**Error**: `spec.security.sasl.credentialsSecretRef.name: Required value: credentialsSecretRef.name is required when SASL is enabled`

### Rejected: TLS Enabled Without Secret

```yaml
apiVersion: memcached.c5c3.io/v1alpha1
kind: Memcached
metadata:
  name: my-cache
spec:
  security:
    tls:
      enabled: true
      # Missing certificateSecretRef.name
```

**Error**: `spec.security.tls.certificateSecretRef.name: Required value: certificateSecretRef.name is required when TLS is enabled`

### Rejected: Graceful Shutdown Timing

```yaml
apiVersion: memcached.c5c3.io/v1alpha1
kind: Memcached
metadata:
  name: my-cache
spec:
  highAvailability:
    gracefulShutdown:
      enabled: true
      preStopDelaySeconds: 10
      terminationGracePeriodSeconds: 10   # Must be > preStopDelaySeconds
```

**Error**: `spec.highAvailability.gracefulShutdown.terminationGracePeriodSeconds: Invalid value: 10: terminationGracePeriodSeconds (10) must exceed preStopDelaySeconds (10)`

### Rejected: Autoscaling With Replicas

```yaml
apiVersion: memcached.c5c3.io/v1alpha1
kind: Memcached
metadata:
  name: my-cache
spec:
  replicas: 3
  autoscaling:
    enabled: true
    minReplicas: 2
    maxReplicas: 10
```

**Error**: `spec.replicas: Invalid value: 3: spec.replicas and spec.autoscaling.enabled are mutually exclusive`

### Rejected: minReplicas Exceeds maxReplicas

```yaml
apiVersion: memcached.c5c3.io/v1alpha1
kind: Memcached
metadata:
  name: my-cache
spec:
  autoscaling:
    enabled: true
    minReplicas: 10
    maxReplicas: 5
```

**Error**: `spec.autoscaling.minReplicas: Invalid value: 10: minReplicas (10) must not exceed maxReplicas (5)`

### Rejected: CPU Utilization Without CPU Request

```yaml
apiVersion: memcached.c5c3.io/v1alpha1
kind: Memcached
metadata:
  name: my-cache
spec:
  autoscaling:
    enabled: true
    minReplicas: 2
    maxReplicas: 10
    # No resources.requests.cpu — defaulted metrics target CPU utilization
```

**Error**: `spec.resources.requests.cpu: Required value: resources.requests.cpu is required when using CPU utilization metrics`

### Rejected: Multiple Violations

A CR with multiple invalid fields returns all errors at once:

```yaml
apiVersion: memcached.c5c3.io/v1alpha1
kind: Memcached
metadata:
  name: my-cache
spec:
  replicas: 3
  memcached:
    maxMemoryMB: 64
  resources:
    limits:
      memory: "64Mi"
  highAvailability:
    podDisruptionBudget:
      enabled: true
      minAvailable: 3
  security:
    sasl:
      enabled: true
```

**Errors** (all returned in a single response):
- `spec.resources.limits.memory: Invalid value: "64Mi": ...`
- `spec.highAvailability.podDisruptionBudget.minAvailable: Invalid value: 3: ...`
- `spec.security.sasl.credentialsSecretRef.name: Required value: ...`

### Accepted: Valid CR With All Features

```yaml
apiVersion: memcached.c5c3.io/v1alpha1
kind: Memcached
metadata:
  name: my-cache
spec:
  replicas: 3
  image: "memcached:1.6.28"
  memcached:
    maxMemoryMB: 256
  resources:
    limits:
      memory: "320Mi"           # 256Mi + 32Mi overhead = 288Mi, 320Mi is sufficient
  highAvailability:
    podDisruptionBudget:
      enabled: true
      minAvailable: 2           # Less than replicas (3)
    gracefulShutdown:
      enabled: true
      preStopDelaySeconds: 10
      terminationGracePeriodSeconds: 30   # Greater than preStopDelaySeconds
  security:
    sasl:
      enabled: true
      credentialsSecretRef:
        name: "sasl-secret"     # Provided
    tls:
      enabled: true
      certificateSecretRef:
        name: "tls-secret"      # Provided
```

### Accepted: Minimal CR (After Defaulting)

A minimal CR with an empty spec passes validation because the defaulting
webhook fills required fields before validation runs:

```yaml
apiVersion: memcached.c5c3.io/v1alpha1
kind: Memcached
metadata:
  name: my-cache
spec: {}
```

After defaulting: `replicas=1`, `maxMemoryMB=64`, no resources/PDB/security
sections — all nil-guarded validations are skipped.

---

## Runtime Behavior

| Action                        | Result                                                            |
|-------------------------------|-------------------------------------------------------------------|
| Create CR with invalid fields | Rejected with 403/422 and all field errors listed                 |
| Create valid CR               | Accepted without modification                                     |
| Update CR to invalid config   | Rejected with field errors                                        |
| Update CR to valid config     | Accepted                                                          |
| Delete CR                     | Always accepted (no validation on delete)                         |
| Webhook unavailable           | Request rejected (failurePolicy=Fail)                             |
| Minimal CR (empty spec)       | Accepted — defaulting webhook fills fields before validation runs |

---

## Implementation

The `MemcachedCustomValidator` struct in
`api/v1alpha1/memcached_validation_webhook.go` implements
`admission.Validator[*Memcached]`:

```go
type MemcachedCustomValidator struct{}

func (v *MemcachedCustomValidator) ValidateCreate(ctx context.Context, obj *Memcached) (admission.Warnings, error)
func (v *MemcachedCustomValidator) ValidateUpdate(ctx context.Context, oldObj *Memcached, newObj *Memcached) (admission.Warnings, error)
func (v *MemcachedCustomValidator) ValidateDelete(ctx context.Context, obj *Memcached) (admission.Warnings, error)
```

Both `ValidateCreate` and `ValidateUpdate` delegate to `validateMemcached`,
which aggregates errors from four internal functions:

```go
func validateMemcached(mc *Memcached) error {
    var allErrs field.ErrorList
    allErrs = append(allErrs, validateMemoryLimit(mc)...)
    allErrs = append(allErrs, validatePDB(mc)...)
    allErrs = append(allErrs, validateGracefulShutdown(mc)...)
    allErrs = append(allErrs, validateSecuritySecretRefs(mc)...)
    allErrs = append(allErrs, validateAutoscaling(mc)...)
    // ...
}
```

Errors are wrapped with `apierrors.NewInvalid()` to produce a properly
formatted Kubernetes `StatusError` with structured field paths.

### Registration

The validator is registered on the same webhook builder as the defaulter:

```go
func SetupMemcachedWebhookWithManager(mgr ctrl.Manager) error {
    return ctrl.NewWebhookManagedBy(mgr, &Memcached{}).
        WithDefaulter(&MemcachedCustomDefaulter{}).
        WithValidator(&MemcachedCustomValidator{}).
        Complete()
}
```

This ensures the mutating webhook (defaulting) runs before the validating
webhook in the Kubernetes admission chain.

### Webhook Configuration

The kubebuilder marker on `MemcachedCustomValidator` generates the webhook
manifest in `config/webhook/manifests.yaml`:

- Path: `/validate-memcached-c5c3-io-v1alpha1-memcached`
- Mutating: `false`
- Failure policy: `Fail`
- Side effects: `None`
- Verbs: `create`, `update`
- API group: `memcached.c5c3.io`
- API version: `v1alpha1`
