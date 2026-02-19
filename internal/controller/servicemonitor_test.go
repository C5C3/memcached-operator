// Package controller implements the reconciliation logic for the memcached-operator.
package controller

import (
	"testing"

	monitoringv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	memcachedv1alpha1 "github.com/c5c3/memcached-operator/api/v1alpha1"
)

func TestConstructServiceMonitor(t *testing.T) {
	tests := []struct {
		name              string
		monitoringSpec    *memcachedv1alpha1.MonitoringSpec
		wantInterval      monitoringv1.Duration
		wantScrapeTimeout monitoringv1.Duration
		wantEndpointPort  string
	}{
		{
			name: "default interval and scrapeTimeout",
			monitoringSpec: &memcachedv1alpha1.MonitoringSpec{
				Enabled: true,
			},
			wantInterval:      "30s",
			wantScrapeTimeout: "10s",
			wantEndpointPort:  "metrics",
		},
		{
			name: "custom interval",
			monitoringSpec: &memcachedv1alpha1.MonitoringSpec{
				Enabled: true,
				ServiceMonitor: &memcachedv1alpha1.ServiceMonitorSpec{
					Interval: "60s",
				},
			},
			wantInterval:      "60s",
			wantScrapeTimeout: "10s",
			wantEndpointPort:  "metrics",
		},
		{
			name: "custom scrapeTimeout",
			monitoringSpec: &memcachedv1alpha1.MonitoringSpec{
				Enabled: true,
				ServiceMonitor: &memcachedv1alpha1.ServiceMonitorSpec{
					ScrapeTimeout: "20s",
				},
			},
			wantInterval:      "30s",
			wantScrapeTimeout: "20s",
			wantEndpointPort:  "metrics",
		},
		{
			name: "custom interval and scrapeTimeout",
			monitoringSpec: &memcachedv1alpha1.MonitoringSpec{
				Enabled: true,
				ServiceMonitor: &memcachedv1alpha1.ServiceMonitorSpec{
					Interval:      "15s",
					ScrapeTimeout: "5s",
				},
			},
			wantInterval:      "15s",
			wantScrapeTimeout: "5s",
			wantEndpointPort:  "metrics",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mc := &memcachedv1alpha1.Memcached{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "my-cache",
					Namespace: "default",
				},
				Spec: memcachedv1alpha1.MemcachedSpec{
					Monitoring: tt.monitoringSpec,
				},
			}
			sm := &monitoringv1.ServiceMonitor{}

			constructServiceMonitor(mc, sm)

			if len(sm.Spec.Endpoints) != 1 {
				t.Fatalf("expected 1 endpoint, got %d", len(sm.Spec.Endpoints))
			}

			ep := sm.Spec.Endpoints[0]
			if ep.Port != tt.wantEndpointPort {
				t.Errorf("endpoint port = %q, want %q", ep.Port, tt.wantEndpointPort)
			}
			if ep.Interval != tt.wantInterval {
				t.Errorf("interval = %q, want %q", ep.Interval, tt.wantInterval)
			}
			if ep.ScrapeTimeout != tt.wantScrapeTimeout {
				t.Errorf("scrapeTimeout = %q, want %q", ep.ScrapeTimeout, tt.wantScrapeTimeout)
			}
		})
	}
}

func TestConstructServiceMonitor_Labels(t *testing.T) {
	mc := &memcachedv1alpha1.Memcached{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "label-test",
			Namespace: "default",
		},
		Spec: memcachedv1alpha1.MemcachedSpec{
			Monitoring: &memcachedv1alpha1.MonitoringSpec{
				Enabled: true,
			},
		},
	}
	sm := &monitoringv1.ServiceMonitor{}

	constructServiceMonitor(mc, sm)

	expectedLabels := labelsForMemcached("label-test")

	// Metadata labels.
	if len(sm.Labels) != len(expectedLabels) {
		t.Errorf("expected %d metadata labels, got %d", len(expectedLabels), len(sm.Labels))
	}
	for k, v := range expectedLabels {
		if sm.Labels[k] != v {
			t.Errorf("label %q = %q, want %q", k, sm.Labels[k], v)
		}
	}

	// Selector labels — should use base labels (no additional labels).
	baseLabels := labelsForMemcached("label-test")
	if len(sm.Spec.Selector.MatchLabels) != len(baseLabels) {
		t.Errorf("expected %d selector labels, got %d", len(baseLabels), len(sm.Spec.Selector.MatchLabels))
	}
	for k, v := range baseLabels {
		if sm.Spec.Selector.MatchLabels[k] != v {
			t.Errorf("selector %q = %q, want %q", k, sm.Spec.Selector.MatchLabels[k], v)
		}
	}
}

func TestConstructServiceMonitor_AdditionalLabels(t *testing.T) {
	mc := &memcachedv1alpha1.Memcached{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "addl-labels",
			Namespace: "default",
		},
		Spec: memcachedv1alpha1.MemcachedSpec{
			Monitoring: &memcachedv1alpha1.MonitoringSpec{
				Enabled: true,
				ServiceMonitor: &memcachedv1alpha1.ServiceMonitorSpec{
					AdditionalLabels: map[string]string{
						"release": "prometheus",
						"team":    "platform",
					},
				},
			},
		},
	}
	sm := &monitoringv1.ServiceMonitor{}

	constructServiceMonitor(mc, sm)

	// Should have base labels + additional labels on metadata.
	if sm.Labels["release"] != "prometheus" {
		t.Errorf("expected additional label release=prometheus, got %q", sm.Labels["release"])
	}
	if sm.Labels["team"] != "platform" {
		t.Errorf("expected additional label team=platform, got %q", sm.Labels["team"])
	}
	// Base labels still present.
	if sm.Labels["app.kubernetes.io/name"] != "memcached" {
		t.Errorf("expected base label app.kubernetes.io/name=memcached, got %q", sm.Labels["app.kubernetes.io/name"])
	}

	// Selector should NOT have additional labels (only base labels for matching).
	if _, exists := sm.Spec.Selector.MatchLabels["release"]; exists {
		t.Error("selector should not contain additional labels")
	}
}

