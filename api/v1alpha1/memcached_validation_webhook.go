// Package v1alpha1 contains the validation webhook for Memcached resources.
package v1alpha1

import (
	"context"
	"fmt"

	autoscalingv2 "k8s.io/api/autoscaling/v2"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// memoryOverhead is the operational overhead (32Mi) added to maxMemoryMB when
// validating the container memory limit. This accounts for connections, threads,
// and internal data structures.
var memoryOverhead = resource.MustParse("32Mi")

// MemcachedCustomValidator validates Memcached resources.
type MemcachedCustomValidator struct{}

// Compile-time interface check.
var _ admission.Validator[*Memcached] = &MemcachedCustomValidator{}

// +kubebuilder:webhook:path=/validate-memcached-c5c3-io-v1alpha1-memcached,mutating=false,failurePolicy=fail,sideEffects=None,groups=memcached.c5c3.io,resources=memcacheds,verbs=create;update,versions=v1alpha1,name=vmemcached-v1alpha1.kb.io,admissionReviewVersions=v1

// ValidateCreate validates a Memcached resource on creation.
func (v *MemcachedCustomValidator) ValidateCreate(_ context.Context, obj *Memcached) (admission.Warnings, error) {
	memcachedlog.Info("validating create", "name", obj.GetName())
	return nil, validateMemcached(obj)
}

// ValidateUpdate validates a Memcached resource on update.
func (v *MemcachedCustomValidator) ValidateUpdate(_ context.Context, _ *Memcached, newObj *Memcached) (admission.Warnings, error) {
	memcachedlog.Info("validating update", "name", newObj.GetName())
	return nil, validateMemcached(newObj)
}

// ValidateDelete validates a Memcached resource on deletion (no-op).
func (v *MemcachedCustomValidator) ValidateDelete(_ context.Context, _ *Memcached) (admission.Warnings, error) {
	return nil, nil
}

// validateMemcached runs all validation rules and aggregates field errors.
func validateMemcached(mc *Memcached) error {
	var allErrs field.ErrorList

	allErrs = append(allErrs, validateMemoryLimit(mc)...)
	allErrs = append(allErrs, validatePDB(mc)...)
	allErrs = append(allErrs, validateGracefulShutdown(mc)...)
	allErrs = append(allErrs, validateSecuritySecretRefs(mc)...)
	allErrs = append(allErrs, validateAutoscaling(mc)...)

	if len(allErrs) == 0 {
		return nil
	}
	return apierrors.NewInvalid(
		mc.GroupVersionKind().GroupKind(),
		mc.GetName(),
		allErrs,
	)
}

// validatePDB validates PodDisruptionBudget rules:
// - minAvailable and maxUnavailable are mutually exclusive.
// - At least one of minAvailable or maxUnavailable must be set when PDB is enabled.
func validatePDB(mc *Memcached) field.ErrorList {
	var errs field.ErrorList

	if mc.Spec.HighAvailability == nil || mc.Spec.HighAvailability.PodDisruptionBudget == nil {
		return errs
	}

	pdb := mc.Spec.HighAvailability.PodDisruptionBudget
	pdbPath := field.NewPath("spec", "highAvailability", "podDisruptionBudget")

	if !pdb.Enabled {
		return errs
	}

	hasMin := pdb.MinAvailable != nil
	hasMax := pdb.MaxUnavailable != nil

	if hasMin && hasMax {
		errs = append(errs, field.Invalid(
			pdbPath,
			"",
			"minAvailable and maxUnavailable are mutually exclusive, specify only one",
		))
	}

	if !hasMin && !hasMax {
		errs = append(errs, field.Required(
			pdbPath,
			"one of minAvailable or maxUnavailable must be set when PDB is enabled",
		))
	}

	// REQ-002: minAvailable (integer) must be strictly less than replicas.
	if hasMin && !hasMax && pdb.MinAvailable.Type == intstr.Int && mc.Spec.Replicas != nil {
		if pdb.MinAvailable.IntVal >= *mc.Spec.Replicas {
			errs = append(errs, field.Invalid(
				pdbPath.Child("minAvailable"),
				pdb.MinAvailable.IntVal,
				fmt.Sprintf("minAvailable (%d) must be less than replicas (%d)", pdb.MinAvailable.IntVal, *mc.Spec.Replicas),
			))
		}
	}

	return errs
}

// validateSecuritySecretRefs validates that secret references are provided when
// security features are enabled:
// - SASL enabled requires credentialsSecretRef.name.
// - TLS enabled requires certificateSecretRef.name.
func validateSecuritySecretRefs(mc *Memcached) field.ErrorList {
	var errs field.ErrorList

	if mc.Spec.Security == nil {
		return errs
	}

	sec := mc.Spec.Security
	secPath := field.NewPath("spec", "security")

	if sec.SASL != nil && sec.SASL.Enabled && sec.SASL.CredentialsSecretRef.Name == "" {
		errs = append(errs, field.Required(
			secPath.Child("sasl", "credentialsSecretRef", "name"),
			"credentialsSecretRef.name is required when SASL is enabled",
		))
	}

	if sec.TLS != nil && sec.TLS.Enabled && sec.TLS.CertificateSecretRef.Name == "" {
		errs = append(errs, field.Required(
			secPath.Child("tls", "certificateSecretRef", "name"),
			"certificateSecretRef.name is required when TLS is enabled",
		))
	}

	return errs
}

// validateMemoryLimit validates that spec.resources.limits.memory is sufficient
// to accommodate spec.memcached.maxMemoryMB plus operational overhead (32Mi).
func validateMemoryLimit(mc *Memcached) field.ErrorList {
	var errs field.ErrorList

	if mc.Spec.Resources == nil || mc.Spec.Memcached == nil {
		return errs
	}

	memLimit, hasMemLimit := mc.Spec.Resources.Limits[corev1.ResourceMemory]
	if !hasMemLimit {
		return errs
	}

	// Required minimum: maxMemoryMB (converted to bytes) + 32Mi overhead.
	maxMemBytes := resource.NewQuantity(int64(mc.Spec.Memcached.MaxMemoryMB)*1024*1024, resource.BinarySI)
	maxMemBytes.Add(memoryOverhead)

	if memLimit.Cmp(*maxMemBytes) < 0 {
		errs = append(errs, field.Invalid(
			field.NewPath("spec", "resources", "limits", "memory"),
			memLimit.String(),
			fmt.Sprintf("memory limit must be at least %s (maxMemoryMB=%dMi + 32Mi overhead)", maxMemBytes.String(), mc.Spec.Memcached.MaxMemoryMB),
		))
	}

	return errs
}

// validateGracefulShutdown validates that terminationGracePeriodSeconds exceeds
// preStopDelaySeconds when graceful shutdown is enabled.
func validateGracefulShutdown(mc *Memcached) field.ErrorList {
	var errs field.ErrorList

	if mc.Spec.HighAvailability == nil || mc.Spec.HighAvailability.GracefulShutdown == nil {
		return errs
	}

	gs := mc.Spec.HighAvailability.GracefulShutdown
	if !gs.Enabled {
		return errs
	}

	if gs.TerminationGracePeriodSeconds <= int64(gs.PreStopDelaySeconds) {
		errs = append(errs, field.Invalid(
			field.NewPath("spec", "highAvailability", "gracefulShutdown", "terminationGracePeriodSeconds"),
			gs.TerminationGracePeriodSeconds,
			fmt.Sprintf("terminationGracePeriodSeconds (%d) must exceed preStopDelaySeconds (%d)", gs.TerminationGracePeriodSeconds, gs.PreStopDelaySeconds),
		))
	}

	return errs
}

// validateAutoscaling validates autoscaling configuration:
// - spec.replicas and autoscaling.enabled are mutually exclusive.
// - minReplicas must not exceed maxReplicas.
// - CPU utilization metrics require resources.requests.cpu.
func validateAutoscaling(mc *Memcached) field.ErrorList {
	var errs field.ErrorList

	if mc.Spec.Autoscaling == nil || !mc.Spec.Autoscaling.Enabled {
		return errs
	}

	as := mc.Spec.Autoscaling
	asPath := field.NewPath("spec", "autoscaling")

	// REQ-005: Mutual exclusivity — spec.replicas and autoscaling.enabled cannot coexist.
	if mc.Spec.Replicas != nil {
		errs = append(errs, field.Invalid(
			field.NewPath("spec", "replicas"),
			*mc.Spec.Replicas,
			"spec.replicas and spec.autoscaling.enabled are mutually exclusive",
		))
	}

	// REQ-006: minReplicas must not exceed maxReplicas.
	if as.MinReplicas != nil && *as.MinReplicas > as.MaxReplicas {
		errs = append(errs, field.Invalid(
			asPath.Child("minReplicas"),
			*as.MinReplicas,
			fmt.Sprintf("minReplicas (%d) must not exceed maxReplicas (%d)", *as.MinReplicas, as.MaxReplicas),
		))
	}

	// REQ-007: CPU utilization metrics require resources.requests.cpu.
	if hasCPUUtilizationMetric(as.Metrics) {
		if mc.Spec.Resources == nil || mc.Spec.Resources.Requests == nil {
			errs = append(errs, field.Required(
				field.NewPath("spec", "resources", "requests", "cpu"),
				"resources.requests.cpu is required when using CPU utilization metrics",
			))
		} else if _, ok := mc.Spec.Resources.Requests[corev1.ResourceCPU]; !ok {
			errs = append(errs, field.Required(
				field.NewPath("spec", "resources", "requests", "cpu"),
				"resources.requests.cpu is required when using CPU utilization metrics",
			))
		}
	}

	return errs
}

// hasCPUUtilizationMetric returns true if any metric in the slice is a CPU Resource
// metric with a Utilization target type.
func hasCPUUtilizationMetric(metrics []autoscalingv2.MetricSpec) bool {
	for i := range metrics {
		m := &metrics[i]
		if m.Type == autoscalingv2.ResourceMetricSourceType &&
			m.Resource != nil &&
			m.Resource.Name == corev1.ResourceCPU &&
			m.Resource.Target.Type == autoscalingv2.UtilizationMetricType {
			return true
		}
	}
	return false
}

// Ensure the runtime.Object interface constraint is satisfied (used by apierrors.NewInvalid).
var _ runtime.Object = &Memcached{}
