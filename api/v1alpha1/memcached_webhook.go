// Package v1alpha1 contains API Schema definitions for the memcached v1alpha1 API group.
package v1alpha1

import (
	"context"

	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// Default values applied by the webhook when fields are omitted.
const (
	DefaultReplicas                    = int32(1)
	DefaultImage                       = "memcached:1.6"
	DefaultMaxMemoryMB                 = int32(64)
	DefaultMaxConnections              = int32(1024)
	DefaultThreads                     = int32(4)
	DefaultMaxItemSize                 = "1m"
	DefaultExporterImage               = "prom/memcached-exporter:v0.15.4"
	DefaultServiceMonitorInterval      = "30s"
	DefaultServiceMonitorScrapeTimeout = "10s"
)

// log is for logging in this package.
var memcachedlog = logf.Log.WithName("memcached-resource")

// MemcachedCustomDefaulter applies defaults to Memcached resources.
type MemcachedCustomDefaulter struct{}

// Compile-time interface check.
var _ admission.Defaulter[*Memcached] = &MemcachedCustomDefaulter{}

// +kubebuilder:webhook:path=/mutate-memcached-c5c3-io-v1alpha1-memcached,mutating=true,failurePolicy=fail,sideEffects=None,groups=memcached.c5c3.io,resources=memcacheds,verbs=create;update,versions=v1alpha1,name=mmemcached-v1alpha1.kb.io,admissionReviewVersions=v1

// SetupMemcachedWebhookWithManager registers the defaulting and validation webhooks with the manager.
func SetupMemcachedWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr, &Memcached{}).
		WithDefaulter(&MemcachedCustomDefaulter{}).
		WithValidator(&MemcachedCustomValidator{}).
		Complete()
}

// Default implements admission.Defaulter and sets sensible defaults for omitted fields.
func (d *MemcachedCustomDefaulter) Default(ctx context.Context, mc *Memcached) error {
	memcachedlog.Info("defaulting", "name", mc.GetName())

	// REQ-001: Default replicas to 1 when nil.
	if mc.Spec.Replicas == nil {
		defaultReplicas := DefaultReplicas
		mc.Spec.Replicas = &defaultReplicas
	}

	// REQ-002: Default image when nil.
	if mc.Spec.Image == nil {
		defaultImage := DefaultImage
		mc.Spec.Image = &defaultImage
	}

	// REQ-003: Default memcached config fields.
	// The memcached section is always initialized because its fields are core operational parameters.
	if mc.Spec.Memcached == nil {
		mc.Spec.Memcached = &MemcachedConfig{}
	}
	if mc.Spec.Memcached.MaxMemoryMB == 0 {
		mc.Spec.Memcached.MaxMemoryMB = DefaultMaxMemoryMB
	}
	if mc.Spec.Memcached.MaxConnections == 0 {
		mc.Spec.Memcached.MaxConnections = DefaultMaxConnections
	}
	if mc.Spec.Memcached.Threads == 0 {
		mc.Spec.Memcached.Threads = DefaultThreads
	}
	if mc.Spec.Memcached.MaxItemSize == "" {
		mc.Spec.Memcached.MaxItemSize = DefaultMaxItemSize
	}
	// Verbosity defaults to 0, which is the Go zero value — no action needed.

	// REQ-004: Default monitoring sub-fields only when the monitoring section already exists.
	if mc.Spec.Monitoring != nil {
		if mc.Spec.Monitoring.ExporterImage == nil {
			defaultExporterImage := DefaultExporterImage
			mc.Spec.Monitoring.ExporterImage = &defaultExporterImage
		}

		// REQ-009: Default serviceMonitor sub-fields when the serviceMonitor section exists.
		if mc.Spec.Monitoring.ServiceMonitor != nil {
			if mc.Spec.Monitoring.ServiceMonitor.Interval == "" {
				mc.Spec.Monitoring.ServiceMonitor.Interval = DefaultServiceMonitorInterval
			}
			if mc.Spec.Monitoring.ServiceMonitor.ScrapeTimeout == "" {
				mc.Spec.Monitoring.ServiceMonitor.ScrapeTimeout = DefaultServiceMonitorScrapeTimeout
			}
		}
	}

	// REQ-005: Default highAvailability sub-fields only when the HA section already exists.
	if mc.Spec.HighAvailability != nil {
		if mc.Spec.HighAvailability.AntiAffinityPreset == nil {
			defaultPreset := AntiAffinityPresetSoft
			mc.Spec.HighAvailability.AntiAffinityPreset = &defaultPreset
		}
	}

	return nil
}
