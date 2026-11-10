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

const msgWaitingForStatefulSet = "Waiting for StatefulSet to be created"

// replicaState holds the computed replica counts used across condition builders.
type replicaState struct {
	desired     int32
	ready       int32
	updated     int32
	total       int32
	hasWorkload bool
	hpaMode     bool
	gen         int64
	now         metav1.Time
}

// newReplicaState computes the replica state from the Memcached spec and StatefulSet status.
func newReplicaState(mc *memcachedv1beta1.Memcached, sts *appsv1.StatefulSet, hpaActive bool) replicaState {
	rs := replicaState{
		hasWorkload: sts != nil,
		hpaMode:     hpaActive,
		gen:         mc.Generation,
		now:         metav1.Now(),
	}

	if hpaActive && sts != nil {
		rs.desired = sts.Status.Replicas
	} else {
		rs.desired = int32(1)
		if mc.Spec.Replicas != nil {
			rs.desired = *mc.Spec.Replicas
		}
	}

	if sts != nil {
		rs.ready = sts.Status.ReadyReplicas
		rs.updated = sts.Status.UpdatedReplicas
		rs.total = sts.Status.Replicas
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
	progressing := !rs.hasWorkload || rs.updated < rs.desired || rs.total != rs.desired
	status, reason := metav1.ConditionFalse, ConditionReasonProgressingComplete
	msg := fmt.Sprintf("All %d replicas are updated", rs.desired)
	if progressing {
		status, reason = metav1.ConditionTrue, ConditionReasonProgressing
		msg = msgWaitingForStatefulSet
		if rs.hasWorkload {
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
			msg = msgWaitingForStatefulSet
			if rs.hasWorkload {
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
	} else if !rs.hasWorkload {
		msg = msgWaitingForStatefulSet
	} else {
		msg = fmt.Sprintf("%d/%d replicas are ready", rs.ready, rs.desired)
	}
	return metav1.Condition{
		Type: ConditionTypeReady, Status: status, Reason: reason,
		Message: msg, LastTransitionTime: rs.now, ObservedGeneration: rs.gen,
	}
}

// computeConditions derives status conditions from the Memcached spec and the current StatefulSet status.
// If sts is nil (StatefulSet not yet created), it reports unavailable/progressing/degraded.
// When missingSecrets is non-empty, the Degraded condition is set to SecretNotFound regardless of replica counts.
// When hpaActive is true, the desired replica count is sourced from the StatefulSet status (HPA-managed)
// rather than from mc.Spec.Replicas.
func computeConditions(mc *memcachedv1beta1.Memcached, sts *appsv1.StatefulSet, missingSecrets []string, hpaActive bool) []metav1.Condition {
	rs := newReplicaState(mc, sts, hpaActive)
	return []metav1.Condition{
		rs.availableCondition(),
		rs.progressingCondition(),
		rs.degradedCondition(missingSecrets),
		rs.readyCondition(),
	}
}

// buildServerList returns one "host:port" entry per pod of the StatefulSet named workloadName,
// ordered by pod index: "<workloadName>-<i>.<serviceName>.<namespace>.svc:11211" for i from 0 to
// replicas-1. It returns nil when replicas is zero or negative.
func buildServerList(workloadName, serviceName, namespace string, replicas int32) []string {
	if replicas <= 0 {
		return nil
	}
	servers := make([]string, 0, replicas)
	for i := range replicas {
		servers = append(servers, fmt.Sprintf("%s-%d.%s.%s.svc:%d", workloadName, i, serviceName, namespace, PortMemcached))
	}
	return servers
}

// reconcileStatus fetches the owned StatefulSet, computes conditions, and updates the Memcached status.
// missingSecrets is the list of Secret names that could not be found during StatefulSet reconciliation.
func (r *MemcachedReconciler) reconcileStatus(ctx context.Context, mc *memcachedv1beta1.Memcached, missingSecrets []string) error {
	logger := log.FromContext(ctx)

	// Fetch the current StatefulSet.
	sts := &appsv1.StatefulSet{}
	err := r.Get(ctx, types.NamespacedName{Name: statefulSetName(mc.Name), Namespace: mc.Namespace}, sts)
	if err != nil {
		if apierrors.IsNotFound(err) {
			sts = nil
		} else {
			return fmt.Errorf("fetching StatefulSet for status: %w", err)
		}
	}

	// Compute new conditions.
	newConditions := computeConditions(mc, sts, missingSecrets, mc.IsAutoscalingEnabled())
	for _, c := range newConditions {
		meta.SetStatusCondition(&mc.Status.Conditions, c)
	}

	// Publish one entry per pod. The list follows the StatefulSet's replica count, which the HPA
	// may change, and does not depend on pod readiness.
	if sts != nil {
		replicas := memcachedv1beta1.DefaultReplicas
		if sts.Spec.Replicas != nil {
			replicas = *sts.Spec.Replicas
		}
		mc.Status.ServerList = buildServerList(sts.Name, mc.Name, mc.Namespace, replicas)
	} else {
		mc.Status.ServerList = nil
	}

	// Set readyReplicas.
	if sts != nil {
		mc.Status.ReadyReplicas = sts.Status.ReadyReplicas
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
