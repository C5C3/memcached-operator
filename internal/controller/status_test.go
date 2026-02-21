// Package controller implements the reconciliation logic for the memcached-operator.
package controller

import (
	"strings"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	memcachedv1alpha1 "github.com/c5c3/memcached-operator/api/v1alpha1"
)

func TestComputeConditions(t *testing.T) {
	tests := []struct {
		name           string
		replicas       *int32
		dep            *appsv1.Deployment
		wantAvailable  metav1.ConditionStatus
		availReason    string
		wantProgress   metav1.ConditionStatus
		progressReason string
		wantDegraded   metav1.ConditionStatus
		degradeReason  string
	}{
		{
			name:           "fully available",
			replicas:       int32Ptr(3),
			dep:            depWithStatus(3, 3, 3),
			wantAvailable:  metav1.ConditionTrue,
			availReason:    ConditionReasonAvailable,
			wantProgress:   metav1.ConditionFalse,
			progressReason: ConditionReasonProgressingComplete,
			wantDegraded:   metav1.ConditionFalse,
			degradeReason:  ConditionReasonNotDegraded,
		},
		{
			name:           "zero replicas desired",
			replicas:       int32Ptr(0),
			dep:            depWithStatus(0, 0, 0),
			wantAvailable:  metav1.ConditionFalse,
			availReason:    ConditionReasonUnavailable,
			wantProgress:   metav1.ConditionFalse,
			progressReason: ConditionReasonProgressingComplete,
			wantDegraded:   metav1.ConditionFalse,
			degradeReason:  ConditionReasonNotDegraded,
		},
		{
			name:           "nil replicas defaults to 1, fully available",
			replicas:       nil,
			dep:            depWithStatus(1, 1, 1),
			wantAvailable:  metav1.ConditionTrue,
			availReason:    ConditionReasonAvailable,
			wantProgress:   metav1.ConditionFalse,
			progressReason: ConditionReasonProgressingComplete,
			wantDegraded:   metav1.ConditionFalse,
			degradeReason:  ConditionReasonNotDegraded,
		},
		{
			name:           "partially available (1/3 ready, all updated)",
			replicas:       int32Ptr(3),
			dep:            depWithStatus(1, 3, 3),
			wantAvailable:  metav1.ConditionTrue,
			availReason:    ConditionReasonAvailable,
			wantProgress:   metav1.ConditionFalse,
			progressReason: ConditionReasonProgressingComplete,
			wantDegraded:   metav1.ConditionTrue,
			degradeReason:  ConditionReasonDegraded,
		},
		{
			name:           "no replicas ready",
			replicas:       int32Ptr(3),
			dep:            depWithStatus(0, 3, 3),
			wantAvailable:  metav1.ConditionFalse,
			availReason:    ConditionReasonUnavailable,
			wantProgress:   metav1.ConditionFalse,
			progressReason: ConditionReasonProgressingComplete,
			wantDegraded:   metav1.ConditionTrue,
			degradeReason:  ConditionReasonDegraded,
		},
		{
			name:           "rolling update (1/3 updated)",
			replicas:       int32Ptr(3),
			dep:            depWithStatus(3, 1, 3),
			wantAvailable:  metav1.ConditionTrue,
			availReason:    ConditionReasonAvailable,
			wantProgress:   metav1.ConditionTrue,
			progressReason: ConditionReasonProgressing,
			wantDegraded:   metav1.ConditionFalse,
			degradeReason:  ConditionReasonNotDegraded,
		},
		{
			name:           "scaling up (3 ready, desired 5, total 3)",
			replicas:       int32Ptr(5),
			dep:            depWithStatus(3, 3, 3),
			wantAvailable:  metav1.ConditionTrue,
			availReason:    ConditionReasonAvailable,
			wantProgress:   metav1.ConditionTrue,
			progressReason: ConditionReasonProgressing,
			wantDegraded:   metav1.ConditionTrue,
			degradeReason:  ConditionReasonDegraded,
		},
		{
			name:           "nil deployment",
			replicas:       int32Ptr(3),
			dep:            nil,
			wantAvailable:  metav1.ConditionFalse,
			availReason:    ConditionReasonUnavailable,
			wantProgress:   metav1.ConditionTrue,
			progressReason: ConditionReasonProgressing,
			wantDegraded:   metav1.ConditionTrue,
			degradeReason:  ConditionReasonDegraded,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mc := &memcachedv1alpha1.Memcached{
				Spec: memcachedv1alpha1.MemcachedSpec{
					Replicas: tt.replicas,
				},
			}

			conditions := computeConditions(mc, tt.dep, nil)

			assertCondition(t, conditions, ConditionTypeAvailable, tt.wantAvailable, tt.availReason)
			assertCondition(t, conditions, ConditionTypeProgressing, tt.wantProgress, tt.progressReason)
			assertCondition(t, conditions, ConditionTypeDegraded, tt.wantDegraded, tt.degradeReason)
		})
	}
}

