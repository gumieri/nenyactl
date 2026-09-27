package cmd

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gumieri/nenyactl/internal/nenya"
	"github.com/spf13/cobra"
)

// recordingRunner is a nenya.Runner that records every invocation and emulates
// the contract. It lets the secret commands be tested without a nenya binary.
// Its state is shared across WithEnv copies (the env the writer applied is what
// the test asserts); Binary's own copy semantics are covered separately.
type recordingRunner struct {
	rec *record
	out []byte
	err error
}

type record struct {
	calls  [][]string
	env    []string
	onCall func(args []string)
}

func newRecordingRunner() *recordingRunner {
	return &recordingRunner{rec: &record{}}
}

func (r *recordingRunner) Output(_ context.Context, args ...string) ([]byte, error) {
	r.rec.calls = append(r.rec.calls, append([]string(nil), args...))
	if r.rec.onCall != nil {
		r.rec.onCall(args)
	}
	if r.err != nil {
		return nil, r.err
	}
	return r.out, nil
}

// WithEnv makes recordingRunner an EnvRunner so SecretWriterFor's
// NENYA_SECRETS_DIR targeting can be asserted.
func (r *recordingRunner) WithEnv(env ...string) nenya.Runner {
	cp := *r
	r.rec.env = append(r.rec.env, env...)
	return &cp
}

// fakeContract points newContractClient at rr for the test and returns a pointer
// to the config directory the command pinned.
func fakeContract(t *testing.T, rr *recordingRunner) *string {
	t.Helper()
	dir := new(string)
	saved := newContractClient
	newContractClient = func(d string) *nenya.Client {
		*dir = d
		return nenya.New(rr).WithConfigDir(d)
	}
	t.Cleanup(func() { newContractClient = saved })
	return dir
}

// testCmd returns a command with a non-nil context, mirroring what cobra
// provides when a command actually runs.
func testCmd() *cobra.Command {
	c := &cobra.Command{}
	c.SetContext(context.Background())
	return c
}

// fakeContractWriting points the contract seam at a runner that materializes a
// client token file under secretsDir when `secret set --client-token` runs,
// emulating nenya's writer so container setup can be tested without a binary.
func fakeContractWriting(t *testing.T, secretsDir string) *recordingRunner {
	t.Helper()
	rr := newRecordingRunner()
	rr.rec.onCall = func(args []string) {
		if len(args) >= 3 && args[0] == "secret" && args[1] == "set" && args[2] == "--client-token" {
			_ = os.MkdirAll(secretsDir, 0o700)
			_ = os.WriteFile(filepath.Join(secretsDir, "01-client.json"), []byte(`{"client_token":"nk-fake"}`), 0o600)
		}
	}
	fakeContract(t, rr)
	return rr
}

func TestRunSecretGenerate(t *testing.T) {
	t.Run("generates client token", func(t *testing.T) {
		secretType = "client"
		err := runSecretGenerate(&cobra.Command{}, nil)
		if err != nil {
			t.Fatalf("runSecretGenerate() error = %v", err)
		}
	})

	t.Run("returns error on apikey without name", func(t *testing.T) {
		secretType = "apikey"
		secretForClient = ""
		err := runSecretGenerate(&cobra.Command{}, nil)
		if err == nil {
			t.Fatal("expected error for missing --name")
		}
		if !strings.Contains(err.Error(), "name is required") {
			t.Errorf("unexpected error: %v", err)
		}
	})

	t.Run("generates apikey with name", func(t *testing.T) {
		secretType = "apikey"
		secretForClient = "test-client"
		err := runSecretGenerate(&cobra.Command{}, nil)
		if err != nil {
			t.Fatalf("runSecretGenerate() error = %v", err)
		}
	})

	t.Run("returns error on unknown type", func(t *testing.T) {
		secretType = "unknown"
		err := runSecretGenerate(&cobra.Command{}, nil)
		if err == nil {
			t.Fatal("expected error for unknown type")
		}
	})
}

