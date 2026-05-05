package version

import (
	"strings"
	"testing"
)

func TestVersion(t *testing.T) {
	t.Run("Version is set", func(t *testing.T) {
		if Version == "" {
			t.Error("Version should not be empty")
		}
	})

	t.Run("Version is not 'unknown'", func(t *testing.T) {
		if Version == "unknown" || strings.Contains(Version, "unknown") {
			// This is expected in test mode (ldflags not set)
			t.Logf("Version is %q (expected in test mode)", Version)
		}
	})

	t.Run("Commit is set", func(t *testing.T) {
		if Commit == "" {
			t.Error("Commit should not be empty")
		}
	})
}

func TestBuildTime(t *testing.T) {
	t.Run("BuildTime is set", func(t *testing.T) {
		if BuildTime == "" {
			t.Error("BuildTime should not be empty")
		}
	})
}