# Webhook Tests

Reference documentation for the webhook test suite covering both defaulting and
validation webhooks at three levels: unit tests, envtest integration tests, and
Chainsaw E2E tests.

**Source**:
- `api/v1alpha1/memcached_webhook_test.go` (defaulting unit tests)
- `api/v1alpha1/memcached_validation_webhook_test.go` (validation unit tests)
- `internal/controller/memcached_webhook_integration_test.go` (defaulting envtest)
- `internal/controller/memcached_validation_webhook_integration_test.go` (validation envtest)
- `test/e2e/webhook-defaulting/` (Chainsaw E2E defaulting)
- `test/e2e/webhook-rejection/` (Chainsaw E2E validation)

## Overview

The webhook test suite validates the Memcached operator's admission webhooks at
three levels of the testing pyramid:

1. **Unit tests** — Direct function calls to `MemcachedCustomDefaulter.Default()`
   and `MemcachedCustomValidator.ValidateCreate/Update/Delete()` without any
   Kubernetes infrastructure. Fast, isolated, comprehensive edge-case coverage.

2. **Envtest integration tests** — CR creation and updates against a real API
   server (controller-runtime envtest) with both webhooks active. Validates
   that defaults survive the API server round-trip and that validation rejections
   come through the admission chain.

3. **Chainsaw E2E tests** — YAML-driven tests on a real kind cluster with
   cert-manager providing webhook TLS. Validates the full admission flow
   including network transport, certificate rotation, and kubectl error output.

---

## Unit Tests: Defaulting Webhook

**File**: `api/v1alpha1/memcached_webhook_test.go`

Standard Go `testing.T` tests calling `MemcachedCustomDefaulter.Default()`
directly on in-memory `Memcached` structs.

### Test Inventory

| Test Function                                                   | REQ                       | What It Verifies                                                                                              |
|-----------------------------------------------------------------|---------------------------|---------------------------------------------------------------------------------------------------------------|
| `TestMemcachedDefaulting_EmptySpec`                             | REQ-001, REQ-002, REQ-003 | Empty spec gets replicas=1, image=memcached:1.6, full memcached config defaults; monitoring and HA remain nil |
| `TestMemcachedDefaulting_PreservesExplicitValues`               | REQ-001, REQ-002, REQ-003 | All explicitly set values (replicas=3, custom image, custom memcached config) are never overwritten           |
| `TestMemcachedDefaulting_NilMemcachedConfig`                    | REQ-003                   | Nil `spec.memcached` is initialized with maxMemoryMB=64, maxConnections=1024, threads=4, maxItemSize=1m       |
| `TestMemcachedDefaulting_PartialMemcachedConfig`                | REQ-003                   | maxMemoryMB=256 preserved; maxConnections, threads, maxItemSize defaulted                                     |
| `TestMemcachedDefaulting_MonitoringExporterImage`               | REQ-004                   | Nil exporterImage defaults to `prom/memcached-exporter:v0.15.4` when monitoring section exists                |
| `TestMemcachedDefaulting_MonitoringExporterImagePreserved`      | REQ-004                   | Custom exporterImage is not overwritten                                                                       |
| `TestMemcachedDefaulting_NilMonitoringStaysNil`                 | REQ-004                   | Nil monitoring section remains nil (opt-in)                                                                   |
| `TestMemcachedDefaulting_ServiceMonitorDefaults`                | REQ-004                   | Empty serviceMonitor gets interval=30s and scrapeTimeout=10s                                                  |
| `TestMemcachedDefaulting_ServiceMonitorPartialPreserved`        | REQ-004                   | Custom interval=15s preserved; scrapeTimeout=10s defaulted                                                    |
| `TestMemcachedDefaulting_NilServiceMonitorStaysNil`             | REQ-004                   | Nil serviceMonitor within monitoring section remains nil                                                      |
| `TestMemcachedDefaulting_AntiAffinityPreset`                    | REQ-005                   | Empty HA section gets antiAffinityPreset=soft                                                                 |
| `TestMemcachedDefaulting_AntiAffinityPresetHardPreserved`       | REQ-005                   | Explicit antiAffinityPreset=hard is not overwritten                                                           |
| `TestMemcachedDefaulting_NilHAStaysNil`                         | REQ-005                   | Nil HA section remains nil (opt-in)                                                                           |
| `TestMemcachedDefaulting_ReplicasZeroPreserved`                 | REQ-001                   | replicas=0 pointer is preserved, not overwritten to default                                                   |
| `TestMemcachedDefaulting_FullySpecifiedCRUnchanged`             | REQ-001–REQ-005           | Fully specified CR with all sections passes through unchanged                                                 |
| `TestMemcachedDefaulting_Idempotent`                            | REQ-001–REQ-003           | Applying defaults twice produces identical results                                                            |
| `TestMemcachedDefaulting_EmptyStringImagePreserved`             | REQ-002                   | Non-nil empty-string image pointer preserved (webhook only defaults nil)                                      |
| `TestMemcachedDefaulting_VerbosityZeroExplicit`                 | REQ-003                   | Verbosity=0 (Go zero value) preserved alongside defaulted fields                                              |
| `TestMemcachedDefaulting_ExtraArgsPreserved`                    | REQ-003                   | ExtraArgs slice preserved through defaulting                                                                  |
| `TestMemcachedDefaulting_MonitoringDisabledStillDefaults`       | REQ-004                   | ExporterImage defaulted even when monitoring.enabled=false (section is non-nil)                               |
| `TestMemcachedDefaulting_ServiceMonitorFullySpecifiedPreserved` | REQ-004                   | Fully specified serviceMonitor with additionalLabels preserved                                                |
| `TestMemcachedDefaulting_HAWithPDBStillDefaultsPreset`          | REQ-005                   | AntiAffinityPreset defaulted to soft even when PDB sub-section exists                                         |
| `TestMemcachedDefaulting_IdempotentWithMonitoringAndHA`         | REQ-004, REQ-005          | Idempotent with monitoring and HA sections present                                                            |

