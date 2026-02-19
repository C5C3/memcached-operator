// Package controller implements the reconciliation logic for the memcached-operator.
package controller

import (
	"context"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	policyv1 "k8s.io/api/policy/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/events"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	memcachedv1alpha1 "github.com/c5c3/memcached-operator/api/v1alpha1"
)

// MemcachedReconciler reconciles a Memcached object.
type MemcachedReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder events.EventRecorder
}

// +kubebuilder:rbac:groups=memcached.c5c3.io,resources=memcacheds,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=memcached.c5c3.io,resources=memcacheds/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=memcached.c5c3.io,resources=memcacheds/finalizers,verbs=update
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=services,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=policy,resources=poddisruptionbudgets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=networking.k8s.io,resources=networkpolicies,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=monitoring.coreos.com,resources=servicemonitors,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=events,verbs=create;patch

// Reconcile handles a reconciliation request for a Memcached resource.
func (r *MemcachedReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	memcached := &memcachedv1alpha1.Memcached{}
	if err := r.Get(ctx, req.NamespacedName, memcached); err != nil {
		if apierrors.IsNotFound(err) {
			logger.Info("Memcached resource not found; ignoring since it must have been deleted")
			return ctrl.Result{}, nil
		}
		logger.Error(err, "Failed to get Memcached resource")
		return ctrl.Result{}, err
	}

	logger.Info("Reconciling Memcached", "name", memcached.Name, "namespace", memcached.Namespace)

	if err := r.reconcileDeployment(ctx, memcached); err != nil {
		return ctrl.Result{}, err
	}

	if err := r.reconcileService(ctx, memcached); err != nil {
		return ctrl.Result{}, err
	}

	if err := r.reconcilePDB(ctx, memcached); err != nil {
		return ctrl.Result{}, err
	}

	if err := r.reconcileStatus(ctx, memcached); err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

// reconcileDeployment ensures the Deployment for the Memcached CR matches the desired state.
// It uses reconcileResource for idempotent create/update with conflict retries.
func (r *MemcachedReconciler) reconcileDeployment(ctx context.Context, mc *memcachedv1alpha1.Memcached) error {
	dep := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      mc.Name,
			Namespace: mc.Namespace,
		},
	}

	_, err := r.reconcileResource(ctx, mc, dep, func() error {
		constructDeployment(mc, dep)
		return nil
	}, "Deployment")
	return err
}

// reconcileService ensures the headless Service for the Memcached CR matches the desired state.
// It uses reconcileResource for idempotent create/update with conflict retries.
func (r *MemcachedReconciler) reconcileService(ctx context.Context, mc *memcachedv1alpha1.Memcached) error {
	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      mc.Name,
			Namespace: mc.Namespace,
		},
	}

	_, err := r.reconcileResource(ctx, mc, svc, func() error {
		constructService(mc, svc)
		return nil
	}, "Service")
	return err
}

// reconcilePDB ensures the PodDisruptionBudget for the Memcached CR matches the desired state.
// It skips reconciliation when PDB is not enabled in the CR spec.
func (r *MemcachedReconciler) reconcilePDB(ctx context.Context, mc *memcachedv1alpha1.Memcached) error {
	if !pdbEnabled(mc) {
		return nil
	}

	pdb := &policyv1.PodDisruptionBudget{
		ObjectMeta: metav1.ObjectMeta{
			Name:      mc.Name,
			Namespace: mc.Namespace,
		},
	}

	_, err := r.reconcileResource(ctx, mc, pdb, func() error {
		constructPDB(mc, pdb)
		return nil
	}, "PodDisruptionBudget")
	return err
}

// SetupWithManager sets up the controller with the Manager.
func (r *MemcachedReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&memcachedv1alpha1.Memcached{}).
		Owns(&appsv1.Deployment{}).
		Owns(&corev1.Service{}).
		Owns(&policyv1.PodDisruptionBudget{}).
		Owns(&networkingv1.NetworkPolicy{}).
		Named("memcached").
		Complete(r)
}