func TestComputeConditions_ReturnsThreeConditions(t *testing.T) {
	mc := &memcachedv1alpha1.Memcached{
		Spec: memcachedv1alpha1.MemcachedSpec{
			Replicas: int32Ptr(1),
		},
	}

	conditions := computeConditions(mc, depWithStatus(1, 1, 1), nil)

	if len(conditions) != 3 {
		t.Fatalf("expected 3 conditions, got %d", len(conditions))
	}

	types := map[string]bool{}
	for _, c := range conditions {
		types[c.Type] = true
	}
	for _, ct := range []string{ConditionTypeAvailable, ConditionTypeProgressing, ConditionTypeDegraded} {
		if !types[ct] {
			t.Errorf("missing condition type %q", ct)
		}
	}
}

func TestComputeConditions_MessagesAreNonEmpty(t *testing.T) {
	mc := &memcachedv1alpha1.Memcached{
		Spec: memcachedv1alpha1.MemcachedSpec{
			Replicas: int32Ptr(3),
		},
	}

	conditions := computeConditions(mc, depWithStatus(1, 2, 3), nil)

	for _, c := range conditions {
		if c.Message == "" {
			t.Errorf("condition %q has empty message", c.Type)
		}
	}
}

func TestComputeConditions_ObservedGeneration(t *testing.T) {
	mc := &memcachedv1alpha1.Memcached{
		ObjectMeta: metav1.ObjectMeta{
			Generation: 5,
		},
		Spec: memcachedv1alpha1.MemcachedSpec{
			Replicas: int32Ptr(3),
		},
	}

	conditions := computeConditions(mc, depWithStatus(2, 3, 3), nil)

	for _, c := range conditions {
		if c.ObservedGeneration != 5 {
			t.Errorf("condition %q: observedGeneration = %d, want 5", c.Type, c.ObservedGeneration)
		}
	}
}

func TestComputeConditions_ObservedGeneration_NilDeployment(t *testing.T) {
	mc := &memcachedv1alpha1.Memcached{
		ObjectMeta: metav1.ObjectMeta{
			Generation: 3,
		},
		Spec: memcachedv1alpha1.MemcachedSpec{
			Replicas: int32Ptr(1),
		},
	}

	conditions := computeConditions(mc, nil, nil)

	for _, c := range conditions {
		if c.ObservedGeneration != 3 {
			t.Errorf("condition %q: observedGeneration = %d, want 3", c.Type, c.ObservedGeneration)
		}
	}
}

// depWithStatus creates a Deployment with the given replica counts for test setup.
func depWithStatus(ready, updated, total int32) *appsv1.Deployment {
	return &appsv1.Deployment{
		Status: appsv1.DeploymentStatus{
			ReadyReplicas:   ready,
			UpdatedReplicas: updated,
			Replicas:        total,
		},
	}
}

