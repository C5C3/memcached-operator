package v1alpha1

import (
	"reflect"
	"testing"

	autoscalingv2 "k8s.io/api/autoscaling/v2"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/conversion"

	v1beta1 "github.com/c5c3/memcached-operator/api/v1beta1"
)

func TestConvertible_InterfaceSatisfied(t *testing.T) {
	var _ conversion.Convertible = &Memcached{}
}

// fullyPopulated returns a Memcached with every field set to exercise round-trip fidelity.
func fullyPopulated() *Memcached {
	replicas := int32(5)
	image := "memcached:1.6.28"
	antiAffinity := AntiAffinityPresetHard
	minAvail := intstr.FromString("50%")
	maxUnavail := intstr.FromInt32(1)
	minReplicas := int32(2)
	exporterImage := "prom/memcached-exporter:v0.15.4"
	runAsNonRoot := true

	return &Memcached{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "memcached.c5c3.io/v1alpha1",
			Kind:       "Memcached",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:            "full-mc",
			Namespace:       "prod",
			Labels:          map[string]string{"app": "memcached", "env": "prod"},
			Annotations:     map[string]string{"note": "conversion-test"},
			ResourceVersion: "12345",
		},
		Spec: MemcachedSpec{
			Replicas: &replicas,
			Image:    &image,
			Resources: &corev1.ResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("100m"),
					corev1.ResourceMemory: resource.MustParse("128Mi"),
				},
				Limits: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("500m"),
					corev1.ResourceMemory: resource.MustParse("256Mi"),
				},
			},
			Memcached: &MemcachedConfig{
				MaxMemoryMB:    128,
				MaxConnections: 2048,
				Threads:        8,
				MaxItemSize:    "2m",
				Verbosity:      1,
				ExtraArgs:      []string{"-o", "modern", "-B", "binary"},
			},
			HighAvailability: &HighAvailabilitySpec{
				AntiAffinityPreset: &antiAffinity,
				TopologySpreadConstraints: []corev1.TopologySpreadConstraint{
					{
						MaxSkew:           1,
						TopologyKey:       "topology.kubernetes.io/zone",
						WhenUnsatisfiable: corev1.DoNotSchedule,
					},
				},
				PodDisruptionBudget: &PDBSpec{
					Enabled:        true,
					MinAvailable:   &minAvail,
					MaxUnavailable: &maxUnavail,
				},
				GracefulShutdown: &GracefulShutdownSpec{
					Enabled:                       true,
					PreStopDelaySeconds:           15,
					TerminationGracePeriodSeconds: 60,
				},
			},
			Monitoring: &MonitoringSpec{
				Enabled:       true,
				ExporterImage: &exporterImage,
				ExporterResources: &corev1.ResourceRequirements{
					Requests: corev1.ResourceList{
						corev1.ResourceCPU: resource.MustParse("50m"),
					},
				},
				ServiceMonitor: &ServiceMonitorSpec{
					AdditionalLabels: map[string]string{"team": "platform"},
					Interval:         v1beta1.DefaultServiceMonitorInterval,
					ScrapeTimeout:    "10s",
				},
			},
			Security: &SecuritySpec{
				PodSecurityContext: &corev1.PodSecurityContext{
					RunAsNonRoot: &runAsNonRoot,
				},
				ContainerSecurityContext: &corev1.SecurityContext{
					RunAsNonRoot: &runAsNonRoot,
				},
				SASL: &SASLSpec{
					Enabled:              true,
					CredentialsSecretRef: corev1.LocalObjectReference{Name: "sasl-secret"},
				},
				TLS: &TLSSpec{
					Enabled:              true,
					CertificateSecretRef: corev1.LocalObjectReference{Name: "tls-secret"},
					EnableClientCert:     true,
				},
				NetworkPolicy: &NetworkPolicySpec{
					Enabled: true,
					AllowedSources: []networkingv1.NetworkPolicyPeer{
						{
							PodSelector: &metav1.LabelSelector{
								MatchLabels: map[string]string{"app": "client"},
							},
						},
					},
				},
			},
			Autoscaling: &AutoscalingSpec{
				Enabled:     true,
				MinReplicas: &minReplicas,
				MaxReplicas: 10,
				Metrics: []autoscalingv2.MetricSpec{
					{
						Type: autoscalingv2.ResourceMetricSourceType,
						Resource: &autoscalingv2.ResourceMetricSource{
							Name: corev1.ResourceCPU,
							Target: autoscalingv2.MetricTarget{
								Type:               autoscalingv2.UtilizationMetricType,
								AverageUtilization: int32Ptr(80),
							},
						},
					},
				},
			},
			Service: &ServiceSpec{
				Annotations: map[string]string{"svc-key": "svc-val"},
			},
		},
		Status: MemcachedStatus{
			Conditions: []metav1.Condition{
				{
					Type:               "Available",
					Status:             metav1.ConditionTrue,
					Reason:             "Ready",
					Message:            "All replicas ready",
					LastTransitionTime: metav1.Now(),
				},
			},
			ReadyReplicas:      5,
			ObservedGeneration: 42,
		},
	}
}

