# Examples

Annotated Memcached CR examples for common deployment scenarios. Each example includes the complete YAML, an explanation of key field choices, and a summary of the resources the operator creates.

---

## 1. Minimal

The simplest possible Memcached CR. Relies entirely on webhook defaults for all configuration.

```yaml
apiVersion: memcached.c5c3.io/v1alpha1
kind: Memcached
metadata:
  name: memcached-dev
spec:
  replicas: 1
```

**What happens:** The defaulting webhook fills in all omitted fields:

| Field | Default Value |
|-------|---------------|
| `image` | `memcached:1.6` |
| `memcached.maxMemoryMB` | `64` |
| `memcached.maxConnections` | `1024` |
| `memcached.threads` | `4` |
| `memcached.maxItemSize` | `1m` |

**Resources created:**

- **Deployment** `memcached-dev` -- 1 replica running `memcached:1.6` with args `-m 64 -c 1024 -t 4 -I 1m`
- **Service** `memcached-dev` -- headless Service (`clusterIP: None`) with port 11211

No PDB, ServiceMonitor, or NetworkPolicy is created because those features are disabled by default.

---

## 2. High Availability

A production-grade HA setup with pod anti-affinity, topology spread across availability zones, a PodDisruptionBudget, and graceful shutdown.

```yaml
apiVersion: memcached.c5c3.io/v1alpha1
kind: Memcached
metadata:
  name: memcached-ha
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
      preStopDelaySeconds: 10
      terminationGracePeriodSeconds: 30
```

**Key field choices:**

- **`replicas: 3`** -- Three pods provide redundancy. Clients using consistent hashing distribute keys across all three.
- **`maxMemoryMB: 512`** with **`limits.memory: 1Gi`** -- Leaves 512Mi headroom above the Memcached allocation for connection buffers, threads, and OS overhead. The validating webhook enforces a minimum of `maxMemoryMB + 32Mi`, but production deployments should allow more headroom.
- **`antiAffinityPreset: soft`** -- Prefers scheduling pods on different nodes but does not block scheduling if insufficient nodes are available. Use `hard` only when you have at least as many nodes as replicas.
- **`topologySpreadConstraints`** -- Spreads pods across availability zones with `maxSkew: 1`. Using `ScheduleAnyway` avoids blocking scheduling in clusters with unbalanced zone sizes.
- **`minAvailable: 2`** -- During voluntary disruptions (e.g., node drain), at least 2 of 3 pods remain running, keeping the cache available.
- **`gracefulShutdown`** -- The preStop hook sleeps 10 seconds to allow in-flight requests to complete and clients to detect the pod leaving the headless Service endpoints. `terminationGracePeriodSeconds: 30` ensures the kubelet does not SIGKILL the pod before the hook finishes.

**Resources created:**

- **Deployment** `memcached-ha` -- 3 replicas with soft anti-affinity, topology spread, rolling update strategy (maxSurge=1, maxUnavailable=0), preStop lifecycle hook (`sleep 10`), and `terminationGracePeriodSeconds: 30`
- **Service** `memcached-ha` -- headless Service with port 11211
- **PodDisruptionBudget** `memcached-ha` -- `minAvailable: 2`

---

## 3. Monitoring-Enabled

Adds a Prometheus memcached-exporter sidecar and a ServiceMonitor for automated Prometheus scraping.

```yaml
apiVersion: memcached.c5c3.io/v1alpha1
kind: Memcached
metadata:
  name: memcached-monitored
spec:
  replicas: 2
  monitoring:
    enabled: true
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
      interval: 15s
```

**Key field choices:**

- **`monitoring.enabled: true`** -- Injects a `prom/memcached-exporter:v0.15.4` sidecar container into each pod. The exporter exposes Memcached metrics on port 9150.
- **`exporterResources`** -- Explicit resource limits for the exporter sidecar prevent it from consuming resources intended for the Memcached process.
- **`additionalLabels.release: prometheus`** -- The `kube-prometheus-stack` Helm chart selects ServiceMonitors with the label `release: prometheus` by default. Adjust this label to match your Prometheus `serviceMonitorSelector`.
- **`interval: 15s`** -- Scrape metrics every 15 seconds (default is 30s). Shorter intervals provide more granular data but increase Prometheus load.

**Resources created:**

