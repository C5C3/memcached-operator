// Package v1alpha1 contains CRD YAML structural verification tests that parse
// the generated manifest and verify its structure programmatically.
package v1alpha1

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"sigs.k8s.io/yaml"
)

// loadCRD reads and parses the CRD YAML file into a CustomResourceDefinition struct.
func loadCRD(t *testing.T) *apiextensionsv1.CustomResourceDefinition {
	t.Helper()
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("unable to get caller info")
	}
	crdPath := filepath.Join(filepath.Dir(filename), "..", "..", "config", "crd", "bases", "memcached.c5c3.io_memcacheds.yaml")
	data, err := os.ReadFile(crdPath)
	if err != nil {
		t.Fatalf("failed to read CRD file: %v", err)
	}
	crd := &apiextensionsv1.CustomResourceDefinition{}
	if err := yaml.Unmarshal(data, crd); err != nil {
		t.Fatalf("failed to unmarshal CRD: %v", err)
	}
	return crd
}

// findVersion returns the CRD version with the given name.
func findVersion(t *testing.T, crd *apiextensionsv1.CustomResourceDefinition, name string) apiextensionsv1.CustomResourceDefinitionVersion {
	t.Helper()
	for _, v := range crd.Spec.Versions {
		if v.Name == name {
			return v
		}
	}
	t.Fatalf("version %q not found in CRD", name)
	return apiextensionsv1.CustomResourceDefinitionVersion{}
}

// getVersionSchema returns the openAPIV3Schema for the v1alpha1 version.
func getVersionSchema(t *testing.T, crd *apiextensionsv1.CustomResourceDefinition) *apiextensionsv1.JSONSchemaProps {
	t.Helper()
	v := findVersion(t, crd, "v1alpha1")
	schema := v.Schema
	if schema == nil || schema.OpenAPIV3Schema == nil {
		t.Fatal("CRD version has no OpenAPIV3Schema")
	}
	return schema.OpenAPIV3Schema
}

// getNestedProperty navigates through the schema property tree using the given path.
func getNestedProperty(t *testing.T, schema *apiextensionsv1.JSONSchemaProps, path ...string) *apiextensionsv1.JSONSchemaProps {
	t.Helper()
	current := schema
	for _, key := range path {
		prop, ok := current.Properties[key]
		if !ok {
			t.Fatalf("property %q not found in path %v", key, path)
		}
		current = &prop
	}
	return current
}

func TestCRDMetadata(t *testing.T) {
	crd := loadCRD(t)

	tests := []struct {
		name string
		got  string
		want string
	}{
		{"apiVersion", crd.APIVersion, "apiextensions.k8s.io/v1"},
		{"kind", crd.Kind, "CustomResourceDefinition"},
		{"metadata.name", crd.Name, "memcacheds.memcached.c5c3.io"},
		{"spec.group", crd.Spec.Group, "memcached.c5c3.io"},
		{"spec.names.kind", crd.Spec.Names.Kind, "Memcached"},
		{"spec.names.listKind", crd.Spec.Names.ListKind, "MemcachedList"},
		{"spec.names.plural", crd.Spec.Names.Plural, "memcacheds"},
		{"spec.names.singular", crd.Spec.Names.Singular, "memcached"},
		{"spec.scope", string(crd.Spec.Scope), "Namespaced"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.got != tt.want {
				t.Errorf("got %q, want %q", tt.got, tt.want)
			}
		})
	}
}

func TestCRDVersion(t *testing.T) {
	crd := loadCRD(t)

	t.Run("v1alpha1 exists and is served", func(t *testing.T) {
		v := findVersion(t, crd, "v1alpha1")
		if !v.Served {
			t.Error("expected v1alpha1 to be served")
		}
	})

	t.Run("v1beta1 exists and is served", func(t *testing.T) {
		v := findVersion(t, crd, "v1beta1")
		if !v.Served {
			t.Error("expected v1beta1 to be served")
		}
	})

	t.Run("exactly one storage version", func(t *testing.T) {
		var storageVersions []string
		for _, v := range crd.Spec.Versions {
			if v.Storage {
				storageVersions = append(storageVersions, v.Name)
			}
		}
		if len(storageVersions) != 1 {
			t.Fatalf("expected exactly 1 storage version, got %v", storageVersions)
		}
	})

	t.Run("v1beta1 is storage version", func(t *testing.T) {
		v := findVersion(t, crd, "v1beta1")
		if !v.Storage {
			t.Error("expected v1beta1 to be the storage version")
		}
	})
}

