# Chainsaw E2E Tests

Reference documentation for the Kyverno Chainsaw end-to-end test suite that
validates the Memcached operator against a real kind cluster, covering
deployment, scaling, configuration changes, monitoring, PDB management,
graceful rolling updates, webhook validation, garbage collection, SASL
authentication, TLS encryption, mutual TLS (mTLS), NetworkPolicy lifecycle,
and Service annotation propagation.

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

| Prerequisite           | Purpose                               | Setup Command                                                                                               |
|------------------------|---------------------------------------|-------------------------------------------------------------------------------------------------------------|
| kind cluster running   | Target cluster for tests              | `kind create cluster`                                                                                       |
| Operator deployed      | Controller manager running in cluster | `make deploy IMG=<image>`                                                                                   |
| cert-manager installed | Webhook TLS certificates              | `kubectl apply -f https://github.com/cert-manager/cert-manager/releases/download/v1.16.1/cert-manager.yaml` |
| ServiceMonitor CRD     | Required for monitoring-toggle test   | Install via Prometheus Operator CRDs                                                                        |

### Shared Test Fixtures (`test/e2e/resources/`)

Reusable YAML templates referenced by multiple test scenarios:

| File                           | Purpose                                                                     |
|--------------------------------|-----------------------------------------------------------------------------|
| `memcached-minimal.yaml`       | Minimal valid Memcached CR (1 replica, memcached:1.6, 64Mi maxMemoryMB)     |
| `assert-deployment.yaml`       | Partial Deployment assertion (labels, replicas, container args, port)       |
| `assert-service.yaml`          | Partial headless Service assertion (clusterIP: None, port 11211, selectors) |
| `assert-status-available.yaml` | Status assertion (readyReplicas: 1, Available=True)                         |

---

## File Structure

