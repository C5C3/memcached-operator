package controller_test

import (
	"context"
	"crypto/rand"
	"fmt"
	"runtime"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	monitoringv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	appsv1 "k8s.io/api/apps/v1"
	autoscalingv2 "k8s.io/api/autoscaling/v2"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	policyv1 "k8s.io/api/policy/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"

	memcachedv1beta1 "github.com/c5c3/memcached-operator/api/v1beta1"
	"github.com/c5c3/memcached-operator/internal/controller"
)

const (
	// footprintObjects is the number of Memcached instances, and so of objects per kind,
	// the footprint spec creates.
	footprintObjects = 100
	// footprintSecretBytes is the payload size of each SASL Secret.
	footprintSecretBytes = 16384
	// footprintSecretBytesLimit is the bound on the cached bytes per Secret: a cached
	// payload alone would exceed it.
	footprintSecretBytesLimit = 4096
)

// footprintKind is one kind the footprint spec measures.
type footprintKind struct {
	newObj  func() client.Object
	newList func() client.ObjectList
}

// name returns the kind's name, as in the scheme.
func (k footprintKind) name() string {
	gvk, err := apiutil.GVKForObject(k.newObj(), scheme.Scheme)
	ExpectWithOffset(1, err).NotTo(HaveOccurred())
	return gvk.Kind
}

// metadataOnly reports whether the kind is cached as metadata only.
func (k footprintKind) metadataOnly() bool {
	_, ok := k.newObj().(*metav1.PartialObjectMetadata)
	return ok
}

// secretMetadata returns the metadata-only form of a Secret, as the reconciler watches it.
func secretMetadata() client.Object {
	obj := &metav1.PartialObjectMetadata{}
	obj.SetGroupVersionKind(corev1.SchemeGroupVersion.WithKind("Secret"))
	return obj
}

func secretMetadataList() client.ObjectList {
	list := &metav1.PartialObjectMetadataList{}
	list.SetGroupVersionKind(corev1.SchemeGroupVersion.WithKind("SecretList"))
	return list
}

// footprintKinds are the kinds the manager caches, Secrets in their metadata-only form.
var footprintKinds = []footprintKind{
	{func() client.Object { return &memcachedv1beta1.Memcached{} }, func() client.ObjectList { return &memcachedv1beta1.MemcachedList{} }},
	{func() client.Object { return &appsv1.StatefulSet{} }, func() client.ObjectList { return &appsv1.StatefulSetList{} }},
	{func() client.Object { return &corev1.Service{} }, func() client.ObjectList { return &corev1.ServiceList{} }},
	{func() client.Object { return &autoscalingv2.HorizontalPodAutoscaler{} }, func() client.ObjectList { return &autoscalingv2.HorizontalPodAutoscalerList{} }},
	{func() client.Object { return &policyv1.PodDisruptionBudget{} }, func() client.ObjectList { return &policyv1.PodDisruptionBudgetList{} }},
	{func() client.Object { return &networkingv1.NetworkPolicy{} }, func() client.ObjectList { return &networkingv1.NetworkPolicyList{} }},
	{func() client.Object { return &monitoringv1.ServiceMonitor{} }, func() client.ObjectList { return &monitoringv1.ServiceMonitorList{} }},
	{secretMetadata, secretMetadataList},
}

// liveHeapBytes returns the bytes of allocated heap objects after two garbage collections.
func liveHeapBytes() int64 {
	runtime.GC()
	runtime.GC()
	var stats runtime.MemStats
	runtime.ReadMemStats(&stats)
	return int64(stats.HeapAlloc)
}

// measureCache builds a cache from controller.CacheOptions scoped to namespace, syncs
// the informer of kind, and returns the growth of the live heap while the cache is
// referenced, together with the number of objects the cache lists.
func measureCache(kind footprintKind, namespace string) (int64, int) {
	opts := controller.CacheOptions(map[string]cache.Config{namespace: {}})
	opts.Scheme = scheme.Scheme

	before := liveHeapBytes()

	c, err := cache.New(cfg, opts)
	ExpectWithOffset(1, err).NotTo(HaveOccurred())
	_, err = c.GetInformer(ctx, kind.newObj())
	ExpectWithOffset(1, err).NotTo(HaveOccurred())

	cacheCtx, stopCache := context.WithCancel(ctx)
	defer stopCache()
	go func() {
		defer GinkgoRecover()
		Expect(c.Start(cacheCtx)).To(Succeed())
	}()
	ExpectWithOffset(1, c.WaitForCacheSync(cacheCtx)).To(BeTrue())

	list := kind.newList()
	ExpectWithOffset(1, c.List(ctx, list, client.InNamespace(namespace))).To(Succeed())
	listed := meta.LenList(list)

	after := liveHeapBytes()
	runtime.KeepAlive(c)
	return after - before, listed
}

