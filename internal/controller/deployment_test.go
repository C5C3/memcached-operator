// Package controller implements the reconciliation logic for the memcached-operator.
package controller

import (
	"strings"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

	memcachedv1alpha1 "github.com/c5c3/memcached-operator/api/v1alpha1"
)

func TestLabelsForMemcached(t *testing.T) {
	tests := []struct {
		name         string
		instanceName string
		wantLabels   map[string]string
	}{
		{
			name:         "standard name",
			instanceName: "my-cache",
			wantLabels: map[string]string{
				"app.kubernetes.io/name":       "memcached",
				"app.kubernetes.io/instance":   "my-cache",
				"app.kubernetes.io/managed-by": "memcached-operator",
			},
		},
		{
			name:         "another name",
			instanceName: "test-instance",
			wantLabels: map[string]string{
				"app.kubernetes.io/name":       "memcached",
				"app.kubernetes.io/instance":   "test-instance",
				"app.kubernetes.io/managed-by": "memcached-operator",
			},
		},
		{
			name:         "empty name",
			instanceName: "",
			wantLabels: map[string]string{
				"app.kubernetes.io/name":       "memcached",
				"app.kubernetes.io/instance":   "",
				"app.kubernetes.io/managed-by": "memcached-operator",
			},
		},
		{
			name:         "long name",
			instanceName: strings.Repeat("a", 253),
			wantLabels: map[string]string{
				"app.kubernetes.io/name":       "memcached",
				"app.kubernetes.io/instance":   strings.Repeat("a", 253),
				"app.kubernetes.io/managed-by": "memcached-operator",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := labelsForMemcached(tt.instanceName)

			if len(got) != len(tt.wantLabels) {
				t.Errorf("labelsForMemcached(%q) returned %d labels, want %d", tt.instanceName, len(got), len(tt.wantLabels))
			}

			for key, wantVal := range tt.wantLabels {
				gotVal, ok := got[key]
				if !ok {
					t.Errorf("labelsForMemcached(%q) missing label %q", tt.instanceName, key)
					continue
				}
				if gotVal != wantVal {
					t.Errorf("labelsForMemcached(%q)[%q] = %q, want %q", tt.instanceName, key, gotVal, wantVal)
				}
			}
		})
	}
}

func TestBuildMemcachedArgs(t *testing.T) {
	tests := []struct {
		name     string
		config   *memcachedv1alpha1.MemcachedConfig
		expected []string
	}{
		{
			name:   "default config",
			config: &memcachedv1alpha1.MemcachedConfig{},
			expected: []string{
				"-m", "64", "-c", "1024", "-t", "4", "-I", "1m",
			},
		},
		{
			name: "custom values",
			config: &memcachedv1alpha1.MemcachedConfig{
				MaxMemoryMB:    256,
				MaxConnections: 2048,
				Threads:        8,
				MaxItemSize:    "2m",
			},
			expected: []string{
				"-m", "256", "-c", "2048", "-t", "8", "-I", "2m",
			},
		},
		{
			name: "verbosity 0 produces no verbosity flag",
			config: &memcachedv1alpha1.MemcachedConfig{
				Verbosity: 0,
			},
			expected: []string{
				"-m", "64", "-c", "1024", "-t", "4", "-I", "1m",
			},
		},
		{
			name: "verbosity 1 produces -v flag",
			config: &memcachedv1alpha1.MemcachedConfig{
				Verbosity: 1,
			},
			expected: []string{
				"-m", "64", "-c", "1024", "-t", "4", "-I", "1m", "-v",
			},
		},
		{
			name: "verbosity 2 produces -vv flag",
			config: &memcachedv1alpha1.MemcachedConfig{
				Verbosity: 2,
			},
			expected: []string{
				"-m", "64", "-c", "1024", "-t", "4", "-I", "1m", "-vv",
			},
		},
		{
			name: "extra args appended after standard flags",
			config: &memcachedv1alpha1.MemcachedConfig{
				ExtraArgs: []string{"-o", "modern"},
			},
			expected: []string{
				"-m", "64", "-c", "1024", "-t", "4", "-I", "1m", "-o", "modern",
			},
		},
		{
			name:   "nil config uses defaults",
			config: nil,
			expected: []string{
				"-m", "64", "-c", "1024", "-t", "4", "-I", "1m",
			},
		},
		{
			name: "combined verbosity 2 and extra args",
			config: &memcachedv1alpha1.MemcachedConfig{
				Verbosity: 2,
				ExtraArgs: []string{"-o", "modern"},
			},
			expected: []string{
				"-m", "64", "-c", "1024", "-t", "4", "-I", "1m", "-vv", "-o", "modern",
			},
		},
		{
			name: "empty extra args produces no extra args",
			config: &memcachedv1alpha1.MemcachedConfig{
				ExtraArgs: []string{},
			},
			expected: []string{
				"-m", "64", "-c", "1024", "-t", "4", "-I", "1m",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := buildMemcachedArgs(tt.config)

			if len(got) != len(tt.expected) {
				t.Fatalf("buildMemcachedArgs() returned %d args, want %d\ngot:  %v\nwant: %v",
					len(got), len(tt.expected), got, tt.expected)
			}

			for i := range tt.expected {
				if got[i] != tt.expected[i] {
					t.Errorf("buildMemcachedArgs()[%d] = %q, want %q\ngot:  %v\nwant: %v",
						i, got[i], tt.expected[i], got, tt.expected)
				}
			}
		})
	}
}

// int32Ptr returns a pointer to an int32 value.
func int32Ptr(i int32) *int32 { return &i }

// stringPtr returns a pointer to a string value.
func stringPtr(s string) *string { return &s }

