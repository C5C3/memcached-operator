package controller_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	monitoringv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	policyv1 "k8s.io/api/policy/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	memcachedv1alpha1 "github.com/c5c3/memcached-operator/api/v1alpha1"
)

// expectOwnerRef asserts that obj has exactly one owner reference pointing to mc
// with all required fields: apiVersion, kind, name, uid, controller, blockOwnerDeletion.
func expectOwnerRef(obj client.Object, mc *memcachedv1alpha1.Memcached) {
	refs := obj.GetOwnerReferences()
	ExpectWithOffset(1, refs).To(HaveLen(1))
	ref := refs[0]
	ExpectWithOffset(1, ref.APIVersion).To(Equal("memcached.c5c3.io/v1alpha1"))
	ExpectWithOffset(1, ref.Kind).To(Equal("Memcached"))
	ExpectWithOffset(1, ref.Name).To(Equal(mc.Name))
	ExpectWithOffset(1, ref.UID).To(Equal(mc.UID))
	ExpectWithOffset(1, *ref.Controller).To(BeTrue())
	ExpectWithOffset(1, *ref.BlockOwnerDeletion).To(BeTrue())
}

// --- Task 1.1: Full reconciliation loop for minimal CR ---

var _ = Describe("Full reconciliation loop: minimal CR", func() {

	Context("when a minimal Memcached CR is created with only defaults", func() {
		var mc *memcachedv1alpha1.Memcached

		BeforeEach(func() {
			mc = validMemcached(uniqueName("integ-minimal"))
			Expect(k8sClient.Create(ctx, mc)).To(Succeed())
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(mc), mc)).To(Succeed())

			result, err := reconcileOnce(mc)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal(ctrl.Result{}))
		})

		It("should create a Deployment with default settings", func() {
			dep := fetchDeployment(mc)
			Expect(*dep.Spec.Replicas).To(Equal(int32(1)))
			Expect(dep.Spec.Template.Spec.Containers).To(HaveLen(1))
			Expect(dep.Spec.Template.Spec.Containers[0].Name).To(Equal("memcached"))
			Expect(dep.Spec.Template.Spec.Containers[0].Image).To(Equal("memcached:1.6"))
			Expect(dep.Spec.Template.Spec.Containers[0].Ports).To(HaveLen(1))
			Expect(dep.Spec.Template.Spec.Containers[0].Ports[0].ContainerPort).To(Equal(int32(11211)))

			expectOwnerRef(dep, mc)
		})

		It("should create a headless Service on port 11211", func() {
			svc := fetchService(mc)
			Expect(svc.Spec.ClusterIP).To(Equal(corev1.ClusterIPNone))
			Expect(svc.Spec.Ports).To(HaveLen(1))
			Expect(svc.Spec.Ports[0].Name).To(Equal("memcached"))
			Expect(svc.Spec.Ports[0].Port).To(Equal(int32(11211)))

			expectOwnerRef(svc, mc)
		})

		It("should NOT create a PDB", func() {
			pdb := &policyv1.PodDisruptionBudget{}
			err := k8sClient.Get(ctx, client.ObjectKeyFromObject(mc), pdb)
			Expect(apierrors.IsNotFound(err)).To(BeTrue())
		})

		It("should NOT create a ServiceMonitor", func() {
			sm := &monitoringv1.ServiceMonitor{}
			err := k8sClient.Get(ctx, client.ObjectKeyFromObject(mc), sm)
			Expect(apierrors.IsNotFound(err)).To(BeTrue())
		})

		It("should NOT create a NetworkPolicy", func() {
			np := &networkingv1.NetworkPolicy{}
			err := k8sClient.Get(ctx, client.ObjectKeyFromObject(mc), np)
			Expect(apierrors.IsNotFound(err)).To(BeTrue())
		})

		It("should be a no-op on second reconcile", func() {
			dep1 := fetchDeployment(mc)
			svc1 := fetchService(mc)

			_, err := reconcileOnce(mc)
			Expect(err).NotTo(HaveOccurred())

			dep2 := fetchDeployment(mc)
			svc2 := fetchService(mc)
			Expect(dep2.ResourceVersion).To(Equal(dep1.ResourceVersion))
			Expect(svc2.ResourceVersion).To(Equal(svc1.ResourceVersion))
		})
	})
})

// --- Task 1.2: Full reconciliation loop for full-featured CR ---

var _ = Describe("Full reconciliation loop: full-featured CR", func() {

	Context("when a Memcached CR has all features enabled", func() {
		var mc *memcachedv1alpha1.Memcached

		BeforeEach(func() {
			mc = validMemcached(uniqueName("integ-full"))
			replicas := int32(3)
			mc.Spec.Replicas = &replicas
			image := "memcached:1.6.29"
			mc.Spec.Image = &image
			mc.Spec.Monitoring = &memcachedv1alpha1.MonitoringSpec{
				Enabled:        true,
				ServiceMonitor: &memcachedv1alpha1.ServiceMonitorSpec{},
			}
			minAvail := intstr.FromInt32(1)
			mc.Spec.HighAvailability = &memcachedv1alpha1.HighAvailabilitySpec{
				PodDisruptionBudget: &memcachedv1alpha1.PDBSpec{
					Enabled:      true,
					MinAvailable: &minAvail,
				},
			}
			mc.Spec.Security = &memcachedv1alpha1.SecuritySpec{
				NetworkPolicy: &memcachedv1alpha1.NetworkPolicySpec{Enabled: true},
			}

			Expect(k8sClient.Create(ctx, mc)).To(Succeed())
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(mc), mc)).To(Succeed())

			result, err := reconcileOnce(mc)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal(ctrl.Result{}))
		})

		It("should create a Deployment with 3 replicas and exporter sidecar", func() {
			dep := fetchDeployment(mc)
			Expect(*dep.Spec.Replicas).To(Equal(int32(3)))
			Expect(dep.Spec.Template.Spec.Containers).To(HaveLen(2))
			Expect(dep.Spec.Template.Spec.Containers[0].Name).To(Equal("memcached"))
			Expect(dep.Spec.Template.Spec.Containers[0].Image).To(Equal("memcached:1.6.29"))
			Expect(dep.Spec.Template.Spec.Containers[1].Name).To(Equal("exporter"))
			expectOwnerRef(dep, mc)
		})

		It("should create a headless Service with memcached and metrics ports", func() {
			svc := fetchService(mc)
			Expect(svc.Spec.ClusterIP).To(Equal(corev1.ClusterIPNone))
			Expect(svc.Spec.Ports).To(HaveLen(2))
			Expect(svc.Spec.Ports[0].Name).To(Equal("memcached"))
			Expect(svc.Spec.Ports[0].Port).To(Equal(int32(11211)))
			Expect(svc.Spec.Ports[1].Name).To(Equal("metrics"))
			Expect(svc.Spec.Ports[1].Port).To(Equal(int32(9150)))
		})

		It("should create a PDB with minAvailable=1", func() {
			pdb := fetchPDB(mc)
			Expect(pdb.Spec.MinAvailable).NotTo(BeNil())
			Expect(pdb.Spec.MinAvailable.IntValue()).To(Equal(1))
			expectOwnerRef(pdb, mc)
		})

		It("should create a ServiceMonitor with default interval", func() {
			sm := fetchServiceMonitor(mc)
			Expect(sm.Spec.Endpoints).To(HaveLen(1))
			Expect(sm.Spec.Endpoints[0].Port).To(Equal("metrics"))
			Expect(sm.Spec.Endpoints[0].Interval).To(Equal(monitoringv1.Duration("30s")))
			expectOwnerRef(sm, mc)
		})

		It("should create a NetworkPolicy with memcached and metrics ports", func() {
			np := fetchNetworkPolicy(mc)
			Expect(np.Spec.PolicyTypes).To(ConsistOf(networkingv1.PolicyTypeIngress))
			Expect(np.Spec.Ingress).To(HaveLen(1))
			Expect(np.Spec.Ingress[0].Ports).To(HaveLen(2))
			Expect(np.Spec.Ingress[0].Ports[0].Port.IntValue()).To(Equal(11211))
			Expect(np.Spec.Ingress[0].Ports[1].Port.IntValue()).To(Equal(9150))
			expectOwnerRef(np, mc)
		})

		It("should set all five resources with standard labels", func() {
			dep := fetchDeployment(mc)
			svc := fetchService(mc)
			pdb := fetchPDB(mc)
			sm := fetchServiceMonitor(mc)
			np := fetchNetworkPolicy(mc)

			for _, obj := range []client.Object{dep, svc, pdb, sm, np} {
				labels := obj.GetLabels()
				Expect(labels).To(HaveKeyWithValue("app.kubernetes.io/name", "memcached"))
				Expect(labels).To(HaveKeyWithValue("app.kubernetes.io/instance", mc.Name))
				Expect(labels).To(HaveKeyWithValue("app.kubernetes.io/managed-by", "memcached-operator"))
			}
		})
	})
})

