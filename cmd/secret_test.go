package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func TestRunSecretGenerate(t *testing.T) {
	t.Run("generates client token", func(t *testing.T) {
		secretType = "client"
		err := runSecretGenerate(&cobra.Command{}, nil)
		if err != nil {
			t.Fatalf("runSecretGenerate() error = %v", err)
		}
	})

	t.Run("returns error on apikey without name", func(t *testing.T) {
		secretType = "apikey"
		secretForClient = ""
		err := runSecretGenerate(&cobra.Command{}, nil)
		if err == nil {
			t.Fatal("expected error for missing --name")
		}
		if !strings.Contains(err.Error(), "name is required") {
			t.Errorf("unexpected error: %v", err)
		}
	})

	t.Run("generates apikey with name", func(t *testing.T) {
		secretType = "apikey"
		secretForClient = "test-client"
		err := runSecretGenerate(&cobra.Command{}, nil)
		if err != nil {
			t.Fatalf("runSecretGenerate() error = %v", err)
		}
	})

	t.Run("returns error on unknown type", func(t *testing.T) {
		secretType = "unknown"
		err := runSecretGenerate(&cobra.Command{}, nil)
		if err == nil {
			t.Fatal("expected error for unknown type")
		}
	})
}

func TestRunSecretBootstrap(t *testing.T) {
	t.Run("creates secrets file", func(t *testing.T) {
		tmp := t.TempDir()
		bootstrapDir = tmp
		err := runSecretBootstrap(&cobra.Command{}, nil)
		if err != nil {
			t.Fatalf("runSecretBootstrap() error = %v", err)
		}
		path := filepath.Join(tmp, "secrets.json")
		info, err := os.Stat(path)
		if err != nil {
			t.Fatalf("client secrets not created: %v", err)
		}
		if info.Mode().Perm() != 0o600 {
			t.Errorf("mode = %o, want 0600", info.Mode().Perm())
		}
	})

	t.Run("refuses to overwrite existing", func(t *testing.T) {
		tmp := t.TempDir()
		if err := os.WriteFile(filepath.Join(tmp, "secrets.json"), []byte("{}"), 0o600); err != nil {
			t.Fatalf("write secrets: %v", err)
		}
		bootstrapDir = tmp
		err := runSecretBootstrap(&cobra.Command{}, nil)
		if err == nil {
			t.Fatal("expected error for existing file")
		}
	})

	t.Run("container dir writes into secrets/", func(t *testing.T) {
		tmp := t.TempDir()
		if err := os.MkdirAll(filepath.Join(tmp, "config"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(tmp, "config", "config.json"), []byte("{}"), 0o644); err != nil {
			t.Fatal(err)
		}
		bootstrapDir = tmp
		if err := runSecretBootstrap(&cobra.Command{}, nil); err != nil {
			t.Fatalf("runSecretBootstrap() error = %v", err)
		}
		if _, err := os.Stat(filepath.Join(tmp, "secrets", "01-client.json")); err != nil {
			t.Errorf("expected secrets/01-client.json: %v", err)
		}
		if _, err := os.Stat(filepath.Join(tmp, "secrets.json")); err == nil {
			t.Error("must not write root-level secrets.json for a container layout")
		}
	})
}
