// Package v1alpha1 contains unit tests for the Memcached CRD type definitions.
package v1alpha1

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

// --- REQ-001: MemcachedConfig ---

func TestMemcachedConfig_ZeroValue(t *testing.T) {
	cfg := MemcachedConfig{}
	if cfg.MaxMemoryMB != 0 {
		t.Error("expected MaxMemoryMB to be 0 for zero value")
	}
	if cfg.MaxConnections != 0 {
		t.Error("expected MaxConnections to be 0 for zero value")
	}
	if cfg.Threads != 0 {
		t.Error("expected Threads to be 0 for zero value")
	}
	if cfg.MaxItemSize != "" {
		t.Error("expected MaxItemSize to be empty for zero value")
	}
	if cfg.Verbosity != 0 {
		t.Error("expected Verbosity to be 0 for zero value")
	}
	if cfg.ExtraArgs != nil {
		t.Error("expected ExtraArgs to be nil for zero value")
	}
}

func TestMemcachedConfig_AllFieldsSet(t *testing.T) {
	cfg := MemcachedConfig{
		MaxMemoryMB:    256,
		MaxConnections: 2048,
		Threads:        8,
		MaxItemSize:    "2m",
		Verbosity:      2,
		ExtraArgs:      []string{"-o", "modern"},
	}

	if cfg.MaxMemoryMB != 256 {
		t.Errorf("expected MaxMemoryMB=256, got %d", cfg.MaxMemoryMB)
	}
	if cfg.MaxConnections != 2048 {
		t.Errorf("expected MaxConnections=2048, got %d", cfg.MaxConnections)
	}
	if cfg.Threads != 8 {
		t.Errorf("expected Threads=8, got %d", cfg.Threads)
	}
	if cfg.MaxItemSize != "2m" {
		t.Errorf("expected MaxItemSize=2m, got %s", cfg.MaxItemSize)
	}
	if cfg.Verbosity != 2 {
		t.Errorf("expected Verbosity=2, got %d", cfg.Verbosity)
	}
	if len(cfg.ExtraArgs) != 2 || cfg.ExtraArgs[0] != "-o" {
		t.Errorf("expected ExtraArgs=[-o modern], got %v", cfg.ExtraArgs)
	}
}

// --- REQ-002: HighAvailabilitySpec + PDBSpec ---

func TestHighAvailabilitySpec_ZeroValue(t *testing.T) {
	ha := HighAvailabilitySpec{}
	if ha.AntiAffinityPreset != nil {
		t.Error("expected AntiAffinityPreset to be nil for zero value")
	}
	if ha.TopologySpreadConstraints != nil {
		t.Error("expected TopologySpreadConstraints to be nil for zero value")
	}
	if ha.PodDisruptionBudget != nil {
		t.Error("expected PodDisruptionBudget to be nil for zero value")
	}
}