// --- Task 1.3: Spec update propagation tests ---

var _ = Describe("Spec update propagation", func() {

	Context("replicas change propagates to Deployment", func() {
		It("should update Deployment replicas from 1 to 3", func() {
			mc := validMemcached(uniqueName("integ-rep-prop"))
			mc.Spec.Replicas = int32Ptr(1)
			Expect(k8sClient.Create(ctx, mc)).To(Succeed())

			_, err := reconcileOnce(mc)
			Expect(err).NotTo(HaveOccurred())

			dep := fetchDeployment(mc)
			Expect(*dep.Spec.Replicas).To(Equal(int32(1)))

			// Update replicas.
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(mc), mc)).To(Succeed())
			mc.Spec.Replicas = int32Ptr(3)
			Expect(k8sClient.Update(ctx, mc)).To(Succeed())

			_, err = reconcileOnce(mc)
			Expect(err).NotTo(HaveOccurred())

			dep = fetchDeployment(mc)
			Expect(*dep.Spec.Replicas).To(Equal(int32(3)))
		})
	})

	Context("image change propagates to Deployment", func() {
		It("should update Deployment container image", func() {
			mc := validMemcached(uniqueName("integ-img-prop"))
			Expect(k8sClient.Create(ctx, mc)).To(Succeed())

			_, err := reconcileOnce(mc)
			Expect(err).NotTo(HaveOccurred())

			dep := fetchDeployment(mc)
			Expect(dep.Spec.Template.Spec.Containers[0].Image).To(Equal("memcached:1.6"))

			// Update image.
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(mc), mc)).To(Succeed())
			mc.Spec.Image = strPtr("memcached:1.6.29")
			Expect(k8sClient.Update(ctx, mc)).To(Succeed())

			_, err = reconcileOnce(mc)
			Expect(err).NotTo(HaveOccurred())

			dep = fetchDeployment(mc)
			Expect(dep.Spec.Template.Spec.Containers[0].Image).To(Equal("memcached:1.6.29"))
		})
	})

	Context("enabling monitoring adds exporter sidecar and metrics port", func() {
		It("should add exporter container and Service metrics port on enable", func() {
			mc := validMemcached(uniqueName("integ-mon-enable"))
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
			mc.Spec.Monitoring = &memcachedv1alpha1.MonitoringSpec{Enabled: true}
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
			Expect(svc.Spec.Ports[1].Port).To(Equal(int32(9150)))
		})
	})

	Context("monitoring enable affects NetworkPolicy ports", func() {
		It("should add metrics port to NetworkPolicy when monitoring is enabled", func() {
			mc := validMemcached(uniqueName("integ-mon-np"))
			mc.Spec.Security = &memcachedv1alpha1.SecuritySpec{
				NetworkPolicy: &memcachedv1alpha1.NetworkPolicySpec{Enabled: true},
			}
			Expect(k8sClient.Create(ctx, mc)).To(Succeed())

			_, err := reconcileOnce(mc)
			Expect(err).NotTo(HaveOccurred())

			np := fetchNetworkPolicy(mc)
			Expect(np.Spec.Ingress[0].Ports).To(HaveLen(1))

			// Enable monitoring.
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(mc), mc)).To(Succeed())
			mc.Spec.Monitoring = &memcachedv1alpha1.MonitoringSpec{Enabled: true}
			Expect(k8sClient.Update(ctx, mc)).To(Succeed())

			_, err = reconcileOnce(mc)
			Expect(err).NotTo(HaveOccurred())

			np = fetchNetworkPolicy(mc)
			Expect(np.Spec.Ingress[0].Ports).To(HaveLen(2))
			Expect(np.Spec.Ingress[0].Ports[1].Port.IntValue()).To(Equal(9150))
		})
	})

	Context("replicas change propagates alongside image change", func() {
		It("should apply both replicas and image changes in a single reconcile", func() {
			mc := validMemcached(uniqueName("integ-multi-prop"))
			mc.Spec.Replicas = int32Ptr(1)
			Expect(k8sClient.Create(ctx, mc)).To(Succeed())

			_, err := reconcileOnce(mc)
			Expect(err).NotTo(HaveOccurred())

			// Update both replicas and image.
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(mc), mc)).To(Succeed())
			mc.Spec.Replicas = int32Ptr(5)
			mc.Spec.Image = strPtr("memcached:1.6.29")
			Expect(k8sClient.Update(ctx, mc)).To(Succeed())

			_, err = reconcileOnce(mc)
			Expect(err).NotTo(HaveOccurred())

			dep := fetchDeployment(mc)
			Expect(*dep.Spec.Replicas).To(Equal(int32(5)))
			Expect(dep.Spec.Template.Spec.Containers[0].Image).To(Equal("memcached:1.6.29"))
		})
	})
})

// --- Task 1.4: Optional resource enable/disable lifecycle tests ---

