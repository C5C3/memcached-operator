// Package controller implements the reconciliation logic for the memcached-operator.
package controller

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	memcachedv1beta1 "github.com/c5c3/memcached-operator/api/v1beta1"
)

func TestComputeConditions(t *testing.T) {
	tests := []struct {
		name           string
		replicas       *int32
		sts            *appsv1.StatefulSet
		wantAvailable  metav1.ConditionStatus
		availReason    string
		wantProgress   metav1.ConditionStatus
		progressReason string
		wantDegraded   metav1.ConditionStatus
		degradeReason  string
		wantReady      metav1.ConditionStatus
		readyReason    string
	}{
		{
			name:           "fully available",
			replicas:       int32Ptr(3),
			sts:            stsWithStatus(3, 3, 3),
			wantAvailable:  metav1.ConditionTrue,
			availReason:    ConditionReasonAvailable,
			wantProgress:   metav1.ConditionFalse,
			progressReason: ConditionReasonProgressingComplete,
			wantDegraded:   metav1.ConditionFalse,
			degradeReason:  ConditionReasonNotDegraded,
			wantReady:      metav1.ConditionTrue,
			readyReason:    ConditionReasonReady,
		},
		{
			name:           "zero replicas desired",
			replicas:       int32Ptr(0),
			sts:            stsWithStatus(0, 0, 0),
			wantAvailable:  metav1.ConditionFalse,
			availReason:    ConditionReasonUnavailable,
			wantProgress:   metav1.ConditionFalse,
			progressReason: ConditionReasonProgressingComplete,
			wantDegraded:   metav1.ConditionFalse,
			degradeReason:  ConditionReasonNotDegraded,
			wantReady:      metav1.ConditionFalse,
			readyReason:    ConditionReasonNotReady,
		},
		{
			name:           "nil replicas defaults to 1, fully available",
			replicas:       nil,
			sts:            stsWithStatus(1, 1, 1),
			wantAvailable:  metav1.ConditionTrue,
			availReason:    ConditionReasonAvailable,
			wantProgress:   metav1.ConditionFalse,
			progressReason: ConditionReasonProgressingComplete,
			wantDegraded:   metav1.ConditionFalse,
			degradeReason:  ConditionReasonNotDegraded,
			wantReady:      metav1.ConditionTrue,
			readyReason:    ConditionReasonReady,
		},
		{
			name:           "partially available (1/3 ready, all updated)",
			replicas:       int32Ptr(3),
			sts:            stsWithStatus(1, 3, 3),
			wantAvailable:  metav1.ConditionTrue,
			availReason:    ConditionReasonAvailable,
			wantProgress:   metav1.ConditionFalse,
			progressReason: ConditionReasonProgressingComplete,
			wantDegraded:   metav1.ConditionTrue,
			degradeReason:  ConditionReasonDegraded,
			wantReady:      metav1.ConditionFalse,
			readyReason:    ConditionReasonNotReady,
		},
		{
			name:           "no replicas ready",
			replicas:       int32Ptr(3),
			sts:            stsWithStatus(0, 3, 3),
			wantAvailable:  metav1.ConditionFalse,
			availReason:    ConditionReasonUnavailable,
			wantProgress:   metav1.ConditionFalse,
			progressReason: ConditionReasonProgressingComplete,
			wantDegraded:   metav1.ConditionTrue,
			degradeReason:  ConditionReasonDegraded,
			wantReady:      metav1.ConditionFalse,
			readyReason:    ConditionReasonNotReady,
		},
		{
			name:           "rolling update (1/3 updated)",
			replicas:       int32Ptr(3),
			sts:            stsWithStatus(3, 1, 3),
			wantAvailable:  metav1.ConditionTrue,
			availReason:    ConditionReasonAvailable,
			wantProgress:   metav1.ConditionTrue,
			progressReason: ConditionReasonProgressing,
			wantDegraded:   metav1.ConditionFalse,
			degradeReason:  ConditionReasonNotDegraded,
			wantReady:      metav1.ConditionTrue,
			readyReason:    ConditionReasonReady,
		},
		{
			name:           "scaling up (3 ready, desired 5, total 3)",
			replicas:       int32Ptr(5),
			sts:            stsWithStatus(3, 3, 3),
			wantAvailable:  metav1.ConditionTrue,
			availReason:    ConditionReasonAvailable,
			wantProgress:   metav1.ConditionTrue,
			progressReason: ConditionReasonProgressing,
			wantDegraded:   metav1.ConditionTrue,
			degradeReason:  ConditionReasonDegraded,
			wantReady:      metav1.ConditionFalse,
			readyReason:    ConditionReasonNotReady,
		},
		{
			name:           "nil StatefulSet",
			replicas:       int32Ptr(3),
			sts:            nil,
			wantAvailable:  metav1.ConditionFalse,
			availReason:    ConditionReasonUnavailable,
			wantProgress:   metav1.ConditionTrue,
			progressReason: ConditionReasonProgressing,
			wantDegraded:   metav1.ConditionTrue,
			degradeReason:  ConditionReasonDegraded,
			wantReady:      metav1.ConditionFalse,
			readyReason:    ConditionReasonNotReady,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mc := &memcachedv1beta1.Memcached{
				Spec: memcachedv1beta1.MemcachedSpec{
					Replicas: tt.replicas,
				},
			}

			conditions := computeConditions(mc, tt.sts, nil, false)

			assertCondition(t, conditions, ConditionTypeAvailable, tt.wantAvailable, tt.availReason)
			assertCondition(t, conditions, ConditionTypeProgressing, tt.wantProgress, tt.progressReason)
			assertCondition(t, conditions, ConditionTypeDegraded, tt.wantDegraded, tt.degradeReason)
			assertCondition(t, conditions, ConditionTypeReady, tt.wantReady, tt.readyReason)
		})
	}
}

