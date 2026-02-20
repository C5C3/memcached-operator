// Package v1alpha1 contains tests that validate example CR YAML files
// in config/samples/ parse correctly as Memcached objects.
package v1alpha1

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"sigs.k8s.io/yaml"
)

// samplesDirPath returns the resolved path to config/samples/
// relative to this test file's location.
func samplesDirPath(t *testing.T) string {
	t.Helper()
	_, testFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("unable to get caller info")
	}
	return filepath.Join(filepath.Dir(testFile), "..", "..", "config", "samples")
}

// loadSampleYAML reads a sample YAML file and unmarshals it into a Memcached struct.
func loadSampleYAML(t *testing.T, filename string) *Memcached {
	t.Helper()
	samplePath := filepath.Join(samplesDirPath(t), filename)
	data, err := os.ReadFile(samplePath)
	if err != nil {
		t.Fatalf("failed to read sample file %q: %v", filename, err)
	}

	memcached := &Memcached{}
	if err := yaml.Unmarshal(data, memcached); err != nil {
		t.Fatalf("failed to unmarshal sample YAML %q: %v", filename, err)
	}

	return memcached
}

// TestSampleYAMLFiles validates that all example CR YAML files in config/samples/
// parse correctly and have the expected apiVersion and kind.
func TestSampleYAMLFiles(t *testing.T) {
	samplesDir := samplesDirPath(t)

	// Discover all memcached_v1alpha1_*.yaml files (excluding kustomization.yaml)
	pattern := filepath.Join(samplesDir, "memcached_v1alpha1_*.yaml")
	matches, err := filepath.Glob(pattern)
	if err != nil {
		t.Fatalf("failed to glob sample files: %v", err)
	}

	// Ensure we found at least one sample file
	if len(matches) == 0 {
		t.Fatalf("no sample files found matching pattern %q - check path configuration", pattern)
	}

	t.Logf("Found %d sample YAML files to validate", len(matches))

	// Test each sample file
	for _, fullPath := range matches {
		filename := filepath.Base(fullPath)
		t.Run(filename, func(t *testing.T) {
			memcached := loadSampleYAML(t, filename)

			// Verify apiVersion
			expectedAPIVersion := "memcached.c5c3.io/v1alpha1"
			if memcached.APIVersion != expectedAPIVersion {
				t.Errorf("apiVersion: got %q, want %q", memcached.APIVersion, expectedAPIVersion)
			}

			// Verify kind
			expectedKind := "Memcached"
			if memcached.Kind != expectedKind {
				t.Errorf("kind: got %q, want %q", memcached.Kind, expectedKind)
			}

			// Verify metadata.name is non-empty
			if memcached.Name == "" {
				t.Error("metadata.name is empty, expected a non-empty name")
			}

			// Log successful validation details
			if memcached.Spec.Replicas != nil {
				t.Logf("Successfully validated %q: name=%q, replicas=%d",
					filename, memcached.Name, *memcached.Spec.Replicas)
			} else {
				t.Logf("Successfully validated %q: name=%q, replicas=nil",
					filename, memcached.Name)
			}
		})
	}
}

// TestSampleYAMLMinimal validates the minimal sample can be parsed and has expected defaults.
func TestSampleYAMLMinimal(t *testing.T) {
	memcached := loadSampleYAML(t, "memcached_v1alpha1_minimal.yaml")

	// Basic metadata validation
	if memcached.Name == "" {
		t.Error("expected non-empty metadata.name")
	}

	// Verify it has spec.replicas set
	if memcached.Spec.Replicas == nil {
		t.Error("expected spec.replicas to be set in minimal sample")
	}
}

// TestSampleYAMLMemcached validates the main memcached sample with configuration.
func TestSampleYAMLMemcached(t *testing.T) {
	memcached := loadSampleYAML(t, "memcached_v1alpha1_memcached.yaml")

	// Basic metadata validation
	if memcached.Name == "" {
		t.Error("expected non-empty metadata.name")
	}

	// Verify it has memcached configuration
	if memcached.Spec.Memcached == nil {
		t.Error("expected spec.memcached to be set in memcached sample")
	}
}

