// Package controller implements the reconciliation logic for the memcached-operator.
package controller

import (
	"maps"
	"reflect"
	"testing"

	policyv1 "k8s.io/api/policy/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

	memcachedv1beta1 "github.com/c5c3/memcached-operator/api/v1beta1"
)

func TestConstructPDB(t *testing.T) {
	tests := []struct {
		name               string
		pdbSpec            *memcachedv1beta1.PDBSpec
		wantMinAvailable   *intstr.IntOrString
		wantMaxUnavailable *intstr.IntOrString
	}{
		{
			name:               "default minAvailable when neither set",
			pdbSpec:            &memcachedv1beta1.PDBSpec{Enabled: true},
			wantMinAvailable:   intOrStringPtr(intstr.FromInt32(1)),
			wantMaxUnavailable: nil,
		},
		{
			name:               "custom minAvailable integer",
			pdbSpec:            &memcachedv1beta1.PDBSpec{Enabled: true, MinAvailable: intOrStringPtr(intstr.FromInt32(2))},
			wantMinAvailable:   intOrStringPtr(intstr.FromInt32(2)),
			wantMaxUnavailable: nil,
		},
		{
			name:               "minAvailable percentage",
			pdbSpec:            &memcachedv1beta1.PDBSpec{Enabled: true, MinAvailable: intOrStringPtr(intstr.FromString("50%"))},
			wantMinAvailable:   intOrStringPtr(intstr.FromString("50%")),
			wantMaxUnavailable: nil,
		},
		{
			name:               "maxUnavailable integer",
			pdbSpec:            &memcachedv1beta1.PDBSpec{Enabled: true, MaxUnavailable: intOrStringPtr(intstr.FromInt32(1))},
			wantMinAvailable:   nil,
			wantMaxUnavailable: intOrStringPtr(intstr.FromInt32(1)),
		},
		{
			name:               "maxUnavailable percentage",
			pdbSpec:            &memcachedv1beta1.PDBSpec{Enabled: true, MaxUnavailable: intOrStringPtr(intstr.FromString("25%"))},
			wantMinAvailable:   nil,
			wantMaxUnavailable: intOrStringPtr(intstr.FromString("25%")),
		},
		{
			name: "minAvailable takes precedence when both set",
			pdbSpec: &memcachedv1beta1.PDBSpec{
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
			mc := &memcachedv1beta1.Memcached{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "my-cache",
					Namespace: "default",
				},
				Spec: memcachedv1beta1.MemcachedSpec{
					HighAvailability: &memcachedv1beta1.HighAvailabilitySpec{
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
	mc := &memcachedv1beta1.Memcached{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "label-test",
			Namespace: "default",
		},
		Spec: memcachedv1beta1.MemcachedSpec{
			HighAvailability: &memcachedv1beta1.HighAvailabilitySpec{
				PodDisruptionBudget: &memcachedv1beta1.PDBSpec{Enabled: true},
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
			mc := &memcachedv1beta1.Memcached{
				ObjectMeta: metav1.ObjectMeta{
					Name:      tt.instanceName,
					Namespace: "default",
				},
				Spec: memcachedv1beta1.MemcachedSpec{
					HighAvailability: &memcachedv1beta1.HighAvailabilitySpec{
						PodDisruptionBudget: &memcachedv1beta1.PDBSpec{Enabled: true},
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
		mc   *memcachedv1beta1.Memcached
		want bool
	}{
		{
			name: "nil HighAvailability",
			mc: &memcachedv1beta1.Memcached{
				Spec: memcachedv1beta1.MemcachedSpec{HighAvailability: nil},
			},
			want: false,
		},
		{
			name: "nil PodDisruptionBudget",
			mc: &memcachedv1beta1.Memcached{
				Spec: memcachedv1beta1.MemcachedSpec{
					HighAvailability: &memcachedv1beta1.HighAvailabilitySpec{
						PodDisruptionBudget: nil,
					},
				},
			},
			want: false,
		},
		{
			name: "enabled is false",
			mc: &memcachedv1beta1.Memcached{
				Spec: memcachedv1beta1.MemcachedSpec{
					HighAvailability: &memcachedv1beta1.HighAvailabilitySpec{
						PodDisruptionBudget: &memcachedv1beta1.PDBSpec{Enabled: false},
					},
				},
			},
			want: false,
		},
		{
			name: "enabled is true",
			mc: &memcachedv1beta1.Memcached{
				Spec: memcachedv1beta1.MemcachedSpec{
					HighAvailability: &memcachedv1beta1.HighAvailabilitySpec{
						PodDisruptionBudget: &memcachedv1beta1.PDBSpec{Enabled: true},
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

func TestConstructPDB_SwitchMinAvailableToMaxUnavailable(t *testing.T) {
	// Step 1: Create a PDB with minAvailable=2.
	mc := &memcachedv1beta1.Memcached{
		ObjectMeta: metav1.ObjectMeta{Name: "switch-min-max", Namespace: "default"},
		Spec: memcachedv1beta1.MemcachedSpec{
			HighAvailability: &memcachedv1beta1.HighAvailabilitySpec{
				PodDisruptionBudget: &memcachedv1beta1.PDBSpec{
					Enabled:      true,
					MinAvailable: intOrStringPtr(intstr.FromInt32(2)),
				},
			},
		},
	}
	pdb := &policyv1.PodDisruptionBudget{}

	constructPDB(mc, pdb)

	// Verify minAvailable is set to 2.
	if pdb.Spec.MinAvailable == nil {
		t.Fatal("after first call: expected MinAvailable to be set, got nil")
	}
	if *pdb.Spec.MinAvailable != intstr.FromInt32(2) {
		t.Errorf("after first call: MinAvailable = %v, want 2", *pdb.Spec.MinAvailable)
	}
	// Verify maxUnavailable is nil.
	if pdb.Spec.MaxUnavailable != nil {
		t.Errorf("after first call: expected MaxUnavailable nil, got %v", *pdb.Spec.MaxUnavailable)
	}

	// Step 2: Change the CR to use maxUnavailable=1, removing minAvailable.
	mc.Spec.HighAvailability.PodDisruptionBudget = &memcachedv1beta1.PDBSpec{
		Enabled:        true,
		MaxUnavailable: intOrStringPtr(intstr.FromInt32(1)),
	}

	constructPDB(mc, pdb)

	// Verify minAvailable is now nil (cleared).
	if pdb.Spec.MinAvailable != nil {
		t.Errorf("after second call: expected MinAvailable nil, got %v", *pdb.Spec.MinAvailable)
	}
	// Verify maxUnavailable is set to 1.
	if pdb.Spec.MaxUnavailable == nil {
		t.Fatal("after second call: expected MaxUnavailable to be set, got nil")
	}
	if *pdb.Spec.MaxUnavailable != intstr.FromInt32(1) {
		t.Errorf("after second call: MaxUnavailable = %v, want 1", *pdb.Spec.MaxUnavailable)
	}

	// Verify labels and selector are still correctly set.
	expectedLabels := labelsForMemcached("switch-min-max")
	for k, v := range expectedLabels {
		if pdb.Labels[k] != v {
			t.Errorf("after second call: label %q = %q, want %q", k, pdb.Labels[k], v)
		}
	}
	if pdb.Spec.Selector == nil {
		t.Fatal("after second call: expected non-nil selector")
	}
	for k, v := range expectedLabels {
		if pdb.Spec.Selector.MatchLabels[k] != v {
			t.Errorf("after second call: selector %q = %q, want %q", k, pdb.Spec.Selector.MatchLabels[k], v)
		}
	}
}

func TestConstructPDB_SwitchMaxUnavailableToMinAvailable(t *testing.T) {
	// Step 1: Create a PDB with maxUnavailable=1.
	mc := &memcachedv1beta1.Memcached{
		ObjectMeta: metav1.ObjectMeta{Name: "switch-max-min", Namespace: "default"},
		Spec: memcachedv1beta1.MemcachedSpec{
			HighAvailability: &memcachedv1beta1.HighAvailabilitySpec{
				PodDisruptionBudget: &memcachedv1beta1.PDBSpec{
					Enabled:        true,
					MaxUnavailable: intOrStringPtr(intstr.FromInt32(1)),
				},
			},
		},
	}
	pdb := &policyv1.PodDisruptionBudget{}

	constructPDB(mc, pdb)

	// Verify maxUnavailable is set to 1.
	if pdb.Spec.MaxUnavailable == nil {
		t.Fatal("after first call: expected MaxUnavailable to be set, got nil")
	}
	if *pdb.Spec.MaxUnavailable != intstr.FromInt32(1) {
		t.Errorf("after first call: MaxUnavailable = %v, want 1", *pdb.Spec.MaxUnavailable)
	}
	// Verify minAvailable is nil.
	if pdb.Spec.MinAvailable != nil {
		t.Errorf("after first call: expected MinAvailable nil, got %v", *pdb.Spec.MinAvailable)
	}

	// Step 2: Change the CR to use minAvailable=3, removing maxUnavailable.
	mc.Spec.HighAvailability.PodDisruptionBudget = &memcachedv1beta1.PDBSpec{
		Enabled:      true,
		MinAvailable: intOrStringPtr(intstr.FromInt32(3)),
	}

	constructPDB(mc, pdb)

	// Verify maxUnavailable is now nil (cleared).
	if pdb.Spec.MaxUnavailable != nil {
		t.Errorf("after second call: expected MaxUnavailable nil, got %v", *pdb.Spec.MaxUnavailable)
	}
	// Verify minAvailable is set to 3.
	if pdb.Spec.MinAvailable == nil {
		t.Fatal("after second call: expected MinAvailable to be set, got nil")
	}
	if *pdb.Spec.MinAvailable != intstr.FromInt32(3) {
		t.Errorf("after second call: MinAvailable = %v, want 3", *pdb.Spec.MinAvailable)
	}

	// Verify labels and selector are still correctly set.
	expectedLabels := labelsForMemcached("switch-max-min")
	for k, v := range expectedLabels {
		if pdb.Labels[k] != v {
			t.Errorf("after second call: label %q = %q, want %q", k, pdb.Labels[k], v)
		}
	}
	if pdb.Spec.Selector == nil {
		t.Fatal("after second call: expected non-nil selector")
	}
	for k, v := range expectedLabels {
		if pdb.Spec.Selector.MatchLabels[k] != v {
			t.Errorf("after second call: selector %q = %q, want %q", k, pdb.Spec.Selector.MatchLabels[k], v)
		}
	}
}

func TestConstructPDB_Idempotent(t *testing.T) {
	mc := &memcachedv1beta1.Memcached{
		ObjectMeta: metav1.ObjectMeta{Name: "idempotent-pdb", Namespace: "default"},
		Spec: memcachedv1beta1.MemcachedSpec{
			HighAvailability: &memcachedv1beta1.HighAvailabilitySpec{
				PodDisruptionBudget: &memcachedv1beta1.PDBSpec{
					Enabled:      true,
					MinAvailable: intOrStringPtr(intstr.FromInt32(2)),
				},
			},
		},
	}
	pdb := &policyv1.PodDisruptionBudget{}

	// First call.
	constructPDB(mc, pdb)
	firstLabels := maps.Clone(pdb.Labels)
	firstSpec := *pdb.Spec.DeepCopy()

	// Second call with the same CR on the same PDB object.
	constructPDB(mc, pdb)

	// Verify Labels unchanged.
	if !reflect.DeepEqual(pdb.Labels, firstLabels) {
		t.Errorf("Labels changed: got %v, want %v", pdb.Labels, firstLabels)
	}

	// Verify full PDB spec is identical after the second call.
	if !reflect.DeepEqual(pdb.Spec, firstSpec) {
		t.Errorf("PDB spec changed between calls:\nfirst:  %+v\nsecond: %+v", firstSpec, pdb.Spec)
	}
}

func intOrStringPtr(val intstr.IntOrString) *intstr.IntOrString {
	return &val
}
