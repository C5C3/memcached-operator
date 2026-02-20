// Package v1alpha1 contains unit tests for the Memcached validation webhook.
package v1alpha1

import (
	"context"
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/util/intstr"
)

// --- Core validator lifecycle ---

func TestValidateCreate_ValidMinimalCR(t *testing.T) {
	v := &MemcachedCustomValidator{}
	_, err := v.ValidateCreate(context.Background(), &Memcached{})
	if err != nil {
		t.Errorf("expected no error for minimal CR, got: %v", err)
	}
}

func TestValidateUpdate_ValidMinimalCR(t *testing.T) {
	mc := &Memcached{}
	v := &MemcachedCustomValidator{}
	_, err := v.ValidateUpdate(context.Background(), mc, mc)
	if err != nil {
		t.Errorf("expected no error for minimal CR update, got: %v", err)
	}
}

func TestValidateDelete_AlwaysSucceeds(t *testing.T) {
	// REQ-009: Delete is always allowed, even for invalid configs.
	mc := &Memcached{
		Spec: MemcachedSpec{
			Security: &SecuritySpec{
				SASL: &SASLSpec{Enabled: true}, // invalid but should not matter for delete
			},
		},
	}
	v := &MemcachedCustomValidator{}
	_, err := v.ValidateDelete(context.Background(), mc)
	if err != nil {
		t.Errorf("expected no error for delete, got: %v", err)
	}
}

// --- Fully populated valid CR (REQ-010) ---

func TestValidateCreate_FullyPopulatedValidCR(t *testing.T) {
	replicas := int32(3)
	image := "memcached:1.6"
	mc := &Memcached{
		Spec: MemcachedSpec{
			Replicas:  &replicas,
			Image:     &image,
			Memcached: &MemcachedConfig{MaxMemoryMB: 64},
			Resources: &corev1.ResourceRequirements{
				Limits: corev1.ResourceList{
					corev1.ResourceMemory: resource.MustParse("128Mi"),
				},
			},
			HighAvailability: &HighAvailabilitySpec{
				PodDisruptionBudget: &PDBSpec{
					Enabled:      true,
					MinAvailable: &intstr.IntOrString{Type: intstr.Int, IntVal: 2},
				},
				GracefulShutdown: &GracefulShutdownSpec{
					Enabled:                       true,
					PreStopDelaySeconds:           10,
					TerminationGracePeriodSeconds: 30,
				},
			},
			Security: &SecuritySpec{
				SASL: &SASLSpec{
					Enabled:              true,
					CredentialsSecretRef: corev1.LocalObjectReference{Name: "sasl-secret"},
				},
				TLS: &TLSSpec{
					Enabled:              true,
					CertificateSecretRef: corev1.LocalObjectReference{Name: "tls-cert"},
				},
			},
		},
	}

	v := &MemcachedCustomValidator{}
	_, err := v.ValidateCreate(context.Background(), mc)
	if err != nil {
		t.Errorf("expected no error for fully valid CR, got: %v", err)
	}
}

// --- REQ-001: Memory limit validation ---

