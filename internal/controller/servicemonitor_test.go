// Package controller implements the reconciliation logic for the memcached-operator.
package controller

import (
	"reflect"
	"testing"

	monitoringv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	memcachedv1beta1 "github.com/c5c3/memcached-operator/api/v1beta1"
)

const testReleaseLabel = "prometheus"

func TestConstructServiceMonitor(t *testing.T) {
	tests := []struct {
		name              string
		monitoringSpec    *memcachedv1beta1.MonitoringSpec
		wantInterval      monitoringv1.Duration
		wantScrapeTimeout monitoringv1.Duration
		wantEndpointPort  string
	}{
		{
			name: "default interval and scrapeTimeout",
			monitoringSpec: &memcachedv1beta1.MonitoringSpec{
				Enabled: true,
			},
			wantInterval:      "30s",
			wantScrapeTimeout: "10s",
			wantEndpointPort:  "metrics",
		},
		{
			name: "custom interval",
			monitoringSpec: &memcachedv1beta1.MonitoringSpec{
				Enabled: true,
				ServiceMonitor: &memcachedv1beta1.ServiceMonitorSpec{
					Interval: "60s",
				},
			},
			wantInterval:      "60s",
			wantScrapeTimeout: "10s",
			wantEndpointPort:  "metrics",
		},
		{
			name: "custom scrapeTimeout",
			monitoringSpec: &memcachedv1beta1.MonitoringSpec{
				Enabled: true,
				ServiceMonitor: &memcachedv1beta1.ServiceMonitorSpec{
					ScrapeTimeout: "20s",
				},
			},
			wantInterval:      "30s",
			wantScrapeTimeout: "20s",
			wantEndpointPort:  "metrics",
		},
		{
			name: "custom interval and scrapeTimeout",
			monitoringSpec: &memcachedv1beta1.MonitoringSpec{
				Enabled: true,
				ServiceMonitor: &memcachedv1beta1.ServiceMonitorSpec{
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
			mc := &memcachedv1beta1.Memcached{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "my-cache",
					Namespace: "default",
				},
				Spec: memcachedv1beta1.MemcachedSpec{
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
	mc := &memcachedv1beta1.Memcached{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "label-test",
			Namespace: "default",
		},
		Spec: memcachedv1beta1.MemcachedSpec{
			Monitoring: &memcachedv1beta1.MonitoringSpec{
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
	mc := &memcachedv1beta1.Memcached{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "addl-labels",
			Namespace: "default",
		},
		Spec: memcachedv1beta1.MemcachedSpec{
			Monitoring: &memcachedv1beta1.MonitoringSpec{
				Enabled: true,
				ServiceMonitor: &memcachedv1beta1.ServiceMonitorSpec{
					AdditionalLabels: map[string]string{
						"release": testReleaseLabel,
						"team":    "platform",
					},
				},
			},
		},
	}
	sm := &monitoringv1.ServiceMonitor{}

	constructServiceMonitor(mc, sm)

	// Should have base labels + additional labels on metadata.
	if sm.Labels["release"] != testReleaseLabel {
		t.Errorf("expected additional label release=prometheus, got %q", sm.Labels["release"])
	}
	if sm.Labels["team"] != "platform" {
		t.Errorf("expected additional label team=platform, got %q", sm.Labels["team"])
	}
	// Base labels still present.
	if sm.Labels["app.kubernetes.io/name"] != testPortName {
		t.Errorf("expected base label app.kubernetes.io/name=memcached, got %q", sm.Labels["app.kubernetes.io/name"])
	}

	// Selector should NOT have additional labels (only base labels for matching).
	if _, exists := sm.Spec.Selector.MatchLabels["release"]; exists {
		t.Error("selector should not contain additional labels")
	}
}

func TestConstructServiceMonitor_AdditionalLabelsConflict(t *testing.T) {
	mc := &memcachedv1beta1.Memcached{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "conflict-labels",
			Namespace: "default",
		},
		Spec: memcachedv1beta1.MemcachedSpec{
			Monitoring: &memcachedv1beta1.MonitoringSpec{
				Enabled: true,
				ServiceMonitor: &memcachedv1beta1.ServiceMonitorSpec{
					AdditionalLabels: map[string]string{
						"app.kubernetes.io/name": "override",
						"release":                testReleaseLabel,
					},
				},
			},
		},
	}
	sm := &monitoringv1.ServiceMonitor{}

	constructServiceMonitor(mc, sm)

	// Standard label must win over the additionalLabels override attempt.
	if sm.Labels["app.kubernetes.io/name"] != testPortName {
		t.Errorf("standard label app.kubernetes.io/name = %q, want %q (standard must take precedence)", sm.Labels["app.kubernetes.io/name"], "memcached")
	}
	if sm.Labels["app.kubernetes.io/instance"] != "conflict-labels" {
		t.Errorf("standard label app.kubernetes.io/instance = %q, want %q", sm.Labels["app.kubernetes.io/instance"], "conflict-labels")
	}
	if sm.Labels["app.kubernetes.io/managed-by"] != "memcached-operator" {
		t.Errorf("standard label app.kubernetes.io/managed-by = %q, want %q", sm.Labels["app.kubernetes.io/managed-by"], "memcached-operator")
	}
	// Non-conflicting additional label should still be present.
	if sm.Labels["release"] != testReleaseLabel {
		t.Errorf("additional label release = %q, want %q", sm.Labels["release"], testReleaseLabel)
	}
}

func TestConstructServiceMonitor_NamespaceSelector(t *testing.T) {
	mc := &memcachedv1beta1.Memcached{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "ns-test",
			Namespace: "production",
		},
		Spec: memcachedv1beta1.MemcachedSpec{
			Monitoring: &memcachedv1beta1.MonitoringSpec{
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
			mc := &memcachedv1beta1.Memcached{
				ObjectMeta: metav1.ObjectMeta{
					Name:      tt.instanceName,
					Namespace: "default",
				},
				Spec: memcachedv1beta1.MemcachedSpec{
					Monitoring: &memcachedv1beta1.MonitoringSpec{
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
		mc   *memcachedv1beta1.Memcached
		want bool
	}{
		{
			name: "nil Monitoring",
			mc: &memcachedv1beta1.Memcached{
				Spec: memcachedv1beta1.MemcachedSpec{Monitoring: nil},
			},
			want: false,
		},
		{
			name: "monitoring disabled",
			mc: &memcachedv1beta1.Memcached{
				Spec: memcachedv1beta1.MemcachedSpec{
					Monitoring: &memcachedv1beta1.MonitoringSpec{Enabled: false},
				},
			},
			want: false,
		},
		{
			name: "monitoring enabled but serviceMonitor nil",
			mc: &memcachedv1beta1.Memcached{
				Spec: memcachedv1beta1.MemcachedSpec{
					Monitoring: &memcachedv1beta1.MonitoringSpec{Enabled: true},
				},
			},
			want: false,
		},
		{
			name: "monitoring enabled with ServiceMonitor spec",
			mc: &memcachedv1beta1.Memcached{
				Spec: memcachedv1beta1.MemcachedSpec{
					Monitoring: &memcachedv1beta1.MonitoringSpec{
						Enabled: true,
						ServiceMonitor: &memcachedv1beta1.ServiceMonitorSpec{
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

func TestConstructServiceMonitor_Idempotent(t *testing.T) {
	releaseLabel := testReleaseLabel
	mc := &memcachedv1beta1.Memcached{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "idem-test",
			Namespace: "monitoring",
		},
		Spec: memcachedv1beta1.MemcachedSpec{
			Monitoring: &memcachedv1beta1.MonitoringSpec{
				Enabled: true,
				ServiceMonitor: &memcachedv1beta1.ServiceMonitorSpec{
					AdditionalLabels: map[string]string{
						"release": releaseLabel,
						"team":    "platform",
					},
					Interval:      "45s",
					ScrapeTimeout: "15s",
				},
			},
		},
	}

	sm := &monitoringv1.ServiceMonitor{}

	// First call.
	constructServiceMonitor(mc, sm)

	// Capture the state after the first call.
	labelsAfterFirst := make(map[string]string, len(sm.Labels))
	for k, v := range sm.Labels {
		labelsAfterFirst[k] = v
	}
	endpointsAfterFirst := make([]monitoringv1.Endpoint, len(sm.Spec.Endpoints))
	copy(endpointsAfterFirst, sm.Spec.Endpoints)
	selectorAfterFirst := sm.Spec.Selector.DeepCopy()
	nsSelectorAfterFirst := sm.Spec.NamespaceSelector.DeepCopy()

	// Second call on the same object.
	constructServiceMonitor(mc, sm)

	// Verify labels are identical.
	if !reflect.DeepEqual(sm.Labels, labelsAfterFirst) {
		t.Errorf("labels after second call = %v, want %v", sm.Labels, labelsAfterFirst)
	}

	// Verify endpoints are identical.
	if !reflect.DeepEqual(sm.Spec.Endpoints, endpointsAfterFirst) {
		t.Errorf("endpoints after second call = %v, want %v", sm.Spec.Endpoints, endpointsAfterFirst)
	}

	// Verify selector is identical.
	if !reflect.DeepEqual(sm.Spec.Selector, *selectorAfterFirst) {
		t.Errorf("selector after second call = %v, want %v", sm.Spec.Selector, *selectorAfterFirst)
	}

	// Verify namespaceSelector is identical.
	if !reflect.DeepEqual(sm.Spec.NamespaceSelector, *nsSelectorAfterFirst) {
		t.Errorf("namespaceSelector after second call = %v, want %v", sm.Spec.NamespaceSelector, *nsSelectorAfterFirst)
	}

	// Sanity check: verify the values are what we expect (not just that two calls match).
	if sm.Labels["release"] != releaseLabel {
		t.Errorf("expected additional label release=%s, got %q", releaseLabel, sm.Labels["release"])
	}
	if sm.Labels["team"] != "platform" {
		t.Errorf("expected additional label team=platform, got %q", sm.Labels["team"])
	}
	if sm.Labels["app.kubernetes.io/name"] != testPortName {
		t.Errorf("expected standard label app.kubernetes.io/name=memcached, got %q", sm.Labels["app.kubernetes.io/name"])
	}
	if len(sm.Spec.Endpoints) != 1 {
		t.Fatalf("expected 1 endpoint, got %d", len(sm.Spec.Endpoints))
	}
	ep := sm.Spec.Endpoints[0]
	if ep.Interval != "45s" {
		t.Errorf("interval = %q, want %q", ep.Interval, "45s")
	}
	if ep.ScrapeTimeout != "15s" {
		t.Errorf("scrapeTimeout = %q, want %q", ep.ScrapeTimeout, "15s")
	}
	if ep.Port != "metrics" {
		t.Errorf("endpoint port = %q, want %q", ep.Port, "metrics")
	}
	if len(sm.Spec.NamespaceSelector.MatchNames) != 1 || sm.Spec.NamespaceSelector.MatchNames[0] != "monitoring" {
		t.Errorf("namespaceSelector = %v, want [monitoring]", sm.Spec.NamespaceSelector.MatchNames)
	}
}
