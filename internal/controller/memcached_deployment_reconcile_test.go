package controller_test

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	memcachedv1beta1 "github.com/c5c3/memcached-operator/api/v1beta1"
	"github.com/c5c3/memcached-operator/internal/controller"
)

// reconcileOnce runs a single Reconcile cycle for the given Memcached CR.
func reconcileOnce(mc *memcachedv1beta1.Memcached) (ctrl.Result, error) {
	r := &controller.MemcachedReconciler{
		Client: k8sClient,
		Scheme: scheme.Scheme,
	}
	return r.Reconcile(ctx, ctrl.Request{
		NamespacedName: client.ObjectKeyFromObject(mc),
	})
}

// fetchStatefulSet retrieves the StatefulSet with the same name/namespace as the Memcached CR.
func fetchStatefulSet(mc *memcachedv1beta1.Memcached) *appsv1.StatefulSet {
	sts := &appsv1.StatefulSet{}
	ExpectWithOffset(1, k8sClient.Get(ctx, client.ObjectKeyFromObject(mc), sts)).To(Succeed())
	return sts
}

// zoneSpreadConstraint returns a standard zone-aware topology spread constraint used across tests.
func zoneSpreadConstraint() corev1.TopologySpreadConstraint {
	return corev1.TopologySpreadConstraint{
		MaxSkew:           1,
		TopologyKey:       "topology.kubernetes.io/zone",
		WhenUnsatisfiable: corev1.DoNotSchedule,
	}
}

// --- Task 4.1: StatefulSet creation from minimal CR with defaults ---

