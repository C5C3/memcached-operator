// Package v1beta1 contains tests that validate example CR YAML files
// in config/samples/ parse correctly as Memcached objects.
package v1beta1

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

	// Discover all memcached_v1beta1_*.yaml files
	pattern := filepath.Join(samplesDir, "memcached_v1beta1_*.yaml")
	matches, err := filepath.Glob(pattern)
	if err != nil {
		t.Fatalf("failed to glob sample files: %v", err)
	}

	if len(matches) == 0 {
		t.Fatalf("no sample files found matching pattern %q - check path configuration", pattern)
	}

	t.Logf("Found %d sample YAML files to validate", len(matches))

	for _, fullPath := range matches {
		filename := filepath.Base(fullPath)
		t.Run(filename, func(t *testing.T) {
			memcached := loadSampleYAML(t, filename)

			expectedAPIVersion := "memcached.c5c3.io/v1beta1"
			if memcached.APIVersion != expectedAPIVersion {
				t.Errorf("apiVersion: got %q, want %q", memcached.APIVersion, expectedAPIVersion)
			}

			expectedKind := "Memcached" //nolint:goconst // test literal
			if memcached.Kind != expectedKind {
				t.Errorf("kind: got %q, want %q", memcached.Kind, expectedKind)
			}

			if memcached.Name == "" {
				t.Error("metadata.name is empty, expected a non-empty name")
			}

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
	memcached := loadSampleYAML(t, "memcached_v1beta1_minimal.yaml")

	if memcached.Name == "" {
		t.Error("expected non-empty metadata.name")
	}

	if memcached.Spec.Replicas == nil {
		t.Error("expected spec.replicas to be set in minimal sample")
	}
}

// TestSampleYAMLHA validates the high availability sample.
func TestSampleYAMLHA(t *testing.T) {
	memcached := loadSampleYAML(t, "memcached_v1beta1_ha.yaml")

	if memcached.Name == "" {
		t.Error("expected non-empty metadata.name")
	}

	if memcached.Spec.HighAvailability == nil {
		t.Error("expected spec.highAvailability to be set in HA sample")
	}
}

// TestSampleYAMLMonitoring validates the monitoring sample.
func TestSampleYAMLMonitoring(t *testing.T) {
	memcached := loadSampleYAML(t, "memcached_v1beta1_monitoring.yaml")

	if memcached.Name == "" {
		t.Error("expected non-empty metadata.name")
	}

	if memcached.Spec.Monitoring == nil {
		t.Fatal("expected spec.monitoring to be set in monitoring sample")
	}
	if !memcached.Spec.Monitoring.Enabled {
		t.Error("expected spec.monitoring.enabled to be true in monitoring sample")
	}
}

// TestSampleYAMLTLS validates the TLS sample.
func TestSampleYAMLTLS(t *testing.T) {
	memcached := loadSampleYAML(t, "memcached_v1beta1_tls.yaml")

	if memcached.Name == "" {
		t.Error("expected non-empty metadata.name")
	}

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
	memcached := loadSampleYAML(t, "memcached_v1beta1_production.yaml")

	if memcached.Name == "" {
		t.Error("expected non-empty metadata.name")
	}

	if memcached.Spec.Replicas != nil && *memcached.Spec.Replicas < 2 {
		t.Errorf("expected production sample to have >= 2 replicas, got %d", *memcached.Spec.Replicas)
	}

	if memcached.Spec.Resources == nil {
		t.Error("expected spec.resources to be set in production sample")
	}

	if memcached.Spec.HighAvailability == nil {
		t.Error("expected spec.highAvailability to be set in production sample")
	}
}

// TestSampleYAMLFull validates the full sample has all major configuration sections.
func TestSampleYAMLFull(t *testing.T) {
	memcached := loadSampleYAML(t, "memcached_v1beta1_full.yaml")

	if memcached.Name == "" {
		t.Error("expected non-empty metadata.name")
	}

	if memcached.Spec.Memcached == nil {
		t.Error("expected spec.memcached to be set in full sample")
	}

	if memcached.Spec.Monitoring == nil {
		t.Error("expected spec.monitoring to be set in full sample")
	}

	if memcached.Spec.Security == nil {
		t.Error("expected spec.security to be set in full sample")
	}

	if memcached.Spec.HighAvailability == nil {
		t.Error("expected spec.highAvailability to be set in full sample")
	}
}

// TestAllSamplesHaveValidStructure is a comprehensive test that validates
// structural requirements across all v1beta1 samples.
func TestAllSamplesHaveValidStructure(t *testing.T) {
	samplesDir := samplesDirPath(t)
	pattern := filepath.Join(samplesDir, "memcached_v1beta1_*.yaml")
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

			if memcached.APIVersion == "" {
				t.Error("TypeMeta.APIVersion is empty")
			}
			if memcached.Kind == "" {
				t.Error("TypeMeta.Kind is empty")
			}

			if memcached.Name == "" {
				t.Error("ObjectMeta.Name is empty")
			}

			if len(memcached.Labels) == 0 {
				t.Logf("INFO: no labels found in %q - consider adding labels for better organization", filename)
			}

			if memcached.Spec.Replicas == nil && memcached.Spec.Image == nil {
				t.Logf("INFO: spec appears minimal in %q - consider setting at least replicas or image", filename)
			}
		})
	}
}
