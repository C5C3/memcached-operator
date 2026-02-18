package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// MemcachedSpec defines the desired state of Memcached.
type MemcachedSpec struct {
	// Replicas is the number of Memcached pods.
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Maximum=64
	// +kubebuilder:default=1
	// +optional
	Replicas *int32 `json:"replicas,omitempty,omitzero"`
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
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Replicas",type="integer",JSONPath=".spec.replicas",description="Number of desired Memcached pods"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// Memcached is the Schema for the memcacheds API.
type Memcached struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   MemcachedSpec   `json:"spec,omitempty,omitzero"`
	Status MemcachedStatus `json:"status,omitempty,omitzero"`
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
