package controller_test

import (
	"context"
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	memcachedv1alpha1 "github.com/c5c3/memcached-operator/api/v1alpha1"
	"github.com/c5c3/memcached-operator/internal/controller"
)

var _ = Describe("Memcached Reconcile", func() {

	newReconciler := func() *controller.MemcachedReconciler {
		return &controller.MemcachedReconciler{
			Client: k8sClient,
			Scheme: scheme.Scheme,
		}
	}

	Context("when the Memcached CR does not exist", func() {
		It("should return empty result and no error", func() {
			r := newReconciler()
			result, err := r.Reconcile(ctx, ctrl.Request{
				NamespacedName: client.ObjectKey{
					Name:      "does-not-exist",
					Namespace: "default",
				},
			})

			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal(ctrl.Result{}))
		})
	})

	Context("when the Memcached CR exists", func() {
		It("should fetch the CR and return empty result with no error", func() {
			mc := validMemcached(uniqueName("reconcile"))
			Expect(k8sClient.Create(ctx, mc)).To(Succeed())

			r := newReconciler()
			result, err := r.Reconcile(ctx, ctrl.Request{
				NamespacedName: client.ObjectKeyFromObject(mc),
			})

			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal(ctrl.Result{}))
		})
	})

	Context("when client.Get returns a non-NotFound error", func() {
		It("should propagate the error", func() {
			getErr := fmt.Errorf("API server unavailable")
			fakeClient := fake.NewClientBuilder().WithScheme(scheme.Scheme).Build()
			failingClient := interceptor.NewClient(fakeClient, interceptor.Funcs{
				Get: func(_ context.Context, _ client.WithWatch, _ client.ObjectKey, _ client.Object, _ ...client.GetOption) error {
					return getErr
				},
			})

			r := &controller.MemcachedReconciler{
				Client: failingClient,
				Scheme: scheme.Scheme,
			}
			result, err := r.Reconcile(ctx, ctrl.Request{
				NamespacedName: client.ObjectKey{
					Name:      "any-name",
					Namespace: "default",
				},
			})

			Expect(err).To(MatchError(getErr))
			Expect(result).To(Equal(ctrl.Result{}))
		})
	})

	Context("when the Memcached CR is deleted before reconciliation", func() {
		It("should return empty result and no error after deletion", func() {
			mc := validMemcached(uniqueName("reconcile-del"))
			Expect(k8sClient.Create(ctx, mc)).To(Succeed())

			// Ensure the CR exists.
			fetched := &memcachedv1alpha1.Memcached{}
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(mc), fetched)).To(Succeed())

			// Delete the CR.
			Expect(k8sClient.Delete(ctx, mc)).To(Succeed())

			r := newReconciler()
			result, err := r.Reconcile(ctx, ctrl.Request{
				NamespacedName: client.ObjectKeyFromObject(mc),
			})

			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal(ctrl.Result{}))
		})
	})
})
