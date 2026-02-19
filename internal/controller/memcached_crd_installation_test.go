package controller_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	memcachedv1alpha1 "github.com/c5c3/memcached-operator/api/v1alpha1"
)

// --- REQ-008: Create and Fetch Minimal CR ---

var _ = Describe("CRD Installation: Create and Fetch Minimal CR", func() {

	Context("minimal CR with empty spec", func() {
		It("should create a minimal CR and apply server defaults", func() {
			mc := validMemcached(uniqueName("install-min"))
			Expect(k8sClient.Create(ctx, mc)).To(Succeed())

			fetched := &memcachedv1alpha1.Memcached{}
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(mc), fetched)).To(Succeed())

			// Verify server-applied defaults for top-level spec fields.
			Expect(fetched.Spec.Replicas).NotTo(BeNil())
			Expect(*fetched.Spec.Replicas).To(Equal(int32(1)))
			Expect(fetched.Spec.Image).NotTo(BeNil())
			Expect(*fetched.Spec.Image).To(Equal("memcached:1.6"))
		})
	})
})

// --- REQ-008: Full CRUD Lifecycle ---

var _ = Describe("CRD Installation: Full CRUD Lifecycle", func() {

	Context("create, read, update spec, update status, and delete", func() {
		It("should complete the full CRUD lifecycle", func() {
			// Create a valid CR.
			mc := validMemcached(uniqueName("crud"))
			mc.Spec.Replicas = int32Ptr(2)
			Expect(k8sClient.Create(ctx, mc)).To(Succeed())

			// Get it and verify it exists.
			fetched := &memcachedv1alpha1.Memcached{}
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(mc), fetched)).To(Succeed())
			Expect(*fetched.Spec.Replicas).To(Equal(int32(2)))

			// Update the spec (change replicas).
			fetched.Spec.Replicas = int32Ptr(4)
			Expect(k8sClient.Update(ctx, fetched)).To(Succeed())

			updated := &memcachedv1alpha1.Memcached{}
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(mc), updated)).To(Succeed())
			Expect(*updated.Spec.Replicas).To(Equal(int32(4)))

			// Update the status (set readyReplicas).
			updated.Status.ReadyReplicas = 3
			Expect(k8sClient.Status().Update(ctx, updated)).To(Succeed())

			statusUpdated := &memcachedv1alpha1.Memcached{}
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(mc), statusUpdated)).To(Succeed())
			Expect(statusUpdated.Status.ReadyReplicas).To(Equal(int32(3)))

			// Delete it and verify it's gone.
			Expect(k8sClient.Delete(ctx, statusUpdated)).To(Succeed())

			err := k8sClient.Get(ctx, client.ObjectKeyFromObject(mc), &memcachedv1alpha1.Memcached{})
			Expect(errors.IsNotFound(err)).To(BeTrue())
		})
	})
})

// --- REQ-007: Status Subresource Isolation ---

var _ = Describe("CRD Installation: Status Subresource Isolation", func() {

	Context("generation tracking across status and spec updates", func() {
		It("should not change generation on status update but increment on spec update", func() {
			mc := validMemcached(uniqueName("gen-iso"))
			mc.Spec.Replicas = int32Ptr(1)
			Expect(k8sClient.Create(ctx, mc)).To(Succeed())

			// Get the initial generation.
			fetched := &memcachedv1alpha1.Memcached{}
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(mc), fetched)).To(Succeed())
			genAfterCreate := fetched.Generation

			// Status update should NOT change generation.
			fetched.Status.ReadyReplicas = 1
			fetched.Status.Conditions = []metav1.Condition{
				{
					Type:               "Available",
					Status:             metav1.ConditionTrue,
					ObservedGeneration: genAfterCreate,
					LastTransitionTime: metav1.Now(),
					Reason:             "Ready",
					Message:            "All replicas ready",
				},
			}
			Expect(k8sClient.Status().Update(ctx, fetched)).To(Succeed())

			afterStatus := &memcachedv1alpha1.Memcached{}
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(mc), afterStatus)).To(Succeed())
			Expect(afterStatus.Generation).To(Equal(genAfterCreate))

			// Spec update should increment generation.
			afterStatus.Spec.Replicas = int32Ptr(3)
			Expect(k8sClient.Update(ctx, afterStatus)).To(Succeed())

			afterSpec := &memcachedv1alpha1.Memcached{}
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(mc), afterSpec)).To(Succeed())
			Expect(afterSpec.Generation).To(BeNumerically(">", genAfterCreate))
		})
	})
})

// --- REQ-006: Server-Applied Defaults ---

var _ = Describe("CRD Installation: Server-Applied Defaults", func() {

	Context("MemcachedConfig defaults on empty block", func() {
		It("should apply all MemcachedConfig defaults when submitting an empty memcached block", func() {
			mc := validMemcached(uniqueName("srv-def"))
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
})

// --- REQ-005: Validation Rejects Invalid CR ---

var _ = Describe("CRD Installation: Validation Rejects Invalid CR", func() {

	Context("replicas exceeding maximum", func() {
		It("should reject a CR with replicas=100", func() {
			mc := validMemcached(uniqueName("inv-rep"))
			mc.Spec.Replicas = int32Ptr(100)
			Expect(k8sClient.Create(ctx, mc)).NotTo(Succeed())
		})
	})

	Context("invalid maxItemSize pattern", func() {
		It("should reject a CR with maxItemSize='1g'", func() {
			mc := validMemcached(uniqueName("inv-item"))
			mc.Spec.Memcached = &memcachedv1alpha1.MemcachedConfig{
				MaxItemSize: "1g",
			}
			Expect(k8sClient.Create(ctx, mc)).NotTo(Succeed())
		})
	})

	Context("invalid antiAffinityPreset enum", func() {
		It("should reject a CR with antiAffinityPreset='invalid'", func() {
			mc := validMemcached(uniqueName("inv-aa"))
			invalid := memcachedv1alpha1.AntiAffinityPreset("invalid")
			mc.Spec.HighAvailability = &memcachedv1alpha1.HighAvailabilitySpec{
				AntiAffinityPreset: &invalid,
			}
			Expect(k8sClient.Create(ctx, mc)).NotTo(Succeed())
		})
	})
})
