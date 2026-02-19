// Package controller implements the reconciliation logic for the memcached-operator.
package controller

import (
	"testing"

	policyv1 "k8s.io/api/policy/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

	memcachedv1alpha1 "github.com/c5c3/memcached-operator/api/v1alpha1"
)

func TestConstructPDB(t *testing.T) {
	tests := []struct {
		name               string
		pdbSpec            *memcachedv1alpha1.PDBSpec
		wantMinAvailable   *intstr.IntOrString
		wantMaxUnavailable *intstr.IntOrString
	}{
		{
			name:               "default minAvailable when neither set",
			pdbSpec:            &memcachedv1alpha1.PDBSpec{Enabled: true},
			wantMinAvailable:   intOrStringPtr(intstr.FromInt32(1)),
			wantMaxUnavailable: nil,
		},
		{
			name:               "custom minAvailable integer",
			pdbSpec:            &memcachedv1alpha1.PDBSpec{Enabled: true, MinAvailable: intOrStringPtr(intstr.FromInt32(2))},
			wantMinAvailable:   intOrStringPtr(intstr.FromInt32(2)),
			wantMaxUnavailable: nil,
		},
		{
			name:               "minAvailable percentage",
			pdbSpec:            &memcachedv1alpha1.PDBSpec{Enabled: true, MinAvailable: intOrStringPtr(intstr.FromString("50%"))},
			wantMinAvailable:   intOrStringPtr(intstr.FromString("50%")),
			wantMaxUnavailable: nil,
		},
		{
			name:               "maxUnavailable integer",
			pdbSpec:            &memcachedv1alpha1.PDBSpec{Enabled: true, MaxUnavailable: intOrStringPtr(intstr.FromInt32(1))},
			wantMinAvailable:   nil,
			wantMaxUnavailable: intOrStringPtr(intstr.FromInt32(1)),
		},
		{
			name:               "maxUnavailable percentage",
			pdbSpec:            &memcachedv1alpha1.PDBSpec{Enabled: true, MaxUnavailable: intOrStringPtr(intstr.FromString("25%"))},
			wantMinAvailable:   nil,
			wantMaxUnavailable: intOrStringPtr(intstr.FromString("25%")),
		},
		{
			name: "minAvailable takes precedence when both set",
			pdbSpec: &memcachedv1alpha1.PDBSpec{
				Enabled:        true,
				MinAvailable:   intOrStringPtr(intstr.FromInt32(1)),
				MaxUnavailable: intOrStringPtr(intstr.FromInt32(1)),
			},
			wantMinAvailable:   intOrStringPtr(intstr.FromInt32(1)),
			wantMaxUnavailable: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mc := &memcachedv1alpha1.Memcached{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "my-cache",
					Namespace: "default",
				},
				Spec: memcachedv1alpha1.MemcachedSpec{
					HighAvailability: &memcachedv1alpha1.HighAvailabilitySpec{
						PodDisruptionBudget: tt.pdbSpec,
					},
				},
			}
			pdb := &policyv1.PodDisruptionBudget{}

			constructPDB(mc, pdb)

			if tt.wantMinAvailable == nil {
				if pdb.Spec.MinAvailable != nil {
					t.Errorf("expected nil MinAvailable, got %v", pdb.Spec.MinAvailable)
				}
			} else {
				if pdb.Spec.MinAvailable == nil {
					t.Fatalf("expected MinAvailable %v, got nil", *tt.wantMinAvailable)
				}
				if *pdb.Spec.MinAvailable != *tt.wantMinAvailable {
					t.Errorf("MinAvailable = %v, want %v", *pdb.Spec.MinAvailable, *tt.wantMinAvailable)
				}
			}

			if tt.wantMaxUnavailable == nil {
				if pdb.Spec.MaxUnavailable != nil {
					t.Errorf("expected nil MaxUnavailable, got %v", pdb.Spec.MaxUnavailable)
				}
			} else {
				if pdb.Spec.MaxUnavailable == nil {
					t.Fatalf("expected MaxUnavailable %v, got nil", *tt.wantMaxUnavailable)
				}
				if *pdb.Spec.MaxUnavailable != *tt.wantMaxUnavailable {
					t.Errorf("MaxUnavailable = %v, want %v", *pdb.Spec.MaxUnavailable, *tt.wantMaxUnavailable)
				}
			}
		})
	}
}

