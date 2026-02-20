// Package controller implements the reconciliation logic for the memcached-operator.
package controller

import (
	"reflect"
	"testing"

	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

	memcachedv1alpha1 "github.com/c5c3/memcached-operator/api/v1alpha1"
)

func TestNetworkPolicyEnabled(t *testing.T) {
	tests := []struct {
		name string
		mc   *memcachedv1alpha1.Memcached
		want bool
	}{
		{
			name: "nil Security",
			mc: &memcachedv1alpha1.Memcached{
				Spec: memcachedv1alpha1.MemcachedSpec{Security: nil},
			},
			want: false,
		},
		{
			name: "nil NetworkPolicy",
			mc: &memcachedv1alpha1.Memcached{
				Spec: memcachedv1alpha1.MemcachedSpec{
					Security: &memcachedv1alpha1.SecuritySpec{
						NetworkPolicy: nil,
					},
				},
			},
			want: false,
		},
		{
			name: "enabled is false",
			mc: &memcachedv1alpha1.Memcached{
				Spec: memcachedv1alpha1.MemcachedSpec{
					Security: &memcachedv1alpha1.SecuritySpec{
						NetworkPolicy: &memcachedv1alpha1.NetworkPolicySpec{Enabled: false},
					},
				},
			},
			want: false,
		},
		{
			name: "enabled is true",
			mc: &memcachedv1alpha1.Memcached{
				Spec: memcachedv1alpha1.MemcachedSpec{
					Security: &memcachedv1alpha1.SecuritySpec{
						NetworkPolicy: &memcachedv1alpha1.NetworkPolicySpec{Enabled: true},
					},
				},
			},
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := networkPolicyEnabled(tt.mc)
			if got != tt.want {
				t.Errorf("networkPolicyEnabled() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestConstructNetworkPolicy(t *testing.T) {
	tcp := corev1.ProtocolTCP

	tests := []struct {
		name      string
		mc        *memcachedv1alpha1.Memcached
		wantPorts []networkingv1.NetworkPolicyPort
		wantFrom  []networkingv1.NetworkPolicyPeer
	}{
		{
			name: "basic with only memcached port",
			mc: &memcachedv1alpha1.Memcached{
				ObjectMeta: metav1.ObjectMeta{Name: "my-cache", Namespace: "default"},
				Spec: memcachedv1alpha1.MemcachedSpec{
					Security: &memcachedv1alpha1.SecuritySpec{
						NetworkPolicy: &memcachedv1alpha1.NetworkPolicySpec{Enabled: true},
					},
				},
			},
			wantPorts: []networkingv1.NetworkPolicyPort{
				{Protocol: &tcp, Port: intstrPtr(intstr.FromInt32(11211))},
			},
			wantFrom: nil,
		},
		{
			name: "with monitoring enabled adds metrics port",
			mc: &memcachedv1alpha1.Memcached{
				ObjectMeta: metav1.ObjectMeta{Name: "my-cache", Namespace: "default"},
				Spec: memcachedv1alpha1.MemcachedSpec{
					Monitoring: &memcachedv1alpha1.MonitoringSpec{Enabled: true},
					Security: &memcachedv1alpha1.SecuritySpec{
						NetworkPolicy: &memcachedv1alpha1.NetworkPolicySpec{Enabled: true},
					},
				},
			},
			wantPorts: []networkingv1.NetworkPolicyPort{
				{Protocol: &tcp, Port: intstrPtr(intstr.FromInt32(11211))},
				{Protocol: &tcp, Port: intstrPtr(intstr.FromInt32(9150))},
			},
			wantFrom: nil,
		},
		{
			name: "with TLS enabled adds TLS port",
			mc: &memcachedv1alpha1.Memcached{
				ObjectMeta: metav1.ObjectMeta{Name: "my-cache", Namespace: "default"},
				Spec: memcachedv1alpha1.MemcachedSpec{
					Security: &memcachedv1alpha1.SecuritySpec{
						TLS:           &memcachedv1alpha1.TLSSpec{Enabled: true},
						NetworkPolicy: &memcachedv1alpha1.NetworkPolicySpec{Enabled: true},
					},
				},
			},
			wantPorts: []networkingv1.NetworkPolicyPort{
				{Protocol: &tcp, Port: intstrPtr(intstr.FromInt32(11211))},
				{Protocol: &tcp, Port: intstrPtr(intstr.FromInt32(11212))},
			},
			wantFrom: nil,
		},
		{
			name: "with both monitoring and TLS",
			mc: &memcachedv1alpha1.Memcached{
				ObjectMeta: metav1.ObjectMeta{Name: "my-cache", Namespace: "default"},
				Spec: memcachedv1alpha1.MemcachedSpec{
					Monitoring: &memcachedv1alpha1.MonitoringSpec{Enabled: true},
					Security: &memcachedv1alpha1.SecuritySpec{
						TLS:           &memcachedv1alpha1.TLSSpec{Enabled: true},
						NetworkPolicy: &memcachedv1alpha1.NetworkPolicySpec{Enabled: true},
					},
				},
			},
			wantPorts: []networkingv1.NetworkPolicyPort{
				{Protocol: &tcp, Port: intstrPtr(intstr.FromInt32(11211))},
				{Protocol: &tcp, Port: intstrPtr(intstr.FromInt32(11212))},
				{Protocol: &tcp, Port: intstrPtr(intstr.FromInt32(9150))},
			},
			wantFrom: nil,
		},
		{
			name: "with allowedSources containing namespaceSelector",
			mc: &memcachedv1alpha1.Memcached{
				ObjectMeta: metav1.ObjectMeta{Name: "my-cache", Namespace: "default"},
				Spec: memcachedv1alpha1.MemcachedSpec{
					Security: &memcachedv1alpha1.SecuritySpec{
						NetworkPolicy: &memcachedv1alpha1.NetworkPolicySpec{
							Enabled: true,
							AllowedSources: []networkingv1.NetworkPolicyPeer{
								{
									NamespaceSelector: &metav1.LabelSelector{
										MatchLabels: map[string]string{"env": "production"},
									},
								},
							},
						},
					},
				},
			},
			wantPorts: []networkingv1.NetworkPolicyPort{
				{Protocol: &tcp, Port: intstrPtr(intstr.FromInt32(11211))},
			},
			wantFrom: []networkingv1.NetworkPolicyPeer{
				{
					NamespaceSelector: &metav1.LabelSelector{
						MatchLabels: map[string]string{"env": "production"},
					},
				},
			},
		},
		{
			name: "with allowedSources containing podSelector",
			mc: &memcachedv1alpha1.Memcached{
				ObjectMeta: metav1.ObjectMeta{Name: "my-cache", Namespace: "default"},
				Spec: memcachedv1alpha1.MemcachedSpec{
					Security: &memcachedv1alpha1.SecuritySpec{
						NetworkPolicy: &memcachedv1alpha1.NetworkPolicySpec{
							Enabled: true,
							AllowedSources: []networkingv1.NetworkPolicyPeer{
								{
									PodSelector: &metav1.LabelSelector{
										MatchLabels: map[string]string{"app": "frontend"},
									},
								},
							},
						},
					},
				},
			},
			wantPorts: []networkingv1.NetworkPolicyPort{
				{Protocol: &tcp, Port: intstrPtr(intstr.FromInt32(11211))},
			},
			wantFrom: []networkingv1.NetworkPolicyPeer{
				{
					PodSelector: &metav1.LabelSelector{
						MatchLabels: map[string]string{"app": "frontend"},
					},
				},
			},
		},
		{
			name: "empty allowedSources produces no from field",
			mc: &memcachedv1alpha1.Memcached{
				ObjectMeta: metav1.ObjectMeta{Name: "my-cache", Namespace: "default"},
				Spec: memcachedv1alpha1.MemcachedSpec{
					Security: &memcachedv1alpha1.SecuritySpec{
						NetworkPolicy: &memcachedv1alpha1.NetworkPolicySpec{
							Enabled:        true,
							AllowedSources: []networkingv1.NetworkPolicyPeer{},
						},
					},
				},
			},
			wantPorts: []networkingv1.NetworkPolicyPort{
				{Protocol: &tcp, Port: intstrPtr(intstr.FromInt32(11211))},
			},
			wantFrom: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			np := &networkingv1.NetworkPolicy{}

			constructNetworkPolicy(tt.mc, np)

			// Verify policyTypes.
			if len(np.Spec.PolicyTypes) != 1 || np.Spec.PolicyTypes[0] != networkingv1.PolicyTypeIngress {
				t.Errorf("policyTypes = %v, want [Ingress]", np.Spec.PolicyTypes)
			}

			// Verify ingress rules count.
			if len(np.Spec.Ingress) != 1 {
				t.Fatalf("expected 1 ingress rule, got %d", len(np.Spec.Ingress))
			}

			rule := np.Spec.Ingress[0]

			// Verify ports.
			if len(rule.Ports) != len(tt.wantPorts) {
				t.Fatalf("expected %d ports, got %d", len(tt.wantPorts), len(rule.Ports))
			}
			for i, wantPort := range tt.wantPorts {
				gotPort := rule.Ports[i]
				if *gotPort.Protocol != *wantPort.Protocol {
					t.Errorf("port[%d] protocol = %v, want %v", i, *gotPort.Protocol, *wantPort.Protocol)
				}
				if gotPort.Port.IntValue() != wantPort.Port.IntValue() {
					t.Errorf("port[%d] = %d, want %d", i, gotPort.Port.IntValue(), wantPort.Port.IntValue())
				}
			}

			// Verify from peers.
			if tt.wantFrom == nil {
				if rule.From != nil {
					t.Errorf("expected nil From, got %v", rule.From)
				}
			} else {
				if len(rule.From) != len(tt.wantFrom) {
					t.Fatalf("expected %d from peers, got %d", len(tt.wantFrom), len(rule.From))
				}
				for i, wantPeer := range tt.wantFrom {
					gotPeer := rule.From[i]
					if wantPeer.NamespaceSelector != nil {
						if gotPeer.NamespaceSelector == nil {
							t.Errorf("from[%d] expected namespaceSelector, got nil", i)
						}
					}
					if wantPeer.PodSelector != nil {
						if gotPeer.PodSelector == nil {
							t.Errorf("from[%d] expected podSelector, got nil", i)
						}
					}
				}
			}
		})
	}
}

func TestConstructNetworkPolicy_Labels(t *testing.T) {
	mc := &memcachedv1alpha1.Memcached{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "label-test",
			Namespace: "default",
		},
		Spec: memcachedv1alpha1.MemcachedSpec{
			Security: &memcachedv1alpha1.SecuritySpec{
				NetworkPolicy: &memcachedv1alpha1.NetworkPolicySpec{Enabled: true},
			},
		},
	}
	np := &networkingv1.NetworkPolicy{}

	constructNetworkPolicy(mc, np)

	expectedLabels := labelsForMemcached("label-test")

	// Metadata labels.
	if len(np.Labels) != len(expectedLabels) {
		t.Errorf("expected %d metadata labels, got %d", len(expectedLabels), len(np.Labels))
	}
	for k, v := range expectedLabels {
		if np.Labels[k] != v {
			t.Errorf("label %q = %q, want %q", k, np.Labels[k], v)
		}
	}

	// PodSelector labels.
	if len(np.Spec.PodSelector.MatchLabels) != len(expectedLabels) {
		t.Errorf("expected %d podSelector labels, got %d", len(expectedLabels), len(np.Spec.PodSelector.MatchLabels))
	}
	for k, v := range expectedLabels {
		if np.Spec.PodSelector.MatchLabels[k] != v {
			t.Errorf("podSelector %q = %q, want %q", k, np.Spec.PodSelector.MatchLabels[k], v)
		}
	}
}

func TestConstructNetworkPolicy_InstanceScopedSelector(t *testing.T) {
	tests := []struct {
		name         string
		instanceName string
	}{
		{name: "cache-alpha", instanceName: "cache-alpha"},
		{name: "cache-beta", instanceName: "cache-beta"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mc := &memcachedv1alpha1.Memcached{
				ObjectMeta: metav1.ObjectMeta{
					Name:      tt.instanceName,
					Namespace: "default",
				},
				Spec: memcachedv1alpha1.MemcachedSpec{
					Security: &memcachedv1alpha1.SecuritySpec{
						NetworkPolicy: &memcachedv1alpha1.NetworkPolicySpec{Enabled: true},
					},
				},
			}
			np := &networkingv1.NetworkPolicy{}

			constructNetworkPolicy(mc, np)

			got := np.Spec.PodSelector.MatchLabels["app.kubernetes.io/instance"]
			if got != tt.instanceName {
				t.Errorf("podSelector app.kubernetes.io/instance = %q, want %q", got, tt.instanceName)
			}
		})
	}
}

func TestConstructNetworkPolicy_MultiplePeers(t *testing.T) {
	mc := &memcachedv1alpha1.Memcached{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "multi-peer",
			Namespace: "default",
		},
		Spec: memcachedv1alpha1.MemcachedSpec{
			Security: &memcachedv1alpha1.SecuritySpec{
				NetworkPolicy: &memcachedv1alpha1.NetworkPolicySpec{
					Enabled: true,
					AllowedSources: []networkingv1.NetworkPolicyPeer{
						{
							NamespaceSelector: &metav1.LabelSelector{
								MatchLabels: map[string]string{"env": "production"},
							},
						},
						{
							PodSelector: &metav1.LabelSelector{
								MatchLabels: map[string]string{"app": "backend"},
							},
						},
					},
				},
			},
		},
	}
	np := &networkingv1.NetworkPolicy{}

	constructNetworkPolicy(mc, np)

	// Verify we have exactly one ingress rule.
	if len(np.Spec.Ingress) != 1 {
		t.Fatalf("expected 1 ingress rule, got %d", len(np.Spec.Ingress))
	}

	rule := np.Spec.Ingress[0]

	// Verify both peers appear in the From field.
	if len(rule.From) != 2 {
		t.Fatalf("expected 2 from peers, got %d", len(rule.From))
	}

	// First peer: namespaceSelector.
	peer0 := rule.From[0]
	if peer0.NamespaceSelector == nil {
		t.Fatal("from[0] expected namespaceSelector, got nil")
	}
	if peer0.PodSelector != nil {
		t.Error("from[0] should not have podSelector")
	}
	if peer0.NamespaceSelector.MatchLabels["env"] != "production" {
		t.Errorf("from[0] namespaceSelector matchLabels[env] = %q, want %q",
			peer0.NamespaceSelector.MatchLabels["env"], "production")
	}

	// Second peer: podSelector.
	peer1 := rule.From[1]
	if peer1.PodSelector == nil {
		t.Fatal("from[1] expected podSelector, got nil")
	}
	if peer1.NamespaceSelector != nil {
		t.Error("from[1] should not have namespaceSelector")
	}
	if peer1.PodSelector.MatchLabels["app"] != "backend" {
		t.Errorf("from[1] podSelector matchLabels[app] = %q, want %q",
			peer1.PodSelector.MatchLabels["app"], "backend")
	}
}

