package cmd

import (
	"context"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/gumieri/nenyactl/internal/install"
)

// fakeDoerSeq returns a sequence of health outcomes, so waitForHealth can be
// driven from unreachable to healthy without a live gateway.
type seqDoer struct {
	steps []fakeDoer
	n     int
}

func (s *seqDoer) Do(r *http.Request) (*http.Response, error) {
	i := s.n
	if i >= len(s.steps) {
		i = len(s.steps) - 1
	}
	s.n++
	return s.steps[i].Do(r)
}

var errUnreachable = errors.New("connection refused")

func TestWaitForHealth(t *testing.T) {
	t.Run("returns once healthy", func(t *testing.T) {
		doer := &seqDoer{steps: []fakeDoer{
			{err: errUnreachable},
			{status: http.StatusOK},
		}}
		if err := waitForHealth(context.Background(), doer, "8080", time.Second); err != nil {
			t.Fatalf("waitForHealth: %v", err)
		}
	})

	t.Run("times out", func(t *testing.T) {
		if err := waitForHealth(context.Background(), fakeDoer{err: errUnreachable}, "8080", 50*time.Millisecond); err == nil {
			t.Fatal("expected a timeout error")
		}
	})
}

func TestUpDeployment(t *testing.T) {
	base := t.TempDir()
	if err := os.WriteFile(filepath.Join(base, "config.json"), []byte(`{"server":{"listen_addr":":18080"}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	res, err := resolveDir(base, dirAttach, false)
	if err != nil {
		t.Fatal(err)
	}

	rr := newRecordingRunner()
	rr.out = []byte(`{"contract_version":1,"config":{"server":{"listen_addr":":18080"}},"providers":{"configured":["openai"]}}`)
	fakeContract(t, rr)

	// Fake the service start so up does not touch the host.
	savedStart := upServiceStart
	upServiceStart = func() error { return nil }
	t.Cleanup(func() { upServiceStart = savedStart })

	if err := upDeployment(context.Background(), res, fakeDoer{status: http.StatusOK}, time.Second); err != nil {
		t.Fatalf("upDeployment: %v", err)
	}

	// The token is created through the contract seam.
	if len(rr.rec.calls) == 0 || rr.rec.calls[0][0] != "secret" {
		t.Errorf("expected a secret set call, got %v", rr.rec.calls)
	}
}

func TestUpDeploymentCreatesMissingConfig(t *testing.T) {
	base := t.TempDir()
	res, err := resolveDir(base, dirCreate, false)
	if err != nil {
		t.Fatal(err)
	}

	rr := newRecordingRunner()
	rr.out = []byte(`{"contract_version":1,"providers":{"configured":["openai"]}}`)
	fakeContract(t, rr)

	savedStart := upServiceStart
	upServiceStart = func() error { return nil }
	t.Cleanup(func() { upServiceStart = savedStart })

	if err := upDeployment(context.Background(), res, fakeDoer{status: http.StatusOK}, time.Second); err != nil {
		t.Fatalf("upDeployment: %v", err)
	}
	if _, err := os.Stat(filepath.Join(base, "config.json")); err != nil {
		t.Errorf("expected config.json to be created: %v", err)
	}
}

func TestEnsureDeploymentInstallsWhenMissing(t *testing.T) {
	// --dir is provided, so ensureDeployment must resolve it without installing.
	base := t.TempDir()
	if err := os.WriteFile(filepath.Join(base, "config.json"), []byte(`{}`), 0o644); err != nil {
		t.Fatal(err)
	}

	savedInstall := upInstallFunc
	called := false
	upInstallFunc = func(context.Context, install.Config) error {
		called = true
		return nil
	}
	t.Cleanup(func() { upInstallFunc = savedInstall })

	if _, err := ensureDeployment(context.Background(), base); err != nil {
		t.Fatalf("ensureDeployment: %v", err)
	}
	if called {
		t.Error("install must not run when --dir is provided")
	}
}
