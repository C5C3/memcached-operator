// Package controller implements the reconciliation logic for the memcached-operator.
package controller

import (
	"context"
	"errors"
	"regexp"
	"strings"
	"sync/atomic"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	memcachedv1beta1 "github.com/c5c3/memcached-operator/api/v1beta1"
)

// ---------------------------------------------------------------------------
// computeSecretHash tests
// ---------------------------------------------------------------------------

func TestComputeSecretHash_Determinism(t *testing.T) {
	s := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "s1"},
		Data:       map[string][]byte{testSecretDataKey: []byte("value")},
	}
	h1 := computeSecretHash(s)
	h2 := computeSecretHash(s)
	if h1 != h2 {
		t.Errorf("expected deterministic hash, got %q and %q", h1, h2)
	}
}

func TestComputeSecretHash_HexFormat(t *testing.T) {
	s := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "s1"},
		Data:       map[string][]byte{testSecretDataKey: []byte("value")},
	}
	h := computeSecretHash(s)
	if len(h) != 64 {
		t.Fatalf("expected 64-char hex string, got length %d: %q", len(h), h)
	}
	if !regexp.MustCompile(`^[0-9a-f]{64}$`).MatchString(h) {
		t.Errorf("expected lowercase hex string, got %q", h)
	}
}

func TestComputeSecretHash_OrderIndependence(t *testing.T) {
	a := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "a"},
		Data:       map[string][]byte{"k": []byte("v")},
	}
	b := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "b"},
		Data:       map[string][]byte{"k": []byte("v")},
	}
	h1 := computeSecretHash(a, b)
	h2 := computeSecretHash(b, a)
	if h1 != h2 {
		t.Errorf("hash should be order-independent: %q vs %q", h1, h2)
	}
}

func TestComputeSecretHash_KeyOrderIndependence(t *testing.T) {
	s1 := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "s"},
		Data:       map[string][]byte{"a": []byte("1"), "b": []byte("2")},
	}
	s2 := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "s"},
		Data:       map[string][]byte{"b": []byte("2"), "a": []byte("1")},
	}
	h1 := computeSecretHash(s1)
	h2 := computeSecretHash(s2)
	if h1 != h2 {
		t.Errorf("hash should be key-order-independent: %q vs %q", h1, h2)
	}
}

func TestComputeSecretHash_EmptyInput(t *testing.T) {
	h := computeSecretHash()
	if h != "" {
		t.Errorf("expected empty string for no args, got %q", h)
	}
}

func TestComputeSecretHash_NilData(t *testing.T) {
	s := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "s"},
	}
	h := computeSecretHash(s)
	if h != "" {
		t.Errorf("expected empty string for nil data, got %q", h)
	}
}

func TestComputeSecretHash_EmptyDataMap(t *testing.T) {
	s := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "s"},
		Data:       map[string][]byte{},
	}
	h := computeSecretHash(s)
	if h != "" {
		t.Errorf("expected empty string for empty data map, got %q", h)
	}
}

func TestComputeSecretHash_DataChange(t *testing.T) {
	s1 := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "s"},
		Data:       map[string][]byte{testSecretDataKey: []byte("value1")},
	}
	s2 := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "s"},
		Data:       map[string][]byte{testSecretDataKey: []byte("value2")},
	}
	h1 := computeSecretHash(s1)
	h2 := computeSecretHash(s2)
	if h1 == h2 {
		t.Errorf("expected different hashes for different data, both got %q", h1)
	}
}

// ---------------------------------------------------------------------------
// fetchReferencedSecrets tests
// ---------------------------------------------------------------------------

