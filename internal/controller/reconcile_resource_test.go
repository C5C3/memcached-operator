// Package controller implements the reconciliation logic for the memcached-operator.
package controller

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/events"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	ctrlmetrics "sigs.k8s.io/controller-runtime/pkg/metrics"

	memcachedv1beta1 "github.com/c5c3/memcached-operator/api/v1beta1"
	// Import metrics package to ensure init() registration runs.
	_ "github.com/c5c3/memcached-operator/internal/metrics"
)

// testScheme returns a scheme with both core Kubernetes types and Memcached types registered.
func testScheme() *runtime.Scheme {
	s := runtime.NewScheme()
	_ = scheme.AddToScheme(s)
	_ = memcachedv1beta1.AddToScheme(s)
	return s
}

func newTestReconciler(c client.Client) *MemcachedReconciler {
	return &MemcachedReconciler{
		Client: c,
		Scheme: testScheme(),
	}
}

func newTestReconcilerWithRecorder(c client.Client, recorder events.EventRecorder) *MemcachedReconciler {
	return &MemcachedReconciler{
		Client:   c,
		Scheme:   testScheme(),
		Recorder: recorder,
	}
}

func newFakeClient(objs ...client.Object) client.WithWatch {
	return fake.NewClientBuilder().WithScheme(testScheme()).WithObjects(objs...).Build()
}

func TestReconcileResource_CreatesNewResource(t *testing.T) {
	mc := &memcachedv1beta1.Memcached{
		ObjectMeta: metav1.ObjectMeta{Name: testInstanceNameTest, Namespace: testDefaultNamespace, UID: testOwnerUID},
		Spec:       memcachedv1beta1.MemcachedSpec{},
	}
	c := newFakeClient(mc)
	r := newTestReconciler(c)

	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: testInstanceNameTest, Namespace: testDefaultNamespace},
	}

	result, err := r.reconcileResource(context.Background(), mc, svc, func() error {
		constructService(mc, svc)
		return nil
	}, "Service")

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != controllerutil.OperationResultCreated {
		t.Errorf("expected OperationResultCreated, got %v", result)
	}

	// Verify the resource was actually created.
	got := &corev1.Service{}
	if err := c.Get(context.Background(), client.ObjectKeyFromObject(svc), got); err != nil {
		t.Fatalf("failed to get created service: %v", err)
	}
}

func TestReconcileResource_UpdatesExistingResource(t *testing.T) {
	mc := &memcachedv1beta1.Memcached{
		ObjectMeta: metav1.ObjectMeta{Name: testInstanceNameTest, Namespace: testDefaultNamespace, UID: testOwnerUID},
		Spec:       memcachedv1beta1.MemcachedSpec{},
	}
	existingSvc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: testInstanceNameTest, Namespace: testDefaultNamespace},
		Spec: corev1.ServiceSpec{
			Ports: []corev1.ServicePort{
				{Name: testStalePortName, Port: 9999},
			},
		},
	}
	c := newFakeClient(mc, existingSvc)
	r := newTestReconciler(c)

	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: testInstanceNameTest, Namespace: testDefaultNamespace},
	}

	_, err := r.reconcileResource(context.Background(), mc, svc, func() error {
		constructService(mc, svc)
		return nil
	}, "Service")

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify the resource was updated.
	got := &corev1.Service{}
	if err := c.Get(context.Background(), client.ObjectKeyFromObject(svc), got); err != nil {
		t.Fatalf("failed to get updated service: %v", err)
	}
	if len(got.Spec.Ports) != 1 || got.Spec.Ports[0].Port != 11211 {
		t.Errorf("expected port 11211, got %v", got.Spec.Ports)
	}
}

