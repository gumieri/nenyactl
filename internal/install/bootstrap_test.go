package install

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// scriptRunner returns canned output per full command line and records calls.
type scriptRunner struct {
	outputs map[string]string
	calls   *[]string
}

func (r scriptRunner) Output(_ context.Context, name string, args ...string) ([]byte, error) {
	key := strings.Join(append([]string{name}, args...), " ")
	if r.calls != nil {
		*r.calls = append(*r.calls, key)
	}
	if out, ok := r.outputs[key]; ok {
		return []byte(out), nil
	}
	return nil, fmt.Errorf("no output for %q", key)
}

func TestQueryNenyaPaths(t *testing.T) {
	t.Run("parses paths json", func(t *testing.T) {
		body := `{"mode":"directory","config_dir":"/custom/nenya","config_file":"/custom/nenya/config.json","secrets_dir":"/run/secrets/nenya","platform":"linux"}`
		r := scriptRunner{outputs: map[string]string{"/bin/nenya paths --json": body}}
		p, ok := queryNenyaPaths(context.Background(), r, "/bin/nenya")
		if !ok || p.ConfigDir != "/custom/nenya" {
			t.Fatalf("got %+v ok=%v", p, ok)
		}
	})

	t.Run("feature-detect failure is not fatal", func(t *testing.T) {
		if _, ok := queryNenyaPaths(context.Background(), scriptRunner{}, "/bin/nenya"); ok {
			t.Fatal("expected ok=false when paths --json is unavailable")
		}
	})
}

func TestBootstrapConfig(t *testing.T) {
	t.Run("uses nenya example-config when available", func(t *testing.T) {
		dir := t.TempDir()
		p := installPaths{configDir: dir, configFile: filepath.Join(dir, "config.json")}
		r := scriptRunner{outputs: map[string]string{"/bin/nenya example-config": `{"from":"nenya"}`}}
		created, err := bootstrapConfig(context.Background(), r, "/bin/nenya", p)
		if err != nil || !created {
			t.Fatalf("created=%v err=%v", created, err)
		}
		data, _ := os.ReadFile(p.configFile)
		if string(data) != `{"from":"nenya"}` {
			t.Errorf("config = %q", data)
		}
	})

	t.Run("falls back to minimal config", func(t *testing.T) {
		dir := t.TempDir()
		p := installPaths{configDir: dir, configFile: filepath.Join(dir, "config.json")}
		created, err := bootstrapConfig(context.Background(), scriptRunner{}, "/bin/nenya", p)
		if err != nil || !created {
			t.Fatalf("created=%v err=%v", created, err)
		}
		data, _ := os.ReadFile(p.configFile)
		var v map[string]any
		if err := json.Unmarshal(data, &v); err != nil {
			t.Fatalf("minimal config is not valid JSON: %v (%s)", err, data)
		}
	})

	t.Run("does not overwrite existing", func(t *testing.T) {
		dir := t.TempDir()
		p := installPaths{configDir: dir, configFile: filepath.Join(dir, "config.json")}
		_ = os.WriteFile(p.configFile, []byte(`{"keep":true}`), 0o644)
		created, err := bootstrapConfig(context.Background(), scriptRunner{}, "/bin/nenya", p)
		if err != nil || created {
			t.Fatalf("created=%v err=%v", created, err)
		}
		data, _ := os.ReadFile(p.configFile)
		if string(data) != `{"keep":true}` {
			t.Errorf("existing config overwritten: %q", data)
		}
	})
}

func TestBootstrapSecrets(t *testing.T) {
	t.Run("creates 0600 token without printing it", func(t *testing.T) {
		dir := t.TempDir()
		p := installPaths{secretsFile: filepath.Join(dir, "secrets.json")}
		created, err := bootstrapSecrets(p)
		if err != nil || !created {
			t.Fatalf("created=%v err=%v", created, err)
		}
		info, err := os.Stat(p.secretsFile)
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm() != 0o600 {
			t.Errorf("mode = %o, want 0600", info.Mode().Perm())
		}
		var s struct {
			ClientToken string `json:"client_token"`
		}
		data, _ := os.ReadFile(p.secretsFile)
		if err := json.Unmarshal(data, &s); err != nil {
			t.Fatal(err)
		}
		if !strings.HasPrefix(s.ClientToken, "nk-") || len(s.ClientToken) < 20 {
			t.Errorf("unexpected client token: %q", s.ClientToken)
		}
	})

	t.Run("does not rotate existing token", func(t *testing.T) {
		dir := t.TempDir()
		p := installPaths{secretsFile: filepath.Join(dir, "secrets.json")}
		_ = os.WriteFile(p.secretsFile, []byte(`{"client_token":"nk-existing"}`), 0o600)
		created, err := bootstrapSecrets(p)
		if err != nil || created {
			t.Fatalf("created=%v err=%v", created, err)
		}
		data, _ := os.ReadFile(p.secretsFile)
		if !strings.Contains(string(data), "nk-existing") {
			t.Errorf("existing token changed: %q", data)
		}
	})
}