func TestConstructDeployment_MinimalSpec(t *testing.T) {
	mc := &memcachedv1alpha1.Memcached{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-cache",
			Namespace: "default",
		},
		Spec: memcachedv1alpha1.MemcachedSpec{},
	}
	dep := &appsv1.Deployment{}

	constructDeployment(mc, dep)

	// Replicas defaults to 1.
	if dep.Spec.Replicas == nil || *dep.Spec.Replicas != 1 {
		t.Errorf("expected 1 replica, got %v", dep.Spec.Replicas)
	}

	// Image defaults to memcached:1.6.
	containers := dep.Spec.Template.Spec.Containers
	if len(containers) != 1 {
		t.Fatalf("expected 1 container, got %d", len(containers))
	}
	if containers[0].Image != "memcached:1.6" {
		t.Errorf("expected image memcached:1.6, got %q", containers[0].Image)
	}
	if containers[0].Name != testPortName {
		t.Errorf("expected container name %q, got %q", testPortName, containers[0].Name)
	}

	// Default args.
	expectedArgs := []string{"-m", "64", "-c", "1024", "-t", "4", "-I", "1m"}
	if len(containers[0].Args) != len(expectedArgs) {
		t.Fatalf("expected %d args, got %d: %v", len(expectedArgs), len(containers[0].Args), containers[0].Args)
	}
	for i, arg := range expectedArgs {
		if containers[0].Args[i] != arg {
			t.Errorf("arg[%d] = %q, want %q", i, containers[0].Args[i], arg)
		}
	}

	// Port 11211.
	if len(containers[0].Ports) != 1 {
		t.Fatalf("expected 1 port, got %d", len(containers[0].Ports))
	}
	port := containers[0].Ports[0]
	if port.Name != testPortName || port.ContainerPort != 11211 || port.Protocol != corev1.ProtocolTCP {
		t.Errorf("unexpected port: %+v", port)
	}

	// Probes exist.
	if containers[0].LivenessProbe == nil {
		t.Error("expected liveness probe")
	}
	if containers[0].ReadinessProbe == nil {
		t.Error("expected readiness probe")
	}

	// Strategy is RollingUpdate.
	if dep.Spec.Strategy.Type != appsv1.RollingUpdateDeploymentStrategyType {
		t.Errorf("expected RollingUpdate strategy, got %q", dep.Spec.Strategy.Type)
	}

	// Labels on selector and pod template.
	expectedLabels := labelsForMemcached("my-cache")
	for k, v := range expectedLabels {
		if dep.Spec.Selector.MatchLabels[k] != v {
			t.Errorf("selector label %q = %q, want %q", k, dep.Spec.Selector.MatchLabels[k], v)
		}
		if dep.Spec.Template.Labels[k] != v {
			t.Errorf("template label %q = %q, want %q", k, dep.Spec.Template.Labels[k], v)
		}
	}

	// Deployment labels.
	for k, v := range expectedLabels {
		if dep.Labels[k] != v {
			t.Errorf("deployment label %q = %q, want %q", k, dep.Labels[k], v)
		}
	}
}

func TestConstructDeployment_CustomSpec(t *testing.T) {
	mc := &memcachedv1alpha1.Memcached{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "custom-cache",
			Namespace: "production",
		},
		Spec: memcachedv1alpha1.MemcachedSpec{
			Replicas: int32Ptr(5),
			Image:    stringPtr("memcached:1.6.29"),
			Resources: &corev1.ResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("100m"),
					corev1.ResourceMemory: resource.MustParse("128Mi"),
				},
				Limits: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("500m"),
					corev1.ResourceMemory: resource.MustParse("256Mi"),
				},
			},
			Memcached: &memcachedv1alpha1.MemcachedConfig{
				MaxMemoryMB:    256,
				MaxConnections: 2048,
				Threads:        8,
				MaxItemSize:    "2m",
			},
		},
	}
	dep := &appsv1.Deployment{}

	constructDeployment(mc, dep)

	if *dep.Spec.Replicas != 5 {
		t.Errorf("expected 5 replicas, got %d", *dep.Spec.Replicas)
	}

	container := dep.Spec.Template.Spec.Containers[0]
	if container.Image != "memcached:1.6.29" {
		t.Errorf("expected image memcached:1.6.29, got %q", container.Image)
	}

	// Custom args.
	expectedArgs := []string{"-m", "256", "-c", "2048", "-t", "8", "-I", "2m"}
	if len(container.Args) != len(expectedArgs) {
		t.Fatalf("expected %d args, got %d: %v", len(expectedArgs), len(container.Args), container.Args)
	}
	for i, arg := range expectedArgs {
		if container.Args[i] != arg {
			t.Errorf("arg[%d] = %q, want %q", i, container.Args[i], arg)
		}
	}

	// Resources.
	cpuReq := container.Resources.Requests[corev1.ResourceCPU]
	if cpuReq.String() != "100m" {
		t.Errorf("expected cpu request 100m, got %s", cpuReq.String())
	}
	memLimit := container.Resources.Limits[corev1.ResourceMemory]
	if memLimit.String() != "256Mi" {
		t.Errorf("expected memory limit 256Mi, got %s", memLimit.String())
	}
}