func TestFetchReferencedSecrets_BothExist(t *testing.T) {
	saslSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: testSASLSecretName, Namespace: testDefaultNamespace},
		Data:       map[string][]byte{"password-file": []byte("pass")},
	}
	tlsSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: testTLSSecretName, Namespace: testDefaultNamespace},
		Data:       map[string][]byte{testTLSCertKey: []byte("cert")},
	}
	c := fake.NewClientBuilder().WithScheme(testScheme()).WithObjects(saslSecret, tlsSecret).Build()

	mc := &memcachedv1beta1.Memcached{
		ObjectMeta: metav1.ObjectMeta{Name: "mc", Namespace: testDefaultNamespace},
		Spec: memcachedv1beta1.MemcachedSpec{
			Security: &memcachedv1beta1.SecuritySpec{
				SASL: &memcachedv1beta1.SASLSpec{
					Enabled:              true,
					CredentialsSecretRef: corev1.LocalObjectReference{Name: testSASLSecretName},
				},
				TLS: &memcachedv1beta1.TLSSpec{
					Enabled:              true,
					CertificateSecretRef: corev1.LocalObjectReference{Name: testTLSSecretName},
				},
			},
		},
	}

	found, missing, err := fetchReferencedSecrets(context.Background(), c, mc)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(found) != 2 {
		t.Errorf("expected 2 found secrets, got %d", len(found))
	}
	if len(missing) != 0 {
		t.Errorf("expected no missing secrets, got %v", missing)
	}
}

func TestFetchReferencedSecrets_SASLMissing(t *testing.T) {
	tlsSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: testTLSSecretName, Namespace: testDefaultNamespace},
		Data:       map[string][]byte{testTLSCertKey: []byte("cert")},
	}
	c := fake.NewClientBuilder().WithScheme(testScheme()).WithObjects(tlsSecret).Build()

	mc := &memcachedv1beta1.Memcached{
		ObjectMeta: metav1.ObjectMeta{Name: "mc", Namespace: testDefaultNamespace},
		Spec: memcachedv1beta1.MemcachedSpec{
			Security: &memcachedv1beta1.SecuritySpec{
				SASL: &memcachedv1beta1.SASLSpec{
					Enabled:              true,
					CredentialsSecretRef: corev1.LocalObjectReference{Name: testSASLSecretName},
				},
				TLS: &memcachedv1beta1.TLSSpec{
					Enabled:              true,
					CertificateSecretRef: corev1.LocalObjectReference{Name: testTLSSecretName},
				},
			},
		},
	}

	found, missing, err := fetchReferencedSecrets(context.Background(), c, mc)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(found) != 1 {
		t.Errorf("expected 1 found secret, got %d", len(found))
	}
	if len(missing) != 1 || missing[0] != testSASLSecretName {
		t.Errorf("expected missing=[sasl-secret], got %v", missing)
	}
}

func TestFetchReferencedSecrets_TLSMissing(t *testing.T) {
	saslSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: testSASLSecretName, Namespace: testDefaultNamespace},
		Data:       map[string][]byte{"password-file": []byte("pass")},
	}
	c := fake.NewClientBuilder().WithScheme(testScheme()).WithObjects(saslSecret).Build()

	mc := &memcachedv1beta1.Memcached{
		ObjectMeta: metav1.ObjectMeta{Name: "mc", Namespace: testDefaultNamespace},
		Spec: memcachedv1beta1.MemcachedSpec{
			Security: &memcachedv1beta1.SecuritySpec{
				SASL: &memcachedv1beta1.SASLSpec{
					Enabled:              true,
					CredentialsSecretRef: corev1.LocalObjectReference{Name: testSASLSecretName},
				},
				TLS: &memcachedv1beta1.TLSSpec{
					Enabled:              true,
					CertificateSecretRef: corev1.LocalObjectReference{Name: testTLSSecretName},
				},
			},
		},
	}

	found, missing, err := fetchReferencedSecrets(context.Background(), c, mc)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(found) != 1 {
		t.Errorf("expected 1 found secret, got %d", len(found))
	}
	if len(missing) != 1 || missing[0] != testTLSSecretName {
		t.Errorf("expected missing=[tls-secret], got %v", missing)
	}
}

func TestFetchReferencedSecrets_NilSecuritySpec(t *testing.T) {
	c := fake.NewClientBuilder().WithScheme(testScheme()).Build()

	mc := &memcachedv1beta1.Memcached{
		ObjectMeta: metav1.ObjectMeta{Name: "mc", Namespace: testDefaultNamespace},
		Spec:       memcachedv1beta1.MemcachedSpec{},
	}

	found, missing, err := fetchReferencedSecrets(context.Background(), c, mc)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if found != nil {
		t.Errorf("expected nil found, got %v", found)
	}
	if missing != nil {
		t.Errorf("expected nil missing, got %v", missing)
	}
}

