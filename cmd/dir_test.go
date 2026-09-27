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
		res, err := resolveDir(bareMetalDir, false)
		if err != nil {
			t.Fatal(err)
		}
		if res.Kind != dirConfigRoot {
			t.Errorf("Kind = %v, want config root", res.Kind)
		}
		if res.Info.ConfigFile != filepath.Join(bareMetalDir, "config.json") {
			t.Errorf("ConfigFile = %s", res.Info.ConfigFile)
		}
		if res.Info.ConfigD != filepath.Join(bareMetalDir, "config.d") {
			t.Errorf("ConfigD = %s", res.Info.ConfigD)
		}
	})

	t.Run("container --dir resolves nested config paths", func(t *testing.T) {
		res, err := resolveDir(containerDir, false)
		if err != nil {
			t.Fatal(err)
		}
		if res.Kind != dirContainerRoot {
			t.Errorf("Kind = %v, want container root", res.Kind)
		}
		if res.Info.ConfigFile != filepath.Join(containerDir, "config", "config.json") {
			t.Errorf("ConfigFile = %s", res.Info.ConfigFile)
		}
		if res.Info.ConfigD != filepath.Join(containerDir, "config", "config.d") {
			t.Errorf("ConfigD = %s", res.Info.ConfigD)
		}
		if res.Info.DataDir != containerDir {
			t.Errorf("DataDir = %s", res.Info.DataDir)
		}
	})

	t.Run("empty --dir defaults to the system config root", func(t *testing.T) {
		res, err := resolveDir("", false)
		if err != nil {
			t.Fatal(err)
		}
		if res.Kind != dirConfigRoot || res.Info.Mode != detect.ModeBareMetal {
			t.Errorf("res = %+v", res)
		}
	})

	t.Run("empty --dir with container default", func(t *testing.T) {
		t.Setenv("XDG_DATA_HOME", t.TempDir())
		res, err := resolveDir("", true)
		if err != nil {
			t.Fatal(err)
		}
		if res.Kind != dirContainerRoot {
			t.Errorf("Kind = %v, want container root", res.Kind)
		}
	})

	t.Run("missing --dir path errors", func(t *testing.T) {
		if _, err := resolveDir("/nonexistent/nenyactl/dir", false); err == nil {
			t.Fatal("expected error for missing directory")
		}
	})
}

func TestInfoSecretsFile(t *testing.T) {
	bare := &detect.Info{Mode: detect.ModeBareMetal, ConfigFile: "/etc/nenya/config.json"}
	if got := bare.SecretsFile(); got != "/etc/nenya/secrets.json" {
		t.Errorf("bare-metal SecretsFile = %s", got)
	}
	container := &detect.Info{Mode: detect.ModeContainer, DataDir: "/data/nenya", ConfigFile: "/data/nenya/config/config.json"}
	if got := container.SecretsFile(); got != "/data/nenya/secrets.json" {
		t.Errorf("container SecretsFile = %s", got)
	}
}
