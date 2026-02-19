package controller_test

import (
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/c5c3/memcached-operator/internal/controller"
)

// Compile-time interface compliance checks (REQ-009).
var _ reconcile.Reconciler = &controller.MemcachedReconciler{}

// Compile-time check that SetupWithManager has the expected signature.
var _ func(ctrl.Manager) error = (&controller.MemcachedReconciler{}).SetupWithManager

var _ = Describe("MemcachedReconciler struct", func() {

	Context("struct fields (REQ-008)", func() {
		It("should be constructible with Client and Scheme fields", func() {
			reconciler := &controller.MemcachedReconciler{
				Client: k8sClient,
				Scheme: runtime.NewScheme(),
			}
			Expect(reconciler).NotTo(BeNil())
		})

		It("should expose the embedded Client field", func() {
			reconciler := &controller.MemcachedReconciler{
				Client: k8sClient,
			}
			// The embedded client.Client should be accessible and non-nil when set.
			c := reconciler.Client
			Expect(c).NotTo(BeNil())
		})

		It("should expose the Scheme field", func() {
			s := runtime.NewScheme()
			reconciler := &controller.MemcachedReconciler{
				Scheme: s,
			}
			Expect(reconciler.Scheme).NotTo(BeNil())
			Expect(reconciler.Scheme).To(BeIdenticalTo(s))
		})
	})

	Context("interface compliance (REQ-009)", func() {
		It("should satisfy the reconcile.Reconciler interface", func() {
			// Runtime verification in addition to the compile-time check above.
			var r reconcile.Reconciler = &controller.MemcachedReconciler{}
			Expect(r).NotTo(BeNil())
		})

		It("should have a SetupWithManager method accepting ctrl.Manager", func() {
			reconciler := &controller.MemcachedReconciler{}
			// Verify the method reference is obtainable (compile-time signature check).
			fn := reconciler.SetupWithManager
			Expect(fn).NotTo(BeNil())
		})
	})
})