func int32Ptr(v int32) *int32 { return &v }

func TestConvertTo_FullyPopulatedObject(t *testing.T) {
	src := fullyPopulated()
	dst := &v1beta1.Memcached{}

	if err := src.ConvertTo(dst); err != nil {
		t.Fatalf("ConvertTo failed: %v", err)
	}

	// ObjectMeta
	if dst.Name != src.Name {
		t.Errorf("Name: got %q, want %q", dst.Name, src.Name)
	}
	if dst.Namespace != src.Namespace {
		t.Errorf("Namespace: got %q, want %q", dst.Namespace, src.Namespace)
	}

	// Spec scalar fields
	if *dst.Spec.Replicas != *src.Spec.Replicas {
		t.Errorf("Replicas: got %d, want %d", *dst.Spec.Replicas, *src.Spec.Replicas)
	}
	if *dst.Spec.Image != *src.Spec.Image {
		t.Errorf("Image: got %q, want %q", *dst.Spec.Image, *src.Spec.Image)
	}

	// Deeply nested: MemcachedConfig
	if dst.Spec.Memcached.MaxMemoryMB != src.Spec.Memcached.MaxMemoryMB {
		t.Errorf("MaxMemoryMB: got %d, want %d", dst.Spec.Memcached.MaxMemoryMB, src.Spec.Memcached.MaxMemoryMB)
	}
	if len(dst.Spec.Memcached.ExtraArgs) != len(src.Spec.Memcached.ExtraArgs) {
		t.Errorf("ExtraArgs length: got %d, want %d", len(dst.Spec.Memcached.ExtraArgs), len(src.Spec.Memcached.ExtraArgs))
	}

	// HighAvailability
	if dst.Spec.HighAvailability == nil {
		t.Fatal("HighAvailability is nil after ConvertTo")
	}
	if string(*dst.Spec.HighAvailability.AntiAffinityPreset) != string(*src.Spec.HighAvailability.AntiAffinityPreset) {
		t.Error("AntiAffinityPreset mismatch")
	}
	if !dst.Spec.HighAvailability.PodDisruptionBudget.Enabled {
		t.Error("PDB.Enabled should be true")
	}
	if dst.Spec.HighAvailability.GracefulShutdown.PreStopDelaySeconds != 15 {
		t.Error("GracefulShutdown.PreStopDelaySeconds mismatch")
	}

	// Security nested
	if dst.Spec.Security == nil {
		t.Fatal("Security is nil after ConvertTo")
	}
	if !dst.Spec.Security.SASL.Enabled {
		t.Error("SASL.Enabled should be true")
	}
	if !dst.Spec.Security.TLS.EnableClientCert {
		t.Error("TLS.EnableClientCert should be true")
	}
	if !dst.Spec.Security.NetworkPolicy.Enabled {
		t.Error("NetworkPolicy.Enabled should be true")
	}

	// Monitoring
	if dst.Spec.Monitoring == nil {
		t.Fatal("Monitoring is nil after ConvertTo")
	}
	if dst.Spec.Monitoring.ServiceMonitor.Interval != v1beta1.DefaultServiceMonitorInterval {
		t.Error("ServiceMonitor.Interval mismatch")
	}

	// Autoscaling
	if dst.Spec.Autoscaling == nil {
		t.Fatal("Autoscaling is nil after ConvertTo")
	}
	if dst.Spec.Autoscaling.MaxReplicas != 10 {
		t.Error("Autoscaling.MaxReplicas mismatch")
	}

	// Status
	if dst.Status.ReadyReplicas != src.Status.ReadyReplicas {
		t.Errorf("ReadyReplicas: got %d, want %d", dst.Status.ReadyReplicas, src.Status.ReadyReplicas)
	}
	if dst.Status.ObservedGeneration != src.Status.ObservedGeneration {
		t.Errorf("ObservedGeneration: got %d, want %d", dst.Status.ObservedGeneration, src.Status.ObservedGeneration)
	}
}

