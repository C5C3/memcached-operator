// Package controller implements the reconciliation logic for the memcached-operator.
package controller

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	memcachedv1beta1 "github.com/c5c3/memcached-operator/api/v1beta1"
	"github.com/c5c3/memcached-operator/internal/metrics"
)

// maxConflictRetries is the number of times reconcileResource retries on
// resource version conflict errors before giving up.
const maxConflictRetries = 5

// reconcileResource performs an idempotent create-or-update for the given
// Kubernetes resource. It sets a controller owner reference to the Memcached CR
// and retries on resource version conflict errors (HTTP 409 Conflict) up to
// maxConflictRetries times.
//
// The mutate function is called to set the desired state on obj before each
// create/update attempt. It must not modify the object's namespace or name.
//
// resourceKind is used for log messages and error wrapping (e.g. "Deployment",
// "Service").
func (r *MemcachedReconciler) reconcileResource(
	ctx context.Context,
	mc *memcachedv1beta1.Memcached,
	obj client.Object,
	mutate func() error,
	resourceKind string,
) (controllerutil.OperationResult, error) {
	logger := log.FromContext(ctx)

	for attempt := range maxConflictRetries {
		result, err := controllerutil.CreateOrUpdate(ctx, r.Client, obj, func() error {
			if err := mutate(); err != nil {
				return err
			}
			return controllerutil.SetControllerReference(mc, obj, r.Scheme)
		})
		if err == nil {
			logger.Info(resourceKind+" reconciled",
				"name", obj.GetName(),
				"operation", result)
			r.emitEventForResult(mc, obj, resourceKind, result)
			metricResult := string(result)
			if result == controllerutil.OperationResultNone {
				metricResult = "unchanged"
			}
			metrics.RecordReconcileResource(resourceKind, metricResult)
			return result, nil
		}

		if !apierrors.IsConflict(err) {
			return "", fmt.Errorf("reconciling %s: %w", resourceKind, err)
		}

		logger.Info("Conflict retrying "+resourceKind+" reconciliation",
			"name", obj.GetName(),
			"attempt", attempt+1,
			"maxRetries", maxConflictRetries)
	}

	// All retries exhausted — return the conflict error.
	return "", apierrors.NewConflict(
		obj.GetObjectKind().GroupVersionKind().GroupVersion().WithResource(resourceKind).GroupResource(),
		obj.GetName(),
		fmt.Errorf("exceeded %d conflict retries", maxConflictRetries),
	)
}

// deleteOwnedResource deletes a resource if it exists, ignoring NotFound errors.
// This is used to clean up optional resources (PDB, ServiceMonitor, NetworkPolicy)
// when their feature is disabled in the CR spec.
func (r *MemcachedReconciler) deleteOwnedResource(ctx context.Context, obj client.Object, resourceKind string) error {
	logger := log.FromContext(ctx)
	if err := r.Delete(ctx, obj); err != nil {
		if apierrors.IsNotFound(err) {
			return nil
		}
		return fmt.Errorf("deleting %s: %w", resourceKind, err)
	}
	logger.Info(resourceKind+" deleted", "name", obj.GetName())
	return nil
}

// emitEventForResult emits a Kubernetes event on the Memcached CR for resource
// creation or update operations. No event is emitted for unchanged resources.
func (r *MemcachedReconciler) emitEventForResult(
	mc *memcachedv1beta1.Memcached,
	obj client.Object,
	resourceKind string,
	result controllerutil.OperationResult,
) {
	if r.Recorder == nil {
		return
	}

	switch result {
	case controllerutil.OperationResultCreated:
		r.Recorder.Eventf(mc, nil, corev1.EventTypeNormal, "Created",
			"Reconcile", "Created %s %s", resourceKind, obj.GetName())
	case controllerutil.OperationResultUpdated:
		r.Recorder.Eventf(mc, nil, corev1.EventTypeNormal, "Updated",
			"Reconcile", "Updated %s %s", resourceKind, obj.GetName())
	}
}