func TestValidateMemoryLimit(t *testing.T) {
	tests := []struct {
		name      string
		mc        *Memcached
		wantError bool
	}{
		{
			name: "sufficient memory limit",
			mc: &Memcached{
				Spec: MemcachedSpec{
					Memcached: &MemcachedConfig{MaxMemoryMB: 64},
					Resources: &corev1.ResourceRequirements{
						Limits: corev1.ResourceList{
							corev1.ResourceMemory: resource.MustParse("128Mi"),
						},
					},
				},
			},
			wantError: false,
		},
		{
			name: "exact boundary (64Mi + 32Mi overhead)",
			mc: &Memcached{
				Spec: MemcachedSpec{
					Memcached: &MemcachedConfig{MaxMemoryMB: 64},
					Resources: &corev1.ResourceRequirements{
						Limits: corev1.ResourceList{
							corev1.ResourceMemory: resource.MustParse("96Mi"),
						},
					},
				},
			},
			wantError: false,
		},
		{
			name: "insufficient memory limit",
			mc: &Memcached{
				Spec: MemcachedSpec{
					Memcached: &MemcachedConfig{MaxMemoryMB: 64},
					Resources: &corev1.ResourceRequirements{
						Limits: corev1.ResourceList{
							corev1.ResourceMemory: resource.MustParse("64Mi"),
						},
					},
				},
			},
			wantError: true,
		},
		{
			name: "no memory limit specified",
			mc: &Memcached{
				Spec: MemcachedSpec{
					Memcached: &MemcachedConfig{MaxMemoryMB: 64},
				},
			},
			wantError: false,
		},
		{
			name: "nil resources and memcached config",
			mc:        &Memcached{},
			wantError: false,
		},
	}

	v := &MemcachedCustomValidator{}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := v.ValidateCreate(context.Background(), tt.mc)
			if (err != nil) != tt.wantError {
				t.Errorf("wantError=%v, got err=%v", tt.wantError, err)
			}
		})
	}
}

// --- REQ-002, REQ-003: PDB validation ---

func TestValidatePDB(t *testing.T) {
	replicas3 := int32(3)

	tests := []struct {
		name      string
		mc        *Memcached
		wantError bool
	}{
		{
			name: "minAvailable only",
			mc: &Memcached{
				Spec: MemcachedSpec{
					HighAvailability: &HighAvailabilitySpec{
						PodDisruptionBudget: &PDBSpec{
							Enabled:      true,
							MinAvailable: &intstr.IntOrString{Type: intstr.Int, IntVal: 1},
						},
					},
				},
			},
			wantError: false,
		},
		{
			name: "maxUnavailable only",
			mc: &Memcached{
				Spec: MemcachedSpec{
					HighAvailability: &HighAvailabilitySpec{
						PodDisruptionBudget: &PDBSpec{
							Enabled:        true,
							MaxUnavailable: &intstr.IntOrString{Type: intstr.String, StrVal: "25%"},
						},
					},
				},
			},
			wantError: false,
		},
		{
			name: "minAvailable as percentage",
			mc: &Memcached{
				Spec: MemcachedSpec{
					HighAvailability: &HighAvailabilitySpec{
						PodDisruptionBudget: &PDBSpec{
							Enabled:      true,
							MinAvailable: &intstr.IntOrString{Type: intstr.String, StrVal: "50%"},
						},
					},
				},
			},
			wantError: false,
		},
		{
			name: "both minAvailable and maxUnavailable set (REQ-003)",
			mc: &Memcached{
				Spec: MemcachedSpec{
					HighAvailability: &HighAvailabilitySpec{
						PodDisruptionBudget: &PDBSpec{
							Enabled:        true,
							MinAvailable:   &intstr.IntOrString{Type: intstr.Int, IntVal: 1},
							MaxUnavailable: &intstr.IntOrString{Type: intstr.Int, IntVal: 1},
						},
					},
				},
			},
			wantError: true,
		},
		{
			name: "neither minAvailable nor maxUnavailable set",
			mc: &Memcached{
				Spec: MemcachedSpec{
					HighAvailability: &HighAvailabilitySpec{
						PodDisruptionBudget: &PDBSpec{
							Enabled: true,
						},
					},
				},
			},
			wantError: true,
		},
		{
			name: "disabled bypasses validation",
			mc: &Memcached{
				Spec: MemcachedSpec{
					HighAvailability: &HighAvailabilitySpec{
						PodDisruptionBudget: &PDBSpec{
							Enabled: false,
						},
					},
				},
			},
			wantError: false,
		},
		{
			name: "nil PDB spec",
			mc: &Memcached{
				Spec: MemcachedSpec{
					HighAvailability: &HighAvailabilitySpec{},
				},
			},
			wantError: false,
		},
		{
			name: "nil HighAvailability",
			mc:        &Memcached{},
			wantError: false,
		},
		{
			name: "minAvailable valid (< replicas) (REQ-002)",
			mc: &Memcached{
				Spec: MemcachedSpec{
					Replicas: &replicas3,
					HighAvailability: &HighAvailabilitySpec{
						PodDisruptionBudget: &PDBSpec{
							Enabled:      true,
							MinAvailable: &intstr.IntOrString{Type: intstr.Int, IntVal: 2},
						},
					},
				},
			},
			wantError: false,
		},
		{
			name: "minAvailable equals replicas (REQ-002)",
			mc: &Memcached{
				Spec: MemcachedSpec{
					Replicas: &replicas3,
					HighAvailability: &HighAvailabilitySpec{
						PodDisruptionBudget: &PDBSpec{
							Enabled:      true,
							MinAvailable: &intstr.IntOrString{Type: intstr.Int, IntVal: 3},
						},
					},
				},
			},
			wantError: true,
		},
	}

	v := &MemcachedCustomValidator{}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := v.ValidateCreate(context.Background(), tt.mc)
			if (err != nil) != tt.wantError {
				t.Errorf("wantError=%v, got err=%v", tt.wantError, err)
			}
		})
	}
}

