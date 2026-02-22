package v1alpha1

import (
	"fmt"

	"sigs.k8s.io/controller-runtime/pkg/conversion"

	v1beta1 "github.com/c5c3/memcached-operator/api/v1beta1"
)

// ConvertTo converts this v1alpha1.Memcached (spoke) to the hub version (v1beta1).
func (src *Memcached) ConvertTo(dstRaw conversion.Hub) error {
	dst, ok := dstRaw.(*v1beta1.Memcached)
	if !ok {
		return fmt.Errorf("expected *v1beta1.Memcached but got %T", dstRaw)
	}
	dst.ObjectMeta = src.ObjectMeta

	// Spec — field-by-field copy (types are structurally identical).
	dst.Spec.Replicas = src.Spec.Replicas
	dst.Spec.Image = src.Spec.Image
	dst.Spec.Resources = src.Spec.Resources

	if src.Spec.Memcached != nil {
		m := v1beta1.MemcachedConfig(*src.Spec.Memcached)
		dst.Spec.Memcached = &m
	}

	if src.Spec.HighAvailability != nil {
		ha := convertHighAvailabilityTo(src.Spec.HighAvailability)
		dst.Spec.HighAvailability = &ha
	}

	if src.Spec.Monitoring != nil {
		mon := convertMonitoringTo(src.Spec.Monitoring)
		dst.Spec.Monitoring = &mon
	}

	if src.Spec.Security != nil {
		sec := convertSecurityTo(src.Spec.Security)
		dst.Spec.Security = &sec
	}

	if src.Spec.Autoscaling != nil {
		as := v1beta1.AutoscalingSpec(*src.Spec.Autoscaling)
		dst.Spec.Autoscaling = &as
	}

	if src.Spec.Service != nil {
		svc := v1beta1.ServiceSpec(*src.Spec.Service)
		dst.Spec.Service = &svc
	}

	// Status
	dst.Status.Conditions = src.Status.Conditions
	dst.Status.ReadyReplicas = src.Status.ReadyReplicas
	dst.Status.ObservedGeneration = src.Status.ObservedGeneration

	return nil
}

// ConvertFrom converts from the hub version (v1beta1) to this v1alpha1.Memcached (spoke).
func (dst *Memcached) ConvertFrom(srcRaw conversion.Hub) error {
	src, ok := srcRaw.(*v1beta1.Memcached)
	if !ok {
		return fmt.Errorf("expected *v1beta1.Memcached but got %T", srcRaw)
	}
	dst.ObjectMeta = src.ObjectMeta

	// Spec — field-by-field copy (types are structurally identical).
	dst.Spec.Replicas = src.Spec.Replicas
	dst.Spec.Image = src.Spec.Image
	dst.Spec.Resources = src.Spec.Resources

	if src.Spec.Memcached != nil {
		m := MemcachedConfig(*src.Spec.Memcached)
		dst.Spec.Memcached = &m
	}

	if src.Spec.HighAvailability != nil {
		ha := convertHighAvailabilityFrom(src.Spec.HighAvailability)
		dst.Spec.HighAvailability = &ha
	}

	if src.Spec.Monitoring != nil {
		mon := convertMonitoringFrom(src.Spec.Monitoring)
		dst.Spec.Monitoring = &mon
	}

	if src.Spec.Security != nil {
		sec := convertSecurityFrom(src.Spec.Security)
		dst.Spec.Security = &sec
	}

	if src.Spec.Autoscaling != nil {
		as := AutoscalingSpec(*src.Spec.Autoscaling)
		dst.Spec.Autoscaling = &as
	}

	if src.Spec.Service != nil {
		svc := ServiceSpec(*src.Spec.Service)
		dst.Spec.Service = &svc
	}

	// Status
	dst.Status.Conditions = src.Status.Conditions
	dst.Status.ReadyReplicas = src.Status.ReadyReplicas
	dst.Status.ObservedGeneration = src.Status.ObservedGeneration

	return nil
}

