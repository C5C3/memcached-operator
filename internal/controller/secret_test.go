// Package controller implements the reconciliation logic for the memcached-operator.
package controller

import (
	"context"
	"regexp"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	memcachedv1beta1 "github.com/c5c3/memcached-operator/api/v1beta1"
)

// ---------------------------------------------------------------------------
// computeSecretHash tests
// ---------------------------------------------------------------------------

func TestComputeSecretHash_Determinism(t *testing.T) {
	s := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "s1"},
		Data:       map[string][]byte{"key": []byte("value")},
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
		Data:       map[string][]byte{"key": []byte("value")},
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
		Data:       map[string][]byte{"key": []byte("value1")},
	}
	s2 := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "s"},
		Data:       map[string][]byte{"key": []byte("value2")},
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
		ObjectMeta: metav1.ObjectMeta{Name: "sasl-secret", Namespace: "default"},
		Data:       map[string][]byte{"password-file": []byte("pass")},
	}
	tlsSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "tls-secret", Namespace: "default"},
		Data:       map[string][]byte{"tls.crt": []byte("cert")},
	}
	c := fake.NewClientBuilder().WithScheme(testScheme()).WithObjects(saslSecret, tlsSecret).Build()

	mc := &memcachedv1beta1.Memcached{
		ObjectMeta: metav1.ObjectMeta{Name: "mc", Namespace: "default"},
		Spec: memcachedv1beta1.MemcachedSpec{
			Security: &memcachedv1beta1.SecuritySpec{
				SASL: &memcachedv1beta1.SASLSpec{
					Enabled:              true,
					CredentialsSecretRef: corev1.LocalObjectReference{Name: "sasl-secret"},
				},
				TLS: &memcachedv1beta1.TLSSpec{
					Enabled:              true,
					CertificateSecretRef: corev1.LocalObjectReference{Name: "tls-secret"},
				},
			},
		},
	}

	found, missing := fetchReferencedSecrets(context.Background(), c, mc)
	if len(found) != 2 {
		t.Errorf("expected 2 found secrets, got %d", len(found))
	}
	if len(missing) != 0 {
		t.Errorf("expected no missing secrets, got %v", missing)
	}
}

func TestFetchReferencedSecrets_SASLMissing(t *testing.T) {
	tlsSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "tls-secret", Namespace: "default"},
		Data:       map[string][]byte{"tls.crt": []byte("cert")},
	}
	c := fake.NewClientBuilder().WithScheme(testScheme()).WithObjects(tlsSecret).Build()

	mc := &memcachedv1beta1.Memcached{
		ObjectMeta: metav1.ObjectMeta{Name: "mc", Namespace: "default"},
		Spec: memcachedv1beta1.MemcachedSpec{
			Security: &memcachedv1beta1.SecuritySpec{
				SASL: &memcachedv1beta1.SASLSpec{
					Enabled:              true,
					CredentialsSecretRef: corev1.LocalObjectReference{Name: "sasl-secret"},
				},
				TLS: &memcachedv1beta1.TLSSpec{
					Enabled:              true,
					CertificateSecretRef: corev1.LocalObjectReference{Name: "tls-secret"},
				},
			},
		},
	}

	found, missing := fetchReferencedSecrets(context.Background(), c, mc)
	if len(found) != 1 {
		t.Errorf("expected 1 found secret, got %d", len(found))
	}
	if len(missing) != 1 || missing[0] != "sasl-secret" {
		t.Errorf("expected missing=[sasl-secret], got %v", missing)
	}
}

func TestFetchReferencedSecrets_TLSMissing(t *testing.T) {
	saslSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "sasl-secret", Namespace: "default"},
		Data:       map[string][]byte{"password-file": []byte("pass")},
	}
	c := fake.NewClientBuilder().WithScheme(testScheme()).WithObjects(saslSecret).Build()

	mc := &memcachedv1beta1.Memcached{
		ObjectMeta: metav1.ObjectMeta{Name: "mc", Namespace: "default"},
		Spec: memcachedv1beta1.MemcachedSpec{
			Security: &memcachedv1beta1.SecuritySpec{
				SASL: &memcachedv1beta1.SASLSpec{
					Enabled:              true,
					CredentialsSecretRef: corev1.LocalObjectReference{Name: "sasl-secret"},
				},
				TLS: &memcachedv1beta1.TLSSpec{
					Enabled:              true,
					CertificateSecretRef: corev1.LocalObjectReference{Name: "tls-secret"},
				},
			},
		},
	}

	found, missing := fetchReferencedSecrets(context.Background(), c, mc)
	if len(found) != 1 {
		t.Errorf("expected 1 found secret, got %d", len(found))
	}
	if len(missing) != 1 || missing[0] != "tls-secret" {
		t.Errorf("expected missing=[tls-secret], got %v", missing)
	}
}

