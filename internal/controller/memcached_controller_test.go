package controller_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/c5c3/memcached-operator/internal/controller"
)

var _ = Describe("Memcached Controller", func() {
	Context("When setting up the reconciler", func() {
		It("should implement the Reconciler interface", func() {
			reconciler := &controller.MemcachedReconciler{
				Client: k8sClient,
				Scheme: scheme.Scheme,
			}
			// Verify the reconciler satisfies the Reconciler interface.
			var _ reconcile.Reconciler = reconciler
			Expect(reconciler).NotTo(BeNil())
			Expect(reconciler.Scheme).NotTo(BeNil())
		})
	})

	Context("When reconciling a non-existent resource", func() {
		It("should return an empty result without error", func() {
			reconciler := &controller.MemcachedReconciler{
				Client: k8sClient,
				Scheme: scheme.Scheme,
			}

			result, err := reconciler.Reconcile(ctx, ctrl.Request{
				NamespacedName: types.NamespacedName{
					Name:      "nonexistent-memcached",
					Namespace: "default",
				},
			})

			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal(ctrl.Result{}))
		})
	})
})
