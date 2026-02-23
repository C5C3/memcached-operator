// Package controller implements the reconciliation logic for the memcached-operator.
package controller

import (
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

	memcachedv1beta1 "github.com/c5c3/memcached-operator/api/v1beta1"
)

// constructNetworkPolicy sets the desired state of the NetworkPolicy based on the Memcached CR spec.
// It mutates np in-place and is designed to be called from within controllerutil.CreateOrUpdate.
func constructNetworkPolicy(mc *memcachedv1beta1.Memcached, np *networkingv1.NetworkPolicy) {
	labels := labelsForMemcached(mc.Name)

	np.Labels = labels
	np.Spec.PodSelector = metav1.LabelSelector{
		MatchLabels: labels,
	}
	np.Spec.PolicyTypes = []networkingv1.PolicyType{networkingv1.PolicyTypeIngress}

	// Build ingress ports: always include memcached.
	ports := []networkingv1.NetworkPolicyPort{
		{
			Protocol: protocolPtr(corev1.ProtocolTCP),
			Port:     intstrPtr(intstr.FromInt32(PortMemcached)),
		},
	}

	// Add TLS port when TLS is enabled.
	if mc.IsTLSEnabled() {
		ports = append(ports, networkingv1.NetworkPolicyPort{
			Protocol: protocolPtr(corev1.ProtocolTCP),
			Port:     intstrPtr(intstr.FromInt32(PortMemcachedTLS)),
		})
	}

	// Add metrics port when monitoring is enabled.
	if mc.IsMonitoringEnabled() {
		ports = append(ports, networkingv1.NetworkPolicyPort{
			Protocol: protocolPtr(corev1.ProtocolTCP),
			Port:     intstrPtr(intstr.FromInt32(PortMetrics)),
		})
	}

	// Build the single ingress rule.
	ingressRule := networkingv1.NetworkPolicyIngressRule{
		Ports: ports,
	}

	// Set from peers only when allowedSources is non-empty.
	if mc.Spec.Security != nil && mc.Spec.Security.NetworkPolicy != nil &&
		len(mc.Spec.Security.NetworkPolicy.AllowedSources) > 0 {
		ingressRule.From = mc.Spec.Security.NetworkPolicy.AllowedSources
	}

	np.Spec.Ingress = []networkingv1.NetworkPolicyIngressRule{ingressRule}
}

func protocolPtr(p corev1.Protocol) *corev1.Protocol {
	return &p
}

func intstrPtr(val intstr.IntOrString) *intstr.IntOrString {
	return &val
}