func TestReconcileResource_RetriesOnConflict(t *testing.T) {
	mc := &memcachedv1beta1.Memcached{
		ObjectMeta: metav1.ObjectMeta{Name: testInstanceNameTest, Namespace: testDefaultNamespace, UID: testOwnerUID},
		Spec:       memcachedv1beta1.MemcachedSpec{},
	}
	existingSvc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: testInstanceNameTest, Namespace: testDefaultNamespace},
	}

	var updateCalls atomic.Int32
	conflictErr := apierrors.NewConflict(
		schema.GroupResource{Group: "", Resource: "services"},
		"test",
		fmt.Errorf("the object has been modified"),
	)

	baseClient := newFakeClient(mc, existingSvc)
	// Intercept Update to return a conflict error on the first call only.
	wrappedClient := interceptor.NewClient(baseClient, interceptor.Funcs{
		Update: func(ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.UpdateOption) error {
			if _, ok := obj.(*corev1.Service); ok {
				call := updateCalls.Add(1)
				if call == 1 {
					return conflictErr
				}
			}
			return c.Update(ctx, obj, opts...)
		},
	})

	r := newTestReconciler(wrappedClient)

	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: testInstanceNameTest, Namespace: testDefaultNamespace},
	}

	var mutateCalls int
	_, err := r.reconcileResource(context.Background(), mc, svc, func() error {
		mutateCalls++
		constructService(mc, svc)
		return nil
	}, "Service")

	if err != nil {
		t.Fatalf("expected retry to succeed, got error: %v", err)
	}
	if got := updateCalls.Load(); got < 2 {
		t.Errorf("expected at least 2 update calls (1 conflict + 1 success), got %d", got)
	}
	if mutateCalls < 2 {
		t.Errorf("expected mutate to be called at least twice on retry, got %d", mutateCalls)
	}
}

func TestReconcileResource_ExhaustsRetriesOnConflict(t *testing.T) {
	mc := &memcachedv1beta1.Memcached{
		ObjectMeta: metav1.ObjectMeta{Name: testInstanceNameTest, Namespace: testDefaultNamespace, UID: testOwnerUID},
		Spec:       memcachedv1beta1.MemcachedSpec{},
	}
	existingSvc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: testInstanceNameTest, Namespace: testDefaultNamespace},
	}

	conflictErr := apierrors.NewConflict(
		schema.GroupResource{Group: "", Resource: "services"},
		"test",
		fmt.Errorf("the object has been modified"),
	)

	baseClient := newFakeClient(mc, existingSvc)
	wrappedClient := interceptor.NewClient(baseClient, interceptor.Funcs{
		Update: func(ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.UpdateOption) error {
			if _, ok := obj.(*corev1.Service); ok {
				return conflictErr
			}
			return c.Update(ctx, obj, opts...)
		},
	})

	r := newTestReconciler(wrappedClient)

	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: testInstanceNameTest, Namespace: testDefaultNamespace},
	}

	_, err := r.reconcileResource(context.Background(), mc, svc, func() error {
		constructService(mc, svc)
		return nil
	}, "Service")

	if err == nil {
		t.Fatal("expected error after exhausting retries, got nil")
	}
	if !apierrors.IsConflict(err) {
		t.Errorf("expected conflict error, got: %v", err)
	}
}

func TestReconcileResource_DoesNotRetryNonConflictErrors(t *testing.T) {
	mc := &memcachedv1beta1.Memcached{
		ObjectMeta: metav1.ObjectMeta{Name: testInstanceNameTest, Namespace: testDefaultNamespace, UID: testOwnerUID},
		Spec:       memcachedv1beta1.MemcachedSpec{},
	}
	existingSvc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: testInstanceNameTest, Namespace: testDefaultNamespace},
	}

	internalErr := apierrors.NewInternalError(fmt.Errorf("server exploded"))

	var updateCalls atomic.Int32
	baseClient := newFakeClient(mc, existingSvc)
	wrappedClient := interceptor.NewClient(baseClient, interceptor.Funcs{
		Update: func(ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.UpdateOption) error {
			if _, ok := obj.(*corev1.Service); ok {
				updateCalls.Add(1)
				return internalErr
			}
			return c.Update(ctx, obj, opts...)
		},
	})

	r := newTestReconciler(wrappedClient)

	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: testInstanceNameTest, Namespace: testDefaultNamespace},
	}

	_, err := r.reconcileResource(context.Background(), mc, svc, func() error {
		constructService(mc, svc)
		return nil
	}, "Service")

	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if got := updateCalls.Load(); got != 1 {
		t.Errorf("expected exactly 1 update call (no retries for non-conflict), got %d", got)
	}
}

