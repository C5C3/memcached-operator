// Package v1alpha1 contains unit tests for the Memcached defaulting webhook.
package v1alpha1

import (
	"context"
	"testing"

	autoscalingv2 "k8s.io/api/autoscaling/v2"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

const (
	testExplicitImage    = "memcached:1.6.28"
	testExplicitInterval = "15s"
)

func TestMemcachedDefaulting_EmptySpec(t *testing.T) {
	mc := &Memcached{}
	d := &MemcachedCustomDefaulter{}

	if err := d.Default(context.Background(), mc); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// REQ-001: replicas defaults to 1.
	if mc.Spec.Replicas == nil || *mc.Spec.Replicas != 1 {
		t.Errorf("expected replicas=1, got %v", mc.Spec.Replicas)
	}

	// REQ-002: image defaults to memcached:1.6.
	if mc.Spec.Image == nil || *mc.Spec.Image != "memcached:1.6" {
		t.Errorf("expected image=memcached:1.6, got %v", mc.Spec.Image)
	}

	// REQ-003: memcached config is initialized with defaults.
	if mc.Spec.Memcached == nil {
		t.Fatal("expected spec.memcached to be initialized")
	}
	if mc.Spec.Memcached.MaxMemoryMB != 64 {
		t.Errorf("expected maxMemoryMB=64, got %d", mc.Spec.Memcached.MaxMemoryMB)
	}
	if mc.Spec.Memcached.MaxConnections != 1024 {
		t.Errorf("expected maxConnections=1024, got %d", mc.Spec.Memcached.MaxConnections)
	}
	if mc.Spec.Memcached.Threads != 4 {
		t.Errorf("expected threads=4, got %d", mc.Spec.Memcached.Threads)
	}
	if mc.Spec.Memcached.MaxItemSize != "1m" {
		t.Errorf("expected maxItemSize=1m, got %s", mc.Spec.Memcached.MaxItemSize)
	}
	if mc.Spec.Memcached.Verbosity != 0 {
		t.Errorf("expected verbosity=0, got %d", mc.Spec.Memcached.Verbosity)
	}

	// Optional sections should remain nil.
	if mc.Spec.Monitoring != nil {
		t.Error("expected monitoring to remain nil")
	}
	if mc.Spec.HighAvailability != nil {
		t.Error("expected highAvailability to remain nil")
	}
}

func TestMemcachedDefaulting_PreservesExplicitValues(t *testing.T) {
	replicas := int32(3)
	image := testExplicitImage
	mc := &Memcached{
		Spec: MemcachedSpec{
			Replicas: &replicas,
			Image:    &image,
			Memcached: &MemcachedConfig{
				MaxMemoryMB:    256,
				MaxConnections: 2048,
				Threads:        8,
				MaxItemSize:    "2m",
				Verbosity:      2,
			},
		},
	}
	d := &MemcachedCustomDefaulter{}

	if err := d.Default(context.Background(), mc); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if *mc.Spec.Replicas != 3 {
		t.Errorf("expected replicas=3, got %d", *mc.Spec.Replicas)
	}
	if *mc.Spec.Image != testExplicitImage {
		t.Errorf("expected image=memcached:1.6.28, got %s", *mc.Spec.Image)
	}
	if mc.Spec.Memcached.MaxMemoryMB != 256 {
		t.Errorf("expected maxMemoryMB=256, got %d", mc.Spec.Memcached.MaxMemoryMB)
	}
	if mc.Spec.Memcached.MaxConnections != 2048 {
		t.Errorf("expected maxConnections=2048, got %d", mc.Spec.Memcached.MaxConnections)
	}
	if mc.Spec.Memcached.Threads != 8 {
		t.Errorf("expected threads=8, got %d", mc.Spec.Memcached.Threads)
	}
	if mc.Spec.Memcached.MaxItemSize != "2m" {
		t.Errorf("expected maxItemSize=2m, got %s", mc.Spec.Memcached.MaxItemSize)
	}
	if mc.Spec.Memcached.Verbosity != 2 {
		t.Errorf("expected verbosity=2, got %d", mc.Spec.Memcached.Verbosity)
	}
}

func TestMemcachedDefaulting_NilMemcachedConfig(t *testing.T) {
	mc := &Memcached{
		Spec: MemcachedSpec{
			Memcached: nil,
		},
	}
	d := &MemcachedCustomDefaulter{}

	if err := d.Default(context.Background(), mc); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if mc.Spec.Memcached == nil {
		t.Fatal("expected spec.memcached to be initialized from nil")
	}
	if mc.Spec.Memcached.MaxMemoryMB != 64 {
		t.Errorf("expected maxMemoryMB=64, got %d", mc.Spec.Memcached.MaxMemoryMB)
	}
	if mc.Spec.Memcached.MaxConnections != 1024 {
		t.Errorf("expected maxConnections=1024, got %d", mc.Spec.Memcached.MaxConnections)
	}
	if mc.Spec.Memcached.Threads != 4 {
		t.Errorf("expected threads=4, got %d", mc.Spec.Memcached.Threads)
	}
	if mc.Spec.Memcached.MaxItemSize != "1m" {
		t.Errorf("expected maxItemSize=1m, got %s", mc.Spec.Memcached.MaxItemSize)
	}
}

func TestMemcachedDefaulting_PartialMemcachedConfig(t *testing.T) {
	mc := &Memcached{
		Spec: MemcachedSpec{
			Memcached: &MemcachedConfig{
				MaxMemoryMB: 256,
			},
		},
	}
	d := &MemcachedCustomDefaulter{}

	if err := d.Default(context.Background(), mc); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if mc.Spec.Memcached.MaxMemoryMB != 256 {
		t.Errorf("expected maxMemoryMB=256 (preserved), got %d", mc.Spec.Memcached.MaxMemoryMB)
	}
	if mc.Spec.Memcached.MaxConnections != 1024 {
		t.Errorf("expected maxConnections=1024 (defaulted), got %d", mc.Spec.Memcached.MaxConnections)
	}
	if mc.Spec.Memcached.Threads != 4 {
		t.Errorf("expected threads=4 (defaulted), got %d", mc.Spec.Memcached.Threads)
	}
	if mc.Spec.Memcached.MaxItemSize != "1m" {
		t.Errorf("expected maxItemSize=1m (defaulted), got %s", mc.Spec.Memcached.MaxItemSize)
	}
}

func TestMemcachedDefaulting_MonitoringExporterImage(t *testing.T) {
	mc := &Memcached{
		Spec: MemcachedSpec{
			Monitoring: &MonitoringSpec{
				Enabled: true,
			},
		},
	}
	d := &MemcachedCustomDefaulter{}

	if err := d.Default(context.Background(), mc); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if mc.Spec.Monitoring.ExporterImage == nil {
		t.Fatal("expected exporterImage to be defaulted")
	}
	if *mc.Spec.Monitoring.ExporterImage != "prom/memcached-exporter:v0.15.4" {
		t.Errorf("expected exporterImage=prom/memcached-exporter:v0.15.4, got %s", *mc.Spec.Monitoring.ExporterImage)
	}
}

func TestMemcachedDefaulting_MonitoringExporterImagePreserved(t *testing.T) {
	customImage := "custom/exporter:v1.0"
	mc := &Memcached{
		Spec: MemcachedSpec{
			Monitoring: &MonitoringSpec{
				Enabled:       true,
				ExporterImage: &customImage,
			},
		},
	}
	d := &MemcachedCustomDefaulter{}

	if err := d.Default(context.Background(), mc); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if *mc.Spec.Monitoring.ExporterImage != "custom/exporter:v1.0" {
		t.Errorf("expected exporterImage=custom/exporter:v1.0 (preserved), got %s", *mc.Spec.Monitoring.ExporterImage)
	}
}

func TestMemcachedDefaulting_NilMonitoringStaysNil(t *testing.T) {
	mc := &Memcached{
		Spec: MemcachedSpec{
			Monitoring: nil,
		},
	}
	d := &MemcachedCustomDefaulter{}

	if err := d.Default(context.Background(), mc); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if mc.Spec.Monitoring != nil {
		t.Error("expected monitoring to remain nil (opt-in section)")
	}
}

func TestMemcachedDefaulting_AntiAffinityPreset(t *testing.T) {
	mc := &Memcached{
		Spec: MemcachedSpec{
			HighAvailability: &HighAvailabilitySpec{},
		},
	}
	d := &MemcachedCustomDefaulter{}

	if err := d.Default(context.Background(), mc); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if mc.Spec.HighAvailability.AntiAffinityPreset == nil {
		t.Fatal("expected antiAffinityPreset to be defaulted")
	}
	if *mc.Spec.HighAvailability.AntiAffinityPreset != AntiAffinityPresetSoft {
		t.Errorf("expected antiAffinityPreset=soft, got %s", *mc.Spec.HighAvailability.AntiAffinityPreset)
	}
}

func TestMemcachedDefaulting_AntiAffinityPresetHardPreserved(t *testing.T) {
	preset := AntiAffinityPresetHard
	mc := &Memcached{
		Spec: MemcachedSpec{
			HighAvailability: &HighAvailabilitySpec{
				AntiAffinityPreset: &preset,
			},
		},
	}
	d := &MemcachedCustomDefaulter{}

	if err := d.Default(context.Background(), mc); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if *mc.Spec.HighAvailability.AntiAffinityPreset != AntiAffinityPresetHard {
		t.Errorf("expected antiAffinityPreset=hard (preserved), got %s", *mc.Spec.HighAvailability.AntiAffinityPreset)
	}
}

func TestMemcachedDefaulting_NilHAStaysNil(t *testing.T) {
	mc := &Memcached{
		Spec: MemcachedSpec{
			HighAvailability: nil,
		},
	}
	d := &MemcachedCustomDefaulter{}

	if err := d.Default(context.Background(), mc); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if mc.Spec.HighAvailability != nil {
		t.Error("expected highAvailability to remain nil (opt-in section)")
	}
}

func TestMemcachedDefaulting_ServiceMonitorDefaults(t *testing.T) {
	mc := &Memcached{
		Spec: MemcachedSpec{
			Monitoring: &MonitoringSpec{
				Enabled:        true,
				ServiceMonitor: &ServiceMonitorSpec{},
			},
		},
	}
	d := &MemcachedCustomDefaulter{}

	if err := d.Default(context.Background(), mc); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if mc.Spec.Monitoring.ServiceMonitor.Interval != "30s" {
		t.Errorf("expected interval=30s, got %s", mc.Spec.Monitoring.ServiceMonitor.Interval)
	}
	if mc.Spec.Monitoring.ServiceMonitor.ScrapeTimeout != "10s" {
		t.Errorf("expected scrapeTimeout=10s, got %s", mc.Spec.Monitoring.ServiceMonitor.ScrapeTimeout)
	}
}

func TestMemcachedDefaulting_ServiceMonitorPartialPreserved(t *testing.T) {
	mc := &Memcached{
		Spec: MemcachedSpec{
			Monitoring: &MonitoringSpec{
				Enabled: true,
				ServiceMonitor: &ServiceMonitorSpec{
					Interval: testExplicitInterval,
				},
			},
		},
	}
	d := &MemcachedCustomDefaulter{}

	if err := d.Default(context.Background(), mc); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if mc.Spec.Monitoring.ServiceMonitor.Interval != testExplicitInterval {
		t.Errorf("expected interval=15s (preserved), got %s", mc.Spec.Monitoring.ServiceMonitor.Interval)
	}
	if mc.Spec.Monitoring.ServiceMonitor.ScrapeTimeout != "10s" {
		t.Errorf("expected scrapeTimeout=10s (defaulted), got %s", mc.Spec.Monitoring.ServiceMonitor.ScrapeTimeout)
	}
}

func TestMemcachedDefaulting_NilServiceMonitorStaysNil(t *testing.T) {
	mc := &Memcached{
		Spec: MemcachedSpec{
			Monitoring: &MonitoringSpec{
				Enabled:        true,
				ServiceMonitor: nil,
			},
		},
	}
	d := &MemcachedCustomDefaulter{}

	if err := d.Default(context.Background(), mc); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if mc.Spec.Monitoring.ServiceMonitor != nil {
		t.Error("expected serviceMonitor to remain nil")
	}
}

func TestMemcachedDefaulting_ReplicasZeroPreserved(t *testing.T) {
	zero := int32(0)
	mc := &Memcached{
		Spec: MemcachedSpec{
			Replicas: &zero,
		},
	}
	d := &MemcachedCustomDefaulter{}

	if err := d.Default(context.Background(), mc); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if mc.Spec.Replicas == nil || *mc.Spec.Replicas != 0 {
		t.Errorf("expected replicas=0 (preserved), got %v", mc.Spec.Replicas)
	}
}

func TestMemcachedDefaulting_FullySpecifiedCRUnchanged(t *testing.T) {
	replicas := int32(5)
	image := testExplicitImage
	exporterImage := "custom/exporter:v2"
	preset := AntiAffinityPresetHard

	mc := &Memcached{
		Spec: MemcachedSpec{
			Replicas: &replicas,
			Image:    &image,
			Memcached: &MemcachedConfig{
				MaxMemoryMB:    512,
				MaxConnections: 4096,
				Threads:        16,
				MaxItemSize:    "4m",
				Verbosity:      1,
				ExtraArgs:      []string{"-o", "modern"},
			},
			Monitoring: &MonitoringSpec{
				Enabled:       true,
				ExporterImage: &exporterImage,
				ServiceMonitor: &ServiceMonitorSpec{
					Interval:      "15s",
					ScrapeTimeout: "5s",
				},
			},
			HighAvailability: &HighAvailabilitySpec{
				AntiAffinityPreset: &preset,
			},
		},
	}
	d := &MemcachedCustomDefaulter{}

	if err := d.Default(context.Background(), mc); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if *mc.Spec.Replicas != 5 {
		t.Errorf("expected replicas=5, got %d", *mc.Spec.Replicas)
	}
	if *mc.Spec.Image != testExplicitImage {
		t.Errorf("expected image=memcached:1.6.28, got %s", *mc.Spec.Image)
	}
	if mc.Spec.Memcached.MaxMemoryMB != 512 {
		t.Errorf("expected maxMemoryMB=512, got %d", mc.Spec.Memcached.MaxMemoryMB)
	}
	if mc.Spec.Memcached.MaxConnections != 4096 {
		t.Errorf("expected maxConnections=4096, got %d", mc.Spec.Memcached.MaxConnections)
	}
	if mc.Spec.Memcached.Threads != 16 {
		t.Errorf("expected threads=16, got %d", mc.Spec.Memcached.Threads)
	}
	if mc.Spec.Memcached.MaxItemSize != "4m" {
		t.Errorf("expected maxItemSize=4m, got %s", mc.Spec.Memcached.MaxItemSize)
	}
	if mc.Spec.Memcached.Verbosity != 1 {
		t.Errorf("expected verbosity=1, got %d", mc.Spec.Memcached.Verbosity)
	}
	if len(mc.Spec.Memcached.ExtraArgs) != 2 {
		t.Errorf("expected 2 extraArgs, got %d", len(mc.Spec.Memcached.ExtraArgs))
	}
	if *mc.Spec.Monitoring.ExporterImage != "custom/exporter:v2" {
		t.Errorf("expected exporterImage=custom/exporter:v2, got %s", *mc.Spec.Monitoring.ExporterImage)
	}
	if mc.Spec.Monitoring.ServiceMonitor.Interval != testExplicitInterval {
		t.Errorf("expected interval=15s, got %s", mc.Spec.Monitoring.ServiceMonitor.Interval)
	}
	if mc.Spec.Monitoring.ServiceMonitor.ScrapeTimeout != "5s" {
		t.Errorf("expected scrapeTimeout=5s, got %s", mc.Spec.Monitoring.ServiceMonitor.ScrapeTimeout)
	}
	if *mc.Spec.HighAvailability.AntiAffinityPreset != AntiAffinityPresetHard {
		t.Errorf("expected antiAffinityPreset=hard, got %s", *mc.Spec.HighAvailability.AntiAffinityPreset)
	}
}

// --- Task 1.1: Additional defaulting edge cases (REQ-001, REQ-002, REQ-003) ---

func TestMemcachedDefaulting_Idempotent(t *testing.T) {
	mc := &Memcached{}
	d := &MemcachedCustomDefaulter{}

	if err := d.Default(context.Background(), mc); err != nil {
		t.Fatalf("first Default call: unexpected error: %v", err)
	}

	// Capture state after first call.
	replicas := *mc.Spec.Replicas
	image := *mc.Spec.Image
	maxMem := mc.Spec.Memcached.MaxMemoryMB
	maxConn := mc.Spec.Memcached.MaxConnections
	threads := mc.Spec.Memcached.Threads
	maxItem := mc.Spec.Memcached.MaxItemSize

	// Apply defaults a second time.
	if err := d.Default(context.Background(), mc); err != nil {
		t.Fatalf("second Default call: unexpected error: %v", err)
	}

	if *mc.Spec.Replicas != replicas {
		t.Errorf("idempotency: replicas changed from %d to %d", replicas, *mc.Spec.Replicas)
	}
	if *mc.Spec.Image != image {
		t.Errorf("idempotency: image changed from %s to %s", image, *mc.Spec.Image)
	}
	if mc.Spec.Memcached.MaxMemoryMB != maxMem {
		t.Errorf("idempotency: maxMemoryMB changed from %d to %d", maxMem, mc.Spec.Memcached.MaxMemoryMB)
	}
	if mc.Spec.Memcached.MaxConnections != maxConn {
		t.Errorf("idempotency: maxConnections changed from %d to %d", maxConn, mc.Spec.Memcached.MaxConnections)
	}
	if mc.Spec.Memcached.Threads != threads {
		t.Errorf("idempotency: threads changed from %d to %d", threads, mc.Spec.Memcached.Threads)
	}
	if mc.Spec.Memcached.MaxItemSize != maxItem {
		t.Errorf("idempotency: maxItemSize changed from %s to %s", maxItem, mc.Spec.Memcached.MaxItemSize)
	}
}

func TestMemcachedDefaulting_EmptyStringImagePreserved(t *testing.T) {
	emptyImage := ""
	mc := &Memcached{
		Spec: MemcachedSpec{
			Image: &emptyImage,
		},
	}
	d := &MemcachedCustomDefaulter{}

	if err := d.Default(context.Background(), mc); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// The webhook only defaults nil image, not empty string; the pointer is non-nil.
	if mc.Spec.Image == nil {
		t.Fatal("expected image pointer to remain non-nil")
	}
	if *mc.Spec.Image != "" {
		t.Errorf("expected empty-string image preserved, got %q", *mc.Spec.Image)
	}
}

func TestMemcachedDefaulting_VerbosityZeroExplicit(t *testing.T) {
	mc := &Memcached{
		Spec: MemcachedSpec{
			Memcached: &MemcachedConfig{
				MaxMemoryMB: 128,
				Verbosity:   0,
			},
		},
	}
	d := &MemcachedCustomDefaulter{}

	if err := d.Default(context.Background(), mc); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verbosity 0 is the Go zero value and also a valid configuration.
	if mc.Spec.Memcached.Verbosity != 0 {
		t.Errorf("expected verbosity=0 preserved, got %d", mc.Spec.Memcached.Verbosity)
	}
	// Other zero-value fields should be defaulted.
	if mc.Spec.Memcached.MaxConnections != DefaultMaxConnections {
		t.Errorf("expected maxConnections=%d, got %d", DefaultMaxConnections, mc.Spec.Memcached.MaxConnections)
	}
	if mc.Spec.Memcached.Threads != DefaultThreads {
		t.Errorf("expected threads=%d, got %d", DefaultThreads, mc.Spec.Memcached.Threads)
	}
}

func TestMemcachedDefaulting_ExtraArgsPreserved(t *testing.T) {
	mc := &Memcached{
		Spec: MemcachedSpec{
			Memcached: &MemcachedConfig{
				ExtraArgs: []string{"-o", "modern", "-B", "binary"},
			},
		},
	}
	d := &MemcachedCustomDefaulter{}

	if err := d.Default(context.Background(), mc); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(mc.Spec.Memcached.ExtraArgs) != 4 {
		t.Errorf("expected 4 extraArgs preserved, got %d", len(mc.Spec.Memcached.ExtraArgs))
	}
	if mc.Spec.Memcached.ExtraArgs[0] != "-o" || mc.Spec.Memcached.ExtraArgs[1] != "modern" {
		t.Errorf("expected first two extraArgs=[-o, modern], got %v", mc.Spec.Memcached.ExtraArgs[:2])
	}
}

// --- Task 1.2: Additional monitoring and HA sub-section defaults (REQ-004, REQ-005) ---

func TestMemcachedDefaulting_MonitoringDisabledStillDefaults(t *testing.T) {
	mc := &Memcached{
		Spec: MemcachedSpec{
			Monitoring: &MonitoringSpec{
				Enabled: false,
			},
		},
	}
	d := &MemcachedCustomDefaulter{}

	if err := d.Default(context.Background(), mc); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Even when monitoring is disabled, the webhook defaults sub-fields because
	// the section is non-nil (opt-in).
	if mc.Spec.Monitoring.ExporterImage == nil {
		t.Fatal("expected exporterImage to be defaulted even when monitoring.enabled=false")
	}
	if *mc.Spec.Monitoring.ExporterImage != DefaultExporterImage {
		t.Errorf("expected exporterImage=%s, got %s", DefaultExporterImage, *mc.Spec.Monitoring.ExporterImage)
	}
}

func TestMemcachedDefaulting_ServiceMonitorFullySpecifiedPreserved(t *testing.T) {
	mc := &Memcached{
		Spec: MemcachedSpec{
			Monitoring: &MonitoringSpec{
				Enabled: true,
				ServiceMonitor: &ServiceMonitorSpec{
					Interval:         "15s",
					ScrapeTimeout:    "5s",
					AdditionalLabels: map[string]string{"team": "platform"},
				},
			},
		},
	}
	d := &MemcachedCustomDefaulter{}

	if err := d.Default(context.Background(), mc); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if mc.Spec.Monitoring.ServiceMonitor.Interval != testExplicitInterval {
		t.Errorf("expected interval=15s, got %s", mc.Spec.Monitoring.ServiceMonitor.Interval)
	}
	if mc.Spec.Monitoring.ServiceMonitor.ScrapeTimeout != "5s" {
		t.Errorf("expected scrapeTimeout=5s, got %s", mc.Spec.Monitoring.ServiceMonitor.ScrapeTimeout)
	}
	if mc.Spec.Monitoring.ServiceMonitor.AdditionalLabels["team"] != "platform" {
		t.Errorf("expected additionalLabels preserved, got %v", mc.Spec.Monitoring.ServiceMonitor.AdditionalLabels)
	}
}

func TestMemcachedDefaulting_HAWithPDBStillDefaultsPreset(t *testing.T) {
	mc := &Memcached{
		Spec: MemcachedSpec{
			HighAvailability: &HighAvailabilitySpec{
				PodDisruptionBudget: &PDBSpec{
					Enabled: true,
				},
			},
		},
	}
	d := &MemcachedCustomDefaulter{}

	if err := d.Default(context.Background(), mc); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// AntiAffinityPreset should be defaulted to "soft" even when other HA fields are set.
	if mc.Spec.HighAvailability.AntiAffinityPreset == nil {
		t.Fatal("expected antiAffinityPreset to be defaulted")
	}
	if *mc.Spec.HighAvailability.AntiAffinityPreset != AntiAffinityPresetSoft {
		t.Errorf("expected antiAffinityPreset=soft, got %s", *mc.Spec.HighAvailability.AntiAffinityPreset)
	}
}

func TestMemcachedDefaulting_IdempotentWithMonitoringAndHA(t *testing.T) {
	mc := &Memcached{
		Spec: MemcachedSpec{
			Monitoring: &MonitoringSpec{
				Enabled:        true,
				ServiceMonitor: &ServiceMonitorSpec{},
			},
			HighAvailability: &HighAvailabilitySpec{},
		},
	}
	d := &MemcachedCustomDefaulter{}

	if err := d.Default(context.Background(), mc); err != nil {
		t.Fatalf("first Default call: unexpected error: %v", err)
	}

	exporterImage := *mc.Spec.Monitoring.ExporterImage
	interval := mc.Spec.Monitoring.ServiceMonitor.Interval
	preset := *mc.Spec.HighAvailability.AntiAffinityPreset

	if err := d.Default(context.Background(), mc); err != nil {
		t.Fatalf("second Default call: unexpected error: %v", err)
	}

	if *mc.Spec.Monitoring.ExporterImage != exporterImage {
		t.Errorf("idempotency: exporterImage changed")
	}
	if mc.Spec.Monitoring.ServiceMonitor.Interval != interval {
		t.Errorf("idempotency: interval changed")
	}
	if *mc.Spec.HighAvailability.AntiAffinityPreset != preset {
		t.Errorf("idempotency: antiAffinityPreset changed")
	}
}

// --- Task 2.1/2.2: Autoscaling defaulting tests (REQ-003, REQ-004) ---

func TestMemcachedDefaulting_AutoscalingDefaultMetrics(t *testing.T) {
	mc := &Memcached{
		Spec: MemcachedSpec{
			Autoscaling: &AutoscalingSpec{
				Enabled:     true,
				MaxReplicas: 10,
			},
		},
	}
	d := &MemcachedCustomDefaulter{}

	if err := d.Default(context.Background(), mc); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(mc.Spec.Autoscaling.Metrics) != 1 {
		t.Fatalf("expected 1 default metric, got %d", len(mc.Spec.Autoscaling.Metrics))
	}
	metric := mc.Spec.Autoscaling.Metrics[0]
	if metric.Type != autoscalingv2.ResourceMetricSourceType {
		t.Errorf("expected metric type=Resource, got %s", metric.Type)
	}
	if metric.Resource == nil {
		t.Fatal("expected resource metric to be non-nil")
	}
	if metric.Resource.Name != corev1.ResourceCPU {
		t.Errorf("expected resource name=cpu, got %s", metric.Resource.Name)
	}
	if metric.Resource.Target.Type != autoscalingv2.UtilizationMetricType {
		t.Errorf("expected target type=Utilization, got %s", metric.Resource.Target.Type)
	}
	if metric.Resource.Target.AverageUtilization == nil || *metric.Resource.Target.AverageUtilization != 80 {
		t.Errorf("expected averageUtilization=80, got %v", metric.Resource.Target.AverageUtilization)
	}
}

func TestMemcachedDefaulting_AutoscalingDefaultBehavior(t *testing.T) {
	mc := &Memcached{
		Spec: MemcachedSpec{
			Autoscaling: &AutoscalingSpec{
				Enabled:     true,
				MaxReplicas: 10,
			},
		},
	}
	d := &MemcachedCustomDefaulter{}

	if err := d.Default(context.Background(), mc); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if mc.Spec.Autoscaling.Behavior == nil {
		t.Fatal("expected behavior to be defaulted")
	}
	if mc.Spec.Autoscaling.Behavior.ScaleDown == nil {
		t.Fatal("expected scaleDown to be set")
	}
	if mc.Spec.Autoscaling.Behavior.ScaleDown.StabilizationWindowSeconds == nil {
		t.Fatal("expected stabilizationWindowSeconds to be set")
	}
	if *mc.Spec.Autoscaling.Behavior.ScaleDown.StabilizationWindowSeconds != 300 {
		t.Errorf("expected stabilizationWindowSeconds=300, got %d", *mc.Spec.Autoscaling.Behavior.ScaleDown.StabilizationWindowSeconds)
	}
}

func TestMemcachedDefaulting_AutoscalingPreservesUserMetrics(t *testing.T) {
	memUtilization := int32(70)
	mc := &Memcached{
		Spec: MemcachedSpec{
			Autoscaling: &AutoscalingSpec{
				Enabled:     true,
				MaxReplicas: 10,
				Metrics: []autoscalingv2.MetricSpec{
					{
						Type: autoscalingv2.ResourceMetricSourceType,
						Resource: &autoscalingv2.ResourceMetricSource{
							Name: corev1.ResourceMemory,
							Target: autoscalingv2.MetricTarget{
								Type:               autoscalingv2.UtilizationMetricType,
								AverageUtilization: &memUtilization,
							},
						},
					},
				},
			},
		},
	}
	d := &MemcachedCustomDefaulter{}

	if err := d.Default(context.Background(), mc); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(mc.Spec.Autoscaling.Metrics) != 1 {
		t.Fatalf("expected 1 user metric preserved, got %d", len(mc.Spec.Autoscaling.Metrics))
	}
	if mc.Spec.Autoscaling.Metrics[0].Resource.Name != corev1.ResourceMemory {
		t.Errorf("expected user memory metric preserved, got %s", mc.Spec.Autoscaling.Metrics[0].Resource.Name)
	}
	if *mc.Spec.Autoscaling.Metrics[0].Resource.Target.AverageUtilization != 70 {
		t.Errorf("expected averageUtilization=70 preserved, got %d", *mc.Spec.Autoscaling.Metrics[0].Resource.Target.AverageUtilization)
	}
}

func TestMemcachedDefaulting_AutoscalingPreservesUserBehavior(t *testing.T) {
	stabilization := int32(600)
	mc := &Memcached{
		Spec: MemcachedSpec{
			Autoscaling: &AutoscalingSpec{
				Enabled:     true,
				MaxReplicas: 10,
				Behavior: &autoscalingv2.HorizontalPodAutoscalerBehavior{
					ScaleDown: &autoscalingv2.HPAScalingRules{
						StabilizationWindowSeconds: &stabilization,
					},
				},
			},
		},
	}
	d := &MemcachedCustomDefaulter{}

	if err := d.Default(context.Background(), mc); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if mc.Spec.Autoscaling.Behavior.ScaleDown.StabilizationWindowSeconds == nil {
		t.Fatal("expected stabilizationWindowSeconds to remain set")
	}
	if *mc.Spec.Autoscaling.Behavior.ScaleDown.StabilizationWindowSeconds != 600 {
		t.Errorf("expected stabilizationWindowSeconds=600 (preserved), got %d", *mc.Spec.Autoscaling.Behavior.ScaleDown.StabilizationWindowSeconds)
	}
}

func TestMemcachedDefaulting_AutoscalingDisabledNoDefaults(t *testing.T) {
	mc := &Memcached{
		Spec: MemcachedSpec{
			Autoscaling: &AutoscalingSpec{
				Enabled:     false,
				MaxReplicas: 10,
			},
		},
	}
	d := &MemcachedCustomDefaulter{}

	if err := d.Default(context.Background(), mc); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(mc.Spec.Autoscaling.Metrics) != 0 {
		t.Errorf("expected no metrics when disabled, got %d", len(mc.Spec.Autoscaling.Metrics))
	}
	if mc.Spec.Autoscaling.Behavior != nil {
		t.Error("expected no behavior when disabled")
	}
}

func TestMemcachedDefaulting_NilAutoscalingStaysNil(t *testing.T) {
	mc := &Memcached{
		Spec: MemcachedSpec{
			Autoscaling: nil,
		},
	}
	d := &MemcachedCustomDefaulter{}

	if err := d.Default(context.Background(), mc); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if mc.Spec.Autoscaling != nil {
		t.Error("expected autoscaling to remain nil (opt-in section)")
	}
}

func TestMemcachedDefaulting_AutoscalingIdempotent(t *testing.T) {
	mc := &Memcached{
		Spec: MemcachedSpec{
			Autoscaling: &AutoscalingSpec{
				Enabled:     true,
				MaxReplicas: 10,
			},
		},
	}
	d := &MemcachedCustomDefaulter{}

	if err := d.Default(context.Background(), mc); err != nil {
		t.Fatalf("first Default call: unexpected error: %v", err)
	}

	metricsLen := len(mc.Spec.Autoscaling.Metrics)
	cpuUtil := *mc.Spec.Autoscaling.Metrics[0].Resource.Target.AverageUtilization
	stabilization := *mc.Spec.Autoscaling.Behavior.ScaleDown.StabilizationWindowSeconds

	if err := d.Default(context.Background(), mc); err != nil {
		t.Fatalf("second Default call: unexpected error: %v", err)
	}

	if len(mc.Spec.Autoscaling.Metrics) != metricsLen {
		t.Errorf("idempotency: metrics count changed from %d to %d", metricsLen, len(mc.Spec.Autoscaling.Metrics))
	}
	if *mc.Spec.Autoscaling.Metrics[0].Resource.Target.AverageUtilization != cpuUtil {
		t.Errorf("idempotency: CPU utilization changed")
	}
	if *mc.Spec.Autoscaling.Behavior.ScaleDown.StabilizationWindowSeconds != stabilization {
		t.Errorf("idempotency: stabilization window changed")
	}
}

// --- Autoscaling + replicas interaction (B1 fix) ---

func TestMemcachedDefaulting_AutoscalingClearsReplicas(t *testing.T) {
	// When autoscaling is enabled, the webhook must clear spec.replicas
	// (the CRD schema default +kubebuilder:default=1 may have set it).
	replicas := int32(1)
	mc := &Memcached{
		Spec: MemcachedSpec{
			Replicas: &replicas,
			Autoscaling: &AutoscalingSpec{
				Enabled:     true,
				MaxReplicas: 10,
			},
		},
	}
	d := &MemcachedCustomDefaulter{}

	if err := d.Default(context.Background(), mc); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if mc.Spec.Replicas != nil {
		t.Errorf("expected spec.replicas to be nil when autoscaling is enabled, got %d", *mc.Spec.Replicas)
	}
}

func TestMemcachedDefaulting_NoAutoscalingStillDefaultsReplicas(t *testing.T) {
	mc := &Memcached{
		Spec: MemcachedSpec{
			Autoscaling: &AutoscalingSpec{
				Enabled: false,
			},
		},
	}
	d := &MemcachedCustomDefaulter{}

	if err := d.Default(context.Background(), mc); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if mc.Spec.Replicas == nil || *mc.Spec.Replicas != DefaultReplicas {
		t.Errorf("expected spec.replicas=%d when autoscaling is disabled, got %v", DefaultReplicas, mc.Spec.Replicas)
	}
}

// --- Chained default + validation tests (catch pipeline interaction bugs) ---

func TestDefaultThenValidate_AutoscalingEnabled(t *testing.T) {
	// Simulates the full admission pipeline: default then validate.
	// Before the B1 fix, this would fail because Default() set replicas=1,
	// then validation rejected replicas + autoscaling.enabled.
	mc := &Memcached{
		Spec: MemcachedSpec{
			Autoscaling: &AutoscalingSpec{
				Enabled:     true,
				MaxReplicas: 10,
			},
			Resources: &corev1.ResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceCPU: resource.MustParse("100m"),
				},
			},
		},
	}

	d := &MemcachedCustomDefaulter{}
	if err := d.Default(context.Background(), mc); err != nil {
		t.Fatalf("defaulting error: %v", err)
	}

	if err := validateMemcached(mc); err != nil {
		t.Errorf("expected no validation error after defaulting autoscaling CR, got: %v", err)
	}
}

