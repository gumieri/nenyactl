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
		if _, err := os.Stat(filepath.Join(tmp, "secrets.json")); os.IsNotExist(err) {
			t.Error("secrets.json not created")
		}
	})

	t.Run("refuses to overwrite existing", func(t *testing.T) {
		tmp := t.TempDir()
		os.WriteFile(filepath.Join(tmp, "secrets.json"), []byte("{}"), 0o644)
		bootstrapDir = tmp
		err := runSecretBootstrap(&cobra.Command{}, nil)
		if err == nil {
			t.Fatal("expected error for existing file")
		}
	})
}
