// Package controller implements the reconciliation logic for the memcached-operator.
package controller

import (
	"context"
	"fmt"
	"reflect"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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
// resourceKind is used for log messages and error wrapping (e.g. "StatefulSet",
// "Service").
//
// The cache holds only objects with the managed-by label. When the create fails
// with AlreadyExists, the object exists but the cache cannot see it: its label
// was removed, it belongs to someone else, or the cache lags behind. updateLive
// then reads it from the API server and updates it only if mc controls it.
func (r *MemcachedReconciler) reconcileResource(
	ctx context.Context,
	mc *memcachedv1beta1.Memcached,
	obj client.Object,
	mutate func() error,
	resourceKind string,
) (controllerutil.OperationResult, error) {
	logger := log.FromContext(ctx)

	mutateWithOwner := func() error {
		if err := mutate(); err != nil {
			return err
		}
		return controllerutil.SetControllerReference(mc, obj, r.Scheme)
	}

	for attempt := range maxConflictRetries {
		result, err := controllerutil.CreateOrUpdate(ctx, r.Client, obj, mutateWithOwner)
		if apierrors.IsAlreadyExists(err) {
			result, err = r.updateLive(ctx, mc, obj, mutateWithOwner, resourceKind, err)
		}
		if err == nil {
			logger.Info("Resource reconciled",
				"kind", resourceKind,
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

		logger.Info("Conflict retrying reconciliation",
			"kind", resourceKind,
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

// updateLive handles a create of obj that failed with createErr, an AlreadyExists
// error. It reads the object from the API server and, when mc controls it, applies
// mutate and updates it if anything changed. The update restores the managed-by
// label, so the object re-enters the cache. An object mc does not control is left
// alone: updateLive emits a Warning event and returns an error that wraps createErr.
func (r *MemcachedReconciler) updateLive(
	ctx context.Context,
	mc *memcachedv1beta1.Memcached,
	obj client.Object,
	mutate func() error,
	resourceKind string,
	createErr error,
) (controllerutil.OperationResult, error) {
	log.FromContext(ctx).Info("Resource exists but is not in the cache, reading it from the API server",
		"kind", resourceKind,
		"name", obj.GetName())

	// Read into a new object: obj holds the desired state of the failed create, and a
	// live read does not overwrite every field of its target the way a cache read does.
	live := reflect.New(reflect.TypeOf(obj).Elem()).Interface().(client.Object)
	if err := r.liveReader().Get(ctx, client.ObjectKeyFromObject(obj), live); err != nil {
		return controllerutil.OperationResultNone, fmt.Errorf("reading %s %s from the API server: %w", resourceKind, obj.GetName(), err)
	}

	if !metav1.IsControlledBy(live, mc) {
		if r.Recorder != nil {
			r.Recorder.Eventf(mc, nil, corev1.EventTypeWarning, "AlreadyExists", "Reconcile",
				"%s %s exists and is not controlled by this Memcached; not adopting it", resourceKind, obj.GetName())
		}
		return controllerutil.OperationResultNone, fmt.Errorf("%s %s/%s exists but is not controlled by Memcached %s: %w",
			resourceKind, obj.GetNamespace(), obj.GetName(), mc.Name, createErr)
	}

	// The caller's mutate closure writes to obj, so obj takes the live state.
	reflect.ValueOf(obj).Elem().Set(reflect.ValueOf(live).Elem())
	existing := obj.DeepCopyObject()
	if err := mutate(); err != nil {
		return controllerutil.OperationResultNone, err
	}
	if equality.Semantic.DeepEqual(existing, obj) {
		return controllerutil.OperationResultNone, nil
	}
	if err := r.Update(ctx, obj); err != nil {
		// obj carries the live resourceVersion now. A retry's create would be rejected
		// for setting one, instead of failing with AlreadyExists and coming back here
		// for a fresh read.
		obj.SetResourceVersion("")
		return controllerutil.OperationResultNone, err
	}
	return controllerutil.OperationResultUpdated, nil
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
	logger.Info("Resource deleted", "kind", resourceKind, "name", obj.GetName())
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
	case controllerutil.OperationResultNone,
		controllerutil.OperationResultUpdatedStatus,
		controllerutil.OperationResultUpdatedStatusOnly:
		// No event emitted for unchanged or status-only updates.
	}
}
