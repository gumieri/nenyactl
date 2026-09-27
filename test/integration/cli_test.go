package integration

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"unicode"

	"github.com/gumieri/nenyactl/internal/secrets"
)

// stripJSONComments removes // line comments from JSON, matching nenya's Config.StripComments.
func stripJSONComments(src []byte) []byte {
	var out []byte
	lines := strings.Split(string(src), "\n")
	for _, line := range lines {
		trimmed := strings.TrimLeftFunc(line, unicode.IsSpace)
		if strings.HasPrefix(trimmed, "//") {
			continue
		}
		// Also strip trailing // comments (but beware of // in strings)
		idx := strings.Index(trimmed, "//")
		if idx > 0 {
			// Check we're not inside a JSON string
			before := trimmed[:idx]
			if strings.Count(before, "\"")%2 == 0 {
				out = append(out, before...)
				out = append(out, '\n')
				continue
			}
		}
		out = append(out, line...)
		out = append(out, '\n')
	}
	return out
}

const nenyactlBin = "bin/nenyactl"

// TestMain always builds the CLI under test, so integration tests can never
// exercise a stale bin/nenyactl left by an earlier build.
func TestMain(m *testing.M) {
	cmd := exec.Command("go", "build", "-o", nenyactlBin, "./cmd/nenyactl/")
	cmd.Dir = filepath.Join("..", "..")
	if out, err := cmd.CombinedOutput(); err != nil {
		fmt.Fprintf(os.Stderr, "build nenyactl: %v\n%s\n", err, string(out))
		os.Exit(1)
	}
	os.Exit(m.Run())
}

// requireNenya skips a test when no nenya binary is installed. These tests
// drive the real CLI, which now writes config and secrets through nenya's
// contract commands; test/e2e covers the same path against a real release.
func requireNenya(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("nenya"); err != nil {
		t.Skip("nenya binary not installed; skipping contract-dependent integration test")
	}
}

func getBinPath() string {
	wd, _ := os.Getwd()
	return filepath.Join(wd, "..", "..", nenyactlBin)
}

func runNenyactl(t *testing.T, args ...string) (string, string, error) {
	t.Helper()

	cmd := exec.Command(getBinPath(), args...)
	stdout := &strings.Builder{}
	stderr := &strings.Builder{}
	cmd.Stdout = stdout
	cmd.Stderr = stderr

	err := cmd.Run()
	return stdout.String(), stderr.String(), err
}

func TestConfigInit(t *testing.T) {
	tmp := t.TempDir()

	out, stderr, err := runNenyactl(t, "config", "init", "--dir", tmp)
	if err != nil {
		t.Fatalf("nenyactl config init: %v\nstdout: %s\nstderr: %s", err, out, stderr)
	}

	configPath := filepath.Join(tmp, "config.json")
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		t.Error("config.json not created")
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("read config.json: %v", err)
	}

	stripped := stripJSONComments(data)
	var cfg map[string]any
	if err := json.Unmarshal(stripped, &cfg); err != nil {
		t.Errorf("config.json is not valid JSON (after stripping comments): %v", err)
	}
}

func TestContainerSetup(t *testing.T) {
	requireNenya(t)
	tmp := t.TempDir()

	out, stderr, err := runNenyactl(t, "containers", "setup", "--dir", tmp)
	if err != nil {
		if strings.Contains(stderr, "permission denied") {
			t.Skip("permission denied - possibly running as non-root in container")
		}
		t.Fatalf("nenyactl containers setup: %v\nstdout: %s\nstderr: %s", err, out, stderr)
	}

	composePath := filepath.Join(tmp, "compose.yml")
	if _, err := os.Stat(composePath); os.IsNotExist(err) {
		t.Error("compose.yml not created")
	}

	configDir := filepath.Join(tmp, "config")
	configPath := filepath.Join(configDir, "config.json")
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		t.Error("config/config.json not created")
	}

	secretsDir := filepath.Join(tmp, "secrets")
	if _, err := os.Stat(secretsDir); os.IsNotExist(err) {
		t.Error("secrets directory not created")
	}

	envPath := filepath.Join(tmp, ".env")
	if _, err := os.Stat(envPath); os.IsNotExist(err) {
		t.Error(".env not created")
	}
}

func TestContainerSetupWithDefaults(t *testing.T) {
	requireNenya(t)
	tmp := t.TempDir()

	_, stderr, err := runNenyactl(t, "containers", "setup", "--dir", tmp)
	if err != nil {
		if strings.Contains(stderr, "permission denied") {
			t.Skip("permission denied - possibly running as non-root in container")
		}
		t.Fatalf("nenyactl containers setup: %v", err)
	}

	composePath := filepath.Join(tmp, "compose.yml")
	data, err := os.ReadFile(composePath)
	if err != nil {
		t.Fatalf("read compose.yml: %v", err)
	}

	composeStr := string(data)
	if !strings.Contains(composeStr, "nenya:") {
		t.Error("compose.yml does not contain nenya service")
	}
	if !strings.Contains(composeStr, "ghcr.io/gumieri/nenya:latest") {
		t.Error("compose.yml does not contain correct image")
	}
	if !strings.Contains(composeStr, ":8080") {
		t.Error("compose.yml does not contain port mapping")
	}
}

