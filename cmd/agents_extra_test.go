package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/gumieri/nenyactl/internal/detect"
)

func TestDropInActive(t *testing.T) {
	dir := t.TempDir()
	configD := filepath.Join(dir, "config.d")
	info := &detect.Info{ConfigD: configD}

	if dropInActive(info) {
		t.Error("missing config.d should not be active")
	}

	if err := os.MkdirAll(configD, 0o755); err != nil {
		t.Fatal(err)
	}
	if dropInActive(info) {
		t.Error("empty config.d should not be active")
	}

	// secrets.json alone does not activate directory mode.
	if err := os.WriteFile(filepath.Join(configD, "secrets.json"), []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	if dropInActive(info) {
		t.Error("config.d with only secrets.json should not be active")
	}

	if err := os.WriteFile(filepath.Join(configD, "20-agents.json"), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	if !dropInActive(info) {
		t.Error("config.d with a drop-in should be active")
	}
}
