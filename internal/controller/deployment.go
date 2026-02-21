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
// based on the provided configuration and optional SASL and TLS specs.
// If config is nil, defaults are used. When SASL is enabled, the -Y flag is
// appended pointing to the mounted password file. When TLS is enabled, the -Z flag
// and ssl_chain_cert/ssl_key options are appended.
func buildMemcachedArgs(config *memcachedv1alpha1.MemcachedConfig, sasl *memcachedv1alpha1.SASLSpec, tls *memcachedv1alpha1.TLSSpec) []string {
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

	// SASL authentication: -Y <password-file>.
	if sasl != nil && sasl.Enabled {
		args = append(args, "-Y", saslMountPath+"/password-file")
	}

	// TLS encryption: -Z, -o ssl_chain_cert, -o ssl_key, optionally -o ssl_ca_cert.
	if tls != nil && tls.Enabled {
		args = append(args,
			"-Z",
			"-o", "ssl_chain_cert="+tlsMountPath+"/tls.crt",
			"-o", "ssl_key="+tlsMountPath+"/tls.key",
		)
		if tls.EnableClientCert {
			args = append(args, "-o", "ssl_ca_cert="+tlsMountPath+"/ca.crt")
		}
	}

	// Append extra args at the end.
	if len(config.ExtraArgs) > 0 {
		args = append(args, config.ExtraArgs...)
	}

	return args
}

// buildAntiAffinity returns a PodAntiAffinity-based Affinity for the given Memcached CR,
// or nil if no anti-affinity is configured.
func buildAntiAffinity(mc *memcachedv1alpha1.Memcached) *corev1.Affinity {
	if mc.Spec.HighAvailability == nil || mc.Spec.HighAvailability.AntiAffinityPreset == nil {
		return nil
	}

	labelSelector := &metav1.LabelSelector{
		MatchLabels: map[string]string{
			"app.kubernetes.io/name":     "memcached",
			"app.kubernetes.io/instance": mc.Name,
		},
	}

	switch *mc.Spec.HighAvailability.AntiAffinityPreset {
	case memcachedv1alpha1.AntiAffinityPresetSoft:
		return &corev1.Affinity{
			PodAntiAffinity: &corev1.PodAntiAffinity{
				PreferredDuringSchedulingIgnoredDuringExecution: []corev1.WeightedPodAffinityTerm{
					{
						Weight: 100,
						PodAffinityTerm: corev1.PodAffinityTerm{
							TopologyKey:   "kubernetes.io/hostname",
							LabelSelector: labelSelector,
						},
					},
				},
			},
		}
	case memcachedv1alpha1.AntiAffinityPresetHard:
		return &corev1.Affinity{
			PodAntiAffinity: &corev1.PodAntiAffinity{
				RequiredDuringSchedulingIgnoredDuringExecution: []corev1.PodAffinityTerm{
					{
						TopologyKey:   "kubernetes.io/hostname",
						LabelSelector: labelSelector,
					},
				},
			},
		}
	default:
		return nil
	}
}

// buildTopologySpreadConstraints returns the topology spread constraints from the Memcached CR,
// or nil if none are configured.
func buildTopologySpreadConstraints(mc *memcachedv1alpha1.Memcached) []corev1.TopologySpreadConstraint {
	if mc.Spec.HighAvailability == nil || len(mc.Spec.HighAvailability.TopologySpreadConstraints) == 0 {
		return nil
	}
	return mc.Spec.HighAvailability.TopologySpreadConstraints
}

// buildGracefulShutdown returns the Lifecycle hook and terminationGracePeriodSeconds for graceful
// shutdown, or (nil, nil) if graceful shutdown is not enabled.
func buildGracefulShutdown(mc *memcachedv1alpha1.Memcached) (*corev1.Lifecycle, *int64) {
	if mc.Spec.HighAvailability == nil || mc.Spec.HighAvailability.GracefulShutdown == nil ||
		!mc.Spec.HighAvailability.GracefulShutdown.Enabled {
		return nil, nil
	}

	gs := mc.Spec.HighAvailability.GracefulShutdown

	preStopDelaySeconds := gs.PreStopDelaySeconds
	if preStopDelaySeconds == 0 {
		preStopDelaySeconds = 10
	}

	terminationGracePeriod := gs.TerminationGracePeriodSeconds
	if terminationGracePeriod == 0 {
		terminationGracePeriod = 30
	}

	lifecycle := &corev1.Lifecycle{
		PreStop: &corev1.LifecycleHandler{
			Exec: &corev1.ExecAction{
				Command: []string{"sleep", fmt.Sprintf("%d", preStopDelaySeconds)},
			},
		},
	}

	return lifecycle, &terminationGracePeriod
}

