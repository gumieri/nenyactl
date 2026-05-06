package detect

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestDetectWith_BareMetal(t *testing.T) {
	t.Run("detects bare-metal from LookPath with readable config", func(t *testing.T) {
		tmp := t.TempDir()
		binPath := filepath.Join(tmp, "bin", "nenya")
		if err := os.MkdirAll(filepath.Dir(binPath), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(binPath, []byte("#!/bin/sh\nexit 0"), 0o755); err != nil {
			t.Fatal(err)
		}

		configDir := filepath.Join(tmp, "etc", "nenya")
		if err := os.MkdirAll(configDir, 0o755); err != nil {
			t.Fatal(err)
		}
		configFile := filepath.Join(configDir, "config.json")
		if err := os.WriteFile(configFile, []byte("{}"), 0o644); err != nil {
			t.Fatal(err)
		}

		look := func(name string) (string, error) {
			if name == "nenya" {
				return binPath, nil
			}
			return "", os.ErrNotExist
		}
		sysCfgDir := func() string { return configDir }

		info, err := DetectWith(look, sysCfgDir)
		if err != nil {
			t.Fatalf("DetectWith() error = %v", err)
		}

		if info.Mode != ModeBareMetal {
			t.Errorf("Mode = %v, want %v", info.Mode, ModeBareMetal)
		}
		if info.BinPath != binPath {
			t.Errorf("BinPath = %v, want %v", info.BinPath, binPath)
		}
		if info.ConfigFile != configFile {
			t.Errorf("ConfigFile = %v, want %v", info.ConfigFile, configFile)
		}
		if info.ConfigD != filepath.Join(configDir, "config.d") {
			t.Errorf("ConfigD = %v, want %v", info.ConfigD, filepath.Join(configDir, "config.d"))
		}
	})

	t.Run("reports ConfigNotFoundError when config missing", func(t *testing.T) {
		tmp := t.TempDir()
		binPath := filepath.Join(tmp, "nenya")
		if err := os.WriteFile(binPath, []byte("binary"), 0o755); err != nil {
			t.Fatal(err)
		}

		configDir := filepath.Join(tmp, "etc", "nenya")

		look := func(name string) (string, error) {
			if name == "nenya" {
				return binPath, nil
			}
			return "", os.ErrNotExist
		}
		sysCfgDir := func() string { return configDir }

		_, err := DetectWith(look, sysCfgDir)
		if err == nil {
			t.Fatal("expected error when config missing")
		}

		var cfgErr *ConfigNotFoundError
		if !errors.As(err, &cfgErr) {
			t.Errorf("expected ConfigNotFoundError, got %T: %v", err, err)
		}
	})

	t.Run("reports PermissionError when config unreadable", func(t *testing.T) {
		if os.Getuid() == 0 {
			t.Skip("running as root, permission test not applicable")
		}

		tmp := t.TempDir()
		binPath := filepath.Join(tmp, "nenya")
		if err := os.WriteFile(binPath, []byte("binary"), 0o755); err != nil {
			t.Fatal(err)
		}

		configDir := filepath.Join(tmp, "etc", "nenya")
		if err := os.MkdirAll(configDir, 0o755); err != nil {
			t.Fatal(err)
		}
		configFile := filepath.Join(configDir, "config.json")
		if err := os.WriteFile(configFile, []byte("{}"), 0o000); err != nil {
			t.Fatal(err)
		}

		look := func(name string) (string, error) {
			if name == "nenya" {
				return binPath, nil
			}
			return "", os.ErrNotExist
		}
		sysCfgDir := func() string { return configDir }

		_, err := DetectWith(look, sysCfgDir)
		if err == nil {
			t.Fatal("expected error for unreadable config")
		}

		var permErr *PermissionError
		if !errors.As(err, &permErr) {
			t.Errorf("expected PermissionError, got %T: %v", err, err)
		}
	})
}

func TestDetectWith_Container(t *testing.T) {
	t.Run("detects container from compose.yml", func(t *testing.T) {
		tmp := t.TempDir()
		t.Setenv("XDG_DATA_HOME", tmp)

		containerDir := filepath.Join(tmp, "nenyactl", "nenya")
		if err := os.MkdirAll(containerDir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(containerDir, "compose.yml"), []byte("services: {}"), 0o644); err != nil {
			t.Fatal(err)
		}

		look := func(string) (string, error) { return "", os.ErrNotExist }
		sysCfgDir := func() string { return filepath.Join(tmp, "etc", "nenya") }

		info, err := DetectWith(look, sysCfgDir)
		if err != nil {
			t.Fatalf("DetectWith() error = %v", err)
		}

		if info.Mode != ModeContainer {
			t.Errorf("Mode = %v, want %v", info.Mode, ModeContainer)
		}
		if info.DataDir != containerDir {
			t.Errorf("DataDir = %v, want %v", info.DataDir, containerDir)
		}
		wantConfigFile := filepath.Join(containerDir, "config", "config.json")
		if info.ConfigFile != wantConfigFile {
			t.Errorf("ConfigFile = %v, want %v", info.ConfigFile, wantConfigFile)
		}
	})

	t.Run("detects container from config/config.json", func(t *testing.T) {
		tmp := t.TempDir()
		t.Setenv("XDG_DATA_HOME", tmp)

		containerDir := filepath.Join(tmp, "nenyactl", "nenya")
		configDir := filepath.Join(containerDir, "config")
		if err := os.MkdirAll(configDir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(configDir, "config.json"), []byte("{}"), 0o644); err != nil {
			t.Fatal(err)
		}

		look := func(string) (string, error) { return "", os.ErrNotExist }
		sysCfgDir := func() string { return filepath.Join(tmp, "etc", "nenya") }

		info, err := DetectWith(look, sysCfgDir)
		if err != nil {
			t.Fatalf("DetectWith() error = %v", err)
		}

		if info.Mode != ModeContainer {
			t.Errorf("Mode = %v, want %v", info.Mode, ModeContainer)
		}
	})
}

func TestDetectWith_Ambiguous(t *testing.T) {
	t.Run("errors when both bare-metal and container found", func(t *testing.T) {
		tmp := t.TempDir()
		t.Setenv("XDG_DATA_HOME", tmp)

		binPath := filepath.Join(tmp, "nenya")
		if err := os.WriteFile(binPath, []byte("binary"), 0o755); err != nil {
			t.Fatal(err)
		}

		configDir := filepath.Join(tmp, "etc", "nenya")
		if err := os.MkdirAll(configDir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(configDir, "config.json"), []byte("{}"), 0o644); err != nil {
			t.Fatal(err)
		}

		containerDir := filepath.Join(tmp, "nenyactl", "nenya")
		if err := os.MkdirAll(containerDir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(containerDir, "compose.yml"), []byte("services: {}"), 0o644); err != nil {
			t.Fatal(err)
		}

		look := func(name string) (string, error) {
			if name == "nenya" {
				return binPath, nil
			}
			return "", os.ErrNotExist
		}
		sysCfgDir := func() string { return configDir }

		_, err := DetectWith(look, sysCfgDir)
		if err == nil {
			t.Fatal("expected error for ambiguous installation")
		}

		var ambig *AmbiguousError
		if !errors.As(err, &ambig) {
			t.Errorf("expected AmbiguousError, got %T: %v", err, err)
		}
	})
}

func TestDetectWith_NotFound(t *testing.T) {
	t.Run("errors when neither installation found", func(t *testing.T) {
		tmp := t.TempDir()
		t.Setenv("XDG_DATA_HOME", tmp)

		look := func(string) (string, error) { return "", os.ErrNotExist }
		sysCfgDir := func() string { return filepath.Join(tmp, "etc", "nenya") }

		_, err := DetectWith(look, sysCfgDir)
		if err == nil {
			t.Fatal("expected error when nenya not found")
		}

		var notFound *NotFoundError
		if !errors.As(err, &notFound) {
			t.Errorf("expected NotFoundError, got %T: %v", err, err)
		}
	})
}

func TestModeString(t *testing.T) {
	tests := []struct {
		mode Mode
		want string
	}{
		{ModeNone, "none"},
		{ModeBareMetal, "bare-metal"},
		{ModeContainer, "container"},
		{Mode(99), "none"},
	}
	for _, tt := range tests {
		if got := tt.mode.String(); got != tt.want {
			t.Errorf("Mode(%d).String() = %q, want %q", tt.mode, got, tt.want)
		}
	}
}

func TestDetectFromDir(t *testing.T) {
	t.Run("bare-metal dir returns config paths at root level", func(t *testing.T) {
		tmp := t.TempDir()
		configFile := filepath.Join(tmp, "config.json")
		if err := os.WriteFile(configFile, []byte("{}"), 0o644); err != nil {
			t.Fatal(err)
		}

		info := DetectFromDir(tmp, ModeBareMetal)

		if info.Mode != ModeBareMetal {
			t.Errorf("Mode = %v, want %v", info.Mode, ModeBareMetal)
		}
		if info.ConfigFile != configFile {
			t.Errorf("ConfigFile = %v, want %v", info.ConfigFile, configFile)
		}
		wantConfigD := filepath.Join(tmp, "config.d")
		if info.ConfigD != wantConfigD {
			t.Errorf("ConfigD = %v, want %v", info.ConfigD, wantConfigD)
		}
	})

	t.Run("container dir returns config paths with nested layout", func(t *testing.T) {
		tmp := t.TempDir()
		configDir := filepath.Join(tmp, "config")
		if err := os.MkdirAll(configDir, 0o755); err != nil {
			t.Fatal(err)
		}
		configFile := filepath.Join(configDir, "config.json")
		if err := os.WriteFile(configFile, []byte("{}"), 0o644); err != nil {
			t.Fatal(err)
		}

		info := DetectFromDir(tmp, ModeContainer)

		if info.Mode != ModeContainer {
			t.Errorf("Mode = %v, want %v", info.Mode, ModeContainer)
		}
		if info.ConfigFile != configFile {
			t.Errorf("ConfigFile = %v, want %v", info.ConfigFile, configFile)
		}
		if info.ConfigD != tmp {
			t.Errorf("ConfigD = %v, want %v", info.ConfigD, tmp)
		}
		if info.DataDir != tmp {
			t.Errorf("DataDir = %v, want %v", info.DataDir, tmp)
		}
	})

	t.Run("unknown mode defaults to bare-metal layout", func(t *testing.T) {
		tmp := t.TempDir()

		info := DetectFromDir(tmp, ModeNone)

		if info.ConfigFile != filepath.Join(tmp, "config.json") {
			t.Errorf("ConfigFile = %v, want %v", info.ConfigFile, filepath.Join(tmp, "config.json"))
		}
	})
}

func TestErrorTypes(t *testing.T) {
	t.Run("AmbiguousError message contains both paths", func(t *testing.T) {
		e := &AmbiguousError{BinPath: "/usr/local/bin/nenya", ContainerDir: "/home/user/.local/share/nenyactl/nenya"}
		msg := e.Error()
		if len(msg) == 0 {
			t.Error("error message should not be empty")
		}
	})

	t.Run("NotFoundError message contains install suggestions", func(t *testing.T) {
		e := &NotFoundError{}
		msg := e.Error()
		if len(msg) == 0 {
			t.Error("error message should not be empty")
		}
	})

	t.Run("PermissionError bare-metal message", func(t *testing.T) {
		e := &PermissionError{Path: "/etc/nenya/config.json", BinPath: "/usr/local/bin/nenya"}
		msg := e.Error()
		if len(msg) == 0 {
			t.Error("error message should not be empty")
		}
	})

	t.Run("PermissionError container message", func(t *testing.T) {
		e := &PermissionError{Path: "/data/config/config.json", DataDir: "/data", IsContainer: true}
		msg := e.Error()
		if len(msg) == 0 {
			t.Error("error message should not be empty")
		}
	})

	t.Run("ConfigNotFoundError message", func(t *testing.T) {
		e := &ConfigNotFoundError{ConfigFile: "/etc/nenya/config.json", BinPath: "/usr/local/bin/nenya"}
		msg := e.Error()
		if len(msg) == 0 {
			t.Error("error message should not be empty")
		}
	})
}
