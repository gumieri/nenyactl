package contract

import (
	"strings"
	"testing"
)

func TestCheck(t *testing.T) {
	for _, v := range []int{1} {
		if err := Check(v); err != nil {
			t.Errorf("Check(%d) unexpected error: %v", v, err)
		}
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
	if got := Range(); got != "[1,1]" {
		t.Errorf("Range() = %q, want [1,1]", got)
	}
}