func TestConstructServiceMonitor_AdditionalLabelsConflict(t *testing.T) {
	mc := &memcachedv1alpha1.Memcached{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "conflict-labels",
			Namespace: "default",
		},
		Spec: memcachedv1alpha1.MemcachedSpec{
			Monitoring: &memcachedv1alpha1.MonitoringSpec{
				Enabled: true,
				ServiceMonitor: &memcachedv1alpha1.ServiceMonitorSpec{
					AdditionalLabels: map[string]string{
						"app.kubernetes.io/name": "override",
						"release":                "prometheus",
					},
				},
			},
		},
	}
	sm := &monitoringv1.ServiceMonitor{}

	constructServiceMonitor(mc, sm)

	// Standard label must win over the additionalLabels override attempt.
	if sm.Labels["app.kubernetes.io/name"] != "memcached" {
		t.Errorf("standard label app.kubernetes.io/name = %q, want %q (standard must take precedence)", sm.Labels["app.kubernetes.io/name"], "memcached")
	}
	if sm.Labels["app.kubernetes.io/instance"] != "conflict-labels" {
		t.Errorf("standard label app.kubernetes.io/instance = %q, want %q", sm.Labels["app.kubernetes.io/instance"], "conflict-labels")
	}
	if sm.Labels["app.kubernetes.io/managed-by"] != "memcached-operator" {
		t.Errorf("standard label app.kubernetes.io/managed-by = %q, want %q", sm.Labels["app.kubernetes.io/managed-by"], "memcached-operator")
	}
	// Non-conflicting additional label should still be present.
	if sm.Labels["release"] != "prometheus" {
		t.Errorf("additional label release = %q, want %q", sm.Labels["release"], "prometheus")
	}
}

func TestConstructServiceMonitor_NamespaceSelector(t *testing.T) {
	mc := &memcachedv1alpha1.Memcached{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "ns-test",
			Namespace: "production",
		},
		Spec: memcachedv1alpha1.MemcachedSpec{
			Monitoring: &memcachedv1alpha1.MonitoringSpec{
				Enabled: true,
			},
		},
	}
	sm := &monitoringv1.ServiceMonitor{}

	constructServiceMonitor(mc, sm)

	if len(sm.Spec.NamespaceSelector.MatchNames) != 1 {
		t.Fatalf("expected 1 namespace in selector, got %d", len(sm.Spec.NamespaceSelector.MatchNames))
	}
	if sm.Spec.NamespaceSelector.MatchNames[0] != "production" {
		t.Errorf("namespace selector = %q, want %q", sm.Spec.NamespaceSelector.MatchNames[0], "production")
	}
}

func TestConstructServiceMonitor_InstanceScopedSelector(t *testing.T) {
	tests := []struct {
		name         string
		instanceName string
	}{
		{name: "cache-alpha", instanceName: "cache-alpha"},
		{name: "cache-beta", instanceName: "cache-beta"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mc := &memcachedv1alpha1.Memcached{
				ObjectMeta: metav1.ObjectMeta{
					Name:      tt.instanceName,
					Namespace: "default",
				},
				Spec: memcachedv1alpha1.MemcachedSpec{
					Monitoring: &memcachedv1alpha1.MonitoringSpec{
						Enabled: true,
					},
				},
			}
			sm := &monitoringv1.ServiceMonitor{}

			constructServiceMonitor(mc, sm)

			got := sm.Spec.Selector.MatchLabels["app.kubernetes.io/instance"]
			if got != tt.instanceName {
				t.Errorf("selector app.kubernetes.io/instance = %q, want %q", got, tt.instanceName)
			}
		})
	}
}

func TestServiceMonitorEnabled(t *testing.T) {
	tests := []struct {
		name string
		mc   *memcachedv1alpha1.Memcached
		want bool
	}{
		{
			name: "nil Monitoring",
			mc: &memcachedv1alpha1.Memcached{
				Spec: memcachedv1alpha1.MemcachedSpec{Monitoring: nil},
			},
			want: false,
		},
		{
			name: "monitoring disabled",
			mc: &memcachedv1alpha1.Memcached{
				Spec: memcachedv1alpha1.MemcachedSpec{
					Monitoring: &memcachedv1alpha1.MonitoringSpec{Enabled: false},
				},
			},
			want: false,
		},
		{
			name: "monitoring enabled but serviceMonitor nil",
			mc: &memcachedv1alpha1.Memcached{
				Spec: memcachedv1alpha1.MemcachedSpec{
					Monitoring: &memcachedv1alpha1.MonitoringSpec{Enabled: true},
				},
			},
			want: false,
		},
		{
			name: "monitoring enabled with ServiceMonitor spec",
			mc: &memcachedv1alpha1.Memcached{
				Spec: memcachedv1alpha1.MemcachedSpec{
					Monitoring: &memcachedv1alpha1.MonitoringSpec{
						Enabled: true,
						ServiceMonitor: &memcachedv1alpha1.ServiceMonitorSpec{
							Interval: "60s",
						},
					},
				},
			},
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := serviceMonitorEnabled(tt.mc)
			if got != tt.want {
				t.Errorf("serviceMonitorEnabled() = %v, want %v", got, tt.want)
			}
		})
	}
}
