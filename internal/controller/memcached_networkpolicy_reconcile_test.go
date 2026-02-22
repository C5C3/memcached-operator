package controller_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	memcachedv1beta1 "github.com/c5c3/memcached-operator/api/v1beta1"
)

// fetchNetworkPolicy retrieves the NetworkPolicy with the same name/namespace as the Memcached CR.
func fetchNetworkPolicy(mc *memcachedv1beta1.Memcached) *networkingv1.NetworkPolicy {
	np := &networkingv1.NetworkPolicy{}
	ExpectWithOffset(1, k8sClient.Get(ctx, client.ObjectKeyFromObject(mc), np)).To(Succeed())
	return np
}

var _ = Describe("NetworkPolicy Reconciliation", func() {

	Context("NetworkPolicy creation with defaults", func() {
		var mc *memcachedv1beta1.Memcached

		BeforeEach(func() {
			mc = validMemcached(uniqueName("np-defaults"))
			mc.Spec.Security = &memcachedv1beta1.SecuritySpec{
				NetworkPolicy: &memcachedv1beta1.NetworkPolicySpec{Enabled: true},
			}
			Expect(k8sClient.Create(ctx, mc)).To(Succeed())
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(mc), mc)).To(Succeed())
			result, err := reconcileOnce(mc)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal(ctrl.Result{}))
		})

		It("should create NetworkPolicy with memcached port 11211", func() {
			np := fetchNetworkPolicy(mc)
			Expect(np.Spec.PolicyTypes).To(ConsistOf(networkingv1.PolicyTypeIngress))
			Expect(np.Spec.Ingress).To(HaveLen(1))

			rule := np.Spec.Ingress[0]
			Expect(rule.Ports).To(HaveLen(1))
			Expect(rule.Ports[0].Port.IntValue()).To(Equal(11211))
			Expect(rule.From).To(BeNil())
		})

		It("should set standard labels on metadata", func() {
			np := fetchNetworkPolicy(mc)
			Expect(np.Labels).To(HaveKeyWithValue("app.kubernetes.io/name", "memcached"))
			Expect(np.Labels).To(HaveKeyWithValue("app.kubernetes.io/instance", mc.Name))
			Expect(np.Labels).To(HaveKeyWithValue("app.kubernetes.io/managed-by", "memcached-operator"))
		})

		It("should set podSelector with standard labels", func() {
			np := fetchNetworkPolicy(mc)
			Expect(np.Spec.PodSelector.MatchLabels).To(HaveKeyWithValue("app.kubernetes.io/name", "memcached"))
			Expect(np.Spec.PodSelector.MatchLabels).To(HaveKeyWithValue("app.kubernetes.io/instance", mc.Name))
			Expect(np.Spec.PodSelector.MatchLabels).To(HaveKeyWithValue("app.kubernetes.io/managed-by", "memcached-operator"))
		})

		It("should set owner reference", func() {
			np := fetchNetworkPolicy(mc)
			Expect(np.OwnerReferences).To(HaveLen(1))
			ownerRef := np.OwnerReferences[0]
			Expect(ownerRef.APIVersion).To(Equal("memcached.c5c3.io/v1beta1"))
			Expect(ownerRef.Kind).To(Equal("Memcached"))
			Expect(ownerRef.Name).To(Equal(mc.Name))
			Expect(ownerRef.UID).To(Equal(mc.UID))
			Expect(*ownerRef.Controller).To(BeTrue())
			Expect(*ownerRef.BlockOwnerDeletion).To(BeTrue())
		})
	})

	Context("NetworkPolicy with monitoring enabled", func() {
		It("should include metrics port 9150", func() {
			mc := validMemcached(uniqueName("np-monitoring"))
			mc.Spec.Monitoring = &memcachedv1beta1.MonitoringSpec{Enabled: true}
			mc.Spec.Security = &memcachedv1beta1.SecuritySpec{
				NetworkPolicy: &memcachedv1beta1.NetworkPolicySpec{Enabled: true},
			}
			Expect(k8sClient.Create(ctx, mc)).To(Succeed())

			_, err := reconcileOnce(mc)
			Expect(err).NotTo(HaveOccurred())

			np := fetchNetworkPolicy(mc)
			Expect(np.Spec.Ingress).To(HaveLen(1))

			ports := np.Spec.Ingress[0].Ports
			Expect(ports).To(HaveLen(2))
			Expect(ports[0].Port.IntValue()).To(Equal(11211))
			Expect(ports[1].Port.IntValue()).To(Equal(9150))
		})
	})

	Context("NetworkPolicy with TLS enabled", func() {
		It("should include TLS port 11212", func() {
			mc := validMemcached(uniqueName("np-tls"))
			mc.Spec.Security = &memcachedv1beta1.SecuritySpec{
				TLS: &memcachedv1beta1.TLSSpec{
					Enabled:              true,
					CertificateSecretRef: corev1.LocalObjectReference{Name: "tls-secret"},
				},
				NetworkPolicy: &memcachedv1beta1.NetworkPolicySpec{Enabled: true},
			}
			Expect(k8sClient.Create(ctx, mc)).To(Succeed())

			_, err := reconcileOnce(mc)
			Expect(err).NotTo(HaveOccurred())

			np := fetchNetworkPolicy(mc)
			Expect(np.Spec.Ingress).To(HaveLen(1))

			ports := np.Spec.Ingress[0].Ports
			Expect(ports).To(HaveLen(2))
			Expect(ports[0].Port.IntValue()).To(Equal(11211))
			Expect(ports[1].Port.IntValue()).To(Equal(11212))
		})
	})

	Context("NetworkPolicy with monitoring and TLS enabled", func() {
		It("should include all three ports", func() {
			mc := validMemcached(uniqueName("np-all-ports"))
			mc.Spec.Monitoring = &memcachedv1beta1.MonitoringSpec{Enabled: true}
			mc.Spec.Security = &memcachedv1beta1.SecuritySpec{
				TLS: &memcachedv1beta1.TLSSpec{
					Enabled:              true,
					CertificateSecretRef: corev1.LocalObjectReference{Name: "tls-secret"},
				},
				NetworkPolicy: &memcachedv1beta1.NetworkPolicySpec{Enabled: true},
			}
			Expect(k8sClient.Create(ctx, mc)).To(Succeed())

			_, err := reconcileOnce(mc)
			Expect(err).NotTo(HaveOccurred())

			np := fetchNetworkPolicy(mc)
			Expect(np.Spec.Ingress).To(HaveLen(1))

			ports := np.Spec.Ingress[0].Ports
			Expect(ports).To(HaveLen(3))
			Expect(ports[0].Port.IntValue()).To(Equal(11211))
			Expect(ports[1].Port.IntValue()).To(Equal(11212))
			Expect(ports[2].Port.IntValue()).To(Equal(9150))
		})
	})

	Context("NetworkPolicy with allowedSources", func() {
		It("should populate ingress from peers", func() {
			mc := validMemcached(uniqueName("np-allowed"))
			mc.Spec.Security = &memcachedv1beta1.SecuritySpec{
				NetworkPolicy: &memcachedv1beta1.NetworkPolicySpec{
					Enabled: true,
					AllowedSources: []networkingv1.NetworkPolicyPeer{
						{
							NamespaceSelector: &metav1.LabelSelector{
								MatchLabels: map[string]string{"env": "production"},
							},
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, mc)).To(Succeed())

			_, err := reconcileOnce(mc)
			Expect(err).NotTo(HaveOccurred())

			np := fetchNetworkPolicy(mc)
			Expect(np.Spec.Ingress).To(HaveLen(1))
			Expect(np.Spec.Ingress[0].From).To(HaveLen(1))
			Expect(np.Spec.Ingress[0].From[0].NamespaceSelector).NotTo(BeNil())
			Expect(np.Spec.Ingress[0].From[0].NamespaceSelector.MatchLabels).To(
				HaveKeyWithValue("env", "production"))
		})
	})

	Context("No NetworkPolicy when disabled", func() {
		It("should not create a NetworkPolicy resource", func() {
			mc := validMemcached(uniqueName("np-disabled"))
			Expect(k8sClient.Create(ctx, mc)).To(Succeed())

			_, err := reconcileOnce(mc)
			Expect(err).NotTo(HaveOccurred())

			np := &networkingv1.NetworkPolicy{}
			err = k8sClient.Get(ctx, client.ObjectKeyFromObject(mc), np)
			Expect(apierrors.IsNotFound(err)).To(BeTrue())
		})
	})

	Context("NetworkPolicy update when monitoring toggled on", func() {
		It("should add metrics port 9150 on update", func() {
			mc := validMemcached(uniqueName("np-toggle-mon"))
			mc.Spec.Security = &memcachedv1beta1.SecuritySpec{
				NetworkPolicy: &memcachedv1beta1.NetworkPolicySpec{Enabled: true},
			}
			Expect(k8sClient.Create(ctx, mc)).To(Succeed())

			_, err := reconcileOnce(mc)
			Expect(err).NotTo(HaveOccurred())

			np := fetchNetworkPolicy(mc)
			Expect(np.Spec.Ingress[0].Ports).To(HaveLen(1))

			// Enable monitoring.
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(mc), mc)).To(Succeed())
			mc.Spec.Monitoring = &memcachedv1beta1.MonitoringSpec{Enabled: true}
			Expect(k8sClient.Update(ctx, mc)).To(Succeed())

			_, err = reconcileOnce(mc)
			Expect(err).NotTo(HaveOccurred())

			np = fetchNetworkPolicy(mc)
			Expect(np.Spec.Ingress[0].Ports).To(HaveLen(2))
			Expect(np.Spec.Ingress[0].Ports[1].Port.IntValue()).To(Equal(9150))
		})
	})

	Context("Idempotent NetworkPolicy reconciliation", func() {
		It("should not change NetworkPolicy resource version on second reconcile", func() {
			mc := validMemcached(uniqueName("np-idempotent"))
			mc.Spec.Security = &memcachedv1beta1.SecuritySpec{
				NetworkPolicy: &memcachedv1beta1.NetworkPolicySpec{Enabled: true},
			}
			Expect(k8sClient.Create(ctx, mc)).To(Succeed())

			_, err := reconcileOnce(mc)
			Expect(err).NotTo(HaveOccurred())

			np1 := fetchNetworkPolicy(mc)
			rv1 := np1.ResourceVersion

			// Reconcile again without changes.
			_, err = reconcileOnce(mc)
			Expect(err).NotTo(HaveOccurred())

			np2 := fetchNetworkPolicy(mc)
			Expect(np2.ResourceVersion).To(Equal(rv1))
		})
	})

	Context("NetworkPolicy drift correction (REQ-007)", func() {
		It("should restore NetworkPolicy ports after external modification", func() {
			mc := validMemcached(uniqueName("np-drift-ports"))
			mc.Spec.Security = &memcachedv1beta1.SecuritySpec{
				NetworkPolicy: &memcachedv1beta1.NetworkPolicySpec{Enabled: true},
			}
			Expect(k8sClient.Create(ctx, mc)).To(Succeed())

			_, err := reconcileOnce(mc)
			Expect(err).NotTo(HaveOccurred())

			np := fetchNetworkPolicy(mc)
			Expect(np.Spec.Ingress).To(HaveLen(1))
			Expect(np.Spec.Ingress[0].Ports).To(HaveLen(1))
			Expect(np.Spec.Ingress[0].Ports[0].Port.IntValue()).To(Equal(11211))

			// Simulate drift: externally modify NetworkPolicy ports.
			patch := client.MergeFrom(np.DeepCopy())
			wrongPort := intstr.FromInt32(9999)
			np.Spec.Ingress[0].Ports = []networkingv1.NetworkPolicyPort{
				{Port: &wrongPort},
			}
			Expect(k8sClient.Patch(ctx, np, patch)).To(Succeed())

			drifted := fetchNetworkPolicy(mc)
			Expect(drifted.Spec.Ingress[0].Ports).To(HaveLen(1))
			Expect(drifted.Spec.Ingress[0].Ports[0].Port.IntValue()).To(Equal(9999))

			// Reconcile should restore the correct port.
			_, err = reconcileOnce(mc)
			Expect(err).NotTo(HaveOccurred())

			corrected := fetchNetworkPolicy(mc)
			Expect(corrected.Spec.Ingress[0].Ports).To(HaveLen(1))
			Expect(corrected.Spec.Ingress[0].Ports[0].Port.IntValue()).To(Equal(11211))
		})
	})

	Context("NetworkPolicy ingress from field when allowedSources is nil", func() {
		It("should have nil from field allowing all sources", func() {
			mc := validMemcached(uniqueName("np-nil-sources"))
			mc.Spec.Security = &memcachedv1beta1.SecuritySpec{
				NetworkPolicy: &memcachedv1beta1.NetworkPolicySpec{Enabled: true},
			}
			Expect(k8sClient.Create(ctx, mc)).To(Succeed())

			_, err := reconcileOnce(mc)
			Expect(err).NotTo(HaveOccurred())

			np := fetchNetworkPolicy(mc)
			Expect(np.Spec.Ingress).To(HaveLen(1))
			Expect(np.Spec.Ingress[0].From).To(BeNil())
		})
	})
})
