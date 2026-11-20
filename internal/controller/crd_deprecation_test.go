package controller_test

import (
	"context"
	"sync"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"

	memcachedv1alpha1 "github.com/c5c3/memcached-operator/api/v1alpha1"
	memcachedv1beta1 "github.com/c5c3/memcached-operator/api/v1beta1"
)

// testDeprecationWarning is the deprecationWarning the CRD sets on v1alpha1.
const testDeprecationWarning = "memcached.c5c3.io/v1alpha1 Memcached is deprecated; use memcached.c5c3.io/v1beta1 Memcached"

// warningRecorder records the warning headers the API server returns.
type warningRecorder struct {
	mu       sync.Mutex
	warnings []string
}

func (r *warningRecorder) HandleWarningHeaderWithContext(_ context.Context, _ int, _ string, text string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.warnings = append(r.warnings, text)
}

func (r *warningRecorder) recorded() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.warnings...)
}

// newWarningRecordingClient returns a client that reads from the API server
// directly and records every warning header instead of logging it.
func newWarningRecordingClient() (client.Client, *warningRecorder) {
	recorder := &warningRecorder{}
	recordingCfg := rest.CopyConfig(cfg)
	recordingCfg.WarningHandler = nil
	recordingCfg.WarningHandlerWithContext = recorder
	c, err := client.New(recordingCfg, client.Options{Scheme: scheme.Scheme})
	ExpectWithOffset(1, err).NotTo(HaveOccurred())
	return c, recorder
}

var _ = Describe("v1alpha1 deprecation warning", func() {

	It("returns the deprecation warning to a client that uses v1alpha1", func() {
		c, recorder := newWarningRecordingClient()
		list := &memcachedv1alpha1.MemcachedList{}
		Expect(c.List(ctx, list, client.InNamespace(uniqueName("deprecation")))).To(Succeed())
		Expect(recorder.recorded()).To(ConsistOf(testDeprecationWarning))
	})

	It("returns no warning to a client that uses v1beta1", func() {
		c, recorder := newWarningRecordingClient()
		list := &memcachedv1beta1.MemcachedList{}
		Expect(c.List(ctx, list, client.InNamespace(uniqueName("deprecation")))).To(Succeed())
		Expect(recorder.recorded()).To(BeEmpty())
	})
})
