// Package controller implements the reconciliation logic for the memcached-operator.
package controller

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"sort"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	memcachedv1alpha1 "github.com/c5c3/memcached-operator/api/v1alpha1"
)

// computeSecretHash returns a deterministic SHA-256 hex digest over the given Secrets' data.
// It returns an empty string if no Secrets are provided or all have nil/empty data.
func computeSecretHash(secrets ...*corev1.Secret) string {
	if len(secrets) == 0 {
		return ""
	}

	// Check whether any Secret has non-empty data.
	hasData := false
	for _, s := range secrets {
		if len(s.Data) > 0 {
			hasData = true
			break
		}
	}
	if !hasData {
		return ""
	}

	// Sort Secrets by name for determinism.
	sorted := make([]*corev1.Secret, len(secrets))
	copy(sorted, secrets)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].Name < sorted[j].Name
	})

	h := sha256.New()
	for _, s := range sorted {
		keys := make([]string, 0, len(s.Data))
		for k := range s.Data {
			keys = append(keys, k)
		}
		sort.Strings(keys)

		for _, k := range keys {
			h.Write([]byte(s.Name))
			h.Write([]byte{0})
			h.Write([]byte(k))
			h.Write([]byte{0})
			h.Write(s.Data[k])
		}
	}

	return hex.EncodeToString(h.Sum(nil))
}

// fetchReferencedSecrets collects the Secrets referenced by the Memcached CR's Security spec
// (SASL credentials and TLS certificates). It returns the found Secrets and the names of
// any that could not be fetched.
func fetchReferencedSecrets(ctx context.Context, c client.Client, mc *memcachedv1alpha1.Memcached) ([]*corev1.Secret, []string) {
	if mc.Spec.Security == nil {
		return nil, nil
	}

	names := make(map[string]struct{})

	if mc.Spec.Security.SASL != nil && mc.Spec.Security.SASL.Enabled {
		if name := mc.Spec.Security.SASL.CredentialsSecretRef.Name; name != "" {
			names[name] = struct{}{}
		}
	}
	if mc.Spec.Security.TLS != nil && mc.Spec.Security.TLS.Enabled {
		if name := mc.Spec.Security.TLS.CertificateSecretRef.Name; name != "" {
			names[name] = struct{}{}
		}
	}

	if len(names) == 0 {
		return nil, nil
	}

	var found []*corev1.Secret
	var missing []string

	for name := range names {
		secret := &corev1.Secret{}
		key := types.NamespacedName{Namespace: mc.Namespace, Name: name}
		if err := c.Get(ctx, key, secret); err != nil {
			missing = append(missing, name)
		} else {
			found = append(found, secret)
		}
	}

	return found, missing
}

// mapSecretToMemcached returns a handler.MapFunc that maps a Secret event to
// reconcile.Requests for all Memcached CRs in the same namespace that reference
// the Secret via their Security spec.
func mapSecretToMemcached(c client.Client) handler.MapFunc {
	return func(ctx context.Context, obj client.Object) []reconcile.Request {
		secretName := obj.GetName()
		secretNamespace := obj.GetNamespace()

		var list memcachedv1alpha1.MemcachedList
		if err := c.List(ctx, &list, client.InNamespace(secretNamespace)); err != nil {
			return nil
		}

		var requests []reconcile.Request
		for i := range list.Items {
			mc := &list.Items[i]
			if mc.Spec.Security == nil {
				continue
			}

			matched := false
			if mc.Spec.Security.SASL != nil && mc.Spec.Security.SASL.CredentialsSecretRef.Name == secretName {
				matched = true
			}
			if mc.Spec.Security.TLS != nil && mc.Spec.Security.TLS.CertificateSecretRef.Name == secretName {
				matched = true
			}

			if matched {
				requests = append(requests, reconcile.Request{
					NamespacedName: types.NamespacedName{
						Name:      mc.Name,
						Namespace: mc.Namespace,
					},
				})
			}
		}

		return requests
	}
}
