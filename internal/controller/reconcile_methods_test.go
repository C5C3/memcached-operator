// Package controller implements the reconciliation logic for the memcached-operator.
package controller

import (
	"context"
	"errors"
	"strings"
	"testing"

	monitoringv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	policyv1 "k8s.io/api/policy/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/events"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	memcachedv1beta1 "github.com/c5c3/memcached-operator/api/v1beta1"
	// Import metrics package to ensure init() registration runs.
	_ "github.com/c5c3/memcached-operator/internal/metrics"
)

const (
	testInstanceName     = "test-mc"
	testMetricsPort      = "metrics"
	testDefaultNamespace = "default"
)

// testSchemeWithMonitoring returns a scheme that includes core types, Memcached, and monitoring/v1.
func testSchemeWithMonitoring() *runtime.Scheme {
	s := runtime.NewScheme()
	_ = scheme.AddToScheme(s)
	_ = memcachedv1beta1.AddToScheme(s)
	_ = monitoringv1.AddToScheme(s)
	return s
}

func newFakeClientWithMonitoring(objs ...client.Object) client.WithWatch {
	return fake.NewClientBuilder().WithScheme(testSchemeWithMonitoring()).WithObjects(objs...).Build()
}

func newTestReconcilerWithMonitoring(c client.Client) *MemcachedReconciler {
	return &MemcachedReconciler{
		Client: c,
		Scheme: testSchemeWithMonitoring(),
	}
}

// --- reconcileStatefulSet ---

func TestReconcileStatefulSet_CreatesStatefulSet(t *testing.T) {
	mc := &memcachedv1beta1.Memcached{
		ObjectMeta: metav1.ObjectMeta{Name: testInstanceName, Namespace: testDefaultNamespace, UID: "uid-1"},
		Spec:       memcachedv1beta1.MemcachedSpec{},
	}
	c := newFakeClient(mc)
	r := newTestReconciler(c)

	if _, err := r.reconcileStatefulSet(context.Background(), mc); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	sts := &appsv1.StatefulSet{}
	if err := c.Get(context.Background(), client.ObjectKey{Name: testInstanceName, Namespace: testDefaultNamespace}, sts); err != nil {
		t.Fatalf("failed to get created StatefulSet: %v", err)
	}

	// Verify name and namespace.
	if sts.Name != testInstanceName {
		t.Errorf("StatefulSet name = %q, want %q", sts.Name, testInstanceName)
	}
	if sts.Namespace != testDefaultNamespace {
		t.Errorf("StatefulSet namespace = %q, want %q", sts.Namespace, testDefaultNamespace)
	}

	// Verify labels.
	expectedLabels := labelsForMemcached(testInstanceName)
	for k, v := range expectedLabels {
		if sts.Labels[k] != v {
			t.Errorf("label %q = %q, want %q", k, sts.Labels[k], v)
		}
	}

	// Verify replicas default to 1.
	if sts.Spec.Replicas == nil || *sts.Spec.Replicas != 1 {
		t.Errorf("replicas = %v, want 1", sts.Spec.Replicas)
	}

	// Verify owner reference.
	if len(sts.OwnerReferences) != 1 {
		t.Fatalf("expected 1 owner reference, got %d", len(sts.OwnerReferences))
	}
	ref := sts.OwnerReferences[0]
	if ref.Name != mc.Name {
		t.Errorf("owner reference name = %q, want %q", ref.Name, mc.Name)
	}
	if ref.UID != mc.UID {
		t.Errorf("owner reference UID = %q, want %q", ref.UID, mc.UID)
	}
}

func TestReconcileStatefulSet_UpdatesExistingStatefulSet(t *testing.T) {
	replicas3 := int32(3)
	mc := &memcachedv1beta1.Memcached{
		ObjectMeta: metav1.ObjectMeta{Name: testInstanceName, Namespace: testDefaultNamespace, UID: "uid-1"},
		Spec:       memcachedv1beta1.MemcachedSpec{Replicas: &replicas3},
	}
	// Create an existing StatefulSet with different replicas.
	replicas1 := int32(1)
	existingSts := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{Name: testInstanceName, Namespace: testDefaultNamespace},
		Spec: appsv1.StatefulSetSpec{
			Replicas: &replicas1,
			Selector: &metav1.LabelSelector{
				MatchLabels: labelsForMemcached(testInstanceName),
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: labelsForMemcached(testInstanceName),
				},
			},
		},
	}
	c := newFakeClient(mc, existingSts)
	r := newTestReconciler(c)

	if _, err := r.reconcileStatefulSet(context.Background(), mc); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	sts := &appsv1.StatefulSet{}
	if err := c.Get(context.Background(), client.ObjectKey{Name: testInstanceName, Namespace: testDefaultNamespace}, sts); err != nil {
		t.Fatalf("failed to get StatefulSet: %v", err)
	}

	// Verify replicas were updated.
	if sts.Spec.Replicas == nil || *sts.Spec.Replicas != 3 {
		t.Errorf("replicas = %v, want 3", sts.Spec.Replicas)
	}
}

