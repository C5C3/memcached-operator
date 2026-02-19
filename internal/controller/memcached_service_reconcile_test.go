package controller_test

import (
	"context"
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	memcachedv1alpha1 "github.com/c5c3/memcached-operator/api/v1alpha1"
	"github.com/c5c3/memcached-operator/internal/controller"
)

// fetchService retrieves the Service with the same name/namespace as the Memcached CR.
func fetchService(mc *memcachedv1alpha1.Memcached) *corev1.Service {
	svc := &corev1.Service{}
	ExpectWithOffset(1, k8sClient.Get(ctx, client.ObjectKeyFromObject(mc), svc)).To(Succeed())
	return svc
}

var _ = Describe("Service Reconciliation", func() {

	Context("minimal CR with defaults", func() {
		var mc *memcachedv1alpha1.Memcached

		BeforeEach(func() {
			mc = validMemcached(uniqueName("svc-minimal"))
			Expect(k8sClient.Create(ctx, mc)).To(Succeed())
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(mc), mc)).To(Succeed())
			result, err := reconcileOnce(mc)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal(ctrl.Result{}))
		})

		It("should create a headless Service with clusterIP None", func() {
			svc := fetchService(mc)
			Expect(svc.Spec.ClusterIP).To(Equal(corev1.ClusterIPNone))
			Expect(svc.Spec.Ports).To(HaveLen(1))
			port := svc.Spec.Ports[0]
			Expect(port.Name).To(Equal("memcached"))
			Expect(port.Port).To(Equal(int32(11211)))
			Expect(port.TargetPort).To(Equal(intstr.FromString("memcached")))
			Expect(port.Protocol).To(Equal(corev1.ProtocolTCP))
		})

		It("should set owner reference pointing to the Memcached CR", func() {
			svc := fetchService(mc)
			Expect(svc.OwnerReferences).To(HaveLen(1))
			ownerRef := svc.OwnerReferences[0]
			Expect(ownerRef.APIVersion).To(Equal("memcached.c5c3.io/v1alpha1"))
			Expect(ownerRef.Kind).To(Equal("Memcached"))
			Expect(ownerRef.Name).To(Equal(mc.Name))
			Expect(ownerRef.UID).To(Equal(mc.UID))
			Expect(*ownerRef.Controller).To(BeTrue())
			Expect(*ownerRef.BlockOwnerDeletion).To(BeTrue())
		})

		It("should set standard labels on Service metadata", func() {
			svc := fetchService(mc)
			Expect(svc.Labels).To(HaveKeyWithValue("app.kubernetes.io/name", "memcached"))
			Expect(svc.Labels).To(HaveKeyWithValue("app.kubernetes.io/instance", mc.Name))
			Expect(svc.Labels).To(HaveKeyWithValue("app.kubernetes.io/managed-by", "memcached-operator"))
		})

		It("should set matching labels on Service selector", func() {
			svc := fetchService(mc)
			Expect(svc.Spec.Selector).To(HaveKeyWithValue("app.kubernetes.io/name", "memcached"))
			Expect(svc.Spec.Selector).To(HaveKeyWithValue("app.kubernetes.io/instance", mc.Name))
			Expect(svc.Spec.Selector).To(HaveKeyWithValue("app.kubernetes.io/managed-by", "memcached-operator"))
		})
	})

	Context("custom annotations", func() {
		It("should apply custom annotations from spec.service.annotations", func() {
			mc := validMemcached(uniqueName("svc-anno"))
			mc.Spec.Service = &memcachedv1alpha1.ServiceSpec{
				Annotations: map[string]string{
					"prometheus.io/scrape": "true",
					"prometheus.io/port":   "9150",
				},
			}
			Expect(k8sClient.Create(ctx, mc)).To(Succeed())

			_, err := reconcileOnce(mc)
			Expect(err).NotTo(HaveOccurred())

			svc := fetchService(mc)
			Expect(svc.Annotations).To(HaveKeyWithValue("prometheus.io/scrape", "true"))
			Expect(svc.Annotations).To(HaveKeyWithValue("prometheus.io/port", "9150"))
		})

		It("should update annotations when CR spec changes", func() {
			mc := validMemcached(uniqueName("svc-anno-upd"))
			mc.Spec.Service = &memcachedv1alpha1.ServiceSpec{
				Annotations: map[string]string{
					"prometheus.io/scrape": "true",
				},
			}
			Expect(k8sClient.Create(ctx, mc)).To(Succeed())

			_, err := reconcileOnce(mc)
			Expect(err).NotTo(HaveOccurred())

			svc := fetchService(mc)
			Expect(svc.Annotations).To(HaveKeyWithValue("prometheus.io/scrape", "true"))

			// Update annotations.
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(mc), mc)).To(Succeed())
			mc.Spec.Service = &memcachedv1alpha1.ServiceSpec{
				Annotations: map[string]string{
					"custom/key": "new-value",
				},
			}
			Expect(k8sClient.Update(ctx, mc)).To(Succeed())

			_, err = reconcileOnce(mc)
			Expect(err).NotTo(HaveOccurred())

			svc = fetchService(mc)
			Expect(svc.Annotations).To(HaveKeyWithValue("custom/key", "new-value"))
			Expect(svc.Annotations).NotTo(HaveKey("prometheus.io/scrape"))
		})
	})

	Context("idempotency and drift detection", func() {
		It("should be idempotent when reconciling without changes", func() {
			mc := validMemcached(uniqueName("svc-idempotent"))
			Expect(k8sClient.Create(ctx, mc)).To(Succeed())

			_, err := reconcileOnce(mc)
			Expect(err).NotTo(HaveOccurred())

			svc1 := fetchService(mc)
			rv1 := svc1.ResourceVersion

			// Reconcile again without changes.
			_, err = reconcileOnce(mc)
			Expect(err).NotTo(HaveOccurred())

			svc2 := fetchService(mc)
			Expect(svc2.ResourceVersion).To(Equal(rv1))
		})
	})

	Context("error handling", func() {
		It("should propagate API errors from Service create/update", func() {
			apiErr := fmt.Errorf("simulated API server error")
			fakeClient := fake.NewClientBuilder().WithScheme(scheme.Scheme).Build()
			failingClient := interceptor.NewClient(fakeClient, interceptor.Funcs{
				Get: func(ctx context.Context, c client.WithWatch, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
					// Allow Get for Memcached CR and Deployment but fail for Service.
					if _, ok := obj.(*corev1.Service); ok {
						return apiErr
					}
					return c.Get(ctx, key, obj, opts...)
				},
			})

			mc := validMemcached(uniqueName("svc-err"))
			Expect(fakeClient.Create(ctx, mc)).To(Succeed())

			r := &controller.MemcachedReconciler{
				Client: failingClient,
				Scheme: scheme.Scheme,
			}
			_, err := r.Reconcile(ctx, ctrl.Request{
				NamespacedName: client.ObjectKeyFromObject(mc),
			})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("reconciling Service"))
		})
	})
})