---

## Unit Tests: Validation Webhook

**File**: `api/v1alpha1/memcached_validation_webhook_test.go`

Standard Go `testing.T` tests with table-driven test patterns calling
`MemcachedCustomValidator.ValidateCreate/Update/Delete()` directly.

### Test Inventory

| Test Function                                             | REQ     | What It Verifies                                                                                                                                                                                                                                                                                     |
|-----------------------------------------------------------|---------|------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `TestValidateCreate_ValidMinimalCR`                       | —       | Minimal CR passes validation                                                                                                                                                                                                                                                                         |
| `TestValidateUpdate_ValidMinimalCR`                       | —       | Minimal CR update passes validation                                                                                                                                                                                                                                                                  |
| `TestValidateDelete_AlwaysSucceeds`                       | REQ-010 | Delete succeeds even for invalid CR (SASL without secret)                                                                                                                                                                                                                                            |
| `TestValidateCreate_FullyPopulatedValidCR`                | REQ-010 | Fully populated valid CR with all features passes                                                                                                                                                                                                                                                    |
| `TestValidateMemoryLimit` (table-driven, 10 cases)        | REQ-006 | Sufficient (pass), exact boundary 96Mi (pass), insufficient (fail), no limit (pass), nil resources (pass), 1-byte-below boundary (fail), large maxMemoryMB sufficient/insufficient, CPU-only limits, nil memcached with resources, empty limits map                                                  |
| `TestValidateMemoryLimit_ErrorMessage`                    | REQ-006 | Error references "memory" and includes required minimum "96Mi"                                                                                                                                                                                                                                       |
| `TestValidatePDB` (table-driven, 14 cases)                | REQ-007 | minAvailable only (pass), maxUnavailable only (pass), percentage minAvailable (pass), both set (fail), neither set (fail), disabled (pass), nil PDB (pass), nil HA (pass), minAvailable < / = / > replicas, percentage skips replicas check, nil replicas skips check, maxUnavailable integer (pass) |
| `TestValidatePDB_ErrorMessages`                           | REQ-007 | Mutual exclusivity error message; minAvailable >= replicas error includes both values                                                                                                                                                                                                                |
| `TestValidateSecuritySecretRefs` (table-driven, 10 cases) | REQ-008 | SASL+secret (pass), SASL-no-secret (fail), SASL disabled (pass), TLS+secret (pass), TLS-no-secret (fail), TLS disabled (pass), both valid (pass), both invalid (fail), nil security (pass), nil SASL/TLS (pass)                                                                                      |
| `TestValidateSecuritySecretRefs_ErrorMessages`            | REQ-008 | SASL error includes "credentialsSecretRef"; TLS error includes "certificateSecretRef"                                                                                                                                                                                                                |
| `TestValidateGracefulShutdown` (table-driven, 7 cases)    | REQ-009 | Valid timing (pass), equal values (fail), grace < preStop (fail), disabled (pass), nil gracefulShutdown (pass), nil HA (pass), minimal margin grace=preStop+1 (pass)                                                                                                                                 |
| `TestValidateGracefulShutdown_ErrorMessage`               | REQ-009 | Error references "terminationGracePeriodSeconds"                                                                                                                                                                                                                                                     |
| `TestValidation_MultipleErrorsCollected`                  | REQ-010 | Memory + PDB + SASL violations return all three in one response                                                                                                                                                                                                                                      |
| `TestValidation_FourSimultaneousViolations`               | REQ-010 | Memory + PDB + graceful shutdown + SASL violations all present in error                                                                                                                                                                                                                              |
| `TestValidation_StatusErrorFormat`                        | REQ-010 | Error is `*apierrors.StatusError` with Status=Failure, Reason=Invalid                                                                                                                                                                                                                                |
| `TestValidateUpdate_PropagatesErrors`                     | REQ-010 | Update with invalid config is rejected                                                                                                                                                                                                                                                               |
| `TestValidateUpdate_ValidToInvalid`                       | REQ-010 | Updating from valid to invalid config rejected with memory error                                                                                                                                                                                                                                     |
| `TestValidateUpdate_ValidCRAccepted`                      | REQ-010 | Valid update accepted                                                                                                                                                                                                                                                                                |
| `TestValidateDelete_InvalidCRStillDeletes`                | REQ-010 | CR with multiple violations can still be deleted; no warnings                                                                                                                                                                                                                                        |