func TestConvertFrom_FullyPopulatedObject(t *testing.T) {
	// Start with v1alpha1, convert to v1beta1, then convert back.
	original := fullyPopulated()
	hub := &v1beta1.Memcached{}
	if err := original.ConvertTo(hub); err != nil {
		t.Fatalf("ConvertTo failed: %v", err)
	}

	dst := &Memcached{}
	if err := dst.ConvertFrom(hub); err != nil {
		t.Fatalf("ConvertFrom failed: %v", err)
	}

	// Spot-check key fields from the hub.
	if dst.Name != original.Name {
		t.Errorf("Name: got %q, want %q", dst.Name, original.Name)
	}
	if *dst.Spec.Replicas != *original.Spec.Replicas {
		t.Errorf("Replicas mismatch")
	}
	if dst.Spec.Memcached.MaxMemoryMB != original.Spec.Memcached.MaxMemoryMB {
		t.Errorf("MaxMemoryMB mismatch")
	}
	if dst.Spec.Security.TLS.EnableClientCert != original.Spec.Security.TLS.EnableClientCert {
		t.Error("TLS.EnableClientCert mismatch")
	}
	if dst.Status.ObservedGeneration != original.Status.ObservedGeneration {
		t.Error("ObservedGeneration mismatch")
	}
}

func TestConvertTo_WrongHubType_ReturnsError(t *testing.T) {
	src := &Memcached{}
	wrongHub := &fakeHub{}

	err := src.ConvertTo(wrongHub)
	if err == nil {
		t.Error("expected error with wrong hub type")
	}
}

func TestConvertFrom_WrongHubType_ReturnsError(t *testing.T) {
	dst := &Memcached{}
	wrongHub := &fakeHub{}

	err := dst.ConvertFrom(wrongHub)
	if err == nil {
		t.Error("expected error with wrong hub type")
	}
}

// fakeHub satisfies conversion.Hub but is not *v1beta1.Memcached.
type fakeHub struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
}

func (*fakeHub) Hub()                             {}
func (f *fakeHub) DeepCopyObject() runtime.Object { return f }

var _ conversion.Hub = &fakeHub{}