// TestDirSemanticsConsistent asserts --dir has one meaning: a container layout
// is written to the nested paths everywhere, and create commands accept a
// directory that does not exist yet.
func TestDirSemanticsConsistent(t *testing.T) {
	t.Run("container layout writes nested paths", func(t *testing.T) {
		requireNenya(t)
		tmp := t.TempDir()
		if err := os.MkdirAll(filepath.Join(tmp, "config"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(tmp, "config", "config.json"), []byte(`{}`), 0o644); err != nil {
			t.Fatal(err)
		}

		// config init must write the file nenya actually reads.
		if _, stderr, err := runNenyactl(t, "config", "init", "--dir", tmp); err != nil {
			t.Fatalf("config init --dir (container): %v\n%s", err, stderr)
		}
		if _, err := os.Stat(filepath.Join(tmp, "config", "config.json")); err != nil {
			t.Errorf("expected config/config.json: %v", err)
		}
		if _, err := os.Stat(filepath.Join(tmp, "config.json")); err == nil {
			t.Error("wrote a root-level config.json nenya never reads")
		}

		// secret bootstrap must land where the generated compose mounts secrets.
		if _, stderr, err := runNenyactl(t, "secret", "bootstrap", "--dir", tmp); err != nil {
			t.Fatalf("secret bootstrap --dir (container): %v\n%s", err, stderr)
		}
		if secrets.ExistingTokenFile(filepath.Join(tmp, "secrets")) == "" {
			t.Errorf("expected a client token under secrets/")
		}
		if _, err := os.Stat(filepath.Join(tmp, "secrets.json")); err == nil {
			t.Error("wrote a root-level secrets.json the container never reads")
		}
	})

	t.Run("create commands accept a missing directory", func(t *testing.T) {
		requireNenya(t)
		base := t.TempDir()
		for _, tc := range []struct {
			args []string
			path string
		}{
			{[]string{"config", "init", "--dir", filepath.Join(base, "cfg")}, filepath.Join(base, "cfg", "config.json")},
			// create mode writes to the resolved secrets dir with a client file.
			{[]string{"secret", "bootstrap", "--dir", filepath.Join(base, "sec")}, filepath.Join(base, "sec", "secrets.json")},
		} {
			_, stderr, err := runNenyactl(t, tc.args...)
			if err != nil {
				t.Fatalf("%v: %v\n%s", tc.args, err, stderr)
			}
			if _, err := os.Stat(tc.path); err != nil {
				t.Errorf("%v did not create %s: %v", tc.args, tc.path, err)
			}
		}
	})
}

func TestVersionCommand(t *testing.T) {
	out, stderr, err := runNenyactl(t, "version")
	if err != nil {
		t.Fatalf("nenyactl version: %v\nstdout: %s\nstderr: %s", err, out, stderr)
	}

	if out == "" {
		t.Error("version output is empty")
	}

	if !strings.Contains(out, "nenyactl") && !strings.Contains(stderr, "nenyactl") {
		t.Error("version output does not contain 'nenyactl'")
	}
}

func TestSecretBootstrap(t *testing.T) {
	tmp := t.TempDir()

	configDir := filepath.Join(tmp, "config")
	secretsDir := filepath.Join(tmp, "secrets")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatalf("mkdir config: %v", err)
	}
	if err := os.MkdirAll(secretsDir, 0o700); err != nil {
		t.Fatalf("mkdir secrets: %v", err)
	}
	if err := os.WriteFile(filepath.Join(configDir, "config.json"), []byte(`{}`), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	requireNenya(t)

	// Run bootstrap
	if _, stderr, err := runNenyactl(t, "secret", "bootstrap", "--dir", tmp); err != nil {
		t.Fatalf("secret bootstrap: %v\n%s", err, stderr)
	}

	tokenPath := secrets.ExistingTokenFile(secretsDir)
	if tokenPath == "" {
		t.Fatal("secret bootstrap did not create a client token")
	}

	data, err := os.ReadFile(tokenPath)
	if err != nil {
		t.Fatalf("read client token: %v", err)
	}

	var client map[string]string
	if err := json.Unmarshal(data, &client); err != nil {
		t.Fatalf("unmarshal client token: %v", err)
	}

	if client["client_token"] == "" {
		t.Error("client_token is empty")
	}
}

func TestContainerStatus(t *testing.T) {
	tmp := t.TempDir()

	configDir := filepath.Join(tmp, "config")
	secretsDir := filepath.Join(tmp, "secrets")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatalf("mkdir config: %v", err)
	}
	if err := os.MkdirAll(secretsDir, 0o700); err != nil {
		t.Fatalf("mkdir secrets: %v", err)
	}
	if err := os.WriteFile(filepath.Join(configDir, "config.json"), []byte(`{}`), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmp, "compose.yml"), []byte(`services: {}`), 0o644); err != nil {
		t.Fatalf("write compose: %v", err)
	}

	startCmd := exec.Command(getBinPath(), "containers", "start", "--dir", tmp)
	if out, err := startCmd.CombinedOutput(); err != nil {
		t.Logf("start failed (no docker runtime): %v\n%s", err, string(out))
		t.Skip("container runtime not available")
	}

	time.Sleep(2 * time.Second)

	out, stderr, err := runNenyactl(t, "containers", "status", "--dir", tmp)
	if err != nil {
		t.Logf("status failed: %v\nstdout: %s\nstderr: %s", err, out, stderr)
	}

	stopCmd := exec.Command(getBinPath(), "containers", "stop", "--dir", tmp)
	if out, err := stopCmd.CombinedOutput(); err != nil {
		t.Logf("stop failed: %v\n%s", err, string(out))
	}
}