// footprintMemcached creates the SASL Secret for a Memcached named name in namespace and
// returns a Memcached with SASL, autoscaling, the PodDisruptionBudget, the ServiceMonitor,
// and the NetworkPolicy enabled.
func footprintMemcached(name, namespace string) *memcachedv1beta1.Memcached {
	payload := make([]byte, footprintSecretBytes)
	_, err := rand.Read(payload)
	ExpectWithOffset(1, err).NotTo(HaveOccurred())
	secret := newSASLSecret(name+"-sasl", string(payload))
	secret.Namespace = namespace
	ExpectWithOffset(1, k8sClient.Create(ctx, secret)).To(Succeed())

	maxUnavailable := intstr.FromInt32(1)
	mc := validMemcached(name)
	mc.Namespace = namespace
	mc.Spec.Resources = cpuResourceRequirements()
	mc.Spec.Autoscaling = &memcachedv1beta1.AutoscalingSpec{Enabled: true, MaxReplicas: 3}
	mc.Spec.HighAvailability = &memcachedv1beta1.HighAvailabilitySpec{
		PodDisruptionBudget: &memcachedv1beta1.PDBSpec{Enabled: true, MaxUnavailable: &maxUnavailable},
	}
	mc.Spec.Monitoring = &memcachedv1beta1.MonitoringSpec{
		Enabled:        true,
		ServiceMonitor: &memcachedv1beta1.ServiceMonitorSpec{},
	}
	mc.Spec.Security = &memcachedv1beta1.SecuritySpec{
		SASL:          saslSpec(secret.Name),
		NetworkPolicy: &memcachedv1beta1.NetworkPolicySpec{Enabled: true},
	}
	return mc
}

var _ = Describe("Manager cache footprint", Label("footprint"), func() {
	It("reports the cached bytes per object for every kind the manager caches", func() {
		populated := createNamespace("footprint")
		empty := createNamespace("footprint-empty")

		for i := range footprintObjects {
			mc := footprintMemcached(fmt.Sprintf("cache-%03d", i), populated)
			Expect(mc.IsSASLEnabled()).To(BeTrue())
			Expect(mc.IsAutoscalingEnabled()).To(BeTrue())
			Expect(mc.IsPDBEnabled()).To(BeTrue())
			Expect(mc.IsServiceMonitorEnabled()).To(BeTrue())
			Expect(mc.IsNetworkPolicyEnabled()).To(BeTrue())
			Expect(k8sClient.Create(ctx, mc)).To(Succeed())
			// Re-fetch to get webhook-applied defaults.
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(mc), mc)).To(Succeed())
			_, err := reconcileOnce(mc)
			Expect(err).NotTo(HaveOccurred())
		}

		for _, kind := range footprintKinds {
			populatedBytes, populatedListed := measureCache(kind, populated)
			emptyBytes, emptyListed := measureCache(kind, empty)
			Expect(populatedListed).To(Equal(footprintObjects), "%s objects listed from the populated namespace", kind.name())
			Expect(emptyListed).To(BeZero(), "%s objects listed from the empty namespace", kind.name())

			perObject := (populatedBytes - emptyBytes) / footprintObjects
			entry := fmt.Sprintf("cache-footprint kind=%s objects=%d bytes_per_object=%d", kind.name(), footprintObjects, perObject)
			AddReportEntry(entry)
			_, _ = fmt.Fprintln(GinkgoWriter, entry)

			if kind.metadataOnly() {
				Expect(perObject).To(BeNumerically("<", footprintSecretBytesLimit),
					"a cached Secret must not hold its %d-byte payload", footprintSecretBytes)
			}
		}
	})
})
