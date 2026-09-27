package cmd

import (
	"context"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

// fakeDoer is a healthDoer that returns a canned response or error.
type fakeDoer struct {
	status int
	err    error
}

func (f fakeDoer) Do(*http.Request) (*http.Response, error) {
	if f.err != nil {
		return nil, f.err
	}
	return &http.Response{
		StatusCode: f.status,
		Status:     http.StatusText(f.status),
		Body:       io.NopCloser(strings.NewReader("")),
	}, nil
}

func TestBuildStatus(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "config.json"), []byte(`{"server":{"listen_addr":":9191"}}`), 0o644); err != nil {
		t.Fatal(err)
	}

	res, err := resolveDir(dir, dirAttach, false)
	if err != nil {
		t.Fatal(err)
	}

	rr := newRecordingRunner()
	rr.out = []byte(`{
	  "contract_version": 1,
	  "version": {"version": "0.15.0"},
	  "paths": {"mode": "directory"},
	  "config": {"server":{"listen_addr":":9191"},"discovery":{"auto_agents":false},"agents":{"build":{"strategy":"fallback","models":["m1","m2"]}}},
	  "providers": {"configured": ["openai"]},
	  "diagnostics": []
	}`)
	fakeContract(t, rr)

	out := buildStatus(context.Background(), res, fakeDoer{status: http.StatusOK})

	for _, want := range []string{
		"Mode:        bare-metal",
		"Config root: " + dir,
		"Contract:    v1",
		"Nenya:       0.15.0",
		"Providers:   openai",
		"Port:        9191",
		"Health:      healthy",
		"Auto-agents: false",
		"Agent build",
		"m1, m2",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("status output missing %q:\n%s", want, out)
		}
	}
}

func TestBuildStatusHandlesUnreachableAndNoContract(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "config.json"), []byte(`{}`), 0o644); err != nil {
		t.Fatal(err)
	}
	res, err := resolveDir(dir, dirAttach, false)
	if err != nil {
		t.Fatal(err)
	}

	rr := newRecordingRunner()
	rr.err = errors.New("nenya not found")
	fakeContract(t, rr)

	out := buildStatus(context.Background(), res, fakeDoer{err: errors.New("connection refused")})
	if !strings.Contains(out, "Contract:    unavailable") {
		t.Errorf("expected an unavailable contract line:\n%s", out)
	}
	if !strings.Contains(out, "Port:        unknown") {
		t.Errorf("expected an unknown port when none resolves:\n%s", out)
	}
	if !strings.Contains(out, "Health:      no port resolved") {
		t.Errorf("expected the no-port health line:\n%s", out)
	}
}

func TestHealthStatus(t *testing.T) {
	if ok, _ := healthStatus(context.Background(), fakeDoer{status: http.StatusOK}, "8080"); !ok {
		t.Error("200 should be healthy")
	}
	if ok, _ := healthStatus(context.Background(), fakeDoer{status: http.StatusServiceUnavailable}, "8080"); ok {
		t.Error("503 should not be healthy")
	}
	if ok, _ := healthStatus(context.Background(), fakeDoer{}, ""); ok {
		t.Error("no port should not be healthy")
	}
}

func TestResolveLifecycleDirExplicit(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "config.json"), []byte(`{}`), 0o644); err != nil {
		t.Fatal(err)
	}
	res, err := resolveLifecycleDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if res.Kind != dirConfigRoot || res.Path != dir {
		t.Errorf("res = %+v", res)
	}
}

func TestRunDownBareMetal(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "config.json"), []byte(`{}`), 0o644); err != nil {
		t.Fatal(err)
	}

	savedDir := downDir
	downDir = dir
	t.Cleanup(func() { downDir = savedDir })

	savedRun := downServiceRun
	stopped := false
	downServiceRun = func() error { stopped = true; return nil }
	t.Cleanup(func() { downServiceRun = savedRun })

	if err := runDown(testCmd(), nil); err != nil {
		t.Fatalf("runDown: %v", err)
	}
	if !stopped {
		t.Error("expected the service stop seam to run for a bare-metal root")
	}
}

func TestLifecycleCommandsRejectArgs(t *testing.T) {
	for _, c := range []*cobra.Command{upCmd, downCmd, statusCmd, doctorCmd} {
		if c.Args == nil {
			t.Errorf("%s: expected an Args validator", c.Name())
			continue
		}
		if err := c.Args(c, []string{"stray"}); err == nil {
			t.Errorf("%s: expected stray args to be rejected", c.Name())
		}
	}
}