func TestHighAvailabilitySpec_AllFieldsSet(t *testing.T) {
	preset := AntiAffinityPresetSoft
	ha := HighAvailabilitySpec{
		AntiAffinityPreset: &preset,
		TopologySpreadConstraints: []corev1.TopologySpreadConstraint{
			{
				MaxSkew:           1,
				TopologyKey:       "topology.kubernetes.io/zone",
				WhenUnsatisfiable: corev1.DoNotSchedule,
			},
		},
		PodDisruptionBudget: &PDBSpec{
			Enabled:      true,
			MinAvailable: &intstr.IntOrString{Type: intstr.Int, IntVal: 1},
		},
	}

	if *ha.AntiAffinityPreset != AntiAffinityPresetSoft {
		t.Errorf("expected AntiAffinityPreset=soft, got %s", *ha.AntiAffinityPreset)
	}
	if len(ha.TopologySpreadConstraints) != 1 {
		t.Fatalf("expected 1 TopologySpreadConstraint, got %d", len(ha.TopologySpreadConstraints))
	}
	if ha.TopologySpreadConstraints[0].TopologyKey != "topology.kubernetes.io/zone" {
		t.Errorf("unexpected TopologyKey: %s", ha.TopologySpreadConstraints[0].TopologyKey)
	}
	if !ha.PodDisruptionBudget.Enabled {
		t.Error("expected PDB to be enabled")
	}
	if ha.PodDisruptionBudget.MinAvailable.IntVal != 1 {
		t.Errorf("expected MinAvailable=1, got %d", ha.PodDisruptionBudget.MinAvailable.IntVal)
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

func TestPDBSpec_ZeroValue(t *testing.T) {
	pdb := PDBSpec{}
	if pdb.Enabled {
		t.Error("expected Enabled to be false for zero value")
	}
	if pdb.MinAvailable != nil {
		t.Error("expected MinAvailable to be nil for zero value")
	}
	if pdb.MaxUnavailable != nil {
		t.Error("expected MaxUnavailable to be nil for zero value")
	}
}

func TestPDBSpec_WithMaxUnavailable(t *testing.T) {
	pdb := PDBSpec{
		Enabled:        true,
		MaxUnavailable: &intstr.IntOrString{Type: intstr.String, StrVal: "25%"},
	}
	if !pdb.Enabled {
		t.Error("expected Enabled to be true")
	}
	if pdb.MaxUnavailable.StrVal != "25%" {
		t.Errorf("expected MaxUnavailable=25%%, got %s", pdb.MaxUnavailable.StrVal)
	}
}

// --- REQ-003: MonitoringSpec + ServiceMonitorSpec ---

func TestMonitoringSpec_ZeroValue(t *testing.T) {
	m := MonitoringSpec{}
	if m.Enabled {
		t.Error("expected Enabled to be false for zero value")
	}
	if m.ExporterImage != nil {
		t.Error("expected ExporterImage to be nil for zero value")
	}
	if m.ExporterResources != nil {
		t.Error("expected ExporterResources to be nil for zero value")
	}
	if m.ServiceMonitor != nil {
		t.Error("expected ServiceMonitor to be nil for zero value")
	}
}

func TestMonitoringSpec_AllFieldsSet(t *testing.T) {
	img := "prom/memcached-exporter:v0.15.4"
	m := MonitoringSpec{
		Enabled:       true,
		ExporterImage: &img,
		ExporterResources: &corev1.ResourceRequirements{
			Requests: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("50m"),
				corev1.ResourceMemory: resource.MustParse("64Mi"),
			},
		},
		ServiceMonitor: &ServiceMonitorSpec{
			Interval:         "30s",
			ScrapeTimeout:    "10s",
			AdditionalLabels: map[string]string{"team": "infra"},
		},
	}

	if !m.Enabled {
		t.Error("expected Enabled to be true")
	}
	if *m.ExporterImage != "prom/memcached-exporter:v0.15.4" {
		t.Errorf("unexpected ExporterImage: %s", *m.ExporterImage)
	}
	if m.ExporterResources.Requests.Cpu().String() != "50m" {
		t.Errorf("unexpected CPU request: %s", m.ExporterResources.Requests.Cpu().String())
	}
	if m.ServiceMonitor.Interval != "30s" {
		t.Errorf("unexpected Interval: %s", m.ServiceMonitor.Interval)
	}
	if m.ServiceMonitor.ScrapeTimeout != "10s" {
		t.Errorf("unexpected ScrapeTimeout: %s", m.ServiceMonitor.ScrapeTimeout)
	}
	if m.ServiceMonitor.AdditionalLabels["team"] != "infra" {
		t.Errorf("unexpected AdditionalLabels: %v", m.ServiceMonitor.AdditionalLabels)
	}
}

func TestServiceMonitorSpec_ZeroValue(t *testing.T) {
	sm := ServiceMonitorSpec{}
	if sm.Interval != "" {
		t.Error("expected Interval to be empty for zero value")
	}
	if sm.ScrapeTimeout != "" {
		t.Error("expected ScrapeTimeout to be empty for zero value")
	}
	if sm.AdditionalLabels != nil {
		t.Error("expected AdditionalLabels to be nil for zero value")
	}
}

// --- REQ-004: SecuritySpec, SASLSpec, TLSSpec ---

func TestSecuritySpec_ZeroValue(t *testing.T) {
	s := SecuritySpec{}
	if s.PodSecurityContext != nil {
		t.Error("expected PodSecurityContext to be nil for zero value")
	}
	if s.ContainerSecurityContext != nil {
		t.Error("expected ContainerSecurityContext to be nil for zero value")
	}
	if s.SASL != nil {
		t.Error("expected SASL to be nil for zero value")
	}
	if s.TLS != nil {
		t.Error("expected TLS to be nil for zero value")
	}
}

func TestSecuritySpec_AllFieldsSet(t *testing.T) {
	runAsNonRoot := true
	runAsUser := int64(1000)
	s := SecuritySpec{
		PodSecurityContext: &corev1.PodSecurityContext{
			RunAsNonRoot: &runAsNonRoot,
		},
		ContainerSecurityContext: &corev1.SecurityContext{
			RunAsUser: &runAsUser,
		},
		SASL: &SASLSpec{
			Enabled: true,
			CredentialsSecretRef: corev1.LocalObjectReference{
				Name: "memcached-sasl-creds",
			},
		},
		TLS: &TLSSpec{
			Enabled: true,
			CertificateSecretRef: corev1.LocalObjectReference{
				Name: "memcached-tls-cert",
			},
		},
	}

	if !*s.PodSecurityContext.RunAsNonRoot {
		t.Error("expected PodSecurityContext.RunAsNonRoot to be true")
	}
	if *s.ContainerSecurityContext.RunAsUser != 1000 {
		t.Errorf("expected ContainerSecurityContext.RunAsUser=1000, got %d", *s.ContainerSecurityContext.RunAsUser)
	}
	if !s.SASL.Enabled {
		t.Error("expected SASL.Enabled to be true")
	}
	if s.SASL.CredentialsSecretRef.Name != "memcached-sasl-creds" {
		t.Errorf("unexpected SASL.CredentialsSecretRef.Name: %s", s.SASL.CredentialsSecretRef.Name)
	}
	if !s.TLS.Enabled {
		t.Error("expected TLS.Enabled to be true")
	}
	if s.TLS.CertificateSecretRef.Name != "memcached-tls-cert" {
		t.Errorf("unexpected TLS.CertificateSecretRef.Name: %s", s.TLS.CertificateSecretRef.Name)
	}
}

func TestSASLSpec_ZeroValue(t *testing.T) {
	s := SASLSpec{}
	if s.Enabled {
		t.Error("expected Enabled to be false for zero value")
	}
	if s.CredentialsSecretRef.Name != "" {
		t.Error("expected CredentialsSecretRef.Name to be empty for zero value")
	}
}

func TestTLSSpec_ZeroValue(t *testing.T) {
	tls := TLSSpec{}
	if tls.Enabled {
		t.Error("expected Enabled to be false for zero value")
	}
	if tls.CertificateSecretRef.Name != "" {
		t.Error("expected CertificateSecretRef.Name to be empty for zero value")
	}
}

// --- Integration: MemcachedSpec with all nested structs ---

func TestMemcachedSpec_WithAllNestedStructs(t *testing.T) {
	replicas := int32(3)
	img := "memcached:1.6.33"
	preset := AntiAffinityPresetHard

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
		HighAvailability: &HighAvailabilitySpec{
			AntiAffinityPreset: &preset,
		},
		Monitoring: &MonitoringSpec{
			Enabled: true,
		},
		Security: &SecuritySpec{
			TLS: &TLSSpec{
				Enabled: true,
				CertificateSecretRef: corev1.LocalObjectReference{
					Name: "tls-secret",
				},
			},
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
	if spec.Memcached.MaxMemoryMB != 128 {
		t.Errorf("expected MaxMemoryMB=128, got %d", spec.Memcached.MaxMemoryMB)
	}
	if *spec.HighAvailability.AntiAffinityPreset != AntiAffinityPresetHard {
		t.Errorf("expected AntiAffinityPreset=hard, got %s", *spec.HighAvailability.AntiAffinityPreset)
	}
	if !spec.Monitoring.Enabled {
		t.Error("expected Monitoring.Enabled to be true")
	}
	if !spec.Security.TLS.Enabled {
		t.Error("expected Security.TLS.Enabled to be true")
	}
}

func TestMemcached_FullResource(t *testing.T) {
	replicas := int32(1)
	mc := Memcached{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "memcached.c5c3.io/v1alpha1",
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

// --- DeepCopy tests for new structs ---

func TestMemcachedConfig_DeepCopy(t *testing.T) {
	cfg := &MemcachedConfig{
		MaxMemoryMB: 64,
		ExtraArgs:   []string{"-o", "modern"},
	}
	clone := cfg.DeepCopy()
	if clone == cfg {
		t.Error("DeepCopy returned same pointer")
	}
	if clone.MaxMemoryMB != 64 {
		t.Errorf("expected MaxMemoryMB=64, got %d", clone.MaxMemoryMB)
	}
	// Mutate original; clone must be unaffected.
	cfg.MaxMemoryMB = 128
	cfg.ExtraArgs[0] = "-v"
	if clone.MaxMemoryMB != 64 {
		t.Error("DeepCopy is not independent: MaxMemoryMB was mutated")
	}
	if clone.ExtraArgs[0] != "-o" {
		t.Error("DeepCopy is not independent: ExtraArgs was mutated")
	}
}

func TestHighAvailabilitySpec_DeepCopy(t *testing.T) {
	preset := AntiAffinityPresetSoft
	ha := &HighAvailabilitySpec{
		AntiAffinityPreset: &preset,
		PodDisruptionBudget: &PDBSpec{
			Enabled:      true,
			MinAvailable: &intstr.IntOrString{Type: intstr.Int, IntVal: 2},
		},
	}
	clone := ha.DeepCopy()
	if clone == ha {
		t.Error("DeepCopy returned same pointer")
	}
	*ha.AntiAffinityPreset = AntiAffinityPresetHard
	if *clone.AntiAffinityPreset != AntiAffinityPresetSoft {
		t.Error("DeepCopy is not independent: AntiAffinityPreset was mutated")
	}
}

func TestMonitoringSpec_DeepCopy(t *testing.T) {
	img := "exporter:v1"
	m := &MonitoringSpec{
		Enabled:       true,
		ExporterImage: &img,
		ServiceMonitor: &ServiceMonitorSpec{
			AdditionalLabels: map[string]string{"k": "v"},
		},
	}
	clone := m.DeepCopy()
	if clone == m {
		t.Error("DeepCopy returned same pointer")
	}
	m.ServiceMonitor.AdditionalLabels["k"] = testMutatedValue
	if clone.ServiceMonitor.AdditionalLabels["k"] != "v" {
		t.Error("DeepCopy is not independent: AdditionalLabels was mutated")
	}
}

func TestSecuritySpec_DeepCopy(t *testing.T) {
	runAsNonRoot := true
	runAsUser := int64(1000)
	s := &SecuritySpec{
		PodSecurityContext: &corev1.PodSecurityContext{
			RunAsNonRoot: &runAsNonRoot,
		},
		ContainerSecurityContext: &corev1.SecurityContext{
			RunAsUser: &runAsUser,
		},
		SASL: &SASLSpec{
			Enabled: true,
			CredentialsSecretRef: corev1.LocalObjectReference{
				Name: "secret",
			},
		},
		TLS: &TLSSpec{
			Enabled: true,
			CertificateSecretRef: corev1.LocalObjectReference{
				Name: "tls-secret",
			},
		},
	}
	clone := s.DeepCopy()
	if clone == s {
		t.Error("DeepCopy returned same pointer")
	}
	*s.PodSecurityContext.RunAsNonRoot = false
	if !*clone.PodSecurityContext.RunAsNonRoot {
		t.Error("DeepCopy is not independent: PodSecurityContext.RunAsNonRoot was mutated")
	}
	*s.ContainerSecurityContext.RunAsUser = 2000
	if *clone.ContainerSecurityContext.RunAsUser != 1000 {
		t.Error("DeepCopy is not independent: ContainerSecurityContext.RunAsUser was mutated")
	}
}

// --- REQ-001: ServiceSpec ---

func TestServiceSpec_ZeroValue(t *testing.T) {
	s := ServiceSpec{}
	if s.Annotations != nil {
		t.Error("expected Annotations to be nil for zero value")
	}
}

func TestServiceSpec_AllFieldsSet(t *testing.T) {
	s := ServiceSpec{
		Annotations: map[string]string{
			"prometheus.io/scrape": "true",
			"prometheus.io/port":   "9150",
		},
	}
	if len(s.Annotations) != 2 {
		t.Errorf("expected 2 annotations, got %d", len(s.Annotations))
	}
	if s.Annotations["prometheus.io/scrape"] != "true" {
		t.Errorf("unexpected annotation value: %s", s.Annotations["prometheus.io/scrape"])
	}
	if s.Annotations["prometheus.io/port"] != "9150" {
		t.Errorf("unexpected annotation value: %s", s.Annotations["prometheus.io/port"])
	}
}

func TestServiceSpec_DeepCopy(t *testing.T) {
	s := &ServiceSpec{
		Annotations: map[string]string{"key": "value"},
	}
	clone := s.DeepCopy()
	if clone == s {
		t.Error("DeepCopy returned same pointer")
	}
	s.Annotations["key"] = testMutatedValue
	if clone.Annotations["key"] != "value" {
		t.Error("DeepCopy is not independent: Annotations was mutated")
	}
}

func TestMemcachedSpec_WithServiceSpec(t *testing.T) {
	replicas := int32(1)
	spec := MemcachedSpec{
		Replicas: &replicas,
		Service: &ServiceSpec{
			Annotations: map[string]string{
				"prometheus.io/scrape": "true",
			},
		},
	}
	if spec.Service == nil {
		t.Fatal("expected Service to be set")
	}
	if spec.Service.Annotations["prometheus.io/scrape"] != "true" {
		t.Errorf("unexpected annotation: %s", spec.Service.Annotations["prometheus.io/scrape"])
	}
}

func TestMemcachedSpec_NilServiceSpec(t *testing.T) {
	spec := MemcachedSpec{}
	if spec.Service != nil {
		t.Error("expected Service to be nil for empty spec")
	}
}
