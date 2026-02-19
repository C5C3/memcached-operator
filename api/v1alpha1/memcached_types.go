// Package v1alpha1 contains API Schema definitions for the memcached v1alpha1 API group.
package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

// AntiAffinityPreset defines the type of pod anti-affinity to apply.
// +kubebuilder:validation:Enum=soft;hard
type AntiAffinityPreset string

const (
	// AntiAffinityPresetSoft uses preferredDuringSchedulingIgnoredDuringExecution.
	AntiAffinityPresetSoft AntiAffinityPreset = "soft"
	// AntiAffinityPresetHard uses requiredDuringSchedulingIgnoredDuringExecution.
	AntiAffinityPresetHard AntiAffinityPreset = "hard"
)

// MemcachedConfig defines the Memcached server configuration parameters.
type MemcachedConfig struct {
	// MaxMemoryMB is the maximum memory for item storage in megabytes (-m flag).
	// +kubebuilder:validation:Minimum=16
	// +kubebuilder:validation:Maximum=65536
	// +kubebuilder:default=64
	// +optional
	MaxMemoryMB int32 `json:"maxMemoryMB,omitempty"`

	// MaxConnections is the maximum number of simultaneous connections (-c flag).
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=65536
	// +kubebuilder:default=1024
	// +optional
	MaxConnections int32 `json:"maxConnections,omitempty"`

	// Threads is the number of threads to use (-t flag).
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=128
	// +kubebuilder:default=4
	// +optional
	Threads int32 `json:"threads,omitempty"`

	// MaxItemSize is the maximum size of an item (-I flag, e.g. "1m", "2m", "512k").
	// +kubebuilder:validation:Pattern=`^[0-9]+(k|m)$`
	// +kubebuilder:default="1m"
	// +optional
	MaxItemSize string `json:"maxItemSize,omitempty"`

	// Verbosity controls the logging verbosity level (0=none, 1=-v, 2=-vv).
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Maximum=2
	// +kubebuilder:default=0
	// +optional
	Verbosity int32 `json:"verbosity,omitempty"`

	// ExtraArgs are additional command-line arguments passed to the Memcached process.
	// +optional
	ExtraArgs []string `json:"extraArgs,omitempty"`
}

// HighAvailabilitySpec defines high-availability settings for Memcached pods.
type HighAvailabilitySpec struct {
	// AntiAffinityPreset controls pod anti-affinity scheduling.
	// "soft" uses preferredDuringSchedulingIgnoredDuringExecution,
	// "hard" uses requiredDuringSchedulingIgnoredDuringExecution.
	// +kubebuilder:default="soft"
	// +optional
	AntiAffinityPreset *AntiAffinityPreset `json:"antiAffinityPreset,omitempty,omitzero"`

	// TopologySpreadConstraints defines how pods are spread across topology domains.
	// +optional
	TopologySpreadConstraints []corev1.TopologySpreadConstraint `json:"topologySpreadConstraints,omitempty,omitzero"`

	// PodDisruptionBudget configures the PDB for Memcached pods.
	// +optional
	PodDisruptionBudget *PDBSpec `json:"podDisruptionBudget,omitempty,omitzero"`
}

// PDBSpec defines the PodDisruptionBudget configuration.
type PDBSpec struct {
	// Enabled controls whether a PodDisruptionBudget is created.
	// +optional
	Enabled bool `json:"enabled,omitempty"`

	// MinAvailable is the minimum number of pods that must be available during disruption.
	// Can be an absolute number or a percentage (e.g. "50%").
	// Defaults to 1 when neither minAvailable nor maxUnavailable is set (applied by the controller).
	// +optional
	MinAvailable *intstr.IntOrString `json:"minAvailable,omitempty,omitzero"`

	// MaxUnavailable is the maximum number of pods that can be unavailable during disruption.
	// Can be an absolute number or a percentage (e.g. "25%").
	// +optional
	MaxUnavailable *intstr.IntOrString `json:"maxUnavailable,omitempty,omitzero"`
}

// MonitoringSpec defines monitoring and metrics configuration.
type MonitoringSpec struct {
	// Enabled controls whether monitoring is active (enables exporter sidecar).
	// +optional
	Enabled bool `json:"enabled,omitempty"`

	// ExporterImage is the container image for the memcached-exporter sidecar.
	// +kubebuilder:default="prom/memcached-exporter:v0.15.4"
	// +optional
	ExporterImage *string `json:"exporterImage,omitempty,omitzero"`

	// ExporterResources defines resource requests/limits for the exporter sidecar.
	// +optional
	ExporterResources *corev1.ResourceRequirements `json:"exporterResources,omitempty,omitzero"`

	// ServiceMonitor configures the Prometheus ServiceMonitor resource.
	// +optional
	ServiceMonitor *ServiceMonitorSpec `json:"serviceMonitor,omitempty,omitzero"`
}

