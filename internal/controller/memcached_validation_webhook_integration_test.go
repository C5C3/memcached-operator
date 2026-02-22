package controller_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	memcachedv1beta1 "github.com/c5c3/memcached-operator/api/v1beta1"
)

var _ = Describe("Webhook Validation via API Server", func() {

	Context("rejects insufficient memory limit", func() {
		It("should reject a CR where resources.limits.memory < maxMemoryMB + 32Mi overhead", func() {
			mc := validMemcachedBeta(uniqueName("val-mem"))
			mc.Spec.Memcached = &memcachedv1beta1.MemcachedConfig{
				MaxMemoryMB: 64,
			}
			mc.Spec.Resources = &corev1.ResourceRequirements{
				Limits: corev1.ResourceList{
					corev1.ResourceMemory: resource.MustParse("64Mi"),
				},
			}
			err := k8sClient.Create(ctx, mc)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("spec.resources.limits.memory"))
		})
	})

	Context("rejects PDB minAvailable >= replicas", func() {
		It("should reject a CR where minAvailable equals replicas", func() {
			mc := validMemcachedBeta(uniqueName("val-pdb"))
			mc.Spec.Replicas = int32Ptr(3)
			mc.Spec.HighAvailability = &memcachedv1beta1.HighAvailabilitySpec{
				PodDisruptionBudget: &memcachedv1beta1.PDBSpec{
					Enabled:      true,
					MinAvailable: &intstr.IntOrString{Type: intstr.Int, IntVal: 3},
				},
			}
			err := k8sClient.Create(ctx, mc)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("spec.highAvailability.podDisruptionBudget.minAvailable"))
		})
	})

	Context("rejects PDB mutual exclusivity", func() {
		It("should reject a CR with both minAvailable and maxUnavailable set", func() {
			mc := validMemcachedBeta(uniqueName("val-pdb-mut"))
			mc.Spec.HighAvailability = &memcachedv1beta1.HighAvailabilitySpec{
				PodDisruptionBudget: &memcachedv1beta1.PDBSpec{
					Enabled:        true,
					MinAvailable:   &intstr.IntOrString{Type: intstr.Int, IntVal: 1},
					MaxUnavailable: &intstr.IntOrString{Type: intstr.Int, IntVal: 1},
				},
			}
			err := k8sClient.Create(ctx, mc)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("spec.highAvailability.podDisruptionBudget"))
			Expect(err.Error()).To(ContainSubstring("mutually exclusive"))
		})
	})

	Context("rejects SASL without secret", func() {
		It("should reject a CR with SASL enabled but no credentialsSecretRef", func() {
			mc := validMemcachedBeta(uniqueName("val-sasl"))
			mc.Spec.Security = &memcachedv1beta1.SecuritySpec{
				SASL: &memcachedv1beta1.SASLSpec{
					Enabled: true,
				},
			}
			err := k8sClient.Create(ctx, mc)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("spec.security.sasl.credentialsSecretRef.name"))
		})
	})

	Context("rejects TLS without secret", func() {
		It("should reject a CR with TLS enabled but no certificateSecretRef", func() {
			mc := validMemcachedBeta(uniqueName("val-tls"))
			mc.Spec.Security = &memcachedv1beta1.SecuritySpec{
				TLS: &memcachedv1beta1.TLSSpec{
					Enabled: true,
				},
			}
			err := k8sClient.Create(ctx, mc)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("spec.security.tls.certificateSecretRef.name"))
		})
	})

	Context("rejects graceful shutdown timing violation", func() {
		It("should reject a CR where terminationGracePeriodSeconds <= preStopDelaySeconds", func() {
			mc := validMemcachedBeta(uniqueName("val-gs"))
			mc.Spec.HighAvailability = &memcachedv1beta1.HighAvailabilitySpec{
				GracefulShutdown: &memcachedv1beta1.GracefulShutdownSpec{
					Enabled:                       true,
					PreStopDelaySeconds:           10,
					TerminationGracePeriodSeconds: 10,
				},
			}
			err := k8sClient.Create(ctx, mc)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("spec.highAvailability.gracefulShutdown.terminationGracePeriodSeconds"))
		})
	})

	Context("accepts valid CR with all features", func() {
		It("should accept a fully valid CR with all optional sections configured correctly", func() {
			mc := validMemcachedBeta(uniqueName("val-full"))
			mc.Spec.Replicas = int32Ptr(3)
			mc.Spec.Memcached = &memcachedv1beta1.MemcachedConfig{
				MaxMemoryMB: 64,
			}
			mc.Spec.Resources = &corev1.ResourceRequirements{
				Limits: corev1.ResourceList{
					corev1.ResourceMemory: resource.MustParse("128Mi"),
				},
			}
			mc.Spec.HighAvailability = &memcachedv1beta1.HighAvailabilitySpec{
				PodDisruptionBudget: &memcachedv1beta1.PDBSpec{
					Enabled:      true,
					MinAvailable: &intstr.IntOrString{Type: intstr.Int, IntVal: 2},
				},
				GracefulShutdown: &memcachedv1beta1.GracefulShutdownSpec{
					Enabled:                       true,
					PreStopDelaySeconds:           10,
					TerminationGracePeriodSeconds: 30,
				},
			}
			mc.Spec.Security = &memcachedv1beta1.SecuritySpec{
				SASL: &memcachedv1beta1.SASLSpec{
					Enabled:              true,
					CredentialsSecretRef: corev1.LocalObjectReference{Name: "sasl-secret"},
				},
				TLS: &memcachedv1beta1.TLSSpec{
					Enabled:              true,
					CertificateSecretRef: corev1.LocalObjectReference{Name: "tls-cert"},
				},
			}
			Expect(k8sClient.Create(ctx, mc)).To(Succeed())
		})
	})

	Context("minimal CR passes after defaulting", func() {
		It("should accept a minimal empty-spec CR because defaulting fills required values", func() {
			mc := validMemcachedBeta(uniqueName("val-minimal"))
			Expect(k8sClient.Create(ctx, mc)).To(Succeed())
		})
	})

	Context("rejects update to invalid config", func() {
		It("should reject an update that changes a valid CR to an invalid configuration", func() {
			mc := validMemcachedBeta(uniqueName("val-upd"))
			mc.Spec.Security = &memcachedv1beta1.SecuritySpec{
				SASL: &memcachedv1beta1.SASLSpec{
					Enabled:              true,
					CredentialsSecretRef: corev1.LocalObjectReference{Name: "sasl-secret"},
				},
			}
			Expect(k8sClient.Create(ctx, mc)).To(Succeed())

			// Fetch the created resource to get the latest resourceVersion.
			fetched := &memcachedv1beta1.Memcached{}
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(mc), fetched)).To(Succeed())

			// Remove the secret ref, making the CR invalid.
			fetched.Spec.Security.SASL.CredentialsSecretRef = corev1.LocalObjectReference{Name: ""}
			err := k8sClient.Update(ctx, fetched)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("spec.security.sasl.credentialsSecretRef.name"))
		})
	})
})
