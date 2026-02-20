// Package controller_test contains kustomize build validation tests for config/default/.
package controller_test

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// cachedKustomizeOutput holds the kustomize build output, computed once in TestMain.
var cachedKustomizeOutput string

func TestMain(m *testing.M) {
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		fmt.Fprintln(os.Stderr, "unable to determine test file location")
		os.Exit(1)
	}
	bin := filepath.Join(filepath.Dir(thisFile), "..", "..", "bin", "kustomize")
	if _, err := os.Stat(bin); err != nil {
		fmt.Fprintf(os.Stderr, "kustomize binary not found at %s: %v — skipping kustomize tests\n", bin, err)
		os.Exit(0)
	}
	configDir := filepath.Join(filepath.Dir(thisFile), "..", "..", "config", "default")
	cmd := exec.Command(bin, "build", configDir)
	out, err := cmd.CombinedOutput()
	if err != nil {
		fmt.Fprintf(os.Stderr, "kustomize build failed: %v\nOutput:\n%s\n", err, string(out))
		os.Exit(1)
	}
	cachedKustomizeOutput = string(out)
	os.Exit(m.Run())
}

func TestKustomizeBuildDefault_SucceedsWithoutErrors(t *testing.T) {
	// Build success is already validated in TestMain; verify output is non-empty.
	if cachedKustomizeOutput == "" {
		t.Fatal("kustomize build produced empty output")
	}
}

func TestKustomizeBuildDefault_IncludesCertManagerResources(t *testing.T) {
	tests := []struct {
		name    string
		keyword string
	}{
		{"Issuer resource present", "\nkind: Issuer\n"},
		{"Certificate resource present", "\nkind: Certificate\n"},
		{"Issuer is self-signed", "selfSigned: {}"},
		{"Issuer in operator namespace", "namespace: memcached-operator-system"},
		{"Certificate has webhook-server-cert secret", "secretName: webhook-server-cert"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if !strings.Contains(cachedKustomizeOutput, tt.keyword) {
				t.Errorf("kustomize build output does not contain %q", tt.keyword)
			}
		})
	}
}

func TestKustomizeBuildDefault_WebhookConfigsHaveCAInjectionAnnotation(t *testing.T) {
	docs := strings.Split(cachedKustomizeOutput, "\n---\n")

	for _, kind := range []string{"MutatingWebhookConfiguration", "ValidatingWebhookConfiguration"} {
		t.Run(kind, func(t *testing.T) {
			for _, doc := range docs {
				if !strings.Contains(doc, "kind: "+kind) {
					continue
				}
				if !strings.Contains(doc, "cert-manager.io/inject-ca-from:") {
					t.Errorf("%s is missing cert-manager.io/inject-ca-from annotation", kind)
				}
				if !strings.Contains(doc, "memcached-operator-system/memcached-operator-serving-cert") {
					t.Errorf("%s has incorrect inject-ca-from value", kind)
				}
				return
			}
			t.Errorf("kustomize build output does not contain %s", kind)
		})
	}
}

func TestKustomizeBuildDefault_ManagerHasCertVolumeAndWebhookPort(t *testing.T) {
	docs := strings.Split(cachedKustomizeOutput, "\n---\n")

	for _, doc := range docs {
		if strings.Contains(doc, "kind: Deployment") && strings.Contains(doc, "controller-manager") {
			// Verify webhook-server port.
			if !strings.Contains(doc, "containerPort: 9443") {
				t.Error("manager Deployment is missing containerPort 9443")
			}
			if !strings.Contains(doc, "name: webhook-server") {
				t.Error("manager Deployment is missing webhook-server port name")
			}

			// Verify cert volume mount.
			if !strings.Contains(doc, "mountPath: /tmp/k8s-webhook-server/serving-certs") {
				t.Error("manager Deployment is missing cert volume mount path")
			}
			if !strings.Contains(doc, "readOnly: true") {
				t.Error("manager Deployment cert volume mount should be readOnly")
			}

			// Verify cert volume from secret.
			if !strings.Contains(doc, "secretName: webhook-server-cert") {
				t.Error("manager Deployment is missing cert volume from webhook-server-cert secret")
			}
			return
		}
	}
	t.Error("kustomize build output does not contain manager Deployment")
}

func TestKustomizeBuildDefault_WebhookServicePortMapping(t *testing.T) {
	docs := strings.Split(cachedKustomizeOutput, "\n---\n")

	for _, doc := range docs {
		if strings.Contains(doc, "kind: Service") && strings.Contains(doc, "webhook-service") {
			if !strings.Contains(doc, "port: 443") {
				t.Error("webhook Service should expose port 443")
			}
			if !strings.Contains(doc, "targetPort: 9443") {
				t.Error("webhook Service should route to targetPort 9443")
			}
			if !strings.Contains(doc, "control-plane: controller-manager") {
				t.Error("webhook Service selector should match controller-manager pods")
			}
			return
		}
	}
	t.Error("kustomize build output does not contain webhook Service")
}
