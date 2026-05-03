package paths

import (
	"os"
	"os/user"
	"path/filepath"
	"runtime"
	"testing"
)

func TestContainerDir(t *testing.T) {
	t.Run("returns correct path with XDG_DATA_HOME", func(t *testing.T) {
		tmp := t.TempDir()
		t.Setenv("XDG_DATA_HOME", tmp)

		got, err := ContainerDir()
		if err != nil {
			t.Fatalf("ContainerDir() error = %v", err)
		}
		want := filepath.Join(tmp, "nenyactl", "nenya")
		if got != want {
			t.Errorf("ContainerDir() = %v, want %v", got, want)
		}
	})

	t.Run("falls back to home/.local/share without XDG_DATA_HOME", func(t *testing.T) {
		t.Setenv("XDG_DATA_HOME", "")

		usr, err := user.Current()
		if err != nil {
			t.Skip("cannot get current user")
		}

		want := filepath.Join(usr.HomeDir, ".local", "share", "nenyactl", "nenya")
		got, err := ContainerDir()
		if err != nil {
			t.Fatalf("ContainerDir() error = %v", err)
		}
		if got != want {
			t.Errorf("ContainerDir() = %v, want %v", got, want)
		}
	})

	t.Run("darwin uses Library/Application Support", func(t *testing.T) {
		if runtime.GOOS != "darwin" {
			t.Skip("darwin only")
		}
		t.Setenv("XDG_DATA_HOME", "")

		usr, err := user.Current()
		if err != nil {
			t.Skip("cannot get current user")
		}

		want := filepath.Join(usr.HomeDir, "Library", "Application Support", "nenyactl", "nenya")
		got, err := ContainerDir()
		if err != nil {
			t.Fatalf("ContainerDir() error = %v", err)
		}
		if got != want {
			t.Errorf("ContainerDir() = %v, want %v", got, want)
		}
	})

	t.Run("windows uses LOCALAPPDATA", func(t *testing.T) {
		if runtime.GOOS != "windows" {
			t.Skip("windows only")
		}
		tmp := t.TempDir()
		t.Setenv("LOCALAPPDATA", tmp)

		want := filepath.Join(tmp, "nenyactl", "nenya")
		got, err := ContainerDir()
		if err != nil {
			t.Fatalf("ContainerDir() error = %v", err)
		}
		if got != want {
			t.Errorf("ContainerDir() = %v, want %v", got, want)
		}
	})
}

func TestSystemConfigDir(t *testing.T) {
	t.Run("linux defaults to /etc/nenya", func(t *testing.T) {
		if runtime.GOOS != "linux" {
			t.Skip("linux only")
		}
		got := SystemConfigDir()
		if got != "/etc/nenya" {
			t.Errorf("SystemConfigDir() = %v, want /etc/nenya", got)
		}
	})

	t.Run("darwin uses Library/Application Support", func(t *testing.T) {
		if runtime.GOOS != "darwin" {
			t.Skip("darwin only")
		}
		got := SystemConfigDir()
		if got != "/Library/Application Support/nenya" {
			t.Errorf("SystemConfigDir() = %v, want /Library/Application Support/nenya", got)
		}
	})
}

func TestSystemBinDir(t *testing.T) {
	t.Run("linux returns /usr/local/bin", func(t *testing.T) {
		if runtime.GOOS != "linux" {
			t.Skip("linux only")
		}
		got := SystemBinDir()
		if got != "/usr/local/bin" {
			t.Errorf("SystemBinDir() = %v, want /usr/local/bin", got)
		}
	})

	t.Run("darwin returns /usr/local/bin", func(t *testing.T) {
		if runtime.GOOS != "darwin" {
			t.Skip("darwin only")
		}
		got := SystemBinDir()
		if got != "/usr/local/bin" {
			t.Errorf("SystemBinDir() = %v, want /usr/local/bin", got)
		}
	})
}

func TestUserBinDir(t *testing.T) {
	t.Run("linux returns ~/.local/bin", func(t *testing.T) {
		if runtime.GOOS != "linux" {
			t.Skip("linux only")
		}
		usr, err := user.Current()
		if err != nil {
			t.Skip("cannot get current user")
		}
		want := filepath.Join(usr.HomeDir, ".local", "bin")
		got, err := UserBinDir()
		if err != nil {
			t.Fatalf("UserBinDir() error = %v", err)
		}
		if got != want {
			t.Errorf("UserBinDir() = %v, want %v", got, want)
		}
	})
}

func TestDetectRuntime(t *testing.T) {
	t.Run("prefers podman if available", func(t *testing.T) {
		tmp := t.TempDir()
		podmanScript := filepath.Join(tmp, "podman")
		if err := os.WriteFile(podmanScript, []byte("#!/bin/sh\nexit 0"), 0o755); err != nil {
			t.Fatalf("failed to create podman script: %v", err)
		}
		oldPath := os.Getenv("PATH")
		t.Cleanup(func() { os.Setenv("PATH", oldPath) })
		os.Setenv("PATH", tmp)

		r := DetectRuntime()
		if r != Podman {
			t.Errorf("DetectRuntime() = %v, want Podman", r)
		}
	})

	t.Run("falls back to docker if podman not available", func(t *testing.T) {
		oldPath := os.Getenv("PATH")
		t.Cleanup(func() { os.Setenv("PATH", oldPath) })
		os.Setenv("PATH", "/nonexistent")

		r := DetectRuntime()
		if r != Docker {
			t.Errorf("DetectRuntime() = %v, want Docker", r)
		}
	})
}

func TestComposeCmd(t *testing.T) {
	tmp := t.TempDir()
	podmanScript := filepath.Join(tmp, "podman")
	if err := os.WriteFile(podmanScript, []byte("#!/bin/sh\nexit 0"), 0o755); err != nil {
		t.Fatalf("failed to create podman script: %v", err)
	}
	oldPath := os.Getenv("PATH")
	t.Cleanup(func() { os.Setenv("PATH", oldPath) })
	os.Setenv("PATH", tmp)

	cmd, args, err := ComposeCmd()
	if err != nil {
		t.Fatalf("ComposeCmd() error = %v", err)
	}
	if cmd != "podman" {
		t.Errorf("ComposeCmd() cmd = %v, want podman", cmd)
	}
	if args[0] != "compose" {
		t.Errorf("ComposeCmd() args[0] = %v, want compose", args[0])
	}
}
