package controller_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	memcachedv1alpha1 "github.com/c5c3/memcached-operator/api/v1alpha1"
)

var _ = Describe("Security Context Reconciliation", func() {

	Context("pod and container security contexts (REQ-001, REQ-002, REQ-003, REQ-007)", func() {

		It("should apply pod and container security contexts to Deployment", func() {
			mc := validMemcached(uniqueName("sec-apply"))
			runAsNonRoot := true
			fsGroup := int64(11211)
			runAsUser := int64(11211)
			readOnly := true
			noPrivEsc := false
			mc.Spec.Security = &memcachedv1alpha1.SecuritySpec{
				PodSecurityContext: &corev1.PodSecurityContext{
					RunAsNonRoot: &runAsNonRoot,
					FSGroup:      &fsGroup,
					SeccompProfile: &corev1.SeccompProfile{
						Type: corev1.SeccompProfileTypeRuntimeDefault,
					},
				},
				ContainerSecurityContext: &corev1.SecurityContext{
					RunAsUser:                &runAsUser,
					RunAsNonRoot:             &runAsNonRoot,
					ReadOnlyRootFilesystem:   &readOnly,
					AllowPrivilegeEscalation: &noPrivEsc,
					Capabilities: &corev1.Capabilities{
						Drop: []corev1.Capability{"ALL"},
					},
				},
			}
			Expect(k8sClient.Create(ctx, mc)).To(Succeed())

			result, err := reconcileOnce(mc)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal(ctrl.Result{}))

			dep := fetchDeployment(mc)

			// Pod security context — full restricted profile.
			podSC := dep.Spec.Template.Spec.SecurityContext
			Expect(podSC).NotTo(BeNil())
			Expect(*podSC.RunAsNonRoot).To(BeTrue())
			Expect(*podSC.FSGroup).To(Equal(int64(11211)))
			Expect(podSC.SeccompProfile).NotTo(BeNil())
			Expect(podSC.SeccompProfile.Type).To(Equal(corev1.SeccompProfileTypeRuntimeDefault))

			// Container security context — full restricted profile.
			containerSC := dep.Spec.Template.Spec.Containers[0].SecurityContext
			Expect(containerSC).NotTo(BeNil())
			Expect(*containerSC.RunAsUser).To(Equal(int64(11211)))
			Expect(*containerSC.RunAsNonRoot).To(BeTrue())
			Expect(*containerSC.ReadOnlyRootFilesystem).To(BeTrue())
			Expect(*containerSC.AllowPrivilegeEscalation).To(BeFalse())
			Expect(containerSC.Capabilities).NotTo(BeNil())
			Expect(containerSC.Capabilities.Drop).To(ConsistOf(corev1.Capability("ALL")))
		})

		It("should apply only pod security context when containerSecurityContext is nil", func() {
			mc := validMemcached(uniqueName("sec-pod-only"))
			runAsNonRoot := true
			fsGroup := int64(11211)
			mc.Spec.Security = &memcachedv1alpha1.SecuritySpec{
				PodSecurityContext: &corev1.PodSecurityContext{
					RunAsNonRoot: &runAsNonRoot,
					FSGroup:      &fsGroup,
				},
			}
			Expect(k8sClient.Create(ctx, mc)).To(Succeed())

			_, err := reconcileOnce(mc)
			Expect(err).NotTo(HaveOccurred())

			dep := fetchDeployment(mc)

			// Pod security context applied.
			podSC := dep.Spec.Template.Spec.SecurityContext
			Expect(podSC).NotTo(BeNil())
			Expect(*podSC.RunAsNonRoot).To(BeTrue())
			Expect(*podSC.FSGroup).To(Equal(int64(11211)))

			// Container security context not set.
			Expect(dep.Spec.Template.Spec.Containers[0].SecurityContext).To(
				Or(BeNil(), Equal(&corev1.SecurityContext{})))
		})

		It("should apply only container security context when podSecurityContext is nil", func() {
			mc := validMemcached(uniqueName("sec-ctr-only"))
			runAsUser := int64(1000)
			readOnly := true
			mc.Spec.Security = &memcachedv1alpha1.SecuritySpec{
				ContainerSecurityContext: &corev1.SecurityContext{
					RunAsUser:              &runAsUser,
					ReadOnlyRootFilesystem: &readOnly,
				},
			}
			Expect(k8sClient.Create(ctx, mc)).To(Succeed())

			_, err := reconcileOnce(mc)
			Expect(err).NotTo(HaveOccurred())

			dep := fetchDeployment(mc)

			// Pod security context not set.
			Expect(dep.Spec.Template.Spec.SecurityContext).To(
				Or(BeNil(), Equal(&corev1.PodSecurityContext{})))

			// Container security context applied.
			containerSC := dep.Spec.Template.Spec.Containers[0].SecurityContext
			Expect(containerSC).NotTo(BeNil())
			Expect(*containerSC.RunAsUser).To(Equal(int64(1000)))
			Expect(*containerSC.ReadOnlyRootFilesystem).To(BeTrue())
		})

		It("should have nil security contexts when security is nil", func() {
			mc := validMemcached(uniqueName("sec-nil"))
			Expect(k8sClient.Create(ctx, mc)).To(Succeed())

			_, err := reconcileOnce(mc)
			Expect(err).NotTo(HaveOccurred())

			dep := fetchDeployment(mc)
			// The API server may return an empty struct instead of nil for PodSecurityContext.
			Expect(dep.Spec.Template.Spec.SecurityContext).To(
				Or(BeNil(), Equal(&corev1.PodSecurityContext{})))
			Expect(dep.Spec.Template.Spec.Containers[0].SecurityContext).To(
				Or(BeNil(), Equal(&corev1.SecurityContext{})))
		})

		It("should apply security contexts to exporter sidecar", func() {
			mc := validMemcached(uniqueName("sec-exp"))
			runAsUser := int64(1000)
			readOnly := true
			mc.Spec.Security = &memcachedv1alpha1.SecuritySpec{
				ContainerSecurityContext: &corev1.SecurityContext{
					RunAsUser:              &runAsUser,
					ReadOnlyRootFilesystem: &readOnly,
				},
			}
			mc.Spec.Monitoring = &memcachedv1alpha1.MonitoringSpec{
				Enabled: true,
			}
			Expect(k8sClient.Create(ctx, mc)).To(Succeed())

			_, err := reconcileOnce(mc)
			Expect(err).NotTo(HaveOccurred())

			dep := fetchDeployment(mc)
			Expect(dep.Spec.Template.Spec.Containers).To(HaveLen(2))

			// Memcached container.
			mcSC := dep.Spec.Template.Spec.Containers[0].SecurityContext
			Expect(mcSC).NotTo(BeNil())
			Expect(*mcSC.RunAsUser).To(Equal(int64(1000)))

			// Exporter container.
			expSC := dep.Spec.Template.Spec.Containers[1].SecurityContext
			Expect(expSC).NotTo(BeNil())
			Expect(*expSC.RunAsUser).To(Equal(int64(1000)))
			Expect(*expSC.ReadOnlyRootFilesystem).To(BeTrue())
		})

		It("should update Deployment when security contexts change", func() {
			mc := validMemcached(uniqueName("sec-update"))
			Expect(k8sClient.Create(ctx, mc)).To(Succeed())

			_, err := reconcileOnce(mc)
			Expect(err).NotTo(HaveOccurred())

			dep := fetchDeployment(mc)
			// The API server may return an empty struct instead of nil for PodSecurityContext.
			Expect(dep.Spec.Template.Spec.SecurityContext).To(
				Or(BeNil(), Equal(&corev1.PodSecurityContext{})))

			// Add security contexts.
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(mc), mc)).To(Succeed())
			runAsNonRoot := true
			runAsUser := int64(1000)
			mc.Spec.Security = &memcachedv1alpha1.SecuritySpec{
				PodSecurityContext: &corev1.PodSecurityContext{
					RunAsNonRoot: &runAsNonRoot,
				},
				ContainerSecurityContext: &corev1.SecurityContext{
					RunAsUser: &runAsUser,
				},
			}
			Expect(k8sClient.Update(ctx, mc)).To(Succeed())

			_, err = reconcileOnce(mc)
			Expect(err).NotTo(HaveOccurred())

			dep = fetchDeployment(mc)
			Expect(dep.Spec.Template.Spec.SecurityContext).NotTo(BeNil())
			Expect(*dep.Spec.Template.Spec.SecurityContext.RunAsNonRoot).To(BeTrue())
			Expect(dep.Spec.Template.Spec.Containers[0].SecurityContext).NotTo(BeNil())
			Expect(*dep.Spec.Template.Spec.Containers[0].SecurityContext.RunAsUser).To(Equal(int64(1000)))

			// Change runAsUser to verify update propagation.
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(mc), mc)).To(Succeed())
			updatedRunAsUser := int64(11211)
			mc.Spec.Security.ContainerSecurityContext.RunAsUser = &updatedRunAsUser
			Expect(k8sClient.Update(ctx, mc)).To(Succeed())

			_, err = reconcileOnce(mc)
			Expect(err).NotTo(HaveOccurred())

			dep = fetchDeployment(mc)
			Expect(dep.Spec.Template.Spec.Containers[0].SecurityContext).NotTo(BeNil())
			Expect(*dep.Spec.Template.Spec.Containers[0].SecurityContext.RunAsUser).To(Equal(int64(11211)))

			// Remove security contexts.
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(mc), mc)).To(Succeed())
			mc.Spec.Security = nil
			Expect(k8sClient.Update(ctx, mc)).To(Succeed())

			_, err = reconcileOnce(mc)
			Expect(err).NotTo(HaveOccurred())

			dep = fetchDeployment(mc)
			// The API server may return an empty struct instead of nil after clearing.
			Expect(dep.Spec.Template.Spec.SecurityContext).To(
				Or(BeNil(), Equal(&corev1.PodSecurityContext{})))
			Expect(dep.Spec.Template.Spec.Containers[0].SecurityContext).To(
				Or(BeNil(), Equal(&corev1.SecurityContext{})))
		})

		It("should be idempotent with security contexts", func() {
			mc := validMemcached(uniqueName("sec-idemp"))
			runAsNonRoot := true
			runAsUser := int64(1000)
			mc.Spec.Security = &memcachedv1alpha1.SecuritySpec{
				PodSecurityContext: &corev1.PodSecurityContext{
					RunAsNonRoot: &runAsNonRoot,
				},
				ContainerSecurityContext: &corev1.SecurityContext{
					RunAsUser: &runAsUser,
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

		It("should coexist with all features", func() {
			mc := validMemcached(uniqueName("sec-coex"))
			runAsNonRoot := true
			runAsUser := int64(1000)
			soft := memcachedv1alpha1.AntiAffinityPresetSoft
			mc.Spec.Security = &memcachedv1alpha1.SecuritySpec{
				PodSecurityContext: &corev1.PodSecurityContext{
					RunAsNonRoot: &runAsNonRoot,
				},
				ContainerSecurityContext: &corev1.SecurityContext{
					RunAsUser: &runAsUser,
				},
			}
			mc.Spec.Monitoring = &memcachedv1alpha1.MonitoringSpec{
				Enabled: true,
			}
			mc.Spec.HighAvailability = &memcachedv1alpha1.HighAvailabilitySpec{
				AntiAffinityPreset:        &soft,
				TopologySpreadConstraints: []corev1.TopologySpreadConstraint{zoneSpreadConstraint()},
				GracefulShutdown: &memcachedv1alpha1.GracefulShutdownSpec{
					Enabled:                       true,
					PreStopDelaySeconds:           10,
					TerminationGracePeriodSeconds: 30,
				},
			}
			Expect(k8sClient.Create(ctx, mc)).To(Succeed())

			_, err := reconcileOnce(mc)
			Expect(err).NotTo(HaveOccurred())

			dep := fetchDeployment(mc)

			// Security contexts.
			Expect(dep.Spec.Template.Spec.SecurityContext).NotTo(BeNil())
			Expect(*dep.Spec.Template.Spec.SecurityContext.RunAsNonRoot).To(BeTrue())
			Expect(dep.Spec.Template.Spec.Containers[0].SecurityContext).NotTo(BeNil())
			Expect(*dep.Spec.Template.Spec.Containers[0].SecurityContext.RunAsUser).To(Equal(int64(1000)))

			// Monitoring: 2 containers, both with container security context.
			Expect(dep.Spec.Template.Spec.Containers).To(HaveLen(2))
			Expect(dep.Spec.Template.Spec.Containers[1].Name).To(Equal("exporter"))
			Expect(dep.Spec.Template.Spec.Containers[1].SecurityContext).NotTo(BeNil())
			Expect(*dep.Spec.Template.Spec.Containers[1].SecurityContext.RunAsUser).To(Equal(int64(1000)))

			// Anti-affinity.
			Expect(dep.Spec.Template.Spec.Affinity).NotTo(BeNil())
			Expect(dep.Spec.Template.Spec.Affinity.PodAntiAffinity).NotTo(BeNil())
			Expect(dep.Spec.Template.Spec.Affinity.PodAntiAffinity.PreferredDuringSchedulingIgnoredDuringExecution).To(HaveLen(1))

			// Topology spread.
			Expect(dep.Spec.Template.Spec.TopologySpreadConstraints).To(HaveLen(1))
			Expect(dep.Spec.Template.Spec.TopologySpreadConstraints[0].TopologyKey).To(Equal("topology.kubernetes.io/zone"))

			// Graceful shutdown.
			Expect(dep.Spec.Template.Spec.Containers[0].Lifecycle).NotTo(BeNil())
			Expect(dep.Spec.Template.Spec.Containers[0].Lifecycle.PreStop.Exec.Command).To(Equal([]string{"sleep", "10"}))
			Expect(*dep.Spec.Template.Spec.TerminationGracePeriodSeconds).To(Equal(int64(30)))
		})
	})
})