func TestReconcileResource_PropagatesMutateError(t *testing.T) {
	mc := &memcachedv1beta1.Memcached{
		ObjectMeta: metav1.ObjectMeta{Name: testInstanceNameTest, Namespace: testDefaultNamespace, UID: testOwnerUID},
		Spec:       memcachedv1beta1.MemcachedSpec{},
	}
	c := newFakeClient(mc)
	r := newTestReconciler(c)

	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: testInstanceNameTest, Namespace: testDefaultNamespace},
	}

	mutateErr := fmt.Errorf("failed to set owner reference")
	_, err := r.reconcileResource(context.Background(), mc, svc, func() error {
		return mutateErr
	}, "Service")

	if err == nil {
		t.Fatal("expected error from mutate, got nil")
	}
	if err.Error() != "reconciling Service: "+mutateErr.Error() {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestReconcileResource_SetsOwnerReference(t *testing.T) {
	mc := &memcachedv1beta1.Memcached{
		ObjectMeta: metav1.ObjectMeta{Name: testInstanceNameTest, Namespace: testDefaultNamespace, UID: testOwnerUID},
		Spec:       memcachedv1beta1.MemcachedSpec{},
	}
	c := newFakeClient(mc)
	r := newTestReconciler(c)

	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: testInstanceNameTest, Namespace: testDefaultNamespace},
	}

	_, err := r.reconcileResource(context.Background(), mc, svc, func() error {
		constructService(mc, svc)
		return nil
	}, "Service")

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify the owner reference was set.
	got := &corev1.Service{}
	if err := c.Get(context.Background(), client.ObjectKeyFromObject(svc), got); err != nil {
		t.Fatalf("failed to get service: %v", err)
	}
	if len(got.OwnerReferences) != 1 {
		t.Fatalf("expected 1 owner reference, got %d", len(got.OwnerReferences))
	}
	ref := got.OwnerReferences[0]
	if ref.Name != mc.Name {
		t.Errorf("expected owner reference name %q, got %q", mc.Name, ref.Name)
	}
	if ref.UID != mc.UID {
		t.Errorf("expected owner reference UID %q, got %q", mc.UID, ref.UID)
	}
	if ref.Kind != "Memcached" {
		t.Errorf("expected owner reference kind %q, got %q", "Memcached", ref.Kind)
	}
	if ref.Controller == nil || !*ref.Controller {
		t.Errorf("expected controller=true in owner reference")
	}
	if ref.BlockOwnerDeletion == nil || !*ref.BlockOwnerDeletion {
		t.Errorf("expected blockOwnerDeletion=true in owner reference")
	}
}

func TestReconcileResource_EmitsCreatedEvent(t *testing.T) {
	mc := &memcachedv1beta1.Memcached{
		ObjectMeta: metav1.ObjectMeta{Name: testInstanceNameTest, Namespace: testDefaultNamespace, UID: testOwnerUID},
		Spec:       memcachedv1beta1.MemcachedSpec{},
	}
	c := newFakeClient(mc)
	recorder := events.NewFakeRecorder(10)
	r := newTestReconcilerWithRecorder(c, recorder)

	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: testInstanceNameTest, Namespace: testDefaultNamespace},
	}

	_, err := r.reconcileResource(context.Background(), mc, svc, func() error {
		constructService(mc, svc)
		return nil
	}, "Service")

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	select {
	case event := <-recorder.Events:
		expected := "Normal Created Created Service test"
		if event != expected {
			t.Errorf("expected event %q, got %q", expected, event)
		}
	default:
		t.Error("expected a Created event, but none was emitted")
	}
}

func TestReconcileResource_EmitsUpdatedEvent(t *testing.T) {
	mc := &memcachedv1beta1.Memcached{
		ObjectMeta: metav1.ObjectMeta{Name: testInstanceNameTest, Namespace: testDefaultNamespace, UID: testOwnerUID},
		Spec:       memcachedv1beta1.MemcachedSpec{},
	}
	existingSvc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: testInstanceNameTest, Namespace: testDefaultNamespace},
		Spec: corev1.ServiceSpec{
			Ports: []corev1.ServicePort{
				{Name: testStalePortName, Port: 9999},
			},
		},
	}
	c := newFakeClient(mc, existingSvc)
	recorder := events.NewFakeRecorder(10)
	r := newTestReconcilerWithRecorder(c, recorder)

	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: testInstanceNameTest, Namespace: testDefaultNamespace},
	}

	_, err := r.reconcileResource(context.Background(), mc, svc, func() error {
		constructService(mc, svc)
		return nil
	}, "Service")

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	select {
	case event := <-recorder.Events:
		expected := "Normal Updated Updated Service test"
		if event != expected {
			t.Errorf("expected event %q, got %q", expected, event)
		}
	default:
		t.Error("expected an Updated event, but none was emitted")
	}
}

