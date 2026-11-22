package controller

import (
	"maps"
	"slices"
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	toolscache "k8s.io/client-go/tools/cache"
	"sigs.k8s.io/controller-runtime/pkg/cache"

	memcachedv1beta1 "github.com/c5c3/memcached-operator/api/v1beta1"
)

func TestCacheOptions_DefaultLabelSelector(t *testing.T) {
	selector := CacheOptions(nil).DefaultLabelSelector
	if selector == nil {
		t.Fatal("expected a default label selector")
	}
	if got := selector.String(); got != "app.kubernetes.io/managed-by=memcached-operator" {
		t.Errorf("unexpected selector %q", got)
	}
	if !selector.Matches(labels.Set(labelsForMemcached("x"))) {
		t.Error("expected the selector to match the labels of an owned object")
	}
}

func TestCacheOptions_ByObject(t *testing.T) {
	byObject := CacheOptions(nil).ByObject
	if len(byObject) != 2 {
		t.Fatalf("expected 2 ByObject entries, got %d", len(byObject))
	}

	var memcachedEntry, secretEntry *cache.ByObject
	for obj, entry := range byObject {
		switch obj.(type) {
		case *memcachedv1beta1.Memcached:
			memcachedEntry = &entry
		case *corev1.Secret:
			secretEntry = &entry
		default:
			t.Errorf("unexpected ByObject key %T", obj)
		}
	}
	if memcachedEntry == nil || secretEntry == nil {
		t.Fatalf("expected entries for Memcached and Secret, got %v", byObject)
	}

	if memcachedEntry.Label == nil || !memcachedEntry.Label.Empty() {
		t.Errorf("expected an empty, non-nil label selector for Memcached, got %v", memcachedEntry.Label)
	}
	if secretEntry.Label == nil || !secretEntry.Label.Empty() {
		t.Errorf("expected an empty, non-nil label selector for Secret, got %v", secretEntry.Label)
	}
	if secretEntry.Transform == nil {
		t.Error("expected a transform for Secret")
	}
	if memcachedEntry.Transform != nil {
		t.Error("expected no transform for Memcached")
	}
}

func TestCacheOptions_DefaultNamespaces(t *testing.T) {
	tests := []struct {
		name     string
		in       map[string]cache.Config
		wantNil  bool
		wantKeys []string
	}{
		{name: "nil map", in: nil, wantNil: true},
		{name: "empty map", in: map[string]cache.Config{}, wantKeys: []string{}},
		{name: "two namespaces", in: map[string]cache.Config{"ns1": {}, "ns2": {}}, wantKeys: []string{"ns1", "ns2"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := CacheOptions(tt.in).DefaultNamespaces
			if tt.wantNil {
				if got != nil {
					t.Errorf("expected nil DefaultNamespaces, got %v", got)
				}
				return
			}
			if got == nil {
				t.Fatal("expected non-nil DefaultNamespaces")
			}
			if keys := slices.Sorted(maps.Keys(got)); !slices.Equal(keys, tt.wantKeys) {
				t.Errorf("expected namespaces %v, got %v", tt.wantKeys, keys)
			}
		})
	}
}

func TestStripSecretMetadata_PartialObjectMetadata(t *testing.T) {
	in := &metav1.PartialObjectMetadata{
		TypeMeta: metav1.TypeMeta{APIVersion: "v1", Kind: "Secret"},
		ObjectMeta: metav1.ObjectMeta{
			Name:            testSASLSecretName,
			Namespace:       testDefaultNamespace,
			UID:             testOwnerUID,
			ResourceVersion: "42",
			Labels:          map[string]string{testLabelKeyEnv: testLabelValueEnvProduction},
			Annotations:     map[string]string{testAnnotationKeyPrometheusScrape: testAnnotationValuePrometheusScrape},
			ManagedFields:   []metav1.ManagedFieldsEntry{{Manager: "kubectl"}},
			OwnerReferences: []metav1.OwnerReference{{Name: "owner"}},
			Finalizers:      []string{"example.com/finalizer"},
		},
	}

	out, err := stripSecretMetadata(in)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got, ok := out.(*metav1.PartialObjectMetadata)
	if !ok {
		t.Fatalf("expected *metav1.PartialObjectMetadata, got %T", out)
	}
	if got.Labels != nil || got.Annotations != nil || got.ManagedFields != nil || got.OwnerReferences != nil || got.Finalizers != nil {
		t.Errorf("expected labels, annotations, managedFields, ownerReferences, and finalizers to be nil, got %+v", got.ObjectMeta)
	}
	if got.Name != testSASLSecretName || got.Namespace != testDefaultNamespace || got.UID != testOwnerUID || got.ResourceVersion != "42" {
		t.Errorf("expected name, namespace, UID, and resourceVersion unchanged, got %+v", got.ObjectMeta)
	}

	want := got.DeepCopy()
	again, err := stripSecretMetadata(got)
	if err != nil {
		t.Fatalf("unexpected error on the second call: %v", err)
	}
	if !equality.Semantic.DeepEqual(again, want) {
		t.Errorf("expected a second call to return an equal object, got %+v", again)
	}
}

func TestStripSecretMetadata_OtherTypes(t *testing.T) {
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:   testSASLSecretName,
			Labels: map[string]string{testLabelKeyEnv: testLabelValueEnvProduction},
		},
		Data: map[string][]byte{testSecretDataKey: []byte("value")},
	}
	want := secret.DeepCopy()
	out, err := stripSecretMetadata(secret)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out != secret || !equality.Semantic.DeepEqual(secret, want) {
		t.Errorf("expected the typed Secret unchanged, got %+v", out)
	}

	tombstone := toolscache.DeletedFinalStateUnknown{}
	out, err = stripSecretMetadata(tombstone)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out != tombstone {
		t.Errorf("expected the tombstone unchanged, got %+v", out)
	}
}

func TestStripSecretMetadata_Nil(t *testing.T) {
	out, err := stripSecretMetadata(nil)
	if out != nil || err != nil {
		t.Errorf("expected nil, nil; got %v, %v", out, err)
	}
}
