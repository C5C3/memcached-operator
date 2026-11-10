// Package controller implements the reconciliation logic for the memcached-operator.
package controller

import (
	"crypto/sha256"
	"encoding/hex"
	"reflect"
	"regexp"
	"strings"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

	memcachedv1beta1 "github.com/c5c3/memcached-operator/api/v1beta1"
)

const (
	testExporterContainer = "exporter"
	testExporterImage     = "my-registry/memcached-exporter:v1.0.0"
	testSASLSecret        = "my-sasl-secret"
	testSASLSecretName    = "sasl-secret"
	testTLSSecret         = "my-tls-secret"
	testCPU100m           = "100m"
	testMem128Mi          = "128Mi"
	testDefaultImage      = "memcached:1.6"
	testHostnameTopology  = "kubernetes.io/hostname"
	testZoneTopology      = "topology.kubernetes.io/zone"
	testPasswordFileKey   = "password-file"
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
		config   *memcachedv1beta1.MemcachedConfig
		expected []string
	}{
		{
			name:   "default config",
			config: &memcachedv1beta1.MemcachedConfig{},
			expected: []string{
				"-m", "64", "-c", "1024", "-t", "4", "-I", "1m",
			},
		},
		{
			name: "custom values",
			config: &memcachedv1beta1.MemcachedConfig{
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
			config: &memcachedv1beta1.MemcachedConfig{
				Verbosity: 0,
			},
			expected: []string{
				"-m", "64", "-c", "1024", "-t", "4", "-I", "1m",
			},
		},
		{
			name: "verbosity 1 produces -v flag",
			config: &memcachedv1beta1.MemcachedConfig{
				Verbosity: 1,
			},
			expected: []string{
				"-m", "64", "-c", "1024", "-t", "4", "-I", "1m", "-v",
			},
		},
		{
			name: "verbosity 2 produces -vv flag",
			config: &memcachedv1beta1.MemcachedConfig{
				Verbosity: 2,
			},
			expected: []string{
				"-m", "64", "-c", "1024", "-t", "4", "-I", "1m", "-vv",
			},
		},
		{
			name: "extra args appended after standard flags",
			config: &memcachedv1beta1.MemcachedConfig{
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
			config: &memcachedv1beta1.MemcachedConfig{
				Verbosity: 2,
				ExtraArgs: []string{"-o", "modern"},
			},
			expected: []string{
				"-m", "64", "-c", "1024", "-t", "4", "-I", "1m", "-vv", "-o", "modern",
			},
		},
		{
			name: "empty extra args produces no extra args",
			config: &memcachedv1beta1.MemcachedConfig{
				ExtraArgs: []string{},
			},
			expected: []string{
				"-m", "64", "-c", "1024", "-t", "4", "-I", "1m",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := buildMemcachedArgs(tt.config, nil, nil)

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

func TestConstructStatefulSet_MinimalSpec(t *testing.T) {
	mc := &memcachedv1beta1.Memcached{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-cache",
			Namespace: "default",
		},
		Spec: memcachedv1beta1.MemcachedSpec{},
	}
	sts := &appsv1.StatefulSet{}

	constructStatefulSet(mc, sts, "", "")

	// Replicas defaults to 1.
	if sts.Spec.Replicas == nil || *sts.Spec.Replicas != 1 {
		t.Errorf("expected 1 replica, got %v", sts.Spec.Replicas)
	}

	// Image defaults to memcached:1.6.
	containers := sts.Spec.Template.Spec.Containers
	if len(containers) != 1 {
		t.Fatalf("expected 1 container, got %d", len(containers))
	}
	if containers[0].Image != testDefaultImage {
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

	// Update strategy is RollingUpdate.
	if sts.Spec.UpdateStrategy.Type != appsv1.RollingUpdateStatefulSetStrategyType {
		t.Errorf("expected RollingUpdate update strategy, got %q", sts.Spec.UpdateStrategy.Type)
	}

	// Labels on selector and pod template.
	expectedLabels := labelsForMemcached("my-cache")
	for k, v := range expectedLabels {
		if sts.Spec.Selector.MatchLabels[k] != v {
			t.Errorf("selector label %q = %q, want %q", k, sts.Spec.Selector.MatchLabels[k], v)
		}
		if sts.Spec.Template.Labels[k] != v {
			t.Errorf("template label %q = %q, want %q", k, sts.Spec.Template.Labels[k], v)
		}
	}

	// StatefulSet labels.
	for k, v := range expectedLabels {
		if sts.Labels[k] != v {
			t.Errorf("StatefulSet label %q = %q, want %q", k, sts.Labels[k], v)
		}
	}
}

func TestConstructStatefulSet_CustomSpec(t *testing.T) {
	mc := &memcachedv1beta1.Memcached{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "custom-cache",
			Namespace: "production",
		},
		Spec: memcachedv1beta1.MemcachedSpec{
			Replicas: int32Ptr(5),
			Image:    stringPtr("memcached:1.6.29"),
			Resources: &corev1.ResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse(testCPU100m),
					corev1.ResourceMemory: resource.MustParse(testMem128Mi),
				},
				Limits: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("500m"),
					corev1.ResourceMemory: resource.MustParse("256Mi"),
				},
			},
			Memcached: &memcachedv1beta1.MemcachedConfig{
				MaxMemoryMB:    256,
				MaxConnections: 2048,
				Threads:        8,
				MaxItemSize:    "2m",
			},
		},
	}
	sts := &appsv1.StatefulSet{}

	constructStatefulSet(mc, sts, "", "")

	if *sts.Spec.Replicas != 5 {
		t.Errorf("expected 5 replicas, got %d", *sts.Spec.Replicas)
	}

	container := sts.Spec.Template.Spec.Containers[0]
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
	if cpuReq.String() != testCPU100m {
		t.Errorf("expected cpu request 100m, got %s", cpuReq.String())
	}
	memLimit := container.Resources.Limits[corev1.ResourceMemory]
	if memLimit.String() != "256Mi" {
		t.Errorf("expected memory limit 256Mi, got %s", memLimit.String())
	}
}

func TestConstructStatefulSet_ContainerPort(t *testing.T) {
	mc := &memcachedv1beta1.Memcached{
		ObjectMeta: metav1.ObjectMeta{Name: "port-test", Namespace: "default"},
		Spec:       memcachedv1beta1.MemcachedSpec{},
	}
	sts := &appsv1.StatefulSet{}

	constructStatefulSet(mc, sts, "", "")

	containers := sts.Spec.Template.Spec.Containers
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

func TestConstructStatefulSet_Probes(t *testing.T) {
	mc := &memcachedv1beta1.Memcached{
		ObjectMeta: metav1.ObjectMeta{Name: "probe-test", Namespace: "default"},
		Spec:       memcachedv1beta1.MemcachedSpec{},
	}
	sts := &appsv1.StatefulSet{}

	constructStatefulSet(mc, sts, "", "")

	container := sts.Spec.Template.Spec.Containers[0]

	// Liveness probe.
	lp := container.LivenessProbe
	if lp == nil {
		t.Fatal("expected liveness probe")
		return
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
		return
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

func TestConstructStatefulSet_Resources(t *testing.T) {
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
					corev1.ResourceCPU:    resource.MustParse(testCPU100m),
					corev1.ResourceMemory: resource.MustParse(testMem128Mi),
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
			mc := &memcachedv1beta1.Memcached{
				ObjectMeta: metav1.ObjectMeta{Name: "res-test", Namespace: "default"},
				Spec: memcachedv1beta1.MemcachedSpec{
					Resources: tt.resources,
				},
			}
			sts := &appsv1.StatefulSet{}

			constructStatefulSet(mc, sts, "", "")

			container := sts.Spec.Template.Spec.Containers[0]
			if tt.wantEmpty {
				if len(container.Resources.Requests) != 0 || len(container.Resources.Limits) != 0 {
					t.Errorf("expected empty resources, got requests=%v limits=%v",
						container.Resources.Requests, container.Resources.Limits)
				}
			} else {
				cpuReq := container.Resources.Requests[corev1.ResourceCPU]
				if cpuReq.String() != testCPU100m {
					t.Errorf("cpu request = %s, want 100m", cpuReq.String())
				}
				memReq := container.Resources.Requests[corev1.ResourceMemory]
				if memReq.String() != testMem128Mi {
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

func antiAffinityPresetPtr(p memcachedv1beta1.AntiAffinityPreset) *memcachedv1beta1.AntiAffinityPreset {
	return &p
}

// zoneSpreadConstraint returns a standard zone-aware topology spread constraint used across tests.
func zoneSpreadConstraint() corev1.TopologySpreadConstraint {
	return corev1.TopologySpreadConstraint{
		MaxSkew:           1,
		TopologyKey:       testZoneTopology,
		WhenUnsatisfiable: corev1.DoNotSchedule,
	}
}

func TestBuildAntiAffinity_Soft(t *testing.T) {
	mc := &memcachedv1beta1.Memcached{
		ObjectMeta: metav1.ObjectMeta{Name: "my-cache", Namespace: "default"},
		Spec: memcachedv1beta1.MemcachedSpec{
			HighAvailability: &memcachedv1beta1.HighAvailabilitySpec{
				AntiAffinityPreset: antiAffinityPresetPtr(memcachedv1beta1.AntiAffinityPresetSoft),
			},
		},
	}

	affinity := buildAntiAffinity(mc)

	if affinity == nil {
		t.Fatal("expected non-nil Affinity for soft preset")
		return
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
	if term.PodAffinityTerm.TopologyKey != testHostnameTopology {
		t.Errorf("expected topologyKey kubernetes.io/hostname, got %q", term.PodAffinityTerm.TopologyKey)
	}
	matchLabels := term.PodAffinityTerm.LabelSelector.MatchLabels
	if matchLabels["app.kubernetes.io/name"] != testPortName {
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
	mc := &memcachedv1beta1.Memcached{
		ObjectMeta: metav1.ObjectMeta{Name: "my-cache", Namespace: "default"},
		Spec: memcachedv1beta1.MemcachedSpec{
			HighAvailability: &memcachedv1beta1.HighAvailabilitySpec{
				AntiAffinityPreset: antiAffinityPresetPtr(memcachedv1beta1.AntiAffinityPresetHard),
			},
		},
	}

	affinity := buildAntiAffinity(mc)

	if affinity == nil {
		t.Fatal("expected non-nil Affinity for hard preset")
		return
	}
	if affinity.PodAntiAffinity == nil {
		t.Fatal("expected non-nil PodAntiAffinity")
	}
	required := affinity.PodAntiAffinity.RequiredDuringSchedulingIgnoredDuringExecution
	if len(required) != 1 {
		t.Fatalf("expected 1 required term, got %d", len(required))
	}
	term := required[0]
	if term.TopologyKey != testHostnameTopology {
		t.Errorf("expected topologyKey kubernetes.io/hostname, got %q", term.TopologyKey)
	}
	matchLabels := term.LabelSelector.MatchLabels
	if matchLabels["app.kubernetes.io/name"] != testPortName {
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
		ha   *memcachedv1beta1.HighAvailabilitySpec
	}{
		{name: "nil HighAvailability", ha: nil},
		{name: "nil AntiAffinityPreset", ha: &memcachedv1beta1.HighAvailabilitySpec{}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mc := &memcachedv1beta1.Memcached{
				ObjectMeta: metav1.ObjectMeta{Name: "my-cache", Namespace: "default"},
				Spec:       memcachedv1beta1.MemcachedSpec{HighAvailability: tt.ha},
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
			mc := &memcachedv1beta1.Memcached{
				ObjectMeta: metav1.ObjectMeta{Name: tt.crName, Namespace: "default"},
				Spec: memcachedv1beta1.MemcachedSpec{
					HighAvailability: &memcachedv1beta1.HighAvailabilitySpec{
						AntiAffinityPreset: antiAffinityPresetPtr(memcachedv1beta1.AntiAffinityPresetSoft),
					},
				},
			}

			affinity := buildAntiAffinity(mc)

			if affinity == nil {
				t.Fatal("expected non-nil Affinity")
				return
			}
			preferred := affinity.PodAntiAffinity.PreferredDuringSchedulingIgnoredDuringExecution
			matchLabels := preferred[0].PodAffinityTerm.LabelSelector.MatchLabels
			if matchLabels["app.kubernetes.io/instance"] != tt.wantInst {
				t.Errorf("expected instance label %q, got %q", tt.wantInst, matchLabels["app.kubernetes.io/instance"])
			}
		})
	}
}

func TestConstructStatefulSet_AntiAffinity(t *testing.T) {
	tests := []struct {
		name  string
		ha    *memcachedv1beta1.HighAvailabilitySpec
		check func(t *testing.T, affinity *corev1.Affinity)
	}{
		{
			name: "soft preset sets preferred anti-affinity",
			ha: &memcachedv1beta1.HighAvailabilitySpec{
				AntiAffinityPreset: antiAffinityPresetPtr(memcachedv1beta1.AntiAffinityPresetSoft),
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
			ha: &memcachedv1beta1.HighAvailabilitySpec{
				AntiAffinityPreset: antiAffinityPresetPtr(memcachedv1beta1.AntiAffinityPresetHard),
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
			mc := &memcachedv1beta1.Memcached{
				ObjectMeta: metav1.ObjectMeta{Name: "aa-test", Namespace: "default"},
				Spec:       memcachedv1beta1.MemcachedSpec{HighAvailability: tt.ha},
			}
			sts := &appsv1.StatefulSet{}

			constructStatefulSet(mc, sts, "", "")

			tt.check(t, sts.Spec.Template.Spec.Affinity)
		})
	}
}

func TestBuildTopologySpreadConstraints_SingleConstraint(t *testing.T) {
	mc := &memcachedv1beta1.Memcached{
		ObjectMeta: metav1.ObjectMeta{Name: "my-cache", Namespace: "default"},
		Spec: memcachedv1beta1.MemcachedSpec{
			HighAvailability: &memcachedv1beta1.HighAvailabilitySpec{
				TopologySpreadConstraints: []corev1.TopologySpreadConstraint{
					{
						MaxSkew:           1,
						TopologyKey:       testZoneTopology,
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
	if got[0].TopologyKey != testZoneTopology {
		t.Errorf("topologyKey = %q, want topology.kubernetes.io/zone", got[0].TopologyKey)
	}
	if got[0].WhenUnsatisfiable != corev1.DoNotSchedule {
		t.Errorf("whenUnsatisfiable = %q, want DoNotSchedule", got[0].WhenUnsatisfiable)
	}
	if got[0].LabelSelector == nil || got[0].LabelSelector.MatchLabels["app.kubernetes.io/name"] != testPortName {
		t.Error("expected labelSelector to be passed through")
	}
}

func TestBuildTopologySpreadConstraints_MultipleConstraints(t *testing.T) {
	mc := &memcachedv1beta1.Memcached{
		ObjectMeta: metav1.ObjectMeta{Name: "my-cache", Namespace: "default"},
		Spec: memcachedv1beta1.MemcachedSpec{
			HighAvailability: &memcachedv1beta1.HighAvailabilitySpec{
				TopologySpreadConstraints: []corev1.TopologySpreadConstraint{
					zoneSpreadConstraint(),
					{
						MaxSkew:           1,
						TopologyKey:       testHostnameTopology,
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
	if got[0].TopologyKey != testZoneTopology {
		t.Errorf("first constraint topologyKey = %q, want topology.kubernetes.io/zone", got[0].TopologyKey)
	}
	if got[1].TopologyKey != testHostnameTopology {
		t.Errorf("second constraint topologyKey = %q, want kubernetes.io/hostname", got[1].TopologyKey)
	}
}

func TestBuildTopologySpreadConstraints_ReturnsNil(t *testing.T) {
	tests := []struct {
		name string
		ha   *memcachedv1beta1.HighAvailabilitySpec
	}{
		{
			name: "nil HighAvailability",
			ha:   nil,
		},
		{
			name: "nil constraints",
			ha:   &memcachedv1beta1.HighAvailabilitySpec{},
		},
		{
			name: "empty slice",
			ha: &memcachedv1beta1.HighAvailabilitySpec{
				TopologySpreadConstraints: []corev1.TopologySpreadConstraint{},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mc := &memcachedv1beta1.Memcached{
				ObjectMeta: metav1.ObjectMeta{Name: "my-cache", Namespace: "default"},
				Spec: memcachedv1beta1.MemcachedSpec{
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

func TestConstructStatefulSet_TopologySpreadConstraints(t *testing.T) {
	mc := &memcachedv1beta1.Memcached{
		ObjectMeta: metav1.ObjectMeta{Name: "tsc-test", Namespace: "default"},
		Spec: memcachedv1beta1.MemcachedSpec{
			HighAvailability: &memcachedv1beta1.HighAvailabilitySpec{
				TopologySpreadConstraints: []corev1.TopologySpreadConstraint{zoneSpreadConstraint()},
			},
		},
	}
	sts := &appsv1.StatefulSet{}

	constructStatefulSet(mc, sts, "", "")

	tsc := sts.Spec.Template.Spec.TopologySpreadConstraints
	if len(tsc) != 1 {
		t.Fatalf("expected 1 topology spread constraint, got %d", len(tsc))
	}
	if tsc[0].MaxSkew != 1 {
		t.Errorf("maxSkew = %d, want 1", tsc[0].MaxSkew)
	}
	if tsc[0].TopologyKey != testZoneTopology {
		t.Errorf("topologyKey = %q, want topology.kubernetes.io/zone", tsc[0].TopologyKey)
	}
}

func TestConstructStatefulSet_TopologySpreadConstraints_NilHA(t *testing.T) {
	mc := &memcachedv1beta1.Memcached{
		ObjectMeta: metav1.ObjectMeta{Name: "tsc-nil-test", Namespace: "default"},
		Spec:       memcachedv1beta1.MemcachedSpec{},
	}
	sts := &appsv1.StatefulSet{}

	constructStatefulSet(mc, sts, "", "")

	if sts.Spec.Template.Spec.TopologySpreadConstraints != nil {
		t.Errorf("expected nil TopologySpreadConstraints, got %v", sts.Spec.Template.Spec.TopologySpreadConstraints)
	}
}

func TestConstructStatefulSet_TopologySpreadAndAntiAffinity(t *testing.T) {
	mc := &memcachedv1beta1.Memcached{
		ObjectMeta: metav1.ObjectMeta{Name: "both-test", Namespace: "default"},
		Spec: memcachedv1beta1.MemcachedSpec{
			HighAvailability: &memcachedv1beta1.HighAvailabilitySpec{
				AntiAffinityPreset:        antiAffinityPresetPtr(memcachedv1beta1.AntiAffinityPresetSoft),
				TopologySpreadConstraints: []corev1.TopologySpreadConstraint{zoneSpreadConstraint()},
			},
		},
	}
	sts := &appsv1.StatefulSet{}

	constructStatefulSet(mc, sts, "", "")

	// Verify anti-affinity is set.
	if sts.Spec.Template.Spec.Affinity == nil || sts.Spec.Template.Spec.Affinity.PodAntiAffinity == nil {
		t.Fatal("expected Affinity with PodAntiAffinity to be set")
	}
	if len(sts.Spec.Template.Spec.Affinity.PodAntiAffinity.PreferredDuringSchedulingIgnoredDuringExecution) != 1 {
		t.Error("expected 1 preferred anti-affinity term")
	}

	// Verify topology spread constraints are set.
	tsc := sts.Spec.Template.Spec.TopologySpreadConstraints
	if len(tsc) != 1 {
		t.Fatalf("expected 1 topology spread constraint, got %d", len(tsc))
	}
	if tsc[0].TopologyKey != testZoneTopology {
		t.Errorf("topologyKey = %q, want topology.kubernetes.io/zone", tsc[0].TopologyKey)
	}
}

func TestConstructStatefulSet_RollingUpdateStrategy(t *testing.T) {
	mc := &memcachedv1beta1.Memcached{
		ObjectMeta: metav1.ObjectMeta{Name: "strategy-test", Namespace: "default"},
		Spec:       memcachedv1beta1.MemcachedSpec{},
	}
	sts := &appsv1.StatefulSet{}

	constructStatefulSet(mc, sts, "", "")

	strategy := sts.Spec.UpdateStrategy
	if strategy.Type != appsv1.RollingUpdateStatefulSetStrategyType {
		t.Errorf("updateStrategy type = %q, want RollingUpdate", strategy.Type)
	}

	// A StatefulSet replaces one pod at a time; the operator sets no partition or maxUnavailable.
	if strategy.RollingUpdate != nil {
		t.Errorf("expected no rollingUpdate parameters, got %+v", *strategy.RollingUpdate)
	}

	if sts.Spec.PodManagementPolicy != appsv1.ParallelPodManagement {
		t.Errorf("podManagementPolicy = %q, want %q", sts.Spec.PodManagementPolicy, appsv1.ParallelPodManagement)
	}

	if sts.Spec.ServiceName != mc.Name {
		t.Errorf("serviceName = %q, want %q", sts.Spec.ServiceName, mc.Name)
	}

	if len(sts.Spec.VolumeClaimTemplates) != 0 {
		t.Errorf("expected no volumeClaimTemplates, got %d", len(sts.Spec.VolumeClaimTemplates))
	}
}

func TestBuildGracefulShutdown_EnabledWithDefaults(t *testing.T) {
	mc := &memcachedv1beta1.Memcached{
		ObjectMeta: metav1.ObjectMeta{Name: "gs-default", Namespace: "default"},
		Spec: memcachedv1beta1.MemcachedSpec{
			HighAvailability: &memcachedv1beta1.HighAvailabilitySpec{
				GracefulShutdown: &memcachedv1beta1.GracefulShutdownSpec{
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
		return
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
		return
	}
	if *terminationGracePeriod != 30 {
		t.Errorf("terminationGracePeriodSeconds = %d, want 30", *terminationGracePeriod)
	}
}

func TestBuildGracefulShutdown_EnabledWithCustomValues(t *testing.T) {
	mc := &memcachedv1beta1.Memcached{
		ObjectMeta: metav1.ObjectMeta{Name: "gs-custom", Namespace: "default"},
		Spec: memcachedv1beta1.MemcachedSpec{
			HighAvailability: &memcachedv1beta1.HighAvailabilitySpec{
				GracefulShutdown: &memcachedv1beta1.GracefulShutdownSpec{
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
		return
	}
	expectedCmd := []string{"sleep", "15"}
	for i, cmd := range expectedCmd {
		if lifecycle.PreStop.Exec.Command[i] != cmd {
			t.Errorf("command[%d] = %q, want %q", i, lifecycle.PreStop.Exec.Command[i], cmd)
		}
	}

	if terminationGracePeriod == nil {
		t.Fatal("expected non-nil terminationGracePeriodSeconds")
		return
	}
	if *terminationGracePeriod != 45 {
		t.Errorf("terminationGracePeriodSeconds = %d, want 45", *terminationGracePeriod)
	}
}

func TestBuildGracefulShutdown_ZeroValuesUseDefaults(t *testing.T) {
	mc := &memcachedv1beta1.Memcached{
		ObjectMeta: metav1.ObjectMeta{Name: "gs-zeros", Namespace: "default"},
		Spec: memcachedv1beta1.MemcachedSpec{
			HighAvailability: &memcachedv1beta1.HighAvailabilitySpec{
				GracefulShutdown: &memcachedv1beta1.GracefulShutdownSpec{
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
		return
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
		return
	}
	if *terminationGracePeriod != 30 {
		t.Errorf("terminationGracePeriodSeconds = %d, want 30", *terminationGracePeriod)
	}
}

func TestBuildGracefulShutdown_Disabled(t *testing.T) {
	mc := &memcachedv1beta1.Memcached{
		ObjectMeta: metav1.ObjectMeta{Name: "gs-disabled", Namespace: "default"},
		Spec: memcachedv1beta1.MemcachedSpec{
			HighAvailability: &memcachedv1beta1.HighAvailabilitySpec{
				GracefulShutdown: &memcachedv1beta1.GracefulShutdownSpec{
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
	mc := &memcachedv1beta1.Memcached{
		ObjectMeta: metav1.ObjectMeta{Name: "gs-nilha", Namespace: "default"},
		Spec:       memcachedv1beta1.MemcachedSpec{},
	}

	lifecycle, terminationGracePeriod := buildGracefulShutdown(mc)

	if lifecycle != nil {
		t.Errorf("expected nil Lifecycle, got %+v", lifecycle)
	}
	if terminationGracePeriod != nil {
		t.Errorf("expected nil terminationGracePeriodSeconds, got %v", terminationGracePeriod)
	}
}

func TestConstructStatefulSet_GracefulShutdownEnabled(t *testing.T) {
	mc := &memcachedv1beta1.Memcached{
		ObjectMeta: metav1.ObjectMeta{Name: "gs-dep-on", Namespace: "default"},
		Spec: memcachedv1beta1.MemcachedSpec{
			HighAvailability: &memcachedv1beta1.HighAvailabilitySpec{
				GracefulShutdown: &memcachedv1beta1.GracefulShutdownSpec{
					Enabled:                       true,
					PreStopDelaySeconds:           10,
					TerminationGracePeriodSeconds: 30,
				},
			},
		},
	}
	sts := &appsv1.StatefulSet{}

	constructStatefulSet(mc, sts, "", "")

	container := sts.Spec.Template.Spec.Containers[0]
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

	tgps := sts.Spec.Template.Spec.TerminationGracePeriodSeconds
	if tgps == nil {
		t.Fatal("expected TerminationGracePeriodSeconds on pod spec")
		return
	}
	if *tgps != 30 {
		t.Errorf("TerminationGracePeriodSeconds = %d, want 30", *tgps)
	}
}

func TestConstructStatefulSet_GracefulShutdownDisabled(t *testing.T) {
	mc := &memcachedv1beta1.Memcached{
		ObjectMeta: metav1.ObjectMeta{Name: "gs-dep-off", Namespace: "default"},
		Spec:       memcachedv1beta1.MemcachedSpec{},
	}
	sts := &appsv1.StatefulSet{}

	constructStatefulSet(mc, sts, "", "")

	container := sts.Spec.Template.Spec.Containers[0]
	if container.Lifecycle != nil {
		t.Errorf("expected nil Lifecycle, got %+v", container.Lifecycle)
	}
	if sts.Spec.Template.Spec.TerminationGracePeriodSeconds != nil {
		t.Errorf("expected nil TerminationGracePeriodSeconds, got %v", sts.Spec.Template.Spec.TerminationGracePeriodSeconds)
	}
}

func TestConstructStatefulSet_GracefulShutdownWithOtherHAFeatures(t *testing.T) {
	mc := &memcachedv1beta1.Memcached{
		ObjectMeta: metav1.ObjectMeta{Name: "gs-dep-all", Namespace: "default"},
		Spec: memcachedv1beta1.MemcachedSpec{
			HighAvailability: &memcachedv1beta1.HighAvailabilitySpec{
				AntiAffinityPreset:        antiAffinityPresetPtr(memcachedv1beta1.AntiAffinityPresetSoft),
				TopologySpreadConstraints: []corev1.TopologySpreadConstraint{zoneSpreadConstraint()},
				GracefulShutdown: &memcachedv1beta1.GracefulShutdownSpec{
					Enabled:                       true,
					PreStopDelaySeconds:           10,
					TerminationGracePeriodSeconds: 30,
				},
			},
		},
	}
	sts := &appsv1.StatefulSet{}

	constructStatefulSet(mc, sts, "", "")

	// Verify anti-affinity.
	if sts.Spec.Template.Spec.Affinity == nil || sts.Spec.Template.Spec.Affinity.PodAntiAffinity == nil {
		t.Fatal("expected Affinity with PodAntiAffinity")
	}
	if len(sts.Spec.Template.Spec.Affinity.PodAntiAffinity.PreferredDuringSchedulingIgnoredDuringExecution) != 1 {
		t.Error("expected 1 preferred anti-affinity term")
	}

	// Verify topology spread constraints.
	if len(sts.Spec.Template.Spec.TopologySpreadConstraints) != 1 {
		t.Fatalf("expected 1 topology spread constraint, got %d", len(sts.Spec.Template.Spec.TopologySpreadConstraints))
	}

	// Verify graceful shutdown.
	container := sts.Spec.Template.Spec.Containers[0]
	if container.Lifecycle == nil || container.Lifecycle.PreStop == nil {
		t.Fatal("expected Lifecycle with PreStop")
	}
	expectedCmd := []string{"sleep", "10"}
	for i, cmd := range expectedCmd {
		if container.Lifecycle.PreStop.Exec.Command[i] != cmd {
			t.Errorf("command[%d] = %q, want %q", i, container.Lifecycle.PreStop.Exec.Command[i], cmd)
		}
	}
	if sts.Spec.Template.Spec.TerminationGracePeriodSeconds == nil || *sts.Spec.Template.Spec.TerminationGracePeriodSeconds != 30 {
		t.Errorf("expected TerminationGracePeriodSeconds=30, got %v", sts.Spec.Template.Spec.TerminationGracePeriodSeconds)
	}
}

func TestBuildExporterContainer_Enabled(t *testing.T) {
	mc := &memcachedv1beta1.Memcached{
		ObjectMeta: metav1.ObjectMeta{Name: "exp-test", Namespace: "default"},
		Spec: memcachedv1beta1.MemcachedSpec{
			Monitoring: &memcachedv1beta1.MonitoringSpec{
				Enabled: true,
			},
		},
	}

	container := buildExporterContainer(mc)

	if container == nil {
		t.Fatal("expected non-nil container")
		return
	}
	if container.Name != testExporterContainer {
		t.Errorf("expected container name 'exporter', got %q", container.Name)
	}
	if container.Image != "prom/memcached-exporter:v0.15.4" {
		t.Errorf("expected default image 'prom/memcached-exporter:v0.15.4', got %q", container.Image)
	}
	if len(container.Ports) != 1 {
		t.Fatalf("expected 1 port, got %d", len(container.Ports))
	}
	port := container.Ports[0]
	if port.Name != testMetricsPort || port.ContainerPort != 9150 || port.Protocol != corev1.ProtocolTCP {
		t.Errorf("unexpected port: %+v", port)
	}
}

func TestBuildExporterContainer_ReturnsNil(t *testing.T) {
	tests := []struct {
		name       string
		monitoring *memcachedv1beta1.MonitoringSpec
	}{
		{name: "monitoring disabled", monitoring: &memcachedv1beta1.MonitoringSpec{Enabled: false}},
		{name: "nil monitoring", monitoring: nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mc := &memcachedv1beta1.Memcached{
				ObjectMeta: metav1.ObjectMeta{Name: "exp-nil", Namespace: "default"},
				Spec:       memcachedv1beta1.MemcachedSpec{Monitoring: tt.monitoring},
			}

			if container := buildExporterContainer(mc); container != nil {
				t.Errorf("expected nil container, got %+v", container)
			}
		})
	}
}

func TestBuildExporterContainer_CustomImage(t *testing.T) {
	customImage := testExporterImage
	mc := &memcachedv1beta1.Memcached{
		ObjectMeta: metav1.ObjectMeta{Name: "exp-custom", Namespace: "default"},
		Spec: memcachedv1beta1.MemcachedSpec{
			Monitoring: &memcachedv1beta1.MonitoringSpec{
				Enabled:       true,
				ExporterImage: &customImage,
			},
		},
	}

	container := buildExporterContainer(mc)

	if container == nil {
		t.Fatal("expected non-nil container")
		return
	}
	if container.Image != customImage {
		t.Errorf("expected custom image %q, got %q", customImage, container.Image)
	}
}

func TestBuildExporterContainer_WithResources(t *testing.T) {
	mc := &memcachedv1beta1.Memcached{
		ObjectMeta: metav1.ObjectMeta{Name: "exp-res", Namespace: "default"},
		Spec: memcachedv1beta1.MemcachedSpec{
			Monitoring: &memcachedv1beta1.MonitoringSpec{
				Enabled: true,
				ExporterResources: &corev1.ResourceRequirements{
					Requests: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("50m"),
						corev1.ResourceMemory: resource.MustParse("64Mi"),
					},
					Limits: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse(testCPU100m),
						corev1.ResourceMemory: resource.MustParse(testMem128Mi),
					},
				},
			},
		},
	}

	container := buildExporterContainer(mc)

	if container == nil {
		t.Fatal("expected non-nil container")
		return
	}
	cpuReq := container.Resources.Requests[corev1.ResourceCPU]
	if cpuReq.String() != "50m" {
		t.Errorf("cpu request = %s, want 50m", cpuReq.String())
	}
	memReq := container.Resources.Requests[corev1.ResourceMemory]
	if memReq.String() != "64Mi" {
		t.Errorf("memory request = %s, want 64Mi", memReq.String())
	}
	cpuLimit := container.Resources.Limits[corev1.ResourceCPU]
	if cpuLimit.String() != testCPU100m {
		t.Errorf("cpu limit = %s, want 100m", cpuLimit.String())
	}
	memLimit := container.Resources.Limits[corev1.ResourceMemory]
	if memLimit.String() != testMem128Mi {
		t.Errorf("memory limit = %s, want 128Mi", memLimit.String())
	}
}

func TestBuildExporterContainer_NilResources(t *testing.T) {
	mc := &memcachedv1beta1.Memcached{
		ObjectMeta: metav1.ObjectMeta{Name: "exp-nilres", Namespace: "default"},
		Spec: memcachedv1beta1.MemcachedSpec{
			Monitoring: &memcachedv1beta1.MonitoringSpec{
				Enabled: true,
			},
		},
	}

	container := buildExporterContainer(mc)

	if container == nil {
		t.Fatal("expected non-nil container")
		return
	}
	if len(container.Resources.Requests) != 0 || len(container.Resources.Limits) != 0 {
		t.Errorf("expected empty resources, got requests=%v limits=%v",
			container.Resources.Requests, container.Resources.Limits)
	}
}

func TestConstructStatefulSet_MonitoringEnabled(t *testing.T) {
	mc := &memcachedv1beta1.Memcached{
		ObjectMeta: metav1.ObjectMeta{Name: "mon-on", Namespace: "default"},
		Spec: memcachedv1beta1.MemcachedSpec{
			Monitoring: &memcachedv1beta1.MonitoringSpec{
				Enabled: true,
			},
		},
	}
	sts := &appsv1.StatefulSet{}

	constructStatefulSet(mc, sts, "", "")

	containers := sts.Spec.Template.Spec.Containers
	if len(containers) != 2 {
		t.Fatalf("expected 2 containers, got %d", len(containers))
	}
	if containers[0].Name != testPortName {
		t.Errorf("first container name = %q, want 'memcached'", containers[0].Name)
	}
	if containers[1].Name != testExporterContainer {
		t.Errorf("second container name = %q, want 'exporter'", containers[1].Name)
	}
}

func TestConstructStatefulSet_MonitoringDisabled(t *testing.T) {
	mc := &memcachedv1beta1.Memcached{
		ObjectMeta: metav1.ObjectMeta{Name: "mon-off", Namespace: "default"},
		Spec:       memcachedv1beta1.MemcachedSpec{},
	}
	sts := &appsv1.StatefulSet{}

	constructStatefulSet(mc, sts, "", "")

	containers := sts.Spec.Template.Spec.Containers
	if len(containers) != 1 {
		t.Fatalf("expected 1 container, got %d", len(containers))
	}
	if containers[0].Name != testPortName {
		t.Errorf("container name = %q, want 'memcached'", containers[0].Name)
	}
}

// --- Security Context Tests ---

func TestBuildPodSecurityContext_WithValue(t *testing.T) {
	runAsNonRoot := true
	fsGroup := int64(1000)
	mc := &memcachedv1beta1.Memcached{
		Spec: memcachedv1beta1.MemcachedSpec{
			Security: &memcachedv1beta1.SecuritySpec{
				PodSecurityContext: &corev1.PodSecurityContext{
					RunAsNonRoot: &runAsNonRoot,
					FSGroup:      &fsGroup,
				},
			},
		},
	}

	got := buildPodSecurityContext(mc)

	if got == nil {
		t.Fatal("expected non-nil PodSecurityContext")
		return
	}
	if got.RunAsNonRoot == nil || !*got.RunAsNonRoot {
		t.Error("expected RunAsNonRoot=true")
	}
	if got.FSGroup == nil || *got.FSGroup != 1000 {
		t.Errorf("expected FSGroup=1000, got %v", got.FSGroup)
	}
}

func TestBuildPodSecurityContext_ReturnsNil(t *testing.T) {
	tests := []struct {
		name     string
		security *memcachedv1beta1.SecuritySpec
	}{
		{name: "nil Security", security: nil},
		{name: "nil PodSecurityContext", security: &memcachedv1beta1.SecuritySpec{}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mc := &memcachedv1beta1.Memcached{
				Spec: memcachedv1beta1.MemcachedSpec{Security: tt.security},
			}

			if got := buildPodSecurityContext(mc); got != nil {
				t.Errorf("expected nil PodSecurityContext, got %+v", got)
			}
		})
	}
}

func TestBuildContainerSecurityContext_WithValue(t *testing.T) {
	runAsUser := int64(1000)
	readOnly := true
	mc := &memcachedv1beta1.Memcached{
		Spec: memcachedv1beta1.MemcachedSpec{
			Security: &memcachedv1beta1.SecuritySpec{
				ContainerSecurityContext: &corev1.SecurityContext{
					RunAsUser:              &runAsUser,
					ReadOnlyRootFilesystem: &readOnly,
				},
			},
		},
	}

	got := buildContainerSecurityContext(mc)

	if got == nil {
		t.Fatal("expected non-nil SecurityContext")
		return
	}
	if got.RunAsUser == nil || *got.RunAsUser != 1000 {
		t.Errorf("expected RunAsUser=1000, got %v", got.RunAsUser)
	}
	if got.ReadOnlyRootFilesystem == nil || !*got.ReadOnlyRootFilesystem {
		t.Error("expected ReadOnlyRootFilesystem=true")
	}
}

func TestBuildContainerSecurityContext_ReturnsNil(t *testing.T) {
	tests := []struct {
		name     string
		security *memcachedv1beta1.SecuritySpec
	}{
		{name: "nil Security", security: nil},
		{name: "nil ContainerSecurityContext", security: &memcachedv1beta1.SecuritySpec{}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mc := &memcachedv1beta1.Memcached{
				Spec: memcachedv1beta1.MemcachedSpec{Security: tt.security},
			}

			if got := buildContainerSecurityContext(mc); got != nil {
				t.Errorf("expected nil SecurityContext, got %+v", got)
			}
		})
	}
}

func TestConstructStatefulSet_SecurityContexts(t *testing.T) {
	runAsNonRoot := true
	fsGroup := int64(1000)
	runAsUser := int64(1000)
	readOnly := true
	mc := &memcachedv1beta1.Memcached{
		ObjectMeta: metav1.ObjectMeta{Name: "sec-test", Namespace: "default"},
		Spec: memcachedv1beta1.MemcachedSpec{
			Security: &memcachedv1beta1.SecuritySpec{
				PodSecurityContext: &corev1.PodSecurityContext{
					RunAsNonRoot: &runAsNonRoot,
					FSGroup:      &fsGroup,
				},
				ContainerSecurityContext: &corev1.SecurityContext{
					RunAsUser:              &runAsUser,
					ReadOnlyRootFilesystem: &readOnly,
				},
			},
		},
	}
	sts := &appsv1.StatefulSet{}

	constructStatefulSet(mc, sts, "", "")

	// Pod security context.
	podSC := sts.Spec.Template.Spec.SecurityContext
	if podSC == nil {
		t.Fatal("expected non-nil pod SecurityContext")
		return
	}
	if podSC.RunAsNonRoot == nil || !*podSC.RunAsNonRoot {
		t.Error("expected pod RunAsNonRoot=true")
	}
	if podSC.FSGroup == nil || *podSC.FSGroup != 1000 {
		t.Errorf("expected pod FSGroup=1000, got %v", podSC.FSGroup)
	}

	// Container security context on memcached container.
	containerSC := sts.Spec.Template.Spec.Containers[0].SecurityContext
	if containerSC == nil {
		t.Fatal("expected non-nil container SecurityContext")
		return
	}
	if containerSC.RunAsUser == nil || *containerSC.RunAsUser != 1000 {
		t.Errorf("expected container RunAsUser=1000, got %v", containerSC.RunAsUser)
	}
	if containerSC.ReadOnlyRootFilesystem == nil || !*containerSC.ReadOnlyRootFilesystem {
		t.Error("expected container ReadOnlyRootFilesystem=true")
	}
}

func TestConstructStatefulSet_SecurityContextsNil(t *testing.T) {
	mc := &memcachedv1beta1.Memcached{
		ObjectMeta: metav1.ObjectMeta{Name: "sec-nil", Namespace: "default"},
		Spec:       memcachedv1beta1.MemcachedSpec{},
	}
	sts := &appsv1.StatefulSet{}

	constructStatefulSet(mc, sts, "", "")

	if sts.Spec.Template.Spec.SecurityContext != nil {
		t.Errorf("expected nil pod SecurityContext, got %+v", sts.Spec.Template.Spec.SecurityContext)
	}
	if sts.Spec.Template.Spec.Containers[0].SecurityContext != nil {
		t.Errorf("expected nil container SecurityContext, got %+v", sts.Spec.Template.Spec.Containers[0].SecurityContext)
	}
}

func TestConstructStatefulSet_SecurityContextsOnExporterSidecar(t *testing.T) {
	runAsUser := int64(1000)
	readOnly := true
	mc := &memcachedv1beta1.Memcached{
		ObjectMeta: metav1.ObjectMeta{Name: "sec-exp", Namespace: "default"},
		Spec: memcachedv1beta1.MemcachedSpec{
			Security: &memcachedv1beta1.SecuritySpec{
				ContainerSecurityContext: &corev1.SecurityContext{
					RunAsUser:              &runAsUser,
					ReadOnlyRootFilesystem: &readOnly,
				},
			},
			Monitoring: &memcachedv1beta1.MonitoringSpec{
				Enabled: true,
			},
		},
	}
	sts := &appsv1.StatefulSet{}

	constructStatefulSet(mc, sts, "", "")

	if len(sts.Spec.Template.Spec.Containers) != 2 {
		t.Fatalf("expected 2 containers, got %d", len(sts.Spec.Template.Spec.Containers))
	}

	// Memcached container has security context.
	mcSC := sts.Spec.Template.Spec.Containers[0].SecurityContext
	if mcSC == nil {
		t.Fatal("expected non-nil SecurityContext on memcached container")
		return
	}
	if mcSC.RunAsUser == nil || *mcSC.RunAsUser != 1000 {
		t.Errorf("memcached container RunAsUser = %v, want 1000", mcSC.RunAsUser)
	}

	// Exporter container has the same security context.
	expSC := sts.Spec.Template.Spec.Containers[1].SecurityContext
	if expSC == nil {
		t.Fatal("expected non-nil SecurityContext on exporter container")
		return
	}
	if expSC.RunAsUser == nil || *expSC.RunAsUser != 1000 {
		t.Errorf("exporter container RunAsUser = %v, want 1000", expSC.RunAsUser)
	}
	if expSC.ReadOnlyRootFilesystem == nil || !*expSC.ReadOnlyRootFilesystem {
		t.Error("expected exporter ReadOnlyRootFilesystem=true")
	}
}

// --- SASL Authentication Tests ---

func TestBuildSASLVolume_Enabled(t *testing.T) {
	mc := &memcachedv1beta1.Memcached{
		ObjectMeta: metav1.ObjectMeta{Name: "sasl-vol", Namespace: "default"},
		Spec: memcachedv1beta1.MemcachedSpec{
			Security: &memcachedv1beta1.SecuritySpec{
				SASL: &memcachedv1beta1.SASLSpec{
					Enabled: true,
					CredentialsSecretRef: corev1.LocalObjectReference{
						Name: testSASLSecret,
					},
				},
			},
		},
	}

	vol := buildSASLVolume(mc)

	if vol == nil {
		t.Fatal("expected non-nil Volume")
		return
	}
	if vol.Name != saslVolumeName {
		t.Errorf("volume name = %q, want %q", vol.Name, saslVolumeName)
	}
	if vol.Secret == nil {
		t.Fatal("expected Secret volume source")
	}
	if vol.Secret.SecretName != testSASLSecret {
		t.Errorf("secretName = %q, want %q", vol.Secret.SecretName, testSASLSecret)
	}
	if len(vol.Secret.Items) != 1 {
		t.Fatalf("expected 1 Items entry, got %d", len(vol.Secret.Items))
	}
	if vol.Secret.Items[0].Key != testPasswordFileKey {
		t.Errorf("Items[0].Key = %q, want %q", vol.Secret.Items[0].Key, testPasswordFileKey)
	}
	if vol.Secret.Items[0].Path != testPasswordFileKey {
		t.Errorf("Items[0].Path = %q, want %q", vol.Secret.Items[0].Path, testPasswordFileKey)
	}
}

func TestBuildSASLVolume_ReturnsNil(t *testing.T) {
	tests := []struct {
		name     string
		security *memcachedv1beta1.SecuritySpec
	}{
		{name: "nil Security", security: nil},
		{name: "nil SASL", security: &memcachedv1beta1.SecuritySpec{}},
		{
			name: "SASL disabled",
			security: &memcachedv1beta1.SecuritySpec{
				SASL: &memcachedv1beta1.SASLSpec{Enabled: false},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mc := &memcachedv1beta1.Memcached{
				Spec: memcachedv1beta1.MemcachedSpec{Security: tt.security},
			}

			if vol := buildSASLVolume(mc); vol != nil {
				t.Errorf("expected nil Volume, got %+v", vol)
			}
		})
	}
}

func TestBuildSASLVolumeMount_Enabled(t *testing.T) {
	mc := &memcachedv1beta1.Memcached{
		ObjectMeta: metav1.ObjectMeta{Name: "sasl-mount", Namespace: "default"},
		Spec: memcachedv1beta1.MemcachedSpec{
			Security: &memcachedv1beta1.SecuritySpec{
				SASL: &memcachedv1beta1.SASLSpec{
					Enabled: true,
					CredentialsSecretRef: corev1.LocalObjectReference{
						Name: testSASLSecret,
					},
				},
			},
		},
	}

	vm := buildSASLVolumeMount(mc)

	if vm == nil {
		t.Fatal("expected non-nil VolumeMount")
		return
	}
	if vm.Name != saslVolumeName {
		t.Errorf("volumeMount name = %q, want %q", vm.Name, saslVolumeName)
	}
	if vm.MountPath != "/etc/memcached/sasl" {
		t.Errorf("mountPath = %q, want %q", vm.MountPath, "/etc/memcached/sasl")
	}
	if !vm.ReadOnly {
		t.Error("expected readOnly=true")
	}
}

func TestBuildSASLVolumeMount_ReturnsNil(t *testing.T) {
	tests := []struct {
		name     string
		security *memcachedv1beta1.SecuritySpec
	}{
		{name: "nil Security", security: nil},
		{name: "nil SASL", security: &memcachedv1beta1.SecuritySpec{}},
		{
			name: "SASL disabled",
			security: &memcachedv1beta1.SecuritySpec{
				SASL: &memcachedv1beta1.SASLSpec{Enabled: false},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mc := &memcachedv1beta1.Memcached{
				Spec: memcachedv1beta1.MemcachedSpec{Security: tt.security},
			}

			if vm := buildSASLVolumeMount(mc); vm != nil {
				t.Errorf("expected nil VolumeMount, got %+v", vm)
			}
		})
	}
}

func TestBuildMemcachedArgs_SASLEnabled(t *testing.T) {
	sasl := &memcachedv1beta1.SASLSpec{
		Enabled: true,
		CredentialsSecretRef: corev1.LocalObjectReference{
			Name: testSASLSecret,
		},
	}

	got := buildMemcachedArgs(nil, sasl, nil)

	// Should contain -Y /etc/memcached/sasl/password-file after standard flags.
	expected := []string{
		"-m", "64", "-c", "1024", "-t", "4", "-I", "1m",
		"-Y", "/etc/memcached/sasl/password-file",
	}
	if len(got) != len(expected) {
		t.Fatalf("buildMemcachedArgs() returned %d args, want %d\ngot:  %v\nwant: %v",
			len(got), len(expected), got, expected)
	}
	for i := range expected {
		if got[i] != expected[i] {
			t.Errorf("buildMemcachedArgs()[%d] = %q, want %q\ngot:  %v\nwant: %v",
				i, got[i], expected[i], got, expected)
		}
	}
}

func TestBuildMemcachedArgs_SASLDisabled(t *testing.T) {
	sasl := &memcachedv1beta1.SASLSpec{
		Enabled: false,
	}

	got := buildMemcachedArgs(nil, sasl, nil)

	// Should NOT contain -Y flag.
	expected := []string{"-m", "64", "-c", "1024", "-t", "4", "-I", "1m"}
	if len(got) != len(expected) {
		t.Fatalf("buildMemcachedArgs() returned %d args, want %d\ngot:  %v\nwant: %v",
			len(got), len(expected), got, expected)
	}
	for i := range expected {
		if got[i] != expected[i] {
			t.Errorf("buildMemcachedArgs()[%d] = %q, want %q", i, got[i], expected[i])
		}
	}
}

func TestBuildMemcachedArgs_SASLNil(t *testing.T) {
	got := buildMemcachedArgs(nil, nil, nil)

	expected := []string{"-m", "64", "-c", "1024", "-t", "4", "-I", "1m"}
	if len(got) != len(expected) {
		t.Fatalf("buildMemcachedArgs() returned %d args, want %d\ngot:  %v\nwant: %v",
			len(got), len(expected), got, expected)
	}
}

func TestBuildMemcachedArgs_SASLWithVerbosityAndExtraArgs(t *testing.T) {
	config := &memcachedv1beta1.MemcachedConfig{
		Verbosity: 1,
		ExtraArgs: []string{"-o", "modern"},
	}
	sasl := &memcachedv1beta1.SASLSpec{
		Enabled: true,
		CredentialsSecretRef: corev1.LocalObjectReference{
			Name: testSASLSecret,
		},
	}

	got := buildMemcachedArgs(config, sasl, nil)

	// Order: standard flags, verbosity, -Y flag, extra args.
	expected := []string{
		"-m", "64", "-c", "1024", "-t", "4", "-I", "1m",
		"-v",
		"-Y", "/etc/memcached/sasl/password-file",
		"-o", "modern",
	}
	if len(got) != len(expected) {
		t.Fatalf("buildMemcachedArgs() returned %d args, want %d\ngot:  %v\nwant: %v",
			len(got), len(expected), got, expected)
	}
	for i := range expected {
		if got[i] != expected[i] {
			t.Errorf("buildMemcachedArgs()[%d] = %q, want %q\ngot:  %v\nwant: %v",
				i, got[i], expected[i], got, expected)
		}
	}
}

func TestConstructStatefulSet_SASLEnabled(t *testing.T) {
	mc := &memcachedv1beta1.Memcached{
		ObjectMeta: metav1.ObjectMeta{Name: "sasl-dep", Namespace: "default"},
		Spec: memcachedv1beta1.MemcachedSpec{
			Security: &memcachedv1beta1.SecuritySpec{
				SASL: &memcachedv1beta1.SASLSpec{
					Enabled: true,
					CredentialsSecretRef: corev1.LocalObjectReference{
						Name: testSASLSecret,
					},
				},
			},
		},
	}
	sts := &appsv1.StatefulSet{}

	constructStatefulSet(mc, sts, "", "")

	// Verify -Y flag in args.
	container := sts.Spec.Template.Spec.Containers[0]
	foundY := false
	for i, arg := range container.Args {
		if arg == "-Y" {
			foundY = true
			if i+1 >= len(container.Args) {
				t.Fatal("-Y flag has no value")
			}
			if container.Args[i+1] != "/etc/memcached/sasl/password-file" {
				t.Errorf("-Y value = %q, want %q", container.Args[i+1], "/etc/memcached/sasl/password-file")
			}
			break
		}
	}
	if !foundY {
		t.Errorf("expected -Y flag in args, got %v", container.Args)
	}

	// Verify volume mount on container.
	if len(container.VolumeMounts) != 1 {
		t.Fatalf("expected 1 volumeMount, got %d", len(container.VolumeMounts))
	}
	vm := container.VolumeMounts[0]
	if vm.Name != saslVolumeName {
		t.Errorf("volumeMount name = %q, want %q", vm.Name, saslVolumeName)
	}
	if vm.MountPath != "/etc/memcached/sasl" {
		t.Errorf("volumeMount mountPath = %q, want %q", vm.MountPath, "/etc/memcached/sasl")
	}
	if !vm.ReadOnly {
		t.Error("expected volumeMount readOnly=true")
	}

	// Verify volume on pod spec.
	volumes := sts.Spec.Template.Spec.Volumes
	if len(volumes) != 1 {
		t.Fatalf("expected 1 volume, got %d", len(volumes))
	}
	vol := volumes[0]
	if vol.Name != saslVolumeName {
		t.Errorf("volume name = %q, want %q", vol.Name, saslVolumeName)
	}
	if vol.Secret == nil {
		t.Fatal("expected Secret volume source")
	}
	if vol.Secret.SecretName != testSASLSecret {
		t.Errorf("volume secretName = %q, want %q", vol.Secret.SecretName, testSASLSecret)
	}
	if len(vol.Secret.Items) != 1 {
		t.Fatalf("expected 1 Items entry, got %d", len(vol.Secret.Items))
	}
	if vol.Secret.Items[0].Key != testPasswordFileKey {
		t.Errorf("Items[0].Key = %q, want %q", vol.Secret.Items[0].Key, testPasswordFileKey)
	}
}

func TestConstructStatefulSet_SASLDisabled(t *testing.T) {
	mc := &memcachedv1beta1.Memcached{
		ObjectMeta: metav1.ObjectMeta{Name: "sasl-off", Namespace: "default"},
		Spec:       memcachedv1beta1.MemcachedSpec{},
	}
	sts := &appsv1.StatefulSet{}

	constructStatefulSet(mc, sts, "", "")

	container := sts.Spec.Template.Spec.Containers[0]

	// No -Y flag.
	for _, arg := range container.Args {
		if arg == "-Y" {
			t.Error("unexpected -Y flag when SASL is not enabled")
		}
	}

	// No volume mounts.
	if len(container.VolumeMounts) != 0 {
		t.Errorf("expected 0 volumeMounts, got %d: %v", len(container.VolumeMounts), container.VolumeMounts)
	}

	// No volumes.
	if len(sts.Spec.Template.Spec.Volumes) != 0 {
		t.Errorf("expected 0 volumes, got %d: %v", len(sts.Spec.Template.Spec.Volumes), sts.Spec.Template.Spec.Volumes)
	}
}

func TestConstructStatefulSet_SASLWithMonitoring(t *testing.T) {
	mc := &memcachedv1beta1.Memcached{
		ObjectMeta: metav1.ObjectMeta{Name: "sasl-mon", Namespace: "default"},
		Spec: memcachedv1beta1.MemcachedSpec{
			Security: &memcachedv1beta1.SecuritySpec{
				SASL: &memcachedv1beta1.SASLSpec{
					Enabled: true,
					CredentialsSecretRef: corev1.LocalObjectReference{
						Name: "sasl-creds",
					},
				},
			},
			Monitoring: &memcachedv1beta1.MonitoringSpec{
				Enabled: true,
			},
		},
	}
	sts := &appsv1.StatefulSet{}

	constructStatefulSet(mc, sts, "", "")

	// 2 containers: memcached + exporter.
	if len(sts.Spec.Template.Spec.Containers) != 2 {
		t.Fatalf("expected 2 containers, got %d", len(sts.Spec.Template.Spec.Containers))
	}

	// Memcached container has SASL volume mount.
	mcContainer := sts.Spec.Template.Spec.Containers[0]
	if len(mcContainer.VolumeMounts) != 1 {
		t.Fatalf("expected 1 volumeMount on memcached, got %d", len(mcContainer.VolumeMounts))
	}
	if mcContainer.VolumeMounts[0].Name != saslVolumeName {
		t.Errorf("memcached volumeMount name = %q, want %q", mcContainer.VolumeMounts[0].Name, saslVolumeName)
	}

	// Pod has SASL volume.
	if len(sts.Spec.Template.Spec.Volumes) != 1 {
		t.Fatalf("expected 1 volume, got %d", len(sts.Spec.Template.Spec.Volumes))
	}
	if sts.Spec.Template.Spec.Volumes[0].Secret.SecretName != "sasl-creds" {
		t.Errorf("volume secretName = %q, want %q", sts.Spec.Template.Spec.Volumes[0].Secret.SecretName, "sasl-creds")
	}
}

func TestConstructStatefulSet_SASLWithGracefulShutdown(t *testing.T) {
	mc := &memcachedv1beta1.Memcached{
		ObjectMeta: metav1.ObjectMeta{Name: "sasl-gs", Namespace: "default"},
		Spec: memcachedv1beta1.MemcachedSpec{
			Security: &memcachedv1beta1.SecuritySpec{
				SASL: &memcachedv1beta1.SASLSpec{
					Enabled: true,
					CredentialsSecretRef: corev1.LocalObjectReference{
						Name: "sasl-secret",
					},
				},
			},
			HighAvailability: &memcachedv1beta1.HighAvailabilitySpec{
				GracefulShutdown: &memcachedv1beta1.GracefulShutdownSpec{
					Enabled:                       true,
					PreStopDelaySeconds:           10,
					TerminationGracePeriodSeconds: 30,
				},
			},
		},
	}
	sts := &appsv1.StatefulSet{}

	constructStatefulSet(mc, sts, "", "")

	container := sts.Spec.Template.Spec.Containers[0]

	// SASL: container has volume mount.
	if len(container.VolumeMounts) != 1 {
		t.Fatalf("expected 1 volumeMount, got %d", len(container.VolumeMounts))
	}
	if container.VolumeMounts[0].Name != saslVolumeName {
		t.Errorf("volumeMount name = %q, want %q", container.VolumeMounts[0].Name, saslVolumeName)
	}

	// SASL: -Y flag in args.
	foundY := false
	for _, arg := range container.Args {
		if arg == "-Y" {
			foundY = true
			break
		}
	}
	if !foundY {
		t.Errorf("expected -Y flag in args, got %v", container.Args)
	}

	// Graceful shutdown: container has lifecycle preStop hook.
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

	// Graceful shutdown: pod has TerminationGracePeriodSeconds.
	tgps := sts.Spec.Template.Spec.TerminationGracePeriodSeconds
	if tgps == nil {
		t.Fatal("expected TerminationGracePeriodSeconds on pod spec")
		return
	}
	if *tgps != 30 {
		t.Errorf("TerminationGracePeriodSeconds = %d, want 30", *tgps)
	}

	// SASL: pod has volume.
	if len(sts.Spec.Template.Spec.Volumes) != 1 {
		t.Fatalf("expected 1 volume, got %d", len(sts.Spec.Template.Spec.Volumes))
	}
	if sts.Spec.Template.Spec.Volumes[0].Secret.SecretName != testSASLSecretName {
		t.Errorf("volume secretName = %q, want %q", sts.Spec.Template.Spec.Volumes[0].Secret.SecretName, testSASLSecretName)
	}
}

func TestConstructStatefulSet_SASLWithSecurityContexts(t *testing.T) {
	runAsNonRoot := true
	runAsUser := int64(1000)
	readOnly := true
	mc := &memcachedv1beta1.Memcached{
		ObjectMeta: metav1.ObjectMeta{Name: "sasl-sec", Namespace: "default"},
		Spec: memcachedv1beta1.MemcachedSpec{
			Security: &memcachedv1beta1.SecuritySpec{
				SASL: &memcachedv1beta1.SASLSpec{
					Enabled: true,
					CredentialsSecretRef: corev1.LocalObjectReference{
						Name: "sasl-secret",
					},
				},
				PodSecurityContext: &corev1.PodSecurityContext{
					RunAsNonRoot: &runAsNonRoot,
				},
				ContainerSecurityContext: &corev1.SecurityContext{
					RunAsUser:              &runAsUser,
					ReadOnlyRootFilesystem: &readOnly,
				},
			},
		},
	}
	sts := &appsv1.StatefulSet{}

	constructStatefulSet(mc, sts, "", "")

	container := sts.Spec.Template.Spec.Containers[0]

	// SASL: container has volume mount.
	if len(container.VolumeMounts) != 1 {
		t.Fatalf("expected 1 volumeMount, got %d", len(container.VolumeMounts))
	}
	if container.VolumeMounts[0].Name != saslVolumeName {
		t.Errorf("volumeMount name = %q, want %q", container.VolumeMounts[0].Name, saslVolumeName)
	}

	// SASL: -Y flag in args.
	foundY := false
	for _, arg := range container.Args {
		if arg == "-Y" {
			foundY = true
			break
		}
	}
	if !foundY {
		t.Errorf("expected -Y flag in args, got %v", container.Args)
	}

	// Pod security context.
	podSC := sts.Spec.Template.Spec.SecurityContext
	if podSC == nil {
		t.Fatal("expected non-nil pod SecurityContext")
		return
	}
	if podSC.RunAsNonRoot == nil || !*podSC.RunAsNonRoot {
		t.Error("expected pod RunAsNonRoot=true")
	}

	// Container security context.
	containerSC := container.SecurityContext
	if containerSC == nil {
		t.Fatal("expected non-nil container SecurityContext")
		return
	}
	if containerSC.RunAsUser == nil || *containerSC.RunAsUser != 1000 {
		t.Errorf("expected container RunAsUser=1000, got %v", containerSC.RunAsUser)
	}
	if containerSC.ReadOnlyRootFilesystem == nil || !*containerSC.ReadOnlyRootFilesystem {
		t.Error("expected container ReadOnlyRootFilesystem=true")
	}

	// SASL: pod has volume.
	if len(sts.Spec.Template.Spec.Volumes) != 1 {
		t.Fatalf("expected 1 volume, got %d", len(sts.Spec.Template.Spec.Volumes))
	}
	if sts.Spec.Template.Spec.Volumes[0].Secret.SecretName != testSASLSecretName {
		t.Errorf("volume secretName = %q, want %q", sts.Spec.Template.Spec.Volumes[0].Secret.SecretName, testSASLSecretName)
	}
}

func TestBuildTLSVolume_Enabled(t *testing.T) {
	mc := &memcachedv1beta1.Memcached{
		ObjectMeta: metav1.ObjectMeta{Name: "tls-vol", Namespace: "default"},
		Spec: memcachedv1beta1.MemcachedSpec{
			Security: &memcachedv1beta1.SecuritySpec{
				TLS: &memcachedv1beta1.TLSSpec{
					Enabled: true,
					CertificateSecretRef: corev1.LocalObjectReference{
						Name: testTLSSecret,
					},
				},
			},
		},
	}

	vol := buildTLSVolume(mc)

	if vol == nil {
		t.Fatal("expected non-nil Volume")
		return
	}
	if vol.Name != tlsVolumeName {
		t.Errorf("volume name = %q, want %q", vol.Name, tlsVolumeName)
	}
	if vol.Secret == nil {
		t.Fatal("expected Secret volume source")
	}
	if vol.Secret.SecretName != testTLSSecret {
		t.Errorf("secretName = %q, want %q", vol.Secret.SecretName, testTLSSecret)
	}
	if len(vol.Secret.Items) != 2 {
		t.Fatalf("expected 2 Items entries, got %d", len(vol.Secret.Items))
	}
	if vol.Secret.Items[0].Key != "tls.crt" {
		t.Errorf("Items[0].Key = %q, want %q", vol.Secret.Items[0].Key, "tls.crt")
	}
	if vol.Secret.Items[0].Path != "tls.crt" {
		t.Errorf("Items[0].Path = %q, want %q", vol.Secret.Items[0].Path, "tls.crt")
	}
	if vol.Secret.Items[1].Key != "tls.key" {
		t.Errorf("Items[1].Key = %q, want %q", vol.Secret.Items[1].Key, "tls.key")
	}
	if vol.Secret.Items[1].Path != "tls.key" {
		t.Errorf("Items[1].Path = %q, want %q", vol.Secret.Items[1].Path, "tls.key")
	}
}

func TestBuildTLSVolume_ReturnsNil(t *testing.T) {
	tests := []struct {
		name     string
		security *memcachedv1beta1.SecuritySpec
	}{
		{name: "nil Security", security: nil},
		{name: "nil TLS", security: &memcachedv1beta1.SecuritySpec{}},
		{
			name: "TLS disabled",
			security: &memcachedv1beta1.SecuritySpec{
				TLS: &memcachedv1beta1.TLSSpec{Enabled: false},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mc := &memcachedv1beta1.Memcached{
				Spec: memcachedv1beta1.MemcachedSpec{Security: tt.security},
			}

			if vol := buildTLSVolume(mc); vol != nil {
				t.Errorf("expected nil Volume, got %+v", vol)
			}
		})
	}
}

func TestBuildTLSVolume_WithClientCert(t *testing.T) {
	mc := &memcachedv1beta1.Memcached{
		ObjectMeta: metav1.ObjectMeta{Name: "tls-mtls", Namespace: "default"},
		Spec: memcachedv1beta1.MemcachedSpec{
			Security: &memcachedv1beta1.SecuritySpec{
				TLS: &memcachedv1beta1.TLSSpec{
					Enabled:          true,
					EnableClientCert: true,
					CertificateSecretRef: corev1.LocalObjectReference{
						Name: testTLSSecret,
					},
				},
			},
		},
	}

	vol := buildTLSVolume(mc)

	if vol == nil {
		t.Fatal("expected non-nil Volume")
		return
	}
	if len(vol.Secret.Items) != 3 {
		t.Fatalf("expected 3 Items entries, got %d", len(vol.Secret.Items))
	}
	if vol.Secret.Items[2].Key != "ca.crt" {
		t.Errorf("Items[2].Key = %q, want %q", vol.Secret.Items[2].Key, "ca.crt")
	}
	if vol.Secret.Items[2].Path != "ca.crt" {
		t.Errorf("Items[2].Path = %q, want %q", vol.Secret.Items[2].Path, "ca.crt")
	}
}

func TestBuildTLSVolumeMount_Enabled(t *testing.T) {
	mc := &memcachedv1beta1.Memcached{
		ObjectMeta: metav1.ObjectMeta{Name: "tls-mount", Namespace: "default"},
		Spec: memcachedv1beta1.MemcachedSpec{
			Security: &memcachedv1beta1.SecuritySpec{
				TLS: &memcachedv1beta1.TLSSpec{
					Enabled: true,
					CertificateSecretRef: corev1.LocalObjectReference{
						Name: testTLSSecret,
					},
				},
			},
		},
	}

	vm := buildTLSVolumeMount(mc)

	if vm == nil {
		t.Fatal("expected non-nil VolumeMount")
		return
	}
	if vm.Name != tlsVolumeName {
		t.Errorf("volumeMount name = %q, want %q", vm.Name, tlsVolumeName)
	}
	if vm.MountPath != tlsMountPath {
		t.Errorf("mountPath = %q, want %q", vm.MountPath, tlsMountPath)
	}
	if !vm.ReadOnly {
		t.Error("expected readOnly=true")
	}
}

func TestBuildTLSVolumeMount_ReturnsNil(t *testing.T) {
	tests := []struct {
		name     string
		security *memcachedv1beta1.SecuritySpec
	}{
		{name: "nil Security", security: nil},
		{name: "nil TLS", security: &memcachedv1beta1.SecuritySpec{}},
		{
			name: "TLS disabled",
			security: &memcachedv1beta1.SecuritySpec{
				TLS: &memcachedv1beta1.TLSSpec{Enabled: false},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mc := &memcachedv1beta1.Memcached{
				Spec: memcachedv1beta1.MemcachedSpec{Security: tt.security},
			}

			if vm := buildTLSVolumeMount(mc); vm != nil {
				t.Errorf("expected nil VolumeMount, got %+v", vm)
			}
		})
	}
}

func TestBuildMemcachedArgs_TLSEnabled(t *testing.T) {
	tls := &memcachedv1beta1.TLSSpec{
		Enabled: true,
		CertificateSecretRef: corev1.LocalObjectReference{
			Name: testTLSSecret,
		},
	}

	got := buildMemcachedArgs(nil, nil, tls)

	expected := []string{
		"-m", "64", "-c", "1024", "-t", "4", "-I", "1m",
		"-Z",
		"-o", "ssl_chain_cert=/etc/memcached/tls/tls.crt",
		"-o", "ssl_key=/etc/memcached/tls/tls.key",
	}
	if len(got) != len(expected) {
		t.Fatalf("buildMemcachedArgs() returned %d args, want %d\ngot:  %v\nwant: %v",
			len(got), len(expected), got, expected)
	}
	for i := range expected {
		if got[i] != expected[i] {
			t.Errorf("buildMemcachedArgs()[%d] = %q, want %q\ngot:  %v\nwant: %v",
				i, got[i], expected[i], got, expected)
		}
	}
}

func TestBuildMemcachedArgs_TLSDisabled(t *testing.T) {
	tls := &memcachedv1beta1.TLSSpec{
		Enabled: false,
	}

	got := buildMemcachedArgs(nil, nil, tls)

	expected := []string{"-m", "64", "-c", "1024", "-t", "4", "-I", "1m"}
	if len(got) != len(expected) {
		t.Fatalf("buildMemcachedArgs() returned %d args, want %d\ngot:  %v\nwant: %v",
			len(got), len(expected), got, expected)
	}
	for i := range expected {
		if got[i] != expected[i] {
			t.Errorf("buildMemcachedArgs()[%d] = %q, want %q", i, got[i], expected[i])
		}
	}
}

func TestBuildMemcachedArgs_TLSNil(t *testing.T) {
	got := buildMemcachedArgs(nil, nil, nil)

	expected := []string{"-m", "64", "-c", "1024", "-t", "4", "-I", "1m"}
	if len(got) != len(expected) {
		t.Fatalf("buildMemcachedArgs() returned %d args, want %d\ngot:  %v\nwant: %v",
			len(got), len(expected), got, expected)
	}
}

func TestBuildMemcachedArgs_TLSWithClientCert(t *testing.T) {
	tls := &memcachedv1beta1.TLSSpec{
		Enabled:          true,
		EnableClientCert: true,
		CertificateSecretRef: corev1.LocalObjectReference{
			Name: testTLSSecret,
		},
	}

	got := buildMemcachedArgs(nil, nil, tls)

	expected := []string{
		"-m", "64", "-c", "1024", "-t", "4", "-I", "1m",
		"-Z",
		"-o", "ssl_chain_cert=/etc/memcached/tls/tls.crt",
		"-o", "ssl_key=/etc/memcached/tls/tls.key",
		"-o", "ssl_ca_cert=/etc/memcached/tls/ca.crt",
	}
	if len(got) != len(expected) {
		t.Fatalf("buildMemcachedArgs() returned %d args, want %d\ngot:  %v\nwant: %v",
			len(got), len(expected), got, expected)
	}
	for i := range expected {
		if got[i] != expected[i] {
			t.Errorf("buildMemcachedArgs()[%d] = %q, want %q\ngot:  %v\nwant: %v",
				i, got[i], expected[i], got, expected)
		}
	}
}

func TestBuildMemcachedArgs_TLSWithSASL(t *testing.T) {
	sasl := &memcachedv1beta1.SASLSpec{
		Enabled: true,
		CredentialsSecretRef: corev1.LocalObjectReference{
			Name: testSASLSecret,
		},
	}
	tls := &memcachedv1beta1.TLSSpec{
		Enabled: true,
		CertificateSecretRef: corev1.LocalObjectReference{
			Name: testTLSSecret,
		},
	}

	got := buildMemcachedArgs(nil, sasl, tls)

	// Order: standard flags, SASL -Y, TLS -Z/ssl flags.
	expected := []string{
		"-m", "64", "-c", "1024", "-t", "4", "-I", "1m",
		"-Y", "/etc/memcached/sasl/password-file",
		"-Z",
		"-o", "ssl_chain_cert=/etc/memcached/tls/tls.crt",
		"-o", "ssl_key=/etc/memcached/tls/tls.key",
	}
	if len(got) != len(expected) {
		t.Fatalf("buildMemcachedArgs() returned %d args, want %d\ngot:  %v\nwant: %v",
			len(got), len(expected), got, expected)
	}
	for i := range expected {
		if got[i] != expected[i] {
			t.Errorf("buildMemcachedArgs()[%d] = %q, want %q\ngot:  %v\nwant: %v",
				i, got[i], expected[i], got, expected)
		}
	}
}

func TestBuildMemcachedArgs_TLSWithVerbosityAndExtraArgs(t *testing.T) {
	config := &memcachedv1beta1.MemcachedConfig{
		Verbosity: 1,
		ExtraArgs: []string{"-o", "modern"},
	}
	tls := &memcachedv1beta1.TLSSpec{
		Enabled: true,
		CertificateSecretRef: corev1.LocalObjectReference{
			Name: testTLSSecret,
		},
	}

	got := buildMemcachedArgs(config, nil, tls)

	// Order: standard flags, verbosity, TLS flags, extra args.
	expected := []string{
		"-m", "64", "-c", "1024", "-t", "4", "-I", "1m",
		"-v",
		"-Z",
		"-o", "ssl_chain_cert=/etc/memcached/tls/tls.crt",
		"-o", "ssl_key=/etc/memcached/tls/tls.key",
		"-o", "modern",
	}
	if len(got) != len(expected) {
		t.Fatalf("buildMemcachedArgs() returned %d args, want %d\ngot:  %v\nwant: %v",
			len(got), len(expected), got, expected)
	}
	for i := range expected {
		if got[i] != expected[i] {
			t.Errorf("buildMemcachedArgs()[%d] = %q, want %q\ngot:  %v\nwant: %v",
				i, got[i], expected[i], got, expected)
		}
	}
}

func TestConstructStatefulSet_TLSEnabled(t *testing.T) {
	mc := &memcachedv1beta1.Memcached{
		ObjectMeta: metav1.ObjectMeta{Name: "tls-dep", Namespace: "default"},
		Spec: memcachedv1beta1.MemcachedSpec{
			Security: &memcachedv1beta1.SecuritySpec{
				TLS: &memcachedv1beta1.TLSSpec{
					Enabled: true,
					CertificateSecretRef: corev1.LocalObjectReference{
						Name: testTLSSecret,
					},
				},
			},
		},
	}
	sts := &appsv1.StatefulSet{}

	constructStatefulSet(mc, sts, "", "")

	container := sts.Spec.Template.Spec.Containers[0]

	// Verify -Z flag in args.
	foundZ := false
	for _, arg := range container.Args {
		if arg == "-Z" {
			foundZ = true
			break
		}
	}
	if !foundZ {
		t.Errorf("expected -Z flag in args, got %v", container.Args)
	}

	// Verify ssl_chain_cert in args.
	foundChainCert := false
	for _, arg := range container.Args {
		if arg == "ssl_chain_cert=/etc/memcached/tls/tls.crt" {
			foundChainCert = true
			break
		}
	}
	if !foundChainCert {
		t.Errorf("expected ssl_chain_cert arg, got %v", container.Args)
	}

	// Verify ssl_key in args.
	foundKey := false
	for _, arg := range container.Args {
		if arg == "ssl_key=/etc/memcached/tls/tls.key" {
			foundKey = true
			break
		}
	}
	if !foundKey {
		t.Errorf("expected ssl_key arg, got %v", container.Args)
	}

	// Verify volume mount on container.
	if len(container.VolumeMounts) != 1 {
		t.Fatalf("expected 1 volumeMount, got %d", len(container.VolumeMounts))
	}
	vm := container.VolumeMounts[0]
	if vm.Name != tlsVolumeName {
		t.Errorf("volumeMount name = %q, want %q", vm.Name, tlsVolumeName)
	}
	if vm.MountPath != tlsMountPath {
		t.Errorf("volumeMount mountPath = %q, want %q", vm.MountPath, tlsMountPath)
	}
	if !vm.ReadOnly {
		t.Error("expected volumeMount readOnly=true")
	}

	// Verify volume on pod spec.
	volumes := sts.Spec.Template.Spec.Volumes
	if len(volumes) != 1 {
		t.Fatalf("expected 1 volume, got %d", len(volumes))
	}
	vol := volumes[0]
	if vol.Name != tlsVolumeName {
		t.Errorf("volume name = %q, want %q", vol.Name, tlsVolumeName)
	}
	if vol.Secret == nil {
		t.Fatal("expected Secret volume source")
	}
	if vol.Secret.SecretName != testTLSSecret {
		t.Errorf("volume secretName = %q, want %q", vol.Secret.SecretName, testTLSSecret)
	}
	if len(vol.Secret.Items) != 2 {
		t.Fatalf("expected 2 Items entries, got %d", len(vol.Secret.Items))
	}
}

func TestConstructStatefulSet_TLSDisabled(t *testing.T) {
	mc := &memcachedv1beta1.Memcached{
		ObjectMeta: metav1.ObjectMeta{Name: "tls-off", Namespace: "default"},
		Spec:       memcachedv1beta1.MemcachedSpec{},
	}
	sts := &appsv1.StatefulSet{}

	constructStatefulSet(mc, sts, "", "")

	container := sts.Spec.Template.Spec.Containers[0]

	// No -Z flag.
	for _, arg := range container.Args {
		if arg == "-Z" {
			t.Error("unexpected -Z flag when TLS is not enabled")
		}
	}

	// No volume mounts.
	if len(container.VolumeMounts) != 0 {
		t.Errorf("expected 0 volumeMounts, got %d: %v", len(container.VolumeMounts), container.VolumeMounts)
	}

	// No volumes.
	if len(sts.Spec.Template.Spec.Volumes) != 0 {
		t.Errorf("expected 0 volumes, got %d: %v", len(sts.Spec.Template.Spec.Volumes), sts.Spec.Template.Spec.Volumes)
	}

	// Only one port (11211).
	if len(container.Ports) != 1 {
		t.Fatalf("expected 1 port, got %d", len(container.Ports))
	}
	if container.Ports[0].ContainerPort != 11211 {
		t.Errorf("port = %d, want 11211", container.Ports[0].ContainerPort)
	}
}

func TestConstructStatefulSet_TLSWithSASL(t *testing.T) {
	mc := &memcachedv1beta1.Memcached{
		ObjectMeta: metav1.ObjectMeta{Name: "tls-sasl", Namespace: "default"},
		Spec: memcachedv1beta1.MemcachedSpec{
			Security: &memcachedv1beta1.SecuritySpec{
				SASL: &memcachedv1beta1.SASLSpec{
					Enabled: true,
					CredentialsSecretRef: corev1.LocalObjectReference{
						Name: "sasl-creds",
					},
				},
				TLS: &memcachedv1beta1.TLSSpec{
					Enabled: true,
					CertificateSecretRef: corev1.LocalObjectReference{
						Name: "tls-certs",
					},
				},
			},
		},
	}
	sts := &appsv1.StatefulSet{}

	constructStatefulSet(mc, sts, "", "")

	container := sts.Spec.Template.Spec.Containers[0]

	// Both -Y and -Z flags in args.
	foundY := false
	foundZ := false
	yIdx := -1
	zIdx := -1
	for i, arg := range container.Args {
		if arg == "-Y" {
			foundY = true
			yIdx = i
		}
		if arg == "-Z" {
			foundZ = true
			zIdx = i
		}
	}
	if !foundY {
		t.Errorf("expected -Y flag in args, got %v", container.Args)
	}
	if !foundZ {
		t.Errorf("expected -Z flag in args, got %v", container.Args)
	}
	if yIdx >= 0 && zIdx >= 0 && yIdx >= zIdx {
		t.Errorf("expected -Y (index %d) before -Z (index %d)", yIdx, zIdx)
	}

	// Both volume mounts: sasl-credentials and tls-certificates.
	if len(container.VolumeMounts) != 2 {
		t.Fatalf("expected 2 volumeMounts, got %d", len(container.VolumeMounts))
	}
	if container.VolumeMounts[0].Name != saslVolumeName {
		t.Errorf("volumeMount[0] name = %q, want %q", container.VolumeMounts[0].Name, saslVolumeName)
	}
	if container.VolumeMounts[1].Name != tlsVolumeName {
		t.Errorf("volumeMount[1] name = %q, want %q", container.VolumeMounts[1].Name, tlsVolumeName)
	}

	// Both volumes.
	volumes := sts.Spec.Template.Spec.Volumes
	if len(volumes) != 2 {
		t.Fatalf("expected 2 volumes, got %d", len(volumes))
	}
	if volumes[0].Name != saslVolumeName {
		t.Errorf("volume[0] name = %q, want %q", volumes[0].Name, saslVolumeName)
	}
	if volumes[1].Name != tlsVolumeName {
		t.Errorf("volume[1] name = %q, want %q", volumes[1].Name, tlsVolumeName)
	}

	// Two ports: 11211 and 11212.
	if len(container.Ports) != 2 {
		t.Fatalf("expected 2 ports, got %d", len(container.Ports))
	}
	if container.Ports[0].ContainerPort != 11211 {
		t.Errorf("port[0] = %d, want 11211", container.Ports[0].ContainerPort)
	}
	if container.Ports[1].ContainerPort != 11212 {
		t.Errorf("port[1] = %d, want 11212", container.Ports[1].ContainerPort)
	}
}

func TestConstructStatefulSet_TLSPort(t *testing.T) {
	mc := &memcachedv1beta1.Memcached{
		ObjectMeta: metav1.ObjectMeta{Name: "tls-port", Namespace: "default"},
		Spec: memcachedv1beta1.MemcachedSpec{
			Security: &memcachedv1beta1.SecuritySpec{
				TLS: &memcachedv1beta1.TLSSpec{
					Enabled: true,
					CertificateSecretRef: corev1.LocalObjectReference{
						Name: testTLSSecret,
					},
				},
			},
		},
	}
	sts := &appsv1.StatefulSet{}

	constructStatefulSet(mc, sts, "", "")

	container := sts.Spec.Template.Spec.Containers[0]

	if len(container.Ports) != 2 {
		t.Fatalf("expected 2 ports, got %d", len(container.Ports))
	}

	// Port 11211 named "memcached".
	if container.Ports[0].Name != testPortName {
		t.Errorf("port[0] name = %q, want %q", container.Ports[0].Name, "memcached")
	}
	if container.Ports[0].ContainerPort != 11211 {
		t.Errorf("port[0] = %d, want 11211", container.Ports[0].ContainerPort)
	}
	if container.Ports[0].Protocol != corev1.ProtocolTCP {
		t.Errorf("port[0] protocol = %q, want TCP", container.Ports[0].Protocol)
	}

	// Port 11212 named "memcached-tls".
	if container.Ports[1].Name != tlsPortName {
		t.Errorf("port[1] name = %q, want %q", container.Ports[1].Name, tlsPortName)
	}
	if container.Ports[1].ContainerPort != 11212 {
		t.Errorf("port[1] = %d, want 11212", container.Ports[1].ContainerPort)
	}
	if container.Ports[1].Protocol != corev1.ProtocolTCP {
		t.Errorf("port[1] protocol = %q, want TCP", container.Ports[1].Protocol)
	}
}

func TestConstructStatefulSet_TLSWithMonitoringAndSecurityContexts(t *testing.T) {
	runAsNonRoot := true
	readOnly := true
	mc := &memcachedv1beta1.Memcached{
		ObjectMeta: metav1.ObjectMeta{Name: "tls-full", Namespace: "default"},
		Spec: memcachedv1beta1.MemcachedSpec{
			Security: &memcachedv1beta1.SecuritySpec{
				TLS: &memcachedv1beta1.TLSSpec{
					Enabled: true,
					CertificateSecretRef: corev1.LocalObjectReference{
						Name: "tls-certs",
					},
				},
				PodSecurityContext: &corev1.PodSecurityContext{
					RunAsNonRoot: &runAsNonRoot,
				},
				ContainerSecurityContext: &corev1.SecurityContext{
					ReadOnlyRootFilesystem: &readOnly,
				},
			},
			Monitoring: &memcachedv1beta1.MonitoringSpec{
				Enabled: true,
			},
		},
	}
	sts := &appsv1.StatefulSet{}

	constructStatefulSet(mc, sts, "", "")

	// 2 containers: memcached + exporter.
	if len(sts.Spec.Template.Spec.Containers) != 2 {
		t.Fatalf("expected 2 containers, got %d", len(sts.Spec.Template.Spec.Containers))
	}

	mcContainer := sts.Spec.Template.Spec.Containers[0]

	// TLS: container has volume mount.
	if len(mcContainer.VolumeMounts) != 1 {
		t.Fatalf("expected 1 volumeMount on memcached, got %d", len(mcContainer.VolumeMounts))
	}
	if mcContainer.VolumeMounts[0].Name != tlsVolumeName {
		t.Errorf("memcached volumeMount name = %q, want %q", mcContainer.VolumeMounts[0].Name, tlsVolumeName)
	}

	// TLS: 2 ports on memcached container.
	if len(mcContainer.Ports) != 2 {
		t.Fatalf("expected 2 ports on memcached, got %d", len(mcContainer.Ports))
	}

	// Pod has TLS volume.
	if len(sts.Spec.Template.Spec.Volumes) != 1 {
		t.Fatalf("expected 1 volume, got %d", len(sts.Spec.Template.Spec.Volumes))
	}

	// Pod security context applied.
	podSC := sts.Spec.Template.Spec.SecurityContext
	if podSC == nil {
		t.Fatal("expected non-nil pod SecurityContext")
		return
	}
	if podSC.RunAsNonRoot == nil || !*podSC.RunAsNonRoot {
		t.Error("expected pod RunAsNonRoot=true")
	}

	// Container security context on both containers.
	if mcContainer.SecurityContext == nil {
		t.Fatal("expected non-nil container SecurityContext on memcached")
	}
	if mcContainer.SecurityContext.ReadOnlyRootFilesystem == nil || !*mcContainer.SecurityContext.ReadOnlyRootFilesystem {
		t.Error("expected ReadOnlyRootFilesystem=true on memcached")
	}
	exporterContainer := sts.Spec.Template.Spec.Containers[1]
	if exporterContainer.SecurityContext == nil {
		t.Fatal("expected non-nil container SecurityContext on exporter")
	}
}

// kitchenSinkStatefulSet constructs a StatefulSet from a Memcached CR with ALL features enabled.
// Used by TestConstructStatefulSet_KitchenSink_* subtests.
func kitchenSinkStatefulSet(t *testing.T) *appsv1.StatefulSet {
	t.Helper()
	runAsNonRoot := true
	runAsUser := int64(1000)
	readOnlyRootFS := true
	allowPrivEsc := false
	exporterImg := testExporterImage

	mc := &memcachedv1beta1.Memcached{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "kitchen-sink",
			Namespace: "production",
		},
		Spec: memcachedv1beta1.MemcachedSpec{
			Replicas: int32Ptr(3),
			Image:    stringPtr("memcached:1.6.29"),
			Resources: &corev1.ResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("250m"),
					corev1.ResourceMemory: resource.MustParse("512Mi"),
				},
				Limits: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("1"),
					corev1.ResourceMemory: resource.MustParse("1Gi"),
				},
			},
			Memcached: &memcachedv1beta1.MemcachedConfig{
				MaxMemoryMB:    256,
				MaxConnections: 2048,
				Threads:        8,
				MaxItemSize:    "2m",
				Verbosity:      2,
				ExtraArgs:      []string{"--max-reqs-per-event", "20"},
			},
			HighAvailability: &memcachedv1beta1.HighAvailabilitySpec{
				AntiAffinityPreset:        antiAffinityPresetPtr(memcachedv1beta1.AntiAffinityPresetSoft),
				TopologySpreadConstraints: []corev1.TopologySpreadConstraint{zoneSpreadConstraint()},
				GracefulShutdown: &memcachedv1beta1.GracefulShutdownSpec{
					Enabled:                       true,
					PreStopDelaySeconds:           15,
					TerminationGracePeriodSeconds: 45,
				},
			},
			Security: &memcachedv1beta1.SecuritySpec{
				PodSecurityContext: &corev1.PodSecurityContext{
					RunAsNonRoot: &runAsNonRoot,
					RunAsUser:    &runAsUser,
				},
				ContainerSecurityContext: &corev1.SecurityContext{
					ReadOnlyRootFilesystem:   &readOnlyRootFS,
					AllowPrivilegeEscalation: &allowPrivEsc,
				},
				SASL: &memcachedv1beta1.SASLSpec{
					Enabled: true,
					CredentialsSecretRef: corev1.LocalObjectReference{
						Name: testSASLSecret,
					},
				},
				TLS: &memcachedv1beta1.TLSSpec{
					Enabled:          true,
					EnableClientCert: true,
					CertificateSecretRef: corev1.LocalObjectReference{
						Name: testTLSSecret,
					},
				},
			},
			Monitoring: &memcachedv1beta1.MonitoringSpec{
				Enabled:       true,
				ExporterImage: &exporterImg,
				ExporterResources: &corev1.ResourceRequirements{
					Requests: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("50m"),
						corev1.ResourceMemory: resource.MustParse("64Mi"),
					},
					Limits: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse(testCPU100m),
						corev1.ResourceMemory: resource.MustParse(testMem128Mi),
					},
				},
			},
			Service: &memcachedv1beta1.ServiceSpec{
				Annotations: map[string]string{
					"prometheus.io/scrape": "true",
				},
			},
		},
	}
	sts := &appsv1.StatefulSet{}
	constructStatefulSet(mc, sts, "", "")
	return sts
}

func TestConstructStatefulSet_KitchenSink_Containers(t *testing.T) {
	sts := kitchenSinkStatefulSet(t)

	if sts.Spec.Replicas == nil || *sts.Spec.Replicas != 3 {
		t.Errorf("expected 3 replicas, got %v", sts.Spec.Replicas)
	}

	containers := sts.Spec.Template.Spec.Containers
	if len(containers) != 2 {
		t.Fatalf("expected 2 containers (memcached + exporter), got %d", len(containers))
	}

	mc := containers[0]
	if mc.Image != "memcached:1.6.29" {
		t.Errorf("memcached image = %q, want memcached:1.6.29", mc.Image)
	}
	if mc.Name != testPortName {
		t.Errorf("memcached container name = %q, want %q", mc.Name, testPortName)
	}
	if mc.LivenessProbe == nil {
		t.Error("expected liveness probe")
	}
	if mc.ReadinessProbe == nil {
		t.Error("expected readiness probe")
	}

	exp := containers[1]
	if exp.Name != testExporterContainer {
		t.Errorf("exporter container name = %q, want exporter", exp.Name)
	}
	if exp.Image != testExporterImage {
		t.Errorf("exporter image = %q, want my-registry/memcached-exporter:v1.0.0", exp.Image)
	}
}

func TestConstructStatefulSet_KitchenSink_Args(t *testing.T) {
	sts := kitchenSinkStatefulSet(t)
	mc := sts.Spec.Template.Spec.Containers[0]

	expectedArgs := []string{
		"-m", "256", "-c", "2048", "-t", "8", "-I", "2m",
		"-vv",
		"-Y", saslMountPath + "/password-file",
		"-Z",
		"-o", "ssl_chain_cert=/etc/memcached/tls/tls.crt",
		"-o", "ssl_key=/etc/memcached/tls/tls.key",
		"-o", "ssl_ca_cert=/etc/memcached/tls/ca.crt",
		"--max-reqs-per-event", "20",
	}
	if len(mc.Args) != len(expectedArgs) {
		t.Fatalf("expected %d args, got %d\ngot:  %v\nwant: %v",
			len(expectedArgs), len(mc.Args), mc.Args, expectedArgs)
	}
	for i, arg := range expectedArgs {
		if mc.Args[i] != arg {
			t.Errorf("args[%d] = %q, want %q", i, mc.Args[i], arg)
		}
	}
}

func TestConstructStatefulSet_KitchenSink_Resources(t *testing.T) {
	sts := kitchenSinkStatefulSet(t)
	mc := sts.Spec.Template.Spec.Containers[0]
	exp := sts.Spec.Template.Spec.Containers[1]

	// Memcached container resources.
	if cpuReq := mc.Resources.Requests[corev1.ResourceCPU]; cpuReq.String() != "250m" {
		t.Errorf("memcached cpu request = %s, want 250m", cpuReq.String())
	}
	if memReq := mc.Resources.Requests[corev1.ResourceMemory]; memReq.String() != "512Mi" {
		t.Errorf("memcached memory request = %s, want 512Mi", memReq.String())
	}
	if cpuLim := mc.Resources.Limits[corev1.ResourceCPU]; cpuLim.String() != "1" {
		t.Errorf("memcached cpu limit = %s, want 1", cpuLim.String())
	}
	if memLim := mc.Resources.Limits[corev1.ResourceMemory]; memLim.String() != "1Gi" {
		t.Errorf("memcached memory limit = %s, want 1Gi", memLim.String())
	}

	// Exporter container resources.
	if cpuReq := exp.Resources.Requests[corev1.ResourceCPU]; cpuReq.String() != "50m" {
		t.Errorf("exporter cpu request = %s, want 50m", cpuReq.String())
	}
	if memReq := exp.Resources.Requests[corev1.ResourceMemory]; memReq.String() != "64Mi" {
		t.Errorf("exporter memory request = %s, want 64Mi", memReq.String())
	}
	if cpuLim := exp.Resources.Limits[corev1.ResourceCPU]; cpuLim.String() != testCPU100m {
		t.Errorf("exporter cpu limit = %s, want 100m", cpuLim.String())
	}
	if memLim := exp.Resources.Limits[corev1.ResourceMemory]; memLim.String() != testMem128Mi {
		t.Errorf("exporter memory limit = %s, want 128Mi", memLim.String())
	}
}

func TestConstructStatefulSet_KitchenSink_Ports(t *testing.T) {
	sts := kitchenSinkStatefulSet(t)
	mc := sts.Spec.Template.Spec.Containers[0]
	exp := sts.Spec.Template.Spec.Containers[1]

	if len(mc.Ports) != 2 {
		t.Fatalf("expected 2 memcached ports, got %d", len(mc.Ports))
	}
	if mc.Ports[0].Name != testPortName || mc.Ports[0].ContainerPort != 11211 {
		t.Errorf("port[0] = %+v, want name=%s containerPort=11211", mc.Ports[0], testPortName)
	}
	if mc.Ports[1].Name != tlsPortName || mc.Ports[1].ContainerPort != 11212 {
		t.Errorf("port[1] = %+v, want name=%s containerPort=11212", mc.Ports[1], tlsPortName)
	}

	if len(exp.Ports) != 1 {
		t.Fatalf("expected 1 exporter port, got %d", len(exp.Ports))
	}
	if exp.Ports[0].Name != testMetricsPort || exp.Ports[0].ContainerPort != 9150 {
		t.Errorf("exporter port = %+v, want name=metrics containerPort=9150", exp.Ports[0])
	}
}

func TestConstructStatefulSet_KitchenSink_SecurityContexts(t *testing.T) {
	sts := kitchenSinkStatefulSet(t)
	mc := sts.Spec.Template.Spec.Containers[0]
	exp := sts.Spec.Template.Spec.Containers[1]

	// Pod-level security context.
	podSC := sts.Spec.Template.Spec.SecurityContext
	if podSC == nil {
		t.Fatal("expected non-nil pod SecurityContext")
		return
	}
	if podSC.RunAsNonRoot == nil || !*podSC.RunAsNonRoot {
		t.Error("expected pod RunAsNonRoot=true")
	}
	if podSC.RunAsUser == nil || *podSC.RunAsUser != 1000 {
		t.Errorf("pod RunAsUser = %v, want 1000", podSC.RunAsUser)
	}

	// Memcached container security context.
	mcSC := mc.SecurityContext
	if mcSC == nil {
		t.Fatal("expected non-nil memcached SecurityContext")
		return
	}
	if mcSC.ReadOnlyRootFilesystem == nil || !*mcSC.ReadOnlyRootFilesystem {
		t.Error("expected memcached ReadOnlyRootFilesystem=true")
	}
	if mcSC.AllowPrivilegeEscalation == nil || *mcSC.AllowPrivilegeEscalation {
		t.Error("expected memcached AllowPrivilegeEscalation=false")
	}

	// Exporter container security context.
	expSC := exp.SecurityContext
	if expSC == nil {
		t.Fatal("expected non-nil exporter SecurityContext")
		return
	}
	if expSC.ReadOnlyRootFilesystem == nil || !*expSC.ReadOnlyRootFilesystem {
		t.Error("expected exporter ReadOnlyRootFilesystem=true")
	}
	if expSC.AllowPrivilegeEscalation == nil || *expSC.AllowPrivilegeEscalation {
		t.Error("expected exporter AllowPrivilegeEscalation=false")
	}
}

func TestConstructStatefulSet_KitchenSink_HA(t *testing.T) {
	sts := kitchenSinkStatefulSet(t)
	mc := sts.Spec.Template.Spec.Containers[0]

	// Graceful shutdown.
	if mc.Lifecycle == nil || mc.Lifecycle.PreStop == nil || mc.Lifecycle.PreStop.Exec == nil {
		t.Fatal("expected PreStop Exec handler on memcached container")
	}
	expectedCmd := []string{"sleep", "15"}
	for i, cmd := range expectedCmd {
		if mc.Lifecycle.PreStop.Exec.Command[i] != cmd {
			t.Errorf("preStop command[%d] = %q, want %q", i, mc.Lifecycle.PreStop.Exec.Command[i], cmd)
		}
	}
	tgps := sts.Spec.Template.Spec.TerminationGracePeriodSeconds
	if tgps == nil || *tgps != 45 {
		t.Errorf("TerminationGracePeriodSeconds = %v, want 45", tgps)
	}

	// Anti-affinity.
	affinity := sts.Spec.Template.Spec.Affinity
	if affinity == nil || affinity.PodAntiAffinity == nil {
		t.Fatal("expected non-nil Affinity with PodAntiAffinity")
	}
	preferred := affinity.PodAntiAffinity.PreferredDuringSchedulingIgnoredDuringExecution
	if len(preferred) != 1 {
		t.Fatalf("expected 1 preferred anti-affinity term, got %d", len(preferred))
	}
	if preferred[0].Weight != 100 {
		t.Errorf("anti-affinity weight = %d, want 100", preferred[0].Weight)
	}
	if preferred[0].PodAffinityTerm.TopologyKey != testHostnameTopology {
		t.Errorf("anti-affinity topologyKey = %q, want kubernetes.io/hostname", preferred[0].PodAffinityTerm.TopologyKey)
	}

	// Topology spread.
	tsc := sts.Spec.Template.Spec.TopologySpreadConstraints
	if len(tsc) != 1 {
		t.Fatalf("expected 1 topology spread constraint, got %d", len(tsc))
	}
	if tsc[0].TopologyKey != testZoneTopology {
		t.Errorf("topology spread topologyKey = %q, want topology.kubernetes.io/zone", tsc[0].TopologyKey)
	}
	if tsc[0].WhenUnsatisfiable != corev1.DoNotSchedule {
		t.Errorf("whenUnsatisfiable = %q, want DoNotSchedule", tsc[0].WhenUnsatisfiable)
	}
}

func TestConstructStatefulSet_KitchenSink_Volumes(t *testing.T) {
	sts := kitchenSinkStatefulSet(t)
	mc := sts.Spec.Template.Spec.Containers[0]

	// Volume mounts.
	if len(mc.VolumeMounts) != 2 {
		t.Fatalf("expected 2 volumeMounts, got %d", len(mc.VolumeMounts))
	}
	if mc.VolumeMounts[0].Name != saslVolumeName || mc.VolumeMounts[0].MountPath != saslMountPath {
		t.Errorf("volumeMount[0] = {Name:%q MountPath:%q}, want {Name:%q MountPath:%q}",
			mc.VolumeMounts[0].Name, mc.VolumeMounts[0].MountPath, saslVolumeName, saslMountPath)
	}
	if !mc.VolumeMounts[0].ReadOnly {
		t.Error("expected SASL volumeMount readOnly=true")
	}
	if mc.VolumeMounts[1].Name != tlsVolumeName {
		t.Errorf("volumeMount[1] name = %q, want %q", mc.VolumeMounts[1].Name, tlsVolumeName)
	}
	if !mc.VolumeMounts[1].ReadOnly {
		t.Error("expected TLS volumeMount readOnly=true")
	}

	// Volumes.
	volumes := sts.Spec.Template.Spec.Volumes
	if len(volumes) != 2 {
		t.Fatalf("expected 2 volumes (SASL + TLS), got %d", len(volumes))
	}
	if volumes[0].Name != saslVolumeName || volumes[0].Secret == nil || volumes[0].Secret.SecretName != testSASLSecret {
		t.Errorf("SASL volume = %+v, want name=%s secret=my-sasl-secret", volumes[0], saslVolumeName)
	}
	if volumes[1].Name != tlsVolumeName || volumes[1].Secret == nil || volumes[1].Secret.SecretName != testTLSSecret {
		t.Errorf("TLS volume = %+v, want name=%s secret=my-tls-secret", volumes[1], tlsVolumeName)
	}
	if len(volumes[1].Secret.Items) != 3 {
		t.Fatalf("expected 3 TLS volume items, got %d", len(volumes[1].Secret.Items))
	}
	wantKeys := []string{"tls.crt", "tls.key", "ca.crt"}
	for i, wantKey := range wantKeys {
		if volumes[1].Secret.Items[i].Key != wantKey {
			t.Errorf("TLS items[%d].Key = %q, want %s", i, volumes[1].Secret.Items[i].Key, wantKey)
		}
	}
}

func TestConstructStatefulSet_KitchenSink_StrategyAndLabels(t *testing.T) {
	sts := kitchenSinkStatefulSet(t)

	// Rolling update strategy, parallel pod management, governing Service, no volume claims.
	if sts.Spec.UpdateStrategy.Type != appsv1.RollingUpdateStatefulSetStrategyType {
		t.Errorf("updateStrategy type = %q, want RollingUpdate", sts.Spec.UpdateStrategy.Type)
	}
	if sts.Spec.UpdateStrategy.RollingUpdate != nil {
		t.Errorf("expected no rollingUpdate parameters, got %+v", *sts.Spec.UpdateStrategy.RollingUpdate)
	}
	if sts.Spec.PodManagementPolicy != appsv1.ParallelPodManagement {
		t.Errorf("podManagementPolicy = %q, want %q", sts.Spec.PodManagementPolicy, appsv1.ParallelPodManagement)
	}
	if sts.Spec.ServiceName != "kitchen-sink" {
		t.Errorf("serviceName = %q, want %q", sts.Spec.ServiceName, "kitchen-sink")
	}
	if len(sts.Spec.VolumeClaimTemplates) != 0 {
		t.Errorf("expected no volumeClaimTemplates, got %d", len(sts.Spec.VolumeClaimTemplates))
	}

	// Labels.
	expectedLabels := labelsForMemcached("kitchen-sink")
	for k, v := range expectedLabels {
		if sts.Labels[k] != v {
			t.Errorf("StatefulSet label %q = %q, want %q", k, sts.Labels[k], v)
		}
		if sts.Spec.Selector.MatchLabels[k] != v {
			t.Errorf("selector label %q = %q, want %q", k, sts.Spec.Selector.MatchLabels[k], v)
		}
		if sts.Spec.Template.Labels[k] != v {
			t.Errorf("template label %q = %q, want %q", k, sts.Spec.Template.Labels[k], v)
		}
	}
}

func TestConstructStatefulSet_ZeroReplicas(t *testing.T) {
	mc := &memcachedv1beta1.Memcached{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "zero-replicas",
			Namespace: "default",
		},
		Spec: memcachedv1beta1.MemcachedSpec{
			Replicas: int32Ptr(0),
		},
	}
	sts := &appsv1.StatefulSet{}

	constructStatefulSet(mc, sts, "", "")

	if sts.Spec.Replicas == nil {
		t.Fatal("expected non-nil Replicas")
	}
	if *sts.Spec.Replicas != 0 {
		t.Errorf("expected 0 replicas, got %d", *sts.Spec.Replicas)
	}

	// Verify the rest of the StatefulSet is still well-formed.
	if len(sts.Spec.Template.Spec.Containers) != 1 {
		t.Fatalf("expected 1 container, got %d", len(sts.Spec.Template.Spec.Containers))
	}
	if sts.Spec.Template.Spec.Containers[0].Image != testDefaultImage {
		t.Errorf("expected default image memcached:1.6, got %q", sts.Spec.Template.Spec.Containers[0].Image)
	}
	if sts.Spec.UpdateStrategy.Type != appsv1.RollingUpdateStatefulSetStrategyType {
		t.Errorf("expected RollingUpdate update strategy, got %q", sts.Spec.UpdateStrategy.Type)
	}
}

func TestConstructStatefulSet_Idempotent(t *testing.T) {
	mc := &memcachedv1beta1.Memcached{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "idempotent-test",
			Namespace: "default",
		},
		Spec: memcachedv1beta1.MemcachedSpec{
			Replicas: int32Ptr(3),
			Image:    stringPtr("memcached:1.6.29"),
			Resources: &corev1.ResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse(testCPU100m),
					corev1.ResourceMemory: resource.MustParse(testMem128Mi),
				},
			},
			Memcached: &memcachedv1beta1.MemcachedConfig{
				MaxMemoryMB:    128,
				MaxConnections: 512,
				Threads:        4,
				MaxItemSize:    "1m",
				Verbosity:      1,
				ExtraArgs:      []string{"-o", "modern"},
			},
			HighAvailability: &memcachedv1beta1.HighAvailabilitySpec{
				AntiAffinityPreset:        antiAffinityPresetPtr(memcachedv1beta1.AntiAffinityPresetSoft),
				TopologySpreadConstraints: []corev1.TopologySpreadConstraint{zoneSpreadConstraint()},
				GracefulShutdown: &memcachedv1beta1.GracefulShutdownSpec{
					Enabled:                       true,
					PreStopDelaySeconds:           10,
					TerminationGracePeriodSeconds: 30,
				},
			},
			Security: &memcachedv1beta1.SecuritySpec{
				SASL: &memcachedv1beta1.SASLSpec{
					Enabled: true,
					CredentialsSecretRef: corev1.LocalObjectReference{
						Name: "sasl-secret",
					},
				},
				TLS: &memcachedv1beta1.TLSSpec{
					Enabled: true,
					CertificateSecretRef: corev1.LocalObjectReference{
						Name: "tls-secret",
					},
				},
			},
			Monitoring: &memcachedv1beta1.MonitoringSpec{
				Enabled: true,
			},
		},
	}

	sts := &appsv1.StatefulSet{}

	// First call.
	constructStatefulSet(mc, sts, "", "")
	firstSpec := *sts.Spec.DeepCopy()

	// Second call on the same StatefulSet object.
	constructStatefulSet(mc, sts, "", "")

	// Verify full spec is identical after the second call.
	if !reflect.DeepEqual(firstSpec, sts.Spec) {
		t.Errorf("StatefulSet spec changed between calls:\nfirst:  %+v\nsecond: %+v", firstSpec, sts.Spec)
	}
}

func TestConstructStatefulSet_Annotations(t *testing.T) {
	tests := []struct {
		name               string
		secretHash         string
		restartTrigger     string
		wantAnnotations    map[string]string
		wantAnnotationsNil bool
	}{
		{
			name:       "non-empty secretHash sets annotation",
			secretHash: "abc123",
			wantAnnotations: map[string]string{
				AnnotationSecretHash: "abc123",
			},
		},
		{
			name:               "empty secretHash results in no annotation",
			secretHash:         "",
			wantAnnotationsNil: true,
		},
		{
			name:           "non-empty restartTrigger sets annotation",
			restartTrigger: "2024-01-15T10:00:00Z",
			wantAnnotations: map[string]string{
				AnnotationRestartTrigger: "2024-01-15T10:00:00Z",
			},
		},
		{
			name:               "empty restartTrigger results in no annotation",
			restartTrigger:     "",
			wantAnnotationsNil: true,
		},
		{
			name:           "both secretHash and restartTrigger set",
			secretHash:     "abc123",
			restartTrigger: "2024-01-15T10:00:00Z",
			wantAnnotations: map[string]string{
				AnnotationSecretHash:     "abc123",
				AnnotationRestartTrigger: "2024-01-15T10:00:00Z",
			},
		},
		{
			name:               "both empty results in nil annotations map",
			secretHash:         "",
			restartTrigger:     "",
			wantAnnotationsNil: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mc := &memcachedv1beta1.Memcached{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "my-cache",
					Namespace: "default",
				},
				Spec: memcachedv1beta1.MemcachedSpec{},
			}
			sts := &appsv1.StatefulSet{}

			constructStatefulSet(mc, sts, tt.secretHash, tt.restartTrigger)

			annotations := sts.Spec.Template.Annotations

			if tt.wantAnnotationsNil {
				if annotations != nil {
					t.Errorf("expected nil annotations, got %v", annotations)
				}
				return
			}

			if annotations == nil {
				t.Fatal("expected non-nil annotations, got nil")
				return
			}

			if len(annotations) != len(tt.wantAnnotations) {
				t.Errorf("expected %d annotations, got %d: %v", len(tt.wantAnnotations), len(annotations), annotations)
			}

			for key, wantVal := range tt.wantAnnotations {
				gotVal, ok := annotations[key]
				if !ok {
					t.Errorf("missing annotation %q", key)
					continue
				}
				if gotVal != wantVal {
					t.Errorf("annotation %q = %q, want %q", key, gotVal, wantVal)
				}
			}

			// Verify no unexpected annotation keys.
			for key := range annotations {
				if _, ok := tt.wantAnnotations[key]; !ok {
					t.Errorf("unexpected annotation %q = %q", key, annotations[key])
				}
			}
		})
	}
}

func TestAnnotationKeyConstants(t *testing.T) {
	if AnnotationSecretHash != "memcached.c5c3.io/secret-hash" {
		t.Errorf("AnnotationSecretHash = %q, want %q", AnnotationSecretHash, "memcached.c5c3.io/secret-hash")
	}
	if AnnotationRestartTrigger != "memcached.c5c3.io/restart-trigger" {
		t.Errorf("AnnotationRestartTrigger = %q, want %q", AnnotationRestartTrigger, "memcached.c5c3.io/restart-trigger")
	}
}

func TestConstructStatefulSet_HPAReplicas(t *testing.T) {
	tests := []struct {
		name         string
		mc           *memcachedv1beta1.Memcached
		current      *int32
		wantReplicas int32
	}{
		{
			name: "HPA disabled with replicas set",
			mc: &memcachedv1beta1.Memcached{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "my-cache",
					Namespace: "default",
				},
				Spec: memcachedv1beta1.MemcachedSpec{
					Replicas: int32Ptr(5),
				},
			},
			wantReplicas: 5,
		},
		{
			name: "HPA disabled with replicas nil uses default",
			mc: &memcachedv1beta1.Memcached{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "my-cache",
					Namespace: "default",
				},
				Spec: memcachedv1beta1.MemcachedSpec{},
			},
			wantReplicas: 1,
		},
		{
			name: "HPA enabled with replicas set ignores spec.replicas and seeds 1",
			mc: &memcachedv1beta1.Memcached{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "my-cache",
					Namespace: "default",
				},
				Spec: memcachedv1beta1.MemcachedSpec{
					Replicas: int32Ptr(5),
					Autoscaling: &memcachedv1beta1.AutoscalingSpec{
						Enabled:     true,
						MaxReplicas: 10,
					},
				},
			},
			wantReplicas: 1,
		},
		{
			name: "HPA enabled with replicas nil seeds 1",
			mc: &memcachedv1beta1.Memcached{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "my-cache",
					Namespace: "default",
				},
				Spec: memcachedv1beta1.MemcachedSpec{
					Autoscaling: &memcachedv1beta1.AutoscalingSpec{
						Enabled:     true,
						MaxReplicas: 10,
					},
				},
			},
			wantReplicas: 1,
		},
		{
			name: "HPA enabled seeds a new StatefulSet with minReplicas",
			mc: &memcachedv1beta1.Memcached{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "my-cache",
					Namespace: "default",
				},
				Spec: memcachedv1beta1.MemcachedSpec{
					Replicas: int32Ptr(5),
					Autoscaling: &memcachedv1beta1.AutoscalingSpec{
						Enabled:     true,
						MinReplicas: int32Ptr(2),
						MaxReplicas: 10,
					},
				},
			},
			wantReplicas: 2,
		},
		{
			name: "HPA enabled keeps the replica count of the existing StatefulSet",
			mc: &memcachedv1beta1.Memcached{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "my-cache",
					Namespace: "default",
				},
				Spec: memcachedv1beta1.MemcachedSpec{
					Autoscaling: &memcachedv1beta1.AutoscalingSpec{
						Enabled:     true,
						MinReplicas: int32Ptr(2),
						MaxReplicas: 10,
					},
				},
			},
			current:      int32Ptr(5),
			wantReplicas: 5,
		},
		{
			name: "HPA disabled restores spec.replicas on the existing StatefulSet",
			mc: &memcachedv1beta1.Memcached{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "my-cache",
					Namespace: "default",
				},
				Spec: memcachedv1beta1.MemcachedSpec{
					Replicas: int32Ptr(3),
				},
			},
			current:      int32Ptr(5),
			wantReplicas: 3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sts := &appsv1.StatefulSet{}
			sts.Spec.Replicas = tt.current
			constructStatefulSet(tt.mc, sts, "", "")

			gotReplicas := sts.Spec.Replicas
			if gotReplicas == nil {
				t.Fatalf("expected Replicas to be %d, got nil", tt.wantReplicas)
			}
			if *gotReplicas != tt.wantReplicas {
				t.Errorf("Replicas = %d, want %d", *gotReplicas, tt.wantReplicas)
			}
		})
	}
}

func TestStatefulSetName(t *testing.T) {
	name52 := strings.Repeat("a", 52)
	name53 := strings.Repeat("b", 53)
	name63 := strings.Repeat("c", 63)
	hexSuffix := regexp.MustCompile(`^[0-9a-f]{8}$`)

	tests := []struct {
		name      string
		input     string
		unchanged bool
	}{
		{name: "empty name is unchanged", input: "", unchanged: true},
		{name: "1-character name is unchanged", input: "a", unchanged: true},
		{name: "52-character name is unchanged", input: name52, unchanged: true},
		{name: "53-character name is shortened", input: name53},
		{name: "63-character name is shortened", input: name63},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := statefulSetName(tt.input)

			if tt.unchanged {
				if got != tt.input {
					t.Errorf("statefulSetName(%q) = %q, want the input unchanged", tt.input, got)
				}
				return
			}

			if len(got) != 52 {
				t.Errorf("len(statefulSetName(%d chars)) = %d, want 52", len(tt.input), len(got))
			}
			if !strings.HasPrefix(got, tt.input[:43]+"-") {
				t.Errorf("statefulSetName(%q) = %q, want prefix %q", tt.input, got, tt.input[:43]+"-")
			}
			suffix := strings.TrimPrefix(got, tt.input[:43]+"-")
			if !hexSuffix.MatchString(suffix) {
				t.Errorf("suffix %q does not match %s", suffix, hexSuffix)
			}
			sum := sha256.Sum256([]byte(tt.input))
			if want := tt.input[:43] + "-" + hex.EncodeToString(sum[:])[:8]; got != want {
				t.Errorf("statefulSetName(%q) = %q, want %q", tt.input, got, want)
			}
		})
	}
}

func TestStatefulSetName_DistinctForSharedPrefix(t *testing.T) {
	prefix := strings.Repeat("p", 43)
	first := prefix + strings.Repeat("x", 20)
	second := prefix + strings.Repeat("y", 20)
	if len(first) != 63 || len(second) != 63 {
		t.Fatalf("test names must have 63 characters, got %d and %d", len(first), len(second))
	}

	gotFirst := statefulSetName(first)
	gotSecond := statefulSetName(second)
	if gotFirst == gotSecond {
		t.Errorf("statefulSetName returned %q for two different names sharing their first 43 characters", gotFirst)
	}
}

func TestStatefulSetReplicas(t *testing.T) {
	autoscaling := func(minReplicas *int32) *memcachedv1beta1.AutoscalingSpec {
		return &memcachedv1beta1.AutoscalingSpec{
			Enabled:     true,
			MinReplicas: minReplicas,
			MaxReplicas: 10,
		}
	}

	tests := []struct {
		name    string
		spec    memcachedv1beta1.MemcachedSpec
		current *int32
		want    int32
	}{
		{
			name: "autoscaling off, spec.replicas nil returns 1",
			spec: memcachedv1beta1.MemcachedSpec{},
			want: 1,
		},
		{
			name: "autoscaling off, spec.replicas 0 returns 0",
			spec: memcachedv1beta1.MemcachedSpec{Replicas: int32Ptr(0)},
			want: 0,
		},
		{
			name:    "autoscaling off, spec.replicas 3 with current 5 returns 3",
			spec:    memcachedv1beta1.MemcachedSpec{Replicas: int32Ptr(3)},
			current: int32Ptr(5),
			want:    3,
		},
		{
			name: "autoscaling on, current nil and minReplicas nil returns 1",
			spec: memcachedv1beta1.MemcachedSpec{Autoscaling: autoscaling(nil)},
			want: 1,
		},
		{
			name: "autoscaling on, current nil and minReplicas 2 returns 2",
			spec: memcachedv1beta1.MemcachedSpec{Autoscaling: autoscaling(int32Ptr(2))},
			want: 2,
		},
		{
			name:    "autoscaling on, current 5 and minReplicas 2 returns 5",
			spec:    memcachedv1beta1.MemcachedSpec{Autoscaling: autoscaling(int32Ptr(2))},
			current: int32Ptr(5),
			want:    5,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mc := &memcachedv1beta1.Memcached{
				ObjectMeta: metav1.ObjectMeta{Name: "my-cache", Namespace: "default"},
				Spec:       tt.spec,
			}

			got := statefulSetReplicas(mc, tt.current)
			if got == nil {
				t.Fatalf("statefulSetReplicas returned nil, want %d", tt.want)
			}
			if *got != tt.want {
				t.Errorf("statefulSetReplicas = %d, want %d", *got, tt.want)
			}
		})
	}
}

func TestConstructStatefulSet_StatefulSetSpec(t *testing.T) {
	tests := []struct {
		name string
		mc   string
	}{
		{name: "short name", mc: "my-cache"},
		{name: "60-character name", mc: strings.Repeat("l", 60)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mc := &memcachedv1beta1.Memcached{
				ObjectMeta: metav1.ObjectMeta{Name: tt.mc, Namespace: "default"},
				Spec: memcachedv1beta1.MemcachedSpec{
					Replicas: int32Ptr(3),
				},
			}
			sts := &appsv1.StatefulSet{}

			constructStatefulSet(mc, sts, "", "")

			// The headless Service keeps the full Memcached name and governs the StatefulSet.
			if sts.Spec.ServiceName != tt.mc {
				t.Errorf("serviceName = %q, want %q", sts.Spec.ServiceName, tt.mc)
			}
			if sts.Spec.PodManagementPolicy != appsv1.ParallelPodManagement {
				t.Errorf("podManagementPolicy = %q, want %q", sts.Spec.PodManagementPolicy, appsv1.ParallelPodManagement)
			}
			if sts.Spec.UpdateStrategy.Type != appsv1.RollingUpdateStatefulSetStrategyType {
				t.Errorf("updateStrategy.type = %q, want %q", sts.Spec.UpdateStrategy.Type, appsv1.RollingUpdateStatefulSetStrategyType)
			}
			if sts.Spec.UpdateStrategy.RollingUpdate != nil {
				t.Errorf("expected no rollingUpdate parameters, got %+v", *sts.Spec.UpdateStrategy.RollingUpdate)
			}
			if sts.Spec.VolumeClaimTemplates != nil {
				t.Errorf("expected no volumeClaimTemplates, got %v", sts.Spec.VolumeClaimTemplates)
			}
			if sts.Spec.Selector == nil {
				t.Fatal("expected a selector")
			}
			if !reflect.DeepEqual(sts.Spec.Selector.MatchLabels, labelsForMemcached(tt.mc)) {
				t.Errorf("selector = %v, want %v", sts.Spec.Selector.MatchLabels, labelsForMemcached(tt.mc))
			}
			if sts.Spec.Replicas == nil || *sts.Spec.Replicas != 3 {
				t.Errorf("replicas = %v, want 3", sts.Spec.Replicas)
			}
			for k, v := range labelsForMemcached(tt.mc) {
				if sts.Spec.Template.Labels[k] != v {
					t.Errorf("template label %q = %q, want %q", k, sts.Spec.Template.Labels[k], v)
				}
			}
		})
	}
}
