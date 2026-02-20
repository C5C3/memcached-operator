# Upgrade

This guide covers upgrading the Memcached Operator to a new version, including
CRD updates, operator rollout, verification, and rollback procedures.

## Pre-Upgrade Checklist

Before starting an upgrade, complete the following steps:

### 1. Back up existing Memcached custom resources

Export all Memcached CRs across all namespaces:

```bash
kubectl get memcached -A -o yaml > memcached-backup.yaml
```

This backup allows you to restore your Memcached definitions if the upgrade
introduces breaking CRD schema changes.

### 2. Check the current operator version

```bash
kubectl get deployment -n memcached-operator-system memcached-operator-controller-manager \
  -o jsonpath='{.spec.template.spec.containers[0].image}'
```

### 3. Review release notes

Check the release notes for the target version at
`https://github.com/c5c3/memcached-operator/releases`. Pay attention to:

- Breaking changes in the CRD schema (field removals, renames, type changes)
- New required fields or changed default values
- Changes to RBAC permissions
- Changes to webhook behavior
- Minimum Kubernetes version requirements

### 4. Check CRD schema changes

Compare the CRD between your current version and the target version:

```bash
# View the currently installed CRD schema
kubectl get crd memcacheds.memcached.c5c3.io -o yaml > crd-current.yaml

# Compare with the new version (from the release or repository)
diff crd-current.yaml <(kustomize build config/crd)
```

### 5. Verify cluster health

Confirm that the cluster and existing Memcached instances are healthy before
proceeding:

```bash
# Check all Memcached CRs
kubectl get memcached -A

# Check operator pod
kubectl get pods -n memcached-operator-system

# Verify no pending issues in operator logs
kubectl logs -n memcached-operator-system deployment/memcached-operator-controller-manager --tail=50
```

All Memcached CRs should show `Available: True` in their status conditions
before upgrading.

## CRD Update

Update the CRD before upgrading the operator. The new operator version may
depend on CRD schema changes (new fields, updated validation rules, or changed
defaults).

### Apply the updated CRD

From the repository at the target version:

```bash
make install
```

Or apply a specific release artifact:

```bash
kubectl apply -f https://github.com/c5c3/memcached-operator/releases/download/<new-version>/memcached.c5c3.io_memcacheds.yaml
```

### CRD compatibility notes

- CRD updates within the `v1alpha1` API version are generally
  backwards-compatible. New optional fields are added with defaults, and existing
  fields retain their behavior.
- **Field removals or renames** between versions require careful handling. If a
  field is removed, existing CRs that specify that field will continue to work
  (the field is ignored), but you should update your CRs to remove deprecated
  fields.
- **New validation rules** may reject existing CRs on the next update. If an
  existing CR violates a new validation constraint, you must update the CR to
  conform before applying changes to it.
- Since `v1alpha1` is an alpha API, breaking changes are possible between minor
  versions. Always review the release notes.

### Verify CRD update

```bash
kubectl get crd memcacheds.memcached.c5c3.io -o jsonpath='{.metadata.resourceVersion}'
```

Confirm the resource version has changed, indicating the CRD was updated.

## Operator Update

After updating the CRD, deploy the new operator version:

```bash
make deploy IMG=ghcr.io/c5c3/memcached-operator:<new-version>
```

Replace `<new-version>` with the target version tag (e.g., `v0.2.0`).

This command:

1. Updates the image reference in the Kustomize overlay.
2. Rebuilds and applies all manifests from `config/default/`, including any
   updated RBAC rules, webhook configurations, and cert-manager resources.
3. Triggers a rolling update of the controller manager Deployment.

### Monitor the rollout

```bash
kubectl rollout status deployment/memcached-operator-controller-manager \
  -n memcached-operator-system
```

Wait until the rollout completes:

```
deployment "memcached-operator-controller-manager" successfully rolled out
```

### Single-file upgrade

If you use the single-file installation method:

```bash
make build-installer IMG=ghcr.io/c5c3/memcached-operator:<new-version>
kubectl apply -f dist/install.yaml
```

## Post-Upgrade Verification

After the new operator version is running, verify that everything is working
correctly.

### Check the operator pod

```bash
kubectl get pods -n memcached-operator-system
```

Confirm the pod is `Running` with `READY 1/1` and has `RESTARTS 0`.

### Verify the operator version

```bash
kubectl get deployment -n memcached-operator-system memcached-operator-controller-manager \
  -o jsonpath='{.spec.template.spec.containers[0].image}'
```

Confirm it shows the new image tag.

### Check operator logs

```bash
kubectl logs -n memcached-operator-system deployment/memcached-operator-controller-manager --tail=100
```

Look for:

- Successful startup messages
- No error-level log entries
- Reconciliation activity resuming for existing Memcached CRs

### Verify all Memcached instances are reconciled

```bash
kubectl get memcached -A
```

All instances should show the expected `READY` count. Inspect individual
instances for healthy status conditions:

```bash
kubectl get memcached <name> -n <namespace> -o jsonpath='{.status.conditions}' | jq .
```

Confirm:

- `Available` is `True`
- `Degraded` is `False`
- `Progressing` is `False`

### Verify managed resources

