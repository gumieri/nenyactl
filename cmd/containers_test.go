package cmd

import (
	"bytes"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// fakeCmd implements cmdRunner.
type fakeCmd struct {
	dir     string
	stdout  io.Writer
	stderr  io.Writer
	runFn   func() error
}

func (f *fakeCmd) Dir(dir string) cmdRunner   { f.dir = dir; return f }
func (f *fakeCmd) Stdout(w io.Writer) cmdRunner { f.stdout = w; return f }
func (f *fakeCmd) Stderr(w io.Writer) cmdRunner { f.stderr = w; return f }
func (f *fakeCmd) Run() error                    { return f.runFn() }

// fakeExec implements execer.
type fakeExec struct {
	cmdFn func(name string, args ...string) cmdRunner
}

func (f fakeExec) Command(name string, args ...string) cmdRunner {
	if f.cmdFn == nil {
		panic("fakeExec.cmdFn is nil — you must set it")
	}
	return f.cmdFn(name, args...)
}

func TestRunContainerStartWithExec(t *testing.T) {
	t.Run("runs compose up -d", func(t *testing.T) {
		var ran bool
		ex := fakeExec{
			cmdFn: func(name string, args ...string) cmdRunner {
				if name != "podman" {
					t.Errorf("expected podman, got %s", name)
				}
				if len(args) < 2 || args[0] != "compose" || args[1] != "up" || args[2] != "-d" {
					t.Errorf("unexpected args: %v", args)
				}
				ran = true
				return &fakeCmd{runFn: func() error { return nil }}
			},
		}
		if err := runContainerStartWithExec(ex, "/tmp/test"); err != nil {
			t.Fatalf("runContainerStartWithExec() error = %v", err)
		}
		if !ran {
			t.Error("command was not executed")
		}
	})

	t.Run("works with empty dir (falls back to default)", func(t *testing.T) {
		var cmdName string
		ex := fakeExec{
			cmdFn: func(name string, args ...string) cmdRunner {
				cmdName = name
				return &fakeCmd{runFn: func() error { return nil }}
			},
		}
		err := runContainerStartWithExec(ex, "")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if cmdName == "" {
			t.Error("expected a command to be executed")
		}
	})

	t.Run("returns command error", func(t *testing.T) {
		ex := fakeExec{
			cmdFn: func(name string, args ...string) cmdRunner {
				return &fakeCmd{runFn: func() error { return errors.New("exec failed") }}
			},
		}
		err := runContainerStartWithExec(ex, "/tmp/test")
		if err == nil {
			t.Fatal("expected error, got nil")
		}
	})

	t.Run("returns error on ComposeCmd failure", func(t *testing.T) {
		t.Setenv("NENYACTL_RUNTIME", "invalid")
		err := runContainerStartWithExec(realExec{}, "")
		if err == nil {
			t.Fatal("expected error for invalid runtime, got nil")
		}
	})
}

func TestRealCmd(t *testing.T) {
	t.Run("Dir sets working directory", func(t *testing.T) {
		tmp := t.TempDir()
		cmd := exec.Command("pwd")
		rc := realCmd{Cmd: cmd}
		rc.Dir(tmp)
		if rc.Cmd.Dir != tmp {
			t.Errorf("expected dir %s, got %s", tmp, rc.Cmd.Dir)
		}
	})

	t.Run("Stdout sets writer", func(t *testing.T) {
		cmd := exec.Command("echo", "test")
		rc := realCmd{Cmd: cmd}
		var buf bytes.Buffer
		rc.Stdout(&buf)
		if rc.Cmd.Stdout != &buf {
			t.Error("Stdout not set")
		}
	})

	t.Run("Stderr sets writer", func(t *testing.T) {
		cmd := exec.Command("sh", "-c", "echo err >&2")
		rc := realCmd{Cmd: cmd}
		var buf bytes.Buffer
		rc.Stderr(&buf)
		if rc.Cmd.Stderr != &buf {
			t.Error("Stderr not set")
		}
	})

	t.Run("Run executes command", func(t *testing.T) {
		cmd := exec.Command("echo", "hello")
		rc := realCmd{Cmd: cmd}
		if err := rc.Run(); err != nil {
			t.Fatalf("Run() error = %v", err)
		}
	})

	t.Run("Run returns error on failure", func(t *testing.T) {
		cmd := exec.Command("false")
		rc := realCmd{Cmd: cmd}
		if err := rc.Run(); err == nil {
			t.Error("expected error, got nil")
		}
	})
}

func TestRealExec(t *testing.T) {
	t.Run("Command returns realCmd", func(t *testing.T) {
		ex := realExec{}
		rc := ex.Command("echo", "test")
		if _, ok := rc.(cmdRunner); !ok {
			t.Error("expected cmdRunner")
		}
		if rc.(realCmd).Cmd.Path == "" {
			t.Error("command not initialized")
		}
	})
}

func TestRunContainerSetup(t *testing.T) {
	t.Run("delegates to runContainerSetupWithExec", func(t *testing.T) {
		saved := defaultExec
		defaultExec = fakeExec{
			cmdFn: func(name string, args ...string) cmdRunner {
				return &fakeCmd{runFn: func() error { return nil }}
			},
		}
		defer func() { defaultExec = saved }()

		containerSetupCfg.dir = t.TempDir()
		containerSetupCfg.listenAddr = ":8080"
		containerSetupCfg.start = false
		if err := runContainerSetup(nil, nil); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})
}

func TestRunContainerStart(t *testing.T) {
	t.Run("delegates to runContainerStartWithExec", func(t *testing.T) {
		saved := defaultExec
		defaultExec = fakeExec{
			cmdFn: func(name string, args ...string) cmdRunner {
				return &fakeCmd{runFn: func() error { return nil }}
			},
		}
		defer func() { defaultExec = saved }()

		containerSetupCfg.dir = "/tmp/test"
		if err := runContainerStart(nil, nil); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})
}

func TestRunContainerStop(t *testing.T) {
	t.Run("delegates to runContainerStopWithExec", func(t *testing.T) {
		saved := defaultExec
		defaultExec = fakeExec{
			cmdFn: func(name string, args ...string) cmdRunner {
				return &fakeCmd{runFn: func() error { return nil }}
			},
		}
		defer func() { defaultExec = saved }()

		containerSetupCfg.dir = "/tmp/test"
		if err := runContainerStop(nil, nil); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})
}

func TestRunContainerStatus(t *testing.T) {
	t.Run("delegates to runContainerStatusWithExec", func(t *testing.T) {
		saved := defaultExec
		defaultExec = fakeExec{
			cmdFn: func(name string, args ...string) cmdRunner {
				return &fakeCmd{runFn: func() error { return nil }}
			},
		}
		defer func() { defaultExec = saved }()

		containerSetupCfg.dir = "/tmp/test"
		if err := runContainerStatus(nil, nil); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})
}

func TestRunContainerSetupWithExec(t *testing.T) {
	t.Run("creates directory structure and files", func(t *testing.T) {
		tmp := t.TempDir()
		err := runContainerSetupWithExec(nil, tmp, ":8080", false)
		if err != nil {
			t.Fatalf("runContainerSetupWithExec() error = %v", err)
		}

		if _, err := os.Stat(filepath.Join(tmp, "config", "config.json")); os.IsNotExist(err) {
			t.Error("config.json not created")
		}
		if _, err := os.Stat(filepath.Join(tmp, "secrets", "01-client.json")); os.IsNotExist(err) {
			t.Error("01-client.json not created")
		}
		if _, err := os.Stat(filepath.Join(tmp, "compose.yml")); os.IsNotExist(err) {
			t.Error("compose.yml not created")
		}
	})

	t.Run("calls start with startAfter=true", func(t *testing.T) {
		var startCalled bool
		ex := fakeExec{
			cmdFn: func(name string, args ...string) cmdRunner {
				if name == "podman" && len(args) > 1 && args[0] == "compose" {
					startCalled = true
				}
				return &fakeCmd{runFn: func() error { return nil }}
			},
		}
		tmp := t.TempDir()
		if err := runContainerSetupWithExec(ex, tmp, ":8080", true); err != nil {
			t.Fatalf("runContainerSetupWithExec() error = %v", err)
		}
		if !startCalled {
			t.Error("expected start command to be called")
		}
	})

	t.Run("returns error on invalid dir path", func(t *testing.T) {
		invalidDir := "/nonexistent/invalid/path/that/does/not/exist/and/should/fail/to/create"
		err := runContainerSetupWithExec(nil, invalidDir, ":8080", false)
		if err == nil {
			t.Fatal("expected error for invalid dir, got nil")
		}
	})

	t.Run("returns error on empty dir and container dir error", func(t *testing.T) {
		// Set XDG_DATA_HOME to non-existent path
		t.Setenv("XDG_DATA_HOME", "/nonexistent")
		err := runContainerSetupWithExec(nil, "", ":8080", false)
		if err == nil {
			t.Fatal("expected error for missing container dir, got nil")
		}
	})
}

func TestRunContainerStopWithExec(t *testing.T) {
	t.Run("runs compose down", func(t *testing.T) {
		ex := fakeExec{
			cmdFn: func(name string, args ...string) cmdRunner {
				if len(args) < 2 || args[0] != "compose" || args[1] != "down" {
					t.Errorf("unexpected args: %v", args)
				}
				return &fakeCmd{runFn: func() error { return nil }}
			},
		}
		if err := runContainerStopWithExec(ex, "/tmp/test"); err != nil {
			t.Fatalf("runContainerStopWithExec() error = %v", err)
		}
	})

	t.Run("returns command error", func(t *testing.T) {
		ex := fakeExec{
			cmdFn: func(name string, args ...string) cmdRunner {
				return &fakeCmd{runFn: func() error { return errors.New("stop failed") }}
			},
		}
		err := runContainerStopWithExec(ex, "/tmp/test")
		if err == nil {
			t.Fatal("expected error, got nil")
		}
	})

	t.Run("works with empty dir (falls back to default)", func(t *testing.T) {
		ex := fakeExec{
			cmdFn: func(name string, args ...string) cmdRunner {
				return &fakeCmd{runFn: func() error { return nil }}
			},
		}
		err := runContainerStopWithExec(ex, "")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("returns error on ComposeCmd failure", func(t *testing.T) {
		t.Setenv("NENYACTL_RUNTIME", "invalid")
		err := runContainerStopWithExec(realExec{}, "")
		if err == nil {
			t.Fatal("expected error for invalid runtime, got nil")
		}
	})
}

func TestRunContainerStatusWithExec(t *testing.T) {
	t.Run("runs compose ps", func(t *testing.T) {
		ex := fakeExec{
			cmdFn: func(name string, args ...string) cmdRunner {
				if len(args) < 2 || args[0] != "compose" || args[1] != "ps" {
					t.Errorf("unexpected args: %v", args)
				}
				return &fakeCmd{runFn: func() error { return nil }}
			},
		}
		err := runContainerStatusWithExec(ex, "/tmp/test")
		if err != nil {
			t.Fatalf("runContainerStatusWithExec() error = %v", err)
		}
	})

	t.Run("returns command error", func(t *testing.T) {
		ex := fakeExec{
			cmdFn: func(name string, args ...string) cmdRunner {
				return &fakeCmd{runFn: func() error { return errors.New("status failed") }}
			},
		}
		err := runContainerStatusWithExec(ex, "/tmp/test")
		if err == nil {
			t.Fatal("expected error, got nil")
		}
	})

	t.Run("works with empty dir (falls back to default)", func(t *testing.T) {
		ex := fakeExec{
			cmdFn: func(name string, args ...string) cmdRunner {
				return &fakeCmd{runFn: func() error { return nil }}
			},
		}
		err := runContainerStatusWithExec(ex, "")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("reads client token when available", func(t *testing.T) {
		tmp := t.TempDir()
		secretsDir := filepath.Join(tmp, "secrets")
		os.MkdirAll(secretsDir, 0o755)
		os.WriteFile(filepath.Join(secretsDir, "01-client.json"), []byte(`{"client_token": "nk-test123"}`), 0o600)

		ex := fakeExec{
			cmdFn: func(name string, args ...string) cmdRunner {
				return &fakeCmd{runFn: func() error { return nil }}
			},
		}
		if err := runContainerStatusWithExec(ex, tmp); err != nil {
			t.Fatalf("runContainerStatusWithExec() error = %v", err)
		}
	})
}