var _ = Describe("Optional resource enable/disable lifecycle", func() {

	Context("PDB enable and then disable", func() {
		It("should create PDB on enable and skip reconciliation when disabled", func() {
			mc := validMemcached(uniqueName("integ-pdb-toggle"))
			replicas := int32(3)
			mc.Spec.Replicas = &replicas
			minAvail := intstr.FromInt32(1)
			mc.Spec.HighAvailability = &memcachedv1alpha1.HighAvailabilitySpec{
				PodDisruptionBudget: &memcachedv1alpha1.PDBSpec{
					Enabled:      true,
					MinAvailable: &minAvail,
				},
			}
			Expect(k8sClient.Create(ctx, mc)).To(Succeed())

			_, err := reconcileOnce(mc)
			Expect(err).NotTo(HaveOccurred())

			// PDB exists.
			pdb := fetchPDB(mc)
			Expect(pdb.Spec.MinAvailable.IntValue()).To(Equal(1))

			// Disable PDB.
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(mc), mc)).To(Succeed())
			mc.Spec.HighAvailability.PodDisruptionBudget.Enabled = false
			Expect(k8sClient.Update(ctx, mc)).To(Succeed())

			_, err = reconcileOnce(mc)
			Expect(err).NotTo(HaveOccurred())

			// Reconciler skips PDB when disabled. Deployment and Service still exist.
			dep := fetchDeployment(mc)
			Expect(dep).NotTo(BeNil())
			svc := fetchService(mc)
			Expect(svc).NotTo(BeNil())
		})
	})

	Context("ServiceMonitor enable and then disable", func() {
		It("should create ServiceMonitor on enable and skip when disabled", func() {
			mc := validMemcached(uniqueName("integ-sm-toggle"))
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

			// Disable monitoring.
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(mc), mc)).To(Succeed())
			mc.Spec.Monitoring.Enabled = false
			Expect(k8sClient.Update(ctx, mc)).To(Succeed())

			_, err = reconcileOnce(mc)
			Expect(err).NotTo(HaveOccurred())

			// Re-enable with different interval.
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(mc), mc)).To(Succeed())
			mc.Spec.Monitoring.Enabled = true
			mc.Spec.Monitoring.ServiceMonitor = &memcachedv1alpha1.ServiceMonitorSpec{
				Interval: "60s",
			}
			Expect(k8sClient.Update(ctx, mc)).To(Succeed())

			_, err = reconcileOnce(mc)
			Expect(err).NotTo(HaveOccurred())

			sm = fetchServiceMonitor(mc)
			Expect(sm.Spec.Endpoints[0].Interval).To(Equal(monitoringv1.Duration("60s")))
		})
	})

	Context("NetworkPolicy enable and then disable", func() {
		It("should create NetworkPolicy on enable and skip when disabled", func() {
			mc := validMemcached(uniqueName("integ-np-toggle"))
			mc.Spec.Security = &memcachedv1alpha1.SecuritySpec{
				NetworkPolicy: &memcachedv1alpha1.NetworkPolicySpec{Enabled: true},
			}
			Expect(k8sClient.Create(ctx, mc)).To(Succeed())

			_, err := reconcileOnce(mc)
			Expect(err).NotTo(HaveOccurred())

			// NetworkPolicy exists.
			np := fetchNetworkPolicy(mc)
			Expect(np.Spec.Ingress).To(HaveLen(1))

			// Disable NetworkPolicy.
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(mc), mc)).To(Succeed())
			mc.Spec.Security.NetworkPolicy.Enabled = false
			Expect(k8sClient.Update(ctx, mc)).To(Succeed())

			_, err = reconcileOnce(mc)
			Expect(err).NotTo(HaveOccurred())

			// Core resources still exist.
			dep := fetchDeployment(mc)
			Expect(dep).NotTo(BeNil())
		})
	})

	Context("toggling all three optional resources", func() {
		It("should handle all three optional resources toggled on then off", func() {
			mc := validMemcached(uniqueName("integ-all-toggle"))
			replicas := int32(3)
			mc.Spec.Replicas = &replicas
			minAvail := intstr.FromInt32(1)
			mc.Spec.HighAvailability = &memcachedv1alpha1.HighAvailabilitySpec{
				PodDisruptionBudget: &memcachedv1alpha1.PDBSpec{
					Enabled:      true,
					MinAvailable: &minAvail,
				},
			}
			mc.Spec.Monitoring = &memcachedv1alpha1.MonitoringSpec{
				Enabled:        true,
				ServiceMonitor: &memcachedv1alpha1.ServiceMonitorSpec{},
			}
			mc.Spec.Security = &memcachedv1alpha1.SecuritySpec{
				NetworkPolicy: &memcachedv1alpha1.NetworkPolicySpec{Enabled: true},
			}
			Expect(k8sClient.Create(ctx, mc)).To(Succeed())

			_, err := reconcileOnce(mc)
			Expect(err).NotTo(HaveOccurred())

			// All five resources exist.
			fetchDeployment(mc)
			fetchService(mc)
			fetchPDB(mc)
			fetchServiceMonitor(mc)
			fetchNetworkPolicy(mc)

			// Disable all optional resources.
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(mc), mc)).To(Succeed())
			mc.Spec.HighAvailability.PodDisruptionBudget.Enabled = false
			mc.Spec.Monitoring.Enabled = false
			mc.Spec.Security.NetworkPolicy.Enabled = false
			Expect(k8sClient.Update(ctx, mc)).To(Succeed())

			_, err = reconcileOnce(mc)
			Expect(err).NotTo(HaveOccurred())

			// Core resources still exist.
			dep := fetchDeployment(mc)
			Expect(dep).NotTo(BeNil())
			svc := fetchService(mc)
			Expect(svc).NotTo(BeNil())
		})
	})
})

// --- Task 1.5: Full idempotency tests ---

var _ = Describe("Full idempotency: three consecutive reconciles on full-featured CR", func() {

	Context("three consecutive reconciles without spec changes", func() {
		It("should not change any resource version after the first reconcile", func() {
			mc := validMemcached(uniqueName("integ-idemp-full"))
			replicas := int32(3)
			mc.Spec.Replicas = &replicas
			mc.Spec.Monitoring = &memcachedv1alpha1.MonitoringSpec{
				Enabled:        true,
				ServiceMonitor: &memcachedv1alpha1.ServiceMonitorSpec{},
			}
			minAvail := intstr.FromInt32(1)
			mc.Spec.HighAvailability = &memcachedv1alpha1.HighAvailabilitySpec{
				PodDisruptionBudget: &memcachedv1alpha1.PDBSpec{
					Enabled:      true,
					MinAvailable: &minAvail,
				},
			}
			mc.Spec.Security = &memcachedv1alpha1.SecuritySpec{
				NetworkPolicy: &memcachedv1alpha1.NetworkPolicySpec{Enabled: true},
			}
			Expect(k8sClient.Create(ctx, mc)).To(Succeed())

			// First reconcile: creates all resources.
			_, err := reconcileOnce(mc)
			Expect(err).NotTo(HaveOccurred())

			dep1 := fetchDeployment(mc)
			svc1 := fetchService(mc)
			pdb1 := fetchPDB(mc)
			sm1 := fetchServiceMonitor(mc)
			np1 := fetchNetworkPolicy(mc)

			depRV := dep1.ResourceVersion
			svcRV := svc1.ResourceVersion
			pdbRV := pdb1.ResourceVersion
			smRV := sm1.ResourceVersion
			npRV := np1.ResourceVersion

			// Second reconcile: no-op.
			_, err = reconcileOnce(mc)
			Expect(err).NotTo(HaveOccurred())

			Expect(fetchDeployment(mc).ResourceVersion).To(Equal(depRV))
			Expect(fetchService(mc).ResourceVersion).To(Equal(svcRV))
			Expect(fetchPDB(mc).ResourceVersion).To(Equal(pdbRV))
			Expect(fetchServiceMonitor(mc).ResourceVersion).To(Equal(smRV))
			Expect(fetchNetworkPolicy(mc).ResourceVersion).To(Equal(npRV))

			// Third reconcile: still no-op.
			_, err = reconcileOnce(mc)
			Expect(err).NotTo(HaveOccurred())

			Expect(fetchDeployment(mc).ResourceVersion).To(Equal(depRV))
			Expect(fetchService(mc).ResourceVersion).To(Equal(svcRV))
			Expect(fetchPDB(mc).ResourceVersion).To(Equal(pdbRV))
			Expect(fetchServiceMonitor(mc).ResourceVersion).To(Equal(smRV))
			Expect(fetchNetworkPolicy(mc).ResourceVersion).To(Equal(npRV))
		})
	})

	Context("three consecutive reconciles on minimal CR", func() {
		It("should not change Deployment or Service resource versions", func() {
			mc := validMemcached(uniqueName("integ-idemp-min"))
			Expect(k8sClient.Create(ctx, mc)).To(Succeed())

			_, err := reconcileOnce(mc)
			Expect(err).NotTo(HaveOccurred())

			depRV := fetchDeployment(mc).ResourceVersion
			svcRV := fetchService(mc).ResourceVersion

			_, err = reconcileOnce(mc)
			Expect(err).NotTo(HaveOccurred())
			Expect(fetchDeployment(mc).ResourceVersion).To(Equal(depRV))
			Expect(fetchService(mc).ResourceVersion).To(Equal(svcRV))

			_, err = reconcileOnce(mc)
			Expect(err).NotTo(HaveOccurred())
			Expect(fetchDeployment(mc).ResourceVersion).To(Equal(depRV))
			Expect(fetchService(mc).ResourceVersion).To(Equal(svcRV))
		})
	})

	Context("idempotency after a spec change", func() {
		It("should update on first reconcile after change then no-op on subsequent two", func() {
			mc := validMemcached(uniqueName("integ-idemp-chg"))
			mc.Spec.Replicas = int32Ptr(1)
			Expect(k8sClient.Create(ctx, mc)).To(Succeed())

			_, err := reconcileOnce(mc)
			Expect(err).NotTo(HaveOccurred())

			// Change replicas.
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(mc), mc)).To(Succeed())
			mc.Spec.Replicas = int32Ptr(4)
			Expect(k8sClient.Update(ctx, mc)).To(Succeed())

			// First reconcile after change: updates Deployment.
			_, err = reconcileOnce(mc)
			Expect(err).NotTo(HaveOccurred())

			dep := fetchDeployment(mc)
			Expect(*dep.Spec.Replicas).To(Equal(int32(4)))
			depRV := dep.ResourceVersion

			// Second reconcile: no-op.
			_, err = reconcileOnce(mc)
			Expect(err).NotTo(HaveOccurred())
			Expect(fetchDeployment(mc).ResourceVersion).To(Equal(depRV))

			// Third reconcile: still no-op.
			_, err = reconcileOnce(mc)
			Expect(err).NotTo(HaveOccurred())
			Expect(fetchDeployment(mc).ResourceVersion).To(Equal(depRV))
		})
	})
})