func TestComputeConditions_ReturnsFourConditions(t *testing.T) {
	mc := &memcachedv1beta1.Memcached{
		Spec: memcachedv1beta1.MemcachedSpec{
			Replicas: int32Ptr(1),
		},
	}

	conditions := computeConditions(mc, stsWithStatus(1, 1, 1), nil, false)

	if len(conditions) != 4 {
		t.Fatalf("expected 4 conditions, got %d", len(conditions))
	}

	types := map[string]bool{}
	for _, c := range conditions {
		types[c.Type] = true
	}
	for _, ct := range []string{ConditionTypeAvailable, ConditionTypeProgressing, ConditionTypeDegraded, ConditionTypeReady} {
		if !types[ct] {
			t.Errorf("missing condition type %q", ct)
		}
	}
}

func TestComputeConditions_MessagesAreNonEmpty(t *testing.T) {
	mc := &memcachedv1beta1.Memcached{
		Spec: memcachedv1beta1.MemcachedSpec{
			Replicas: int32Ptr(3),
		},
	}

	conditions := computeConditions(mc, stsWithStatus(1, 2, 3), nil, false)

	for _, c := range conditions {
		if c.Message == "" {
			t.Errorf("condition %q has empty message", c.Type)
		}
	}
}

func TestComputeConditions_ObservedGeneration(t *testing.T) {
	mc := &memcachedv1beta1.Memcached{
		ObjectMeta: metav1.ObjectMeta{
			Generation: 5,
		},
		Spec: memcachedv1beta1.MemcachedSpec{
			Replicas: int32Ptr(3),
		},
	}

	conditions := computeConditions(mc, stsWithStatus(2, 3, 3), nil, false)

	for _, c := range conditions {
		if c.ObservedGeneration != 5 {
			t.Errorf("condition %q: observedGeneration = %d, want 5", c.Type, c.ObservedGeneration)
		}
	}
}

func TestComputeConditions_ObservedGeneration_NilStatefulSet(t *testing.T) {
	mc := &memcachedv1beta1.Memcached{
		ObjectMeta: metav1.ObjectMeta{
			Generation: 3,
		},
		Spec: memcachedv1beta1.MemcachedSpec{
			Replicas: int32Ptr(1),
		},
	}

	conditions := computeConditions(mc, nil, nil, false)

	for _, c := range conditions {
		if c.ObservedGeneration != 3 {
			t.Errorf("condition %q: observedGeneration = %d, want 3", c.Type, c.ObservedGeneration)
		}
	}
}

