// Package controller implements the reconciliation logic for the memcached-operator.
package controller

import (
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

	memcachedv1alpha1 "github.com/c5c3/memcached-operator/api/v1alpha1"
)

// constructService sets the desired state of the headless Service based on the Memcached CR spec.
// It mutates svc in-place and is designed to be called from within controllerutil.CreateOrUpdate.
func constructService(mc *memcachedv1alpha1.Memcached, svc *corev1.Service) {
	labels := labelsForMemcached(mc.Name)

	svc.Labels = labels

	// Apply custom annotations from spec.service.annotations if present.
	if mc.Spec.Service != nil && len(mc.Spec.Service.Annotations) > 0 {
		svc.Annotations = mc.Spec.Service.Annotations
	} else {
		svc.Annotations = nil
	}

	svc.Spec.ClusterIP = corev1.ClusterIPNone
	svc.Spec.Selector = labels
	ports := []corev1.ServicePort{
		{
			Name:       "memcached",
			Port:       11211,
			TargetPort: intstr.FromString("memcached"),
			Protocol:   corev1.ProtocolTCP,
		},
	}

	if mc.Spec.Monitoring != nil && mc.Spec.Monitoring.Enabled {
		ports = append(ports, corev1.ServicePort{
			Name:       "metrics",
			Port:       9150,
			TargetPort: intstr.FromString("metrics"),
			Protocol:   corev1.ProtocolTCP,
		})
	}

	svc.Spec.Ports = ports
}
