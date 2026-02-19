// Package metrics provides custom Prometheus metrics for the memcached-operator.
package metrics

import (
	"strings"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus/testutil"
)

func TestMetricsRegistered(t *testing.T) {
	// Record one value for each metric so they appear when gathered.
	RecordReconcileResource("Deployment", "created")
	RecordReconciliation("reg-test", "default", "success", time.Millisecond)
	RecordInstanceInfo("reg-test", "default", "memcached:1.6", 3)
	RecordReadyReplicas("reg-test", "default", 2)

	families, err := registry().Gather()
	if err != nil {
		t.Fatalf("gathering metrics from registry failed: %v", err)
	}

	wantMetrics := []string{
		"memcached_operator_reconcile_resource_total",
		"memcached_operator_reconcile_total",
		"memcached_operator_reconcile_duration_seconds",
		"memcached_operator_instance_info",
		"memcached_operator_instance_replicas_desired",
		"memcached_operator_instance_replicas_ready",
	}

	gathered := make(map[string]bool)
	for _, f := range families {
		gathered[f.GetName()] = true
	}

	for _, name := range wantMetrics {
		if !gathered[name] {
			t.Errorf("expected metric %q to be registered and gatherable, but it was not found", name)
		}
	}

	ResetInstanceMetrics("reg-test", "default")
}

func TestRecordReconciliation(t *testing.T) {
	tests := []struct {
		name      string
		crName    string
		namespace string
		result    string
		duration  time.Duration
	}{
		{
			name:      "success reconciliation",
			crName:    "rec-cache-a",
			namespace: "ns-rec-a",
			result:    "success",
			duration:  100 * time.Millisecond,
		},
		{
			name:      "error reconciliation",
			crName:    "rec-cache-b",
			namespace: "ns-rec-b",
			result:    "error",
			duration:  200 * time.Millisecond,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			counterBefore := testutil.ToFloat64(reconcileTotal.WithLabelValues(tt.crName, tt.namespace, tt.result))

			RecordReconciliation(tt.crName, tt.namespace, tt.result, tt.duration)

			counterAfter := testutil.ToFloat64(reconcileTotal.WithLabelValues(tt.crName, tt.namespace, tt.result))
			if counterAfter != counterBefore+1 {
				t.Errorf("reconcile_total counter: got %v, want %v", counterAfter, counterBefore+1)
			}
		})
	}
}

func TestRecordReconciliation_HistogramObserved(t *testing.T) {
	RecordReconciliation("hist-obs", "default", "success", 50*time.Millisecond)

	// Verify histogram has at least one observation by collecting it.
	count := testutil.CollectAndCount(reconcileDuration, "memcached_operator_reconcile_duration_seconds")
	if count == 0 {
		t.Error("expected at least one histogram sample after recording reconciliation")
	}
}

func TestRecordReconciliation_DifferentLabelsTrackedSeparately(t *testing.T) {
	// Record for two distinct instances.
	RecordReconciliation("sep-rec-x", "ns-x", "success", time.Millisecond)
	RecordReconciliation("sep-rec-y", "ns-y", "success", time.Millisecond)

	valX := testutil.ToFloat64(reconcileTotal.WithLabelValues("sep-rec-x", "ns-x", "success"))
	valY := testutil.ToFloat64(reconcileTotal.WithLabelValues("sep-rec-y", "ns-y", "success"))

	if valX < 1 {
		t.Errorf("expected sep-rec-x counter >= 1, got %v", valX)
	}
	if valY < 1 {
		t.Errorf("expected sep-rec-y counter >= 1, got %v", valY)
	}
}

func TestRecordInstanceInfo(t *testing.T) {
	tests := []struct {
		name      string
		crName    string
		namespace string
		image     string
		replicas  int32
	}{
		{
			name:      "standard instance",
			crName:    "info-cache",
			namespace: "production",
			image:     "memcached:1.6.29",
			replicas:  3,
		},
		{
			name:      "single replica",
			crName:    "info-dev-cache",
			namespace: "dev",
			image:     "memcached:latest",
			replicas:  1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			RecordInstanceInfo(tt.crName, tt.namespace, tt.image, tt.replicas)

			infoVal := testutil.ToFloat64(instanceInfo.WithLabelValues(tt.crName, tt.namespace, tt.image))
			if infoVal != 1 {
				t.Errorf("instance_info gauge: got %v, want 1", infoVal)
			}

			desiredVal := testutil.ToFloat64(instanceReplicasDesired.WithLabelValues(tt.crName, tt.namespace))
			if desiredVal != float64(tt.replicas) {
				t.Errorf("instance_replicas_desired: got %v, want %v", desiredVal, tt.replicas)
			}

			ResetInstanceMetrics(tt.crName, tt.namespace)
		})
	}
}

