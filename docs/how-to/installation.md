# Installation

This guide covers installing the Memcached Operator on a Kubernetes cluster, verifying the deployment, and creating your first Memcached instance.

## Prerequisites

### Cluster Requirements

- **Kubernetes** v1.28 or later
- **kubectl** configured to communicate with your cluster
- **cert-manager** installed (required for webhook TLS certificate provisioning)
- **Prometheus Operator CRDs** installed (optional, required only if you plan to
  use `ServiceMonitor` resources for monitoring)

#### Install cert-manager

If cert-manager is not yet installed on your cluster, install it before
deploying the operator:

```bash
kubectl apply -f https://github.com/cert-manager/cert-manager/releases/latest/download/cert-manager.yaml
kubectl wait --for=condition=Available deployment --all -n cert-manager --timeout=120s
```

#### Verify prerequisites

```bash
# Check Kubernetes version
kubectl version

# Verify cert-manager is running
kubectl get pods -n cert-manager

# Check if Prometheus Operator CRDs are available (optional)
kubectl get crd servicemonitors.monitoring.coreos.com
```

### Local Development Requirements

If you plan to build the operator from source or run it locally, you also need:

- **Go** 1.25 or later
- **Docker** or **Podman** (for building container images)
- **kind** or **minikube** (for local Kubernetes clusters)
- **Operator SDK** v1.42 or later (optional, for scaffolding changes)

## Install CRDs

Before deploying the operator, install the `Memcached` Custom Resource Definition
into your cluster. Choose one of the following methods:

### Option A: From source (recommended for development)

Clone the repository and use the Makefile target:

```bash
git clone https://github.com/c5c3/memcached-operator.git
cd memcached-operator
make install
```

This runs `kustomize build config/crd | kubectl apply -f -` under the hood,
which generates and applies the CRD manifest from the Kustomize overlay in
`config/crd/`.

### Option B: Using kustomize directly

If you have the repository checked out but prefer to run the command yourself:

```bash
kustomize build config/crd | kubectl apply -f -
```

### Option C: From a release artifact

Apply the CRD manifest directly from a GitHub Release:

```bash
kubectl apply -f https://github.com/c5c3/memcached-operator/releases/download/v0.1.0/memcached.c5c3.io_memcacheds.yaml
```