// stsWithStatus creates a StatefulSet with the given replica counts for test setup.
func stsWithStatus(ready, updated, total int32) *appsv1.StatefulSet {
	return &appsv1.StatefulSet{
		Status: appsv1.StatefulSetStatus{
			ReadyReplicas:   ready,
			UpdatedReplicas: updated,
			Replicas:        total,
		},
	}
}

func TestComputeConditions_SecretNotFound_SingleMissing(t *testing.T) {
	mc := &memcachedv1beta1.Memcached{
		Spec: memcachedv1beta1.MemcachedSpec{
			Replicas: int32Ptr(3),
		},
	}

	conditions := computeConditions(mc, stsWithStatus(3, 3, 3), []string{"sasl-secret"}, false)

	assertCondition(t, conditions, ConditionTypeDegraded, metav1.ConditionTrue, ConditionReasonSecretNotFound)
	assertCondition(t, conditions, ConditionTypeReady, metav1.ConditionTrue, ConditionReasonReady)
	assertConditionMessageContains(t, conditions, ConditionTypeDegraded, "sasl-secret")
}

func TestComputeConditions_SecretNotFound_MultipleMissing(t *testing.T) {
	mc := &memcachedv1beta1.Memcached{
		Spec: memcachedv1beta1.MemcachedSpec{
			Replicas: int32Ptr(3),
		},
	}

	conditions := computeConditions(mc, stsWithStatus(3, 3, 3), []string{"sasl-secret", "tls-secret"}, false)

	assertCondition(t, conditions, ConditionTypeDegraded, metav1.ConditionTrue, ConditionReasonSecretNotFound)
	assertCondition(t, conditions, ConditionTypeReady, metav1.ConditionTrue, ConditionReasonReady)
	assertConditionMessageContains(t, conditions, ConditionTypeDegraded, "sasl-secret")
	assertConditionMessageContains(t, conditions, ConditionTypeDegraded, "tls-secret")
}

func TestComputeConditions_SecretNotFound_PrecedenceOverReplica(t *testing.T) {
	mc := &memcachedv1beta1.Memcached{
		Spec: memcachedv1beta1.MemcachedSpec{
			Replicas: int32Ptr(3),
		},
	}

	// All replicas ready, but missing secrets should still trigger Degraded=True with SecretNotFound.
	conditions := computeConditions(mc, stsWithStatus(3, 3, 3), []string{"my-secret"}, false)

	assertCondition(t, conditions, ConditionTypeDegraded, metav1.ConditionTrue, ConditionReasonSecretNotFound)
	assertCondition(t, conditions, ConditionTypeReady, metav1.ConditionTrue, ConditionReasonReady)
}

func TestComputeConditions_NoMissingSecrets_NilSlice(t *testing.T) {
	mc := &memcachedv1beta1.Memcached{
		Spec: memcachedv1beta1.MemcachedSpec{
			Replicas: int32Ptr(3),
		},
	}

	conditions := computeConditions(mc, stsWithStatus(3, 3, 3), nil, false)

	assertCondition(t, conditions, ConditionTypeDegraded, metav1.ConditionFalse, ConditionReasonNotDegraded)
	assertCondition(t, conditions, ConditionTypeReady, metav1.ConditionTrue, ConditionReasonReady)
}

func TestComputeConditions_NoMissingSecrets_EmptySlice(t *testing.T) {
	mc := &memcachedv1beta1.Memcached{
		Spec: memcachedv1beta1.MemcachedSpec{
			Replicas: int32Ptr(3),
		},
	}

	conditions := computeConditions(mc, stsWithStatus(3, 3, 3), []string{}, false)

	assertCondition(t, conditions, ConditionTypeDegraded, metav1.ConditionFalse, ConditionReasonNotDegraded)
	assertCondition(t, conditions, ConditionTypeReady, metav1.ConditionTrue, ConditionReasonReady)
}

