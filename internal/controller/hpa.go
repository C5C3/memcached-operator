// Package controller implements the reconciliation logic for the memcached-operator.
package controller

import (
	autoscalingv2 "k8s.io/api/autoscaling/v2"

	memcachedv1beta1 "github.com/c5c3/memcached-operator/api/v1beta1"
)

// constructHPA sets the desired state of the HorizontalPodAutoscaler based on the Memcached CR spec.
// It mutates hpa in-place and is designed to be called from within controllerutil.CreateOrUpdate.
//
// Precondition: mc.Spec.Autoscaling must not be nil (callers must guard with hpaEnabled).
func constructHPA(mc *memcachedv1beta1.Memcached, hpa *autoscalingv2.HorizontalPodAutoscaler) {
	hpa.Labels = labelsForMemcached(mc.Name)

	hpa.Spec.ScaleTargetRef = autoscalingv2.CrossVersionObjectReference{
		APIVersion: "apps/v1",
		Kind:       "Deployment",
		Name:       mc.Name,
	}

	hpa.Spec.MinReplicas = mc.Spec.Autoscaling.MinReplicas
	hpa.Spec.MaxReplicas = mc.Spec.Autoscaling.MaxReplicas
	hpa.Spec.Metrics = mc.Spec.Autoscaling.Metrics
	hpa.Spec.Behavior = mc.Spec.Autoscaling.Behavior
}

// hpaEnabled returns true only when HPA creation is explicitly enabled in the CR spec.
func hpaEnabled(mc *memcachedv1beta1.Memcached) bool {
	return mc.Spec.Autoscaling != nil && mc.Spec.Autoscaling.Enabled
}
