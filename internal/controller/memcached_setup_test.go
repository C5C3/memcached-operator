package controller_test

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"

	"github.com/c5c3/memcached-operator/internal/controller"
)

var _ = Describe("Memcached Controller SetupWithManager", func() {

	Context("When configuring watches (REQ-001, REQ-002, REQ-003, REQ-004, REQ-005)", func() {

		It("should set up the controller and start the manager with watches for owned Deployments and Services", func() {
			mgr, err := ctrl.NewManager(cfg, ctrl.Options{
				Scheme:                 scheme.Scheme,
				Metrics:                metricsserver.Options{BindAddress: "0"},
				HealthProbeBindAddress: "0",
			})
			Expect(err).NotTo(HaveOccurred())

			reconciler := &controller.MemcachedReconciler{
				Client: mgr.GetClient(),
				Scheme: mgr.GetScheme(),
			}
			err = reconciler.SetupWithManager(mgr)
			Expect(err).NotTo(HaveOccurred())

			// Starting the manager validates that all watches are properly
			// registered and the informers can be created for the watched types
			// (Memcached CR, Deployments, Services).
			mgrCtx, mgrCancel := context.WithCancel(context.Background())
			go func() {
				defer GinkgoRecover()
				err := mgr.Start(mgrCtx)
				Expect(err).NotTo(HaveOccurred())
			}()

			// Allow the manager to start and register informers.
			Eventually(func() bool {
				return mgr.GetCache().WaitForCacheSync(mgrCtx)
			}, 10*time.Second, 250*time.Millisecond).Should(BeTrue())

			mgrCancel()
		})
	})
})
