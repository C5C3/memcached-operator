# Memcached Operator

[![CI](https://github.com/c5c3/memcached-operator/actions/workflows/ci.yml/badge.svg)](https://github.com/c5c3/memcached-operator/actions/workflows/ci.yml)
[![Release](https://github.com/c5c3/memcached-operator/actions/workflows/release.yml/badge.svg)](https://github.com/c5c3/memcached-operator/actions/workflows/release.yml)

A Kubernetes operator for managing Memcached clusters, built with
[Operator SDK](https://sdk.operatorframework.io/) (Go) and
[controller-runtime](https://github.com/kubernetes-sigs/controller-runtime).

Container images are published to GHCR:

```text
ghcr.io/c5c3/memcached-operator:latest
```

Part of the [CobaltCore (C5C3)](https://github.com/c5c3/c5c3) ecosystem — a
Kubernetes-native OpenStack distribution for operating Hosted Control Planes on
bare-metal infrastructure.

## Documentation

Full documentation is available at
**<https://c5c3.github.io/memcached-operator/>** and covers:

- [Architecture Overview](https://c5c3.github.io/memcached-operator/explanation/architecture-overview) — design principles, reconciliation loop, CobaltCore context
- [Installation](https://c5c3.github.io/memcached-operator/how-to/installation) — prerequisites, local development, cluster deployment
- [CRD Reference](https://c5c3.github.io/memcached-operator/reference/crd-reference) — complete field reference for the `Memcached` custom resource
- [Examples](https://c5c3.github.io/memcached-operator/how-to/examples) — basic, HA, TLS, and production configurations
- [Troubleshooting](https://c5c3.github.io/memcached-operator/how-to/troubleshooting) — common issues and diagnostics

## Quick Start

```bash
# Install CRDs
make install

# Run the operator locally (connects to current kubeconfig context)
make run

# In another terminal, create a Memcached instance
kubectl apply -f config/samples/memcached_v1alpha1_memcached.yaml

# Verify
kubectl get memcached
```

See the [Installation guide](https://c5c3.github.io/memcached-operator/how-to/installation)
for cluster deployment and production setup.

## License

[Apache 2.0](LICENSE)
