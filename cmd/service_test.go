package cmd

import (
	"errors"
	"fmt"
	"strings"
	"testing"
)

func TestRunServiceCommands(t *testing.T) {
	// These are tested via TestRunServiceWrappers now
}

func TestSystemctlWithExec(t *testing.T) {
	t.Run("calls systemctl with args", func(t *testing.T) {
		ex := fakeExec{
			cmdFn: func(name string, args ...string) cmdRunner {
				if name != "systemctl" {
					t.Errorf("expected systemctl, got %s", name)
				}
				if args[0] != "status" || args[1] != "test" {
					t.Errorf("unexpected args: %v", args)
				}
				return &fakeCmd{runFn: func() error { return nil }}
			},
		}
		if err := systemctlWithExec(ex, "status", "test"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("returns command error", func(t *testing.T) {
		ex := fakeExec{
			cmdFn: func(name string, args ...string) cmdRunner {
				return &fakeCmd{runFn: func() error { return errors.New("failed") }}
			},
		}
		err := systemctlWithExec(ex, "status", "test")
		if err == nil {
			t.Fatal("expected error")
		}
		if !strings.Contains(err.Error(), "systemctl") {
			t.Errorf("unexpected error: %v", err)
		}
	})
}

func TestLaunchctlWithExec(t *testing.T) {
	t.Run("calls launchctl with args", func(t *testing.T) {
		ex := fakeExec{
			cmdFn: func(name string, args ...string) cmdRunner {
				if name != "launchctl" {
					t.Errorf("expected launchctl, got %s", name)
				}
				if args[0] != "load" {
					t.Errorf("unexpected args: %v", args)
				}
				return &fakeCmd{runFn: func() error { return nil }}
			},
		}
		if err := launchctlWithExec(ex, "load", "test.plist"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("returns command error", func(t *testing.T) {
		ex := fakeExec{
			cmdFn: func(name string, args ...string) cmdRunner {
				return &fakeCmd{runFn: func() error { return fmt.Errorf("failed") }}
			},
		}
		err := launchctlWithExec(ex, "load", "test.plist")
		if err == nil {
			t.Fatal("expected error")
		}
		if !strings.Contains(err.Error(), "launchctl") {
			t.Errorf("unexpected error: %v", err)
		}
	})
}

func TestSystemctl(t *testing.T) {
	t.Run("delegates to defaultExec", func(t *testing.T) {
		saved := defaultExec
		defaultExec = fakeExec{
			cmdFn: func(name string, args ...string) cmdRunner {
				return &fakeCmd{runFn: func() error { return nil }}
			},
		}
		defer func() { defaultExec = saved }()
		if err := systemctl("start", "nenya"); err != nil {
			t.Fatalf("systemctl() error = %v", err)
		}
	})
}

func TestLaunchctl(t *testing.T) {
	t.Run("delegates to defaultExec", func(t *testing.T) {
		saved := defaultExec
		defaultExec = fakeExec{
			cmdFn: func(name string, args ...string) cmdRunner {
				return &fakeCmd{runFn: func() error { return nil }}
			},
		}
		defer func() { defaultExec = saved }()
		if err := launchctl("load", "test.plist"); err != nil {
			t.Fatalf("launchctl() error = %v", err)
		}
	})
}

func TestRunServiceWrappers(t *testing.T) {
	t.Run("start delegates to defaultExec", func(t *testing.T) {
		saved := defaultExec
		defaultExec = fakeExec{
			cmdFn: func(name string, args ...string) cmdRunner {
				return &fakeCmd{runFn: func() error { return nil }}
			},
		}
		defer func() { defaultExec = saved }()
		if err := runServiceStart(nil, nil); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("stop delegates to defaultExec", func(t *testing.T) {
		saved := defaultExec
		defaultExec = fakeExec{
			cmdFn: func(name string, args ...string) cmdRunner {
				return &fakeCmd{runFn: func() error { return nil }}
			},
		}
		defer func() { defaultExec = saved }()
		if err := runServiceStop(nil, nil); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("status delegates to defaultExec", func(t *testing.T) {
		saved := defaultExec
		defaultExec = fakeExec{
			cmdFn: func(name string, args ...string) cmdRunner {
				return &fakeCmd{runFn: func() error { return nil }}
			},
		}
		defer func() { defaultExec = saved }()
		if err := runServiceStatus(nil, nil); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("reload delegates to defaultExec", func(t *testing.T) {
		saved := defaultExec
		defaultExec = fakeExec{
			cmdFn: func(name string, args ...string) cmdRunner {
				return &fakeCmd{runFn: func() error { return nil }}
			},
		}
		defer func() { defaultExec = saved }()
		if err := runServiceReload(nil, nil); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})
}