// TestSampleYAMLHA validates the high availability sample.
func TestSampleYAMLHA(t *testing.T) {
	memcached := loadSampleYAML(t, "memcached_v1alpha1_ha.yaml")

	// Basic metadata validation
	if memcached.Name == "" {
		t.Error("expected non-empty metadata.name")
	}

	// Verify it has high availability configuration
	if memcached.Spec.HighAvailability == nil {
		t.Error("expected spec.highAvailability to be set in HA sample")
	}
}

// TestSampleYAMLMonitoring validates the monitoring sample.
func TestSampleYAMLMonitoring(t *testing.T) {
	memcached := loadSampleYAML(t, "memcached_v1alpha1_monitoring.yaml")

	// Basic metadata validation
	if memcached.Name == "" {
		t.Error("expected non-empty metadata.name")
	}

	// Verify monitoring configuration
	if memcached.Spec.Monitoring == nil {
		t.Fatal("expected spec.monitoring to be set in monitoring sample")
	}
	if !memcached.Spec.Monitoring.Enabled {
		t.Error("expected spec.monitoring.enabled to be true in monitoring sample")
	}
}

// TestSampleYAMLTLS validates the TLS sample.
func TestSampleYAMLTLS(t *testing.T) {
	memcached := loadSampleYAML(t, "memcached_v1alpha1_tls.yaml")

	// Basic metadata validation
	if memcached.Name == "" {
		t.Error("expected non-empty metadata.name")
	}

	// Verify security and TLS configuration
	if memcached.Spec.Security == nil {
		t.Fatal("expected spec.security to be set in TLS sample")
	}
	if memcached.Spec.Security.TLS == nil {
		t.Fatal("expected spec.security.tls to be set in TLS sample")
	}
	if !memcached.Spec.Security.TLS.Enabled {
		t.Error("expected spec.security.tls.enabled to be true in TLS sample")
	}
}

// TestSampleYAMLProduction validates the production sample with comprehensive configuration.
func TestSampleYAMLProduction(t *testing.T) {
	memcached := loadSampleYAML(t, "memcached_v1alpha1_production.yaml")

	// Basic metadata validation
	if memcached.Name == "" {
		t.Error("expected non-empty metadata.name")
	}

	// Verify production sample has higher replica count
	if memcached.Spec.Replicas != nil && *memcached.Spec.Replicas < 2 {
		t.Errorf("expected production sample to have >= 2 replicas, got %d", *memcached.Spec.Replicas)
	}

	// Verify it has resources configured
	if memcached.Spec.Resources == nil {
		t.Error("expected spec.resources to be set in production sample")
	}

	// Verify it has high availability configured
	if memcached.Spec.HighAvailability == nil {
		t.Error("expected spec.highAvailability to be set in production sample")
	}
}

// TestAllSamplesHaveValidStructure is a comprehensive test that validates
// structural requirements across all samples.
func TestAllSamplesHaveValidStructure(t *testing.T) {
	samplesDir := samplesDirPath(t)
	pattern := filepath.Join(samplesDir, "memcached_v1alpha1_*.yaml")
	matches, err := filepath.Glob(pattern)
	if err != nil {
		t.Fatalf("failed to glob sample files: %v", err)
	}

	if len(matches) == 0 {
		t.Fatalf("no sample files found")
	}

	for _, fullPath := range matches {
		filename := filepath.Base(fullPath)
		t.Run(filename, func(t *testing.T) {
			memcached := loadSampleYAML(t, filename)

			// All samples must have TypeMeta set
			if memcached.APIVersion == "" {
				t.Error("TypeMeta.APIVersion is empty")
			}
			if memcached.Kind == "" {
				t.Error("TypeMeta.Kind is empty")
			}

			// All samples must have metadata
			if memcached.Name == "" {
				t.Error("ObjectMeta.Name is empty")
			}

			// All samples should have labels
			if len(memcached.Labels) == 0 {
				t.Logf("INFO: no labels found in %q - consider adding labels for better organization", filename)
			}

			// Spec should be present (even if minimal)
			// Note: In Go, struct fields are never nil, but pointer fields can be
			// The Spec field itself is a struct, so we check if replicas is set as a basic validation
			if memcached.Spec.Replicas == nil && memcached.Spec.Image == nil {
				t.Logf("INFO: spec appears minimal in %q - consider setting at least replicas or image", filename)
			}
		})
	}
}