func TestReconcileStatefulSet_SetsContainerImage(t *testing.T) {
	customImage := "memcached:1.7-alpine"
	mc := &memcachedv1beta1.Memcached{
		ObjectMeta: metav1.ObjectMeta{Name: "img-mc", Namespace: testDefaultNamespace, UID: "uid-2"},
		Spec:       memcachedv1beta1.MemcachedSpec{Image: &customImage},
	}
	c := newFakeClient(mc)
	r := newTestReconciler(c)

	if _, err := r.reconcileStatefulSet(context.Background(), mc); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	sts := &appsv1.StatefulSet{}
	if err := c.Get(context.Background(), client.ObjectKey{Name: "img-mc", Namespace: testDefaultNamespace}, sts); err != nil {
		t.Fatalf("failed to get StatefulSet: %v", err)
	}

	if len(sts.Spec.Template.Spec.Containers) == 0 {
		t.Fatal("expected at least 1 container")
	}
	if sts.Spec.Template.Spec.Containers[0].Image != customImage {
		t.Errorf("container image = %q, want %q", sts.Spec.Template.Spec.Containers[0].Image, customImage)
	}
}

// --- reconcileStatefulSet: replica count and naming ---

func TestReconcileStatefulSet_KeepsReplicaCountSetByHPA(t *testing.T) {
	mc := &memcachedv1beta1.Memcached{
		ObjectMeta: metav1.ObjectMeta{Name: testInstanceName, Namespace: testDefaultNamespace, UID: "uid-1"},
		Spec: memcachedv1beta1.MemcachedSpec{
			Autoscaling: &memcachedv1beta1.AutoscalingSpec{
				Enabled:     true,
				MinReplicas: int32Ptr(2),
				MaxReplicas: 10,
			},
		},
	}
	// The HPA scaled the existing StatefulSet to 5 replicas.
	existingSts := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{Name: testInstanceName, Namespace: testDefaultNamespace},
		Spec:       appsv1.StatefulSetSpec{Replicas: int32Ptr(5)},
	}
	c := newFakeClient(mc, existingSts)
	r := newTestReconciler(c)

	if _, err := r.reconcileStatefulSet(context.Background(), mc); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	sts := &appsv1.StatefulSet{}
	if err := c.Get(context.Background(), client.ObjectKey{Name: testInstanceName, Namespace: testDefaultNamespace}, sts); err != nil {
		t.Fatalf("failed to get StatefulSet: %v", err)
	}
	if sts.Spec.Replicas == nil || *sts.Spec.Replicas != 5 {
		t.Errorf("replicas = %v, want 5", sts.Spec.Replicas)
	}
}

func TestReconcileStatefulSet_LongNameUsesShortenedStatefulSetName(t *testing.T) {
	name := strings.Repeat("n", 60)
	mc := &memcachedv1beta1.Memcached{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: testDefaultNamespace, UID: "uid-long"},
	}
	c := newFakeClient(mc)
	r := newTestReconciler(c)

	if _, err := r.reconcileStatefulSet(context.Background(), mc); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	sts := &appsv1.StatefulSet{}
	if err := c.Get(context.Background(), client.ObjectKey{Name: statefulSetName(name), Namespace: testDefaultNamespace}, sts); err != nil {
		t.Fatalf("failed to get StatefulSet %q: %v", statefulSetName(name), err)
	}
	if sts.Spec.ServiceName != name {
		t.Errorf("serviceName = %q, want %q", sts.Spec.ServiceName, name)
	}

	err := c.Get(context.Background(), client.ObjectKey{Name: name, Namespace: testDefaultNamespace}, &appsv1.StatefulSet{})
	if !apierrors.IsNotFound(err) {
		t.Errorf("expected no StatefulSet with the full name, got err = %v", err)
	}
}

// --- reconcileStatefulSet: legacy Deployment migration ---

// legacyDeployment returns a Deployment named like mc, like the one that operator releases
// without the StatefulSet ran the pods in. A non-empty controllerUID adds a controller owner
// reference with that UID; an empty one leaves the Deployment without owner references.
func legacyDeployment(mc *memcachedv1beta1.Memcached, controllerUID types.UID) *appsv1.Deployment {
	dep := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      mc.Name,
			Namespace: mc.Namespace,
			Labels:    labelsForMemcached(mc.Name),
		},
		Spec: appsv1.DeploymentSpec{
			Selector: &metav1.LabelSelector{MatchLabels: labelsForMemcached(mc.Name)},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: labelsForMemcached(mc.Name)},
			},
		},
	}
	if controllerUID != "" {
		isController := true
		dep.OwnerReferences = []metav1.OwnerReference{{
			APIVersion: memcachedv1beta1.GroupVersion.String(),
			Kind:       "Memcached",
			Name:       mc.Name,
			UID:        controllerUID,
			Controller: &isController,
		}}
	}
	return dep
}

