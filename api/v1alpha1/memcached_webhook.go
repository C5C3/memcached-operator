// Package v1alpha1 contains API Schema definitions for the memcached v1alpha1 API group.
package v1alpha1

import (
	"context"

	autoscalingv2 "k8s.io/api/autoscaling/v2"
	corev1 "k8s.io/api/core/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// Default values applied by the webhook when fields are omitted.
const (
	DefaultReplicas                      = int32(1)
	DefaultImage                         = "memcached:1.6"
	DefaultMaxMemoryMB                   = int32(64)
	DefaultMaxConnections                = int32(1024)
	DefaultThreads                       = int32(4)
	DefaultMaxItemSize                   = "1m"
	DefaultExporterImage                 = "prom/memcached-exporter:v0.15.4"
	DefaultServiceMonitorInterval        = "30s"
	DefaultServiceMonitorScrapeTimeout   = "10s"
	DefaultAutoscalingCPUUtilization     = int32(80)
	DefaultScaleDownStabilizationSeconds = int32(300)
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

	// REQ-001: Default replicas to 1 when nil, unless autoscaling is enabled
	// (spec.replicas and autoscaling.enabled are mutually exclusive).
	autoscalingEnabled := mc.Spec.Autoscaling != nil && mc.Spec.Autoscaling.Enabled
	if mc.Spec.Replicas == nil && !autoscalingEnabled {
		defaultReplicas := DefaultReplicas
		mc.Spec.Replicas = &defaultReplicas
	}

	// REQ-002: Default image when nil.
	if mc.Spec.Image == nil {
		defaultImage := DefaultImage
		mc.Spec.Image = &defaultImage
	}

	defaultMemcachedConfig(mc)
	defaultMonitoring(mc)

	// REQ-005: Default highAvailability sub-fields only when the HA section already exists.
	if mc.Spec.HighAvailability != nil {
		if mc.Spec.HighAvailability.AntiAffinityPreset == nil {
			defaultPreset := AntiAffinityPresetSoft
			mc.Spec.HighAvailability.AntiAffinityPreset = &defaultPreset
		}
	}

	if autoscalingEnabled {
		defaultAutoscaling(mc)
	}

	return nil
}

// defaultMemcachedConfig initializes the memcached section and populates zero-valued fields.
// The memcached section is always initialized because its fields are core operational parameters.
func defaultMemcachedConfig(mc *Memcached) {
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
}

// defaultMonitoring sets defaults for monitoring sub-fields only when the monitoring section already exists.
func defaultMonitoring(mc *Memcached) {
	if mc.Spec.Monitoring == nil {
		return
	}
	if mc.Spec.Monitoring.ExporterImage == nil {
		defaultExporterImage := DefaultExporterImage
		mc.Spec.Monitoring.ExporterImage = &defaultExporterImage
	}
	if mc.Spec.Monitoring.ServiceMonitor != nil {
		if mc.Spec.Monitoring.ServiceMonitor.Interval == "" {
			mc.Spec.Monitoring.ServiceMonitor.Interval = DefaultServiceMonitorInterval
		}
		if mc.Spec.Monitoring.ServiceMonitor.ScrapeTimeout == "" {
			mc.Spec.Monitoring.ServiceMonitor.ScrapeTimeout = DefaultServiceMonitorScrapeTimeout
		}
	}
}

// defaultAutoscaling sets defaults for autoscaling sub-fields and clears spec.replicas.
// Must only be called when autoscaling is enabled.
func defaultAutoscaling(mc *Memcached) {
	// Clear spec.replicas — it is mutually exclusive with autoscaling.enabled.
	// The CRD schema default (+kubebuilder:default=1) may have set replicas before
	// the webhook runs; clear it so validation does not reject the CR.
	mc.Spec.Replicas = nil

	// Inject 80% CPU utilization metric when Metrics is empty.
	if len(mc.Spec.Autoscaling.Metrics) == 0 {
		cpuUtilization := DefaultAutoscalingCPUUtilization
		mc.Spec.Autoscaling.Metrics = []autoscalingv2.MetricSpec{
			{
				Type: autoscalingv2.ResourceMetricSourceType,
				Resource: &autoscalingv2.ResourceMetricSource{
					Name: corev1.ResourceCPU,
					Target: autoscalingv2.MetricTarget{
						Type:               autoscalingv2.UtilizationMetricType,
						AverageUtilization: &cpuUtilization,
					},
				},
			},
		}
	}

	// Inject scaleDown stabilization window when Behavior is nil.
	if mc.Spec.Autoscaling.Behavior == nil {
		stabilizationWindow := DefaultScaleDownStabilizationSeconds
		mc.Spec.Autoscaling.Behavior = &autoscalingv2.HorizontalPodAutoscalerBehavior{
			ScaleDown: &autoscalingv2.HPAScalingRules{
				StabilizationWindowSeconds: &stabilizationWindow,
			},
		}
	}
}
