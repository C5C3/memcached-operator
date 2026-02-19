// Package controller implements the reconciliation logic for the memcached-operator.
package controller

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

	memcachedv1alpha1 "github.com/c5c3/memcached-operator/api/v1alpha1"
)

const testPortName = "memcached"

func TestConstructService_MinimalSpec(t *testing.T) {
	mc := &memcachedv1alpha1.Memcached{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-cache",
			Namespace: "default",
		},
		Spec: memcachedv1alpha1.MemcachedSpec{},
	}
	svc := &corev1.Service{}

	constructService(mc, svc)

	// ClusterIP must be None (headless).
	if svc.Spec.ClusterIP != corev1.ClusterIPNone {
		t.Errorf("expected clusterIP %q, got %q", corev1.ClusterIPNone, svc.Spec.ClusterIP)
	}

	// Exactly one port: 11211/TCP named "memcached".
	if len(svc.Spec.Ports) != 1 {
		t.Fatalf("expected 1 port, got %d", len(svc.Spec.Ports))
	}
	port := svc.Spec.Ports[0]
	if port.Name != testPortName {
		t.Errorf("expected port name %q, got %q", testPortName, port.Name)
	}
	if port.Port != 11211 {
		t.Errorf("expected port 11211, got %d", port.Port)
	}
	if port.TargetPort != intstr.FromString(testPortName) {
		t.Errorf("expected targetPort 'memcached', got %v", port.TargetPort)
	}
	if port.Protocol != corev1.ProtocolTCP {
		t.Errorf("expected protocol TCP, got %q", port.Protocol)
	}

	// Labels on metadata.
	expectedLabels := labelsForMemcached("my-cache")
	for k, v := range expectedLabels {
		if svc.Labels[k] != v {
			t.Errorf("label %q = %q, want %q", k, svc.Labels[k], v)
		}
	}

	// Selector matches labels.
	for k, v := range expectedLabels {
		if svc.Spec.Selector[k] != v {
			t.Errorf("selector %q = %q, want %q", k, svc.Spec.Selector[k], v)
		}
	}

	// No custom annotations on minimal spec.
	if len(svc.Annotations) != 0 {
		t.Errorf("expected no annotations, got %v", svc.Annotations)
	}
}

func TestConstructService_ClusterIPNone(t *testing.T) {
	mc := &memcachedv1alpha1.Memcached{
		ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"},
		Spec:       memcachedv1alpha1.MemcachedSpec{},
	}
	svc := &corev1.Service{}

	constructService(mc, svc)

	if svc.Spec.ClusterIP != corev1.ClusterIPNone {
		t.Errorf("expected clusterIP %q, got %q", corev1.ClusterIPNone, svc.Spec.ClusterIP)
	}
}

func TestConstructService_PortConfig(t *testing.T) {
	mc := &memcachedv1alpha1.Memcached{
		ObjectMeta: metav1.ObjectMeta{Name: "port-test", Namespace: "default"},
		Spec:       memcachedv1alpha1.MemcachedSpec{},
	}
	svc := &corev1.Service{}

	constructService(mc, svc)

	if len(svc.Spec.Ports) != 1 {
		t.Fatalf("expected 1 port, got %d", len(svc.Spec.Ports))
	}

	port := svc.Spec.Ports[0]
	if port.Name != testPortName {
		t.Errorf("port name = %q, want %q", port.Name, testPortName)
	}
	if port.Port != 11211 {
		t.Errorf("port = %d, want 11211", port.Port)
	}
	if port.TargetPort != intstr.FromString(testPortName) {
		t.Errorf("targetPort = %v, want 'memcached'", port.TargetPort)
	}
	if port.Protocol != corev1.ProtocolTCP {
		t.Errorf("protocol = %q, want TCP", port.Protocol)
	}
}

func TestConstructService_Labels(t *testing.T) {
	mc := &memcachedv1alpha1.Memcached{
		ObjectMeta: metav1.ObjectMeta{Name: "label-test", Namespace: "default"},
		Spec:       memcachedv1alpha1.MemcachedSpec{},
	}
	svc := &corev1.Service{}

	constructService(mc, svc)

	expectedLabels := labelsForMemcached("label-test")

	// Metadata labels.
	if len(svc.Labels) != len(expectedLabels) {
		t.Errorf("expected %d labels, got %d", len(expectedLabels), len(svc.Labels))
	}
	for k, v := range expectedLabels {
		if svc.Labels[k] != v {
			t.Errorf("label %q = %q, want %q", k, svc.Labels[k], v)
		}
	}

	// Selector labels.
	if len(svc.Spec.Selector) != len(expectedLabels) {
		t.Errorf("expected %d selector entries, got %d", len(expectedLabels), len(svc.Spec.Selector))
	}
	for k, v := range expectedLabels {
		if svc.Spec.Selector[k] != v {
			t.Errorf("selector %q = %q, want %q", k, svc.Spec.Selector[k], v)
		}
	}
}

