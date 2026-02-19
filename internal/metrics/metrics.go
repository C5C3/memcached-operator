// Package metrics defines and registers custom Prometheus metrics for the memcached-operator.
package metrics

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"
	ctrlmetrics "sigs.k8s.io/controller-runtime/pkg/metrics"
)

var (
	// reconcileResourceTotal counts reconcileResource outcomes per resource kind.
	reconcileResourceTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "memcached_operator_reconcile_resource_total",
			Help: "Total number of per-resource reconciliation operations.",
		},
		[]string{"resource_kind", "result"},
	)

	// reconcileTotal counts the total number of reconciliations by result and instance.
	reconcileTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "memcached_operator_reconcile_total",
			Help: "Total number of Memcached reconciliations.",
		},
		[]string{"name", "namespace", "result"},
	)

	// reconcileDuration tracks the duration of reconciliations in seconds.
	reconcileDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "memcached_operator_reconcile_duration_seconds",
			Help:    "Duration of Memcached reconciliation in seconds.",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"name", "namespace"},
	)

	// instanceInfo is an info-style gauge for Memcached instances with labels
	// for the image in use. The gauge value is always 1 when the instance exists.
	instanceInfo = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "memcached_operator_instance_info",
			Help: "Information about a Memcached instance.",
		},
		[]string{"name", "namespace", "image"},
	)

	// instanceReplicasDesired tracks the desired replica count per Memcached instance.
	instanceReplicasDesired = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "memcached_operator_instance_replicas_desired",
			Help: "Desired number of replicas for a Memcached instance.",
		},
		[]string{"name", "namespace"},
	)

	// instanceReplicasReady tracks the ready replica count per Memcached instance.
	instanceReplicasReady = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "memcached_operator_instance_replicas_ready",
			Help: "Number of ready replicas for a Memcached instance.",
		},
		[]string{"name", "namespace"},
	)
)

func init() {
	ctrlmetrics.Registry.MustRegister(
		reconcileResourceTotal,
		reconcileTotal,
		reconcileDuration,
		instanceInfo,
		instanceReplicasDesired,
		instanceReplicasReady,
	)
}

// registry returns the controller-runtime metrics registry. This is exposed
// for testing purposes so that tests can gather metrics from the same registry
// where custom metrics are registered.
func registry() prometheus.Gatherer {
	return ctrlmetrics.Registry
}

// RecordReconciliation records a reconciliation event by incrementing the
// reconcile counter and observing the duration in the histogram.
func RecordReconciliation(name, namespace, result string, duration time.Duration) {
	reconcileTotal.WithLabelValues(name, namespace, result).Inc()
	reconcileDuration.WithLabelValues(name, namespace).Observe(duration.Seconds())
}

// RecordReconcileResource increments the per-resource reconciliation counter.
// The result should be one of "created", "updated", or "unchanged".
func RecordReconcileResource(resourceKind, result string) {
	reconcileResourceTotal.WithLabelValues(resourceKind, result).Inc()
}

// RecordInstanceInfo sets the info gauge and desired replicas gauge for a
// Memcached instance. The info gauge is set to 1 with the current image label.
// If the image changes, the old info gauge series is implicitly replaced
// because the label set differs.
func RecordInstanceInfo(name, namespace, image string, replicas int32) {
	// Delete any existing info gauge series for this instance (different image
	// label values produce different series, so we need to clean up stale ones).
	instanceInfo.DeletePartialMatch(prometheus.Labels{
		"name": name, "namespace": namespace,
	})
	instanceInfo.WithLabelValues(name, namespace, image).Set(1)
	instanceReplicasDesired.WithLabelValues(name, namespace).Set(float64(replicas))
}

// RecordReadyReplicas sets the ready replicas gauge for a Memcached instance.
func RecordReadyReplicas(name, namespace string, ready int32) {
	instanceReplicasReady.WithLabelValues(name, namespace).Set(float64(ready))
}

// ResetInstanceMetrics removes all metric series associated with a Memcached
// instance. This should be called when an instance is deleted.
func ResetInstanceMetrics(name, namespace string) {
	labels := prometheus.Labels{"name": name, "namespace": namespace}
	instanceInfo.DeletePartialMatch(labels)
	instanceReplicasDesired.DeletePartialMatch(labels)
	instanceReplicasReady.DeletePartialMatch(labels)
	reconcileTotal.DeletePartialMatch(labels)
	reconcileDuration.DeletePartialMatch(labels)
}
