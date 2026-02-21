// Package controller implements the reconciliation logic for the memcached-operator.
package controller

import (
	"context"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	autoscalingv2 "k8s.io/api/autoscaling/v2"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	policyv1 "k8s.io/api/policy/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/events"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"

	monitoringv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"

	memcachedv1alpha1 "github.com/c5c3/memcached-operator/api/v1alpha1"
	"github.com/c5c3/memcached-operator/internal/metrics"
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
// +kubebuilder:rbac:groups=autoscaling,resources=horizontalpodautoscalers,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=events,verbs=create;patch
// +kubebuilder:rbac:groups=events.k8s.io,resources=events,verbs=create;patch

// Reconcile handles a reconciliation request for a Memcached resource.
func (r *MemcachedReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	memcached := &memcachedv1alpha1.Memcached{}
	if err := r.Get(ctx, req.NamespacedName, memcached); err != nil {
		if apierrors.IsNotFound(err) {
			logger.Info("Memcached resource not found; ignoring since it must have been deleted")
			metrics.ResetInstanceMetrics(req.Name, req.Namespace)
			return ctrl.Result{}, nil
		}
		logger.Error(err, "Failed to get Memcached resource")
		return ctrl.Result{}, err
	}

	logger.Info("Reconciling Memcached", "name", memcached.Name, "namespace", memcached.Namespace)

	reconcileStart := time.Now()
	var reconcileErr error
	defer func() {
		result := "success"
		if reconcileErr != nil {
			result = "error"
		}
		metrics.RecordReconciliation(memcached.Name, memcached.Namespace, result, time.Since(reconcileStart))
	}()

	// Record instance info gauge with current spec values.
	image := "memcached:1.6"
	if memcached.Spec.Image != nil {
		image = *memcached.Spec.Image
	}
	desiredReplicas := int32(1)
	if memcached.Spec.Replicas != nil {
		desiredReplicas = *memcached.Spec.Replicas
	}
	metrics.RecordInstanceInfo(memcached.Name, memcached.Namespace, image, desiredReplicas)

	var missingSecrets []string
	missingSecrets, reconcileErr = r.reconcileDeployment(ctx, memcached)
	if reconcileErr != nil {
		return ctrl.Result{}, reconcileErr
	}

	if reconcileErr = r.reconcileHPA(ctx, memcached); reconcileErr != nil {
		return ctrl.Result{}, reconcileErr
	}

	if reconcileErr = r.reconcileService(ctx, memcached); reconcileErr != nil {
		return ctrl.Result{}, reconcileErr
	}

	if reconcileErr = r.reconcilePDB(ctx, memcached); reconcileErr != nil {
		return ctrl.Result{}, reconcileErr
	}

	if reconcileErr = r.reconcileServiceMonitor(ctx, memcached); reconcileErr != nil {
		return ctrl.Result{}, reconcileErr
	}

	if reconcileErr = r.reconcileNetworkPolicy(ctx, memcached); reconcileErr != nil {
		return ctrl.Result{}, reconcileErr
	}

	if reconcileErr = r.reconcileStatus(ctx, memcached, missingSecrets); reconcileErr != nil {
		return ctrl.Result{}, reconcileErr
	}

	metrics.RecordReadyReplicas(memcached.Name, memcached.Namespace, memcached.Status.ReadyReplicas)

	return ctrl.Result{}, nil
}

// reconcileDeployment ensures the Deployment for the Memcached CR matches the desired state.
// It fetches referenced Secrets, computes a hash for rolling-update annotations, reads the
// restart-trigger annotation from the CR, and passes everything to constructDeployment.
// It returns the names of any missing Secrets for use by status reconciliation.
func (r *MemcachedReconciler) reconcileDeployment(ctx context.Context, mc *memcachedv1alpha1.Memcached) ([]string, error) {
	found, missing := fetchReferencedSecrets(ctx, r.Client, mc)
	secretHash := computeSecretHash(found...)
	restartTrigger := mc.Annotations[AnnotationRestartTrigger]

	dep := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      mc.Name,
			Namespace: mc.Namespace,
		},
	}

	_, err := r.reconcileResource(ctx, mc, dep, func() error {
		constructDeployment(mc, dep, secretHash, restartTrigger)
		return nil
	}, "Deployment")
	return missing, err
}