func TestEnableService(t *testing.T) {
	t.Run("systemd reloads and enables the socket", func(t *testing.T) {
		var calls []string
		enableService(context.Background(), scriptRunner{calls: &calls}, "/etc/systemd/system")
		joined := strings.Join(calls, "\n")
		if !strings.Contains(joined, "systemctl daemon-reload") {
			t.Errorf("missing daemon-reload: %s", joined)
		}
		if !strings.Contains(joined, "systemctl enable --now nenya.socket") {
			t.Errorf("missing enable: %s", joined)
		}
	})

	t.Run("launchd loads the plist", func(t *testing.T) {
		var calls []string
		enableService(context.Background(), scriptRunner{calls: &calls}, "/Library/LaunchDaemons")
		if len(calls) != 1 || !strings.Contains(calls[0], "launchctl load -w") {
			t.Errorf("unexpected calls: %v", calls)
		}
	})

	t.Run("no unit dir is a no-op", func(t *testing.T) {
		var calls []string
		enableService(context.Background(), scriptRunner{calls: &calls}, "")
		if len(calls) != 0 {
			t.Errorf("expected no calls, got %v", calls)
		}
	})
}

func TestInstallSystemBootstraps(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("systemd install path is linux-only")
	}

	archive := containTarGz(t, map[string]string{
		"nenya":                "fake-binary-content",
		"deploy/nenya.service": "[Unit]\nDescription=nenya",
		"deploy/nenya.socket":  "[Socket]\nListenStream=8080",
	})
	server := releaseServer(t, archive, "v0.0.0-test")
	defer server.Close()
	pointAtServer(t, server)

	tmp := t.TempDir()
	binDir := filepath.Join(tmp, "bin")
	configDir := filepath.Join(tmp, "etc", "nenya")
	unitDir := filepath.Join(tmp, "systemd")

	dest := filepath.Join(binDir, "nenya")
	r := scriptRunner{outputs: map[string]string{
		dest + " example-config": `{"server":{"listen_addr":":8080"}}`,
	}}
	// Cosign verification is exercised separately; this test focuses on the
	// bootstrap + service side effects, which need systemctl faked too.
	var calls []string
	runner := multiRunner{scriptRunner{outputs: r.outputs, calls: &calls}}

	cfg := Config{
		Version:           "v0.0.0-test",
		SkipVerify:        true,
		binDirOverride:    binDir,
		configDirOverride: configDir,
		unitDirOverride:   unitDir,
	}
	if err := InstallWithHTTPAndRunner(context.Background(), cfg, server.Client(), runner); err != nil {
		t.Fatalf("install: %v", err)
	}

	for _, f := range []string{
		filepath.Join(binDir, "nenya"),
		filepath.Join(configDir, "config.json"),
		filepath.Join(configDir, "secrets.json"),
		filepath.Join(unitDir, "nenya.service"),
		filepath.Join(unitDir, "nenya.socket"),
	} {
		if _, err := os.Stat(f); err != nil {
			t.Errorf("expected %s: %v", f, err)
		}
	}

	joined := strings.Join(calls, "\n")
	if !strings.Contains(joined, "systemctl daemon-reload") || !strings.Contains(joined, "systemctl enable --now nenya.socket") {
		t.Errorf("service was not enabled; calls: %s", joined)
	}
}

// multiRunner dispatches by command name to a per-name runner, recording calls.
type multiRunner struct {
	fallback scriptRunner
}

func (m multiRunner) Output(ctx context.Context, name string, args ...string) ([]byte, error) {
	if out, ok := m.fallback.outputs[strings.Join(append([]string{name}, args...), " ")]; ok {
		if m.fallback.calls != nil {
			*m.fallback.calls = append(*m.fallback.calls, strings.Join(append([]string{name}, args...), " "))
		}
		return []byte(out), nil
	}
	// systemctl/launchctl and cosign succeed; other probe commands succeed too.
	if m.fallback.calls != nil {
		*m.fallback.calls = append(*m.fallback.calls, strings.Join(append([]string{name}, args...), " "))
	}
	return []byte("ok"), nil
}

