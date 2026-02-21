package controller_test

import (
	"regexp"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	memcachedv1alpha1 "github.com/c5c3/memcached-operator/api/v1alpha1"
	"github.com/c5c3/memcached-operator/internal/controller"
)

// hexHash64 matches a 64-character lowercase hex string (SHA-256).
var hexHash64 = regexp.MustCompile(`^[0-9a-f]{64}$`)

// newSASLSecret creates a Secret with SASL credential data.
func newSASLSecret(name, password string) *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "default"},
		Data:       map[string][]byte{"password-file": []byte(password)},
	}
}

// newTLSSecret creates a Secret with TLS certificate data.
func newTLSSecret(name string) *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "default"},
		Data: map[string][]byte{
			"tls.crt": []byte("cert-data"),
			"tls.key": []byte("key-data"),
		},
	}
}

// saslSpec returns a SASLSpec referencing the given Secret.
func saslSpec(secretName string) *memcachedv1alpha1.SASLSpec {
	return &memcachedv1alpha1.SASLSpec{
		Enabled:              true,
		CredentialsSecretRef: corev1.LocalObjectReference{Name: secretName},
	}
}

// tlsSpec returns a TLSSpec referencing the given Secret.
func tlsSpec(secretName string) *memcachedv1alpha1.TLSSpec {
	return &memcachedv1alpha1.TLSSpec{
		Enabled:              true,
		CertificateSecretRef: corev1.LocalObjectReference{Name: secretName},
	}
}