// reconcileHPA ensures the HorizontalPodAutoscaler for the Memcached CR matches the desired state.
// When autoscaling is disabled, it deletes any existing HPA owned by the CR.
func (r *MemcachedReconciler) reconcileHPA(ctx context.Context, mc *memcachedv1alpha1.Memcached) error {
	if !hpaEnabled(mc) {
		return r.deleteOwnedResource(ctx, &autoscalingv2.HorizontalPodAutoscaler{
			ObjectMeta: metav1.ObjectMeta{Name: mc.Name, Namespace: mc.Namespace},
		}, "HorizontalPodAutoscaler")
	}

	hpa := &autoscalingv2.HorizontalPodAutoscaler{
		ObjectMeta: metav1.ObjectMeta{
			Name:      mc.Name,
			Namespace: mc.Namespace,
		},
	}

	_, err := r.reconcileResource(ctx, mc, hpa, func() error {
		constructHPA(mc, hpa)
		return nil
	}, "HorizontalPodAutoscaler")
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
// When PDB is disabled, it deletes any existing PDB owned by the CR.
func (r *MemcachedReconciler) reconcilePDB(ctx context.Context, mc *memcachedv1alpha1.Memcached) error {
	if !pdbEnabled(mc) {
		return r.deleteOwnedResource(ctx, &policyv1.PodDisruptionBudget{
			ObjectMeta: metav1.ObjectMeta{Name: mc.Name, Namespace: mc.Namespace},
		}, "PodDisruptionBudget")
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

// reconcileServiceMonitor ensures the ServiceMonitor for the Memcached CR matches the desired state.
// When monitoring is disabled, it deletes any existing ServiceMonitor owned by the CR.
func (r *MemcachedReconciler) reconcileServiceMonitor(ctx context.Context, mc *memcachedv1alpha1.Memcached) error {
	if !serviceMonitorEnabled(mc) {
		return r.deleteOwnedResource(ctx, &monitoringv1.ServiceMonitor{
			ObjectMeta: metav1.ObjectMeta{Name: mc.Name, Namespace: mc.Namespace},
		}, "ServiceMonitor")
	}

	sm := &monitoringv1.ServiceMonitor{
		ObjectMeta: metav1.ObjectMeta{
			Name:      mc.Name,
			Namespace: mc.Namespace,
		},
	}

	_, err := r.reconcileResource(ctx, mc, sm, func() error {
		constructServiceMonitor(mc, sm)
		return nil
	}, "ServiceMonitor")
	return err
}

// reconcileNetworkPolicy ensures the NetworkPolicy for the Memcached CR matches the desired state.
// When NetworkPolicy is disabled, it deletes any existing NetworkPolicy owned by the CR.
func (r *MemcachedReconciler) reconcileNetworkPolicy(ctx context.Context, mc *memcachedv1alpha1.Memcached) error {
	if !networkPolicyEnabled(mc) {
		return r.deleteOwnedResource(ctx, &networkingv1.NetworkPolicy{
			ObjectMeta: metav1.ObjectMeta{Name: mc.Name, Namespace: mc.Namespace},
		}, "NetworkPolicy")
	}

	np := &networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      mc.Name,
			Namespace: mc.Namespace,
		},
	}

	_, err := r.reconcileResource(ctx, mc, np, func() error {
		constructNetworkPolicy(mc, np)
		return nil
	}, "NetworkPolicy")
	return err
}

// SetupWithManager sets up the controller with the Manager.
func (r *MemcachedReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&memcachedv1alpha1.Memcached{}).
		Owns(&appsv1.Deployment{}).
		Owns(&autoscalingv2.HorizontalPodAutoscaler{}).
		Owns(&corev1.Service{}).
		Owns(&policyv1.PodDisruptionBudget{}).
		Owns(&networkingv1.NetworkPolicy{}).
		Owns(&monitoringv1.ServiceMonitor{}).
		Watches(&corev1.Secret{}, handler.EnqueueRequestsFromMapFunc(mapSecretToMemcached(mgr.GetClient()))).
		Named("memcached").
		Complete(r)
}
