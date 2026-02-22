package main

import (
	"reflect"
	"testing"

	"sigs.k8s.io/controller-runtime/pkg/cache"
)

func TestParseWatchNamespaces(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected map[string]cache.Config
	}{
		{
			name:     "empty string returns nil",
			input:    "",
			expected: nil,
		},
		{
			name:     "whitespace-only returns nil",
			input:    "   ",
			expected: nil,
		},
		{
			name:  "single namespace",
			input: "production",
			expected: map[string]cache.Config{
				"production": {},
			},
		},
		{
			name:  "single namespace with whitespace",
			input: " production ",
			expected: map[string]cache.Config{
				"production": {},
			},
		},
		{
			name:  "multiple namespaces",
			input: "ns1,ns2",
			expected: map[string]cache.Config{
				"ns1": {},
				"ns2": {},
			},
		},
		{
			name:  "multiple namespaces with whitespace",
			input: " ns1 , ns2 , ns3 ",
			expected: map[string]cache.Config{
				"ns1": {},
				"ns2": {},
				"ns3": {},
			},
		},
		{
			name:  "trailing comma skips empty segment",
			input: "ns1,ns2,",
			expected: map[string]cache.Config{
				"ns1": {},
				"ns2": {},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseWatchNamespaces(tt.input)

			if tt.expected == nil {
				if result != nil {
					t.Fatalf("expected nil, got %v", result)
				}
				return
			}

			if result == nil {
				t.Fatalf("expected %v, got nil", tt.expected)
			}

			if len(result) != len(tt.expected) {
				t.Fatalf("expected %d entries, got %d: %v", len(tt.expected), len(result), result)
			}

			for ns := range tt.expected {
				cfg, ok := result[ns]
				if !ok {
					t.Errorf("expected key %q not found in result %v", ns, result)
					continue
				}
				if !reflect.DeepEqual(cfg, cache.Config{}) {
					t.Errorf("expected empty cache.Config for key %q, got %v", ns, cfg)
				}
			}
		})
	}
}