var _ = Describe("Secret rotation rolling restart", func() {

	Context("with SASL Secret", func() {
		It("should create Deployment with secret-hash annotation when SASL Secret exists", func() {
			secretName := uniqueName("sasl-secret")
			Expect(k8sClient.Create(ctx, newSASLSecret(secretName, "initial-password"))).To(Succeed())

			mc := validMemcached(uniqueName("rot-sasl"))
			mc.Spec.Security = &memcachedv1alpha1.SecuritySpec{SASL: saslSpec(secretName)}
			Expect(k8sClient.Create(ctx, mc)).To(Succeed())

			_, err := reconcileOnce(mc)
			Expect(err).NotTo(HaveOccurred())

			dep := fetchDeployment(mc)
			hash := dep.Spec.Template.Annotations[controller.AnnotationSecretHash]
			Expect(hash).NotTo(BeEmpty())
			Expect(hexHash64.MatchString(hash)).To(BeTrue())
		})

		It("should update secret-hash and change Deployment ResourceVersion when Secret data changes", func() {
			secretName := uniqueName("sasl-rotate")
			secret := newSASLSecret(secretName, "initial-password")
			Expect(k8sClient.Create(ctx, secret)).To(Succeed())

			mc := validMemcached(uniqueName("rot-sasl-upd"))
			mc.Spec.Security = &memcachedv1alpha1.SecuritySpec{SASL: saslSpec(secretName)}
			Expect(k8sClient.Create(ctx, mc)).To(Succeed())

			_, err := reconcileOnce(mc)
			Expect(err).NotTo(HaveOccurred())

			dep := fetchDeployment(mc)
			initialHash := dep.Spec.Template.Annotations[controller.AnnotationSecretHash]
			initialRV := dep.ResourceVersion
			Expect(initialHash).NotTo(BeEmpty())

			// Update Secret data.
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(secret), secret)).To(Succeed())
			secret.Data["password-file"] = []byte("rotated-password")
			Expect(k8sClient.Update(ctx, secret)).To(Succeed())

			_, err = reconcileOnce(mc)
			Expect(err).NotTo(HaveOccurred())

			dep = fetchDeployment(mc)
			newHash := dep.Spec.Template.Annotations[controller.AnnotationSecretHash]
			Expect(newHash).NotTo(Equal(initialHash))
			Expect(dep.ResourceVersion).NotTo(Equal(initialRV))
		})
	})

	Context("with TLS Secret", func() {
		It("should create Deployment with secret-hash annotation when TLS Secret exists", func() {
			secretName := uniqueName("tls-secret")
			Expect(k8sClient.Create(ctx, newTLSSecret(secretName))).To(Succeed())

			mc := validMemcached(uniqueName("rot-tls"))
			mc.Spec.Security = &memcachedv1alpha1.SecuritySpec{TLS: tlsSpec(secretName)}
			Expect(k8sClient.Create(ctx, mc)).To(Succeed())

			_, err := reconcileOnce(mc)
			Expect(err).NotTo(HaveOccurred())

			dep := fetchDeployment(mc)
			hash := dep.Spec.Template.Annotations[controller.AnnotationSecretHash]
			Expect(hash).NotTo(BeEmpty())
			Expect(hexHash64.MatchString(hash)).To(BeTrue())
		})
	})

	Context("with both SASL and TLS Secrets", func() {
		It("should handle both SASL and TLS Secrets together", func() {
			saslName := uniqueName("sasl-both")
			tlsName := uniqueName("tls-both")

			saslSecret := newSASLSecret(saslName, "sasl-pass")
			Expect(k8sClient.Create(ctx, saslSecret)).To(Succeed())
			Expect(k8sClient.Create(ctx, newTLSSecret(tlsName))).To(Succeed())

			mc := validMemcached(uniqueName("rot-both"))
			mc.Spec.Security = &memcachedv1alpha1.SecuritySpec{
				SASL: saslSpec(saslName),
				TLS:  tlsSpec(tlsName),
			}
			Expect(k8sClient.Create(ctx, mc)).To(Succeed())

			_, err := reconcileOnce(mc)
			Expect(err).NotTo(HaveOccurred())

			dep := fetchDeployment(mc)
			initialHash := dep.Spec.Template.Annotations[controller.AnnotationSecretHash]
			initialRV := dep.ResourceVersion
			Expect(initialHash).NotTo(BeEmpty())
			Expect(hexHash64.MatchString(initialHash)).To(BeTrue())

			// Update only SASL Secret — hash should change.
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(saslSecret), saslSecret)).To(Succeed())
			saslSecret.Data["password-file"] = []byte("rotated-sasl")
			Expect(k8sClient.Update(ctx, saslSecret)).To(Succeed())

			_, err = reconcileOnce(mc)
			Expect(err).NotTo(HaveOccurred())

			dep = fetchDeployment(mc)
			newHash := dep.Spec.Template.Annotations[controller.AnnotationSecretHash]
			Expect(newHash).NotTo(Equal(initialHash))
			Expect(dep.ResourceVersion).NotTo(Equal(initialRV))
		})
	})

	Context("without Secret references", func() {
		It("should not set secret-hash annotation when no Secrets are referenced", func() {
			mc := validMemcached(uniqueName("rot-nosecret"))
			Expect(k8sClient.Create(ctx, mc)).To(Succeed())

			_, err := reconcileOnce(mc)
			Expect(err).NotTo(HaveOccurred())

			dep := fetchDeployment(mc)
			Expect(dep.Spec.Template.Annotations).NotTo(HaveKey(controller.AnnotationSecretHash))
		})
	})

	Context("idempotency", func() {
		It("should not change Deployment ResourceVersion when Secret data is unchanged", func() {
			secretName := uniqueName("sasl-idemp")
			Expect(k8sClient.Create(ctx, newSASLSecret(secretName, "stable-password"))).To(Succeed())

			mc := validMemcached(uniqueName("rot-idemp"))
			mc.Spec.Security = &memcachedv1alpha1.SecuritySpec{SASL: saslSpec(secretName)}
			Expect(k8sClient.Create(ctx, mc)).To(Succeed())

			_, err := reconcileOnce(mc)
			Expect(err).NotTo(HaveOccurred())

			dep := fetchDeployment(mc)
			rv := dep.ResourceVersion

			// Second reconcile without changes — should be a no-op.
			_, err = reconcileOnce(mc)
			Expect(err).NotTo(HaveOccurred())

			dep = fetchDeployment(mc)
			Expect(dep.ResourceVersion).To(Equal(rv))
		})
	})
})

