package cmd

import (
	"context"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/gumieri/nenyactl/internal/nenya"
)

func TestDiagnose(t *testing.T) {
	base := t.TempDir()
	if err := os.WriteFile(filepath.Join(base, "config.json"), []byte(`{"server":{"listen_addr":":18080"}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(base, "secrets.json"), []byte(`{"client_token":"nk-x"}`), 0o600); err != nil {
		t.Fatal(err)
	}

	res, err := resolveDir(context.Background(), base, dirAttach, false)
	if err != nil {
		t.Fatal(err)
	}

	rr := newRecordingRunner()
	rr.out = []byte(`{"contract_version":1,"version":{"version":"0.15.0"},"config":{"server":{"listen_addr":":18080"}},"providers":{"configured":["openai"]},"diagnostics":[]}`)
	fakeContract(t, rr)

	byName := map[string]checkResult{}
	for _, r := range diagnose(context.Background(), res, fakeDoer{status: http.StatusOK}) {
		byName[r.Name] = r
	}

	if got := byName["config"].Status; got != checkOK {
		t.Errorf("config = %v (%s)", got, byName["config"].Detail)
	}
	if got := byName["secrets"].Status; got != checkOK {
		t.Errorf("secrets = %v (%s)", got, byName["secrets"].Detail)
	}
	if got := byName["contract"].Status; got != checkOK {
		t.Errorf("contract = %v (%s)", got, byName["contract"].Detail)
	}
	if got := byName["providers"].Status; got != checkOK {
		t.Errorf("providers = %v (%s)", got, byName["providers"].Detail)
	}
	if got := byName["port"].Status; got != checkOK {
		t.Errorf("port = %v (%s)", got, byName["port"].Detail)
	}
}

func TestDiagnoseFailures(t *testing.T) {
	t.Run("missing config fails", func(t *testing.T) {
		dir := t.TempDir()
		res, err := resolveDir(context.Background(), dir, dirAttach, false)
		if err != nil {
			t.Fatal(err)
		}
		r := checkConfig(res, errors.New("describe unavailable"), false)
		if r.Status != checkFail || r.Fix == "" {
			t.Errorf("got %+v, want a fail with a fix", r)
		}
	})

	t.Run("config.d-only root is configured", func(t *testing.T) {
		dir := t.TempDir()
		if err := os.MkdirAll(filepath.Join(dir, "config.d"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "config.d", "00-server.json"), []byte("{}"), 0o644); err != nil {
			t.Fatal(err)
		}
		res, err := resolveDir(context.Background(), dir, dirAttach, false)
		if err != nil {
			t.Fatal(err)
		}
		if r := checkConfig(res, nil, true); r.Status != checkOK {
			t.Errorf("got %+v, want ok for a config.d-only root", r)
		}
	})

	t.Run("missing token fails", func(t *testing.T) {
		dir := t.TempDir()
		res, err := resolveDir(context.Background(), dir, dirAttach, false)
		if err != nil {
			t.Fatal(err)
		}
		if r := checkSecrets(context.Background(), res, nenya.Description{}, false); r.Status != checkFail || r.Fix == "" {
			t.Errorf("got %+v, want a fail with a fix", r)
		}
	})

	t.Run("any loose token file fails", func(t *testing.T) {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, "secrets.json"), []byte(`{"client_token":"nk-x"}`), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "02-extra.json"), []byte(`{"client_token":"nk-y"}`), 0o644); err != nil {
			t.Fatal(err)
		}
		res, err := resolveDir(context.Background(), dir, dirAttach, false)
		if err != nil {
			t.Fatal(err)
		}
		r := checkSecrets(context.Background(), res, nenya.Description{}, false)
		if r.Status != checkFail {
			t.Errorf("got %+v, want fail for the loose file", r)
		}
	})

	t.Run("shim token with a working-but-empty reader warns", func(t *testing.T) {
		// The reader exists (supported probe) and resolves nothing, while the
		// local shim finds a token: a real disagreement, surfaced as a warning.
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, "secrets.json"), []byte(`{"client_token":"nk-x"}`), 0o600); err != nil {
			t.Fatal(err)
		}
		res, err := resolveDir(context.Background(), dir, dirAttach, false)
		if err != nil {
			t.Fatal(err)
		}
		saved := newContractClient
		rr := newRecordingRunner()
		rr.rec.onCall = func(args []string) {
			rr.err = nil
			if args[0] == "secret" && args[1] == "get" {
				if len(args) > 2 && args[2] == "-h" {
					rr.out = []byte("usage: nenya secret get …") // reader exists
					return
				}
				// Reader runs clean but resolves nothing (exit 0, empty
				// stdout): the sentinel disagreement path.
				rr.out = nil
			}
		}
		newContractClient = func(d string) *nenya.Client {
			return nenya.New(rr).WithConfigDir(d)
		}
		t.Cleanup(func() { newContractClient = saved })

		r := checkSecrets(context.Background(), res, nenya.Description{}, true)
		if r.Status != checkWarn || !strings.Contains(r.Detail, "resolved no client token") {
			t.Errorf("got %+v, want a warn naming the empty-success disagreement", r)
		}
	})

	t.Run("systemd credential source is ok", func(t *testing.T) {
		dir := t.TempDir()
		res, err := resolveDir(context.Background(), dir, dirAttach, false)
		if err != nil {
			t.Fatal(err)
		}
		desc := nenya.Description{}
		desc.Secrets.ActiveSource = "$CREDENTIALS_DIRECTORY/secrets"
		if r := checkSecrets(context.Background(), res, desc, true); r.Status != checkOK {
			t.Errorf("got %+v, want ok for a credential source", r)
		}
	})

	t.Run("out-of-range contract fails", func(t *testing.T) {
		dir := t.TempDir()
		res, err := resolveDir(context.Background(), dir, dirAttach, false)
		if err != nil {
			t.Fatal(err)
		}
		rr := newRecordingRunner()
		rr.out = []byte(`{"contract_version":99}`)
		fakeContract(t, rr)

		_, err = res.Contract().Describe(context.Background())
		if err == nil {
			t.Fatal("expected an error from Describe")
		}
		if r := checkContract(nenya.Description{}, err); r.Status != checkFail {
			t.Errorf("got %+v, want fail", r)
		}
	})

	t.Run("no providers warns", func(t *testing.T) {
		if r := checkProviders(nenya.Description{}, nil); r.Status != checkWarn || r.Fix == "" {
			t.Errorf("got %+v, want a warn with a fix", r)
		}
	})
}

func TestCheckService(t *testing.T) {
	t.Run("container root has no unit", func(t *testing.T) {
		if r := checkService(dirResolution{Kind: dirContainerRoot}); r.Status != checkOK {
			t.Errorf("got %+v, want ok", r)
		}
	})

	t.Run("bare-metal without a unit warns", func(t *testing.T) {
		r := checkService(dirResolution{Kind: dirConfigRoot})
		if r.Status != checkWarn || r.Fix == "" {
			t.Errorf("got %+v, want a warn with a fix", r)
		}
	})

	t.Run("unit name matches the platform", func(t *testing.T) {
		r := checkService(dirResolution{Kind: dirConfigRoot})
		if runtime.GOOS == "darwin" {
			if !contains(r.Detail, "com.gumieri.nenya.plist") {
				t.Errorf("darwin unit detail = %q", r.Detail)
			}
		} else if !contains(r.Detail, "nenya.service") {
			t.Errorf("linux unit detail = %q", r.Detail)
		}
	})
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || indexOf(s, sub) >= 0)
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
