package nenya

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// fakeRunner records the last invocation and returns canned output.
type fakeRunner struct {
	args []string
	out  []byte
	err  error
}

func (f *fakeRunner) Output(_ context.Context, args ...string) ([]byte, error) {
	f.args = append([]string(nil), args...)
	return f.out, f.err
}

func equalArgs(a, b []string) bool {
	return strings.Join(a, "\x00") == strings.Join(b, "\x00")
}

func TestPathsTargetsConfigDir(t *testing.T) {
	fr := &fakeRunner{out: []byte(`{"mode":"file","config_dir":"/etc/nenya","config_file":"/etc/nenya/config.json","config_d":"/etc/nenya/config.d","secrets_dir":"/etc/nenya/secrets","socket_path":"/run/nenya.sock","platform":"linux"}`)}
	c := New(fr).WithConfigDir("/tmp/cfg")

	p, err := c.Paths(context.Background())
	if err != nil {
		t.Fatalf("Paths: %v", err)
	}
	want := []string{"paths", "--json", "--config-dir", "/tmp/cfg"}
	if !equalArgs(fr.args, want) {
		t.Errorf("args = %v, want %v", fr.args, want)
	}
	if p.ConfigDir != "/etc/nenya" || p.SecretsDir != "/etc/nenya/secrets" || p.Platform != "linux" {
		t.Errorf("parsed Paths = %+v", p)
	}
	if p.SocketPath == nil || *p.SocketPath != "/run/nenya.sock" {
		t.Errorf("socket_path = %v", p.SocketPath)
	}
}

func TestTargetSelectsConfigFile(t *testing.T) {
	fr := &fakeRunner{out: []byte(`{}`)}
	c := New(fr).WithConfigFile("/tmp/config.json")

	if _, err := c.Describe(context.Background()); err != nil {
		t.Fatalf("Describe: %v", err)
	}
	want := []string{"describe", "--json", "--config", "/tmp/config.json"}
	if !equalArgs(fr.args, want) {
		t.Errorf("args = %v, want %v", fr.args, want)
	}
}

func TestDescribeParsesEffectiveConfig(t *testing.T) {
	fr := &fakeRunner{out: []byte(`{
	  "contract_version": 1,
	  "version": {"version": "0.15.0", "contract_version": 1},
	  "paths": {"mode": "file", "config_dir": "/etc/nenya"},
	  "secrets": {"active_source": "/run/secrets/nenya/01-client.json", "searched": ["a", "b"]},
	  "config": {"server": {"listen_addr": ":8080"}},
	  "providers": {"configured": ["openai"], "catalog": [{"provider": "openai", "model": "gpt", "context_window": 128000, "max_output": 4096}]},
	  "diagnostics": [{"level": "warn", "code": "x", "message": "m", "source": "s"}]
	}`)}
	c := New(fr)

	d, err := c.Describe(context.Background())
	if err != nil {
		t.Fatalf("Describe: %v", err)
	}
	if d.ContractVersion != 1 || d.Version.Version != "0.15.0" {
		t.Errorf("version surface = %+v", d)
	}
	if d.Secrets.ActiveSource == "" || len(d.Secrets.Searched) != 2 {
		t.Errorf("secrets = %+v", d.Secrets)
	}
	if !strings.Contains(string(d.Config), "listen_addr") {
		t.Errorf("config = %s", d.Config)
	}
	if len(d.Providers.Configured) != 1 || d.Providers.Catalog[0].ContextWindow != 128000 {
		t.Errorf("providers = %+v", d.Providers)
	}
	if len(d.Diagnostics) != 1 || d.Diagnostics[0].Code != "x" {
		t.Errorf("diagnostics = %+v", d.Diagnostics)
	}
}

func TestVersionHasNoTarget(t *testing.T) {
	fr := &fakeRunner{out: []byte(`{"version":"0.15.0","contract_version":1}`)}
	c := New(fr).WithConfigDir("/tmp/cfg")

	v, err := c.Version(context.Background())
	if err != nil {
		t.Fatalf("Version: %v", err)
	}
	if !equalArgs(fr.args, []string{"version", "--json"}) {
		t.Errorf("args = %v, want version --json", fr.args)
	}
	if v.ContractVersion != 1 {
		t.Errorf("contract_version = %d", v.ContractVersion)
	}
}

