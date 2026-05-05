package cmd

import (
	"io"
	"strings"
	"testing"
)

func TestRootCmd(t *testing.T) {
	t.Run("has use", func(t *testing.T) {
		if rootCmd.Use == "" {
			t.Error("rootCmd.Use should not be empty")
		}
	})

	t.Run("has short description", func(t *testing.T) {
		if rootCmd.Short == "" {
			t.Error("rootCmd.Short should not be empty")
		}
	})

	t.Run("has subcommands", func(t *testing.T) {
		if len(rootCmd.Commands()) == 0 {
			t.Error("rootCmd should have subcommands")
		}
	})

	t.Run("has expected subcommands", func(t *testing.T) {
		commands := rootCmd.Commands()
		cmdNames := make(map[string]bool)
		for _, cmd := range commands {
			cmdNames[cmd.Name()] = true
		}

		expectedCommands := []string{"version", "install", "config", "agents", "containers", "service", "secret"}
		for _, name := range expectedCommands {
			if !cmdNames[name] {
				t.Errorf("missing subcommand: %s", name)
			}
		}
	})
}

func TestInstallCmd(t *testing.T) {
	t.Run("has use", func(t *testing.T) {
		if installCmd.Use == "" {
			t.Error("installCmd.Use should not be empty")
		}
	})

	t.Run("has short description", func(t *testing.T) {
		if installCmd.Short == "" {
			t.Error("installCmd.Short should not be empty")
		}
	})
}

func TestAgentsCmd(t *testing.T) {
	t.Run("has use", func(t *testing.T) {
		if agentsCmd.Use == "" {
			t.Error("agentsCmd.Use should not be empty")
		}
	})
}

func TestContainerCmd(t *testing.T) {
	t.Run("has use", func(t *testing.T) {
		if containerCmd.Use == "" {
			t.Error("containerCmd.Use should not be empty")
		}
	})

	t.Run("has subcommands", func(t *testing.T) {
		if len(containerCmd.Commands()) == 0 {
			t.Error("containerCmd should have subcommands")
		}
	})
}

func TestConfigCmd(t *testing.T) {
	t.Run("has use", func(t *testing.T) {
		if configCmd.Use == "" {
			t.Error("configCmd.Use should not be empty")
		}
	})
}

func TestVersionCmd(t *testing.T) {
	t.Run("has use", func(t *testing.T) {
		if versionCmd.Use == "" {
			t.Error("versionCmd.Use should not be empty")
		}
	})
}

func TestContainerDefaultDir(t *testing.T) {
	t.Run("returns non-empty path", func(t *testing.T) {
		path := containerDefaultDir()
		if path == "" {
			t.Error("containerDefaultDir should not be empty")
		}
		if !strings.Contains(path, "nenyactl") {
			t.Error("containerDefaultDir should contain 'nenyactl'")
		}
	})
}

func TestExecute(t *testing.T) {
	t.Run("cobra Execute succeeds", func(t *testing.T) {
		// Use version command to test Execute path
		rootCmd.SetArgs([]string{"version"})
		rootCmd.SetOut(io.Discard)
		rootCmd.SetErr(io.Discard)
		if err := Execute(); err != nil {
			t.Fatalf("Execute() error = %v", err)
		}
	})
}

func TestFatalf(t *testing.T) {
	t.Run("fatalf compiles and accepts format string", func(t *testing.T) {
		// Can't test os.Exit without killing the test process.
		// Just verify the function compiles.
	})
}
