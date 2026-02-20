package controller_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	memcachedv1alpha1 "github.com/c5c3/memcached-operator/api/v1alpha1"
)

var _ = Describe("TLS Service Reconciliation", func() {

	Context("TLS port on Service (REQ-005, REQ-007)", func() {

		It("should add TLS port to Service when TLS is enabled", func() {
			mc := validMemcached(uniqueName("tls-svc-on"))
			mc.Spec.Security = &memcachedv1alpha1.SecuritySpec{
				TLS: &memcachedv1alpha1.TLSSpec{
					Enabled:              true,
					CertificateSecretRef: corev1.LocalObjectReference{Name: "tls-secret"},
				},
			}
			Expect(k8sClient.Create(ctx, mc)).To(Succeed())

			result, err := reconcileOnce(mc)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal(ctrl.Result{}))

			svc := fetchService(mc)
			Expect(svc.Spec.Ports).To(HaveLen(2))

			// Standard port.
			Expect(svc.Spec.Ports[0].Name).To(Equal("memcached"))
			Expect(svc.Spec.Ports[0].Port).To(Equal(int32(11211)))
			Expect(svc.Spec.Ports[0].TargetPort).To(Equal(intstr.FromString("memcached")))

			// TLS port.
			Expect(svc.Spec.Ports[1].Name).To(Equal("memcached-tls"))
			Expect(svc.Spec.Ports[1].Port).To(Equal(int32(11212)))
			Expect(svc.Spec.Ports[1].TargetPort).To(Equal(intstr.FromString("memcached-tls")))
			Expect(svc.Spec.Ports[1].Protocol).To(Equal(corev1.ProtocolTCP))
		})

		It("should not have TLS port when TLS is not configured", func() {
			mc := validMemcached(uniqueName("tls-svc-off"))
			Expect(k8sClient.Create(ctx, mc)).To(Succeed())

			_, err := reconcileOnce(mc)
			Expect(err).NotTo(HaveOccurred())

			svc := fetchService(mc)
			Expect(svc.Spec.Ports).To(HaveLen(1))
			Expect(svc.Spec.Ports[0].Name).To(Equal("memcached"))
		})
	})

	Context("TLS Service enable/disable toggle (REQ-005, REQ-007)", func() {

		It("should add TLS port when enabling and remove when disabling", func() {
			mc := validMemcached(uniqueName("tls-svc-toggle"))
			Expect(k8sClient.Create(ctx, mc)).To(Succeed())

			// Initial reconcile without TLS.
			_, err := reconcileOnce(mc)
			Expect(err).NotTo(HaveOccurred())

			svc := fetchService(mc)
			Expect(svc.Spec.Ports).To(HaveLen(1))

			// Enable TLS.
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(mc), mc)).To(Succeed())
			mc.Spec.Security = &memcachedv1alpha1.SecuritySpec{
				TLS: &memcachedv1alpha1.TLSSpec{
					Enabled:              true,
					CertificateSecretRef: corev1.LocalObjectReference{Name: "tls-secret"},
				},
			}
			Expect(k8sClient.Update(ctx, mc)).To(Succeed())

			_, err = reconcileOnce(mc)
			Expect(err).NotTo(HaveOccurred())

			svc = fetchService(mc)
			Expect(svc.Spec.Ports).To(HaveLen(2))
			Expect(svc.Spec.Ports[1].Name).To(Equal("memcached-tls"))
			Expect(svc.Spec.Ports[1].Port).To(Equal(int32(11212)))

			// Disable TLS.
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(mc), mc)).To(Succeed())
			mc.Spec.Security = nil
			Expect(k8sClient.Update(ctx, mc)).To(Succeed())

			_, err = reconcileOnce(mc)
			Expect(err).NotTo(HaveOccurred())

			svc = fetchService(mc)
			Expect(svc.Spec.Ports).To(HaveLen(1))
			Expect(svc.Spec.Ports[0].Name).To(Equal("memcached"))
		})
	})

	Context("TLS Service idempotency (REQ-005, REQ-007)", func() {

		It("should be idempotent when reconciling Service with TLS enabled", func() {
			mc := validMemcached(uniqueName("tls-svc-idemp"))
			mc.Spec.Security = &memcachedv1alpha1.SecuritySpec{
				TLS: &memcachedv1alpha1.TLSSpec{
					Enabled:              true,
					CertificateSecretRef: corev1.LocalObjectReference{Name: "tls-secret"},
				},
			}
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

	Context("TLS Service with monitoring (REQ-005, REQ-007)", func() {

		It("should have all three ports when TLS and monitoring are both enabled", func() {
			mc := validMemcached(uniqueName("tls-svc-mon"))
			mc.Spec.Security = &memcachedv1alpha1.SecuritySpec{
				TLS: &memcachedv1alpha1.TLSSpec{
					Enabled:              true,
					CertificateSecretRef: corev1.LocalObjectReference{Name: "tls-secret"},
				},
			}
			mc.Spec.Monitoring = &memcachedv1alpha1.MonitoringSpec{
				Enabled: true,
			}
			Expect(k8sClient.Create(ctx, mc)).To(Succeed())

			_, err := reconcileOnce(mc)
			Expect(err).NotTo(HaveOccurred())

			svc := fetchService(mc)
			Expect(svc.Spec.Ports).To(HaveLen(3))
			Expect(svc.Spec.Ports[0].Name).To(Equal("memcached"))
			Expect(svc.Spec.Ports[0].Port).To(Equal(int32(11211)))
			Expect(svc.Spec.Ports[1].Name).To(Equal("memcached-tls"))
			Expect(svc.Spec.Ports[1].Port).To(Equal(int32(11212)))
			Expect(svc.Spec.Ports[2].Name).To(Equal("metrics"))
			Expect(svc.Spec.Ports[2].Port).To(Equal(int32(9150)))
		})
	})
})
