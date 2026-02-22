package controller_test

import (
	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	memcachedv1alpha1 "github.com/c5c3/memcached-operator/api/v1alpha1"
	memcachedv1beta1 "github.com/c5c3/memcached-operator/api/v1beta1"
)

// uniqueName returns a unique resource name for test isolation.
func uniqueName(prefix string) string {
	return prefix + "-" + uuid.New().String()[:8]
}

// validMemcached returns a minimal valid Memcached resource for use in tests.
func validMemcached(name string) *memcachedv1alpha1.Memcached {
	return &memcachedv1alpha1.Memcached{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: "default",
		},
		Spec: memcachedv1alpha1.MemcachedSpec{},
	}
}

// validMemcachedBeta returns a minimal valid v1beta1 Memcached resource for use in tests.
func validMemcachedBeta(name string) *memcachedv1beta1.Memcached {
	return &memcachedv1beta1.Memcached{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: "default",
		},
		Spec: memcachedv1beta1.MemcachedSpec{},
	}
}

func int32Ptr(i int32) *int32 { return &i }
func strPtr(s string) *string { return &s }

// --- Task 4.1: MemcachedConfig and MemcachedSpec field validation (REQ-001, REQ-005) ---

var _ = Describe("CRD Validation: MemcachedConfig and Spec fields", func() {

	Context("minimal valid resource", func() {
		It("should accept a Memcached with empty spec", func() {
			mc := validMemcached(uniqueName("minimal"))
			Expect(k8sClient.Create(ctx, mc)).To(Succeed())
		})
	})

	Context("spec.replicas validation", func() {
		It("should accept replicas=0", func() {
			mc := validMemcached(uniqueName("rep0"))
			mc.Spec.Replicas = int32Ptr(0)
			Expect(k8sClient.Create(ctx, mc)).To(Succeed())
		})

		It("should accept replicas=64", func() {
			mc := validMemcached(uniqueName("rep64"))
			mc.Spec.Replicas = int32Ptr(64)
			Expect(k8sClient.Create(ctx, mc)).To(Succeed())
		})

		It("should reject replicas > 64", func() {
			mc := validMemcached(uniqueName("rep65"))
			mc.Spec.Replicas = int32Ptr(65)
			Expect(k8sClient.Create(ctx, mc)).NotTo(Succeed())
		})

		It("should reject negative replicas", func() {
			mc := validMemcached(uniqueName("repneg"))
			mc.Spec.Replicas = int32Ptr(-1)
			Expect(k8sClient.Create(ctx, mc)).NotTo(Succeed())
		})
	})

	Context("spec.memcached.maxMemoryMB validation", func() {
		It("should accept maxMemoryMB=16", func() {
			mc := validMemcached(uniqueName("mem16"))
			mc.Spec.Memcached = &memcachedv1alpha1.MemcachedConfig{
				MaxMemoryMB: 16,
			}
			Expect(k8sClient.Create(ctx, mc)).To(Succeed())
		})

		It("should accept maxMemoryMB=65536", func() {
			mc := validMemcached(uniqueName("mem65536"))
			mc.Spec.Memcached = &memcachedv1alpha1.MemcachedConfig{
				MaxMemoryMB: 65536,
			}
			Expect(k8sClient.Create(ctx, mc)).To(Succeed())
		})

		It("should reject maxMemoryMB=15", func() {
			mc := validMemcached(uniqueName("mem15"))
			mc.Spec.Memcached = &memcachedv1alpha1.MemcachedConfig{
				MaxMemoryMB: 15,
			}
			Expect(k8sClient.Create(ctx, mc)).NotTo(Succeed())
		})

		It("should reject maxMemoryMB > 65536", func() {
			mc := validMemcached(uniqueName("memover"))
			mc.Spec.Memcached = &memcachedv1alpha1.MemcachedConfig{
				MaxMemoryMB: 65537,
			}
			Expect(k8sClient.Create(ctx, mc)).NotTo(Succeed())
		})
	})

	Context("spec.memcached.maxConnections validation", func() {
		It("should accept maxConnections=1", func() {
			mc := validMemcached(uniqueName("conn1"))
			mc.Spec.Memcached = &memcachedv1alpha1.MemcachedConfig{
				MaxConnections: 1,
			}
			Expect(k8sClient.Create(ctx, mc)).To(Succeed())
		})

		It("should accept maxConnections=65536", func() {
			mc := validMemcached(uniqueName("conn65536"))
			mc.Spec.Memcached = &memcachedv1alpha1.MemcachedConfig{
				MaxConnections: 65536,
			}
			Expect(k8sClient.Create(ctx, mc)).To(Succeed())
		})

		It("should accept maxConnections=0 (omitempty sends zero value as omitted, server applies default)", func() {
			mc := validMemcached(uniqueName("conn0"))
			mc.Spec.Memcached = &memcachedv1alpha1.MemcachedConfig{
				MaxConnections: 0,
			}
			Expect(k8sClient.Create(ctx, mc)).To(Succeed())
		})

		It("should reject maxConnections > 65536", func() {
			mc := validMemcached(uniqueName("connover"))
			mc.Spec.Memcached = &memcachedv1alpha1.MemcachedConfig{
				MaxConnections: 65537,
			}
			Expect(k8sClient.Create(ctx, mc)).NotTo(Succeed())
		})
	})

	Context("spec.memcached.threads validation", func() {
		It("should accept threads=1", func() {
			mc := validMemcached(uniqueName("thr1"))
			mc.Spec.Memcached = &memcachedv1alpha1.MemcachedConfig{
				Threads: 1,
			}
			Expect(k8sClient.Create(ctx, mc)).To(Succeed())
		})

		It("should accept threads=128", func() {
			mc := validMemcached(uniqueName("thr128"))
			mc.Spec.Memcached = &memcachedv1alpha1.MemcachedConfig{
				Threads: 128,
			}
			Expect(k8sClient.Create(ctx, mc)).To(Succeed())
		})

		It("should reject threads > 128", func() {
			mc := validMemcached(uniqueName("throver"))
			mc.Spec.Memcached = &memcachedv1alpha1.MemcachedConfig{
				Threads: 129,
			}
			Expect(k8sClient.Create(ctx, mc)).NotTo(Succeed())
		})
	})

	Context("spec.memcached.maxItemSize validation (pattern)", func() {
		It("should accept '1m'", func() {
			mc := validMemcached(uniqueName("item1m"))
			mc.Spec.Memcached = &memcachedv1alpha1.MemcachedConfig{
				MaxItemSize: "1m",
			}
			Expect(k8sClient.Create(ctx, mc)).To(Succeed())
		})

		It("should accept '512k'", func() {
			mc := validMemcached(uniqueName("item512k"))
			mc.Spec.Memcached = &memcachedv1alpha1.MemcachedConfig{
				MaxItemSize: "512k",
			}
			Expect(k8sClient.Create(ctx, mc)).To(Succeed())
		})

		It("should accept '2m'", func() {
			mc := validMemcached(uniqueName("item2m"))
			mc.Spec.Memcached = &memcachedv1alpha1.MemcachedConfig{
				MaxItemSize: "2m",
			}
			Expect(k8sClient.Create(ctx, mc)).To(Succeed())
		})

		It("should reject '1g' (invalid unit)", func() {
			mc := validMemcached(uniqueName("item1g"))
			mc.Spec.Memcached = &memcachedv1alpha1.MemcachedConfig{
				MaxItemSize: "1g",
			}
			Expect(k8sClient.Create(ctx, mc)).NotTo(Succeed())
		})

		It("should reject 'abc' (non-numeric)", func() {
			mc := validMemcached(uniqueName("itemabc"))
			mc.Spec.Memcached = &memcachedv1alpha1.MemcachedConfig{
				MaxItemSize: "abc",
			}
			Expect(k8sClient.Create(ctx, mc)).NotTo(Succeed())
		})
	})

	Context("spec.memcached.verbosity validation", func() {
		It("should accept verbosity=0", func() {
			mc := validMemcached(uniqueName("verb0"))
			mc.Spec.Memcached = &memcachedv1alpha1.MemcachedConfig{
				Verbosity: 0,
			}
			Expect(k8sClient.Create(ctx, mc)).To(Succeed())
		})

		It("should accept verbosity=2", func() {
			mc := validMemcached(uniqueName("verb2"))
			mc.Spec.Memcached = &memcachedv1alpha1.MemcachedConfig{
				Verbosity: 2,
			}
			Expect(k8sClient.Create(ctx, mc)).To(Succeed())
		})

		It("should reject verbosity=3", func() {
			mc := validMemcached(uniqueName("verb3"))
			mc.Spec.Memcached = &memcachedv1alpha1.MemcachedConfig{
				Verbosity: 3,
			}
			Expect(k8sClient.Create(ctx, mc)).NotTo(Succeed())
		})
	})

	Context("spec.memcached.extraArgs", func() {
		It("should accept a list of extra args", func() {
			mc := validMemcached(uniqueName("extra"))
			mc.Spec.Memcached = &memcachedv1alpha1.MemcachedConfig{
				ExtraArgs: []string{"-o", "modern", "-B", "binary"},
			}
			Expect(k8sClient.Create(ctx, mc)).To(Succeed())
		})
	})

	Context("spec.image", func() {
		It("should accept a custom image", func() {
			mc := validMemcached(uniqueName("img"))
			mc.Spec.Image = strPtr("memcached:1.6.33-alpine")
			Expect(k8sClient.Create(ctx, mc)).To(Succeed())
		})
	})

	Context("spec.resources", func() {
		It("should accept valid resource requirements", func() {
			mc := validMemcached(uniqueName("res"))
			mc.Spec.Resources = &corev1.ResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("100m"),
					corev1.ResourceMemory: resource.MustParse("128Mi"),
				},
				Limits: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("500m"),
					corev1.ResourceMemory: resource.MustParse("256Mi"),
				},
			}
			Expect(k8sClient.Create(ctx, mc)).To(Succeed())
		})
	})

	Context("all MemcachedConfig fields together", func() {
		It("should accept all fields set to valid values", func() {
			mc := validMemcached(uniqueName("allcfg"))
			mc.Spec.Memcached = &memcachedv1alpha1.MemcachedConfig{
				MaxMemoryMB:    256,
				MaxConnections: 4096,
				Threads:        8,
				MaxItemSize:    "2m",
				Verbosity:      1,
				ExtraArgs:      []string{"-o", "modern"},
			}
			Expect(k8sClient.Create(ctx, mc)).To(Succeed())
		})
	})
})

