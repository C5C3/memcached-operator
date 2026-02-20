# TLS Encryption

Reference documentation for the Memcached operator's optional TLS encryption
feature that configures Memcached to accept encrypted client connections using
TLS certificates from a Kubernetes Secret.

**Source**: `internal/controller/deployment.go`, `internal/controller/service.go`, `api/v1alpha1/memcached_types.go`

## Overview

When `spec.security.tls.enabled` is `true`, the operator configures the
Memcached process with TLS encryption using certificates from a referenced
Kubernetes Secret. This enables encrypted in-transit communication between
Memcached clients and the server.

The feature configures three resources:

1. **Deployment** — the TLS certificate Secret is mounted as a volume, Memcached
   args include `-Z` (enable TLS) and `-o ssl_chain_cert`/`ssl_key` options, and
   an additional container port 11212 (`memcached-tls`) is exposed.
2. **Service** — a `memcached-tls` port (11212/TCP) is added to the headless
   Service alongside the existing `memcached` port (11211/TCP).
3. **CRD** — the `TLSSpec` struct defines `enabled`, `certificateSecretRef`, and
   `enableClientCert` fields under `spec.security.tls`.

When TLS is enabled, both plaintext (11211) and TLS (11212) ports are exposed,
allowing gradual client migration. Probes continue to use port 11211 for
simplicity.

Optionally, mutual TLS (mTLS) can be enabled via `enableClientCert: true`, which
adds the `-o ssl_ca_cert` flag so Memcached verifies client certificates using
the CA certificate from the same Secret.

The feature is opt-in — no TLS flags, volumes, mounts, or TLS port are
configured unless `spec.security.tls.enabled` is explicitly set to `true`.

---

## CRD Field Path

```text
spec.security.tls
```

Defined in `api/v1alpha1/memcached_types.go` on the `TLSSpec` struct:

```go
type TLSSpec struct {
    Enabled              bool                        `json:"enabled,omitempty"`
    CertificateSecretRef corev1.LocalObjectReference `json:"certificateSecretRef,omitempty"`
    EnableClientCert     bool                        `json:"enableClientCert,omitempty"`
}
```

| Field                  | Type                   | Required | Default | Description                                                                                                        |
|------------------------|------------------------|----------|---------|--------------------------------------------------------------------------------------------------------------------|
| `enabled`              | `bool`                 | No       | `false` | Controls whether TLS encryption is active                                                                          |
| `certificateSecretRef` | `LocalObjectReference` | No       | —       | Reference to the Secret containing `tls.crt`, `tls.key`, and optionally `ca.crt`                                   |
| `enableClientCert`     | `bool`                 | No       | `false` | When true, enables mutual TLS — Memcached requires and verifies client certificates using `ca.crt` from the Secret |

The Secret referenced by `certificateSecretRef` must contain:

| Key       | Required                           | Description                                      |
|-----------|------------------------------------|--------------------------------------------------|
| `tls.crt` | Yes                                | TLS certificate chain                            |
| `tls.key` | Yes                                | TLS private key                                  |
| `ca.crt`  | Only when `enableClientCert: true` | CA certificate for verifying client certificates |

---

## Helper Functions

### `buildTLSVolume`

```go
func buildTLSVolume(mc *Memcached) *corev1.Volume
```

Returns a `Secret` volume named `tls-certificates` that projects the TLS
certificate Secret with `tls.crt` and `tls.key` items. When `enableClientCert`
is `true`, `ca.crt` is included as an additional item.

Returns `nil` when:

- `spec.security` is nil
- `spec.security.tls` is nil
- `spec.security.tls.enabled` is `false`

### `buildTLSVolumeMount`

```go
func buildTLSVolumeMount(mc *Memcached) *corev1.VolumeMount
```

Returns a read-only `VolumeMount` named `tls-certificates` at
`/etc/memcached/tls`.

Returns `nil` when:

- `spec.security` is nil
- `spec.security.tls` is nil
- `spec.security.tls.enabled` is `false`