func TestDefaultThenValidate_AutoscalingWithExplicitReplicas(t *testing.T) {
	// Autoscaling enabled + explicit replicas: defaulting clears replicas,
	// validation passes.
	replicas := int32(3)
	mc := &Memcached{
		Spec: MemcachedSpec{
			Replicas: &replicas,
			Autoscaling: &AutoscalingSpec{
				Enabled:     true,
				MaxReplicas: 10,
			},
			Resources: &corev1.ResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceCPU: resource.MustParse("100m"),
				},
			},
		},
	}

	d := &MemcachedCustomDefaulter{}
	if err := d.Default(context.Background(), mc); err != nil {
		t.Fatalf("defaulting error: %v", err)
	}

	if mc.Spec.Replicas != nil {
		t.Errorf("expected replicas to be cleared, got %d", *mc.Spec.Replicas)
	}

	if err := validateMemcached(mc); err != nil {
		t.Errorf("expected no validation error after defaulting, got: %v", err)
	}
}

func TestDefaultThenValidate_NoAutoscaling(t *testing.T) {
	// Without autoscaling, replicas should be defaulted to 1 and validation passes.
	mc := &Memcached{}

	d := &MemcachedCustomDefaulter{}
	if err := d.Default(context.Background(), mc); err != nil {
		t.Fatalf("defaulting error: %v", err)
	}

	if mc.Spec.Replicas == nil || *mc.Spec.Replicas != 1 {
		t.Errorf("expected replicas=1, got %v", mc.Spec.Replicas)
	}

	if err := validateMemcached(mc); err != nil {
		t.Errorf("expected no validation error after defaulting minimal CR, got: %v", err)
	}
}
