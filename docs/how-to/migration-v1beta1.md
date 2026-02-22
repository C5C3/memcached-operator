# Migrating from v1alpha1 to v1beta1

This guide covers migrating `Memcached` custom resources from
`memcached.c5c3.io/v1alpha1` to `memcached.c5c3.io/v1beta1`.

## Overview

The operator now serves `v1beta1` as the storage version (hub) and `v1alpha1`
as a deprecated spoke version. Both versions have identical schemas — no fields
were added, removed, or renamed. The Kubernetes API server automatically
converts between versions using the hub/spoke conversion pattern.

| Version   | Role  | Storage | Status     |
|-----------|-------|---------|------------|
| `v1beta1` | Hub   | Yes     | Active     |
| `v1alpha1`| Spoke | No      | Deprecated |

## Do I need to migrate?

**Existing resources work without changes.** The API server stores all objects
in `v1beta1` format internally and converts on the fly when clients request
`v1alpha1`. You can continue using `v1alpha1` manifests for now.

However, you should migrate to `v1beta1` because:

- `v1alpha1` will be marked as deprecated in the CRD served-versions list in a
  future release (Kubernetes 1.28+ clients will see deprecation warnings).
- `v1alpha1` will eventually be removed from served versions entirely.
- New documentation and samples use `v1beta1` exclusively.

## Migration Steps

### 1. Update the apiVersion field

Change `apiVersion` in your manifests from `v1alpha1` to `v1beta1`:

```diff
-apiVersion: memcached.c5c3.io/v1alpha1
+apiVersion: memcached.c5c3.io/v1beta1
 kind: Memcached
 metadata:
   name: my-cache
 spec:
   replicas: 3
```

No other changes are needed. The spec and status schemas are identical between
versions.

### 2. Re-apply the updated manifests

Apply the updated manifests to your cluster:

```bash
kubectl apply -f memcached.yaml
```

The API server accepts the `v1beta1` manifest and stores it directly (no
conversion needed since `v1beta1` is the storage version).

### 3. Verify the migration

Confirm the resource is accessible via `v1beta1`:

```bash
kubectl get memcached.v1beta1.memcached.c5c3.io -A
```

Check that the resource status is healthy:

```bash
kubectl get memcached <name> -n <namespace> -o jsonpath='{.status.conditions}' | jq .
```

Confirm `Available` is `True` and `Degraded` is `False`.

### 4. Update CI/CD pipelines and tooling

Search your repositories for references to `v1alpha1` and update them:

```bash
grep -r "memcached.c5c3.io/v1alpha1" .
```

Common locations to update:

- Kustomize overlays and patches
- Helm value files or templates
- CI/CD pipeline manifests
- ArgoCD Application resources
- Terraform configurations
- Scripts that use `kubectl` with explicit API versions

### 5. Update RBAC rules (if applicable)

If your RBAC rules reference specific API versions (uncommon but possible),
update them. Standard Kubernetes RBAC operates at the API group level
(`memcached.c5c3.io`), not the version level, so most RBAC configurations
require no changes.

## Batch migration

To migrate all `Memcached` resources across all namespaces at once:

```bash
# Export all resources as v1beta1
kubectl get memcached -A -o yaml | \
  sed 's|memcached.c5c3.io/v1alpha1|memcached.c5c3.io/v1beta1|g' \
  > memcached-v1beta1.yaml

# Review the changes
diff <(kubectl get memcached -A -o yaml) memcached-v1beta1.yaml

# Apply the v1beta1 manifests
kubectl apply -f memcached-v1beta1.yaml
```

## Deprecation timeline

| Phase                | Action                                                                                                                    |
|----------------------|---------------------------------------------------------------------------------------------------------------------------|
| Current              | `v1beta1` is the storage version. `v1alpha1` remains served with automatic conversion.                                    |
| Next minor release   | `v1alpha1` is marked `deprecated: true` in the CRD served-versions list. Clients on Kubernetes 1.28+ see warning headers. |
| Future major release | `v1alpha1` is removed from served versions. No data migration needed — objects are already stored as `v1beta1`.           |

## Frequently asked questions

### Will my existing resources break?

No. Existing resources stored in etcd are already in `v1beta1` format (the
storage version). The API server converts them transparently when clients
request `v1alpha1`.

### Are there schema differences between v1alpha1 and v1beta1?

No. The schemas are identical. All fields, defaults, validation rules, and
webhook behavior are the same in both versions.

### Can I use both versions simultaneously?

Yes. The API server serves both versions and converts between them
automatically. A `v1alpha1` manifest applied to the cluster is stored as
`v1beta1` and can be retrieved as either version.

### What happens when v1alpha1 is removed?

Clients requesting `v1alpha1` will receive a `404 Not Found` error. Since all
objects are already stored as `v1beta1`, no data migration is needed. You only
need to update your manifests and tooling to use `v1beta1`.
