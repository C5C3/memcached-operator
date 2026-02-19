package controller_test

import (
	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	ctrlmetrics "sigs.k8s.io/controller-runtime/pkg/metrics"

	"github.com/c5c3/memcached-operator/internal/controller"
	// Import metrics package to ensure init() registration runs.
	_ "github.com/c5c3/memcached-operator/internal/metrics"
)

// gatherMetricValue gathers the named metric from the controller-runtime registry
// and returns the value matching the given label set. For counters it returns the
// counter value; for gauges it returns the gauge value. Returns 0 if not found.
func gatherMetricValue(metricName string, labelMatch map[string]string) float64 {
	gatherer, ok := ctrlmetrics.Registry.(prometheus.Gatherer)
	if !ok {
		return 0
	}
	families, err := gatherer.Gather()
	if err != nil {
		return 0
	}
	for _, f := range families {
		if f.GetName() != metricName {
			continue
		}
		for _, m := range f.GetMetric() {
			if labelsMatchAll(m.GetLabel(), labelMatch) {
				if m.GetCounter() != nil {
					return m.GetCounter().GetValue()
				}
				if m.GetGauge() != nil {
					return m.GetGauge().GetValue()
				}
			}
		}
	}
	return 0
}

// gatherMetricExists checks if a metric series with the given labels exists.
func gatherMetricExists(metricName string, labelMatch map[string]string) bool {
	gatherer, ok := ctrlmetrics.Registry.(prometheus.Gatherer)
	if !ok {
		return false
	}
	families, err := gatherer.Gather()
	if err != nil {
		return false
	}
	for _, f := range families {
		if f.GetName() != metricName {
			continue
		}
		for _, m := range f.GetMetric() {
			if labelsMatchAll(m.GetLabel(), labelMatch) {
				return true
			}
		}
	}
	return false
}

func labelsMatchAll(labels []*dto.LabelPair, expected map[string]string) bool {
	matched := 0
	for _, l := range labels {
		if val, ok := expected[l.GetName()]; ok && l.GetValue() == val {
			matched++
		}
	}
	return matched == len(expected)
}