func TestReconcileResource_NoEventOnUnchanged(t *testing.T) {
	mc := &memcachedv1beta1.Memcached{
		ObjectMeta: metav1.ObjectMeta{Name: testInstanceNameTest, Namespace: testDefaultNamespace, UID: testOwnerUID},
		Spec:       memcachedv1beta1.MemcachedSpec{},
	}
	c := newFakeClient(mc)
	recorder := events.NewFakeRecorder(10)
	r := newTestReconcilerWithRecorder(c, recorder)

	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: testInstanceNameTest, Namespace: testDefaultNamespace},
	}

	// First call: creates the resource.
	_, err := r.reconcileResource(context.Background(), mc, svc, func() error {
		constructService(mc, svc)
		return nil
	}, "Service")
	if err != nil {
		t.Fatalf("unexpected error on create: %v", err)
	}
	// Drain the Created event.
	<-recorder.Events

	// Second call with same state: should be a no-op.
	_, err = r.reconcileResource(context.Background(), mc, svc, func() error {
		constructService(mc, svc)
		return nil
	}, "Service")
	if err != nil {
		t.Fatalf("unexpected error on no-op: %v", err)
	}

	select {
	case event := <-recorder.Events:
		t.Errorf("expected no event on unchanged resource, but got: %q", event)
	default:
		// No event emitted — correct behavior.
	}
}

func TestReconcileResource_NilRecorderDoesNotPanic(t *testing.T) {
	mc := &memcachedv1beta1.Memcached{
		ObjectMeta: metav1.ObjectMeta{Name: testInstanceNameTest, Namespace: testDefaultNamespace, UID: testOwnerUID},
		Spec:       memcachedv1beta1.MemcachedSpec{},
	}
	c := newFakeClient(mc)
	// Use newTestReconciler which does not set a Recorder (nil).
	r := newTestReconciler(c)

	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: testInstanceNameTest, Namespace: testDefaultNamespace},
	}

	_, err := r.reconcileResource(context.Background(), mc, svc, func() error {
		constructService(mc, svc)
		return nil
	}, "Service")

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Test passes if no panic occurred.
}

// getReconcileResourceCounter reads the current value of memcached_reconcile_resource_total
// for the "Service" resource_kind and result label from the controller-runtime metrics registry.
func getReconcileResourceCounter(t *testing.T, result string) float64 {
	const resourceKind = "Service"
	t.Helper()
	gatherer, ok := ctrlmetrics.Registry.(prometheus.Gatherer)
	if !ok {
		t.Fatal("controller-runtime registry does not implement prometheus.Gatherer")
	}
	families, err := gatherer.Gather()
	if err != nil {
		t.Fatalf("failed to gather metrics: %v", err)
	}
	for _, f := range families {
		if f.GetName() != "memcached_reconcile_resource_total" {
			continue
		}
		for _, m := range f.GetMetric() {
			labels := m.GetLabel()
			if matchLabels(labels, resourceKind, result) {
				return m.GetCounter().GetValue()
			}
		}
	}
	return 0
}

func matchLabels(labels []*dto.LabelPair, resourceKind, result string) bool {
	var kindMatch, resultMatch bool
	for _, l := range labels {
		if l.GetName() == "resource_kind" && l.GetValue() == resourceKind {
			kindMatch = true
		}
		if l.GetName() == "result" && l.GetValue() == result {
			resultMatch = true
		}
	}
	return kindMatch && resultMatch
}

func TestReconcileResource_IncrementsMetricOnCreate(t *testing.T) {
	mc := &memcachedv1beta1.Memcached{
		ObjectMeta: metav1.ObjectMeta{Name: "metric-create", Namespace: testDefaultNamespace, UID: "m-1"},
		Spec:       memcachedv1beta1.MemcachedSpec{},
	}
	c := newFakeClient(mc)
	r := newTestReconciler(c)

	before := getReconcileResourceCounter(t, "created")

	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: "metric-create", Namespace: testDefaultNamespace},
	}
	_, err := r.reconcileResource(context.Background(), mc, svc, func() error {
		constructService(mc, svc)
		return nil
	}, "Service")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	after := getReconcileResourceCounter(t, "created")
	if after != before+1 {
		t.Errorf("expected Service/created counter to increment by 1, got before=%v after=%v", before, after)
	}
}

