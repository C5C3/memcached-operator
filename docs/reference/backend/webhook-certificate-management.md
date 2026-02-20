# Webhook Certificate Management

Reference documentation for TLS certificate provisioning and rotation for the
operator's admission webhooks using cert-manager.

**Source**: `config/certmanager/`, `config/default/kustomization.yaml`

## Overview

The Memcached operator exposes mutating and validating admission webhooks that
the Kubernetes API server calls over HTTPS. These webhooks require a valid TLS
certificate that the API server trusts. Rather than managing certificates
manually, the operator integrates with
[cert-manager](https://cert-manager.io/) to automatically provision and rotate
webhook serving certificates.

The certificate management architecture has three layers:

1. **Base webhook manifests** (`config/webhook/`) — generated
   `MutatingWebhookConfiguration`, `ValidatingWebhookConfiguration`, and
   `Service` resources. These are kept clean for use with envtest.
2. **Cert-manager resources** (`config/certmanager/`) — a self-signed `Issuer`
   and a `Certificate` that provisions TLS credentials for the webhook Service
   DNS name.
3. **Default overlay patches** (`config/default/`) — Kustomize patches that
   wire certificate volume mounts, webhook ports, and CA injection annotations
   onto the base resources.

### Deployment Modes

| Mode | Certificate source | Configuration |
|------|--------------------|---------------|
| Production (cert-manager) | cert-manager provisions and rotates certs | Default `config/default/kustomization.yaml` |
| Local / envtest | controller-runtime auto-generates self-signed certs | `WebhookInstallOptions` in `suite_test.go` |
| Manual (no cert-manager) | Comment out cert-manager lines in `config/default/kustomization.yaml` | Provide certs manually in the `webhook-server-cert` Secret |

---

## Cert-Manager Resources

### Self-Signed Issuer

File: `config/certmanager/certificate.yaml`

```yaml
apiVersion: cert-manager.io/v1
kind: Issuer
metadata:
  name: selfsigned-issuer
  namespace: system
spec:
  selfSigned: {}
```

The Issuer uses `selfSigned` mode, meaning the Certificate's private key signs
its own certificate. This is sufficient for webhook TLS because the API server
only needs to verify the certificate against the CA bundle injected into the
webhook configuration — it does not need a publicly trusted CA chain.

After Kustomize processing, the Issuer is created as
`memcached-operator-selfsigned-issuer` in the
`memcached-operator-system` namespace.

### Certificate

File: `config/certmanager/certificate.yaml`

```yaml
apiVersion: cert-manager.io/v1
kind: Certificate
metadata:
  name: serving-cert
  namespace: system
spec:
  dnsNames:
  - $(SERVICE_NAME).$(SERVICE_NAMESPACE).svc
  - $(SERVICE_NAME).$(SERVICE_NAMESPACE).svc.cluster.local
  issuerRef:
    kind: Issuer
    name: selfsigned-issuer
  secretName: webhook-server-cert
```

| Field | Purpose |
|-------|---------|
| `dnsNames` | SAN entries matching the webhook Service's cluster DNS names. Kustomize substitutes `SERVICE_NAME` and `SERVICE_NAMESPACE` at build time. |
| `issuerRef` | Points to the self-signed Issuer. Kustomize's `nameReference` configuration automatically updates this when the Issuer is renamed by `namePrefix`. |
| `secretName` | The Kubernetes Secret where cert-manager stores the TLS key pair. This name is **not** prefixed by Kustomize — it is referenced directly by the Deployment volume mount. |

After Kustomize processing, `dnsNames` resolve to:
- `memcached-operator-webhook-service.memcached-operator-system.svc`
- `memcached-operator-webhook-service.memcached-operator-system.svc.cluster.local`

### Kustomize Configuration

File: `config/certmanager/kustomizeconfig.yaml`

Teaches Kustomize how to follow name and namespace references within
cert-manager resources:

- **nameReference**: Updates `Certificate.spec.issuerRef.name` when the Issuer
  is renamed by `namePrefix`.
- **varReference**: Allows `$(SERVICE_NAME)` and `$(SERVICE_NAMESPACE)` variable
  substitution in `Certificate.spec.dnsNames`.

---

## CA Injection Patch

File: `config/default/webhookcainjection_patch.yaml`

```yaml
apiVersion: admissionregistration.k8s.io/v1
kind: MutatingWebhookConfiguration
metadata:
  name: mutating-webhook-configuration
  annotations:
    cert-manager.io/inject-ca-from: $(CERTIFICATE_NAMESPACE)/$(CERTIFICATE_NAME)
---
apiVersion: admissionregistration.k8s.io/v1
kind: ValidatingWebhookConfiguration
metadata:
  name: validating-webhook-configuration
  annotations:
    cert-manager.io/inject-ca-from: $(CERTIFICATE_NAMESPACE)/$(CERTIFICATE_NAME)
```

The `cert-manager.io/inject-ca-from` annotation tells cert-manager's
[CA Injector](https://cert-manager.io/docs/concepts/ca-injector/) to
automatically populate the `caBundle` field in both webhook configurations with
the CA certificate from the referenced Certificate resource. This ensures the
API server trusts the webhook's TLS certificate without manual `caBundle`
management.

After Kustomize variable substitution, the annotation resolves to:
```
cert-manager.io/inject-ca-from: memcached-operator-system/memcached-operator-serving-cert
```

### Kustomize Variable Definitions

The variable substitution is configured in `config/default/kustomization.yaml`:

| Variable | Source | Field |
|----------|--------|-------|
| `CERTIFICATE_NAMESPACE` | `Certificate/serving-cert` | `metadata.namespace` |
| `CERTIFICATE_NAME` | `Certificate/serving-cert` | `metadata.name` (default) |
| `SERVICE_NAMESPACE` | `Service/webhook-service` | `metadata.namespace` |
| `SERVICE_NAME` | `Service/webhook-service` | `metadata.name` (default) |

---

## Manager Deployment Patch

File: `config/default/manager_webhook_patch.yaml`

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: controller-manager
  namespace: system
spec:
  template:
    spec:
      containers:
      - name: manager
        ports:
        - containerPort: 9443
          name: webhook-server
          protocol: TCP
        volumeMounts:
        - mountPath: /tmp/k8s-webhook-server/serving-certs
          name: cert
          readOnly: true
      volumes:
      - name: cert
        secret:
          defaultMode: 420
          secretName: webhook-server-cert
```

This patch adds three things to the manager Deployment:

| Addition | Value | Purpose |
|----------|-------|---------|
| Container port | `9443` (named `webhook-server`) | Exposes the webhook HTTPS endpoint. Controller-runtime's default webhook port is 9443. |
| Volume mount | `/tmp/k8s-webhook-server/serving-certs` | Controller-runtime's default `CertDir`. The webhook server reads `tls.crt` and `tls.key` from this path. |
| Volume | Secret `webhook-server-cert` | The Secret provisioned by cert-manager (matching `Certificate.spec.secretName`). |

---

## Webhook Service

File: `config/webhook/service.yaml`

```yaml
apiVersion: v1
kind: Service
metadata:
  name: webhook-service
  namespace: system
spec:
  ports:
  - port: 443
    protocol: TCP
    targetPort: 9443
  selector:
    control-plane: controller-manager
```

The Service routes HTTPS traffic from the API server (port 443) to the manager
container's webhook port (9443). The `selector` matches pods created by the
manager Deployment.

---

## Webhook Server Configuration

File: `cmd/main.go`

```go
webhookServer := webhook.NewServer(webhook.Options{
    TLSOpts: tlsOpts,
})
```

The webhook server uses controller-runtime defaults:

| Setting | Default | Source |
|---------|---------|--------|
| Port | `9443` | `webhook.DefaultPort` |
| CertDir | `/tmp/k8s-webhook-server/serving-certs` | `webhook.DefaultCertDir` |
| CertName | `tls.crt` | `webhook.DefaultCertName` |
| KeyName | `tls.key` | `webhook.DefaultKeyName` |

No explicit `CertDir` or `Port` override is needed because the cert-manager
Certificate and Deployment patch use these same defaults.

---

## File Summary

| File | Purpose |
|------|---------|
| `config/certmanager/certificate.yaml` | Self-signed Issuer and Certificate resources |
| `config/certmanager/kustomization.yaml` | Includes certificate.yaml and kustomizeconfig.yaml |
| `config/certmanager/kustomizeconfig.yaml` | Teaches Kustomize name/var reference resolution for cert-manager |
| `config/default/kustomization.yaml` | Wires cert-manager resources, patches, and variable substitutions |
| `config/default/webhookcainjection_patch.yaml` | Adds CA injection annotations to webhook configurations |
| `config/default/manager_webhook_patch.yaml` | Adds cert volume mount and webhook port to manager Deployment |
| `config/webhook/service.yaml` | Routes port 443 to manager's webhook port 9443 |
| `cmd/main.go` | Creates webhook server with default TLS options |

---

## Disabling Cert-Manager

To deploy without cert-manager (e.g., when providing certificates manually),
comment out the cert-manager lines in `config/default/kustomization.yaml`:

1. Remove `../certmanager` from the `resources` list.
2. Remove the `webhookcainjection_patch.yaml` entry from `patches`.
3. Remove the `manager_webhook_patch.yaml` entry from `patches`.
4. Remove the `vars` block containing `CERTIFICATE_NAMESPACE`,
   `CERTIFICATE_NAME`, `SERVICE_NAMESPACE`, and `SERVICE_NAME`.
5. Manually create the `webhook-server-cert` Secret with `tls.crt` and
   `tls.key` entries, and set `caBundle` in the webhook configurations.

For local development with envtest, no changes are needed — envtest uses
`WebhookInstallOptions` to auto-generate certificates independently of the
Kustomize overlay.
