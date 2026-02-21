package controller_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	autoscalingv2 "k8s.io/api/autoscaling/v2"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	memcachedv1alpha1 "github.com/c5c3/memcached-operator/api/v1alpha1"
)

// fetchHPA retrieves the HPA with the same name/namespace as the Memcached CR.
func fetchHPA(mc *memcachedv1alpha1.Memcached) *autoscalingv2.HorizontalPodAutoscaler {
	hpa := &autoscalingv2.HorizontalPodAutoscaler{}
	ExpectWithOffset(1, k8sClient.Get(ctx, client.ObjectKeyFromObject(mc), hpa)).To(Succeed())
	return hpa
}

// cpuResourceRequirements returns ResourceRequirements with a CPU request,
// needed for CRs that use CPU utilization metrics (all autoscaling-enabled CRs).
func cpuResourceRequirements() *corev1.ResourceRequirements {
	return &corev1.ResourceRequirements{
		Requests: corev1.ResourceList{
			corev1.ResourceCPU: resource.MustParse("100m"),
		},
	}
}

// --- Task 3.1: HPA creation when autoscaling enabled (REQ-001, REQ-005, REQ-009) ---

var _ = Describe("HPA Reconciliation", func() {

	Context("HPA creation with full spec", func() {
		var mc *memcachedv1alpha1.Memcached

		BeforeEach(func() {
			mc = validMemcached(uniqueName("hpa-create"))
			mc.Spec.Resources = cpuResourceRequirements()
			mc.Spec.Autoscaling = &memcachedv1alpha1.AutoscalingSpec{
				Enabled:     true,
				MinReplicas: int32Ptr(2),
				MaxReplicas: 10,
				Metrics: []autoscalingv2.MetricSpec{
					{
						Type: autoscalingv2.ResourceMetricSourceType,
						Resource: &autoscalingv2.ResourceMetricSource{
							Name: corev1.ResourceCPU,
							Target: autoscalingv2.MetricTarget{
								Type:               autoscalingv2.UtilizationMetricType,
								AverageUtilization: int32Ptr(70),
							},
						},
					},
				},
				Behavior: &autoscalingv2.HorizontalPodAutoscalerBehavior{
					ScaleDown: &autoscalingv2.HPAScalingRules{
						StabilizationWindowSeconds: int32Ptr(600),
					},
				},
			}
			Expect(k8sClient.Create(ctx, mc)).To(Succeed())
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(mc), mc)).To(Succeed())

			result, err := reconcileOnce(mc)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal(ctrl.Result{}))
		})

		It("should create HPA targeting the Deployment", func() {
			hpa := fetchHPA(mc)
			Expect(hpa.Spec.ScaleTargetRef.APIVersion).To(Equal("apps/v1"))
			Expect(hpa.Spec.ScaleTargetRef.Kind).To(Equal("Deployment"))
			Expect(hpa.Spec.ScaleTargetRef.Name).To(Equal(mc.Name))
		})

		It("should set minReplicas and maxReplicas from spec", func() {
			hpa := fetchHPA(mc)
			Expect(hpa.Spec.MinReplicas).NotTo(BeNil())
			Expect(*hpa.Spec.MinReplicas).To(Equal(int32(2)))
			Expect(hpa.Spec.MaxReplicas).To(Equal(int32(10)))
		})

		It("should set custom metrics from spec", func() {
			hpa := fetchHPA(mc)
			Expect(hpa.Spec.Metrics).To(HaveLen(1))
			Expect(hpa.Spec.Metrics[0].Type).To(Equal(autoscalingv2.ResourceMetricSourceType))
			Expect(hpa.Spec.Metrics[0].Resource.Name).To(Equal(corev1.ResourceCPU))
			Expect(*hpa.Spec.Metrics[0].Resource.Target.AverageUtilization).To(Equal(int32(70)))
		})

		It("should set custom behavior from spec", func() {
			hpa := fetchHPA(mc)
			Expect(hpa.Spec.Behavior).NotTo(BeNil())
			Expect(hpa.Spec.Behavior.ScaleDown).NotTo(BeNil())
			Expect(*hpa.Spec.Behavior.ScaleDown.StabilizationWindowSeconds).To(Equal(int32(600)))
		})

		It("should set standard labels", func() {
			hpa := fetchHPA(mc)
			Expect(hpa.Labels).To(HaveKeyWithValue("app.kubernetes.io/name", "memcached"))
			Expect(hpa.Labels).To(HaveKeyWithValue("app.kubernetes.io/instance", mc.Name))
			Expect(hpa.Labels).To(HaveKeyWithValue("app.kubernetes.io/managed-by", "memcached-operator"))
		})

		It("should set owner reference", func() {
			hpa := fetchHPA(mc)
			expectOwnerRef(hpa, mc)
		})
	})

	Context("HPA creation with webhook defaults", func() {
		It("should create HPA with default CPU metric and scaleDown behavior when not specified", func() {
			mc := validMemcached(uniqueName("hpa-defaults"))
			mc.Spec.Resources = cpuResourceRequirements()
			mc.Spec.Autoscaling = &memcachedv1alpha1.AutoscalingSpec{
				Enabled:     true,
				MaxReplicas: 5,
			}
			Expect(k8sClient.Create(ctx, mc)).To(Succeed())
			// Re-fetch to get webhook-applied defaults.
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(mc), mc)).To(Succeed())

			_, err := reconcileOnce(mc)
			Expect(err).NotTo(HaveOccurred())

			hpa := fetchHPA(mc)

			// Webhook injects 80% CPU utilization metric.
			Expect(hpa.Spec.Metrics).To(HaveLen(1))
			Expect(hpa.Spec.Metrics[0].Type).To(Equal(autoscalingv2.ResourceMetricSourceType))
			Expect(hpa.Spec.Metrics[0].Resource.Name).To(Equal(corev1.ResourceCPU))
			Expect(*hpa.Spec.Metrics[0].Resource.Target.AverageUtilization).To(Equal(int32(80)))

			// Webhook injects scaleDown stabilization window of 300s.
			Expect(hpa.Spec.Behavior).NotTo(BeNil())
			Expect(hpa.Spec.Behavior.ScaleDown).NotTo(BeNil())
			Expect(*hpa.Spec.Behavior.ScaleDown.StabilizationWindowSeconds).To(Equal(int32(300)))

			Expect(hpa.Spec.MaxReplicas).To(Equal(int32(5)))
		})
	})

	Context("HPA creation with unset minReplicas", func() {
		It("should create HPA where Kubernetes defaults minReplicas to 1", func() {
			mc := validMemcached(uniqueName("hpa-nilmin"))
			mc.Spec.Resources = cpuResourceRequirements()
			mc.Spec.Autoscaling = &memcachedv1alpha1.AutoscalingSpec{
				Enabled:     true,
				MaxReplicas: 3,
			}
			Expect(k8sClient.Create(ctx, mc)).To(Succeed())

			_, err := reconcileOnce(mc)
			Expect(err).NotTo(HaveOccurred())

			hpa := fetchHPA(mc)
			// MinReplicas not set in CR spec → nil passed to HPA API.
			// The Kubernetes HPA API defaults minReplicas to 1 on storage.
			Expect(hpa.Spec.MinReplicas).NotTo(BeNil())
			Expect(*hpa.Spec.MinReplicas).To(Equal(int32(1)))
			Expect(hpa.Spec.MaxReplicas).To(Equal(int32(3)))
		})
	})

	// --- Task 3.2: HPA update on spec change and idempotency (REQ-003) ---

	Context("HPA update on spec change", func() {
		It("should update HPA maxReplicas when CR spec changes", func() {
			mc := validMemcached(uniqueName("hpa-update-max"))
			mc.Spec.Resources = cpuResourceRequirements()
			mc.Spec.Autoscaling = &memcachedv1alpha1.AutoscalingSpec{
				Enabled:     true,
				MinReplicas: int32Ptr(2),
				MaxReplicas: 5,
			}
			Expect(k8sClient.Create(ctx, mc)).To(Succeed())

			_, err := reconcileOnce(mc)
			Expect(err).NotTo(HaveOccurred())

			hpa := fetchHPA(mc)
			Expect(hpa.Spec.MaxReplicas).To(Equal(int32(5)))

			// Update maxReplicas.
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(mc), mc)).To(Succeed())
			mc.Spec.Autoscaling.MaxReplicas = 10
			Expect(k8sClient.Update(ctx, mc)).To(Succeed())

			_, err = reconcileOnce(mc)
			Expect(err).NotTo(HaveOccurred())

			hpa = fetchHPA(mc)
			Expect(hpa.Spec.MaxReplicas).To(Equal(int32(10)))
		})

		It("should update HPA minReplicas when CR spec changes", func() {
			mc := validMemcached(uniqueName("hpa-update-min"))
			mc.Spec.Resources = cpuResourceRequirements()
			mc.Spec.Autoscaling = &memcachedv1alpha1.AutoscalingSpec{
				Enabled:     true,
				MinReplicas: int32Ptr(1),
				MaxReplicas: 5,
			}
			Expect(k8sClient.Create(ctx, mc)).To(Succeed())

			_, err := reconcileOnce(mc)
			Expect(err).NotTo(HaveOccurred())

			hpa := fetchHPA(mc)
			Expect(*hpa.Spec.MinReplicas).To(Equal(int32(1)))

			// Update minReplicas.
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(mc), mc)).To(Succeed())
			mc.Spec.Autoscaling.MinReplicas = int32Ptr(3)
			Expect(k8sClient.Update(ctx, mc)).To(Succeed())

			_, err = reconcileOnce(mc)
			Expect(err).NotTo(HaveOccurred())

			hpa = fetchHPA(mc)
			Expect(*hpa.Spec.MinReplicas).To(Equal(int32(3)))
		})

		It("should update HPA metrics when CR spec changes", func() {
			mc := validMemcached(uniqueName("hpa-update-met"))
			mc.Spec.Resources = cpuResourceRequirements()
			mc.Spec.Autoscaling = &memcachedv1alpha1.AutoscalingSpec{
				Enabled:     true,
				MaxReplicas: 5,
				Metrics: []autoscalingv2.MetricSpec{
					{
						Type: autoscalingv2.ResourceMetricSourceType,
						Resource: &autoscalingv2.ResourceMetricSource{
							Name: corev1.ResourceCPU,
							Target: autoscalingv2.MetricTarget{
								Type:               autoscalingv2.UtilizationMetricType,
								AverageUtilization: int32Ptr(70),
							},
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, mc)).To(Succeed())

			_, err := reconcileOnce(mc)
			Expect(err).NotTo(HaveOccurred())

			hpa := fetchHPA(mc)
			Expect(*hpa.Spec.Metrics[0].Resource.Target.AverageUtilization).To(Equal(int32(70)))

			// Update CPU target to 90%.
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(mc), mc)).To(Succeed())
			mc.Spec.Autoscaling.Metrics[0].Resource.Target.AverageUtilization = int32Ptr(90)
			Expect(k8sClient.Update(ctx, mc)).To(Succeed())

			_, err = reconcileOnce(mc)
			Expect(err).NotTo(HaveOccurred())

			hpa = fetchHPA(mc)
			Expect(*hpa.Spec.Metrics[0].Resource.Target.AverageUtilization).To(Equal(int32(90)))
		})
	})

	Context("HPA idempotency", func() {
		It("should not change HPA resource version on second reconcile", func() {
			mc := validMemcached(uniqueName("hpa-idempotent"))
			mc.Spec.Resources = cpuResourceRequirements()
			mc.Spec.Autoscaling = &memcachedv1alpha1.AutoscalingSpec{
				Enabled:     true,
				MinReplicas: int32Ptr(2),
				MaxReplicas: 5,
			}
			Expect(k8sClient.Create(ctx, mc)).To(Succeed())

			_, err := reconcileOnce(mc)
			Expect(err).NotTo(HaveOccurred())

			hpa1 := fetchHPA(mc)
			rv1 := hpa1.ResourceVersion

			// Reconcile again without changes.
			_, err = reconcileOnce(mc)
			Expect(err).NotTo(HaveOccurred())

			hpa2 := fetchHPA(mc)
			Expect(hpa2.ResourceVersion).To(Equal(rv1))
		})
	})

	// --- Task 3.3: HPA deletion when autoscaling disabled (REQ-002) ---

	Context("HPA deletion when autoscaling disabled", func() {
		It("should not create HPA when autoscaling is not enabled", func() {
			mc := validMemcached(uniqueName("hpa-disabled"))
			Expect(k8sClient.Create(ctx, mc)).To(Succeed())

			_, err := reconcileOnce(mc)
			Expect(err).NotTo(HaveOccurred())

			hpa := &autoscalingv2.HorizontalPodAutoscaler{}
			err = k8sClient.Get(ctx, client.ObjectKeyFromObject(mc), hpa)
			Expect(apierrors.IsNotFound(err)).To(BeTrue())
		})

		It("should delete HPA when autoscaling is disabled after being enabled", func() {
			mc := validMemcached(uniqueName("hpa-toggle"))
			mc.Spec.Resources = cpuResourceRequirements()
			mc.Spec.Autoscaling = &memcachedv1alpha1.AutoscalingSpec{
				Enabled:     true,
				MaxReplicas: 5,
			}
			Expect(k8sClient.Create(ctx, mc)).To(Succeed())

			_, err := reconcileOnce(mc)
			Expect(err).NotTo(HaveOccurred())

			// HPA exists.
			fetchHPA(mc)

			// Disable autoscaling and set replicas (required when disabling).
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(mc), mc)).To(Succeed())
			mc.Spec.Autoscaling.Enabled = false
			mc.Spec.Replicas = int32Ptr(3)
			Expect(k8sClient.Update(ctx, mc)).To(Succeed())

			_, err = reconcileOnce(mc)
			Expect(err).NotTo(HaveOccurred())

			// HPA should be deleted.
			hpa := &autoscalingv2.HorizontalPodAutoscaler{}
			err = k8sClient.Get(ctx, client.ObjectKeyFromObject(mc), hpa)
			Expect(apierrors.IsNotFound(err)).To(BeTrue())
		})

		It("should not create HPA when autoscaling spec is nil", func() {
			mc := validMemcached(uniqueName("hpa-nil-spec"))
			mc.Spec.Autoscaling = nil
			Expect(k8sClient.Create(ctx, mc)).To(Succeed())

			_, err := reconcileOnce(mc)
			Expect(err).NotTo(HaveOccurred())

			hpa := &autoscalingv2.HorizontalPodAutoscaler{}
			err = k8sClient.Get(ctx, client.ObjectKeyFromObject(mc), hpa)
			Expect(apierrors.IsNotFound(err)).To(BeTrue())
		})

		It("should handle idempotent deletion when reconciled twice with autoscaling disabled", func() {
			mc := validMemcached(uniqueName("hpa-del-idem"))
			mc.Spec.Resources = cpuResourceRequirements()
			mc.Spec.Autoscaling = &memcachedv1alpha1.AutoscalingSpec{
				Enabled:     true,
				MaxReplicas: 5,
			}
			Expect(k8sClient.Create(ctx, mc)).To(Succeed())

			_, err := reconcileOnce(mc)
			Expect(err).NotTo(HaveOccurred())
			fetchHPA(mc)

			// Disable autoscaling.
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(mc), mc)).To(Succeed())
			mc.Spec.Autoscaling.Enabled = false
			mc.Spec.Replicas = int32Ptr(3)
			Expect(k8sClient.Update(ctx, mc)).To(Succeed())

			// First reconcile deletes the HPA.
			_, err = reconcileOnce(mc)
			Expect(err).NotTo(HaveOccurred())

			hpa := &autoscalingv2.HorizontalPodAutoscaler{}
			err = k8sClient.Get(ctx, client.ObjectKeyFromObject(mc), hpa)
			Expect(apierrors.IsNotFound(err)).To(BeTrue())

			// Second reconcile with autoscaling still disabled — deleteOwnedResource handles NotFound silently.
			_, err = reconcileOnce(mc)
			Expect(err).NotTo(HaveOccurred())

			err = k8sClient.Get(ctx, client.ObjectKeyFromObject(mc), hpa)
			Expect(apierrors.IsNotFound(err)).To(BeTrue())
		})

		It("should re-enable HPA after disabling and re-enabling", func() {
			mc := validMemcached(uniqueName("hpa-reenable"))
			mc.Spec.Resources = cpuResourceRequirements()
			mc.Spec.Autoscaling = &memcachedv1alpha1.AutoscalingSpec{
				Enabled:     true,
				MinReplicas: int32Ptr(2),
				MaxReplicas: 5,
			}
			Expect(k8sClient.Create(ctx, mc)).To(Succeed())

			_, err := reconcileOnce(mc)
			Expect(err).NotTo(HaveOccurred())
			fetchHPA(mc)

			// Disable.
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(mc), mc)).To(Succeed())
			mc.Spec.Autoscaling.Enabled = false
			mc.Spec.Replicas = int32Ptr(3)
			Expect(k8sClient.Update(ctx, mc)).To(Succeed())

			_, err = reconcileOnce(mc)
			Expect(err).NotTo(HaveOccurred())

			hpa := &autoscalingv2.HorizontalPodAutoscaler{}
			err = k8sClient.Get(ctx, client.ObjectKeyFromObject(mc), hpa)
			Expect(apierrors.IsNotFound(err)).To(BeTrue())

			// Re-enable with different maxReplicas.
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(mc), mc)).To(Succeed())
			mc.Spec.Autoscaling.Enabled = true
			mc.Spec.Autoscaling.MaxReplicas = 8
			mc.Spec.Replicas = nil
			Expect(k8sClient.Update(ctx, mc)).To(Succeed())

			_, err = reconcileOnce(mc)
			Expect(err).NotTo(HaveOccurred())

			newHPA := fetchHPA(mc)
			Expect(newHPA.Spec.MaxReplicas).To(Equal(int32(8)))
		})
	})

	// --- Task 3.4: Deployment replicas interaction with autoscaling (REQ-004) ---

	Context("Deployment replicas interaction with autoscaling", func() {
		It("should not hardcode Deployment replicas from CR spec when autoscaling is enabled", func() {
			mc := validMemcached(uniqueName("hpa-dep-nil"))
			mc.Spec.Resources = cpuResourceRequirements()
			mc.Spec.Autoscaling = &memcachedv1alpha1.AutoscalingSpec{
				Enabled:     true,
				MaxReplicas: 5,
			}
			Expect(k8sClient.Create(ctx, mc)).To(Succeed())

			_, err := reconcileOnce(mc)
			Expect(err).NotTo(HaveOccurred())

			// The operator passes nil replicas to the Deployment when HPA is active.
			// The Kubernetes Deployment API defaults nil replicas to 1 on storage.
			// This is expected — the HPA will adjust replicas as needed.
			dep := fetchDeployment(mc)
			Expect(dep.Spec.Replicas).NotTo(BeNil())
			Expect(*dep.Spec.Replicas).To(Equal(int32(1)))
		})

		It("should restore Deployment spec.replicas when autoscaling is disabled", func() {
			mc := validMemcached(uniqueName("hpa-dep-restore"))
			mc.Spec.Resources = cpuResourceRequirements()
			mc.Spec.Autoscaling = &memcachedv1alpha1.AutoscalingSpec{
				Enabled:     true,
				MaxReplicas: 5,
			}
			Expect(k8sClient.Create(ctx, mc)).To(Succeed())

			_, err := reconcileOnce(mc)
			Expect(err).NotTo(HaveOccurred())

			// With HPA active, Kubernetes defaults Deployment replicas to 1.
			dep := fetchDeployment(mc)
			Expect(dep.Spec.Replicas).NotTo(BeNil())

			// Disable autoscaling and set replicas.
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(mc), mc)).To(Succeed())
			mc.Spec.Autoscaling.Enabled = false
			mc.Spec.Replicas = int32Ptr(3)
			Expect(k8sClient.Update(ctx, mc)).To(Succeed())

			_, err = reconcileOnce(mc)
			Expect(err).NotTo(HaveOccurred())

			dep = fetchDeployment(mc)
			Expect(dep.Spec.Replicas).NotTo(BeNil())
			Expect(*dep.Spec.Replicas).To(Equal(int32(3)))
		})

		It("should not override Deployment replicas from CR spec when autoscaling is active", func() {
			mc := validMemcached(uniqueName("hpa-dep-noset"))
			mc.Spec.Resources = cpuResourceRequirements()
			mc.Spec.Autoscaling = &memcachedv1alpha1.AutoscalingSpec{
				Enabled:     true,
				MinReplicas: int32Ptr(2),
				MaxReplicas: 10,
			}
			// spec.replicas is nil (webhook ensures this for autoscaling-enabled CRs).
			Expect(k8sClient.Create(ctx, mc)).To(Succeed())
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(mc), mc)).To(Succeed())

			// Verify webhook cleared replicas.
			Expect(mc.Spec.Replicas).To(BeNil())

			_, err := reconcileOnce(mc)
			Expect(err).NotTo(HaveOccurred())

			// The Kubernetes Deployment API defaults nil replicas to 1.
			dep := fetchDeployment(mc)
			Expect(dep.Spec.Replicas).NotTo(BeNil())

			// HPA exists alongside the Deployment.
			hpa := fetchHPA(mc)
			Expect(hpa.Spec.MinReplicas).NotTo(BeNil())
			Expect(*hpa.Spec.MinReplicas).To(Equal(int32(2)))
			Expect(hpa.Spec.MaxReplicas).To(Equal(int32(10)))
		})
	})

	// --- Task 3.5: Status conditions with HPA-managed scaling (REQ-008) ---

	Context("Status conditions with HPA-managed scaling", func() {
		It("should include HPA-managed annotation in Available condition message", func() {
			mc := validMemcached(uniqueName("hpa-status-msg"))
			mc.Spec.Resources = cpuResourceRequirements()
			mc.Spec.Autoscaling = &memcachedv1alpha1.AutoscalingSpec{
				Enabled:     true,
				MaxReplicas: 5,
			}
			Expect(k8sClient.Create(ctx, mc)).To(Succeed())
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(mc), mc)).To(Succeed())

			_, err := reconcileOnce(mc)
			Expect(err).NotTo(HaveOccurred())

			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(mc), mc)).To(Succeed())

			available := findCondition(mc.Status.Conditions, "Available")
			Expect(available).NotTo(BeNil())
			Expect(available.Message).To(ContainSubstring("(HPA-managed)"))
		})

		It("should not include HPA-managed annotation when autoscaling is disabled", func() {
			mc := validMemcached(uniqueName("hpa-status-nomsg"))
			mc.Spec.Replicas = int32Ptr(2)
			Expect(k8sClient.Create(ctx, mc)).To(Succeed())
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(mc), mc)).To(Succeed())

			_, err := reconcileOnce(mc)
			Expect(err).NotTo(HaveOccurred())

			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(mc), mc)).To(Succeed())

			available := findCondition(mc.Status.Conditions, "Available")
			Expect(available).NotTo(BeNil())
			Expect(available.Message).NotTo(ContainSubstring("HPA-managed"))
		})

		It("should set all three conditions with HPA active", func() {
			mc := validMemcached(uniqueName("hpa-status-all"))
			mc.Spec.Resources = cpuResourceRequirements()
			mc.Spec.Autoscaling = &memcachedv1alpha1.AutoscalingSpec{
				Enabled:     true,
				MaxReplicas: 5,
			}
			Expect(k8sClient.Create(ctx, mc)).To(Succeed())
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(mc), mc)).To(Succeed())

			_, err := reconcileOnce(mc)
			Expect(err).NotTo(HaveOccurred())

			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(mc), mc)).To(Succeed())
			Expect(mc.Status.Conditions).To(HaveLen(3))

			available := findCondition(mc.Status.Conditions, "Available")
			Expect(available).NotTo(BeNil())

			progressing := findCondition(mc.Status.Conditions, "Progressing")
			Expect(progressing).NotTo(BeNil())

			degraded := findCondition(mc.Status.Conditions, "Degraded")
			Expect(degraded).NotTo(BeNil())

			Expect(mc.Status.ObservedGeneration).To(Equal(mc.Generation))
		})

		It("should use Deployment status replicas as desired count when HPA active", func() {
			mc := validMemcached(uniqueName("hpa-status-dep"))
			mc.Spec.Resources = cpuResourceRequirements()
			mc.Spec.Autoscaling = &memcachedv1alpha1.AutoscalingSpec{
				Enabled:     true,
				MaxReplicas: 5,
			}
			Expect(k8sClient.Create(ctx, mc)).To(Succeed())
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(mc), mc)).To(Succeed())

			_, err := reconcileOnce(mc)
			Expect(err).NotTo(HaveOccurred())

			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(mc), mc)).To(Succeed())

			// In envtest, Deployment status replicas is 0 (no real pods).
			// With HPA active, desired = dep.Status.Replicas = 0.
			// So: Available=True (0 ready, 0 desired → readyReplicas > 0 is false, but
			// actually: readyReplicas(0) > 0 is false → Available=False).
			available := findCondition(mc.Status.Conditions, "Available")
			Expect(available).NotTo(BeNil())
			// 0/0 replicas ready (HPA-managed) — desiredReplicas is 0 from dep.Status.Replicas.
			Expect(available.Message).To(ContainSubstring("0/0"))
			Expect(available.Message).To(ContainSubstring("(HPA-managed)"))
		})

		It("should transition status when switching from HPA to manual scaling", func() {
			mc := validMemcached(uniqueName("hpa-status-trans"))
			mc.Spec.Resources = cpuResourceRequirements()
			mc.Spec.Autoscaling = &memcachedv1alpha1.AutoscalingSpec{
				Enabled:     true,
				MaxReplicas: 5,
			}
			Expect(k8sClient.Create(ctx, mc)).To(Succeed())
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(mc), mc)).To(Succeed())

			_, err := reconcileOnce(mc)
			Expect(err).NotTo(HaveOccurred())

			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(mc), mc)).To(Succeed())
			available := findCondition(mc.Status.Conditions, "Available")
			Expect(available.Message).To(ContainSubstring("(HPA-managed)"))

			// Switch to manual scaling.
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(mc), mc)).To(Succeed())
			mc.Spec.Autoscaling.Enabled = false
			mc.Spec.Replicas = int32Ptr(3)
			Expect(k8sClient.Update(ctx, mc)).To(Succeed())

			_, err = reconcileOnce(mc)
			Expect(err).NotTo(HaveOccurred())

			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(mc), mc)).To(Succeed())
			available = findCondition(mc.Status.Conditions, "Available")
			Expect(available).NotTo(BeNil())
			Expect(available.Message).NotTo(ContainSubstring("HPA-managed"))
			// Now using spec.replicas = 3 as desired.
			Expect(available.Message).To(ContainSubstring("/3"))
		})
	})

	// --- Task 3.6 (partial): SetupWithManager Owns() test (REQ-006) ---

	Context("HPA recreation after external deletion", func() {
		It("should recreate the HPA on next reconcile after external deletion", func() {
			mc := validMemcached(uniqueName("hpa-recreate"))
			mc.Spec.Resources = cpuResourceRequirements()
			mc.Spec.Autoscaling = &memcachedv1alpha1.AutoscalingSpec{
				Enabled:     true,
				MinReplicas: int32Ptr(2),
				MaxReplicas: 5,
			}
			Expect(k8sClient.Create(ctx, mc)).To(Succeed())

			_, err := reconcileOnce(mc)
			Expect(err).NotTo(HaveOccurred())

			hpa := fetchHPA(mc)
			originalUID := hpa.UID

			// Externally delete the HPA.
			Expect(k8sClient.Delete(ctx, hpa)).To(Succeed())

			// Verify it's gone.
			err = k8sClient.Get(ctx, client.ObjectKeyFromObject(mc), &autoscalingv2.HorizontalPodAutoscaler{})
			Expect(apierrors.IsNotFound(err)).To(BeTrue())

			// Reconcile should recreate it.
			_, err = reconcileOnce(mc)
			Expect(err).NotTo(HaveOccurred())

			newHPA := fetchHPA(mc)
			Expect(newHPA.UID).NotTo(Equal(originalUID))
			expectOwnerRef(newHPA, mc)
			Expect(newHPA.Spec.MaxReplicas).To(Equal(int32(5)))
		})
	})
})