func TestReconcileResource_IncrementsMetricOnUpdate(t *testing.T) {
	mc := &memcachedv1beta1.Memcached{
		ObjectMeta: metav1.ObjectMeta{Name: testInstanceNameMetricUpdate, Namespace: testDefaultNamespace, UID: "m-2"},
		Spec:       memcachedv1beta1.MemcachedSpec{},
	}
	existingSvc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: testInstanceNameMetricUpdate, Namespace: testDefaultNamespace},
		Spec: corev1.ServiceSpec{
			Ports: []corev1.ServicePort{{Name: testStalePortName, Port: 9999}},
		},
	}
	c := newFakeClient(mc, existingSvc)
	r := newTestReconciler(c)

	before := getReconcileResourceCounter(t, "updated")

	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: testInstanceNameMetricUpdate, Namespace: testDefaultNamespace},
	}
	_, err := r.reconcileResource(context.Background(), mc, svc, func() error {
		constructService(mc, svc)
		return nil
	}, "Service")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	after := getReconcileResourceCounter(t, "updated")
	if after != before+1 {
		t.Errorf("expected Service/updated counter to increment by 1, got before=%v after=%v", before, after)
	}
}

func TestReconcileResource_IncrementsMetricOnUnchanged(t *testing.T) {
	mc := &memcachedv1beta1.Memcached{
		ObjectMeta: metav1.ObjectMeta{Name: "metric-unchanged", Namespace: testDefaultNamespace, UID: "m-3"},
		Spec:       memcachedv1beta1.MemcachedSpec{},
	}
	c := newFakeClient(mc)
	r := newTestReconciler(c)

	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: "metric-unchanged", Namespace: testDefaultNamespace},
	}
	// First call: create.
	_, err := r.reconcileResource(context.Background(), mc, svc, func() error {
		constructService(mc, svc)
		return nil
	}, "Service")
	if err != nil {
		t.Fatalf("unexpected error on create: %v", err)
	}

	before := getReconcileResourceCounter(t, "unchanged")

	// Second call: unchanged.
	_, err = r.reconcileResource(context.Background(), mc, svc, func() error {
		constructService(mc, svc)
		return nil
	}, "Service")
	if err != nil {
		t.Fatalf("unexpected error on unchanged: %v", err)
	}

	after := getReconcileResourceCounter(t, "unchanged")
	if after != before+1 {
		t.Errorf("expected Service/unchanged counter to increment by 1, got before=%v after=%v", before, after)
	}
}

func TestReconcileResource_DoesNotIncrementMetricOnError(t *testing.T) {
	mc := &memcachedv1beta1.Memcached{
		ObjectMeta: metav1.ObjectMeta{Name: testInstanceNameMetricErr, Namespace: testDefaultNamespace, UID: "m-4"},
		Spec:       memcachedv1beta1.MemcachedSpec{},
	}
	existingSvc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: testInstanceNameMetricErr, Namespace: testDefaultNamespace},
	}

	internalErr := apierrors.NewInternalError(fmt.Errorf("server exploded"))
	baseClient := newFakeClient(mc, existingSvc)
	wrappedClient := interceptor.NewClient(baseClient, interceptor.Funcs{
		Update: func(ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.UpdateOption) error {
			if _, ok := obj.(*corev1.Service); ok {
				return internalErr
			}
			return c.Update(ctx, obj, opts...)
		},
	})
	r := newTestReconciler(wrappedClient)

	// Gather all Service counters before to compare delta.
	createdBefore := getReconcileResourceCounter(t, "created")
	updatedBefore := getReconcileResourceCounter(t, "updated")
	unchangedBefore := getReconcileResourceCounter(t, "unchanged")

	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: testInstanceNameMetricErr, Namespace: testDefaultNamespace},
	}
	_, _ = r.reconcileResource(context.Background(), mc, svc, func() error {
		constructService(mc, svc)
		return nil
	}, "Service")

	createdAfter := getReconcileResourceCounter(t, "created")
	updatedAfter := getReconcileResourceCounter(t, "updated")
	unchangedAfter := getReconcileResourceCounter(t, "unchanged")

	if createdAfter != createdBefore {
		t.Errorf("Service/created counter changed on error: before=%v after=%v", createdBefore, createdAfter)
	}
	if updatedAfter != updatedBefore {
		t.Errorf("Service/updated counter changed on error: before=%v after=%v", updatedBefore, updatedAfter)
	}
	if unchangedAfter != unchangedBefore {
		t.Errorf("Service/unchanged counter changed on error: before=%v after=%v", unchangedBefore, unchangedAfter)
	}
}