---

## Integration Tests: Defaulting Webhook

**File**: `internal/controller/memcached_webhook_integration_test.go`

Ginkgo/Gomega tests running against an envtest API server with webhooks active.
CRs are created via `k8sClient.Create()` and fetched via `k8sClient.Get()` to
verify defaults survive the full admission round-trip.

### Test Inventory

| Test (Describe/Context/It)                                         | REQ                       | What It Verifies                                                                                                                       |
|--------------------------------------------------------------------|---------------------------|----------------------------------------------------------------------------------------------------------------------------------------|
| `Webhook Defaulting via API Server` / `minimal CR with empty spec` | REQ-001, REQ-002, REQ-003 | After Create+Get: replicas=1, image=memcached:1.6, full memcached config, monitoring=nil, HA=nil                                       |
| `Webhook Defaulting via API Server` / `fully specified CR`         | REQ-001–REQ-005           | All explicit values preserved after round-trip: replicas=5, custom image, full memcached config, custom monitoring, hard anti-affinity |

---

## Integration Tests: Validation Webhook

**File**: `internal/controller/memcached_validation_webhook_integration_test.go`

Ginkgo/Gomega tests verifying that validation rejections come through the
Kubernetes admission chain (not just the function directly).

### Test Inventory

| Test (Describe/Context/It)                   | REQ             | What It Verifies                                                                                       |
|----------------------------------------------|-----------------|--------------------------------------------------------------------------------------------------------|
| `rejects insufficient memory limit`          | REQ-006         | Create returns error containing `spec.resources.limits.memory`                                         |
| `rejects PDB minAvailable >= replicas`       | REQ-007         | Create returns error containing `spec.highAvailability.podDisruptionBudget.minAvailable`               |
| `rejects PDB mutual exclusivity`             | REQ-007         | Create returns error containing "mutually exclusive"                                                   |
| `rejects SASL without secret`                | REQ-008         | Create returns error containing `spec.security.sasl.credentialsSecretRef.name`                         |
| `rejects TLS without secret`                 | REQ-008         | Create returns error containing `spec.security.tls.certificateSecretRef.name`                          |
| `rejects graceful shutdown timing violation` | REQ-009         | Create returns error containing `spec.highAvailability.gracefulShutdown.terminationGracePeriodSeconds` |
| `accepts valid CR with all features`         | REQ-006–REQ-010 | Fully valid CR with all features succeeds                                                              |
| `minimal CR passes after defaulting`         | REQ-001         | Minimal empty-spec CR accepted because defaults fill values                                            |
| `rejects update to invalid config`           | REQ-010         | Updating valid CR to remove SASL secret is rejected                                                    |

---

## E2E Tests: Webhook Defaulting

**Directory**: `test/e2e/webhook-defaulting/`

Chainsaw test that creates a minimal CR on a real kind cluster and asserts that
webhook defaults are applied.

### Files

| File                              | Purpose                                                        |
|-----------------------------------|----------------------------------------------------------------|
| `chainsaw-test.yaml`              | Test definition with create + assert steps                     |
| `00-memcached-minimal.yaml`       | Minimal CR with only resources specified                       |
| `01-assert-defaults-applied.yaml` | Asserts replicas=1, image=memcached:1.6, full memcached config |

### Steps

