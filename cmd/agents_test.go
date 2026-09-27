package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/gumieri/nenyactl/internal/detect"
)

func TestResolveAgentsMode(t *testing.T) {
	saved := agentsMode
	t.Cleanup(func() { agentsMode = saved })

	containerDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(containerDir, "config"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(containerDir, "config", "config.json"), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	bareMetalDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(bareMetalDir, "config.json"), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}

	t.Run("auto-detects container layout", func(t *testing.T) {
		agentsMode = ""
		mode, err := resolveAgentsMode(containerDir)
		if err != nil || mode != detect.ModeContainer {
			t.Fatalf("mode=%v err=%v", mode, err)
		}
	})

	t.Run("auto-detects bare-metal layout", func(t *testing.T) {
		agentsMode = ""
		mode, err := resolveAgentsMode(bareMetalDir)
		if err != nil || mode != detect.ModeBareMetal {
			t.Fatalf("mode=%v err=%v", mode, err)
		}
	})

	t.Run("explicit mode overrides detection", func(t *testing.T) {
		agentsMode = "bare-metal"
		mode, err := resolveAgentsMode(containerDir)
		if err != nil || mode != detect.ModeBareMetal {
			t.Fatalf("mode=%v err=%v", mode, err)
		}
	})

	t.Run("rejects unknown mode", func(t *testing.T) {
		agentsMode = "bogus"
		if _, err := resolveAgentsMode(containerDir); err == nil {
			t.Fatal("expected error for unknown mode")
		}
	})
}