func TestFetchReferencedSecrets_NeitherEnabled(t *testing.T) {
	c := fake.NewClientBuilder().WithScheme(testScheme()).Build()

	mc := &memcachedv1beta1.Memcached{
		ObjectMeta: metav1.ObjectMeta{Name: "mc", Namespace: testDefaultNamespace},
		Spec: memcachedv1beta1.MemcachedSpec{
			Security: &memcachedv1beta1.SecuritySpec{
				SASL: &memcachedv1beta1.SASLSpec{Enabled: false},
				TLS:  &memcachedv1beta1.TLSSpec{Enabled: false},
			},
		},
	}

	found, missing, err := fetchReferencedSecrets(context.Background(), c, mc)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if found != nil {
		t.Errorf("expected nil found, got %v", found)
	}
	if missing != nil {
		t.Errorf("expected nil missing, got %v", missing)
	}
}

func TestFetchReferencedSecrets_Dedup(t *testing.T) {
	sharedSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: testSASLAndTLSSecretName, Namespace: testDefaultNamespace},
		Data:       map[string][]byte{testSecretDataKey: []byte("val")},
	}
	c := fake.NewClientBuilder().WithScheme(testScheme()).WithObjects(sharedSecret).Build()

	mc := &memcachedv1beta1.Memcached{
		ObjectMeta: metav1.ObjectMeta{Name: "mc", Namespace: testDefaultNamespace},
		Spec: memcachedv1beta1.MemcachedSpec{
			Security: &memcachedv1beta1.SecuritySpec{
				SASL: &memcachedv1beta1.SASLSpec{
					Enabled:              true,
					CredentialsSecretRef: corev1.LocalObjectReference{Name: testSASLAndTLSSecretName},
				},
				TLS: &memcachedv1beta1.TLSSpec{
					Enabled:              true,
					CertificateSecretRef: corev1.LocalObjectReference{Name: testSASLAndTLSSecretName},
				},
			},
		},
	}

	found, missing, err := fetchReferencedSecrets(context.Background(), c, mc)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(found) != 1 {
		t.Errorf("expected 1 found secret (deduped), got %d", len(found))
	}
	if len(missing) != 0 {
		t.Errorf("expected no missing secrets, got %v", missing)
	}
}

func TestFetchReferencedSecrets_DedupMissing(t *testing.T) {
	c := fake.NewClientBuilder().WithScheme(testScheme()).Build()

	mc := &memcachedv1beta1.Memcached{
		ObjectMeta: metav1.ObjectMeta{Name: "mc", Namespace: testDefaultNamespace},
		Spec: memcachedv1beta1.MemcachedSpec{
			Security: &memcachedv1beta1.SecuritySpec{
				SASL: &memcachedv1beta1.SASLSpec{
					Enabled:              true,
					CredentialsSecretRef: corev1.LocalObjectReference{Name: testSASLAndTLSSecretName},
				},
				TLS: &memcachedv1beta1.TLSSpec{
					Enabled:              true,
					CertificateSecretRef: corev1.LocalObjectReference{Name: testSASLAndTLSSecretName},
				},
			},
		},
	}

	found, missing, err := fetchReferencedSecrets(context.Background(), c, mc)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(found) != 0 {
		t.Errorf("expected 0 found secrets, got %d", len(found))
	}
	if len(missing) != 1 || missing[0] != testSASLAndTLSSecretName {
		t.Errorf("expected missing=[shared-secret] (deduped), got %v", missing)
	}
}

// saslMemcached returns a Memcached in the default namespace with SASL enabled and
// its credentials read from the Secret secretName.
func saslMemcached(secretName string) *memcachedv1beta1.Memcached {
	return &memcachedv1beta1.Memcached{
		ObjectMeta: metav1.ObjectMeta{Name: "mc", Namespace: testDefaultNamespace},
		Spec: memcachedv1beta1.MemcachedSpec{
			Security: &memcachedv1beta1.SecuritySpec{
				SASL: &memcachedv1beta1.SASLSpec{
					Enabled:              true,
					CredentialsSecretRef: corev1.LocalObjectReference{Name: secretName},
				},
			},
		},
	}
}

