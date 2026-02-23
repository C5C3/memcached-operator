// Package v1beta1 contains unit tests for the Memcached CRD type definitions.
package v1beta1

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// newTestMemcached creates a minimal Memcached resource and applies functional options.
func newTestMemcached(opts ...func(*Memcached)) *Memcached {
	mc := &Memcached{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test",
			Namespace: "default",
		},
	}
	for _, opt := range opts {
		opt(mc)
	}
	return mc
}

func withSecurity() func(*Memcached) {
	return func(mc *Memcached) {
		if mc.Spec.Security == nil {
			mc.Spec.Security = &SecuritySpec{}
		}
	}
}

func withTLS(enabled bool) func(*Memcached) {
	return func(mc *Memcached) {
		withSecurity()(mc)
		mc.Spec.Security.TLS = &TLSSpec{Enabled: enabled}
	}
}

func withSASL(enabled bool) func(*Memcached) {
	return func(mc *Memcached) {
		withSecurity()(mc)
		mc.Spec.Security.SASL = &SASLSpec{Enabled: enabled}
	}
}

func withMonitoring(enabled bool) func(*Memcached) {
	return func(mc *Memcached) {
		mc.Spec.Monitoring = &MonitoringSpec{Enabled: enabled}
	}
}

func withServiceMonitor() func(*Memcached) {
	return func(mc *Memcached) {
		withMonitoring(true)(mc)
		mc.Spec.Monitoring.ServiceMonitor = &ServiceMonitorSpec{}
	}
}

func withAutoscaling(enabled bool) func(*Memcached) {
	return func(mc *Memcached) {
		mc.Spec.Autoscaling = &AutoscalingSpec{Enabled: enabled, MaxReplicas: 10}
	}
}

func withHighAvailability() func(*Memcached) {
	return func(mc *Memcached) {
		if mc.Spec.HighAvailability == nil {
			mc.Spec.HighAvailability = &HighAvailabilitySpec{}
		}
	}
}

func withPDB(enabled bool) func(*Memcached) {
	return func(mc *Memcached) {
		withHighAvailability()(mc)
		mc.Spec.HighAvailability.PodDisruptionBudget = &PDBSpec{Enabled: enabled}
	}
}

func withGracefulShutdown(enabled bool) func(*Memcached) {
	return func(mc *Memcached) {
		withHighAvailability()(mc)
		mc.Spec.HighAvailability.GracefulShutdown = &GracefulShutdownSpec{Enabled: enabled}
	}
}

func withNetworkPolicy(enabled bool) func(*Memcached) {
	return func(mc *Memcached) {
		withSecurity()(mc)
		mc.Spec.Security.NetworkPolicy = &NetworkPolicySpec{Enabled: enabled}
	}
}

