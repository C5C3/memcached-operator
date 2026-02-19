// Package v1alpha1 contains unit tests for scheme registration, GVK resolution,
// and type mapping of the Memcached v1alpha1 API types.
package v1alpha1

import (
	"reflect"
	"testing"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

func TestSchemeBuilder_IsNotNil(t *testing.T) {
	if SchemeBuilder == nil {
		t.Fatal("expected SchemeBuilder to be non-nil")
	}
}

func TestScheme_GroupVersionFields(t *testing.T) {
	if GroupVersion.Group != "memcached.c5c3.io" {
		t.Errorf("expected GroupVersion.Group=%q, got %q", "memcached.c5c3.io", GroupVersion.Group)
	}
	if GroupVersion.Version != "v1alpha1" {
		t.Errorf("expected GroupVersion.Version=%q, got %q", "v1alpha1", GroupVersion.Version)
	}
}

func TestScheme_AddToSchemeSucceeds(t *testing.T) {
	s := runtime.NewScheme()
	if err := AddToScheme(s); err != nil {
		t.Fatalf("AddToScheme returned unexpected error: %v", err)
	}
}

func TestScheme_GVKResolution(t *testing.T) {
	s := runtime.NewScheme()
	if err := AddToScheme(s); err != nil {
		t.Fatalf("AddToScheme failed: %v", err)
	}

	tests := []struct {
		name        string
		obj         runtime.Object
		expectedGVK schema.GroupVersionKind
	}{
		{
			name: "Memcached resolves to correct GVK",
			obj:  &Memcached{},
			expectedGVK: schema.GroupVersionKind{
				Group:   "memcached.c5c3.io",
				Version: "v1alpha1",
				Kind:    "Memcached",
			},
		},
		{
			name: "MemcachedList resolves to correct GVK",
			obj:  &MemcachedList{},
			expectedGVK: schema.GroupVersionKind{
				Group:   "memcached.c5c3.io",
				Version: "v1alpha1",
				Kind:    "MemcachedList",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gvks, _, err := s.ObjectKinds(tt.obj)
			if err != nil {
				t.Fatalf("ObjectKinds returned error: %v", err)
			}
			if len(gvks) == 0 {
				t.Fatal("expected at least one GVK, got none")
			}
			if gvks[0] != tt.expectedGVK {
				t.Errorf("expected GVK %v, got %v", tt.expectedGVK, gvks[0])
			}
		})
	}
}

func TestScheme_TypeMappingFromGVK(t *testing.T) {
	s := runtime.NewScheme()
	if err := AddToScheme(s); err != nil {
		t.Fatalf("AddToScheme failed: %v", err)
	}

	tests := []struct {
		name         string
		gvk          schema.GroupVersionKind
		expectedType reflect.Type
	}{
		{
			name: "GVK maps to *Memcached",
			gvk: schema.GroupVersionKind{
				Group:   "memcached.c5c3.io",
				Version: "v1alpha1",
				Kind:    "Memcached",
			},
			expectedType: reflect.TypeOf(&Memcached{}),
		},
		{
			name: "GVK maps to *MemcachedList",
			gvk: schema.GroupVersionKind{
				Group:   "memcached.c5c3.io",
				Version: "v1alpha1",
				Kind:    "MemcachedList",
			},
			expectedType: reflect.TypeOf(&MemcachedList{}),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			obj, err := s.New(tt.gvk)
			if err != nil {
				t.Fatalf("scheme.New(%v) returned error: %v", tt.gvk, err)
			}
			actualType := reflect.TypeOf(obj)
			if actualType != tt.expectedType {
				t.Errorf("expected type %v, got %v", tt.expectedType, actualType)
			}
		})
	}
}