func TestFetchReferencedSecrets_EmptyRefName(t *testing.T) {
	var gets atomic.Int32
	c := interceptor.NewClient(newFakeClient(), interceptor.Funcs{
		Get: func(ctx context.Context, c client.WithWatch, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
			gets.Add(1)
			return c.Get(ctx, key, obj, opts...)
		},
	})

	found, missing, err := fetchReferencedSecrets(context.Background(), c, saslMemcached(""))
	if found != nil || missing != nil || err != nil {
		t.Errorf("expected nil, nil, nil; got %v, %v, %v", found, missing, err)
	}
	if n := gets.Load(); n != 0 {
		t.Errorf("expected no read for an empty Secret name, got %d", n)
	}
}

func TestFetchReferencedSecrets_ReadErrorIsReturned(t *testing.T) {
	tests := []struct {
		name string
		err  error
	}{
		{name: "service unavailable", err: apierrors.NewServiceUnavailable("x")},
		{name: "forbidden", err: apierrors.NewForbidden(schema.GroupResource{Resource: "secrets"}, testSASLSecretName, errors.New("denied"))},
		{name: "context canceled", err: context.Canceled},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := interceptor.NewClient(newFakeClient(), interceptor.Funcs{
				Get: func(context.Context, client.WithWatch, client.ObjectKey, client.Object, ...client.GetOption) error {
					return tt.err
				},
			})

			found, missing, err := fetchReferencedSecrets(context.Background(), c, saslMemcached(testSASLSecretName))
			if found != nil {
				t.Errorf("expected nil found, got %v", found)
			}
			if missing != nil {
				t.Errorf("expected nil missing, got %v", missing)
			}
			if !errors.Is(err, tt.err) {
				t.Fatalf("expected an error wrapping %v, got %v", tt.err, err)
			}
			if !strings.HasPrefix(err.Error(), "fetching Secret default/") {
				t.Errorf("expected the message to start with %q, got %q", "fetching Secret default/", err.Error())
			}
		})
	}
}

// ---------------------------------------------------------------------------
// reconcileStatefulSet Secret read tests
// ---------------------------------------------------------------------------

func TestReconcileStatefulSet_SecretReadErrorLeavesStatefulSetUntouched(t *testing.T) {
	mc := saslMemcached(testSASLSecretName)
	mc.UID = testOwnerUID
	stored := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{Name: statefulSetName(mc.Name), Namespace: mc.Namespace},
		Spec: appsv1.StatefulSetSpec{
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Annotations: map[string]string{AnnotationSecretHash: testSecretHash}},
			},
		},
	}
	c := newFakeClient(mc, stored)
	before := &appsv1.StatefulSet{}
	if err := c.Get(context.Background(), client.ObjectKeyFromObject(stored), before); err != nil {
		t.Fatalf("failed to get stored StatefulSet: %v", err)
	}

	unavailable := apierrors.NewServiceUnavailable("x")
	r := newTestReconciler(c)
	r.APIReader = interceptor.NewClient(newFakeClient(), interceptor.Funcs{
		Get: func(ctx context.Context, c client.WithWatch, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
			if _, ok := obj.(*corev1.Secret); ok {
				return unavailable
			}
			return c.Get(ctx, key, obj, opts...)
		},
	})

	missing, err := r.reconcileStatefulSet(context.Background(), mc)
	if !errors.Is(err, unavailable) {
		t.Fatalf("expected the Secret read error, got %v", err)
	}
	if missing != nil {
		t.Errorf("expected nil missing, got %v", missing)
	}

	after := &appsv1.StatefulSet{}
	if err := c.Get(context.Background(), client.ObjectKeyFromObject(stored), after); err != nil {
		t.Fatalf("failed to get StatefulSet: %v", err)
	}
	if after.ResourceVersion != before.ResourceVersion {
		t.Errorf("expected resourceVersion %q, got %q", before.ResourceVersion, after.ResourceVersion)
	}
	if got := after.Spec.Template.Annotations[AnnotationSecretHash]; got != testSecretHash {
		t.Errorf("expected secret-hash annotation %q, got %q", testSecretHash, got)
	}
}