// newLegacyTestMemcached returns a Memcached with an explicit UID, so that a legacy
// Deployment's controller owner reference can match it.
func newLegacyTestMemcached() *memcachedv1beta1.Memcached {
	return &memcachedv1beta1.Memcached{
		ObjectMeta: metav1.ObjectMeta{Name: testInstanceName, Namespace: testDefaultNamespace, UID: "uid-legacy"},
	}
}

// drainEvents returns every event recorded so far without blocking.
func drainEvents(recorder *events.FakeRecorder) []string {
	var got []string
	for {
		select {
		case e := <-recorder.Events:
			got = append(got, e)
		default:
			return got
		}
	}
}

func TestReconcileStatefulSet_DeletesControlledLegacyDeployment(t *testing.T) {
	mc := newLegacyTestMemcached()
	var deleteOpts client.DeleteOptions
	var deleteCalls int
	c := interceptor.NewClient(newFakeClient(mc, legacyDeployment(mc, mc.UID)), interceptor.Funcs{
		Delete: func(ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.DeleteOption) error {
			if _, ok := obj.(*appsv1.Deployment); ok {
				deleteCalls++
				deleteOpts.ApplyOptions(opts)
			}
			return c.Delete(ctx, obj, opts...)
		},
	})
	r := newTestReconciler(c)

	if _, err := r.reconcileStatefulSet(context.Background(), mc); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	key := client.ObjectKey{Name: mc.Name, Namespace: mc.Namespace}
	if err := c.Get(context.Background(), key, &appsv1.Deployment{}); !apierrors.IsNotFound(err) {
		t.Errorf("expected legacy Deployment to be deleted, got err = %v", err)
	}
	if err := c.Get(context.Background(), key, &appsv1.StatefulSet{}); err != nil {
		t.Errorf("expected StatefulSet to exist: %v", err)
	}
	if deleteCalls != 1 {
		t.Errorf("Deployment delete calls = %d, want 1", deleteCalls)
	}
	if deleteOpts.PropagationPolicy == nil || *deleteOpts.PropagationPolicy != metav1.DeletePropagationBackground {
		t.Errorf("propagation policy = %v, want %q", deleteOpts.PropagationPolicy, metav1.DeletePropagationBackground)
	}
}

// TestReconcileStatefulSet_LongNameDeletesLegacyDeployment checks that the legacy
// Deployment is looked up by the full CR name, not by the shortened StatefulSet name,
// for a CR name longer than 52 characters.
func TestReconcileStatefulSet_LongNameDeletesLegacyDeployment(t *testing.T) {
	mc := &memcachedv1beta1.Memcached{
		ObjectMeta: metav1.ObjectMeta{Name: strings.Repeat("d", 60), Namespace: testDefaultNamespace, UID: "uid-legacy-long"},
	}
	if statefulSetName(mc.Name) == mc.Name {
		t.Fatalf("statefulSetName(%q) should differ from the CR name", mc.Name)
	}
	c := newFakeClient(mc, legacyDeployment(mc, mc.UID))
	r := newTestReconciler(c)

	if _, err := r.reconcileStatefulSet(context.Background(), mc); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if err := c.Get(context.Background(), client.ObjectKey{Name: mc.Name, Namespace: mc.Namespace}, &appsv1.Deployment{}); !apierrors.IsNotFound(err) {
		t.Errorf("expected legacy Deployment %q to be deleted, got err = %v", mc.Name, err)
	}
	if err := c.Get(context.Background(), client.ObjectKey{Name: statefulSetName(mc.Name), Namespace: mc.Namespace}, &appsv1.StatefulSet{}); err != nil {
		t.Errorf("expected StatefulSet to exist: %v", err)
	}
}