// ServiceMonitorSpec defines the Prometheus ServiceMonitor configuration.
type ServiceMonitorSpec struct {
	// AdditionalLabels are extra labels added to the ServiceMonitor resource.
	// +optional
	AdditionalLabels map[string]string `json:"additionalLabels,omitempty,omitzero"`

	// Interval is the Prometheus scrape interval (e.g. "30s").
	// +kubebuilder:default="30s"
	// +optional
	Interval string `json:"interval,omitempty"`

	// ScrapeTimeout is the Prometheus scrape timeout (e.g. "10s").
	// +kubebuilder:default="10s"
	// +optional
	ScrapeTimeout string `json:"scrapeTimeout,omitempty"`
}

// SecuritySpec defines security settings for Memcached.
type SecuritySpec struct {
	// PodSecurityContext defines the security context for the Memcached pod.
	// +optional
	PodSecurityContext *corev1.PodSecurityContext `json:"podSecurityContext,omitempty,omitzero"`

	// ContainerSecurityContext defines the security context for the Memcached container.
	// +optional
	ContainerSecurityContext *corev1.SecurityContext `json:"containerSecurityContext,omitempty,omitzero"`

	// SASL configures optional SASL authentication.
	// +optional
	SASL *SASLSpec `json:"sasl,omitempty,omitzero"`

	// TLS configures optional TLS encryption.
	// +optional
	TLS *TLSSpec `json:"tls,omitempty,omitzero"`
}

// SASLSpec defines SASL authentication configuration.
type SASLSpec struct {
	// Enabled controls whether SASL authentication is active.
	// +optional
	Enabled bool `json:"enabled,omitempty"`

	// CredentialsSecretRef is a reference to the Secret containing SASL credentials.
	// The Secret must contain a "password-file" key with the SASL password file content.
	// +optional
	CredentialsSecretRef corev1.LocalObjectReference `json:"credentialsSecretRef,omitempty"`
}

// TLSSpec defines TLS encryption configuration.
type TLSSpec struct {
	// Enabled controls whether TLS encryption is active.
	// +optional
	Enabled bool `json:"enabled,omitempty"`

	// CertificateSecretRef is a reference to the Secret containing TLS certificates.
	// The Secret must contain "tls.crt", "tls.key", and optionally "ca.crt" keys.
	// +optional
	CertificateSecretRef corev1.LocalObjectReference `json:"certificateSecretRef,omitempty"`
}

// ServiceSpec defines configuration for the headless Service.
type ServiceSpec struct {
	// Annotations are custom annotations added to the Service metadata.
	// +optional
	Annotations map[string]string `json:"annotations,omitempty,omitzero"`
}

// MemcachedSpec defines the desired state of Memcached.
type MemcachedSpec struct {
	// Replicas is the number of Memcached pods.
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Maximum=64
	// +kubebuilder:default=1
	// +optional
	Replicas *int32 `json:"replicas,omitempty,omitzero"`

	// Image is the container image for the Memcached server.
	// +kubebuilder:default="memcached:1.6"
	// +optional
	Image *string `json:"image,omitempty,omitzero"`

	// Resources defines resource requests and limits for the Memcached container.
	// +optional
	Resources *corev1.ResourceRequirements `json:"resources,omitempty,omitzero"`

	// Memcached contains the Memcached server configuration.
	// +optional
	Memcached *MemcachedConfig `json:"memcached,omitempty,omitzero"`

	// HighAvailability contains high-availability settings.
	// +optional
	HighAvailability *HighAvailabilitySpec `json:"highAvailability,omitempty,omitzero"`

	// Monitoring contains monitoring and metrics configuration.
	// +optional
	Monitoring *MonitoringSpec `json:"monitoring,omitempty,omitzero"`

	// Security contains security settings.
	// +optional
	Security *SecuritySpec `json:"security,omitempty,omitzero"`

	// Service contains configuration for the headless Service.
	// +optional
	Service *ServiceSpec `json:"service,omitempty,omitzero"`
}

// MemcachedStatus defines the observed state of Memcached.
type MemcachedStatus struct {
	// Conditions represent the latest available observations of the Memcached's state.
	// +optional
	// +patchMergeKey=type
	// +patchStrategy=merge
	// +listType=map
	// +listMapKey=type
	Conditions []metav1.Condition `json:"conditions,omitempty,omitzero" patchStrategy:"merge" patchMergeKey:"type" protobuf:"bytes,1,rep,name=conditions"`

	// ReadyReplicas is the number of Memcached pods that are ready.
	// +optional
	ReadyReplicas int32 `json:"readyReplicas,omitempty"`

	// ObservedGeneration is the most recent generation observed by the controller.
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Replicas",type="integer",JSONPath=".spec.replicas",description="Number of desired Memcached pods"
// +kubebuilder:printcolumn:name="Ready",type="integer",JSONPath=".status.readyReplicas",description="Number of ready Memcached pods"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// Memcached is the Schema for the memcacheds API.
type Memcached struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   MemcachedSpec   `json:"spec,omitempty"`
	Status MemcachedStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// MemcachedList contains a list of Memcached.
type MemcachedList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Memcached `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Memcached{}, &MemcachedList{})
}