func TestConstructDeployment_ContainerPort(t *testing.T) {
	mc := &memcachedv1alpha1.Memcached{
		ObjectMeta: metav1.ObjectMeta{Name: "port-test", Namespace: "default"},
		Spec:       memcachedv1alpha1.MemcachedSpec{},
	}
	dep := &appsv1.Deployment{}

	constructDeployment(mc, dep)

	containers := dep.Spec.Template.Spec.Containers
	if len(containers) != 1 {
		t.Fatalf("expected 1 container, got %d", len(containers))
	}

	ports := containers[0].Ports
	if len(ports) != 1 {
		t.Fatalf("expected 1 port, got %d", len(ports))
	}

	if ports[0].Name != testPortName {
		t.Errorf("expected port name %q, got %q", testPortName, ports[0].Name)
	}
	if ports[0].ContainerPort != 11211 {
		t.Errorf("expected containerPort 11211, got %d", ports[0].ContainerPort)
	}
	if ports[0].Protocol != corev1.ProtocolTCP {
		t.Errorf("expected protocol TCP, got %q", ports[0].Protocol)
	}
}

func TestConstructDeployment_Probes(t *testing.T) {
	mc := &memcachedv1alpha1.Memcached{
		ObjectMeta: metav1.ObjectMeta{Name: "probe-test", Namespace: "default"},
		Spec:       memcachedv1alpha1.MemcachedSpec{},
	}
	dep := &appsv1.Deployment{}

	constructDeployment(mc, dep)

	container := dep.Spec.Template.Spec.Containers[0]

	// Liveness probe.
	lp := container.LivenessProbe
	if lp == nil {
		t.Fatal("expected liveness probe")
	}
	if lp.TCPSocket == nil {
		t.Fatal("expected tcpSocket liveness probe")
	}
	if lp.TCPSocket.Port != intstr.FromString(testPortName) {
		t.Errorf("liveness probe port = %v, want %q", lp.TCPSocket.Port, testPortName)
	}
	if lp.InitialDelaySeconds != 10 {
		t.Errorf("liveness initialDelaySeconds = %d, want 10", lp.InitialDelaySeconds)
	}
	if lp.PeriodSeconds != 10 {
		t.Errorf("liveness periodSeconds = %d, want 10", lp.PeriodSeconds)
	}

	// Readiness probe.
	rp := container.ReadinessProbe
	if rp == nil {
		t.Fatal("expected readiness probe")
	}
	if rp.TCPSocket == nil {
		t.Fatal("expected tcpSocket readiness probe")
	}
	if rp.TCPSocket.Port != intstr.FromString(testPortName) {
		t.Errorf("readiness probe port = %v, want %q", rp.TCPSocket.Port, testPortName)
	}
	if rp.InitialDelaySeconds != 5 {
		t.Errorf("readiness initialDelaySeconds = %d, want 5", rp.InitialDelaySeconds)
	}
	if rp.PeriodSeconds != 5 {
		t.Errorf("readiness periodSeconds = %d, want 5", rp.PeriodSeconds)
	}
}

func TestConstructDeployment_Resources(t *testing.T) {
	tests := []struct {
		name      string
		resources *corev1.ResourceRequirements
		wantEmpty bool
	}{
		{
			name:      "nil resources results in empty resources",
			resources: nil,
			wantEmpty: true,
		},
		{
			name: "resources from spec are set on container",
			resources: &corev1.ResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("100m"),
					corev1.ResourceMemory: resource.MustParse("128Mi"),
				},
				Limits: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("500m"),
					corev1.ResourceMemory: resource.MustParse("256Mi"),
				},
			},
			wantEmpty: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mc := &memcachedv1alpha1.Memcached{
				ObjectMeta: metav1.ObjectMeta{Name: "res-test", Namespace: "default"},
				Spec: memcachedv1alpha1.MemcachedSpec{
					Resources: tt.resources,
				},
			}
			dep := &appsv1.Deployment{}

			constructDeployment(mc, dep)

			container := dep.Spec.Template.Spec.Containers[0]
			if tt.wantEmpty {
				if len(container.Resources.Requests) != 0 || len(container.Resources.Limits) != 0 {
					t.Errorf("expected empty resources, got requests=%v limits=%v",
						container.Resources.Requests, container.Resources.Limits)
				}
			} else {
				cpuReq := container.Resources.Requests[corev1.ResourceCPU]
				if cpuReq.String() != "100m" {
					t.Errorf("cpu request = %s, want 100m", cpuReq.String())
				}
				memReq := container.Resources.Requests[corev1.ResourceMemory]
				if memReq.String() != "128Mi" {
					t.Errorf("memory request = %s, want 128Mi", memReq.String())
				}
				cpuLimit := container.Resources.Limits[corev1.ResourceCPU]
				if cpuLimit.String() != "500m" {
					t.Errorf("cpu limit = %s, want 500m", cpuLimit.String())
				}
				memLimit := container.Resources.Limits[corev1.ResourceMemory]
				if memLimit.String() != "256Mi" {
					t.Errorf("memory limit = %s, want 256Mi", memLimit.String())
				}
			}
		})
	}
}

func antiAffinityPresetPtr(p memcachedv1alpha1.AntiAffinityPreset) *memcachedv1alpha1.AntiAffinityPreset {
	return &p
}

// zoneSpreadConstraint returns a standard zone-aware topology spread constraint used across tests.
func zoneSpreadConstraint() corev1.TopologySpreadConstraint {
	return corev1.TopologySpreadConstraint{
		MaxSkew:           1,
		TopologyKey:       "topology.kubernetes.io/zone",
		WhenUnsatisfiable: corev1.DoNotSchedule,
	}
}