- **Deployment** `memcached-monitored` -- 2 replicas, each with two containers: `memcached` and `exporter`
- **Service** `memcached-monitored` -- headless Service with ports 11211 (memcached) and 9150 (metrics)
- **ServiceMonitor** `memcached-monitored` -- targets the `metrics` port with 15s scrape interval

**Prerequisites:** The Prometheus Operator CRDs must be installed in the cluster. See the [troubleshooting guide](troubleshooting.md#3-servicemonitor-not-created) if the ServiceMonitor is not created.

---

## 4. TLS-Enabled

Encrypts client-to-Memcached traffic using TLS. Memcached listens on the standard port 11211 (plaintext) and an additional TLS port 11212.

### Create the TLS Secret

Generate a self-signed certificate (for testing) or use certificates issued by your PKI:

```bash
# Generate a self-signed certificate (testing only)
openssl req -x509 -nodes -days 365 -newkey rsa:2048 \
  -keyout tls.key -out tls.crt \
  -subj "/CN=memcached-tls"

# Create the Kubernetes Secret
kubectl create secret tls memcached-tls-certs -n <namespace> \
  --cert=tls.crt \
  --key=tls.key
```

For mutual TLS (mTLS), include a CA certificate:

```bash
kubectl create secret generic memcached-tls-certs -n <namespace> \
  --from-file=tls.crt=tls.crt \
  --from-file=tls.key=tls.key \
  --from-file=ca.crt=ca.crt
```

### Apply the CR

```yaml
apiVersion: memcached.c5c3.io/v1alpha1
kind: Memcached
metadata:
  name: memcached-tls
spec:
  replicas: 3
  security:
    tls:
      enabled: true
      certificateSecretRef:
        name: memcached-tls-certs
```

**Key field choices:**

- **`security.tls.enabled: true`** -- Adds the `-Z` flag and TLS certificate options to the memcached process args. The TLS certificate Secret is mounted at `/etc/memcached/tls/`.
- **`certificateSecretRef.name`** -- References the Secret containing `tls.crt` and `tls.key` keys. The Secret must exist before the CR is applied; otherwise, pods will fail to start due to a volume mount error. The validating webhook rejects the CR if this field is empty when TLS is enabled.

To also require client certificates (mTLS), add `enableClientCert: true`:

```yaml
    tls:
      enabled: true
      enableClientCert: true
      certificateSecretRef:
        name: memcached-tls-certs
```

**Resources created:**

- **Deployment** `memcached-tls` -- 3 replicas with memcached args including `-Z -o ssl_chain_cert=/etc/memcached/tls/tls.crt -o ssl_key=/etc/memcached/tls/tls.key`, a volume mount for the TLS Secret, and container ports 11211 + 11212
- **Service** `memcached-tls` -- headless Service with ports 11211 (memcached) and 11212 (memcached-tls)

---

## 5. SASL Authentication

Enables SASL authentication so that only clients with valid credentials can access the Memcached cache.

### Create the SASL Credentials Secret

The Secret must contain a `password-file` key with the SASL credentials in the format expected by memcached's `-Y` flag:

```bash
# Create the password file (username:password format)
echo -n "myuser:mypassword" > password-file

# Create the Kubernetes Secret
kubectl create secret generic memcached-sasl-credentials -n <namespace> \
  --from-file=password-file=password-file

# Clean up the local file
rm password-file
```

### Apply the CR

```yaml
apiVersion: memcached.c5c3.io/v1alpha1
kind: Memcached
metadata:
  name: memcached-sasl
spec:
  replicas: 2
  security:
    sasl:
      enabled: true
      credentialsSecretRef:
        name: memcached-sasl-credentials
```

**Key field choices:**

- **`security.sasl.enabled: true`** -- Adds the `-Y /etc/memcached/sasl/password-file` flag to the memcached process. The credentials Secret is mounted read-only at `/etc/memcached/sasl/`.
- **`credentialsSecretRef.name`** -- References the Secret containing the `password-file` key. The Secret must exist before the CR is applied. The validating webhook rejects the CR if this field is empty when SASL is enabled.

**Resources created:**

- **Deployment** `memcached-sasl` -- 2 replicas with memcached args including `-Y /etc/memcached/sasl/password-file` and a read-only volume mount for the SASL Secret
- **Service** `memcached-sasl` -- headless Service with port 11211

---

## 6. Full Production

A comprehensive production configuration combining HA, monitoring, TLS, SASL, NetworkPolicy, custom security contexts, and resource limits.

```yaml
apiVersion: memcached.c5c3.io/v1alpha1
kind: Memcached
metadata:
  name: memcached-production
  namespace: cache
spec:
  replicas: 5
  image: "memcached:1.6"
  resources:
    requests:
      cpu: "1"
      memory: 2Gi
    limits:
      cpu: "2"
      memory: 4Gi
  memcached:
    maxMemoryMB: 2048
    maxConnections: 4096
    threads: 8
    maxItemSize: "2m"
    verbosity: 0
    extraArgs:
      - "-o"
      - "modern"
  highAvailability:
    antiAffinityPreset: soft
    topologySpreadConstraints:
      - maxSkew: 1
        topologyKey: topology.kubernetes.io/zone
        whenUnsatisfiable: ScheduleAnyway
    podDisruptionBudget:
      enabled: true
      maxUnavailable: 1
    gracefulShutdown:
      enabled: true
      preStopDelaySeconds: 15
      terminationGracePeriodSeconds: 45
  monitoring:
    enabled: true
    exporterResources:
      requests:
        cpu: 50m
        memory: 32Mi
      limits:
        cpu: 200m
        memory: 64Mi
    serviceMonitor:
      additionalLabels:
        release: prometheus
      interval: 15s
      scrapeTimeout: 10s
  security:
    podSecurityContext:
      runAsNonRoot: true
      seccompProfile:
        type: RuntimeDefault
    containerSecurityContext:
      runAsUser: 11211
      runAsNonRoot: true
      allowPrivilegeEscalation: false
      readOnlyRootFilesystem: true
      capabilities:
        drop:
          - ALL
      seccompProfile:
        type: RuntimeDefault
    sasl:
      enabled: true
      credentialsSecretRef:
        name: memcached-prod-sasl
    tls:
      enabled: true
      certificateSecretRef:
        name: memcached-prod-tls
    networkPolicy:
      enabled: true
      allowedSources:
        - podSelector:
            matchLabels:
              app: api-server
        - namespaceSelector:
            matchLabels:
              kubernetes.io/metadata.name: monitoring
          podSelector:
            matchLabels:
              app.kubernetes.io/name: prometheus
```

**Key field choices:**

- **`replicas: 5`** -- Five pods provide both redundancy and throughput for high-traffic production workloads.
- **`maxMemoryMB: 2048`** with **`limits.memory: 4Gi`** -- 2 GiB dedicated to Memcached item storage with 2 GiB headroom for connections, threads, and operating overhead.
- **`threads: 8`** -- Matches the CPU limit (2 cores) with some headroom for connection parallelism.
- **`extraArgs: ["-o", "modern"]`** -- Enables modern optimizations in memcached (slab rebalancing, LRU crawler, hash algorithm improvements).
- **`maxUnavailable: 1`** -- During disruptions, at most 1 pod is unavailable. This is generally preferable to `minAvailable` for maintenance operations because it directly controls the disruption count regardless of total replicas.
- **`preStopDelaySeconds: 15`** -- A 15-second delay before termination allows endpoints controllers to remove the pod from Service endpoints and clients to detect the change.
- **Security contexts** -- Full pod hardening: non-root user (UID 11211), read-only filesystem, all capabilities dropped, seccomp RuntimeDefault profile. These settings align with Kubernetes Pod Security Standards at the `restricted` level.
- **NetworkPolicy** -- Restricts ingress to `api-server` pods in the same namespace and `prometheus` pods in the `monitoring` namespace. Ports 11211 (memcached), 11212 (TLS), and 9150 (metrics) are included automatically based on the enabled features.

**Prerequisites:**

```bash
# Create SASL credentials
echo -n "cache-user:strong-password-here" > password-file
kubectl create secret generic memcached-prod-sasl -n cache \
  --from-file=password-file=password-file
rm password-file

# Create TLS certificates
kubectl create secret tls memcached-prod-tls -n cache \
  --cert=tls.crt --key=tls.key
```

**Resources created:**

- **Deployment** `memcached-production` -- 5 replicas, 2 containers per pod (memcached + exporter), SASL + TLS volume mounts, soft anti-affinity, topology spread, graceful shutdown, full security contexts
- **Service** `memcached-production` -- headless Service with ports 11211, 11212, and 9150
- **PodDisruptionBudget** `memcached-production` -- `maxUnavailable: 1`
- **ServiceMonitor** `memcached-production` -- scrapes port 9150 every 15s
- **NetworkPolicy** `memcached-production` -- ingress restricted to specified pods on ports 11211, 11212, and 9150

---

## 7. CobaltCore/Keystone Integration

Configured specifically for [OpenStack Keystone](https://docs.openstack.org/keystone/latest/) token caching in a [CobaltCore (C5C3)](https://github.com/c5c3/c5c3) Hosted Control Plane environment. Keystone uses [pymemcache](https://pymemcache.readthedocs.io/) with the `HashClient`, which connects to individual Memcached pods via the headless Service DNS records.

```yaml
apiVersion: memcached.c5c3.io/v1alpha1
kind: Memcached
metadata:
  name: keystone-cache
  namespace: openstack
spec:
  replicas: 3
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
  highAvailability:
    antiAffinityPreset: soft
    podDisruptionBudget:
      enabled: true
      maxUnavailable: 1
    gracefulShutdown:
      enabled: true
      preStopDelaySeconds: 5
      terminationGracePeriodSeconds: 15
  monitoring:
    enabled: true
    serviceMonitor:
      additionalLabels:
        release: prometheus
      interval: 30s
  security:
    sasl:
      enabled: true
      credentialsSecretRef:
        name: keystone-memcached-sasl
    networkPolicy:
      enabled: true
      allowedSources:
        - podSelector:
            matchLabels:
              application: keystone
        - namespaceSelector:
            matchLabels:
              kubernetes.io/metadata.name: monitoring
          podSelector:
            matchLabels:
              app.kubernetes.io/name: prometheus
  service:
    annotations:
      service.alpha.kubernetes.io/tolerate-unready-endpoints: "true"
```

**Key field choices:**

- **`replicas: 3`** -- Three Memcached pods provide adequate redundancy for Keystone token caching. pymemcache's `HashClient` distributes tokens across all pods using consistent hashing.
- **Headless Service** -- The operator always creates a headless Service (`clusterIP: None`). pymemcache discovers individual pod IPs by resolving the Service DNS name (`keystone-cache.openstack.svc.cluster.local`), which returns A records for each ready pod.
- **`maxItemSize: "2m"`** -- Keystone tokens (especially Fernet tokens with large service catalogs) can exceed the default 1 MB item size limit.
- **`maxMemoryMB: 256`** -- Sufficient for most Keystone deployments. Token TTLs are typically short (hours), so the working set stays bounded.
- **SASL authentication** -- Protects the cache from unauthorized access by other workloads in the namespace.
- **NetworkPolicy** -- Restricts ingress to `keystone` pods and Prometheus. This prevents other OpenStack services in the namespace from directly accessing the Keystone cache.
- **`service.annotations`** -- The `tolerate-unready-endpoints` annotation ensures that during rolling updates, Keystone clients can still resolve DNS for pods that are terminating but still serving traffic (within the graceful shutdown window).
- **`gracefulShutdown.preStopDelaySeconds: 5`** -- A shorter delay than the HA example because Keystone token cache misses are recoverable (Keystone simply revalidates the token).

**Prerequisites:**

```bash
# Create SASL credentials for Keystone
echo -n "keystone:keystone-cache-password" > password-file
kubectl create secret generic keystone-memcached-sasl -n openstack \
  --from-file=password-file=password-file
rm password-file
```

**Keystone configuration** (`keystone.conf`):

```ini
[cache]
enabled = true
backend = dogpile.cache.pymemcache
memcache_servers = keystone-cache.openstack.svc.cluster.local:11211
```

pymemcache resolves the headless Service name to individual pod IPs automatically, enabling client-side consistent hashing across all Memcached replicas.

**Resources created:**

- **Deployment** `keystone-cache` -- 3 replicas with SASL volume mount, soft anti-affinity, graceful shutdown
- **Service** `keystone-cache` -- headless Service with ports 11211 and 9150, custom annotations
- **PodDisruptionBudget** `keystone-cache` -- `maxUnavailable: 1`
- **ServiceMonitor** `keystone-cache` -- scrapes metrics port every 30s
- **NetworkPolicy** `keystone-cache` -- ingress restricted to Keystone pods and Prometheus
