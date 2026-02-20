// Package controller implements the reconciliation logic for the memcached-operator.
package controller

import (
	"reflect"
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

	if svc.Spec.Ports[1].Name != testMetricsPort {
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

	if metricsPort.Name != testMetricsPort {
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

func TestConstructService_TLSEnabled(t *testing.T) {
	mc := &memcachedv1alpha1.Memcached{
		ObjectMeta: metav1.ObjectMeta{Name: "tls-svc", Namespace: "default"},
		Spec: memcachedv1alpha1.MemcachedSpec{
			Security: &memcachedv1alpha1.SecuritySpec{
				TLS: &memcachedv1alpha1.TLSSpec{
					Enabled: true,
					CertificateSecretRef: corev1.LocalObjectReference{
						Name: "my-tls-secret",
					},
				},
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

	if svc.Spec.Ports[1].Name != tlsPortName {
		t.Errorf("second port name = %q, want %q", svc.Spec.Ports[1].Name, tlsPortName)
	}
	if svc.Spec.Ports[1].Port != 11212 {
		t.Errorf("second port = %d, want 11212", svc.Spec.Ports[1].Port)
	}
	if svc.Spec.Ports[1].TargetPort != intstr.FromString(tlsPortName) {
		t.Errorf("second targetPort = %v, want %q", svc.Spec.Ports[1].TargetPort, tlsPortName)
	}
	if svc.Spec.Ports[1].Protocol != corev1.ProtocolTCP {
		t.Errorf("second protocol = %q, want TCP", svc.Spec.Ports[1].Protocol)
	}
}

func TestConstructService_TLSDisabled(t *testing.T) {
	tests := []struct {
		name     string
		security *memcachedv1alpha1.SecuritySpec
	}{
		{name: "nil Security", security: nil},
		{name: "nil TLS", security: &memcachedv1alpha1.SecuritySpec{}},
		{
			name: "TLS disabled",
			security: &memcachedv1alpha1.SecuritySpec{
				TLS: &memcachedv1alpha1.TLSSpec{Enabled: false},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mc := &memcachedv1alpha1.Memcached{
				ObjectMeta: metav1.ObjectMeta{Name: "tls-off", Namespace: "default"},
				Spec:       memcachedv1alpha1.MemcachedSpec{Security: tt.security},
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

func TestConstructService_TLSWithMonitoring(t *testing.T) {
	mc := &memcachedv1alpha1.Memcached{
		ObjectMeta: metav1.ObjectMeta{Name: "tls-mon", Namespace: "default"},
		Spec: memcachedv1alpha1.MemcachedSpec{
			Security: &memcachedv1alpha1.SecuritySpec{
				TLS: &memcachedv1alpha1.TLSSpec{
					Enabled: true,
					CertificateSecretRef: corev1.LocalObjectReference{
						Name: "my-tls-secret",
					},
				},
			},
			Monitoring: &memcachedv1alpha1.MonitoringSpec{
				Enabled: true,
			},
		},
	}
	svc := &corev1.Service{}

	constructService(mc, svc)

	if len(svc.Spec.Ports) != 3 {
		t.Fatalf("expected 3 ports, got %d", len(svc.Spec.Ports))
	}

	// Port 11211 (memcached).
	if svc.Spec.Ports[0].Name != testPortName {
		t.Errorf("port[0] name = %q, want %q", svc.Spec.Ports[0].Name, testPortName)
	}
	if svc.Spec.Ports[0].Port != 11211 {
		t.Errorf("port[0] = %d, want 11211", svc.Spec.Ports[0].Port)
	}

	// Port 11212 (memcached-tls).
	if svc.Spec.Ports[1].Name != tlsPortName {
		t.Errorf("port[1] name = %q, want %q", svc.Spec.Ports[1].Name, tlsPortName)
	}
	if svc.Spec.Ports[1].Port != 11212 {
		t.Errorf("port[1] = %d, want 11212", svc.Spec.Ports[1].Port)
	}

	// Port 9150 (metrics).
	if svc.Spec.Ports[2].Name != testMetricsPort {
		t.Errorf("port[2] name = %q, want %q", svc.Spec.Ports[2].Name, "metrics")
	}
	if svc.Spec.Ports[2].Port != 9150 {
		t.Errorf("port[2] = %d, want 9150", svc.Spec.Ports[2].Port)
	}
}

func TestConstructService_TLSNilSecurity(t *testing.T) {
	mc := &memcachedv1alpha1.Memcached{
		ObjectMeta: metav1.ObjectMeta{Name: "tls-nil", Namespace: "default"},
		Spec:       memcachedv1alpha1.MemcachedSpec{Security: nil},
	}
	svc := &corev1.Service{}

	constructService(mc, svc)

	// Should not panic and only have memcached port.
	if len(svc.Spec.Ports) != 1 {
		t.Fatalf("expected 1 port, got %d", len(svc.Spec.Ports))
	}
	if svc.Spec.Ports[0].Port != 11211 {
		t.Errorf("port = %d, want 11211", svc.Spec.Ports[0].Port)
	}
}

func TestConstructService_AnnotationClearing(t *testing.T) {
	tests := []struct {
		name       string
		secondSpec memcachedv1alpha1.MemcachedSpec
	}{
		{
			name:       "Service field set to nil clears annotations",
			secondSpec: memcachedv1alpha1.MemcachedSpec{Service: nil},
		},
		{
			name: "empty annotations map clears annotations",
			secondSpec: memcachedv1alpha1.MemcachedSpec{
				Service: &memcachedv1alpha1.ServiceSpec{
					Annotations: map[string]string{},
				},
			},
		},
		{
			name: "nil annotations map clears annotations",
			secondSpec: memcachedv1alpha1.MemcachedSpec{
				Service: &memcachedv1alpha1.ServiceSpec{
					Annotations: nil,
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Step 1: Create a CR with annotations and apply to Service.
			mc := &memcachedv1alpha1.Memcached{
				ObjectMeta: metav1.ObjectMeta{Name: "anno-clear", Namespace: "default"},
				Spec: memcachedv1alpha1.MemcachedSpec{
					Service: &memcachedv1alpha1.ServiceSpec{
						Annotations: map[string]string{
							"prometheus.io/scrape": "true",
							"custom/key":           "value",
						},
					},
				},
			}
			svc := &corev1.Service{}

			constructService(mc, svc)

			// Verify annotations are set after first call.
			if len(svc.Annotations) != 2 {
				t.Fatalf("after first call: expected 2 annotations, got %d: %v", len(svc.Annotations), svc.Annotations)
			}
			if svc.Annotations["prometheus.io/scrape"] != "true" {
				t.Errorf("after first call: annotation prometheus.io/scrape = %q, want %q", svc.Annotations["prometheus.io/scrape"], "true")
			}
			if svc.Annotations["custom/key"] != "value" {
				t.Errorf("after first call: annotation custom/key = %q, want %q", svc.Annotations["custom/key"], "value")
			}

			// Step 2: Change the CR to have no annotations and re-apply on the same Service.
			mc.Spec = tt.secondSpec

			constructService(mc, svc)

			// Verify annotations are cleared to nil (not just empty map).
			if svc.Annotations != nil {
				t.Errorf("after second call: expected nil annotations, got %v", svc.Annotations)
			}

			// Verify other fields are still correctly set.
			if svc.Spec.ClusterIP != corev1.ClusterIPNone {
				t.Errorf("after second call: expected clusterIP %q, got %q", corev1.ClusterIPNone, svc.Spec.ClusterIP)
			}
			if len(svc.Spec.Ports) != 1 {
				t.Fatalf("after second call: expected 1 port, got %d", len(svc.Spec.Ports))
			}
			if svc.Spec.Ports[0].Name != testPortName {
				t.Errorf("after second call: port name = %q, want %q", svc.Spec.Ports[0].Name, testPortName)
			}
		})
	}
}

func TestConstructService_Idempotent(t *testing.T) {
	mc := &memcachedv1alpha1.Memcached{
		ObjectMeta: metav1.ObjectMeta{Name: "idempotent-test", Namespace: "default"},
		Spec: memcachedv1alpha1.MemcachedSpec{
			Service: &memcachedv1alpha1.ServiceSpec{
				Annotations: map[string]string{
					"example.com/key": "val",
				},
			},
		},
	}
	svc := &corev1.Service{}

	// First call.
	constructService(mc, svc)

	// Snapshot all relevant fields after the first call.
	firstClusterIP := svc.Spec.ClusterIP
	firstPorts := make([]corev1.ServicePort, len(svc.Spec.Ports))
	copy(firstPorts, svc.Spec.Ports)
	firstLabels := make(map[string]string, len(svc.Labels))
	for k, v := range svc.Labels {
		firstLabels[k] = v
	}
	firstSelector := make(map[string]string, len(svc.Spec.Selector))
	for k, v := range svc.Spec.Selector {
		firstSelector[k] = v
	}
	firstAnnotations := make(map[string]string, len(svc.Annotations))
	for k, v := range svc.Annotations {
		firstAnnotations[k] = v
	}

	// Second call with the same CR on the same Service object.
	constructService(mc, svc)

	// Verify ClusterIP unchanged.
	if svc.Spec.ClusterIP != firstClusterIP {
		t.Errorf("ClusterIP changed: got %q, want %q", svc.Spec.ClusterIP, firstClusterIP)
	}

	// Verify Ports unchanged.
	if len(svc.Spec.Ports) != len(firstPorts) {
		t.Fatalf("port count changed: got %d, want %d", len(svc.Spec.Ports), len(firstPorts))
	}
	for i, p := range svc.Spec.Ports {
		if p.Name != firstPorts[i].Name {
			t.Errorf("port[%d].Name changed: got %q, want %q", i, p.Name, firstPorts[i].Name)
		}
		if p.Port != firstPorts[i].Port {
			t.Errorf("port[%d].Port changed: got %d, want %d", i, p.Port, firstPorts[i].Port)
		}
		if p.TargetPort != firstPorts[i].TargetPort {
			t.Errorf("port[%d].TargetPort changed: got %v, want %v", i, p.TargetPort, firstPorts[i].TargetPort)
		}
		if p.Protocol != firstPorts[i].Protocol {
			t.Errorf("port[%d].Protocol changed: got %q, want %q", i, p.Protocol, firstPorts[i].Protocol)
		}
	}

	// Verify Labels unchanged.
	if !reflect.DeepEqual(svc.Labels, firstLabels) {
		t.Errorf("Labels changed: got %v, want %v", svc.Labels, firstLabels)
	}

	// Verify Selector unchanged.
	if !reflect.DeepEqual(svc.Spec.Selector, firstSelector) {
		t.Errorf("Selector changed: got %v, want %v", svc.Spec.Selector, firstSelector)
	}

	// Verify Annotations unchanged.
	if !reflect.DeepEqual(svc.Annotations, firstAnnotations) {
		t.Errorf("Annotations changed: got %v, want %v", svc.Annotations, firstAnnotations)
	}
}