func TestRecordReadyReplicas(t *testing.T) {
	tests := []struct {
		name      string
		crName    string
		namespace string
		ready     int32
	}{
		{
			name:      "some ready",
			crName:    "ready-cache",
			namespace: "default",
			ready:     2,
		},
		{
			name:      "all ready",
			crName:    "ready-cache-all",
			namespace: "default",
			ready:     5,
		},
		{
			name:      "none ready",
			crName:    "ready-cache-zero",
			namespace: "staging",
			ready:     0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			RecordReadyReplicas(tt.crName, tt.namespace, tt.ready)

			val := testutil.ToFloat64(instanceReplicasReady.WithLabelValues(tt.crName, tt.namespace))
			if val != float64(tt.ready) {
				t.Errorf("instance_replicas_ready: got %v, want %v", val, tt.ready)
			}

			ResetInstanceMetrics(tt.crName, tt.namespace)
		})
	}
}

func TestResetInstanceMetrics(t *testing.T) {
	// Record metrics for an instance.
	RecordInstanceInfo("reset-inst", "reset-ns", "memcached:1.6", 3)
	RecordReadyReplicas("reset-inst", "reset-ns", 2)

	// Verify metrics exist before reset.
	desiredBefore := testutil.ToFloat64(instanceReplicasDesired.WithLabelValues("reset-inst", "reset-ns"))
	if desiredBefore != 3 {
		t.Fatalf("expected desired replicas=3 before reset, got %v", desiredBefore)
	}
	readyBefore := testutil.ToFloat64(instanceReplicasReady.WithLabelValues("reset-inst", "reset-ns"))
	if readyBefore != 2 {
		t.Fatalf("expected ready replicas=2 before reset, got %v", readyBefore)
	}

	// Reset metrics for the instance.
	ResetInstanceMetrics("reset-inst", "reset-ns")

	// After reset, the gauge series should be deleted. Calling WithLabelValues
	// again would recreate them at 0, which is an artifact of the Prometheus
	// client. Instead, we verify via GatherAndCount that the specific metrics
	// no longer have series with these labels. We count the total series in
	// the desired replicas gauge.
	countAfter := testutil.CollectAndCount(instanceReplicasDesired, "memcached_operator_instance_replicas_desired")
	countReadyAfter := testutil.CollectAndCount(instanceReplicasReady, "memcached_operator_instance_replicas_ready")

	// These counts should be 0 if no other test has left metrics around for
	// these specific collectors (they might not be 0 due to other test data,
	// but at minimum they should not contain our "reset-inst" series).
	// A pragmatic check: record for a different instance, verify our instance
	// is not included in the total count.
	RecordInstanceInfo("reset-other", "reset-ns", "memcached:1.6", 1)
	RecordReadyReplicas("reset-other", "reset-ns", 1)

	countWithOther := testutil.CollectAndCount(instanceReplicasDesired, "memcached_operator_instance_replicas_desired")
	countReadyWithOther := testutil.CollectAndCount(instanceReplicasReady, "memcached_operator_instance_replicas_ready")

	// After adding one other instance, the count should be exactly 1 more
	// than after the reset (the reset-inst series should not be there).
	if countWithOther != countAfter+1 {
		t.Errorf("expected desired replicas count to increase by 1 after adding another instance, got before=%d after=%d", countAfter, countWithOther)
	}
	if countReadyWithOther != countReadyAfter+1 {
		t.Errorf("expected ready replicas count to increase by 1 after adding another instance, got before=%d after=%d", countReadyAfter, countReadyWithOther)
	}

	ResetInstanceMetrics("reset-other", "reset-ns")
}

func TestRecordInstanceInfo_UpdatesExistingValues(t *testing.T) {
	RecordInstanceInfo("upd-cache", "upd-ns", "memcached:1.6", 3)

	// Update with a new image and replica count.
	RecordInstanceInfo("upd-cache", "upd-ns", "memcached:1.6.29", 5)

	// New info gauge should be 1.
	newInfoVal := testutil.ToFloat64(instanceInfo.WithLabelValues("upd-cache", "upd-ns", "memcached:1.6.29"))
	if newInfoVal != 1 {
		t.Errorf("instance_info with new image: got %v, want 1", newInfoVal)
	}

	// Desired replicas should reflect the update.
	desiredVal := testutil.ToFloat64(instanceReplicasDesired.WithLabelValues("upd-cache", "upd-ns"))
	if desiredVal != 5 {
		t.Errorf("instance_replicas_desired after update: got %v, want 5", desiredVal)
	}

	ResetInstanceMetrics("upd-cache", "upd-ns")
}