// --- Task 4.2: HA, Monitoring, Security field validation (REQ-002, REQ-003, REQ-004) ---

var _ = Describe("CRD Validation: HighAvailability, Monitoring, and Security fields", func() {

	Context("spec.highAvailability.antiAffinityPreset (enum)", func() {
		It("should accept 'soft'", func() {
			mc := validMemcached(uniqueName("ha-soft"))
			soft := memcachedv1alpha1.AntiAffinityPresetSoft
			mc.Spec.HighAvailability = &memcachedv1alpha1.HighAvailabilitySpec{
				AntiAffinityPreset: &soft,
			}
			Expect(k8sClient.Create(ctx, mc)).To(Succeed())
		})

		It("should accept 'hard'", func() {
			mc := validMemcached(uniqueName("ha-hard"))
			hard := memcachedv1alpha1.AntiAffinityPresetHard
			mc.Spec.HighAvailability = &memcachedv1alpha1.HighAvailabilitySpec{
				AntiAffinityPreset: &hard,
			}
			Expect(k8sClient.Create(ctx, mc)).To(Succeed())
		})

		It("should reject invalid enum value", func() {
			mc := validMemcached(uniqueName("ha-invalid"))
			invalid := memcachedv1alpha1.AntiAffinityPreset("medium")
			mc.Spec.HighAvailability = &memcachedv1alpha1.HighAvailabilitySpec{
				AntiAffinityPreset: &invalid,
			}
			Expect(k8sClient.Create(ctx, mc)).NotTo(Succeed())
		})
	})

	Context("spec.highAvailability.podDisruptionBudget", func() {
		It("should accept PDB with integer minAvailable", func() {
			mc := validMemcached(uniqueName("pdb-int"))
			replicas := int32(3)
			mc.Spec.Replicas = &replicas
			mc.Spec.HighAvailability = &memcachedv1alpha1.HighAvailabilitySpec{
				PodDisruptionBudget: &memcachedv1alpha1.PDBSpec{
					Enabled:      true,
					MinAvailable: &intstr.IntOrString{Type: intstr.Int, IntVal: 2},
				},
			}
			Expect(k8sClient.Create(ctx, mc)).To(Succeed())
		})

		It("should accept PDB with percentage minAvailable", func() {
			mc := validMemcached(uniqueName("pdb-pct"))
			mc.Spec.HighAvailability = &memcachedv1alpha1.HighAvailabilitySpec{
				PodDisruptionBudget: &memcachedv1alpha1.PDBSpec{
					Enabled:      true,
					MinAvailable: &intstr.IntOrString{Type: intstr.String, StrVal: "50%"},
				},
			}
			Expect(k8sClient.Create(ctx, mc)).To(Succeed())
		})

		It("should accept PDB with maxUnavailable", func() {
			mc := validMemcached(uniqueName("pdb-maxu"))
			mc.Spec.HighAvailability = &memcachedv1alpha1.HighAvailabilitySpec{
				PodDisruptionBudget: &memcachedv1alpha1.PDBSpec{
					Enabled:        true,
					MaxUnavailable: &intstr.IntOrString{Type: intstr.String, StrVal: "25%"},
				},
			}
			Expect(k8sClient.Create(ctx, mc)).To(Succeed())
		})

		It("should accept PDB disabled", func() {
			mc := validMemcached(uniqueName("pdb-off"))
			mc.Spec.HighAvailability = &memcachedv1alpha1.HighAvailabilitySpec{
				PodDisruptionBudget: &memcachedv1alpha1.PDBSpec{
					Enabled: false,
				},
			}
			Expect(k8sClient.Create(ctx, mc)).To(Succeed())
		})
	})

	Context("spec.highAvailability.topologySpreadConstraints", func() {
		It("should accept valid topology spread constraints", func() {
			mc := validMemcached(uniqueName("tsc"))
			mc.Spec.HighAvailability = &memcachedv1alpha1.HighAvailabilitySpec{
				TopologySpreadConstraints: []corev1.TopologySpreadConstraint{
					{
						MaxSkew:           1,
						TopologyKey:       "topology.kubernetes.io/zone",
						WhenUnsatisfiable: corev1.DoNotSchedule,
					},
				},
			}
			Expect(k8sClient.Create(ctx, mc)).To(Succeed())
		})
	})

	Context("spec.highAvailability.gracefulShutdown", func() {
		It("should accept gracefulShutdown with valid preStopDelaySeconds", func() {
			// Minimum: 1
			mc := validMemcached(uniqueName("gs-min"))
			mc.Spec.HighAvailability = &memcachedv1alpha1.HighAvailabilitySpec{
				GracefulShutdown: &memcachedv1alpha1.GracefulShutdownSpec{
					Enabled:                       true,
					PreStopDelaySeconds:           1,
					TerminationGracePeriodSeconds: 30,
				},
			}
			Expect(k8sClient.Create(ctx, mc)).To(Succeed())

			// Maximum: 300
			mc2 := validMemcached(uniqueName("gs-max"))
			mc2.Spec.HighAvailability = &memcachedv1alpha1.HighAvailabilitySpec{
				GracefulShutdown: &memcachedv1alpha1.GracefulShutdownSpec{
					Enabled:                       true,
					PreStopDelaySeconds:           300,
					TerminationGracePeriodSeconds: 600,
				},
			}
			Expect(k8sClient.Create(ctx, mc2)).To(Succeed())
		})

		It("should reject gracefulShutdown with invalid preStopDelaySeconds", func() {
			// preStopDelaySeconds=0 should be rejected (below minimum of 1).
			// With omitempty on int32, 0 is treated as omitted and server applies default=10.
			// So we test 301 which exceeds max.
			mc := validMemcached(uniqueName("gs-over"))
			mc.Spec.HighAvailability = &memcachedv1alpha1.HighAvailabilitySpec{
				GracefulShutdown: &memcachedv1alpha1.GracefulShutdownSpec{
					Enabled:                       true,
					PreStopDelaySeconds:           301,
					TerminationGracePeriodSeconds: 600,
				},
			}
			Expect(k8sClient.Create(ctx, mc)).NotTo(Succeed())
		})

		It("should accept gracefulShutdown with valid terminationGracePeriodSeconds", func() {
			// Minimum: terminationGracePeriodSeconds must exceed preStopDelaySeconds.
			mc := validMemcached(uniqueName("gs-tgps-min"))
			mc.Spec.HighAvailability = &memcachedv1alpha1.HighAvailabilitySpec{
				GracefulShutdown: &memcachedv1alpha1.GracefulShutdownSpec{
					Enabled:                       true,
					PreStopDelaySeconds:           1,
					TerminationGracePeriodSeconds: 2,
				},
			}
			Expect(k8sClient.Create(ctx, mc)).To(Succeed())

			// Maximum: 600
			mc2 := validMemcached(uniqueName("gs-tgps-max"))
			mc2.Spec.HighAvailability = &memcachedv1alpha1.HighAvailabilitySpec{
				GracefulShutdown: &memcachedv1alpha1.GracefulShutdownSpec{
					Enabled:                       true,
					PreStopDelaySeconds:           300,
					TerminationGracePeriodSeconds: 600,
				},
			}
			Expect(k8sClient.Create(ctx, mc2)).To(Succeed())
		})

		It("should reject gracefulShutdown with invalid terminationGracePeriodSeconds", func() {
			// terminationGracePeriodSeconds=601 exceeds max of 600.
			mc := validMemcached(uniqueName("gs-tgps-over"))
			mc.Spec.HighAvailability = &memcachedv1alpha1.HighAvailabilitySpec{
				GracefulShutdown: &memcachedv1alpha1.GracefulShutdownSpec{
					Enabled:                       true,
					PreStopDelaySeconds:           10,
					TerminationGracePeriodSeconds: 601,
				},
			}
			Expect(k8sClient.Create(ctx, mc)).NotTo(Succeed())
		})
	})

	Context("spec.highAvailability with all sub-fields", func() {
		It("should accept a fully populated HA spec", func() {
			mc := validMemcached(uniqueName("ha-full"))
			replicas := int32(3)
			mc.Spec.Replicas = &replicas
			hard := memcachedv1alpha1.AntiAffinityPresetHard
			mc.Spec.HighAvailability = &memcachedv1alpha1.HighAvailabilitySpec{
				AntiAffinityPreset: &hard,
				TopologySpreadConstraints: []corev1.TopologySpreadConstraint{
					{
						MaxSkew:           1,
						TopologyKey:       "kubernetes.io/hostname",
						WhenUnsatisfiable: corev1.ScheduleAnyway,
					},
				},
				PodDisruptionBudget: &memcachedv1alpha1.PDBSpec{
					Enabled:      true,
					MinAvailable: &intstr.IntOrString{Type: intstr.Int, IntVal: 1},
				},
				GracefulShutdown: &memcachedv1alpha1.GracefulShutdownSpec{
					Enabled:                       true,
					PreStopDelaySeconds:           10,
					TerminationGracePeriodSeconds: 30,
				},
			}
			Expect(k8sClient.Create(ctx, mc)).To(Succeed())
		})
	})

	Context("spec.monitoring", func() {
		It("should accept monitoring disabled (default)", func() {
			mc := validMemcached(uniqueName("mon-off"))
			mc.Spec.Monitoring = &memcachedv1alpha1.MonitoringSpec{
				Enabled: false,
			}
			Expect(k8sClient.Create(ctx, mc)).To(Succeed())
		})

		It("should accept monitoring enabled with custom exporter image", func() {
			mc := validMemcached(uniqueName("mon-img"))
			mc.Spec.Monitoring = &memcachedv1alpha1.MonitoringSpec{
				Enabled:       true,
				ExporterImage: strPtr("prom/memcached-exporter:v0.15.4"),
			}
			Expect(k8sClient.Create(ctx, mc)).To(Succeed())
		})

		It("should accept monitoring with exporter resources", func() {
			mc := validMemcached(uniqueName("mon-res"))
			mc.Spec.Monitoring = &memcachedv1alpha1.MonitoringSpec{
				Enabled: true,
				ExporterResources: &corev1.ResourceRequirements{
					Requests: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("50m"),
						corev1.ResourceMemory: resource.MustParse("64Mi"),
					},
				},
			}
			Expect(k8sClient.Create(ctx, mc)).To(Succeed())
		})

		It("should accept monitoring with service monitor", func() {
			mc := validMemcached(uniqueName("mon-sm"))
			mc.Spec.Monitoring = &memcachedv1alpha1.MonitoringSpec{
				Enabled: true,
				ServiceMonitor: &memcachedv1alpha1.ServiceMonitorSpec{
					Interval:      "30s",
					ScrapeTimeout: "10s",
					AdditionalLabels: map[string]string{
						"team": "platform",
					},
				},
			}
			Expect(k8sClient.Create(ctx, mc)).To(Succeed())
		})

		It("should accept fully populated monitoring spec", func() {
			mc := validMemcached(uniqueName("mon-full"))
			mc.Spec.Monitoring = &memcachedv1alpha1.MonitoringSpec{
				Enabled:       true,
				ExporterImage: strPtr("custom/exporter:latest"),
				ExporterResources: &corev1.ResourceRequirements{
					Limits: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("200m"),
						corev1.ResourceMemory: resource.MustParse("128Mi"),
					},
				},
				ServiceMonitor: &memcachedv1alpha1.ServiceMonitorSpec{
					Interval:      "15s",
					ScrapeTimeout: "5s",
					AdditionalLabels: map[string]string{
						"prometheus": "default",
					},
				},
			}
			Expect(k8sClient.Create(ctx, mc)).To(Succeed())
		})
	})

	Context("spec.security.sasl", func() {
		It("should accept SASL enabled with credentialsSecretRef", func() {
			mc := validMemcached(uniqueName("sasl-on"))
			mc.Spec.Security = &memcachedv1alpha1.SecuritySpec{
				SASL: &memcachedv1alpha1.SASLSpec{
					Enabled: true,
					CredentialsSecretRef: corev1.LocalObjectReference{
						Name: "sasl-secret",
					},
				},
			}
			Expect(k8sClient.Create(ctx, mc)).To(Succeed())
		})

		It("should accept SASL disabled", func() {
			mc := validMemcached(uniqueName("sasl-off"))
			mc.Spec.Security = &memcachedv1alpha1.SecuritySpec{
				SASL: &memcachedv1alpha1.SASLSpec{
					Enabled: false,
				},
			}
			Expect(k8sClient.Create(ctx, mc)).To(Succeed())
		})
	})

	Context("spec.security.tls", func() {
		It("should accept TLS enabled with certificateSecretRef", func() {
			mc := validMemcached(uniqueName("tls-on"))
			mc.Spec.Security = &memcachedv1alpha1.SecuritySpec{
				TLS: &memcachedv1alpha1.TLSSpec{
					Enabled: true,
					CertificateSecretRef: corev1.LocalObjectReference{
						Name: "tls-cert",
					},
				},
			}
			Expect(k8sClient.Create(ctx, mc)).To(Succeed())
		})

		It("should accept TLS disabled", func() {
			mc := validMemcached(uniqueName("tls-off"))
			mc.Spec.Security = &memcachedv1alpha1.SecuritySpec{
				TLS: &memcachedv1alpha1.TLSSpec{
					Enabled: false,
				},
			}
			Expect(k8sClient.Create(ctx, mc)).To(Succeed())
		})

		It("should accept TLS with enableClientCert=true", func() {
			mc := validMemcached(uniqueName("tls-mtls"))
			mc.Spec.Security = &memcachedv1alpha1.SecuritySpec{
				TLS: &memcachedv1alpha1.TLSSpec{
					Enabled: true,
					CertificateSecretRef: corev1.LocalObjectReference{
						Name: "tls-cert",
					},
					EnableClientCert: true,
				},
			}
			Expect(k8sClient.Create(ctx, mc)).To(Succeed())

			// Verify round-trip: enableClientCert persists correctly.
			fetched := &memcachedv1alpha1.Memcached{}
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(mc), fetched)).To(Succeed())
			Expect(fetched.Spec.Security.TLS.EnableClientCert).To(BeTrue())
			Expect(fetched.Spec.Security.TLS.Enabled).To(BeTrue())
			Expect(fetched.Spec.Security.TLS.CertificateSecretRef.Name).To(Equal("tls-cert"))
		})

		It("should default enableClientCert to false when not specified", func() {
			mc := validMemcached(uniqueName("tls-nomtls"))
			mc.Spec.Security = &memcachedv1alpha1.SecuritySpec{
				TLS: &memcachedv1alpha1.TLSSpec{
					Enabled: true,
					CertificateSecretRef: corev1.LocalObjectReference{
						Name: "tls-cert",
					},
				},
			}
			Expect(k8sClient.Create(ctx, mc)).To(Succeed())

			fetched := &memcachedv1alpha1.Memcached{}
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(mc), fetched)).To(Succeed())
			Expect(fetched.Spec.Security.TLS.EnableClientCert).To(BeFalse())
		})
	})

	Context("spec.security.networkPolicy", func() {
		It("should accept networkPolicy enabled", func() {
			mc := validMemcached(uniqueName("np-on"))
			mc.Spec.Security = &memcachedv1alpha1.SecuritySpec{
				NetworkPolicy: &memcachedv1alpha1.NetworkPolicySpec{
					Enabled: true,
				},
			}
			Expect(k8sClient.Create(ctx, mc)).To(Succeed())
		})

		It("should accept networkPolicy disabled", func() {
			mc := validMemcached(uniqueName("np-off"))
			mc.Spec.Security = &memcachedv1alpha1.SecuritySpec{
				NetworkPolicy: &memcachedv1alpha1.NetworkPolicySpec{
					Enabled: false,
				},
			}
			Expect(k8sClient.Create(ctx, mc)).To(Succeed())
		})

		It("should accept networkPolicy with allowedSources containing namespaceSelector", func() {
			mc := validMemcached(uniqueName("np-ns"))
			mc.Spec.Security = &memcachedv1alpha1.SecuritySpec{
				NetworkPolicy: &memcachedv1alpha1.NetworkPolicySpec{
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

			// Verify round-trip: allowedSources persists correctly.
			fetched := &memcachedv1alpha1.Memcached{}
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(mc), fetched)).To(Succeed())
			Expect(fetched.Spec.Security.NetworkPolicy).NotTo(BeNil())
			Expect(fetched.Spec.Security.NetworkPolicy.Enabled).To(BeTrue())
			Expect(fetched.Spec.Security.NetworkPolicy.AllowedSources).To(HaveLen(1))
			Expect(fetched.Spec.Security.NetworkPolicy.AllowedSources[0].NamespaceSelector).NotTo(BeNil())
			Expect(fetched.Spec.Security.NetworkPolicy.AllowedSources[0].NamespaceSelector.MatchLabels).To(
				HaveKeyWithValue("env", "production"),
			)
		})

		It("should accept networkPolicy with allowedSources containing podSelector", func() {
			mc := validMemcached(uniqueName("np-pod"))
			mc.Spec.Security = &memcachedv1alpha1.SecuritySpec{
				NetworkPolicy: &memcachedv1alpha1.NetworkPolicySpec{
					Enabled: true,
					AllowedSources: []networkingv1.NetworkPolicyPeer{
						{
							PodSelector: &metav1.LabelSelector{
								MatchLabels: map[string]string{"app": "keystone"},
							},
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, mc)).To(Succeed())

			// Verify round-trip.
			fetched := &memcachedv1alpha1.Memcached{}
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(mc), fetched)).To(Succeed())
			Expect(fetched.Spec.Security.NetworkPolicy.AllowedSources).To(HaveLen(1))
			Expect(fetched.Spec.Security.NetworkPolicy.AllowedSources[0].PodSelector).NotTo(BeNil())
			Expect(fetched.Spec.Security.NetworkPolicy.AllowedSources[0].PodSelector.MatchLabels).To(
				HaveKeyWithValue("app", "keystone"),
			)
		})

		It("should accept networkPolicy with multiple allowedSources", func() {
			mc := validMemcached(uniqueName("np-multi"))
			mc.Spec.Security = &memcachedv1alpha1.SecuritySpec{
				NetworkPolicy: &memcachedv1alpha1.NetworkPolicySpec{
					Enabled: true,
					AllowedSources: []networkingv1.NetworkPolicyPeer{
						{
							NamespaceSelector: &metav1.LabelSelector{
								MatchLabels: map[string]string{"env": "prod"},
							},
						},
						{
							PodSelector: &metav1.LabelSelector{
								MatchLabels: map[string]string{"role": "api"},
							},
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, mc)).To(Succeed())

			fetched := &memcachedv1alpha1.Memcached{}
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(mc), fetched)).To(Succeed())
			Expect(fetched.Spec.Security.NetworkPolicy.AllowedSources).To(HaveLen(2))
		})
	})

	Context("spec.security.podSecurityContext and containerSecurityContext", func() {
		It("should accept pod and container security contexts", func() {
			mc := validMemcached(uniqueName("sec-ctx"))
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

			fetched := &memcachedv1alpha1.Memcached{}
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(mc), fetched)).To(Succeed())
			Expect(*fetched.Spec.Security.PodSecurityContext.RunAsNonRoot).To(BeTrue())
			Expect(*fetched.Spec.Security.ContainerSecurityContext.RunAsUser).To(Equal(int64(1000)))
		})
	})

	Context("spec.security with all sub-fields", func() {
		It("should accept a fully populated security spec", func() {
			mc := validMemcached(uniqueName("sec-full"))
			runAsNonRoot := true
			mc.Spec.Security = &memcachedv1alpha1.SecuritySpec{
				PodSecurityContext: &corev1.PodSecurityContext{
					RunAsNonRoot: &runAsNonRoot,
				},
				SASL: &memcachedv1alpha1.SASLSpec{
					Enabled: true,
					CredentialsSecretRef: corev1.LocalObjectReference{
						Name: "sasl-creds",
					},
				},
				TLS: &memcachedv1alpha1.TLSSpec{
					Enabled: true,
					CertificateSecretRef: corev1.LocalObjectReference{
						Name: "tls-cert",
					},
				},
				NetworkPolicy: &memcachedv1alpha1.NetworkPolicySpec{
					Enabled: true,
					AllowedSources: []networkingv1.NetworkPolicyPeer{
						{
							NamespaceSelector: &metav1.LabelSelector{
								MatchLabels: map[string]string{"env": "prod"},
							},
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, mc)).To(Succeed())
		})
	})
})

