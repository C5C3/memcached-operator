// Package controller implements the reconciliation logic for the memcached-operator.
package controller

import (
	"context"
	"fmt"

	appsv1 "k8s.io/api/apps/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/log"

	memcachedv1alpha1 "github.com/c5c3/memcached-operator/api/v1alpha1"
)

// Condition type constants following Kubernetes API conventions.
const (
	// ConditionTypeAvailable indicates the Memcached instance has ready replicas serving traffic.
	ConditionTypeAvailable = "Available"

	// ConditionTypeProgressing indicates a rollout or scaling operation is in progress.
	ConditionTypeProgressing = "Progressing"

	// ConditionTypeDegraded indicates the Memcached instance has fewer ready replicas than desired.
	ConditionTypeDegraded = "Degraded"
)

// Condition reason constants.
const (
	ConditionReasonAvailable           = "Available"
	ConditionReasonUnavailable         = "Unavailable"
	ConditionReasonProgressing         = "Progressing"
	ConditionReasonProgressingComplete = "ProgressingComplete"
	ConditionReasonDegraded            = "Degraded"
	ConditionReasonNotDegraded         = "NotDegraded"
)

// computeConditions derives status conditions from the Memcached spec and the current Deployment status.
// If dep is nil (Deployment not yet created), it reports unavailable/progressing/degraded.
func computeConditions(mc *memcachedv1alpha1.Memcached, dep *appsv1.Deployment) []metav1.Condition {
	desiredReplicas := int32(1)
	if mc.Spec.Replicas != nil {
		desiredReplicas = *mc.Spec.Replicas
	}

	var readyReplicas, updatedReplicas, totalReplicas int32
	if dep != nil {
		readyReplicas = dep.Status.ReadyReplicas
		updatedReplicas = dep.Status.UpdatedReplicas
		totalReplicas = dep.Status.Replicas
	}

	now := metav1.Now()
	conditions := make([]metav1.Condition, 0, 3)

	// Available: true when at least one replica is ready, or when 0 replicas are desired.
	available := readyReplicas > 0 || desiredReplicas == 0
	availableStatus, availableReason := metav1.ConditionFalse, ConditionReasonUnavailable
	if available {
		availableStatus, availableReason = metav1.ConditionTrue, ConditionReasonAvailable
	}
	conditions = append(conditions, metav1.Condition{
		Type:               ConditionTypeAvailable,
		Status:             availableStatus,
		Reason:             availableReason,
		Message:            fmt.Sprintf("%d/%d replicas are ready", readyReplicas, desiredReplicas),
		LastTransitionTime: now,
		ObservedGeneration: mc.Generation,
	})

	// Progressing: true when the Deployment doesn't exist yet, when updatedReplicas < desired
	// (a rollout is underway), or when totalReplicas != desired (scaling in/out).
	progressing := dep == nil || updatedReplicas < desiredReplicas || totalReplicas != desiredReplicas
	progressingStatus, progressingReason := metav1.ConditionFalse, ConditionReasonProgressingComplete
	progressingMsg := fmt.Sprintf("All %d replicas are updated", desiredReplicas)
	if progressing {
		progressingStatus, progressingReason = metav1.ConditionTrue, ConditionReasonProgressing
		progressingMsg = "Waiting for deployment to be created"
		if dep != nil {
			progressingMsg = fmt.Sprintf("Rollout in progress: %d/%d replicas updated", updatedReplicas, desiredReplicas)
		}
	}
	conditions = append(conditions, metav1.Condition{
		Type:               ConditionTypeProgressing,
		Status:             progressingStatus,
		Reason:             progressingReason,
		Message:            progressingMsg,
		LastTransitionTime: now,
		ObservedGeneration: mc.Generation,
	})

	// Degraded: true when ready < desired and desired > 0.
	// (When dep is nil, readyReplicas is 0, so this naturally covers that case.)
	degraded := desiredReplicas > 0 && readyReplicas < desiredReplicas
	degradedStatus, degradedReason := metav1.ConditionFalse, ConditionReasonNotDegraded
	degradedMsg := fmt.Sprintf("All %d desired replicas are ready", desiredReplicas)
	if degraded {
		degradedStatus, degradedReason = metav1.ConditionTrue, ConditionReasonDegraded
		degradedMsg = "Waiting for deployment to be created"
		if dep != nil {
			degradedMsg = fmt.Sprintf("Only %d/%d replicas are ready", readyReplicas, desiredReplicas)
		}
	}
	conditions = append(conditions, metav1.Condition{
		Type:               ConditionTypeDegraded,
		Status:             degradedStatus,
		Reason:             degradedReason,
		Message:            degradedMsg,
		LastTransitionTime: now,
		ObservedGeneration: mc.Generation,
	})

	return conditions
}

// reconcileStatus fetches the owned Deployment, computes conditions, and updates the Memcached status.
func (r *MemcachedReconciler) reconcileStatus(ctx context.Context, mc *memcachedv1alpha1.Memcached) error {
	logger := log.FromContext(ctx)

	// Fetch the current Deployment.
	dep := &appsv1.Deployment{}
	err := r.Get(ctx, types.NamespacedName{Name: mc.Name, Namespace: mc.Namespace}, dep)
	if err != nil {
		if apierrors.IsNotFound(err) {
			dep = nil
		} else {
			return fmt.Errorf("fetching Deployment for status: %w", err)
		}
	}

	// Compute new conditions.
	newConditions := computeConditions(mc, dep)
	for _, c := range newConditions {
		meta.SetStatusCondition(&mc.Status.Conditions, c)
	}

	// Set readyReplicas.
	if dep != nil {
		mc.Status.ReadyReplicas = dep.Status.ReadyReplicas
	} else {
		mc.Status.ReadyReplicas = 0
	}

	// Set observedGeneration.
	mc.Status.ObservedGeneration = mc.Generation

	logger.Info("Updating Memcached status",
		"readyReplicas", mc.Status.ReadyReplicas,
		"observedGeneration", mc.Status.ObservedGeneration)

	if err := r.Status().Update(ctx, mc); err != nil {
		return fmt.Errorf("updating Memcached status: %w", err)
	}

	return nil
}