func TestSetConfigTrimsOutput(t *testing.T) {
	fr := &fakeRunner{out: []byte("/etc/nenya/config.d/20-agents.json\n")}
	c := New(fr).WithConfigDir("/tmp/cfg")

	path, err := c.SetConfig(context.Background(), "agents", `{"build":{}}`)
	if err != nil {
		t.Fatalf("SetConfig: %v", err)
	}
	if path != "/etc/nenya/config.d/20-agents.json" {
		t.Errorf("path = %q", path)
	}
	want := []string{"config", "set", "--config-dir", "/tmp/cfg", "agents", `{"build":{}}`}
	if !equalArgs(fr.args, want) {
		t.Errorf("args = %v, want %v", fr.args, want)
	}
}

func TestServiceUnitBuildsFlags(t *testing.T) {
	fr := &fakeRunner{out: []byte("[Unit]\n")}
	c := New(fr)

	if _, err := c.ServiceUnit(context.Background(), ServiceUnitOptions{
		Init: "systemd", ExecPath: "/usr/bin/nenya", ConfigDir: "/etc/nenya", SecretsFile: "/etc/nenya/secrets.json",
	}); err != nil {
		t.Fatalf("ServiceUnit: %v", err)
	}
	want := []string{"service-unit", "--init", "systemd", "--exec-path", "/usr/bin/nenya", "--config-dir", "/etc/nenya", "--secrets-file", "/etc/nenya/secrets.json"}
	if !equalArgs(fr.args, want) {
		t.Errorf("args = %v, want %v", fr.args, want)
	}
}

func TestSecretWriterArgs(t *testing.T) {
	fr := &fakeRunner{out: []byte("/tmp/secrets/01-client.json\n")}
	w := New(fr).SecretWriterFor("/tmp/secrets")

	path, err := w.SetClientToken(context.Background(), "")
	if err != nil {
		t.Fatalf("SetClientToken: %v", err)
	}
	if path != "/tmp/secrets/01-client.json" {
		t.Errorf("path = %q", path)
	}
	if !equalArgs(fr.args, []string{"secret", "set", "--client-token"}) {
		t.Errorf("args = %v", fr.args)
	}

	if _, err := w.SetProviderKey(context.Background(), "openai", "sk-x"); err != nil {
		t.Fatalf("SetProviderKey: %v", err)
	}
	if !equalArgs(fr.args, []string{"secret", "set", "--provider", "openai", "sk-x"}) {
		t.Errorf("args = %v", fr.args)
	}
}

func TestClientPropagatesRunnerError(t *testing.T) {
	fr := &fakeRunner{err: errors.New("boom")}
	c := New(fr)

	_, err := c.Paths(context.Background())
	if err == nil || !strings.Contains(err.Error(), "nenya paths --json") {
		t.Errorf("err = %v", err)
	}
}

func TestOutputBoundsTheContext(t *testing.T) {
	var hasDeadline bool
	probe := runnerFunc(func(ctx context.Context, _ ...string) ([]byte, error) {
		_, hasDeadline = ctx.Deadline()
		return []byte("{}"), nil
	})
	if _, err := New(probe).Paths(context.Background()); err != nil {
		t.Fatal(err)
	}
	if !hasDeadline {
		t.Error("contract calls must run under a bounded context")
	}
}

func TestOutputToleratesNilContext(t *testing.T) {
	fr := &fakeRunner{out: []byte(`{}`)}
	if _, err := New(fr).Paths(nil); err != nil { //nolint:staticcheck // exercising the nil-context guard
		t.Fatalf("nil context: %v", err)
	}
}

func TestBinaryOutputWrapsStderr(t *testing.T) {
	b := Binary{Path: "sh"}
	_, err := b.Output(context.Background(), "-c", "echo boom >&2; exit 3")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "boom") {
		t.Errorf("err = %v, want stderr in the message", err)
	}
}

func TestBinaryOutputRunsCommand(t *testing.T) {
	b := Binary{Path: "sh"}
	out, err := b.Output(context.Background(), "-c", "printf hello")
	if err != nil {
		t.Fatalf("Output: %v", err)
	}
	if string(out) != "hello" {
		t.Errorf("out = %q", out)
	}
}

// runnerFunc adapts a function to Runner for tests.
type runnerFunc func(ctx context.Context, args ...string) ([]byte, error)

func (f runnerFunc) Output(ctx context.Context, args ...string) ([]byte, error) {
	return f(ctx, args...)
}