// --- Task 1.6: Status conditions lifecycle tests ---

var _ = Describe("Status conditions lifecycle", func() {

	Context("initial status after first reconcile", func() {
		It("should set Available=False, Progressing=True, Degraded=True for 1 replica", func() {
			mc := validMemcached(uniqueName("integ-status-init"))
			Expect(k8sClient.Create(ctx, mc)).To(Succeed())
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(mc), mc)).To(Succeed())

			_, err := reconcileOnce(mc)
			Expect(err).NotTo(HaveOccurred())

			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(mc), mc)).To(Succeed())

			available := findCondition(mc.Status.Conditions, "Available")
			Expect(available).NotTo(BeNil())
			Expect(available.Status).To(Equal(metav1.ConditionFalse))

			progressing := findCondition(mc.Status.Conditions, "Progressing")
			Expect(progressing).NotTo(BeNil())
			Expect(progressing.Status).To(Equal(metav1.ConditionTrue))

			degraded := findCondition(mc.Status.Conditions, "Degraded")
			Expect(degraded).NotTo(BeNil())
			Expect(degraded.Status).To(Equal(metav1.ConditionTrue))

			Expect(mc.Status.ObservedGeneration).To(Equal(mc.Generation))
			Expect(mc.Status.ReadyReplicas).To(Equal(int32(0)))
		})
	})

	Context("status after spec change", func() {
		It("should update observedGeneration after replicas change", func() {
			mc := validMemcached(uniqueName("integ-status-gen"))
			mc.Spec.Replicas = int32Ptr(1)
			Expect(k8sClient.Create(ctx, mc)).To(Succeed())
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(mc), mc)).To(Succeed())

			_, err := reconcileOnce(mc)
			Expect(err).NotTo(HaveOccurred())

			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(mc), mc)).To(Succeed())
			gen1 := mc.Generation
			Expect(mc.Status.ObservedGeneration).To(Equal(gen1))

			// Update spec.
			mc.Spec.Replicas = int32Ptr(3)
			Expect(k8sClient.Update(ctx, mc)).To(Succeed())
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(mc), mc)).To(Succeed())
			gen2 := mc.Generation
			Expect(gen2).To(BeNumerically(">", gen1))

			_, err = reconcileOnce(mc)
			Expect(err).NotTo(HaveOccurred())

			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(mc), mc)).To(Succeed())
			Expect(mc.Status.ObservedGeneration).To(Equal(gen2))

			// Conditions still reflect envtest state (no real pods).
			Expect(mc.Status.Conditions).To(HaveLen(3))
			for _, c := range mc.Status.Conditions {
				Expect(c.ObservedGeneration).To(Equal(gen2))
				Expect(c.Reason).NotTo(BeEmpty())
				Expect(c.Message).NotTo(BeEmpty())
			}
		})
	})

	Context("status with zero replicas", func() {
		It("should set Available=True, Progressing=False, Degraded=False for 0 replicas", func() {
			mc := validMemcached(uniqueName("integ-status-zero"))
			mc.Spec.Replicas = int32Ptr(0)
			Expect(k8sClient.Create(ctx, mc)).To(Succeed())
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(mc), mc)).To(Succeed())

			_, err := reconcileOnce(mc)
			Expect(err).NotTo(HaveOccurred())

			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(mc), mc)).To(Succeed())

			available := findCondition(mc.Status.Conditions, "Available")
			Expect(available).NotTo(BeNil())
			Expect(available.Status).To(Equal(metav1.ConditionTrue))

			progressing := findCondition(mc.Status.Conditions, "Progressing")
			Expect(progressing).NotTo(BeNil())
			Expect(progressing.Status).To(Equal(metav1.ConditionFalse))

			degraded := findCondition(mc.Status.Conditions, "Degraded")
			Expect(degraded).NotTo(BeNil())
			Expect(degraded.Status).To(Equal(metav1.ConditionFalse))

			Expect(mc.Status.ReadyReplicas).To(Equal(int32(0)))
		})
	})

	Context("status transitions from non-zero to zero replicas", func() {
		It("should transition conditions when scaling from 1 to 0", func() {
			mc := validMemcached(uniqueName("integ-status-trans"))
			mc.Spec.Replicas = int32Ptr(1)
			Expect(k8sClient.Create(ctx, mc)).To(Succeed())
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(mc), mc)).To(Succeed())

			_, err := reconcileOnce(mc)
			Expect(err).NotTo(HaveOccurred())

			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(mc), mc)).To(Succeed())

			// With 1 replica desired but 0 ready (envtest): Available=False, Degraded=True.
			available := findCondition(mc.Status.Conditions, "Available")
			Expect(available.Status).To(Equal(metav1.ConditionFalse))
			degraded := findCondition(mc.Status.Conditions, "Degraded")
			Expect(degraded.Status).To(Equal(metav1.ConditionTrue))

			// Scale to 0.
			mc.Spec.Replicas = int32Ptr(0)
			Expect(k8sClient.Update(ctx, mc)).To(Succeed())

			_, err = reconcileOnce(mc)
			Expect(err).NotTo(HaveOccurred())

			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(mc), mc)).To(Succeed())

			// With 0 desired: Available=True, Progressing=False, Degraded=False.
			available = findCondition(mc.Status.Conditions, "Available")
			Expect(available.Status).To(Equal(metav1.ConditionTrue))
			progressing := findCondition(mc.Status.Conditions, "Progressing")
			Expect(progressing.Status).To(Equal(metav1.ConditionFalse))
			degraded = findCondition(mc.Status.Conditions, "Degraded")
			Expect(degraded.Status).To(Equal(metav1.ConditionFalse))
		})
	})

	Context("status within full reconcile loop with all features", func() {
		It("should set correct status alongside all five resources", func() {
			mc := validMemcached(uniqueName("integ-status-full"))
			replicas := int32(3)
			mc.Spec.Replicas = &replicas
			mc.Spec.Monitoring = &memcachedv1alpha1.MonitoringSpec{
				Enabled:        true,
				ServiceMonitor: &memcachedv1alpha1.ServiceMonitorSpec{},
			}
			minAvail := intstr.FromInt32(1)
			mc.Spec.HighAvailability = &memcachedv1alpha1.HighAvailabilitySpec{
				PodDisruptionBudget: &memcachedv1alpha1.PDBSpec{
					Enabled:      true,
					MinAvailable: &minAvail,
				},
			}
			mc.Spec.Security = &memcachedv1alpha1.SecuritySpec{
				NetworkPolicy: &memcachedv1alpha1.NetworkPolicySpec{Enabled: true},
			}
			Expect(k8sClient.Create(ctx, mc)).To(Succeed())
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(mc), mc)).To(Succeed())

			_, err := reconcileOnce(mc)
			Expect(err).NotTo(HaveOccurred())

			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(mc), mc)).To(Succeed())

			// All 5 resources exist.
			fetchDeployment(mc)
			fetchService(mc)
			fetchPDB(mc)
			fetchServiceMonitor(mc)
			fetchNetworkPolicy(mc)

			// Status is set.
			Expect(mc.Status.Conditions).To(HaveLen(3))
			Expect(mc.Status.ObservedGeneration).To(Equal(mc.Generation))

			// In envtest: 3 desired, 0 ready → Degraded=True, Available=False.
			degraded := findCondition(mc.Status.Conditions, "Degraded")
			Expect(degraded).NotTo(BeNil())
			Expect(degraded.Status).To(Equal(metav1.ConditionTrue))

			available := findCondition(mc.Status.Conditions, "Available")
			Expect(available).NotTo(BeNil())
			Expect(available.Status).To(Equal(metav1.ConditionFalse))
		})
	})
})