func TestReconcileStatefulSet_KeepsDeploymentNotControlledByMemcached(t *testing.T) {
	mc := newLegacyTestMemcached()
	notController := false

	tests := []struct {
		name string
		dep  *appsv1.Deployment
	}{
		{
			name: "no owner references",
			dep:  legacyDeployment(mc, ""),
		},
		{
			name: "controlled by a different UID",
			dep:  legacyDeployment(mc, "uid-other"),
		},
		{
			name: "owned by the Memcached without controller flag",
			dep: func() *appsv1.Deployment {
				dep := legacyDeployment(mc, mc.UID)
				dep.OwnerReferences[0].Controller = &notController
				return dep
			}(),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var deleteCalls int
			c := interceptor.NewClient(newFakeClient(mc, tt.dep), interceptor.Funcs{
				Delete: func(ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.DeleteOption) error {
					if _, ok := obj.(*appsv1.Deployment); ok {
						deleteCalls++
					}
					return c.Delete(ctx, obj, opts...)
				},
			})
			r := newTestReconciler(c)

			for i := range 2 {
				if _, err := r.reconcileStatefulSet(context.Background(), mc); err != nil {
					t.Fatalf("reconcile %d: unexpected error: %v", i+1, err)
				}
			}

			if err := c.Get(context.Background(), client.ObjectKeyFromObject(tt.dep), &appsv1.Deployment{}); err != nil {
				t.Errorf("expected Deployment to survive, got err = %v", err)
			}
			if deleteCalls != 0 {
				t.Errorf("Deployment delete calls = %d, want 0", deleteCalls)
			}
		})
	}
}

func TestReconcileStatefulSet_LegacyDeploymentDeleteError(t *testing.T) {
	mc := newLegacyTestMemcached()
	injected := errors.New("injected delete error")
	c := interceptor.NewClient(newFakeClient(mc, legacyDeployment(mc, mc.UID)), interceptor.Funcs{
		Delete: func(ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.DeleteOption) error {
			if _, ok := obj.(*appsv1.Deployment); ok {
				return injected
			}
			return c.Delete(ctx, obj, opts...)
		},
	})
	r := newTestReconciler(c)

	_, err := r.reconcileStatefulSet(context.Background(), mc)
	if err == nil {
		t.Fatal("expected an error, got nil")
	}
	if !strings.HasPrefix(err.Error(), "deleting legacy Deployment: ") {
		t.Errorf("error = %q, want prefix %q", err.Error(), "deleting legacy Deployment: ")
	}
	if !errors.Is(err, injected) {
		t.Errorf("error %v does not wrap the injected error", err)
	}

	key := client.ObjectKey{Name: mc.Name, Namespace: mc.Namespace}
	if err := c.Get(context.Background(), key, &appsv1.StatefulSet{}); err != nil {
		t.Errorf("expected StatefulSet to exist: %v", err)
	}
}

func TestReconcileStatefulSet_StatefulSetCreateErrorKeepsLegacyDeployment(t *testing.T) {
	mc := newLegacyTestMemcached()
	injected := errors.New("injected create error")
	c := interceptor.NewClient(newFakeClient(mc, legacyDeployment(mc, mc.UID)), interceptor.Funcs{
		Create: func(ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.CreateOption) error {
			if _, ok := obj.(*appsv1.StatefulSet); ok {
				return injected
			}
			return c.Create(ctx, obj, opts...)
		},
	})
	r := newTestReconciler(c)

	_, err := r.reconcileStatefulSet(context.Background(), mc)
	if err == nil {
		t.Fatal("expected an error, got nil")
	}
	if !strings.HasPrefix(err.Error(), "reconciling StatefulSet: ") {
		t.Errorf("error = %q, want prefix %q", err.Error(), "reconciling StatefulSet: ")
	}
	if !errors.Is(err, injected) {
		t.Errorf("error %v does not wrap the injected error", err)
	}

	key := client.ObjectKey{Name: mc.Name, Namespace: mc.Namespace}
	if err := c.Get(context.Background(), key, &appsv1.Deployment{}); err != nil {
		t.Errorf("expected legacy Deployment to survive a failed StatefulSet create: %v", err)
	}
}

func TestReconcileStatefulSet_LegacyDeploymentGetError(t *testing.T) {
	mc := newLegacyTestMemcached()
	injected := errors.New("injected get error")
	var deleteCalls int
	c := interceptor.NewClient(newFakeClient(mc, legacyDeployment(mc, mc.UID)), interceptor.Funcs{
		Get: func(ctx context.Context, c client.WithWatch, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
			if _, ok := obj.(*appsv1.Deployment); ok {
				return injected
			}
			return c.Get(ctx, key, obj, opts...)
		},
		Delete: func(ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.DeleteOption) error {
			deleteCalls++
			return c.Delete(ctx, obj, opts...)
		},
	})
	r := newTestReconciler(c)

	_, err := r.reconcileStatefulSet(context.Background(), mc)
	if err == nil {
		t.Fatal("expected an error, got nil")
	}
	if !strings.HasPrefix(err.Error(), "fetching legacy Deployment: ") {
		t.Errorf("error = %q, want prefix %q", err.Error(), "fetching legacy Deployment: ")
	}
	if !errors.Is(err, injected) {
		t.Errorf("error %v does not wrap the injected error", err)
	}
	if deleteCalls != 0 {
		t.Errorf("delete calls = %d, want 0", deleteCalls)
	}
}

