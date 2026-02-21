// Package controller implements the reconciliation logic for the memcached-operator.
package controller

import (
	"context"
	"testing"

	monitoringv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	policyv1 "k8s.io/api/policy/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	memcachedv1alpha1 "github.com/c5c3/memcached-operator/api/v1alpha1"
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
	_ = memcachedv1alpha1.AddToScheme(s)
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

// --- reconcileDeployment ---

func TestReconcileDeployment_CreatesDeployment(t *testing.T) {
	mc := &memcachedv1alpha1.Memcached{
		ObjectMeta: metav1.ObjectMeta{Name: testInstanceName, Namespace: testDefaultNamespace, UID: "uid-1"},
		Spec:       memcachedv1alpha1.MemcachedSpec{},
	}
	c := newFakeClient(mc)
	r := newTestReconciler(c)

	if err := r.reconcileDeployment(context.Background(), mc); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	dep := &appsv1.Deployment{}
	if err := c.Get(context.Background(), client.ObjectKey{Name: testInstanceName, Namespace: testDefaultNamespace}, dep); err != nil {
		t.Fatalf("failed to get created deployment: %v", err)
	}

	// Verify name and namespace.
	if dep.Name != testInstanceName {
		t.Errorf("deployment name = %q, want %q", dep.Name, testInstanceName)
	}
	if dep.Namespace != testDefaultNamespace {
		t.Errorf("deployment namespace = %q, want %q", dep.Namespace, testDefaultNamespace)
	}

	// Verify labels.
	expectedLabels := labelsForMemcached(testInstanceName)
	for k, v := range expectedLabels {
		if dep.Labels[k] != v {
			t.Errorf("label %q = %q, want %q", k, dep.Labels[k], v)
		}
	}

	// Verify replicas default to 1.
	if dep.Spec.Replicas == nil || *dep.Spec.Replicas != 1 {
		t.Errorf("replicas = %v, want 1", dep.Spec.Replicas)
	}

	// Verify owner reference.
	if len(dep.OwnerReferences) != 1 {
		t.Fatalf("expected 1 owner reference, got %d", len(dep.OwnerReferences))
	}
	ref := dep.OwnerReferences[0]
	if ref.Name != mc.Name {
		t.Errorf("owner reference name = %q, want %q", ref.Name, mc.Name)
	}
	if ref.UID != mc.UID {
		t.Errorf("owner reference UID = %q, want %q", ref.UID, mc.UID)
	}
}

func TestReconcileDeployment_UpdatesExistingDeployment(t *testing.T) {
	replicas3 := int32(3)
	mc := &memcachedv1alpha1.Memcached{
		ObjectMeta: metav1.ObjectMeta{Name: testInstanceName, Namespace: testDefaultNamespace, UID: "uid-1"},
		Spec:       memcachedv1alpha1.MemcachedSpec{Replicas: &replicas3},
	}
	// Create an existing deployment with different replicas.
	replicas1 := int32(1)
	existingDep := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: testInstanceName, Namespace: testDefaultNamespace},
		Spec: appsv1.DeploymentSpec{
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
	c := newFakeClient(mc, existingDep)
	r := newTestReconciler(c)

	if err := r.reconcileDeployment(context.Background(), mc); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	dep := &appsv1.Deployment{}
	if err := c.Get(context.Background(), client.ObjectKey{Name: testInstanceName, Namespace: testDefaultNamespace}, dep); err != nil {
		t.Fatalf("failed to get deployment: %v", err)
	}

	// Verify replicas were updated.
	if dep.Spec.Replicas == nil || *dep.Spec.Replicas != 3 {
		t.Errorf("replicas = %v, want 3", dep.Spec.Replicas)
	}
}

func TestReconcileDeployment_SetsContainerImage(t *testing.T) {
	customImage := "memcached:1.7-alpine"
	mc := &memcachedv1alpha1.Memcached{
		ObjectMeta: metav1.ObjectMeta{Name: "img-mc", Namespace: testDefaultNamespace, UID: "uid-2"},
		Spec:       memcachedv1alpha1.MemcachedSpec{Image: &customImage},
	}
	c := newFakeClient(mc)
	r := newTestReconciler(c)

	if err := r.reconcileDeployment(context.Background(), mc); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	dep := &appsv1.Deployment{}
	if err := c.Get(context.Background(), client.ObjectKey{Name: "img-mc", Namespace: testDefaultNamespace}, dep); err != nil {
		t.Fatalf("failed to get deployment: %v", err)
	}

	if len(dep.Spec.Template.Spec.Containers) == 0 {
		t.Fatal("expected at least 1 container")
	}
	if dep.Spec.Template.Spec.Containers[0].Image != customImage {
		t.Errorf("container image = %q, want %q", dep.Spec.Template.Spec.Containers[0].Image, customImage)
	}
}

// --- reconcileService ---

func TestReconcileService_CreatesService(t *testing.T) {
	mc := &memcachedv1alpha1.Memcached{
		ObjectMeta: metav1.ObjectMeta{Name: testInstanceName, Namespace: testDefaultNamespace, UID: "uid-1"},
		Spec:       memcachedv1alpha1.MemcachedSpec{},
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
	mc := &memcachedv1alpha1.Memcached{
		ObjectMeta: metav1.ObjectMeta{Name: testInstanceName, Namespace: testDefaultNamespace, UID: "uid-1"},
		Spec:       memcachedv1alpha1.MemcachedSpec{},
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
	mc := &memcachedv1alpha1.Memcached{
		ObjectMeta: metav1.ObjectMeta{Name: testInstanceName, Namespace: testDefaultNamespace, UID: "uid-1"},
		Spec:       memcachedv1alpha1.MemcachedSpec{},
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
	mc := &memcachedv1alpha1.Memcached{
		ObjectMeta: metav1.ObjectMeta{Name: testInstanceName, Namespace: testDefaultNamespace, UID: "uid-1"},
		Spec: memcachedv1alpha1.MemcachedSpec{
			HighAvailability: &memcachedv1alpha1.HighAvailabilitySpec{
				PodDisruptionBudget: &memcachedv1alpha1.PDBSpec{
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
	mc := &memcachedv1alpha1.Memcached{
		ObjectMeta: metav1.ObjectMeta{Name: testInstanceName, Namespace: testDefaultNamespace, UID: "uid-1"},
		Spec: memcachedv1alpha1.MemcachedSpec{
			HighAvailability: &memcachedv1alpha1.HighAvailabilitySpec{
				PodDisruptionBudget: &memcachedv1alpha1.PDBSpec{
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
	mc := &memcachedv1alpha1.Memcached{
		ObjectMeta: metav1.ObjectMeta{Name: testInstanceName, Namespace: testDefaultNamespace, UID: "uid-1"},
		Spec:       memcachedv1alpha1.MemcachedSpec{},
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
	mc := &memcachedv1alpha1.Memcached{
		ObjectMeta: metav1.ObjectMeta{Name: testInstanceName, Namespace: testDefaultNamespace, UID: "uid-1"},
		Spec: memcachedv1alpha1.MemcachedSpec{
			Monitoring: &memcachedv1alpha1.MonitoringSpec{
				Enabled: true,
				ServiceMonitor: &memcachedv1alpha1.ServiceMonitorSpec{
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
	mc := &memcachedv1alpha1.Memcached{
		ObjectMeta: metav1.ObjectMeta{Name: testInstanceName, Namespace: testDefaultNamespace, UID: "uid-1"},
		Spec: memcachedv1alpha1.MemcachedSpec{
			Monitoring: &memcachedv1alpha1.MonitoringSpec{
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
	mc := &memcachedv1alpha1.Memcached{
		ObjectMeta: metav1.ObjectMeta{Name: testInstanceName, Namespace: testDefaultNamespace, UID: "uid-1"},
		Spec:       memcachedv1alpha1.MemcachedSpec{},
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
	mc := &memcachedv1alpha1.Memcached{
		ObjectMeta: metav1.ObjectMeta{Name: testInstanceName, Namespace: testDefaultNamespace, UID: "uid-1"},
		Spec: memcachedv1alpha1.MemcachedSpec{
			Security: &memcachedv1alpha1.SecuritySpec{
				NetworkPolicy: &memcachedv1alpha1.NetworkPolicySpec{
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
	mc := &memcachedv1alpha1.Memcached{
		ObjectMeta: metav1.ObjectMeta{Name: testInstanceName, Namespace: testDefaultNamespace, UID: "uid-1"},
		Spec: memcachedv1alpha1.MemcachedSpec{
			Monitoring: &memcachedv1alpha1.MonitoringSpec{Enabled: true},
			Security: &memcachedv1alpha1.SecuritySpec{
				NetworkPolicy: &memcachedv1alpha1.NetworkPolicySpec{
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
