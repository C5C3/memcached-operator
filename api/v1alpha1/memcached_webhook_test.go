// Package v1alpha1 contains unit tests for the Memcached defaulting webhook.
package v1alpha1

import (
	"context"
	"testing"
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
	image := "memcached:1.6.28"
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
	if *mc.Spec.Image != "memcached:1.6.28" {
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
					Interval: "15s",
				},
			},
		},
	}
	d := &MemcachedCustomDefaulter{}

	if err := d.Default(context.Background(), mc); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if mc.Spec.Monitoring.ServiceMonitor.Interval != "15s" {
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
	image := "memcached:1.6.28"
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
	if *mc.Spec.Image != "memcached:1.6.28" {
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
	if mc.Spec.Monitoring.ServiceMonitor.Interval != "15s" {
		t.Errorf("expected interval=15s, got %s", mc.Spec.Monitoring.ServiceMonitor.Interval)
	}
	if mc.Spec.Monitoring.ServiceMonitor.ScrapeTimeout != "5s" {
		t.Errorf("expected scrapeTimeout=5s, got %s", mc.Spec.Monitoring.ServiceMonitor.ScrapeTimeout)
	}
	if *mc.Spec.HighAvailability.AntiAffinityPreset != AntiAffinityPresetHard {
		t.Errorf("expected antiAffinityPreset=hard, got %s", *mc.Spec.HighAvailability.AntiAffinityPreset)
	}
}