// --- Task 2.1: CR deletion and GC cleanup test ---

var _ = Describe("CR deletion and garbage collection", func() {

	Context("when a full-featured Memcached CR is deleted", func() {
		It("should have owner references on all resources that enable garbage collection", func() {
			mc := validMemcached(uniqueName("integ-gc-full"))
			replicas := int32(3)
			mc.Spec.Replicas = &replicas
			mc.Spec.Monitoring = &memcachedv1alpha1.MonitoringSpec{
				Enabled:        true,
				ServiceMonitor: &memcachedv1alpha1.ServiceMonitorSpec{},
			}
			minAvail := intstr.FromInt32(1)
			mc.Spec.HighAvailability = &memcachedv1alpha1.HighAvailabilitySpec{
				PodDisruptionBudget: &memcachedv1alpha1.PDBSpec{
					Enabled:      true,
					MinAvailable: &minAvail,
				},
			}
			mc.Spec.Security = &memcachedv1alpha1.SecuritySpec{
				NetworkPolicy: &memcachedv1alpha1.NetworkPolicySpec{Enabled: true},
			}
			Expect(k8sClient.Create(ctx, mc)).To(Succeed())
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(mc), mc)).To(Succeed())

			_, err := reconcileOnce(mc)
			Expect(err).NotTo(HaveOccurred())

			// Verify all five resources have correct owner references for GC.
			dep := fetchDeployment(mc)
			svc := fetchService(mc)
			pdb := fetchPDB(mc)
			sm := fetchServiceMonitor(mc)
			np := fetchNetworkPolicy(mc)

			for _, obj := range []client.Object{dep, svc, pdb, sm, np} {
				expectOwnerRef(obj, mc)
			}
		})

		It("should return not-found when reconciling a deleted CR", func() {
			mc := validMemcached(uniqueName("integ-gc-notfound"))
			Expect(k8sClient.Create(ctx, mc)).To(Succeed())

			_, err := reconcileOnce(mc)
			Expect(err).NotTo(HaveOccurred())

			// Delete the CR.
			Expect(k8sClient.Delete(ctx, mc)).To(Succeed())

			// Reconciling a deleted CR should succeed (no-op).
			result, err := reconcileOnce(mc)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal(ctrl.Result{}))
		})

		It("should have owner references on minimal CR resources for GC", func() {
			mc := validMemcached(uniqueName("integ-gc-min"))
			Expect(k8sClient.Create(ctx, mc)).To(Succeed())
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(mc), mc)).To(Succeed())

			_, err := reconcileOnce(mc)
			Expect(err).NotTo(HaveOccurred())

			dep := fetchDeployment(mc)
			svc := fetchService(mc)

			for _, obj := range []client.Object{dep, svc} {
				expectOwnerRef(obj, mc)
			}
		})
	})
})

// --- Task 2.2: Owned resource recreation tests ---

