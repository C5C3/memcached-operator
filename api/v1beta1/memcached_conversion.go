package v1beta1

import "sigs.k8s.io/controller-runtime/pkg/conversion"

// Compile-time assertion that Memcached implements the Hub interface.
var _ conversion.Hub = &Memcached{}

// Hub marks v1beta1.Memcached as the hub type for conversion.
// The storage version (v1beta1) is the hub; all other versions (spokes)
// convert to and from it.
func (*Memcached) Hub() {}
