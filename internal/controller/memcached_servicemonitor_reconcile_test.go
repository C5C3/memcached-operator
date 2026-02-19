package controller_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	monitoringv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/util/intstr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	memcachedv1alpha1 "github.com/c5c3/memcached-operator/api/v1alpha1"
)

// fetchServiceMonitor retrieves the ServiceMonitor with the same name/namespace as the Memcached CR.
func fetchServiceMonitor(mc *memcachedv1alpha1.Memcached) *monitoringv1.ServiceMonitor {
	sm := &monitoringv1.ServiceMonitor{}
	ExpectWithOffset(1, k8sClient.Get(ctx, client.ObjectKeyFromObject(mc), sm)).To(Succeed())
	return sm
}

// --- Task 3.1: ServiceMonitor creation with defaults and custom configuration (REQ-001, REQ-002, REQ-003) ---

var _ = Describe("ServiceMonitor Reconciliation", func() {

	Context("ServiceMonitor creation with defaults (REQ-001, REQ-002, REQ-003)", func() {
		var mc *memcachedv1alpha1.Memcached

		BeforeEach(func() {
			mc = validMemcached(uniqueName("sm-defaults"))
			mc.Spec.Monitoring = &memcachedv1alpha1.MonitoringSpec{
				Enabled:        true,
				ServiceMonitor: &memcachedv1alpha1.ServiceMonitorSpec{},
			}
			Expect(k8sClient.Create(ctx, mc)).To(Succeed())
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(mc), mc)).To(Succeed())
			result, err := reconcileOnce(mc)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal(ctrl.Result{}))
		})

		It("should create ServiceMonitor with default interval and scrapeTimeout", func() {
			sm := fetchServiceMonitor(mc)
			Expect(sm.Spec.Endpoints).To(HaveLen(1))
			Expect(sm.Spec.Endpoints[0].Port).To(Equal("metrics"))
			Expect(sm.Spec.Endpoints[0].Interval).To(Equal(monitoringv1.Duration("30s")))
			Expect(sm.Spec.Endpoints[0].ScrapeTimeout).To(Equal(monitoringv1.Duration("10s")))
		})

		It("should set standard labels on metadata", func() {
			sm := fetchServiceMonitor(mc)
			Expect(sm.Labels).To(HaveKeyWithValue("app.kubernetes.io/name", "memcached"))
			Expect(sm.Labels).To(HaveKeyWithValue("app.kubernetes.io/instance", mc.Name))
			Expect(sm.Labels).To(HaveKeyWithValue("app.kubernetes.io/managed-by", "memcached-operator"))
		})

		It("should set selector matching the CR instance", func() {
			sm := fetchServiceMonitor(mc)
			Expect(sm.Spec.Selector.MatchLabels).To(HaveKeyWithValue("app.kubernetes.io/name", "memcached"))
			Expect(sm.Spec.Selector.MatchLabels).To(HaveKeyWithValue("app.kubernetes.io/instance", mc.Name))
			Expect(sm.Spec.Selector.MatchLabels).To(HaveKeyWithValue("app.kubernetes.io/managed-by", "memcached-operator"))
		})

		It("should set namespace selector to the CR namespace", func() {
			sm := fetchServiceMonitor(mc)
			Expect(sm.Spec.NamespaceSelector.MatchNames).To(ConsistOf(mc.Namespace))
		})

		It("should set owner reference", func() {
			sm := fetchServiceMonitor(mc)
			Expect(sm.OwnerReferences).To(HaveLen(1))
			ownerRef := sm.OwnerReferences[0]
			Expect(ownerRef.APIVersion).To(Equal("memcached.c5c3.io/v1alpha1"))
			Expect(ownerRef.Kind).To(Equal("Memcached"))
			Expect(ownerRef.Name).To(Equal(mc.Name))
			Expect(ownerRef.UID).To(Equal(mc.UID))
			Expect(*ownerRef.Controller).To(BeTrue())
			Expect(*ownerRef.BlockOwnerDeletion).To(BeTrue())
		})
	})

	Context("ServiceMonitor with custom interval", func() {
		It("should use custom interval", func() {
			mc := validMemcached(uniqueName("sm-custint"))
			mc.Spec.Monitoring = &memcachedv1alpha1.MonitoringSpec{
				Enabled: true,
				ServiceMonitor: &memcachedv1alpha1.ServiceMonitorSpec{
					Interval: "60s",
				},
			}
			Expect(k8sClient.Create(ctx, mc)).To(Succeed())

			_, err := reconcileOnce(mc)
			Expect(err).NotTo(HaveOccurred())

			sm := fetchServiceMonitor(mc)
			Expect(sm.Spec.Endpoints[0].Interval).To(Equal(monitoringv1.Duration("60s")))
			Expect(sm.Spec.Endpoints[0].ScrapeTimeout).To(Equal(monitoringv1.Duration("10s")))
		})
	})

	Context("ServiceMonitor with custom scrapeTimeout", func() {
		It("should use custom scrapeTimeout", func() {
			mc := validMemcached(uniqueName("sm-custto"))
			mc.Spec.Monitoring = &memcachedv1alpha1.MonitoringSpec{
				Enabled: true,
				ServiceMonitor: &memcachedv1alpha1.ServiceMonitorSpec{
					ScrapeTimeout: "20s",
				},
			}
			Expect(k8sClient.Create(ctx, mc)).To(Succeed())

			_, err := reconcileOnce(mc)
			Expect(err).NotTo(HaveOccurred())

			sm := fetchServiceMonitor(mc)
			Expect(sm.Spec.Endpoints[0].Interval).To(Equal(monitoringv1.Duration("30s")))
			Expect(sm.Spec.Endpoints[0].ScrapeTimeout).To(Equal(monitoringv1.Duration("20s")))
		})
	})

	Context("ServiceMonitor with custom interval and scrapeTimeout", func() {
		It("should use both custom values", func() {
			mc := validMemcached(uniqueName("sm-custboth"))
			mc.Spec.Monitoring = &memcachedv1alpha1.MonitoringSpec{
				Enabled: true,
				ServiceMonitor: &memcachedv1alpha1.ServiceMonitorSpec{
					Interval:      "15s",
					ScrapeTimeout: "5s",
				},
			}
			Expect(k8sClient.Create(ctx, mc)).To(Succeed())

			_, err := reconcileOnce(mc)
			Expect(err).NotTo(HaveOccurred())

			sm := fetchServiceMonitor(mc)
			Expect(sm.Spec.Endpoints[0].Interval).To(Equal(monitoringv1.Duration("15s")))
			Expect(sm.Spec.Endpoints[0].ScrapeTimeout).To(Equal(monitoringv1.Duration("5s")))
		})
	})

	Context("ServiceMonitor with additional labels", func() {
		It("should include additional labels on metadata but not on selector", func() {
			mc := validMemcached(uniqueName("sm-addlbl"))
			mc.Spec.Monitoring = &memcachedv1alpha1.MonitoringSpec{
				Enabled: true,
				ServiceMonitor: &memcachedv1alpha1.ServiceMonitorSpec{
					AdditionalLabels: map[string]string{
						"release": "prometheus",
						"team":    "platform",
					},
				},
			}
			Expect(k8sClient.Create(ctx, mc)).To(Succeed())

			_, err := reconcileOnce(mc)
			Expect(err).NotTo(HaveOccurred())

			sm := fetchServiceMonitor(mc)
			// Additional labels on metadata.
			Expect(sm.Labels).To(HaveKeyWithValue("release", "prometheus"))
			Expect(sm.Labels).To(HaveKeyWithValue("team", "platform"))
			// Standard labels still present.
			Expect(sm.Labels).To(HaveKeyWithValue("app.kubernetes.io/name", "memcached"))

			// Selector should NOT have additional labels.
			Expect(sm.Spec.Selector.MatchLabels).NotTo(HaveKey("release"))
			Expect(sm.Spec.Selector.MatchLabels).NotTo(HaveKey("team"))
		})

		It("should not allow additionalLabels to override standard labels", func() {
			mc := validMemcached(uniqueName("sm-lbl-ovr"))
			mc.Spec.Monitoring = &memcachedv1alpha1.MonitoringSpec{
				Enabled: true,
				ServiceMonitor: &memcachedv1alpha1.ServiceMonitorSpec{
					AdditionalLabels: map[string]string{
						"app.kubernetes.io/name": "override",
						"release":                "prometheus",
					},
				},
			}
			Expect(k8sClient.Create(ctx, mc)).To(Succeed())

			_, err := reconcileOnce(mc)
			Expect(err).NotTo(HaveOccurred())

			sm := fetchServiceMonitor(mc)
			// Standard labels must take precedence over additionalLabels.
			Expect(sm.Labels).To(HaveKeyWithValue("app.kubernetes.io/name", "memcached"))
			Expect(sm.Labels).To(HaveKeyWithValue("app.kubernetes.io/instance", mc.Name))
			Expect(sm.Labels).To(HaveKeyWithValue("app.kubernetes.io/managed-by", "memcached-operator"))
			// Non-conflicting additional label should still be present.
			Expect(sm.Labels).To(HaveKeyWithValue("release", "prometheus"))
		})
	})

	Context("ServiceMonitor instance-scoped selector", func() {
		It("should scope selector to the specific CR instance", func() {
			mcA := validMemcached(uniqueName("sm-inst-a"))
			mcA.Spec.Monitoring = &memcachedv1alpha1.MonitoringSpec{
				Enabled:        true,
				ServiceMonitor: &memcachedv1alpha1.ServiceMonitorSpec{},
			}
			Expect(k8sClient.Create(ctx, mcA)).To(Succeed())

			mcB := validMemcached(uniqueName("sm-inst-b"))
			mcB.Spec.Monitoring = &memcachedv1alpha1.MonitoringSpec{
				Enabled:        true,
				ServiceMonitor: &memcachedv1alpha1.ServiceMonitorSpec{},
			}
			Expect(k8sClient.Create(ctx, mcB)).To(Succeed())

			_, err := reconcileOnce(mcA)
			Expect(err).NotTo(HaveOccurred())
			_, err = reconcileOnce(mcB)
			Expect(err).NotTo(HaveOccurred())

			smA := fetchServiceMonitor(mcA)
			smB := fetchServiceMonitor(mcB)

			Expect(smA.Spec.Selector.MatchLabels["app.kubernetes.io/instance"]).To(Equal(mcA.Name))
			Expect(smB.Spec.Selector.MatchLabels["app.kubernetes.io/instance"]).To(Equal(mcB.Name))
		})
	})

	// --- Task 3.2: ServiceMonitor skip logic (REQ-005) ---

	Context("No ServiceMonitor when monitoring is disabled (REQ-005)", func() {
		It("should not create a ServiceMonitor when monitoring is disabled", func() {
			mc := validMemcached(uniqueName("sm-disabled"))
			Expect(k8sClient.Create(ctx, mc)).To(Succeed())

			_, err := reconcileOnce(mc)
			Expect(err).NotTo(HaveOccurred())

			sm := &monitoringv1.ServiceMonitor{}
			err = k8sClient.Get(ctx, client.ObjectKeyFromObject(mc), sm)
			Expect(apierrors.IsNotFound(err)).To(BeTrue())
		})

		It("should not create a ServiceMonitor when monitoring is enabled but serviceMonitor is nil", func() {
			mc := validMemcached(uniqueName("sm-nil-sm"))
			mc.Spec.Monitoring = &memcachedv1alpha1.MonitoringSpec{
				Enabled: true,
			}
			Expect(k8sClient.Create(ctx, mc)).To(Succeed())

			_, err := reconcileOnce(mc)
			Expect(err).NotTo(HaveOccurred())

			sm := &monitoringv1.ServiceMonitor{}
			err = k8sClient.Get(ctx, client.ObjectKeyFromObject(mc), sm)
			Expect(apierrors.IsNotFound(err)).To(BeTrue())
		})

		It("should not create a ServiceMonitor when monitoring.enabled is false with serviceMonitor set", func() {
			mc := validMemcached(uniqueName("sm-false"))
			mc.Spec.Monitoring = &memcachedv1alpha1.MonitoringSpec{
				Enabled: false,
				ServiceMonitor: &memcachedv1alpha1.ServiceMonitorSpec{
					Interval: "30s",
				},
			}
			Expect(k8sClient.Create(ctx, mc)).To(Succeed())

			_, err := reconcileOnce(mc)
			Expect(err).NotTo(HaveOccurred())

			sm := &monitoringv1.ServiceMonitor{}
			err = k8sClient.Get(ctx, client.ObjectKeyFromObject(mc), sm)
			Expect(apierrors.IsNotFound(err)).To(BeTrue())
		})
	})

	// --- Task 3.3: ServiceMonitor idempotency and drift correction (REQ-006) ---

	Context("Idempotent ServiceMonitor reconciliation (REQ-006)", func() {
		It("should not change ServiceMonitor resource version on second reconcile", func() {
			mc := validMemcached(uniqueName("sm-idemp"))
			mc.Spec.Monitoring = &memcachedv1alpha1.MonitoringSpec{
				Enabled:        true,
				ServiceMonitor: &memcachedv1alpha1.ServiceMonitorSpec{},
			}
			Expect(k8sClient.Create(ctx, mc)).To(Succeed())

			_, err := reconcileOnce(mc)
			Expect(err).NotTo(HaveOccurred())

			sm1 := fetchServiceMonitor(mc)
			rv1 := sm1.ResourceVersion

			// Reconcile again without changes.
			_, err = reconcileOnce(mc)
			Expect(err).NotTo(HaveOccurred())

			sm2 := fetchServiceMonitor(mc)
			Expect(sm2.ResourceVersion).To(Equal(rv1))
		})
	})

	Context("ServiceMonitor drift correction (REQ-006)", func() {
		It("should restore ServiceMonitor endpoint after manual modification", func() {
			mc := validMemcached(uniqueName("sm-drift"))
			mc.Spec.Monitoring = &memcachedv1alpha1.MonitoringSpec{
				Enabled:        true,
				ServiceMonitor: &memcachedv1alpha1.ServiceMonitorSpec{},
			}
			Expect(k8sClient.Create(ctx, mc)).To(Succeed())

			_, err := reconcileOnce(mc)
			Expect(err).NotTo(HaveOccurred())

			sm := fetchServiceMonitor(mc)
			Expect(sm.Spec.Endpoints).To(HaveLen(1))
			Expect(sm.Spec.Endpoints[0].Interval).To(Equal(monitoringv1.Duration("30s")))

			// Simulate drift: manually change the interval.
			patch := client.MergeFrom(sm.DeepCopy())
			sm.Spec.Endpoints[0].Interval = "999s"
			Expect(k8sClient.Patch(ctx, sm, patch)).To(Succeed())

			drifted := fetchServiceMonitor(mc)
			Expect(drifted.Spec.Endpoints[0].Interval).To(Equal(monitoringv1.Duration("999s")))

			// Reconcile should restore the correct interval.
			_, err = reconcileOnce(mc)
			Expect(err).NotTo(HaveOccurred())

			corrected := fetchServiceMonitor(mc)
			Expect(corrected.Spec.Endpoints[0].Interval).To(Equal(monitoringv1.Duration("30s")))
		})

		It("should restore ServiceMonitor labels after manual modification", func() {
			mc := validMemcached(uniqueName("sm-drift-lbl"))
			mc.Spec.Monitoring = &memcachedv1alpha1.MonitoringSpec{
				Enabled:        true,
				ServiceMonitor: &memcachedv1alpha1.ServiceMonitorSpec{},
			}
			Expect(k8sClient.Create(ctx, mc)).To(Succeed())

			_, err := reconcileOnce(mc)
			Expect(err).NotTo(HaveOccurred())

			sm := fetchServiceMonitor(mc)
			Expect(sm.Labels).To(HaveKeyWithValue("app.kubernetes.io/managed-by", "memcached-operator"))

			// Simulate drift: manually change a label.
			patch := client.MergeFrom(sm.DeepCopy())
			sm.Labels["app.kubernetes.io/managed-by"] = "manual"
			Expect(k8sClient.Patch(ctx, sm, patch)).To(Succeed())

			drifted := fetchServiceMonitor(mc)
			Expect(drifted.Labels["app.kubernetes.io/managed-by"]).To(Equal("manual"))

			// Reconcile should restore the correct label.
			_, err = reconcileOnce(mc)
			Expect(err).NotTo(HaveOccurred())

			corrected := fetchServiceMonitor(mc)
			Expect(corrected.Labels["app.kubernetes.io/managed-by"]).To(Equal("memcached-operator"))
		})
	})

	Context("ServiceMonitor update when spec changes (REQ-006)", func() {
		It("should update ServiceMonitor when interval changes", func() {
			mc := validMemcached(uniqueName("sm-update"))
			mc.Spec.Monitoring = &memcachedv1alpha1.MonitoringSpec{
				Enabled:        true,
				ServiceMonitor: &memcachedv1alpha1.ServiceMonitorSpec{},
			}
			Expect(k8sClient.Create(ctx, mc)).To(Succeed())

			_, err := reconcileOnce(mc)
			Expect(err).NotTo(HaveOccurred())

			sm := fetchServiceMonitor(mc)
			Expect(sm.Spec.Endpoints[0].Interval).To(Equal(monitoringv1.Duration("30s")))

			// Update the CR's ServiceMonitor interval.
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(mc), mc)).To(Succeed())
			mc.Spec.Monitoring.ServiceMonitor.Interval = "60s"
			Expect(k8sClient.Update(ctx, mc)).To(Succeed())

			_, err = reconcileOnce(mc)
			Expect(err).NotTo(HaveOccurred())

			updated := fetchServiceMonitor(mc)
			Expect(updated.Spec.Endpoints[0].Interval).To(Equal(monitoringv1.Duration("60s")))
		})

		It("should update ServiceMonitor when additional labels change", func() {
			mc := validMemcached(uniqueName("sm-updlbl"))
			mc.Spec.Monitoring = &memcachedv1alpha1.MonitoringSpec{
				Enabled: true,
				ServiceMonitor: &memcachedv1alpha1.ServiceMonitorSpec{
					AdditionalLabels: map[string]string{
						"release": "prometheus",
					},
				},
			}
			Expect(k8sClient.Create(ctx, mc)).To(Succeed())

			_, err := reconcileOnce(mc)
			Expect(err).NotTo(HaveOccurred())

			sm := fetchServiceMonitor(mc)
			Expect(sm.Labels).To(HaveKeyWithValue("release", "prometheus"))

			// Update additional labels.
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(mc), mc)).To(Succeed())
			mc.Spec.Monitoring.ServiceMonitor.AdditionalLabels = map[string]string{
				"release": "kube-prom-stack",
				"env":     "production",
			}
			Expect(k8sClient.Update(ctx, mc)).To(Succeed())

			_, err = reconcileOnce(mc)
			Expect(err).NotTo(HaveOccurred())

			updated := fetchServiceMonitor(mc)
			Expect(updated.Labels).To(HaveKeyWithValue("release", "kube-prom-stack"))
			Expect(updated.Labels).To(HaveKeyWithValue("env", "production"))
		})
	})

	// --- Task 3.4: ServiceMonitor coexistence with other features (REQ-001, REQ-004) ---

	Context("ServiceMonitor coexistence with other features (REQ-001, REQ-004)", func() {

		It("should coexist with PDB and monitoring sidecar", func() {
			mc := validMemcached(uniqueName("sm-coexist"))
			mc.Spec.HighAvailability = &memcachedv1alpha1.HighAvailabilitySpec{
				PodDisruptionBudget: &memcachedv1alpha1.PDBSpec{Enabled: true},
			}
			mc.Spec.Monitoring = &memcachedv1alpha1.MonitoringSpec{
				Enabled:        true,
				ServiceMonitor: &memcachedv1alpha1.ServiceMonitorSpec{},
			}
			Expect(k8sClient.Create(ctx, mc)).To(Succeed())

			_, err := reconcileOnce(mc)
			Expect(err).NotTo(HaveOccurred())

			// ServiceMonitor exists.
			sm := fetchServiceMonitor(mc)
			Expect(sm.Spec.Endpoints).To(HaveLen(1))
			Expect(sm.Spec.Endpoints[0].Port).To(Equal("metrics"))

			// PDB exists.
			pdb := fetchPDB(mc)
			Expect(pdb.Spec.MinAvailable).NotTo(BeNil())
			Expect(pdb.Spec.MinAvailable.IntValue()).To(Equal(1))

			// Deployment has exporter sidecar.
			dep := fetchDeployment(mc)
			Expect(dep.Spec.Template.Spec.Containers).To(HaveLen(2))
			Expect(dep.Spec.Template.Spec.Containers[1].Name).To(Equal("exporter"))

			// Service has metrics port.
			svc := fetchService(mc)
			Expect(svc.Spec.Ports).To(HaveLen(2))
			Expect(svc.Spec.Ports[1].Name).To(Equal("metrics"))
		})

		It("should coexist with graceful shutdown and anti-affinity", func() {
			mc := validMemcached(uniqueName("sm-coex-ha"))
			soft := memcachedv1alpha1.AntiAffinityPresetSoft
			mc.Spec.HighAvailability = &memcachedv1alpha1.HighAvailabilitySpec{
				AntiAffinityPreset: &soft,
				GracefulShutdown: &memcachedv1alpha1.GracefulShutdownSpec{
					Enabled:                       true,
					PreStopDelaySeconds:           10,
					TerminationGracePeriodSeconds: 30,
				},
				PodDisruptionBudget: &memcachedv1alpha1.PDBSpec{Enabled: true},
			}
			mc.Spec.Monitoring = &memcachedv1alpha1.MonitoringSpec{
				Enabled: true,
				ServiceMonitor: &memcachedv1alpha1.ServiceMonitorSpec{
					Interval: "15s",
					AdditionalLabels: map[string]string{
						"release": "prometheus",
					},
				},
			}
			Expect(k8sClient.Create(ctx, mc)).To(Succeed())

			_, err := reconcileOnce(mc)
			Expect(err).NotTo(HaveOccurred())

			// ServiceMonitor with custom config.
			sm := fetchServiceMonitor(mc)
			Expect(sm.Spec.Endpoints[0].Interval).To(Equal(monitoringv1.Duration("15s")))
			Expect(sm.Labels).To(HaveKeyWithValue("release", "prometheus"))

			// Deployment: 2 containers, graceful shutdown, anti-affinity.
			dep := fetchDeployment(mc)
			Expect(dep.Spec.Template.Spec.Containers).To(HaveLen(2))
			Expect(dep.Spec.Template.Spec.Containers[0].Lifecycle).NotTo(BeNil())
			Expect(dep.Spec.Template.Spec.Containers[0].Lifecycle.PreStop.Exec.Command).To(Equal([]string{"sleep", "10"}))
			Expect(*dep.Spec.Template.Spec.TerminationGracePeriodSeconds).To(Equal(int64(30)))
			Expect(dep.Spec.Template.Spec.Affinity).NotTo(BeNil())
			Expect(dep.Spec.Template.Spec.Affinity.PodAntiAffinity).NotTo(BeNil())

			// PDB exists.
			pdb := fetchPDB(mc)
			Expect(pdb.Spec.MinAvailable).NotTo(BeNil())
		})

		It("should create ServiceMonitor alongside custom PDB configuration", func() {
			mc := validMemcached(uniqueName("sm-coex-pdb"))
			minAvail := intstr.FromInt32(2)
			mc.Spec.HighAvailability = &memcachedv1alpha1.HighAvailabilitySpec{
				PodDisruptionBudget: &memcachedv1alpha1.PDBSpec{
					Enabled:      true,
					MinAvailable: &minAvail,
				},
			}
			mc.Spec.Monitoring = &memcachedv1alpha1.MonitoringSpec{
				Enabled: true,
				ServiceMonitor: &memcachedv1alpha1.ServiceMonitorSpec{
					ScrapeTimeout: "5s",
				},
			}
			Expect(k8sClient.Create(ctx, mc)).To(Succeed())

			_, err := reconcileOnce(mc)
			Expect(err).NotTo(HaveOccurred())

			// ServiceMonitor with custom scrapeTimeout.
			sm := fetchServiceMonitor(mc)
			Expect(sm.Spec.Endpoints[0].ScrapeTimeout).To(Equal(monitoringv1.Duration("5s")))

			// PDB with custom minAvailable.
			pdb := fetchPDB(mc)
			Expect(pdb.Spec.MinAvailable.IntValue()).To(Equal(2))
		})

		It("should handle toggling monitoring off and back on", func() {
			mc := validMemcached(uniqueName("sm-toggle"))
			mc.Spec.Monitoring = &memcachedv1alpha1.MonitoringSpec{
				Enabled:        true,
				ServiceMonitor: &memcachedv1alpha1.ServiceMonitorSpec{},
			}
			Expect(k8sClient.Create(ctx, mc)).To(Succeed())

			// Step 1: Initial reconcile — ServiceMonitor should exist.
			_, err := reconcileOnce(mc)
			Expect(err).NotTo(HaveOccurred())

			sm := fetchServiceMonitor(mc)
			Expect(sm.Spec.Endpoints).To(HaveLen(1))
			Expect(sm.Spec.Endpoints[0].Port).To(Equal("metrics"))

			// Step 2: Disable monitoring — ServiceMonitor should no longer be created on subsequent reconciles.
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(mc), mc)).To(Succeed())
			mc.Spec.Monitoring.Enabled = false
			Expect(k8sClient.Update(ctx, mc)).To(Succeed())

			_, err = reconcileOnce(mc)
			Expect(err).NotTo(HaveOccurred())

			// The reconciler skips ServiceMonitor when disabled; the existing one remains
			// (owned by the CR, so GC would eventually remove it), but a new reconcile
			// should not fail and should not update it. Verify the guard returns false.
			// For a full delete we'd need GC, but we verify the reconciler does not
			// re-create or error.
			_, err = reconcileOnce(mc)
			Expect(err).NotTo(HaveOccurred())

			// Step 3: Re-enable monitoring — ServiceMonitor should be created again.
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(mc), mc)).To(Succeed())
			mc.Spec.Monitoring.Enabled = true
			mc.Spec.Monitoring.ServiceMonitor = &memcachedv1alpha1.ServiceMonitorSpec{
				Interval: "45s",
			}
			Expect(k8sClient.Update(ctx, mc)).To(Succeed())

			_, err = reconcileOnce(mc)
			Expect(err).NotTo(HaveOccurred())

			sm = fetchServiceMonitor(mc)
			Expect(sm.Spec.Endpoints).To(HaveLen(1))
			Expect(sm.Spec.Endpoints[0].Port).To(Equal("metrics"))
			Expect(sm.Spec.Endpoints[0].Interval).To(Equal(monitoringv1.Duration("45s")))
		})

		It("should not create ServiceMonitor when monitoring enabled but serviceMonitor nil alongside PDB", func() {
			mc := validMemcached(uniqueName("sm-coex-nosm"))
			mc.Spec.HighAvailability = &memcachedv1alpha1.HighAvailabilitySpec{
				PodDisruptionBudget: &memcachedv1alpha1.PDBSpec{Enabled: true},
			}
			mc.Spec.Monitoring = &memcachedv1alpha1.MonitoringSpec{
				Enabled: true,
			}
			Expect(k8sClient.Create(ctx, mc)).To(Succeed())

			_, err := reconcileOnce(mc)
			Expect(err).NotTo(HaveOccurred())

			// PDB should exist.
			pdb := fetchPDB(mc)
			Expect(pdb.Spec.MinAvailable).NotTo(BeNil())

			// ServiceMonitor should NOT exist.
			sm := &monitoringv1.ServiceMonitor{}
			err = k8sClient.Get(ctx, client.ObjectKeyFromObject(mc), sm)
			Expect(apierrors.IsNotFound(err)).To(BeTrue())

			// Exporter sidecar should still be injected.
			dep := fetchDeployment(mc)
			Expect(dep.Spec.Template.Spec.Containers).To(HaveLen(2))
		})
	})
})
