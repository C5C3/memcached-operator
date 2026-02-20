# Troubleshooting

This guide covers common failure scenarios when running the Memcached Operator and provides step-by-step diagnosis and resolution procedures.

---

## 1. Memcached CR Stuck in Progressing State

### Symptom

The Memcached custom resource shows `Progressing=True` and `Available=False` for an extended period. The `readyReplicas` count does not reach the desired `replicas`.

```bash
kubectl get memcached <name> -n <namespace>
# Ready column remains lower than Replicas column
```

### Diagnosis

**Check the CR status conditions:**

```bash
kubectl get memcached <name> -n <namespace> -o jsonpath='{.status.conditions}' | jq .
```

Look for the `Progressing` condition message, which reports the rollout state (e.g., `Rollout in progress: 0/3 replicas updated`).

**Check the owned Deployment:**

```bash
kubectl get deployment <name> -n <namespace>
kubectl describe deployment <name> -n <namespace>
```

**Check Pod status:**

```bash
kubectl get pods -n <namespace> -l app.kubernetes.io/name=memcached,app.kubernetes.io/instance=<name>
```

**Check events on the CR and Deployment:**

```bash
kubectl describe memcached <name> -n <namespace>
kubectl get events -n <namespace> --field-selector involvedObject.name=<name> --sort-by='.lastTimestamp'
```

**Check operator logs:**

```bash
kubectl logs -n memcached-operator-system deployment/memcached-operator-controller-manager -c manager
```

### Common Causes and Fixes

**Insufficient cluster resources**

Pods stay in `Pending` state because CPU or memory requests cannot be satisfied.

```bash
kubectl describe pod <pod-name> -n <namespace>
# Look for "Insufficient cpu" or "Insufficient memory" in Events
```

Fix: Reduce resource requests in the CR, add cluster capacity, or scale down other workloads.

```yaml
spec:
  resources:
    requests:
      cpu: 100m      # Lower CPU request
      memory: 128Mi  # Lower memory request
```

**Image pull errors**

Pods show `ImagePullBackOff` or `ErrImagePull`.

```bash
kubectl describe pod <pod-name> -n <namespace>
# Look for "Failed to pull image" in Events
```

Fix: Verify the image name and tag, ensure the image registry is accessible, and check pull secrets if using a private registry.

**Node selector or toleration mismatch**

No nodes match the scheduling constraints.

Fix: Verify that nodes have the required labels and taints. Adjust the pod template or node configuration accordingly.

**Hard anti-affinity with too few nodes**

When `antiAffinityPreset: hard` is set and the cluster has fewer nodes than the requested `replicas`, some pods cannot be scheduled because `requiredDuringSchedulingIgnoredDuringExecution` prevents two Memcached pods from running on the same node.

```bash
kubectl get pods -n <namespace> -l app.kubernetes.io/instance=<name> -o wide
# Pending pods indicate scheduling failure
```

Fix: Either switch to `soft` anti-affinity or add more nodes to the cluster.

```yaml
spec:
  highAvailability:
    antiAffinityPreset: soft
```

---

## 2. Pods CrashLooping

### Symptom

Pods are in `CrashLoopBackOff` status.

```bash
kubectl get pods -n <namespace> -l app.kubernetes.io/instance=<name>
# STATUS shows CrashLoopBackOff
```

### Diagnosis

**Check logs for the memcached container:**

```bash
kubectl logs <pod-name> -n <namespace> -c memcached --previous
```

**Check logs for the exporter sidecar (if monitoring is enabled):**

```bash
kubectl logs <pod-name> -n <namespace> -c exporter --previous
```

**Check if the pod was OOMKilled:**

```bash
kubectl get pod <pod-name> -n <namespace> -o jsonpath='{.status.containerStatuses[*].lastState.terminated.reason}'
```

### Common Causes and Fixes

**maxMemoryMB exceeds container memory limit (OOMKilled)**

Memcached allocates the amount of memory specified by `maxMemoryMB` for item storage. If the container memory limit does not leave enough room for this plus operational overhead (connections, threads, internal structures), the kernel OOM-kills the process.

The validating webhook rejects configurations where `resources.limits.memory < maxMemoryMB + 32Mi`, but if the webhook is bypassed or the limit is only slightly above the threshold, runtime OOM is still possible.

Fix: Ensure the container memory limit is sufficiently above `maxMemoryMB`. A safe guideline is to set the limit to at least `maxMemoryMB + 64Mi`.

