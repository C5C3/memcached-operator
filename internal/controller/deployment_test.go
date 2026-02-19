// Package controller implements the reconciliation logic for the memcached-operator.
package controller

import (
	"strings"
	"testing"

	memcachedv1alpha1 "github.com/c5c3/memcached-operator/api/v1alpha1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
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
	if containers[0].Name != "memcached" {
		t.Errorf("expected container name 'memcached', got %q", containers[0].Name)
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
	if port.Name != "memcached" || port.ContainerPort != 11211 || port.Protocol != corev1.ProtocolTCP {
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

	if ports[0].Name != "memcached" {
		t.Errorf("expected port name 'memcached', got %q", ports[0].Name)
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
	if lp.ProbeHandler.TCPSocket == nil {
		t.Fatal("expected tcpSocket liveness probe")
	}
	if lp.TCPSocket.Port != intstr.FromString("memcached") {
		t.Errorf("liveness probe port = %v, want 'memcached'", lp.TCPSocket.Port)
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
	if rp.ProbeHandler.TCPSocket == nil {
		t.Fatal("expected tcpSocket readiness probe")
	}
	if rp.TCPSocket.Port != intstr.FromString("memcached") {
		t.Errorf("readiness probe port = %v, want 'memcached'", rp.TCPSocket.Port)
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