func TestReconcileStatefulSet_LegacyDeploymentAlreadyDeleted(t *testing.T) {
	mc := newLegacyTestMemcached()
	c := interceptor.NewClient(newFakeClient(mc, legacyDeployment(mc, mc.UID)), interceptor.Funcs{
		Delete: func(ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.DeleteOption) error {
			if _, ok := obj.(*appsv1.Deployment); ok {
				return apierrors.NewNotFound(schema.GroupResource{Group: "apps", Resource: "deployments"}, obj.GetName())
			}
			return c.Delete(ctx, obj, opts...)
		},
	})
	r := newTestReconciler(c)

	if _, err := r.reconcileStatefulSet(context.Background(), mc); err != nil {
		t.Fatalf("expected nil error when the Deployment is already gone, got %v", err)
	}
}

func TestReconcileStatefulSet_LegacyDeploymentEvent(t *testing.T) {
	wantEvent := "Normal Deleted Deleted legacy Deployment " + testInstanceName

	tests := []struct {
		name       string
		withLegacy bool
		wantCount  int
	}{
		{name: "successful migration emits one event", withLegacy: true, wantCount: 1},
		{name: "no legacy Deployment emits no event", withLegacy: false, wantCount: 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mc := newLegacyTestMemcached()
			objs := []client.Object{mc}
			if tt.withLegacy {
				objs = append(objs, legacyDeployment(mc, mc.UID))
			}
			recorder := events.NewFakeRecorder(10)
			r := newTestReconcilerWithRecorder(newFakeClient(objs...), recorder)

			if _, err := r.reconcileStatefulSet(context.Background(), mc); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			recorded := drainEvents(recorder)
			count := 0
			for _, e := range recorded {
				if e == wantEvent {
					count++
				}
			}
			if count != tt.wantCount {
				t.Errorf("got %d events %q, want %d (recorded: %v)", count, wantEvent, tt.wantCount, recorded)
			}
		})
	}
}

// --- reconcileService ---

func TestReconcileService_CreatesService(t *testing.T) {
	mc := &memcachedv1beta1.Memcached{
		ObjectMeta: metav1.ObjectMeta{Name: testInstanceName, Namespace: testDefaultNamespace, UID: "uid-1"},
		Spec:       memcachedv1beta1.MemcachedSpec{},
	}
	c := newFakeClient(mc)
	r := newTestReconciler(c)

	if err := r.reconcileService(context.Background(), mc); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	svc := &corev1.Service{}
	if err := c.Get(context.Background(), client.ObjectKey{Name: testInstanceName, Namespace: testDefaultNamespace}, svc); err != nil {
		t.Fatalf("failed to get created service: %v", err)
	}

	// Verify name and namespace.
	if svc.Name != testInstanceName {
		t.Errorf("service name = %q, want %q", svc.Name, testInstanceName)
	}
	if svc.Namespace != testDefaultNamespace {
		t.Errorf("service namespace = %q, want %q", svc.Namespace, testDefaultNamespace)
	}

	// Verify headless.
	if svc.Spec.ClusterIP != corev1.ClusterIPNone {
		t.Errorf("clusterIP = %q, want %q", svc.Spec.ClusterIP, corev1.ClusterIPNone)
	}

	// Verify port.
	if len(svc.Spec.Ports) != 1 {
		t.Fatalf("expected 1 port, got %d", len(svc.Spec.Ports))
	}
	if svc.Spec.Ports[0].Port != 11211 {
		t.Errorf("port = %d, want 11211", svc.Spec.Ports[0].Port)
	}

	// Verify owner reference.
	if len(svc.OwnerReferences) != 1 {
		t.Fatalf("expected 1 owner reference, got %d", len(svc.OwnerReferences))
	}
	if svc.OwnerReferences[0].Name != mc.Name {
		t.Errorf("owner reference name = %q, want %q", svc.OwnerReferences[0].Name, mc.Name)
	}
}

func TestReconcileService_UpdatesExistingService(t *testing.T) {
	mc := &memcachedv1beta1.Memcached{
		ObjectMeta: metav1.ObjectMeta{Name: testInstanceName, Namespace: testDefaultNamespace, UID: "uid-1"},
		Spec:       memcachedv1beta1.MemcachedSpec{},
	}
	existingSvc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: testInstanceName, Namespace: testDefaultNamespace},
		Spec: corev1.ServiceSpec{
			Ports: []corev1.ServicePort{
				{Name: "old-port", Port: 9999},
			},
		},
	}
	c := newFakeClient(mc, existingSvc)
	r := newTestReconciler(c)

	if err := r.reconcileService(context.Background(), mc); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	svc := &corev1.Service{}
	if err := c.Get(context.Background(), client.ObjectKey{Name: testInstanceName, Namespace: testDefaultNamespace}, svc); err != nil {
		t.Fatalf("failed to get service: %v", err)
	}

	// Verify port was updated to memcached default.
	if len(svc.Spec.Ports) != 1 || svc.Spec.Ports[0].Port != 11211 {
		t.Errorf("expected port 11211, got %v", svc.Spec.Ports)
	}
}