func TestConstructNetworkPolicy_Idempotent(t *testing.T) {
	mc := &memcachedv1alpha1.Memcached{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "idem-np",
			Namespace: "production",
		},
		Spec: memcachedv1alpha1.MemcachedSpec{
			Monitoring: &memcachedv1alpha1.MonitoringSpec{Enabled: true},
			Security: &memcachedv1alpha1.SecuritySpec{
				TLS: &memcachedv1alpha1.TLSSpec{Enabled: true},
				NetworkPolicy: &memcachedv1alpha1.NetworkPolicySpec{
					Enabled: true,
					AllowedSources: []networkingv1.NetworkPolicyPeer{
						{
							NamespaceSelector: &metav1.LabelSelector{
								MatchLabels: map[string]string{"env": "prod"},
							},
						},
						{
							PodSelector: &metav1.LabelSelector{
								MatchLabels: map[string]string{"role": "client"},
							},
						},
					},
				},
			},
		},
	}

	np := &networkingv1.NetworkPolicy{}

	// First call.
	constructNetworkPolicy(mc, np)

	// Capture state after first call using deep copies.
	labelsAfterFirst := make(map[string]string, len(np.Labels))
	for k, v := range np.Labels {
		labelsAfterFirst[k] = v
	}
	podSelectorAfterFirst := np.Spec.PodSelector.DeepCopy()
	policyTypesAfterFirst := make([]networkingv1.PolicyType, len(np.Spec.PolicyTypes))
	copy(policyTypesAfterFirst, np.Spec.PolicyTypes)
	ingressAfterFirst := make([]networkingv1.NetworkPolicyIngressRule, len(np.Spec.Ingress))
	for i := range np.Spec.Ingress {
		ingressAfterFirst[i] = *np.Spec.Ingress[i].DeepCopy()
	}

	// Second call on the same object.
	constructNetworkPolicy(mc, np)

	// Verify labels are identical.
	if !reflect.DeepEqual(np.Labels, labelsAfterFirst) {
		t.Errorf("labels after second call = %v, want %v", np.Labels, labelsAfterFirst)
	}

	// Verify podSelector is identical.
	if !reflect.DeepEqual(np.Spec.PodSelector, *podSelectorAfterFirst) {
		t.Errorf("podSelector after second call = %v, want %v", np.Spec.PodSelector, *podSelectorAfterFirst)
	}

	// Verify policyTypes are identical.
	if !reflect.DeepEqual(np.Spec.PolicyTypes, policyTypesAfterFirst) {
		t.Errorf("policyTypes after second call = %v, want %v", np.Spec.PolicyTypes, policyTypesAfterFirst)
	}

	// Verify ingress rules are identical.
	if !reflect.DeepEqual(np.Spec.Ingress, ingressAfterFirst) {
		t.Errorf("ingress after second call = %v, want %v", np.Spec.Ingress, ingressAfterFirst)
	}

	// Sanity checks: verify the actual values are correct.
	expectedLabels := labelsForMemcached("idem-np")
	for k, v := range expectedLabels {
		if np.Labels[k] != v {
			t.Errorf("label %q = %q, want %q", k, np.Labels[k], v)
		}
	}

	if len(np.Spec.PolicyTypes) != 1 || np.Spec.PolicyTypes[0] != networkingv1.PolicyTypeIngress {
		t.Errorf("policyTypes = %v, want [Ingress]", np.Spec.PolicyTypes)
	}

	if len(np.Spec.Ingress) != 1 {
		t.Fatalf("expected 1 ingress rule, got %d", len(np.Spec.Ingress))
	}

	rule := np.Spec.Ingress[0]

	// With both TLS and monitoring enabled, expect 3 ports: 11211, 11212, 9150.
	if len(rule.Ports) != 3 {
		t.Fatalf("expected 3 ports, got %d", len(rule.Ports))
	}
	wantPortValues := []int32{11211, 11212, 9150}
	for i, wantVal := range wantPortValues {
		if int32(rule.Ports[i].Port.IntValue()) != wantVal {
			t.Errorf("port[%d] = %d, want %d", i, rule.Ports[i].Port.IntValue(), wantVal)
		}
	}

	// Verify 2 from peers.
	if len(rule.From) != 2 {
		t.Fatalf("expected 2 from peers, got %d", len(rule.From))
	}
	if rule.From[0].NamespaceSelector == nil {
		t.Error("from[0] expected namespaceSelector, got nil")
	} else if rule.From[0].NamespaceSelector.MatchLabels["env"] != "prod" {
		t.Errorf("from[0] namespaceSelector matchLabels[env] = %q, want %q",
			rule.From[0].NamespaceSelector.MatchLabels["env"], "prod")
	}
	if rule.From[1].PodSelector == nil {
		t.Error("from[1] expected podSelector, got nil")
	} else if rule.From[1].PodSelector.MatchLabels["role"] != "client" {
		t.Errorf("from[1] podSelector matchLabels[role] = %q, want %q",
			rule.From[1].PodSelector.MatchLabels["role"], "client")
	}
}
