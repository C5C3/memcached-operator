# Defaulting Webhook

Reference documentation for the mutating admission webhook that sets sensible
defaults on Memcached custom resources when fields are omitted.

**Source**: `api/v1alpha1/memcached_webhook.go`

## Overview

The Memcached operator includes a mutating admission webhook that intercepts
`CREATE` and `UPDATE` requests for Memcached resources and populates omitted
fields with sensible default values. This allows cluster operators to create a
Memcached CR with only metadata and have all core spec fields populated
automatically.

The webhook uses the controller-runtime `CustomDefaulter` interface
(`admission.Defaulter[*Memcached]`). It is registered with the manager via
`SetupMemcachedWebhookWithManager` and served at the conventional Kubebuilder
webhook path.

### Defaulting Precedence

Values are resolved in the following order (highest priority first):

1. **User-specified values** — explicitly set fields in the CR spec are never
   overwritten by the webhook.
2. **Webhook defaults** — applied at admission time for omitted fields (this
   webhook).
3. **CRD schema defaults** — applied by the API server for fields with
   `+kubebuilder:default` markers, before the webhook runs.

### Webhook Path

```
/mutate-memcached-c5c3-io-v1alpha1-memcached
```

Admission type: **Mutating** (`mutating=true`)
Failure policy: **Fail** — rejects the request if the webhook is unavailable.
Side effects: **None**

---

## Defaulted Fields

The table below lists every field that the webhook defaults, its default value,
and the condition under which the default is applied.

### Core Fields (Always Defaulted)

These fields are defaulted on every Memcached resource, regardless of which
optional sections are present.

| Field | Type | Default | Condition |
|-------|------|---------|-----------|
| `spec.replicas` | `*int32` | `1` | When nil (pointer) |
| `spec.image` | `*string` | `memcached:1.6` | When nil (pointer) |
| `spec.memcached.maxMemoryMB` | `int32` | `64` | When 0 (struct initialized if nil) |
| `spec.memcached.maxConnections` | `int32` | `1024` | When 0 (struct initialized if nil) |
| `spec.memcached.threads` | `int32` | `4` | When 0 (struct initialized if nil) |
| `spec.memcached.maxItemSize` | `string` | `1m` | When empty string |
| `spec.memcached.verbosity` | `int32` | `0` | Go zero value — no action needed |

The `spec.memcached` struct is always initialized (created if nil) because its
fields are core operational parameters required by every Memcached deployment.

### Monitoring Fields (Opt-In)

These fields are only defaulted when `spec.monitoring` is already non-nil.
If the monitoring section is omitted entirely, it remains nil — the webhook
does not force-initialize optional sections.

| Field | Type | Default | Condition |
|-------|------|---------|-----------|
| `spec.monitoring.exporterImage` | `*string` | `prom/memcached-exporter:v0.15.4` | When nil and monitoring section exists |
| `spec.monitoring.serviceMonitor.interval` | `string` | `30s` | When empty and serviceMonitor section exists |
| `spec.monitoring.serviceMonitor.scrapeTimeout` | `string` | `10s` | When empty and serviceMonitor section exists |

### High Availability Fields (Opt-In)

These fields are only defaulted when `spec.highAvailability` is already non-nil.

| Field | Type | Default | Condition |
|-------|------|---------|-----------|
| `spec.highAvailability.antiAffinityPreset` | `*AntiAffinityPreset` | `soft` | When nil and highAvailability section exists |

---

## CR Examples

### Minimal CR (Empty Spec)

```yaml
apiVersion: memcached.c5c3.io/v1alpha1
kind: Memcached
metadata:
  name: my-cache
  namespace: default
spec: {}
```

After the webhook applies defaults:

```yaml
spec:
  replicas: 1
  image: "memcached:1.6"
  memcached:
    maxMemoryMB: 64
    maxConnections: 1024
    threads: 4
    maxItemSize: "1m"
    verbosity: 0
```

### Partially Specified (User Values Preserved)

```yaml
apiVersion: memcached.c5c3.io/v1alpha1
kind: Memcached
metadata:
  name: my-cache
spec:
  replicas: 3
  image: "memcached:1.6.28"
  memcached:
    maxMemoryMB: 256
```

After the webhook:

```yaml
spec:
  replicas: 3                    # preserved
  image: "memcached:1.6.28"     # preserved
  memcached:
    maxMemoryMB: 256             # preserved
    maxConnections: 1024         # defaulted
    threads: 4                   # defaulted
    maxItemSize: "1m"            # defaulted
    verbosity: 0                 # zero value
```

