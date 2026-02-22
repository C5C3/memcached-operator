package v1beta1

import (
	"testing"
)

func TestHub_InterfaceSatisfied(t *testing.T) {
	// Runtime: Hub() must not panic.
	mc := &Memcached{}
	mc.Hub()
}