// --- REQ-004, REQ-005: Security secret reference validation ---

func TestValidateSecuritySecretRefs(t *testing.T) {
	tests := []struct {
		name      string
		mc        *Memcached
		wantError bool
	}{
		{
			name: "SASL enabled with secret",
			mc: &Memcached{
				Spec: MemcachedSpec{
					Security: &SecuritySpec{
						SASL: &SASLSpec{
							Enabled:              true,
							CredentialsSecretRef: corev1.LocalObjectReference{Name: "sasl-secret"},
						},
					},
				},
			},
			wantError: false,
		},
		{
			name: "SASL enabled without secret (REQ-004)",
			mc: &Memcached{
				Spec: MemcachedSpec{
					Security: &SecuritySpec{
						SASL: &SASLSpec{Enabled: true},
					},
				},
			},
			wantError: true,
		},
		{
			name: "SASL disabled without secret",
			mc: &Memcached{
				Spec: MemcachedSpec{
					Security: &SecuritySpec{
						SASL: &SASLSpec{Enabled: false},
					},
				},
			},
			wantError: false,
		},
		{
			name: "TLS enabled with secret",
			mc: &Memcached{
				Spec: MemcachedSpec{
					Security: &SecuritySpec{
						TLS: &TLSSpec{
							Enabled:              true,
							CertificateSecretRef: corev1.LocalObjectReference{Name: "tls-cert"},
						},
					},
				},
			},
			wantError: false,
		},
		{
			name: "TLS enabled without secret (REQ-005)",
			mc: &Memcached{
				Spec: MemcachedSpec{
					Security: &SecuritySpec{
						TLS: &TLSSpec{Enabled: true},
					},
				},
			},
			wantError: true,
		},
		{
			name: "TLS disabled without secret",
			mc: &Memcached{
				Spec: MemcachedSpec{
					Security: &SecuritySpec{
						TLS: &TLSSpec{Enabled: false},
					},
				},
			},
			wantError: false,
		},
		{
			name: "both SASL and TLS enabled with secrets",
			mc: &Memcached{
				Spec: MemcachedSpec{
					Security: &SecuritySpec{
						SASL: &SASLSpec{
							Enabled:              true,
							CredentialsSecretRef: corev1.LocalObjectReference{Name: "sasl-secret"},
						},
						TLS: &TLSSpec{
							Enabled:              true,
							CertificateSecretRef: corev1.LocalObjectReference{Name: "tls-cert"},
						},
					},
				},
			},
			wantError: false,
		},
		{
			name: "both SASL and TLS enabled without secrets",
			mc: &Memcached{
				Spec: MemcachedSpec{
					Security: &SecuritySpec{
						SASL: &SASLSpec{Enabled: true},
						TLS:  &TLSSpec{Enabled: true},
					},
				},
			},
			wantError: true,
		},
		{
			name: "nil security spec",
			mc:        &Memcached{},
			wantError: false,
		},
		{
			name: "nil SASL and TLS",
			mc: &Memcached{
				Spec: MemcachedSpec{
					Security: &SecuritySpec{},
				},
			},
			wantError: false,
		},
	}

	v := &MemcachedCustomValidator{}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := v.ValidateCreate(context.Background(), tt.mc)
			if (err != nil) != tt.wantError {
				t.Errorf("wantError=%v, got err=%v", tt.wantError, err)
			}
		})
	}
}

