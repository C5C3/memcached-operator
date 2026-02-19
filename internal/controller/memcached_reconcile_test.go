package controller_test

import (
	"context"
	"fmt"
	"sync/atomic"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	memcachedv1alpha1 "github.com/c5c3/memcached-operator/api/v1alpha1"
	"github.com/c5c3/memcached-operator/internal/controller"
)

var _ = Describe("Memcached Reconcile", func() {

	newReconciler := func() *controller.MemcachedReconciler {
		return &controller.MemcachedReconciler{
			Client: k8sClient,
			Scheme: scheme.Scheme,
		}
	}

	Context("when the Memcached CR does not exist", func() {
		It("should return empty result and no error", func() {
			r := newReconciler()
			result, err := r.Reconcile(ctx, ctrl.Request{
				NamespacedName: client.ObjectKey{
					Name:      "does-not-exist",
					Namespace: "default",
				},
			})

			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal(ctrl.Result{}))
		})
	})

	Context("when the Memcached CR exists", func() {
		It("should fetch the CR and return empty result with no error", func() {
			mc := validMemcached(uniqueName("reconcile"))
			Expect(k8sClient.Create(ctx, mc)).To(Succeed())

			r := newReconciler()
			result, err := r.Reconcile(ctx, ctrl.Request{
				NamespacedName: client.ObjectKeyFromObject(mc),
			})

			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal(ctrl.Result{}))
		})
	})

	Context("when client.Get returns a non-NotFound error", func() {
		It("should propagate the error", func() {
			getErr := fmt.Errorf("API server unavailable")
			fakeClient := fake.NewClientBuilder().WithScheme(scheme.Scheme).Build()
			failingClient := interceptor.NewClient(fakeClient, interceptor.Funcs{
				Get: func(_ context.Context, _ client.WithWatch, _ client.ObjectKey, _ client.Object, _ ...client.GetOption) error {
					return getErr
				},
			})

			r := &controller.MemcachedReconciler{
				Client: failingClient,
				Scheme: scheme.Scheme,
			}
			result, err := r.Reconcile(ctx, ctrl.Request{
				NamespacedName: client.ObjectKey{
					Name:      "any-name",
					Namespace: "default",
				},
			})

			Expect(err).To(MatchError(getErr))
			Expect(result).To(Equal(ctrl.Result{}))
		})
	})

	Context("when the Memcached CR is deleted before reconciliation", func() {
		It("should return empty result and no error after deletion", func() {
			mc := validMemcached(uniqueName("reconcile-del"))
			Expect(k8sClient.Create(ctx, mc)).To(Succeed())

			// Ensure the CR exists.
			fetched := &memcachedv1alpha1.Memcached{}
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(mc), fetched)).To(Succeed())

			// Delete the CR.
			Expect(k8sClient.Delete(ctx, mc)).To(Succeed())

			r := newReconciler()
			result, err := r.Reconcile(ctx, ctrl.Request{
				NamespacedName: client.ObjectKeyFromObject(mc),
			})

			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal(ctrl.Result{}))
		})
	})
})

// --- Task 3.1: Idempotent Create-or-Update Reconciliation (REQ-001, REQ-006) ---