Replace `v0.1.0` with the version you want to install. Available releases are
listed at
[github.com/c5c3/memcached-operator/releases](https://github.com/c5c3/memcached-operator/releases).

### Verify CRD installation

```bash
kubectl get crd memcacheds.memcached.c5c3.io
```

Expected output:

```text
NAME                            CREATED AT
memcacheds.memcached.c5c3.io    2025-01-15T10:00:00Z
```

## Deploy the Operator

Deploy the operator and all supporting resources to your cluster:

```bash
make deploy IMG=ghcr.io/c5c3/memcached-operator:v0.1.0
```

Replace `v0.1.0` with the desired version tag.

This command performs the following steps:

1. Sets the controller manager image to the specified `IMG` in the Kustomize
   overlay (`config/manager/`).
2. Builds the full manifest from `config/default/` using Kustomize, which
   includes:
   - CRD definitions (`config/crd/`)
   - RBAC resources: ClusterRole, ClusterRoleBinding, ServiceAccount
     (`config/rbac/`)
   - Operator Deployment in the `memcached-operator-system` namespace
     (`config/manager/`)
   - Webhook configurations: MutatingWebhookConfiguration and
     ValidatingWebhookConfiguration (`config/webhook/`)
   - cert-manager Certificate and Issuer for webhook TLS (`config/certmanager/`)
3. Applies all resources to the cluster via `kubectl apply`.

All resources are created under the `memcached-operator-system` namespace with
the `memcached-operator-` name prefix.

## Verify the Installation

After deploying, verify that all components are running correctly.

### Check the operator pod

```bash
kubectl get pods -n memcached-operator-system
```

Expected output:

```text
NAME                                                     READY   STATUS    RESTARTS   AGE
memcached-operator-controller-manager-5b8f4c7d9f-x2k4l   1/1     Running   0          30s
```

Wait until the pod status shows `Running` and `READY` is `1/1`.

### Check the CRD

```bash
kubectl get crd memcacheds.memcached.c5c3.io
```

### Check webhook configurations

```bash
kubectl get mutatingwebhookconfigurations,validatingwebhookconfigurations | grep memcached
```

Expected output:

```text
mutatingwebhookconfiguration.admissionregistration.k8s.io/memcached-operator-mutating-webhook-configuration      1          30s
validatingwebhookconfiguration.admissionregistration.k8s.io/memcached-operator-validating-webhook-configuration   1          30s
```

### Check operator logs

```bash
kubectl logs -n memcached-operator-system deployment/memcached-operator-controller-manager
```

Look for successful startup messages and confirm there are no error-level log
entries. The operator exposes health probes on port 8081 (`/healthz` and
`/readyz`) and serves metrics on port 8443 (`/metrics`).

## Create Your First Memcached Instance

Once the operator is running, create a Memcached instance by applying a custom
resource.

### Minimal example

Create a file named `memcached.yaml`:

```yaml
apiVersion: memcached.c5c3.io/v1alpha1
kind: Memcached
metadata:
  name: my-cache
  namespace: default
spec:
  replicas: 3
  image: "memcached:1.6"
  memcached:
    maxMemoryMB: 64
```

Apply it:

```bash
kubectl apply -f memcached.yaml
```

### Verify the instance

Check the Memcached custom resource:

```bash
kubectl get memcached my-cache
```

Check the managed Deployment and pods:

```bash
kubectl get deployment my-cache
kubectl get pods -l app.kubernetes.io/instance=my-cache
```

Check the headless Service:

```bash
kubectl get service my-cache
```

Inspect the full status, including conditions and metrics:

```bash
kubectl get memcached my-cache -o yaml
```

A healthy instance shows `Available: True` and `Degraded: False` in its status
conditions.

### Using the sample CR

The repository includes a more complete sample CR at
`config/samples/memcached_v1alpha1_memcached.yaml` that demonstrates additional
configuration options including resource limits, high availability, monitoring,
and security contexts:

```bash
kubectl apply -f config/samples/memcached_v1alpha1_memcached.yaml
```

## Local Development Setup

For developing and testing the operator locally without deploying it to a
cluster:

### 1. Install CRDs

```bash
make install
```

### 2. Run the operator locally

The operator runs on your machine and connects to the cluster configured in your
current kubeconfig context:

```bash
make run
```

This compiles and runs the operator binary directly. Webhooks are not active in
this mode because they require TLS certificates provisioned by cert-manager
inside the cluster.

### 3. Apply a sample CR

In a separate terminal:

```bash
kubectl apply -f config/samples/memcached_v1alpha1_memcached.yaml
```

### 4. Observe reconciliation

Watch the operator logs in the first terminal and verify that managed resources
are created:

```bash
kubectl get deployment,service,pdb -l app.kubernetes.io/managed-by=memcached-operator
```

## Single-File Install

For environments where you prefer a single manifest file (such as GitOps
pipelines or air-gapped clusters), you can either download a pre-built manifest
from a GitHub Release or generate one locally.

### Option A: From a GitHub Release (recommended)

Each release includes a ready-to-use `install.yaml`:

```bash
kubectl apply -f https://github.com/c5c3/memcached-operator/releases/download/v0.1.0/install.yaml
```

Replace `v0.1.0` with the desired version.

### Option B: Generate locally

```bash
make build-installer IMG=ghcr.io/c5c3/memcached-operator:v0.1.0
kubectl apply -f dist/install.yaml
```

Both options produce an identical consolidated manifest containing all resources
(CRDs, RBAC, operator Deployment, webhooks, cert-manager resources) in a single
file.

## Uninstallation

### Remove all operator resources

To remove the operator Deployment, RBAC, webhooks, cert-manager resources, and
the CRDs:

```bash
make undeploy
```

This deletes all resources created by `make deploy`, including the
`memcached-operator-system` namespace.

**Warning:** Removing the CRDs also deletes all `Memcached` custom resources and
their managed workloads (Deployments, Services, PDBs, etc.) across all
namespaces. Back up your Memcached CRs before proceeding if you need to preserve
their definitions.

### Remove only CRDs

To remove just the CRDs without removing the operator:

```bash
make uninstall
```

### Manual cleanup

If you installed using the single-file method:

```bash
kubectl delete -f dist/install.yaml
```