// --- reconcilePDB ---

func TestReconcilePDB_SkipsWhenDisabled(t *testing.T) {
	mc := &memcachedv1beta1.Memcached{
		ObjectMeta: metav1.ObjectMeta{Name: testInstanceName, Namespace: testDefaultNamespace, UID: "uid-1"},
		Spec:       memcachedv1beta1.MemcachedSpec{},
	}
	c := newFakeClient(mc)
	r := newTestReconciler(c)

	if err := r.reconcilePDB(context.Background(), mc); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify no PDB was created.
	pdb := &policyv1.PodDisruptionBudget{}
	err := c.Get(context.Background(), client.ObjectKey{Name: testInstanceName, Namespace: testDefaultNamespace}, pdb)
	if err == nil {
		t.Error("expected PDB to not be created when disabled")
	}
}

func TestReconcilePDB_CreatesPDB(t *testing.T) {
	mc := &memcachedv1beta1.Memcached{
		ObjectMeta: metav1.ObjectMeta{Name: testInstanceName, Namespace: testDefaultNamespace, UID: "uid-1"},
		Spec: memcachedv1beta1.MemcachedSpec{
			HighAvailability: &memcachedv1beta1.HighAvailabilitySpec{
				PodDisruptionBudget: &memcachedv1beta1.PDBSpec{
					Enabled:      true,
					MinAvailable: intOrStringPtr(intstr.FromInt32(2)),
				},
			},
		},
	}
	c := newFakeClient(mc)
	r := newTestReconciler(c)

	if err := r.reconcilePDB(context.Background(), mc); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	pdb := &policyv1.PodDisruptionBudget{}
	if err := c.Get(context.Background(), client.ObjectKey{Name: testInstanceName, Namespace: testDefaultNamespace}, pdb); err != nil {
		t.Fatalf("failed to get created PDB: %v", err)
	}

	// Verify name and namespace.
	if pdb.Name != testInstanceName {
		t.Errorf("PDB name = %q, want %q", pdb.Name, testInstanceName)
	}

	// Verify minAvailable.
	if pdb.Spec.MinAvailable == nil || *pdb.Spec.MinAvailable != intstr.FromInt32(2) {
		t.Errorf("MinAvailable = %v, want 2", pdb.Spec.MinAvailable)
	}

	// Verify owner reference.
	if len(pdb.OwnerReferences) != 1 {
		t.Fatalf("expected 1 owner reference, got %d", len(pdb.OwnerReferences))
	}
	if pdb.OwnerReferences[0].Name != mc.Name {
		t.Errorf("owner reference name = %q, want %q", pdb.OwnerReferences[0].Name, mc.Name)
	}
}

func TestReconcilePDB_UpdatesExistingPDB(t *testing.T) {
	mc := &memcachedv1beta1.Memcached{
		ObjectMeta: metav1.ObjectMeta{Name: testInstanceName, Namespace: testDefaultNamespace, UID: "uid-1"},
		Spec: memcachedv1beta1.MemcachedSpec{
			HighAvailability: &memcachedv1beta1.HighAvailabilitySpec{
				PodDisruptionBudget: &memcachedv1beta1.PDBSpec{
					Enabled:        true,
					MaxUnavailable: intOrStringPtr(intstr.FromInt32(1)),
				},
			},
		},
	}
	// Existing PDB with different settings.
	existingMinAvail := intstr.FromInt32(2)
	existingPDB := &policyv1.PodDisruptionBudget{
		ObjectMeta: metav1.ObjectMeta{Name: testInstanceName, Namespace: testDefaultNamespace},
		Spec: policyv1.PodDisruptionBudgetSpec{
			MinAvailable: &existingMinAvail,
		},
	}
	c := newFakeClient(mc, existingPDB)
	r := newTestReconciler(c)

	if err := r.reconcilePDB(context.Background(), mc); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	pdb := &policyv1.PodDisruptionBudget{}
	if err := c.Get(context.Background(), client.ObjectKey{Name: testInstanceName, Namespace: testDefaultNamespace}, pdb); err != nil {
		t.Fatalf("failed to get PDB: %v", err)
	}

	// Verify switched to maxUnavailable.
	if pdb.Spec.MaxUnavailable == nil || *pdb.Spec.MaxUnavailable != intstr.FromInt32(1) {
		t.Errorf("MaxUnavailable = %v, want 1", pdb.Spec.MaxUnavailable)
	}
	if pdb.Spec.MinAvailable != nil {
		t.Errorf("MinAvailable should be nil after switch, got %v", *pdb.Spec.MinAvailable)
	}
}

// --- reconcileServiceMonitor ---

