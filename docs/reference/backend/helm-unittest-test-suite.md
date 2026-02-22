# Helm Unit Tests

Reference documentation for the helm-unittest test suite that validates all
Helm chart templates render correctly with default and custom values, covering
document counts, kind/apiVersion validation, toggle gating, deployment
parameterization, label/selector correctness, cross-template name propagation,
and NOTES.txt output.

**Source**: `charts/memcached-operator/tests/`

## Overview

The helm-unittest test suite exercises every Helm template in
`charts/memcached-operator/templates/` without requiring a Kubernetes cluster.
Unlike envtest or Chainsaw E2E tests that run against a real API server, these
tests validate template rendering logic: conditional inclusion, value
propagation, label generation, and cross-template reference consistency.

The suite uses the [helm-unittest](https://github.com/helm-unittest/helm-unittest)
plugin (v0.5.x or later). Each test file is a YAML document containing a suite
name, template references, release metadata, and assertion blocks.

---

## Test Infrastructure

### Running Tests

```bash
# Run all helm-unittest tests
helm unittest charts/memcached-operator

# Run with verbose output
helm unittest -v charts/memcached-operator
```

### Prerequisites

| Prerequisite         | Purpose                | Install Command                                                                    |
|----------------------|------------------------|------------------------------------------------------------------------------------|
| Helm 3.x             | Chart rendering engine | `curl https://raw.githubusercontent.com/helm/helm/main/scripts/get-helm-3 \| bash` |
| helm-unittest plugin | Test runner            | `helm plugin install https://github.com/helm-unittest/helm-unittest`               |

No cluster, cert-manager, or CRDs are required — tests run entirely offline.

### Release Metadata Convention

All test files use identical release metadata for predictable assertion values:

```yaml
release:
  name: test-release
  namespace: test-ns
chart:
  appVersion: "0.1.0"
  version: "0.1.0"
```

This produces the fullname `test-release-memcached-operator` via the
`memcached-operator.fullname` helper.

---

## File Structure

```text
charts/memcached-operator/tests/
├── __snapshot__/                          # Auto-generated snapshot files
├── deployment_test.yaml                   # Deployment template (56 tests)
├── serviceaccount_test.yaml               # ServiceAccount template
├── rbac_clusterrole_test.yaml             # ClusterRole + ClusterRoleBinding
├── rbac_leader_election_test.yaml         # Leader-election Role + RoleBinding
├── rbac_metrics_test.yaml                 # Metrics auth roles (3 documents)
├── webhook_service_test.yaml              # Webhook Service
├── webhook_mutating_test.yaml             # MutatingWebhookConfiguration
├── webhook_validating_test.yaml           # ValidatingWebhookConfiguration
├── certmanager_certificate_test.yaml      # cert-manager Certificate
├── certmanager_issuer_test.yaml           # cert-manager Issuer
├── servicemonitor_test.yaml               # ServiceMonitor
├── networkpolicy_test.yaml                # NetworkPolicy
├── crd_template_test.yaml                 # CRD template (crds.managedByHelm)
├── helpers_test.yaml                      # _helpers.tpl functions via proxy
├── chart_metadata_test.yaml               # NOTES.txt output validation
└── integration_test.yaml                  # Cross-template integration tests
```

### File Organization

Each template has a dedicated test file for isolated testing:

| Test File                           | Template(s) Under Test                                  | Documents                                 |
|-------------------------------------|---------------------------------------------------------|-------------------------------------------|
| `deployment_test.yaml`              | `templates/deployment.yaml`                             | 1                                         |
| `serviceaccount_test.yaml`          | `templates/serviceaccount.yaml`                         | 1                                         |
| `rbac_clusterrole_test.yaml`        | `templates/rbac/clusterrole.yaml`                       | 2 (ClusterRole + ClusterRoleBinding)      |
| `rbac_leader_election_test.yaml`    | `templates/rbac/leader-election-role.yaml`              | 2 (Role + RoleBinding)                    |
| `rbac_metrics_test.yaml`            | `templates/rbac/metrics-role.yaml`                      | 3 (2 ClusterRoles + 1 ClusterRoleBinding) |
| `webhook_service_test.yaml`         | `templates/webhook/service.yaml`                        | 1                                         |
| `webhook_mutating_test.yaml`        | `templates/webhook/mutatingwebhookconfiguration.yaml`   | 1                                         |
| `webhook_validating_test.yaml`      | `templates/webhook/validatingwebhookconfiguration.yaml` | 1                                         |
| `certmanager_certificate_test.yaml` | `templates/certmanager/certificate.yaml`                | 1                                         |
| `certmanager_issuer_test.yaml`      | `templates/certmanager/issuer.yaml`                     | 1                                         |
| `servicemonitor_test.yaml`          | `templates/servicemonitor.yaml`                         | 0 or 1                                    |
| `networkpolicy_test.yaml`           | `templates/networkpolicy.yaml`                          | 1                                         |
| `crd_template_test.yaml`            | `templates/crd-template.yaml`                           | 0 or 1                                    |
| `helpers_test.yaml`                 | `templates/crd-template.yaml`, `templates/NOTES.txt`    | proxy                                     |
| `chart_metadata_test.yaml`          | `templates/NOTES.txt`                                   | raw text                                  |
| `integration_test.yaml`             | All 13 template files simultaneously                    | 15 total                                  |

---

## Test Patterns

### 1. Toggle Gating with `hasDocuments`

Validates that a `values.yaml` toggle produces the correct number of documents:

```yaml
- it: should not render when webhook.enabled is false
  set:
    webhook.enabled: false
  asserts:
    - hasDocuments:
        count: 0
```

### 2. Kind and API Version Validation

Verifies the rendered document type:

```yaml
- it: should render correct kind and apiVersion
  asserts:
    - isKind:
        of: Deployment
    - isAPIVersion:
        of: apps/v1
```

### 3. Exact Value Matching with `equal`

Asserts a specific value at a JSON path:

```yaml
- it: should set default image to controller:0.1.0
  asserts:
    - equal:
        path: spec.template.spec.containers[0].image
        value: "controller:0.1.0"
```

### 4. Array Membership with `contains`

Verifies an item exists in a list:

```yaml
- it: should expose health port 8081
  asserts:
    - contains:
        path: spec.template.spec.containers[0].ports
        content:
          containerPort: 8081
          name: health
          protocol: TCP
```

### 5. Absence Verification with `notExists` / `notContains`

Asserts that a path or array element is absent:

```yaml
- it: should not include imagePullSecrets by default
  asserts:
    - notExists:
        path: spec.template.spec.imagePullSecrets

- it: should not include webhook-server port when webhook.enabled is false
  set:
    webhook.enabled: false
  asserts:
    - notContains:
        path: spec.template.spec.containers[0].ports
        content:
          containerPort: 9443
          name: webhook-server
          protocol: TCP
```

### 6. NOTES.txt Regex Matching with `matchRegexRaw`

Validates raw text output (NOTES.txt is not YAML):

```yaml
- it: should show RBAC warning when rbac.create is false
  set:
    rbac.create: false
  asserts:
    - matchRegexRaw:
        pattern: "WARNING: RBAC resources are not managed by this chart"
```

### 7. Multi-Document Indexing with `documentIndex`

For templates that produce multiple YAML documents (RBAC files):

```yaml
- it: should render ClusterRole as first document
  documentIndex: 0
  asserts:
    - isKind:
        of: ClusterRole

- it: should render ClusterRoleBinding as second document
  documentIndex: 1
  asserts:
    - isKind:
        of: ClusterRoleBinding
```

### 8. Value Overrides with `set`

Injects custom values for a single test:

```yaml
- it: should use custom repository and tag
  set:
    image.repository: "ghcr.io/my-org/my-operator"
    image.tag: "v2.0.0"
  asserts:
    - equal:
        path: spec.template.spec.containers[0].image
        value: "ghcr.io/my-org/my-operator:v2.0.0"
```

### 9. Template Scoping in Integration Tests

Integration tests render all templates but scope assertions to one:

```yaml
- it: should propagate fullnameOverride to Deployment name
  set:
    fullnameOverride: my-custom-name
  template: templates/deployment.yaml
  asserts:
    - equal:
        path: metadata.name
        value: my-custom-name
```

### 10. Helper Function Testing via Proxy Templates

`_helpers.tpl` functions cannot be tested directly. The suite uses
`crd-template.yaml` (with `crds.managedByHelm: true`) as a proxy to verify
label helpers, and `NOTES.txt` to verify fullname/selector helpers:

```yaml
# Tests memcached-operator.fullname via NOTES.txt
- it: should use fullnameOverride when set
  set:
    fullnameOverride: my-override
  asserts:
    - matchRegexRaw:
        pattern: "my-override has been installed"
```

---

## Test Suites

### 1. Deployment (`deployment_test.yaml`)

The most comprehensive test file, organized into 6 sections:

| Section             | Tests                                                                                                                                                                                           | Coverage |
|---------------------|-------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|----------|
| Default rendering   | Document count, kind/apiVersion, replicas, image, resources, serviceAccountName, labels, selectors, namespace, container name/command, ports, probes, pod annotations, termination grace period | REQ-001  |
| Container args      | `--leader-elect` (toggle), `--health-probe-bind-address`, `--metrics-bind-address`, `--metrics-secure`, `--watch-namespaces` (conditional)                                                      | REQ-007  |
| Custom image        | Custom tag, custom repository+tag, imagePullSecrets, pullPolicy                                                                                                                                 | REQ-007  |
| Security context    | Pod securityContext (runAsNonRoot, runAsUser, seccompProfile), container securityContext (allowPrivilegeEscalation, readOnlyRootFilesystem, capabilities.drop)                                  | REQ-007  |
| Scheduling          | nodeSelector, tolerations, affinity — absent by default, rendered when set                                                                                                                      | REQ-007  |
| Conditional webhook | Port 9443, volumeMounts, volumes — present/absent based on `webhook.enabled`; fullnameOverride propagation to cert volume secretName                                                            | REQ-002  |

### 2. ServiceAccount (`serviceaccount_test.yaml`)

| Test                                   | Assertion                                                  |
|----------------------------------------|------------------------------------------------------------|
| `serviceAccount.create=true` (default) | 1 document rendered                                        |
| `serviceAccount.create=false`          | 0 documents                                                |
| Kind/apiVersion                        | ServiceAccount / v1                                        |
| Name                                   | Fullname by default, custom `serviceAccount.name` when set |
| Labels                                 | All 5 standard labels, managed-by=Helm                     |
| Annotations                            | Absent by default, rendered when set                       |

### 3. RBAC ClusterRole (`rbac_clusterrole_test.yaml`)

2-document template (ClusterRole + ClusterRoleBinding):

| Test                         | Assertion                                                                                                                                                    |
|------------------------------|--------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `rbac.create=true` (default) | 2 documents                                                                                                                                                  |
| `rbac.create=false`          | 0 documents                                                                                                                                                  |
| ClusterRole (doc 0)          | 11 rules covering memcacheds, memcacheds/status, memcacheds/finalizers, deployments, services, PDBs, networkpolicies, servicemonitors, HPAs, secrets, events |
| ClusterRoleBinding (doc 1)   | roleRef to ClusterRole, subjects binding to SA in release namespace                                                                                          |

### 4. Leader Election RBAC (`rbac_leader_election_test.yaml`)

2-document template (Role + RoleBinding) with dual toggle:

| Toggle Combination                                  | Result      |
|-----------------------------------------------------|-------------|
| `rbac.create=true`, `leaderElection.enabled=true`   | 2 documents |
| `rbac.create=true`, `leaderElection.enabled=false`  | 0 documents |
| `rbac.create=false`, `leaderElection.enabled=true`  | 0 documents |
| `rbac.create=false`, `leaderElection.enabled=false` | 0 documents |

The Role has exactly 3 rules: configmaps, leases, and events.

### 5. Metrics RBAC (`rbac_metrics_test.yaml`)

3-document template:

| Document | Kind               | Name Suffix                 | Rules                                  |
|----------|--------------------|-----------------------------|----------------------------------------|
| 0        | ClusterRole        | `-metrics-auth-role`        | 2 (tokenreviews, subjectaccessreviews) |
| 1        | ClusterRoleBinding | `-metrics-auth-rolebinding` | roleRef + subjects                     |
| 2        | ClusterRole        | `-metrics-reader`           | 1 (/metrics nonResourceURL)            |

### 6. Webhook Service (`webhook_service_test.yaml`)

| Test                             | Assertion                                            |
|----------------------------------|------------------------------------------------------|
| `webhook.enabled=true` (default) | 1 document                                           |
| `webhook.enabled=false`          | 0 documents                                          |
| Port                             | 443 targeting 9443                                   |
| Selector                         | selectorLabels + `control-plane: controller-manager` |

### 7-8. Webhook Configurations (`webhook_mutating_test.yaml`, `webhook_validating_test.yaml`)

Both follow the same pattern:

| Test                    | Assertion                                                             |
|-------------------------|-----------------------------------------------------------------------|
| Toggle gating           | `webhook.enabled` controls rendering                                  |
| cert-manager annotation | Present when `certmanager.enabled=true`, absent when false            |
| Webhook config          | Correct path, service reference, failurePolicy=Fail, sideEffects=None |
| Rules                   | `memcached.c5c3.io`, `v1beta1`, `CREATE`+`UPDATE`, `memcacheds`       |

### 9. cert-manager Certificate (`certmanager_certificate_test.yaml`)

Dual toggle — requires both `webhook.enabled` AND `certmanager.enabled`:

| Toggle Combination          | Result      |
|-----------------------------|-------------|
| Both true (default)         | 1 document  |
| `certmanager.enabled=false` | 0 documents |
| `webhook.enabled=false`     | 0 documents |

Validates dnsNames with `.svc` and `.svc.cluster.local` suffixes, issuerRef,
privateKey.rotationPolicy, and secretName.

### 10. cert-manager Issuer (`certmanager_issuer_test.yaml`)

Same dual toggle as Certificate. Validates `spec.selfSigned: {}` and standard
labels.

### 11. ServiceMonitor (`servicemonitor_test.yaml`)

| Test                                     | Assertion                                                             |
|------------------------------------------|-----------------------------------------------------------------------|
| `serviceMonitor.enabled=false` (default) | 0 documents                                                           |
| `serviceMonitor.enabled=true`            | 1 document with endpoint config                                       |
| Endpoint                                 | path=/metrics, port=metrics, scheme=https, bearerTokenFile, tlsConfig |
| Optional fields                          | interval and scrapeTimeout absent by default, rendered when set       |
| Additional labels                        | Merged into metadata.labels when configured                           |

### 12. NetworkPolicy (`networkpolicy_test.yaml`)

| Test                                   | Assertion                                            |
|----------------------------------------|------------------------------------------------------|
| `networkPolicy.enabled=true` (default) | 1 document                                           |
| `networkPolicy.enabled=false`          | 0 documents                                          |
| Ingress ports (webhook enabled)        | 8081 (health), 8443 (metrics), 9443 (webhook)        |
| Ingress ports (webhook disabled)       | 8081, 8443 only — 9443 absent                        |
| Egress                                 | Allow all (empty rule)                               |
| Pod selector                           | selectorLabels + `control-plane: controller-manager` |

### 13. CRD Template (`crd_template_test.yaml`)

| Test                                 | Assertion                                                                      |
|--------------------------------------|--------------------------------------------------------------------------------|
| `crds.managedByHelm=false` (default) | 0 documents                                                                    |
| `crds.managedByHelm=true`            | 1 CustomResourceDefinition                                                     |
| CRD spec                             | group=`memcached.c5c3.io`, kind=Memcached, plural=memcacheds, scope=Namespaced |
| CRD name                             | `memcacheds.memcached.c5c3.io`                                                 |

### 14. Helper Functions (`helpers_test.yaml`)

Uses two proxy templates across two test suites:

**Suite 1** — CRD template (`crds.managedByHelm: true`) for label functions:

| Test                                | Assertion                                                    |
|-------------------------------------|--------------------------------------------------------------|
| `memcached-operator.name`           | Chart name by default, nameOverride when set                 |
| `memcached-operator.chart`          | Formats as `name-version` (e.g., `memcached-operator-0.1.0`) |
| `memcached-operator.labels`         | All 5 standard labels present with correct values            |
| `memcached-operator.selectorLabels` | name + instance subset                                       |

**Suite 2** — NOTES.txt for fullname and selector functions:

| Test                             | Assertion                                                           |
|----------------------------------|---------------------------------------------------------------------|
| Default fullname                 | `test-release-memcached-operator`                                   |
| Release name contains chart name | Uses release name directly                                          |
| nameOverride                     | Included in fullname computation                                    |
| fullnameOverride                 | Replaces computed fullname entirely                                 |
| 63-char truncation               | Names exceeding 63 chars are truncated per DNS spec                 |
| selectorLabelsKubectl            | Renders `app.kubernetes.io/name=...,app.kubernetes.io/instance=...` |

### 15. Chart Metadata / NOTES.txt (`chart_metadata_test.yaml`)

Validates post-install notes reflect toggle states:

| Test                  | Assertion                                                |
|-----------------------|----------------------------------------------------------|
| Default fullname      | `test-release-memcached-operator has been installed`     |
| CRD mode (default)    | `CRD management: Helm crds/ directory (default)`         |
| CRD mode (managed)    | `CRD management: Helm-managed (crds.managedByHelm=true)` |
| ServiceAccount name   | Default fullname or custom name                          |
| RBAC status           | `Helm-managed (rbac.create=true)` by default             |
| RBAC warning          | WARNING when `rbac.create=false`                         |
| Webhook status        | `enabled` / `disabled`                                   |
| cert-manager warning  | WARNING when webhooks enabled but cert-manager disabled  |
| ServiceMonitor status | `enabled` when toggled, absent by default                |

### 16. Integration Tests (`integration_test.yaml`)

Renders all 13 template files simultaneously to validate cross-template
consistency:

| Test                            | Assertion                                                                                                                                    |
|---------------------------------|----------------------------------------------------------------------------------------------------------------------------------------------|
| Default resource count          | 15 total: SA=1, Deployment=1, ClusterRole=2, LeaderElection=2, Metrics=3, NetworkPolicy=1, Webhook=3, CertManager=2, ServiceMonitor=0, CRD=0 |
| `rbac.create=false`             | 7 RBAC documents suppressed, others unchanged                                                                                                |
| `serviceAccount.create=false`   | SA suppressed, Deployment serviceAccountName=`default`                                                                                       |
| `webhook.enabled=false`         | 5 webhook+certmanager documents suppressed; webhook port removed from Deployment and NetworkPolicy                                           |
| `certmanager.enabled=false`     | 2 certmanager documents suppressed, webhooks still render                                                                                    |
| `serviceMonitor.enabled=true`   | 1 ServiceMonitor added                                                                                                                       |
| `crds.managedByHelm=true`       | 1 CRD added                                                                                                                                  |
| fullnameOverride propagation    | Verified on all resource names (Deployment, SA, all RBAC, webhook Service, webhook configs, certmanager, ServiceMonitor, NetworkPolicy)      |
| serviceAccount.name propagation | Verified on SA, Deployment, all binding subjects                                                                                             |
| Cross-references                | Webhook configs reference correct service name; cert-manager annotation references correct certificate name with fullnameOverride            |

---

## Toggle Coverage Matrix

Shows which test files verify each `values.yaml` toggle:

| Toggle                   | Test Files                                                                                                                                                                                                                                                        |
|--------------------------|-------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `webhook.enabled`        | `deployment_test` (ports/volumes), `webhook_service_test`, `webhook_mutating_test`, `webhook_validating_test`, `certmanager_certificate_test`, `certmanager_issuer_test`, `networkpolicy_test` (ingress ports), `chart_metadata_test` (NOTES), `integration_test` |
| `certmanager.enabled`    | `webhook_mutating_test` (annotation), `webhook_validating_test` (annotation), `certmanager_certificate_test`, `certmanager_issuer_test`, `chart_metadata_test` (NOTES), `integration_test`                                                                        |
| `serviceMonitor.enabled` | `servicemonitor_test`, `chart_metadata_test` (NOTES), `integration_test`                                                                                                                                                                                          |
| `networkPolicy.enabled`  | `networkpolicy_test`, `integration_test`                                                                                                                                                                                                                          |
| `rbac.create`            | `rbac_clusterrole_test`, `rbac_leader_election_test`, `rbac_metrics_test`, `chart_metadata_test` (NOTES), `integration_test`                                                                                                                                      |
| `leaderElection.enabled` | `deployment_test` (args), `rbac_leader_election_test`                                                                                                                                                                                                             |
| `serviceAccount.create`  | `serviceaccount_test`, `integration_test`                                                                                                                                                                                                                         |
| `crds.managedByHelm`     | `crd_template_test`, `chart_metadata_test` (NOTES), `integration_test`                                                                                                                                                                                            |

---

## Adding a New Test

### 1. Create the test file

Create `charts/memcached-operator/tests/<template_name>_test.yaml` following
the naming convention:

```yaml
---
suite: <Template description>
templates:
  - templates/<path-to-template>.yaml
release:
  name: test-release
  namespace: test-ns
chart:
  appVersion: "0.1.0"
  version: "0.1.0"
tests:
  - it: should render one document with defaults
    asserts:
      - hasDocuments:
          count: 1

  - it: should render correct kind and apiVersion
    asserts:
      - isKind:
          of: <ExpectedKind>
      - isAPIVersion:
          of: <expected/version>
```

### 2. Cover the standard assertions

Every template test file should validate at minimum:

1. **Document count** — `hasDocuments` with default values and with toggle disabled
2. **Kind and apiVersion** — `isKind` + `isAPIVersion`
3. **Metadata** — `metadata.name` (from fullname helper), `metadata.namespace`, standard labels
4. **Toggle gating** — 0 documents when the controlling toggle is false
5. **Value propagation** — Any configurable field renders correctly with custom values

### 3. Add to integration tests

If the new template is controlled by a toggle, add document count assertions
to the relevant tests in `integration_test.yaml`:

```yaml
- it: should produce N resources with all defaults
  asserts:
    # ... existing assertions ...
    - hasDocuments:
        count: 1
      template: templates/<new-template>.yaml
```

### 4. Run the tests

```bash
helm unittest charts/memcached-operator
```

All tests must pass with 0 failures before merging.

---

## Known Limitations

| Limitation                         | Impact                                                                                        | Mitigation                                                          |
|------------------------------------|-----------------------------------------------------------------------------------------------|---------------------------------------------------------------------|
| No cluster validation              | Templates may render valid YAML but be rejected by the API server (e.g., invalid field names) | Chainsaw E2E tests catch runtime issues                             |
| NOTES.txt regex fragility          | Regex assertions break if NOTES format changes                                                | Keep NOTES format stable; update regex patterns when format changes |
| Multi-document ordering assumption | Tests assume a fixed document order within templates                                          | Use `documentIndex` consistently; document order is stable in Helm  |
| No dynamic value computation       | Cannot test Go template logic like `join`, only its output                                    | Test the rendered output with known inputs                          |
| Helper functions tested indirectly | `_helpers.tpl` requires proxy templates (CRD, NOTES.txt)                                      | Acceptable trade-off; proxy templates exercise all helper paths     |
| helm-unittest plugin version       | Plugin API may change across major versions                                                   | Pin to v0.5.x or later                                              |

---

## Requirement Coverage Matrix

| REQ-ID  | Requirement                                                                          | Test Files                                                                                          | Key Assertions                                                                                            |
|---------|--------------------------------------------------------------------------------------|-----------------------------------------------------------------------------------------------------|-----------------------------------------------------------------------------------------------------------|
| REQ-001 | Default rendering: correct document counts, kind/apiVersion, metadata                | All template test files, `integration_test`                                                         | `hasDocuments`, `isKind`, `isAPIVersion`, `equal` on metadata                                             |
| REQ-002 | `webhook.enabled=false` excludes webhook resources and artifacts                     | `deployment_test`, `webhook_*_test`, `certmanager_*_test`, `networkpolicy_test`, `integration_test` | 0 documents, `notContains` port 9443, `notExists` volumeMounts/volumes                                    |
| REQ-003 | `certmanager.enabled` controls cert-manager resources and annotations                | `webhook_mutating_test`, `webhook_validating_test`, `certmanager_*_test`, `integration_test`        | 0 documents, annotation present/absent                                                                    |
| REQ-004 | `serviceMonitor.enabled` controls ServiceMonitor rendering                           | `servicemonitor_test`, `integration_test`                                                           | 0/1 documents, endpoint config, optional fields                                                           |
| REQ-005 | `networkPolicy.enabled`, `rbac.create`, `leaderElection.enabled` toggles             | `networkpolicy_test`, `rbac_*_test`, `deployment_test`, `integration_test`                          | 0 documents when disabled, correct rule counts                                                            |
| REQ-006 | `crds.managedByHelm` toggle and CRD spec correctness                                 | `crd_template_test`, `chart_metadata_test`                                                          | 0/1 documents, group/kind/plural/scope values                                                             |
| REQ-007 | Deployment parameterization (image, replicas, resources, args, security, scheduling) | `deployment_test`                                                                                   | Custom image, replicaCount, resources, watchNamespaces, security contexts, scheduling                     |
| REQ-008 | `_helpers.tpl` functions: labels, names, selectors                                   | `helpers_test`                                                                                      | fullname variants, label sets, truncation at 63 chars                                                     |
| REQ-009 | Cross-template consistency with fullnameOverride and serviceAccount.name             | `integration_test`                                                                                  | Name propagation to all resources, binding subjects, webhook service references, cert-manager annotations |
| REQ-010 | RBAC rules: exact verb sets and resource targets                                     | `rbac_clusterrole_test`, `rbac_leader_election_test`, `rbac_metrics_test`                           | 11 ClusterRole rules, 3 leader-election rules, 3-document metrics structure                               |
| REQ-011 | NOTES.txt reflects toggle states and shows warnings                                  | `chart_metadata_test`                                                                               | Regex matching for CRD mode, RBAC status, webhook status, cert-manager warnings                           |
| REQ-012 | `serviceAccount.create` toggle and annotation propagation                            | `serviceaccount_test`, `integration_test`                                                           | 0 documents when disabled, custom name propagation, annotations                                           |