func TestBuildAntiAffinity_Soft(t *testing.T) {
	mc := &memcachedv1alpha1.Memcached{
		ObjectMeta: metav1.ObjectMeta{Name: "my-cache", Namespace: "default"},
		Spec: memcachedv1alpha1.MemcachedSpec{
			HighAvailability: &memcachedv1alpha1.HighAvailabilitySpec{
				AntiAffinityPreset: antiAffinityPresetPtr(memcachedv1alpha1.AntiAffinityPresetSoft),
			},
		},
	}

	affinity := buildAntiAffinity(mc)

	if affinity == nil {
		t.Fatal("expected non-nil Affinity for soft preset")
	}
	if affinity.PodAntiAffinity == nil {
		t.Fatal("expected non-nil PodAntiAffinity")
	}
	preferred := affinity.PodAntiAffinity.PreferredDuringSchedulingIgnoredDuringExecution
	if len(preferred) != 1 {
		t.Fatalf("expected 1 preferred term, got %d", len(preferred))
	}
	term := preferred[0]
	if term.Weight != 100 {
		t.Errorf("expected weight 100, got %d", term.Weight)
	}
	if term.PodAffinityTerm.TopologyKey != "kubernetes.io/hostname" {
		t.Errorf("expected topologyKey kubernetes.io/hostname, got %q", term.PodAffinityTerm.TopologyKey)
	}
	matchLabels := term.PodAffinityTerm.LabelSelector.MatchLabels
	if matchLabels["app.kubernetes.io/name"] != "memcached" {
		t.Errorf("expected label app.kubernetes.io/name=memcached, got %q", matchLabels["app.kubernetes.io/name"])
	}
	if matchLabels["app.kubernetes.io/instance"] != "my-cache" {
		t.Errorf("expected label app.kubernetes.io/instance=my-cache, got %q", matchLabels["app.kubernetes.io/instance"])
	}
	// Should NOT have required terms.
	if len(affinity.PodAntiAffinity.RequiredDuringSchedulingIgnoredDuringExecution) != 0 {
		t.Error("expected no required anti-affinity terms for soft preset")
	}
}

func TestBuildAntiAffinity_Hard(t *testing.T) {
	mc := &memcachedv1alpha1.Memcached{
		ObjectMeta: metav1.ObjectMeta{Name: "my-cache", Namespace: "default"},
		Spec: memcachedv1alpha1.MemcachedSpec{
			HighAvailability: &memcachedv1alpha1.HighAvailabilitySpec{
				AntiAffinityPreset: antiAffinityPresetPtr(memcachedv1alpha1.AntiAffinityPresetHard),
			},
		},
	}

	affinity := buildAntiAffinity(mc)

	if affinity == nil {
		t.Fatal("expected non-nil Affinity for hard preset")
	}
	if affinity.PodAntiAffinity == nil {
		t.Fatal("expected non-nil PodAntiAffinity")
	}
	required := affinity.PodAntiAffinity.RequiredDuringSchedulingIgnoredDuringExecution
	if len(required) != 1 {
		t.Fatalf("expected 1 required term, got %d", len(required))
	}
	term := required[0]
	if term.TopologyKey != "kubernetes.io/hostname" {
		t.Errorf("expected topologyKey kubernetes.io/hostname, got %q", term.TopologyKey)
	}
	matchLabels := term.LabelSelector.MatchLabels
	if matchLabels["app.kubernetes.io/name"] != "memcached" {
		t.Errorf("expected label app.kubernetes.io/name=memcached, got %q", matchLabels["app.kubernetes.io/name"])
	}
	if matchLabels["app.kubernetes.io/instance"] != "my-cache" {
		t.Errorf("expected label app.kubernetes.io/instance=my-cache, got %q", matchLabels["app.kubernetes.io/instance"])
	}
	// Should NOT have preferred terms.
	if len(affinity.PodAntiAffinity.PreferredDuringSchedulingIgnoredDuringExecution) != 0 {
		t.Error("expected no preferred anti-affinity terms for hard preset")
	}
}

func TestBuildAntiAffinity_ReturnsNil(t *testing.T) {
	tests := []struct {
		name string
		ha   *memcachedv1alpha1.HighAvailabilitySpec
	}{
		{name: "nil HighAvailability", ha: nil},
		{name: "nil AntiAffinityPreset", ha: &memcachedv1alpha1.HighAvailabilitySpec{}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mc := &memcachedv1alpha1.Memcached{
				ObjectMeta: metav1.ObjectMeta{Name: "my-cache", Namespace: "default"},
				Spec:       memcachedv1alpha1.MemcachedSpec{HighAvailability: tt.ha},
			}

			if affinity := buildAntiAffinity(mc); affinity != nil {
				t.Errorf("expected nil Affinity, got %+v", affinity)
			}
		})
	}
}

func TestBuildAntiAffinity_InstanceScopedLabels(t *testing.T) {
	tests := []struct {
		name     string
		crName   string
		wantInst string
	}{
		{name: "first instance", crName: "cache-alpha", wantInst: "cache-alpha"},
		{name: "second instance", crName: "cache-beta", wantInst: "cache-beta"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mc := &memcachedv1alpha1.Memcached{
				ObjectMeta: metav1.ObjectMeta{Name: tt.crName, Namespace: "default"},
				Spec: memcachedv1alpha1.MemcachedSpec{
					HighAvailability: &memcachedv1alpha1.HighAvailabilitySpec{
						AntiAffinityPreset: antiAffinityPresetPtr(memcachedv1alpha1.AntiAffinityPresetSoft),
					},
				},
			}

			affinity := buildAntiAffinity(mc)

			if affinity == nil {
				t.Fatal("expected non-nil Affinity")
			}
			preferred := affinity.PodAntiAffinity.PreferredDuringSchedulingIgnoredDuringExecution
			matchLabels := preferred[0].PodAffinityTerm.LabelSelector.MatchLabels
			if matchLabels["app.kubernetes.io/instance"] != tt.wantInst {
				t.Errorf("expected instance label %q, got %q", tt.wantInst, matchLabels["app.kubernetes.io/instance"])
			}
		})
	}
}