func TestComputeConditions_HPAActive_UsesStatefulSetReplicas(t *testing.T) {
	mc := &memcachedv1beta1.Memcached{
		Spec: memcachedv1beta1.MemcachedSpec{
			Replicas: int32Ptr(2), // CR says 2, but HPA scaled to 5
		},
	}

	// StatefulSet has 5 total replicas (HPA-managed), all ready and updated.
	conditions := computeConditions(mc, stsWithStatus(5, 5, 5), nil, true)

	// When HPA is active, desired=5 (from sts.Status.Replicas), not 2 (from spec).
	assertCondition(t, conditions, ConditionTypeAvailable, metav1.ConditionTrue, ConditionReasonAvailable)
	assertCondition(t, conditions, ConditionTypeProgressing, metav1.ConditionFalse, ConditionReasonProgressingComplete)
	assertCondition(t, conditions, ConditionTypeDegraded, metav1.ConditionFalse, ConditionReasonNotDegraded)
	assertCondition(t, conditions, ConditionTypeReady, metav1.ConditionTrue, ConditionReasonReady)
	assertConditionMessageContains(t, conditions, ConditionTypeAvailable, "HPA-managed")
}

func TestComputeConditions_HPAActive_PartiallyReady(t *testing.T) {
	mc := &memcachedv1beta1.Memcached{
		Spec: memcachedv1beta1.MemcachedSpec{
			Replicas: int32Ptr(2),
		},
	}

	// HPA scaled to 5, only 3 ready.
	conditions := computeConditions(mc, stsWithStatus(3, 5, 5), nil, true)

	assertCondition(t, conditions, ConditionTypeAvailable, metav1.ConditionTrue, ConditionReasonAvailable)
	assertCondition(t, conditions, ConditionTypeProgressing, metav1.ConditionFalse, ConditionReasonProgressingComplete)
	assertCondition(t, conditions, ConditionTypeDegraded, metav1.ConditionTrue, ConditionReasonDegraded)
	assertCondition(t, conditions, ConditionTypeReady, metav1.ConditionFalse, ConditionReasonNotReady)
	assertConditionMessageContains(t, conditions, ConditionTypeAvailable, "HPA-managed")
}

func TestComputeConditions_HPAActive_NilStatefulSet(t *testing.T) {
	mc := &memcachedv1beta1.Memcached{
		Spec: memcachedv1beta1.MemcachedSpec{
			Replicas: int32Ptr(2),
		},
	}

	// HPA active but StatefulSet not yet created: falls back to spec replicas.
	conditions := computeConditions(mc, nil, nil, true)

	assertCondition(t, conditions, ConditionTypeAvailable, metav1.ConditionFalse, ConditionReasonUnavailable)
	assertCondition(t, conditions, ConditionTypeProgressing, metav1.ConditionTrue, ConditionReasonProgressing)
	assertCondition(t, conditions, ConditionTypeDegraded, metav1.ConditionTrue, ConditionReasonDegraded)
	assertCondition(t, conditions, ConditionTypeReady, metav1.ConditionFalse, ConditionReasonNotReady)
}

func TestComputeConditions_HPAInactive_IgnoresStatefulSetTotal(t *testing.T) {
	mc := &memcachedv1beta1.Memcached{
		Spec: memcachedv1beta1.MemcachedSpec{
			Replicas: int32Ptr(3),
		},
	}

	// HPA inactive, spec says 3, sts has 3 ready.
	conditions := computeConditions(mc, stsWithStatus(3, 3, 3), nil, false)

	assertCondition(t, conditions, ConditionTypeAvailable, metav1.ConditionTrue, ConditionReasonAvailable)
	assertCondition(t, conditions, ConditionTypeProgressing, metav1.ConditionFalse, ConditionReasonProgressingComplete)
	assertCondition(t, conditions, ConditionTypeDegraded, metav1.ConditionFalse, ConditionReasonNotDegraded)
	assertCondition(t, conditions, ConditionTypeReady, metav1.ConditionTrue, ConditionReasonReady)
	// Should NOT have "HPA-managed" in message when HPA is inactive.
	for _, c := range conditions {
		if c.Type == ConditionTypeAvailable && strings.Contains(c.Message, "HPA-managed") {
			t.Error("Available message should not mention HPA-managed when HPA is inactive")
		}
	}
}