### TLS args in `buildMemcachedArgs`

```go
func buildMemcachedArgs(
    config *MemcachedConfig,
    sasl *SASLSpec,
    tls *TLSSpec,
) []string
```

When `tls` is non-nil and `tls.Enabled` is `true`, the following flags are
appended to the args slice:

| Flag | Value                                       | Description                                                |
|------|---------------------------------------------|------------------------------------------------------------|
| `-Z` | —                                           | Enables TLS in Memcached                                   |
| `-o` | `ssl_chain_cert=/etc/memcached/tls/tls.crt` | Path to the TLS certificate chain                          |
| `-o` | `ssl_key=/etc/memcached/tls/tls.key`        | Path to the TLS private key                                |
| `-o` | `ssl_ca_cert=/etc/memcached/tls/ca.crt`     | Path to the CA cert (only when `enableClientCert` is true) |

TLS flags are appended after SASL flags (`-Y`) when both are enabled, ensuring
both features coexist.

---

## Deployment Mapping

In `constructDeployment`, TLS configuration is applied as follows:

| CR Field                                 | Deployment Field                                                        |
|------------------------------------------|-------------------------------------------------------------------------|
| `spec.security.tls.enabled`              | Container args include `-Z`, `-o ssl_chain_cert`, `-o ssl_key`          |
| `spec.security.tls.certificateSecretRef` | `spec.template.spec.volumes[]` — Secret volume named `tls-certificates` |
| `spec.security.tls.enableClientCert`     | Container args include `-o ssl_ca_cert`; volume includes `ca.crt` item  |

Container ports when TLS is enabled:

| Port  | Name            | Protocol |
|-------|-----------------|----------|
| 11211 | `memcached`     | TCP      |
| 11212 | `memcached-tls` | TCP      |

Volume mount on the `memcached` container:

| Name               | Mount Path           | Read-Only |
|--------------------|----------------------|-----------|
| `tls-certificates` | `/etc/memcached/tls` | Yes       |

---

## Service Mapping

In `constructService`, when TLS is enabled, port 11212 is added:

| Port  | Name            | Target Port     | Protocol |
|-------|-----------------|-----------------|----------|
| 11211 | `memcached`     | `memcached`     | TCP      |
| 11212 | `memcached-tls` | `memcached-tls` | TCP      |

When TLS is disabled, only port 11211 is present. Metrics port 9150 continues to
be included independently when `spec.monitoring.enabled` is `true`.

---

## CR Examples

### TLS Enabled

```yaml
apiVersion: memcached.c5c3.io/v1alpha1
kind: Memcached
metadata:
  name: my-cache
  namespace: default
spec:
  replicas: 3
  security:
    tls:
      enabled: true
      certificateSecretRef:
        name: memcached-tls-certs
```

Produces a Deployment with:

```yaml
spec:
  template:
    spec:
      containers:
        - name: memcached
          args:
            - "-m"
            - "64"
            - "-c"
            - "1024"
            - "-t"
            - "4"
            - "-I"
            - "1m"
            - "-Z"
            - "-o"
            - "ssl_chain_cert=/etc/memcached/tls/tls.crt"
            - "-o"
            - "ssl_key=/etc/memcached/tls/tls.key"
          ports:
            - name: memcached
              containerPort: 11211
              protocol: TCP
            - name: memcached-tls
              containerPort: 11212
              protocol: TCP
          volumeMounts:
            - name: tls-certificates
              mountPath: /etc/memcached/tls
              readOnly: true
      volumes:
        - name: tls-certificates
          secret:
            secretName: memcached-tls-certs
            items:
              - key: tls.crt
                path: tls.crt
              - key: tls.key
                path: tls.key
```

And a Service with:

```yaml
spec:
  ports:
    - name: memcached
      port: 11211
      targetPort: memcached
      protocol: TCP
    - name: memcached-tls
      port: 11212
      targetPort: memcached-tls
      protocol: TCP
```