var _ = Describe("Missing Secret Degraded condition", func() {

	It("should set Degraded=True with SecretNotFound when Secret does not exist", func() {
		missingName := uniqueName("missing-sasl")

		mc := validMemcached(uniqueName("deg-missing"))
		mc.Spec.Security = &memcachedv1alpha1.SecuritySpec{SASL: saslSpec(missingName)}
		Expect(k8sClient.Create(ctx, mc)).To(Succeed())

		_, err := reconcileOnce(mc)
		Expect(err).NotTo(HaveOccurred())

		Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(mc), mc)).To(Succeed())

		degraded := findCondition(mc.Status.Conditions, "Degraded")
		Expect(degraded).NotTo(BeNil())
		Expect(degraded.Status).To(Equal(metav1.ConditionTrue))
		Expect(degraded.Reason).To(Equal("SecretNotFound"))
		Expect(degraded.Message).To(ContainSubstring(missingName))
	})

	It("should clear Degraded SecretNotFound when missing Secret is created", func() {
		secretName := uniqueName("missing-then-create")

		mc := validMemcached(uniqueName("deg-clear"))
		mc.Spec.Security = &memcachedv1alpha1.SecuritySpec{SASL: saslSpec(secretName)}
		Expect(k8sClient.Create(ctx, mc)).To(Succeed())

		// First reconcile — Secret missing.
		_, err := reconcileOnce(mc)
		Expect(err).NotTo(HaveOccurred())

		Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(mc), mc)).To(Succeed())
		degraded := findCondition(mc.Status.Conditions, "Degraded")
		Expect(degraded).NotTo(BeNil())
		Expect(degraded.Reason).To(Equal("SecretNotFound"))

		// Create the missing Secret.
		Expect(k8sClient.Create(ctx, newSASLSecret(secretName, "password"))).To(Succeed())

		// Second reconcile — Secret now exists.
		_, err = reconcileOnce(mc)
		Expect(err).NotTo(HaveOccurred())

		Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(mc), mc)).To(Succeed())
		degraded = findCondition(mc.Status.Conditions, "Degraded")
		Expect(degraded).NotTo(BeNil())
		Expect(degraded.Reason).NotTo(Equal("SecretNotFound"))
	})

	It("should set Degraded=True with multiple missing Secret names in message", func() {
		saslMissing := uniqueName("miss-sasl")
		tlsMissing := uniqueName("miss-tls")

		mc := validMemcached(uniqueName("deg-multi"))
		mc.Spec.Security = &memcachedv1alpha1.SecuritySpec{
			SASL: saslSpec(saslMissing),
			TLS:  tlsSpec(tlsMissing),
		}
		Expect(k8sClient.Create(ctx, mc)).To(Succeed())

		_, err := reconcileOnce(mc)
		Expect(err).NotTo(HaveOccurred())

		Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(mc), mc)).To(Succeed())

		degraded := findCondition(mc.Status.Conditions, "Degraded")
		Expect(degraded).NotTo(BeNil())
		Expect(degraded.Status).To(Equal(metav1.ConditionTrue))
		Expect(degraded.Reason).To(Equal("SecretNotFound"))
		Expect(degraded.Message).To(ContainSubstring(saslMissing))
		Expect(degraded.Message).To(ContainSubstring(tlsMissing))
	})
})