// --- REQ-006: Graceful shutdown timing validation ---

func TestValidateGracefulShutdown(t *testing.T) {
	tests := []struct {
		name      string
		mc        *Memcached
		wantError bool
	}{
		{
			name: "valid timing (terminationGrace > preStop)",
			mc: &Memcached{
				Spec: MemcachedSpec{
					HighAvailability: &HighAvailabilitySpec{
						GracefulShutdown: &GracefulShutdownSpec{
							Enabled:                       true,
							PreStopDelaySeconds:           10,
							TerminationGracePeriodSeconds: 30,
						},
					},
				},
			},
			wantError: false,
		},
		{
			name: "invalid timing (terminationGrace == preStop)",
			mc: &Memcached{
				Spec: MemcachedSpec{
					HighAvailability: &HighAvailabilitySpec{
						GracefulShutdown: &GracefulShutdownSpec{
							Enabled:                       true,
							PreStopDelaySeconds:           10,
							TerminationGracePeriodSeconds: 10,
						},
					},
				},
			},
			wantError: true,
		},
	}

	v := &MemcachedCustomValidator{}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := v.ValidateCreate(context.Background(), tt.mc)
			if (err != nil) != tt.wantError {
				t.Errorf("wantError=%v, got err=%v", tt.wantError, err)
			}
		})
	}
}

// --- REQ-008: Multiple errors collected ---

func TestValidation_MultipleErrorsCollected(t *testing.T) {
	replicas := int32(3)
	mc := &Memcached{
		Spec: MemcachedSpec{
			Replicas:  &replicas,
			Memcached: &MemcachedConfig{MaxMemoryMB: 64},
			Resources: &corev1.ResourceRequirements{
				Limits: corev1.ResourceList{
					corev1.ResourceMemory: resource.MustParse("64Mi"),
				},
			},
			HighAvailability: &HighAvailabilitySpec{
				PodDisruptionBudget: &PDBSpec{
					Enabled:      true,
					MinAvailable: &intstr.IntOrString{Type: intstr.Int, IntVal: 3},
				},
			},
			Security: &SecuritySpec{
				SASL: &SASLSpec{Enabled: true},
			},
		},
	}

	v := &MemcachedCustomValidator{}
	_, err := v.ValidateCreate(context.Background(), mc)
	if err == nil {
		t.Fatal("expected error for multiple violations")
	}
	errMsg := err.Error()
	// Verify multiple errors are present, not just the first one.
	containsMemory := strings.Contains(errMsg, "resources.limits.memory") || strings.Contains(errMsg, "memory")
	containsPDB := strings.Contains(errMsg, "minAvailable") || strings.Contains(errMsg, "podDisruptionBudget")
	containsSASL := strings.Contains(errMsg, "sasl") || strings.Contains(errMsg, "credentialsSecretRef")
	if !containsMemory || !containsPDB || !containsSASL {
		t.Errorf("expected error to contain all three violations (memory, PDB, SASL), got: %v", err)
	}
}

// --- Update propagates validation ---

func TestValidateUpdate_PropagatesErrors(t *testing.T) {
	old := &Memcached{}
	mc := &Memcached{
		Spec: MemcachedSpec{
			Security: &SecuritySpec{
				SASL: &SASLSpec{Enabled: true},
			},
		},
	}

	v := &MemcachedCustomValidator{}
	_, err := v.ValidateUpdate(context.Background(), old, mc)
	if err == nil {
		t.Error("expected error on update with invalid config")
	}
}
