# Envtest Integration Tests

Reference documentation for the envtest integration test suite that validates the
full reconciliation loop against a real API server, covering resource creation,
spec propagation, idempotency, deletion, status conditions, and multi-instance
isolation.

**Source**: `internal/controller/memcached_envtest_integration_test.go`

## Overview

The envtest integration tests exercise the reconciler end-to-end by running
`reconcileOnce()` against a real API server (provided by controller-runtime's
envtest framework). Unlike unit tests that use fake clients or test individual
builder functions, these tests create actual Memcached CRs via the API server
with webhook validation active, invoke the reconciler, and verify the resulting
Kubernetes resources.

All integration tests live in a single file
(`memcached_envtest_integration_test.go`) within the `controller_test` package,
sharing the envtest bootstrap from `suite_test.go`.

---

## Test Infrastructure

### Envtest Bootstrap (`suite_test.go`)

The test suite configures a real API server with:

- **CRD installation** from `config/crd/bases` and `config/crd/thirdparty`
  (includes Prometheus Operator CRDs for ServiceMonitor)
- **Webhook server** from `config/webhook` (defaulting and validation webhooks
  are active during all tests)
- **Controller manager** started in a background goroutine (enables garbage
  collection via owner references)

```
suite_test.go variables available to all tests:
├── k8sClient  client.Client     — envtest API client
├── ctx        context.Context    — cancellable context
├── cfg        *rest.Config       — API server config
└── testEnv    *envtest.Environment
```

### Shared Helper Functions

Helpers are defined across existing test files and reused by integration tests:

| Helper | Defined In | Purpose |
|--------|-----------|---------|
| `uniqueName(prefix)` | `memcached_crd_validation_test.go` | Generates `prefix-<uuid8>` for test isolation |
| `validMemcached(name)` | `memcached_crd_validation_test.go` | Returns a minimal valid CR in `default` namespace |
| `int32Ptr(i)` | `memcached_crd_validation_test.go` | Returns `*int32` for spec fields |
| `strPtr(s)` | `memcached_crd_validation_test.go` | Returns `*string` for spec fields |
| `reconcileOnce(mc)` | `memcached_deployment_reconcile_test.go` | Creates a reconciler and runs one reconcile cycle |
| `fetchDeployment(mc)` | `memcached_deployment_reconcile_test.go` | Gets the Deployment with same name/namespace as CR |
| `fetchService(mc)` | `memcached_service_reconcile_test.go` | Gets the Service with same name/namespace as CR |
| `fetchPDB(mc)` | `memcached_pdb_reconcile_test.go` | Gets the PDB with same name/namespace as CR |
| `fetchServiceMonitor(mc)` | `memcached_servicemonitor_reconcile_test.go` | Gets the ServiceMonitor with same name/namespace as CR |
| `fetchNetworkPolicy(mc)` | `memcached_networkpolicy_reconcile_test.go` | Gets the NetworkPolicy with same name/namespace as CR |
| `findCondition(conditions, type)` | `memcached_status_reconcile_test.go` | Finds a status condition by type string |

### `reconcileOnce` Implementation

```go
func reconcileOnce(mc *memcachedv1alpha1.Memcached) (ctrl.Result, error) {
    r := &controller.MemcachedReconciler{
        Client: k8sClient,
        Scheme: scheme.Scheme,
    }
    return r.Reconcile(ctx, ctrl.Request{
        NamespacedName: client.ObjectKeyFromObject(mc),
    })
}
```

This creates a fresh reconciler for each call, using the envtest `k8sClient`.
It does **not** set up watches or event recording — it invokes the `Reconcile`
method directly with a `ctrl.Request`.

---

## Test Organization

The integration tests are organized into Ginkgo `Describe` blocks by concern:

| Describe Block | Concern | REQ Coverage |
|---------------|---------|--------------|
| Full reconciliation loop: minimal CR | Deployment + Service creation, no optional resources | REQ-001, REQ-002 |
| Full reconciliation loop: full-featured CR | All five resources created in one reconcile | REQ-001, REQ-002 |
| Spec update propagation | Replicas, image, monitoring changes propagate | REQ-003 |
| Optional resource enable/disable lifecycle | PDB, ServiceMonitor, NetworkPolicy toggle on/off | REQ-001, REQ-003 |
| Full idempotency | Three consecutive reconciles without resource version changes | REQ-006 |
| Status conditions lifecycle | Available, Progressing, Degraded through lifecycle stages | REQ-005 |
| CR deletion and garbage collection | Owner references enable GC, reconcile returns success for deleted CR | REQ-004 |
| Owned resource recreation after external deletion | Drift correction recreates deleted resources | REQ-007 |
| Multi-instance isolation | Two CRs in same namespace with independent resources | REQ-008 |
| Cross-resource consistency | Monitoring toggle updates Deployment, Service, NetworkPolicy atomically | REQ-003 |
| Full create-update-delete lifecycle | End-to-end lifecycle in a single test | REQ-001, REQ-003, REQ-004, REQ-005 |

---

## Test Patterns

### Standard Test Structure

Every integration test follows this pattern:

```go
var _ = Describe("Category", func() {
    Context("scenario description", func() {
        var mc *memcachedv1alpha1.Memcached

        BeforeEach(func() {
            mc = validMemcached(uniqueName("prefix"))
            // Configure spec fields...
            Expect(k8sClient.Create(ctx, mc)).To(Succeed())
            Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(mc), mc)).To(Succeed())

            result, err := reconcileOnce(mc)
            Expect(err).NotTo(HaveOccurred())
            Expect(result).To(Equal(ctrl.Result{}))
        })

        It("should verify resource state", func() {
            dep := fetchDeployment(mc)
            Expect(*dep.Spec.Replicas).To(Equal(int32(1)))
        })
    })
})
```

Key conventions:
1. **`uniqueName()`** for every CR — prevents cross-test interference
2. **`validMemcached()`** as the starting point — webhook-valid minimal CR
3. **`BeforeEach`** creates the CR and runs the initial reconcile
4. **`It`** blocks verify specific aspects of the resulting state
5. **Re-fetch before mutation** — `k8sClient.Get()` before `k8sClient.Update()`
   to get a fresh `resourceVersion`

### Spec Update Pattern

```go
// Always re-fetch before updating to get current resourceVersion.
Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(mc), mc)).To(Succeed())
mc.Spec.Replicas = int32Ptr(3)
Expect(k8sClient.Update(ctx, mc)).To(Succeed())

_, err = reconcileOnce(mc)
Expect(err).NotTo(HaveOccurred())

dep = fetchDeployment(mc)
Expect(*dep.Spec.Replicas).To(Equal(int32(3)))
```

### Resource Absence Verification

```go
pdb := &policyv1.PodDisruptionBudget{}
err := k8sClient.Get(ctx, client.ObjectKeyFromObject(mc), pdb)
Expect(apierrors.IsNotFound(err)).To(BeTrue())
```

### Idempotency Verification

```go
dep1 := fetchDeployment(mc)
rv := dep1.ResourceVersion

_, err := reconcileOnce(mc)
Expect(err).NotTo(HaveOccurred())

dep2 := fetchDeployment(mc)
Expect(dep2.ResourceVersion).To(Equal(rv))
```

### Owner Reference Verification

```go
for _, obj := range []client.Object{dep, svc, pdb, sm, np} {
    refs := obj.GetOwnerReferences()
    Expect(refs).To(HaveLen(1))
    Expect(refs[0].Name).To(Equal(mc.Name))
    Expect(refs[0].UID).To(Equal(mc.UID))
    Expect(refs[0].Kind).To(Equal("Memcached"))
    Expect(*refs[0].Controller).To(BeTrue())
    Expect(*refs[0].BlockOwnerDeletion).To(BeTrue())
}
```

### Label Consistency Verification

```go
for _, obj := range []client.Object{dep, svc, pdb, sm, np} {
    labels := obj.GetLabels()
    Expect(labels).To(HaveKeyWithValue("app.kubernetes.io/name", "memcached"))
    Expect(labels).To(HaveKeyWithValue("app.kubernetes.io/instance", mc.Name))
    Expect(labels).To(HaveKeyWithValue("app.kubernetes.io/managed-by", "memcached-operator"))
}
```

