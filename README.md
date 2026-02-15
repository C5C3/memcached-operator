# Memcached Operator

A Kubernetes operator for managing Memcached clusters, built with the
[Operator SDK](https://sdk.operatorframework.io/) (Go) and
[controller-runtime](https://github.com/kubernetes-sigs/controller-runtime).

## Why This Operator Exists

This operator is part of [CobaltCore (C5C3)](https://github.com/c5c3/c5c3), a
Kubernetes-native OpenStack distribution for operating Hosted Control Planes on
bare-metal infrastructure.

CobaltCore follows a strict **operator-per-service architecture**: every
component — from MariaDB to Keystone to Nova — gets its own dedicated Kubernetes
operator. The central `c5c3-operator` orchestrates the overall lifecycle by
creating CRs that the individual operators reconcile.

Memcached is one of the four infrastructure services in the Control Plane
Cluster, deployed in **Phase 1** of the dependency graph — before any OpenStack
service can start:

```
Phase 1: Infrastructure → MariaDB, RabbitMQ, Valkey, Memcached
Phase 2: Identity       → Keystone (depends: MariaDB, Memcached)
Phase 3: Service Catalog→ K-ORC
Phase 4: Core           → Glance, Placement
Phase 5: Compute        → Nova, Neutron, Cinder
```

Its primary consumer is **Keystone**, which uses Memcached for token caching via
the `dogpile.cache.pymemcache` backend. Every Keystone instance connects to the
individual Memcached pods by address (e.g.
`memcached-0.memcached:11211, memcached-1.memcached:11211, ...`), which is why
this operator provisions a **headless Service** for direct pod discovery.

The other three infrastructure services — MariaDB
([mariadb-operator/mariadb-operator](https://github.com/mariadb-operator/mariadb-operator)),
RabbitMQ
([rabbitmq/cluster-operator](https://github.com/rabbitmq/cluster-operator)),
and Valkey
([SAP/valkey-operator](https://github.com/SAP/valkey-operator))
— all have production-ready operators. Memcached was the last gap: currently
deployed as a plain StatefulSet without lifecycle management. The CobaltCore
documentation explicitly notes: *"There is no mature production-ready operator
for Memcached."*

This operator fills that gap — intentionally kept thin, because Memcached itself
is simple. No custom failover logic (that would be fighting the architecture),
no auto-sharding (that belongs in the client), no persistence (if you need that,
use Redis/Valkey).

---

## Overview

The Memcached Operator automates the deployment, configuration, and lifecycle
management of [Memcached](https://memcached.org/) instances on Kubernetes. It
encodes operational knowledge into a Kubernetes controller so that teams can
declare their desired Memcached topology and let the operator handle the rest.

### Capabilities

- Declarative management of Memcached clusters via a Custom Resource
- Automated creation and reconciliation of Deployments, Services,
  PodDisruptionBudgets, ServiceMonitors, and NetworkPolicies
- Memcached configuration through CRD fields (memory, connections, threads, ...)
- Built-in Prometheus monitoring via a `memcached-exporter` sidecar
- High-availability primitives: pod anti-affinity, topology spread constraints,
  PodDisruptionBudgets, graceful shutdown
- Security: least-privilege RBAC, pod security contexts, optional SASL
  authentication, optional TLS encryption, NetworkPolicy generation
- Validation and defaulting webhooks

### Tech Stack

| Component           | Technology                                   |
|---------------------|----------------------------------------------|
| Language            | Go 1.24+                                     |
| Scaffolding         | Operator SDK / Kubebuilder                   |
| Runtime             | controller-runtime                           |
| CRD API group       | `memcached.c5c3.io`                          |
| Initial API version | `v1alpha1`                                   |
| Memcached image     | `memcached:1.6`                              |
| Exporter image      | `prom/memcached-exporter:v0.15.4`            |
| Testing             | envtest, KUTTL / Chainsaw                    |

---

## Architecture

```
┌─────────────────────────────────────────────────────────────────────┐
│                        Kubernetes Cluster                           │
│                                                                     │
│  ┌──────────────────────────────────────────────────────────────┐   │
│  │                    Operator Manager                          │   │
│  │                                                              │   │
│  │  ┌─────────────────────┐    ┌─────────────────────────────┐  │   │
│  │  │  MemcachedReconciler│    │  Webhook Server             │  │   │
│  │  │                     │    │  - Defaulting               │  │   │
│  │  │  Watches:           │    │  - Validation               │  │   │
│  │  │  - Memcached CR     │    └─────────────────────────────┘  │   │
│  │  │  - Deployments      │                                     │   │
│  │  │  - Services         │    ┌─────────────────────────────┐  │   │
│  │  │  - PDBs             │    │  Metrics Server             │  │   │
│  │  │  - ServiceMonitors  │    │  :8443 /metrics             │  │   │
│  │  │  - NetworkPolicies  │    └─────────────────────────────┘  │   │
│  │  └────────┬────────────┘                                     │   │
│  └───────────┼──────────────────────────────────────────────────┘   │
│              │ creates / updates / deletes                          │
│              ▼                                                      │
│  ┌──────────────────────────────────────────────────────────────┐   │
│  │                    Managed Resources                         │   │
│  │                                                              │   │
│  │  ┌────────────┐  ┌──────────┐  ┌──────┐  ┌───────────────┐   │   │
│  │  │ Deployment │  │ Service  │  │ PDB  │  │ ServiceMonitor│   │   │
│  │  │            │  │ (headless│  │      │  │               │   │   │
│  │  │ ┌────────┐ │  │ clusterIP│  │min:1 │  │ prometheus    │   │   │
│  │  │ │memcachd│ │  │ :11211   │  │      │  │ scrape config │   │   │
│  │  │ │:11211  │ │  └──────────┘  └──────┘  └───────────────┘   │   │
│  │  │ ├────────┤ │                                              │   │
│  │  │ │exporter│ │  ┌───────────────┐                           │   │
│  │  │ │:9150   │ │  │ NetworkPolicy │                           │   │
│  │  │ └────────┘ │  │               │                           │   │
│  │  └────────────┘  └───────────────┘                           │   │
│  └──────────────────────────────────────────────────────────────┘   │
└─────────────────────────────────────────────────────────────────────┘
```

### Design Principles

- **Declarative** - Users describe the desired state; the operator drives
  reality towards it.
- **Idempotent** - Every reconciliation produces the same outcome regardless of
  how many times it runs.
- **Kubernetes-native** - Uses owner references, conditions, finalizers, and
  standard API conventions.
- **Level-triggered** - Reconciliation reacts to the current state, not to a
  sequence of events.

### Watched Resources

The controller sets up watches for the following resources (all filtered to
resources owned by a `Memcached` CR):

| Resource         | Watch Reason                                         |
|------------------|------------------------------------------------------|
| `Memcached`      | Primary resource; triggers reconciliation on changes |
| `Deployment`     | Detect drift, observe rollout status                 |
| `Service`        | Detect drift                                         |
| `PDB`            | Detect drift                                         |
| `ServiceMonitor` | Detect drift (if monitoring enabled)                 |
| `NetworkPolicy`  | Detect drift (if network policy enabled)             |

---

## Custom Resource Definitions

### Memcached (v1alpha1)

The `Memcached` custom resource is the primary API of this operator. It lives
under API group `memcached.c5c3.io`, version `v1alpha1`.

#### Full Example

```yaml
apiVersion: memcached.c5c3.io/v1alpha1
kind: Memcached
metadata:
  name: my-cache
  namespace: default
spec:
  # Number of Memcached pods
  replicas: 3

  # Memcached container image (optional, has default)
  image: memcached:1.6

  # Resource requirements for the memcached container
  resources:
    requests:
      cpu: 250m
      memory: 256Mi
    limits:
      cpu: "1"
      memory: 512Mi

  # Memcached configuration
  memcached:
    # Maximum memory in megabytes (maps to -m flag)
    maxMemoryMB: 256
    # Maximum simultaneous connections (maps to -c flag)
    maxConnections: 1024
    # Number of threads (maps to -t flag)
    threads: 4
    # Maximum item size (maps to -I flag, e.g. "2m", "512k")
    maxItemSize: "1m"
    # Verbose logging level: 0=none, 1=verbose (-v), 2=very verbose (-vv)
    verbosity: 0
    # Additional command-line arguments
    extraArgs: []

  # High availability settings
  highAvailability:
    # Pod anti-affinity preset: "soft" (preferred) or "hard" (required)
    antiAffinityPreset: soft
    # Topology spread constraints
    topologySpreadConstraints:
      - maxSkew: 1
        topologyKey: topology.kubernetes.io/zone
        whenUnsatisfiable: ScheduleAnyway
    # PodDisruptionBudget configuration
    podDisruptionBudget:
      enabled: true
      minAvailable: 1

  # Monitoring configuration
  monitoring:
    enabled: true
    # Prometheus exporter sidecar image
    exporterImage: prom/memcached-exporter:v0.15.4
    # Resource requirements for the exporter container
    exporterResources:
      requests:
        cpu: 50m
        memory: 32Mi
      limits:
        cpu: 100m
        memory: 64Mi
    # ServiceMonitor configuration
    serviceMonitor:
      # Additional labels for the ServiceMonitor (e.g. release: prometheus)
      additionalLabels: {}
      # Scrape interval
      interval: 30s
      # Scrape timeout
      scrapeTimeout: 10s

  # Security settings
  security:
    # Pod security context
    podSecurityContext:
      runAsNonRoot: true
      runAsUser: 11211
      runAsGroup: 11211
      fsGroup: 11211
      seccompProfile:
        type: RuntimeDefault
    # Container security context
    containerSecurityContext:
      allowPrivilegeEscalation: false
      readOnlyRootFilesystem: true
      capabilities:
        drop:
          - ALL
    # SASL authentication (Phase 3)
    sasl:
      enabled: false
      # Secret containing SASL credentials
      credentialsSecretRef:
        name: memcached-sasl-credentials
    # TLS configuration (Phase 3)
    tls:
      enabled: false
      # Secret containing TLS certificates
      certificateSecretRef:
        name: memcached-tls

  # NetworkPolicy configuration (Phase 3)
  networkPolicy:
    enabled: false
    # Allowed ingress sources (pod/namespace selectors)
    allowedSources:
      - podSelector:
          matchLabels:
            app: my-app
      - namespaceSelector:
          matchLabels:
            team: backend

  # Service configuration
  service:
    # Additional annotations for the Service
    annotations: {}

  # Additional pod labels
  podLabels: {}
  # Additional pod annotations
  podAnnotations: {}
  # Node selector
  nodeSelector: {}
  # Tolerations
  tolerations: []
  # Image pull secrets
  imagePullSecrets: []

status:
  # Total number of desired replicas
  replicas: 3
  # Number of ready replicas
  readyReplicas: 3
  # Memcached version detected from the running pods
  memcachedVersion: "1.6.40"
  # Current number of open connections (from memcached stats)
  currentConnections: 42
  # Cache hit ratio (hits / (hits + misses), 0.0-1.0)
  hitRatio: 0.95
  # Conditions following standard Kubernetes conventions
  conditions:
    - type: Available
      status: "True"
      reason: DeploymentAvailable
      message: "Deployment has minimum availability"
      lastTransitionTime: "2025-01-15T10:30:00Z"
    - type: Progressing
      status: "False"
      reason: DeploymentComplete
      message: "Deployment has completed"
      lastTransitionTime: "2025-01-15T10:30:00Z"
    - type: Degraded
      status: "False"
      reason: AllReplicasReady
      message: "All replicas are ready"
      lastTransitionTime: "2025-01-15T10:30:00Z"
```

#### Kubebuilder Validation Markers

The CRD uses kubebuilder markers for validation and defaults:

```go
// MemcachedSpec defines the desired state of Memcached.
type MemcachedSpec struct {
    // +kubebuilder:validation:Minimum=0
    // +kubebuilder:validation:Maximum=64
    // +kubebuilder:default=1
    Replicas *int32 `json:"replicas,omitempty"`

    // +kubebuilder:default="memcached:1.6"
    Image string `json:"image,omitempty"`

    // +optional
    Resources corev1.ResourceRequirements `json:"resources,omitempty"`

    // +optional
    Memcached MemcachedConfig `json:"memcached,omitempty"`

    // +optional
    HighAvailability *HighAvailabilitySpec `json:"highAvailability,omitempty"`

    // +optional
    Monitoring *MonitoringSpec `json:"monitoring,omitempty"`

    // +optional
    Security *SecuritySpec `json:"security,omitempty"`

    // +optional
    NetworkPolicy *NetworkPolicySpec `json:"networkPolicy,omitempty"`

    // +optional
    Service *ServiceSpec `json:"service,omitempty"`

    // +optional
    PodLabels map[string]string `json:"podLabels,omitempty"`

    // +optional
    PodAnnotations map[string]string `json:"podAnnotations,omitempty"`

    // +optional
    NodeSelector map[string]string `json:"nodeSelector,omitempty"`

    // +optional
    Tolerations []corev1.Toleration `json:"tolerations,omitempty"`

    // +optional
    ImagePullSecrets []corev1.LocalObjectReference `json:"imagePullSecrets,omitempty"`
}

// MemcachedConfig holds memcached runtime parameters.
type MemcachedConfig struct {
    // +kubebuilder:validation:Minimum=16
    // +kubebuilder:validation:Maximum=65536
    // +kubebuilder:default=64
    MaxMemoryMB int32 `json:"maxMemoryMB,omitempty"`

    // +kubebuilder:validation:Minimum=1
    // +kubebuilder:validation:Maximum=65536
    // +kubebuilder:default=1024
    MaxConnections int32 `json:"maxConnections,omitempty"`

    // +kubebuilder:validation:Minimum=1
    // +kubebuilder:validation:Maximum=128
    // +kubebuilder:default=4
    Threads int32 `json:"threads,omitempty"`

    // +kubebuilder:default="1m"
    // +kubebuilder:validation:Pattern=`^[0-9]+(k|m)$`
    MaxItemSize string `json:"maxItemSize,omitempty"`

    // +kubebuilder:validation:Minimum=0
    // +kubebuilder:validation:Maximum=2
    // +kubebuilder:default=0
    Verbosity int32 `json:"verbosity,omitempty"`

    // +optional
    ExtraArgs []string `json:"extraArgs,omitempty"`
}

// HighAvailabilitySpec configures pod spreading and disruption budgets.
type HighAvailabilitySpec struct {
    // +kubebuilder:validation:Enum=soft;hard
    // +optional
    AntiAffinityPreset string `json:"antiAffinityPreset,omitempty"`

    // +optional
    TopologySpreadConstraints []corev1.TopologySpreadConstraint `json:"topologySpreadConstraints,omitempty"`

    // +optional
    PodDisruptionBudget *PDBSpec `json:"podDisruptionBudget,omitempty"`
}

// PDBSpec configures the PodDisruptionBudget.
type PDBSpec struct {
    // +kubebuilder:default=false
    Enabled bool `json:"enabled,omitempty"`

    // +optional
    MinAvailable *intstr.IntOrString `json:"minAvailable,omitempty"`

    // +optional
    MaxUnavailable *intstr.IntOrString `json:"maxUnavailable,omitempty"`
}

// MonitoringSpec configures Prometheus monitoring.
type MonitoringSpec struct {
    // +kubebuilder:default=false
    Enabled bool `json:"enabled,omitempty"`

    // +kubebuilder:default="prom/memcached-exporter:v0.15.4"
    // +optional
    ExporterImage string `json:"exporterImage,omitempty"`

    // +optional
    ExporterResources corev1.ResourceRequirements `json:"exporterResources,omitempty"`

    // +optional
    ServiceMonitor *ServiceMonitorSpec `json:"serviceMonitor,omitempty"`
}

// ServiceMonitorSpec configures the Prometheus ServiceMonitor.
type ServiceMonitorSpec struct {
    // +optional
    AdditionalLabels map[string]string `json:"additionalLabels,omitempty"`

    // +kubebuilder:default="30s"
    // +optional
    Interval string `json:"interval,omitempty"`

    // +kubebuilder:default="10s"
    // +optional
    ScrapeTimeout string `json:"scrapeTimeout,omitempty"`
}

// SecuritySpec configures pod security and optional auth/encryption.
type SecuritySpec struct {
    // +optional
    PodSecurityContext *corev1.PodSecurityContext `json:"podSecurityContext,omitempty"`

    // +optional
    ContainerSecurityContext *corev1.SecurityContext `json:"containerSecurityContext,omitempty"`

    // +optional
    SASL *SASLSpec `json:"sasl,omitempty"`

    // +optional
    TLS *TLSSpec `json:"tls,omitempty"`
}

// SASLSpec configures SASL authentication (Phase 3).
type SASLSpec struct {
    // +kubebuilder:default=false
    Enabled bool `json:"enabled,omitempty"`

    // +optional
    CredentialsSecretRef corev1.LocalObjectReference `json:"credentialsSecretRef,omitempty"`
}

// TLSSpec configures TLS encryption (Phase 3).
type TLSSpec struct {
    // +kubebuilder:default=false
    Enabled bool `json:"enabled,omitempty"`

    // +optional
    CertificateSecretRef corev1.LocalObjectReference `json:"certificateSecretRef,omitempty"`
}

// NetworkPolicySpec configures NetworkPolicy generation (Phase 3).
type NetworkPolicySpec struct {
    // +kubebuilder:default=false
    Enabled bool `json:"enabled,omitempty"`

    // +optional
    AllowedSources []networkingv1.NetworkPolicyPeer `json:"allowedSources,omitempty"`
}

// ServiceSpec configures the headless Service.
type ServiceSpec struct {
    // +optional
    Annotations map[string]string `json:"annotations,omitempty"`
}

// MemcachedStatus defines the observed state of Memcached.
type MemcachedStatus struct {
    // +optional
    Replicas int32 `json:"replicas,omitempty"`

    // +optional
    ReadyReplicas int32 `json:"readyReplicas,omitempty"`

    // +optional
    MemcachedVersion string `json:"memcachedVersion,omitempty"`

    // +optional
    CurrentConnections int32 `json:"currentConnections,omitempty"`

    // +kubebuilder:validation:Pattern=`^[0-1]\.\d{2}$`
    // +optional
    HitRatio string `json:"hitRatio,omitempty"`

    // +optional
    Conditions []metav1.Condition `json:"conditions,omitempty"`
}
```

---

## Controller Reconciliation Logic

The `MemcachedReconciler` follows a structured reconciliation flow that runs
every time the state of a watched resource changes.

### Reconciliation Flow

```
Reconcile(ctx, req)
│
├── 1. Fetch the Memcached CR
│   ├── Not found? → return (deleted, nothing to do)
│   └── Found → continue
│
├── 2. Set status condition: Progressing=True
│
├── 3. Reconcile Deployment
│   ├── Build desired Deployment spec
│   ├── CreateOrUpdate (controller-runtime controllerutil)
│   │   ├── Create if not exists
│   │   └── Update if spec differs
│   └── Set owner reference on Deployment
│
├── 4. Reconcile Service
│   ├── Build desired Service spec
│   ├── CreateOrUpdate
│   └── Set owner reference
│
├── 5. Reconcile PodDisruptionBudget (if HA enabled)
│   ├── Build desired PDB spec
│   ├── CreateOrUpdate
│   └── Set owner reference
│
├── 6. Reconcile ServiceMonitor (if monitoring enabled)
│   ├── Check if ServiceMonitor CRD exists
│   ├── Build desired ServiceMonitor spec
│   ├── CreateOrUpdate
│   └── Set owner reference
│
├── 7. Reconcile NetworkPolicy (if enabled, Phase 3)
│   ├── Build desired NetworkPolicy spec
│   ├── CreateOrUpdate
│   └── Set owner reference
│
├── 8. Update Status
│   ├── Read Deployment status (replicas, readyReplicas)
│   ├── Query memcached stats via TCP (see below)
│   ├── Compute hitRatio = hits / (hits + misses)
│   ├── Set conditions: Available, Progressing, Degraded
│   └── Patch status sub-resource
│
└── 9. Return result
    ├── Requeue after 10s if Deployment not yet ready
    └── Requeue after 60s if fully reconciled (to refresh stats)
```

### Resource Management Pattern

All managed resources follow the **create-or-update** pattern using
`controllerutil.CreateOrUpdate`:

```go
func (r *MemcachedReconciler) reconcileDeployment(
    ctx context.Context,
    mc *memcachedv1alpha1.Memcached,
) error {
    dep := &appsv1.Deployment{
        ObjectMeta: metav1.ObjectMeta{
            Name:      mc.Name,
            Namespace: mc.Namespace,
        },
    }

    op, err := controllerutil.CreateOrUpdate(ctx, r.Client, dep, func() error {
        // Set the desired state
        r.constructDeployment(mc, dep)
        // Set owner reference for garbage collection
        return controllerutil.SetControllerReference(mc, dep, r.Scheme)
    })

    if err != nil {
        return fmt.Errorf("failed to reconcile Deployment: %w", err)
    }

    log.FromContext(ctx).Info("Deployment reconciled",
        "operation", op,
        "name", dep.Name,
    )
    return nil
}
```

### Status Conditions

The operator uses standard Kubernetes conditions to communicate state:

| Condition     | Meaning                                                 |
|---------------|---------------------------------------------------------|
| `Available`   | `True` when the Deployment has minimum availability     |
| `Progressing` | `True` when a rollout or scale operation is in progress |
| `Degraded`    | `True` when fewer replicas than desired are ready       |

Condition updates use `meta.SetStatusCondition` to handle `lastTransitionTime`
correctly (only updated when the status value changes).

### Owner References and Garbage Collection

Every managed resource gets an owner reference pointing back to the `Memcached`
CR. This enables Kubernetes garbage collection: when the `Memcached` CR is
deleted, all owned resources are automatically cleaned up without requiring a
finalizer.

Finalizers are reserved for Phase 3 scenarios that involve external resource
cleanup (e.g., deregistering from an external service discovery system).

### Stats Collection for Status

The operator queries live Memcached stats to populate `currentConnections` and
`hitRatio` in the CR status. This runs as part of every reconciliation cycle.

**Protocol:** The operator opens a TCP connection to each ready pod (discovered
via the headless Service's Endpoints) and sends the memcached text protocol
`stats\r\n` command. The response is a series of `STAT <name> <value>\r\n`
lines terminated by `END\r\n`.

**Relevant stats keys:**

| Stats Key           | Used For                         |
|---------------------|----------------------------------|
| `curr_connections`  | `status.currentConnections`      |
| `get_hits`          | `status.hitRatio` (numerator)    |
| `get_misses`        | `status.hitRatio` (denominator)  |

**Aggregation across pods:**

- `currentConnections`: **sum** across all ready pods
- `hitRatio`: **weighted average** — `sum(get_hits) / (sum(get_hits) + sum(get_misses))`
  across all ready pods. Set to `"0.00"` if there are no gets yet.

**Implementation sketch:**

```go
func (r *MemcachedReconciler) queryMemcachedStats(
    ctx context.Context,
    podIPs []string,
) (totalConns int32, hitRatio string, err error) {
    var totalHits, totalMisses, conns int64

    for _, ip := range podIPs {
        stats, err := r.fetchStats(ctx, ip, 11211)
        if err != nil {
            // Log and skip unreachable pods — don't fail reconciliation
            log.FromContext(ctx).V(1).Info("Failed to query stats",
                "pod", ip, "error", err)
            continue
        }
        conns += stats.CurrConnections
        totalHits += stats.GetHits
        totalMisses += stats.GetMisses
    }

    total := totalHits + totalMisses
    if total > 0 {
        hitRatio = fmt.Sprintf("%.2f", float64(totalHits)/float64(total))
    } else {
        hitRatio = "0.00"
    }
    return int32(conns), hitRatio, nil
}

func (r *MemcachedReconciler) fetchStats(
    ctx context.Context,
    ip string,
    port int,
) (*MemcachedStats, error) {
    addr := fmt.Sprintf("%s:%d", ip, port)
    conn, err := net.DialTimeout("tcp", addr, 2*time.Second)
    if err != nil {
        return nil, err
    }
    defer conn.Close()

    conn.SetDeadline(time.Now().Add(3 * time.Second))
    fmt.Fprintf(conn, "stats\r\n")

    // Parse STAT lines until END
    scanner := bufio.NewScanner(conn)
    stats := &MemcachedStats{}
    for scanner.Scan() {
        line := scanner.Text()
        if line == "END" {
            break
        }
        // Parse "STAT <key> <value>"
        parts := strings.Fields(line)
        if len(parts) != 3 || parts[0] != "STAT" {
            continue
        }
        val, _ := strconv.ParseInt(parts[2], 10, 64)
        switch parts[1] {
        case "curr_connections":
            stats.CurrConnections = val
        case "get_hits":
            stats.GetHits = val
        case "get_misses":
            stats.GetMisses = val
        }
    }
    return stats, scanner.Err()
}
```

**Error handling:**

- Unreachable pods are **skipped** (logged at debug level). The reconciler does
  not fail because of a single unresponsive pod.
- If **no** pods are reachable, `currentConnections` is set to `0` and
  `hitRatio` to `"0.00"`.
- TCP connections use a **2s dial timeout** and **3s read deadline** to prevent
  the reconciler from blocking.

**Requeue strategy:** The reconciler always requeues — after **10s** while the
Deployment is not ready, and after **60s** when fully reconciled to keep stats
fresh.

---

## Managed Resources

The operator creates and manages the following Kubernetes resources for each
`Memcached` CR:

### Deployment

- Runs `N` replicas of the Memcached container
- Optionally includes a Prometheus exporter sidecar
- Configures liveness and readiness probes
- Sets resource requests and limits
- Applies pod security contexts
- Applies pod anti-affinity and topology spread constraints

```yaml
# Managed Deployment (simplified)
apiVersion: apps/v1
kind: Deployment
metadata:
  name: my-cache
  namespace: default
  ownerReferences:
    - apiVersion: memcached.c5c3.io/v1alpha1
      kind: Memcached
      name: my-cache
      controller: true
      blockOwnerDeletion: true
spec:
  replicas: 3
  strategy:
    type: RollingUpdate
    rollingUpdate:
      maxSurge: 1
      maxUnavailable: 0
  selector:
    matchLabels:
      app.kubernetes.io/name: memcached
      app.kubernetes.io/instance: my-cache
      app.kubernetes.io/managed-by: memcached-operator
  template:
    metadata:
      labels:
        app.kubernetes.io/name: memcached
        app.kubernetes.io/instance: my-cache
        app.kubernetes.io/managed-by: memcached-operator
    spec:
      terminationGracePeriodSeconds: 30
      securityContext:
        runAsNonRoot: true
        runAsUser: 11211
        runAsGroup: 11211
        fsGroup: 11211
        seccompProfile:
          type: RuntimeDefault
      containers:
        - name: memcached
          image: memcached:1.6
          args:
            - "-m"
            - "256"
            - "-c"
            - "1024"
            - "-t"
            - "4"
            - "-I"
            - "1m"
          ports:
            - name: memcached
              containerPort: 11211
              protocol: TCP
          livenessProbe:
            tcpSocket:
              port: memcached
            initialDelaySeconds: 10
            periodSeconds: 10
          readinessProbe:
            tcpSocket:
              port: memcached
            initialDelaySeconds: 5
            periodSeconds: 5
          resources:
            requests:
              cpu: 250m
              memory: 256Mi
            limits:
              cpu: "1"
              memory: 512Mi
          securityContext:
            allowPrivilegeEscalation: false
            readOnlyRootFilesystem: true
            capabilities:
              drop:
                - ALL
          lifecycle:
            preStop:
              exec:
                command:
                  - /bin/sh
                  - -c
                  - "sleep 5"
        - name: exporter
          image: prom/memcached-exporter:v0.15.4
          ports:
            - name: metrics
              containerPort: 9150
              protocol: TCP
          resources:
            requests:
              cpu: 50m
              memory: 32Mi
            limits:
              cpu: 100m
              memory: 64Mi
          securityContext:
            allowPrivilegeEscalation: false
            readOnlyRootFilesystem: true
            capabilities:
              drop:
                - ALL
```

### Headless Service

- Headless service (`clusterIP: None`) exposing port 11211 (memcached) and 9150
  (metrics)
- Enables clients to discover individual pod IPs for consistent hashing
- Selector matches the Deployment pods

```yaml
apiVersion: v1
kind: Service
metadata:
  name: my-cache
  namespace: default
  labels:
    app.kubernetes.io/name: memcached
    app.kubernetes.io/instance: my-cache
    app.kubernetes.io/managed-by: memcached-operator
spec:
  clusterIP: None
  ports:
    - name: memcached
      port: 11211
      targetPort: memcached
      protocol: TCP
    - name: metrics
      port: 9150
      targetPort: metrics
      protocol: TCP
  selector:
    app.kubernetes.io/name: memcached
    app.kubernetes.io/instance: my-cache
    app.kubernetes.io/managed-by: memcached-operator
```

### PodDisruptionBudget

- Created when `spec.highAvailability.podDisruptionBudget.enabled` is `true`
- Ensures minimum availability during voluntary disruptions (node drains,
  upgrades)

```yaml
apiVersion: policy/v1
kind: PodDisruptionBudget
metadata:
  name: my-cache
  namespace: default
spec:
  minAvailable: 1
  selector:
    matchLabels:
      app.kubernetes.io/name: memcached
      app.kubernetes.io/instance: my-cache
      app.kubernetes.io/managed-by: memcached-operator
```

### ServiceMonitor

- Created when `spec.monitoring.enabled` is `true`
- Requires the Prometheus Operator CRDs to be installed

```yaml
apiVersion: monitoring.coreos.com/v1
kind: ServiceMonitor
metadata:
  name: my-cache
  namespace: default
  labels:
    app.kubernetes.io/name: memcached
    app.kubernetes.io/instance: my-cache
    app.kubernetes.io/managed-by: memcached-operator
spec:
  selector:
    matchLabels:
      app.kubernetes.io/name: memcached
      app.kubernetes.io/instance: my-cache
      app.kubernetes.io/managed-by: memcached-operator
  endpoints:
    - port: metrics
      interval: 30s
      scrapeTimeout: 10s
```

### NetworkPolicy (Phase 3)

- Created when `spec.networkPolicy.enabled` is `true`
- Restricts ingress traffic to allowed sources only

```yaml
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: my-cache
  namespace: default
spec:
  podSelector:
    matchLabels:
      app.kubernetes.io/name: memcached
      app.kubernetes.io/instance: my-cache
      app.kubernetes.io/managed-by: memcached-operator
  policyTypes:
    - Ingress
  ingress:
    - from:
        - podSelector:
            matchLabels:
              app: my-app
        - namespaceSelector:
            matchLabels:
              team: backend
      ports:
        - port: 11211
          protocol: TCP
```

---

## Memcached Configuration

The operator translates CRD fields into `memcached` command-line arguments.

### Parameter Mapping

| CRD Field                    | Memcached Flag | Default | Description                     |
|------------------------------|----------------|---------|---------------------------------|
| `spec.memcached.maxMemoryMB` | `-m`           | `64`    | Maximum memory in megabytes     |
| `spec.memcached.maxConnections` | `-c`        | `1024`  | Maximum simultaneous connections|
| `spec.memcached.threads`     | `-t`           | `4`     | Number of worker threads        |
| `spec.memcached.maxItemSize` | `-I`           | `1m`    | Maximum item size               |
| `spec.memcached.verbosity`   | `-v` / `-vv`   | `0`     | Logging verbosity (0, 1, or 2)  |
| `spec.memcached.extraArgs`   | (raw)          | `[]`    | Additional raw arguments        |

### Container Args Construction

The controller builds the container arguments as follows:

```go
func buildMemcachedArgs(config MemcachedConfig) []string {
    args := []string{
        "-m", strconv.Itoa(int(config.MaxMemoryMB)),
        "-c", strconv.Itoa(int(config.MaxConnections)),
        "-t", strconv.Itoa(int(config.Threads)),
        "-I", config.MaxItemSize,
    }

    // Verbosity: -v for 1, -vv for 2
    switch config.Verbosity {
    case 1:
        args = append(args, "-v")
    case 2:
        args = append(args, "-vv")
    }

    args = append(args, config.ExtraArgs...)
    return args
}
```

---

## High Availability

### Pod Anti-Affinity

The operator supports two anti-affinity presets to spread Memcached pods across
nodes:

**Soft (preferred, default)** - Best-effort spreading; pods prefer different
nodes but can be co-located if necessary:

```yaml
affinity:
  podAntiAffinity:
    preferredDuringSchedulingIgnoredDuringExecution:
      - weight: 100
        podAffinityTerm:
          labelSelector:
            matchLabels:
              app.kubernetes.io/name: memcached
              app.kubernetes.io/instance: my-cache
          topologyKey: kubernetes.io/hostname
```

**Hard (required)** - Strict spreading; pods must be on different nodes:

```yaml
affinity:
  podAntiAffinity:
    requiredDuringSchedulingIgnoredDuringExecution:
      - labelSelector:
          matchLabels:
            app.kubernetes.io/name: memcached
            app.kubernetes.io/instance: my-cache
        topologyKey: kubernetes.io/hostname
```

### Topology Spread Constraints

For zone-aware scheduling, the CRD accepts standard Kubernetes
`topologySpreadConstraints` that are applied directly to the pod spec:

```yaml
spec:
  highAvailability:
    topologySpreadConstraints:
      - maxSkew: 1
        topologyKey: topology.kubernetes.io/zone
        whenUnsatisfiable: ScheduleAnyway
```

### PodDisruptionBudget

When enabled, the operator creates a PDB that guarantees a minimum number of
pods remain available during voluntary disruptions:

```yaml
spec:
  highAvailability:
    podDisruptionBudget:
      enabled: true
      minAvailable: 1   # or use maxUnavailable: 1
```

### Graceful Shutdown

The Deployment template includes a `preStop` lifecycle hook that gives in-flight
requests time to complete before the container stops:

```yaml
lifecycle:
  preStop:
    exec:
      command:
        - /bin/sh
        - -c
        - "sleep 5"
```

Combined with an appropriate `terminationGracePeriodSeconds` (default: 30s),
this prevents dropped connections during rolling updates and node drains.

---

## Monitoring and Observability

### Prometheus Exporter Sidecar

When `spec.monitoring.enabled` is `true`, the operator injects a
[memcached-exporter](https://github.com/prometheus/memcached_exporter) sidecar
container into the pod. The exporter connects to `localhost:11211` and exposes
Prometheus metrics on port `9150`.

### Key Memcached Metrics

| Metric                                    | Description                              |
|-------------------------------------------|------------------------------------------|
| `memcached_current_connections`           | Number of open connections               |
| `memcached_commands_total`                | Total commands by type (get, set, etc.)  |
| `memcached_current_bytes`                 | Current bytes used for item storage      |
| `memcached_limit_bytes`                   | Maximum bytes allowed for storage        |
| `memcached_items_evicted_total`           | Total items evicted                      |
| `memcached_items_total`                   | Total items stored                       |
| `memcached_read_bytes_total`              | Total bytes read from network            |
| `memcached_written_bytes_total`           | Total bytes written to network           |
| `memcached_get_hits_total`                | Total cache hits                         |
| `memcached_get_misses_total`              | Total cache misses                       |
| `memcached_up`                            | Whether the memcached server is up       |

### Operator-Level Metrics

The operator itself exposes controller-runtime metrics on `:8443/metrics`:

| Metric                                      | Description                            |
|---------------------------------------------|----------------------------------------|
| `controller_runtime_reconcile_total`        | Total reconciliation attempts          |
| `controller_runtime_reconcile_errors_total` | Total reconciliation errors            |
| `controller_runtime_reconcile_time_seconds` | Reconciliation duration histogram      |
| `workqueue_depth`                           | Current depth of the work queue        |
| `workqueue_adds_total`                      | Total items added to the work queue    |

### ServiceMonitor

The operator creates a `ServiceMonitor` resource (if the Prometheus Operator
CRDs are present) to enable automatic scrape target discovery. The controller
checks for CRD availability at reconciliation time and skips ServiceMonitor
creation if the CRD is not installed.

---

## Security

### RBAC (Least Privilege)

The operator requests only the minimum permissions required:

```yaml
# ClusterRole for the operator
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: memcached-operator-manager-role
rules:
  # Core resources
  - apiGroups: [""]
    resources: ["services"]
    verbs: ["get", "list", "watch", "create", "update", "patch", "delete"]
  - apiGroups: ["apps"]
    resources: ["deployments"]
    verbs: ["get", "list", "watch", "create", "update", "patch", "delete"]
  - apiGroups: ["policy"]
    resources: ["poddisruptionbudgets"]
    verbs: ["get", "list", "watch", "create", "update", "patch", "delete"]

  # Custom resources
  - apiGroups: ["memcached.c5c3.io"]
    resources: ["memcacheds"]
    verbs: ["get", "list", "watch", "create", "update", "patch", "delete"]
  - apiGroups: ["memcached.c5c3.io"]
    resources: ["memcacheds/status"]
    verbs: ["get", "update", "patch"]
  - apiGroups: ["memcached.c5c3.io"]
    resources: ["memcacheds/finalizers"]
    verbs: ["update"]

  # Monitoring (optional)
  - apiGroups: ["monitoring.coreos.com"]
    resources: ["servicemonitors"]
    verbs: ["get", "list", "watch", "create", "update", "patch", "delete"]

  # NetworkPolicy (Phase 3)
  - apiGroups: ["networking.k8s.io"]
    resources: ["networkpolicies"]
    verbs: ["get", "list", "watch", "create", "update", "patch", "delete"]

  # Leader election
  - apiGroups: ["coordination.k8s.io"]
    resources: ["leases"]
    verbs: ["get", "list", "watch", "create", "update", "patch", "delete"]
  - apiGroups: [""]
    resources: ["events"]
    verbs: ["create", "patch"]
```

### Pod Security Contexts

Default security contexts enforce the principle of least privilege:

- `runAsNonRoot: true` - Containers never run as root
- `runAsUser: 11211` - Memcached's dedicated UID
- `readOnlyRootFilesystem: true` - Prevents filesystem writes
- `allowPrivilegeEscalation: false` - Blocks privilege escalation
- `capabilities.drop: [ALL]` - Drops all Linux capabilities
- `seccompProfile.type: RuntimeDefault` - Uses the default seccomp profile

### SASL Authentication (Phase 3)

Optional SASL authentication can be enabled to require clients to authenticate:

```yaml
spec:
  security:
    sasl:
      enabled: true
      credentialsSecretRef:
        name: memcached-sasl-credentials
```

The referenced Secret must contain a `memcached-sasl-db` key with the SASL
password database. The operator mounts this Secret into the container and adds
the `-S` flag to enable SASL.

### TLS Encryption (Phase 3)

Optional TLS encryption for client connections:

```yaml
spec:
  security:
    tls:
      enabled: true
      certificateSecretRef:
        name: memcached-tls
```

The referenced Secret must contain `tls.crt`, `tls.key`, and optionally
`ca.crt`. The operator mounts the certificates and configures memcached with
`--enable-ssl`, `--ssl-cert`, `--ssl-key`, and `--ssl-ca-cert` flags.

### NetworkPolicy Generation (Phase 3)

When `spec.networkPolicy.enabled` is `true`, the operator creates a
`NetworkPolicy` that restricts ingress traffic to only the specified sources,
locking down access to the Memcached port (11211).

### Validation and Defaulting Webhooks

The operator implements admission webhooks for:

- **Defaulting webhook** - Sets default values for optional fields (replicas,
  image, memory, etc.) that are not already handled by CRD-level defaults
- **Validation webhook** - Enforces complex validation rules that cannot be
  expressed via kubebuilder markers alone, such as:
  - `spec.memcached.maxMemoryMB` must not exceed the container memory limit
  - `spec.highAvailability.podDisruptionBudget.minAvailable` must be less than
    `spec.replicas`
  - TLS Secret references are valid when TLS is enabled

---

## Testing Strategy

### Unit Tests

Standard Go unit tests for:

- Memcached argument construction
- Resource builder functions (Deployment, Service, PDB construction)
- Status condition logic
- Webhook validation logic
- Helper/utility functions

```bash
make test-unit
```

### Integration Tests (envtest)

Integration tests using the controller-runtime
[envtest](https://book.kubebuilder.io/reference/envtest.html) framework, which
runs a real API server and etcd but no kubelet:

- Full reconciliation loop tests
- Create a Memcached CR and verify all managed resources are created correctly
- Update the CR and verify managed resources are updated
- Delete the CR and verify managed resources are garbage collected
- Status condition transitions
- Webhook admission tests

```bash
make test
```

### End-to-End Tests (KUTTL / Chainsaw)

E2E tests that run against a real cluster (e.g., kind) using
[KUTTL](https://kuttl.dev/) or
[Chainsaw](https://kyverno.github.io/chainsaw/):

- Deploy the operator to a kind cluster
- Create Memcached CRs with various configurations
- Verify pods are running and memcached is accessible
- Test scaling, updates, and deletion
- Test monitoring integration (if Prometheus Operator is present)
- Test failure scenarios (pod deletion, node drain simulation)

```bash
make test-e2e
```

### Test Coverage Targets

| Test Type   | Coverage Target                                      |
|-------------|------------------------------------------------------|
| Unit        | All exported functions, argument builders, validators |
| Integration | All reconciliation paths, status transitions          |
| E2E         | Key user workflows, failure recovery                  |

---

## Deployment

### Local Development

Run the operator outside the cluster, connecting to the current kubeconfig
context:

```bash
# Install CRDs
make install

# Run the operator locally
make run
```

### Cluster Deployment

Build and deploy the operator as a container in the cluster:

```bash
# Build the operator image
make docker-build IMG=ghcr.io/c5c3/memcached-operator:latest

# Push the image
make docker-push IMG=ghcr.io/c5c3/memcached-operator:latest

# Deploy to the cluster
make deploy IMG=ghcr.io/c5c3/memcached-operator:latest
```

### OLM Bundle (Phase 3)

For clusters using the Operator Lifecycle Manager:

```bash
# Generate the OLM bundle
make bundle IMG=ghcr.io/c5c3/memcached-operator:latest

# Build the bundle image
make bundle-build BUNDLE_IMG=ghcr.io/c5c3/memcached-operator-bundle:latest

# Push the bundle image
make bundle-push BUNDLE_IMG=ghcr.io/c5c3/memcached-operator-bundle:latest

# Run the bundle on a cluster
operator-sdk run bundle ghcr.io/c5c3/memcached-operator-bundle:latest
```

---

## Implementation Phases

### Phase 1: MVP

**Goal:** A working operator that manages a basic Memcached deployment.

**Deliverables:**

- Project scaffolding with Operator SDK (`operator-sdk init`, `operator-sdk
  create api`)
- `Memcached` CRD (`v1alpha1`) with core fields: `replicas`, `image`,
  `resources`, `memcached` (config block)
- `MemcachedReconciler` implementing the reconciliation loop
- Managed resources: Deployment and Service
- Owner references on all managed resources
- Status updates: `replicas`, `readyReplicas`, `currentConnections`,
  `hitRatio`, `conditions` (`Available`, `Progressing`, `Degraded`)
- Standard Kubernetes labels on all resources
- Unit tests for resource builders and argument construction
- Integration tests (envtest) for the full reconciliation loop
- `Makefile` targets: `install`, `run`, `test`, `test-unit`, `docker-build`,
  `docker-push`, `deploy`
- Basic README updates

**Exit Criteria:**

- `make test` passes
- Creating a `Memcached` CR results in a running Deployment and Service
- Updating `spec.replicas` scales the Deployment
- Deleting the CR removes all managed resources
- Status reflects the current state accurately

### Phase 2: Production Hardening

**Goal:** Make the operator production-ready with monitoring, HA, security, and
webhooks.

**Deliverables:**

- Full `MemcachedSpec` with all fields (HA, monitoring, security, service, pod
  metadata, scheduling)
- Defaulting and validation webhooks
- Prometheus exporter sidecar injection
- ServiceMonitor creation (with CRD existence check)
- PodDisruptionBudget management
- Pod anti-affinity presets (soft / hard)
- Topology spread constraints pass-through
- Graceful shutdown (preStop hook)
- Pod security contexts (non-root, read-only filesystem, drop capabilities)
- Liveness and readiness probes
- Operator-level metrics
- E2E test suite (KUTTL or Chainsaw)
- Comprehensive documentation

**Exit Criteria:**

- All Phase 1 exit criteria still pass
- Webhooks correctly default and validate CRs
- Monitoring sidecar is injected when enabled
- PDB is created when configured
- Security contexts are applied correctly
- E2E tests pass on a kind cluster

### Phase 3: Advanced Features

**Goal:** Add enterprise features and advanced integrations.

**Deliverables:**

- SASL authentication support
- TLS encryption support
- NetworkPolicy generation
- OLM bundle and catalog
- Finalizers for external resource cleanup
- Advanced status reporting (memcached version detection)
- Helm chart alternative deployment

**Exit Criteria:**

- SASL authentication prevents unauthenticated access
- TLS encrypts client-server communication
- NetworkPolicy restricts traffic to allowed sources
- OLM bundle installs and upgrades correctly

---

## Development Guide

### Prerequisites

- Go 1.24+
- Docker or Podman
- kubectl
- Access to a Kubernetes cluster (or kind/minikube for local development)
- [Operator SDK](https://sdk.operatorframework.io/docs/installation/) v1.42+

### Project Setup

```bash
# Initialize the project
operator-sdk init \
  --domain c5c3.io \
  --repo github.com/c5c3/memcached-operator

# Create the Memcached API and controller
operator-sdk create api \
  --group memcached \
  --version v1alpha1 \
  --kind Memcached \
  --resource --controller

# Generate CRD manifests and RBAC
make manifests

# Generate Go code (DeepCopy, etc.)
make generate
```

### Build and Deploy Workflow

```bash
# Edit types in api/v1alpha1/memcached_types.go
# Edit controller in internal/controller/memcached_controller.go

# Regenerate after type changes
make generate manifests

# Run tests
make test

# Install CRDs and run locally
make install
make run

# In another terminal, create a sample CR
kubectl apply -f config/samples/memcached_v1alpha1_memcached.yaml

# Verify
kubectl get memcached
kubectl get deployments
kubectl get services
kubectl get pods
```

### Project Structure

```
memcached-operator/
├── api/
│   └── v1alpha1/
│       ├── memcached_types.go         # CRD type definitions
│       ├── memcached_webhook.go       # Webhooks (Phase 2)
│       ├── groupversion_info.go       # API group metadata
│       └── zz_generated.deepcopy.go   # Generated DeepCopy
├── cmd/
│   └── main.go                        # Operator entrypoint
├── config/
│   ├── crd/                           # Generated CRD manifests
│   ├── manager/                       # Operator Deployment manifests
│   ├── rbac/                          # RBAC manifests
│   ├── samples/                       # Sample CRs
│   └── webhook/                       # Webhook configuration
├── internal/
│   └── controller/
│       ├── memcached_controller.go    # Reconciler implementation
│       └── memcached_controller_test.go
├── test/
│   └── e2e/                           # E2E test cases
├── Dockerfile
├── Makefile
├── go.mod
├── go.sum
└── README.md
```

---

## API Reference

### MemcachedSpec

| Field              | Type                          | Default          | Description                                           |
|--------------------|-------------------------------|------------------|-------------------------------------------------------|
| `replicas`         | `*int32`                      | `1`              | Number of Memcached replicas (0-64)                   |
| `image`            | `string`                      | `memcached:1.6`  | Container image for Memcached                         |
| `resources`        | `ResourceRequirements`        | -                | CPU/memory requests and limits                        |
| `memcached`        | `MemcachedConfig`             | see below        | Memcached runtime configuration                       |
| `highAvailability` | `*HighAvailabilitySpec`       | -                | HA settings (anti-affinity, PDB, topology spread)     |
| `monitoring`       | `*MonitoringSpec`             | -                | Prometheus monitoring configuration                   |
| `security`         | `*SecuritySpec`               | -                | Security contexts, SASL, TLS                          |
| `networkPolicy`    | `*NetworkPolicySpec`          | -                | NetworkPolicy generation settings                     |
| `service`          | `*ServiceSpec`                | -                | Headless Service annotations                          |
| `podLabels`        | `map[string]string`           | -                | Additional labels for pods                            |
| `podAnnotations`   | `map[string]string`           | -                | Additional annotations for pods                       |
| `nodeSelector`     | `map[string]string`           | -                | Node selection constraints                            |
| `tolerations`      | `[]Toleration`                | -                | Pod tolerations                                       |
| `imagePullSecrets` | `[]LocalObjectReference`      | -                | Image pull secrets                                    |

### MemcachedConfig

| Field            | Type       | Default | Validation             | Description                          |
|------------------|------------|---------|------------------------|--------------------------------------|
| `maxMemoryMB`    | `int32`    | `64`    | min=16, max=65536      | Maximum memory in MB (`-m`)          |
| `maxConnections` | `int32`    | `1024`  | min=1, max=65536       | Maximum connections (`-c`)           |
| `threads`        | `int32`    | `4`     | min=1, max=128         | Worker threads (`-t`)                |
| `maxItemSize`    | `string`   | `1m`    | pattern=`^[0-9]+(k|m)$`| Maximum item size (`-I`)            |
| `verbosity`      | `int32`    | `0`     | min=0, max=2           | Log verbosity (0=none, 1=-v, 2=-vv) |
| `extraArgs`      | `[]string` | `[]`    | -                      | Additional command-line arguments     |

### HighAvailabilitySpec

| Field                        | Type                             | Default | Description                              |
|------------------------------|----------------------------------|---------|------------------------------------------|
| `antiAffinityPreset`         | `string`                         | -       | `soft` (preferred) or `hard` (required)  |
| `topologySpreadConstraints`  | `[]TopologySpreadConstraint`     | -       | Standard topology spread constraints     |
| `podDisruptionBudget`        | `*PDBSpec`                       | -       | PDB configuration                        |

### PDBSpec

| Field            | Type                          | Default | Description                            |
|------------------|-------------------------------|---------|----------------------------------------|
| `enabled`        | `bool`                        | `false` | Whether to create a PDB                |
| `minAvailable`   | `*intstr.IntOrString`         | -       | Minimum available pods (int or %)      |
| `maxUnavailable` | `*intstr.IntOrString`         | -       | Maximum unavailable pods (int or %)    |

### MonitoringSpec

| Field               | Type                     | Default                            | Description                      |
|---------------------|--------------------------|------------------------------------|----------------------------------|
| `enabled`           | `bool`                   | `false`                            | Enable Prometheus monitoring      |
| `exporterImage`     | `string`                 | `prom/memcached-exporter:v0.15.4`  | Exporter sidecar image           |
| `exporterResources` | `ResourceRequirements`   | -                                  | Exporter resource requests/limits |
| `serviceMonitor`    | `*ServiceMonitorSpec`    | -                                  | ServiceMonitor configuration      |

### ServiceMonitorSpec

| Field              | Type                | Default | Description                           |
|--------------------|---------------------|---------|---------------------------------------|
| `additionalLabels` | `map[string]string` | -       | Extra labels on the ServiceMonitor    |
| `interval`         | `string`            | `30s`   | Prometheus scrape interval            |
| `scrapeTimeout`    | `string`            | `10s`   | Prometheus scrape timeout             |

### SecuritySpec

| Field                      | Type                      | Default | Description                     |
|----------------------------|---------------------------|---------|---------------------------------|
| `podSecurityContext`       | `*PodSecurityContext`     | -       | Pod-level security context      |
| `containerSecurityContext` | `*SecurityContext`        | -       | Container-level security context|
| `sasl`                     | `*SASLSpec`               | -       | SASL authentication (Phase 3)  |
| `tls`                      | `*TLSSpec`                | -       | TLS encryption (Phase 3)       |

### MemcachedStatus

| Field                | Type                 | Description                                      |
|----------------------|----------------------|--------------------------------------------------|
| `replicas`           | `int32`              | Total desired replicas                           |
| `readyReplicas`      | `int32`              | Number of ready replicas                         |
| `memcachedVersion`   | `string`             | Detected Memcached version                       |
| `currentConnections` | `int32`              | Current number of open connections               |
| `hitRatio`           | `string`             | Cache hit ratio (hits / (hits + misses), 0.0-1.0)|
| `conditions`         | `[]metav1.Condition` | Standard Kubernetes conditions                   |

---

## Examples

### Basic Memcached Instance

```yaml
apiVersion: memcached.c5c3.io/v1alpha1
kind: Memcached
metadata:
  name: basic-cache
  namespace: default
spec:
  replicas: 1
  memcached:
    maxMemoryMB: 64
```

### High-Availability Configuration

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
  monitoring:
    enabled: true
    serviceMonitor:
      additionalLabels:
        release: prometheus
      interval: 15s
```

### TLS-Enabled Configuration (Phase 3)

```yaml
apiVersion: memcached.c5c3.io/v1alpha1
kind: Memcached
metadata:
  name: secure-cache
  namespace: production
spec:
  replicas: 3
  memcached:
    maxMemoryMB: 256
  security:
    sasl:
      enabled: true
      credentialsSecretRef:
        name: memcached-sasl-credentials
    tls:
      enabled: true
      certificateSecretRef:
        name: memcached-tls
  networkPolicy:
    enabled: true
    allowedSources:
      - podSelector:
          matchLabels:
            app: api-server
```

### Production Configuration

```yaml
apiVersion: memcached.c5c3.io/v1alpha1
kind: Memcached
metadata:
  name: prod-cache
  namespace: production
spec:
  replicas: 5
  image: memcached:1.6
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
  highAvailability:
    antiAffinityPreset: hard
    topologySpreadConstraints:
      - maxSkew: 1
        topologyKey: topology.kubernetes.io/zone
        whenUnsatisfiable: DoNotSchedule
    podDisruptionBudget:
      enabled: true
      maxUnavailable: 1
  monitoring:
    enabled: true
    exporterResources:
      requests:
        cpu: 100m
        memory: 64Mi
      limits:
        cpu: 200m
        memory: 128Mi
    serviceMonitor:
      additionalLabels:
        release: kube-prometheus-stack
      interval: 15s
      scrapeTimeout: 5s
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
  service:
    annotations:
      prometheus.io/scrape: "true"
      prometheus.io/port: "9150"
  podLabels:
    environment: production
    team: platform
  podAnnotations:
    cluster-autoscaler.kubernetes.io/safe-to-evict: "false"
  nodeSelector:
    node-role.kubernetes.io/cache: ""
  tolerations:
    - key: dedicated
      value: cache
      effect: NoSchedule
```

---

## Troubleshooting

### Memcached CR Stuck in Progressing State

**Symptom:** The `Progressing` condition remains `True` and `Available` stays
`False`.

**Diagnosis:**

```bash
# Check the Memcached CR status
kubectl get memcached my-cache -o yaml

# Check the managed Deployment
kubectl get deployment my-cache -o yaml

# Check pod status and events
kubectl get pods -l app.kubernetes.io/instance=my-cache
kubectl describe pod <pod-name>

# Check operator logs
kubectl logs -n memcached-operator-system deployment/memcached-operator-controller-manager
```

**Common Causes:**

- Insufficient cluster resources (CPU/memory) for the requested resources
- Image pull errors (wrong image name, missing pull secrets)
- Node selector or tolerations preventing scheduling
- Hard anti-affinity with fewer nodes than replicas

### Pods CrashLooping

**Symptom:** Memcached pods are in `CrashLoopBackOff` state.

**Diagnosis:**

```bash
kubectl logs <pod-name> -c memcached
kubectl logs <pod-name> -c exporter
```

**Common Causes:**

- `maxMemoryMB` exceeds the container memory limit (OOMKilled)
- Invalid `extraArgs` passed to memcached
- SASL credentials Secret not found or malformed
- TLS certificate Secret not found or invalid

### ServiceMonitor Not Created

**Symptom:** `monitoring.enabled: true` but no ServiceMonitor appears.

**Diagnosis:**

```bash
# Check if the ServiceMonitor CRD exists
kubectl get crd servicemonitors.monitoring.coreos.com

# Check operator logs for CRD detection
kubectl logs -n memcached-operator-system deployment/memcached-operator-controller-manager | grep -i servicemonitor
```

**Fix:** Install the Prometheus Operator CRDs. The operator checks for CRD
existence and skips ServiceMonitor creation if the CRD is not installed.

### Webhook Admission Errors

**Symptom:** Creating or updating a Memcached CR fails with an admission error.

**Diagnosis:**

```bash
# The error message from kubectl will indicate the validation failure
kubectl apply -f memcached.yaml
```

**Common Causes:**

- `maxMemoryMB` exceeds the memory limit in `resources.limits.memory`
- `minAvailable` in PDB is greater than or equal to `replicas`
- TLS or SASL enabled without the required Secret references
- `replicas` outside the valid range (0-64)

### Operator Not Reconciling

**Symptom:** Changes to Memcached CRs are not reflected in managed resources.

**Diagnosis:**

```bash
# Check if the operator is running
kubectl get pods -n memcached-operator-system

# Check operator logs for errors
kubectl logs -n memcached-operator-system deployment/memcached-operator-controller-manager

# Check RBAC
kubectl auth can-i list deployments --as system:serviceaccount:memcached-operator-system:memcached-operator-controller-manager
```

**Common Causes:**

- Operator pod not running or crash-looping
- RBAC permissions missing or incorrect
- Leader election issues (if running multiple replicas)
- CRDs not installed (`make install`)

### Metrics Not Available

**Symptom:** Prometheus cannot scrape memcached metrics.

**Diagnosis:**

```bash
# Check if the exporter sidecar is running
kubectl get pods -l app.kubernetes.io/instance=my-cache -o jsonpath='{.items[0].spec.containers[*].name}'

# Port-forward and test the metrics endpoint
kubectl port-forward svc/my-cache 9150:9150
curl http://localhost:9150/metrics

# Check the ServiceMonitor
kubectl get servicemonitor my-cache -o yaml

# Verify Prometheus targets
# (in the Prometheus UI, check Status -> Targets)
```

**Common Causes:**

- `monitoring.enabled` is `false`
- ServiceMonitor labels do not match the Prometheus `serviceMonitorSelector`
- Network policies blocking the metrics port
- Exporter container failing (check its logs)
