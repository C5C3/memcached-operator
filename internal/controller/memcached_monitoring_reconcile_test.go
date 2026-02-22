package controller_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	memcachedv1beta1 "github.com/c5c3/memcached-operator/api/v1beta1"
)

var _ = Describe("Monitoring Reconciliation", func() {

	Context("exporter sidecar injection (REQ-001, REQ-002, REQ-003, REQ-004)", func() {

		It("should create Deployment with exporter sidecar and Service with metrics port", func() {
			mc := validMemcached(uniqueName("mon-create"))
			mc.Spec.Monitoring = &memcachedv1beta1.MonitoringSpec{
				Enabled: true,
			}
			Expect(k8sClient.Create(ctx, mc)).To(Succeed())

			result, err := reconcileOnce(mc)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal(ctrl.Result{}))

			// Deployment should have 2 containers.
			dep := fetchDeployment(mc)
			Expect(dep.Spec.Template.Spec.Containers).To(HaveLen(2))
			Expect(dep.Spec.Template.Spec.Containers[0].Name).To(Equal("memcached"))
			Expect(dep.Spec.Template.Spec.Containers[1].Name).To(Equal("exporter"))

			// Exporter container defaults.
			exporter := dep.Spec.Template.Spec.Containers[1]
			Expect(exporter.Image).To(Equal("prom/memcached-exporter:v0.15.4"))
			Expect(exporter.Ports).To(HaveLen(1))
			Expect(exporter.Ports[0].Name).To(Equal("metrics"))
			Expect(exporter.Ports[0].ContainerPort).To(Equal(int32(9150)))
			Expect(exporter.Ports[0].Protocol).To(Equal(corev1.ProtocolTCP))

			// Service should have 2 ports.
			svc := fetchService(mc)
			Expect(svc.Spec.Ports).To(HaveLen(2))
			Expect(svc.Spec.Ports[0].Name).To(Equal("memcached"))
			Expect(svc.Spec.Ports[0].Port).To(Equal(int32(11211)))
			Expect(svc.Spec.Ports[1].Name).To(Equal("metrics"))
			Expect(svc.Spec.Ports[1].Port).To(Equal(int32(9150)))
		})

		It("should use custom exporter image when specified", func() {
			mc := validMemcached(uniqueName("mon-custimg"))
			customImage := "my-registry/memcached-exporter:v1.0.0"
			mc.Spec.Monitoring = &memcachedv1beta1.MonitoringSpec{
				Enabled:       true,
				ExporterImage: &customImage,
			}
			Expect(k8sClient.Create(ctx, mc)).To(Succeed())

			_, err := reconcileOnce(mc)
			Expect(err).NotTo(HaveOccurred())

			dep := fetchDeployment(mc)
			Expect(dep.Spec.Template.Spec.Containers).To(HaveLen(2))
			Expect(dep.Spec.Template.Spec.Containers[1].Image).To(Equal(customImage))
		})

		It("should apply exporter resources when specified", func() {
			mc := validMemcached(uniqueName("mon-res"))
			mc.Spec.Monitoring = &memcachedv1beta1.MonitoringSpec{
				Enabled: true,
				ExporterResources: &corev1.ResourceRequirements{
					Requests: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("50m"),
						corev1.ResourceMemory: resource.MustParse("64Mi"),
					},
					Limits: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("100m"),
						corev1.ResourceMemory: resource.MustParse("128Mi"),
					},
				},
			}
			Expect(k8sClient.Create(ctx, mc)).To(Succeed())

			_, err := reconcileOnce(mc)
			Expect(err).NotTo(HaveOccurred())

			dep := fetchDeployment(mc)
			Expect(dep.Spec.Template.Spec.Containers).To(HaveLen(2))
			exporter := dep.Spec.Template.Spec.Containers[1]
			Expect(exporter.Resources.Requests[corev1.ResourceCPU]).To(Equal(resource.MustParse("50m")))
			Expect(exporter.Resources.Requests[corev1.ResourceMemory]).To(Equal(resource.MustParse("64Mi")))
			Expect(exporter.Resources.Limits[corev1.ResourceCPU]).To(Equal(resource.MustParse("100m")))
			Expect(exporter.Resources.Limits[corev1.ResourceMemory]).To(Equal(resource.MustParse("128Mi")))
		})

		It("should have no exporter resources when exporterResources is nil", func() {
			mc := validMemcached(uniqueName("mon-nilres"))
			mc.Spec.Monitoring = &memcachedv1beta1.MonitoringSpec{
				Enabled: true,
			}
			Expect(k8sClient.Create(ctx, mc)).To(Succeed())

			_, err := reconcileOnce(mc)
			Expect(err).NotTo(HaveOccurred())

			dep := fetchDeployment(mc)
			exporter := dep.Spec.Template.Spec.Containers[1]
			Expect(exporter.Resources.Requests).To(BeEmpty())
			Expect(exporter.Resources.Limits).To(BeEmpty())
		})
	})

	Context("toggling monitoring (REQ-005, REQ-006)", func() {

		It("should add exporter sidecar when monitoring is enabled on existing CR", func() {
			mc := validMemcached(uniqueName("mon-toggle-on"))
			Expect(k8sClient.Create(ctx, mc)).To(Succeed())

			_, err := reconcileOnce(mc)
			Expect(err).NotTo(HaveOccurred())

			// Initially: 1 container, 1 service port.
			dep := fetchDeployment(mc)
			Expect(dep.Spec.Template.Spec.Containers).To(HaveLen(1))
			svc := fetchService(mc)
			Expect(svc.Spec.Ports).To(HaveLen(1))

			// Enable monitoring.
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(mc), mc)).To(Succeed())
			mc.Spec.Monitoring = &memcachedv1beta1.MonitoringSpec{
				Enabled: true,
			}
			Expect(k8sClient.Update(ctx, mc)).To(Succeed())

			_, err = reconcileOnce(mc)
			Expect(err).NotTo(HaveOccurred())

			// After: 2 containers, 2 service ports.
			dep = fetchDeployment(mc)
			Expect(dep.Spec.Template.Spec.Containers).To(HaveLen(2))
			Expect(dep.Spec.Template.Spec.Containers[1].Name).To(Equal("exporter"))

			svc = fetchService(mc)
			Expect(svc.Spec.Ports).To(HaveLen(2))
			Expect(svc.Spec.Ports[1].Name).To(Equal("metrics"))
		})

		It("should remove exporter sidecar when monitoring is disabled", func() {
			mc := validMemcached(uniqueName("mon-toggle-off"))
			mc.Spec.Monitoring = &memcachedv1beta1.MonitoringSpec{
				Enabled: true,
			}
			Expect(k8sClient.Create(ctx, mc)).To(Succeed())

			_, err := reconcileOnce(mc)
			Expect(err).NotTo(HaveOccurred())

			dep := fetchDeployment(mc)
			Expect(dep.Spec.Template.Spec.Containers).To(HaveLen(2))
			svc := fetchService(mc)
			Expect(svc.Spec.Ports).To(HaveLen(2))

			// Disable monitoring.
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(mc), mc)).To(Succeed())
			mc.Spec.Monitoring.Enabled = false
			Expect(k8sClient.Update(ctx, mc)).To(Succeed())

			_, err = reconcileOnce(mc)
			Expect(err).NotTo(HaveOccurred())

			dep = fetchDeployment(mc)
			Expect(dep.Spec.Template.Spec.Containers).To(HaveLen(1))
			Expect(dep.Spec.Template.Spec.Containers[0].Name).To(Equal("memcached"))

			svc = fetchService(mc)
			Expect(svc.Spec.Ports).To(HaveLen(1))
			Expect(svc.Spec.Ports[0].Name).To(Equal("memcached"))
		})

		It("should remove exporter sidecar when monitoring section is removed", func() {
			mc := validMemcached(uniqueName("mon-remove"))
			mc.Spec.Monitoring = &memcachedv1beta1.MonitoringSpec{
				Enabled: true,
			}
			Expect(k8sClient.Create(ctx, mc)).To(Succeed())

			_, err := reconcileOnce(mc)
			Expect(err).NotTo(HaveOccurred())

			dep := fetchDeployment(mc)
			Expect(dep.Spec.Template.Spec.Containers).To(HaveLen(2))

			// Remove monitoring section entirely.
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(mc), mc)).To(Succeed())
			mc.Spec.Monitoring = nil
			Expect(k8sClient.Update(ctx, mc)).To(Succeed())

			_, err = reconcileOnce(mc)
			Expect(err).NotTo(HaveOccurred())

			dep = fetchDeployment(mc)
			Expect(dep.Spec.Template.Spec.Containers).To(HaveLen(1))
			Expect(dep.Spec.Template.Spec.Containers[0].Name).To(Equal("memcached"))

			svc := fetchService(mc)
			Expect(svc.Spec.Ports).To(HaveLen(1))
		})

		It("should update exporter image when exporterImage changes", func() {
			mc := validMemcached(uniqueName("mon-imgchg"))
			mc.Spec.Monitoring = &memcachedv1beta1.MonitoringSpec{
				Enabled: true,
			}
			Expect(k8sClient.Create(ctx, mc)).To(Succeed())

			_, err := reconcileOnce(mc)
			Expect(err).NotTo(HaveOccurred())

			dep := fetchDeployment(mc)
			Expect(dep.Spec.Template.Spec.Containers[1].Image).To(Equal("prom/memcached-exporter:v0.15.4"))

			// Update exporter image.
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(mc), mc)).To(Succeed())
			newImage := "custom/exporter:v2.0.0"
			mc.Spec.Monitoring.ExporterImage = &newImage
			Expect(k8sClient.Update(ctx, mc)).To(Succeed())

			_, err = reconcileOnce(mc)
			Expect(err).NotTo(HaveOccurred())

			dep = fetchDeployment(mc)
			Expect(dep.Spec.Template.Spec.Containers[1].Image).To(Equal(newImage))
		})

		It("should update exporter resources when exporterResources changes", func() {
			mc := validMemcached(uniqueName("mon-reschg"))
			mc.Spec.Monitoring = &memcachedv1beta1.MonitoringSpec{
				Enabled: true,
			}
			Expect(k8sClient.Create(ctx, mc)).To(Succeed())

			_, err := reconcileOnce(mc)
			Expect(err).NotTo(HaveOccurred())

			dep := fetchDeployment(mc)
			Expect(dep.Spec.Template.Spec.Containers[1].Resources.Requests).To(BeEmpty())

			// Add resources.
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(mc), mc)).To(Succeed())
			mc.Spec.Monitoring.ExporterResources = &corev1.ResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceCPU: resource.MustParse("100m"),
				},
			}
			Expect(k8sClient.Update(ctx, mc)).To(Succeed())

			_, err = reconcileOnce(mc)
			Expect(err).NotTo(HaveOccurred())

			dep = fetchDeployment(mc)
			Expect(dep.Spec.Template.Spec.Containers[1].Resources.Requests[corev1.ResourceCPU]).To(Equal(resource.MustParse("100m")))
		})
	})

	Context("idempotency (REQ-007)", func() {

		It("should be idempotent with monitoring enabled", func() {
			mc := validMemcached(uniqueName("mon-idemp"))
			mc.Spec.Monitoring = &memcachedv1beta1.MonitoringSpec{
				Enabled: true,
			}
			Expect(k8sClient.Create(ctx, mc)).To(Succeed())

			_, err := reconcileOnce(mc)
			Expect(err).NotTo(HaveOccurred())

			dep1 := fetchDeployment(mc)
			depRV1 := dep1.ResourceVersion
			svc1 := fetchService(mc)
			svcRV1 := svc1.ResourceVersion

			// Reconcile again without changes.
			_, err = reconcileOnce(mc)
			Expect(err).NotTo(HaveOccurred())

			dep2 := fetchDeployment(mc)
			Expect(dep2.ResourceVersion).To(Equal(depRV1))
			svc2 := fetchService(mc)
			Expect(svc2.ResourceVersion).To(Equal(svcRV1))
		})

		It("should converge after drift (manual container removal)", func() {
			mc := validMemcached(uniqueName("mon-drift"))
			mc.Spec.Monitoring = &memcachedv1beta1.MonitoringSpec{
				Enabled: true,
			}
			Expect(k8sClient.Create(ctx, mc)).To(Succeed())

			_, err := reconcileOnce(mc)
			Expect(err).NotTo(HaveOccurred())

			dep := fetchDeployment(mc)
			Expect(dep.Spec.Template.Spec.Containers).To(HaveLen(2))

			// Simulate drift: manually remove the exporter container.
			patch := client.MergeFrom(dep.DeepCopy())
			dep.Spec.Template.Spec.Containers = dep.Spec.Template.Spec.Containers[:1]
			Expect(k8sClient.Patch(ctx, dep, patch)).To(Succeed())

			drifted := &appsv1.Deployment{}
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(mc), drifted)).To(Succeed())
			Expect(drifted.Spec.Template.Spec.Containers).To(HaveLen(1))

			// Reconcile should restore the exporter container.
			_, err = reconcileOnce(mc)
			Expect(err).NotTo(HaveOccurred())

			corrected := fetchDeployment(mc)
			Expect(corrected.Spec.Template.Spec.Containers).To(HaveLen(2))
			Expect(corrected.Spec.Template.Spec.Containers[1].Name).To(Equal("exporter"))
		})
	})

	Context("coexistence with other HA features", func() {

		It("should coexist with graceful shutdown and anti-affinity", func() {
			mc := validMemcached(uniqueName("mon-coexist"))
			soft := memcachedv1beta1.AntiAffinityPresetSoft
			mc.Spec.HighAvailability = &memcachedv1beta1.HighAvailabilitySpec{
				AntiAffinityPreset: &soft,
				TopologySpreadConstraints: []corev1.TopologySpreadConstraint{
					zoneSpreadConstraint(),
				},
				GracefulShutdown: &memcachedv1beta1.GracefulShutdownSpec{
					Enabled:                       true,
					PreStopDelaySeconds:           10,
					TerminationGracePeriodSeconds: 30,
				},
			}
			mc.Spec.Monitoring = &memcachedv1beta1.MonitoringSpec{
				Enabled: true,
			}
			Expect(k8sClient.Create(ctx, mc)).To(Succeed())

			_, err := reconcileOnce(mc)
			Expect(err).NotTo(HaveOccurred())

			dep := fetchDeployment(mc)

			// Monitoring: 2 containers.
			Expect(dep.Spec.Template.Spec.Containers).To(HaveLen(2))
			Expect(dep.Spec.Template.Spec.Containers[0].Name).To(Equal("memcached"))
			Expect(dep.Spec.Template.Spec.Containers[1].Name).To(Equal("exporter"))

			// Graceful shutdown: preStop hook on memcached container.
			Expect(dep.Spec.Template.Spec.Containers[0].Lifecycle).NotTo(BeNil())
			Expect(dep.Spec.Template.Spec.Containers[0].Lifecycle.PreStop.Exec.Command).To(Equal([]string{"sleep", "10"}))
			Expect(*dep.Spec.Template.Spec.TerminationGracePeriodSeconds).To(Equal(int64(30)))

			// Anti-affinity.
			Expect(dep.Spec.Template.Spec.Affinity).NotTo(BeNil())
			Expect(dep.Spec.Template.Spec.Affinity.PodAntiAffinity).NotTo(BeNil())
			Expect(dep.Spec.Template.Spec.Affinity.PodAntiAffinity.PreferredDuringSchedulingIgnoredDuringExecution).To(HaveLen(1))

			// Topology spread.
			Expect(dep.Spec.Template.Spec.TopologySpreadConstraints).To(HaveLen(1))
			Expect(dep.Spec.Template.Spec.TopologySpreadConstraints[0].TopologyKey).To(Equal("topology.kubernetes.io/zone"))

			// Service: 2 ports.
			svc := fetchService(mc)
			Expect(svc.Spec.Ports).To(HaveLen(2))
			Expect(svc.Spec.Ports[0].Name).To(Equal("memcached"))
			Expect(svc.Spec.Ports[1].Name).To(Equal("metrics"))
		})
	})
})