func TestConstructDeployment_AntiAffinity(t *testing.T) {
	tests := []struct {
		name  string
		ha    *memcachedv1alpha1.HighAvailabilitySpec
		check func(t *testing.T, affinity *corev1.Affinity)
	}{
		{
			name: "soft preset sets preferred anti-affinity",
			ha: &memcachedv1alpha1.HighAvailabilitySpec{
				AntiAffinityPreset: antiAffinityPresetPtr(memcachedv1alpha1.AntiAffinityPresetSoft),
			},
			check: func(t *testing.T, affinity *corev1.Affinity) {
				t.Helper()
				if affinity == nil || affinity.PodAntiAffinity == nil {
					t.Fatal("expected non-nil PodAntiAffinity")
				}
				if len(affinity.PodAntiAffinity.PreferredDuringSchedulingIgnoredDuringExecution) != 1 {
					t.Fatalf("expected 1 preferred term, got %d",
						len(affinity.PodAntiAffinity.PreferredDuringSchedulingIgnoredDuringExecution))
				}
			},
		},
		{
			name: "hard preset sets required anti-affinity",
			ha: &memcachedv1alpha1.HighAvailabilitySpec{
				AntiAffinityPreset: antiAffinityPresetPtr(memcachedv1alpha1.AntiAffinityPresetHard),
			},
			check: func(t *testing.T, affinity *corev1.Affinity) {
				t.Helper()
				if affinity == nil || affinity.PodAntiAffinity == nil {
					t.Fatal("expected non-nil PodAntiAffinity")
				}
				if len(affinity.PodAntiAffinity.RequiredDuringSchedulingIgnoredDuringExecution) != 1 {
					t.Fatalf("expected 1 required term, got %d",
						len(affinity.PodAntiAffinity.RequiredDuringSchedulingIgnoredDuringExecution))
				}
			},
		},
		{
			name: "nil HA produces nil affinity",
			ha:   nil,
			check: func(t *testing.T, affinity *corev1.Affinity) {
				t.Helper()
				if affinity != nil {
					t.Errorf("expected nil Affinity, got %+v", affinity)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mc := &memcachedv1alpha1.Memcached{
				ObjectMeta: metav1.ObjectMeta{Name: "aa-test", Namespace: "default"},
				Spec:       memcachedv1alpha1.MemcachedSpec{HighAvailability: tt.ha},
			}
			dep := &appsv1.Deployment{}

			constructDeployment(mc, dep)

			tt.check(t, dep.Spec.Template.Spec.Affinity)
		})
	}
}

func TestBuildTopologySpreadConstraints_SingleConstraint(t *testing.T) {
	mc := &memcachedv1alpha1.Memcached{
		ObjectMeta: metav1.ObjectMeta{Name: "my-cache", Namespace: "default"},
		Spec: memcachedv1alpha1.MemcachedSpec{
			HighAvailability: &memcachedv1alpha1.HighAvailabilitySpec{
				TopologySpreadConstraints: []corev1.TopologySpreadConstraint{
					{
						MaxSkew:           1,
						TopologyKey:       "topology.kubernetes.io/zone",
						WhenUnsatisfiable: corev1.DoNotSchedule,
						LabelSelector: &metav1.LabelSelector{
							MatchLabels: map[string]string{
								"app.kubernetes.io/name": "memcached",
							},
						},
					},
				},
			},
		},
	}

	got := buildTopologySpreadConstraints(mc)

	if len(got) != 1 {
		t.Fatalf("expected 1 constraint, got %d", len(got))
	}
	if got[0].MaxSkew != 1 {
		t.Errorf("maxSkew = %d, want 1", got[0].MaxSkew)
	}
	if got[0].TopologyKey != "topology.kubernetes.io/zone" {
		t.Errorf("topologyKey = %q, want topology.kubernetes.io/zone", got[0].TopologyKey)
	}
	if got[0].WhenUnsatisfiable != corev1.DoNotSchedule {
		t.Errorf("whenUnsatisfiable = %q, want DoNotSchedule", got[0].WhenUnsatisfiable)
	}
	if got[0].LabelSelector == nil || got[0].LabelSelector.MatchLabels["app.kubernetes.io/name"] != "memcached" {
		t.Error("expected labelSelector to be passed through")
	}
}

func TestBuildTopologySpreadConstraints_MultipleConstraints(t *testing.T) {
	mc := &memcachedv1alpha1.Memcached{
		ObjectMeta: metav1.ObjectMeta{Name: "my-cache", Namespace: "default"},
		Spec: memcachedv1alpha1.MemcachedSpec{
			HighAvailability: &memcachedv1alpha1.HighAvailabilitySpec{
				TopologySpreadConstraints: []corev1.TopologySpreadConstraint{
					zoneSpreadConstraint(),
					{
						MaxSkew:           1,
						TopologyKey:       "kubernetes.io/hostname",
						WhenUnsatisfiable: corev1.ScheduleAnyway,
					},
				},
			},
		},
	}

	got := buildTopologySpreadConstraints(mc)

	if len(got) != 2 {
		t.Fatalf("expected 2 constraints, got %d", len(got))
	}
	if got[0].TopologyKey != "topology.kubernetes.io/zone" {
		t.Errorf("first constraint topologyKey = %q, want topology.kubernetes.io/zone", got[0].TopologyKey)
	}
	if got[1].TopologyKey != "kubernetes.io/hostname" {
		t.Errorf("second constraint topologyKey = %q, want kubernetes.io/hostname", got[1].TopologyKey)
	}
}

func TestBuildTopologySpreadConstraints_ReturnsNil(t *testing.T) {
	tests := []struct {
		name string
		ha   *memcachedv1alpha1.HighAvailabilitySpec
	}{
		{
			name: "nil HighAvailability",
			ha:   nil,
		},
		{
			name: "nil constraints",
			ha:   &memcachedv1alpha1.HighAvailabilitySpec{},
		},
		{
			name: "empty slice",
			ha: &memcachedv1alpha1.HighAvailabilitySpec{
				TopologySpreadConstraints: []corev1.TopologySpreadConstraint{},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mc := &memcachedv1alpha1.Memcached{
				ObjectMeta: metav1.ObjectMeta{Name: "my-cache", Namespace: "default"},
				Spec: memcachedv1alpha1.MemcachedSpec{
					HighAvailability: tt.ha,
				},
			}

			got := buildTopologySpreadConstraints(mc)

			if got != nil {
				t.Errorf("expected nil, got %v", got)
			}
		})
	}
}

func TestConstructDeployment_TopologySpreadConstraints(t *testing.T) {
	mc := &memcachedv1alpha1.Memcached{
		ObjectMeta: metav1.ObjectMeta{Name: "tsc-test", Namespace: "default"},
		Spec: memcachedv1alpha1.MemcachedSpec{
			HighAvailability: &memcachedv1alpha1.HighAvailabilitySpec{
				TopologySpreadConstraints: []corev1.TopologySpreadConstraint{zoneSpreadConstraint()},
			},
		},
	}
	dep := &appsv1.Deployment{}

	constructDeployment(mc, dep)

	tsc := dep.Spec.Template.Spec.TopologySpreadConstraints
	if len(tsc) != 1 {
		t.Fatalf("expected 1 topology spread constraint, got %d", len(tsc))
	}
	if tsc[0].MaxSkew != 1 {
		t.Errorf("maxSkew = %d, want 1", tsc[0].MaxSkew)
	}
	if tsc[0].TopologyKey != "topology.kubernetes.io/zone" {
		t.Errorf("topologyKey = %q, want topology.kubernetes.io/zone", tsc[0].TopologyKey)
	}
}

func TestConstructDeployment_TopologySpreadConstraints_NilHA(t *testing.T) {
	mc := &memcachedv1alpha1.Memcached{
		ObjectMeta: metav1.ObjectMeta{Name: "tsc-nil-test", Namespace: "default"},
		Spec:       memcachedv1alpha1.MemcachedSpec{},
	}
	dep := &appsv1.Deployment{}

	constructDeployment(mc, dep)

	if dep.Spec.Template.Spec.TopologySpreadConstraints != nil {
		t.Errorf("expected nil TopologySpreadConstraints, got %v", dep.Spec.Template.Spec.TopologySpreadConstraints)
	}
}

func TestConstructDeployment_TopologySpreadAndAntiAffinity(t *testing.T) {
	mc := &memcachedv1alpha1.Memcached{
		ObjectMeta: metav1.ObjectMeta{Name: "both-test", Namespace: "default"},
		Spec: memcachedv1alpha1.MemcachedSpec{
			HighAvailability: &memcachedv1alpha1.HighAvailabilitySpec{
				AntiAffinityPreset:        antiAffinityPresetPtr(memcachedv1alpha1.AntiAffinityPresetSoft),
				TopologySpreadConstraints: []corev1.TopologySpreadConstraint{zoneSpreadConstraint()},
			},
		},
	}
	dep := &appsv1.Deployment{}

	constructDeployment(mc, dep)

	// Verify anti-affinity is set.
	if dep.Spec.Template.Spec.Affinity == nil || dep.Spec.Template.Spec.Affinity.PodAntiAffinity == nil {
		t.Fatal("expected Affinity with PodAntiAffinity to be set")
	}
	if len(dep.Spec.Template.Spec.Affinity.PodAntiAffinity.PreferredDuringSchedulingIgnoredDuringExecution) != 1 {
		t.Error("expected 1 preferred anti-affinity term")
	}

	// Verify topology spread constraints are set.
	tsc := dep.Spec.Template.Spec.TopologySpreadConstraints
	if len(tsc) != 1 {
		t.Fatalf("expected 1 topology spread constraint, got %d", len(tsc))
	}
	if tsc[0].TopologyKey != "topology.kubernetes.io/zone" {
		t.Errorf("topologyKey = %q, want topology.kubernetes.io/zone", tsc[0].TopologyKey)
	}
}

func TestConstructDeployment_RollingUpdateStrategy(t *testing.T) {
	mc := &memcachedv1alpha1.Memcached{
		ObjectMeta: metav1.ObjectMeta{Name: "strategy-test", Namespace: "default"},
		Spec:       memcachedv1alpha1.MemcachedSpec{},
	}
	dep := &appsv1.Deployment{}

	constructDeployment(mc, dep)

	strategy := dep.Spec.Strategy
	if strategy.Type != appsv1.RollingUpdateDeploymentStrategyType {
		t.Errorf("strategy type = %q, want RollingUpdate", strategy.Type)
	}

	if strategy.RollingUpdate == nil {
		t.Fatal("expected rollingUpdate config")
	}

	wantMaxSurge := intstr.FromInt32(1)
	if *strategy.RollingUpdate.MaxSurge != wantMaxSurge {
		t.Errorf("maxSurge = %v, want %v", *strategy.RollingUpdate.MaxSurge, wantMaxSurge)
	}

	wantMaxUnavailable := intstr.FromInt32(0)
	if *strategy.RollingUpdate.MaxUnavailable != wantMaxUnavailable {
		t.Errorf("maxUnavailable = %v, want %v", *strategy.RollingUpdate.MaxUnavailable, wantMaxUnavailable)
	}
}

func TestBuildGracefulShutdown_EnabledWithDefaults(t *testing.T) {
	mc := &memcachedv1alpha1.Memcached{
		ObjectMeta: metav1.ObjectMeta{Name: "gs-default", Namespace: "default"},
		Spec: memcachedv1alpha1.MemcachedSpec{
			HighAvailability: &memcachedv1alpha1.HighAvailabilitySpec{
				GracefulShutdown: &memcachedv1alpha1.GracefulShutdownSpec{
					Enabled:                       true,
					PreStopDelaySeconds:           10,
					TerminationGracePeriodSeconds: 30,
				},
			},
		},
	}

	lifecycle, terminationGracePeriod := buildGracefulShutdown(mc)

	if lifecycle == nil {
		t.Fatal("expected non-nil Lifecycle")
	}
	if lifecycle.PreStop == nil {
		t.Fatal("expected non-nil PreStop")
	}
	if lifecycle.PreStop.Exec == nil {
		t.Fatal("expected Exec handler on PreStop")
	}
	expectedCmd := []string{"sleep", "10"}
	if len(lifecycle.PreStop.Exec.Command) != len(expectedCmd) {
		t.Fatalf("expected command %v, got %v", expectedCmd, lifecycle.PreStop.Exec.Command)
	}
	for i, cmd := range expectedCmd {
		if lifecycle.PreStop.Exec.Command[i] != cmd {
			t.Errorf("command[%d] = %q, want %q", i, lifecycle.PreStop.Exec.Command[i], cmd)
		}
	}

	if terminationGracePeriod == nil {
		t.Fatal("expected non-nil terminationGracePeriodSeconds")
	}
	if *terminationGracePeriod != 30 {
		t.Errorf("terminationGracePeriodSeconds = %d, want 30", *terminationGracePeriod)
	}
}

func TestBuildGracefulShutdown_EnabledWithCustomValues(t *testing.T) {
	mc := &memcachedv1alpha1.Memcached{
		ObjectMeta: metav1.ObjectMeta{Name: "gs-custom", Namespace: "default"},
		Spec: memcachedv1alpha1.MemcachedSpec{
			HighAvailability: &memcachedv1alpha1.HighAvailabilitySpec{
				GracefulShutdown: &memcachedv1alpha1.GracefulShutdownSpec{
					Enabled:                       true,
					PreStopDelaySeconds:           15,
					TerminationGracePeriodSeconds: 45,
				},
			},
		},
	}

	lifecycle, terminationGracePeriod := buildGracefulShutdown(mc)

	if lifecycle == nil {
		t.Fatal("expected non-nil Lifecycle")
	}
	expectedCmd := []string{"sleep", "15"}
	for i, cmd := range expectedCmd {
		if lifecycle.PreStop.Exec.Command[i] != cmd {
			t.Errorf("command[%d] = %q, want %q", i, lifecycle.PreStop.Exec.Command[i], cmd)
		}
	}

	if terminationGracePeriod == nil {
		t.Fatal("expected non-nil terminationGracePeriodSeconds")
	}
	if *terminationGracePeriod != 45 {
		t.Errorf("terminationGracePeriodSeconds = %d, want 45", *terminationGracePeriod)
	}
}

func TestBuildGracefulShutdown_ZeroValuesUseDefaults(t *testing.T) {
	mc := &memcachedv1alpha1.Memcached{
		ObjectMeta: metav1.ObjectMeta{Name: "gs-zeros", Namespace: "default"},
		Spec: memcachedv1alpha1.MemcachedSpec{
			HighAvailability: &memcachedv1alpha1.HighAvailabilitySpec{
				GracefulShutdown: &memcachedv1alpha1.GracefulShutdownSpec{
					Enabled:                       true,
					PreStopDelaySeconds:           0,
					TerminationGracePeriodSeconds: 0,
				},
			},
		},
	}

	lifecycle, terminationGracePeriod := buildGracefulShutdown(mc)

	if lifecycle == nil {
		t.Fatal("expected non-nil Lifecycle")
	}
	expectedCmd := []string{"sleep", "10"}
	if len(lifecycle.PreStop.Exec.Command) != len(expectedCmd) {
		t.Fatalf("expected command %v, got %v", expectedCmd, lifecycle.PreStop.Exec.Command)
	}
	for i, cmd := range expectedCmd {
		if lifecycle.PreStop.Exec.Command[i] != cmd {
			t.Errorf("command[%d] = %q, want %q", i, lifecycle.PreStop.Exec.Command[i], cmd)
		}
	}

	if terminationGracePeriod == nil {
		t.Fatal("expected non-nil terminationGracePeriodSeconds")
	}
	if *terminationGracePeriod != 30 {
		t.Errorf("terminationGracePeriodSeconds = %d, want 30", *terminationGracePeriod)
	}
}

func TestBuildGracefulShutdown_Disabled(t *testing.T) {
	mc := &memcachedv1alpha1.Memcached{
		ObjectMeta: metav1.ObjectMeta{Name: "gs-disabled", Namespace: "default"},
		Spec: memcachedv1alpha1.MemcachedSpec{
			HighAvailability: &memcachedv1alpha1.HighAvailabilitySpec{
				GracefulShutdown: &memcachedv1alpha1.GracefulShutdownSpec{
					Enabled: false,
				},
			},
		},
	}

	lifecycle, terminationGracePeriod := buildGracefulShutdown(mc)

	if lifecycle != nil {
		t.Errorf("expected nil Lifecycle, got %+v", lifecycle)
	}
	if terminationGracePeriod != nil {
		t.Errorf("expected nil terminationGracePeriodSeconds, got %v", terminationGracePeriod)
	}
}

func TestBuildGracefulShutdown_NilHA(t *testing.T) {
	mc := &memcachedv1alpha1.Memcached{
		ObjectMeta: metav1.ObjectMeta{Name: "gs-nilha", Namespace: "default"},
		Spec:       memcachedv1alpha1.MemcachedSpec{},
	}

	lifecycle, terminationGracePeriod := buildGracefulShutdown(mc)

	if lifecycle != nil {
		t.Errorf("expected nil Lifecycle, got %+v", lifecycle)
	}
	if terminationGracePeriod != nil {
		t.Errorf("expected nil terminationGracePeriodSeconds, got %v", terminationGracePeriod)
	}
}

func TestConstructDeployment_GracefulShutdownEnabled(t *testing.T) {
	mc := &memcachedv1alpha1.Memcached{
		ObjectMeta: metav1.ObjectMeta{Name: "gs-dep-on", Namespace: "default"},
		Spec: memcachedv1alpha1.MemcachedSpec{
			HighAvailability: &memcachedv1alpha1.HighAvailabilitySpec{
				GracefulShutdown: &memcachedv1alpha1.GracefulShutdownSpec{
					Enabled:                       true,
					PreStopDelaySeconds:           10,
					TerminationGracePeriodSeconds: 30,
				},
			},
		},
	}
	dep := &appsv1.Deployment{}

	constructDeployment(mc, dep)

	container := dep.Spec.Template.Spec.Containers[0]
	if container.Lifecycle == nil {
		t.Fatal("expected Lifecycle on container")
	}
	if container.Lifecycle.PreStop == nil {
		t.Fatal("expected PreStop on Lifecycle")
	}
	expectedCmd := []string{"sleep", "10"}
	if len(container.Lifecycle.PreStop.Exec.Command) != len(expectedCmd) {
		t.Fatalf("expected command %v, got %v", expectedCmd, container.Lifecycle.PreStop.Exec.Command)
	}
	for i, cmd := range expectedCmd {
		if container.Lifecycle.PreStop.Exec.Command[i] != cmd {
			t.Errorf("command[%d] = %q, want %q", i, container.Lifecycle.PreStop.Exec.Command[i], cmd)
		}
	}

	tgps := dep.Spec.Template.Spec.TerminationGracePeriodSeconds
	if tgps == nil {
		t.Fatal("expected TerminationGracePeriodSeconds on pod spec")
	}
	if *tgps != 30 {
		t.Errorf("TerminationGracePeriodSeconds = %d, want 30", *tgps)
	}
}

func TestConstructDeployment_GracefulShutdownDisabled(t *testing.T) {
	mc := &memcachedv1alpha1.Memcached{
		ObjectMeta: metav1.ObjectMeta{Name: "gs-dep-off", Namespace: "default"},
		Spec:       memcachedv1alpha1.MemcachedSpec{},
	}
	dep := &appsv1.Deployment{}

	constructDeployment(mc, dep)

	container := dep.Spec.Template.Spec.Containers[0]
	if container.Lifecycle != nil {
		t.Errorf("expected nil Lifecycle, got %+v", container.Lifecycle)
	}
	if dep.Spec.Template.Spec.TerminationGracePeriodSeconds != nil {
		t.Errorf("expected nil TerminationGracePeriodSeconds, got %v", dep.Spec.Template.Spec.TerminationGracePeriodSeconds)
	}
}

func TestConstructDeployment_GracefulShutdownWithOtherHAFeatures(t *testing.T) {
	mc := &memcachedv1alpha1.Memcached{
		ObjectMeta: metav1.ObjectMeta{Name: "gs-dep-all", Namespace: "default"},
		Spec: memcachedv1alpha1.MemcachedSpec{
			HighAvailability: &memcachedv1alpha1.HighAvailabilitySpec{
				AntiAffinityPreset:        antiAffinityPresetPtr(memcachedv1alpha1.AntiAffinityPresetSoft),
				TopologySpreadConstraints: []corev1.TopologySpreadConstraint{zoneSpreadConstraint()},
				GracefulShutdown: &memcachedv1alpha1.GracefulShutdownSpec{
					Enabled:                       true,
					PreStopDelaySeconds:           10,
					TerminationGracePeriodSeconds: 30,
				},
			},
		},
	}
	dep := &appsv1.Deployment{}

	constructDeployment(mc, dep)

	// Verify anti-affinity.
	if dep.Spec.Template.Spec.Affinity == nil || dep.Spec.Template.Spec.Affinity.PodAntiAffinity == nil {
		t.Fatal("expected Affinity with PodAntiAffinity")
	}
	if len(dep.Spec.Template.Spec.Affinity.PodAntiAffinity.PreferredDuringSchedulingIgnoredDuringExecution) != 1 {
		t.Error("expected 1 preferred anti-affinity term")
	}

	// Verify topology spread constraints.
	if len(dep.Spec.Template.Spec.TopologySpreadConstraints) != 1 {
		t.Fatalf("expected 1 topology spread constraint, got %d", len(dep.Spec.Template.Spec.TopologySpreadConstraints))
	}

	// Verify graceful shutdown.
	container := dep.Spec.Template.Spec.Containers[0]
	if container.Lifecycle == nil || container.Lifecycle.PreStop == nil {
		t.Fatal("expected Lifecycle with PreStop")
	}
	expectedCmd := []string{"sleep", "10"}
	for i, cmd := range expectedCmd {
		if container.Lifecycle.PreStop.Exec.Command[i] != cmd {
			t.Errorf("command[%d] = %q, want %q", i, container.Lifecycle.PreStop.Exec.Command[i], cmd)
		}
	}
	if dep.Spec.Template.Spec.TerminationGracePeriodSeconds == nil || *dep.Spec.Template.Spec.TerminationGracePeriodSeconds != 30 {
		t.Errorf("expected TerminationGracePeriodSeconds=30, got %v", dep.Spec.Template.Spec.TerminationGracePeriodSeconds)
	}
}
