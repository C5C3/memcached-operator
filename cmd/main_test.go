package main

import (
	"crypto/tls"
	"reflect"
	"testing"

	"sigs.k8s.io/controller-runtime/pkg/cache"
)

func TestBuildWebhookServer(t *testing.T) {
	tests := []struct {
		name    string
		enabled bool
		tlsOpts []func(*tls.Config)
		wantNil bool
	}{
		{
			name:    "enabled returns non-nil server",
			enabled: true,
			tlsOpts: nil,
			wantNil: false,
		},
		{
			name:    "enabled with TLS opts returns non-nil server",
			enabled: true,
			tlsOpts: []func(*tls.Config){
				func(c *tls.Config) { c.NextProtos = []string{"http/1.1"} },
			},
			wantNil: false,
		},
		{
			name:    "disabled returns nil",
			enabled: false,
			tlsOpts: nil,
			wantNil: true,
		},
		{
			name:    "disabled with TLS opts still returns nil",
			enabled: false,
			tlsOpts: []func(*tls.Config){
				func(c *tls.Config) { c.NextProtos = []string{"http/1.1"} },
			},
			wantNil: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := buildWebhookServer(tt.enabled, tt.tlsOpts)
			if tt.wantNil && result != nil {
				t.Fatalf("expected nil, got %v", result)
			}
			if !tt.wantNil && result == nil {
				t.Fatal("expected non-nil webhook.Server, got nil")
			}
		})
	}
}

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