func TestReconcileServiceMonitor_SkipsWhenDisabled(t *testing.T) {
	mc := &memcachedv1beta1.Memcached{
		ObjectMeta: metav1.ObjectMeta{Name: testInstanceName, Namespace: testDefaultNamespace, UID: "uid-1"},
		Spec:       memcachedv1beta1.MemcachedSpec{},
	}
	c := newFakeClientWithMonitoring(mc)
	r := newTestReconcilerWithMonitoring(c)

	if err := r.reconcileServiceMonitor(context.Background(), mc); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify no ServiceMonitor was created.
	sm := &monitoringv1.ServiceMonitor{}
	err := c.Get(context.Background(), client.ObjectKey{Name: testInstanceName, Namespace: testDefaultNamespace}, sm)
	if err == nil {
		t.Error("expected ServiceMonitor to not be created when disabled")
	}
}

func TestReconcileServiceMonitor_CreatesServiceMonitor(t *testing.T) {
	mc := &memcachedv1beta1.Memcached{
		ObjectMeta: metav1.ObjectMeta{Name: testInstanceName, Namespace: testDefaultNamespace, UID: "uid-1"},
		Spec: memcachedv1beta1.MemcachedSpec{
			Monitoring: &memcachedv1beta1.MonitoringSpec{
				Enabled: true,
				ServiceMonitor: &memcachedv1beta1.ServiceMonitorSpec{
					Interval: "60s",
				},
			},
		},
	}
	c := newFakeClientWithMonitoring(mc)
	r := newTestReconcilerWithMonitoring(c)

	if err := r.reconcileServiceMonitor(context.Background(), mc); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	sm := &monitoringv1.ServiceMonitor{}
	if err := c.Get(context.Background(), client.ObjectKey{Name: testInstanceName, Namespace: testDefaultNamespace}, sm); err != nil {
		t.Fatalf("failed to get created ServiceMonitor: %v", err)
	}

	// Verify name and namespace.
	if sm.Name != testInstanceName {
		t.Errorf("ServiceMonitor name = %q, want %q", sm.Name, testInstanceName)
	}

	// Verify endpoint.
	if len(sm.Spec.Endpoints) != 1 {
		t.Fatalf("expected 1 endpoint, got %d", len(sm.Spec.Endpoints))
	}
	if sm.Spec.Endpoints[0].Port != testMetricsPort {
		t.Errorf("endpoint port = %q, want %q", sm.Spec.Endpoints[0].Port, testMetricsPort)
	}
	if sm.Spec.Endpoints[0].Interval != "60s" {
		t.Errorf("interval = %q, want %q", sm.Spec.Endpoints[0].Interval, "60s")
	}

	// Verify owner reference.
	if len(sm.OwnerReferences) != 1 {
		t.Fatalf("expected 1 owner reference, got %d", len(sm.OwnerReferences))
	}
	if sm.OwnerReferences[0].Name != mc.Name {
		t.Errorf("owner reference name = %q, want %q", sm.OwnerReferences[0].Name, mc.Name)
	}
}

func TestReconcileServiceMonitor_SkipsWhenMonitoringEnabledButNoServiceMonitorSpec(t *testing.T) {
	mc := &memcachedv1beta1.Memcached{
		ObjectMeta: metav1.ObjectMeta{Name: testInstanceName, Namespace: testDefaultNamespace, UID: "uid-1"},
		Spec: memcachedv1beta1.MemcachedSpec{
			Monitoring: &memcachedv1beta1.MonitoringSpec{
				Enabled: true,
				// ServiceMonitor is nil.
			},
		},
	}
	c := newFakeClientWithMonitoring(mc)
	r := newTestReconcilerWithMonitoring(c)

	if err := r.reconcileServiceMonitor(context.Background(), mc); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify no ServiceMonitor was created.
	sm := &monitoringv1.ServiceMonitor{}
	err := c.Get(context.Background(), client.ObjectKey{Name: testInstanceName, Namespace: testDefaultNamespace}, sm)
	if err == nil {
		t.Error("expected ServiceMonitor to not be created when ServiceMonitor spec is nil")
	}
}

// --- reconcileNetworkPolicy ---

func TestReconcileNetworkPolicy_SkipsWhenDisabled(t *testing.T) {
	mc := &memcachedv1beta1.Memcached{
		ObjectMeta: metav1.ObjectMeta{Name: testInstanceName, Namespace: testDefaultNamespace, UID: "uid-1"},
		Spec:       memcachedv1beta1.MemcachedSpec{},
	}
	c := newFakeClient(mc)
	r := newTestReconciler(c)

	if err := r.reconcileNetworkPolicy(context.Background(), mc); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify no NetworkPolicy was created.
	np := &networkingv1.NetworkPolicy{}
	err := c.Get(context.Background(), client.ObjectKey{Name: testInstanceName, Namespace: testDefaultNamespace}, np)
	if err == nil {
		t.Error("expected NetworkPolicy to not be created when disabled")
	}
}

