package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/gumieri/nenyactl/internal/detect"
)

func TestResolveDir(t *testing.T) {
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

	t.Run("config root --dir resolves bare-metal paths", func(t *testing.T) {
		res, err := resolveDir(bareMetalDir, dirAttach, false)
		if err != nil {
			t.Fatal(err)
		}
		if res.Kind != dirConfigRoot {
			t.Errorf("Kind = %v, want config root", res.Kind)
		}
		if res.Info.ConfigFile != filepath.Join(bareMetalDir, "config.json") {
			t.Errorf("ConfigFile = %s", res.Info.ConfigFile)
		}
		if res.ConfigDir() != bareMetalDir {
			t.Errorf("ConfigDir = %s", res.ConfigDir())
		}
		if res.SecretsDir() != bareMetalDir {
			t.Errorf("SecretsDir = %s", res.SecretsDir())
		}
	})

	t.Run("container --dir resolves nested config paths", func(t *testing.T) {
		res, err := resolveDir(containerDir, dirAttach, false)
		if err != nil {
			t.Fatal(err)
		}
		if res.Kind != dirContainerRoot {
			t.Errorf("Kind = %v, want container root", res.Kind)
		}
		if res.Info.ConfigFile != filepath.Join(containerDir, "config", "config.json") {
			t.Errorf("ConfigFile = %s", res.Info.ConfigFile)
		}
		if res.ConfigDir() != filepath.Join(containerDir, "config") {
			t.Errorf("ConfigDir = %s, want <root>/config", res.ConfigDir())
		}
		if res.SecretsDir() != filepath.Join(containerDir, "secrets") {
			t.Errorf("SecretsDir = %s, want <root>/secrets", res.SecretsDir())
		}
	})

	t.Run("attach mode rejects a missing path", func(t *testing.T) {
		if _, err := resolveDir("/nonexistent/nenyactl/dir", dirAttach, false); err == nil {
			t.Fatal("expected error for missing directory in attach mode")
		}
	})

	t.Run("create mode treats a missing --dir as a config root", func(t *testing.T) {
		missing := filepath.Join(t.TempDir(), "new-config-root")
		res, err := resolveDir(missing, dirCreate, false)
		if err != nil {
			t.Fatalf("create mode should accept a missing path: %v", err)
		}
		if res.Kind != dirConfigRoot {
			t.Errorf("Kind = %v, want config root", res.Kind)
		}
	})

	t.Run("create mode with container default accepts a missing path", func(t *testing.T) {
		missing := filepath.Join(t.TempDir(), "new-container")
		res, err := resolveDir(missing, dirCreate, true)
		if err != nil {
			t.Fatal(err)
		}
		// --dir overrides the container default; with no layout markers the
		// explicit path is a config root.
		if res.Path != missing {
			t.Errorf("Path = %s, want %s", res.Path, missing)
		}
	})

	t.Run("empty --dir defaults to the system config root", func(t *testing.T) {
		res, err := resolveDir("", dirAttach, false)
		if err != nil {
			t.Fatal(err)
		}
		if res.Kind != dirConfigRoot || res.Info.Mode != detect.ModeBareMetal {
			t.Errorf("res = %+v", res)
		}
	})

	t.Run("empty --dir with container default", func(t *testing.T) {
		t.Setenv("XDG_DATA_HOME", t.TempDir())
		res, err := resolveDir("", dirAttach, true)
		if err != nil {
			t.Fatal(err)
		}
		if res.Kind != dirContainerRoot {
			t.Errorf("Kind = %v, want container root", res.Kind)
		}
	})
}

func TestInfoSecretsFile(t *testing.T) {
	bare := &detect.Info{Mode: detect.ModeBareMetal, ConfigFile: "/etc/nenya/config.json"}
	if got := bare.SecretsFile(); got != "/etc/nenya/secrets.json" {
		t.Errorf("bare-metal SecretsFile = %s", got)
	}
	container := &detect.Info{Mode: detect.ModeContainer, DataDir: "/data/nenya", ConfigFile: "/data/nenya/config/config.json"}
	if got := container.SecretsFile(); got != "/data/nenya/secrets/01-client.json" {
		t.Errorf("container SecretsFile = %s", got)
	}
	if got := container.SecretsDir(); got != "/data/nenya/secrets" {
		t.Errorf("container SecretsDir = %s", got)
	}
}
