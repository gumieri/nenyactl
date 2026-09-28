package cmd

import (
	"context"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/gumieri/nenyactl/internal/containers"
	"github.com/gumieri/nenyactl/internal/detect"
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
	res, err := resolveDir(context.Background(), base, dirAttach, false)
	if err != nil {
		t.Fatal(err)
	}

	rr := newRecordingRunner()
	rr.out = []byte(`{"contract_version":1,"config":{"server":{"listen_addr":":18080"}},"providers":{"configured":["openai"]}}`)
	fakeContract(t, rr)

	// Fake the service start so up does not touch the host.
	savedStart := upServiceRun
	upServiceRun = func() error { return nil }
	t.Cleanup(func() { upServiceRun = savedStart })
	reloaded := false
	savedReload := upServiceReload
	upServiceReload = func() error { reloaded = true; return nil }
	t.Cleanup(func() { upServiceReload = savedReload })

	if err := upDeployment(context.Background(), res, fakeDoer{status: http.StatusOK}, time.Second); err != nil {
		t.Fatalf("upDeployment: %v", err)
	}

	// The token is created through the contract seam with no token value, so
	// an existing token cannot be overwritten or echoed.
	found := false
	for _, c := range rr.rec.calls {
		if len(c) == 3 && c[0] == "secret" && c[1] == "set" && c[2] == "--client-token" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected a bare `secret set --client-token` call, got %v", rr.rec.calls)
	}
	// Providers were already configured, so no keys were written and no
	// service reload should happen.
	if reloaded {
		t.Error("service must not reload when no provider keys were written")
	}
}

func TestUpDeploymentCreatesMissingConfig(t *testing.T) {
	base := t.TempDir()
	res, err := resolveDir(context.Background(), base, dirCreate, false)
	if err != nil {
		t.Fatal(err)
	}

	// describe is unavailable (a released nenya without it); up must still
	// resolve the port from the config it just bootstrapped and reach health.
	rr := newRecordingRunner()
	rr.rec.onCall = func(args []string) {
		if len(args) > 0 && args[0] == "describe" {
			rr.err = errors.New("nenya describe: unknown command")
			return
		}
		rr.err = nil
		rr.out = []byte("/tmp/secrets.json")
	}
	fakeContract(t, rr)

	savedStart := upServiceRun
	upServiceRun = func() error { return nil }
	t.Cleanup(func() { upServiceRun = savedStart })
	reloaded := false
	savedReload := upServiceReload
	upServiceReload = func() error { reloaded = true; return nil }
	t.Cleanup(func() { upServiceReload = savedReload })

	if err := upDeployment(context.Background(), res, fakeDoer{status: http.StatusOK}, time.Second); err != nil {
		t.Fatalf("upDeployment: %v", err)
	}
	if _, err := os.Stat(filepath.Join(base, "config.json")); err != nil {
		t.Errorf("expected config.json to be created: %v", err)
	}
	if reloaded {
		t.Error("service must not reload after a failed describe")
	}
}