func TestFetchReferencedSecrets_NilSecuritySpec(t *testing.T) {
	c := fake.NewClientBuilder().WithScheme(testScheme()).Build()

	mc := &memcachedv1beta1.Memcached{
		ObjectMeta: metav1.ObjectMeta{Name: "mc", Namespace: "default"},
		Spec:       memcachedv1beta1.MemcachedSpec{},
	}

	found, missing := fetchReferencedSecrets(context.Background(), c, mc)
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
		ObjectMeta: metav1.ObjectMeta{Name: "mc", Namespace: "default"},
		Spec: memcachedv1beta1.MemcachedSpec{
			Security: &memcachedv1beta1.SecuritySpec{
				SASL: &memcachedv1beta1.SASLSpec{Enabled: false},
				TLS:  &memcachedv1beta1.TLSSpec{Enabled: false},
			},
		},
	}

	found, missing := fetchReferencedSecrets(context.Background(), c, mc)
	if found != nil {
		t.Errorf("expected nil found, got %v", found)
	}
	if missing != nil {
		t.Errorf("expected nil missing, got %v", missing)
	}
}

func TestFetchReferencedSecrets_Dedup(t *testing.T) {
	sharedSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "shared-secret", Namespace: "default"},
		Data:       map[string][]byte{"key": []byte("val")},
	}
	c := fake.NewClientBuilder().WithScheme(testScheme()).WithObjects(sharedSecret).Build()

	mc := &memcachedv1beta1.Memcached{
		ObjectMeta: metav1.ObjectMeta{Name: "mc", Namespace: "default"},
		Spec: memcachedv1beta1.MemcachedSpec{
			Security: &memcachedv1beta1.SecuritySpec{
				SASL: &memcachedv1beta1.SASLSpec{
					Enabled:              true,
					CredentialsSecretRef: corev1.LocalObjectReference{Name: "shared-secret"},
				},
				TLS: &memcachedv1beta1.TLSSpec{
					Enabled:              true,
					CertificateSecretRef: corev1.LocalObjectReference{Name: "shared-secret"},
				},
			},
		},
	}

	found, missing := fetchReferencedSecrets(context.Background(), c, mc)
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
		ObjectMeta: metav1.ObjectMeta{Name: "mc", Namespace: "default"},
		Spec: memcachedv1beta1.MemcachedSpec{
			Security: &memcachedv1beta1.SecuritySpec{
				SASL: &memcachedv1beta1.SASLSpec{
					Enabled:              true,
					CredentialsSecretRef: corev1.LocalObjectReference{Name: "shared-secret"},
				},
				TLS: &memcachedv1beta1.TLSSpec{
					Enabled:              true,
					CertificateSecretRef: corev1.LocalObjectReference{Name: "shared-secret"},
				},
			},
		},
	}

	found, missing := fetchReferencedSecrets(context.Background(), c, mc)
	if len(found) != 0 {
		t.Errorf("expected 0 found secrets, got %d", len(found))
	}
	if len(missing) != 1 || missing[0] != "shared-secret" {
		t.Errorf("expected missing=[shared-secret] (deduped), got %v", missing)
	}
}

// ---------------------------------------------------------------------------
// mapSecretToMemcached tests
// ---------------------------------------------------------------------------

func TestMapSecretToMemcached_SASLRef(t *testing.T) {
	mc := &memcachedv1beta1.Memcached{
		ObjectMeta: metav1.ObjectMeta{Name: "mc1", Namespace: "default"},
		Spec: memcachedv1beta1.MemcachedSpec{
			Security: &memcachedv1beta1.SecuritySpec{
				SASL: &memcachedv1beta1.SASLSpec{
					Enabled:              true,
					CredentialsSecretRef: corev1.LocalObjectReference{Name: "my-secret"},
				},
			},
		},
	}
	c := fake.NewClientBuilder().WithScheme(testScheme()).WithObjects(mc).Build()

	mapFn := mapSecretToMemcached(c)
	secret := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "my-secret", Namespace: "default"}}
	requests := mapFn(context.Background(), secret)

	if len(requests) != 1 {
		t.Fatalf("expected 1 request, got %d", len(requests))
	}
	if requests[0].Name != "mc1" || requests[0].Namespace != "default" {
		t.Errorf("unexpected request: %v", requests[0])
	}
}

