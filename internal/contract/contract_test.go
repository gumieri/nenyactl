package contract

import (
	"errors"
	"strings"
	"testing"
)

func TestCheck(t *testing.T) {
	if err := Check(1); err != nil {
		t.Errorf("Check(1) unexpected error: %v", err)
	}
	for _, v := range []int{0, 2, 99} {
		err := Check(v)
		if err == nil {
			t.Fatalf("Check(%d) expected error", v)
		}
		if !strings.Contains(err.Error(), "contract_version") {
			t.Errorf("error should name the contract version: %v", err)
		}
	}
}

func TestRange(t *testing.T) {
	if got, want := Range(), "1..1"; got != want {
		t.Errorf("Range() = %q, want %q", got, want)
	}
	if !strings.Contains(Check(99).Error(), Range()) {
		t.Error("Check message should include Range()")
	}
}

func TestUnsupportedErrorRoundTrip(t *testing.T) {
	err := Check(99)
	v, ok := UnsupportedContractVersion(err)
	if !ok {
		t.Fatalf("UnsupportedContractVersion(%v) = not found", err)
	}
	if v != 99 {
		t.Errorf("version = %d, want 99", v)
	}

	if _, ok := UnsupportedContractVersion(errors.New("other")); ok {
		t.Error("a plain error must not report an unsupported contract version")
	}
}