// buildExporterContainer returns a memcached-exporter sidecar container when monitoring is enabled,
// or nil if monitoring is disabled or not configured.
func buildExporterContainer(mc *memcachedv1alpha1.Memcached) *corev1.Container {
	if mc.Spec.Monitoring == nil || !mc.Spec.Monitoring.Enabled {
		return nil
	}

	image := "prom/memcached-exporter:v0.15.4"
	if mc.Spec.Monitoring.ExporterImage != nil {
		image = *mc.Spec.Monitoring.ExporterImage
	}

	var resources corev1.ResourceRequirements
	if mc.Spec.Monitoring.ExporterResources != nil {
		resources = *mc.Spec.Monitoring.ExporterResources
	}

	return &corev1.Container{
		Name:      "exporter",
		Image:     image,
		Resources: resources,
		Ports: []corev1.ContainerPort{
			{
				Name:          "metrics",
				ContainerPort: 9150,
				Protocol:      corev1.ProtocolTCP,
			},
		},
	}
}

// AnnotationSecretHash is the Pod template annotation key for the computed secret hash.
const AnnotationSecretHash = "memcached.c5c3.io/secret-hash"

// AnnotationRestartTrigger is the Pod template annotation key for the manual restart trigger.
const AnnotationRestartTrigger = "memcached.c5c3.io/restart-trigger"

// saslVolumeName is the name used for the SASL credentials volume.
const saslVolumeName = "sasl-credentials"

// saslMountPath is the path where SASL credentials are mounted in the container.
const saslMountPath = "/etc/memcached/sasl"

// buildSASLVolume returns a Volume that projects the SASL credentials Secret,
// or nil if SASL is not enabled.
func buildSASLVolume(mc *memcachedv1alpha1.Memcached) *corev1.Volume {
	if mc.Spec.Security == nil || mc.Spec.Security.SASL == nil || !mc.Spec.Security.SASL.Enabled {
		return nil
	}
	return &corev1.Volume{
		Name: saslVolumeName,
		VolumeSource: corev1.VolumeSource{
			Secret: &corev1.SecretVolumeSource{
				SecretName: mc.Spec.Security.SASL.CredentialsSecretRef.Name,
				Items: []corev1.KeyToPath{
					{Key: "password-file", Path: "password-file"},
				},
			},
		},
	}
}

// buildSASLVolumeMount returns a VolumeMount for the SASL credentials,
// or nil if SASL is not enabled.
func buildSASLVolumeMount(mc *memcachedv1alpha1.Memcached) *corev1.VolumeMount {
	if mc.Spec.Security == nil || mc.Spec.Security.SASL == nil || !mc.Spec.Security.SASL.Enabled {
		return nil
	}
	return &corev1.VolumeMount{
		Name:      saslVolumeName,
		MountPath: saslMountPath,
		ReadOnly:  true,
	}
}

// tlsVolumeName is the name used for the TLS certificates volume.
const tlsVolumeName = "tls-certificates"

// tlsMountPath is the path where TLS certificates are mounted in the container.
const tlsMountPath = "/etc/memcached/tls"

// tlsPortName is the name used for the TLS container and service port.
const tlsPortName = "memcached-tls"

// buildTLSVolume returns a Volume that projects the TLS certificate Secret,
// or nil if TLS is not enabled.
func buildTLSVolume(mc *memcachedv1alpha1.Memcached) *corev1.Volume {
	if mc.Spec.Security == nil || mc.Spec.Security.TLS == nil || !mc.Spec.Security.TLS.Enabled {
		return nil
	}
	items := []corev1.KeyToPath{
		{Key: "tls.crt", Path: "tls.crt"},
		{Key: "tls.key", Path: "tls.key"},
	}
	if mc.Spec.Security.TLS.EnableClientCert {
		items = append(items, corev1.KeyToPath{Key: "ca.crt", Path: "ca.crt"})
	}
	return &corev1.Volume{
		Name: tlsVolumeName,
		VolumeSource: corev1.VolumeSource{
			Secret: &corev1.SecretVolumeSource{
				SecretName: mc.Spec.Security.TLS.CertificateSecretRef.Name,
				Items:      items,
			},
		},
	}
}

// buildTLSVolumeMount returns a VolumeMount for the TLS certificates,
// or nil if TLS is not enabled.
func buildTLSVolumeMount(mc *memcachedv1alpha1.Memcached) *corev1.VolumeMount {
	if mc.Spec.Security == nil || mc.Spec.Security.TLS == nil || !mc.Spec.Security.TLS.Enabled {
		return nil
	}
	return &corev1.VolumeMount{
		Name:      tlsVolumeName,
		MountPath: tlsMountPath,
		ReadOnly:  true,
	}
}

// buildPodSecurityContext returns the PodSecurityContext from the Memcached CR,
// or nil if no pod security context is configured.
func buildPodSecurityContext(mc *memcachedv1alpha1.Memcached) *corev1.PodSecurityContext {
	if mc.Spec.Security == nil || mc.Spec.Security.PodSecurityContext == nil {
		return nil
	}
	return mc.Spec.Security.PodSecurityContext
}