### With Monitoring Enabled

```yaml
apiVersion: memcached.c5c3.io/v1alpha1
kind: Memcached
metadata:
  name: my-cache
spec:
  monitoring:
    serviceMonitor: {}
```

After the webhook:

```yaml
spec:
  replicas: 1
  image: "memcached:1.6"
  memcached:
    maxMemoryMB: 64
    maxConnections: 1024
    threads: 4
    maxItemSize: "1m"
  monitoring:
    exporterImage: "prom/memcached-exporter:v0.15.4"
    serviceMonitor:
      interval: "30s"
      scrapeTimeout: "10s"
```

### With High Availability Enabled

```yaml
apiVersion: memcached.c5c3.io/v1alpha1
kind: Memcached
metadata:
  name: my-cache
spec:
  replicas: 3
  highAvailability: {}
```

After the webhook:

```yaml
spec:
  replicas: 3
  image: "memcached:1.6"
  memcached:
    maxMemoryMB: 64
    maxConnections: 1024
    threads: 4
    maxItemSize: "1m"
  highAvailability:
    antiAffinityPreset: soft
```

### Fully Specified CR (No Changes)

A CR with all fields explicitly set passes through the webhook unchanged:

```yaml
apiVersion: memcached.c5c3.io/v1alpha1
kind: Memcached
metadata:
  name: my-cache
spec:
  replicas: 5
  image: "memcached:1.6.28"
  memcached:
    maxMemoryMB: 512
    maxConnections: 2048
    threads: 8
    maxItemSize: "2m"
    verbosity: 1
  monitoring:
    exporterImage: "prom/memcached-exporter:v0.14.0"
    serviceMonitor:
      interval: "15s"
      scrapeTimeout: "5s"
  highAvailability:
    antiAffinityPreset: hard
```

---

## Nil Struct Initialization

The webhook handles nil nested structs carefully to respect the opt-in nature
of optional sections:

| Section | Nil Behavior |
|---------|-------------|
| `spec.memcached` | **Always initialized** — created and populated with defaults because memcached config is required for every deployment |
| `spec.monitoring` | **Not initialized** — remains nil; sub-field defaults only apply when the section already exists |
| `spec.highAvailability` | **Not initialized** — remains nil; sub-field defaults only apply when the section already exists |

This design means:

- Omitting `spec.monitoring` entirely produces a deployment with no exporter
  sidecar — the webhook does not opt users into monitoring.
- Omitting `spec.highAvailability` entirely produces a deployment with no
  anti-affinity rules — the webhook does not force HA settings.
- Omitting `spec.memcached` still results in a fully configured deployment
  because the webhook creates and populates the struct.

---

## Runtime Behavior

| Action | Result |
|--------|--------|
| Create CR with empty spec | Core fields defaulted; optional sections remain nil |
| Create CR with explicit values | User values preserved; only omitted fields defaulted |
| Create CR with monitoring section | Monitoring sub-fields defaulted; core fields defaulted |
| Create CR with HA section | HA sub-fields defaulted; core fields defaulted |
| Update CR clearing a field to nil/zero | Webhook re-applies the default for that field |
| Webhook unavailable | Request rejected (failurePolicy=Fail) |

---

## Implementation

The `MemcachedCustomDefaulter` struct in `api/v1alpha1/memcached_webhook.go`
implements `admission.Defaulter[*Memcached]`:

```go
type MemcachedCustomDefaulter struct{}

func (d *MemcachedCustomDefaulter) Default(ctx context.Context, mc *Memcached) error
```

Registration is handled by `SetupMemcachedWebhookWithManager`:

```go
func SetupMemcachedWebhookWithManager(mgr ctrl.Manager) error {
    return ctrl.NewWebhookManagedBy(mgr, &Memcached{}).
        WithDefaulter(&MemcachedCustomDefaulter{}).
        Complete()
}
```

This function is called from `cmd/main.go` during operator startup.

### Webhook Configuration

The kubebuilder marker on `SetupMemcachedWebhookWithManager` generates the
webhook manifest in `config/webhook/`. The marker defines:

- Path: `/mutate-memcached-c5c3-io-v1alpha1-memcached`
- Mutating: `true`
- Failure policy: `Fail`
- Side effects: `None`
- Verbs: `create`, `update`
- API group: `memcached.c5c3.io`
- API version: `v1alpha1`