var _ = Describe("Idempotent Create-or-Update Reconciliation", func() {

	Context("multiple reconciliation cycles without changes (REQ-001)", func() {
		It("should not update Deployment or Service resourceVersions after initial creation", func() {
			mc := validMemcached(uniqueName("idem-noop"))
			Expect(k8sClient.Create(ctx, mc)).To(Succeed())

			// First reconcile: creates Deployment and Service.
			_, err := reconcileOnce(mc)
			Expect(err).NotTo(HaveOccurred())

			dep1 := fetchDeployment(mc)
			svc1 := fetchService(mc)
			depRV1 := dep1.ResourceVersion
			svcRV1 := svc1.ResourceVersion

			// Second reconcile: no changes — should be a no-op.
			_, err = reconcileOnce(mc)
			Expect(err).NotTo(HaveOccurred())

			dep2 := fetchDeployment(mc)
			svc2 := fetchService(mc)
			Expect(dep2.ResourceVersion).To(Equal(depRV1))
			Expect(svc2.ResourceVersion).To(Equal(svcRV1))

			// Third reconcile: still no changes.
			_, err = reconcileOnce(mc)
			Expect(err).NotTo(HaveOccurred())

			dep3 := fetchDeployment(mc)
			svc3 := fetchService(mc)
			Expect(dep3.ResourceVersion).To(Equal(depRV1))
			Expect(svc3.ResourceVersion).To(Equal(svcRV1))
		})
	})

	Context("drift correction on manually patched Deployment (REQ-001)", func() {
		It("should restore Deployment replicas to CR spec after manual patch", func() {
			mc := validMemcached(uniqueName("idem-drift"))
			mc.Spec.Replicas = int32Ptr(2)
			Expect(k8sClient.Create(ctx, mc)).To(Succeed())

			_, err := reconcileOnce(mc)
			Expect(err).NotTo(HaveOccurred())

			dep := fetchDeployment(mc)
			Expect(*dep.Spec.Replicas).To(Equal(int32(2)))

			// Manually patch Deployment replicas to a different value.
			patch := client.MergeFrom(dep.DeepCopy())
			wrongReplicas := int32(10)
			dep.Spec.Replicas = &wrongReplicas
			Expect(k8sClient.Patch(ctx, dep, patch)).To(Succeed())

			// Verify the drift happened.
			drifted := fetchDeployment(mc)
			Expect(*drifted.Spec.Replicas).To(Equal(int32(10)))

			// Reconcile should correct the drift.
			_, err = reconcileOnce(mc)
			Expect(err).NotTo(HaveOccurred())

			corrected := fetchDeployment(mc)
			Expect(*corrected.Spec.Replicas).To(Equal(int32(2)))
		})
	})

	Context("spec update followed by idempotent reconciliation (REQ-001)", func() {
		It("should update Deployment then no-op on subsequent reconcile", func() {
			mc := validMemcached(uniqueName("idem-update"))
			mc.Spec.Replicas = int32Ptr(1)
			Expect(k8sClient.Create(ctx, mc)).To(Succeed())

			_, err := reconcileOnce(mc)
			Expect(err).NotTo(HaveOccurred())

			// Update CR spec.
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(mc), mc)).To(Succeed())
			mc.Spec.Replicas = int32Ptr(3)
			Expect(k8sClient.Update(ctx, mc)).To(Succeed())

			// Reconcile applies the update.
			_, err = reconcileOnce(mc)
			Expect(err).NotTo(HaveOccurred())

			dep := fetchDeployment(mc)
			Expect(*dep.Spec.Replicas).To(Equal(int32(3)))
			depRV := dep.ResourceVersion

			// Reconcile again without changes: no-op.
			_, err = reconcileOnce(mc)
			Expect(err).NotTo(HaveOccurred())

			dep2 := fetchDeployment(mc)
			Expect(dep2.ResourceVersion).To(Equal(depRV))
		})
	})

	Context("CR deleted between event delivery and reconciliation (REQ-006)", func() {
		It("should return no error when CR is deleted", func() {
			mc := validMemcached(uniqueName("idem-deleted"))
			Expect(k8sClient.Create(ctx, mc)).To(Succeed())

			_, err := reconcileOnce(mc)
			Expect(err).NotTo(HaveOccurred())

			// Delete the CR.
			Expect(k8sClient.Delete(ctx, mc)).To(Succeed())

			// Reconcile after deletion: should return successfully.
			r := &controller.MemcachedReconciler{
				Client: k8sClient,
				Scheme: scheme.Scheme,
			}
			result, err := r.Reconcile(ctx, ctrl.Request{
				NamespacedName: client.ObjectKeyFromObject(mc),
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal(ctrl.Result{}))
		})
	})
})

// --- Task 3.2: Level-Triggered Reconciliation Semantics (REQ-006) ---

