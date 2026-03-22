// Package controller implements the reconciliation logic for the memcached-operator.
package controller

import (
	"context"
	"fmt"
	"strings"

	appsv1 "k8s.io/api/apps/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/log"

	memcachedv1beta1 "github.com/c5c3/memcached-operator/api/v1beta1"
)

// Condition type constants following Kubernetes API conventions.
const (
	// ConditionTypeAvailable indicates the Memcached instance has ready replicas serving traffic.
	ConditionTypeAvailable = "Available"

	// ConditionTypeProgressing indicates a rollout or scaling operation is in progress.
	ConditionTypeProgressing = "Progressing"

	// ConditionTypeDegraded indicates the Memcached instance has fewer ready replicas than desired.
	ConditionTypeDegraded = "Degraded"

	// ConditionTypeReady indicates all desired replicas are ready and the instance is fully operational.
	ConditionTypeReady = "Ready"
)

// Condition reason constants.
const (
	ConditionReasonAvailable           = "Available"
	ConditionReasonUnavailable         = "Unavailable"
	ConditionReasonProgressing         = "Progressing"
	ConditionReasonProgressingComplete = "ProgressingComplete"
	ConditionReasonDegraded            = "Degraded"
	ConditionReasonNotDegraded         = "NotDegraded"
	ConditionReasonSecretNotFound      = "SecretNotFound"
	ConditionReasonReady               = "MemcachedReady"
	ConditionReasonNotReady            = "MemcachedNotReady"
)

const msgWaitingForDeployment = "Waiting for deployment to be created"

// replicaState holds the computed replica counts used across condition builders.
type replicaState struct {
	desired int32
	ready   int32
	updated int32
	total   int32
	hasDep  bool
	hpaMode bool
	gen     int64
	now     metav1.Time
}

// newReplicaState computes the replica state from the Memcached spec and Deployment status.
func newReplicaState(mc *memcachedv1beta1.Memcached, dep *appsv1.Deployment, hpaActive bool) replicaState {
	rs := replicaState{
		hasDep:  dep != nil,
		hpaMode: hpaActive,
		gen:     mc.Generation,
		now:     metav1.Now(),
	}

	if hpaActive && dep != nil {
		rs.desired = dep.Status.Replicas
	} else {
		rs.desired = int32(1)
		if mc.Spec.Replicas != nil {
			rs.desired = *mc.Spec.Replicas
		}
	}

	if dep != nil {
		rs.ready = dep.Status.ReadyReplicas
		rs.updated = dep.Status.UpdatedReplicas
		rs.total = dep.Status.Replicas
	}

	return rs
}

func (rs replicaState) availableCondition() metav1.Condition {
	available := rs.ready > 0
	status, reason := metav1.ConditionFalse, ConditionReasonUnavailable
	if available {
		status, reason = metav1.ConditionTrue, ConditionReasonAvailable
	}
	msg := fmt.Sprintf("%d/%d replicas are ready", rs.ready, rs.desired)
	if rs.hpaMode {
		msg += " (HPA-managed)"
	}
	return metav1.Condition{
		Type: ConditionTypeAvailable, Status: status, Reason: reason,
		Message: msg, LastTransitionTime: rs.now, ObservedGeneration: rs.gen,
	}
}

func (rs replicaState) progressingCondition() metav1.Condition {
	progressing := !rs.hasDep || rs.updated < rs.desired || rs.total != rs.desired
	status, reason := metav1.ConditionFalse, ConditionReasonProgressingComplete
	msg := fmt.Sprintf("All %d replicas are updated", rs.desired)
	if progressing {
		status, reason = metav1.ConditionTrue, ConditionReasonProgressing
		msg = msgWaitingForDeployment
		if rs.hasDep {
			msg = fmt.Sprintf("Rollout in progress: %d/%d replicas updated", rs.updated, rs.desired)
		}
	}
	return metav1.Condition{
		Type: ConditionTypeProgressing, Status: status, Reason: reason,
		Message: msg, LastTransitionTime: rs.now, ObservedGeneration: rs.gen,
	}
}