Check that all managed resources are present and healthy:

```bash
# Deployments
kubectl get deployments -l app.kubernetes.io/managed-by=memcached-operator -A

# Services
kubectl get services -l app.kubernetes.io/managed-by=memcached-operator -A

# PodDisruptionBudgets (if HA is enabled)
kubectl get pdb -l app.kubernetes.io/managed-by=memcached-operator -A

# ServiceMonitors (if monitoring is enabled)
kubectl get servicemonitor -l app.kubernetes.io/managed-by=memcached-operator -A
```

### Check webhooks

```bash
kubectl get mutatingwebhookconfigurations,validatingwebhookconfigurations | grep memcached
```

Verify the webhook configurations are present and that creating or updating a
Memcached CR succeeds without admission errors.

## Rollback

If the upgrade causes issues, roll back to the previous operator version.

### Redeploy the previous version

```bash
make deploy IMG=ghcr.io/c5c3/memcached-operator:<previous-version>
```

Replace `<previous-version>` with the version you were running before the
upgrade.

### Restore CRDs if needed

If the CRD was updated and the rollback requires the old schema:

```bash
# From the previous version's branch or release:
git checkout <previous-version>
make install
```

Or apply the CRD from the previous release:

```bash
kubectl apply -f https://github.com/c5c3/memcached-operator/releases/download/<previous-version>/memcached.c5c3.io_memcacheds.yaml
```

### Restore Memcached CRs if needed

If CRs were modified or deleted during the upgrade, restore them from backup:

```bash
kubectl apply -f memcached-backup.yaml
```

### Verify the rollback

Follow the same post-upgrade verification steps to confirm the previous version
is running correctly and all Memcached instances are healthy.

## Version Compatibility Matrix

| Operator Version | Kubernetes Version | CRD API Version | cert-manager Version | Notes                    |
|------------------|--------------------|-----------------|----------------------|--------------------------|
| v0.1.0           | v1.28 -- v1.32     | v1alpha1        | v1.12+               | Initial release          |

Check the release notes for each version to confirm compatibility with your
Kubernetes cluster version. The operator is tested against the Kubernetes
versions listed in this matrix.

## Common Upgrade Issues

### Webhook certificate renewal

**Symptom:** After upgrading, creating or updating Memcached CRs fails with TLS
errors such as `x509: certificate signed by unknown authority` or webhook
connection refused.

**Cause:** cert-manager may need to reissue the webhook serving certificate after
the upgrade. This typically resolves itself within a few minutes as cert-manager
detects the new webhook Service and renews the certificate.

**Resolution:**

```bash
# Check the certificate status
kubectl get certificate -n memcached-operator-system

# Check cert-manager logs if the certificate is not ready
kubectl logs -n cert-manager deployment/cert-manager --tail=50

# Force certificate renewal if needed
kubectl delete secret -n memcached-operator-system memcached-operator-webhook-server-cert
```

Deleting the Secret triggers cert-manager to re-create it with a fresh
certificate.

### CRD validation changes rejecting existing CRs

**Symptom:** Existing Memcached CRs cannot be updated. `kubectl apply` returns
validation errors for fields that were previously accepted.

**Cause:** The new CRD version introduced stricter validation rules (tighter
ranges, new required fields, changed patterns).

**Resolution:**

1. Identify the validation error from the `kubectl apply` output.
2. Update the affected CRs to conform to the new validation rules.
3. If the change is unintentional, file an issue and roll back to the previous
   CRD version.

Note: Existing CRs that are already stored in etcd are not re-validated until
they are next updated. They continue to function, but you cannot modify them
until they conform to the new schema.

### RBAC permission changes

**Symptom:** The operator logs show `Forbidden` errors after upgrading, and
reconciliation stops for some or all resources.

**Cause:** The new operator version requires additional RBAC permissions that
were not present in the previous version.

**Resolution:**

Ensure you ran `make deploy` (not just updated the Deployment image), as
`make deploy` applies the full Kustomize overlay including updated RBAC
manifests. If you upgraded only the container image, apply the RBAC changes
separately:

```bash
kustomize build config/rbac | kubectl apply -f -
```

### Operator pod crash-looping after upgrade

**Symptom:** The new operator pod enters `CrashLoopBackOff` immediately after
deployment.

**Cause:** Common causes include missing CRDs, missing RBAC permissions, or
incompatible Kubernetes API versions.

**Resolution:**

```bash
# Check the pod logs for the specific error
kubectl logs -n memcached-operator-system deployment/memcached-operator-controller-manager

# Ensure CRDs are installed for the new version
make install

# Ensure RBAC is up to date
kustomize build config/default | kubectl apply -f -
```

### Reconciliation lag after upgrade

**Symptom:** After upgrading, existing Memcached instances take several minutes
to reconcile.

**Cause:** This is expected behavior. The new controller manager starts with
empty caches and must re-list and re-watch all relevant resources. The
reconciliation queue processes all existing Memcached CRs, which takes time
proportional to the number of instances.

**Resolution:** No action needed. Monitor the operator logs and wait for all
instances to be reconciled. The operator requeues reconciliation after 10 seconds
for instances that are not yet ready, and after 60 seconds for healthy instances.
