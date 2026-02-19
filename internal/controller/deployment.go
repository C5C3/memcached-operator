// Package controller implements the reconciliation logic for the memcached-operator.
package controller

import (
	"fmt"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

	memcachedv1alpha1 "github.com/c5c3/memcached-operator/api/v1alpha1"
)

// labelsForMemcached returns the standard Kubernetes recommended labels for a Memcached resource.
func labelsForMemcached(name string) map[string]string {
	return map[string]string{
		"app.kubernetes.io/name":       "memcached",
		"app.kubernetes.io/instance":   name,
		"app.kubernetes.io/managed-by": "memcached-operator",
	}
}

// buildMemcachedArgs constructs the command-line arguments for a memcached process
// based on the provided configuration. If config is nil, defaults are used.
func buildMemcachedArgs(config *memcachedv1alpha1.MemcachedConfig) []string {
	// Apply defaults when config is nil.
	if config == nil {
		config = &memcachedv1alpha1.MemcachedConfig{}
	}

	maxMemoryMB := config.MaxMemoryMB
	if maxMemoryMB == 0 {
		maxMemoryMB = 64
	}

	maxConnections := config.MaxConnections
	if maxConnections == 0 {
		maxConnections = 1024
	}

	threads := config.Threads
	if threads == 0 {
		threads = 4
	}

	maxItemSize := config.MaxItemSize
	if maxItemSize == "" {
		maxItemSize = "1m"
	}

	args := []string{
		"-m", fmt.Sprintf("%d", maxMemoryMB),
		"-c", fmt.Sprintf("%d", maxConnections),
		"-t", fmt.Sprintf("%d", threads),
		"-I", maxItemSize,
	}

	// Verbosity: 1 → "-v", 2 → "-vv".
	switch config.Verbosity {
	case 1:
		args = append(args, "-v")
	case 2:
		args = append(args, "-vv")
	}

	// Append extra args at the end.
	if len(config.ExtraArgs) > 0 {
		args = append(args, config.ExtraArgs...)
	}

	return args
}

// constructDeployment sets the desired state of the Deployment based on the Memcached CR spec.
// It mutates dep in-place and is designed to be called from within controllerutil.CreateOrUpdate.
func constructDeployment(mc *memcachedv1alpha1.Memcached, dep *appsv1.Deployment) {
	labels := labelsForMemcached(mc.Name)

	// Defaults.
	replicas := int32(1)
	if mc.Spec.Replicas != nil {
		replicas = *mc.Spec.Replicas
	}
	image := "memcached:1.6"
	if mc.Spec.Image != nil {
		image = *mc.Spec.Image
	}

	args := buildMemcachedArgs(mc.Spec.Memcached)

	var resources corev1.ResourceRequirements
	if mc.Spec.Resources != nil {
		resources = *mc.Spec.Resources
	}

	maxSurge := intstr.FromInt32(1)
	maxUnavailable := intstr.FromInt32(0)

	dep.Labels = labels
	dep.Spec = appsv1.DeploymentSpec{
		Replicas: &replicas,
		Selector: &metav1.LabelSelector{
			MatchLabels: labels,
		},
		Strategy: appsv1.DeploymentStrategy{
			Type: appsv1.RollingUpdateDeploymentStrategyType,
			RollingUpdate: &appsv1.RollingUpdateDeployment{
				MaxSurge:       &maxSurge,
				MaxUnavailable: &maxUnavailable,
			},
		},
		Template: corev1.PodTemplateSpec{
			ObjectMeta: metav1.ObjectMeta{
				Labels: labels,
			},
			Spec: corev1.PodSpec{
				Containers: []corev1.Container{
					{
						Name:      "memcached",
						Image:     image,
						Args:      args,
						Resources: resources,
						Ports: []corev1.ContainerPort{
							{
								Name:          "memcached",
								ContainerPort: 11211,
								Protocol:      corev1.ProtocolTCP,
							},
						},
						LivenessProbe: &corev1.Probe{
							ProbeHandler: corev1.ProbeHandler{
								TCPSocket: &corev1.TCPSocketAction{
									Port: intstr.FromString("memcached"),
								},
							},
							InitialDelaySeconds: 10,
							PeriodSeconds:       10,
						},
						ReadinessProbe: &corev1.Probe{
							ProbeHandler: corev1.ProbeHandler{
								TCPSocket: &corev1.TCPSocketAction{
									Port: intstr.FromString("memcached"),
								},
							},
							InitialDelaySeconds: 5,
							PeriodSeconds:       5,
						},
					},
				},
			},
		},
	}
}