func (rs replicaState) degradedCondition(missingSecrets []string) metav1.Condition {
	var status metav1.ConditionStatus
	var reason, msg string
	if len(missingSecrets) > 0 {
		status = metav1.ConditionTrue
		reason = ConditionReasonSecretNotFound
		msg = fmt.Sprintf("Referenced Secrets not found: %s", strings.Join(missingSecrets, ", "))
	} else {
		degraded := rs.desired > 0 && rs.ready < rs.desired
		status, reason = metav1.ConditionFalse, ConditionReasonNotDegraded
		msg = fmt.Sprintf("All %d desired replicas are ready", rs.desired)
		if degraded {
			status, reason = metav1.ConditionTrue, ConditionReasonDegraded
			msg = msgWaitingForDeployment
			if rs.hasDep {
				msg = fmt.Sprintf("Only %d/%d replicas are ready", rs.ready, rs.desired)
			}
		}
	}
	return metav1.Condition{
		Type: ConditionTypeDegraded, Status: status, Reason: reason,
		Message: msg, LastTransitionTime: rs.now, ObservedGeneration: rs.gen,
	}
}

func (rs replicaState) readyCondition() metav1.Condition {
	ready := rs.desired > 0 && rs.ready == rs.desired
	status, reason := metav1.ConditionFalse, ConditionReasonNotReady
	var msg string
	if ready {
		status, reason = metav1.ConditionTrue, ConditionReasonReady
		msg = fmt.Sprintf("All %d replicas are ready", rs.desired)
	} else if rs.desired == 0 {
		msg = "Instance has zero desired replicas"
	} else if !rs.hasDep {
		msg = msgWaitingForDeployment
	} else {
		msg = fmt.Sprintf("%d/%d replicas are ready", rs.ready, rs.desired)
	}
	return metav1.Condition{
		Type: ConditionTypeReady, Status: status, Reason: reason,
		Message: msg, LastTransitionTime: rs.now, ObservedGeneration: rs.gen,
	}
}

// computeConditions derives status conditions from the Memcached spec and the current Deployment status.
// If dep is nil (Deployment not yet created), it reports unavailable/progressing/degraded.
// When missingSecrets is non-empty, the Degraded condition is set to SecretNotFound regardless of replica counts.
// When hpaActive is true, the desired replica count is sourced from the Deployment status (HPA-managed)
// rather than from mc.Spec.Replicas.
func computeConditions(mc *memcachedv1beta1.Memcached, dep *appsv1.Deployment, missingSecrets []string, hpaActive bool) []metav1.Condition {
	rs := newReplicaState(mc, dep, hpaActive)
	return []metav1.Condition{
		rs.availableCondition(),
		rs.progressingCondition(),
		rs.degradedCondition(missingSecrets),
		rs.readyCondition(),
	}
}

// reconcileStatus fetches the owned Deployment, computes conditions, and updates the Memcached status.
// missingSecrets is the list of Secret names that could not be found during deployment reconciliation.
func (r *MemcachedReconciler) reconcileStatus(ctx context.Context, mc *memcachedv1beta1.Memcached, missingSecrets []string) error {
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
	newConditions := computeConditions(mc, dep, missingSecrets, mc.IsAutoscalingEnabled())
	for _, c := range newConditions {
		meta.SetStatusCondition(&mc.Status.Conditions, c)
	}

	// Populate serverList when Ready=True (REQ-004, MO-0056).
	readyCond := meta.FindStatusCondition(mc.Status.Conditions, ConditionTypeReady)
	if readyCond != nil && readyCond.Status == metav1.ConditionTrue {
		mc.Status.ServerList = []string{fmt.Sprintf("%s.%s:%d", mc.Name, mc.Namespace, PortMemcached)}
	} else {
		mc.Status.ServerList = nil
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
		"observedGeneration", mc.Status.ObservedGeneration,
		"serverList", mc.Status.ServerList)

	if err := r.Status().Update(ctx, mc); err != nil {
		return fmt.Errorf("updating Memcached status: %w", err)
	}

	return nil
}
