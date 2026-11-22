package controller_test

import (
	"context"
	"net/http"
	"net/url"
	"path"
	"sync"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	eventsv1 "k8s.io/api/events/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/config"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"

	memcachedv1beta1 "github.com/c5c3/memcached-operator/api/v1beta1"
	"github.com/c5c3/memcached-operator/internal/controller"
)

// managedBySelector is the label selector the cache sends for every owned kind.
const managedBySelector = "app.kubernetes.io/managed-by=memcached-operator"

// recordedRequest is one request a client sent to the API server.
type recordedRequest struct {
	method string
	path   string
	query  url.Values
	accept string
}

// requestLog records the requests of every client built from a rest.Config whose
// WrapTransport is requestLog.wrap.
type requestLog struct {
	mu       sync.Mutex
	requests []recordedRequest
}

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }

func (l *requestLog) wrap(next http.RoundTripper) http.RoundTripper {
	return roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		l.mu.Lock()
		l.requests = append(l.requests, recordedRequest{
			method: req.Method,
			path:   req.URL.Path,
			query:  req.URL.Query(),
			accept: req.Header.Get("Accept"),
		})
		l.mu.Unlock()
		return next.RoundTrip(req)
	})
}

// collectionRequests returns the recorded GET requests on the collection of
// resource: lists and watches, whose path ends in the resource name.
func (l *requestLog) collectionRequests(resource string) []recordedRequest {
	l.mu.Lock()
	defer l.mu.Unlock()
	var out []recordedRequest
	for _, req := range l.requests {
		if req.method == http.MethodGet && path.Base(req.path) == resource {
			out = append(out, req)
		}
	}
	return out
}

// namedGetCount counts the recorded GET requests for a single object of resource.
func (l *requestLog) namedGetCount(resource string) int {
	l.mu.Lock()
	defer l.mu.Unlock()
	n := 0
	for _, req := range l.requests {
		if req.method == http.MethodGet && path.Base(path.Dir(req.path)) == resource {
			n++
		}
	}
	return n
}

// portService returns a Service with one port and the given labels.
func portService(name, namespace string, labels map[string]string) *corev1.Service {
	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace, Labels: labels},
		Spec:       corev1.ServiceSpec{Ports: []corev1.ServicePort{{Name: "http", Port: 80}}},
	}
}

// createNamespace creates a namespace with a unique name and returns the name.
func createNamespace(prefix string) string {
	name := uniqueName(prefix)
	ExpectWithOffset(1, k8sClient.Create(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: name}})).To(Succeed())
	return name
}