// --- Task 3.6 (partial): HPA in full-featured CR lifecycle ---

var _ = Describe("Full-featured CR with HPA", func() {

	Context("when all features including autoscaling are enabled", func() {
		var mc *memcachedv1alpha1.Memcached

		BeforeEach(func() {
			mc = validMemcached(uniqueName("hpa-full"))
			mc.Spec.Resources = cpuResourceRequirements()
			mc.Spec.Autoscaling = &memcachedv1alpha1.AutoscalingSpec{
				Enabled:     true,
				MinReplicas: int32Ptr(2),
				MaxReplicas: 10,
			}
			mc.Spec.Monitoring = &memcachedv1alpha1.MonitoringSpec{
				Enabled:        true,
				ServiceMonitor: &memcachedv1alpha1.ServiceMonitorSpec{},
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

		It("should create HPA alongside other resources", func() {
			fetchDeployment(mc)
			fetchService(mc)
			fetchHPA(mc)
			fetchServiceMonitor(mc)
			fetchNetworkPolicy(mc)
		})

		It("should not hardcode Deployment replicas from CR spec with HPA active", func() {
			dep := fetchDeployment(mc)
			// Operator passes nil replicas; Kubernetes defaults to 1 on storage.
			Expect(dep.Spec.Replicas).NotTo(BeNil())
			Expect(*dep.Spec.Replicas).To(Equal(int32(1)))
		})

		It("should set HPA-managed status condition", func() {
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(mc), mc)).To(Succeed())
			available := findCondition(mc.Status.Conditions, "Available")
			Expect(available).NotTo(BeNil())
			Expect(available.Message).To(ContainSubstring("(HPA-managed)"))
		})

		It("should set standard labels on HPA matching other resources", func() {
			hpa := fetchHPA(mc)
			dep := fetchDeployment(mc)
			svc := fetchService(mc)

			for _, obj := range []client.Object{hpa, dep, svc} {
				labels := obj.GetLabels()
				Expect(labels).To(HaveKeyWithValue("app.kubernetes.io/name", "memcached"))
				Expect(labels).To(HaveKeyWithValue("app.kubernetes.io/instance", mc.Name))
				Expect(labels).To(HaveKeyWithValue("app.kubernetes.io/managed-by", "memcached-operator"))
			}
		})

		It("should be idempotent with all features including HPA", func() {
			depRV := fetchDeployment(mc).ResourceVersion
			svcRV := fetchService(mc).ResourceVersion
			hpaRV := fetchHPA(mc).ResourceVersion

			_, err := reconcileOnce(mc)
			Expect(err).NotTo(HaveOccurred())

			Expect(fetchDeployment(mc).ResourceVersion).To(Equal(depRV))
			Expect(fetchService(mc).ResourceVersion).To(Equal(svcRV))
			Expect(fetchHPA(mc).ResourceVersion).To(Equal(hpaRV))
		})
	})
})
