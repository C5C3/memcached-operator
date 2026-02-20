# Memcached Operator Documentation

The **Memcached Operator** is a Kubernetes operator for managing Memcached clusters, built with the
[Operator SDK](https://sdk.operatorframework.io/) and [controller-runtime](https://github.com/kubernetes-sigs/controller-runtime).
It is part of the [CobaltCore (C5C3)](https://github.com/c5c3/c5c3) ecosystem --
a Kubernetes-native OpenStack distribution for operating Hosted Control Planes on bare-metal infrastructure.

The operator automates the deployment, configuration, and lifecycle management of Memcached instances on Kubernetes. Users declare their desired Memcached topology via a Custom Resource, and the operator reconciles the underlying Deployments, Services, PodDisruptionBudgets, ServiceMonitors, and NetworkPolicies.

## Capabilities

- Declarative management of Memcached clusters via a Custom Resource
- Automated creation and reconciliation of Deployments, Services, PodDisruptionBudgets, ServiceMonitors, and NetworkPolicies
- Memcached configuration through CRD fields (memory, connections, threads, item size, verbosity)
- Built-in Prometheus monitoring via a `memcached-exporter` sidecar
- High-availability primitives: pod anti-affinity, topology spread constraints, PodDisruptionBudgets, graceful shutdown
- Security: least-privilege RBAC, pod security contexts, optional SASL authentication, optional TLS encryption, NetworkPolicy generation
- Validation and defaulting webhooks

## Tech Stack

| Component           | Technology                        |
|---------------------|-----------------------------------|
| Language            | Go 1.24+                          |
| Scaffolding         | Operator SDK / Kubebuilder        |
| Runtime             | controller-runtime                |
| CRD API group       | `memcached.c5c3.io`               |
| Initial API version | `v1alpha1`                        |
| Memcached image     | `memcached:1.6`                   |
| Exporter image      | `prom/memcached-exporter:v0.15.4` |
| Testing             | envtest, Chainsaw                 |

---

## Documentation

### Explanation

Background knowledge and architectural context for the operator.

| Document                                                      | Description                                                                           |
|---------------------------------------------------------------|---------------------------------------------------------------------------------------|
| [Architecture Overview](explanation/architecture-overview.md) | Operator architecture, reconciliation loop, design principles, and CobaltCore context |

### How-To Guides

Step-by-step instructions for common tasks.

| Document                                     | Description                                              |
|----------------------------------------------|----------------------------------------------------------|
| [Installation](how-to/installation.md)       | Install the operator and its prerequisites               |
| [Upgrade](how-to/upgrade.md)                 | Upgrade the operator to a new version                    |
| [Troubleshooting](how-to/troubleshooting.md) | Diagnose and resolve common issues                       |
| [Examples](how-to/examples.md)               | Example Memcached CR configurations for common scenarios |

### Reference

Detailed technical reference material.

| Document                                                         | Description                                                                                                   |
|------------------------------------------------------------------|---------------------------------------------------------------------------------------------------------------|
| [CRD Reference](reference/crd-reference.md)                      | Complete field reference for the Memcached Custom Resource Definition                                         |
| [Backend Reference Docs](reference/backend/project-structure.md) | Detailed per-feature implementation references covering reconciliation, webhooks, testing, security, and more |

The `reference/backend/` directory contains in-depth implementation references for individual features, including:

- [Deployment Reconciliation](reference/backend/deployment-reconciliation.md)
- [Headless Service Reconciliation](reference/backend/headless-service-reconciliation.md)
- [PDB Reconciliation](reference/backend/pdb-reconciliation.md)
- [ServiceMonitor Reconciliation](reference/backend/servicemonitor-reconciliation.md)
- [NetworkPolicy Reconciliation](reference/backend/networkpolicy-reconciliation.md)
- [Idempotent Create-or-Update Pattern](reference/backend/idempotent-create-or-update.md)
- [CRD Generation and Registration](reference/backend/crd-generation-registration.md)
- [Memcached CRD Types](reference/backend/memcached-crd-types.md)
- [Validation Webhook](reference/backend/validation-webhook.md)
- [Defaulting Webhook](reference/backend/defaulting-webhook.md)
- [Webhook Tests](reference/backend/webhook-tests.md)
- [Webhook Certificate Management](reference/backend/webhook-certificate-management.md)
- [Reconciler Scaffold and Watches](reference/backend/reconciler-scaffold-watches.md)
- [Status Conditions and ObservedGeneration](reference/backend/status-conditions-observedgeneration.md)
- [Operator Metrics Server](reference/backend/operator-metrics-server.md)
- [Exporter Sidecar Injection](reference/backend/exporter-sidecar-injection.md)
- [Pod Anti-Affinity Presets](reference/backend/pod-anti-affinity-presets.md)
- [Topology Spread Constraints](reference/backend/topology-spread-constraints.md)
- [Graceful Shutdown](reference/backend/graceful-shutdown.md)
- [Pod Security Contexts](reference/backend/pod-security-contexts.md)
- [TLS Encryption](reference/backend/tls-encryption.md)
- [Operator RBAC (Least Privilege)](reference/backend/operator-rbac-least-privilege.md)
- [envtest Integration Tests](reference/backend/envtest-integration-tests.md)
- [Chainsaw E2E Tests](reference/backend/chainsaw-e2e-tests.md)
- [CI Pipeline](reference/backend/ci-pipeline.md)
- [Multi-Stage Dockerfile](reference/backend/multi-stage-dockerfile.md)
- [Kustomize Deployment Manifests](reference/backend/kustomize-deployment-manifests.md)
- [Project Structure](reference/backend/project-structure.md)
