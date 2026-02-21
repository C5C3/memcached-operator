# Secret Hash Computation and References

Reference documentation for the secret hash computation, secret fetching, and
secret-to-Memcached mapping functions used by the reconciler to detect Secret
content changes and trigger re-reconciliation.

**Source**: `internal/controller/secret.go`

## Overview

The operator needs to detect when referenced Secrets (SASL credentials or TLS
certificates) change so it can update the Memcached Deployment accordingly. Three
helper functions in `internal/controller/secret.go` support this:

1. **`computeSecretHash`** — produces a deterministic SHA-256 hex digest over
   Secret `.data` fields, enabling the reconciler to detect content changes by
   comparing hash values across reconciliation cycles.
2. **`fetchReferencedSecrets`** — resolves `SASLSpec.CredentialsSecretRef` and
   `TLSSpec.CertificateSecretRef` from a Memcached CR into actual Secret objects,
   reporting which Secrets are present and which are missing.
3. **`mapSecretToMemcached`** — returns a `handler.MapFunc` that maps Secret
   change events to `reconcile.Request`s for Memcached CRs referencing that
   Secret, enabling watch-based re-reconciliation on Secret updates.

These functions are pure helpers — they do not modify the `MemcachedReconciler`
struct or the reconciliation loop directly.

---

## `computeSecretHash`

```go
func computeSecretHash(secrets ...*corev1.Secret) string
```

Returns a deterministic SHA-256 hex digest over the `.data` fields of the
provided Secrets. The hash is stable regardless of argument order or Go map
iteration order.

### Algorithm

1. **Filter**: If no Secrets are provided, or all Secrets have nil/empty `.data`,
   return `""`.
2. **Sort Secrets**: Copy the input slice and sort by `Secret.Name`
   (lexicographic). This ensures `computeSecretHash(A, B)` and
   `computeSecretHash(B, A)` produce the same result.
3. **Initialize hasher**: Create a `sha256.New()` hasher.
4. **Write data**: For each Secret (in sorted order), sort its `.data` keys
   lexicographically, then for each key write:
   ```text
   secretName \x00 key \x00 value
   ```
   The null byte (`\x00`) separator prevents ambiguity between key/value
   boundaries (e.g., key `"ab"` + value `"cd"` vs key `"a"` + value `"bcd"`).
5. **Return hex**: `hex.EncodeToString(h.Sum(nil))` — a 64-character lowercase
   hex string.

### Behavior

| Input                                     | Output                             |
|-------------------------------------------|------------------------------------|
| No Secrets (zero arguments)               | `""`                               |
| All Secrets have nil `.data`              | `""`                               |
| All Secrets have empty `.data`            | `""`                               |
| One or more Secrets with non-empty data   | 64-char lowercase hex SHA-256 hash |
| Same Secrets in different argument order  | Same hash (order-independent)      |
| Same Secret data with different key order | Same hash (key-order-independent)  |
| Any `.data` value changed                 | Different hash                     |
| Duplicate Secret (same object twice)      | Deterministic (sorted by name)     |

### Determinism Guarantees

- **Secret order**: Secrets are sorted by `.Name` before hashing.
- **Key order**: `.data` keys within each Secret are sorted lexicographically.
- **Binary safety**: Values are written as raw bytes; non-UTF-8 data is handled
  correctly.

---

## `fetchReferencedSecrets`

```go
func fetchReferencedSecrets(
    ctx context.Context,
    c client.Client,
    mc *memcachedv1alpha1.Memcached,
) ([]*corev1.Secret, []string)
```

Resolves the Secret references from the Memcached CR's `spec.security` into
actual Secret objects. Returns two slices:

- `found` — Secrets that were successfully fetched.
- `missing` — names of Secrets that could not be fetched (not found or error).

### Resolution Logic

1. **Collect unique names**: Inspect `spec.security.sasl.credentialsSecretRef.name`
   (when SASL is enabled) and `spec.security.tls.certificateSecretRef.name`
   (when TLS is enabled). Uses a `map[string]struct{}` to deduplicate — if both
   refs point to the same Secret name, it is fetched only once.
2. **Nil-safe checks**: Each level of the spec is nil-checked:
   - `spec.security` nil → no refs collected
   - `spec.security.sasl` nil → SASL ref skipped
   - `spec.security.tls` nil → TLS ref skipped