var _ = Describe("StatefulSet Reconciliation", func() {

	Context("minimal CR with defaults (REQ-002, REQ-004, REQ-005, REQ-007, REQ-008, REQ-009)", func() {
		var mc *memcachedv1beta1.Memcached

		BeforeEach(func() {
			mc = validMemcached(uniqueName("dep-minimal"))
			Expect(k8sClient.Create(ctx, mc)).To(Succeed())
			// Re-fetch to get server-applied defaults and resource version.
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(mc), mc)).To(Succeed())
			result, err := reconcileOnce(mc)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal(ctrl.Result{}))
		})

		It("should create a StatefulSet with 1 replica and default image", func() {
			sts := fetchStatefulSet(mc)
			Expect(*sts.Spec.Replicas).To(Equal(int32(1)))
			Expect(sts.Spec.Template.Spec.Containers[0].Image).To(Equal("memcached:1.6"))
		})

		It("should set default container args", func() {
			sts := fetchStatefulSet(mc)
			expectedArgs := []string{"-m", "64", "-c", "1024", "-t", "4", "-I", "1m"}
			Expect(sts.Spec.Template.Spec.Containers[0].Args).To(Equal(expectedArgs))
		})

		It("should expose port 11211/TCP named 'memcached'", func() {
			sts := fetchStatefulSet(mc)
			ports := sts.Spec.Template.Spec.Containers[0].Ports
			Expect(ports).To(HaveLen(1))
			Expect(ports[0].Name).To(Equal("memcached"))
			Expect(ports[0].ContainerPort).To(Equal(int32(11211)))
			Expect(ports[0].Protocol).To(Equal(corev1.ProtocolTCP))
		})

		It("should configure readiness probe", func() {
			sts := fetchStatefulSet(mc)
			rp := sts.Spec.Template.Spec.Containers[0].ReadinessProbe
			Expect(rp).NotTo(BeNil())
			Expect(rp.TCPSocket).NotTo(BeNil())
			Expect(rp.TCPSocket.Port).To(Equal(intstr.FromString("memcached")))
			Expect(rp.InitialDelaySeconds).To(Equal(int32(5)))
			Expect(rp.PeriodSeconds).To(Equal(int32(5)))
		})

		It("should configure liveness probe", func() {
			sts := fetchStatefulSet(mc)
			lp := sts.Spec.Template.Spec.Containers[0].LivenessProbe
			Expect(lp).NotTo(BeNil())
			Expect(lp.TCPSocket).NotTo(BeNil())
			Expect(lp.TCPSocket.Port).To(Equal(intstr.FromString("memcached")))
			Expect(lp.InitialDelaySeconds).To(Equal(int32(10)))
			Expect(lp.PeriodSeconds).To(Equal(int32(10)))
		})

		It("should use RollingUpdate update strategy, Parallel pod management, and the headless Service", func() {
			sts := fetchStatefulSet(mc)
			Expect(sts.Spec.UpdateStrategy.Type).To(Equal(appsv1.RollingUpdateStatefulSetStrategyType))
			Expect(sts.Spec.PodManagementPolicy).To(Equal(appsv1.ParallelPodManagement))
			Expect(sts.Spec.ServiceName).To(Equal(mc.Name))
			Expect(sts.Spec.VolumeClaimTemplates).To(BeEmpty())
		})

		It("should set standard labels on StatefulSet metadata", func() {
			sts := fetchStatefulSet(mc)
			Expect(sts.Labels).To(HaveKeyWithValue("app.kubernetes.io/name", "memcached"))
			Expect(sts.Labels).To(HaveKeyWithValue("app.kubernetes.io/instance", mc.Name))
			Expect(sts.Labels).To(HaveKeyWithValue("app.kubernetes.io/managed-by", "memcached-operator"))
		})

		It("should set matching labels on pod template and selector", func() {
			sts := fetchStatefulSet(mc)
			expectedLabels := map[string]string{
				"app.kubernetes.io/name":       "memcached",
				"app.kubernetes.io/instance":   mc.Name,
				"app.kubernetes.io/managed-by": "memcached-operator",
			}
			for k, v := range expectedLabels {
				Expect(sts.Spec.Template.Labels).To(HaveKeyWithValue(k, v))
				Expect(sts.Spec.Selector.MatchLabels).To(HaveKeyWithValue(k, v))
			}
		})

		It("should set owner reference pointing to the Memcached CR", func() {
			sts := fetchStatefulSet(mc)
			Expect(sts.OwnerReferences).To(HaveLen(1))
			ownerRef := sts.OwnerReferences[0]
			Expect(ownerRef.APIVersion).To(Equal("memcached.c5c3.io/v1beta1"))
			Expect(ownerRef.Kind).To(Equal("Memcached"))
			Expect(ownerRef.Name).To(Equal(mc.Name))
			Expect(ownerRef.UID).To(Equal(mc.UID))
			Expect(*ownerRef.Controller).To(BeTrue())
			Expect(*ownerRef.BlockOwnerDeletion).To(BeTrue())
		})

		It("should have no resource requests/limits when spec.resources is nil", func() {
			sts := fetchStatefulSet(mc)
			container := sts.Spec.Template.Spec.Containers[0]
			Expect(container.Resources.Requests).To(BeEmpty())
			Expect(container.Resources.Limits).To(BeEmpty())
		})
	})

	// --- Task 4.2: StatefulSet creation with full custom spec ---

	Context("full custom spec (REQ-001, REQ-002, REQ-003)", func() {
		It("should create a StatefulSet matching all custom spec fields", func() {
			mc := validMemcached(uniqueName("dep-custom"))
			mc.Spec.Replicas = int32Ptr(3)
			mc.Spec.Image = strPtr("memcached:1.6.29")
			mc.Spec.Resources = &corev1.ResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("100m"),
					corev1.ResourceMemory: resource.MustParse("128Mi"),
				},
				Limits: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("500m"),
					corev1.ResourceMemory: resource.MustParse("300Mi"),
				},
			}
			mc.Spec.Memcached = &memcachedv1beta1.MemcachedConfig{
				MaxMemoryMB:    256,
				MaxConnections: 2048,
				Threads:        8,
				MaxItemSize:    "2m",
				Verbosity:      2,
				ExtraArgs:      []string{"-o", "modern"},
			}
			Expect(k8sClient.Create(ctx, mc)).To(Succeed())

			result, err := reconcileOnce(mc)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal(ctrl.Result{}))

			sts := fetchStatefulSet(mc)

			// Replicas.
			Expect(*sts.Spec.Replicas).To(Equal(int32(3)))

			// Image.
			container := sts.Spec.Template.Spec.Containers[0]
			Expect(container.Image).To(Equal("memcached:1.6.29"))

			// Args with custom values, verbosity=2, and extraArgs.
			expectedArgs := []string{
				"-m", "256", "-c", "2048", "-t", "8", "-I", "2m",
				"-vv", "-o", "modern",
			}
			Expect(container.Args).To(Equal(expectedArgs))

			// Resources.
			Expect(container.Resources.Requests[corev1.ResourceCPU]).To(Equal(resource.MustParse("100m")))
			Expect(container.Resources.Requests[corev1.ResourceMemory]).To(Equal(resource.MustParse("128Mi")))
			Expect(container.Resources.Limits[corev1.ResourceCPU]).To(Equal(resource.MustParse("500m")))
			Expect(container.Resources.Limits[corev1.ResourceMemory]).To(Equal(resource.MustParse("300Mi")))
		})

		It("should create a StatefulSet with requests-only resources", func() {
			mc := validMemcached(uniqueName("dep-reqonly"))
			mc.Spec.Resources = &corev1.ResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("50m"),
					corev1.ResourceMemory: resource.MustParse("64Mi"),
				},
			}
			Expect(k8sClient.Create(ctx, mc)).To(Succeed())

			result, err := reconcileOnce(mc)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal(ctrl.Result{}))

			sts := fetchStatefulSet(mc)
			container := sts.Spec.Template.Spec.Containers[0]
			Expect(container.Resources.Requests[corev1.ResourceCPU]).To(Equal(resource.MustParse("50m")))
			Expect(container.Resources.Requests[corev1.ResourceMemory]).To(Equal(resource.MustParse("64Mi")))
			Expect(container.Resources.Limits).To(BeEmpty())
		})
	})

	// --- Task 4.3: StatefulSet update on CR spec change - drift detection ---

	Context("drift detection and update (REQ-006)", func() {
		It("should update StatefulSet replicas when CR spec.replicas changes", func() {
			mc := validMemcached(uniqueName("dep-drift-rep"))
			mc.Spec.Replicas = int32Ptr(1)
			Expect(k8sClient.Create(ctx, mc)).To(Succeed())

			_, err := reconcileOnce(mc)
			Expect(err).NotTo(HaveOccurred())

			sts := fetchStatefulSet(mc)
			Expect(*sts.Spec.Replicas).To(Equal(int32(1)))

			// Update CR replicas.
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(mc), mc)).To(Succeed())
			mc.Spec.Replicas = int32Ptr(3)
			Expect(k8sClient.Update(ctx, mc)).To(Succeed())

			_, err = reconcileOnce(mc)
			Expect(err).NotTo(HaveOccurred())

			sts = fetchStatefulSet(mc)
			Expect(*sts.Spec.Replicas).To(Equal(int32(3)))
		})

		It("should update StatefulSet image when CR spec.image changes", func() {
			mc := validMemcached(uniqueName("dep-drift-img"))
			mc.Spec.Image = strPtr("memcached:1.6")
			Expect(k8sClient.Create(ctx, mc)).To(Succeed())

			_, err := reconcileOnce(mc)
			Expect(err).NotTo(HaveOccurred())

			sts := fetchStatefulSet(mc)
			Expect(sts.Spec.Template.Spec.Containers[0].Image).To(Equal("memcached:1.6"))

			// Update CR image.
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(mc), mc)).To(Succeed())
			mc.Spec.Image = strPtr("memcached:1.6.29")
			Expect(k8sClient.Update(ctx, mc)).To(Succeed())

			_, err = reconcileOnce(mc)
			Expect(err).NotTo(HaveOccurred())

			sts = fetchStatefulSet(mc)
			Expect(sts.Spec.Template.Spec.Containers[0].Image).To(Equal("memcached:1.6.29"))
		})

		It("should update StatefulSet resources when CR spec.resources changes", func() {
			mc := validMemcached(uniqueName("dep-drift-res"))
			Expect(k8sClient.Create(ctx, mc)).To(Succeed())

			_, err := reconcileOnce(mc)
			Expect(err).NotTo(HaveOccurred())

			// Initially no resources.
			sts := fetchStatefulSet(mc)
			Expect(sts.Spec.Template.Spec.Containers[0].Resources.Requests).To(BeEmpty())

			// Add resources.
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(mc), mc)).To(Succeed())
			mc.Spec.Resources = &corev1.ResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceCPU: resource.MustParse("200m"),
				},
			}
			Expect(k8sClient.Update(ctx, mc)).To(Succeed())

			_, err = reconcileOnce(mc)
			Expect(err).NotTo(HaveOccurred())

			sts = fetchStatefulSet(mc)
			Expect(sts.Spec.Template.Spec.Containers[0].Resources.Requests[corev1.ResourceCPU]).To(Equal(resource.MustParse("200m")))
		})

		It("should update container args when CR spec.memcached changes", func() {
			mc := validMemcached(uniqueName("dep-drift-args"))
			Expect(k8sClient.Create(ctx, mc)).To(Succeed())

			_, err := reconcileOnce(mc)
			Expect(err).NotTo(HaveOccurred())

			// Update memcached config.
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(mc), mc)).To(Succeed())
			mc.Spec.Memcached = &memcachedv1beta1.MemcachedConfig{
				MaxMemoryMB: 512,
			}
			Expect(k8sClient.Update(ctx, mc)).To(Succeed())

			_, err = reconcileOnce(mc)
			Expect(err).NotTo(HaveOccurred())

			sts := fetchStatefulSet(mc)
			args := sts.Spec.Template.Spec.Containers[0].Args
			// MaxMemoryMB=512 should appear; other values get defaults from CRD defaults.
			Expect(args[0]).To(Equal("-m"))
			Expect(args[1]).To(Equal("512"))
		})

		It("should be idempotent when reconciling without changes", func() {
			mc := validMemcached(uniqueName("dep-idempotent"))
			Expect(k8sClient.Create(ctx, mc)).To(Succeed())

			_, err := reconcileOnce(mc)
			Expect(err).NotTo(HaveOccurred())

			sts1 := fetchStatefulSet(mc)
			rv1 := sts1.ResourceVersion

			// Reconcile again without any changes.
			_, err = reconcileOnce(mc)
			Expect(err).NotTo(HaveOccurred())

			sts2 := fetchStatefulSet(mc)
			Expect(sts2.ResourceVersion).To(Equal(rv1))
		})
	})

	// --- Task 4.4: Owner reference and error handling edge cases ---

	Context("owner reference details (REQ-005)", func() {
		It("should set exactly one owner reference with correct fields", func() {
			mc := validMemcached(uniqueName("dep-ownerref"))
			Expect(k8sClient.Create(ctx, mc)).To(Succeed())
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(mc), mc)).To(Succeed())

			_, err := reconcileOnce(mc)
			Expect(err).NotTo(HaveOccurred())

			sts := fetchStatefulSet(mc)
			Expect(sts.OwnerReferences).To(HaveLen(1))

			ref := sts.OwnerReferences[0]
			Expect(ref.APIVersion).To(Equal("memcached.c5c3.io/v1beta1"))
			Expect(ref.Kind).To(Equal("Memcached"))
			Expect(ref.Name).To(Equal(mc.Name))
			Expect(ref.UID).To(Equal(mc.UID))
			Expect(*ref.Controller).To(BeTrue())
			Expect(*ref.BlockOwnerDeletion).To(BeTrue())
		})

		It("should preserve owner reference after update", func() {
			mc := validMemcached(uniqueName("dep-ownerref-upd"))
			Expect(k8sClient.Create(ctx, mc)).To(Succeed())
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(mc), mc)).To(Succeed())

			_, err := reconcileOnce(mc)
			Expect(err).NotTo(HaveOccurred())

			// Update CR and reconcile again.
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(mc), mc)).To(Succeed())
			mc.Spec.Replicas = int32Ptr(2)
			Expect(k8sClient.Update(ctx, mc)).To(Succeed())

			_, err = reconcileOnce(mc)
			Expect(err).NotTo(HaveOccurred())

			sts := fetchStatefulSet(mc)
			Expect(sts.OwnerReferences).To(HaveLen(1))
			Expect(sts.OwnerReferences[0].Name).To(Equal(mc.Name))
			Expect(*sts.OwnerReferences[0].Controller).To(BeTrue())
		})
	})

	Context("zero replicas (REQ-002)", func() {
		It("should create a StatefulSet with 0 replicas", func() {
			mc := validMemcached(uniqueName("dep-zero-rep"))
			mc.Spec.Replicas = int32Ptr(0)
			Expect(k8sClient.Create(ctx, mc)).To(Succeed())

			_, err := reconcileOnce(mc)
			Expect(err).NotTo(HaveOccurred())

			sts := fetchStatefulSet(mc)
			Expect(*sts.Spec.Replicas).To(Equal(int32(0)))
		})
	})

	Context("error handling (REQ-010)", func() {
		It("should propagate API errors from StatefulSet create/update", func() {
			apiErr := fmt.Errorf("simulated API server error")
			fakeClient := fake.NewClientBuilder().WithScheme(scheme.Scheme).Build()
			failingClient := interceptor.NewClient(fakeClient, interceptor.Funcs{
				Get: func(ctx context.Context, c client.WithWatch, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
					// Allow Get for Memcached CR but fail for StatefulSet.
					if _, ok := obj.(*memcachedv1beta1.Memcached); ok {
						return c.Get(ctx, key, obj, opts...)
					}
					return apiErr
				},
			})

			// Create a Memcached CR in the fake client.
			mc := validMemcached(uniqueName("dep-err"))
			Expect(fakeClient.Create(ctx, mc)).To(Succeed())

			r := &controller.MemcachedReconciler{
				Client: failingClient,
				Scheme: scheme.Scheme,
			}
			_, err := r.Reconcile(ctx, ctrl.Request{
				NamespacedName: client.ObjectKeyFromObject(mc),
			})
			Expect(err).To(HaveOccurred())
		})
	})

	// --- Task 4.5: extraArgs, verbosity, and nil memcached config edge cases ---

	Context("nil spec.memcached config (REQ-001)", func() {
		It("should use default args when spec.memcached is nil", func() {
			mc := validMemcached(uniqueName("dep-nilmc"))
			// spec.memcached is nil by default from validMemcached.
			Expect(k8sClient.Create(ctx, mc)).To(Succeed())

			_, err := reconcileOnce(mc)
			Expect(err).NotTo(HaveOccurred())

			sts := fetchStatefulSet(mc)
			expectedArgs := []string{"-m", "64", "-c", "1024", "-t", "4", "-I", "1m"}
			Expect(sts.Spec.Template.Spec.Containers[0].Args).To(Equal(expectedArgs))
		})
	})

	Context("verbosity levels (REQ-001)", func() {
		It("should include -v for verbosity=1", func() {
			mc := validMemcached(uniqueName("dep-verb1"))
			mc.Spec.Memcached = &memcachedv1beta1.MemcachedConfig{
				Verbosity: 1,
			}
			Expect(k8sClient.Create(ctx, mc)).To(Succeed())

			_, err := reconcileOnce(mc)
			Expect(err).NotTo(HaveOccurred())

			sts := fetchStatefulSet(mc)
			args := sts.Spec.Template.Spec.Containers[0].Args
			Expect(args).To(ContainElement("-v"))
			Expect(args).NotTo(ContainElement("-vv"))
		})

		It("should include -vv for verbosity=2", func() {
			mc := validMemcached(uniqueName("dep-verb2"))
			mc.Spec.Memcached = &memcachedv1beta1.MemcachedConfig{
				Verbosity: 2,
			}
			Expect(k8sClient.Create(ctx, mc)).To(Succeed())

			_, err := reconcileOnce(mc)
			Expect(err).NotTo(HaveOccurred())

			sts := fetchStatefulSet(mc)
			args := sts.Spec.Template.Spec.Containers[0].Args
			Expect(args).To(ContainElement("-vv"))
			Expect(args).NotTo(ContainElement("-v"))
		})

		It("should not include verbosity flag for verbosity=0", func() {
			mc := validMemcached(uniqueName("dep-verb0"))
			mc.Spec.Memcached = &memcachedv1beta1.MemcachedConfig{
				Verbosity: 0,
			}
			Expect(k8sClient.Create(ctx, mc)).To(Succeed())

			_, err := reconcileOnce(mc)
			Expect(err).NotTo(HaveOccurred())

			sts := fetchStatefulSet(mc)
			args := sts.Spec.Template.Spec.Containers[0].Args
			Expect(args).NotTo(ContainElement("-v"))
			Expect(args).NotTo(ContainElement("-vv"))
		})
	})

	// --- Task 1.1: Pod anti-affinity presets ---

	Context("pod anti-affinity presets (REQ-001, REQ-002, REQ-003, REQ-004, REQ-005)", func() {
		It("should set preferredDuringScheduling anti-affinity when preset is soft", func() {
			mc := validMemcached(uniqueName("dep-aa-soft"))
			soft := memcachedv1beta1.AntiAffinityPresetSoft
			mc.Spec.HighAvailability = &memcachedv1beta1.HighAvailabilitySpec{
				AntiAffinityPreset: &soft,
			}
			Expect(k8sClient.Create(ctx, mc)).To(Succeed())

			_, err := reconcileOnce(mc)
			Expect(err).NotTo(HaveOccurred())

			sts := fetchStatefulSet(mc)
			Expect(sts.Spec.Template.Spec.Affinity).NotTo(BeNil())
			Expect(sts.Spec.Template.Spec.Affinity.PodAntiAffinity).NotTo(BeNil())

			preferred := sts.Spec.Template.Spec.Affinity.PodAntiAffinity.PreferredDuringSchedulingIgnoredDuringExecution
			Expect(preferred).To(HaveLen(1))
			Expect(preferred[0].Weight).To(Equal(int32(100)))
			Expect(preferred[0].PodAffinityTerm.TopologyKey).To(Equal("kubernetes.io/hostname"))
			Expect(preferred[0].PodAffinityTerm.LabelSelector.MatchLabels).To(HaveKeyWithValue("app.kubernetes.io/name", "memcached"))
			Expect(preferred[0].PodAffinityTerm.LabelSelector.MatchLabels).To(HaveKeyWithValue("app.kubernetes.io/instance", mc.Name))
		})

		It("should set requiredDuringScheduling anti-affinity when preset is hard", func() {
			mc := validMemcached(uniqueName("dep-aa-hard"))
			hard := memcachedv1beta1.AntiAffinityPresetHard
			mc.Spec.HighAvailability = &memcachedv1beta1.HighAvailabilitySpec{
				AntiAffinityPreset: &hard,
			}
			Expect(k8sClient.Create(ctx, mc)).To(Succeed())

			_, err := reconcileOnce(mc)
			Expect(err).NotTo(HaveOccurred())

			sts := fetchStatefulSet(mc)
			Expect(sts.Spec.Template.Spec.Affinity).NotTo(BeNil())
			Expect(sts.Spec.Template.Spec.Affinity.PodAntiAffinity).NotTo(BeNil())

			required := sts.Spec.Template.Spec.Affinity.PodAntiAffinity.RequiredDuringSchedulingIgnoredDuringExecution
			Expect(required).To(HaveLen(1))
			Expect(required[0].TopologyKey).To(Equal("kubernetes.io/hostname"))
			Expect(required[0].LabelSelector.MatchLabels).To(HaveKeyWithValue("app.kubernetes.io/name", "memcached"))
			Expect(required[0].LabelSelector.MatchLabels).To(HaveKeyWithValue("app.kubernetes.io/instance", mc.Name))
		})

		It("should have no affinity when highAvailability is nil", func() {
			mc := validMemcached(uniqueName("dep-aa-nil"))
			Expect(k8sClient.Create(ctx, mc)).To(Succeed())

			_, err := reconcileOnce(mc)
			Expect(err).NotTo(HaveOccurred())

			sts := fetchStatefulSet(mc)
			Expect(sts.Spec.Template.Spec.Affinity).To(BeNil())
		})

		It("should update StatefulSet affinity when antiAffinityPreset changes from soft to hard", func() {
			mc := validMemcached(uniqueName("dep-aa-change"))
			soft := memcachedv1beta1.AntiAffinityPresetSoft
			mc.Spec.HighAvailability = &memcachedv1beta1.HighAvailabilitySpec{
				AntiAffinityPreset: &soft,
			}
			Expect(k8sClient.Create(ctx, mc)).To(Succeed())

			_, err := reconcileOnce(mc)
			Expect(err).NotTo(HaveOccurred())

			sts := fetchStatefulSet(mc)
			Expect(sts.Spec.Template.Spec.Affinity.PodAntiAffinity.PreferredDuringSchedulingIgnoredDuringExecution).To(HaveLen(1))

			// Update to hard.
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(mc), mc)).To(Succeed())
			hard := memcachedv1beta1.AntiAffinityPresetHard
			mc.Spec.HighAvailability.AntiAffinityPreset = &hard
			Expect(k8sClient.Update(ctx, mc)).To(Succeed())

			_, err = reconcileOnce(mc)
			Expect(err).NotTo(HaveOccurred())

			sts = fetchStatefulSet(mc)
			Expect(sts.Spec.Template.Spec.Affinity.PodAntiAffinity.RequiredDuringSchedulingIgnoredDuringExecution).To(HaveLen(1))
			Expect(sts.Spec.Template.Spec.Affinity.PodAntiAffinity.PreferredDuringSchedulingIgnoredDuringExecution).To(BeEmpty())
		})

		It("should clear affinity when highAvailability is removed", func() {
			mc := validMemcached(uniqueName("dep-aa-clear"))
			soft := memcachedv1beta1.AntiAffinityPresetSoft
			mc.Spec.HighAvailability = &memcachedv1beta1.HighAvailabilitySpec{
				AntiAffinityPreset: &soft,
			}
			Expect(k8sClient.Create(ctx, mc)).To(Succeed())

			_, err := reconcileOnce(mc)
			Expect(err).NotTo(HaveOccurred())

			sts := fetchStatefulSet(mc)
			Expect(sts.Spec.Template.Spec.Affinity).NotTo(BeNil())

			// Remove HA.
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(mc), mc)).To(Succeed())
			mc.Spec.HighAvailability = nil
			Expect(k8sClient.Update(ctx, mc)).To(Succeed())

			_, err = reconcileOnce(mc)
			Expect(err).NotTo(HaveOccurred())

			sts = fetchStatefulSet(mc)
			Expect(sts.Spec.Template.Spec.Affinity).To(BeNil())
		})

		It("should be idempotent with soft anti-affinity", func() {
			mc := validMemcached(uniqueName("dep-aa-idemp"))
			soft := memcachedv1beta1.AntiAffinityPresetSoft
			mc.Spec.HighAvailability = &memcachedv1beta1.HighAvailabilitySpec{
				AntiAffinityPreset: &soft,
			}
			Expect(k8sClient.Create(ctx, mc)).To(Succeed())

			_, err := reconcileOnce(mc)
			Expect(err).NotTo(HaveOccurred())

			sts1 := fetchStatefulSet(mc)
			rv1 := sts1.ResourceVersion

			// Reconcile again without changes.
			_, err = reconcileOnce(mc)
			Expect(err).NotTo(HaveOccurred())

			sts2 := fetchStatefulSet(mc)
			Expect(sts2.ResourceVersion).To(Equal(rv1))
		})
	})

	// --- Task 2.1: Topology spread constraints ---

	Context("topology spread constraints (REQ-001, REQ-002, REQ-003, REQ-004, REQ-005)", func() {
		It("should create StatefulSet with zone-aware topology spread constraint", func() {
			mc := validMemcached(uniqueName("dep-tsc-zone"))
			constraint := zoneSpreadConstraint()
			constraint.LabelSelector = &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app.kubernetes.io/name": "memcached",
				},
			}
			mc.Spec.HighAvailability = &memcachedv1beta1.HighAvailabilitySpec{
				TopologySpreadConstraints: []corev1.TopologySpreadConstraint{constraint},
			}
			Expect(k8sClient.Create(ctx, mc)).To(Succeed())

			_, err := reconcileOnce(mc)
			Expect(err).NotTo(HaveOccurred())

			sts := fetchStatefulSet(mc)
			tsc := sts.Spec.Template.Spec.TopologySpreadConstraints
			Expect(tsc).To(HaveLen(1))
			Expect(tsc[0].MaxSkew).To(Equal(int32(1)))
			Expect(tsc[0].TopologyKey).To(Equal("topology.kubernetes.io/zone"))
			Expect(tsc[0].WhenUnsatisfiable).To(Equal(corev1.DoNotSchedule))
			Expect(tsc[0].LabelSelector).NotTo(BeNil())
			Expect(tsc[0].LabelSelector.MatchLabels).To(HaveKeyWithValue("app.kubernetes.io/name", "memcached"))
		})

		It("should preserve multiple constraints in order", func() {
			mc := validMemcached(uniqueName("dep-tsc-multi"))
			mc.Spec.HighAvailability = &memcachedv1beta1.HighAvailabilitySpec{
				TopologySpreadConstraints: []corev1.TopologySpreadConstraint{
					zoneSpreadConstraint(),
					{
						MaxSkew:           2,
						TopologyKey:       "kubernetes.io/hostname",
						WhenUnsatisfiable: corev1.ScheduleAnyway,
					},
				},
			}
			Expect(k8sClient.Create(ctx, mc)).To(Succeed())

			_, err := reconcileOnce(mc)
			Expect(err).NotTo(HaveOccurred())

			sts := fetchStatefulSet(mc)
			tsc := sts.Spec.Template.Spec.TopologySpreadConstraints
			Expect(tsc).To(HaveLen(2))
			Expect(tsc[0].TopologyKey).To(Equal("topology.kubernetes.io/zone"))
			Expect(tsc[0].MaxSkew).To(Equal(int32(1)))
			Expect(tsc[0].WhenUnsatisfiable).To(Equal(corev1.DoNotSchedule))
			Expect(tsc[1].TopologyKey).To(Equal("kubernetes.io/hostname"))
			Expect(tsc[1].MaxSkew).To(Equal(int32(2)))
			Expect(tsc[1].WhenUnsatisfiable).To(Equal(corev1.ScheduleAnyway))
		})

		It("should update StatefulSet when topology spread constraints change", func() {
			mc := validMemcached(uniqueName("dep-tsc-upd"))
			mc.Spec.HighAvailability = &memcachedv1beta1.HighAvailabilitySpec{
				TopologySpreadConstraints: []corev1.TopologySpreadConstraint{zoneSpreadConstraint()},
			}
			Expect(k8sClient.Create(ctx, mc)).To(Succeed())

			_, err := reconcileOnce(mc)
			Expect(err).NotTo(HaveOccurred())

			sts := fetchStatefulSet(mc)
			Expect(sts.Spec.Template.Spec.TopologySpreadConstraints[0].MaxSkew).To(Equal(int32(1)))

			// Update maxSkew to 2.
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(mc), mc)).To(Succeed())
			mc.Spec.HighAvailability.TopologySpreadConstraints[0].MaxSkew = 2
			Expect(k8sClient.Update(ctx, mc)).To(Succeed())

			_, err = reconcileOnce(mc)
			Expect(err).NotTo(HaveOccurred())

			sts = fetchStatefulSet(mc)
			Expect(sts.Spec.Template.Spec.TopologySpreadConstraints[0].MaxSkew).To(Equal(int32(2)))
		})

		It("should clear topology spread constraints when removed from CR", func() {
			mc := validMemcached(uniqueName("dep-tsc-clear"))
			mc.Spec.HighAvailability = &memcachedv1beta1.HighAvailabilitySpec{
				TopologySpreadConstraints: []corev1.TopologySpreadConstraint{zoneSpreadConstraint()},
			}
			Expect(k8sClient.Create(ctx, mc)).To(Succeed())

			_, err := reconcileOnce(mc)
			Expect(err).NotTo(HaveOccurred())

			sts := fetchStatefulSet(mc)
			Expect(sts.Spec.Template.Spec.TopologySpreadConstraints).To(HaveLen(1))

			// Remove topology spread constraints.
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(mc), mc)).To(Succeed())
			mc.Spec.HighAvailability = nil
			Expect(k8sClient.Update(ctx, mc)).To(Succeed())

			_, err = reconcileOnce(mc)
			Expect(err).NotTo(HaveOccurred())

			sts = fetchStatefulSet(mc)
			Expect(sts.Spec.Template.Spec.TopologySpreadConstraints).To(BeEmpty())
		})

		It("should clear only topologySpreadConstraints when field is removed from HA section", func() {
			mc := validMemcached(uniqueName("dep-tsc-field"))
			soft := memcachedv1beta1.AntiAffinityPresetSoft
			mc.Spec.HighAvailability = &memcachedv1beta1.HighAvailabilitySpec{
				AntiAffinityPreset:        &soft,
				TopologySpreadConstraints: []corev1.TopologySpreadConstraint{zoneSpreadConstraint()},
			}
			Expect(k8sClient.Create(ctx, mc)).To(Succeed())

			_, err := reconcileOnce(mc)
			Expect(err).NotTo(HaveOccurred())

			sts := fetchStatefulSet(mc)
			Expect(sts.Spec.Template.Spec.TopologySpreadConstraints).To(HaveLen(1))
			Expect(sts.Spec.Template.Spec.Affinity).NotTo(BeNil())

			// Remove only topologySpreadConstraints, keep HA section with antiAffinityPreset.
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(mc), mc)).To(Succeed())
			mc.Spec.HighAvailability.TopologySpreadConstraints = nil
			Expect(k8sClient.Update(ctx, mc)).To(Succeed())

			_, err = reconcileOnce(mc)
			Expect(err).NotTo(HaveOccurred())

			sts = fetchStatefulSet(mc)
			Expect(sts.Spec.Template.Spec.TopologySpreadConstraints).To(BeEmpty())
			// AntiAffinity should still be present.
			Expect(sts.Spec.Template.Spec.Affinity).NotTo(BeNil())
			Expect(sts.Spec.Template.Spec.Affinity.PodAntiAffinity).NotTo(BeNil())
		})

		It("should be idempotent with topology spread constraints", func() {
			mc := validMemcached(uniqueName("dep-tsc-idemp"))
			mc.Spec.HighAvailability = &memcachedv1beta1.HighAvailabilitySpec{
				TopologySpreadConstraints: []corev1.TopologySpreadConstraint{zoneSpreadConstraint()},
			}
			Expect(k8sClient.Create(ctx, mc)).To(Succeed())

			_, err := reconcileOnce(mc)
			Expect(err).NotTo(HaveOccurred())

			sts1 := fetchStatefulSet(mc)
			rv1 := sts1.ResourceVersion

			// Reconcile again without changes.
			_, err = reconcileOnce(mc)
			Expect(err).NotTo(HaveOccurred())

			sts2 := fetchStatefulSet(mc)
			Expect(sts2.ResourceVersion).To(Equal(rv1))
		})

		It("should support both antiAffinityPreset and topologySpreadConstraints", func() {
			mc := validMemcached(uniqueName("dep-tsc-both"))
			soft := memcachedv1beta1.AntiAffinityPresetSoft
			mc.Spec.HighAvailability = &memcachedv1beta1.HighAvailabilitySpec{
				AntiAffinityPreset:        &soft,
				TopologySpreadConstraints: []corev1.TopologySpreadConstraint{zoneSpreadConstraint()},
			}
			Expect(k8sClient.Create(ctx, mc)).To(Succeed())

			_, err := reconcileOnce(mc)
			Expect(err).NotTo(HaveOccurred())

			sts := fetchStatefulSet(mc)

			// Affinity from antiAffinityPreset.
			Expect(sts.Spec.Template.Spec.Affinity).NotTo(BeNil())
			Expect(sts.Spec.Template.Spec.Affinity.PodAntiAffinity).NotTo(BeNil())
			Expect(sts.Spec.Template.Spec.Affinity.PodAntiAffinity.PreferredDuringSchedulingIgnoredDuringExecution).To(HaveLen(1))

			// TopologySpreadConstraints.
			tsc := sts.Spec.Template.Spec.TopologySpreadConstraints
			Expect(tsc).To(HaveLen(1))
			Expect(tsc[0].TopologyKey).To(Equal("topology.kubernetes.io/zone"))
		})

		It("should keep topologySpreadConstraints when antiAffinityPreset is removed", func() {
			mc := validMemcached(uniqueName("dep-tsc-keepc"))
			soft := memcachedv1beta1.AntiAffinityPresetSoft
			mc.Spec.HighAvailability = &memcachedv1beta1.HighAvailabilitySpec{
				AntiAffinityPreset:        &soft,
				TopologySpreadConstraints: []corev1.TopologySpreadConstraint{zoneSpreadConstraint()},
			}
			Expect(k8sClient.Create(ctx, mc)).To(Succeed())

			_, err := reconcileOnce(mc)
			Expect(err).NotTo(HaveOccurred())

			sts := fetchStatefulSet(mc)
			Expect(sts.Spec.Template.Spec.Affinity).NotTo(BeNil())
			Expect(sts.Spec.Template.Spec.TopologySpreadConstraints).To(HaveLen(1))

			// Remove antiAffinityPreset, keep topologySpreadConstraints.
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(mc), mc)).To(Succeed())
			mc.Spec.HighAvailability.AntiAffinityPreset = nil
			Expect(k8sClient.Update(ctx, mc)).To(Succeed())

			_, err = reconcileOnce(mc)
			Expect(err).NotTo(HaveOccurred())

			sts = fetchStatefulSet(mc)

			// TopologySpreadConstraints should remain.
			tsc := sts.Spec.Template.Spec.TopologySpreadConstraints
			Expect(tsc).To(HaveLen(1))
			Expect(tsc[0].TopologyKey).To(Equal("topology.kubernetes.io/zone"))
		})

		It("should keep antiAffinityPreset when topologySpreadConstraints are removed", func() {
			mc := validMemcached(uniqueName("dep-tsc-keep"))
			hard := memcachedv1beta1.AntiAffinityPresetHard
			mc.Spec.HighAvailability = &memcachedv1beta1.HighAvailabilitySpec{
				AntiAffinityPreset:        &hard,
				TopologySpreadConstraints: []corev1.TopologySpreadConstraint{zoneSpreadConstraint()},
			}
			Expect(k8sClient.Create(ctx, mc)).To(Succeed())

			_, err := reconcileOnce(mc)
			Expect(err).NotTo(HaveOccurred())

			sts := fetchStatefulSet(mc)
			// Both should be set initially.
			Expect(sts.Spec.Template.Spec.Affinity).NotTo(BeNil())
			Expect(sts.Spec.Template.Spec.TopologySpreadConstraints).To(HaveLen(1))

			// Remove topologySpreadConstraints, keep antiAffinityPreset.
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(mc), mc)).To(Succeed())
			mc.Spec.HighAvailability.TopologySpreadConstraints = nil
			Expect(k8sClient.Update(ctx, mc)).To(Succeed())

			_, err = reconcileOnce(mc)
			Expect(err).NotTo(HaveOccurred())

			sts = fetchStatefulSet(mc)

			// Anti-affinity should remain (hard preset).
			Expect(sts.Spec.Template.Spec.Affinity).NotTo(BeNil())
			Expect(sts.Spec.Template.Spec.Affinity.PodAntiAffinity).NotTo(BeNil())
			Expect(sts.Spec.Template.Spec.Affinity.PodAntiAffinity.RequiredDuringSchedulingIgnoredDuringExecution).To(HaveLen(1))

			// TopologySpreadConstraints should be cleared.
			Expect(sts.Spec.Template.Spec.TopologySpreadConstraints).To(BeEmpty())
		})
	})

	Context("extraArgs edge cases (REQ-001)", func() {
		It("should append extraArgs after standard flags", func() {
			mc := validMemcached(uniqueName("dep-extra"))
			mc.Spec.Memcached = &memcachedv1beta1.MemcachedConfig{
				ExtraArgs: []string{"--extended", "ext_item_size=2M"},
			}
			Expect(k8sClient.Create(ctx, mc)).To(Succeed())

			_, err := reconcileOnce(mc)
			Expect(err).NotTo(HaveOccurred())

			sts := fetchStatefulSet(mc)
			args := sts.Spec.Template.Spec.Containers[0].Args
			// Last two args should be the extra args.
			Expect(args[len(args)-2]).To(Equal("--extended"))
			Expect(args[len(args)-1]).To(Equal("ext_item_size=2M"))
		})

		It("should not append extra args when extraArgs is empty", func() {
			mc := validMemcached(uniqueName("dep-noextra"))
			mc.Spec.Memcached = &memcachedv1beta1.MemcachedConfig{
				ExtraArgs: []string{},
			}
			Expect(k8sClient.Create(ctx, mc)).To(Succeed())

			_, err := reconcileOnce(mc)
			Expect(err).NotTo(HaveOccurred())

			sts := fetchStatefulSet(mc)
			args := sts.Spec.Template.Spec.Containers[0].Args
			// Should have exactly the standard args (with CRD defaults).
			Expect(args).To(HaveLen(8)) // -m 64 -c 1024 -t 4 -I 1m
		})
	})

	// --- Task 1.1: Graceful shutdown ---

	Context("graceful shutdown (REQ-001, REQ-002, REQ-003, REQ-004, REQ-005)", func() {
		It("should create StatefulSet with preStop hook and terminationGracePeriodSeconds when graceful shutdown is enabled", func() {
			mc := validMemcached(uniqueName("dep-gs-on"))
			mc.Spec.HighAvailability = &memcachedv1beta1.HighAvailabilitySpec{
				GracefulShutdown: &memcachedv1beta1.GracefulShutdownSpec{
					Enabled:                       true,
					PreStopDelaySeconds:           10,
					TerminationGracePeriodSeconds: 30,
				},
			}
			Expect(k8sClient.Create(ctx, mc)).To(Succeed())

			_, err := reconcileOnce(mc)
			Expect(err).NotTo(HaveOccurred())

			sts := fetchStatefulSet(mc)
			container := sts.Spec.Template.Spec.Containers[0]
			Expect(container.Lifecycle).NotTo(BeNil())
			Expect(container.Lifecycle.PreStop).NotTo(BeNil())
			Expect(container.Lifecycle.PreStop.Exec).NotTo(BeNil())
			Expect(container.Lifecycle.PreStop.Exec.Command).To(Equal([]string{"sleep", "10"}))

			Expect(sts.Spec.Template.Spec.TerminationGracePeriodSeconds).NotTo(BeNil())
			Expect(*sts.Spec.Template.Spec.TerminationGracePeriodSeconds).To(Equal(int64(30)))
		})

		It("should create StatefulSet with custom graceful shutdown values from creation", func() {
			mc := validMemcached(uniqueName("dep-gs-cust"))
			mc.Spec.HighAvailability = &memcachedv1beta1.HighAvailabilitySpec{
				GracefulShutdown: &memcachedv1beta1.GracefulShutdownSpec{
					Enabled:                       true,
					PreStopDelaySeconds:           15,
					TerminationGracePeriodSeconds: 45,
				},
			}
			Expect(k8sClient.Create(ctx, mc)).To(Succeed())

			_, err := reconcileOnce(mc)
			Expect(err).NotTo(HaveOccurred())

			sts := fetchStatefulSet(mc)
			container := sts.Spec.Template.Spec.Containers[0]
			Expect(container.Lifecycle).NotTo(BeNil())
			Expect(container.Lifecycle.PreStop).NotTo(BeNil())
			Expect(container.Lifecycle.PreStop.Exec).NotTo(BeNil())
			Expect(container.Lifecycle.PreStop.Exec.Command).To(Equal([]string{"sleep", "15"}))

			Expect(sts.Spec.Template.Spec.TerminationGracePeriodSeconds).NotTo(BeNil())
			Expect(*sts.Spec.Template.Spec.TerminationGracePeriodSeconds).To(Equal(int64(45)))
		})

		It("should update StatefulSet when preStopDelaySeconds changes", func() {
			mc := validMemcached(uniqueName("dep-gs-upd"))
			mc.Spec.HighAvailability = &memcachedv1beta1.HighAvailabilitySpec{
				GracefulShutdown: &memcachedv1beta1.GracefulShutdownSpec{
					Enabled:                       true,
					PreStopDelaySeconds:           10,
					TerminationGracePeriodSeconds: 30,
				},
			}
			Expect(k8sClient.Create(ctx, mc)).To(Succeed())

			_, err := reconcileOnce(mc)
			Expect(err).NotTo(HaveOccurred())

			sts := fetchStatefulSet(mc)
			Expect(sts.Spec.Template.Spec.Containers[0].Lifecycle.PreStop.Exec.Command).To(Equal([]string{"sleep", "10"}))

			// Update preStopDelaySeconds to 20.
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(mc), mc)).To(Succeed())
			mc.Spec.HighAvailability.GracefulShutdown.PreStopDelaySeconds = 20
			mc.Spec.HighAvailability.GracefulShutdown.TerminationGracePeriodSeconds = 45
			Expect(k8sClient.Update(ctx, mc)).To(Succeed())

			_, err = reconcileOnce(mc)
			Expect(err).NotTo(HaveOccurred())

			sts = fetchStatefulSet(mc)
			Expect(sts.Spec.Template.Spec.Containers[0].Lifecycle.PreStop.Exec.Command).To(Equal([]string{"sleep", "20"}))
			Expect(*sts.Spec.Template.Spec.TerminationGracePeriodSeconds).To(Equal(int64(45)))
		})

		It("should remove preStop hook when graceful shutdown is disabled", func() {
			mc := validMemcached(uniqueName("dep-gs-rm"))
			mc.Spec.HighAvailability = &memcachedv1beta1.HighAvailabilitySpec{
				GracefulShutdown: &memcachedv1beta1.GracefulShutdownSpec{
					Enabled:                       true,
					PreStopDelaySeconds:           10,
					TerminationGracePeriodSeconds: 60,
				},
			}
			Expect(k8sClient.Create(ctx, mc)).To(Succeed())

			_, err := reconcileOnce(mc)
			Expect(err).NotTo(HaveOccurred())

			sts := fetchStatefulSet(mc)
			Expect(sts.Spec.Template.Spec.Containers[0].Lifecycle).NotTo(BeNil())
			Expect(*sts.Spec.Template.Spec.TerminationGracePeriodSeconds).To(Equal(int64(60)))

			// Disable graceful shutdown.
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(mc), mc)).To(Succeed())
			mc.Spec.HighAvailability.GracefulShutdown.Enabled = false
			Expect(k8sClient.Update(ctx, mc)).To(Succeed())

			_, err = reconcileOnce(mc)
			Expect(err).NotTo(HaveOccurred())

			sts = fetchStatefulSet(mc)
			Expect(sts.Spec.Template.Spec.Containers[0].Lifecycle).To(BeNil())
			// When graceful shutdown is disabled, terminationGracePeriodSeconds is set to nil
			// by the controller, but the Kubernetes API server applies its default of 30.
			// Verify the controller is not setting a custom value.
			Expect(sts.Spec.Template.Spec.TerminationGracePeriodSeconds).NotTo(BeNil())
			Expect(*sts.Spec.Template.Spec.TerminationGracePeriodSeconds).To(Equal(int64(30)))
		})

		It("should be idempotent with graceful shutdown enabled", func() {
			mc := validMemcached(uniqueName("dep-gs-idemp"))
			mc.Spec.HighAvailability = &memcachedv1beta1.HighAvailabilitySpec{
				GracefulShutdown: &memcachedv1beta1.GracefulShutdownSpec{
					Enabled:                       true,
					PreStopDelaySeconds:           10,
					TerminationGracePeriodSeconds: 30,
				},
			}
			Expect(k8sClient.Create(ctx, mc)).To(Succeed())

			_, err := reconcileOnce(mc)
			Expect(err).NotTo(HaveOccurred())

			sts1 := fetchStatefulSet(mc)
			rv1 := sts1.ResourceVersion

			// Reconcile again without changes.
			_, err = reconcileOnce(mc)
			Expect(err).NotTo(HaveOccurred())

			sts2 := fetchStatefulSet(mc)
			Expect(sts2.ResourceVersion).To(Equal(rv1))
		})

		It("should support graceful shutdown alongside anti-affinity and topology spread", func() {
			mc := validMemcached(uniqueName("dep-gs-all"))
			soft := memcachedv1beta1.AntiAffinityPresetSoft
			mc.Spec.HighAvailability = &memcachedv1beta1.HighAvailabilitySpec{
				AntiAffinityPreset:        &soft,
				TopologySpreadConstraints: []corev1.TopologySpreadConstraint{zoneSpreadConstraint()},
				GracefulShutdown: &memcachedv1beta1.GracefulShutdownSpec{
					Enabled:                       true,
					PreStopDelaySeconds:           10,
					TerminationGracePeriodSeconds: 30,
				},
			}
			Expect(k8sClient.Create(ctx, mc)).To(Succeed())

			_, err := reconcileOnce(mc)
			Expect(err).NotTo(HaveOccurred())

			sts := fetchStatefulSet(mc)

			// Anti-affinity.
			Expect(sts.Spec.Template.Spec.Affinity).NotTo(BeNil())
			Expect(sts.Spec.Template.Spec.Affinity.PodAntiAffinity).NotTo(BeNil())
			Expect(sts.Spec.Template.Spec.Affinity.PodAntiAffinity.PreferredDuringSchedulingIgnoredDuringExecution).To(HaveLen(1))

			// Topology spread.
			Expect(sts.Spec.Template.Spec.TopologySpreadConstraints).To(HaveLen(1))
			Expect(sts.Spec.Template.Spec.TopologySpreadConstraints[0].TopologyKey).To(Equal("topology.kubernetes.io/zone"))

			// Graceful shutdown.
			container := sts.Spec.Template.Spec.Containers[0]
			Expect(container.Lifecycle).NotTo(BeNil())
			Expect(container.Lifecycle.PreStop.Exec.Command).To(Equal([]string{"sleep", "10"}))
			Expect(*sts.Spec.Template.Spec.TerminationGracePeriodSeconds).To(Equal(int64(30)))
		})
	})
})

