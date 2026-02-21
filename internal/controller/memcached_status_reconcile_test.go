package controller_test

import (
	"context"
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	memcachedv1alpha1 "github.com/c5c3/memcached-operator/api/v1alpha1"
	"github.com/c5c3/memcached-operator/internal/controller"
)

// findCondition returns the condition with the given type from the conditions slice, or nil.
func findCondition(conditions []metav1.Condition, condType string) *metav1.Condition {
	for i := range conditions {
		if conditions[i].Type == condType {
			return &conditions[i]
		}
	}
	return nil
}

var _ = Describe("Status Reconciliation", func() {

	// --- Task 3.1: Status after initial reconciliation ---

	Context("initial reconciliation with default 1 replica (REQ-001, REQ-002, REQ-003, REQ-004, REQ-005)", func() {
		var mc *memcachedv1alpha1.Memcached

		BeforeEach(func() {
			mc = validMemcached(uniqueName("status-init"))
			Expect(k8sClient.Create(ctx, mc)).To(Succeed())
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(mc), mc)).To(Succeed())

			result, err := reconcileOnce(mc)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal(ctrl.Result{}))

			// Re-fetch to get updated status.
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(mc), mc)).To(Succeed())
		})

		It("should set observedGeneration equal to metadata.generation (REQ-001)", func() {
			Expect(mc.Status.ObservedGeneration).To(Equal(mc.Generation))
		})

		It("should set readyReplicas to 0 in envtest (REQ-002)", func() {
			Expect(mc.Status.ReadyReplicas).To(Equal(int32(0)))
		})

		It("should set Available=False since no replicas are ready (REQ-003)", func() {
			cond := findCondition(mc.Status.Conditions, "Available")
			Expect(cond).NotTo(BeNil())
			Expect(cond.Status).To(Equal(metav1.ConditionFalse))
		})

		It("should set Progressing=True since desired > ready (REQ-004)", func() {
			cond := findCondition(mc.Status.Conditions, "Progressing")
			Expect(cond).NotTo(BeNil())
			Expect(cond.Status).To(Equal(metav1.ConditionTrue))
		})

		It("should set Degraded=True since ready < desired (REQ-005)", func() {
			cond := findCondition(mc.Status.Conditions, "Degraded")
			Expect(cond).NotTo(BeNil())
			Expect(cond.Status).To(Equal(metav1.ConditionTrue))
		})

		It("should have all three conditions present", func() {
			Expect(mc.Status.Conditions).To(HaveLen(3))
			types := map[string]bool{}
			for _, c := range mc.Status.Conditions {
				types[c.Type] = true
			}
			Expect(types).To(HaveKey("Available"))
			Expect(types).To(HaveKey("Progressing"))
			Expect(types).To(HaveKey("Degraded"))
		})

		It("should set per-condition observedGeneration matching metadata.generation (REQ-006)", func() {
			for _, c := range mc.Status.Conditions {
				Expect(c.ObservedGeneration).To(Equal(mc.Generation),
					"condition %q: observedGeneration should match metadata.generation", c.Type)
			}
		})
	})

	Context("initial reconciliation with explicit 3 replicas (REQ-002, REQ-005)", func() {
		It("should set readyReplicas to 0 and Degraded=True with 3 desired replicas", func() {
			mc := validMemcached(uniqueName("status-3rep"))
			mc.Spec.Replicas = int32Ptr(3)
			Expect(k8sClient.Create(ctx, mc)).To(Succeed())
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(mc), mc)).To(Succeed())

			_, err := reconcileOnce(mc)
			Expect(err).NotTo(HaveOccurred())

			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(mc), mc)).To(Succeed())
			Expect(mc.Status.ReadyReplicas).To(Equal(int32(0)))

			cond := findCondition(mc.Status.Conditions, "Degraded")
			Expect(cond).NotTo(BeNil())
			Expect(cond.Status).To(Equal(metav1.ConditionTrue))
		})
	})

	// --- Task 3.2: Status after spec changes and zero-replica scaling ---

	Context("observedGeneration tracks spec changes (REQ-001)", func() {
		It("should update observedGeneration after spec.replicas change", func() {
			mc := validMemcached(uniqueName("status-gen"))
			mc.Spec.Replicas = int32Ptr(1)
			Expect(k8sClient.Create(ctx, mc)).To(Succeed())
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(mc), mc)).To(Succeed())

			_, err := reconcileOnce(mc)
			Expect(err).NotTo(HaveOccurred())

			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(mc), mc)).To(Succeed())
			gen1 := mc.Generation
			Expect(mc.Status.ObservedGeneration).To(Equal(gen1))

			// Update spec to increment generation.
			mc.Spec.Replicas = int32Ptr(3)
			Expect(k8sClient.Update(ctx, mc)).To(Succeed())
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(mc), mc)).To(Succeed())
			gen2 := mc.Generation
			Expect(gen2).To(BeNumerically(">", gen1))

			_, err = reconcileOnce(mc)
			Expect(err).NotTo(HaveOccurred())

			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(mc), mc)).To(Succeed())
			Expect(mc.Status.ObservedGeneration).To(Equal(gen2))
		})
	})

	Context("scaled to zero replicas (REQ-003, REQ-005)", func() {
		It("should set Available=False, Progressing=False, Degraded=False with 0 replicas", func() {
			mc := validMemcached(uniqueName("status-zero"))
			mc.Spec.Replicas = int32Ptr(0)
			Expect(k8sClient.Create(ctx, mc)).To(Succeed())
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(mc), mc)).To(Succeed())

			_, err := reconcileOnce(mc)
			Expect(err).NotTo(HaveOccurred())

			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(mc), mc)).To(Succeed())

			available := findCondition(mc.Status.Conditions, "Available")
			Expect(available).NotTo(BeNil())
			Expect(available.Status).To(Equal(metav1.ConditionFalse))

			progressing := findCondition(mc.Status.Conditions, "Progressing")
			Expect(progressing).NotTo(BeNil())
			Expect(progressing.Status).To(Equal(metav1.ConditionFalse))

			degraded := findCondition(mc.Status.Conditions, "Degraded")
			Expect(degraded).NotTo(BeNil())
			Expect(degraded.Status).To(Equal(metav1.ConditionFalse))
		})
	})

	Context("all three conditions always present after reconciliation (REQ-003, REQ-004, REQ-005)", func() {
		It("should have exactly three conditions with non-empty messages", func() {
			mc := validMemcached(uniqueName("status-allcond"))
			mc.Spec.Replicas = int32Ptr(2)
			Expect(k8sClient.Create(ctx, mc)).To(Succeed())
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(mc), mc)).To(Succeed())

			_, err := reconcileOnce(mc)
			Expect(err).NotTo(HaveOccurred())

			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(mc), mc)).To(Succeed())
			Expect(mc.Status.Conditions).To(HaveLen(3))

			for _, c := range mc.Status.Conditions {
				Expect(c.Message).NotTo(BeEmpty(), "condition %q has empty message", c.Type)
				Expect(c.Reason).NotTo(BeEmpty(), "condition %q has empty reason", c.Type)
				Expect(c.LastTransitionTime.IsZero()).To(BeFalse(), "condition %q has zero lastTransitionTime", c.Type)
			}
		})
	})

	// --- Task 3.3: Status update error propagation ---

	Context("status update error propagation (REQ-007)", func() {
		It("should propagate status update errors while Deployment and Service are created", func() {
			statusErr := fmt.Errorf("simulated status update error")
			fakeClient := fake.NewClientBuilder().WithScheme(scheme.Scheme).
				WithStatusSubresource(&memcachedv1alpha1.Memcached{}).
				Build()
			failingClient := interceptor.NewClient(fakeClient, interceptor.Funcs{
				SubResourceUpdate: func(_ context.Context, _ client.Client, _ string, obj client.Object, _ ...client.SubResourceUpdateOption) error {
					// Fail status updates for Memcached CR.
					if _, ok := obj.(*memcachedv1alpha1.Memcached); ok {
						return statusErr
					}
					return nil
				},
			})

			mc := validMemcached(uniqueName("status-err"))
			Expect(fakeClient.Create(ctx, mc)).To(Succeed())

			r := &controller.MemcachedReconciler{
				Client: failingClient,
				Scheme: scheme.Scheme,
			}
			_, err := r.Reconcile(ctx, ctrl.Request{
				NamespacedName: client.ObjectKeyFromObject(mc),
			})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("status"))

			// Verify Deployment and Service were created despite status update failure.
			depKey := client.ObjectKey{Name: mc.Name, Namespace: mc.Namespace}
			dep := &appsv1.Deployment{}
			Expect(fakeClient.Get(ctx, depKey, dep)).To(Succeed(), "Deployment should exist despite status update failure")

			svc := &corev1.Service{}
			Expect(fakeClient.Get(ctx, depKey, svc)).To(Succeed(), "Service should exist despite status update failure")
		})
	})
})