// ---------------------------------------------------------------------------
// AlreadyExists path: an object the cache cannot see
// ---------------------------------------------------------------------------

// cacheMissCounts counts the writes a cacheMissClient sends.
type cacheMissCounts struct {
	creates atomic.Int32
	updates atomic.Int32
}

// newCacheMissClient wraps base so that a Get of a Service returns NotFound, as the
// label-filtered cache does for a Service without the managed-by label. updateErr, when
// not nil, returns the error for the given Update call (1-based) or nil to let it through.
func newCacheMissClient(base client.WithWatch, counts *cacheMissCounts, updateErr func(call int32) error) client.WithWatch {
	return interceptor.NewClient(base, interceptor.Funcs{
		Get: func(ctx context.Context, c client.WithWatch, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
			if _, ok := obj.(*corev1.Service); ok {
				return apierrors.NewNotFound(corev1.Resource("services"), key.Name)
			}
			return c.Get(ctx, key, obj, opts...)
		},
		Create: func(ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.CreateOption) error {
			counts.creates.Add(1)
			return c.Create(ctx, obj, opts...)
		},
		Update: func(ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.UpdateOption) error {
			call := counts.updates.Add(1)
			if updateErr != nil {
				if err := updateErr(call); err != nil {
					return err
				}
			}
			return c.Update(ctx, obj, opts...)
		},
	})
}

// alreadyExistsMemcached returns the Memcached the AlreadyExists tests reconcile.
func alreadyExistsMemcached() *memcachedv1beta1.Memcached {
	return &memcachedv1beta1.Memcached{
		ObjectMeta: metav1.ObjectMeta{Name: testInstanceNameTest, Namespace: testDefaultNamespace, UID: testOwnerUID},
	}
}

// controlledServiceWithoutLabel returns a Service that mc controls and that lacks the
// managed-by label, with a stale port.
func controlledServiceWithoutLabel(t *testing.T, mc *memcachedv1beta1.Memcached) *corev1.Service {
	t.Helper()
	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: mc.Name, Namespace: mc.Namespace},
		Spec:       corev1.ServiceSpec{Ports: []corev1.ServicePort{{Name: testStalePortName, Port: 9999}}},
	}
	if err := controllerutil.SetControllerReference(mc, svc, testScheme()); err != nil {
		t.Fatalf("failed to set controller reference: %v", err)
	}
	return svc
}

// reconcileTestService runs reconcileResource for the headless Service of mc.
func reconcileTestService(r *MemcachedReconciler, mc *memcachedv1beta1.Memcached) (controllerutil.OperationResult, error) {
	svc := &corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: mc.Name, Namespace: mc.Namespace}}
	return r.reconcileResource(context.Background(), mc, svc, func() error {
		constructService(mc, svc)
		return nil
	}, "Service")
}

func getStoredService(t *testing.T, c client.Client, mc *memcachedv1beta1.Memcached) *corev1.Service {
	t.Helper()
	svc := &corev1.Service{}
	if err := c.Get(context.Background(), client.ObjectKeyFromObject(mc), svc); err != nil {
		t.Fatalf("failed to get stored Service: %v", err)
	}
	return svc
}

func TestReconcileResource_AlreadyExists_UpdatesControlledObject(t *testing.T) {
	mc := alreadyExistsMemcached()
	base := newFakeClient(mc, controlledServiceWithoutLabel(t, mc))
	var counts cacheMissCounts
	recorder := events.NewFakeRecorder(10)
	r := newTestReconcilerWithRecorder(newCacheMissClient(base, &counts, nil), recorder)
	r.APIReader = base

	result, err := reconcileTestService(r, mc)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != controllerutil.OperationResultUpdated {
		t.Errorf("expected OperationResultUpdated, got %v", result)
	}
	if got := getStoredService(t, base, mc).Labels[testLabelKeyAppManagedBy]; got != testLabelValueAppManagedBy {
		t.Errorf("expected the managed-by label %q, got %q", testLabelValueAppManagedBy, got)
	}
	if got := drainEvents(recorder); len(got) != 1 || got[0] != "Normal Updated Updated Service test" {
		t.Errorf("expected one Updated event, got %v", got)
	}
}