var _ = Describe("Manager behind the production cache options", Ordered, func() {
	const (
		timeout  = 20 * time.Second
		interval = 250 * time.Millisecond
	)

	var (
		ns          string
		requests    *requestLog
		mgr         ctrl.Manager
		stopManager context.CancelFunc
		plain       *memcachedv1beta1.Memcached
		saslSecret  *corev1.Secret
	)

	BeforeAll(func() {
		ns = createNamespace("cache-scope")

		requests = &requestLog{}
		mgrCfg := rest.CopyConfig(cfg)
		mgrCfg.WrapTransport = requests.wrap
		skipNameValidation := true
		var err error
		mgr, err = ctrl.NewManager(mgrCfg, ctrl.Options{
			Scheme:                 scheme.Scheme,
			Metrics:                metricsserver.Options{BindAddress: "0"},
			HealthProbeBindAddress: "0",
			Controller:             config.Controller{SkipNameValidation: &skipNameValidation},
			Cache:                  controller.CacheOptions(map[string]cache.Config{ns: {}}),
		})
		Expect(err).NotTo(HaveOccurred())

		Expect((&controller.MemcachedReconciler{
			Client:    mgr.GetClient(),
			Scheme:    mgr.GetScheme(),
			Recorder:  mgr.GetEventRecorder("memcached-controller"),
			APIReader: mgr.GetAPIReader(),
		}).SetupWithManager(mgr)).To(Succeed())

		var mgrCtx context.Context
		mgrCtx, stopManager = context.WithCancel(ctx)
		go func() {
			defer GinkgoRecover()
			Expect(mgr.Start(mgrCtx)).To(Succeed())
		}()
		Eventually(func() bool {
			return mgr.GetCache().WaitForCacheSync(mgrCtx)
		}, timeout, interval).Should(BeTrue())
	})

	AfterAll(func() {
		stopManager()
	})

	It("reconciles a Memcached without a managed-by label", func() {
		plain = validMemcached(uniqueName("unlabeled"))
		plain.Namespace = ns
		Expect(k8sClient.Create(ctx, plain)).To(Succeed())

		Eventually(func(g Gomega) {
			g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(plain), &appsv1.StatefulSet{})).To(Succeed())
			g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(plain), &corev1.Service{})).To(Succeed())
		}, timeout, interval).Should(Succeed())
	})

	It("hashes a labeled, annotated SASL Secret and follows a change of its data", func() {
		saslSecret = newSASLSecret(uniqueName("sasl"), "initial-password")
		saslSecret.Namespace = ns
		saslSecret.Labels = map[string]string{testLabelKeyEnv: testLabelValueEnvProduction}
		saslSecret.Annotations = map[string]string{"example.com/rotated-by": "test"}
		Expect(k8sClient.Create(ctx, saslSecret)).To(Succeed())

		mc := validMemcached(uniqueName("sasl-cache"))
		mc.Namespace = ns
		mc.Spec.Security = &memcachedv1beta1.SecuritySpec{SASL: saslSpec(saslSecret.Name)}
		Expect(k8sClient.Create(ctx, mc)).To(Succeed())

		secretHash := func(g Gomega) string {
			sts := &appsv1.StatefulSet{}
			g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(mc), sts)).To(Succeed())
			return sts.Spec.Template.Annotations[controller.AnnotationSecretHash]
		}
		var firstHash string
		Eventually(func(g Gomega) {
			firstHash = secretHash(g)
			g.Expect(firstHash).NotTo(BeEmpty())
		}, timeout, interval).Should(Succeed())

		Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(saslSecret), saslSecret)).To(Succeed())
		saslSecret.Data["password-file"] = []byte("rotated-password")
		Expect(k8sClient.Update(ctx, saslSecret)).To(Succeed())

		Eventually(func(g Gomega) {
			g.Expect(secretHash(g)).NotTo(SatisfyAny(BeEmpty(), Equal(firstHash)))
		}, timeout, interval).Should(Succeed())
	})

	It("caches the Secret as metadata without labels, annotations, managedFields, ownerReferences, and finalizers", func() {
		cached := &metav1.PartialObjectMetadata{}
		cached.SetGroupVersionKind(corev1.SchemeGroupVersion.WithKind("Secret"))
		Expect(mgr.GetCache().Get(ctx, client.ObjectKeyFromObject(saslSecret), cached)).To(Succeed())

		Expect(cached.Name).To(Equal(saslSecret.Name))
		Expect(cached.Labels).To(BeEmpty())
		Expect(cached.Annotations).To(BeEmpty())
		Expect(cached.ManagedFields).To(BeEmpty())
		Expect(cached.OwnerReferences).To(BeEmpty())
		Expect(cached.Finalizers).To(BeEmpty())
	})

	It("sends the managed-by label selector on every list and watch of an owned kind and of Deployments", func() {
		for _, resource := range []string{
			testResourceStatefulSets, "services", "horizontalpodautoscalers", "poddisruptionbudgets",
			"networkpolicies", "servicemonitors", "deployments",
		} {
			collection := requests.collectionRequests(resource)
			Expect(collection).NotTo(BeEmpty(), "no list or watch for %s", resource)
			for _, req := range collection {
				Expect(req.query.Get("labelSelector")).To(Equal(managedBySelector), "%s %s?%s", req.method, req.path, req.query.Encode())
			}
		}
	})

	It("lists Memcacheds without a selector and Secrets as metadata, and reads Secrets by name", func() {
		memcacheds := requests.collectionRequests("memcacheds")
		Expect(memcacheds).NotTo(BeEmpty())
		for _, req := range memcacheds {
			Expect(req.query).NotTo(HaveKey("labelSelector"), "%s?%s", req.path, req.query.Encode())
		}

		secrets := requests.collectionRequests("secrets")
		Expect(secrets).NotTo(BeEmpty())
		for _, req := range secrets {
			Expect(req.accept).To(ContainSubstring("PartialObjectMetadata"), "%s?%s", req.path, req.query.Encode())
			Expect(req.query).NotTo(HaveKey("labelSelector"), "%s?%s", req.path, req.query.Encode())
		}

		Expect(requests.namedGetCount("secrets")).To(BeNumerically(">=", 1))
	})

	It("puts the managed-by label back on an owned Service that lost it", func() {
		svc := &corev1.Service{}
		Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(plain), svc)).To(Succeed())
		unlabeled := svc.DeepCopy()
		delete(unlabeled.Labels, testLabelKeyAppManagedBy)
		Expect(k8sClient.Patch(ctx, unlabeled, client.MergeFrom(svc))).To(Succeed())

		Eventually(func(g Gomega) {
			g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(plain), svc)).To(Succeed())
			g.Expect(svc.Labels).To(HaveKeyWithValue(testLabelKeyAppManagedBy, testLabelValueAppManagedBy))
		}, timeout, interval).Should(Succeed())
	})

	It("leaves a same-named Service it does not control alone and records a Warning event", func() {
		name := uniqueName("conflict")
		foreign := portService(name, ns, nil)
		Expect(k8sClient.Create(ctx, foreign)).To(Succeed())

		mc := validMemcached(name)
		mc.Namespace = ns
		Expect(k8sClient.Create(ctx, mc)).To(Succeed())

		Eventually(func(g Gomega) {
			var list eventsv1.EventList
			g.Expect(k8sClient.List(ctx, &list, client.InNamespace(ns))).To(Succeed())
			g.Expect(list.Items).To(ContainElement(SatisfyAll(
				HaveField("Regarding.Kind", "Memcached"),
				HaveField("Regarding.Name", mc.Name),
				HaveField("Reason", "AlreadyExists"),
				HaveField("Type", corev1.EventTypeWarning),
			)))
		}, timeout, interval).Should(Succeed())

		Consistently(func(g Gomega) {
			svc := &corev1.Service{}
			g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(foreign), svc)).To(Succeed())
			g.Expect(svc.OwnerReferences).To(BeEmpty())
			g.Expect(svc.Labels).NotTo(HaveKey(testLabelKeyAppManagedBy))
		}, 2*time.Second, interval).Should(Succeed())
	})

	It("filters a cluster-wide cache by the managed-by label for Services but not for Memcacheds", func() {
		wideNS := createNamespace("cache-wide")
		unlabeledSvc := portService(uniqueName("svc-unlabeled"), wideNS, nil)
		labeledSvc := portService(uniqueName("svc-labeled"), wideNS,
			map[string]string{testLabelKeyAppManagedBy: testLabelValueAppManagedBy})
		unlabeledMC := validMemcached(uniqueName("mc-unlabeled"))
		unlabeledMC.Namespace = wideNS
		Expect(k8sClient.Create(ctx, unlabeledSvc)).To(Succeed())
		Expect(k8sClient.Create(ctx, labeledSvc)).To(Succeed())
		Expect(k8sClient.Create(ctx, unlabeledMC)).To(Succeed())

		wide, err := cache.New(cfg, controller.CacheOptions(nil))
		Expect(err).NotTo(HaveOccurred())
		cacheCtx, stopCache := context.WithCancel(ctx)
		defer stopCache()
		go func() {
			defer GinkgoRecover()
			Expect(wide.Start(cacheCtx)).To(Succeed())
		}()

		Eventually(func(g Gomega) {
			g.Expect(wide.Get(ctx, client.ObjectKeyFromObject(labeledSvc), &corev1.Service{})).To(Succeed())
			g.Expect(wide.Get(ctx, client.ObjectKeyFromObject(unlabeledMC), &memcachedv1beta1.Memcached{})).To(Succeed())
		}, timeout, interval).Should(Succeed())

		err = wide.Get(ctx, client.ObjectKeyFromObject(unlabeledSvc), &corev1.Service{})
		Expect(apierrors.IsNotFound(err)).To(BeTrue(), "expected NotFound, got %v", err)
	})
})
