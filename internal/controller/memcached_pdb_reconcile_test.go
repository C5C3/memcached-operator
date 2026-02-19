package controller_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	policyv1 "k8s.io/api/policy/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/util/intstr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	memcachedv1alpha1 "github.com/c5c3/memcached-operator/api/v1alpha1"
)

// fetchPDB retrieves the PDB with the same name/namespace as the Memcached CR.
func fetchPDB(mc *memcachedv1alpha1.Memcached) *policyv1.PodDisruptionBudget {
	pdb := &policyv1.PodDisruptionBudget{}
	ExpectWithOffset(1, k8sClient.Get(ctx, client.ObjectKeyFromObject(mc), pdb)).To(Succeed())
	return pdb
}

var _ = Describe("PDB Reconciliation", func() {

	Context("PDB creation with defaults", func() {
		var mc *memcachedv1alpha1.Memcached

		BeforeEach(func() {
			mc = validMemcached(uniqueName("pdb-defaults"))
			mc.Spec.HighAvailability = &memcachedv1alpha1.HighAvailabilitySpec{
				PodDisruptionBudget: &memcachedv1alpha1.PDBSpec{Enabled: true},
			}
			Expect(k8sClient.Create(ctx, mc)).To(Succeed())
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(mc), mc)).To(Succeed())
			result, err := reconcileOnce(mc)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal(ctrl.Result{}))
		})

		It("should create PDB with minAvailable=1", func() {
			pdb := fetchPDB(mc)
			Expect(pdb.Spec.MinAvailable).NotTo(BeNil())
			Expect(pdb.Spec.MinAvailable.IntValue()).To(Equal(1))
			Expect(pdb.Spec.MaxUnavailable).To(BeNil())

			// Standard labels on metadata.
			Expect(pdb.Labels).To(HaveKeyWithValue("app.kubernetes.io/name", "memcached"))
			Expect(pdb.Labels).To(HaveKeyWithValue("app.kubernetes.io/instance", mc.Name))
			Expect(pdb.Labels).To(HaveKeyWithValue("app.kubernetes.io/managed-by", "memcached-operator"))

			// Standard labels on selector.
			Expect(pdb.Spec.Selector).NotTo(BeNil())
			Expect(pdb.Spec.Selector.MatchLabels).To(HaveKeyWithValue("app.kubernetes.io/name", "memcached"))
			Expect(pdb.Spec.Selector.MatchLabels).To(HaveKeyWithValue("app.kubernetes.io/instance", mc.Name))
			Expect(pdb.Spec.Selector.MatchLabels).To(HaveKeyWithValue("app.kubernetes.io/managed-by", "memcached-operator"))
		})

		It("should set owner reference", func() {
			pdb := fetchPDB(mc)
			Expect(pdb.OwnerReferences).To(HaveLen(1))
			ownerRef := pdb.OwnerReferences[0]
			Expect(ownerRef.APIVersion).To(Equal("memcached.c5c3.io/v1alpha1"))
			Expect(ownerRef.Kind).To(Equal("Memcached"))
			Expect(ownerRef.Name).To(Equal(mc.Name))
			Expect(ownerRef.UID).To(Equal(mc.UID))
			Expect(*ownerRef.Controller).To(BeTrue())
			Expect(*ownerRef.BlockOwnerDeletion).To(BeTrue())
		})
	})

	Context("PDB with custom minAvailable", func() {
		It("should create PDB with minAvailable=2", func() {
			mc := validMemcached(uniqueName("pdb-minavail"))
			minAvail := intstr.FromInt32(2)
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
			Expect(pdb.Spec.MinAvailable).NotTo(BeNil())
			Expect(pdb.Spec.MinAvailable.IntValue()).To(Equal(2))
			Expect(pdb.Spec.MaxUnavailable).To(BeNil())
		})
	})

	Context("PDB with maxUnavailable", func() {
		It("should create PDB with maxUnavailable=1 and no minAvailable", func() {
			mc := validMemcached(uniqueName("pdb-maxunavail"))
			maxUnavail := intstr.FromInt32(1)
			mc.Spec.HighAvailability = &memcachedv1alpha1.HighAvailabilitySpec{
				PodDisruptionBudget: &memcachedv1alpha1.PDBSpec{
					Enabled:        true,
					MaxUnavailable: &maxUnavail,
				},
			}
			Expect(k8sClient.Create(ctx, mc)).To(Succeed())

			_, err := reconcileOnce(mc)
			Expect(err).NotTo(HaveOccurred())

			pdb := fetchPDB(mc)
			Expect(pdb.Spec.MaxUnavailable).NotTo(BeNil())
			Expect(pdb.Spec.MaxUnavailable.IntValue()).To(Equal(1))
			Expect(pdb.Spec.MinAvailable).To(BeNil())
		})
	})

	Context("PDB with minAvailable percentage", func() {
		It("should create PDB with minAvailable=50%", func() {
			mc := validMemcached(uniqueName("pdb-minavail-pct"))
			minAvail := intstr.FromString("50%")
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
			Expect(pdb.Spec.MinAvailable).NotTo(BeNil())
			Expect(pdb.Spec.MinAvailable.String()).To(Equal("50%"))
			Expect(pdb.Spec.MaxUnavailable).To(BeNil())
		})
	})

	Context("PDB with maxUnavailable percentage", func() {
		It("should create PDB with maxUnavailable=25%", func() {
			mc := validMemcached(uniqueName("pdb-maxunavail-pct"))
			maxUnavail := intstr.FromString("25%")
			mc.Spec.HighAvailability = &memcachedv1alpha1.HighAvailabilitySpec{
				PodDisruptionBudget: &memcachedv1alpha1.PDBSpec{
					Enabled:        true,
					MaxUnavailable: &maxUnavail,
				},
			}
			Expect(k8sClient.Create(ctx, mc)).To(Succeed())

			_, err := reconcileOnce(mc)
			Expect(err).NotTo(HaveOccurred())

			pdb := fetchPDB(mc)
			Expect(pdb.Spec.MaxUnavailable).NotTo(BeNil())
			Expect(pdb.Spec.MaxUnavailable.String()).To(Equal("25%"))
			Expect(pdb.Spec.MinAvailable).To(BeNil())
		})
	})

	Context("PDB with both minAvailable and maxUnavailable set", func() {
		It("should use minAvailable and clear maxUnavailable", func() {
			mc := validMemcached(uniqueName("pdb-both-set"))
			minAvail := intstr.FromInt32(2)
			maxUnavail := intstr.FromInt32(1)
			mc.Spec.HighAvailability = &memcachedv1alpha1.HighAvailabilitySpec{
				PodDisruptionBudget: &memcachedv1alpha1.PDBSpec{
					Enabled:        true,
					MinAvailable:   &minAvail,
					MaxUnavailable: &maxUnavail,
				},
			}
			Expect(k8sClient.Create(ctx, mc)).To(Succeed())

			_, err := reconcileOnce(mc)
			Expect(err).NotTo(HaveOccurred())

			pdb := fetchPDB(mc)
			Expect(pdb.Spec.MinAvailable).NotTo(BeNil())
			Expect(pdb.Spec.MinAvailable.IntValue()).To(Equal(2))
			Expect(pdb.Spec.MaxUnavailable).To(BeNil())
		})
	})

	Context("No PDB when not enabled", func() {
		It("should not create a PDB resource", func() {
			mc := validMemcached(uniqueName("pdb-disabled"))
			Expect(k8sClient.Create(ctx, mc)).To(Succeed())

			_, err := reconcileOnce(mc)
			Expect(err).NotTo(HaveOccurred())

			pdb := &policyv1.PodDisruptionBudget{}
			err = k8sClient.Get(ctx, client.ObjectKeyFromObject(mc), pdb)
			Expect(apierrors.IsNotFound(err)).To(BeTrue())
		})
	})

	Context("PDB update when minAvailable changes", func() {
		It("should update PDB when minAvailable changes from 1 to 2", func() {
			mc := validMemcached(uniqueName("pdb-update"))
			mc.Spec.HighAvailability = &memcachedv1alpha1.HighAvailabilitySpec{
				PodDisruptionBudget: &memcachedv1alpha1.PDBSpec{Enabled: true},
			}
			Expect(k8sClient.Create(ctx, mc)).To(Succeed())

			_, err := reconcileOnce(mc)
			Expect(err).NotTo(HaveOccurred())

			pdb := fetchPDB(mc)
			Expect(pdb.Spec.MinAvailable.IntValue()).To(Equal(1))

			// Update minAvailable to 2.
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(mc), mc)).To(Succeed())
			minAvail := intstr.FromInt32(2)
			mc.Spec.HighAvailability.PodDisruptionBudget.MinAvailable = &minAvail
			Expect(k8sClient.Update(ctx, mc)).To(Succeed())

			_, err = reconcileOnce(mc)
			Expect(err).NotTo(HaveOccurred())

			pdb = fetchPDB(mc)
			Expect(pdb.Spec.MinAvailable).NotTo(BeNil())
			Expect(pdb.Spec.MinAvailable.IntValue()).To(Equal(2))
		})
	})

	Context("Idempotent PDB reconciliation", func() {
		It("should not change PDB resource version on second reconcile", func() {
			mc := validMemcached(uniqueName("pdb-idempotent"))
			mc.Spec.HighAvailability = &memcachedv1alpha1.HighAvailabilitySpec{
				PodDisruptionBudget: &memcachedv1alpha1.PDBSpec{Enabled: true},
			}
			Expect(k8sClient.Create(ctx, mc)).To(Succeed())

			_, err := reconcileOnce(mc)
			Expect(err).NotTo(HaveOccurred())

			pdb1 := fetchPDB(mc)
			rv1 := pdb1.ResourceVersion

			// Reconcile again without changes.
			_, err = reconcileOnce(mc)
			Expect(err).NotTo(HaveOccurred())

			pdb2 := fetchPDB(mc)
			Expect(pdb2.ResourceVersion).To(Equal(rv1))
		})
	})
})