// buildContainerSecurityContext returns the container SecurityContext from the Memcached CR,
// or nil if no container security context is configured.
func buildContainerSecurityContext(mc *memcachedv1alpha1.Memcached) *corev1.SecurityContext {
	if mc.Spec.Security == nil || mc.Spec.Security.ContainerSecurityContext == nil {
		return nil
	}
	return mc.Spec.Security.ContainerSecurityContext
}

// constructDeployment sets the desired state of the Deployment based on the Memcached CR spec.
// It mutates dep in-place and is designed to be called from within controllerutil.CreateOrUpdate.
// secretHash and restartTrigger are propagated as Pod template annotations to trigger rolling updates.
func constructDeployment(mc *memcachedv1alpha1.Memcached, dep *appsv1.Deployment, secretHash, restartTrigger string) {
	labels := labelsForMemcached(mc.Name)

	// Determine replicas: nil when HPA is active (let HPA control scaling),
	// otherwise use spec value or default.
	var replicasPtr *int32
	if !hpaEnabled(mc) {
		replicas := int32(1)
		if mc.Spec.Replicas != nil {
			replicas = *mc.Spec.Replicas
		}
		replicasPtr = &replicas
	}
	image := "memcached:1.6"
	if mc.Spec.Image != nil {
		image = *mc.Spec.Image
	}

	// Resolve SASL and TLS specs for args and volume/mount helpers.
	var saslSpec *memcachedv1alpha1.SASLSpec
	var tlsSpec *memcachedv1alpha1.TLSSpec
	if mc.Spec.Security != nil {
		saslSpec = mc.Spec.Security.SASL
		tlsSpec = mc.Spec.Security.TLS
	}

	args := buildMemcachedArgs(mc.Spec.Memcached, saslSpec, tlsSpec)

	var resources corev1.ResourceRequirements
	if mc.Spec.Resources != nil {
		resources = *mc.Spec.Resources
	}

	maxSurge := intstr.FromInt32(1)
	maxUnavailable := intstr.FromInt32(0)

	affinity := buildAntiAffinity(mc)
	topologySpreadConstraints := buildTopologySpreadConstraints(mc)
	lifecycle, terminationGracePeriodSeconds := buildGracefulShutdown(mc)
	podSecurityContext := buildPodSecurityContext(mc)
	containerSecurityContext := buildContainerSecurityContext(mc)

	var volumeMounts []corev1.VolumeMount
	if vm := buildSASLVolumeMount(mc); vm != nil {
		volumeMounts = append(volumeMounts, *vm)
	}
	if vm := buildTLSVolumeMount(mc); vm != nil {
		volumeMounts = append(volumeMounts, *vm)
	}

	ports := []corev1.ContainerPort{
		{
			Name:          "memcached",
			ContainerPort: 11211,
			Protocol:      corev1.ProtocolTCP,
		},
	}
	if tlsSpec != nil && tlsSpec.Enabled {
		ports = append(ports, corev1.ContainerPort{
			Name:          tlsPortName,
			ContainerPort: 11212,
			Protocol:      corev1.ProtocolTCP,
		})
	}

	memcachedContainer := corev1.Container{
		Name:            "memcached",
		Image:           image,
		Args:            args,
		Resources:       resources,
		Lifecycle:       lifecycle,
		SecurityContext: containerSecurityContext,
		VolumeMounts:    volumeMounts,
		Ports:           ports,
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
	}

	containers := []corev1.Container{memcachedContainer}
	if exporterContainer := buildExporterContainer(mc); exporterContainer != nil {
		exporterContainer.SecurityContext = containerSecurityContext
		containers = append(containers, *exporterContainer)
	}

	var volumes []corev1.Volume
	if v := buildSASLVolume(mc); v != nil {
		volumes = append(volumes, *v)
	}
	if v := buildTLSVolume(mc); v != nil {
		volumes = append(volumes, *v)
	}

	podAnnotations := buildPodAnnotations(secretHash, restartTrigger)

	dep.Labels = labels
	dep.Spec = appsv1.DeploymentSpec{
		Replicas: replicasPtr,
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
				Labels:      labels,
				Annotations: podAnnotations,
			},
			Spec: corev1.PodSpec{
				Affinity:                      affinity,
				TopologySpreadConstraints:     topologySpreadConstraints,
				TerminationGracePeriodSeconds: terminationGracePeriodSeconds,
				SecurityContext:               podSecurityContext,
				Containers:                    containers,
				Volumes:                       volumes,
			},
		},
	}
}

// buildPodAnnotations returns Pod template annotations for secret-hash and restart-trigger.
// Returns nil if both values are empty.
func buildPodAnnotations(secretHash, restartTrigger string) map[string]string {
	if secretHash == "" && restartTrigger == "" {
		return nil
	}
	annotations := make(map[string]string)
	if secretHash != "" {
		annotations[AnnotationSecretHash] = secretHash
	}
	if restartTrigger != "" {
		annotations[AnnotationRestartTrigger] = restartTrigger
	}
	return annotations
}