func TestCRDSubresources(t *testing.T) {
	crd := loadCRD(t)
	v := findVersion(t, crd, "v1alpha1")

	t.Run("status subresource enabled", func(t *testing.T) {
		if v.Subresources == nil {
			t.Fatal("expected subresources to be defined")
		}
		if v.Subresources.Status == nil {
			t.Error("expected status subresource to be enabled")
		}
	})
}

func TestCRDPrinterColumns(t *testing.T) {
	crd := loadCRD(t)

	columnsByName := make(map[string]apiextensionsv1.CustomResourceColumnDefinition)
	v := findVersion(t, crd, "v1alpha1")
	for _, col := range v.AdditionalPrinterColumns {
		columnsByName[col.Name] = col
	}

	tests := []struct {
		name     string
		jsonPath string
		colType  string
	}{
		{"Replicas", ".spec.replicas", "integer"},
		{"Ready", ".status.readyReplicas", "integer"},
		{"Age", ".metadata.creationTimestamp", "date"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			col, ok := columnsByName[tt.name]
			if !ok {
				t.Fatalf("printer column %q not found", tt.name)
			}
			if col.JSONPath != tt.jsonPath {
				t.Errorf("expected JSONPath %q, got %q", tt.jsonPath, col.JSONPath)
			}
			if col.Type != tt.colType {
				t.Errorf("expected type %q, got %q", tt.colType, col.Type)
			}
		})
	}
}

func TestCRDSchemaTopLevelProperties(t *testing.T) {
	crd := loadCRD(t)
	schema := getVersionSchema(t, crd)

	t.Run("spec and status exist", func(t *testing.T) {
		for _, prop := range []string{"spec", "status"} {
			if _, ok := schema.Properties[prop]; !ok {
				t.Errorf("top-level property %q not found", prop)
			}
		}
	})

	t.Run("spec properties", func(t *testing.T) {
		specSchema := getNestedProperty(t, schema, "spec")
		expectedProps := []string{
			"replicas", "image", "resources", "memcached",
			"highAvailability", "monitoring", "security",
		}
		for _, prop := range expectedProps {
			if _, ok := specSchema.Properties[prop]; !ok {
				t.Errorf("spec property %q not found", prop)
			}
		}
	})

	t.Run("status properties", func(t *testing.T) {
		statusSchema := getNestedProperty(t, schema, "status")
		expectedProps := []string{"conditions", "readyReplicas", "observedGeneration"}
		for _, prop := range expectedProps {
			if _, ok := statusSchema.Properties[prop]; !ok {
				t.Errorf("status property %q not found", prop)
			}
		}
	})
}

