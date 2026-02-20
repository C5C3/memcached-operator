# Chainsaw E2E Tests

Reference documentation for the Kyverno Chainsaw end-to-end test suite that
validates the Memcached operator against a real kind cluster, covering
deployment, scaling, configuration changes, monitoring, PDB management,
graceful rolling updates, webhook validation, and garbage collection.

**Source**: `test/e2e/`

## Overview

The E2E test suite exercises the operator end-to-end by deploying it to a kind
cluster and applying Memcached custom resources via `kubectl`. Unlike envtest
integration tests that run against an in-process API server, these tests
validate the full operator lifecycle including controller watches, leader
election, webhook TLS, and Kubernetes garbage collection.

The suite uses [Kyverno Chainsaw](https://kyverno.github.io/chainsaw/) v0.2.12,
a declarative Kubernetes E2E testing framework. Each test scenario is defined
in YAML with steps that apply resources, patch them, and assert on the
resulting cluster state using partial object matching.

---

## Test Infrastructure

### Chainsaw Configuration (`.chainsaw.yaml`)

The global configuration at the project root controls timeouts and execution:

```yaml
apiVersion: chainsaw.kyverno.io/v1alpha2
kind: Configuration
metadata:
  name: memcached-operator-e2e
spec:
  timeouts:
    apply: 30s      # Resource creation timeout
    assert: 120s    # Assertion timeout (allows for pod scheduling)
    cleanup: 60s    # Namespace cleanup timeout
    delete: 30s     # Deletion timeout
    error: 30s      # Error assertion timeout
  cleanup:
    skipDelete: false
  execution:
    failFast: true   # Stop on first failure
    parallel: 1      # Sequential execution across test cases
  discovery:
    testDirs:
      - test/e2e
```

Key timeout rationale:
- **assert: 120s** — Pod scheduling and readiness can vary significantly in CI;
  120s accommodates slow schedulers without causing false positives.
- **cleanup: 60s** — Allows Kubernetes garbage collection to cascade through
  owner references before the namespace is force-deleted.
- **parallel: 1** — Tests run sequentially to avoid resource contention on
  small kind clusters.

### Makefile Target

```bash
make test-e2e
```

Downloads Chainsaw v0.2.12 via `go install` (using the same `go-install-tool`
pattern as controller-gen, kustomize, and other project tools) and runs the
test suite:

```makefile
CHAINSAW ?= $(LOCALBIN)/chainsaw
CHAINSAW_VERSION ?= v0.2.12

.PHONY: test-e2e
test-e2e: chainsaw ## Run end-to-end tests against a kind cluster using Chainsaw.
	$(CHAINSAW) test
```

### Prerequisites

Before running `make test-e2e`, the following must be in place:

| Prerequisite | Purpose | Setup Command |
|-------------|---------|---------------|
| kind cluster running | Target cluster for tests | `kind create cluster` |
| Operator deployed | Controller manager running in cluster | `make deploy IMG=<image>` |
| cert-manager installed | Webhook TLS certificates | `kubectl apply -f https://github.com/cert-manager/cert-manager/releases/download/v1.16.1/cert-manager.yaml` |
| ServiceMonitor CRD | Required for monitoring-toggle test | Install via Prometheus Operator CRDs |

### Shared Test Fixtures (`test/e2e/resources/`)

Reusable YAML templates referenced by multiple test scenarios:

| File | Purpose |
|------|---------|
| `memcached-minimal.yaml` | Minimal valid Memcached CR (1 replica, memcached:1.6, 64Mi maxMemoryMB) |
| `assert-deployment.yaml` | Partial Deployment assertion (labels, replicas, container args, port) |
| `assert-service.yaml` | Partial headless Service assertion (clusterIP: None, port 11211, selectors) |
| `assert-status-available.yaml` | Status assertion (readyReplicas: 1, Available=True) |

---

## File Structure

```
test/e2e/
├── basic-deployment/
│   ├── chainsaw-test.yaml          # Test definition
│   ├── 00-memcached.yaml           # Minimal CR (test-basic)
│   ├── 01-assert-deployment.yaml   # Deployment assertions
│   ├── 01-assert-service.yaml      # Service assertions
│   └── 02-assert-status.yaml       # Status condition assertions
├── scaling/
│   ├── chainsaw-test.yaml
│   ├── 00-memcached.yaml           # CR with replicas=1
│   ├── 01-assert-one-replica.yaml
│   ├── 02-patch-scale-up.yaml      # Patch replicas to 3
│   ├── 03-assert-three-replicas.yaml
│   ├── 03-assert-status-scaled.yaml
│   ├── 04-patch-scale-down.yaml    # Patch replicas to 1
│   └── 05-assert-one-replica.yaml
├── configuration-changes/
│   ├── chainsaw-test.yaml
│   ├── 00-memcached.yaml           # CR with default config
│   ├── 01-assert-initial-args.yaml
│   ├── 02-patch-config.yaml        # Patch maxMemoryMB, threads, maxItemSize
│   └── 03-assert-updated-args.yaml
├── monitoring-toggle/
│   ├── chainsaw-test.yaml
│   ├── 00-memcached.yaml           # CR without monitoring
│   ├── 01-assert-no-exporter.yaml
│   ├── 02-patch-enable-monitoring.yaml
│   ├── 03-assert-exporter.yaml     # Exporter sidecar on port 9150
│   ├── 03-assert-service-metrics.yaml
│   ├── 03-assert-servicemonitor.yaml  # ServiceMonitor with labels and endpoints
│   ├── 04-patch-disable-monitoring.yaml
│   ├── 05-assert-no-exporter.yaml  # Exporter sidecar removed
│   └── 05-error-servicemonitor-gone.yaml
├── pdb-creation/
│   ├── chainsaw-test.yaml
│   ├── 00-memcached.yaml           # CR with PDB enabled (replicas=3)
│   ├── 01-assert-deployment.yaml
│   ├── 01-assert-pdb.yaml          # PDB with minAvailable=1
│   ├── 02-patch-disable-pdb.yaml
│   └── 03-error-pdb-gone.yaml
├── graceful-rolling-update/
│   ├── chainsaw-test.yaml
│   ├── 00-memcached.yaml           # CR with gracefulShutdown enabled
│   ├── 01-assert-deployment.yaml   # Strategy + preStop + terminationGracePeriod
│   ├── 02-patch-update-image.yaml  # Image change to trigger rollout
│   └── 03-assert-rolling-update.yaml
├── webhook-rejection/
│   ├── chainsaw-test.yaml
│   ├── 00-invalid-memory-limit.yaml
│   ├── 01-invalid-pdb-both.yaml
│   ├── 02-invalid-graceful-shutdown.yaml
│   ├── 03-invalid-sasl-no-secret.yaml
│   └── 04-invalid-tls-no-secret.yaml
├── cr-deletion/
│   ├── chainsaw-test.yaml
│   ├── 00-memcached.yaml           # CR with monitoring and PDB enabled
│   ├── 01-assert-deployment.yaml
│   ├── 01-assert-service.yaml
│   ├── 01-assert-pdb.yaml
│   ├── 01-assert-servicemonitor.yaml
│   ├── 02-error-deployment-gone.yaml
│   ├── 02-error-service-gone.yaml
│   ├── 02-error-pdb-gone.yaml
│   ├── 02-error-servicemonitor-gone.yaml
│   └── 02-error-cr-gone.yaml
└── resources/
    ├── memcached-minimal.yaml
    ├── assert-deployment.yaml
    ├── assert-service.yaml
    └── assert-status-available.yaml
```

---

## Test Scenarios

### 1. Basic Deployment (REQ-002)

**Directory**: `test/e2e/basic-deployment/`

Verifies that creating a minimal Memcached CR produces the expected Deployment,
headless Service, and status conditions.

| Step | Operation | Assertion |
|------|-----------|-----------|
| create-memcached-cr | `apply` 00-memcached.yaml | CR created |
| assert-deployment-created | `assert` 01-assert-deployment.yaml | Deployment with correct labels, args (`-m 64 -c 1024 -t 4 -I 1m`), port 11211 |
| assert-service-created | `assert` 01-assert-service.yaml | Headless Service (clusterIP: None), port 11211, correct selectors |
| assert-status-available | `assert` 02-assert-status.yaml | readyReplicas: 1, Available=True |

Owner references on Deployment and Service are verified as part of the
Deployment and Service assertion files (Chainsaw partial matching includes
`metadata.ownerReferences`).

### 2. Scaling (REQ-003)

**Directory**: `test/e2e/scaling/`

Verifies that updating `spec.replicas` scales the Deployment and updates
`status.readyReplicas`.

| Step | Operation | Assertion |
|------|-----------|-----------|
| create-memcached-cr | `apply` | CR with replicas=1 |
| assert-initial-deployment | `assert` | Deployment.spec.replicas=1 |
| scale-up-to-3 | `patch` replicas=3 | — |
| assert-scaled-deployment | `assert` | Deployment.spec.replicas=3 |
| assert-scaled-status | `assert` | status.readyReplicas=3 |
| scale-down-to-1 | `patch` replicas=1 | — |
| assert-scaled-down | `assert` | Deployment.spec.replicas=1 |

### 3. Configuration Changes (REQ-004)

**Directory**: `test/e2e/configuration-changes/`

Verifies that changing memcached config fields triggers a rolling update with
correct container args.

| Step | Operation | Assertion |
|------|-----------|-----------|
| create-memcached-cr | `apply` | CR with maxMemoryMB=64, threads=4 |
| assert-initial-args | `assert` | Container args: `-m 64 -c 1024 -t 4 -I 1m` |
| update-configuration | `patch` maxMemoryMB=256, threads=8, maxItemSize=2m | — |
| assert-updated-args | `assert` | Container args: `-m 256 ... -t 8 -I 2m` |

### 4. Monitoring Toggle (REQ-005)

**Directory**: `test/e2e/monitoring-toggle/`

Verifies that enabling monitoring injects the exporter sidecar and adds a
metrics port to the Service.

| Step | Operation | Assertion |
|------|-----------|-----------|
| create-memcached-without-monitoring | `apply` | CR without monitoring |
| assert-no-exporter-sidecar | `assert` | Deployment has 1 container (memcached only) |
| enable-monitoring | `patch` monitoring.enabled=true | — |
| assert-exporter-sidecar-injected | `assert` | 2 containers: memcached (port 11211) + exporter (port 9150) |
| assert-service-metrics-port | `assert` | Service has metrics port |
| assert-servicemonitor-created | `assert` | ServiceMonitor with correct labels, endpoints, and selector |
| disable-monitoring | `patch` monitoring.enabled=false | — |
| assert-exporter-sidecar-removed | `assert` | Deployment has 1 container (memcached only) |
| assert-servicemonitor-deleted | `error` | ServiceMonitor is removed |

**Prerequisite**: ServiceMonitor CRD must be installed in the cluster.

### 5. PDB Creation (REQ-006)

**Directory**: `test/e2e/pdb-creation/`

Verifies that enabling PDB creates a PodDisruptionBudget with correct settings.

| Step | Operation | Assertion |
|------|-----------|-----------|
| create-memcached-with-pdb | `apply` | CR with replicas=3, PDB enabled, minAvailable=1 |
| assert-deployment-ready | `assert` | Deployment with 3 replicas |
| assert-pdb-created | `assert` | PDB with minAvailable=1, correct selector, owner reference |
| disable-pdb | `patch` PDB enabled=false | — |
| assert-pdb-deleted | `error` | PDB is removed |

### 6. Graceful Rolling Update (REQ-007)

**Directory**: `test/e2e/graceful-rolling-update/`

Verifies that graceful shutdown configures preStop hooks and the RollingUpdate
strategy, and that image changes trigger a correct rolling update.

| Step | Operation | Assertion |
|------|-----------|-----------|
| create-memcached-with-graceful-shutdown | `apply` | CR with gracefulShutdown enabled |
| assert-graceful-shutdown-config | `assert` | RollingUpdate (maxSurge=1, maxUnavailable=0), preStop hook, terminationGracePeriodSeconds |
| trigger-rolling-update | `patch` image | — |
| assert-rolling-update-strategy | `assert` | All pods running new image, strategy preserved |

### 7. Webhook Rejection (REQ-008)

**Directory**: `test/e2e/webhook-rejection/`

Verifies that the validating webhook rejects invalid CRs. Each step uses
Chainsaw's `expect` with `($error != null): true` to assert that the `apply`
operation fails.

| Step | Invalid CR | Expected Rejection Reason |
|------|-----------|--------------------------|
| reject-insufficient-memory-limit | maxMemoryMB=64, memory limit=32Mi | Memory limit < maxMemoryMB + 32Mi overhead |
| reject-pdb-mutual-exclusivity | Both minAvailable and maxUnavailable set | Mutually exclusive fields |
| reject-graceful-shutdown-invalid-period | terminationGracePeriodSeconds <= preStopDelaySeconds | Termination period must exceed pre-stop delay |
| reject-sasl-without-secret-ref | sasl.enabled=true, no credentialsSecretRef.name | Missing required secret reference |
| reject-tls-without-secret-ref | tls.enabled=true, no certificateSecretRef.name | Missing required secret reference |

### 8. CR Deletion & Garbage Collection (REQ-009)

**Directory**: `test/e2e/cr-deletion/`

Verifies that deleting a Memcached CR garbage-collects all owned resources.

| Step | Operation | Assertion |
|------|-----------|-----------|
| create-memcached-with-all-resources | `apply` | CR with monitoring and PDB enabled |
| assert-all-resources-exist | `assert` | Deployment, Service, PDB, ServiceMonitor all present |
| delete-memcached-cr | `delete` Memcached/test-deletion | — |
| assert-all-resources-garbage-collected | `error` | Deployment, Service, PDB, ServiceMonitor, and CR are all gone |

The `error` operation asserts that the specified resource does **not** exist —
the assertion succeeds when the GET returns `NotFound`.

---

## Test Patterns

### Partial Object Matching

Chainsaw asserts on partial objects — only the fields specified in the
assertion YAML must match. This avoids brittleness from defaulted or
controller-managed fields.

```yaml
# Only checks these specific fields, ignores everything else
apiVersion: apps/v1
kind: Deployment
metadata:
  name: test-basic
  labels:
    app.kubernetes.io/name: memcached
    app.kubernetes.io/instance: test-basic
    app.kubernetes.io/managed-by: memcached-operator
spec:
  replicas: 1
  template:
    spec:
      containers:
        - name: memcached
          args: ["-m", "64", "-c", "1024", "-t", "4", "-I", "1m"]
```

### Apply-Assert-Patch-Assert Flow

Most tests follow a four-phase pattern:

1. **Apply** — Create the initial Memcached CR
2. **Assert** — Verify the initial resource state
3. **Patch** — Modify the CR spec (scaling, config change, feature toggle)
4. **Assert** — Verify the updated resource state

### Error Expectations for Webhook Tests

Webhook rejection tests use Chainsaw's `expect` mechanism on `apply` operations
to assert that resource creation fails:

```yaml
steps:
  - name: reject-insufficient-memory-limit
    try:
      - apply:
          file: 00-invalid-memory-limit.yaml
          expect:
            - check:
                ($error != null): true
```

### Negative Assertions for Deletion Tests

Deletion tests use the `error` operation type, which succeeds when the resource
does **not** exist:

```yaml
steps:
  - name: assert-all-resources-garbage-collected
    try:
      - error:
          file: 02-error-deployment-gone.yaml
```

Where the error file contains a resource reference that should no longer exist:

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: test-deletion
```

### Namespace Isolation

Chainsaw automatically creates a unique namespace for each test and cleans it
up afterward. Test resources do not specify a namespace — Chainsaw injects it
at runtime. This provides complete isolation between test cases.

---

## Requirement Coverage Matrix

| REQ-ID | Requirement | Test Scenario | Key Assertions |
|--------|------------|---------------|----------------|
| REQ-001 | Chainsaw configuration and Makefile target | All (infrastructure) | `.chainsaw.yaml` config, `make test-e2e` target |
| REQ-002 | Basic deployment: Deployment, Service, status | basic-deployment | Labels, container args, headless Service, Available=True |
| REQ-003 | Scaling: replicas up and down | scaling | Deployment.spec.replicas, status.readyReplicas |
| REQ-004 | Configuration changes: container args updated | configuration-changes | Args reflect maxMemoryMB, threads, maxItemSize |
| REQ-005 | Monitoring toggle: exporter sidecar, ServiceMonitor | monitoring-toggle | Container count, port 9150, Service metrics port, ServiceMonitor labels/endpoints, disable removes sidecar and ServiceMonitor |
| REQ-006 | PDB creation and deletion: minAvailable, selector | pdb-creation | PDB spec, selector labels, owner reference, disable removes PDB |
| REQ-007 | Graceful rolling update: strategy, preStop, image update | graceful-rolling-update | maxSurge=1, maxUnavailable=0, preStop hook, new image |
| REQ-008 | Webhook rejection: invalid CRs rejected | webhook-rejection | Five invalid CR variants all rejected |
| REQ-009 | CR deletion: garbage collection | cr-deletion | Deployment, Service, PDB, ServiceMonitor, CR all removed |
| REQ-010 | Makefile integration | All (infrastructure) | `make test-e2e` runs `chainsaw test` |

---

## Known Limitations

| Limitation | Impact | Mitigation |
|-----------|--------|------------|
| Pod scheduling time varies | Assert timeouts may need adjustment in slow CI | Global assert timeout set to 120s |
| cert-manager required | Webhook tests fail without TLS certificates | Documented as prerequisite; tests fail clearly with connection refused |
| ServiceMonitor CRD required | monitoring-toggle and cr-deletion tests fail without CRD | Documented as prerequisite; Chainsaw reports clear assertion error |
| Sequential execution | Full suite takes longer than parallel execution | `parallel: 1` avoids resource contention on small clusters |

---

## Troubleshooting

### cert-manager not ready

If webhook tests fail with `connection refused` or TLS handshake errors,
cert-manager may not be fully ready:

```bash
# Check cert-manager pods are Running
kubectl get pods -n cert-manager

# Wait for webhook to be ready
kubectl wait --for=condition=Available deployment/cert-manager-webhook \
  -n cert-manager --timeout=120s

# Verify certificates are issued
kubectl get certificates -A
```

### ServiceMonitor CRD missing

The monitoring-toggle and cr-deletion tests require the ServiceMonitor CRD.
If assertions fail with `no matches for kind "ServiceMonitor"`:

```bash
# Install Prometheus Operator CRDs
kubectl apply -f https://raw.githubusercontent.com/prometheus-operator/prometheus-operator/main/example/prometheus-operator-crd/monitoring.coreos.com_servicemonitors.yaml

# Verify the CRD is installed
kubectl get crd servicemonitors.monitoring.coreos.com
```

### Pod scheduling timeout

If assertions timeout waiting for pods to become ready:

```bash
# Check pending pods and events
kubectl get pods -A --field-selector=status.phase!=Running
kubectl get events --sort-by='.lastTimestamp' -A | tail -20

# Check node resources
kubectl describe nodes | grep -A 5 "Allocated resources"

# Increase assert timeout if needed (in .chainsaw.yaml)
# spec.timeouts.assert: 180s
```

### Debugging test failures with kubectl logs

```bash
# Check operator logs for reconciliation errors
kubectl logs -n memcached-operator-system deployment/memcached-operator-controller-manager \
  -c manager --tail=50

# Check specific test namespace (Chainsaw creates unique namespaces)
kubectl get ns | grep chainsaw
kubectl get all -n <chainsaw-namespace>

# Run a single test with verbose output
$(LOCALBIN)/chainsaw test --test-dir test/e2e/monitoring-toggle/ -v 3
```

---

## Adding a New E2E Test

### 1. Create the test directory

```bash
mkdir test/e2e/my-new-test/
```

### 2. Create the test definition

```yaml
# test/e2e/my-new-test/chainsaw-test.yaml
apiVersion: chainsaw.kyverno.io/v1alpha1
kind: Test
metadata:
  name: my-new-test
spec:
  description: >
    Verify that <feature> works end-to-end (REQ-XXX).
  steps:
    - name: create-memcached-cr
      try:
        - apply:
            file: 00-memcached.yaml
    - name: assert-expected-state
      try:
        - assert:
            file: 01-assert-result.yaml
```

### 3. Create resource and assertion files

Use the naming convention:
- `00-*.yaml` — Initial resource to apply
- `01-assert-*.yaml` — Assertions on initial state
- `02-patch-*.yaml` — Patches to modify state
- `03-assert-*.yaml` — Assertions on modified state
- `0N-error-*-gone.yaml` — Negative assertions (resource should not exist)

### 4. Follow conventions

- Use partial objects in assertions — only specify fields you care about
- Use the standard label set: `app.kubernetes.io/name`, `app.kubernetes.io/instance`, `app.kubernetes.io/managed-by`
- Reference shared fixtures from `test/e2e/resources/` when the minimal CR template applies
- For webhook rejection tests, use `expect` with `($error != null): true` on `apply`
- For deletion tests, use `error` operations with resource references

### 5. Run the test

```bash
# Run all E2E tests
make test-e2e

# Run a specific test directory
$(LOCALBIN)/chainsaw test --test-dir test/e2e/my-new-test/
```