var _ = Describe("Level-triggered reconciliation", func() {

	Context("only latest CR state is applied (REQ-006)", func() {
		It("should apply only the latest replicas value after multiple updates without reconciling", func() {
			mc := validMemcached(uniqueName("level-latest"))
			mc.Spec.Replicas = int32Ptr(1)
			Expect(k8sClient.Create(ctx, mc)).To(Succeed())

			// Initial reconcile.
			_, err := reconcileOnce(mc)
			Expect(err).NotTo(HaveOccurred())

			dep := fetchDeployment(mc)
			Expect(*dep.Spec.Replicas).To(Equal(int32(1)))

			// Multiple spec updates without intermediate reconciliation.
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(mc), mc)).To(Succeed())
			mc.Spec.Replicas = int32Ptr(3)
			Expect(k8sClient.Update(ctx, mc)).To(Succeed())

			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(mc), mc)).To(Succeed())
			mc.Spec.Replicas = int32Ptr(5)
			Expect(k8sClient.Update(ctx, mc)).To(Succeed())

			// Single reconcile should apply only the latest state (replicas=5).
			_, err = reconcileOnce(mc)
			Expect(err).NotTo(HaveOccurred())

			dep = fetchDeployment(mc)
			Expect(*dep.Spec.Replicas).To(Equal(int32(5)))
		})
	})

	Context("deleted owned Deployment is recreated (REQ-006)", func() {
		It("should recreate the Deployment when it is deleted externally", func() {
			mc := validMemcached(uniqueName("level-recreate"))
			Expect(k8sClient.Create(ctx, mc)).To(Succeed())

			_, err := reconcileOnce(mc)
			Expect(err).NotTo(HaveOccurred())

			// Verify Deployment exists.
			dep := fetchDeployment(mc)
			Expect(dep).NotTo(BeNil())

			// Delete the Deployment directly.
			Expect(k8sClient.Delete(ctx, dep)).To(Succeed())

			// Verify it is gone.
			gone := &appsv1.Deployment{}
			err = k8sClient.Get(ctx, client.ObjectKeyFromObject(mc), gone)
			Expect(apierrors.IsNotFound(err)).To(BeTrue())

			// Reconcile should recreate it.
			_, err = reconcileOnce(mc)
			Expect(err).NotTo(HaveOccurred())

			recreated := fetchDeployment(mc)
			Expect(recreated).NotTo(BeNil())
			Expect(recreated.OwnerReferences).To(HaveLen(1))
			Expect(recreated.OwnerReferences[0].Name).To(Equal(mc.Name))
		})
	})

	Context("deleted owned Service is recreated (REQ-006)", func() {
		It("should recreate the Service when it is deleted externally", func() {
			mc := validMemcached(uniqueName("level-svc-recreate"))
			Expect(k8sClient.Create(ctx, mc)).To(Succeed())

			_, err := reconcileOnce(mc)
			Expect(err).NotTo(HaveOccurred())

			// Verify Service exists.
			svc := fetchService(mc)
			Expect(svc).NotTo(BeNil())

			// Delete the Service directly.
			Expect(k8sClient.Delete(ctx, svc)).To(Succeed())

			// Verify it is gone.
			gone := &corev1.Service{}
			err = k8sClient.Get(ctx, client.ObjectKeyFromObject(mc), gone)
			Expect(apierrors.IsNotFound(err)).To(BeTrue())

			// Reconcile should recreate it.
			_, err = reconcileOnce(mc)
			Expect(err).NotTo(HaveOccurred())

			recreated := fetchService(mc)
			Expect(recreated).NotTo(BeNil())
			Expect(recreated.Spec.ClusterIP).To(Equal(corev1.ClusterIPNone))
		})
	})

	Context("builder functions are deterministic (REQ-006)", func() {
		It("should produce identical Deployment specs on repeated calls with the same input", func() {
			mc := validMemcached(uniqueName("level-determ-dep"))
			mc.Spec.Replicas = int32Ptr(2)
			mc.Spec.Image = strPtr("memcached:1.6.29")
			Expect(k8sClient.Create(ctx, mc)).To(Succeed())
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(mc), mc)).To(Succeed())

			// First reconcile creates the Deployment.
			_, err := reconcileOnce(mc)
			Expect(err).NotTo(HaveOccurred())

			dep1 := fetchDeployment(mc)
			spec1 := dep1.Spec

			// Second reconcile — Deployment should be unchanged.
			_, err = reconcileOnce(mc)
			Expect(err).NotTo(HaveOccurred())

			dep2 := fetchDeployment(mc)
			Expect(dep2.ResourceVersion).To(Equal(dep1.ResourceVersion),
				"Deployment should not be updated when builder output is deterministic")
			Expect(dep2.Spec.Template.Spec.Containers[0].Image).To(Equal(spec1.Template.Spec.Containers[0].Image))
			Expect(*dep2.Spec.Replicas).To(Equal(*spec1.Replicas))
		})

		It("should produce identical Service specs on repeated calls with the same input", func() {
			mc := validMemcached(uniqueName("level-determ-svc"))
			Expect(k8sClient.Create(ctx, mc)).To(Succeed())
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(mc), mc)).To(Succeed())

			// First reconcile creates the Service.
			_, err := reconcileOnce(mc)
			Expect(err).NotTo(HaveOccurred())

			svc1 := fetchService(mc)

			// Second reconcile — Service should be unchanged.
			_, err = reconcileOnce(mc)
			Expect(err).NotTo(HaveOccurred())

			svc2 := fetchService(mc)
			Expect(svc2.ResourceVersion).To(Equal(svc1.ResourceVersion),
				"Service should not be updated when builder output is deterministic")
		})
	})
})

// --- Task 3.3: Conflict Retry with envtest (REQ-002) ---

