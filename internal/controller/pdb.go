// Package controller implements the reconciliation logic for the memcached-operator.
package controller

import (
	policyv1 "k8s.io/api/policy/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

	memcachedv1beta1 "github.com/c5c3/memcached-operator/api/v1beta1"
)

// constructPDB sets the desired state of the PodDisruptionBudget based on the Memcached CR spec.
// It mutates pdb in-place and is designed to be called from within controllerutil.CreateOrUpdate.
func constructPDB(mc *memcachedv1beta1.Memcached, pdb *policyv1.PodDisruptionBudget) {
	labels := labelsForMemcached(mc.Name)

	pdb.Labels = labels
	pdb.Spec.Selector = &metav1.LabelSelector{
		MatchLabels: labels,
	}

	pdbSpec := mc.Spec.HighAvailability.PodDisruptionBudget

	switch {
	case pdbSpec.MinAvailable != nil:
		// Explicit minAvailable takes precedence; clear maxUnavailable.
		pdb.Spec.MinAvailable = pdbSpec.MinAvailable
		pdb.Spec.MaxUnavailable = nil
	case pdbSpec.MaxUnavailable != nil:
		// Only maxUnavailable set; clear minAvailable.
		pdb.Spec.MaxUnavailable = pdbSpec.MaxUnavailable
		pdb.Spec.MinAvailable = nil
	default:
		// Neither set: default minAvailable to 1.
		defaultMinAvailable := intstr.FromInt32(1)
		pdb.Spec.MinAvailable = &defaultMinAvailable
		pdb.Spec.MaxUnavailable = nil
	}
}

// pdbEnabled returns true only when PDB creation is explicitly enabled in the CR spec.
func pdbEnabled(mc *memcachedv1beta1.Memcached) bool {
	return mc.Spec.HighAvailability != nil &&
		mc.Spec.HighAvailability.PodDisruptionBudget != nil &&
		mc.Spec.HighAvailability.PodDisruptionBudget.Enabled
}
