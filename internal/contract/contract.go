// Package contract declares the nenya consumer contract range nenyactl
// supports and the fail-fast check for out-of-range installations.
package contract

import "fmt"

// Supported is the inclusive [min, max] contract_version range this build
// supports. Bump it when nenyactl adopts a newer contract.
var Supported = [2]int{1, 1}

// Check returns an actionable error when v is outside Supported.
func Check(v int) error {
	if v < Supported[0] || v > Supported[1] {
		return fmt.Errorf("nenya contract_version %d is not supported by this nenyactl build (supports %d..%d); update nenyactl or install a compatible nenya",
			v, Supported[0], Supported[1])
	}
	return nil
}

// Range renders Supported as "[min,max]" for display.
func Range() string {
	return fmt.Sprintf("[%d,%d]", Supported[0], Supported[1])
}
