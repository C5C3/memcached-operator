# memcached-operator

A Helm chart for the Memcached Kubernetes Operator (`memcached.c5c3.io`).

## Prerequisites

- Kubernetes 1.26+
- Helm 3.8+
- [cert-manager](https://cert-manager.io/) (required when `webhook.enabled=true` and `certmanager.enabled=true`, which is the default)
- Prometheus Operator CRDs (required when `serviceMonitor.enabled=true`)

## Install

Install the chart from the GHCR OCI registry:

```bash
helm install memcached-operator oci://ghcr.io/c5c3/charts/memcached-operator --version 0.2.0
```

To install into a specific namespace:

```bash
helm install memcached-operator oci://ghcr.io/c5c3/charts/memcached-operator \
  --version 0.2.0 \
  --namespace memcached-system \
  --create-namespace
```

### Download chart locally

```bash
helm pull oci://ghcr.io/c5c3/charts/memcached-operator --version 0.2.0
```

This downloads `memcached-operator-0.2.0.tgz` to your current directory. Extract and inspect with:

```bash
tar xzf memcached-operator-0.2.0.tgz
```

## Configuration

Override values with `--set` or provide a values file with `-f`:

```bash
helm install memcached-operator oci://ghcr.io/c5c3/charts/memcached-operator \
  --version 0.2.0 \
  --set image.tag=v0.2.0 \
  --set replicaCount=2 \
  --set webhook.enabled=false
```

### Common values

| Key                      | Default           | Description                             |
|--------------------------|-------------------|-----------------------------------------|
| `replicaCount`           | `1`               | Number of operator replicas             |
| `image.repository`       | `controller`      | Container image repository              |
| `image.tag`              | `""` (appVersion) | Container image tag                     |
| `webhook.enabled`        | `true`            | Enable admission webhooks               |
| `certmanager.enabled`    | `true`            | Enable cert-manager for webhook TLS     |
| `serviceMonitor.enabled` | `false`           | Enable Prometheus ServiceMonitor        |
| `networkPolicy.enabled`  | `true`            | Enable NetworkPolicy                    |
| `rbac.create`            | `true`            | Create RBAC resources                   |
| `leaderElection.enabled` | `true`            | Enable leader election for HA           |
| `watchNamespaces`        | `[]`              | Namespaces to watch (empty = all)       |
| `crds.managedByHelm`     | `false`           | Manage CRD lifecycle via Helm templates |

See [values.yaml](values.yaml) for the full list of configurable values.

## Upgrading

### Back up Memcached custom resources

Before upgrading, back up your Memcached custom resources:

```bash
kubectl get memcacheds -A -o yaml > memcacheds-backup.yaml
```

### Upgrade the chart

```bash
helm upgrade memcached-operator oci://ghcr.io/c5c3/charts/memcached-operator --version <new-version>
```

### CRD upgrades

Helm does not upgrade CRDs that are placed in the `crds/` directory. This chart supports two CRD management strategies.

#### Default strategy: `crds/` directory (install-only)

By default (`crds.managedByHelm=false`), the Memcached CRD is installed from the `crds/` directory on the first `helm install`. Subsequent `helm upgrade` commands will **not** update the CRD.

To manually upgrade the CRD:

```bash
kubectl apply -f https://raw.githubusercontent.com/c5c3/memcached-operator/main/config/crd/bases/memcached.c5c3.io_memcacheds.yaml
```

#### Helm-managed strategy: `crds.managedByHelm=true`

When `crds.managedByHelm=true`, the CRD is rendered as a Helm template and will be upgraded automatically on `helm upgrade`:

```bash
helm upgrade memcached-operator oci://ghcr.io/c5c3/charts/memcached-operator \
  --version <new-version> \
  --set crds.managedByHelm=true
```

> **Note:** Switching from the default strategy to Helm-managed requires adopting the existing CRD into Helm's management. See the [Helm documentation on CRDs](https://helm.sh/docs/chart_best_practices/custom_resource_definitions/) for details.

## Uninstall

```bash
helm uninstall memcached-operator
```

> **Note:** Uninstalling the chart does not remove CRDs or Memcached custom resources. To remove the CRD and all Memcached resources:
>
> ```bash
> kubectl delete crd memcacheds.memcached.c5c3.io
> ```
