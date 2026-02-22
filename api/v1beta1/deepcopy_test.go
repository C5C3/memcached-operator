package v1beta1

import (
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestMemcachedDeepCopy_Independence(t *testing.T) {
	replicas := int32(3)
	image := DefaultImage

	original := &Memcached{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "memcached.c5c3.io/v1beta1",
			Kind:       "Memcached",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:        "test-mc",
			Namespace:   "default",
			Labels:      map[string]string{"app": "memcached"},
			Annotations: map[string]string{"note": "original"},
		},
		Spec: MemcachedSpec{
			Replicas: &replicas,
			Image:    &image,
			Memcached: &MemcachedConfig{
				MaxMemoryMB: 64,
				ExtraArgs:   []string{"-o", "modern"},
			},
		},
		Status: MemcachedStatus{
			Conditions: []metav1.Condition{
				{
					Type:   "Available",
					Status: metav1.ConditionTrue,
					Reason: "Ready",
				},
			},
			ReadyReplicas:      3,
			ObservedGeneration: 1,
		},
	}

	clone := original.DeepCopy()

	if clone == original {
		t.Fatal("DeepCopy returned the same pointer")
	}

	// Mutate Labels in original.
	original.Labels["app"] = "changed"
	if clone.Labels["app"] != "memcached" {
		t.Error("clone Labels were affected by mutating original Labels")
	}

	// Mutate Annotations in original.
	original.Annotations["note"] = "mutated"
	if clone.Annotations["note"] != "original" {
		t.Error("clone Annotations were affected by mutating original Annotations")
	}

	// Mutate Replicas pointer in original.
	*original.Spec.Replicas = 99
	if *clone.Spec.Replicas != 3 {
		t.Error("clone Spec.Replicas was affected by mutating original")
	}

	// Mutate Image pointer in original.
	*original.Spec.Image = "changed:latest"
	if *clone.Spec.Image != DefaultImage {
		t.Error("clone Spec.Image was affected by mutating original")
	}

	// Mutate ExtraArgs slice in original.
	original.Spec.Memcached.ExtraArgs[0] = "-v"
	if clone.Spec.Memcached.ExtraArgs[0] != "-o" {
		t.Error("clone Spec.Memcached.ExtraArgs was affected by mutating original")
	}

	// Mutate Conditions slice in original.
	original.Status.Conditions[0].Reason = "NotReady"
	if clone.Status.Conditions[0].Reason != "Ready" {
		t.Error("clone Status.Conditions was affected by mutating original")
	}

	// Mutate scalar status fields.
	original.Status.ReadyReplicas = 0
	if clone.Status.ReadyReplicas != 3 {
		t.Error("clone Status.ReadyReplicas was affected by mutating original")
	}

	original.Status.ObservedGeneration = 99
	if clone.Status.ObservedGeneration != 1 {
		t.Error("clone Status.ObservedGeneration was affected by mutating original")
	}
}

func TestMemcachedDeepCopy_NilReceiver(t *testing.T) {
	var mc *Memcached
	clone := mc.DeepCopy()
	if clone != nil {
		t.Error("DeepCopy of nil receiver should return nil")
	}
}

func TestMemcachedDeepCopyObject_ReturnsRuntimeObject(t *testing.T) {
	replicas := int32(2)
	original := &Memcached{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-mc",
			Namespace: "default",
		},
		Spec: MemcachedSpec{
			Replicas: &replicas,
		},
	}

	obj := original.DeepCopyObject()

	mc, ok := obj.(*Memcached)
	if !ok {
		t.Fatal("DeepCopyObject did not return a *Memcached")
	}

	if mc == original {
		t.Error("DeepCopyObject returned the same pointer")
	}

	if mc.Name != "test-mc" {
		t.Errorf("expected Name=test-mc, got %s", mc.Name)
	}
	if mc.Namespace != "default" {
		t.Errorf("expected Namespace=default, got %s", mc.Namespace)
	}
	if *mc.Spec.Replicas != 2 {
		t.Errorf("expected Replicas=2, got %d", *mc.Spec.Replicas)
	}
}

func TestMemcachedListDeepCopy_Independence(t *testing.T) {
	original := &MemcachedList{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "memcached.c5c3.io/v1beta1",
			Kind:       "MemcachedList",
		},
		Items: []Memcached{
			{
				ObjectMeta: metav1.ObjectMeta{
					Name:   "mc-0",
					Labels: map[string]string{"index": "0"},
				},
			},
			{
				ObjectMeta: metav1.ObjectMeta{
					Name:   "mc-1",
					Labels: map[string]string{"index": "1"},
				},
			},
		},
	}

	clone := original.DeepCopy()

	if clone == original {
		t.Fatal("DeepCopy returned the same pointer")
	}

	// Verify initial length.
	if len(clone.Items) != 2 {
		t.Fatalf("expected 2 items in clone, got %d", len(clone.Items))
	}

	// Append to original Items slice.
	original.Items = append(original.Items, Memcached{
		ObjectMeta: metav1.ObjectMeta{Name: "mc-2"},
	})
	if len(clone.Items) != 2 {
		t.Error("clone Items length was affected by appending to original")
	}

	// Mutate existing item's Labels in original.
	original.Items[0].Labels["index"] = "changed"
	if clone.Items[0].Labels["index"] != "0" {
		t.Error("clone Items[0].Labels was affected by mutating original")
	}

	// Mutate existing item's Name in original.
	original.Items[1].Name = "renamed"
	if clone.Items[1].Name != "mc-1" {
		t.Error("clone Items[1].Name was affected by mutating original")
	}
}

func TestMemcachedListDeepCopyObject_ReturnsRuntimeObject(t *testing.T) {
	original := &MemcachedList{
		Items: []Memcached{
			{ObjectMeta: metav1.ObjectMeta{Name: "mc-0"}},
		},
	}

	obj := original.DeepCopyObject()

	ml, ok := obj.(*MemcachedList)
	if !ok {
		t.Fatal("DeepCopyObject did not return a *MemcachedList")
	}

	if ml == original {
		t.Error("DeepCopyObject returned the same pointer")
	}

	if len(ml.Items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(ml.Items))
	}
	if ml.Items[0].Name != "mc-0" {
		t.Errorf("expected item Name=mc-0, got %s", ml.Items[0].Name)
	}
}

func TestMemcachedListDeepCopy_NilReceiver(t *testing.T) {
	var ml *MemcachedList
	clone := ml.DeepCopy()
	if clone != nil {
		t.Error("DeepCopy of nil receiver should return nil")
	}
}