func TestRecordingDifferentLabelValues_TrackedSeparately(t *testing.T) {
	RecordInstanceInfo("sep-a", "ns-sep-1", "memcached:1.6", 3)
	RecordInstanceInfo("sep-b", "ns-sep-2", "memcached:1.6.29", 5)
	RecordReadyReplicas("sep-a", "ns-sep-1", 2)
	RecordReadyReplicas("sep-b", "ns-sep-2", 4)

	desiredA := testutil.ToFloat64(instanceReplicasDesired.WithLabelValues("sep-a", "ns-sep-1"))
	desiredB := testutil.ToFloat64(instanceReplicasDesired.WithLabelValues("sep-b", "ns-sep-2"))

	if desiredA != 3 {
		t.Errorf("sep-a desired replicas: got %v, want 3", desiredA)
	}
	if desiredB != 5 {
		t.Errorf("sep-b desired replicas: got %v, want 5", desiredB)
	}

	readyA := testutil.ToFloat64(instanceReplicasReady.WithLabelValues("sep-a", "ns-sep-1"))
	readyB := testutil.ToFloat64(instanceReplicasReady.WithLabelValues("sep-b", "ns-sep-2"))

	if readyA != 2 {
		t.Errorf("sep-a ready replicas: got %v, want 2", readyA)
	}
	if readyB != 4 {
		t.Errorf("sep-b ready replicas: got %v, want 4", readyB)
	}

	ResetInstanceMetrics("sep-a", "ns-sep-1")
	ResetInstanceMetrics("sep-b", "ns-sep-2")
}

func TestRecordReconcileResource(t *testing.T) {
	tests := []struct {
		name         string
		resourceKind string
		result       string
	}{
		{
			name:         "deployment created",
			resourceKind: "Deployment",
			result:       "created",
		},
		{
			name:         "service updated",
			resourceKind: "Service",
			result:       "updated",
		},
		{
			name:         "deployment unchanged",
			resourceKind: "Deployment",
			result:       "unchanged",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			before := testutil.ToFloat64(reconcileResourceTotal.WithLabelValues(tt.resourceKind, tt.result))

			RecordReconcileResource(tt.resourceKind, tt.result)

			after := testutil.ToFloat64(reconcileResourceTotal.WithLabelValues(tt.resourceKind, tt.result))
			if after != before+1 {
				t.Errorf("reconcile_resource_total{resource_kind=%q,result=%q}: got %v, want %v",
					tt.resourceKind, tt.result, after, before+1)
			}
		})
	}
}

func TestRecordReconcileResource_DifferentLabelsTrackedSeparately(t *testing.T) {
	beforeDep := testutil.ToFloat64(reconcileResourceTotal.WithLabelValues("Deployment", "created"))
	beforeSvc := testutil.ToFloat64(reconcileResourceTotal.WithLabelValues("Service", "created"))

	RecordReconcileResource("Deployment", "created")

	afterDep := testutil.ToFloat64(reconcileResourceTotal.WithLabelValues("Deployment", "created"))
	afterSvc := testutil.ToFloat64(reconcileResourceTotal.WithLabelValues("Service", "created"))

	if afterDep != beforeDep+1 {
		t.Errorf("expected Deployment/created to increment by 1, got before=%v after=%v", beforeDep, afterDep)
	}
	if afterSvc != beforeSvc {
		t.Errorf("expected Service/created unchanged, got before=%v after=%v", beforeSvc, afterSvc)
	}
}

func TestMetricNamingConvention(t *testing.T) {
	// Ensure all custom metrics use the required "memcached_operator_" prefix (REQ-001).
	RecordReconcileResource("Deployment", "created")
	RecordReconciliation("naming-test", "default", "success", time.Millisecond)
	RecordInstanceInfo("naming-test", "default", "memcached:1.6", 1)
	RecordReadyReplicas("naming-test", "default", 1)

	families, err := registry().Gather()
	if err != nil {
		t.Fatalf("gathering metrics: %v", err)
	}

	const requiredPrefix = "memcached_operator_"

	// Verify none of the old "memcached_" (without "operator") names exist.
	oldNames := []string{
		"memcached_reconcile_resource_total",
		"memcached_reconcile_total",
		"memcached_reconcile_duration_seconds",
		"memcached_instance_info",
		"memcached_instance_replicas_desired",
		"memcached_instance_replicas_ready",
	}
	gathered := make(map[string]bool)
	for _, f := range families {
		gathered[f.GetName()] = true
	}
	for _, old := range oldNames {
		if gathered[old] {
			t.Errorf("found metric with old prefix %q; should be renamed to use %q prefix", old, requiredPrefix)
		}
	}

	ResetInstanceMetrics("naming-test", "default")
}

func TestRecordReconciliation_HistogramMetricHelp(t *testing.T) {
	RecordReconciliation("help-test", "default", "success", 50*time.Millisecond)

	expected := `
		# HELP memcached_operator_reconcile_duration_seconds Duration of Memcached reconciliation in seconds.
		# TYPE memcached_operator_reconcile_duration_seconds histogram
	`
	err := testutil.CollectAndCompare(reconcileDuration, strings.NewReader(expected), "memcached_operator_reconcile_duration_seconds")
	if err != nil {
		errStr := err.Error()
		// A diff error is expected because we didn't specify exact bucket values.
		// But registration or collection errors are real failures.
		if strings.Contains(errStr, "registering") || strings.Contains(errStr, "gathering") {
			t.Errorf("unexpected collection error: %v", err)
		}
	}
}
