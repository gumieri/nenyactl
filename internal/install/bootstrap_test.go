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
	"time"
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
	t.Run("routes through nenya secret set when supported", func(t *testing.T) {
		dir := t.TempDir()
		p := installPaths{configDir: dir, secretsFile: filepath.Join(dir, "secrets.json")}
		var calls []string
		r := scriptRunner{
			outputs: map[string]string{
				"/bin/nenya secret set -h":                                      "usage: nenya secret set …",
				"/bin/nenya secret set --config-dir " + dir + " --client-token": filepath.Join(dir, "secrets.json") + "\n",
			},
			calls: &calls,
		}
		created, err := bootstrapSecrets(context.Background(), r, "/bin/nenya", p)
		if err != nil || !created {
			t.Fatalf("created=%v err=%v", created, err)
		}
		joined := strings.Join(calls, "\n")
		// The write must be pinned to the install's config root: an untargeted
		// `secret set` would resolve nenya's §3.3 default instead.
		want := "/bin/nenya secret set --config-dir " + dir + " --client-token"
		if !strings.Contains(joined, want) {
			t.Errorf("the write must go through the single writer pinned to the root:\n want %q\n got  %s", want, joined)
		}
		// The shim must not also have written a file behind nenya's back.
		if _, err := os.Stat(p.secretsFile); !os.IsNotExist(err) {
			t.Errorf("nenyactl wrote %s itself: %v", p.secretsFile, err)
		}
	})

	t.Run("fails closed with the writer instead of hand-writing", func(t *testing.T) {
		dir := t.TempDir()
		p := installPaths{secretsFile: filepath.Join(dir, "secrets.json")}
		var calls []string
		r := scriptRunner{
			outputs: map[string]string{
				// Supported binary; the write fails (e.g. an active systemd
				// credential source). The shim must NOT kick in here.
				"/bin/nenya secret set -h": "usage: nenya secret set …",
			},
			calls: &calls,
		}
		created, err := bootstrapSecrets(context.Background(), r, "/bin/nenya", p)
		if err == nil || created {
			t.Fatalf("created=%v err=%v, want a surfaced failure", created, err)
		}
		if _, err := os.Stat(p.secretsFile); !os.IsNotExist(err) {
			t.Errorf("shim wrote a shadowed secrets file: %v", err)
		}
	})

	t.Run("falls back to a hand-written 0600 token without the writer", func(t *testing.T) {
		dir := t.TempDir()
		p := installPaths{secretsFile: filepath.Join(dir, "secrets.json")}
		// scriptRunner errors on every command: the writer probe is absent.
		var calls []string
		r := scriptRunner{calls: &calls}
		created, err := bootstrapSecrets(context.Background(), r, "/bin/nenya", p)
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

	t.Run("rejects a write that landed outside the config root", func(t *testing.T) {
		dir := t.TempDir()
		other := t.TempDir()
		p := installPaths{configDir: dir, secretsFile: filepath.Join(dir, "secrets.json")}
		r := scriptRunner{
			outputs: map[string]string{
				"/bin/nenya secret set -h": "usage: nenya secret set …",
				// An ambient NENYA_SECRETS_DIR redirected the write elsewhere.
				"/bin/nenya secret set --config-dir " + dir + " --client-token": filepath.Join(other, "secrets.json") + "\n",
			},
		}
		created, err := bootstrapSecrets(context.Background(), r, "/bin/nenya", p)
		if err == nil || created {
			t.Fatalf("created=%v err=%v, want a surfaced misdirection error", created, err)
		}
		if !strings.Contains(err.Error(), other) {
			t.Errorf("err = %v, want it to name the misdirected path", err)
		}
	})

	t.Run("complements the guard with the local scan when the reader errors", func(t *testing.T) {
		// Writer supported, reader supported-but-failing, and a local token in
		// a merge file (NOT the nominal target): the complement must prevent
		// rotation — the nominal-target stat guard does not fire here.
		dir := t.TempDir()
		p := installPaths{
			configDir:   dir,
			secretsFile: filepath.Join(dir, "secrets.json"),
			secretsDir:  dir,
		}
		if err := os.WriteFile(filepath.Join(dir, "01-client.json"), []byte(`{"client_token":"nk-existing"}`), 0o600); err != nil {
			t.Fatal(err)
		}
		var calls []string
		r := scriptRunner{
			outputs: map[string]string{
				"/bin/nenya secret set -h": "usage: nenya secret set …",
				// Reader exists but fails (e.g. an invalid sibling document).
				"/bin/nenya secret get --client-token --config-dir " + dir: "",
			},
			calls: &calls,
		}
		created, err := bootstrapSecrets(context.Background(), r, "/bin/nenya", p)
		if err != nil || created {
			t.Fatalf("created=%v err=%v, want a skipped bootstrap", created, err)
		}
		if _, err := os.Stat(p.secretsFile); !os.IsNotExist(err) {
			t.Errorf("bootstrap wrote %s over an existing merge token: %v", p.secretsFile, err)
		}
		data, _ := os.ReadFile(filepath.Join(dir, "01-client.json"))
		if !strings.Contains(string(data), "nk-existing") {
			t.Errorf("existing token changed: %q", data)
		}
		for _, c := range calls {
			if strings.Contains(c, "secret set --client-token") {
				t.Errorf("install rotated the token via %q", c)
			}
		}
	})

	t.Run("does not rotate existing token", func(t *testing.T) {
		dir := t.TempDir()
		p := installPaths{secretsFile: filepath.Join(dir, "secrets.json")}
		_ = os.WriteFile(p.secretsFile, []byte(`{"client_token":"nk-existing"}`), 0o600)
		r := scriptRunner{}
		created, err := bootstrapSecrets(context.Background(), r, "/bin/nenya", p)
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
	// bootstrap + service side effects, which need systemctl faked too. The
	// installed binary pretends to lack `secret get`, so the documented
	// hand-write shim creates the token file this test asserts on.
	var calls []string
	runner := multiRunner{
		fallback: scriptRunner{outputs: r.outputs, calls: &calls},
		failKeys: []string{dest + " secret set -h"},
	}

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

func TestInstallSecretsDirFromPaths(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("systemd install path is linux-only")
	}

	archive := containTarGz(t, map[string]string{
		"nenya":                "fake-binary-content",
		"deploy/nenya.service": "[Unit]\nDescription=nenya\nLoadCredential=secrets:/etc/nenya/secrets.json",
		"deploy/nenya.socket":  "[Socket]\nListenStream=8080",
	})
	server := releaseServer(t, archive, "v0.0.0-test")
	defer server.Close()
	pointAtServer(t, server)

	tmp := t.TempDir()
	binDir := filepath.Join(tmp, "bin")
	configDir := filepath.Join(tmp, "cfg")
	unitDir := filepath.Join(tmp, "systemd")
	dest := filepath.Join(binDir, "nenya")
	pathsJSON := `{"mode":"directory","config_dir":"` + configDir + `","config_file":"` + configDir + `/config.json","secrets_dir":"/run/secrets/nenya","platform":"linux"}`

	var calls []string
	runner := multiRunner{
		fallback: scriptRunner{outputs: map[string]string{
			dest + " example-config": `{"server":{"listen_addr":":8080"}}`,
			dest + " paths --json":   pathsJSON,
		}, calls: &calls},
		failKeys: []string{dest + " secret set -h"},
	}

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

	// The token lands in the config root (what the shipped unit loads), never
	// in the wildcard secrets merge directory.
	if _, err := os.Stat(filepath.Join(configDir, "secrets.json")); err != nil {
		t.Errorf("expected %s/secrets.json: %v", configDir, err)
	}
	if _, err := os.Stat("/run/secrets/nenya/secrets.json"); err == nil {
		t.Error("install wrote a token into the wildcard secrets_dir")
	}
}

func TestInstallRoutesSecretsThroughSecretSet(t *testing.T) {
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
	configDir := filepath.Join(tmp, "cfg")
	unitDir := filepath.Join(tmp, "systemd")
	dest := filepath.Join(binDir, "nenya")

	var calls []string
	// The installed binary supports the writer surface (secret set -h exits 0),
	// so the write must be delegated to `secret set` — nenyactl never writes
	// the secrets file itself on this path.
	runner := multiRunner{fallback: scriptRunner{outputs: map[string]string{
		dest + " example-config": `{"server":{"listen_addr":":8080"}}`,
		dest + " secret set -h":  "usage: nenya secret set …",
		dest + " secret get --client-token --config-dir " + configDir:      "",
		dest + " secret set --config-dir " + configDir + " --client-token": filepath.Join(configDir, "secrets.json") + "\n",
	}, calls: &calls}}

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

	want := dest + " secret set --config-dir " + configDir + " --client-token"
	found := false
	for _, c := range calls {
		if c == want {
			found = true
		}
	}
	if !found {
		t.Errorf("fresh-install token was not routed through the pinned single writer; want %q in:\n%s", want, strings.Join(calls, "\n"))
	}
}

func TestInstallUserRoutesSecretsThroughSecretSet(t *testing.T) {
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

	var calls []string
	dest := filepath.Join(tmp, ".local", "bin", "nenya")
	userRoot := filepath.Join(tmp, ".config", "nenya")
	runner := multiRunner{fallback: scriptRunner{outputs: map[string]string{
		dest + " example-config": `{"server":{"listen_addr":":8080"}}`,
		dest + " secret set -h":  "usage: nenya secret set …",
		dest + " secret get --client-token --config-dir " + userRoot:      "",
		dest + " secret set --config-dir " + userRoot + " --client-token": filepath.Join(userRoot, "secrets.json") + "\n",
	}, calls: &calls}}

	cfg := Config{Version: "v0.0.0-test", UserInstall: true, SkipVerify: true}
	if err := InstallWithHTTPAndRunner(context.Background(), cfg, server.Client(), runner); err != nil {
		t.Fatalf("user install: %v", err)
	}

	// A user install must pin the USER config root: an untargeted writer
	// would land on (or rotate) the system deployment's token.
	want := dest + " secret set --config-dir " + userRoot + " --client-token"
	found := false
	for _, c := range calls {
		if c == want {
			found = true
		}
	}
	if !found {
		t.Errorf("user install did not pin the writer to %s; want %q in:\n%s", userRoot, want, strings.Join(calls, "\n"))
	}
}

// blockingRunner blocks until the context is done, simulating a released binary
// that ignores an unknown subcommand and starts a server.
type blockingRunner struct{}

func (blockingRunner) Output(ctx context.Context, _ string, _ ...string) ([]byte, error) {
	<-ctx.Done()
	return nil, ctx.Err()
}

func TestProbeOutputIsBounded(t *testing.T) {
	saved := probeTimeout
	probeTimeout = 50 * time.Millisecond
	t.Cleanup(func() { probeTimeout = saved })
	start := time.Now()
	_, err := probeOutput(context.Background(), blockingRunner{}, "/bin/nenya", "describe", "--json")
	if err == nil {
		t.Fatal("expected a timeout error")
	}
	elapsed := time.Since(start)
	if elapsed < probeTimeout {
		t.Errorf("probe returned in %s, want it bounded at %s", elapsed, probeTimeout)
	}
	if elapsed > probeTimeout+2*time.Second {
		t.Fatalf("probe took %s, want bounded near %s", elapsed, probeTimeout)
	}
}

func TestBootstrapConfigContentFallback(t *testing.T) {
	t.Run("uses example-config when available", func(t *testing.T) {
		r := scriptRunner{outputs: map[string]string{"/bin/nenya example-config": `{"from":"nenya"}`}}
		if got := BootstrapConfigContent(context.Background(), r, "/bin/nenya"); string(got) != `{"from":"nenya"}` {
			t.Errorf("got %q", got)
		}
	})

	t.Run("falls back to the minimal shim", func(t *testing.T) {
		if got := BootstrapConfigContent(context.Background(), scriptRunner{}, "/bin/nenya"); string(got) != minimalConfig {
			t.Errorf("got %q, want the minimal shim", got)
		}
	})
}

func TestQueryNenyaPathsTimesOut(t *testing.T) {
	saved := probeTimeout
	probeTimeout = 50 * time.Millisecond
	t.Cleanup(func() { probeTimeout = saved })
	if _, ok := queryNenyaPaths(context.Background(), blockingRunner{}, "/bin/nenya"); ok {
		t.Fatal("a hanging probe must be treated as absent")
	}
}

// multiRunner dispatches by command name to a per-name runner, recording calls.
// Unknown commands succeed with "ok" (cosign, systemctl, …), except keys in
// failKeys, which fail — used to simulate a binary without a given subcommand.
type multiRunner struct {
	fallback scriptRunner
	failKeys []string
}

func (m multiRunner) Output(ctx context.Context, name string, args ...string) ([]byte, error) {
	key := strings.Join(append([]string{name}, args...), " ")
	for _, k := range m.failKeys {
		if k == key {
			return nil, fmt.Errorf("%s: not supported", key)
		}
	}
	if out, ok := m.fallback.outputs[key]; ok {
		if m.fallback.calls != nil {
			*m.fallback.calls = append(*m.fallback.calls, key)
		}
		return []byte(out), nil
	}
	// systemctl/launchctl and cosign succeed; other probe commands succeed too.
	if m.fallback.calls != nil {
		*m.fallback.calls = append(*m.fallback.calls, key)
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
	runner := multiRunner{
		fallback: scriptRunner{
			outputs: map[string]string{dest + " example-config": `{"server":{"listen_addr":":8080"}}`},
			calls:   &calls,
		},
		failKeys: []string{dest + " secret set -h"},
	}

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
		runner := multiRunner{fallback: scriptRunner{
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
		runner := multiRunner{fallback: scriptRunner{
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
		runner := multiRunner{fallback: scriptRunner{
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
		runner := multiRunner{fallback: scriptRunner{outputs: map[string]string{}, calls: &calls}}
		if err := checkInstalledContract(context.Background(), runner, dest); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})
}