// legacyMemcachedDeployment returns a valid apps/v1 Deployment named like mc, shaped like the
// Deployment that operator releases without the StatefulSet ran the pods in, with the given
// owner references.
func legacyMemcachedDeployment(mc *memcachedv1beta1.Memcached, ownerRefs []metav1.OwnerReference) *appsv1.Deployment {
	labels := map[string]string{
		"app.kubernetes.io/name":       "memcached",
		"app.kubernetes.io/instance":   mc.Name,
		"app.kubernetes.io/managed-by": "memcached-operator",
	}
	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:            mc.Name,
			Namespace:       mc.Namespace,
			Labels:          labels,
			OwnerReferences: ownerRefs,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: int32Ptr(1),
			Selector: &metav1.LabelSelector{MatchLabels: labels},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: labels},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{Name: "memcached", Image: "memcached:1.6"}},
				},
			},
		},
	}
}

// memcachedControllerRef returns a controller owner reference to a Memcached named name with the given UID.
func memcachedControllerRef(name string, uid types.UID) []metav1.OwnerReference {
	isController := true
	return []metav1.OwnerReference{{
		APIVersion: "memcached.c5c3.io/v1beta1",
		Kind:       "Memcached",
		Name:       name,
		UID:        uid,
		Controller: &isController,
	}}
}