var _ = Describe("Manual restart trigger", func() {

	It("should propagate restart-trigger annotation to Deployment pod template", func() {
		secretName := uniqueName("sasl-restart")
		Expect(k8sClient.Create(ctx, newSASLSecret(secretName, "pass"))).To(Succeed())

		mc := validMemcached(uniqueName("restart-prop"))
		mc.Spec.Security = &memcachedv1alpha1.SecuritySpec{SASL: saslSpec(secretName)}
		Expect(k8sClient.Create(ctx, mc)).To(Succeed())

		_, err := reconcileOnce(mc)
		Expect(err).NotTo(HaveOccurred())

		dep := fetchDeployment(mc)
		initialRV := dep.ResourceVersion

		// Set restart-trigger annotation on the CR.
		Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(mc), mc)).To(Succeed())
		if mc.Annotations == nil {
			mc.Annotations = map[string]string{}
		}
		mc.Annotations[controller.AnnotationRestartTrigger] = "2024-01-15T10:00:00Z"
		Expect(k8sClient.Update(ctx, mc)).To(Succeed())

		_, err = reconcileOnce(mc)
		Expect(err).NotTo(HaveOccurred())

		dep = fetchDeployment(mc)
		Expect(dep.Spec.Template.Annotations[controller.AnnotationRestartTrigger]).To(Equal("2024-01-15T10:00:00Z"))
		Expect(dep.ResourceVersion).NotTo(Equal(initialRV))
	})

	It("should update Deployment when restart-trigger annotation value changes", func() {
		mc := validMemcached(uniqueName("restart-upd"))
		mc.Annotations = map[string]string{
			controller.AnnotationRestartTrigger: "2024-01-15T10:00:00Z",
		}
		Expect(k8sClient.Create(ctx, mc)).To(Succeed())

		_, err := reconcileOnce(mc)
		Expect(err).NotTo(HaveOccurred())

		dep := fetchDeployment(mc)
		Expect(dep.Spec.Template.Annotations[controller.AnnotationRestartTrigger]).To(Equal("2024-01-15T10:00:00Z"))
		rv1 := dep.ResourceVersion

		// Update the trigger value.
		Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(mc), mc)).To(Succeed())
		mc.Annotations[controller.AnnotationRestartTrigger] = "2024-01-15T11:00:00Z"
		Expect(k8sClient.Update(ctx, mc)).To(Succeed())

		_, err = reconcileOnce(mc)
		Expect(err).NotTo(HaveOccurred())

		dep = fetchDeployment(mc)
		Expect(dep.Spec.Template.Annotations[controller.AnnotationRestartTrigger]).To(Equal("2024-01-15T11:00:00Z"))
		Expect(dep.ResourceVersion).NotTo(Equal(rv1))
	})

	It("should coexist with secret-hash annotation", func() {
		secretName := uniqueName("sasl-coexist")
		Expect(k8sClient.Create(ctx, newSASLSecret(secretName, "pass"))).To(Succeed())

		mc := validMemcached(uniqueName("restart-coexist"))
		mc.Annotations = map[string]string{
			controller.AnnotationRestartTrigger: "2024-01-15T10:00:00Z",
		}
		mc.Spec.Security = &memcachedv1alpha1.SecuritySpec{SASL: saslSpec(secretName)}
		Expect(k8sClient.Create(ctx, mc)).To(Succeed())

		_, err := reconcileOnce(mc)
		Expect(err).NotTo(HaveOccurred())

		dep := fetchDeployment(mc)
		Expect(dep.Spec.Template.Annotations).To(HaveKey(controller.AnnotationSecretHash))
		Expect(dep.Spec.Template.Annotations).To(HaveKey(controller.AnnotationRestartTrigger))
		Expect(dep.Spec.Template.Annotations[controller.AnnotationSecretHash]).NotTo(BeEmpty())
		Expect(dep.Spec.Template.Annotations[controller.AnnotationRestartTrigger]).To(Equal("2024-01-15T10:00:00Z"))
	})

	It("should propagate restart-trigger on CR without Secrets", func() {
		mc := validMemcached(uniqueName("restart-nosec"))
		mc.Annotations = map[string]string{
			controller.AnnotationRestartTrigger: "2024-01-15T10:00:00Z",
		}
		Expect(k8sClient.Create(ctx, mc)).To(Succeed())

		_, err := reconcileOnce(mc)
		Expect(err).NotTo(HaveOccurred())

		dep := fetchDeployment(mc)
		Expect(dep.Spec.Template.Annotations[controller.AnnotationRestartTrigger]).To(Equal("2024-01-15T10:00:00Z"))
		Expect(dep.Spec.Template.Annotations).NotTo(HaveKey(controller.AnnotationSecretHash))
	})

	It("should not set restart-trigger annotation when CR has no restart-trigger", func() {
		mc := validMemcached(uniqueName("restart-none"))
		Expect(k8sClient.Create(ctx, mc)).To(Succeed())

		_, err := reconcileOnce(mc)
		Expect(err).NotTo(HaveOccurred())

		dep := fetchDeployment(mc)
		Expect(dep.Spec.Template.Annotations).NotTo(HaveKey(controller.AnnotationRestartTrigger))
	})
})

