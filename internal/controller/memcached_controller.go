// Package controller implements the reconciliation logic for the memcached-operator.
package controller

import (
	"context"

	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	memcachedv1alpha1 "github.com/c5c3/memcached-operator/api/v1alpha1"
)

// MemcachedReconciler reconciles a Memcached object.
type MemcachedReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=memcached.c5c3.io,resources=memcacheds,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=memcached.c5c3.io,resources=memcacheds/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=memcached.c5c3.io,resources=memcacheds/finalizers,verbs=update

// Reconcile handles a reconciliation request for a Memcached resource.
func (r *MemcachedReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	_ = log.FromContext(ctx)

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *MemcachedReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&memcachedv1alpha1.Memcached{}).
		Named("memcached").
		Complete(r)
}