func TestCRDSchemaValidationMarkers(t *testing.T) {
	crd := loadCRD(t)
	schema := getVersionSchema(t, crd)

	t.Run("spec.replicas min/max", func(t *testing.T) {
		prop := getNestedProperty(t, schema, "spec", "replicas")
		if prop.Minimum == nil || *prop.Minimum != 0 {
			t.Errorf("expected spec.replicas minimum=0, got %v", prop.Minimum)
		}
		if prop.Maximum == nil || *prop.Maximum != 64 {
			t.Errorf("expected spec.replicas maximum=64, got %v", prop.Maximum)
		}
	})

	t.Run("spec.memcached.maxMemoryMB min/max", func(t *testing.T) {
		prop := getNestedProperty(t, schema, "spec", "memcached", "maxMemoryMB")
		if prop.Minimum == nil || *prop.Minimum != 16 {
			t.Errorf("expected minimum=16, got %v", prop.Minimum)
		}
		if prop.Maximum == nil || *prop.Maximum != 65536 {
			t.Errorf("expected maximum=65536, got %v", prop.Maximum)
		}
	})

	t.Run("spec.memcached.threads min/max", func(t *testing.T) {
		prop := getNestedProperty(t, schema, "spec", "memcached", "threads")
		if prop.Minimum == nil || *prop.Minimum != 1 {
			t.Errorf("expected minimum=1, got %v", prop.Minimum)
		}
		if prop.Maximum == nil || *prop.Maximum != 128 {
			t.Errorf("expected maximum=128, got %v", prop.Maximum)
		}
	})

	t.Run("spec.memcached.maxItemSize pattern", func(t *testing.T) {
		prop := getNestedProperty(t, schema, "spec", "memcached", "maxItemSize")
		expectedPattern := `^[0-9]+(k|m)$`
		if prop.Pattern != expectedPattern {
			t.Errorf("expected pattern %q, got %q", expectedPattern, prop.Pattern)
		}
	})

	t.Run("spec.memcached.verbosity min/max", func(t *testing.T) {
		prop := getNestedProperty(t, schema, "spec", "memcached", "verbosity")
		if prop.Minimum == nil || *prop.Minimum != 0 {
			t.Errorf("expected minimum=0, got %v", prop.Minimum)
		}
		if prop.Maximum == nil || *prop.Maximum != 2 {
			t.Errorf("expected maximum=2, got %v", prop.Maximum)
		}
	})

	t.Run("spec.security.tls has enableClientCert property", func(t *testing.T) {
		tlsSchema := getNestedProperty(t, schema, "spec", "security", "tls")
		expectedProps := []string{"enabled", "certificateSecretRef", "enableClientCert"}
		for _, prop := range expectedProps {
			if _, ok := tlsSchema.Properties[prop]; !ok {
				t.Errorf("tls property %q not found", prop)
			}
		}
	})

	t.Run("spec.security.tls.enableClientCert is boolean", func(t *testing.T) {
		prop := getNestedProperty(t, schema, "spec", "security", "tls", "enableClientCert")
		if prop.Type != "boolean" {
			t.Errorf("expected type boolean, got %q", prop.Type)
		}
	})

	t.Run("spec.highAvailability.antiAffinityPreset enum", func(t *testing.T) {
		prop := getNestedProperty(t, schema, "spec", "highAvailability", "antiAffinityPreset")
		if len(prop.Enum) != 2 {
			t.Fatalf("expected 2 enum values, got %d", len(prop.Enum))
		}
		enumValues := make([]string, len(prop.Enum))
		for i, e := range prop.Enum {
			var s string
			if err := json.Unmarshal(e.Raw, &s); err != nil {
				t.Fatalf("failed to unmarshal enum value: %v", err)
			}
			enumValues[i] = s
		}
		expected := map[string]bool{"soft": true, "hard": true}
		for _, v := range enumValues {
			if !expected[v] {
				t.Errorf("unexpected enum value %q", v)
			}
		}
		if len(enumValues) != len(expected) {
			t.Errorf("expected enum values %v, got %v", expected, enumValues)
		}
	})
}

func TestCRDSchemaDefaultValues(t *testing.T) {
	crd := loadCRD(t)
	schema := getVersionSchema(t, crd)

	tests := []struct {
		name         string
		path         []string
		expectedJSON string
	}{
		{"spec.image default=memcached:1.6", []string{"spec", "image"}, `"memcached:1.6"`},
		{"spec.memcached.maxMemoryMB default=64", []string{"spec", "memcached", "maxMemoryMB"}, "64"},
		{"spec.memcached.maxConnections default=1024", []string{"spec", "memcached", "maxConnections"}, "1024"},
		{"spec.memcached.threads default=4", []string{"spec", "memcached", "threads"}, "4"},
		{"spec.memcached.maxItemSize default=1m", []string{"spec", "memcached", "maxItemSize"}, `"1m"`},
		{"spec.memcached.verbosity default=0", []string{"spec", "memcached", "verbosity"}, "0"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			prop := getNestedProperty(t, schema, tt.path...)
			if prop.Default == nil {
				t.Fatalf("expected default value, got nil")
			}
			got := string(prop.Default.Raw)
			if got != tt.expectedJSON {
				t.Errorf("expected default %s, got %s", tt.expectedJSON, got)
			}
		})
	}
}