---

## Envtest Pitfalls

| Pitfall | Impact | Mitigation |
|---------|--------|------------|
| No kubelet in envtest | `readyReplicas` always remains `0` in Deployment status | Tests assert `Degraded=True` for non-zero desired replicas; use `replicas=0` to test `Available=True` |
| Webhook validation is active | CR mutations that violate CRD validation will be rejected | Always start from `validMemcached()` and make incremental changes |
| Resource names must be unique | Shared envtest instance across all tests in the suite | Always use `uniqueName()` with a descriptive prefix |
| Reconciler does not delete optional resources | Disabling a feature flag does not remove the resource | Tests verify the reconciler succeeds (no error) but do not assert NotFound for disabled resources |
| GC requires controller manager | Owner reference cascade only works when the manager is running | `suite_test.go` starts the manager; GC-dependent tests use `Eventually()` |
| Re-fetch before update | `k8sClient.Update()` requires current `resourceVersion` | Always call `k8sClient.Get()` before `k8sClient.Update()` |

---

## Requirement Coverage Matrix

| REQ-ID | Requirement | Test Scenarios |
|--------|------------|----------------|
| REQ-001 | Create all required resources per CR | Minimal CR: Deployment + Service; Full CR: all five resources; Optional resources NotFound when disabled |
| REQ-002 | Owner references with controller=true, blockOwnerDeletion=true | Verified on all resources for both minimal and full-featured CRs |
| REQ-003 | Spec changes propagate in single reconcile | Replicas, image, monitoring enable/disable, cross-resource consistency (Deployment + Service + NetworkPolicy updated atomically) |
| REQ-004 | CR deletion handled gracefully | Reconcile returns `ctrl.Result{}` + nil for deleted CR; owner references enable GC |
| REQ-005 | Status conditions set correctly | Initial: Available=False/Progressing=True/Degraded=True; Zero replicas: Available=True/Progressing=False/Degraded=False; ObservedGeneration tracks changes |
| REQ-006 | Idempotent reconciliation | Three consecutive reconciles on full-featured CR: no resource version changes after first; post-update idempotency verified |
| REQ-007 | Recreate externally deleted resources | Deployment, Service, PDB, ServiceMonitor, NetworkPolicy each tested individually; multi-resource simultaneous deletion tested |
| REQ-008 | Multi-instance isolation | Two CRs: independent resources, distinct labels, deletion of one does not affect the other |

---

## Adding a New Integration Test

To add a new integration test scenario:

### 1. Choose the test file

All integration tests go in `internal/controller/memcached_envtest_integration_test.go`.

### 2. Write the test

```go
var _ = Describe("New Feature Integration", func() {
    Context("when the new feature is enabled", func() {
        It("should produce the expected resource state", func() {
            mc := validMemcached(uniqueName("integ-new-feature"))
            // Configure the CR spec for your scenario.
            mc.Spec.NewField = someValue

            Expect(k8sClient.Create(ctx, mc)).To(Succeed())
            Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(mc), mc)).To(Succeed())

            _, err := reconcileOnce(mc)
            Expect(err).NotTo(HaveOccurred())

            // Verify the expected resource state.
            dep := fetchDeployment(mc)
            Expect(dep.Spec.SomeField).To(Equal(expectedValue))
        })
    })
})
```

### 3. Follow conventions

- Use `uniqueName("integ-<short-descriptor>")` for the CR name
- Start from `validMemcached()` and add only the fields your test needs
- Use `fetch*` helpers for resource retrieval (they fail the test on NotFound)
- Use `apierrors.IsNotFound(err)` when asserting resource absence
- Re-fetch the CR with `k8sClient.Get()` before any `k8sClient.Update()`
- If adding a new `fetch*` helper, define it in the corresponding
  `*_reconcile_test.go` file using `ExpectWithOffset(1, ...)` for correct
  failure line reporting

### 4. Run the tests

```bash
make test
```

All integration tests run as part of the `Controller Suite` Ginkgo suite alongside
unit and reconcile-level tests.