func TestUpDeploymentReloadsAfterSavingKeys(t *testing.T) {
	base := t.TempDir()
	if err := os.WriteFile(filepath.Join(base, "config.json"), []byte(`{"server":{"listen_addr":":18081"}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	res, err := resolveDir(context.Background(), base, dirAttach, false)
	if err != nil {
		t.Fatal(err)
	}

	rr := newRecordingRunner()
	rr.out = []byte(`{"contract_version":1,"config":{"server":{"listen_addr":":18081"}},"providers":{"configured":[]}}`)
	fakeContract(t, rr)

	savedKeys := upCollectKeys
	upCollectKeys = func([]containers.ProviderDef) (map[string]string, error) {
		return map[string]string{"openai": "sk-x"}, nil
	}
	t.Cleanup(func() { upCollectKeys = savedKeys })

	savedStart := upServiceRun
	upServiceRun = func() error { return nil }
	t.Cleanup(func() { upServiceRun = savedStart })
	reloaded := false
	savedReload := upServiceReload
	upServiceReload = func() error { reloaded = true; return nil }
	t.Cleanup(func() { upServiceReload = savedReload })

	if err := upDeployment(context.Background(), res, fakeDoer{status: http.StatusOK}, time.Second); err != nil {
		t.Fatalf("upDeployment: %v", err)
	}
	if !reloaded {
		t.Error("expected the service to reload after saving provider keys")
	}
}

func TestUpDeploymentRestartsContainersAfterSavingKeys(t *testing.T) {
	base := t.TempDir()
	if err := os.MkdirAll(filepath.Join(base, "config"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(base, "config", "config.json"), []byte(`{"server":{"listen_addr":":18082"}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(base, "compose.yml"), []byte("services: {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	res, err := resolveDir(context.Background(), base, dirAttach, false)
	if err != nil {
		t.Fatal(err)
	}
	if res.Kind != dirContainerRoot {
		t.Fatalf("expected a container root, got %+v", res)
	}

	rr := newRecordingRunner()
	rr.out = []byte(`{"contract_version":1,"config":{"server":{"listen_addr":":18082"}},"providers":{"configured":[]}}`)
	fakeContract(t, rr)

	savedKeys := upCollectKeys
	upCollectKeys = func([]containers.ProviderDef) (map[string]string, error) {
		return map[string]string{"openai": "sk-x"}, nil
	}
	t.Cleanup(func() { upCollectKeys = savedKeys })

	restarted := false
	savedRestart := upContainerRestart
	upContainerRestart = func(dir string) error { restarted = true; return nil }
	t.Cleanup(func() { upContainerRestart = savedRestart })

	started := false
	savedContainerStart := upContainerStart
	upContainerStart = func(dir string) error { started = true; return nil }
	t.Cleanup(func() { upContainerStart = savedContainerStart })

	reloaded := false
	savedReload := upServiceReload
	upServiceReload = func() error { reloaded = true; return nil }
	t.Cleanup(func() { upServiceReload = savedReload })

	if err := upDeployment(context.Background(), res, fakeDoer{status: http.StatusOK}, time.Second); err != nil {
		t.Fatalf("upDeployment: %v", err)
	}
	if !started {
		t.Error("expected containers to start")
	}
	if !restarted {
		t.Error("expected containers to restart after saving provider keys")
	}
	if reloaded {
		t.Error("a container deployment must not reload the host service")
	}
}

func TestEnsureDeploymentSkipsInstallWithDir(t *testing.T) {
	// --dir is provided, so ensureDeployment must resolve it without installing.
	base := t.TempDir()
	if err := os.WriteFile(filepath.Join(base, "config.json"), []byte(`{}`), 0o644); err != nil {
		t.Fatal(err)
	}

	savedInstall := upInstall
	called := false
	upInstall = func(context.Context, install.Config) error {
		called = true
		return nil
	}
	t.Cleanup(func() { upInstall = savedInstall })

	if _, err := ensureDeployment(context.Background(), base); err != nil {
		t.Fatalf("ensureDeployment: %v", err)
	}
	if called {
		t.Error("install must not run when --dir is provided")
	}
}

func TestEnsureDeploymentInstallsWhenMissing(t *testing.T) {
	// No detection, so up installs and re-detects; the detect seam lets the
	// second call report a deployment without touching the host.
	base := t.TempDir()
	if err := os.WriteFile(filepath.Join(base, "config.json"), []byte(`{}`), 0o644); err != nil {
		t.Fatal(err)
	}

	savedDetect := upDetect
	detectCalls := 0
	upDetect = func() (*detect.Info, error) {
		detectCalls++
		if detectCalls == 1 {
			return nil, errors.New("nothing installed")
		}
		return &detect.Info{Mode: detect.ModeBareMetal, ConfigFile: filepath.Join(base, "config.json"), ConfigD: filepath.Join(base, "config.d")}, nil
	}
	t.Cleanup(func() { upDetect = savedDetect })

	savedInstall := upInstall
	installed := false
	upInstall = func(context.Context, install.Config) error {
		installed = true
		return nil
	}
	t.Cleanup(func() { upInstall = savedInstall })

	res, err := ensureDeployment(context.Background(), "")
	if err != nil {
		t.Fatalf("ensureDeployment: %v", err)
	}
	if !installed {
		t.Error("expected up to install when nothing is detected")
	}
	if res.Kind != dirConfigRoot {
		t.Errorf("res = %+v", res)
	}
}

func TestEnsureDeploymentStateErrorDoesNotReinstall(t *testing.T) {
	// A present binary with a missing config is a state problem, not a reason
	// to reinstall.
	savedDetect := upDetect
	upDetect = func() (*detect.Info, error) {
		return nil, &detect.ConfigNotFoundError{ConfigFile: "/etc/nenya/config.json", BinPath: "/usr/bin/nenya"}
	}
	t.Cleanup(func() { upDetect = savedDetect })

	savedInstall := upInstall
	installed := false
	upInstall = func(context.Context, install.Config) error {
		installed = true
		return nil
	}
	t.Cleanup(func() { upInstall = savedInstall })

	if _, err := ensureDeployment(context.Background(), ""); err == nil {
		t.Fatal("expected the detection error to surface")
	}
	if installed {
		t.Error("must not reinstall when the binary is present but config is missing")
	}
}