func TestReconcileStatefulSet_ReadsSecretsThroughAPIReader(t *testing.T) {
	mc := saslMemcached(testSASLSecretName)
	mc.UID = testOwnerUID
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: testSASLSecretName, Namespace: testDefaultNamespace},
		Data:       map[string][]byte{testPasswordFileKey: []byte("pass")},
	}

	c := interceptor.NewClient(newFakeClient(mc), interceptor.Funcs{
		Get: func(ctx context.Context, c client.WithWatch, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
			if _, ok := obj.(*corev1.Secret); ok {
				t.Errorf("Secret %s read through Client, expected APIReader", key)
				return apierrors.NewNotFound(schema.GroupResource{Resource: "secrets"}, key.Name)
			}
			return c.Get(ctx, key, obj, opts...)
		},
	})
	r := newTestReconciler(c)
	r.APIReader = newFakeClient(secret)

	if _, err := r.reconcileStatefulSet(context.Background(), mc); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	sts := &appsv1.StatefulSet{}
	if err := c.Get(context.Background(), client.ObjectKey{Name: statefulSetName(mc.Name), Namespace: mc.Namespace}, sts); err != nil {
		t.Fatalf("failed to get StatefulSet: %v", err)
	}
	if sts.Spec.Template.Annotations[AnnotationSecretHash] == "" {
		t.Error("expected a non-empty secret-hash annotation")
	}
}

func TestLiveReader_FallsBackToClient(t *testing.T) {
	c := newFakeClient()
	r := newTestReconciler(c)
	if r.liveReader() != c {
		t.Error("expected liveReader to return Client when APIReader is nil")
	}

	apiReader := newFakeClient()
	r.APIReader = apiReader
	if r.liveReader() != apiReader {
		t.Error("expected liveReader to return APIReader when it is set")
	}
}

// ---------------------------------------------------------------------------
// mapSecretToMemcached tests
// ---------------------------------------------------------------------------

func TestMapSecretToMemcached_SASLRef(t *testing.T) {
	mc := &memcachedv1beta1.Memcached{
		ObjectMeta: metav1.ObjectMeta{Name: testInstanceNameMC1, Namespace: testDefaultNamespace},
		Spec: memcachedv1beta1.MemcachedSpec{
			Security: &memcachedv1beta1.SecuritySpec{
				SASL: &memcachedv1beta1.SASLSpec{
					Enabled:              true,
					CredentialsSecretRef: corev1.LocalObjectReference{Name: testReferencedSecretName},
				},
			},
		},
	}
	c := fake.NewClientBuilder().WithScheme(testScheme()).WithObjects(mc).Build()

	mapFn := mapSecretToMemcached(c)
	secret := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: testReferencedSecretName, Namespace: testDefaultNamespace}}
	requests := mapFn(context.Background(), secret)

	if len(requests) != 1 {
		t.Fatalf("expected 1 request, got %d", len(requests))
	}
	if requests[0].Name != testInstanceNameMC1 || requests[0].Namespace != testDefaultNamespace {
		t.Errorf("unexpected request: %v", requests[0])
	}
}

func TestMapSecretToMemcached_TLSRef(t *testing.T) {
	mc := &memcachedv1beta1.Memcached{
		ObjectMeta: metav1.ObjectMeta{Name: testInstanceNameMC1, Namespace: testDefaultNamespace},
		Spec: memcachedv1beta1.MemcachedSpec{
			Security: &memcachedv1beta1.SecuritySpec{
				TLS: &memcachedv1beta1.TLSSpec{
					Enabled:              true,
					CertificateSecretRef: corev1.LocalObjectReference{Name: "my-tls"},
				},
			},
		},
	}
	c := fake.NewClientBuilder().WithScheme(testScheme()).WithObjects(mc).Build()

	mapFn := mapSecretToMemcached(c)
	secret := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "my-tls", Namespace: testDefaultNamespace}}
	requests := mapFn(context.Background(), secret)

	if len(requests) != 1 {
		t.Fatalf("expected 1 request, got %d", len(requests))
	}
	if requests[0].Name != testInstanceNameMC1 || requests[0].Namespace != testDefaultNamespace {
		t.Errorf("unexpected request: %v", requests[0])
	}
}

