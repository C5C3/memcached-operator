# Secret Rotation Integration Tests

Reference documentation for the envtest integration test suite that validates
Secret rotation, missing Secret degraded conditions, manual restart triggers,
and Secret watch filtering end-to-end against a real API server.

**Source**: `internal/controller/memcached_secret_rotation_integration_test.go`

## Overview

The Secret rotation integration tests exercise the reconciler's Secret-aware
behavior by running `reconcileOnce()` against a real API server (provided by
controller-runtime's envtest framework). These tests validate that:

1. **Secret hash annotation** — the Deployment pod template carries a
   `memcached.c5c3.io/secret-hash` annotation whose value changes when
   referenced Secret data changes, triggering a rolling restart.
2. **Missing Secret degraded condition** — the Memcached CR reports
   `Degraded=True` with reason `SecretNotFound` when a referenced Secret does
   not exist, and clears the condition when the Secret is created.
3. **Manual restart trigger** — the `memcached.c5c3.io/restart-trigger`
   annotation on the CR is propagated to the Deployment pod template, enabling
   operator-initiated rolling restarts.
4. **Secret watch filtering** — reconciling a CR only produces Deployment
   changes when the Secrets it references are modified, not when unrelated
   Secrets change.

All tests live in a single file
(`memcached_secret_rotation_integration_test.go`) within the `controller_test`
package, sharing the envtest bootstrap from `suite_test.go`.

---

## Test Infrastructure

These tests reuse the shared envtest infrastructure and helpers described in
[Envtest Integration Tests](envtest-integration-tests.md):

- **`k8sClient`**, **`ctx`**, **`scheme`** from `suite_test.go`
- **`uniqueName(prefix)`** for test isolation
- **`validMemcached(name)`** for webhook-valid minimal CRs
- **`reconcileOnce(mc)`** for explicit reconciliation control
- **`fetchDeployment(mc)`** for Deployment retrieval
- **`findCondition(conditions, type)`** for status condition lookup

### Additional Test-Local Setup

The test file defines a `hexHash64` regex for validating SHA-256 hash format:

```go
var hexHash64 = regexp.MustCompile(`^[0-9a-f]{64}$`)
```

Secrets are created **before** the Memcached CR in tests that expect the Secret
to exist, since `reconcileOnce()` fetches Secrets synchronously during
reconciliation.

---

## Test Organization

The tests are organized into four Ginkgo `Describe` blocks:

| Describe Block                    | Concern                                                          | REQ Coverage                       |
|-----------------------------------|------------------------------------------------------------------|------------------------------------|
| Secret rotation rolling restart   | Hash annotation lifecycle: creation, rotation, idempotency       | REQ-001, REQ-005, REQ-006, REQ-007 |
| Missing Secret Degraded condition | Degraded status on missing Secrets, clearance on creation        | REQ-002, REQ-005, REQ-007          |
| Manual restart trigger            | Restart-trigger annotation propagation and coexistence with hash | REQ-003, REQ-005, REQ-006          |
| Secret watch filtering            | Per-CR isolation when only one CR's Secret changes               | REQ-004, REQ-005                   |

---

## Test Scenarios

### Secret Rotation Rolling Restart

| Scenario                                                         | What It Validates                                                                                     |
|------------------------------------------------------------------|-------------------------------------------------------------------------------------------------------|
| SASL Secret exists → Deployment has secret-hash annotation       | Non-empty 64-char hex hash in `memcached.c5c3.io/secret-hash` pod template annotation                 |
| Secret data changes → hash and ResourceVersion change            | Hash value differs after Secret rotation; Deployment ResourceVersion changes (rolling update trigger) |
| TLS Secret exists → Deployment has secret-hash annotation        | Same hash behavior for TLS Secret references                                                          |
| Both SASL and TLS Secrets → combined hash, either change updates | Single hash covers both Secrets; updating either Secret changes the hash                              |
| No Secret references → no secret-hash annotation                 | Annotation is absent when no Secrets are configured                                                   |
| Secret unchanged → Deployment ResourceVersion stable             | Idempotent reconciliation: no Deployment update when Secret data is unchanged                         |

### Missing Secret Degraded Condition

| Scenario                                        | What It Validates                                                                              |
|-------------------------------------------------|------------------------------------------------------------------------------------------------|
| Secret does not exist → Degraded=True           | `Degraded` condition with `Status=True`, `Reason=SecretNotFound`, message contains Secret name |
| Missing Secret created → Degraded clears        | After creating the Secret and reconciling, `Degraded.Reason` is no longer `SecretNotFound`     |
| Both SASL and TLS Secrets missing → both in msg | Degraded condition message includes both missing Secret names                                  |

### Manual Restart Trigger

| Scenario                                         | What It Validates                                                                       |
|--------------------------------------------------|-----------------------------------------------------------------------------------------|
| Set restart-trigger annotation → propagated      | Pod template annotation matches CR annotation value; Deployment ResourceVersion changes |
| Update restart-trigger value → Deployment update | New annotation value propagated; Deployment ResourceVersion changes                     |
| Coexist with secret-hash annotation              | Both `secret-hash` and `restart-trigger` annotations present simultaneously             |
| Restart-trigger without Secrets                  | Annotation propagated even when no Secret references are configured                     |
| No restart-trigger annotation → not set          | Annotation absent from pod template when CR has no restart-trigger                      |

### Secret Watch Filtering

| Scenario                                       | What It Validates                                                                          |
|------------------------------------------------|--------------------------------------------------------------------------------------------|
| Only referenced CR affected by Secret update   | CR-A's hash changes after SecretA update; CR-B's hash and ResourceVersion remain unchanged |
| Both CRs reference same Secret → both affected | Shared Secret update changes hash for both CRs; new hashes are equal                       |

---

## Test Patterns

### Secret-Before-CR Pattern

Secrets must be created before the Memcached CR when the test expects the
Secret to exist during reconciliation:

```go
secret := &corev1.Secret{
    ObjectMeta: metav1.ObjectMeta{Name: secretName, Namespace: "default"},
    Data:       map[string][]byte{"password-file": []byte("initial-password")},
}
Expect(k8sClient.Create(ctx, secret)).To(Succeed())

mc := validMemcached(uniqueName("prefix"))
mc.Spec.Security = &memcachedv1alpha1.SecuritySpec{
    SASL: &memcachedv1alpha1.SASLSpec{
        Enabled:              true,
        CredentialsSecretRef: corev1.LocalObjectReference{Name: secretName},
    },
}
Expect(k8sClient.Create(ctx, mc)).To(Succeed())
```

For missing-Secret tests, the Secret is intentionally **not** created before
the CR.

### Hash Rotation Pattern

```go
// Initial reconcile — capture hash and ResourceVersion.
dep := fetchDeployment(mc)
initialHash := dep.Spec.Template.Annotations[controller.AnnotationSecretHash]
initialRV := dep.ResourceVersion

// Update Secret data.
Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(secret), secret)).To(Succeed())
secret.Data["password-file"] = []byte("rotated-password")
Expect(k8sClient.Update(ctx, secret)).To(Succeed())

// Reconcile — hash and ResourceVersion should change.
_, err = reconcileOnce(mc)
Expect(err).NotTo(HaveOccurred())

dep = fetchDeployment(mc)
Expect(dep.Spec.Template.Annotations[controller.AnnotationSecretHash]).NotTo(Equal(initialHash))
Expect(dep.ResourceVersion).NotTo(Equal(initialRV))
```

### Degraded Condition Verification Pattern

```go
Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(mc), mc)).To(Succeed())

degraded := findCondition(mc.Status.Conditions, "Degraded")
Expect(degraded).NotTo(BeNil())
Expect(degraded.Status).To(Equal(metav1.ConditionTrue))
Expect(degraded.Reason).To(Equal("SecretNotFound"))
Expect(degraded.Message).To(ContainSubstring(missingSecretName))
```

### CR Annotation Update Pattern

```go
// Re-fetch to get current resourceVersion before updating annotations.
Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(mc), mc)).To(Succeed())
if mc.Annotations == nil {
    mc.Annotations = map[string]string{}
}
mc.Annotations[controller.AnnotationRestartTrigger] = "2024-01-15T10:00:00Z"
Expect(k8sClient.Update(ctx, mc)).To(Succeed())
```

---

## Annotation Constants

Tests use exported constants from the `controller` package for annotation keys:

| Constant                              | Value                               | Used In                          |
|---------------------------------------|-------------------------------------|----------------------------------|
| `controller.AnnotationSecretHash`     | `memcached.c5c3.io/secret-hash`     | Secret rotation, watch filtering |
| `controller.AnnotationRestartTrigger` | `memcached.c5c3.io/restart-trigger` | Manual restart trigger           |

---

## Requirement Coverage Matrix

| REQ-ID  | Requirement                                                                      | Test Scenarios                                                                                                               |
|---------|----------------------------------------------------------------------------------|------------------------------------------------------------------------------------------------------------------------------|
| REQ-001 | Deployment has secret-hash annotation when Secret exists                         | SASL Secret hash, TLS Secret hash, both Secrets combined hash                                                                |
| REQ-002 | Degraded=True with SecretNotFound when Secret missing                            | Single missing Secret, both SASL and TLS missing, clearance after creation                                                   |
| REQ-003 | Restart-trigger annotation propagated to Deployment                              | Initial propagation, value update, coexistence with secret-hash, without Secrets, absent when not set                        |
| REQ-004 | Only referenced CRs reconciled on Secret change                                  | Isolated Secret update affects only referencing CR; shared Secret update affects both CRs                                    |
| REQ-005 | Tests use envtest with reconcileOnce() and verify actual Kubernetes object state | All tests use reconcileOnce() + k8sClient.Get() to verify Deployment annotations and CR status conditions                    |
| REQ-006 | Hash changes trigger Deployment ResourceVersion change (rolling update)          | Secret rotation changes ResourceVersion; restart-trigger changes ResourceVersion; unchanged Secret preserves ResourceVersion |
| REQ-007 | Degraded condition lifecycle: set on missing, clear on creation                  | SecretNotFound set, then cleared after Secret creation; hash absent → present after Secret creation                          |
| REQ-008 | All four test scenarios pass in envtest with Ginkgo/Gomega patterns              | This documentation covers the complete test suite                                                                            |

---

## Running the Tests

```bash
make test
```

All Secret rotation integration tests run as part of the `Controller Suite`
Ginkgo suite alongside unit and other integration tests. To run only the Secret
rotation tests:

```bash
go test ./internal/controller/ -v -run "Secret rotation|Missing Secret|Manual restart|Secret watch"
```
