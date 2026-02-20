// Package controller implements the reconciliation logic for the memcached-operator.
package controller

import (
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

	memcachedv1alpha1 "github.com/c5c3/memcached-operator/api/v1alpha1"
)

// constructNetworkPolicy sets the desired state of the NetworkPolicy based on the Memcached CR spec.
// It mutates np in-place and is designed to be called from within controllerutil.CreateOrUpdate.
func constructNetworkPolicy(mc *memcachedv1alpha1.Memcached, np *networkingv1.NetworkPolicy) {
	labels := labelsForMemcached(mc.Name)

	np.Labels = labels
	np.Spec.PodSelector = metav1.LabelSelector{
		MatchLabels: labels,
	}
	np.Spec.PolicyTypes = []networkingv1.PolicyType{networkingv1.PolicyTypeIngress}

	// Build ingress ports: always include memcached (11211).
	ports := []networkingv1.NetworkPolicyPort{
		{
			Protocol: protocolPtr(corev1.ProtocolTCP),
			Port:     intstrPtr(intstr.FromInt32(11211)),
		},
	}

	// Add TLS port (11212) when TLS is enabled.
	if mc.Spec.Security != nil && mc.Spec.Security.TLS != nil && mc.Spec.Security.TLS.Enabled {
		ports = append(ports, networkingv1.NetworkPolicyPort{
			Protocol: protocolPtr(corev1.ProtocolTCP),
			Port:     intstrPtr(intstr.FromInt32(11212)),
		})
	}

	// Add metrics port (9150) when monitoring is enabled.
	if mc.Spec.Monitoring != nil && mc.Spec.Monitoring.Enabled {
		ports = append(ports, networkingv1.NetworkPolicyPort{
			Protocol: protocolPtr(corev1.ProtocolTCP),
			Port:     intstrPtr(intstr.FromInt32(9150)),
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

// networkPolicyEnabled returns true only when NetworkPolicy creation is explicitly enabled in the CR spec.
func networkPolicyEnabled(mc *memcachedv1alpha1.Memcached) bool {
	return mc.Spec.Security != nil &&
		mc.Spec.Security.NetworkPolicy != nil &&
		mc.Spec.Security.NetworkPolicy.Enabled
}

func protocolPtr(p corev1.Protocol) *corev1.Protocol {
	return &p
}

func intstrPtr(val intstr.IntOrString) *intstr.IntOrString {
	return &val
}
