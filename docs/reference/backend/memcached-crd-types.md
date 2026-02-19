# Memcached CRD Types Reference

API reference for the `memcached.c5c3.io/v1alpha1` Custom Resource Definition types.

**Source**: `api/v1alpha1/memcached_types.go`

## Memcached

Top-level custom resource representing a Memcached cluster.

**Group**: `memcached.c5c3.io`
**Version**: `v1alpha1`
**Kind**: `Memcached`
**Scope**: Namespaced

### Printer Columns

| Column   | Type    | JSONPath                         | Description                         |
|----------|---------|----------------------------------|-------------------------------------|
| Replicas | integer | `.spec.replicas`                 | Number of desired Memcached pods    |
| Ready    | integer | `.status.readyReplicas`          | Number of ready Memcached pods      |
| Age      | date    | `.metadata.creationTimestamp`    | Resource creation timestamp         |

### Subresources

- **status**: Enabled. Status updates use the `/status` subresource endpoint.

---

## MemcachedSpec

Defines the desired state of a Memcached cluster.

| Field              | Type                                                 | Required | Default          | Validation                  | Description                                      |
|--------------------|------------------------------------------------------|----------|------------------|-----------------------------|--------------------------------------------------|
| `replicas`         | `*int32`                                             | No       | `1`              | Minimum: 0, Maximum: 64    | Number of Memcached pods                         |
| `image`            | `*string`                                            | No       | `"memcached:1.6"`| —                           | Container image for the Memcached server         |
| `resources`        | [`*corev1.ResourceRequirements`][resource-reqs]      | No       | —                | —                           | Resource requests and limits for the container   |
| `memcached`        | [`*MemcachedConfig`](#memcachedconfig)               | No       | —                | —                           | Memcached server configuration parameters        |
| `highAvailability` | [`*HighAvailabilitySpec`](#highavailabilityspec)      | No       | —                | —                           | High-availability settings                       |
| `monitoring`       | [`*MonitoringSpec`](#monitoringspec)                  | No       | —                | —                           | Monitoring and metrics configuration             |
| `security`         | [`*SecuritySpec`](#securityspec)                      | No       | —                | —                           | Security settings                                |

---

## MemcachedConfig

Defines Memcached server runtime parameters. These are translated into memcached command-line flags by the reconciler.

| Field            | Type       | Required | Default  | Validation                           | Description                                                 |
|------------------|------------|----------|----------|--------------------------------------|-------------------------------------------------------------|
| `maxMemoryMB`    | `int32`    | No       | `64`     | Minimum: 16, Maximum: 65536         | Maximum memory for item storage in MB (`-m` flag)           |
| `maxConnections` | `int32`    | No       | `1024`   | Minimum: 1, Maximum: 65536          | Maximum simultaneous connections (`-c` flag)                |
| `threads`        | `int32`    | No       | `4`      | Minimum: 1, Maximum: 128            | Number of worker threads (`-t` flag)                        |
| `maxItemSize`    | `string`   | No       | `"1m"`   | Pattern: `^[0-9]+(k\|m)$`           | Maximum size of a single item (`-I` flag, e.g. `"1m"`, `"512k"`) |
| `verbosity`      | `int32`    | No       | `0`      | Minimum: 0, Maximum: 2              | Logging verbosity (0=none, 1=`-v`, 2=`-vv`)                |
| `extraArgs`      | `[]string` | No       | —        | —                                    | Additional command-line arguments passed to memcached       |

---

## HighAvailabilitySpec

Defines high-availability settings for Memcached pods, including anti-affinity, topology spread, and pod disruption budgets.

| Field                        | Type                                                          | Required | Default  | Validation          | Description                                         |
|------------------------------|---------------------------------------------------------------|----------|----------|---------------------|-----------------------------------------------------|
| `antiAffinityPreset`         | [`*AntiAffinityPreset`](#antiaffinitypreset)                  | No       | `"soft"` | Enum: `soft`, `hard`| Pod anti-affinity scheduling mode                   |
| `topologySpreadConstraints`  | [`[]corev1.TopologySpreadConstraint`][topology-spread]        | No       | —        | —                   | Topology domain spread constraints for pods         |
| `podDisruptionBudget`        | [`*PDBSpec`](#pdbspec)                                        | No       | —        | —                   | PodDisruptionBudget configuration                   |

### AntiAffinityPreset

String enum type controlling pod anti-affinity behavior.

| Value  | Description                                              |
|--------|----------------------------------------------------------|
| `soft` | Uses `preferredDuringSchedulingIgnoredDuringExecution`   |
| `hard` | Uses `requiredDuringSchedulingIgnoredDuringExecution`    |

---

## PDBSpec

Defines PodDisruptionBudget configuration for Memcached pods.

| Field            | Type                          | Required | Default | Validation | Description                                                  |
|------------------|-------------------------------|----------|---------|------------|--------------------------------------------------------------|
| `enabled`        | `bool`                        | No       | `false` | —          | Whether a PodDisruptionBudget is created                     |
| `minAvailable`   | `*intstr.IntOrString`         | No       | `1`     | —          | Minimum available pods during disruption (absolute or `%`)    |
| `maxUnavailable` | `*intstr.IntOrString`         | No       | —       | —          | Maximum unavailable pods during disruption (absolute or `%`)  |

---

## MonitoringSpec

Defines monitoring and metrics collection configuration, including the memcached-exporter sidecar and Prometheus ServiceMonitor.

| Field               | Type                                                 | Required | Default                               | Validation | Description                                       |
|---------------------|------------------------------------------------------|----------|---------------------------------------|------------|---------------------------------------------------|
| `enabled`           | `bool`                                               | No       | `false`                               | —          | Enables the memcached-exporter sidecar            |
| `exporterImage`     | `*string`                                            | No       | `"prom/memcached-exporter:v0.15.4"`   | —          | Container image for the exporter sidecar          |
| `exporterResources` | [`*corev1.ResourceRequirements`][resource-reqs]      | No       | —                                     | —          | Resource requests/limits for the exporter sidecar |
| `serviceMonitor`    | [`*ServiceMonitorSpec`](#servicemonitorspec)          | No       | —                                     | —          | Prometheus ServiceMonitor configuration           |

---

## ServiceMonitorSpec

Defines Prometheus ServiceMonitor resource configuration.

| Field              | Type                | Required | Default  | Validation | Description                                       |
|--------------------|---------------------|----------|----------|------------|---------------------------------------------------|
| `additionalLabels` | `map[string]string` | No       | —        | —          | Extra labels added to the ServiceMonitor resource |
| `interval`         | `string`            | No       | `"30s"`  | —          | Prometheus scrape interval (e.g. `"30s"`)         |
| `scrapeTimeout`    | `string`            | No       | `"10s"`  | —          | Prometheus scrape timeout (e.g. `"10s"`)          |

---

## SecuritySpec

Defines security settings for Memcached pods, including pod/container security contexts, authentication, and encryption.

| Field                        | Type                                                         | Required | Default | Validation | Description                                           |
|------------------------------|--------------------------------------------------------------|----------|---------|------------|-------------------------------------------------------|
| `podSecurityContext`         | [`*corev1.PodSecurityContext`][pod-security-context]         | No       | —       | —          | Security context for the Memcached pod                |
| `containerSecurityContext`   | [`*corev1.SecurityContext`][security-context]                | No       | —       | —          | Security context for the Memcached container          |
| `sasl`                       | [`*SASLSpec`](#saslspec)                                     | No       | —       | —          | SASL authentication configuration                     |
| `tls`                        | [`*TLSSpec`](#tlsspec)                                       | No       | —       | —          | TLS encryption configuration                          |

---

## SASLSpec

Defines SASL authentication configuration for Memcached.

| Field                  | Type                            | Required | Default | Validation | Description                                                        |
|------------------------|---------------------------------|----------|---------|------------|--------------------------------------------------------------------|
| `enabled`              | `bool`                          | No       | `false` | —          | Whether SASL authentication is active                              |
| `credentialsSecretRef` | `corev1.LocalObjectReference`   | No       | —       | —          | Reference to the Secret containing a `password-file` key with SASL credentials |

---

## TLSSpec

Defines TLS encryption configuration for Memcached.

| Field                  | Type                            | Required | Default | Validation | Description                                                                |
|------------------------|---------------------------------|----------|---------|------------|----------------------------------------------------------------------------|
| `enabled`              | `bool`                          | No       | `false` | —          | Whether TLS encryption is active                                           |
| `certificateSecretRef` | `corev1.LocalObjectReference`   | No       | —       | —          | Reference to the Secret containing `tls.crt`, `tls.key`, and optionally `ca.crt` |

---

## MemcachedStatus

Defines the observed state of a Memcached cluster, updated by the reconciler via the status subresource.

| Field                | Type                   | Required | Description                                         |
|----------------------|------------------------|----------|-----------------------------------------------------|
| `conditions`         | `[]metav1.Condition`   | No       | Standard conditions with merge patch strategy (key: `type`) |
| `readyReplicas`      | `int32`                | No       | Number of Memcached pods in Ready state             |
| `observedGeneration` | `int64`                | No       | Most recent `.metadata.generation` observed by the controller |

---

## Full Example

A fully-specified Memcached CR demonstrating all available fields:

```yaml
apiVersion: memcached.c5c3.io/v1alpha1
kind: Memcached
metadata:
  name: memcached-production
  namespace: cache
spec:
  replicas: 3
  image: "memcached:1.6"
  resources:
    requests:
      cpu: "100m"
      memory: "128Mi"
    limits:
      cpu: "500m"
      memory: "256Mi"
  memcached:
    maxMemoryMB: 128
    maxConnections: 2048
    threads: 4
    maxItemSize: "2m"
    verbosity: 0
    extraArgs:
      - "-o"
      - "modern"
  highAvailability:
    antiAffinityPreset: "soft"
    topologySpreadConstraints:
      - maxSkew: 1
        topologyKey: kubernetes.io/hostname
        whenUnsatisfiable: ScheduleAnyway
        labelSelector:
          matchLabels:
            app.kubernetes.io/name: memcached-production
    podDisruptionBudget:
      enabled: true
      minAvailable: 2
  monitoring:
    enabled: true
    exporterImage: "prom/memcached-exporter:v0.15.4"
    exporterResources:
      requests:
        cpu: "50m"
        memory: "32Mi"
      limits:
        cpu: "100m"
        memory: "64Mi"
    serviceMonitor:
      interval: "30s"
      scrapeTimeout: "10s"
      additionalLabels:
        release: prometheus
  security:
    podSecurityContext:
      runAsNonRoot: true
    containerSecurityContext:
      runAsUser: 11211
      allowPrivilegeEscalation: false
    sasl:
      enabled: true
      credentialsSecretRef:
        name: memcached-sasl-credentials
    tls:
      enabled: true
      certificateSecretRef:
        name: memcached-tls-certs
```

---

## Minimal Example

A minimal CR relying on defaults:

```yaml
apiVersion: memcached.c5c3.io/v1alpha1
kind: Memcached
metadata:
  name: memcached-dev
spec:
  replicas: 1
```

This creates a single-replica Memcached pod with image `memcached:1.6`, 64 MB memory limit, 1024 max connections, 4 threads, and `1m` max item size.

[resource-reqs]: https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.31/#resourcerequirements-v1-core
[topology-spread]: https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.31/#topologyspreadconstraint-v1-core
[pod-security-context]: https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.31/#podsecuritycontext-v1-core
[security-context]: https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.31/#securitycontext-v1-core