func TestReconcileResource_AlreadyExists_UnchangedWhenLiveObjectMatches(t *testing.T) {
	mc := alreadyExistsMemcached()
	desired := &corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: mc.Name, Namespace: mc.Namespace}}
	constructService(mc, desired)
	if err := controllerutil.SetControllerReference(mc, desired, testScheme()); err != nil {
		t.Fatalf("failed to set controller reference: %v", err)
	}
	base := newFakeClient(mc, desired)
	var counts cacheMissCounts
	recorder := events.NewFakeRecorder(10)
	r := newTestReconcilerWithRecorder(newCacheMissClient(base, &counts, nil), recorder)
	r.APIReader = base

	result, err := reconcileTestService(r, mc)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != controllerutil.OperationResultNone {
		t.Errorf("expected OperationResultNone, got %v", result)
	}
	if n := counts.updates.Load(); n != 0 {
		t.Errorf("expected no Update call, got %d", n)
	}
	if got := drainEvents(recorder); len(got) != 0 {
		t.Errorf("expected no event, got %v", got)
	}
}

func TestReconcileResource_AlreadyExists_RejectsUncontrolledObject(t *testing.T) {
	tests := []struct {
		name string
		// controllerUID, when set, adds a controller reference to a Memcached with this UID.
		controllerUID types.UID
	}{
		{name: "no owner reference"},
		{name: "controller reference with another UID", controllerUID: "other-uid"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mc := alreadyExistsMemcached()
			stored := &corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: mc.Name, Namespace: mc.Namespace}}
			if tt.controllerUID != "" {
				other := alreadyExistsMemcached()
				other.UID = tt.controllerUID
				if err := controllerutil.SetControllerReference(other, stored, testScheme()); err != nil {
					t.Fatalf("failed to set controller reference: %v", err)
				}
			}
			base := newFakeClient(mc, stored)
			before := getStoredService(t, base, mc)
			var counts cacheMissCounts
			recorder := events.NewFakeRecorder(10)
			r := newTestReconcilerWithRecorder(newCacheMissClient(base, &counts, nil), recorder)
			r.APIReader = base

			_, err := reconcileTestService(r, mc)
			if !apierrors.IsAlreadyExists(err) {
				t.Fatalf("expected an AlreadyExists error, got %v", err)
			}
			if !strings.Contains(err.Error(), "exists but is not controlled by Memcached") {
				t.Errorf("unexpected error message: %v", err)
			}
			if after := getStoredService(t, base, mc); !equality.Semantic.DeepEqual(before, after) {
				t.Errorf("expected the stored Service unchanged, got %+v", after)
			}
			if n := counts.updates.Load(); n != 0 {
				t.Errorf("expected no Update call, got %d", n)
			}
			got := drainEvents(recorder)
			if len(got) != 1 || !strings.HasPrefix(got[0], "Warning AlreadyExists ") {
				t.Errorf("expected exactly one Warning AlreadyExists event, got %v", got)
			}
		})
	}
}

func TestReconcileResource_AlreadyExists_LiveReadNotFound(t *testing.T) {
	mc := alreadyExistsMemcached()
	base := newFakeClient(mc, controlledServiceWithoutLabel(t, mc))
	var counts cacheMissCounts
	recorder := events.NewFakeRecorder(10)
	r := newTestReconcilerWithRecorder(newCacheMissClient(base, &counts, nil), recorder)
	r.APIReader = interceptor.NewClient(base, interceptor.Funcs{
		Get: func(_ context.Context, _ client.WithWatch, key client.ObjectKey, _ client.Object, _ ...client.GetOption) error {
			return apierrors.NewNotFound(corev1.Resource("services"), key.Name)
		},
	})

	_, err := reconcileTestService(r, mc)
	if !apierrors.IsNotFound(err) {
		t.Fatalf("expected a NotFound error, got %v", err)
	}
	if got := drainEvents(recorder); len(got) != 0 {
		t.Errorf("expected no event, got %v", got)
	}
	if n := counts.creates.Load(); n != 1 {
		t.Errorf("expected one Create call, got %d", n)
	}
}