func TestBuildServerList(t *testing.T) {
	tests := []struct {
		name         string
		workloadName string
		serviceName  string
		namespace    string
		replicas     int32
		want         []string
	}{
		{
			name:         "three replicas in index order",
			workloadName: "c",
			serviceName:  "c",
			namespace:    "ns",
			replicas:     3,
			want:         []string{"c-0.c.ns.svc:11211", "c-1.c.ns.svc:11211", "c-2.c.ns.svc:11211"},
		},
		{
			name:         "one replica",
			workloadName: "cache",
			serviceName:  "cache",
			namespace:    "default",
			replicas:     1,
			want:         []string{"cache-0.cache.default.svc:11211"},
		},
		{
			name:         "workload name differs from service name",
			workloadName: "short",
			serviceName:  "service",
			namespace:    "ns",
			replicas:     2,
			want:         []string{"short-0.service.ns.svc:11211", "short-1.service.ns.svc:11211"},
		},
		{
			name:         "zero replicas returns nil",
			workloadName: "c",
			serviceName:  "c",
			namespace:    "ns",
			replicas:     0,
			want:         nil,
		},
		{
			name:         "negative replicas returns nil",
			workloadName: "c",
			serviceName:  "c",
			namespace:    "ns",
			replicas:     -1,
			want:         nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := buildServerList(tt.workloadName, tt.serviceName, tt.namespace, tt.replicas)
			if tt.want == nil {
				if got != nil {
					t.Errorf("buildServerList = %v, want nil", got)
				}
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("buildServerList = %v, want %v", got, tt.want)
			}
		})
	}
}

// newStatusTestClient returns a fake client that serves the Memcached status subresource.
func newStatusTestClient(objs ...client.Object) client.WithWatch {
	return fake.NewClientBuilder().
		WithScheme(testScheme()).
		WithStatusSubresource(&memcachedv1beta1.Memcached{}).
		WithObjects(objs...).
		Build()
}

// statusTestStatefulSet returns a StatefulSet named name in namespace "ns" with the given
// spec.replicas and status.readyReplicas.
func statusTestStatefulSet(name string, replicas *int32, ready int32) *appsv1.StatefulSet {
	return &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "ns"},
		Spec:       appsv1.StatefulSetSpec{Replicas: replicas},
		Status:     appsv1.StatefulSetStatus{ReadyReplicas: ready},
	}
}

// runReconcileStatus calls reconcileStatus for the Memcached stored in c and returns the
// Memcached as stored after the status update.
func runReconcileStatus(t *testing.T, c client.Client, key client.ObjectKey) *memcachedv1beta1.Memcached {
	t.Helper()
	mc := &memcachedv1beta1.Memcached{}
	if err := c.Get(context.Background(), key, mc); err != nil {
		t.Fatalf("failed to get Memcached: %v", err)
	}
	if err := newTestReconciler(c).reconcileStatus(context.Background(), mc, nil); err != nil {
		t.Fatalf("reconcileStatus: unexpected error: %v", err)
	}
	stored := &memcachedv1beta1.Memcached{}
	if err := c.Get(context.Background(), key, stored); err != nil {
		t.Fatalf("failed to get Memcached after status update: %v", err)
	}
	return stored
}

