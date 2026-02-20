# RBAC Least-Privilege Permissions

Reference documentation for the operator's RBAC (Role-Based Access Control)
configuration, describing every ClusterRole rule, the ClusterRoleBinding, and
the ServiceAccount.

**Source**: `config/rbac/role.yaml`, `config/rbac/role_binding.yaml`, `config/rbac/service_account.yaml`

## Overview

The memcached-operator follows the principle of least privilege: it requests only
the minimum Kubernetes API permissions required for reconciliation. All RBAC
rules are generated from `+kubebuilder:rbac` markers on the `MemcachedReconciler`
in `internal/controller/memcached_controller.go`. No hand-edited rules exist in
`config/rbac/role.yaml`.

The operator uses a single ClusterRole (`manager-role`) bound to a single
ServiceAccount (`controller-manager`) via a ClusterRoleBinding
(`manager-rolebinding`). The ClusterRole contains exactly **10 policy rules**.

---

## ClusterRole Rules

The `manager-role` ClusterRole contains the following rules, grouped by purpose.

### Memcached Custom Resource

| API Group           | Resource                | Verbs                                           | Purpose                                                      |
|---------------------|-------------------------|-------------------------------------------------|--------------------------------------------------------------|
| `memcached.c5c3.io` | `memcacheds`            | create, delete, get, list, patch, update, watch | Full CRUD on the primary custom resource                     |
| `memcached.c5c3.io` | `memcacheds/status`     | get, patch, update                              | Update status subresource with conditions and observed state |
| `memcached.c5c3.io` | `memcacheds/finalizers` | update                                          | Add/remove finalizers for cleanup logic                      |

**Rationale**: The reconciler must read, create, update, and delete Memcached CRs.
Status updates require separate subresource permissions. Finalizer management
requires update on the finalizers subresource.

### Owned Resources (Full CRUD)

These are Kubernetes resources the reconciler creates and manages as owned
objects of each Memcached CR. Each requires full CRUD verbs so the reconciler can
create, update, and clean up owned resources without permission errors.

| API Group               | Resource               | Verbs                                           | Reconciler Method                                                    |
|-------------------------|------------------------|-------------------------------------------------|----------------------------------------------------------------------|
| `apps`                  | `deployments`          | create, delete, get, list, patch, update, watch | `reconcileDeployment` — manages the Memcached StatefulSet/Deployment |
| _(core)_                | `services`             | create, delete, get, list, patch, update, watch | `reconcileService` — manages the headless Service for pod discovery  |
| `policy`                | `poddisruptionbudgets` | create, delete, get, list, patch, update, watch | `reconcilePDB` — manages the PodDisruptionBudget for availability    |
| `networking.k8s.io`     | `networkpolicies`      | create, delete, get, list, patch, update, watch | `reconcileNetworkPolicy` — manages ingress NetworkPolicy             |
| `monitoring.coreos.com` | `servicemonitors`      | create, delete, get, list, patch, update, watch | `reconcileServiceMonitor` — manages Prometheus ServiceMonitor        |

**Rationale**: Each owned resource goes through `controllerutil.CreateOrUpdate`,
which requires get (to check existence), create (for initial creation), and
update/patch (for subsequent reconciliation). Delete is needed for garbage
collection when the owner reference cascade does not apply. List and watch
support the controller's informer-based watch mechanism.

### Read-Only Resources

| API Group | Resource  | Verbs            | Purpose                                                         |
|-----------|-----------|------------------|-----------------------------------------------------------------|
| _(core)_  | `secrets` | get, list, watch | Mount SASL credentials and TLS certificates into Memcached pods |

**Rationale**: The operator reads Secrets to reference them in volume mounts for
SASL authentication (`spec.security.sasl.secretRef`) and TLS encryption
(`spec.security.tls.certificateSecretRef`). It never creates, updates, or
deletes Secrets — credential management is the cluster administrator's
responsibility. This read-only constraint minimizes the blast radius if the
operator pod is compromised.

### Event Recording

| API Group | Resource | Verbs         | Purpose                                        |
|-----------|----------|---------------|------------------------------------------------|
| _(core)_  | `events` | create, patch | Record Kubernetes events during reconciliation |

**Rationale**: The reconciler uses the Kubernetes event recorder to emit events
(e.g., resource created, updated, or errored). Event recording requires only
create (for new events) and patch (for updating event counts on repeated
occurrences). No read or delete access is needed.

---

## Least-Privilege Constraints

The ClusterRole enforces the following constraints:

| Constraint                   | Enforcement                                                                              |
|------------------------------|------------------------------------------------------------------------------------------|
| No wildcard verbs (`*`)      | Every rule lists explicit verbs                                                          |
| No wildcard resources (`*`)  | Every rule names specific resources                                                      |
| No wildcard API groups (`*`) | Every rule names specific API groups (or empty string for core)                          |
| Exactly 10 rules             | Prevents unintended permission creep; any new rule requires updating the rule-count test |
| Secrets are read-only        | Only get, list, watch — no create, update, patch, or delete                              |
| Events are write-only        | Only create, patch — no get, list, watch, or delete                                      |

These constraints are enforced by automated tests in
`internal/controller/memcached_rbac_test.go`.

---

## ClusterRoleBinding

The `manager-rolebinding` ClusterRoleBinding binds the ClusterRole to the
operator's ServiceAccount:

```yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: manager-rolebinding
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: manager-role
subjects:
- kind: ServiceAccount
  name: controller-manager
  namespace: system
```

| Field                   | Value                | Description                                                                         |
|-------------------------|----------------------|-------------------------------------------------------------------------------------|
| `roleRef.kind`          | `ClusterRole`        | References the cluster-scoped role                                                  |
| `roleRef.name`          | `manager-role`       | The ClusterRole containing all operator permissions                                 |
| `subjects[0].kind`      | `ServiceAccount`     | The operator runs as a Kubernetes ServiceAccount                                    |
| `subjects[0].name`      | `controller-manager` | ServiceAccount name used by the operator pod                                        |
| `subjects[0].namespace` | `system`             | Namespace where the operator is deployed (kustomize replaces with actual namespace) |

---

## ServiceAccount

```yaml
apiVersion: v1
kind: ServiceAccount
metadata:
  name: controller-manager
  namespace: system
```

The `controller-manager` ServiceAccount is created in the operator's namespace.
The operator Deployment references this ServiceAccount in
`spec.template.spec.serviceAccountName`, and Kubernetes automatically mounts
a projected token that carries the ClusterRole permissions.

---

## Kubebuilder RBAC Markers

All 10 RBAC rules are generated from markers on the `MemcachedReconciler` in
`internal/controller/memcached_controller.go`:

```go
// +kubebuilder:rbac:groups=memcached.c5c3.io,resources=memcacheds,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=memcached.c5c3.io,resources=memcacheds/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=memcached.c5c3.io,resources=memcacheds/finalizers,verbs=update
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=services,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=policy,resources=poddisruptionbudgets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=networking.k8s.io,resources=networkpolicies,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=monitoring.coreos.com,resources=servicemonitors,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=events,verbs=create;patch
```

These markers are the single source of truth. `controller-gen` reads them and
generates `config/rbac/role.yaml`.

---

## Regenerating Manifests

To regenerate the ClusterRole from RBAC markers:

```bash
make manifests
```

This runs `controller-gen rbac:roleName=manager-role` which reads the
`+kubebuilder:rbac` markers and writes `config/rbac/role.yaml`.

To verify that committed manifests match the generated output:

```bash
make verify-manifests
```

This regenerates manifests and checks for any diff. If the committed files
are out of date, the command fails with an error.

---

## Automated Test Coverage

The RBAC configuration is verified by tests in
`internal/controller/memcached_rbac_test.go`:

| Test                                                                            | What It Verifies                                   |
|---------------------------------------------------------------------------------|----------------------------------------------------|
| ClusterRole metadata / should have exactly 10 rules                             | Rule count prevents unintended permission creep    |
| Memcached CR permissions / should grant full CRUD on memcacheds                 | All 7 CRUD verbs on the primary CR                 |
| Memcached CR permissions / should grant get, update, patch on memcacheds/status | Status subresource verbs                           |
| Memcached CR permissions / should grant update on memcacheds/finalizers         | Finalizer subresource verbs                        |
| owned resource permissions / Deployments                                        | Full CRUD on apps/deployments                      |
| owned resource permissions / Services                                           | Full CRUD on core/services                         |
| owned resource permissions / PodDisruptionBudgets                               | Full CRUD on policy/poddisruptionbudgets           |
| owned resource permissions / NetworkPolicies                                    | Full CRUD on networking.k8s.io/networkpolicies     |
| owned resource permissions / ServiceMonitors                                    | Full CRUD on monitoring.coreos.com/servicemonitors |
| Secrets permission / should grant read-only access                              | Secrets limited to get, list, watch                |
| events permission / should grant create and patch                               | Events limited to create, patch                    |
| least-privilege constraints / should not contain wildcard verbs                 | No `*` in any verb list                            |
| least-privilege constraints / should not contain wildcard resources             | No `*` in any resource list                        |
| least-privilege constraints / should not contain wildcard API groups            | No `*` in any API group list                       |
| ClusterRoleBinding / should reference the manager-role ClusterRole              | Binding references correct ClusterRole             |
| ClusterRoleBinding / should bind to controller-manager ServiceAccount           | Binding targets correct SA in system namespace     |