func TestRunSecretBootstrap(t *testing.T) {
	t.Run("delegates token creation to the contract", func(t *testing.T) {
		tmp := t.TempDir()
		bootstrapDir = tmp
		bootstrapForce = false

		rr := newRecordingRunner()
		rr.out = []byte(filepath.Join(tmp, "secrets.json"))
		dir := fakeContract(t, rr)

		if err := runSecretBootstrap(testCmd(), nil); err != nil {
			t.Fatalf("runSecretBootstrap() error = %v", err)
		}
		if *dir != tmp {
			t.Errorf("contract pinned to %q, want %q", *dir, tmp)
		}
		if len(rr.rec.calls) != 1 {
			t.Fatalf("got %d contract calls, want 1: %v", len(rr.rec.calls), rr.rec.calls)
		}
		want := []string{"secret", "set", "--client-token"}
		if strings.Join(rr.rec.calls[0], " ") != strings.Join(want, " ") {
			t.Errorf("call = %v, want %v", rr.rec.calls[0], want)
		}
		if len(rr.rec.env) != 1 || rr.rec.env[0] != "NENYA_SECRETS_DIR="+tmp {
			t.Errorf("env = %v, want NENYA_SECRETS_DIR=%s", rr.rec.env, tmp)
		}
	})

	t.Run("refuses to replace an existing token", func(t *testing.T) {
		tmp := t.TempDir()
		if err := os.WriteFile(filepath.Join(tmp, "secrets.json"), []byte(`{"client_token":"nk-existing"}`), 0o600); err != nil {
			t.Fatalf("write secrets: %v", err)
		}
		bootstrapDir = tmp
		bootstrapForce = false

		rr := newRecordingRunner()
		fakeContract(t, rr)

		if err := runSecretBootstrap(testCmd(), nil); err == nil {
			t.Fatal("expected error for existing token")
		}
		if len(rr.rec.calls) != 0 {
			t.Errorf("contract must not be called when a token exists: %v", rr.rec.calls)
		}
	})

	t.Run("container layout pins the nested config dir", func(t *testing.T) {
		tmp := t.TempDir()
		if err := os.MkdirAll(filepath.Join(tmp, "config"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(tmp, "config", "config.json"), []byte("{}"), 0o644); err != nil {
			t.Fatal(err)
		}
		bootstrapDir = tmp
		bootstrapForce = false

		rr := newRecordingRunner()
		rr.out = []byte(filepath.Join(tmp, "secrets", "01-client.json"))
		dir := fakeContract(t, rr)

		if err := runSecretBootstrap(testCmd(), nil); err != nil {
			t.Fatalf("runSecretBootstrap() error = %v", err)
		}
		if want := filepath.Join(tmp, "config"); *dir != want {
			t.Errorf("contract pinned to %q, want %q", *dir, want)
		}
	})
}

func TestRunSecretSet(t *testing.T) {
	tmp := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmp, "config.json"), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	secretSetDir = tmp
	secretSetProvider = "openai"

	rr := newRecordingRunner()
	rr.out = []byte(filepath.Join(tmp, "secrets.json"))
	dir := fakeContract(t, rr)

	if err := runSecretSet(testCmd(), []string{"sk-test"}); err != nil {
		t.Fatalf("runSecretSet() error = %v", err)
	}
	if *dir != tmp {
		t.Errorf("contract pinned to %q, want %q", *dir, tmp)
	}
	want := []string{"secret", "set", "--provider", "openai", "sk-test"}
	if len(rr.rec.calls) != 1 || strings.Join(rr.rec.calls[0], " ") != strings.Join(want, " ") {
		t.Errorf("calls = %v, want one call %v", rr.rec.calls, want)
	}

	t.Run("requires --provider", func(t *testing.T) {
		secretSetProvider = ""
		if err := runSecretSet(testCmd(), []string{"sk-test"}); err == nil {
			t.Fatal("expected error without --provider")
		}
		secretSetProvider = "openai"
	})

	t.Run("requires exactly one argument", func(t *testing.T) {
		if err := runSecretSet(testCmd(), nil); err == nil {
			t.Fatal("expected error without an api key")
		}
	})
}
