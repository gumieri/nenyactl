package cmd

import (
	"os"
	"path/filepath"
	"testing"
)

func TestBootstrapConfig(t *testing.T) {
	t.Run("creates config directory and files", func(t *testing.T) {
		tmp := t.TempDir()
		if err := bootstrapConfig(tmp); err != nil {
			t.Fatalf("bootstrapConfig() error = %v", err)
		}

		configPath := filepath.Join(tmp, "config.json")
		if _, err := os.Stat(configPath); os.IsNotExist(err) {
			t.Error("config.json not created")
		}

		data, err := os.ReadFile(configPath)
		if err != nil {
			t.Fatalf("read config.json: %v", err)
		}
		if len(data) == 0 {
			t.Error("config.json is empty")
		}
	})

	t.Run("does not overwrite existing files", func(t *testing.T) {
		tmp := t.TempDir()
		configPath := filepath.Join(tmp, "config.json")

		customContent := `{"custom": true}`
		if err := os.WriteFile(configPath, []byte(customContent), 0o644); err != nil {
			t.Fatalf("write custom config: %v", err)
		}

		if err := bootstrapConfig(tmp); err != nil {
			t.Fatalf("bootstrapConfig() error = %v", err)
		}

		data, err := os.ReadFile(configPath)
		if err != nil {
			t.Fatalf("read config.json: %v", err)
		}
		if string(data) != customContent {
			t.Error("config.json was unexpectedly overwritten")
		}
	})
}

func TestRunConfigInit(t *testing.T) {
	t.Run("creates config via wrapper", func(t *testing.T) {
		tmp := t.TempDir()
		saved := configDir
		configDir = tmp
		defer func() { configDir = saved }()

		if err := runConfigInit(nil, nil); err != nil {
			t.Fatalf("runConfigInit() error = %v", err)
		}
		if _, err := os.Stat(filepath.Join(tmp, "config.json")); os.IsNotExist(err) {
			t.Error("config.json not created by runConfigInit")
		}
	})
}