var _ = Describe("StatefulSet naming and legacy Deployment migration", func() {

	Context("new instance with three replicas", func() {
		It("should create a StatefulSet named like the CR with 3 replicas and no Deployment", func() {
			mc := validMemcached(uniqueName("sts-new"))
			mc.Spec.Replicas = int32Ptr(3)
			Expect(k8sClient.Create(ctx, mc)).To(Succeed())
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(mc), mc)).To(Succeed())

			_, err := reconcileOnce(mc)
			Expect(err).NotTo(HaveOccurred())

			sts := fetchStatefulSet(mc)
			Expect(sts.Name).To(Equal(mc.Name))
			Expect(sts.Spec.Replicas).NotTo(BeNil())
			Expect(*sts.Spec.Replicas).To(Equal(int32(3)))
			expectOwnerRef(sts, mc)

			err = k8sClient.Get(ctx, client.ObjectKeyFromObject(mc), &appsv1.Deployment{})
			Expect(apierrors.IsNotFound(err)).To(BeTrue(), "no Deployment should exist for the instance, got err = %v", err)
		})
	})

	Context("Memcached name longer than 52 characters", func() {
		It("should shorten the StatefulSet name and point the Service, HPA, and serverList at it", func() {
			name := uniqueName("sts-long-name-" + strings.Repeat("x", 37))
			Expect(name).To(HaveLen(60))

			mc := validMemcached(name)
			mc.Spec.Resources = cpuResourceRequirements()
			mc.Spec.Autoscaling = &memcachedv1beta1.AutoscalingSpec{
				Enabled:     true,
				MinReplicas: int32Ptr(2),
				MaxReplicas: 5,
			}
			Expect(k8sClient.Create(ctx, mc)).To(Succeed())
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(mc), mc)).To(Succeed())

			_, err := reconcileOnce(mc)
			Expect(err).NotTo(HaveOccurred())

			stsList := &appsv1.StatefulSetList{}
			Expect(k8sClient.List(ctx, stsList,
				client.InNamespace(mc.Namespace),
				client.MatchingLabels{"app.kubernetes.io/instance": name},
			)).To(Succeed())
			Expect(stsList.Items).To(HaveLen(1))
			sts := &stsList.Items[0]

			// <first 43 characters>-<8 lowercase hex characters>, 52 characters in all.
			Expect(sts.Name).To(HaveLen(52))
			Expect(sts.Name).To(HavePrefix(name[:43] + "-"))
			Expect(sts.Name[44:]).To(MatchRegexp(`^[0-9a-f]{8}$`))
			expectOwnerRef(sts, mc)
			Expect(sts.Spec.Replicas).NotTo(BeNil())
			Expect(*sts.Spec.Replicas).To(Equal(int32(2)))

			err = k8sClient.Get(ctx, client.ObjectKeyFromObject(mc), &appsv1.StatefulSet{})
			Expect(apierrors.IsNotFound(err)).To(BeTrue(), "no StatefulSet should carry the full name, got err = %v", err)

			// The headless Service keeps the full name and governs the StatefulSet.
			Expect(sts.Spec.ServiceName).To(Equal(name))
			svc := fetchService(mc)
			Expect(svc.Name).To(Equal(name))

			hpa := fetchHPA(mc)
			Expect(hpa.Spec.ScaleTargetRef.APIVersion).To(Equal("apps/v1"))
			Expect(hpa.Spec.ScaleTargetRef.Kind).To(Equal("StatefulSet"))
			Expect(hpa.Spec.ScaleTargetRef.Name).To(Equal(sts.Name))

			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(mc), mc)).To(Succeed())
			Expect(mc.Status.ServerList).To(Equal([]string{
				fmt.Sprintf("%s-0.%s.%s.svc:11211", sts.Name, name, mc.Namespace),
				fmt.Sprintf("%s-1.%s.%s.svc:11211", sts.Name, name, mc.Namespace),
			}))

			// A second reconcile finds the shortened StatefulSet and leaves it unchanged.
			rv := sts.ResourceVersion
			_, err = reconcileOnce(mc)
			Expect(err).NotTo(HaveOccurred())

			Expect(k8sClient.List(ctx, stsList,
				client.InNamespace(mc.Namespace),
				client.MatchingLabels{"app.kubernetes.io/instance": name},
			)).To(Succeed())
			Expect(stsList.Items).To(HaveLen(1))
			Expect(stsList.Items[0].Name).To(Equal(sts.Name))
			Expect(stsList.Items[0].ResourceVersion).To(Equal(rv))
		})
	})

	Context("legacy Deployment controlled by the Memcached", func() {
		It("should delete the Deployment in the first reconcile and keep the StatefulSet", func() {
			mc := validMemcached(uniqueName("sts-legacy"))
			Expect(k8sClient.Create(ctx, mc)).To(Succeed())
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(mc), mc)).To(Succeed())

			legacy := legacyMemcachedDeployment(mc, memcachedControllerRef(mc.Name, mc.UID))
			Expect(k8sClient.Create(ctx, legacy)).To(Succeed())

			_, err := reconcileOnce(mc)
			Expect(err).NotTo(HaveOccurred())

			err = k8sClient.Get(ctx, client.ObjectKeyFromObject(legacy), &appsv1.Deployment{})
			Expect(apierrors.IsNotFound(err)).To(BeTrue(), "legacy Deployment should be deleted, got err = %v", err)

			sts := fetchStatefulSet(mc)
			expectOwnerRef(sts, mc)
		})
	})

	Context("same-named Deployment the Memcached does not control", func() {
		It("should keep a Deployment without owner references", func() {
			mc := validMemcached(uniqueName("sts-unowned"))
			Expect(k8sClient.Create(ctx, mc)).To(Succeed())
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(mc), mc)).To(Succeed())

			other := legacyMemcachedDeployment(mc, nil)
			Expect(k8sClient.Create(ctx, other)).To(Succeed())

			for i := range 2 {
				_, err := reconcileOnce(mc)
				Expect(err).NotTo(HaveOccurred())
				Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(other), &appsv1.Deployment{})).To(Succeed(),
					"Deployment without owner references should survive reconcile %d", i+1)
			}
			fetchStatefulSet(mc)
		})

		It("should keep a Deployment controlled by a different UID", func() {
			mc := validMemcached(uniqueName("sts-foreign"))
			Expect(k8sClient.Create(ctx, mc)).To(Succeed())
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(mc), mc)).To(Succeed())

			otherUID := types.UID(uuid.New().String())
			Expect(otherUID).NotTo(Equal(mc.UID))
			other := legacyMemcachedDeployment(mc, memcachedControllerRef(mc.Name, otherUID))
			Expect(k8sClient.Create(ctx, other)).To(Succeed())

			for i := range 2 {
				_, err := reconcileOnce(mc)
				Expect(err).NotTo(HaveOccurred())
				Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(other), &appsv1.Deployment{})).To(Succeed(),
					"Deployment controlled by a different UID should survive reconcile %d", i+1)
			}
			fetchStatefulSet(mc)
		})
	})
})