| Step                    | Operation                                | Assertion                                                                                       |
|-------------------------|------------------------------------------|-------------------------------------------------------------------------------------------------|
| create-minimal-cr       | `apply` 00-memcached-minimal.yaml        | CR created                                                                                      |
| assert-defaults-applied | `assert` 01-assert-defaults-applied.yaml | replicas=1, image=memcached:1.6, maxMemoryMB=64, maxConnections=1024, threads=4, maxItemSize=1m |

---

## E2E Tests: Webhook Rejection

**Directory**: `test/e2e/webhook-rejection/`

Chainsaw test that attempts to create invalid CRs and asserts they are rejected
by the validating webhook using `expect` with `($error != null): true`.

### Files

| File                                  | Invalid Configuration                                    |
|---------------------------------------|----------------------------------------------------------|
| `00-invalid-memory-limit.yaml`        | maxMemoryMB=64 with memory limit=32Mi                    |
| `01-invalid-pdb-both.yaml`            | Both minAvailable and maxUnavailable set                 |
| `02-invalid-graceful-shutdown.yaml`   | terminationGracePeriodSeconds <= preStopDelaySeconds     |
| `03-invalid-sasl-no-secret.yaml`      | SASL enabled without credentialsSecretRef                |
| `04-invalid-tls-no-secret.yaml`       | TLS enabled without certificateSecretRef                 |
| `05-invalid-pdb-neither.yaml`         | PDB enabled with neither minAvailable nor maxUnavailable |
| `06-invalid-pdb-min-ge-replicas.yaml` | minAvailable=3 equals replicas=3                         |

### Steps

| Step                                    | Invalid CR                          | Expected         |
|-----------------------------------------|-------------------------------------|------------------|
| reject-insufficient-memory-limit        | 00-invalid-memory-limit.yaml        | `$error != null` |
| reject-pdb-mutual-exclusivity           | 01-invalid-pdb-both.yaml            | `$error != null` |
| reject-graceful-shutdown-invalid-period | 02-invalid-graceful-shutdown.yaml   | `$error != null` |
| reject-sasl-without-secret-ref          | 03-invalid-sasl-no-secret.yaml      | `$error != null` |
| reject-tls-without-secret-ref           | 04-invalid-tls-no-secret.yaml       | `$error != null` |
| reject-pdb-neither-set                  | 05-invalid-pdb-neither.yaml         | `$error != null` |
| reject-pdb-min-available-ge-replicas    | 06-invalid-pdb-min-ge-replicas.yaml | `$error != null` |

---

## Test Patterns

### Unit Test Pattern: Direct Webhook Invocation

Unit tests call the webhook functions directly without Kubernetes infrastructure:

```go
func TestMemcachedDefaulting_EmptySpec(t *testing.T) {
    mc := &Memcached{}
    d := &MemcachedCustomDefaulter{}

    if err := d.Default(context.Background(), mc); err != nil {
        t.Fatalf("unexpected error: %v", err)
    }

    if mc.Spec.Replicas == nil || *mc.Spec.Replicas != 1 {
        t.Errorf("expected replicas=1, got %v", mc.Spec.Replicas)
    }
}
```

### Unit Test Pattern: Table-Driven Validation

Validation tests use table-driven patterns with `wantError` for pass/fail cases:

```go
func TestValidateMemoryLimit(t *testing.T) {
    tests := []struct {
        name      string
        mc        *Memcached
        wantError bool
    }{
        {name: "sufficient", mc: ..., wantError: false},
        {name: "insufficient", mc: ..., wantError: true},
    }

    v := &MemcachedCustomValidator{}
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            _, err := v.ValidateCreate(context.Background(), tt.mc)
            if (err != nil) != tt.wantError {
                t.Errorf("wantError=%v, got err=%v", tt.wantError, err)
            }
        })
    }
}
```

### Integration Test Pattern: Envtest Round-Trip

Integration tests create CRs via the API server and verify the result:

```go
var _ = Describe("Webhook Defaulting via API Server", func() {
    Context("minimal CR", func() {
        It("should apply defaults", func() {
            mc := validMemcached(uniqueName("wh-minimal"))
            Expect(k8sClient.Create(ctx, mc)).To(Succeed())

            fetched := &memcachedv1alpha1.Memcached{}
            Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(mc), fetched)).To(Succeed())

            Expect(*fetched.Spec.Replicas).To(Equal(int32(1)))
        })
    })
})
```

### E2E Test Pattern: Chainsaw Error Expectation

E2E rejection tests use Chainsaw's `expect` to assert that `apply` fails:

```yaml
steps:
  - name: reject-invalid-cr
    try:
      - apply:
          file: 00-invalid-cr.yaml
          expect:
            - check:
                ($error != null): true
```

---

## Requirement Coverage Matrix

### REQ-001: Replicas defaults to 1; zero/explicit preserved

