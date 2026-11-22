package main

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"errors"
	"flag"
	"io"
	"reflect"
	"strings"
	"testing"
	"time"

	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

const (
	testNamespaceProduction = "production"
	testFirstNamespace      = "ns1"
	testSecondNamespace     = "ns2"
	testInfoLine            = "info-line"
	testDebugLine           = "debug-line"
	testErrorLine           = "error-line"
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
			input: testNamespaceProduction,
			expected: map[string]cache.Config{
				testNamespaceProduction: {},
			},
		},
		{
			name:  "single namespace with whitespace",
			input: " production ",
			expected: map[string]cache.Config{
				testNamespaceProduction: {},
			},
		},
		{
			name:  "multiple namespaces",
			input: "ns1,ns2",
			expected: map[string]cache.Config{
				testFirstNamespace:  {},
				testSecondNamespace: {},
			},
		},
		{
			name:  "multiple namespaces with whitespace",
			input: " ns1 , ns2 , ns3 ",
			expected: map[string]cache.Config{
				testFirstNamespace:  {},
				testSecondNamespace: {},
				"ns3":               {},
			},
		},
		{
			name:  "trailing comma skips empty segment",
			input: "ns1,ns2,",
			expected: map[string]cache.Config{
				testFirstNamespace:  {},
				testSecondNamespace: {},
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

func TestBindLogFlags(t *testing.T) {
	tests := []struct {
		name         string
		args         []string
		wantParseErr string // substring of the fs.Parse error; empty when parsing succeeds
		wantDevMode  bool
		wantJSON     bool // every line is a JSON object; otherwise the first line is not
		wantInfo     bool
		wantDebug    bool
		wantError    bool
	}{
		{
			name:      "no flags log JSON at info level",
			wantJSON:  true,
			wantInfo:  true,
			wantError: true,
		},
		{
			name:        "zap-devel logs console at debug level",
			args:        []string{"--zap-devel"},
			wantDevMode: true,
			wantInfo:    true,
			wantDebug:   true,
			wantError:   true,
		},
		{
			name:      "zap-devel=false keeps production mode",
			args:      []string{"--zap-devel=false"},
			wantJSON:  true,
			wantInfo:  true,
			wantError: true,
		},
		{
			name:      "zap-log-level=debug adds debug lines",
			args:      []string{"--zap-log-level=debug"},
			wantJSON:  true,
			wantInfo:  true,
			wantDebug: true,
			wantError: true,
		},
		{
			name:      "zap-log-level=error drops info lines",
			args:      []string{"--zap-log-level=error"},
			wantJSON:  true,
			wantError: true,
		},
		{
			name:      "zap-encoder=console keeps info level",
			args:      []string{"--zap-encoder=console"},
			wantInfo:  true,
			wantError: true,
		},
		{
			name:         "zap-log-level=warn is rejected",
			args:         []string{"--zap-log-level=warn"},
			wantParseErr: `invalid log level "warn"`,
		},
		{
			name:         "empty zap-log-level is rejected",
			args:         []string{"--zap-log-level="},
			wantParseErr: `invalid log level ""`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fs := flag.NewFlagSet("manager", flag.ContinueOnError)
			fs.SetOutput(io.Discard)
			opts := bindLogFlags(fs)

			err := fs.Parse(tt.args)
			if tt.wantParseErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantParseErr) {
					t.Fatalf("expected a parse error containing %q, got %v", tt.wantParseErr, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected parse error: %v", err)
			}
			if opts.Development != tt.wantDevMode {
				t.Errorf("expected Development=%t, got %t", tt.wantDevMode, opts.Development)
			}

			var buf bytes.Buffer
			logger := zap.New(zap.UseFlagOptions(opts), zap.WriteTo(&buf))
			logger.Info(testInfoLine)
			logger.V(1).Info(testDebugLine)
			logger.Error(errors.New("boom"), testErrorLine)
			out := buf.String()

			for msg, want := range map[string]bool{
				testInfoLine:  tt.wantInfo,
				testDebugLine: tt.wantDebug,
				testErrorLine: tt.wantError,
			} {
				if got := strings.Contains(out, msg); got != want {
					t.Errorf("expected %s present=%t, got %t in output:\n%s", msg, want, got, out)
				}
			}

			lines := strings.Split(strings.TrimSpace(out), "\n")
			if tt.wantJSON {
				assertJSONLogLines(t, lines)
				return
			}
			var entry map[string]any
			if json.Unmarshal([]byte(lines[0]), &entry) == nil {
				t.Errorf("expected a console line, got a JSON object: %s", lines[0])
			}
			if tt.wantDevMode && !strings.Contains(lines[0], "\tINFO\t") {
				t.Errorf("expected a development console line containing \\tINFO\\t, got %q", lines[0])
			}
		})
	}
}

// assertJSONLogLines checks that every line is a JSON object, that the info
// line carries "level":"info" and an RFC 3339 ts, that the debug line carries
// "level":"debug", and that the error line carries a stack trace.
func assertJSONLogLines(t *testing.T, lines []string) {
	t.Helper()
	entries := make(map[string]map[string]any, len(lines))
	for _, line := range lines {
		var entry map[string]any
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			t.Fatalf("expected a JSON object, got %q: %v", line, err)
		}
		msg, _ := entry["msg"].(string)
		entries[msg] = entry
	}

	if info, ok := entries[testInfoLine]; ok {
		if info["level"] != "info" {
			t.Errorf("expected level info on %s, got %v", testInfoLine, info["level"])
		}
		ts, _ := info["ts"].(string)
		if _, err := time.Parse(time.RFC3339, ts); err != nil {
			t.Errorf("expected an RFC 3339 ts on %s, got %q: %v", testInfoLine, ts, err)
		}
	}
	if debug, ok := entries[testDebugLine]; ok && debug["level"] != "debug" {
		t.Errorf("expected level debug on %s, got %v", testDebugLine, debug["level"])
	}
	if errEntry, ok := entries[testErrorLine]; ok {
		if stack, _ := errEntry["stacktrace"].(string); stack == "" {
			t.Errorf("expected a non-empty stacktrace on %s, got %v", testErrorLine, errEntry)
		}
	}
}