3. **Fetch each Secret**: For each unique name, performs `client.Get` with the
   Memcached CR's namespace. On success, the Secret is appended to `found`; on
   any error, the name is appended to `missing`.
4. **Empty case**: If no refs are collected (neither SASL nor TLS enabled, or
   both specs are nil), returns `nil, nil`.

### Behavior

| Spec State                     | `found`         | `missing`      |
|--------------------------------|-----------------|----------------|
| Neither SASL nor TLS enabled   | `nil`           | `nil`          |
| `spec.security` is nil         | `nil`           | `nil`          |
| SASL enabled, Secret exists    | `[saslSecret]`  | `[]`           |
| SASL enabled, Secret missing   | `[]`            | `["sasl-ref"]` |
| TLS enabled, Secret exists     | `[tlsSecret]`   | `[]`           |
| TLS enabled, Secret missing    | `[]`            | `["tls-ref"]`  |
| Both enabled, both exist       | `[sasl, tls]`   | `[]`           |
| Both enabled, same Secret name | `[secret]` (1x) | `[]`           |
| Both enabled, SASL missing     | `[tlsSecret]`   | `["sasl-ref"]` |

### Relationship to CRD Types

The function reads from these CRD field paths (defined in
`api/v1alpha1/memcached_types.go`):

```text
spec.security.sasl.credentialsSecretRef.name  → SASLSpec.CredentialsSecretRef
spec.security.tls.certificateSecretRef.name   → TLSSpec.CertificateSecretRef
```

Both use `corev1.LocalObjectReference`, which contains a single `Name` field.
The Secret is fetched from the same namespace as the Memcached CR.

---

## `mapSecretToMemcached`

```go
func mapSecretToMemcached(c client.Client) handler.MapFunc
```

Returns a `handler.MapFunc` closure that maps a Secret event to
`[]reconcile.Request` for Memcached CRs that reference the changed Secret. This
enables the controller to watch Secrets and re-reconcile affected Memcached CRs
when a Secret is updated (e.g., certificate rotation, credential change).

### Mapping Logic

1. **Extract Secret identity**: Read `Name` and `Namespace` from the event
   object.
2. **List Memcached CRs**: List all `MemcachedList` items in the Secret's
   namespace using `client.InNamespace`.
3. **Filter by reference**: For each Memcached CR, check if:
   - `spec.security.sasl.credentialsSecretRef.name` matches the Secret name, OR
   - `spec.security.tls.certificateSecretRef.name` matches the Secret name.
4. **Build requests**: For each matching CR, append a `reconcile.Request` with
   the CR's `NamespacedName`.
5. **Return**: The list of requests (may be empty if no CRs reference the
   Secret).

### Behavior

| Scenario                               | Result                                  |
|----------------------------------------|-----------------------------------------|
| CR's SASL ref matches Secret name      | `reconcile.Request` for that CR         |
| CR's TLS ref matches Secret name       | `reconcile.Request` for that CR         |
| No CR references the Secret            | Empty list                              |
| Multiple CRs reference the same Secret | One `reconcile.Request` per matching CR |
| CR in different namespace than Secret  | Not matched (namespace-scoped listing)  |
| CR has nil `spec.security`             | Safely skipped, no panic                |
| List API call fails                    | Returns `nil`                           |

### Namespace Scoping

The function only lists Memcached CRs in the same namespace as the changed
Secret (`client.InNamespace(secretNamespace)`). CRs in other namespaces are
never evaluated, ensuring correct multi-tenant behavior.

### Safety

- CRs with nil `spec.security` are skipped without panic.
- CRs with nil `spec.security.sasl` or `spec.security.tls` are handled — the
  SASL/TLS ref check is guarded by a nil check on the respective spec.

---

## Integration Point

These three functions are designed for integration into the reconciliation loop
(a separate feature). The intended usage pattern is:

```text
Secret updated
      │
      ▼
mapSecretToMemcached    ← watch handler triggers reconcile
      │
      ▼
fetchReferencedSecrets  ← reconciler resolves Secret refs
      │
      ▼
computeSecretHash       ← reconciler computes hash for annotation
      │
      ▼
Compare with existing annotation → update Deployment if changed
```

The hash is intended to be stored as a pod template annotation on the
Deployment, causing Kubernetes to roll pods when the hash changes (indicating
Secret content was modified).