```yaml
spec:
  memcached:
    maxMemoryMB: 512
  resources:
    limits:
      memory: 640Mi  # 512Mi + 128Mi headroom
```

**Invalid extraArgs**

The `extraArgs` field passes arguments directly to the memcached process. Unrecognized or conflicting flags cause the process to exit immediately.

```bash
kubectl logs <pod-name> -n <namespace> -c memcached --previous
# Look for "unknown option" or "illegal argument" messages
```

Fix: Remove or correct the invalid arguments in `spec.memcached.extraArgs`. Refer to the [memcached man page](https://man7.org/linux/man-pages/man1/memcached.1.html) for valid flags.

**Missing SASL Secret**

When SASL is enabled (`security.sasl.enabled: true`), the operator mounts the Secret referenced by `credentialsSecretRef` at `/etc/memcached/sasl/`. If the Secret does not exist or lacks the `password-file` key, the pod fails to start because the volume mount fails.

```bash
kubectl get secret <secret-name> -n <namespace>
kubectl describe pod <pod-name> -n <namespace>
# Look for "MountVolume.SetUp failed" events
```

Fix: Create the required Secret before applying the CR.

```bash
kubectl create secret generic <secret-name> -n <namespace> \
  --from-file=password-file=/path/to/password-file
```

**Missing TLS Secret**

When TLS is enabled (`security.tls.enabled: true`), the operator mounts the Secret referenced by `certificateSecretRef` at `/etc/memcached/tls/`. If the Secret does not exist or is missing the required keys (`tls.crt`, `tls.key`), the pod fails to start.

```bash
kubectl get secret <secret-name> -n <namespace>
kubectl describe pod <pod-name> -n <namespace>
# Look for "MountVolume.SetUp failed" events
```

Fix: Create the TLS Secret with the required keys.

```bash
kubectl create secret tls <secret-name> -n <namespace> \
  --cert=/path/to/tls.crt \
  --key=/path/to/tls.key
```

If `enableClientCert: true` is set, the Secret must also contain a `ca.crt` key:

```bash
kubectl create secret generic <secret-name> -n <namespace> \
  --from-file=tls.crt=/path/to/tls.crt \
  --from-file=tls.key=/path/to/tls.key \
  --from-file=ca.crt=/path/to/ca.crt
```

---

## 3. ServiceMonitor Not Created

### Symptom

`monitoring.enabled` is set to `true` and `monitoring.serviceMonitor` is configured, but no ServiceMonitor resource exists in the namespace.

```bash
kubectl get servicemonitor -n <namespace>
# No ServiceMonitor for the Memcached instance
```

### Diagnosis

**Verify the CR spec includes the serviceMonitor section:**

The operator only creates a ServiceMonitor when `monitoring.enabled: true` AND `monitoring.serviceMonitor` is present (not nil) in the spec.

```bash
kubectl get memcached <name> -n <namespace> -o jsonpath='{.spec.monitoring}' | jq .
```

**Check whether the ServiceMonitor CRD is installed:**

```bash
kubectl get crd servicemonitors.monitoring.coreos.com
```

**Check operator logs for errors:**

```bash
kubectl logs -n memcached-operator-system deployment/memcached-operator-controller-manager -c manager | grep -i servicemonitor
```

### Cause

The Prometheus Operator CRDs are not installed in the cluster. The operator controller watches `ServiceMonitor` resources and will fail to reconcile them if the CRD does not exist.

### Fix

Install the Prometheus Operator CRDs. If you use the `kube-prometheus-stack` Helm chart, the CRDs are included automatically. Otherwise, install them manually:

```bash
kubectl apply -f https://raw.githubusercontent.com/prometheus-operator/prometheus-operator/main/example/prometheus-operator-crd/monitoring.coreos.com_servicemonitors.yaml
```

After the CRD is installed, the operator will create the ServiceMonitor on the next reconciliation cycle.

---

## 4. Webhook Admission Errors

### Symptom

Creating or updating a Memcached CR is rejected with a validation error.

```
Error from server (Invalid): error when creating "memcached.yaml":
admission webhook "vmemcached-v1alpha1.kb.io" denied the request: ...
```

### Diagnosis

Read the error message returned by `kubectl`. The validating webhook provides specific field-level error messages that identify the exact issue.

```bash
kubectl apply -f memcached.yaml
# The error message identifies which field failed validation and why
```

### Common Causes and Fixes

**maxMemoryMB exceeds container memory limit**

The webhook validates that `resources.limits.memory >= maxMemoryMB (in bytes) + 32Mi` (operational overhead).

```
spec.resources.limits.memory: Invalid value: "128Mi": memory limit must be at least 96Mi (maxMemoryMB=64Mi + 32Mi overhead)
```

Fix: Increase `resources.limits.memory` or decrease `maxMemoryMB`.

**minAvailable >= replicas**

The webhook validates that PDB `minAvailable` (when set as an integer) must be strictly less than `replicas`.

```
spec.highAvailability.podDisruptionBudget.minAvailable: Invalid value: 3: minAvailable (3) must be less than replicas (3)
```

Fix: Set `minAvailable` to a value less than `replicas`.

```yaml
spec:
  replicas: 3
  highAvailability:
    podDisruptionBudget:
      enabled: true
      minAvailable: 2  # Must be < replicas (3)
```

**minAvailable and maxUnavailable both set**

The webhook enforces that `minAvailable` and `maxUnavailable` are mutually exclusive.

Fix: Specify only one of `minAvailable` or `maxUnavailable`.

**Missing Secret references when security features are enabled**

When SASL is enabled, `credentialsSecretRef.name` must be set. When TLS is enabled, `certificateSecretRef.name` must be set.

```
spec.security.sasl.credentialsSecretRef.name: Required value: credentialsSecretRef.name is required when SASL is enabled
```

Fix: Provide the required Secret reference name.

**terminationGracePeriodSeconds <= preStopDelaySeconds**

When graceful shutdown is enabled, `terminationGracePeriodSeconds` must exceed `preStopDelaySeconds` to ensure the preStop hook completes before the kubelet sends SIGKILL.

```
spec.highAvailability.gracefulShutdown.terminationGracePeriodSeconds: Invalid value: 10: terminationGracePeriodSeconds (10) must exceed preStopDelaySeconds (10)
```

Fix: Increase `terminationGracePeriodSeconds` or decrease `preStopDelaySeconds`.

```yaml
spec:
  highAvailability:
    gracefulShutdown:
      enabled: true
      preStopDelaySeconds: 10
      terminationGracePeriodSeconds: 30  # Must be > preStopDelaySeconds
```

**Replicas out of range**

The CRD schema enforces `replicas` to be between 0 and 64 (inclusive).

Fix: Set `replicas` to a value within the allowed range.

---

## 5. Operator Not Reconciling

### Symptom

Changes to a Memcached CR (e.g., scaling replicas) are not reflected in the managed Deployment, Service, or other resources.

### Diagnosis

**Check the operator pod status:**

```bash
kubectl get pods -n memcached-operator-system
```

**Check operator logs:**

```bash
kubectl logs -n memcached-operator-system deployment/memcached-operator-controller-manager -c manager --tail=100
```

**Check the operator health endpoints:**

```bash
kubectl port-forward -n memcached-operator-system deployment/memcached-operator-controller-manager 8081:8081
curl http://localhost:8081/healthz
curl http://localhost:8081/readyz
```

**Check RBAC permissions:**

```bash
kubectl auth can-i get deployments --as=system:serviceaccount:memcached-operator-system:memcached-operator-controller-manager -n <namespace>
```

### Common Causes and Fixes

**Operator pod not running**

The operator Deployment has zero ready replicas.

```bash
kubectl get deployment memcached-operator-controller-manager -n memcached-operator-system
kubectl describe deployment memcached-operator-controller-manager -n memcached-operator-system
```

Fix: Investigate why the operator pod is not running (image pull issues, resource constraints, crash loop). Check pod events and logs for the specific error.

**RBAC permissions missing**

The operator's ServiceAccount lacks the required ClusterRole/Role bindings to manage the target resources (Deployments, Services, PodDisruptionBudgets, NetworkPolicies, ServiceMonitors, Secrets).

```bash
kubectl logs -n memcached-operator-system deployment/memcached-operator-controller-manager -c manager | grep -i forbidden
```

Fix: Verify that the ClusterRole and ClusterRoleBinding for the operator are correctly applied. Re-apply the operator manifests if necessary.

```bash
kubectl apply -k config/default
```

**Leader election issues**

In multi-replica operator deployments, only the leader instance performs reconciliation. If leader election is stuck (e.g., a stale lease), no reconciliation occurs.

```bash
kubectl get lease -n memcached-operator-system
kubectl logs -n memcached-operator-system deployment/memcached-operator-controller-manager -c manager | grep -i "leader"
```

Fix: Delete the stale Lease object to allow a new leader election.

```bash
kubectl delete lease <lease-name> -n memcached-operator-system
```

**CRDs not installed**

The Memcached CRD is not installed in the cluster.

```bash
kubectl get crd memcacheds.memcached.c5c3.io
```

Fix: Install the CRD.

```bash
kubectl apply -k config/crd
```

---

## 6. Metrics Not Available

### Symptom

Prometheus cannot scrape memcached metrics. The exporter target is down or missing from Prometheus targets.

### Diagnosis

**Check if the exporter sidecar is running:**

```bash
kubectl get pod <pod-name> -n <namespace> -o jsonpath='{.spec.containers[*].name}'
# Should include "exporter" if monitoring is enabled
```

**Test the metrics endpoint directly:**

```bash
kubectl port-forward <pod-name> -n <namespace> 9150:9150
curl http://localhost:9150/metrics
```

**Check the ServiceMonitor exists and has correct labels:**

```bash
kubectl get servicemonitor <name> -n <namespace> -o yaml
```

**Check Prometheus targets:**

In the Prometheus UI (Status > Targets), look for the memcached target. If it is missing, check that the ServiceMonitor labels match the Prometheus `serviceMonitorSelector`.

**Check the Service exposes the metrics port:**

```bash
kubectl get svc <name> -n <namespace> -o jsonpath='{.spec.ports}' | jq .
# Should include port 9150 named "metrics"
```

### Common Causes and Fixes

**Monitoring not enabled**

The CR does not have `monitoring.enabled: true`.

Fix: Enable monitoring in the CR.

```yaml
spec:
  monitoring:
    enabled: true
    serviceMonitor:
      additionalLabels:
        release: prometheus  # Must match Prometheus serviceMonitorSelector
```

**ServiceMonitor label mismatch**

Prometheus selects ServiceMonitors by label. If the ServiceMonitor's labels do not match the Prometheus `serviceMonitorSelector`, the target is ignored.

```bash
# Check what labels Prometheus expects
kubectl get prometheus -A -o jsonpath='{.items[*].spec.serviceMonitorSelector}' | jq .
```

Fix: Add the required labels via `monitoring.serviceMonitor.additionalLabels`.

```yaml
spec:
  monitoring:
    enabled: true
    serviceMonitor:
      additionalLabels:
        release: prometheus  # Common label for kube-prometheus-stack
```

**NetworkPolicy blocking Prometheus scrapes**

If a NetworkPolicy is enabled, the ingress rules must allow traffic to port 9150 from Prometheus pods.

```bash
kubectl get networkpolicy <name> -n <namespace> -o yaml
```

The operator automatically includes port 9150 in the NetworkPolicy when monitoring is enabled. However, if `allowedSources` is configured, ensure that the Prometheus pods match the allowed peer selectors.

Fix: Add Prometheus pods to the `allowedSources` list.

```yaml
spec:
  security:
    networkPolicy:
      enabled: true
      allowedSources:
        - podSelector:
            matchLabels:
              app.kubernetes.io/name: prometheus
```

**Exporter sidecar crashing**

The exporter container may be crashing independently.

```bash
kubectl logs <pod-name> -n <namespace> -c exporter
```

Fix: Check the exporter image is valid and resource limits are sufficient. Increase exporter resources if the container is OOMKilled.

```yaml
spec:
  monitoring:
    enabled: true
    exporterResources:
      requests:
        cpu: 50m
        memory: 32Mi
      limits:
        cpu: 100m
        memory: 64Mi
```

---

## 7. PDB Blocking Node Drain

### Symptom

`kubectl drain <node>` hangs indefinitely. The drain operation cannot evict Memcached pods because the PodDisruptionBudget prevents it.

```bash
kubectl drain <node> --ignore-daemonsets --delete-emptydir-data
# Hangs with: "Cannot evict pod as it would violate the pod's disruption budget"
```

### Diagnosis

**Check the PDB status:**

```bash
kubectl get pdb <name> -n <namespace>
kubectl describe pdb <name> -n <namespace>
```

Key fields to examine:

- `ALLOWED DISRUPTIONS`: If this is 0, no pods can be evicted.
- `MIN AVAILABLE`: The minimum number of pods that must remain running.
- `CURRENT`: The current number of healthy pods.

```bash
kubectl get pdb <name> -n <namespace> -o jsonpath='{.status}' | jq .
```

### Cause

`minAvailable` is set too high relative to the current `replicas` count. For example, with `replicas: 3` and `minAvailable: 2`, only 1 pod can be disrupted at a time. If two nodes need to be drained simultaneously, the second drain will block.

The validating webhook prevents `minAvailable >= replicas`, but even valid configurations (e.g., `minAvailable: 2` with `replicas: 3`) can cause drain issues when multiple nodes are involved.

### Fix

**Option A: Lower minAvailable**

```yaml
spec:
  highAvailability:
    podDisruptionBudget:
      enabled: true
      minAvailable: 1
```

**Option B: Switch to maxUnavailable**

Using `maxUnavailable` is often more practical for drain operations because it directly controls how many pods can be down simultaneously. Note that `minAvailable` and `maxUnavailable` are mutually exclusive.

```yaml
spec:
  highAvailability:
    podDisruptionBudget:
      enabled: true
      maxUnavailable: 1
```

**Option C: Temporarily disable PDB for maintenance**

Set `podDisruptionBudget.enabled: false` during maintenance windows, then re-enable it after drains are complete.

---

## 8. NetworkPolicy Blocking Client Traffic

### Symptom

Application pods cannot connect to Memcached. Connections to port 11211 (or 11212 for TLS) time out or are refused.

```bash
# From a client pod:
nc -zv <memcached-service>.<namespace>.svc.cluster.local 11211
# Connection timed out
```

### Diagnosis

**Check if a NetworkPolicy exists:**

```bash
kubectl get networkpolicy <name> -n <namespace>
kubectl describe networkpolicy <name> -n <namespace>
```

**Examine the ingress rules:**

```bash
kubectl get networkpolicy <name> -n <namespace> -o yaml
```

Look at the `spec.ingress[].from` field. If `allowedSources` is configured, only pods matching those selectors can reach the Memcached pods.

**Verify client pod labels:**

```bash
kubectl get pod <client-pod> -n <client-namespace> --show-labels
```

Compare the client pod labels against the `allowedSources` peer selectors in the NetworkPolicy.

### Cause

The `allowedSources` list in `spec.security.networkPolicy` does not include the client application pods. When `allowedSources` is non-empty, the operator creates ingress rules that restrict traffic to only the listed peers.

### Fix

Update the `allowedSources` to include the client pods. The operator supports both `podSelector` (for same-namespace peers) and `namespaceSelector` (for cross-namespace access).

**Allow specific pods by label:**

```yaml
spec:
  security:
    networkPolicy:
      enabled: true
      allowedSources:
        - podSelector:
            matchLabels:
              app: my-application
```

**Allow all pods in a specific namespace:**

```yaml
spec:
  security:
    networkPolicy:
      enabled: true
      allowedSources:
        - namespaceSelector:
            matchLabels:
              kubernetes.io/metadata.name: my-app-namespace
```

**Allow all traffic (remove restrictions):**

Set `allowedSources` to an empty list or omit it entirely. When `allowedSources` is empty, the NetworkPolicy allows ingress from all sources on the Memcached ports.

```yaml
spec:
  security:
    networkPolicy:
      enabled: true
      # allowedSources omitted = all sources allowed on Memcached ports
```

---

## General Debugging Tips

### Checking Operator Logs

The operator logs contain detailed information about reconciliation activity:

```bash
kubectl logs -n memcached-operator-system deployment/memcached-operator-controller-manager -c manager -f
```

Filter for a specific Memcached instance:

```bash
kubectl logs -n memcached-operator-system deployment/memcached-operator-controller-manager -c manager | grep '"name":"<instance-name>"'
```

### Inspecting Managed Resources

All resources created by the operator carry standard labels. Use them to find all resources for a given instance:

```bash
kubectl get all,pdb,networkpolicy,servicemonitor -n <namespace> \
  -l app.kubernetes.io/name=memcached,app.kubernetes.io/instance=<name>
```

### Checking Owner References

Every managed resource has an `ownerReference` pointing to the parent Memcached CR. This ensures garbage collection when the CR is deleted:

```bash
kubectl get deployment <name> -n <namespace> -o jsonpath='{.metadata.ownerReferences}' | jq .
```

### Verifying Webhook Configuration

If webhook admission fails unexpectedly, verify the webhook configuration:

```bash
kubectl get validatingwebhookconfigurations
kubectl get mutatingwebhookconfigurations
```

Check that the webhook CA bundle is valid and that the cert-manager Certificate is in a `Ready` state:

```bash
kubectl get certificate -n memcached-operator-system
```