var _ = Describe("Secret watch filtering", func() {

	It("should only reconcile the CR referencing the updated Secret", func() {
		secretAName := uniqueName("secret-a")
		secretBName := uniqueName("secret-b")

		secretA := newSASLSecret(secretAName, "password-a")
		Expect(k8sClient.Create(ctx, secretA)).To(Succeed())
		Expect(k8sClient.Create(ctx, newSASLSecret(secretBName, "password-b"))).To(Succeed())

		mcA := validMemcached(uniqueName("filter-a"))
		mcA.Spec.Security = &memcachedv1alpha1.SecuritySpec{SASL: saslSpec(secretAName)}
		mcB := validMemcached(uniqueName("filter-b"))
		mcB.Spec.Security = &memcachedv1alpha1.SecuritySpec{SASL: saslSpec(secretBName)}

		Expect(k8sClient.Create(ctx, mcA)).To(Succeed())
		Expect(k8sClient.Create(ctx, mcB)).To(Succeed())

		_, err := reconcileOnce(mcA)
		Expect(err).NotTo(HaveOccurred())
		_, err = reconcileOnce(mcB)
		Expect(err).NotTo(HaveOccurred())

		depA := fetchDeployment(mcA)
		depB := fetchDeployment(mcB)
		hashA := depA.Spec.Template.Annotations[controller.AnnotationSecretHash]
		hashB := depB.Spec.Template.Annotations[controller.AnnotationSecretHash]
		rvB := depB.ResourceVersion

		// Update SecretA only.
		Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(secretA), secretA)).To(Succeed())
		secretA.Data["password-file"] = []byte("rotated-a")
		Expect(k8sClient.Update(ctx, secretA)).To(Succeed())

		// Reconcile CR-A — hash should change.
		_, err = reconcileOnce(mcA)
		Expect(err).NotTo(HaveOccurred())

		depA = fetchDeployment(mcA)
		Expect(depA.Spec.Template.Annotations[controller.AnnotationSecretHash]).NotTo(Equal(hashA))

		// Reconcile CR-B — hash and ResourceVersion should remain the same
		// because SecretB was not modified.
		_, err = reconcileOnce(mcB)
		Expect(err).NotTo(HaveOccurred())

		depB = fetchDeployment(mcB)
		Expect(depB.Spec.Template.Annotations[controller.AnnotationSecretHash]).To(Equal(hashB))
		Expect(depB.ResourceVersion).To(Equal(rvB))
	})

	It("should reconcile both CRs when they reference the same Secret", func() {
		sharedName := uniqueName("shared-secret")
		sharedSecret := newSASLSecret(sharedName, "shared-pass")
		Expect(k8sClient.Create(ctx, sharedSecret)).To(Succeed())

		mcA := validMemcached(uniqueName("shared-a"))
		mcA.Spec.Security = &memcachedv1alpha1.SecuritySpec{SASL: saslSpec(sharedName)}
		mcB := validMemcached(uniqueName("shared-b"))
		mcB.Spec.Security = &memcachedv1alpha1.SecuritySpec{SASL: saslSpec(sharedName)}

		Expect(k8sClient.Create(ctx, mcA)).To(Succeed())
		Expect(k8sClient.Create(ctx, mcB)).To(Succeed())

		_, err := reconcileOnce(mcA)
		Expect(err).NotTo(HaveOccurred())
		_, err = reconcileOnce(mcB)
		Expect(err).NotTo(HaveOccurred())

		depA := fetchDeployment(mcA)
		depB := fetchDeployment(mcB)
		hashA := depA.Spec.Template.Annotations[controller.AnnotationSecretHash]
		hashB := depB.Spec.Template.Annotations[controller.AnnotationSecretHash]
		// Both should have the same hash since they reference the same Secret.
		Expect(hashA).To(Equal(hashB))

		// Update the shared Secret.
		Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(sharedSecret), sharedSecret)).To(Succeed())
		sharedSecret.Data["password-file"] = []byte("rotated-shared")
		Expect(k8sClient.Update(ctx, sharedSecret)).To(Succeed())

		// Reconcile both CRs — both hashes should change.
		_, err = reconcileOnce(mcA)
		Expect(err).NotTo(HaveOccurred())
		_, err = reconcileOnce(mcB)
		Expect(err).NotTo(HaveOccurred())

		depA = fetchDeployment(mcA)
		depB = fetchDeployment(mcB)
		newHashA := depA.Spec.Template.Annotations[controller.AnnotationSecretHash]
		newHashB := depB.Spec.Template.Annotations[controller.AnnotationSecretHash]

		Expect(newHashA).NotTo(Equal(hashA))
		Expect(newHashB).NotTo(Equal(hashB))
		Expect(newHashA).To(Equal(newHashB))
	})
})