var _ = Describe("Conflict retry", func() {

	Context("transient conflict on Deployment update (REQ-002)", func() {
		It("should succeed despite a transient 409 Conflict error", func() {
			// Use a fake client to control conflict injection precisely.
			mc := validMemcached(uniqueName("conflict-transient"))
			mc.Spec.Replicas = int32Ptr(1)
			fakeClient := fake.NewClientBuilder().
				WithScheme(scheme.Scheme).
				WithStatusSubresource(&memcachedv1alpha1.Memcached{}).
				WithObjects(mc).
				Build()

			// Initial reconcile to create resources.
			r := &controller.MemcachedReconciler{
				Client: fakeClient,
				Scheme: scheme.Scheme,
			}
			_, err := r.Reconcile(ctx, ctrl.Request{
				NamespacedName: client.ObjectKeyFromObject(mc),
			})
			Expect(err).NotTo(HaveOccurred())

			// Update CR to trigger an update on next reconcile.
			Expect(fakeClient.Get(ctx, client.ObjectKeyFromObject(mc), mc)).To(Succeed())
			mc.Spec.Replicas = int32Ptr(4)
			Expect(fakeClient.Update(ctx, mc)).To(Succeed())

			// Wrap the fake client to inject a single conflict error on Update for Deployments.
			var updateCalls atomic.Int32
			conflictErr := apierrors.NewConflict(
				schema.GroupResource{Group: "apps", Resource: "deployments"},
				mc.Name,
				fmt.Errorf("the object has been modified"),
			)
			wrappedClient := interceptor.NewClient(fakeClient, interceptor.Funcs{
				Update: func(ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.UpdateOption) error {
					if _, ok := obj.(*appsv1.Deployment); ok {
						call := updateCalls.Add(1)
						if call == 1 {
							return conflictErr
						}
					}
					return c.Update(ctx, obj, opts...)
				},
			})

			r2 := &controller.MemcachedReconciler{
				Client: wrappedClient,
				Scheme: scheme.Scheme,
			}
			result, err := r2.Reconcile(ctx, ctrl.Request{
				NamespacedName: client.ObjectKeyFromObject(mc),
			})

			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal(ctrl.Result{}))

			// Verify the Deployment ended up with the correct state.
			dep := &appsv1.Deployment{}
			Expect(fakeClient.Get(ctx, client.ObjectKeyFromObject(mc), dep)).To(Succeed())
			Expect(*dep.Spec.Replicas).To(Equal(int32(4)))

			// Verify retry happened: at least 2 update calls.
			Expect(updateCalls.Load()).To(BeNumerically(">=", int32(2)))
		})
	})

	Context("persistent conflict exhausts retries (REQ-002)", func() {
		It("should return an error after exhausting all conflict retries", func() {
			// Use a fake client to control conflict injection precisely.
			mc := validMemcached(uniqueName("conflict-exhaust"))
			mc.Spec.Replicas = int32Ptr(1)
			fakeClient := fake.NewClientBuilder().
				WithScheme(scheme.Scheme).
				WithStatusSubresource(&memcachedv1alpha1.Memcached{}).
				WithObjects(mc).
				Build()

			// Initial reconcile to create resources.
			r := &controller.MemcachedReconciler{
				Client: fakeClient,
				Scheme: scheme.Scheme,
			}
			_, err := r.Reconcile(ctx, ctrl.Request{
				NamespacedName: client.ObjectKeyFromObject(mc),
			})
			Expect(err).NotTo(HaveOccurred())

			// Update CR to trigger an update.
			Expect(fakeClient.Get(ctx, client.ObjectKeyFromObject(mc), mc)).To(Succeed())
			mc.Spec.Replicas = int32Ptr(3)
			Expect(fakeClient.Update(ctx, mc)).To(Succeed())

			// Wrap the fake client to always return conflict on Deployment updates.
			conflictErr := apierrors.NewConflict(
				schema.GroupResource{Group: "apps", Resource: "deployments"},
				mc.Name,
				fmt.Errorf("the object has been modified"),
			)
			wrappedClient := interceptor.NewClient(fakeClient, interceptor.Funcs{
				Update: func(ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.UpdateOption) error {
					if _, ok := obj.(*appsv1.Deployment); ok {
						return conflictErr
					}
					return c.Update(ctx, obj, opts...)
				},
			})

			r2 := &controller.MemcachedReconciler{
				Client: wrappedClient,
				Scheme: scheme.Scheme,
			}
			_, err = r2.Reconcile(ctx, ctrl.Request{
				NamespacedName: client.ObjectKeyFromObject(mc),
			})

			Expect(err).To(HaveOccurred())
			Expect(apierrors.IsConflict(err)).To(BeTrue())
		})
	})
})
