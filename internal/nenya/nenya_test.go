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

// envRunner records NENYA_SECRETS_DIR when SecretWriterFor targets a directory.
type envRunner struct {
	env  *[]string
	args []string
	out  []byte
	err  error
}

func (r *envRunner) Output(_ context.Context, args ...string) ([]byte, error) {
	r.args = append([]string(nil), args...)
	return r.out, r.err
}

func (r *envRunner) WithEnv(env ...string) Runner {
	*r.env = append(*r.env, env...)
	return r
}

func equalArgs(a, b []string) bool {
	return strings.Join(a, "\x00") == strings.Join(b, "\x00")
}

const describeJSON = `{
  "contract_version": 1,
  "version": {"version": "0.15.0", "commit": "abc", "build_time": "t", "contract_version": 1},
  "paths": {"mode": "directory", "config_dir": "/etc/nenya", "config_file": "/etc/nenya/config.json", "config_d": "/etc/nenya/config.d", "secrets_dir": "/run/secrets/nenya", "socket_path": null, "platform": "linux"},
  "secrets": {"active_source": "/run/secrets/nenya", "searched": ["a", "b"]},
  "config": {"server": {"listen_addr": ":8080"}},
  "providers": {"configured": ["openai"], "catalog": [{"provider": "openai", "model": "gpt", "context_window": 128000, "max_output": 4096}]},
  "diagnostics": [{"level": "warn", "code": "x", "message": "m", "source": "s"}]
}`

func TestDescribeParsesEffectiveState(t *testing.T) {
	fr := &fakeRunner{out: []byte(describeJSON)}
	c := New(fr).WithConfigDir("/tmp/cfg")

	d, err := c.Describe(context.Background())
	if err != nil {
		t.Fatalf("Describe: %v", err)
	}
	if !equalArgs(fr.args, []string{"describe", "--json", "--config-dir", "/tmp/cfg"}) {
		t.Errorf("args = %v", fr.args)
	}
	if d.ContractVersion != 1 || d.Version.Version != "0.15.0" {
		t.Errorf("version surface = %+v", d)
	}
	if d.Paths.ConfigDir != "/etc/nenya" || d.Paths.SocketPath != nil {
		t.Errorf("paths = %+v", d.Paths)
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

func TestDescribeConfigNull(t *testing.T) {
	fr := &fakeRunner{out: []byte(`{"contract_version":1,"config":null}`)}
	d, err := New(fr).Describe(context.Background())
	if err != nil {
		t.Fatalf("Describe: %v", err)
	}
	if len(d.Config) != 0 {
		t.Errorf("config = %q, want empty", d.Config)
	}
}

func TestDescribeRejectsUnsupportedContract(t *testing.T) {
	fr := &fakeRunner{out: []byte(`{"contract_version":99}`)}
	if _, err := New(fr).Describe(context.Background()); err == nil {
		t.Fatal("expected an unsupported contract error")
	}
}

func TestCommandErrorScrubsValues(t *testing.T) {
	cfgErr := commandError(
		[]string{"config", "set", "--config-dir", "/etc/nenya", "providers.openai.key", "sk-secret"},
		errors.New("boom"),
		"invalid value sk-secret",
	)
	if strings.Contains(cfgErr.Error(), "sk-secret") {
		t.Errorf("config set value leaked: %v", cfgErr)
	}

	secErr := commandError(
		[]string{"secret", "set", "--provider", "openai", "sk-secret"},
		errors.New("boom"),
		"echoed sk-secret",
	)
	if strings.Contains(secErr.Error(), "sk-secret") {
		t.Errorf("secret value leaked: %v", secErr)
	}
}

func TestRunBoundedSurfacesCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	probe := runnerFunc(func(ctx context.Context, _ ...string) ([]byte, error) {
		<-ctx.Done()
		return nil, ctx.Err()
	})
	_, err := New(probe).Describe(ctx)
	if !errors.Is(err, context.Canceled) {
		t.Errorf("err = %v, want context.Canceled", err)
	}
}

func TestSetConfigTargetsAndTrims(t *testing.T) {
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

func TestSecretWriterArgsAndEnv(t *testing.T) {
	env := new([]string)
	er := &envRunner{env: env, out: []byte("/tmp/secrets/01-client.json\n")}
	w := New(er).SecretWriterFor("/tmp/secrets")

	path, err := w.SetClientToken(context.Background(), "")
	if err != nil {
		t.Fatalf("SetClientToken: %v", err)
	}
	if path != "/tmp/secrets/01-client.json" {
		t.Errorf("path = %q", path)
	}
	if !equalArgs(er.args, []string{"secret", "set", "--client-token"}) {
		t.Errorf("args = %v", er.args)
	}
	if !equalArgs(*env, []string{"NENYA_SECRETS_DIR=/tmp/secrets"}) {
		t.Errorf("env = %v", *env)
	}

	if _, err := w.SetProviderKey(context.Background(), "openai", "sk-x"); err != nil {
		t.Fatalf("SetProviderKey: %v", err)
	}
	if !equalArgs(er.args, []string{"secret", "set", "--provider", "openai", "sk-x"}) {
		t.Errorf("args = %v", er.args)
	}
}

func TestClientPropagatesRunnerError(t *testing.T) {
	fr := &fakeRunner{err: errors.New("boom")}
	_, err := New(fr).Describe(context.Background())
	if err == nil || !strings.Contains(err.Error(), "nenya describe --json") {
		t.Errorf("err = %v", err)
	}
}

func TestSecretErrorsDoNotLeakValues(t *testing.T) {
	fr := &fakeRunner{err: errors.New("boom")}
	_, err := New(fr).SecretWriterFor("/tmp/secrets").SetProviderKey(context.Background(), "openai", "sk-super-secret")
	if err == nil {
		t.Fatal("expected error")
	}
	if strings.Contains(err.Error(), "sk-super-secret") {
		t.Errorf("secret leaked in error: %v", err)
	}

	// Binary must redact the argument values and drop stderr entirely.
	_, berr := Binary{Path: "sh"}.Output(context.Background(), "secret", "set", "--provider", "openai", "sk-super-secret")
	if berr == nil {
		t.Fatal("expected error")
	}
	if strings.Contains(berr.Error(), "sk-super-secret") {
		t.Errorf("secret leaked in binary error: %v", berr)
	}
}

func TestRunBoundedSetsDeadlineAndToleratesNil(t *testing.T) {
	var hasDeadline bool
	probe := runnerFunc(func(ctx context.Context, _ ...string) ([]byte, error) {
		_, hasDeadline = ctx.Deadline()
		return []byte("{}"), nil
	})
	if _, err := New(probe).Describe(context.Background()); err != nil {
		t.Fatal(err)
	}
	if !hasDeadline {
		t.Error("contract calls must run under a bounded context")
	}
	if _, err := New(probe).Describe(nil); err != nil { //nolint:staticcheck // exercising the nil-context guard
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