var _ = Describe("Owned resource recreation after external deletion", func() {

	Context("when a Deployment is externally deleted", func() {
		It("should recreate the Deployment on next reconcile", func() {
			mc := validMemcached(uniqueName("integ-recreate-dep"))
			Expect(k8sClient.Create(ctx, mc)).To(Succeed())

			_, err := reconcileOnce(mc)
			Expect(err).NotTo(HaveOccurred())

			dep := fetchDeployment(mc)
			originalUID := dep.UID

			// Externally delete the Deployment.
			Expect(k8sClient.Delete(ctx, dep)).To(Succeed())

			// Verify it's gone.
			err = k8sClient.Get(ctx, client.ObjectKeyFromObject(mc), &appsv1.Deployment{})
			Expect(apierrors.IsNotFound(err)).To(BeTrue())

			// Reconcile should recreate it.
			_, err = reconcileOnce(mc)
			Expect(err).NotTo(HaveOccurred())

			newDep := fetchDeployment(mc)
			Expect(newDep.UID).NotTo(Equal(originalUID))
			expectOwnerRef(newDep, mc)
		})
	})

	Context("when a Service is externally deleted", func() {
		It("should recreate the Service on next reconcile", func() {
			mc := validMemcached(uniqueName("integ-recreate-svc"))
			Expect(k8sClient.Create(ctx, mc)).To(Succeed())

			_, err := reconcileOnce(mc)
			Expect(err).NotTo(HaveOccurred())

			svc := fetchService(mc)
			originalUID := svc.UID

			// Externally delete the Service.
			Expect(k8sClient.Delete(ctx, svc)).To(Succeed())

			// Reconcile should recreate it.
			_, err = reconcileOnce(mc)
			Expect(err).NotTo(HaveOccurred())

			newSvc := fetchService(mc)
			Expect(newSvc.UID).NotTo(Equal(originalUID))
			Expect(newSvc.Spec.ClusterIP).To(Equal(corev1.ClusterIPNone))
			expectOwnerRef(newSvc, mc)
		})
	})

	Context("when a PDB is externally deleted", func() {
		It("should recreate the PDB on next reconcile", func() {
			mc := validMemcached(uniqueName("integ-recreate-pdb"))
			replicas := int32(3)
			mc.Spec.Replicas = &replicas
			minAvail := intstr.FromInt32(1)
			mc.Spec.HighAvailability = &memcachedv1alpha1.HighAvailabilitySpec{
				PodDisruptionBudget: &memcachedv1alpha1.PDBSpec{
					Enabled:      true,
					MinAvailable: &minAvail,
				},
			}
			Expect(k8sClient.Create(ctx, mc)).To(Succeed())

			_, err := reconcileOnce(mc)
			Expect(err).NotTo(HaveOccurred())

			pdb := fetchPDB(mc)
			originalUID := pdb.UID

			// Externally delete the PDB.
			Expect(k8sClient.Delete(ctx, pdb)).To(Succeed())

			// Reconcile should recreate it.
			_, err = reconcileOnce(mc)
			Expect(err).NotTo(HaveOccurred())

			newPDB := fetchPDB(mc)
			Expect(newPDB.UID).NotTo(Equal(originalUID))
			Expect(newPDB.Spec.MinAvailable.IntValue()).To(Equal(1))
			expectOwnerRef(newPDB, mc)
		})
	})

	Context("when a ServiceMonitor is externally deleted", func() {
		It("should recreate the ServiceMonitor on next reconcile", func() {
			mc := validMemcached(uniqueName("integ-recreate-sm"))
			mc.Spec.Monitoring = &memcachedv1alpha1.MonitoringSpec{
				Enabled:        true,
				ServiceMonitor: &memcachedv1alpha1.ServiceMonitorSpec{},
			}
			Expect(k8sClient.Create(ctx, mc)).To(Succeed())

			_, err := reconcileOnce(mc)
			Expect(err).NotTo(HaveOccurred())

			sm := fetchServiceMonitor(mc)
			originalUID := sm.UID

			// Externally delete the ServiceMonitor.
			Expect(k8sClient.Delete(ctx, sm)).To(Succeed())

			// Reconcile should recreate it.
			_, err = reconcileOnce(mc)
			Expect(err).NotTo(HaveOccurred())

			newSM := fetchServiceMonitor(mc)
			Expect(newSM.UID).NotTo(Equal(originalUID))
			Expect(newSM.Spec.Endpoints).To(HaveLen(1))
			expectOwnerRef(newSM, mc)
		})
	})

	Context("when a NetworkPolicy is externally deleted", func() {
		It("should recreate the NetworkPolicy on next reconcile", func() {
			mc := validMemcached(uniqueName("integ-recreate-np"))
			mc.Spec.Security = &memcachedv1alpha1.SecuritySpec{
				NetworkPolicy: &memcachedv1alpha1.NetworkPolicySpec{Enabled: true},
			}
			Expect(k8sClient.Create(ctx, mc)).To(Succeed())

			_, err := reconcileOnce(mc)
			Expect(err).NotTo(HaveOccurred())

			np := fetchNetworkPolicy(mc)
			originalUID := np.UID

			// Externally delete the NetworkPolicy.
			Expect(k8sClient.Delete(ctx, np)).To(Succeed())

			// Reconcile should recreate it.
			_, err = reconcileOnce(mc)
			Expect(err).NotTo(HaveOccurred())

			newNP := fetchNetworkPolicy(mc)
			Expect(newNP.UID).NotTo(Equal(originalUID))
			Expect(newNP.Spec.Ingress).To(HaveLen(1))
			expectOwnerRef(newNP, mc)
		})
	})

	Context("when multiple resources are externally deleted simultaneously", func() {
		It("should recreate all deleted resources in a single reconcile", func() {
			mc := validMemcached(uniqueName("integ-recreate-all"))
			replicas := int32(3)
			mc.Spec.Replicas = &replicas
			mc.Spec.Monitoring = &memcachedv1alpha1.MonitoringSpec{
				Enabled:        true,
				ServiceMonitor: &memcachedv1alpha1.ServiceMonitorSpec{},
			}
			minAvail := intstr.FromInt32(1)
			mc.Spec.HighAvailability = &memcachedv1alpha1.HighAvailabilitySpec{
				PodDisruptionBudget: &memcachedv1alpha1.PDBSpec{
					Enabled:      true,
					MinAvailable: &minAvail,
				},
			}
			mc.Spec.Security = &memcachedv1alpha1.SecuritySpec{
				NetworkPolicy: &memcachedv1alpha1.NetworkPolicySpec{Enabled: true},
			}
			Expect(k8sClient.Create(ctx, mc)).To(Succeed())

			_, err := reconcileOnce(mc)
			Expect(err).NotTo(HaveOccurred())

			// Record original UIDs.
			depUID := fetchDeployment(mc).UID
			svcUID := fetchService(mc).UID

			// Delete Deployment and Service.
			Expect(k8sClient.Delete(ctx, fetchDeployment(mc))).To(Succeed())
			Expect(k8sClient.Delete(ctx, fetchService(mc))).To(Succeed())

			// Single reconcile should recreate both.
			_, err = reconcileOnce(mc)
			Expect(err).NotTo(HaveOccurred())

			newDep := fetchDeployment(mc)
			newSvc := fetchService(mc)
			Expect(newDep.UID).NotTo(Equal(depUID))
			Expect(newSvc.UID).NotTo(Equal(svcUID))

			// Other resources should still exist.
			fetchPDB(mc)
			fetchServiceMonitor(mc)
			fetchNetworkPolicy(mc)
		})
	})
})

// --- Task 2.3: Multi-instance isolation tests ---

