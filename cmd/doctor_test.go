package cmd

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestDiagnose(t *testing.T) {
	base := t.TempDir()
	if err := os.WriteFile(filepath.Join(base, "config.json"), []byte(`{"server":{"listen_addr":":18080"}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(base, "secrets.json"), []byte(`{"client_token":"nk-x"}`), 0o600); err != nil {
		t.Fatal(err)
	}

	res, err := resolveDir(base, dirAttach, false)
	if err != nil {
		t.Fatal(err)
	}

	rr := newRecordingRunner()
	rr.out = []byte(`{"contract_version":1,"version":{"version":"0.15.0"},"config":{"server":{"listen_addr":":18080"}},"providers":{"configured":["openai"]},"diagnostics":[]}`)
	fakeContract(t, rr)

	byName := map[string]checkResult{}
	for _, r := range diagnose(context.Background(), res) {
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
}

func TestDiagnoseFailures(t *testing.T) {
	t.Run("missing config fails", func(t *testing.T) {
		dir := t.TempDir()
		res, err := resolveDir(dir, dirAttach, false)
		if err != nil {
			t.Fatal(err)
		}
		rr := newRecordingRunner()
		rr.err = os.ErrNotExist
		fakeContract(t, rr)

		r := checkConfig(res.Contract(), context.Background(), res)
		if r.Status != checkFail || r.Fix == "" {
			t.Errorf("got %+v, want a fail with a fix", r)
		}
	})

	t.Run("missing token fails", func(t *testing.T) {
		dir := t.TempDir()
		res, err := resolveDir(dir, dirAttach, false)
		if err != nil {
			t.Fatal(err)
		}
		r := checkSecrets(res)
		if r.Status != checkFail || r.Fix == "" {
			t.Errorf("got %+v, want a fail with a fix", r)
		}
	})

	t.Run("loose token permissions fail", func(t *testing.T) {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, "secrets.json"), []byte(`{"client_token":"nk-x"}`), 0o644); err != nil {
			t.Fatal(err)
		}
		res, err := resolveDir(dir, dirAttach, false)
		if err != nil {
			t.Fatal(err)
		}
		r := checkSecrets(res)
		if r.Status != checkFail || r.Fix == "" {
			t.Errorf("got %+v, want a fail with a chmod fix", r)
		}
	})

	t.Run("out-of-range contract fails", func(t *testing.T) {
		dir := t.TempDir()
		res, err := resolveDir(dir, dirAttach, false)
		if err != nil {
			t.Fatal(err)
		}
		rr := newRecordingRunner()
		rr.out = []byte(`{"contract_version":99}`)
		fakeContract(t, rr)

		r := checkContract(res.Contract(), context.Background())
		if r.Status != checkFail {
			t.Errorf("got %+v, want fail", r)
		}
	})

	t.Run("no providers warns", func(t *testing.T) {
		dir := t.TempDir()
		res, err := resolveDir(dir, dirAttach, false)
		if err != nil {
			t.Fatal(err)
		}
		rr := newRecordingRunner()
		rr.out = []byte(`{"contract_version":1,"providers":{"configured":[]}}`)
		fakeContract(t, rr)

		r := checkProviders(res.Contract(), context.Background())
		if r.Status != checkWarn || r.Fix == "" {
			t.Errorf("got %+v, want a warn with a fix", r)
		}
	})
}
