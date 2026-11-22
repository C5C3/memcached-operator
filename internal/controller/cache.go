// Package controller implements the reconciliation logic for the memcached-operator.
package controller

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"

	memcachedv1beta1 "github.com/c5c3/memcached-operator/api/v1beta1"
)

// CacheOptions returns the manager's cache configuration. defaultNamespaces is
// the result of parseWatchNamespaces; nil means every namespace.
//
// Every kind is cached only for objects with the managed-by label that
// labelsForMemcached sets. Memcached resources are cached whatever their labels,
// and so are Secrets, which the reconciler watches as metadata only; their
// payload is read from the API server. The selector is the cache-wide default
// rather than a ByObject entry per owned kind, because a ByObject entry is
// resolved through the RESTMapper when the cache is built.
func CacheOptions(defaultNamespaces map[string]cache.Config) cache.Options {
	return cache.Options{
		DefaultNamespaces:    defaultNamespaces,
		DefaultLabelSelector: labels.SelectorFromSet(labels.Set{labelKeyManagedBy: labelValueManagedBy}),
		ByObject: map[client.Object]cache.ByObject{
			&memcachedv1beta1.Memcached{}: {Label: labels.Everything()},
			&corev1.Secret{}:              {Label: labels.Everything(), Transform: stripSecretMetadata},
		},
	}
}

// stripSecretMetadata is the cache transform for Secrets. For the metadata-only
// form it drops the labels, annotations, managedFields, ownerReferences, and
// finalizers, and keeps the name, namespace, UID, resourceVersion, and
// timestamps. Any other value is returned unchanged. It never touches a typed
// Secret: dropping its data would turn a cached read into an empty secret hash
// and a rolling restart.
func stripSecretMetadata(in any) (any, error) {
	if obj, ok := in.(*metav1.PartialObjectMetadata); ok && obj != nil {
		obj.Labels = nil
		obj.Annotations = nil
		obj.ManagedFields = nil
		obj.OwnerReferences = nil
		obj.Finalizers = nil
	}
	return in, nil
}
