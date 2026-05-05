package containers

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSetup(t *testing.T) {
	t.Run("creates directory structure", func(t *testing.T) {
		tmp := t.TempDir()
		cfg := SetupConfig{
			ListenAddr: ":8080",
			Dir:        tmp,
		}

		if err := Setup(cfg); err != nil {
			t.Fatalf("Setup() error = %v", err)
		}

		configDir := filepath.Join(tmp, "config")
		if _, err := os.Stat(configDir); os.IsNotExist(err) {
			t.Error("config directory not created")
		}

		secretsDir := filepath.Join(tmp, "secrets")
		if _, err := os.Stat(secretsDir); os.IsNotExist(err) {
			t.Error("secrets directory not created")
		}
	})

	t.Run("creates config.json", func(t *testing.T) {
		tmp := t.TempDir()
		cfg := SetupConfig{
			ListenAddr: ":8080",
			Dir:        tmp,
		}

		if err := Setup(cfg); err != nil {
			t.Fatalf("Setup() error = %v", err)
		}

		configPath := filepath.Join(tmp, "config", "config.json")
		if _, err := os.Stat(configPath); os.IsNotExist(err) {
			t.Error("config.json not created")
		}
	})

	t.Run("creates compose.yml with ListenAddr", func(t *testing.T) {
		tmp := t.TempDir()
		listenAddr := ":9090"
		cfg := SetupConfig{
			ListenAddr: listenAddr,
			Dir:        tmp,
		}

		if err := Setup(cfg); err != nil {
			t.Fatalf("Setup() error = %v", err)
		}

		composePath := filepath.Join(tmp, "compose.yml")
		data, err := os.ReadFile(composePath)
		if err != nil {
			t.Fatalf("read compose.yml: %v", err)
		}

		composeStr := string(data)
		if !strings.Contains(composeStr, listenAddr) {
			t.Errorf("compose.yml does not contain ListenAddr %s", listenAddr)
		}
		if !strings.Contains(composeStr, "ghcr.io/gumieri/nenya:latest") {
			t.Error("compose.yml does not contain correct image")
		}
	})

	t.Run("creates .env file", func(t *testing.T) {
		tmp := t.TempDir()
		cfg := SetupConfig{
			ListenAddr: ":8080",
			Dir:        tmp,
		}

		if err := Setup(cfg); err != nil {
			t.Fatalf("Setup() error = %v", err)
		}

		envPath := filepath.Join(tmp, ".env")
		if _, err := os.Stat(envPath); os.IsNotExist(err) {
			t.Error(".env not created")
		}

		data, err := os.ReadFile(envPath)
		if err != nil {
			t.Fatalf("read .env: %v", err)
		}

		envStr := string(data)
		if !strings.Contains(envStr, "NENYA_IMAGE") {
			t.Error(".env does not contain NENYA_IMAGE")
		}
	})

	t.Run("does not overwrite existing config.json", func(t *testing.T) {
		tmp := t.TempDir()
		cfg := SetupConfig{
			ListenAddr: ":8080",
			Dir:        tmp,
		}

		configPath := filepath.Join(tmp, "config", "config.json")
		configDir := filepath.Dir(configPath)
		if err := os.MkdirAll(configDir, 0o755); err != nil {
			t.Fatalf("mkdir config: %v", err)
		}

		customContent := `{"custom": true}`
		if err := os.WriteFile(configPath, []byte(customContent), 0o644); err != nil {
			t.Fatalf("write custom config: %v", err)
		}

		if err := Setup(cfg); err != nil {
			t.Fatalf("Setup() error = %v", err)
		}

		data, err := os.ReadFile(configPath)
		if err != nil {
			t.Fatalf("read config.json: %v", err)
		}

		if string(data) != customContent {
			t.Error("config.json was overwritten")
		}
	})

	t.Run("does not overwrite existing .env", func(t *testing.T) {
		tmp := t.TempDir()
		cfg := SetupConfig{
			ListenAddr: ":8080",
			Dir:        tmp,
		}

		envPath := filepath.Join(tmp, ".env")
		customContent := "CUSTOM=value"
		if err := os.WriteFile(envPath, []byte(customContent), 0o644); err != nil {
			t.Fatalf("write custom .env: %v", err)
		}

		if err := Setup(cfg); err != nil {
			t.Fatalf("Setup() error = %v", err)
		}

		data, err := os.ReadFile(envPath)
		if err != nil {
			t.Fatalf("read .env: %v", err)
		}

		if string(data) != customContent {
			t.Error(".env was overwritten")
		}
	})

	t.Run("secrets directory has correct permissions", func(t *testing.T) {
		tmp := t.TempDir()
		cfg := SetupConfig{
			ListenAddr: ":8080",
			Dir:        tmp,
		}

		if err := Setup(cfg); err != nil {
			t.Fatalf("Setup() error = %v", err)
		}

		secretsDir := filepath.Join(tmp, "secrets")
		info, err := os.Stat(secretsDir)
		if err != nil {
			t.Fatalf("stat secrets: %v", err)
		}

		if info.Mode().Perm()&0o700 != 0o700 {
			t.Errorf("secrets directory has incorrect permissions: %v", info.Mode().Perm())
		}
	})
}