func TestRoundTrip_FullyPopulated_NoDataLoss(t *testing.T) {
	original := fullyPopulated()
	hub := &v1beta1.Memcached{}

	if err := original.ConvertTo(hub); err != nil {
		t.Fatalf("ConvertTo failed: %v", err)
	}

	roundTripped := &Memcached{}
	if err := roundTripped.ConvertFrom(hub); err != nil {
		t.Fatalf("ConvertFrom failed: %v", err)
	}

	if !reflect.DeepEqual(original.Spec, roundTripped.Spec) {
		t.Errorf("Spec not equal after round trip.\nOriginal: %+v\nRoundTripped: %+v", original.Spec, roundTripped.Spec)
	}
	if !reflect.DeepEqual(original.Status, roundTripped.Status) {
		t.Errorf("Status not equal after round trip.\nOriginal: %+v\nRoundTripped: %+v", original.Status, roundTripped.Status)
	}
}

func TestRoundTrip_MinimalObject_PreservesNils(t *testing.T) {
	original := &Memcached{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "minimal",
			Namespace: "default",
		},
	}

	hub := &v1beta1.Memcached{}
	if err := original.ConvertTo(hub); err != nil {
		t.Fatalf("ConvertTo failed: %v", err)
	}

	roundTripped := &Memcached{}
	if err := roundTripped.ConvertFrom(hub); err != nil {
		t.Fatalf("ConvertFrom failed: %v", err)
	}

	// All optional pointer fields should remain nil.
	if roundTripped.Spec.Replicas != nil {
		t.Error("Replicas should be nil")
	}
	if roundTripped.Spec.Image != nil {
		t.Error("Image should be nil")
	}
	if roundTripped.Spec.Memcached != nil {
		t.Error("Memcached should be nil")
	}
	if roundTripped.Spec.HighAvailability != nil {
		t.Error("HighAvailability should be nil")
	}
	if roundTripped.Spec.Monitoring != nil {
		t.Error("Monitoring should be nil")
	}
	if roundTripped.Spec.Security != nil {
		t.Error("Security should be nil")
	}
	if roundTripped.Spec.Autoscaling != nil {
		t.Error("Autoscaling should be nil")
	}
	if roundTripped.Spec.Service != nil {
		t.Error("Service should be nil")
	}
}

func TestRoundTrip_ObjectMeta_Preserved(t *testing.T) {
	original := &Memcached{
		ObjectMeta: metav1.ObjectMeta{
			Name:            "meta-test",
			Namespace:       "ns",
			Labels:          map[string]string{"l1": "v1", "l2": "v2"},
			Annotations:     map[string]string{"a1": "av1"},
			ResourceVersion: "999",
		},
	}

	hub := &v1beta1.Memcached{}
	if err := original.ConvertTo(hub); err != nil {
		t.Fatalf("ConvertTo failed: %v", err)
	}

	roundTripped := &Memcached{}
	if err := roundTripped.ConvertFrom(hub); err != nil {
		t.Fatalf("ConvertFrom failed: %v", err)
	}

	if roundTripped.Name != original.Name {
		t.Errorf("Name: got %q, want %q", roundTripped.Name, original.Name)
	}
	if roundTripped.Namespace != original.Namespace {
		t.Errorf("Namespace: got %q, want %q", roundTripped.Namespace, original.Namespace)
	}
	if !reflect.DeepEqual(roundTripped.Labels, original.Labels) {
		t.Error("Labels mismatch after round trip")
	}
	if !reflect.DeepEqual(roundTripped.Annotations, original.Annotations) {
		t.Error("Annotations mismatch after round trip")
	}
	if roundTripped.ResourceVersion != original.ResourceVersion {
		t.Errorf("ResourceVersion: got %q, want %q", roundTripped.ResourceVersion, original.ResourceVersion)
	}
}

func TestConvertTo_MinimalObject(t *testing.T) {
	src := &Memcached{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "minimal",
			Namespace: "default",
		},
	}
	dst := &v1beta1.Memcached{}

	if err := src.ConvertTo(dst); err != nil {
		t.Fatalf("ConvertTo minimal object failed: %v", err)
	}

	if dst.Name != "minimal" {
		t.Errorf("Name: got %q, want %q", dst.Name, "minimal")
	}
}