// --- Task 4.3: MemcachedStatus fields and printer columns (REQ-006, REQ-009) ---

var _ = Describe("CRD Validation: MemcachedStatus and printer columns", func() {

	Context("status subresource", func() {
		It("should allow updating status independently of spec", func() {
			mc := validMemcached(uniqueName("status-update"))
			mc.Spec.Replicas = int32Ptr(3)
			Expect(k8sClient.Create(ctx, mc)).To(Succeed())

			// Fetch the created resource to get the resource version.
			fetched := &memcachedv1alpha1.Memcached{}
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(mc), fetched)).To(Succeed())

			// Update status fields.
			fetched.Status.ReadyReplicas = 2
			fetched.Status.ObservedGeneration = fetched.Generation
			Expect(k8sClient.Status().Update(ctx, fetched)).To(Succeed())

			// Verify status was persisted.
			updated := &memcachedv1alpha1.Memcached{}
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(mc), updated)).To(Succeed())
			Expect(updated.Status.ReadyReplicas).To(Equal(int32(2)))
			Expect(updated.Status.ObservedGeneration).To(Equal(fetched.Generation))
		})

		It("should not change spec.generation on status-only update", func() {
			mc := validMemcached(uniqueName("status-gen"))
			Expect(k8sClient.Create(ctx, mc)).To(Succeed())

			fetched := &memcachedv1alpha1.Memcached{}
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(mc), fetched)).To(Succeed())
			genBefore := fetched.Generation

			fetched.Status.ReadyReplicas = 1
			Expect(k8sClient.Status().Update(ctx, fetched)).To(Succeed())

			updated := &memcachedv1alpha1.Memcached{}
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(mc), updated)).To(Succeed())
			Expect(updated.Generation).To(Equal(genBefore))
		})
	})

	Context("status.conditions", func() {
		It("should accept conditions with standard fields", func() {
			mc := validMemcached(uniqueName("status-cond"))
			Expect(k8sClient.Create(ctx, mc)).To(Succeed())

			fetched := &memcachedv1alpha1.Memcached{}
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(mc), fetched)).To(Succeed())

			now := metav1.Now()
			fetched.Status.Conditions = []metav1.Condition{
				{
					Type:               "Available",
					Status:             metav1.ConditionTrue,
					ObservedGeneration: fetched.Generation,
					LastTransitionTime: now,
					Reason:             "AllReplicasReady",
					Message:            "All pods are ready",
				},
			}
			Expect(k8sClient.Status().Update(ctx, fetched)).To(Succeed())

			updated := &memcachedv1alpha1.Memcached{}
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(mc), updated)).To(Succeed())
			Expect(updated.Status.Conditions).To(HaveLen(1))
			Expect(updated.Status.Conditions[0].Type).To(Equal("Available"))
			Expect(updated.Status.Conditions[0].Status).To(Equal(metav1.ConditionTrue))
			Expect(updated.Status.Conditions[0].Reason).To(Equal("AllReplicasReady"))
		})

		It("should support multiple conditions", func() {
			mc := validMemcached(uniqueName("status-multi"))
			Expect(k8sClient.Create(ctx, mc)).To(Succeed())

			fetched := &memcachedv1alpha1.Memcached{}
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(mc), fetched)).To(Succeed())

			now := metav1.Now()
			fetched.Status.Conditions = []metav1.Condition{
				{
					Type:               "Available",
					Status:             metav1.ConditionTrue,
					ObservedGeneration: fetched.Generation,
					LastTransitionTime: now,
					Reason:             "Ready",
					Message:            "Available",
				},
				{
					Type:               "Progressing",
					Status:             metav1.ConditionFalse,
					ObservedGeneration: fetched.Generation,
					LastTransitionTime: now,
					Reason:             "Complete",
					Message:            "Rollout complete",
				},
			}
			Expect(k8sClient.Status().Update(ctx, fetched)).To(Succeed())

			updated := &memcachedv1alpha1.Memcached{}
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(mc), updated)).To(Succeed())
			Expect(updated.Status.Conditions).To(HaveLen(2))
		})
	})

	Context("status.readyReplicas", func() {
		It("should persist readyReplicas=0", func() {
			mc := validMemcached(uniqueName("ready0"))
			Expect(k8sClient.Create(ctx, mc)).To(Succeed())

			fetched := &memcachedv1alpha1.Memcached{}
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(mc), fetched)).To(Succeed())

			fetched.Status.ReadyReplicas = 0
			Expect(k8sClient.Status().Update(ctx, fetched)).To(Succeed())

			updated := &memcachedv1alpha1.Memcached{}
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(mc), updated)).To(Succeed())
			Expect(updated.Status.ReadyReplicas).To(Equal(int32(0)))
		})

		It("should persist a positive readyReplicas count", func() {
			mc := validMemcached(uniqueName("ready5"))
			mc.Spec.Replicas = int32Ptr(5)
			Expect(k8sClient.Create(ctx, mc)).To(Succeed())

			fetched := &memcachedv1alpha1.Memcached{}
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(mc), fetched)).To(Succeed())

			fetched.Status.ReadyReplicas = 5
			Expect(k8sClient.Status().Update(ctx, fetched)).To(Succeed())

			updated := &memcachedv1alpha1.Memcached{}
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(mc), updated)).To(Succeed())
			Expect(updated.Status.ReadyReplicas).To(Equal(int32(5)))
		})
	})

	Context("status.observedGeneration", func() {
		It("should track observedGeneration correctly across spec updates", func() {
			mc := validMemcached(uniqueName("obsgen"))
			mc.Spec.Replicas = int32Ptr(1)
			Expect(k8sClient.Create(ctx, mc)).To(Succeed())

			fetched := &memcachedv1alpha1.Memcached{}
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(mc), fetched)).To(Succeed())
			gen1 := fetched.Generation

			// Update spec to increment generation.
			fetched.Spec.Replicas = int32Ptr(3)
			Expect(k8sClient.Update(ctx, fetched)).To(Succeed())

			refetched := &memcachedv1alpha1.Memcached{}
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(mc), refetched)).To(Succeed())
			gen2 := refetched.Generation
			Expect(gen2).To(BeNumerically(">", gen1))

			// Set observedGeneration to the new generation.
			refetched.Status.ObservedGeneration = gen2
			Expect(k8sClient.Status().Update(ctx, refetched)).To(Succeed())

			updated := &memcachedv1alpha1.Memcached{}
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(mc), updated)).To(Succeed())
			Expect(updated.Status.ObservedGeneration).To(Equal(gen2))
		})
	})

	Context("printer columns (REQ-009)", func() {
		It("should have Replicas, Ready, and Age columns defined in the CRD", func() {
			mc := validMemcached(uniqueName("printer"))
			mc.Spec.Replicas = int32Ptr(3)
			Expect(k8sClient.Create(ctx, mc)).To(Succeed())

			fetched := &memcachedv1alpha1.Memcached{}
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(mc), fetched)).To(Succeed())

			// Verify the spec.replicas field (Replicas column source) is accessible.
			Expect(*fetched.Spec.Replicas).To(Equal(int32(3)))

			// Update status.readyReplicas (Ready column source).
			fetched.Status.ReadyReplicas = 2
			Expect(k8sClient.Status().Update(ctx, fetched)).To(Succeed())

			updated := &memcachedv1alpha1.Memcached{}
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(mc), updated)).To(Succeed())
			Expect(updated.Status.ReadyReplicas).To(Equal(int32(2)))

			// Age column is based on metadata.creationTimestamp, which is auto-set.
			Expect(updated.CreationTimestamp.IsZero()).To(BeFalse())
		})
	})

	Context("full resource with all fields", func() {
		It("should accept a fully populated Memcached resource", func() {
			mc := validMemcached(uniqueName("full"))
			hard := memcachedv1alpha1.AntiAffinityPresetHard
			runAsNonRoot := true
			mc.Spec = memcachedv1alpha1.MemcachedSpec{
				Replicas: int32Ptr(3),
				Image:    strPtr("memcached:1.6.33"),
				Resources: &corev1.ResourceRequirements{
					Requests: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("100m"),
						corev1.ResourceMemory: resource.MustParse("128Mi"),
					},
					Limits: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("500m"),
						corev1.ResourceMemory: resource.MustParse("256Mi"),
					},
				},
				Memcached: &memcachedv1alpha1.MemcachedConfig{
					MaxMemoryMB:    128,
					MaxConnections: 2048,
					Threads:        4,
					MaxItemSize:    "2m",
					Verbosity:      1,
					ExtraArgs:      []string{"-o", "modern"},
				},
				HighAvailability: &memcachedv1alpha1.HighAvailabilitySpec{
					AntiAffinityPreset: &hard,
					PodDisruptionBudget: &memcachedv1alpha1.PDBSpec{
						Enabled:      true,
						MinAvailable: &intstr.IntOrString{Type: intstr.Int, IntVal: 1},
					},
				},
				Monitoring: &memcachedv1alpha1.MonitoringSpec{
					Enabled:       true,
					ExporterImage: strPtr("prom/memcached-exporter:v0.15.4"),
					ServiceMonitor: &memcachedv1alpha1.ServiceMonitorSpec{
						Interval: "30s",
					},
				},
				Security: &memcachedv1alpha1.SecuritySpec{
					PodSecurityContext: &corev1.PodSecurityContext{
						RunAsNonRoot: &runAsNonRoot,
					},
					SASL: &memcachedv1alpha1.SASLSpec{
						Enabled: true,
						CredentialsSecretRef: corev1.LocalObjectReference{
							Name: "sasl-creds",
						},
					},
					TLS: &memcachedv1alpha1.TLSSpec{
						Enabled: true,
						CertificateSecretRef: corev1.LocalObjectReference{
							Name: "tls-cert",
						},
					},
					NetworkPolicy: &memcachedv1alpha1.NetworkPolicySpec{
						Enabled: true,
						AllowedSources: []networkingv1.NetworkPolicyPeer{
							{
								NamespaceSelector: &metav1.LabelSelector{
									MatchLabels: map[string]string{"env": "prod"},
								},
							},
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, mc)).To(Succeed())

			// Verify round-trip.
			fetched := &memcachedv1alpha1.Memcached{}
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(mc), fetched)).To(Succeed())
			Expect(*fetched.Spec.Replicas).To(Equal(int32(3)))
			Expect(*fetched.Spec.Image).To(Equal("memcached:1.6.33"))
			Expect(fetched.Spec.Memcached.MaxMemoryMB).To(Equal(int32(128)))
			Expect(*fetched.Spec.HighAvailability.AntiAffinityPreset).To(Equal(memcachedv1alpha1.AntiAffinityPresetHard))
			Expect(fetched.Spec.Monitoring.Enabled).To(BeTrue())
			Expect(fetched.Spec.Security.SASL.Enabled).To(BeTrue())
			Expect(fetched.Spec.Security.TLS.Enabled).To(BeTrue())
			Expect(fetched.Spec.Security.NetworkPolicy).NotTo(BeNil())
			Expect(fetched.Spec.Security.NetworkPolicy.Enabled).To(BeTrue())
			Expect(fetched.Spec.Security.NetworkPolicy.AllowedSources).To(HaveLen(1))
		})
	})
})