func TestComputeConditions_SecretNotFound_SingleMissing(t *testing.T) {
	mc := &memcachedv1alpha1.Memcached{
		Spec: memcachedv1alpha1.MemcachedSpec{
			Replicas: int32Ptr(3),
		},
	}

	conditions := computeConditions(mc, depWithStatus(3, 3, 3), []string{"sasl-secret"})

	assertCondition(t, conditions, ConditionTypeDegraded, metav1.ConditionTrue, ConditionReasonSecretNotFound)
	assertConditionMessageContains(t, conditions, ConditionTypeDegraded, "sasl-secret")
}

func TestComputeConditions_SecretNotFound_MultipleMissing(t *testing.T) {
	mc := &memcachedv1alpha1.Memcached{
		Spec: memcachedv1alpha1.MemcachedSpec{
			Replicas: int32Ptr(3),
		},
	}

	conditions := computeConditions(mc, depWithStatus(3, 3, 3), []string{"sasl-secret", "tls-secret"})

	assertCondition(t, conditions, ConditionTypeDegraded, metav1.ConditionTrue, ConditionReasonSecretNotFound)
	assertConditionMessageContains(t, conditions, ConditionTypeDegraded, "sasl-secret")
	assertConditionMessageContains(t, conditions, ConditionTypeDegraded, "tls-secret")
}

func TestComputeConditions_SecretNotFound_PrecedenceOverReplica(t *testing.T) {
	mc := &memcachedv1alpha1.Memcached{
		Spec: memcachedv1alpha1.MemcachedSpec{
			Replicas: int32Ptr(3),
		},
	}

	// All replicas ready, but missing secrets should still trigger Degraded=True with SecretNotFound.
	conditions := computeConditions(mc, depWithStatus(3, 3, 3), []string{"my-secret"})

	assertCondition(t, conditions, ConditionTypeDegraded, metav1.ConditionTrue, ConditionReasonSecretNotFound)
}

func TestComputeConditions_NoMissingSecrets_NilSlice(t *testing.T) {
	mc := &memcachedv1alpha1.Memcached{
		Spec: memcachedv1alpha1.MemcachedSpec{
			Replicas: int32Ptr(3),
		},
	}

	conditions := computeConditions(mc, depWithStatus(3, 3, 3), nil)

	assertCondition(t, conditions, ConditionTypeDegraded, metav1.ConditionFalse, ConditionReasonNotDegraded)
}

func TestComputeConditions_NoMissingSecrets_EmptySlice(t *testing.T) {
	mc := &memcachedv1alpha1.Memcached{
		Spec: memcachedv1alpha1.MemcachedSpec{
			Replicas: int32Ptr(3),
		},
	}

	conditions := computeConditions(mc, depWithStatus(3, 3, 3), []string{})

	assertCondition(t, conditions, ConditionTypeDegraded, metav1.ConditionFalse, ConditionReasonNotDegraded)
}

func TestConditionReasonSecretNotFound_Constant(t *testing.T) {
	if ConditionReasonSecretNotFound != "SecretNotFound" {
		t.Errorf("ConditionReasonSecretNotFound = %q, want %q", ConditionReasonSecretNotFound, "SecretNotFound")
	}
}

// assertConditionMessageContains checks that a condition's message contains the given substring.
func assertConditionMessageContains(t *testing.T, conditions []metav1.Condition, condType, substr string) {
	t.Helper()
	for _, c := range conditions {
		if c.Type == condType {
			if !strings.Contains(c.Message, substr) {
				t.Errorf("condition %q: message %q does not contain %q", condType, c.Message, substr)
			}
			return
		}
	}
	t.Errorf("condition %q not found", condType)
}

// assertCondition checks that a condition with the given type, status, and reason exists.
func assertCondition(t *testing.T, conditions []metav1.Condition, condType string, status metav1.ConditionStatus, reason string) {
	t.Helper()
	for _, c := range conditions {
		if c.Type == condType {
			if c.Status != status {
				t.Errorf("condition %q: status = %q, want %q", condType, c.Status, status)
			}
			if c.Reason != reason {
				t.Errorf("condition %q: reason = %q, want %q", condType, c.Reason, reason)
			}
			return
		}
	}
	t.Errorf("condition %q not found", condType)
}