func TestReconcileStatus_ServerListIgnoresReadiness(t *testing.T) {
	mc := &memcachedv1beta1.Memcached{
		ObjectMeta: metav1.ObjectMeta{Name: "cache", Namespace: "ns", UID: "uid-status"},
		Spec:       memcachedv1beta1.MemcachedSpec{Replicas: int32Ptr(3)},
	}
	c := newStatusTestClient(mc, statusTestStatefulSet("cache", int32Ptr(3), 0))

	got := runReconcileStatus(t, c, client.ObjectKeyFromObject(mc))

	want := []string{
		"cache-0.cache.ns.svc:11211",
		"cache-1.cache.ns.svc:11211",
		"cache-2.cache.ns.svc:11211",
	}
	if !reflect.DeepEqual(got.Status.ServerList, want) {
		t.Errorf("serverList = %v, want %v", got.Status.ServerList, want)
	}
	ready := meta.FindStatusCondition(got.Status.Conditions, ConditionTypeReady)
	if ready == nil {
		t.Fatal("Ready condition not found")
	}
	if ready.Status != metav1.ConditionFalse {
		t.Errorf("Ready = %q, want %q", ready.Status, metav1.ConditionFalse)
	}
	if got.Status.ReadyReplicas != 0 {
		t.Errorf("readyReplicas = %d, want 0", got.Status.ReadyReplicas)
	}
}

func TestReconcileStatus_ZeroReplicasClearsServerList(t *testing.T) {
	mc := &memcachedv1beta1.Memcached{
		ObjectMeta: metav1.ObjectMeta{Name: "cache", Namespace: "ns", UID: "uid-status"},
		Spec:       memcachedv1beta1.MemcachedSpec{Replicas: int32Ptr(0)},
		Status: memcachedv1beta1.MemcachedStatus{
			ServerList: []string{"cache-0.cache.ns.svc:11211"},
		},
	}
	c := newStatusTestClient(mc, statusTestStatefulSet("cache", int32Ptr(0), 0))

	got := runReconcileStatus(t, c, client.ObjectKeyFromObject(mc))

	if got.Status.ServerList != nil {
		t.Errorf("serverList = %v, want nil", got.Status.ServerList)
	}
	ready := meta.FindStatusCondition(got.Status.Conditions, ConditionTypeReady)
	if ready == nil {
		t.Fatal("Ready condition not found")
	}
	if ready.Status != metav1.ConditionFalse {
		t.Errorf("Ready = %q, want %q", ready.Status, metav1.ConditionFalse)
	}
	if ready.Message != "Instance has zero desired replicas" {
		t.Errorf("Ready message = %q, want %q", ready.Message, "Instance has zero desired replicas")
	}
}

func TestReconcileStatus_NilReplicasYieldsOneEntry(t *testing.T) {
	// The Memcached asks for 3 replicas, so the single entry can only come from the
	// StatefulSet's nil spec.replicas counting as 1.
	mc := &memcachedv1beta1.Memcached{
		ObjectMeta: metav1.ObjectMeta{Name: "cache", Namespace: "ns", UID: "uid-status"},
		Spec:       memcachedv1beta1.MemcachedSpec{Replicas: int32Ptr(3)},
	}
	// The fake client does not default spec.replicas, so it stays nil.
	c := newStatusTestClient(mc, statusTestStatefulSet("cache", nil, 0))

	got := runReconcileStatus(t, c, client.ObjectKeyFromObject(mc))

	want := []string{"cache-0.cache.ns.svc:11211"}
	if !reflect.DeepEqual(got.Status.ServerList, want) {
		t.Errorf("serverList = %v, want %v", got.Status.ServerList, want)
	}
}

func TestReconcileStatus_LongNameUsesStatefulSetNameAsHost(t *testing.T) {
	name := strings.Repeat("s", 60)
	mc := &memcachedv1beta1.Memcached{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "ns", UID: "uid-status"},
		Spec:       memcachedv1beta1.MemcachedSpec{Replicas: int32Ptr(2)},
	}
	stsName := statefulSetName(name)
	c := newStatusTestClient(mc, statusTestStatefulSet(stsName, int32Ptr(2), 0))

	got := runReconcileStatus(t, c, client.ObjectKeyFromObject(mc))

	want := []string{
		stsName + "-0." + name + ".ns.svc:11211",
		stsName + "-1." + name + ".ns.svc:11211",
	}
	if !reflect.DeepEqual(got.Status.ServerList, want) {
		t.Errorf("serverList = %v, want %v", got.Status.ServerList, want)
	}
}