// --- Task 4.1 (blocking): Server-applied defaults verification ---

var _ = Describe("CRD Defaults: Server-applied defaults verification", func() {

	Context("MemcachedConfig defaults", func() {
		It("should apply defaults when creating CR with empty memcached block", func() {
			mc := validMemcached(uniqueName("defaults"))
			mc.Spec.Memcached = &memcachedv1alpha1.MemcachedConfig{}
			Expect(k8sClient.Create(ctx, mc)).To(Succeed())

			fetched := &memcachedv1alpha1.Memcached{}
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(mc), fetched)).To(Succeed())

			Expect(fetched.Spec.Memcached).NotTo(BeNil())
			Expect(fetched.Spec.Memcached.MaxMemoryMB).To(Equal(int32(64)))
			Expect(fetched.Spec.Memcached.MaxConnections).To(Equal(int32(1024)))
			Expect(fetched.Spec.Memcached.Threads).To(Equal(int32(4)))
			Expect(fetched.Spec.Memcached.MaxItemSize).To(Equal("1m"))
			Expect(fetched.Spec.Memcached.Verbosity).To(Equal(int32(0)))
		})
	})

	Context("MemcachedSpec top-level defaults", func() {
		It("should apply image and replicas defaults on minimal CR", func() {
			mc := validMemcached(uniqueName("spec-defaults"))
			Expect(k8sClient.Create(ctx, mc)).To(Succeed())

			fetched := &memcachedv1alpha1.Memcached{}
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(mc), fetched)).To(Succeed())

			Expect(fetched.Spec.Replicas).NotTo(BeNil())
			Expect(*fetched.Spec.Replicas).To(Equal(int32(1)))
			Expect(fetched.Spec.Image).NotTo(BeNil())
			Expect(*fetched.Spec.Image).To(Equal("memcached:1.6"))
		})
	})

	Context("ServiceMonitorSpec defaults", func() {
		It("should apply interval and scrapeTimeout defaults", func() {
			mc := validMemcached(uniqueName("sm-defaults"))
			mc.Spec.Monitoring = &memcachedv1alpha1.MonitoringSpec{
				Enabled:        true,
				ServiceMonitor: &memcachedv1alpha1.ServiceMonitorSpec{},
			}
			Expect(k8sClient.Create(ctx, mc)).To(Succeed())

			fetched := &memcachedv1alpha1.Memcached{}
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(mc), fetched)).To(Succeed())

			Expect(fetched.Spec.Monitoring.ServiceMonitor).NotTo(BeNil())
			Expect(fetched.Spec.Monitoring.ServiceMonitor.Interval).To(Equal("30s"))
			Expect(fetched.Spec.Monitoring.ServiceMonitor.ScrapeTimeout).To(Equal("10s"))
		})
	})
})