var _ = Describe("Multi-instance isolation: two CRs in same namespace", func() {

	Context("when two minimal CRs exist in the same namespace", func() {
		var mcA, mcB *memcachedv1alpha1.Memcached

		BeforeEach(func() {
			mcA = validMemcached(uniqueName("integ-iso-a"))
			mcB = validMemcached(uniqueName("integ-iso-b"))

			Expect(k8sClient.Create(ctx, mcA)).To(Succeed())
			Expect(k8sClient.Create(ctx, mcB)).To(Succeed())
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(mcA), mcA)).To(Succeed())
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(mcB), mcB)).To(Succeed())

			_, err := reconcileOnce(mcA)
			Expect(err).NotTo(HaveOccurred())
			_, err = reconcileOnce(mcB)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should create independent Deployments for each CR", func() {
			depA := fetchDeployment(mcA)
			depB := fetchDeployment(mcB)

			Expect(depA.Name).NotTo(Equal(depB.Name))
			Expect(depA.UID).NotTo(Equal(depB.UID))
			Expect(depA.OwnerReferences[0].UID).To(Equal(mcA.UID))
			Expect(depB.OwnerReferences[0].UID).To(Equal(mcB.UID))
		})

		It("should create independent Services for each CR", func() {
			svcA := fetchService(mcA)
			svcB := fetchService(mcB)

			Expect(svcA.Name).NotTo(Equal(svcB.Name))
			Expect(svcA.UID).NotTo(Equal(svcB.UID))
			Expect(svcA.OwnerReferences[0].UID).To(Equal(mcA.UID))
			Expect(svcB.OwnerReferences[0].UID).To(Equal(mcB.UID))
		})

		It("should update one CR without affecting the other", func() {
			// Update CR A replicas.
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(mcA), mcA)).To(Succeed())
			mcA.Spec.Replicas = int32Ptr(5)
			Expect(k8sClient.Update(ctx, mcA)).To(Succeed())

			_, err := reconcileOnce(mcA)
			Expect(err).NotTo(HaveOccurred())

			depA := fetchDeployment(mcA)
			depB := fetchDeployment(mcB)

			Expect(*depA.Spec.Replicas).To(Equal(int32(5)))
			Expect(*depB.Spec.Replicas).To(Equal(int32(1)))
		})

		It("should set distinct instance labels on each CR's resources", func() {
			depA := fetchDeployment(mcA)
			depB := fetchDeployment(mcB)

			Expect(depA.Labels["app.kubernetes.io/instance"]).To(Equal(mcA.Name))
			Expect(depB.Labels["app.kubernetes.io/instance"]).To(Equal(mcB.Name))
			Expect(depA.Labels["app.kubernetes.io/instance"]).NotTo(Equal(depB.Labels["app.kubernetes.io/instance"]))
		})
	})

	Context("when two full-featured CRs exist in the same namespace", func() {
		var mcA, mcB *memcachedv1alpha1.Memcached

		BeforeEach(func() {
			mcA = validMemcached(uniqueName("integ-iso-full-a"))
			replicas := int32(2)
			mcA.Spec.Replicas = &replicas
			mcA.Spec.Monitoring = &memcachedv1alpha1.MonitoringSpec{
				Enabled:        true,
				ServiceMonitor: &memcachedv1alpha1.ServiceMonitorSpec{},
			}
			minAvail := intstr.FromInt32(1)
			mcA.Spec.HighAvailability = &memcachedv1alpha1.HighAvailabilitySpec{
				PodDisruptionBudget: &memcachedv1alpha1.PDBSpec{
					Enabled:      true,
					MinAvailable: &minAvail,
				},
			}
			mcA.Spec.Security = &memcachedv1alpha1.SecuritySpec{
				NetworkPolicy: &memcachedv1alpha1.NetworkPolicySpec{Enabled: true},
			}

			mcB = validMemcached(uniqueName("integ-iso-full-b"))
			replicasB := int32(4)
			mcB.Spec.Replicas = &replicasB
			mcB.Spec.Monitoring = &memcachedv1alpha1.MonitoringSpec{
				Enabled:        true,
				ServiceMonitor: &memcachedv1alpha1.ServiceMonitorSpec{},
			}
			minAvailB := intstr.FromInt32(2)
			mcB.Spec.HighAvailability = &memcachedv1alpha1.HighAvailabilitySpec{
				PodDisruptionBudget: &memcachedv1alpha1.PDBSpec{
					Enabled:      true,
					MinAvailable: &minAvailB,
				},
			}
			mcB.Spec.Security = &memcachedv1alpha1.SecuritySpec{
				NetworkPolicy: &memcachedv1alpha1.NetworkPolicySpec{Enabled: true},
			}

			Expect(k8sClient.Create(ctx, mcA)).To(Succeed())
			Expect(k8sClient.Create(ctx, mcB)).To(Succeed())
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(mcA), mcA)).To(Succeed())
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(mcB), mcB)).To(Succeed())

			_, err := reconcileOnce(mcA)
			Expect(err).NotTo(HaveOccurred())
			_, err = reconcileOnce(mcB)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should create all five resources independently for each CR", func() {
			// CR A resources.
			depA := fetchDeployment(mcA)
			svcA := fetchService(mcA)
			pdbA := fetchPDB(mcA)
			smA := fetchServiceMonitor(mcA)
			npA := fetchNetworkPolicy(mcA)

			// CR B resources.
			depB := fetchDeployment(mcB)
			svcB := fetchService(mcB)
			pdbB := fetchPDB(mcB)
			smB := fetchServiceMonitor(mcB)
			npB := fetchNetworkPolicy(mcB)

			// UIDs must differ.
			Expect(depA.UID).NotTo(Equal(depB.UID))
			Expect(svcA.UID).NotTo(Equal(svcB.UID))
			Expect(pdbA.UID).NotTo(Equal(pdbB.UID))
			Expect(smA.UID).NotTo(Equal(smB.UID))
			Expect(npA.UID).NotTo(Equal(npB.UID))

			// Owner references point to correct CRs.
			Expect(depA.OwnerReferences[0].UID).To(Equal(mcA.UID))
			Expect(depB.OwnerReferences[0].UID).To(Equal(mcB.UID))

			// Spec values differ.
			Expect(*depA.Spec.Replicas).To(Equal(int32(2)))
			Expect(*depB.Spec.Replicas).To(Equal(int32(4)))
			Expect(pdbA.Spec.MinAvailable.IntValue()).To(Equal(1))
			Expect(pdbB.Spec.MinAvailable.IntValue()).To(Equal(2))
		})

		It("should allow deleting one CR without affecting the other's resources", func() {
			// Delete CR A.
			Expect(k8sClient.Delete(ctx, mcA)).To(Succeed())

			// Reconcile CR A should be a no-op (CR gone).
			result, err := reconcileOnce(mcA)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal(ctrl.Result{}))

			// CR B resources should still exist and be unaffected.
			depB := fetchDeployment(mcB)
			Expect(*depB.Spec.Replicas).To(Equal(int32(4)))
			fetchService(mcB)
			fetchPDB(mcB)
			fetchServiceMonitor(mcB)
			fetchNetworkPolicy(mcB)
		})
	})
})

// --- Task 3.1: Cross-resource consistency test ---