func TestMemcached_IsTLSEnabled(t *testing.T) {
	tests := []struct {
		name string
		mc   *Memcached
		want bool
	}{
		{"nil Security", newTestMemcached(), false},
		{"nil TLS", newTestMemcached(withSecurity()), false},
		{"TLS disabled", newTestMemcached(withTLS(false)), false},
		{"TLS enabled", newTestMemcached(withTLS(true)), true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.mc.IsTLSEnabled(); got != tt.want {
				t.Errorf("IsTLSEnabled() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestMemcached_IsSASLEnabled(t *testing.T) {
	tests := []struct {
		name string
		mc   *Memcached
		want bool
	}{
		{"nil Security", newTestMemcached(), false},
		{"nil SASL", newTestMemcached(withSecurity()), false},
		{"SASL disabled", newTestMemcached(withSASL(false)), false},
		{"SASL enabled", newTestMemcached(withSASL(true)), true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.mc.IsSASLEnabled(); got != tt.want {
				t.Errorf("IsSASLEnabled() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestMemcached_IsMonitoringEnabled(t *testing.T) {
	tests := []struct {
		name string
		mc   *Memcached
		want bool
	}{
		{"nil Monitoring", newTestMemcached(), false},
		{"Monitoring disabled", newTestMemcached(withMonitoring(false)), false},
		{"Monitoring enabled", newTestMemcached(withMonitoring(true)), true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.mc.IsMonitoringEnabled(); got != tt.want {
				t.Errorf("IsMonitoringEnabled() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestMemcached_IsServiceMonitorEnabled(t *testing.T) {
	tests := []struct {
		name string
		mc   *Memcached
		want bool
	}{
		{"nil Monitoring", newTestMemcached(), false},
		{"Monitoring disabled", newTestMemcached(withMonitoring(false)), false},
		{"Monitoring enabled but nil ServiceMonitor", newTestMemcached(withMonitoring(true)), false},
		{"ServiceMonitor enabled", newTestMemcached(withServiceMonitor()), true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.mc.IsServiceMonitorEnabled(); got != tt.want {
				t.Errorf("IsServiceMonitorEnabled() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestMemcached_IsAutoscalingEnabled(t *testing.T) {
	tests := []struct {
		name string
		mc   *Memcached
		want bool
	}{
		{"nil Autoscaling", newTestMemcached(), false},
		{"Autoscaling disabled", newTestMemcached(withAutoscaling(false)), false},
		{"Autoscaling enabled", newTestMemcached(withAutoscaling(true)), true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.mc.IsAutoscalingEnabled(); got != tt.want {
				t.Errorf("IsAutoscalingEnabled() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestMemcached_IsPDBEnabled(t *testing.T) {
	tests := []struct {
		name string
		mc   *Memcached
		want bool
	}{
		{"nil HighAvailability", newTestMemcached(), false},
		{"nil PDB", newTestMemcached(withHighAvailability()), false},
		{"PDB disabled", newTestMemcached(withPDB(false)), false},
		{"PDB enabled", newTestMemcached(withPDB(true)), true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.mc.IsPDBEnabled(); got != tt.want {
				t.Errorf("IsPDBEnabled() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestMemcached_IsGracefulShutdownEnabled(t *testing.T) {
	tests := []struct {
		name string
		mc   *Memcached
		want bool
	}{
		{"nil HighAvailability", newTestMemcached(), false},
		{"nil GracefulShutdown", newTestMemcached(withHighAvailability()), false},
		{"GracefulShutdown disabled", newTestMemcached(withGracefulShutdown(false)), false},
		{"GracefulShutdown enabled", newTestMemcached(withGracefulShutdown(true)), true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.mc.IsGracefulShutdownEnabled(); got != tt.want {
				t.Errorf("IsGracefulShutdownEnabled() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestMemcached_IsNetworkPolicyEnabled(t *testing.T) {
	tests := []struct {
		name string
		mc   *Memcached
		want bool
	}{
		{"nil Security", newTestMemcached(), false},
		{"nil NetworkPolicy", newTestMemcached(withSecurity()), false},
		{"NetworkPolicy disabled", newTestMemcached(withNetworkPolicy(false)), false},
		{"NetworkPolicy enabled", newTestMemcached(withNetworkPolicy(true)), true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.mc.IsNetworkPolicyEnabled(); got != tt.want {
				t.Errorf("IsNetworkPolicyEnabled() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestMemcachedSpec_AllFieldsPresent(t *testing.T) {
	replicas := int32(3)
	img := "memcached:1.6.33"

	spec := MemcachedSpec{
		Replicas: &replicas,
		Image:    &img,
		Resources: &corev1.ResourceRequirements{
			Limits: corev1.ResourceList{
				corev1.ResourceMemory: resource.MustParse("256Mi"),
			},
		},
		Memcached: &MemcachedConfig{
			MaxMemoryMB: 128,
		},
		HighAvailability: &HighAvailabilitySpec{},
		Monitoring: &MonitoringSpec{
			Enabled: true,
		},
		Security: &SecuritySpec{},
		Autoscaling: &AutoscalingSpec{
			Enabled:     true,
			MaxReplicas: 10,
		},
		Service: &ServiceSpec{
			Annotations: map[string]string{"key": "value"},
		},
	}

	if *spec.Replicas != 3 {
		t.Errorf("expected Replicas=3, got %d", *spec.Replicas)
	}
	if *spec.Image != "memcached:1.6.33" {
		t.Errorf("unexpected Image: %s", *spec.Image)
	}
	if spec.Resources == nil {
		t.Fatal("expected Resources to be set")
	}
	if spec.Memcached == nil {
		t.Fatal("expected Memcached to be set")
	}
	if spec.Memcached.MaxMemoryMB != 128 {
		t.Errorf("expected MaxMemoryMB=128, got %d", spec.Memcached.MaxMemoryMB)
	}
	if spec.HighAvailability == nil {
		t.Fatal("expected HighAvailability to be set")
	}
	if !spec.Monitoring.Enabled {
		t.Error("expected Monitoring.Enabled to be true")
	}
	if spec.Security == nil {
		t.Fatal("expected Security to be set")
	}
	if !spec.Autoscaling.Enabled {
		t.Error("expected Autoscaling.Enabled to be true")
	}
	if spec.Autoscaling.MaxReplicas != 10 {
		t.Errorf("expected Autoscaling.MaxReplicas=10, got %d", spec.Autoscaling.MaxReplicas)
	}
	if spec.Service == nil {
		t.Fatal("expected Service to be set")
	}
	if spec.Service.Annotations["key"] != "value" {
		t.Errorf("unexpected Service annotation: %v", spec.Service.Annotations)
	}
}

func TestAntiAffinityPreset_ValidValues(t *testing.T) {
	tests := []struct {
		name  string
		value AntiAffinityPreset
	}{
		{"soft", AntiAffinityPresetSoft},
		{"hard", AntiAffinityPresetHard},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if string(tt.value) != tt.name {
				t.Errorf("expected %s, got %s", tt.name, tt.value)
			}
		})
	}
}

func TestMemcached_FullResource(t *testing.T) {
	replicas := int32(1)
	mc := Memcached{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "memcached.c5c3.io/v1beta1",
			Kind:       "Memcached",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-memcached",
			Namespace: "default",
		},
		Spec: MemcachedSpec{
			Replicas: &replicas,
		},
		Status: MemcachedStatus{
			Conditions: []metav1.Condition{
				{
					Type:   "Available",
					Status: metav1.ConditionTrue,
				},
			},
		},
	}

	if mc.Name != "test-memcached" {
		t.Errorf("unexpected Name: %s", mc.Name)
	}
	if mc.Kind != "Memcached" {
		t.Errorf("unexpected Kind: %s", mc.Kind)
	}
	if mc.APIVersion != "memcached.c5c3.io/v1beta1" {
		t.Errorf("unexpected APIVersion: %s", mc.APIVersion)
	}
	if *mc.Spec.Replicas != 1 {
		t.Errorf("expected Replicas=1, got %d", *mc.Spec.Replicas)
	}
	if len(mc.Status.Conditions) != 1 {
		t.Fatalf("expected 1 condition, got %d", len(mc.Status.Conditions))
	}
	if mc.Status.Conditions[0].Type != "Available" {
		t.Errorf("unexpected condition type: %s", mc.Status.Conditions[0].Type)
	}
}