func TestReconcileStatus_NoStatefulSet(t *testing.T) {
	mc := &memcachedv1beta1.Memcached{
		ObjectMeta: metav1.ObjectMeta{Name: "cache", Namespace: "ns", UID: "uid-status"},
		Spec:       memcachedv1beta1.MemcachedSpec{Replicas: int32Ptr(3)},
		Status: memcachedv1beta1.MemcachedStatus{
			ServerList:    []string{"cache-0.cache.ns.svc:11211"},
			ReadyReplicas: 1,
		},
	}
	c := newStatusTestClient(mc)

	got := runReconcileStatus(t, c, client.ObjectKeyFromObject(mc))

	if got.Status.ServerList != nil {
		t.Errorf("serverList = %v, want nil", got.Status.ServerList)
	}
	if got.Status.ReadyReplicas != 0 {
		t.Errorf("readyReplicas = %d, want 0", got.Status.ReadyReplicas)
	}
	progressing := meta.FindStatusCondition(got.Status.Conditions, ConditionTypeProgressing)
	if progressing == nil {
		t.Fatal("Progressing condition not found")
	}
	if progressing.Message != "Waiting for StatefulSet to be created" {
		t.Errorf("Progressing message = %q, want %q", progressing.Message, "Waiting for StatefulSet to be created")
	}
}

func TestReconcileStatus_StatefulSetGetError(t *testing.T) {
	mc := &memcachedv1beta1.Memcached{
		ObjectMeta: metav1.ObjectMeta{Name: "cache", Namespace: "ns", UID: "uid-status"},
		Spec:       memcachedv1beta1.MemcachedSpec{Replicas: int32Ptr(3)},
	}
	injected := errors.New("injected get error")
	var statusUpdates int
	c := interceptor.NewClient(newStatusTestClient(mc, statusTestStatefulSet("cache", int32Ptr(3), 3)), interceptor.Funcs{
		Get: func(ctx context.Context, c client.WithWatch, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
			if _, ok := obj.(*appsv1.StatefulSet); ok {
				return injected
			}
			return c.Get(ctx, key, obj, opts...)
		},
		SubResourceUpdate: func(ctx context.Context, c client.Client, subResourceName string, obj client.Object, opts ...client.SubResourceUpdateOption) error {
			statusUpdates++
			return c.SubResource(subResourceName).Update(ctx, obj, opts...)
		},
	})

	stored := &memcachedv1beta1.Memcached{}
	if err := c.Get(context.Background(), client.ObjectKeyFromObject(mc), stored); err != nil {
		t.Fatalf("failed to get Memcached: %v", err)
	}
	err := newTestReconciler(c).reconcileStatus(context.Background(), stored, nil)
	if err == nil {
		t.Fatal("expected an error, got nil")
	}
	if !strings.HasPrefix(err.Error(), "fetching StatefulSet for status: ") {
		t.Errorf("error = %q, want prefix %q", err.Error(), "fetching StatefulSet for status: ")
	}
	if !errors.Is(err, injected) {
		t.Errorf("error %v does not wrap the injected error", err)
	}
	if statusUpdates != 0 {
		t.Errorf("status updates = %d, want 0", statusUpdates)
	}
}

func TestReconcileStatus_ServerListStableAcrossReconciles(t *testing.T) {
	mc := &memcachedv1beta1.Memcached{
		ObjectMeta: metav1.ObjectMeta{Name: "cache", Namespace: "ns", UID: "uid-status"},
		Spec:       memcachedv1beta1.MemcachedSpec{Replicas: int32Ptr(3)},
	}
	c := newStatusTestClient(mc, statusTestStatefulSet("cache", int32Ptr(3), 1))
	key := client.ObjectKeyFromObject(mc)

	first := runReconcileStatus(t, c, key)
	second := runReconcileStatus(t, c, key)

	if len(first.Status.ServerList) != 3 {
		t.Fatalf("serverList = %v, want 3 entries", first.Status.ServerList)
	}
	if !reflect.DeepEqual(first.Status.ServerList, second.Status.ServerList) {
		t.Errorf("serverList changed between reconciles: %v, then %v", first.Status.ServerList, second.Status.ServerList)
	}
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