func TestInstallUserWritesNoSystemUnits(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("user install path differs on windows")
	}

	archive := containTarGz(t, map[string]string{
		"nenya":                "fake-binary-content",
		"deploy/nenya.service": "[Unit]\nDescription=nenya",
		"deploy/nenya.socket":  "[Socket]\nListenStream=8080",
	})
	server := releaseServer(t, archive, "v0.0.0-test")
	defer server.Close()
	pointAtServer(t, server)

	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("XDG_CONFIG_HOME", "")

	trapUnit := filepath.Join(tmp, "trap-systemd")
	trapConfig := filepath.Join(tmp, "trap-etc")
	savedUnit, savedConfig := systemUnitDir, systemConfigDir
	systemUnitDir = func() string { return trapUnit }
	systemConfigDir = func() string { return trapConfig }
	t.Cleanup(func() { systemUnitDir, systemConfigDir = savedUnit, savedConfig })

	var calls []string
	dest := filepath.Join(tmp, ".local", "bin", "nenya")
	runner := multiRunner{scriptRunner{
		outputs: map[string]string{dest + " example-config": `{"server":{"listen_addr":":8080"}}`},
		calls:   &calls,
	}}

	cfg := Config{Version: "v0.0.0-test", UserInstall: true, SkipVerify: true}
	if err := InstallWithHTTPAndRunner(context.Background(), cfg, server.Client(), runner); err != nil {
		t.Fatalf("user install: %v", err)
	}

	if entries, err := os.ReadDir(trapUnit); err == nil && len(entries) != 0 {
		t.Errorf("user install wrote system units: %v", entries)
	}
	if _, err := os.Stat(trapConfig); !os.IsNotExist(err) {
		t.Errorf("user install touched system config dir %s", trapConfig)
	}
	for _, c := range calls {
		if strings.HasPrefix(c, "systemctl") || strings.HasPrefix(c, "launchctl") {
			t.Errorf("user install invoked a system service manager: %s", c)
		}
	}

	for _, f := range []string{
		filepath.Join(tmp, ".local", "bin", "nenya"),
		filepath.Join(tmp, ".config", "nenya", "config.json"),
		filepath.Join(tmp, ".config", "nenya", "secrets.json"),
	} {
		if _, err := os.Stat(f); err != nil {
			t.Errorf("expected user artifact %s: %v", f, err)
		}
	}

	data, err := os.ReadFile(filepath.Join(tmp, ".config", "nenya", "config.json"))
	if err != nil {
		t.Fatal(err)
	}
	var v map[string]any
	if err := json.Unmarshal(data, &v); err != nil {
		t.Errorf("user config is not valid JSON: %v (%s)", err, data)
	}
}

func TestCheckInstalledContract(t *testing.T) {
	t.Run("fails closed on an unsupported contract", func(t *testing.T) {
		var calls []string
		dest := "/bin/nenya"
		runner := multiRunner{scriptRunner{
			outputs: map[string]string{dest + " describe --json": `{"contract_version": 99}`},
			calls:   &calls,
		}}
		err := checkInstalledContract(context.Background(), runner, dest)
		if err == nil || !strings.Contains(err.Error(), "contract_version") {
			t.Fatalf("expected contract error, got %v", err)
		}
	})

	t.Run("accepts a supported contract", func(t *testing.T) {
		var calls []string
		dest := "/bin/nenya"
		runner := multiRunner{scriptRunner{
			outputs: map[string]string{dest + " describe --json": `{"contract_version": 1}`},
			calls:   &calls,
		}}
		if err := checkInstalledContract(context.Background(), runner, dest); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("feature-detects through version --json", func(t *testing.T) {
		var calls []string
		dest := "/bin/nenya"
		runner := multiRunner{scriptRunner{
			outputs: map[string]string{dest + " version --json": `{"version":"0.15.0","contract_version":1}`},
			calls:   &calls,
		}}
		if err := checkInstalledContract(context.Background(), runner, dest); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("no version surface is accepted", func(t *testing.T) {
		var calls []string
		dest := "/bin/nenya"
		runner := multiRunner{scriptRunner{outputs: map[string]string{}, calls: &calls}}
		if err := checkInstalledContract(context.Background(), runner, dest); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})
}