```text
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
├── sasl-authentication/
│   ├── chainsaw-test.yaml          # Test definition
│   ├── 00-sasl-secret.yaml         # Opaque Secret with password-file key
│   ├── 00-memcached.yaml           # CR with security.sasl.enabled: true
│   ├── 01-assert-deployment.yaml   # SASL volume, mount, and args assertions
│   └── 02-assert-status.yaml       # Status condition assertions
├── tls-encryption/
│   ├── chainsaw-test.yaml          # Test definition
│   ├── 00-cert-manager.yaml        # Self-signed Issuer + Certificate
│   ├── 00-assert-certificate-ready.yaml  # Certificate Ready=True assertion
│   ├── 01-memcached.yaml           # CR with security.tls.enabled: true
│   ├── 02-assert-deployment.yaml   # TLS volume, mount, args, port assertions
│   ├── 02-assert-service.yaml      # Service TLS port assertion
│   └── 03-assert-status.yaml       # Status condition assertions
├── tls-mtls/
│   ├── chainsaw-test.yaml          # Test definition
│   ├── 00-cert-manager.yaml        # Self-signed Issuer + Certificate (with CA)
│   ├── 00-assert-certificate-ready.yaml  # Certificate Ready=True assertion
│   ├── 01-memcached.yaml           # CR with tls.enabled + enableClientCert
│   ├── 02-assert-deployment.yaml   # mTLS volume (ca.crt), args (ssl_ca_cert)
│   ├── 02-assert-service.yaml      # Service TLS port assertion
│   └── 03-assert-status.yaml       # Status condition assertions
├── network-policy/
│   ├── chainsaw-test.yaml          # Test definition
│   ├── 00-memcached.yaml           # CR with networkPolicy.enabled: true
│   ├── 01-assert-deployment.yaml   # Deployment ready assertion
│   ├── 01-assert-networkpolicy.yaml # NetworkPolicy with podSelector, port 11211
│   ├── 02-patch-allowed-sources.yaml # Patch allowedSources with podSelector
│   ├── 03-assert-networkpolicy-allowed-sources.yaml # NetworkPolicy with from peer
│   ├── 04-cert-manager.yaml        # Self-signed Issuer + Certificate
│   ├── 04-assert-certificate-ready.yaml # Certificate Ready=True assertion
│   ├── 05-patch-enable-tls-monitoring.yaml # Enable TLS and monitoring
│   ├── 06-assert-networkpolicy-all-ports.yaml # Ports 11211, 11212, 9150
│   ├── 07-patch-disable-networkpolicy.yaml # Disable networkPolicy
│   └── 08-error-networkpolicy-gone.yaml # NetworkPolicy deleted assertion
├── service-annotations/
│   ├── chainsaw-test.yaml          # Test definition
│   ├── 00-memcached.yaml           # CR with service.annotations
│   ├── 01-assert-service.yaml      # Service with custom annotations
│   ├── 02-patch-update-annotations.yaml # Patch with new annotations
│   ├── 03-assert-service-updated.yaml # Service with updated annotations
│   ├── 04-patch-remove-annotations.yaml # Remove annotations (service: null)
│   └── 05-assert-service-no-annotations.yaml # Service without annotations
├── pdb-max-unavailable/
│   ├── chainsaw-test.yaml          # Test definition
│   ├── 00-memcached.yaml           # CR with PDB maxUnavailable=1 (replicas=3)
│   ├── 01-assert-deployment.yaml   # Deployment ready assertion
│   ├── 01-assert-pdb.yaml          # PDB with maxUnavailable=1
│   ├── 02-patch-max-unavailable.yaml # Patch maxUnavailable to 2
│   └── 03-assert-pdb-updated.yaml  # PDB with maxUnavailable=2
├── verbosity-extra-args/
│   ├── chainsaw-test.yaml          # Test definition
│   ├── 00-memcached.yaml           # CR with verbosity=1 and extraArgs
│   ├── 01-assert-deployment.yaml   # Args with -v and -o modern
│   ├── 02-patch-config.yaml        # Patch verbosity=2, new extraArgs
│   └── 03-assert-deployment.yaml   # Args with -vv and new extraArgs
├── custom-exporter-image/
│   ├── chainsaw-test.yaml          # Test definition
│   ├── 00-memcached.yaml           # CR with custom exporterImage
│   ├── 01-assert-deployment.yaml   # Exporter with custom image
│   ├── 02-patch-exporter-image.yaml # Patch to default exporter image
│   └── 03-assert-deployment.yaml   # Exporter with updated image
├── security-contexts/
│   ├── chainsaw-test.yaml          # Test definition
│   ├── 00-memcached.yaml           # CR with pod and container security contexts
│   ├── 01-assert-deployment.yaml   # Security contexts on pod and container
│   ├── 02-patch-security-contexts.yaml # Patch with runAsUser=1000
│   └── 03-assert-deployment.yaml   # Updated security contexts
├── hard-anti-affinity/
│   ├── chainsaw-test.yaml          # Test definition
│   ├── 00-memcached.yaml           # CR with antiAffinityPreset=hard
│   └── 01-assert-deployment.yaml   # requiredDuringScheduling anti-affinity
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

| Step                      | Operation                          | Assertion                                                                     |
|---------------------------|------------------------------------|-------------------------------------------------------------------------------|
| create-memcached-cr       | `apply` 00-memcached.yaml          | CR created                                                                    |
| assert-deployment-created | `assert` 01-assert-deployment.yaml | Deployment with correct labels, args (`-m 64 -c 1024 -t 4 -I 1m`), port 11211 |
| assert-service-created    | `assert` 01-assert-service.yaml    | Headless Service (clusterIP: None), port 11211, correct selectors             |
| assert-status-available   | `assert` 02-assert-status.yaml     | readyReplicas: 1, Available=True                                              |

Owner references on Deployment and Service are verified as part of the
Deployment and Service assertion files (Chainsaw partial matching includes
`metadata.ownerReferences`).

### 2. Scaling (REQ-003)

**Directory**: `test/e2e/scaling/`

Verifies that updating `spec.replicas` scales the Deployment and updates
`status.readyReplicas`.

| Step                      | Operation          | Assertion                  |
|---------------------------|--------------------|----------------------------|
| create-memcached-cr       | `apply`            | CR with replicas=1         |
| assert-initial-deployment | `assert`           | Deployment.spec.replicas=1 |
| scale-up-to-3             | `patch` replicas=3 | —                          |
| assert-scaled-deployment  | `assert`           | Deployment.spec.replicas=3 |
| assert-scaled-status      | `assert`           | status.readyReplicas=3     |
| scale-down-to-1           | `patch` replicas=1 | —                          |
| assert-scaled-down        | `assert`           | Deployment.spec.replicas=1 |

### 3. Configuration Changes (REQ-004)

**Directory**: `test/e2e/configuration-changes/`

Verifies that changing memcached config fields triggers a rolling update with
correct container args.

| Step                 | Operation                                          | Assertion                                  |
|----------------------|----------------------------------------------------|--------------------------------------------|
| create-memcached-cr  | `apply`                                            | CR with maxMemoryMB=64, threads=4          |
| assert-initial-args  | `assert`                                           | Container args: `-m 64 -c 1024 -t 4 -I 1m` |
| update-configuration | `patch` maxMemoryMB=256, threads=8, maxItemSize=2m | —                                          |
| assert-updated-args  | `assert`                                           | Container args: `-m 256 ... -t 8 -I 2m`    |

### 4. Monitoring Toggle (REQ-005)

**Directory**: `test/e2e/monitoring-toggle/`

Verifies that enabling monitoring injects the exporter sidecar and adds a
metrics port to the Service.

| Step                                | Operation                        | Assertion                                                   |
|-------------------------------------|----------------------------------|-------------------------------------------------------------|
| create-memcached-without-monitoring | `apply`                          | CR without monitoring                                       |
| assert-no-exporter-sidecar          | `assert`                         | Deployment has 1 container (memcached only)                 |
| enable-monitoring                   | `patch` monitoring.enabled=true  | —                                                           |
| assert-exporter-sidecar-injected    | `assert`                         | 2 containers: memcached (port 11211) + exporter (port 9150) |
| assert-service-metrics-port         | `assert`                         | Service has metrics port                                    |
| assert-servicemonitor-created       | `assert`                         | ServiceMonitor with correct labels, endpoints, and selector |
| disable-monitoring                  | `patch` monitoring.enabled=false | —                                                           |
| assert-exporter-sidecar-removed     | `assert`                         | Deployment has 1 container (memcached only)                 |
| assert-servicemonitor-deleted       | `error`                          | ServiceMonitor is removed                                   |

**Prerequisite**: ServiceMonitor CRD must be installed in the cluster.

### 5. PDB Creation (REQ-006)

**Directory**: `test/e2e/pdb-creation/`

Verifies that enabling PDB creates a PodDisruptionBudget with correct settings.

| Step                      | Operation                 | Assertion                                                  |
|---------------------------|---------------------------|------------------------------------------------------------|
| create-memcached-with-pdb | `apply`                   | CR with replicas=3, PDB enabled, minAvailable=1            |
| assert-deployment-ready   | `assert`                  | Deployment with 3 replicas                                 |
| assert-pdb-created        | `assert`                  | PDB with minAvailable=1, correct selector, owner reference |
| disable-pdb               | `patch` PDB enabled=false | —                                                          |
| assert-pdb-deleted        | `error`                   | PDB is removed                                             |

### 6. Graceful Rolling Update (REQ-007)

**Directory**: `test/e2e/graceful-rolling-update/`

Verifies that graceful shutdown configures preStop hooks and the RollingUpdate
strategy, and that image changes trigger a correct rolling update.

| Step                                    | Operation     | Assertion                                                                                 |
|-----------------------------------------|---------------|-------------------------------------------------------------------------------------------|
| create-memcached-with-graceful-shutdown | `apply`       | CR with gracefulShutdown enabled                                                          |
| assert-graceful-shutdown-config         | `assert`      | RollingUpdate (maxSurge=1, maxUnavailable=0), preStop hook, terminationGracePeriodSeconds |
| trigger-rolling-update                  | `patch` image | —                                                                                         |
| assert-rolling-update-strategy          | `assert`      | All pods running new image, strategy preserved                                            |

### 7. Webhook Rejection (REQ-008)

**Directory**: `test/e2e/webhook-rejection/`

Verifies that the validating webhook rejects invalid CRs. Each step uses
Chainsaw's `expect` with `($error != null): true` to assert that the `apply`
operation fails.

| Step                                    | Invalid CR                                           | Expected Rejection Reason                     |
|-----------------------------------------|------------------------------------------------------|-----------------------------------------------|
| reject-insufficient-memory-limit        | maxMemoryMB=64, memory limit=32Mi                    | Memory limit < maxMemoryMB + 32Mi overhead    |
| reject-pdb-mutual-exclusivity           | Both minAvailable and maxUnavailable set             | Mutually exclusive fields                     |
| reject-graceful-shutdown-invalid-period | terminationGracePeriodSeconds <= preStopDelaySeconds | Termination period must exceed pre-stop delay |
| reject-sasl-without-secret-ref          | sasl.enabled=true, no credentialsSecretRef.name      | Missing required secret reference             |
| reject-tls-without-secret-ref           | tls.enabled=true, no certificateSecretRef.name       | Missing required secret reference             |

### 8. CR Deletion & Garbage Collection (REQ-009)

**Directory**: `test/e2e/cr-deletion/`

Verifies that deleting a Memcached CR garbage-collects all owned resources.

| Step                                   | Operation                        | Assertion                                                     |
|----------------------------------------|----------------------------------|---------------------------------------------------------------|
| create-memcached-with-all-resources    | `apply`                          | CR with monitoring and PDB enabled                            |
| assert-all-resources-exist             | `assert`                         | Deployment, Service, PDB, ServiceMonitor all present          |
| delete-memcached-cr                    | `delete` Memcached/test-deletion | —                                                             |
| assert-all-resources-garbage-collected | `error`                          | Deployment, Service, PDB, ServiceMonitor, and CR are all gone |

The `error` operation asserts that the specified resource does **not** exist —
the assertion succeeds when the GET returns `NotFound`.

### 9. SASL Authentication (MO-0032 REQ-001, REQ-006, REQ-008)

**Directory**: `test/e2e/sasl-authentication/`

Verifies that enabling SASL authentication creates the correct Secret volume,
volumeMount, and container args (`-Y <authfile>`) in the Deployment.

| Step                   | Operation                     | Assertion                                                                                              |
|------------------------|-------------------------------|--------------------------------------------------------------------------------------------------------|
| create-sasl-secret     | `apply` 00-sasl-secret.yaml   | Opaque Secret with `password-file` key created                                                         |
| create-memcached-cr    | `apply` 00-memcached.yaml     | CR with `security.sasl.enabled: true`, `credentialsSecretRef.name: test-sasl-credentials`              |
| assert-deployment-sasl | `assert` 01-assert-deployment | Volume `sasl-credentials` with item `{key: password-file, path: password-file}`, mount at `/etc/memcached/sasl` (readOnly), args include `-Y /etc/memcached/sasl/password-file` |
| assert-status-available | `assert` 02-assert-status    | readyReplicas: 1, Available=True                                                                       |

The SASL Secret must be created **before** the Memcached CR because the
validating webhook requires `credentialsSecretRef.name` to reference an existing
Secret.

**CRD fields tested**:
- `spec.security.sasl.enabled` — Enables SASL authentication
- `spec.security.sasl.credentialsSecretRef.name` — References the Secret containing the password file

### 10. TLS Encryption (MO-0032 REQ-002, REQ-003, REQ-004, REQ-007, REQ-008, REQ-009)

**Directory**: `test/e2e/tls-encryption/`

Verifies that enabling TLS encryption creates a cert-manager Certificate, adds
the TLS volume, volumeMount, `-Z` and `ssl_chain_cert`/`ssl_key` container args,
and configures port 11212 on the Deployment and Service.

| Step                       | Operation                              | Assertion                                                                                         |
|----------------------------|----------------------------------------|---------------------------------------------------------------------------------------------------|
| create-cert-manager-resources | `apply` 00-cert-manager.yaml       | Self-signed Issuer + Certificate (secretName: `test-tls-certs`)                                   |
| assert-certificate-ready   | `assert` 00-assert-certificate-ready   | Certificate status Ready=True                                                                     |
| create-memcached-cr        | `apply` 01-memcached.yaml              | CR with `security.tls.enabled: true`, `certificateSecretRef.name: test-tls-certs`                 |
| assert-deployment-tls      | `assert` 02-assert-deployment          | Volume `tls-certificates` with items `tls.crt`, `tls.key`; mount at `/etc/memcached/tls` (readOnly); args include `-Z -o ssl_chain_cert=... -o ssl_key=...`; port `memcached-tls` on 11212 |
| assert-service-tls-port    | `assert` 02-assert-service             | Service ports include `memcached-tls` on port 11212 targeting `memcached-tls`                     |
| assert-status-available    | `assert` 03-assert-status              | readyReplicas: 1, Available=True                                                                  |

The Certificate must reach `Ready=True` before applying the Memcached CR to
ensure the TLS Secret exists (avoiding a race condition where the operator
cannot mount the Secret volume).

**CRD fields tested**:
- `spec.security.tls.enabled` — Enables TLS encryption
- `spec.security.tls.certificateSecretRef.name` — References the cert-manager Secret

**cert-manager resources**:
- Self-signed `Issuer` (`test-tls-selfsigned`)
- `Certificate` (`test-tls-cert`) generating Secret `test-tls-certs` with `tls.crt` and `tls.key`

### 11. Mutual TLS / mTLS (MO-0032 REQ-004, REQ-005, REQ-008, REQ-009)

**Directory**: `test/e2e/tls-mtls/`

Verifies that enabling TLS with `enableClientCert: true` adds the `ca.crt` key
projection to the TLS volume and the `ssl_ca_cert` arg to the container, in
addition to the standard TLS configuration.

| Step                       | Operation                              | Assertion                                                                                         |
|----------------------------|----------------------------------------|---------------------------------------------------------------------------------------------------|
| create-cert-manager-resources | `apply` 00-cert-manager.yaml       | Self-signed Issuer + Certificate (secretName: `test-mtls-certs`)                                  |
| assert-certificate-ready   | `assert` 00-assert-certificate-ready   | Certificate status Ready=True                                                                     |
| create-memcached-cr        | `apply` 01-memcached.yaml              | CR with `tls.enabled: true`, `enableClientCert: true`, `certificateSecretRef.name: test-mtls-certs` |
| assert-deployment-mtls     | `assert` 02-assert-deployment          | Volume items include `ca.crt` alongside `tls.crt`/`tls.key`; args include `-o ssl_ca_cert=/etc/memcached/tls/ca.crt`; port `memcached-tls` on 11212 |
| assert-service-tls-port    | `assert` 02-assert-service             | Service ports include `memcached-tls` on port 11212                                               |
| assert-status-available    | `assert` 03-assert-status              | readyReplicas: 1, Available=True                                                                  |

The mTLS test extends TLS by verifying that `enableClientCert: true` causes the
operator to project the `ca.crt` key from the Secret and add the
`ssl_ca_cert=/etc/memcached/tls/ca.crt` arg to enable client certificate
verification.

**CRD fields tested**:
- `spec.security.tls.enabled` — Enables TLS encryption
- `spec.security.tls.enableClientCert` — Enables mutual TLS (client cert verification)
- `spec.security.tls.certificateSecretRef.name` — References the cert-manager Secret

**Difference from TLS test**: The TLS volume includes three items (`tls.crt`,
`tls.key`, `ca.crt`) instead of two, and the container args include an
additional `-o ssl_ca_cert=/etc/memcached/tls/ca.crt`.

### 12. NetworkPolicy Lifecycle (MO-0033 REQ-E2E-NP-001 through NP-005)

**Directory**: `test/e2e/network-policy/`

Verifies the full NetworkPolicy lifecycle: creation with correct podSelector and
ingress port 11211, allowedSources propagation, port adaptation when TLS and
monitoring are enabled (11211, 11212, 9150), and deletion when networkPolicy is
disabled.

| Step                              | Operation                                        | Assertion                                                                                         |
|-----------------------------------|--------------------------------------------------|---------------------------------------------------------------------------------------------------|
| create-memcached-with-networkpolicy | `apply` 00-memcached.yaml                      | CR with `security.networkPolicy.enabled: true` (`test-netpol`)                                    |
| assert-deployment-ready           | `assert` 01-assert-deployment.yaml               | Deployment with correct labels                                                                    |
| assert-networkpolicy-created      | `assert` 01-assert-networkpolicy.yaml            | NetworkPolicy with podSelector matching operator labels, policyTypes: [Ingress], port 11211/TCP   |
| patch-allowed-sources             | `patch` 02-patch-allowed-sources.yaml            | Add `allowedSources` with podSelector `app: allowed-client`                                       |
| assert-networkpolicy-allowed-sources | `assert` 03-assert-networkpolicy-allowed-sources.yaml | NetworkPolicy ingress `from` field contains podSelector with `app: allowed-client`             |
| create-cert-manager-resources     | `apply` 04-cert-manager.yaml                     | Self-signed Issuer + Certificate (secretName: `test-netpol-certs`)                                |
| assert-certificate-ready          | `assert` 04-assert-certificate-ready.yaml        | Certificate status Ready=True                                                                     |
| patch-enable-tls-monitoring       | `patch` 05-patch-enable-tls-monitoring.yaml      | Enable TLS (`certificateSecretRef.name: test-netpol-certs`) and monitoring                        |
| assert-networkpolicy-all-ports    | `assert` 06-assert-networkpolicy-all-ports.yaml  | NetworkPolicy ingress ports: 11211/TCP, 11212/TCP, 9150/TCP; `from` peer preserved                |
| disable-networkpolicy             | `patch` 07-patch-disable-networkpolicy.yaml      | Patch `security.networkPolicy.enabled: false`                                                     |
| assert-networkpolicy-deleted      | `error` 08-error-networkpolicy-gone.yaml         | NetworkPolicy resource no longer exists                                                           |

**Prerequisite**: cert-manager must be installed in the cluster (required for the
TLS port adaptation step).

**CRD fields tested**:
- `spec.security.networkPolicy.enabled` — Enables/disables the NetworkPolicy
- `spec.security.networkPolicy.allowedSources` — Configures ingress `from` peers
- `spec.security.tls.enabled` — Adds port 11212 to the NetworkPolicy
- `spec.monitoring.enabled` — Adds port 9150 to the NetworkPolicy

### 13. Service Annotations (MO-0033 REQ-E2E-SA-001, REQ-E2E-SA-002)

**Directory**: `test/e2e/service-annotations/`

Verifies that custom annotations defined in `spec.service.annotations` are
propagated to the managed headless Service, that updating annotations propagates
the changes, and that removing annotations clears them from the Service.

| Step                         | Operation                                | Assertion                                                                                                                 |
|------------------------------|------------------------------------------|---------------------------------------------------------------------------------------------------------------------------|
| create-memcached-with-annotations | `apply` 00-memcached.yaml          | CR with two annotations: `external-dns.alpha.kubernetes.io/hostname` and `service.beta.kubernetes.io/aws-load-balancer-internal` (`test-svc-ann`) |
| assert-service-has-annotations | `assert` 01-assert-service.yaml        | Service has both custom annotations, correct labels, headless (clusterIP: None), port 11211                               |
| update-annotations           | `patch` 02-patch-update-annotations.yaml | Replace annotations with `external-dns.alpha.kubernetes.io/hostname: memcached-updated.example.com` and `prometheus.io/scrape: "true"` |
| assert-service-annotations-updated | `assert` 03-assert-service-updated.yaml | Service annotations contain the updated key-value pairs                                                              |
| remove-annotations           | `patch` 04-patch-remove-annotations.yaml | Patch `spec.service: null` to remove all annotations                                                                      |
| assert-service-no-annotations | `assert` 05-assert-service-no-annotations.yaml | Service has correct labels and spec; JMESPath expression asserts annotations are absent or empty                     |

**CRD fields tested**:
- `spec.service.annotations` — Custom annotations propagated to the managed Service

### 14. PDB maxUnavailable (MO-0034 REQ-001)

**Directory**: `test/e2e/pdb-max-unavailable/`

Verifies that configuring PDB with `maxUnavailable` (instead of `minAvailable`)
creates a PodDisruptionBudget with the correct `maxUnavailable` setting, and that
updating it propagates to the PDB.

| Step                                    | Operation                        | Assertion                                                  |
|-----------------------------------------|----------------------------------|------------------------------------------------------------|
| create-memcached-with-pdb-max-unavailable | `apply` 00-memcached.yaml     | CR with replicas=3, PDB enabled, maxUnavailable=1          |
| assert-deployment-ready                 | `assert` 01-assert-deployment    | Deployment with 3 replicas                                 |
| assert-pdb-max-unavailable              | `assert` 01-assert-pdb          | PDB with maxUnavailable=1, correct selector, labels        |
| update-max-unavailable                  | `patch` maxUnavailable=2         | —                                                          |
| assert-pdb-updated                      | `assert` 03-assert-pdb-updated  | PDB with maxUnavailable=2                                  |

**CRD fields tested**:
- `spec.highAvailability.podDisruptionBudget.enabled` — Enables the PDB
- `spec.highAvailability.podDisruptionBudget.maxUnavailable` — Sets maxUnavailable on the PDB

### 15. Verbosity and Extra Args (MO-0034 REQ-002, REQ-003)

**Directory**: `test/e2e/verbosity-extra-args/`

Verifies that setting `memcached.verbosity` and `memcached.extraArgs` propagates
to the Deployment container args, and that updating them triggers a rolling update
with the correct args.

| Step                                         | Operation                                         | Assertion                                             |
|----------------------------------------------|----------------------------------------------------|-------------------------------------------------------|
| create-memcached-with-verbosity-and-extra-args | `apply` 00-memcached.yaml                       | CR with verbosity=1, extraArgs=["-o", "modern"]       |
| assert-initial-args                          | `assert` 01-assert-deployment                      | Args include `-v -o modern` after standard flags      |
| update-verbosity-and-extra-args              | `patch` verbosity=2, extraArgs=["--max-reqs-per-event", "20"] | —                                          |
| assert-updated-args                          | `assert` 03-assert-deployment                      | Args include `-vv --max-reqs-per-event 20`            |

**CRD fields tested**:
- `spec.memcached.verbosity` — Controls verbosity flag (0=none, 1=-v, 2=-vv)
- `spec.memcached.extraArgs` — Additional command-line arguments appended after standard flags

### 16. Custom Exporter Image (MO-0034 REQ-004)

**Directory**: `test/e2e/custom-exporter-image/`

Verifies that specifying a custom exporter image in the monitoring config uses
that image for the exporter sidecar instead of the default.

| Step                               | Operation                              | Assertion                                                      |
|------------------------------------|----------------------------------------|----------------------------------------------------------------|
| create-memcached-with-custom-exporter | `apply` 00-memcached.yaml          | CR with monitoring enabled, exporterImage=v0.14.0              |
| assert-custom-exporter-image       | `assert` 01-assert-deployment          | Exporter sidecar uses custom image v0.14.0                     |
| update-exporter-image              | `patch` exporterImage=v0.15.4          | —                                                              |
| assert-updated-exporter-image      | `assert` 03-assert-deployment          | Exporter sidecar uses updated image v0.15.4                    |

**CRD fields tested**:
- `spec.monitoring.enabled` — Enables the exporter sidecar
- `spec.monitoring.exporterImage` — Custom image for the exporter sidecar

### 17. Security Contexts (MO-0034 REQ-005, REQ-006)

**Directory**: `test/e2e/security-contexts/`

Verifies that custom pod and container security contexts defined in
`spec.security` are propagated to the Deployment pod template, and that
updating them triggers a rolling update with the new settings.

| Step                                    | Operation                                      | Assertion                                                  |
|-----------------------------------------|------------------------------------------------|------------------------------------------------------------|
| create-memcached-with-security-contexts | `apply` 00-memcached.yaml                     | CR with runAsNonRoot, readOnlyRootFilesystem, drop ALL     |
| assert-security-contexts                | `assert` 01-assert-deployment                  | Pod and container security contexts match CR spec          |
| update-security-contexts                | `patch` runAsUser=1000, fsGroup=1000           | —                                                          |
| assert-updated-security-contexts        | `assert` 03-assert-deployment                  | Updated security contexts with runAsUser=1000              |

**CRD fields tested**:
- `spec.security.podSecurityContext` — Pod-level security context (runAsNonRoot, fsGroup)
- `spec.security.containerSecurityContext` — Container-level security context (readOnlyRootFilesystem, capabilities)

### 18. Hard Anti-Affinity (MO-0034 REQ-007)

**Directory**: `test/e2e/hard-anti-affinity/`

Verifies that setting `antiAffinityPreset` to `"hard"` configures
`requiredDuringSchedulingIgnoredDuringExecution` pod anti-affinity on the
Deployment, with the correct topology key and label selector.

| Step                                      | Operation                  | Assertion                                                                           |
|-------------------------------------------|----------------------------|-------------------------------------------------------------------------------------|
| create-memcached-with-hard-anti-affinity  | `apply` 00-memcached.yaml | CR with antiAffinityPreset="hard"                                                   |
| assert-hard-anti-affinity                 | `assert` 01-assert-deployment | requiredDuringScheduling anti-affinity with topologyKey and instance label selector |

**CRD fields tested**:
- `spec.highAvailability.antiAffinityPreset` — Controls pod anti-affinity ("soft" or "hard")

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

### Prerequisite Resource Ordering (Security Tests)

Security tests require resources to exist before the Memcached CR is applied:

1. **SASL** — The SASL Secret must be created first because the validating
   webhook checks that `credentialsSecretRef.name` references an existing
   Secret. Applying the CR before the Secret causes a webhook rejection.

2. **TLS/mTLS** — cert-manager Issuer and Certificate must be created first,
   and the Certificate must reach `Ready=True` before the CR is applied. This
   ensures the TLS Secret exists so the operator can mount it as a volume.

This is implemented as separate Chainsaw steps with `apply` followed by
`assert` (for the Certificate readiness check) before the CR `apply` step.

### Spec-Level Assertions (Security Tests)

Security tests assert exclusively on Kubernetes resource specs — they do not
verify runtime protocol behavior. This means:

- No test step connects to memcached via TLS or SASL
- All assertions target Deployment spec (volumes, mounts, args, ports), Service
  spec (ports), or CR status (conditions)
- Tests pass in a kind cluster without any memcached client tools
- Tests complete deterministically within the 120s assert timeout

---

## Requirement Coverage Matrix

| REQ-ID  | Requirement                                              | Test Scenario           | Key Assertions                                                                                                                |
|---------|----------------------------------------------------------|-------------------------|-------------------------------------------------------------------------------------------------------------------------------|
| REQ-001 | Chainsaw configuration and Makefile target               | All (infrastructure)    | `.chainsaw.yaml` config, `make test-e2e` target                                                                               |
| REQ-002 | Basic deployment: Deployment, Service, status            | basic-deployment        | Labels, container args, headless Service, Available=True                                                                      |
| REQ-003 | Scaling: replicas up and down                            | scaling                 | Deployment.spec.replicas, status.readyReplicas                                                                                |
| REQ-004 | Configuration changes: container args updated            | configuration-changes   | Args reflect maxMemoryMB, threads, maxItemSize                                                                                |
| REQ-005 | Monitoring toggle: exporter sidecar, ServiceMonitor      | monitoring-toggle       | Container count, port 9150, Service metrics port, ServiceMonitor labels/endpoints, disable removes sidecar and ServiceMonitor |
| REQ-006 | PDB creation and deletion: minAvailable, selector        | pdb-creation            | PDB spec, selector labels, owner reference, disable removes PDB                                                               |
| REQ-007 | Graceful rolling update: strategy, preStop, image update | graceful-rolling-update | maxSurge=1, maxUnavailable=0, preStop hook, new image                                                                         |
| REQ-008 | Webhook rejection: invalid CRs rejected                  | webhook-rejection       | Five invalid CR variants all rejected                                                                                         |
| REQ-009 | CR deletion: garbage collection                          | cr-deletion             | Deployment, Service, PDB, ServiceMonitor, CR all removed                                                                      |
| REQ-010 | Makefile integration                                     | All (infrastructure)    | `make test-e2e` runs `chainsaw test`                                                                                          |

### Security E2E Tests (MO-0032)

| REQ-ID      | Requirement                                                    | Test Scenario       | Key Assertions                                                                                                     |
|-------------|----------------------------------------------------------------|---------------------|--------------------------------------------------------------------------------------------------------------------|
| MO-0032-001 | SASL Secret and CR configuration propagation                   | sasl-authentication | Secret with `password-file` key, CR with `sasl.enabled: true` and `credentialsSecretRef`                           |
| MO-0032-002 | SASL Deployment volume, mount, and args                        | sasl-authentication | Volume `sasl-credentials`, mount at `/etc/memcached/sasl`, args `-Y /etc/memcached/sasl/password-file`             |
| MO-0032-003 | TLS cert-manager Certificate creation                          | tls-encryption      | Self-signed Issuer, Certificate with `Ready=True`, Secret with `tls.crt`/`tls.key`                                |
| MO-0032-004 | TLS Deployment volume, mount, args, and port                   | tls-encryption      | Volume `tls-certificates`, mount at `/etc/memcached/tls`, args `-Z -o ssl_chain_cert -o ssl_key`, port 11212       |
| MO-0032-005 | TLS Service port configuration                                 | tls-encryption      | Service port `memcached-tls` on 11212 targeting `memcached-tls`                                                    |
| MO-0032-006 | mTLS ca.crt volume projection and ssl_ca_cert arg              | tls-mtls            | Volume items include `ca.crt`, args include `-o ssl_ca_cert=/etc/memcached/tls/ca.crt`                             |
| MO-0032-007 | mTLS preserves standard TLS configuration                      | tls-mtls            | All TLS assertions (volume, mount, args, ports) plus `ca.crt` additions                                           |
| MO-0032-008 | Security tests follow Chainsaw conventions                     | All security tests  | Numbered YAML files, apply/assert flow, partial object matching, standard timeouts, `test-{name}` CR naming        |
| MO-0032-009 | Tests are spec-level assertions only (no runtime verification) | All security tests  | Assertions on Deployment spec, Service spec, CR status — no pod logs or protocol connections                       |

### Network & Service E2E Tests (MO-0033)

| REQ-ID           | Requirement                                                        | Test Scenario       | Key Assertions                                                                                                      |
|------------------|--------------------------------------------------------------------|---------------------|---------------------------------------------------------------------------------------------------------------------|
| REQ-E2E-NP-001  | NetworkPolicy creation with podSelector and port 11211             | network-policy      | NetworkPolicy with operator labels, policyTypes: [Ingress], ingress port 11211/TCP                                  |
| REQ-E2E-NP-002  | allowedSources propagation to NetworkPolicy ingress from field     | network-policy      | Ingress `from` contains podSelector with `app: allowed-client`                                                      |
| REQ-E2E-NP-003  | TLS port 11212 added to NetworkPolicy when TLS enabled             | network-policy      | Ingress ports include 11211/TCP, 11212/TCP, 9150/TCP after enabling TLS and monitoring                              |
| REQ-E2E-NP-004  | NetworkPolicy deleted when networkPolicy disabled                  | network-policy      | Error assertion confirms NetworkPolicy no longer exists after disabling                                             |
| REQ-E2E-NP-005  | Monitoring port 9150 added to NetworkPolicy when monitoring enabled | network-policy      | Ingress ports include 9150/TCP alongside 11211/TCP and 11212/TCP                                                    |
| REQ-E2E-SA-001  | Service annotations propagated from CR spec                        | service-annotations | Service metadata.annotations contains custom annotations, labels and headless spec preserved                        |
| REQ-E2E-SA-002  | Service annotations cleared when removed from CR spec              | service-annotations | Service metadata.annotations empty after patching `spec.service: null`, Service spec unchanged                      |
| REQ-E2E-DOC-001 | Documentation updated with new test entries                        | (this document)     | network-policy and service-annotations sections, file structure, requirement coverage matrix                         |

### Deployment Config E2E Tests (MO-0034)

| REQ-ID           | Requirement                                                        | Test Scenario            | Key Assertions                                                                                                      |
|------------------|--------------------------------------------------------------------|--------------------------|---------------------------------------------------------------------------------------------------------------------|
| MO-0034-001      | PDB with maxUnavailable creates correct PDB and supports updates   | pdb-max-unavailable      | PDB with maxUnavailable=1, correct selector/labels; update to maxUnavailable=2 propagates                           |
| MO-0034-002      | Verbosity level propagates to container args (-v, -vv)             | verbosity-extra-args     | Args include `-v` for verbosity=1, `-vv` for verbosity=2, placed after standard flags                              |
| MO-0034-003      | extraArgs appended to container args after standard flags          | verbosity-extra-args     | Args include `-o modern` after standard flags; update to new extraArgs propagates                                   |
| MO-0034-004      | Custom exporter image used for monitoring sidecar                  | custom-exporter-image    | Exporter sidecar uses custom image v0.14.0; update to v0.15.4 propagates                                           |
| MO-0034-005      | Pod security context propagated to Deployment                      | security-contexts        | Pod securityContext with runAsNonRoot, fsGroup; update to runAsUser=1000 propagates                                 |
| MO-0034-006      | Container security context propagated to Deployment                | security-contexts        | Container securityContext with readOnlyRootFilesystem, drop ALL; update propagates                                  |
| MO-0034-007      | Hard anti-affinity creates requiredDuringScheduling affinity       | hard-anti-affinity       | requiredDuringSchedulingIgnoredDuringExecution with topologyKey and instance label selector                          |

---

## Known Limitations

| Limitation                  | Impact                                                   | Mitigation                                                             |
|-----------------------------|----------------------------------------------------------|------------------------------------------------------------------------|
| Pod scheduling time varies  | Assert timeouts may need adjustment in slow CI           | Global assert timeout set to 120s                                      |
| cert-manager required       | Webhook and TLS/mTLS tests fail without cert-manager     | Documented as prerequisite; tests fail clearly with connection refused |
| ServiceMonitor CRD required | monitoring-toggle and cr-deletion tests fail without CRD | Documented as prerequisite; Chainsaw reports clear assertion error     |
| Sequential execution        | Full suite takes longer than parallel execution          | `parallel: 1` avoids resource contention on small clusters             |
| No runtime protocol testing | SASL/TLS/mTLS tests verify Deployment spec, not actual memcached protocol | By design: tests are fast, deterministic, and need no memcached client |
| Certificate issuance delay  | cert-manager may take time to issue certificates in CI   | Explicit `assert-certificate-ready` step waits for Ready=True within 120s |
| No absence assertion for `ssl_ca_cert` in TLS test | Chainsaw partial matching asserts presence of fields but cannot assert absence; the TLS test does not verify that `ssl_ca_cert` is absent when `enableClientCert` is false | The mTLS test provides the complementary positive assertion that `ssl_ca_cert` is present only when `enableClientCert: true`; combined, the two tests confirm correct conditional behavior |
| Annotation removal uses JMESPath absence check | The service-annotations test uses a JMESPath expression to positively assert that annotations are absent or empty after removal | This upgrades confidence over simple field omission: the assertion actively fails if annotations remain on the Service |
| Hard anti-affinity with single-node kind | The hard-anti-affinity test uses replicas=1 to avoid scheduling failures on single-node kind clusters; the test verifies the Deployment spec, not scheduling behavior | The spec-level assertion confirms the operator correctly translates `antiAffinityPreset: hard` to `requiredDuringSchedulingIgnoredDuringExecution` |

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

### TLS/mTLS Certificate not ready

If TLS or mTLS tests fail at the `assert-certificate-ready` step, the
cert-manager Certificate may not have been issued:

```bash
# Check Certificate status in the test namespace
kubectl get certificates -A
kubectl describe certificate test-tls-cert -n <chainsaw-namespace>

# Check cert-manager logs for issuance errors
kubectl logs -n cert-manager deployment/cert-manager -c cert-manager --tail=20

# Verify the Issuer is ready
kubectl get issuers -A
```

Common causes:
- cert-manager pods not yet running (check `kubectl get pods -n cert-manager`)
- cert-manager webhook not ready (self-signed Issuer needs the webhook to validate)
- Namespace mismatch (Chainsaw auto-injects namespaces; the Issuer and Certificate
  must be in the same namespace)

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
