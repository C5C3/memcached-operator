package controller_test

import (
	"os"
	"path/filepath"
	"runtime"
	"sort"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	rbacv1 "k8s.io/api/rbac/v1"
	"sigs.k8s.io/yaml"
)

// loadRBACRole reads and parses config/rbac/role.yaml into a ClusterRole.
func loadRBACRole() (*rbacv1.ClusterRole, error) {
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		return nil, os.ErrNotExist
	}
	rolePath := filepath.Join(filepath.Dir(filename), "..", "..", "config", "rbac", "role.yaml")
	data, err := os.ReadFile(rolePath)
	if err != nil {
		return nil, err
	}
	role := &rbacv1.ClusterRole{}
	if err := yaml.Unmarshal(data, role); err != nil {
		return nil, err
	}
	return role, nil
}

// findRule returns the first PolicyRule matching the given apiGroup and resource.
func findRule(rules []rbacv1.PolicyRule, apiGroup, resource string) *rbacv1.PolicyRule {
	for i := range rules {
		for _, g := range rules[i].APIGroups {
			if g != apiGroup {
				continue
			}
			for _, r := range rules[i].Resources {
				if r == resource {
					return &rules[i]
				}
			}
		}
	}
	return nil
}

// sortedVerbs returns a sorted copy of the verbs slice for deterministic comparison.
func sortedVerbs(verbs []string) []string {
	sorted := make([]string, len(verbs))
	copy(sorted, verbs)
	sort.Strings(sorted)
	return sorted
}

var _ = Describe("RBAC Manifest Verification (REQ-007)", func() {

	var role *rbacv1.ClusterRole

	BeforeEach(func() {
		var err error
		role, err = loadRBACRole()
		Expect(err).NotTo(HaveOccurred())
		Expect(role).NotTo(BeNil())
	})

	Context("ClusterRole metadata", func() {
		It("should have the correct name", func() {
			Expect(role.Name).To(Equal("manager-role"))
		})
	})

	fullCRUDVerbs := []string{"create", "delete", "get", "list", "patch", "update", "watch"}

	Context("Memcached CR permissions", func() {
		It("should grant full CRUD on memcacheds", func() {
			rule := findRule(role.Rules, "memcached.c5c3.io", "memcacheds")
			Expect(rule).NotTo(BeNil(), "rule for memcacheds not found")
			Expect(sortedVerbs(rule.Verbs)).To(Equal(fullCRUDVerbs))
		})

		It("should grant get, update, patch on memcacheds/status", func() {
			rule := findRule(role.Rules, "memcached.c5c3.io", "memcacheds/status")
			Expect(rule).NotTo(BeNil(), "rule for memcacheds/status not found")
			Expect(sortedVerbs(rule.Verbs)).To(Equal([]string{"get", "patch", "update"}))
		})

		It("should grant update on memcacheds/finalizers", func() {
			rule := findRule(role.Rules, "memcached.c5c3.io", "memcacheds/finalizers")
			Expect(rule).NotTo(BeNil(), "rule for memcacheds/finalizers not found")
			Expect(sortedVerbs(rule.Verbs)).To(Equal([]string{"update"}))
		})
	})

	Context("owned resource permissions", func() {
		DescribeTable("should grant full CRUD verbs",
			func(apiGroup, resource string) {
				rule := findRule(role.Rules, apiGroup, resource)
				Expect(rule).NotTo(BeNil(), "rule for %s/%s not found", apiGroup, resource)
				Expect(sortedVerbs(rule.Verbs)).To(Equal(fullCRUDVerbs))
			},
			Entry("Deployments", "apps", "deployments"),
			Entry("Services", "", "services"),
			Entry("PodDisruptionBudgets", "policy", "poddisruptionbudgets"),
			Entry("NetworkPolicies", "networking.k8s.io", "networkpolicies"),
			Entry("ServiceMonitors", "monitoring.coreos.com", "servicemonitors"),
		)
	})

	Context("events permission", func() {
		It("should grant create and patch on events", func() {
			rule := findRule(role.Rules, "", "events")
			Expect(rule).NotTo(BeNil(), "rule for events not found")
			Expect(sortedVerbs(rule.Verbs)).To(Equal([]string{"create", "patch"}))
		})
	})
})
