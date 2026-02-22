package controller_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	memcachedv1beta1 "github.com/c5c3/memcached-operator/api/v1beta1"
)

const testTLSVolumeName = "tls-certificates"

var _ = Describe("TLS Deployment Reconciliation", func() {

	Context("TLS enabled (REQ-001, REQ-003, REQ-004, REQ-006, REQ-007)", func() {

		It("should add TLS volume, mount, args, and port when TLS is enabled", func() {
			mc := validMemcached(uniqueName("tls-dep-on"))
			mc.Spec.Security = &memcachedv1beta1.SecuritySpec{
				TLS: &memcachedv1beta1.TLSSpec{
					Enabled:              true,
					CertificateSecretRef: corev1.LocalObjectReference{Name: "tls-secret"},
				},
			}
			Expect(k8sClient.Create(ctx, mc)).To(Succeed())

			result, err := reconcileOnce(mc)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal(ctrl.Result{}))

			dep := fetchDeployment(mc)
			container := dep.Spec.Template.Spec.Containers[0]

			// TLS args: -Z, -o ssl_chain_cert=..., -o ssl_key=...
			Expect(container.Args).To(ContainElement("-Z"))
			Expect(container.Args).To(ContainElement("ssl_chain_cert=/etc/memcached/tls/tls.crt"))
			Expect(container.Args).To(ContainElement("ssl_key=/etc/memcached/tls/tls.key"))
			// No ssl_ca_cert without enableClientCert.
			for _, arg := range container.Args {
				Expect(arg).NotTo(ContainSubstring("ssl_ca_cert"))
			}

			// TLS port 11212.
			Expect(container.Ports).To(HaveLen(2))
			Expect(container.Ports[0].Name).To(Equal("memcached"))
			Expect(container.Ports[0].ContainerPort).To(Equal(int32(11211)))
			Expect(container.Ports[1].Name).To(Equal("memcached-tls"))
			Expect(container.Ports[1].ContainerPort).To(Equal(int32(11212)))
			Expect(container.Ports[1].Protocol).To(Equal(corev1.ProtocolTCP))

			// TLS volume mount.
			var tlsMount *corev1.VolumeMount
			for i := range container.VolumeMounts {
				if container.VolumeMounts[i].Name == testTLSVolumeName {
					tlsMount = &container.VolumeMounts[i]
					break
				}
			}
			Expect(tlsMount).NotTo(BeNil())
			Expect(tlsMount.MountPath).To(Equal("/etc/memcached/tls"))
			Expect(tlsMount.ReadOnly).To(BeTrue())

			// TLS volume.
			var tlsVol *corev1.Volume
			for i := range dep.Spec.Template.Spec.Volumes {
				if dep.Spec.Template.Spec.Volumes[i].Name == testTLSVolumeName {
					tlsVol = &dep.Spec.Template.Spec.Volumes[i]
					break
				}
			}
			Expect(tlsVol).NotTo(BeNil())
			Expect(tlsVol.Secret).NotTo(BeNil())
			Expect(tlsVol.Secret.SecretName).To(Equal("tls-secret"))
			Expect(tlsVol.Secret.Items).To(HaveLen(2))
			Expect(tlsVol.Secret.Items[0].Key).To(Equal("tls.crt"))
			Expect(tlsVol.Secret.Items[1].Key).To(Equal("tls.key"))
		})

		It("should include ssl_ca_cert arg and ca.crt item when enableClientCert is true", func() {
			mc := validMemcached(uniqueName("tls-dep-mtls"))
			mc.Spec.Security = &memcachedv1beta1.SecuritySpec{
				TLS: &memcachedv1beta1.TLSSpec{
					Enabled:              true,
					CertificateSecretRef: corev1.LocalObjectReference{Name: "mtls-secret"},
					EnableClientCert:     true,
				},
			}
			Expect(k8sClient.Create(ctx, mc)).To(Succeed())

			_, err := reconcileOnce(mc)
			Expect(err).NotTo(HaveOccurred())

			dep := fetchDeployment(mc)
			container := dep.Spec.Template.Spec.Containers[0]

			// ssl_ca_cert arg present.
			Expect(container.Args).To(ContainElement("ssl_ca_cert=/etc/memcached/tls/ca.crt"))

			// Volume should include ca.crt item.
			var tlsVol *corev1.Volume
			for i := range dep.Spec.Template.Spec.Volumes {
				if dep.Spec.Template.Spec.Volumes[i].Name == testTLSVolumeName {
					tlsVol = &dep.Spec.Template.Spec.Volumes[i]
					break
				}
			}
			Expect(tlsVol).NotTo(BeNil())
			Expect(tlsVol.Secret.Items).To(HaveLen(3))
			Expect(tlsVol.Secret.Items[2].Key).To(Equal("ca.crt"))
		})
	})

	Context("TLS disabled (REQ-007)", func() {

		It("should have no TLS volume, mount, args, or extra port when TLS is not configured", func() {
			mc := validMemcached(uniqueName("tls-dep-off"))
			Expect(k8sClient.Create(ctx, mc)).To(Succeed())

			_, err := reconcileOnce(mc)
			Expect(err).NotTo(HaveOccurred())

			dep := fetchDeployment(mc)
			container := dep.Spec.Template.Spec.Containers[0]

			// No TLS args.
			Expect(container.Args).NotTo(ContainElement("-Z"))

			// Only the standard memcached port.
			Expect(container.Ports).To(HaveLen(1))
			Expect(container.Ports[0].Name).To(Equal("memcached"))

			// No TLS volume mount.
			for _, vm := range container.VolumeMounts {
				Expect(vm.Name).NotTo(Equal(testTLSVolumeName))
			}

			// No TLS volume.
			for _, v := range dep.Spec.Template.Spec.Volumes {
				Expect(v.Name).NotTo(Equal(testTLSVolumeName))
			}
		})
	})

	Context("TLS enable/disable toggle (REQ-007)", func() {

		It("should add TLS configuration when enabling and remove when disabling", func() {
			mc := validMemcached(uniqueName("tls-dep-toggle"))
			Expect(k8sClient.Create(ctx, mc)).To(Succeed())

			// Initial reconcile without TLS.
			_, err := reconcileOnce(mc)
			Expect(err).NotTo(HaveOccurred())

			dep := fetchDeployment(mc)
			Expect(dep.Spec.Template.Spec.Containers[0].Args).NotTo(ContainElement("-Z"))
			Expect(dep.Spec.Template.Spec.Containers[0].Ports).To(HaveLen(1))

			// Enable TLS.
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(mc), mc)).To(Succeed())
			mc.Spec.Security = &memcachedv1beta1.SecuritySpec{
				TLS: &memcachedv1beta1.TLSSpec{
					Enabled:              true,
					CertificateSecretRef: corev1.LocalObjectReference{Name: "tls-secret"},
				},
			}
			Expect(k8sClient.Update(ctx, mc)).To(Succeed())

			_, err = reconcileOnce(mc)
			Expect(err).NotTo(HaveOccurred())

			dep = fetchDeployment(mc)
			Expect(dep.Spec.Template.Spec.Containers[0].Args).To(ContainElement("-Z"))
			Expect(dep.Spec.Template.Spec.Containers[0].Ports).To(HaveLen(2))

			// Verify TLS volume exists.
			var hasTLSVol bool
			for _, v := range dep.Spec.Template.Spec.Volumes {
				if v.Name == testTLSVolumeName {
					hasTLSVol = true
					break
				}
			}
			Expect(hasTLSVol).To(BeTrue())

			// Disable TLS.
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(mc), mc)).To(Succeed())
			mc.Spec.Security = nil
			Expect(k8sClient.Update(ctx, mc)).To(Succeed())

			_, err = reconcileOnce(mc)
			Expect(err).NotTo(HaveOccurred())

			dep = fetchDeployment(mc)
			Expect(dep.Spec.Template.Spec.Containers[0].Args).NotTo(ContainElement("-Z"))
			Expect(dep.Spec.Template.Spec.Containers[0].Ports).To(HaveLen(1))

			// Verify TLS volume removed.
			for _, v := range dep.Spec.Template.Spec.Volumes {
				Expect(v.Name).NotTo(Equal(testTLSVolumeName))
			}
		})
	})

	Context("TLS idempotency (REQ-007)", func() {

		It("should be idempotent when reconciling with TLS enabled", func() {
			mc := validMemcached(uniqueName("tls-dep-idemp"))
			mc.Spec.Security = &memcachedv1beta1.SecuritySpec{
				TLS: &memcachedv1beta1.TLSSpec{
					Enabled:              true,
					CertificateSecretRef: corev1.LocalObjectReference{Name: "tls-secret"},
				},
			}
			Expect(k8sClient.Create(ctx, mc)).To(Succeed())

			_, err := reconcileOnce(mc)
			Expect(err).NotTo(HaveOccurred())

			dep1 := fetchDeployment(mc)
			rv1 := dep1.ResourceVersion

			// Reconcile again without changes.
			_, err = reconcileOnce(mc)
			Expect(err).NotTo(HaveOccurred())

			dep2 := fetchDeployment(mc)
			Expect(dep2.ResourceVersion).To(Equal(rv1))
		})

		It("should be idempotent when reconciling with mTLS enabled", func() {
			mc := validMemcached(uniqueName("tls-dep-idemp-mtls"))
			mc.Spec.Security = &memcachedv1beta1.SecuritySpec{
				TLS: &memcachedv1beta1.TLSSpec{
					Enabled:              true,
					CertificateSecretRef: corev1.LocalObjectReference{Name: "mtls-secret"},
					EnableClientCert:     true,
				},
			}
			Expect(k8sClient.Create(ctx, mc)).To(Succeed())

			_, err := reconcileOnce(mc)
			Expect(err).NotTo(HaveOccurred())

			dep1 := fetchDeployment(mc)
			rv1 := dep1.ResourceVersion

			// Reconcile again without changes.
			_, err = reconcileOnce(mc)
			Expect(err).NotTo(HaveOccurred())

			dep2 := fetchDeployment(mc)
			Expect(dep2.ResourceVersion).To(Equal(rv1))
		})
	})

	Context("TLS coexistence with SASL (REQ-007)", func() {

		It("should support TLS and SASL simultaneously", func() {
			mc := validMemcached(uniqueName("tls-dep-sasl"))
			mc.Spec.Security = &memcachedv1beta1.SecuritySpec{
				SASL: &memcachedv1beta1.SASLSpec{
					Enabled:              true,
					CredentialsSecretRef: corev1.LocalObjectReference{Name: "sasl-secret"},
				},
				TLS: &memcachedv1beta1.TLSSpec{
					Enabled:              true,
					CertificateSecretRef: corev1.LocalObjectReference{Name: "tls-secret"},
				},
			}
			Expect(k8sClient.Create(ctx, mc)).To(Succeed())

			_, err := reconcileOnce(mc)
			Expect(err).NotTo(HaveOccurred())

			dep := fetchDeployment(mc)
			container := dep.Spec.Template.Spec.Containers[0]

			// Both SASL and TLS args present.
			Expect(container.Args).To(ContainElement("-Y"))
			Expect(container.Args).To(ContainElement("-Z"))

			// Both volume mounts.
			var hasSASLMount, hasTLSMount bool
			for _, vm := range container.VolumeMounts {
				if vm.Name == "sasl-credentials" {
					hasSASLMount = true
				}
				if vm.Name == testTLSVolumeName {
					hasTLSMount = true
				}
			}
			Expect(hasSASLMount).To(BeTrue())
			Expect(hasTLSMount).To(BeTrue())

			// Both volumes.
			var hasSASLVol, hasTLSVol bool
			for _, v := range dep.Spec.Template.Spec.Volumes {
				if v.Name == "sasl-credentials" {
					hasSASLVol = true
				}
				if v.Name == testTLSVolumeName {
					hasTLSVol = true
				}
			}
			Expect(hasSASLVol).To(BeTrue())
			Expect(hasTLSVol).To(BeTrue())

			// TLS port present.
			Expect(container.Ports).To(HaveLen(2))
		})
	})

	Context("TLS secret reference update (REQ-007)", func() {

		It("should update Deployment when TLS secret reference changes", func() {
			mc := validMemcached(uniqueName("tls-dep-secupd"))
			mc.Spec.Security = &memcachedv1beta1.SecuritySpec{
				TLS: &memcachedv1beta1.TLSSpec{
					Enabled:              true,
					CertificateSecretRef: corev1.LocalObjectReference{Name: "tls-secret-v1"},
				},
			}
			Expect(k8sClient.Create(ctx, mc)).To(Succeed())

			_, err := reconcileOnce(mc)
			Expect(err).NotTo(HaveOccurred())

			dep := fetchDeployment(mc)
			var tlsVol *corev1.Volume
			for i := range dep.Spec.Template.Spec.Volumes {
				if dep.Spec.Template.Spec.Volumes[i].Name == testTLSVolumeName {
					tlsVol = &dep.Spec.Template.Spec.Volumes[i]
					break
				}
			}
			Expect(tlsVol).NotTo(BeNil())
			Expect(tlsVol.Secret.SecretName).To(Equal("tls-secret-v1"))

			// Update secret reference.
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(mc), mc)).To(Succeed())
			mc.Spec.Security.TLS.CertificateSecretRef.Name = "tls-secret-v2"
			Expect(k8sClient.Update(ctx, mc)).To(Succeed())

			_, err = reconcileOnce(mc)
			Expect(err).NotTo(HaveOccurred())

			dep = fetchDeployment(mc)
			for i := range dep.Spec.Template.Spec.Volumes {
				if dep.Spec.Template.Spec.Volumes[i].Name == testTLSVolumeName {
					tlsVol = &dep.Spec.Template.Spec.Volumes[i]
					break
				}
			}
			Expect(tlsVol.Secret.SecretName).To(Equal("tls-secret-v2"))
		})
	})
})