- **Unit**: `EmptySpec`, `ReplicasZeroPreserved`, `PreservesExplicitValues`,
  `FullySpecifiedCRUnchanged`, `Idempotent`
- **Envtest**: `minimal CR`, `fully specified CR`
- **E2E**: `webhook-defaulting`

### REQ-002: Image defaults to memcached:1.6; custom preserved

- **Unit**: `EmptySpec`, `PreservesExplicitValues`, `EmptyStringImagePreserved`,
  `FullySpecifiedCRUnchanged`
- **Envtest**: `minimal CR`, `fully specified CR`
- **E2E**: `webhook-defaulting`

### REQ-003: Memcached config initialized and partial-filled

- **Unit**: `NilMemcachedConfig`, `PartialMemcachedConfig`, `VerbosityZeroExplicit`,
  `ExtraArgsPreserved`, `FullySpecifiedCRUnchanged`
- **Envtest**: `minimal CR`, `fully specified CR`
- **E2E**: `webhook-defaulting`

### REQ-004: Monitoring sub-fields defaulted when section non-nil

- **Unit**: `MonitoringExporterImage`, `MonitoringExporterImagePreserved`,
  `NilMonitoringStaysNil`, `ServiceMonitorDefaults`, `ServiceMonitorPartialPreserved`,
  `NilServiceMonitorStaysNil`, `MonitoringDisabledStillDefaults`,
  `ServiceMonitorFullySpecifiedPreserved`, `IdempotentWithMonitoringAndHA`
- **Envtest**: `fully specified CR`
- **E2E**: —

### REQ-005: HA antiAffinityPreset defaulted when section non-nil

- **Unit**: `AntiAffinityPreset`, `AntiAffinityPresetHardPreserved`, `NilHAStaysNil`,
  `HAWithPDBStillDefaultsPreset`, `IdempotentWithMonitoringAndHA`
- **Envtest**: `fully specified CR`
- **E2E**: —

### REQ-006: Memory limit >= maxMemoryMB + 32Mi overhead

- **Unit**: `TestValidateMemoryLimit` (10 cases), `TestValidateMemoryLimit_ErrorMessage`
- **Envtest**: `rejects insufficient memory limit`
- **E2E**: `reject-insufficient-memory-limit`

### REQ-007: PDB mutual exclusivity, neither-set, minAvailable < replicas

- **Unit**: `TestValidatePDB` (14 cases), `TestValidatePDB_ErrorMessages`
- **Envtest**: `rejects PDB minAvailable >= replicas`, `rejects PDB mutual exclusivity`
- **E2E**: `reject-pdb-mutual-exclusivity`, `reject-pdb-neither-set`,
  `reject-pdb-min-available-ge-replicas`

### REQ-008: SASL/TLS require secret references when enabled

- **Unit**: `TestValidateSecuritySecretRefs` (10 cases),
  `TestValidateSecuritySecretRefs_ErrorMessages`
- **Envtest**: `rejects SASL without secret`, `rejects TLS without secret`
- **E2E**: `reject-sasl-without-secret-ref`, `reject-tls-without-secret-ref`

### REQ-009: Graceful shutdown: terminationGrace > preStopDelay

- **Unit**: `TestValidateGracefulShutdown` (7 cases),
  `TestValidateGracefulShutdown_ErrorMessage`
- **Envtest**: `rejects graceful shutdown timing violation`
- **E2E**: `reject-graceful-shutdown-invalid-period`

### REQ-010: Error aggregation, StatusError format, delete bypass, update propagation

- **Unit**: `MultipleErrorsCollected`, `FourSimultaneousViolations`, `StatusErrorFormat`,
  `PropagatesErrors`, `ValidToInvalid`, `ValidCRAccepted`, `DeleteAlwaysSucceeds`,
  `InvalidCRStillDeletes`, `FullyPopulatedValidCR`
- **Envtest**: `accepts valid CR`, `minimal CR passes`,
  `rejects update to invalid config`
- **E2E**: —

---

## Running the Tests

### Unit tests only

```bash
# Defaulting webhook
go test ./api/v1alpha1/ -run TestMemcachedDefaulting -v

# Validation webhook
go test ./api/v1alpha1/ -run 'TestValidate|TestValidation' -v
```

### All tests (unit + envtest integration)

```bash
make test
```

### E2E tests (requires kind cluster with operator deployed)

```bash
make test-e2e
```

### Single E2E scenario

```bash
$(LOCALBIN)/chainsaw test --test-dir test/e2e/webhook-defaulting/
$(LOCALBIN)/chainsaw test --test-dir test/e2e/webhook-rejection/
```