func TestReconcileResource_AlreadyExists_LiveReadError(t *testing.T) {
	mc := alreadyExistsMemcached()
	base := newFakeClient(mc, controlledServiceWithoutLabel(t, mc))
	var counts cacheMissCounts
	r := newTestReconciler(newCacheMissClient(base, &counts, nil))
	r.APIReader = interceptor.NewClient(base, interceptor.Funcs{
		Get: func(context.Context, client.WithWatch, client.ObjectKey, client.Object, ...client.GetOption) error {
			return apierrors.NewServiceUnavailable("x")
		},
	})

	_, err := reconcileTestService(r, mc)
	if !apierrors.IsServiceUnavailable(err) {
		t.Fatalf("expected a ServiceUnavailable error, got %v", err)
	}
	if !strings.HasPrefix(err.Error(), "reconciling Service: reading Service") {
		t.Errorf("unexpected error message: %v", err)
	}
	if n := counts.creates.Load(); n != 1 {
		t.Errorf("expected one Create call, got %d", n)
	}
}

func TestReconcileResource_AlreadyExists_UpdateConflictRetries(t *testing.T) {
	mc := alreadyExistsMemcached()
	base := newFakeClient(mc, controlledServiceWithoutLabel(t, mc))
	var counts cacheMissCounts
	r := newTestReconciler(newCacheMissClient(base, &counts, func(call int32) error {
		if call == 1 {
			return apierrors.NewConflict(corev1.Resource("services"), mc.Name, errors.New("the object has been modified"))
		}
		return nil
	}))
	r.APIReader = base

	result, err := reconcileTestService(r, mc)
	if err != nil {
		t.Fatalf("expected the retry to succeed, got %v", err)
	}
	if result != controllerutil.OperationResultUpdated {
		t.Errorf("expected OperationResultUpdated, got %v", result)
	}
	if n := counts.updates.Load(); n != 2 {
		t.Errorf("expected two Update attempts, got %d", n)
	}
	if got := getStoredService(t, base, mc).Labels[testLabelKeyAppManagedBy]; got != testLabelValueAppManagedBy {
		t.Errorf("expected the managed-by label %q, got %q", testLabelValueAppManagedBy, got)
	}
}

func TestReconcileResource_AlreadyExists_NilRecorder(t *testing.T) {
	mc := alreadyExistsMemcached()
	base := newFakeClient(mc, &corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: mc.Name, Namespace: mc.Namespace}})
	var counts cacheMissCounts
	r := newTestReconciler(newCacheMissClient(base, &counts, nil))
	r.APIReader = base

	_, err := reconcileTestService(r, mc)
	if !apierrors.IsAlreadyExists(err) {
		t.Fatalf("expected an AlreadyExists error, got %v", err)
	}
	if !strings.Contains(err.Error(), "exists but is not controlled by Memcached") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestReconcileResource_AlreadyExists_NilAPIReader(t *testing.T) {
	mc := alreadyExistsMemcached()
	base := newFakeClient(mc, controlledServiceWithoutLabel(t, mc))
	var serviceGets atomic.Int32
	c := interceptor.NewClient(base, interceptor.Funcs{
		Get: func(ctx context.Context, c client.WithWatch, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
			if _, ok := obj.(*corev1.Service); ok && serviceGets.Add(1) == 1 {
				return apierrors.NewNotFound(corev1.Resource("services"), key.Name)
			}
			return c.Get(ctx, key, obj, opts...)
		},
	})
	r := newTestReconciler(c)

	result, err := reconcileTestService(r, mc)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != controllerutil.OperationResultUpdated {
		t.Errorf("expected OperationResultUpdated, got %v", result)
	}
	if n := serviceGets.Load(); n != 2 {
		t.Errorf("expected the live read to go to Client (2 Service Gets), got %d", n)
	}
	if got := getStoredService(t, base, mc).Labels[testLabelKeyAppManagedBy]; got != testLabelValueAppManagedBy {
		t.Errorf("expected the managed-by label %q, got %q", testLabelValueAppManagedBy, got)
	}
}