var _ = Describe("Memcached Metrics Reconciliation", func() {

	newReconciler := func() *controller.MemcachedReconciler {
		return &controller.MemcachedReconciler{
			Client: k8sClient,
			Scheme: scheme.Scheme,
		}
	}

	Context("when a Memcached CR is reconciled successfully (REQ-003, REQ-004)", func() {
		It("should set info and replicas gauges after successful reconciliation", func() {
			mc := validMemcached(uniqueName("metrics-info"))
			mc.Spec.Replicas = int32Ptr(2)
			mc.Spec.Image = strPtr("memcached:1.6.29")
			Expect(k8sClient.Create(ctx, mc)).To(Succeed())

			r := newReconciler()
			_, err := r.Reconcile(ctx, ctrl.Request{
				NamespacedName: client.ObjectKeyFromObject(mc),
			})
			Expect(err).NotTo(HaveOccurred())

			// Verify instance info gauge is set to 1 with the image label.
			infoLabels := map[string]string{
				"name":      mc.Name,
				"namespace": mc.Namespace,
				"image":     "memcached:1.6.29",
			}
			Expect(gatherMetricValue("memcached_operator_instance_info", infoLabels)).To(Equal(float64(1)))

			// Verify desired replicas gauge.
			desiredLabels := map[string]string{
				"name":      mc.Name,
				"namespace": mc.Namespace,
			}
			Expect(gatherMetricValue("memcached_operator_instance_replicas_desired", desiredLabels)).To(Equal(float64(2)))

			// Verify ready replicas gauge is set (will be 0 since envtest
			// doesn't run pods, but the metric should exist).
			Expect(gatherMetricExists("memcached_operator_instance_replicas_ready", desiredLabels)).To(BeTrue())
		})
	})

	Context("when reconcileResource creates resources (REQ-002)", func() {
		It("should increment reconcile_resource_total for Deployment and Service", func() {
			mc := validMemcached(uniqueName("metrics-res"))
			Expect(k8sClient.Create(ctx, mc)).To(Succeed())

			depCreatedBefore := gatherMetricValue("memcached_operator_reconcile_resource_total", map[string]string{
				"resource_kind": "Deployment",
				"result":        "created",
			})
			svcCreatedBefore := gatherMetricValue("memcached_operator_reconcile_resource_total", map[string]string{
				"resource_kind": "Service",
				"result":        "created",
			})

			r := newReconciler()
			_, err := r.Reconcile(ctx, ctrl.Request{
				NamespacedName: client.ObjectKeyFromObject(mc),
			})
			Expect(err).NotTo(HaveOccurred())

			depCreatedAfter := gatherMetricValue("memcached_operator_reconcile_resource_total", map[string]string{
				"resource_kind": "Deployment",
				"result":        "created",
			})
			svcCreatedAfter := gatherMetricValue("memcached_operator_reconcile_resource_total", map[string]string{
				"resource_kind": "Service",
				"result":        "created",
			})

			Expect(depCreatedAfter).To(BeNumerically(">", depCreatedBefore))
			Expect(svcCreatedAfter).To(BeNumerically(">", svcCreatedBefore))
		})
	})

	Context("when a Memcached CR is deleted (REQ-007)", func() {
		It("should clean up gauges when Memcached CR is deleted", func() {
			mc := validMemcached(uniqueName("metrics-del"))
			mc.Spec.Replicas = int32Ptr(1)
			Expect(k8sClient.Create(ctx, mc)).To(Succeed())

			r := newReconciler()
			_, err := r.Reconcile(ctx, ctrl.Request{
				NamespacedName: client.ObjectKeyFromObject(mc),
			})
			Expect(err).NotTo(HaveOccurred())

			// Verify metrics exist before deletion.
			labels := map[string]string{
				"name":      mc.Name,
				"namespace": mc.Namespace,
			}
			Expect(gatherMetricExists("memcached_operator_instance_replicas_desired", labels)).To(BeTrue())

			// Delete the CR.
			Expect(k8sClient.Delete(ctx, mc)).To(Succeed())

			// Reconcile after deletion should clean up metrics.
			_, err = r.Reconcile(ctx, ctrl.Request{
				NamespacedName: client.ObjectKeyFromObject(mc),
			})
			Expect(err).NotTo(HaveOccurred())

			// After cleanup, the label set should no longer exist for this instance.
			Expect(gatherMetricExists("memcached_operator_instance_replicas_desired", labels)).To(BeFalse())
			Expect(gatherMetricExists("memcached_operator_instance_replicas_ready", labels)).To(BeFalse())
		})
	})

	Context("when one CR is deleted while another exists (REQ-007)", func() {
		It("should not affect other CR metrics when one CR is deleted", func() {
			mcA := validMemcached(uniqueName("metrics-a"))
			mcA.Spec.Replicas = int32Ptr(2)
			Expect(k8sClient.Create(ctx, mcA)).To(Succeed())

			mcB := validMemcached(uniqueName("metrics-b"))
			mcB.Spec.Replicas = int32Ptr(3)
			Expect(k8sClient.Create(ctx, mcB)).To(Succeed())

			r := newReconciler()

			// Reconcile both CRs.
			_, err := r.Reconcile(ctx, ctrl.Request{
				NamespacedName: client.ObjectKeyFromObject(mcA),
			})
			Expect(err).NotTo(HaveOccurred())

			_, err = r.Reconcile(ctx, ctrl.Request{
				NamespacedName: client.ObjectKeyFromObject(mcB),
			})
			Expect(err).NotTo(HaveOccurred())

			// Verify both have metrics.
			labelsA := map[string]string{"name": mcA.Name, "namespace": mcA.Namespace}
			labelsB := map[string]string{"name": mcB.Name, "namespace": mcB.Namespace}
			Expect(gatherMetricExists("memcached_operator_instance_replicas_desired", labelsA)).To(BeTrue())
			Expect(gatherMetricExists("memcached_operator_instance_replicas_desired", labelsB)).To(BeTrue())

			// Delete CR A.
			Expect(k8sClient.Delete(ctx, mcA)).To(Succeed())
			_, err = r.Reconcile(ctx, ctrl.Request{
				NamespacedName: client.ObjectKeyFromObject(mcA),
			})
			Expect(err).NotTo(HaveOccurred())

			// CR A metrics should be gone, CR B metrics should remain.
			Expect(gatherMetricExists("memcached_operator_instance_replicas_desired", labelsA)).To(BeFalse())
			Expect(gatherMetricExists("memcached_operator_instance_replicas_desired", labelsB)).To(BeTrue())
			Expect(gatherMetricValue("memcached_operator_instance_replicas_desired", labelsB)).To(Equal(float64(3)))
		})
	})
})