func TestMapSecretToMemcached_TLSRef(t *testing.T) {
	mc := &memcachedv1beta1.Memcached{
		ObjectMeta: metav1.ObjectMeta{Name: "mc1", Namespace: "default"},
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
	secret := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "my-tls", Namespace: "default"}}
	requests := mapFn(context.Background(), secret)

	if len(requests) != 1 {
		t.Fatalf("expected 1 request, got %d", len(requests))
	}
	if requests[0].Name != "mc1" || requests[0].Namespace != "default" {
		t.Errorf("unexpected request: %v", requests[0])
	}
}

func TestMapSecretToMemcached_Unreferenced(t *testing.T) {
	mc := &memcachedv1beta1.Memcached{
		ObjectMeta: metav1.ObjectMeta{Name: "mc1", Namespace: "default"},
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
	secret := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "unrelated", Namespace: "default"}}
	requests := mapFn(context.Background(), secret)

	if len(requests) != 0 {
		t.Errorf("expected 0 requests for unreferenced secret, got %d", len(requests))
	}
}

func TestMapSecretToMemcached_NamespaceScoping(t *testing.T) {
	mc := &memcachedv1beta1.Memcached{
		ObjectMeta: metav1.ObjectMeta{Name: "mc1", Namespace: "other-ns"},
		Spec: memcachedv1beta1.MemcachedSpec{
			Security: &memcachedv1beta1.SecuritySpec{
				SASL: &memcachedv1beta1.SASLSpec{
					Enabled:              true,
					CredentialsSecretRef: corev1.LocalObjectReference{Name: "my-secret"},
				},
			},
		},
	}
	c := fake.NewClientBuilder().WithScheme(testScheme()).WithObjects(mc).Build()

	mapFn := mapSecretToMemcached(c)
	secret := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "my-secret", Namespace: "default"}}
	requests := mapFn(context.Background(), secret)

	if len(requests) != 0 {
		t.Errorf("expected 0 requests for different namespace, got %d", len(requests))
	}
}

func TestMapSecretToMemcached_NilSecuritySpec(t *testing.T) {
	mc := &memcachedv1beta1.Memcached{
		ObjectMeta: metav1.ObjectMeta{Name: "mc1", Namespace: "default"},
		Spec:       memcachedv1beta1.MemcachedSpec{},
	}
	c := fake.NewClientBuilder().WithScheme(testScheme()).WithObjects(mc).Build()

	mapFn := mapSecretToMemcached(c)
	secret := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "my-secret", Namespace: "default"}}
	requests := mapFn(context.Background(), secret)

	if len(requests) != 0 {
		t.Errorf("expected 0 requests for nil security spec, got %d", len(requests))
	}
}

func TestMapSecretToMemcached_MultipleCRs(t *testing.T) {
	mc1 := &memcachedv1beta1.Memcached{
		ObjectMeta: metav1.ObjectMeta{Name: "mc1", Namespace: "default"},
		Spec: memcachedv1beta1.MemcachedSpec{
			Security: &memcachedv1beta1.SecuritySpec{
				SASL: &memcachedv1beta1.SASLSpec{
					Enabled:              true,
					CredentialsSecretRef: corev1.LocalObjectReference{Name: "shared"},
				},
			},
		},
	}
	mc2 := &memcachedv1beta1.Memcached{
		ObjectMeta: metav1.ObjectMeta{Name: "mc2", Namespace: "default"},
		Spec: memcachedv1beta1.MemcachedSpec{
			Security: &memcachedv1beta1.SecuritySpec{
				TLS: &memcachedv1beta1.TLSSpec{
					Enabled:              true,
					CertificateSecretRef: corev1.LocalObjectReference{Name: "shared"},
				},
			},
		},
	}
	c := fake.NewClientBuilder().WithScheme(testScheme()).WithObjects(mc1, mc2).Build()

	mapFn := mapSecretToMemcached(c)
	secret := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "shared", Namespace: "default"}}
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
