package cmd

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/gumieri/nenyactl/internal/detect"
	"github.com/gumieri/nenyactl/internal/nenya"
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
	})

	t.Run("--dir is refined through the paths contract", func(t *testing.T) {
		saved := pathsProbe
		pathsProbe = func(dir string) (*nenya.Paths, error) {
			if dir != bareMetalDir {
				t.Errorf("probe dir = %q, want %q", dir, bareMetalDir)
			}
			return &nenya.Paths{
				Mode:       "directory",
				ConfigDir:  bareMetalDir,
				ConfigFile: filepath.Join(bareMetalDir, "config.json"),
				ConfigD:    filepath.Join(bareMetalDir, "config.d"),
				SecretsDir: "/run/secrets/nenya",
			}, nil
		}
		t.Cleanup(func() { pathsProbe = saved })

		res, err := resolveDir(bareMetalDir, dirAttach, false)
		if err != nil {
			t.Fatal(err)
		}
		cp := res.ContractPaths()
		if cp == nil {
			t.Fatal("ContractPaths = nil, want the resolved contract")
		}
		if cp.ConfigFile != filepath.Join(bareMetalDir, "config.json") {
			t.Errorf("ContractPaths.ConfigFile = %s", cp.ConfigFile)
		}
	})

	t.Run("empty --dir takes the server-resolved root (honors NENYA_CONFIG_DIR)", func(t *testing.T) {
		saved := pathsProbe
		envRoot := filepath.Join(t.TempDir(), "env-root")
		pathsProbe = func(dir string) (*nenya.Paths, error) {
			if dir != "" {
				t.Errorf("default resolution must not pin a config dir, got %q", dir)
			}
			return &nenya.Paths{
				Mode:       "directory",
				ConfigDir:  envRoot,
				ConfigFile: filepath.Join(envRoot, "config.json"),
				ConfigD:    filepath.Join(envRoot, "config.d"),
			}, nil
		}
		t.Cleanup(func() { pathsProbe = saved })

		res, err := resolveDir("", dirAttach, false)
		if err != nil {
			t.Fatal(err)
		}
		if res.Path != envRoot {
			t.Errorf("Path = %s, want the §3.3-resolved %s", res.Path, envRoot)
		}
		if res.Info.ConfigFile != filepath.Join(envRoot, "config.json") {
			t.Errorf("ConfigFile = %s", res.Info.ConfigFile)
		}
	})

	t.Run("a failed probe falls back to the local layout", func(t *testing.T) {
		saved := pathsProbe
		pathsProbe = func(dir string) (*nenya.Paths, error) { return nil, errors.New("nenya paths: unknown command") }
		t.Cleanup(func() { pathsProbe = saved })

		res, err := resolveDir(bareMetalDir, dirAttach, false)
		if err != nil {
			t.Fatal(err)
		}
		if res.ContractPaths() != nil {
			t.Errorf("ContractPaths = %+v, want nil after a failed probe", res.ContractPaths())
		}
		if res.Info.ConfigFile != filepath.Join(bareMetalDir, "config.json") {
			t.Errorf("ConfigFile = %s, want the local structural layout", res.Info.ConfigFile)
		}
	})

	t.Run("container roots are never probed", func(t *testing.T) {
		saved := pathsProbe
		pathsProbe = func(dir string) (*nenya.Paths, error) {
			t.Error("container resolution must not call the contract")
			return nil, errors.New("should not be called")
		}
		t.Cleanup(func() { pathsProbe = saved })

		res, err := resolveDir(containerDir, dirAttach, false)
		if err != nil {
			t.Fatal(err)
		}
		if res.ContractPaths() != nil {
			t.Errorf("ContractPaths = %+v, want nil for a container", res.ContractPaths())
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
		if res.Info.SecretsDir() != filepath.Join(containerDir, "secrets") {
			t.Errorf("SecretsDir = %s", res.Info.SecretsDir())
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

func TestInfoSecretsDir(t *testing.T) {
	container := &detect.Info{Mode: detect.ModeContainer, DataDir: "/data/nenya", ConfigFile: "/data/nenya/config/config.json"}
	if got := container.SecretsDir(); got != "/data/nenya/secrets" {
		t.Errorf("container SecretsDir = %s", got)
	}
	bare := &detect.Info{Mode: detect.ModeBareMetal, ConfigFile: "/etc/nenya/config.json"}
	if got := bare.SecretsDir(); got != "/etc/nenya" {
		t.Errorf("bare-metal SecretsDir = %s", got)
	}
}