// --- Helper converters for nested structs with pointer fields ---

func convertHighAvailabilityTo(src *HighAvailabilitySpec) v1beta1.HighAvailabilitySpec {
	dst := v1beta1.HighAvailabilitySpec{
		TopologySpreadConstraints: src.TopologySpreadConstraints,
	}
	if src.AntiAffinityPreset != nil {
		v := v1beta1.AntiAffinityPreset(*src.AntiAffinityPreset)
		dst.AntiAffinityPreset = &v
	}
	if src.PodDisruptionBudget != nil {
		p := v1beta1.PDBSpec(*src.PodDisruptionBudget)
		dst.PodDisruptionBudget = &p
	}
	if src.GracefulShutdown != nil {
		g := v1beta1.GracefulShutdownSpec(*src.GracefulShutdown)
		dst.GracefulShutdown = &g
	}
	return dst
}

func convertHighAvailabilityFrom(src *v1beta1.HighAvailabilitySpec) HighAvailabilitySpec {
	dst := HighAvailabilitySpec{
		TopologySpreadConstraints: src.TopologySpreadConstraints,
	}
	if src.AntiAffinityPreset != nil {
		v := AntiAffinityPreset(*src.AntiAffinityPreset)
		dst.AntiAffinityPreset = &v
	}
	if src.PodDisruptionBudget != nil {
		p := PDBSpec(*src.PodDisruptionBudget)
		dst.PodDisruptionBudget = &p
	}
	if src.GracefulShutdown != nil {
		g := GracefulShutdownSpec(*src.GracefulShutdown)
		dst.GracefulShutdown = &g
	}
	return dst
}

func convertMonitoringTo(src *MonitoringSpec) v1beta1.MonitoringSpec {
	dst := v1beta1.MonitoringSpec{
		Enabled:           src.Enabled,
		ExporterImage:     src.ExporterImage,
		ExporterResources: src.ExporterResources,
	}
	if src.ServiceMonitor != nil {
		sm := v1beta1.ServiceMonitorSpec(*src.ServiceMonitor)
		dst.ServiceMonitor = &sm
	}
	return dst
}

func convertMonitoringFrom(src *v1beta1.MonitoringSpec) MonitoringSpec {
	dst := MonitoringSpec{
		Enabled:           src.Enabled,
		ExporterImage:     src.ExporterImage,
		ExporterResources: src.ExporterResources,
	}
	if src.ServiceMonitor != nil {
		sm := ServiceMonitorSpec(*src.ServiceMonitor)
		dst.ServiceMonitor = &sm
	}
	return dst
}

func convertSecurityTo(src *SecuritySpec) v1beta1.SecuritySpec {
	dst := v1beta1.SecuritySpec{
		PodSecurityContext:       src.PodSecurityContext,
		ContainerSecurityContext: src.ContainerSecurityContext,
	}
	if src.SASL != nil {
		s := v1beta1.SASLSpec(*src.SASL)
		dst.SASL = &s
	}
	if src.TLS != nil {
		t := v1beta1.TLSSpec(*src.TLS)
		dst.TLS = &t
	}
	if src.NetworkPolicy != nil {
		n := v1beta1.NetworkPolicySpec(*src.NetworkPolicy)
		dst.NetworkPolicy = &n
	}
	return dst
}

func convertSecurityFrom(src *v1beta1.SecuritySpec) SecuritySpec {
	dst := SecuritySpec{
		PodSecurityContext:       src.PodSecurityContext,
		ContainerSecurityContext: src.ContainerSecurityContext,
	}
	if src.SASL != nil {
		s := SASLSpec(*src.SASL)
		dst.SASL = &s
	}
	if src.TLS != nil {
		t := TLSSpec(*src.TLS)
		dst.TLS = &t
	}
	if src.NetworkPolicy != nil {
		n := NetworkPolicySpec(*src.NetworkPolicy)
		dst.NetworkPolicy = &n
	}
	return dst
}