func TestReconcileNetworkPolicy_CreatesNetworkPolicy(t *testing.T) {
	mc := &memcachedv1beta1.Memcached{
		ObjectMeta: metav1.ObjectMeta{Name: testInstanceName, Namespace: testDefaultNamespace, UID: "uid-1"},
		Spec: memcachedv1beta1.MemcachedSpec{
			Security: &memcachedv1beta1.SecuritySpec{
				NetworkPolicy: &memcachedv1beta1.NetworkPolicySpec{
					Enabled: true,
				},
			},
		},
	}
	c := newFakeClient(mc)
	r := newTestReconciler(c)

	if err := r.reconcileNetworkPolicy(context.Background(), mc); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	np := &networkingv1.NetworkPolicy{}
	if err := c.Get(context.Background(), client.ObjectKey{Name: testInstanceName, Namespace: testDefaultNamespace}, np); err != nil {
		t.Fatalf("failed to get created NetworkPolicy: %v", err)
	}

	// Verify name and namespace.
	if np.Name != testInstanceName {
		t.Errorf("NetworkPolicy name = %q, want %q", np.Name, testInstanceName)
	}

	// Verify policy types.
	if len(np.Spec.PolicyTypes) != 1 || np.Spec.PolicyTypes[0] != networkingv1.PolicyTypeIngress {
		t.Errorf("policyTypes = %v, want [Ingress]", np.Spec.PolicyTypes)
	}

	// Verify ingress rules include memcached port.
	if len(np.Spec.Ingress) != 1 {
		t.Fatalf("expected 1 ingress rule, got %d", len(np.Spec.Ingress))
	}
	if len(np.Spec.Ingress[0].Ports) < 1 {
		t.Fatal("expected at least 1 port in ingress rule")
	}
	if np.Spec.Ingress[0].Ports[0].Port.IntValue() != 11211 {
		t.Errorf("ingress port = %d, want 11211", np.Spec.Ingress[0].Ports[0].Port.IntValue())
	}

	// Verify owner reference.
	if len(np.OwnerReferences) != 1 {
		t.Fatalf("expected 1 owner reference, got %d", len(np.OwnerReferences))
	}
	if np.OwnerReferences[0].Name != mc.Name {
		t.Errorf("owner reference name = %q, want %q", np.OwnerReferences[0].Name, mc.Name)
	}
}

func TestReconcileNetworkPolicy_UpdatesExistingNetworkPolicy(t *testing.T) {
	mc := &memcachedv1beta1.Memcached{
		ObjectMeta: metav1.ObjectMeta{Name: testInstanceName, Namespace: testDefaultNamespace, UID: "uid-1"},
		Spec: memcachedv1beta1.MemcachedSpec{
			Monitoring: &memcachedv1beta1.MonitoringSpec{Enabled: true},
			Security: &memcachedv1beta1.SecuritySpec{
				NetworkPolicy: &memcachedv1beta1.NetworkPolicySpec{
					Enabled: true,
				},
			},
		},
	}
	// Existing policy with only memcached port.
	existingNP := &networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{Name: testInstanceName, Namespace: testDefaultNamespace},
		Spec: networkingv1.NetworkPolicySpec{
			Ingress: []networkingv1.NetworkPolicyIngressRule{
				{
					Ports: []networkingv1.NetworkPolicyPort{
						{Port: intstrPtr(intstr.FromInt32(11211))},
					},
				},
			},
		},
	}
	c := newFakeClient(mc, existingNP)
	r := newTestReconciler(c)

	if err := r.reconcileNetworkPolicy(context.Background(), mc); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	np := &networkingv1.NetworkPolicy{}
	if err := c.Get(context.Background(), client.ObjectKey{Name: testInstanceName, Namespace: testDefaultNamespace}, np); err != nil {
		t.Fatalf("failed to get network policy: %v", err)
	}

	// With monitoring enabled, should now have memcached + metrics ports.
	if len(np.Spec.Ingress) != 1 {
		t.Fatalf("expected 1 ingress rule, got %d", len(np.Spec.Ingress))
	}
	if len(np.Spec.Ingress[0].Ports) != 2 {
		t.Fatalf("expected 2 ports (memcached + metrics), got %d", len(np.Spec.Ingress[0].Ports))
	}
	gotPorts := make([]int, len(np.Spec.Ingress[0].Ports))
	for i, p := range np.Spec.Ingress[0].Ports {
		gotPorts[i] = p.Port.IntValue()
	}
	if gotPorts[0] != 11211 {
		t.Errorf("port[0] = %d, want 11211", gotPorts[0])
	}
	if gotPorts[1] != 9150 {
		t.Errorf("port[1] = %d, want 9150", gotPorts[1])
	}
}
