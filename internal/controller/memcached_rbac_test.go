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

// rbacConfigPath returns the absolute path to a file under config/rbac/,
// resolved relative to this test file's location.
func rbacConfigPath(filename string) (string, error) {
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		return "", os.ErrNotExist
	}
	return filepath.Join(filepath.Dir(thisFile), "..", "..", "config", "rbac", filename), nil
}

// loadRBACRole reads and parses config/rbac/role.yaml into a ClusterRole.
func loadRBACRole() (*rbacv1.ClusterRole, error) {
	path, err := rbacConfigPath("role.yaml")
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	role := &rbacv1.ClusterRole{}
	if err := yaml.Unmarshal(data, role); err != nil {
		return nil, err
	}
	return role, nil
}

// loadRBACRoleBinding reads and parses config/rbac/role_binding.yaml into a ClusterRoleBinding.
func loadRBACRoleBinding() (*rbacv1.ClusterRoleBinding, error) {
	path, err := rbacConfigPath("role_binding.yaml")
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	binding := &rbacv1.ClusterRoleBinding{}
	if err := yaml.Unmarshal(data, binding); err != nil {
		return nil, err
	}
	return binding, nil
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

		It("should have exactly 11 rules to prevent permission creep", func() {
			Expect(role.Rules).To(HaveLen(11), "unexpected number of rules — update this test if a new rule is legitimately needed")
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
			Entry("HorizontalPodAutoscalers", "autoscaling", "horizontalpodautoscalers"),
			Entry("Services", "", "services"),
			Entry("PodDisruptionBudgets", "policy", "poddisruptionbudgets"),
			Entry("NetworkPolicies", "networking.k8s.io", "networkpolicies"),
			Entry("ServiceMonitors", "monitoring.coreos.com", "servicemonitors"),
		)
	})

	Context("Secrets permission", func() {
		It("should grant read-only access on secrets", func() {
			rule := findRule(role.Rules, "", "secrets")
			Expect(rule).NotTo(BeNil(), "rule for secrets not found")
			Expect(sortedVerbs(rule.Verbs)).To(Equal([]string{"get", "list", "watch"}))
		})
	})

	Context("events permission", func() {
		It("should grant create and patch on events", func() {
			rule := findRule(role.Rules, "", "events")
			Expect(rule).NotTo(BeNil(), "rule for events not found")
			Expect(sortedVerbs(rule.Verbs)).To(Equal([]string{"create", "patch"}))
		})
	})

	Context("least-privilege constraints", func() {
		It("should not contain wildcard verbs", func() {
			for _, rule := range role.Rules {
				for _, verb := range rule.Verbs {
					Expect(verb).NotTo(Equal("*"), "wildcard verb found in rule for %v", rule.Resources)
				}
			}
		})

		It("should not contain wildcard resources", func() {
			for _, rule := range role.Rules {
				for _, resource := range rule.Resources {
					Expect(resource).NotTo(Equal("*"), "wildcard resource found in rule for apiGroups %v", rule.APIGroups)
				}
			}
		})

		It("should not contain wildcard API groups", func() {
			for _, rule := range role.Rules {
				for _, group := range rule.APIGroups {
					Expect(group).NotTo(Equal("*"), "wildcard apiGroup found in rule for %v", rule.Resources)
				}
			}
		})
	})

})

var _ = Describe("RBAC ClusterRoleBinding Verification (REQ-011)", func() {

	var binding *rbacv1.ClusterRoleBinding

	BeforeEach(func() {
		var err error
		binding, err = loadRBACRoleBinding()
		Expect(err).NotTo(HaveOccurred())
		Expect(binding).NotTo(BeNil())
	})

	It("should reference the manager-role ClusterRole", func() {
		Expect(binding.RoleRef.Kind).To(Equal("ClusterRole"))
		Expect(binding.RoleRef.Name).To(Equal("manager-role"))
		Expect(binding.RoleRef.APIGroup).To(Equal("rbac.authorization.k8s.io"))
	})

	It("should bind to the controller-manager ServiceAccount in system namespace", func() {
		Expect(binding.Subjects).To(HaveLen(1))
		Expect(binding.Subjects[0].Kind).To(Equal("ServiceAccount"))
		Expect(binding.Subjects[0].Name).To(Equal("controller-manager"))
		Expect(binding.Subjects[0].Namespace).To(Equal("system"))
	})
})