func TestConstructPDB_Labels(t *testing.T) {
	mc := &memcachedv1alpha1.Memcached{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "label-test",
			Namespace: "default",
		},
		Spec: memcachedv1alpha1.MemcachedSpec{
			HighAvailability: &memcachedv1alpha1.HighAvailabilitySpec{
				PodDisruptionBudget: &memcachedv1alpha1.PDBSpec{Enabled: true},
			},
		},
	}
	pdb := &policyv1.PodDisruptionBudget{}

	constructPDB(mc, pdb)

	expectedLabels := labelsForMemcached("label-test")

	// Metadata labels.
	if len(pdb.Labels) != len(expectedLabels) {
		t.Errorf("expected %d metadata labels, got %d", len(expectedLabels), len(pdb.Labels))
	}
	for k, v := range expectedLabels {
		if pdb.Labels[k] != v {
			t.Errorf("label %q = %q, want %q", k, pdb.Labels[k], v)
		}
	}

	// Selector labels.
	if pdb.Spec.Selector == nil {
		t.Fatal("expected non-nil selector")
	}
	if len(pdb.Spec.Selector.MatchLabels) != len(expectedLabels) {
		t.Errorf("expected %d selector labels, got %d", len(expectedLabels), len(pdb.Spec.Selector.MatchLabels))
	}
	for k, v := range expectedLabels {
		if pdb.Spec.Selector.MatchLabels[k] != v {
			t.Errorf("selector %q = %q, want %q", k, pdb.Spec.Selector.MatchLabels[k], v)
		}
	}
}

func TestConstructPDB_InstanceScopedSelector(t *testing.T) {
	tests := []struct {
		name         string
		instanceName string
	}{
		{name: "cache-alpha", instanceName: "cache-alpha"},
		{name: "cache-beta", instanceName: "cache-beta"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mc := &memcachedv1alpha1.Memcached{
				ObjectMeta: metav1.ObjectMeta{
					Name:      tt.instanceName,
					Namespace: "default",
				},
				Spec: memcachedv1alpha1.MemcachedSpec{
					HighAvailability: &memcachedv1alpha1.HighAvailabilitySpec{
						PodDisruptionBudget: &memcachedv1alpha1.PDBSpec{Enabled: true},
					},
				},
			}
			pdb := &policyv1.PodDisruptionBudget{}

			constructPDB(mc, pdb)

			got := pdb.Spec.Selector.MatchLabels["app.kubernetes.io/instance"]
			if got != tt.instanceName {
				t.Errorf("selector app.kubernetes.io/instance = %q, want %q", got, tt.instanceName)
			}
		})
	}
}

func TestPDBEnabled(t *testing.T) {
	tests := []struct {
		name string
		mc   *memcachedv1alpha1.Memcached
		want bool
	}{
		{
			name: "nil HighAvailability",
			mc: &memcachedv1alpha1.Memcached{
				Spec: memcachedv1alpha1.MemcachedSpec{HighAvailability: nil},
			},
			want: false,
		},
		{
			name: "nil PodDisruptionBudget",
			mc: &memcachedv1alpha1.Memcached{
				Spec: memcachedv1alpha1.MemcachedSpec{
					HighAvailability: &memcachedv1alpha1.HighAvailabilitySpec{
						PodDisruptionBudget: nil,
					},
				},
			},
			want: false,
		},
		{
			name: "enabled is false",
			mc: &memcachedv1alpha1.Memcached{
				Spec: memcachedv1alpha1.MemcachedSpec{
					HighAvailability: &memcachedv1alpha1.HighAvailabilitySpec{
						PodDisruptionBudget: &memcachedv1alpha1.PDBSpec{Enabled: false},
					},
				},
			},
			want: false,
		},
		{
			name: "enabled is true",
			mc: &memcachedv1alpha1.Memcached{
				Spec: memcachedv1alpha1.MemcachedSpec{
					HighAvailability: &memcachedv1alpha1.HighAvailabilitySpec{
						PodDisruptionBudget: &memcachedv1alpha1.PDBSpec{Enabled: true},
					},
				},
			},
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := pdbEnabled(tt.mc)
			if got != tt.want {
				t.Errorf("pdbEnabled() = %v, want %v", got, tt.want)
			}
		})
	}
}

func intOrStringPtr(val intstr.IntOrString) *intstr.IntOrString {
	return &val
}
