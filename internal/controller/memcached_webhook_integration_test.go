package controller_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"sigs.k8s.io/controller-runtime/pkg/client"

	memcachedv1beta1 "github.com/c5c3/memcached-operator/api/v1beta1"
)

var _ = Describe("Webhook Defaulting via API Server", func() {

	Context("minimal CR with empty spec", func() {
		It("should apply all webhook defaults to a minimal CR", func() {
			mc := validMemcachedBeta(uniqueName("wh-minimal"))
			Expect(k8sClient.Create(ctx, mc)).To(Succeed())

			fetched := &memcachedv1beta1.Memcached{}
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(mc), fetched)).To(Succeed())

			// REQ-001: replicas defaults to 1.
			Expect(fetched.Spec.Replicas).NotTo(BeNil())
			Expect(*fetched.Spec.Replicas).To(Equal(int32(1)))

			// REQ-002: image defaults to memcached:1.6.
			Expect(fetched.Spec.Image).NotTo(BeNil())
			Expect(*fetched.Spec.Image).To(Equal("memcached:1.6"))

			// REQ-003: memcached config is initialized with all defaults.
			Expect(fetched.Spec.Memcached).NotTo(BeNil())
			Expect(fetched.Spec.Memcached.MaxMemoryMB).To(Equal(int32(64)))
			Expect(fetched.Spec.Memcached.MaxConnections).To(Equal(int32(1024)))
			Expect(fetched.Spec.Memcached.Threads).To(Equal(int32(4)))
			Expect(fetched.Spec.Memcached.MaxItemSize).To(Equal("1m"))
			Expect(fetched.Spec.Memcached.Verbosity).To(Equal(int32(0)))

			// Optional sections remain nil (opt-in).
			Expect(fetched.Spec.Monitoring).To(BeNil())
			Expect(fetched.Spec.HighAvailability).To(BeNil())
		})
	})

	Context("fully specified CR", func() {
		It("should not modify a CR with all fields explicitly set", func() {
			replicas := int32(5)
			image := "memcached:1.6.28"
			exporterImage := "custom/exporter:v2"
			preset := memcachedv1beta1.AntiAffinityPresetHard

			mc := validMemcachedBeta(uniqueName("wh-full"))
			mc.Spec = memcachedv1beta1.MemcachedSpec{
				Replicas: &replicas,
				Image:    &image,
				Memcached: &memcachedv1beta1.MemcachedConfig{
					MaxMemoryMB:    512,
					MaxConnections: 4096,
					Threads:        16,
					MaxItemSize:    "4m",
					Verbosity:      1,
					ExtraArgs:      []string{"-o", "modern"},
				},
				Monitoring: &memcachedv1beta1.MonitoringSpec{
					Enabled:       true,
					ExporterImage: &exporterImage,
					ServiceMonitor: &memcachedv1beta1.ServiceMonitorSpec{
						Interval:      "15s",
						ScrapeTimeout: "5s",
					},
				},
				HighAvailability: &memcachedv1beta1.HighAvailabilitySpec{
					AntiAffinityPreset: &preset,
				},
			}
			Expect(k8sClient.Create(ctx, mc)).To(Succeed())

			fetched := &memcachedv1beta1.Memcached{}
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(mc), fetched)).To(Succeed())

			// All explicit values must be preserved.
			Expect(*fetched.Spec.Replicas).To(Equal(int32(5)))
			Expect(*fetched.Spec.Image).To(Equal("memcached:1.6.28"))
			Expect(fetched.Spec.Memcached.MaxMemoryMB).To(Equal(int32(512)))
			Expect(fetched.Spec.Memcached.MaxConnections).To(Equal(int32(4096)))
			Expect(fetched.Spec.Memcached.Threads).To(Equal(int32(16)))
			Expect(fetched.Spec.Memcached.MaxItemSize).To(Equal("4m"))
			Expect(fetched.Spec.Memcached.Verbosity).To(Equal(int32(1)))
			Expect(fetched.Spec.Memcached.ExtraArgs).To(Equal([]string{"-o", "modern"}))
			Expect(*fetched.Spec.Monitoring.ExporterImage).To(Equal("custom/exporter:v2"))
			Expect(fetched.Spec.Monitoring.ServiceMonitor.Interval).To(Equal("15s"))
			Expect(fetched.Spec.Monitoring.ServiceMonitor.ScrapeTimeout).To(Equal("5s"))
			Expect(*fetched.Spec.HighAvailability.AntiAffinityPreset).To(Equal(memcachedv1beta1.AntiAffinityPresetHard))
		})
	})
})