func TestMapSecretToMemcached_Unreferenced(t *testing.T) {
	mc := &memcachedv1beta1.Memcached{
		ObjectMeta: metav1.ObjectMeta{Name: testInstanceNameMC1, Namespace: testDefaultNamespace},
		Spec: memcachedv1beta1.MemcachedSpec{
			Security: &memcachedv1beta1.SecuritySpec{
				SASL: &memcachedv1beta1.SASLSpec{
					Enabled:              true,
					CredentialsSecretRef: corev1.LocalObjectReference{Name: "other-secret"},
				},
			},
		},
	}
	c := fake.NewClientBuilder().WithScheme(testScheme()).WithObjects(mc).Build()

	mapFn := mapSecretToMemcached(c)
	secret := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "unrelated", Namespace: testDefaultNamespace}}
	requests := mapFn(context.Background(), secret)

	if len(requests) != 0 {
		t.Errorf("expected 0 requests for unreferenced secret, got %d", len(requests))
	}
}

func TestMapSecretToMemcached_NamespaceScoping(t *testing.T) {
	mc := &memcachedv1beta1.Memcached{
		ObjectMeta: metav1.ObjectMeta{Name: testInstanceNameMC1, Namespace: "other-ns"},
		Spec: memcachedv1beta1.MemcachedSpec{
			Security: &memcachedv1beta1.SecuritySpec{
				SASL: &memcachedv1beta1.SASLSpec{
					Enabled:              true,
					CredentialsSecretRef: corev1.LocalObjectReference{Name: testReferencedSecretName},
				},
			},
		},
	}
	c := fake.NewClientBuilder().WithScheme(testScheme()).WithObjects(mc).Build()

	mapFn := mapSecretToMemcached(c)
	secret := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: testReferencedSecretName, Namespace: testDefaultNamespace}}
	requests := mapFn(context.Background(), secret)

	if len(requests) != 0 {
		t.Errorf("expected 0 requests for different namespace, got %d", len(requests))
	}
}

func TestMapSecretToMemcached_NilSecuritySpec(t *testing.T) {
	mc := &memcachedv1beta1.Memcached{
		ObjectMeta: metav1.ObjectMeta{Name: testInstanceNameMC1, Namespace: testDefaultNamespace},
		Spec:       memcachedv1beta1.MemcachedSpec{},
	}
	c := fake.NewClientBuilder().WithScheme(testScheme()).WithObjects(mc).Build()

	mapFn := mapSecretToMemcached(c)
	secret := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: testReferencedSecretName, Namespace: testDefaultNamespace}}
	requests := mapFn(context.Background(), secret)

	if len(requests) != 0 {
		t.Errorf("expected 0 requests for nil security spec, got %d", len(requests))
	}
}

func TestMapSecretToMemcached_MultipleCRs(t *testing.T) {
	mc1 := &memcachedv1beta1.Memcached{
		ObjectMeta: metav1.ObjectMeta{Name: testInstanceNameMC1, Namespace: testDefaultNamespace},
		Spec: memcachedv1beta1.MemcachedSpec{
			Security: &memcachedv1beta1.SecuritySpec{
				SASL: &memcachedv1beta1.SASLSpec{
					Enabled:              true,
					CredentialsSecretRef: corev1.LocalObjectReference{Name: testMultiInstanceSecretName},
				},
			},
		},
	}
	mc2 := &memcachedv1beta1.Memcached{
		ObjectMeta: metav1.ObjectMeta{Name: "mc2", Namespace: testDefaultNamespace},
		Spec: memcachedv1beta1.MemcachedSpec{
			Security: &memcachedv1beta1.SecuritySpec{
				TLS: &memcachedv1beta1.TLSSpec{
					Enabled:              true,
					CertificateSecretRef: corev1.LocalObjectReference{Name: testMultiInstanceSecretName},
				},
			},
		},
	}
	c := fake.NewClientBuilder().WithScheme(testScheme()).WithObjects(mc1, mc2).Build()

	mapFn := mapSecretToMemcached(c)
	secret := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: testMultiInstanceSecretName, Namespace: testDefaultNamespace}}
	requests := mapFn(context.Background(), secret)

	if len(requests) != 2 {
		t.Fatalf("expected 2 requests for multiple matching CRs, got %d", len(requests))
	}

	names := map[string]bool{}
	for _, r := range requests {
		names[r.Name] = true
	}
	if !names["mc1"] || !names["mc2"] {
		t.Errorf("expected requests for mc1 and mc2, got %v", requests)
	}
}