var _ = Describe("Cross-resource consistency: enabling monitoring updates multiple resources", func() {

	Context("when monitoring is enabled on a CR with NetworkPolicy", func() {
		It("should update Deployment (sidecar), Service (metrics port), and NetworkPolicy (metrics port) in one reconcile", func() {
			mc := validMemcached(uniqueName("integ-cross-mon"))
			mc.Spec.Security = &memcachedv1alpha1.SecuritySpec{
				NetworkPolicy: &memcachedv1alpha1.NetworkPolicySpec{Enabled: true},
			}
			Expect(k8sClient.Create(ctx, mc)).To(Succeed())

			_, err := reconcileOnce(mc)
			Expect(err).NotTo(HaveOccurred())

			// Before monitoring: 1 container, 1 service port, 1 NP port.
			dep := fetchDeployment(mc)
			Expect(dep.Spec.Template.Spec.Containers).To(HaveLen(1))
			svc := fetchService(mc)
			Expect(svc.Spec.Ports).To(HaveLen(1))
			np := fetchNetworkPolicy(mc)
			Expect(np.Spec.Ingress[0].Ports).To(HaveLen(1))

			// Enable monitoring.
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(mc), mc)).To(Succeed())
			mc.Spec.Monitoring = &memcachedv1alpha1.MonitoringSpec{
				Enabled:        true,
				ServiceMonitor: &memcachedv1alpha1.ServiceMonitorSpec{},
			}
			Expect(k8sClient.Update(ctx, mc)).To(Succeed())

			// Single reconcile updates all three resources plus creates ServiceMonitor.
			_, err = reconcileOnce(mc)
			Expect(err).NotTo(HaveOccurred())

			// Deployment: exporter sidecar added.
			dep = fetchDeployment(mc)
			Expect(dep.Spec.Template.Spec.Containers).To(HaveLen(2))
			Expect(dep.Spec.Template.Spec.Containers[1].Name).To(Equal("exporter"))

			// Service: metrics port added.
			svc = fetchService(mc)
			Expect(svc.Spec.Ports).To(HaveLen(2))
			Expect(svc.Spec.Ports[1].Name).To(Equal("metrics"))
			Expect(svc.Spec.Ports[1].Port).To(Equal(int32(9150)))

			// NetworkPolicy: metrics port added.
			np = fetchNetworkPolicy(mc)
			Expect(np.Spec.Ingress[0].Ports).To(HaveLen(2))
			Expect(np.Spec.Ingress[0].Ports[1].Port.IntValue()).To(Equal(9150))

			// ServiceMonitor: created.
			sm := fetchServiceMonitor(mc)
			Expect(sm.Spec.Endpoints).To(HaveLen(1))
			Expect(sm.Spec.Endpoints[0].Port).To(Equal("metrics"))
		})
	})

	Context("when monitoring is disabled on a full-featured CR", func() {
		It("should remove sidecar from Deployment, metrics port from Service and NetworkPolicy in one reconcile", func() {
			mc := validMemcached(uniqueName("integ-cross-mon-off"))
			mc.Spec.Monitoring = &memcachedv1alpha1.MonitoringSpec{
				Enabled:        true,
				ServiceMonitor: &memcachedv1alpha1.ServiceMonitorSpec{},
			}
			mc.Spec.Security = &memcachedv1alpha1.SecuritySpec{
				NetworkPolicy: &memcachedv1alpha1.NetworkPolicySpec{Enabled: true},
			}
			Expect(k8sClient.Create(ctx, mc)).To(Succeed())

			_, err := reconcileOnce(mc)
			Expect(err).NotTo(HaveOccurred())

			// Before: 2 containers, 2 service ports, 2 NP ports, ServiceMonitor exists.
			dep := fetchDeployment(mc)
			Expect(dep.Spec.Template.Spec.Containers).To(HaveLen(2))
			svc := fetchService(mc)
			Expect(svc.Spec.Ports).To(HaveLen(2))
			np := fetchNetworkPolicy(mc)
			Expect(np.Spec.Ingress[0].Ports).To(HaveLen(2))
			fetchServiceMonitor(mc)

			// Disable monitoring.
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(mc), mc)).To(Succeed())
			mc.Spec.Monitoring.Enabled = false
			Expect(k8sClient.Update(ctx, mc)).To(Succeed())

			_, err = reconcileOnce(mc)
			Expect(err).NotTo(HaveOccurred())

			// Deployment: exporter sidecar removed.
			dep = fetchDeployment(mc)
			Expect(dep.Spec.Template.Spec.Containers).To(HaveLen(1))
			Expect(dep.Spec.Template.Spec.Containers[0].Name).To(Equal("memcached"))

			// Service: metrics port removed.
			svc = fetchService(mc)
			Expect(svc.Spec.Ports).To(HaveLen(1))
			Expect(svc.Spec.Ports[0].Name).To(Equal("memcached"))

			// NetworkPolicy: metrics port removed.
			np = fetchNetworkPolicy(mc)
			Expect(np.Spec.Ingress[0].Ports).To(HaveLen(1))
			Expect(np.Spec.Ingress[0].Ports[0].Port.IntValue()).To(Equal(11211))
		})
	})
})

// --- Task 3.2: Full create-update-delete lifecycle integration test ---

var _ = Describe("Full create-update-delete lifecycle", func() {

	It("should handle the complete lifecycle: create → update → delete", func() {
		// Phase 1: Create a minimal CR and verify initial resources.
		mc := validMemcached(uniqueName("integ-lifecycle"))
		mc.Spec.Replicas = int32Ptr(1)
		Expect(k8sClient.Create(ctx, mc)).To(Succeed())
		Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(mc), mc)).To(Succeed())

		_, err := reconcileOnce(mc)
		Expect(err).NotTo(HaveOccurred())

		// Verify initial state: Deployment + Service only.
		dep := fetchDeployment(mc)
		Expect(*dep.Spec.Replicas).To(Equal(int32(1)))
		Expect(dep.Spec.Template.Spec.Containers).To(HaveLen(1))
		svc := fetchService(mc)
		Expect(svc.Spec.Ports).To(HaveLen(1))

		// Status set after first reconcile.
		Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(mc), mc)).To(Succeed())
		Expect(mc.Status.Conditions).To(HaveLen(3))
		gen1 := mc.Generation
		Expect(mc.Status.ObservedGeneration).To(Equal(gen1))

		// Phase 2: Update — scale up, enable all optional resources.
		mc.Spec.Replicas = int32Ptr(3)
		mc.Spec.Image = strPtr("memcached:1.6.29")
		mc.Spec.Monitoring = &memcachedv1alpha1.MonitoringSpec{
			Enabled:        true,
			ServiceMonitor: &memcachedv1alpha1.ServiceMonitorSpec{},
		}
		minAvail := intstr.FromInt32(1)
		mc.Spec.HighAvailability = &memcachedv1alpha1.HighAvailabilitySpec{
			PodDisruptionBudget: &memcachedv1alpha1.PDBSpec{
				Enabled:      true,
				MinAvailable: &minAvail,
			},
		}
		mc.Spec.Security = &memcachedv1alpha1.SecuritySpec{
			NetworkPolicy: &memcachedv1alpha1.NetworkPolicySpec{Enabled: true},
		}
		Expect(k8sClient.Update(ctx, mc)).To(Succeed())

		_, err = reconcileOnce(mc)
		Expect(err).NotTo(HaveOccurred())

		// Verify updated Deployment.
		dep = fetchDeployment(mc)
		Expect(*dep.Spec.Replicas).To(Equal(int32(3)))
		Expect(dep.Spec.Template.Spec.Containers[0].Image).To(Equal("memcached:1.6.29"))
		Expect(dep.Spec.Template.Spec.Containers).To(HaveLen(2))
		Expect(dep.Spec.Template.Spec.Containers[1].Name).To(Equal("exporter"))

		// Verify updated Service.
		svc = fetchService(mc)
		Expect(svc.Spec.Ports).To(HaveLen(2))
		Expect(svc.Spec.Ports[1].Name).To(Equal("metrics"))

		// Verify optional resources created.
		pdb := fetchPDB(mc)
		Expect(pdb.Spec.MinAvailable.IntValue()).To(Equal(1))
		sm := fetchServiceMonitor(mc)
		Expect(sm.Spec.Endpoints).To(HaveLen(1))
		np := fetchNetworkPolicy(mc)
		Expect(np.Spec.Ingress[0].Ports).To(HaveLen(2))

		// Status updated with new generation.
		Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(mc), mc)).To(Succeed())
		gen2 := mc.Generation
		Expect(gen2).To(BeNumerically(">", gen1))
		Expect(mc.Status.ObservedGeneration).To(Equal(gen2))

		// All resources have correct owner references.
		for _, obj := range []client.Object{dep, svc, pdb, sm, np} {
			expectOwnerRef(obj, mc)
		}

		// Phase 3: Idempotency — second reconcile is a no-op.
		depRV := fetchDeployment(mc).ResourceVersion
		svcRV := fetchService(mc).ResourceVersion

		_, err = reconcileOnce(mc)
		Expect(err).NotTo(HaveOccurred())

		Expect(fetchDeployment(mc).ResourceVersion).To(Equal(depRV))
		Expect(fetchService(mc).ResourceVersion).To(Equal(svcRV))

		// Phase 4: Delete the CR.
		Expect(k8sClient.Delete(ctx, mc)).To(Succeed())

		// Reconciling deleted CR returns success (no-op).
		result, err := reconcileOnce(mc)
		Expect(err).NotTo(HaveOccurred())
		Expect(result).To(Equal(ctrl.Result{}))
	})
})
