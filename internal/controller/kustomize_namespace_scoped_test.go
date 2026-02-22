package controller_test

import (
	"strings"
	"testing"
)

func TestKustomizeBuildNamespaceScoped_SucceedsWithoutErrors(t *testing.T) {
	if cachedNamespaceScopedOutput == "" {
		t.Fatal("kustomize build produced empty output")
	}
}

func TestKustomizeBuildNamespaceScoped_ContainsRoleNotClusterRole(t *testing.T) {
	docs := strings.Split(cachedNamespaceScopedOutput, "\n---\n")

	foundRole := false
	for _, doc := range docs {
		if strings.Contains(doc, "name: manager-role") {
			if strings.Contains(doc, "kind: ClusterRole") {
				t.Error("manager-role should be Role, not ClusterRole")
			}
			if !strings.Contains(doc, "kind: Role") {
				t.Error("manager-role document does not contain kind: Role")
			}
			foundRole = true
		}
	}
	if !foundRole {
		t.Error("kustomize build output does not contain manager-role")
	}
}

func TestKustomizeBuildNamespaceScoped_ContainsRoleBindingNotClusterRoleBinding(t *testing.T) {
	docs := strings.Split(cachedNamespaceScopedOutput, "\n---\n")

	foundBinding := false
	for _, doc := range docs {
		if strings.Contains(doc, "name: manager-rolebinding") {
			if strings.Contains(doc, "kind: ClusterRoleBinding") {
				t.Error("manager-rolebinding should be RoleBinding, not ClusterRoleBinding")
			}
			if !strings.Contains(doc, "kind: RoleBinding") {
				t.Error("manager-rolebinding document does not contain kind: RoleBinding")
			}
			foundBinding = true
		}
	}
	if !foundBinding {
		t.Error("kustomize build output does not contain manager-rolebinding")
	}
}

func TestKustomizeBuildNamespaceScoped_RoleBindingRefsRole(t *testing.T) {
	docs := strings.Split(cachedNamespaceScopedOutput, "\n---\n")

	for _, doc := range docs {
		if strings.Contains(doc, "name: manager-rolebinding") && strings.Contains(doc, "kind: RoleBinding") {
			// Parse the roleRef block to verify kind is Role (not ClusterRole).
			lines := strings.Split(doc, "\n")
			inRoleRef := false
			found := false
			for _, line := range lines {
				trimmed := strings.TrimSpace(line)
				if trimmed == "roleRef:" {
					inRoleRef = true
					continue
				}
				if inRoleRef && strings.HasPrefix(trimmed, "kind:") {
					if trimmed != "kind: Role" {
						t.Errorf("roleRef.kind should be Role, got %q", trimmed)
					}
					found = true
					break
				}
				// If we hit a non-indented line after roleRef, we've left the block.
				if inRoleRef && !strings.HasPrefix(line, " ") && !strings.HasPrefix(line, "\t") && trimmed != "" {
					break
				}
			}
			if !found {
				t.Error("could not find roleRef.kind in manager-rolebinding")
			}
			return
		}
	}
	t.Error("kustomize build output does not contain manager-rolebinding RoleBinding")
}
