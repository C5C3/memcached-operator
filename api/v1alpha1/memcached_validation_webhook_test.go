// Package v1alpha1 contains unit tests for the Memcached validation webhook.
package v1alpha1

import (
	"context"
	"errors"
	"strings"
	"testing"

	autoscalingv2 "k8s.io/api/autoscaling/v2"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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
	image := DefaultImage
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

// --- REQ-001: Memory limit validation (REQ-006) ---

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
			name:      "nil resources and memcached config",
			mc:        &Memcached{},
			wantError: false,
		},
		// Task 1.3 additions: boundary conditions for memory limit.
		{
			name: "one byte below boundary rejected (95Mi for 64MB max)",
			mc: &Memcached{
				Spec: MemcachedSpec{
					Memcached: &MemcachedConfig{MaxMemoryMB: 64},
					Resources: &corev1.ResourceRequirements{
						Limits: corev1.ResourceList{
							// 95Mi = 99614720 bytes; required = 64Mi + 32Mi = 96Mi = 100663296 bytes.
							corev1.ResourceMemory: resource.MustParse("95Mi"),
						},
					},
				},
			},
			wantError: true,
		},
		{
			name: "large maxMemoryMB with sufficient limit",
			mc: &Memcached{
				Spec: MemcachedSpec{
					Memcached: &MemcachedConfig{MaxMemoryMB: 4096},
					Resources: &corev1.ResourceRequirements{
						Limits: corev1.ResourceList{
							corev1.ResourceMemory: resource.MustParse("4200Mi"),
						},
					},
				},
			},
			wantError: false,
		},
		{
			name: "large maxMemoryMB with insufficient limit",
			mc: &Memcached{
				Spec: MemcachedSpec{
					Memcached: &MemcachedConfig{MaxMemoryMB: 4096},
					Resources: &corev1.ResourceRequirements{
						Limits: corev1.ResourceList{
							corev1.ResourceMemory: resource.MustParse("4096Mi"),
						},
					},
				},
			},
			wantError: true,
		},
		{
			name: "resources with CPU limit only (no memory limit)",
			mc: &Memcached{
				Spec: MemcachedSpec{
					Memcached: &MemcachedConfig{MaxMemoryMB: 64},
					Resources: &corev1.ResourceRequirements{
						Limits: corev1.ResourceList{
							corev1.ResourceCPU: resource.MustParse("500m"),
						},
					},
				},
			},
			wantError: false,
		},
		{
			name: "nil memcached config with resources set",
			mc: &Memcached{
				Spec: MemcachedSpec{
					Resources: &corev1.ResourceRequirements{
						Limits: corev1.ResourceList{
							corev1.ResourceMemory: resource.MustParse("64Mi"),
						},
					},
				},
			},
			wantError: false,
		},
		{
			name: "resources with empty limits map",
			mc: &Memcached{
				Spec: MemcachedSpec{
					Memcached: &MemcachedConfig{MaxMemoryMB: 64},
					Resources: &corev1.ResourceRequirements{
						Limits: corev1.ResourceList{},
					},
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

func TestValidateMemoryLimit_ErrorMessage(t *testing.T) {
	mc := &Memcached{
		Spec: MemcachedSpec{
			Memcached: &MemcachedConfig{MaxMemoryMB: 64},
			Resources: &corev1.ResourceRequirements{
				Limits: corev1.ResourceList{
					corev1.ResourceMemory: resource.MustParse("64Mi"),
				},
			},
		},
	}
	v := &MemcachedCustomValidator{}
	_, err := v.ValidateCreate(context.Background(), mc)
	if err == nil {
		t.Fatal("expected error for insufficient memory limit")
	}
	errMsg := err.Error()
	if !strings.Contains(errMsg, "memory") {
		t.Errorf("expected error to reference memory, got: %s", errMsg)
	}
	if !strings.Contains(errMsg, "96Mi") {
		t.Errorf("expected error to include required minimum (96Mi), got: %s", errMsg)
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
			name:      "nil HighAvailability",
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
		// Task 1.4 additions: PDB edge cases.
		{
			name: "minAvailable exceeds replicas",
			mc: &Memcached{
				Spec: MemcachedSpec{
					Replicas: &replicas3,
					HighAvailability: &HighAvailabilitySpec{
						PodDisruptionBudget: &PDBSpec{
							Enabled:      true,
							MinAvailable: &intstr.IntOrString{Type: intstr.Int, IntVal: 5},
						},
					},
				},
			},
			wantError: true,
		},
		{
			name: "minAvailable percentage skips replicas check",
			mc: &Memcached{
				Spec: MemcachedSpec{
					Replicas: &replicas3,
					HighAvailability: &HighAvailabilitySpec{
						PodDisruptionBudget: &PDBSpec{
							Enabled:      true,
							MinAvailable: &intstr.IntOrString{Type: intstr.String, StrVal: "100%"},
						},
					},
				},
			},
			wantError: false,
		},
		{
			name: "minAvailable with nil replicas skips replicas check",
			mc: &Memcached{
				Spec: MemcachedSpec{
					HighAvailability: &HighAvailabilitySpec{
						PodDisruptionBudget: &PDBSpec{
							Enabled:      true,
							MinAvailable: &intstr.IntOrString{Type: intstr.Int, IntVal: 5},
						},
					},
				},
			},
			wantError: false,
		},
		{
			name: "maxUnavailable integer value valid",
			mc: &Memcached{
				Spec: MemcachedSpec{
					HighAvailability: &HighAvailabilitySpec{
						PodDisruptionBudget: &PDBSpec{
							Enabled:        true,
							MaxUnavailable: &intstr.IntOrString{Type: intstr.Int, IntVal: 1},
						},
					},
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

func TestValidatePDB_ErrorMessages(t *testing.T) {
	replicas3 := int32(3)

	t.Run("mutual exclusivity error message", func(t *testing.T) {
		mc := &Memcached{
			Spec: MemcachedSpec{
				HighAvailability: &HighAvailabilitySpec{
					PodDisruptionBudget: &PDBSpec{
						Enabled:        true,
						MinAvailable:   &intstr.IntOrString{Type: intstr.Int, IntVal: 1},
						MaxUnavailable: &intstr.IntOrString{Type: intstr.Int, IntVal: 1},
					},
				},
			},
		}
		v := &MemcachedCustomValidator{}
		_, err := v.ValidateCreate(context.Background(), mc)
		if err == nil {
			t.Fatal("expected error for mutual exclusivity")
		}
		if !strings.Contains(err.Error(), "mutually exclusive") {
			t.Errorf("expected error to mention 'mutually exclusive', got: %s", err.Error())
		}
	})

	t.Run("minAvailable >= replicas error includes both values", func(t *testing.T) {
		mc := &Memcached{
			Spec: MemcachedSpec{
				Replicas: &replicas3,
				HighAvailability: &HighAvailabilitySpec{
					PodDisruptionBudget: &PDBSpec{
						Enabled:      true,
						MinAvailable: &intstr.IntOrString{Type: intstr.Int, IntVal: 3},
					},
				},
			},
		}
		v := &MemcachedCustomValidator{}
		_, err := v.ValidateCreate(context.Background(), mc)
		if err == nil {
			t.Fatal("expected error for minAvailable >= replicas")
		}
		errMsg := err.Error()
		if !strings.Contains(errMsg, "3") {
			t.Errorf("expected error to include both values, got: %s", errMsg)
		}
	})
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
			name:      "nil security spec",
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
		// Task 1.5 additions: graceful shutdown edge cases.
		{
			name: "invalid timing (terminationGrace < preStop)",
			mc: &Memcached{
				Spec: MemcachedSpec{
					HighAvailability: &HighAvailabilitySpec{
						GracefulShutdown: &GracefulShutdownSpec{
							Enabled:                       true,
							PreStopDelaySeconds:           30,
							TerminationGracePeriodSeconds: 10,
						},
					},
				},
			},
			wantError: true,
		},
		{
			name: "disabled graceful shutdown bypasses timing validation",
			mc: &Memcached{
				Spec: MemcachedSpec{
					HighAvailability: &HighAvailabilitySpec{
						GracefulShutdown: &GracefulShutdownSpec{
							Enabled:                       false,
							PreStopDelaySeconds:           30,
							TerminationGracePeriodSeconds: 10,
						},
					},
				},
			},
			wantError: false,
		},
		{
			name: "nil graceful shutdown spec",
			mc: &Memcached{
				Spec: MemcachedSpec{
					HighAvailability: &HighAvailabilitySpec{
						GracefulShutdown: nil,
					},
				},
			},
			wantError: false,
		},
		{
			name:      "nil highAvailability skips graceful shutdown validation",
			mc:        &Memcached{},
			wantError: false,
		},
		{
			name: "valid timing with minimal margin (grace = preStop + 1)",
			mc: &Memcached{
				Spec: MemcachedSpec{
					HighAvailability: &HighAvailabilitySpec{
						GracefulShutdown: &GracefulShutdownSpec{
							Enabled:                       true,
							PreStopDelaySeconds:           10,
							TerminationGracePeriodSeconds: 11,
						},
					},
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

func TestValidateGracefulShutdown_ErrorMessage(t *testing.T) {
	mc := &Memcached{
		Spec: MemcachedSpec{
			HighAvailability: &HighAvailabilitySpec{
				GracefulShutdown: &GracefulShutdownSpec{
					Enabled:                       true,
					PreStopDelaySeconds:           30,
					TerminationGracePeriodSeconds: 10,
				},
			},
		},
	}
	v := &MemcachedCustomValidator{}
	_, err := v.ValidateCreate(context.Background(), mc)
	if err == nil {
		t.Fatal("expected error for invalid graceful shutdown timing")
	}
	errMsg := err.Error()
	if !strings.Contains(errMsg, "terminationGracePeriodSeconds") {
		t.Errorf("expected error to reference terminationGracePeriodSeconds, got: %s", errMsg)
	}
}

func TestValidateSecuritySecretRefs_ErrorMessages(t *testing.T) {
	t.Run("SASL error includes field path", func(t *testing.T) {
		mc := &Memcached{
			Spec: MemcachedSpec{
				Security: &SecuritySpec{
					SASL: &SASLSpec{Enabled: true},
				},
			},
		}
		v := &MemcachedCustomValidator{}
		_, err := v.ValidateCreate(context.Background(), mc)
		if err == nil {
			t.Fatal("expected error for SASL without secret")
		}
		if !strings.Contains(err.Error(), "credentialsSecretRef") {
			t.Errorf("expected error to reference credentialsSecretRef, got: %s", err.Error())
		}
	})

	t.Run("TLS error includes field path", func(t *testing.T) {
		mc := &Memcached{
			Spec: MemcachedSpec{
				Security: &SecuritySpec{
					TLS: &TLSSpec{Enabled: true},
				},
			},
		}
		v := &MemcachedCustomValidator{}
		_, err := v.ValidateCreate(context.Background(), mc)
		if err == nil {
			t.Fatal("expected error for TLS without secret")
		}
		if !strings.Contains(err.Error(), "certificateSecretRef") {
			t.Errorf("expected error to reference certificateSecretRef, got: %s", err.Error())
		}
	})
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
	checks := []struct {
		desc   string
		needle string
	}{
		{"memory limit", "memory"},
		{"PDB minAvailable", "minAvailable"},
		{"SASL secret", "credentialsSecretRef"},
	}
	for _, c := range checks {
		if !strings.Contains(errMsg, c.needle) {
			t.Errorf("expected error to contain %s (%q), got: %s", c.desc, c.needle, errMsg)
		}
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

// --- Task 1.6: Error aggregation, delete bypass, and update propagation (REQ-010) ---

func TestValidation_FourSimultaneousViolations(t *testing.T) {
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
				GracefulShutdown: &GracefulShutdownSpec{
					Enabled:                       true,
					PreStopDelaySeconds:           30,
					TerminationGracePeriodSeconds: 10,
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
		t.Fatal("expected error for four simultaneous violations")
	}
	errMsg := err.Error()

	// Verify all four errors are present.
	checks := []struct {
		desc   string
		needle string
	}{
		{"memory limit", "memory"},
		{"PDB minAvailable", "minAvailable"},
		{"graceful shutdown", "terminationGracePeriodSeconds"},
		{"SASL secret", "credentialsSecretRef"},
	}
	for _, c := range checks {
		if !strings.Contains(errMsg, c.needle) {
			t.Errorf("expected error to contain %s (%q), got: %s", c.desc, c.needle, errMsg)
		}
	}
}

func TestValidation_StatusErrorFormat(t *testing.T) {
	mc := &Memcached{
		Spec: MemcachedSpec{
			Security: &SecuritySpec{
				SASL: &SASLSpec{Enabled: true},
			},
		},
	}

	v := &MemcachedCustomValidator{}
	_, err := v.ValidateCreate(context.Background(), mc)
	if err == nil {
		t.Fatal("expected error for SASL without secret")
	}

	// Verify the error is a Kubernetes StatusError (apierrors.StatusError).
	var statusErr *apierrors.StatusError
	if !errors.As(err, &statusErr) {
		t.Fatalf("expected *apierrors.StatusError, got %T", err)
	}
	if statusErr.Status().Status != metav1.StatusFailure {
		t.Errorf("expected Status=Failure, got %s", statusErr.Status().Status)
	}
	if statusErr.Status().Reason != metav1.StatusReasonInvalid {
		t.Errorf("expected Reason=Invalid, got %s", statusErr.Status().Reason)
	}
}

func TestValidateUpdate_ValidToInvalid(t *testing.T) {
	replicas := int32(3)
	image := DefaultImage

	old := &Memcached{
		Spec: MemcachedSpec{
			Replicas: &replicas,
			Image:    &image,
		},
	}
	newObj := &Memcached{
		Spec: MemcachedSpec{
			Replicas:  &replicas,
			Image:     &image,
			Memcached: &MemcachedConfig{MaxMemoryMB: 64},
			Resources: &corev1.ResourceRequirements{
				Limits: corev1.ResourceList{
					corev1.ResourceMemory: resource.MustParse("64Mi"),
				},
			},
		},
	}

	v := &MemcachedCustomValidator{}
	_, err := v.ValidateUpdate(context.Background(), old, newObj)
	if err == nil {
		t.Error("expected error when updating from valid to invalid config")
	}
	if !strings.Contains(err.Error(), "memory") {
		t.Errorf("expected error to reference memory limit, got: %s", err.Error())
	}
}

func TestValidateUpdate_ValidCRAccepted(t *testing.T) {
	replicas := int32(3)
	image := DefaultImage

	old := &Memcached{
		Spec: MemcachedSpec{
			Replicas: &replicas,
			Image:    &image,
		},
	}
	newObj := &Memcached{
		Spec: MemcachedSpec{
			Replicas:  &replicas,
			Image:     &image,
			Memcached: &MemcachedConfig{MaxMemoryMB: 64},
			Resources: &corev1.ResourceRequirements{
				Limits: corev1.ResourceList{
					corev1.ResourceMemory: resource.MustParse("128Mi"),
				},
			},
		},
	}

	v := &MemcachedCustomValidator{}
	_, err := v.ValidateUpdate(context.Background(), old, newObj)
	if err != nil {
		t.Errorf("expected no error for valid update, got: %v", err)
	}
}

func TestValidateDelete_InvalidCRStillDeletes(t *testing.T) {
	// Verify that a CR with multiple violations can still be deleted.
	replicas := int32(3)
	mc := &Memcached{
		Spec: MemcachedSpec{
			Replicas:  &replicas,
			Memcached: &MemcachedConfig{MaxMemoryMB: 64},
			Resources: &corev1.ResourceRequirements{
				Limits: corev1.ResourceList{
					corev1.ResourceMemory: resource.MustParse("32Mi"),
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
				TLS:  &TLSSpec{Enabled: true},
			},
		},
	}

	v := &MemcachedCustomValidator{}
	warnings, err := v.ValidateDelete(context.Background(), mc)
	if err != nil {
		t.Errorf("expected no error for delete, got: %v", err)
	}
	if warnings != nil {
		t.Errorf("expected no warnings for delete, got: %v", warnings)
	}
}

// --- REQ-005: Replicas/autoscaling mutual exclusivity ---

func TestValidateAutoscalingReplicasMutualExclusivity(t *testing.T) {
	replicas := int32(3)
	zeroReplicas := int32(0)

	tests := []struct {
		name      string
		mc        *Memcached
		wantError bool
	}{
		{
			name: "replicas set + autoscaling enabled (rejected)",
			mc: &Memcached{
				Spec: MemcachedSpec{
					Replicas: &replicas,
					Autoscaling: &AutoscalingSpec{
						Enabled:     true,
						MaxReplicas: 10,
					},
				},
			},
			wantError: true,
		},
		{
			name: "replicas nil + autoscaling enabled (accepted)",
			mc: &Memcached{
				Spec: MemcachedSpec{
					Autoscaling: &AutoscalingSpec{
						Enabled:     true,
						MaxReplicas: 10,
					},
				},
			},
			wantError: false,
		},
		{
			name: "replicas set + autoscaling disabled (accepted)",
			mc: &Memcached{
				Spec: MemcachedSpec{
					Replicas: &replicas,
					Autoscaling: &AutoscalingSpec{
						Enabled: false,
					},
				},
			},
			wantError: false,
		},
		{
			name: "replicas set + autoscaling nil (accepted)",
			mc: &Memcached{
				Spec: MemcachedSpec{
					Replicas: &replicas,
				},
			},
			wantError: false,
		},
		{
			name: "replicas=0 pointer + autoscaling enabled (rejected, pointer is non-nil)",
			mc: &Memcached{
				Spec: MemcachedSpec{
					Replicas: &zeroReplicas,
					Autoscaling: &AutoscalingSpec{
						Enabled:     true,
						MaxReplicas: 10,
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

// --- REQ-006: Autoscaling minReplicas/maxReplicas validation ---

func TestValidateAutoscalingMinMaxReplicas(t *testing.T) {
	min1 := int32(1)
	min3 := int32(3)
	min5 := int32(5)

	tests := []struct {
		name      string
		mc        *Memcached
		wantError bool
	}{
		{
			name: "min > max (rejected)",
			mc: &Memcached{
				Spec: MemcachedSpec{
					Autoscaling: &AutoscalingSpec{
						Enabled:     true,
						MinReplicas: &min5,
						MaxReplicas: 3,
					},
				},
			},
			wantError: true,
		},
		{
			name: "min == max (accepted, fixed scaling)",
			mc: &Memcached{
				Spec: MemcachedSpec{
					Autoscaling: &AutoscalingSpec{
						Enabled:     true,
						MinReplicas: &min3,
						MaxReplicas: 3,
					},
				},
			},
			wantError: false,
		},
		{
			name: "min < max (accepted)",
			mc: &Memcached{
				Spec: MemcachedSpec{
					Autoscaling: &AutoscalingSpec{
						Enabled:     true,
						MinReplicas: &min1,
						MaxReplicas: 10,
					},
				},
			},
			wantError: false,
		},
		{
			name: "min nil (accepted, HPA defaults min to 1)",
			mc: &Memcached{
				Spec: MemcachedSpec{
					Autoscaling: &AutoscalingSpec{
						Enabled:     true,
						MaxReplicas: 10,
					},
				},
			},
			wantError: false,
		},
		{
			name: "autoscaling disabled skips min/max check",
			mc: &Memcached{
				Spec: MemcachedSpec{
					Autoscaling: &AutoscalingSpec{
						Enabled:     false,
						MinReplicas: &min5,
						MaxReplicas: 3,
					},
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

// --- REQ-007: CPU metric requires CPU requests ---

func TestValidateAutoscalingCPURequests(t *testing.T) {
	cpuUtilization := int32(80)
	cpuMetric := []autoscalingv2.MetricSpec{
		{
			Type: autoscalingv2.ResourceMetricSourceType,
			Resource: &autoscalingv2.ResourceMetricSource{
				Name: corev1.ResourceCPU,
				Target: autoscalingv2.MetricTarget{
					Type:               autoscalingv2.UtilizationMetricType,
					AverageUtilization: &cpuUtilization,
				},
			},
		},
	}
	memoryMetric := []autoscalingv2.MetricSpec{
		{
			Type: autoscalingv2.ResourceMetricSourceType,
			Resource: &autoscalingv2.ResourceMetricSource{
				Name: corev1.ResourceMemory,
				Target: autoscalingv2.MetricTarget{
					Type:               autoscalingv2.UtilizationMetricType,
					AverageUtilization: &cpuUtilization,
				},
			},
		},
	}
	cpuAverageValue := resource.MustParse("500m")
	cpuAverageValueMetric := []autoscalingv2.MetricSpec{
		{
			Type: autoscalingv2.ResourceMetricSourceType,
			Resource: &autoscalingv2.ResourceMetricSource{
				Name: corev1.ResourceCPU,
				Target: autoscalingv2.MetricTarget{
					Type:         autoscalingv2.AverageValueMetricType,
					AverageValue: &cpuAverageValue,
				},
			},
		},
	}

	tests := []struct {
		name      string
		mc        *Memcached
		wantError bool
	}{
		{
			name: "CPU metric + no resources (rejected)",
			mc: &Memcached{
				Spec: MemcachedSpec{
					Autoscaling: &AutoscalingSpec{
						Enabled:     true,
						MaxReplicas: 10,
						Metrics:     cpuMetric,
					},
				},
			},
			wantError: true,
		},
		{
			name: "CPU metric + no requests (rejected)",
			mc: &Memcached{
				Spec: MemcachedSpec{
					Autoscaling: &AutoscalingSpec{
						Enabled:     true,
						MaxReplicas: 10,
						Metrics:     cpuMetric,
					},
					Resources: &corev1.ResourceRequirements{
						Limits: corev1.ResourceList{
							corev1.ResourceCPU: resource.MustParse("500m"),
						},
					},
				},
			},
			wantError: true,
		},
		{
			name: "CPU metric + no CPU request (rejected)",
			mc: &Memcached{
				Spec: MemcachedSpec{
					Autoscaling: &AutoscalingSpec{
						Enabled:     true,
						MaxReplicas: 10,
						Metrics:     cpuMetric,
					},
					Resources: &corev1.ResourceRequirements{
						Requests: corev1.ResourceList{
							corev1.ResourceMemory: resource.MustParse("128Mi"),
						},
					},
				},
			},
			wantError: true,
		},
		{
			name: "CPU metric + CPU request set (accepted)",
			mc: &Memcached{
				Spec: MemcachedSpec{
					Autoscaling: &AutoscalingSpec{
						Enabled:     true,
						MaxReplicas: 10,
						Metrics:     cpuMetric,
					},
					Resources: &corev1.ResourceRequirements{
						Requests: corev1.ResourceList{
							corev1.ResourceCPU: resource.MustParse("100m"),
						},
					},
				},
			},
			wantError: false,
		},
		{
			name: "memory metric only + no CPU request (accepted)",
			mc: &Memcached{
				Spec: MemcachedSpec{
					Autoscaling: &AutoscalingSpec{
						Enabled:     true,
						MaxReplicas: 10,
						Metrics:     memoryMetric,
					},
				},
			},
			wantError: false,
		},
		{
			name: "no metrics (accepted)",
			mc: &Memcached{
				Spec: MemcachedSpec{
					Autoscaling: &AutoscalingSpec{
						Enabled:     true,
						MaxReplicas: 10,
					},
				},
			},
			wantError: false,
		},
		{
			name: "CPU AverageValue metric + no CPU request (accepted, only Utilization requires it)",
			mc: &Memcached{
				Spec: MemcachedSpec{
					Autoscaling: &AutoscalingSpec{
						Enabled:     true,
						MaxReplicas: 10,
						Metrics:     cpuAverageValueMetric,
					},
				},
			},
			wantError: false,
		},
		{
			name: "autoscaling disabled with CPU metric (accepted)",
			mc: &Memcached{
				Spec: MemcachedSpec{
					Autoscaling: &AutoscalingSpec{
						Enabled: false,
						Metrics: cpuMetric,
					},
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

// --- REQ-008: Multiple autoscaling errors collected ---

func TestValidation_AutoscalingMultipleErrors(t *testing.T) {
	replicas := int32(3)
	min5 := int32(5)
	mc := &Memcached{
		Spec: MemcachedSpec{
			Replicas: &replicas,
			Autoscaling: &AutoscalingSpec{
				Enabled:     true,
				MinReplicas: &min5,
				MaxReplicas: 3,
			},
		},
	}

	v := &MemcachedCustomValidator{}
	_, err := v.ValidateCreate(context.Background(), mc)
	if err == nil {
		t.Fatal("expected error for multiple autoscaling violations")
	}
	errMsg := err.Error()
	// Both mutual exclusivity and min > max errors should be present.
	if !strings.Contains(errMsg, "mutually exclusive") {
		t.Errorf("expected error to mention 'mutually exclusive', got: %s", errMsg)
	}
	if !strings.Contains(errMsg, "minReplicas") {
		t.Errorf("expected error to mention 'minReplicas', got: %s", errMsg)
	}
}

func TestValidation_AutoscalingWithExistingErrors(t *testing.T) {
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
			Autoscaling: &AutoscalingSpec{
				Enabled:     true,
				MaxReplicas: 10,
			},
		},
	}

	v := &MemcachedCustomValidator{}
	_, err := v.ValidateCreate(context.Background(), mc)
	if err == nil {
		t.Fatal("expected error for combined violations")
	}
	errMsg := err.Error()
	// Both memory limit and autoscaling mutual exclusivity errors should be present.
	if !strings.Contains(errMsg, "memory") {
		t.Errorf("expected error to contain 'memory', got: %s", errMsg)
	}
	if !strings.Contains(errMsg, "mutually exclusive") {
		t.Errorf("expected error to contain 'mutually exclusive', got: %s", errMsg)
	}
}

func TestValidateAutoscaling_ErrorMessages(t *testing.T) {
	t.Run("mutual exclusivity error includes field path", func(t *testing.T) {
		replicas := int32(3)
		mc := &Memcached{
			Spec: MemcachedSpec{
				Replicas: &replicas,
				Autoscaling: &AutoscalingSpec{
					Enabled:     true,
					MaxReplicas: 10,
				},
			},
		}
		v := &MemcachedCustomValidator{}
		_, err := v.ValidateCreate(context.Background(), mc)
		if err == nil {
			t.Fatal("expected error for mutual exclusivity")
		}
		errMsg := err.Error()
		if !strings.Contains(errMsg, "spec.replicas") {
			t.Errorf("expected error to reference spec.replicas, got: %s", errMsg)
		}
		if !strings.Contains(errMsg, "mutually exclusive") {
			t.Errorf("expected error to mention 'mutually exclusive', got: %s", errMsg)
		}
	})

	t.Run("minReplicas error includes both values", func(t *testing.T) {
		min5 := int32(5)
		mc := &Memcached{
			Spec: MemcachedSpec{
				Autoscaling: &AutoscalingSpec{
					Enabled:     true,
					MinReplicas: &min5,
					MaxReplicas: 3,
				},
			},
		}
		v := &MemcachedCustomValidator{}
		_, err := v.ValidateCreate(context.Background(), mc)
		if err == nil {
			t.Fatal("expected error for min > max")
		}
		errMsg := err.Error()
		if !strings.Contains(errMsg, "spec.autoscaling.minReplicas") {
			t.Errorf("expected error to reference spec.autoscaling.minReplicas, got: %s", errMsg)
		}
		if !strings.Contains(errMsg, "5") || !strings.Contains(errMsg, "3") {
			t.Errorf("expected error to include both values (5 and 3), got: %s", errMsg)
		}
	})

	t.Run("CPU request error includes field path", func(t *testing.T) {
		cpuUtilization := int32(80)
		mc := &Memcached{
			Spec: MemcachedSpec{
				Autoscaling: &AutoscalingSpec{
					Enabled:     true,
					MaxReplicas: 10,
					Metrics: []autoscalingv2.MetricSpec{
						{
							Type: autoscalingv2.ResourceMetricSourceType,
							Resource: &autoscalingv2.ResourceMetricSource{
								Name: corev1.ResourceCPU,
								Target: autoscalingv2.MetricTarget{
									Type:               autoscalingv2.UtilizationMetricType,
									AverageUtilization: &cpuUtilization,
								},
							},
						},
					},
				},
			},
		}
		v := &MemcachedCustomValidator{}
		_, err := v.ValidateCreate(context.Background(), mc)
		if err == nil {
			t.Fatal("expected error for CPU metric without CPU request")
		}
		errMsg := err.Error()
		if !strings.Contains(errMsg, "spec.resources.requests.cpu") {
			t.Errorf("expected error to reference spec.resources.requests.cpu, got: %s", errMsg)
		}
		if !strings.Contains(errMsg, "CPU utilization") {
			t.Errorf("expected error to mention CPU utilization, got: %s", errMsg)
		}
	})
}
