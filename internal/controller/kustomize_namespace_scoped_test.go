package controller_test

import (
	"strings"
	"testing"
)

// splitNamespaceScopedDocs splits the cached kustomize output into YAML documents.
func splitNamespaceScopedDocs(t *testing.T) []string {
	t.Helper()
	if cachedNamespaceScopedOutput == "" {
		t.Fatal("kustomize build produced empty output")
	}
	return strings.Split(cachedNamespaceScopedOutput, "\n---\n")
}

// findDocByName returns the first YAML document containing "name: <name>\n", or empty string if not found.
func findDocByName(docs []string, name string) string {
	for _, doc := range docs {
		if strings.Contains(doc, "name: "+name+"\n") {
			return doc
		}
	}
	return ""
}

func TestKustomizeBuildNamespaceScoped_ManagerRolesAreNamespaceScoped(t *testing.T) {
	docs := splitNamespaceScopedDocs(t)

	tests := []struct {
		name         string
		resourceName string
		expectedKind string
		rejectedKind string
	}{
		{"manager-role is Role not ClusterRole", "manager-role", "Role", "ClusterRole"},
		{"manager-rolebinding is RoleBinding not ClusterRoleBinding", "manager-rolebinding", "RoleBinding", "ClusterRoleBinding"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doc := findDocByName(docs, tt.resourceName)
			if doc == "" {
				t.Fatalf("kustomize build output does not contain %s", tt.resourceName)
			}
			if strings.Contains(doc, "kind: "+tt.rejectedKind) {
				t.Errorf("%s should be %s, not %s", tt.resourceName, tt.expectedKind, tt.rejectedKind)
			}
			if !strings.Contains(doc, "kind: "+tt.expectedKind) {
				t.Errorf("%s document does not contain kind: %s", tt.resourceName, tt.expectedKind)
			}
		})
	}
}

func TestKustomizeBuildNamespaceScoped_MetricsRolesRemainClusterScoped(t *testing.T) {
	docs := splitNamespaceScopedDocs(t)

	tests := []struct {
		name         string
		resourceName string
		expectedKind string
	}{
		{"metrics-auth-role stays ClusterRole", "metrics-auth-role", "ClusterRole"},
		{"metrics-reader stays ClusterRole", "metrics-reader", "ClusterRole"},
		{"metrics-auth-rolebinding stays ClusterRoleBinding", "metrics-auth-rolebinding", "ClusterRoleBinding"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doc := findDocByName(docs, tt.resourceName)
			if doc == "" {
				t.Fatalf("kustomize build output does not contain %s", tt.resourceName)
			}
			if !strings.Contains(doc, "kind: "+tt.expectedKind) {
				t.Errorf("%s should remain %s", tt.resourceName, tt.expectedKind)
			}
		})
	}
}

func TestKustomizeBuildNamespaceScoped_LeaderElectionResourcesUnchanged(t *testing.T) {
	docs := splitNamespaceScopedDocs(t)

	tests := []struct {
		name         string
		resourceName string
		expectedKind string
	}{
		{"leader-election-role stays Role", "leader-election-role", "Role"},
		{"leader-election-rolebinding stays RoleBinding", "leader-election-rolebinding", "RoleBinding"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doc := findDocByName(docs, tt.resourceName)
			if doc == "" {
				t.Fatalf("kustomize build output does not contain %s", tt.resourceName)
			}
			if !strings.Contains(doc, "kind: "+tt.expectedKind) {
				t.Errorf("%s should remain %s", tt.resourceName, tt.expectedKind)
			}
		})
	}
}

func TestKustomizeBuildNamespaceScoped_ServiceAccountUnchanged(t *testing.T) {
	docs := splitNamespaceScopedDocs(t)

	for _, doc := range docs {
		if strings.Contains(doc, "kind: ServiceAccount") && strings.Contains(doc, "name: controller-manager") {
			return
		}
	}
	t.Error("kustomize build output does not contain ServiceAccount controller-manager")
}

func TestKustomizeBuildNamespaceScoped_TotalResourceCount(t *testing.T) {
	docs := splitNamespaceScopedDocs(t)

	// Filter out empty documents (leading/trailing separators).
	count := 0
	for _, doc := range docs {
		if strings.TrimSpace(doc) != "" {
			count++
		}
	}

	// The namespace-scoped overlay should produce the same 8 resources as config/rbac:
	// manager-role (Role), manager-rolebinding (RoleBinding),
	// leader-election-role, leader-election-rolebinding,
	// controller-manager ServiceAccount,
	// metrics-auth-role, metrics-auth-rolebinding, metrics-reader.
	const expectedCount = 8
	if count != expectedCount {
		t.Errorf("expected %d resources, got %d", expectedCount, count)
	}
}

func TestKustomizeBuildNamespaceScoped_RolePreservesRBACRules(t *testing.T) {
	docs := splitNamespaceScopedDocs(t)

	doc := findDocByName(docs, "manager-role")
	if doc == "" || !strings.Contains(doc, "kind: Role") {
		t.Fatal("kustomize build output does not contain Role manager-role")
	}

	// These are representative rule fragments from config/rbac/role.yaml that must
	// survive the ClusterRole-to-Role conversion.
	requiredRuleFragments := []struct {
		name    string
		keyword string
	}{
		{"secrets read access", "- secrets"},
		{"services CRUD", "- services"},
		{"deployments CRUD", "- deployments"},
		{"memcacheds CRD access", "- memcacheds"},
		{"memcacheds finalizers", "- memcacheds/finalizers"},
		{"memcacheds status", "- memcacheds/status"},
		{"horizontalpodautoscalers CRUD", "- horizontalpodautoscalers"},
		{"servicemonitors CRUD", "- servicemonitors"},
		{"networkpolicies CRUD", "- networkpolicies"},
		{"poddisruptionbudgets CRUD", "- poddisruptionbudgets"},
		{"events create", "- events"},
	}

	for _, tt := range requiredRuleFragments {
		t.Run(tt.name, func(t *testing.T) {
			if !strings.Contains(doc, tt.keyword) {
				t.Errorf("Role manager-role is missing rule for %s", tt.name)
			}
		})
	}
}

func TestKustomizeBuildNamespaceScoped_RoleBindingRefsRole(t *testing.T) {
	docs := splitNamespaceScopedDocs(t)

	doc := findDocByName(docs, "manager-rolebinding")
	if doc == "" || !strings.Contains(doc, "kind: RoleBinding") {
		t.Fatal("kustomize build output does not contain manager-rolebinding RoleBinding")
	}

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
}