func TestConstructService_CustomAnnotations(t *testing.T) {
	mc := &memcachedv1alpha1.Memcached{
		ObjectMeta: metav1.ObjectMeta{Name: "anno-test", Namespace: "default"},
		Spec: memcachedv1alpha1.MemcachedSpec{
			Service: &memcachedv1alpha1.ServiceSpec{
				Annotations: map[string]string{
					"prometheus.io/scrape": "true",
					"prometheus.io/port":   "9150",
				},
			},
		},
	}
	svc := &corev1.Service{}

	constructService(mc, svc)

	if len(svc.Annotations) != 2 {
		t.Fatalf("expected 2 annotations, got %d: %v", len(svc.Annotations), svc.Annotations)
	}
	if svc.Annotations["prometheus.io/scrape"] != "true" {
		t.Errorf("annotation prometheus.io/scrape = %q, want %q", svc.Annotations["prometheus.io/scrape"], "true")
	}
	if svc.Annotations["prometheus.io/port"] != "9150" {
		t.Errorf("annotation prometheus.io/port = %q, want %q", svc.Annotations["prometheus.io/port"], "9150")
	}
}

func TestConstructService_NilServiceSpec(t *testing.T) {
	mc := &memcachedv1alpha1.Memcached{
		ObjectMeta: metav1.ObjectMeta{Name: "nil-svc", Namespace: "default"},
		Spec:       memcachedv1alpha1.MemcachedSpec{Service: nil},
	}
	svc := &corev1.Service{}

	constructService(mc, svc)

	// Should not panic and should have no custom annotations.
	if len(svc.Annotations) != 0 {
		t.Errorf("expected no annotations, got %v", svc.Annotations)
	}

	// Still headless.
	if svc.Spec.ClusterIP != corev1.ClusterIPNone {
		t.Errorf("expected clusterIP %q, got %q", corev1.ClusterIPNone, svc.Spec.ClusterIP)
	}

	// Still has port.
	if len(svc.Spec.Ports) != 1 {
		t.Fatalf("expected 1 port, got %d", len(svc.Spec.Ports))
	}
}

func TestConstructService_MonitoringEnabled(t *testing.T) {
	mc := &memcachedv1alpha1.Memcached{
		ObjectMeta: metav1.ObjectMeta{Name: "mon-enabled", Namespace: "default"},
		Spec: memcachedv1alpha1.MemcachedSpec{
			Monitoring: &memcachedv1alpha1.MonitoringSpec{
				Enabled: true,
			},
		},
	}
	svc := &corev1.Service{}

	constructService(mc, svc)

	if len(svc.Spec.Ports) != 2 {
		t.Fatalf("expected 2 ports, got %d", len(svc.Spec.Ports))
	}

	if svc.Spec.Ports[0].Name != testPortName {
		t.Errorf("first port name = %q, want %q", svc.Spec.Ports[0].Name, testPortName)
	}
	if svc.Spec.Ports[0].Port != 11211 {
		t.Errorf("first port = %d, want 11211", svc.Spec.Ports[0].Port)
	}

	if svc.Spec.Ports[1].Name != "metrics" {
		t.Errorf("second port name = %q, want %q", svc.Spec.Ports[1].Name, "metrics")
	}
	if svc.Spec.Ports[1].Port != 9150 {
		t.Errorf("second port = %d, want 9150", svc.Spec.Ports[1].Port)
	}
}

func TestConstructService_MonitoringOff(t *testing.T) {
	tests := []struct {
		name       string
		monitoring *memcachedv1alpha1.MonitoringSpec
	}{
		{name: "monitoring disabled", monitoring: &memcachedv1alpha1.MonitoringSpec{Enabled: false}},
		{name: "nil monitoring", monitoring: nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mc := &memcachedv1alpha1.Memcached{
				ObjectMeta: metav1.ObjectMeta{Name: "mon-off", Namespace: "default"},
				Spec:       memcachedv1alpha1.MemcachedSpec{Monitoring: tt.monitoring},
			}
			svc := &corev1.Service{}

			constructService(mc, svc)

			if len(svc.Spec.Ports) != 1 {
				t.Fatalf("expected 1 port, got %d", len(svc.Spec.Ports))
			}
			if svc.Spec.Ports[0].Name != testPortName {
				t.Errorf("port name = %q, want %q", svc.Spec.Ports[0].Name, testPortName)
			}
			if svc.Spec.Ports[0].Port != 11211 {
				t.Errorf("port = %d, want 11211", svc.Spec.Ports[0].Port)
			}
		})
	}
}

func TestConstructService_MonitoringEnabledPortDetails(t *testing.T) {
	mc := &memcachedv1alpha1.Memcached{
		ObjectMeta: metav1.ObjectMeta{Name: "mon-details", Namespace: "default"},
		Spec: memcachedv1alpha1.MemcachedSpec{
			Monitoring: &memcachedv1alpha1.MonitoringSpec{
				Enabled: true,
			},
		},
	}
	svc := &corev1.Service{}

	constructService(mc, svc)

	if len(svc.Spec.Ports) < 2 {
		t.Fatalf("expected at least 2 ports, got %d", len(svc.Spec.Ports))
	}

	metricsPort := svc.Spec.Ports[1]

	if metricsPort.Name != "metrics" {
		t.Errorf("metrics port name = %q, want %q", metricsPort.Name, "metrics")
	}
	if metricsPort.Port != 9150 {
		t.Errorf("metrics port = %d, want 9150", metricsPort.Port)
	}
	if metricsPort.TargetPort != intstr.FromString("metrics") {
		t.Errorf("metrics targetPort = %v, want 'metrics'", metricsPort.TargetPort)
	}
	if metricsPort.Protocol != corev1.ProtocolTCP {
		t.Errorf("metrics protocol = %q, want TCP", metricsPort.Protocol)
	}
}