### TLS with CA Certificate (Mutual TLS)

```yaml
spec:
  replicas: 3
  security:
    tls:
      enabled: true
      certificateSecretRef:
        name: memcached-tls-certs
      enableClientCert: true
```

Adds `-o ssl_ca_cert=/etc/memcached/tls/ca.crt` to container args and includes
`ca.crt` in the Secret volume items:

```yaml
volumes:
  - name: tls-certificates
    secret:
      secretName: memcached-tls-certs
      items:
        - key: tls.crt
          path: tls.crt
        - key: tls.key
          path: tls.key
        - key: ca.crt
          path: ca.crt
```

### TLS with SASL Authentication

```yaml
spec:
  replicas: 3
  security:
    sasl:
      enabled: true
      credentialsSecretRef:
        name: memcached-sasl-creds
    tls:
      enabled: true
      certificateSecretRef:
        name: memcached-tls-certs
```

Both SASL and TLS are configured simultaneously:

- Container args include both `-Y /etc/memcached/sasl/password-file` (SASL) and
  `-Z`, `-o ssl_chain_cert`, `-o ssl_key` (TLS)
- Two volumes are mounted: `sasl-credentials` at `/etc/memcached/sasl` and
  `tls-certificates` at `/etc/memcached/tls`
- Container exposes both port 11211 (`memcached`) and port 11212
  (`memcached-tls`)

### TLS Disabled (Default)

```yaml
apiVersion: memcached.c5c3.io/v1alpha1
kind: Memcached
metadata:
  name: my-cache
spec:
  replicas: 3
```

No TLS flags, volumes, volume mounts, or TLS port are configured on the
Deployment. The Service has only the `memcached` port (11211/TCP).

---

## Runtime Behavior

| Action                          | Result                                                                                              |
|---------------------------------|-----------------------------------------------------------------------------------------------------|
| Enable TLS (`enabled: true`)    | `-Z` and `ssl_*` args added; TLS volume and mount added; port 11212 added to Deployment and Service |
| Set `enableClientCert: true`    | `-o ssl_ca_cert` arg added; `ca.crt` included in volume items                                       |
| Change `certificateSecretRef`   | Deployment updated with new Secret reference                                                        |
| Disable TLS (`enabled: false`)  | All TLS args, volume, mount, and port 11212 removed from Deployment and Service                     |
| Remove `spec.security.tls`      | Same as disabled — all TLS artifacts removed                                                        |
| Enable TLS + SASL               | Both feature sets coexist: separate volumes, mounts, and args                                       |
| Reconcile twice with same spec  | No Deployment or Service update (idempotent)                                                        |
| External drift (manual removal) | Corrected on next reconciliation cycle                                                              |

---

## Implementation

The implementation adds TLS-specific helpers to
`internal/controller/deployment.go` and extends existing functions:

1. `buildTLSVolume` — returns `*corev1.Volume` or nil
2. `buildTLSVolumeMount` — returns `*corev1.VolumeMount` or nil
3. `buildMemcachedArgs` — extended with a `tls *TLSSpec` parameter to append TLS
   flags when enabled

In `constructDeployment`:
- `buildTLSVolume` result is appended to the volumes slice when non-nil
- `buildTLSVolumeMount` result is appended to the memcached container's volume
  mounts when non-nil
- Port 11212 (`memcached-tls`) is appended to container ports when TLS is enabled
- TLS flags are included in args via `buildMemcachedArgs`

In `constructService`:
- Port 11212 (`memcached-tls`) is appended to the Service ports when
  `spec.security.tls.enabled` is `true`

No changes to the controller (`memcached_controller.go`) are needed — the
existing `reconcileDeployment` and `reconcileService` call the builder functions
which now include TLS logic.

The TLS implementation follows the same pattern as SASL authentication
(MO-0017): pure builder functions for volume/mount, extended args function, and
integration into the existing `constructDeployment` and `constructService`
functions.
