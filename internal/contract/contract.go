// Package contract declares the nenya consumer contract range nenyactl
// supports and the fail-fast check for out-of-range installations.
package contract

import (
	"errors"
	"fmt"
)

// SupportedMin and SupportedMax bound the inclusive contract_version range this
// build supports. Bump SupportedMax when nenyactl adopts a newer contract.
const (
	SupportedMin = 1
	SupportedMax = 1
)

// UnsupportedError reports an installed contract_version outside the supported
// range. It is typed so callers (e.g. doctor) can distinguish a range failure
// from a command that is simply absent.
type UnsupportedError struct {
	Version int
}

func (e *UnsupportedError) Error() string {
	return fmt.Sprintf("nenya contract_version %d is not supported by this nenyactl build (supports %s); update nenyactl or install a compatible nenya",
		e.Version, Range())
}

// UnsupportedContractVersion reports the installed contract_version that an
// unsupported-contract error carried, and whether err was such an error.
func UnsupportedContractVersion(err error) (int, bool) {
	var unsupported *UnsupportedError
	if errors.As(err, &unsupported) {
		return unsupported.Version, true
	}
	return 0, false
}

// Check returns an actionable error when v is outside the supported range.
func Check(v int) error {
	if v < SupportedMin || v > SupportedMax {
		return &UnsupportedError{Version: v}
	}
	return nil
}

// Range renders the supported range for display.
func Range() string {
	return fmt.Sprintf("%d..%d", SupportedMin, SupportedMax)
}
