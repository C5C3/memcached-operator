# CRD Reference

Complete field reference for the `Memcached` Custom Resource Definition.

---

## Resource Info

| Property | Value |
|----------|-------|
| API Group | `memcached.c5c3.io` |
| API Version | `v1alpha1` |
| Kind | `Memcached` |
| List Kind | `MemcachedList` |
| Scope | Namespaced |
| Subresources | `status` |

---

## MemcachedSpec

`MemcachedSpec` defines the desired state of a Memcached instance.

| Field | Type | Default | Validation | Description |
|-------|------|---------|------------|-------------|
| `replicas` | `*int32` | `1` | min=0, max=64 | Number of Memcached pods |
| `image` | `*string` | `"memcached:1.6"` | -- | Container image for the Memcached server |
| `resources` | [`*ResourceRequirements`](https://kubernetes.io/docs/reference/kubernetes-api/workload-resources/pod-v1/#resources) | -- | -- | CPU/memory requests and limits for the Memcached container |
| `memcached` | [`*MemcachedConfig`](#memcachedconfig) | -- | -- | Memcached server configuration parameters |
| `highAvailability` | [`*HighAvailabilitySpec`](#highavailabilityspec) | -- | -- | High-availability settings (anti-affinity, PDB, topology spread, graceful shutdown) |
| `monitoring` | [`*MonitoringSpec`](#monitoringspec) | -- | -- | Monitoring and metrics configuration |
| `security` | [`*SecuritySpec`](#securityspec) | -- | -- | Security settings (security contexts, SASL, TLS, NetworkPolicy) |
| `service` | [`*ServiceSpec`](#servicespec) | -- | -- | Configuration for the headless Service |

---

## MemcachedConfig

`MemcachedConfig` defines the Memcached server runtime configuration. Each field maps to a memcached command-line flag.

| Field | Type | Default | Validation | Memcached Flag | Description |
|-------|------|---------|------------|----------------|-------------|
| `maxMemoryMB` | `int32` | `64` | min=16, max=65536 | `-m` | Maximum memory for item storage in megabytes |
| `maxConnections` | `int32` | `1024` | min=1, max=65536 | `-c` | Maximum number of simultaneous connections |
| `threads` | `int32` | `4` | min=1, max=128 | `-t` | Number of worker threads |
| `maxItemSize` | `string` | `"1m"` | pattern=`^[0-9]+(k\|m)$` | `-I` | Maximum size of an item (e.g., `"1m"`, `"2m"`, `"512k"`) |
| `verbosity` | `int32` | `0` | min=0, max=2 | `-v` / `-vv` | Logging verbosity level (0=none, 1=verbose, 2=very verbose) |
| `extraArgs` | `[]string` | `[]` | -- | (raw) | Additional command-line arguments passed directly to the Memcached process |

### Verbosity Mapping

| Value | Memcached Flag | Effect |
|-------|---------------|--------|
| `0` | (none) | No verbose logging |
| `1` | `-v` | Verbose logging |
| `2` | `-vv` | Very verbose logging |

---

## HighAvailabilitySpec

`HighAvailabilitySpec` defines high-availability settings for Memcached pods.

| Field | Type | Default | Validation | Description |
|-------|------|---------|------------|-------------|
| `antiAffinityPreset` | `*AntiAffinityPreset` | `"soft"` | enum: `soft`, `hard` | Controls pod anti-affinity scheduling preset |
| `topologySpreadConstraints` | [`[]TopologySpreadConstraint`](https://kubernetes.io/docs/reference/kubernetes-api/workload-resources/pod-v1/#scheduling) | -- | -- | Defines how pods are spread across topology domains |
| `podDisruptionBudget` | [`*PDBSpec`](#pdbspec) | -- | -- | PodDisruptionBudget configuration |
| `gracefulShutdown` | [`*GracefulShutdownSpec`](#gracefulshutdownspec) | -- | -- | Configures preStop lifecycle hooks and termination grace period |

### AntiAffinityPreset Values

| Value | Scheduling Rule | Behavior |
|-------|----------------|----------|
| `soft` | `preferredDuringSchedulingIgnoredDuringExecution` | Best-effort spreading; pods prefer different nodes but can be co-located if necessary |
| `hard` | `requiredDuringSchedulingIgnoredDuringExecution` | Strict spreading; pods must be on different nodes |

---

## GracefulShutdownSpec

`GracefulShutdownSpec` defines the graceful shutdown configuration, allowing in-flight connections to drain before pod termination.

| Field | Type | Default | Validation | Description |
|-------|------|---------|------------|-------------|
| `enabled` | `bool` | `false` | -- | Controls whether graceful shutdown is configured |
| `preStopDelaySeconds` | `int32` | `10` | min=1, max=300 | Number of seconds the preStop hook sleeps to allow connection draining |
| `terminationGracePeriodSeconds` | `int64` | `30` | min=1, max=600 | Duration in seconds the pod needs to terminate gracefully. Must exceed `preStopDelaySeconds` to allow the hook to complete before SIGKILL. |

---

## PDBSpec

`PDBSpec` defines the PodDisruptionBudget configuration. When enabled, a PDB is created to guarantee a minimum number of pods remain available during voluntary disruptions (node drains, upgrades).

| Field | Type | Default | Validation | Description |
|-------|------|---------|------------|-------------|
| `enabled` | `bool` | `false` | -- | Controls whether a PodDisruptionBudget is created |
| `minAvailable` | `*IntOrString` | -- | -- | Minimum number of pods that must be available during disruption. Can be an absolute number (e.g., `1`) or a percentage (e.g., `"50%"`). The controller defaults to `1` when neither `minAvailable` nor `maxUnavailable` is set. |
| `maxUnavailable` | `*IntOrString` | -- | -- | Maximum number of pods that can be unavailable during disruption. Can be an absolute number or a percentage. |

> **Note:** Only one of `minAvailable` or `maxUnavailable` should be set. If both are specified, the behavior follows the standard Kubernetes PDB semantics.

---

## MonitoringSpec

`MonitoringSpec` defines monitoring and metrics configuration. When enabled, a Prometheus `memcached-exporter` sidecar is injected into the Memcached pods.

| Field | Type | Default | Validation | Description |
|-------|------|---------|------------|-------------|
| `enabled` | `bool` | `false` | -- | Controls whether monitoring is active (enables the exporter sidecar) |
| `exporterImage` | `*string` | `"prom/memcached-exporter:v0.15.4"` | -- | Container image for the memcached-exporter sidecar |
| `exporterResources` | [`*ResourceRequirements`](https://kubernetes.io/docs/reference/kubernetes-api/workload-resources/pod-v1/#resources) | -- | -- | Resource requests/limits for the exporter sidecar container |
| `serviceMonitor` | [`*ServiceMonitorSpec`](#servicemonitorspec) | -- | -- | Prometheus ServiceMonitor resource configuration |

---

## ServiceMonitorSpec

`ServiceMonitorSpec` defines the Prometheus ServiceMonitor configuration. The ServiceMonitor is only created when the `ServiceMonitor` CRD exists in the cluster (i.e., the Prometheus Operator is installed).

| Field | Type | Default | Validation | Description |
|-------|------|---------|------------|-------------|
| `additionalLabels` | `map[string]string` | -- | -- | Extra labels added to the ServiceMonitor resource (e.g., `release: prometheus`) |
| `interval` | `string` | `"30s"` | -- | Prometheus scrape interval |
| `scrapeTimeout` | `string` | `"10s"` | -- | Prometheus scrape timeout |

---

## SecuritySpec

`SecuritySpec` defines security settings for Memcached, including pod/container security contexts, authentication, encryption, and network policy.

| Field | Type | Default | Validation | Description |
|-------|------|---------|------------|-------------|
| `podSecurityContext` | [`*PodSecurityContext`](https://kubernetes.io/docs/reference/kubernetes-api/workload-resources/pod-v1/#security-context) | -- | -- | Security context applied at the pod level |
| `containerSecurityContext` | [`*SecurityContext`](https://kubernetes.io/docs/reference/kubernetes-api/workload-resources/pod-v1/#security-context-1) | -- | -- | Security context applied to the Memcached container |
| `sasl` | [`*SASLSpec`](#saslspec) | -- | -- | Optional SASL authentication configuration |
| `tls` | [`*TLSSpec`](#tlsspec) | -- | -- | Optional TLS encryption configuration |
| `networkPolicy` | [`*NetworkPolicySpec`](#networkpolicyspec) | -- | -- | Kubernetes NetworkPolicy configuration for Memcached pods |

---

## SASLSpec

`SASLSpec` defines SASL authentication configuration. When enabled, the operator mounts the credentials Secret into the container and adds the `-S` flag to Memcached.

| Field | Type | Default | Validation | Description |
|-------|------|---------|------------|-------------|
| `enabled` | `bool` | `false` | -- | Controls whether SASL authentication is active |
| `credentialsSecretRef` | [`LocalObjectReference`](https://kubernetes.io/docs/reference/kubernetes-api/common-definitions/local-object-reference/) | -- | -- | Reference to the Secret containing SASL credentials. The Secret must contain a `password-file` key with the SASL password file content. |

---

## TLSSpec

`TLSSpec` defines TLS encryption configuration. When enabled, the operator mounts the certificate Secret and configures memcached with TLS flags (`--enable-ssl`, `--ssl-cert`, `--ssl-key`, `--ssl-ca-cert`).

| Field | Type | Default | Validation | Description |
|-------|------|---------|------------|-------------|
| `enabled` | `bool` | `false` | -- | Controls whether TLS encryption is active |
| `certificateSecretRef` | [`LocalObjectReference`](https://kubernetes.io/docs/reference/kubernetes-api/common-definitions/local-object-reference/) | -- | -- | Reference to the Secret containing TLS certificates. The Secret must contain `tls.crt`, `tls.key`, and optionally `ca.crt` keys. |
| `enableClientCert` | `bool` | `false` | -- | Controls whether mutual TLS (mTLS) is required. When `true`, Memcached requires clients to present a valid TLS certificate. The CA certificate (`ca.crt`) in the Secret is used to verify client certificates. |

---

## NetworkPolicySpec

`NetworkPolicySpec` defines the Kubernetes NetworkPolicy configuration for Memcached. When enabled, a NetworkPolicy is created that restricts ingress traffic to the Memcached port (11211).

> **Note:** In the CRD, `NetworkPolicySpec` is nested under `spec.security.networkPolicy`, not at the top level.

| Field | Type | Default | Validation | Description |
|-------|------|---------|------------|-------------|
| `enabled` | `bool` | `false` | -- | Controls whether a NetworkPolicy is created |
| `allowedSources` | [`[]NetworkPolicyPeer`](https://kubernetes.io/docs/reference/kubernetes-api/policy-resources/network-policy-v1/#NetworkPolicyPeer) | -- | -- | List of peers allowed to access Memcached. When empty or nil, all sources are allowed. Supports `podSelector`, `namespaceSelector`, and `ipBlock`. |

---

## ServiceSpec

`ServiceSpec` defines configuration for the headless Service created for each Memcached instance.

| Field | Type | Default | Validation | Description |
|-------|------|---------|------------|-------------|
| `annotations` | `map[string]string` | -- | -- | Custom annotations added to the Service metadata |

---

## MemcachedStatus

`MemcachedStatus` defines the observed state of a Memcached instance. The status is updated by the controller during each reconciliation cycle.

| Field | Type | Description |
|-------|------|-------------|
| `conditions` | `[]metav1.Condition` | Standard Kubernetes conditions representing the latest available observations of the Memcached instance's state. Uses merge-patch with `type` as the merge key. See [Status Conditions](#status-conditions) below. |
| `readyReplicas` | `int32` | Number of Memcached pods that are ready |
| `observedGeneration` | `int64` | Most recent generation observed by the controller. Clients can compare this to `metadata.generation` to determine if the status is up-to-date with the latest spec changes. |

### Status Conditions

| Condition Type | Status Values | Description |
|---------------|---------------|-------------|
| `Available` | `True` / `False` | `True` when the Deployment has minimum availability |
| `Progressing` | `True` / `False` | `True` when a rollout or scale operation is in progress |
| `Degraded` | `True` / `False` | `True` when fewer replicas than desired are ready |

---

## Printer Columns

When using `kubectl get memcached`, the following columns are displayed:

| Column | Source | Type | Description |
|--------|--------|------|-------------|
| `Replicas` | `.spec.replicas` | integer | Number of desired Memcached pods |
| `Ready` | `.status.readyReplicas` | integer | Number of ready Memcached pods |
| `Age` | `.metadata.creationTimestamp` | date | Time since the resource was created |

---

## Webhook Behavior

The operator registers a **defaulting webhook** (mutating) and a **validation webhook** (validating) for `Memcached` resources. Both webhooks run on `create` and `update` operations.

### Defaulting Rules

The defaulting webhook sets values for omitted fields before the resource is persisted. Fields with CRD-level defaults (via `+kubebuilder:default`) are handled by the API server; the webhook handles pointer fields and conditional defaults.

| Field | Default | Condition |
|-------|---------|-----------|
| `spec.replicas` | `1` | When nil |
| `spec.image` | `"memcached:1.6"` | When nil |
| `spec.memcached.maxMemoryMB` | `64` | When 0 (section initialized if nil) |
| `spec.memcached.maxConnections` | `1024` | When 0 |
| `spec.memcached.threads` | `4` | When 0 |
| `spec.memcached.maxItemSize` | `"1m"` | When empty |
| `spec.monitoring.exporterImage` | `"prom/memcached-exporter:v0.15.4"` | When nil (only if `monitoring` section exists) |
| `spec.monitoring.serviceMonitor.interval` | `"30s"` | When empty (only if `serviceMonitor` section exists) |
| `spec.monitoring.serviceMonitor.scrapeTimeout` | `"10s"` | When empty (only if `serviceMonitor` section exists) |
| `spec.highAvailability.antiAffinityPreset` | `"soft"` | When nil (only if `highAvailability` section exists) |

### Validation Rules

The validation webhook enforces cross-field constraints that cannot be expressed with kubebuilder markers alone.

| Rule | Condition | Error |
|------|-----------|-------|
| Memory limit sufficient | `resources.limits.memory` is set and `memcached` section exists | `resources.limits.memory` must be at least `maxMemoryMB + 32Mi` (operational overhead for connections, threads, internal structures) |
| PDB mutual exclusivity | PDB is enabled | `minAvailable` and `maxUnavailable` cannot both be set |
| PDB requires a budget field | PDB is enabled | One of `minAvailable` or `maxUnavailable` must be set |
| PDB minAvailable < replicas | PDB is enabled with integer `minAvailable` | `minAvailable` must be strictly less than `replicas` |
| Graceful shutdown timing | Graceful shutdown is enabled | `terminationGracePeriodSeconds` must exceed `preStopDelaySeconds` |
| SASL secret required | `security.sasl.enabled` is `true` | `credentialsSecretRef.name` must be non-empty |
| TLS secret required | `security.tls.enabled` is `true` | `certificateSecretRef.name` must be non-empty |

---

## Examples

### Minimal

The simplest valid `Memcached` resource. All fields use their defaults (1 replica, `memcached:1.6` image, 64MB memory, 1024 max connections, 4 threads).

```yaml
apiVersion: memcached.c5c3.io/v1alpha1
kind: Memcached
metadata:
  name: basic-cache
  namespace: default
spec: {}
```

### Full

A comprehensive example using all available fields.

```yaml
apiVersion: memcached.c5c3.io/v1alpha1
kind: Memcached
metadata:
  name: prod-cache
  namespace: production
spec:
  replicas: 3
  image: memcached:1.6
  resources:
    requests:
      cpu: 250m
      memory: 256Mi
    limits:
      cpu: "1"
      memory: 512Mi

  memcached:
    maxMemoryMB: 256
    maxConnections: 2048
    threads: 4
    maxItemSize: "2m"
    verbosity: 0
    extraArgs: []

  highAvailability:
    antiAffinityPreset: soft
    topologySpreadConstraints:
      - maxSkew: 1
        topologyKey: topology.kubernetes.io/zone
        whenUnsatisfiable: ScheduleAnyway
    podDisruptionBudget:
      enabled: true
      minAvailable: 1
    gracefulShutdown:
      enabled: true
      preStopDelaySeconds: 10
      terminationGracePeriodSeconds: 30

  monitoring:
    enabled: true
    exporterImage: prom/memcached-exporter:v0.15.4
    exporterResources:
      requests:
        cpu: 50m
        memory: 32Mi
      limits:
        cpu: 100m
        memory: 64Mi
    serviceMonitor:
      additionalLabels:
        release: prometheus
      interval: 30s
      scrapeTimeout: 10s

  security:
    podSecurityContext:
      runAsNonRoot: true
      runAsUser: 11211
      runAsGroup: 11211
      fsGroup: 11211
      seccompProfile:
        type: RuntimeDefault
    containerSecurityContext:
      allowPrivilegeEscalation: false
      readOnlyRootFilesystem: true
      capabilities:
        drop:
          - ALL
    sasl:
      enabled: true
      credentialsSecretRef:
        name: memcached-sasl-credentials
    tls:
      enabled: true
      certificateSecretRef:
        name: memcached-tls
      enableClientCert: true
    networkPolicy:
      enabled: true
      allowedSources:
        - podSelector:
            matchLabels:
              app: keystone
        - namespaceSelector:
            matchLabels:
              team: platform

  service:
    annotations:
      prometheus.io/scrape: "true"
      prometheus.io/port: "9150"
```

### High-Availability with Monitoring

A production-oriented configuration focusing on availability and observability.

```yaml
apiVersion: memcached.c5c3.io/v1alpha1
kind: Memcached
metadata:
  name: ha-cache
  namespace: production
spec:
  replicas: 3
  resources:
    requests:
      cpu: 500m
      memory: 512Mi
    limits:
      cpu: "2"
      memory: 1Gi
  memcached:
    maxMemoryMB: 512
    maxConnections: 2048
    threads: 4
  highAvailability:
    antiAffinityPreset: soft
    topologySpreadConstraints:
      - maxSkew: 1
        topologyKey: topology.kubernetes.io/zone
        whenUnsatisfiable: ScheduleAnyway
    podDisruptionBudget:
      enabled: true
      minAvailable: 2
    gracefulShutdown:
      enabled: true
      preStopDelaySeconds: 15
      terminationGracePeriodSeconds: 45
  monitoring:
    enabled: true
    serviceMonitor:
      additionalLabels:
        release: kube-prometheus-stack
      interval: 15s
```
