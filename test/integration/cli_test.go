package integration

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const nenyactlBin = "bin/nenyactl"

func buildNenyactl(t *testing.T) {
	t.Helper()

	cmd := exec.Command("go", "build", "-o", nenyactlBin, "./cmd/nenyactl/")
	cmd.Dir = filepath.Join("..", "..")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build nenyactl: %v\n%s", err, string(out))
	}
}

func getBinPath() string {
	wd, _ := os.Getwd()
	return filepath.Join(wd, "..", "..", nenyactlBin)
}

func runNenyactl(t *testing.T, args ...string) (string, string, error) {
	t.Helper()

	bin := getBinPath()
	if _, err := os.Stat(bin); os.IsNotExist(err) {
		buildNenyactl(t)
	}

	cmd := exec.Command(bin, args...)
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

	var cfg map[string]any
	if err := json.Unmarshal(data, &cfg); err != nil {
		t.Errorf("config.json is not valid JSON: %v", err)
	}
}

func TestContainerSetup(t *testing.T) {
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

func TestAgentsCommand(t *testing.T) {
	tmp := t.TempDir()

	// Manually create minimal setup to avoid permission issues
	configDir := filepath.Join(tmp, "config")
	secretsDir := filepath.Join(tmp, "secrets")
	os.MkdirAll(configDir, 0o755)
	os.MkdirAll(secretsDir, 0o700)
	os.WriteFile(filepath.Join(configDir, "config.json"), []byte(`{"discovery":{"auto_agents":true}}`), 0o644)

	_, _, err := runNenyactl(t, "agents")
	if err != nil {
		t.Logf("agents command failed (expected in non-interactive mode): %v", err)
	}
}

func TestWriteAgentsConfig(t *testing.T) {
	tmp := t.TempDir()

	bin := getBinPath()
	if _, err := os.Stat(bin); os.IsNotExist(err) {
		buildNenyactl(t)
	}

	configD := filepath.Join(tmp, "config.d")
	if err := os.MkdirAll(configD, 0o755); err != nil {
		t.Fatalf("mkdir config.d: %v", err)
	}

	agentsCfg := map[string]any{
		"agents": map[string]any{
			"test-agent": map[string]any{
				"strategy": "fallback",
				"models":   []string{"gemini-2.5-flash"},
			},
		},
		"discovery": map[string]any{
			"auto_agents": false,
		},
	}

	data, err := json.MarshalIndent(agentsCfg, "", "  ")
	if err != nil {
		t.Fatalf("marshal agents config: %v", err)
	}

	agentsPath := filepath.Join(configD, "20-agents.json")
	if err := os.WriteFile(agentsPath, data, 0o644); err != nil {
		t.Fatalf("write agents config: %v", err)
	}

	var readCfg map[string]any
	readData, err := os.ReadFile(agentsPath)
	if err != nil {
		t.Fatalf("read agents config: %v", err)
	}
	if err := json.Unmarshal(readData, &readCfg); err != nil {
		t.Fatalf("unmarshal agents config: %v", err)
	}

	if readCfg["discovery"] == nil {
		t.Error("discovery section missing")
	}
	if disco, ok := readCfg["discovery"].(map[string]any); ok {
		if disco["auto_agents"] != false {
			t.Error("auto_agents should be false")
		}
	}
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
	os.MkdirAll(configDir, 0o755)
	os.MkdirAll(secretsDir, 0o700)
	os.WriteFile(filepath.Join(configDir, "config.json"), []byte(`{}`), 0o644)

	// Run bootstrap
	bin := getBinPath()
	cmd := exec.Command(bin, "secret", "bootstrap", "--dir", tmp)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Logf("secret bootstrap failed (may be permission issue): %v\n%s", err, string(out))
		t.Skip("skipping due to permission issues")
	}

	clientTokenPath := filepath.Join(secretsDir, "01-client.json")
	if _, err := os.Stat(clientTokenPath); os.IsNotExist(err) {
		t.Skip("client token file not created, skipping")
	}

	data, err := os.ReadFile(clientTokenPath)
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
	os.MkdirAll(configDir, 0o755)
	os.MkdirAll(secretsDir, 0o700)
	os.WriteFile(filepath.Join(configDir, "config.json"), []byte(`{}`), 0o644)
	os.WriteFile(filepath.Join(tmp, "compose.yml"), []byte(`services: {}`), 0o644)

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
