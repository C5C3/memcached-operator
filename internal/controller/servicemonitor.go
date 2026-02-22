// Package controller implements the reconciliation logic for the memcached-operator.
package controller

import (
	monitoringv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	memcachedv1beta1 "github.com/c5c3/memcached-operator/api/v1beta1"
)

// constructServiceMonitor sets the desired state of the ServiceMonitor based on the Memcached CR spec.
// It mutates sm in-place and is designed to be called from within controllerutil.CreateOrUpdate.
func constructServiceMonitor(mc *memcachedv1beta1.Memcached, sm *monitoringv1.ServiceMonitor) {
	// Resolve the optional ServiceMonitor spec once to avoid repeated nil-checks.
	var smSpec *memcachedv1beta1.ServiceMonitorSpec
	if mc.Spec.Monitoring != nil {
		smSpec = mc.Spec.Monitoring.ServiceMonitor
	}

	// Build labels: start with additionalLabels, then overlay standard labels so
	// standard labels always take precedence and cannot be overridden.
	labels := make(map[string]string)
	if smSpec != nil {
		for k, v := range smSpec.AdditionalLabels {
			labels[k] = v
		}
	}
	for k, v := range labelsForMemcached(mc.Name) {
		labels[k] = v
	}

	sm.Labels = labels
	sm.Spec.Selector = metav1.LabelSelector{
		MatchLabels: labelsForMemcached(mc.Name),
	}
	sm.Spec.NamespaceSelector = monitoringv1.NamespaceSelector{
		MatchNames: []string{mc.Namespace},
	}

	// Scrape interval and timeout defaults.
	interval := monitoringv1.Duration("30s")
	scrapeTimeout := monitoringv1.Duration("10s")
	if smSpec != nil {
		if smSpec.Interval != "" {
			interval = monitoringv1.Duration(smSpec.Interval)
		}
		if smSpec.ScrapeTimeout != "" {
			scrapeTimeout = monitoringv1.Duration(smSpec.ScrapeTimeout)
		}
	}

	sm.Spec.Endpoints = []monitoringv1.Endpoint{
		{
			Port:          "metrics",
			Interval:      interval,
			ScrapeTimeout: scrapeTimeout,
		},
	}
}

// serviceMonitorEnabled returns true when monitoring is enabled and the ServiceMonitor
// sub-section is present in the CR spec.
func serviceMonitorEnabled(mc *memcachedv1beta1.Memcached) bool {
	return mc.Spec.Monitoring != nil &&
		mc.Spec.Monitoring.Enabled &&
		mc.Spec.Monitoring.ServiceMonitor != nil
}
