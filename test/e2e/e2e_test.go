// Package e2e exercises the real nenyactl <-> nenya seam against a real nenya
// release: it installs through nenyactl's own verified install path, then runs
// the installed binary. It is gated on NENYACTL_E2E=1 so it never runs in the
// unit job.
package e2e

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/gumieri/nenyactl/internal/contract"
	"github.com/gumieri/nenyactl/internal/install"
)

const (
	owner = "gumieri"
	repo  = "nenya"
)

func TestGoldenPathAgainstLatestRelease(t *testing.T) {
	if os.Getenv("NENYACTL_E2E") != "1" {
		t.Skip("set NENYACTL_E2E=1 to run the real-release golden-path E2E")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	tag := os.Getenv("NENYA_E2E_VERSION")
	if tag == "" {
		var err error
		tag, err = latestTag(ctx)
		if err != nil {
			t.Fatalf("resolve latest nenya release: %v", err)
		}
	}
	tag = install.NormalizeTag(tag)
	t.Logf("nenya release under test: %s (%s/%s)", tag, runtime.GOOS, runtime.GOARCH)

	// Install through nenyactl's own path: real download, SHA-256 + cosign
	// verification, archive extraction, and --user bootstrap.
	dir := t.TempDir()
	home := filepath.Join(dir, "home")
	if err := os.MkdirAll(home, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", "")

	if err := install.InstallWithHTTPAndRunner(ctx, install.Config{
		UserInstall: true,
		Version:     tag,
	}, http.DefaultClient, install.NewExecRunner()); err != nil {
		t.Fatalf("nenyactl install against real release %s: %v", tag, err)
	}

	binPath := filepath.Join(home, ".local", "bin", "nenya")
	if _, err := os.Stat(binPath); err != nil {
		t.Fatalf("installed binary missing at %s: %v", binPath, err)
	}

	configDir := filepath.Join(home, ".config", "nenya")
	if _, err := os.Stat(filepath.Join(configDir, "config.json")); err != nil {
		t.Fatalf("install did not bootstrap config: %v", err)
	}
	tokenFile := filepath.Join(configDir, "secrets.json")
	if info, err := os.Stat(tokenFile); err != nil {
		t.Fatalf("install did not bootstrap secrets: %v", err)
	} else if info.Mode().Perm() != 0o600 {
		t.Fatalf("secrets.json mode = %o, want 0600", info.Mode().Perm())
	}
	token := readClientToken(t, tokenFile)

	// Point the installed config at a mock OpenAI-compatible upstream.
	mock := httptest.NewServer(mockUpstream())
	defer mock.Close()

	listen := freeAddr(t)
	writeConfig(t, filepath.Join(configDir, "config.json"), listen, mock.URL+"/v1/chat/completions")

	logPath := filepath.Join(dir, "nenya.log")
	logFile, err := os.Create(logPath)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = logFile.Close() }()

	cmd := exec.CommandContext(ctx, binPath, "-config", filepath.Join(configDir, "config.json"))
	cmd.Env = append(os.Environ(), "NENYA_SECRETS_DIR="+configDir)
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	if err := cmd.Start(); err != nil {
		t.Fatalf("start nenya: %v", err)
	}
	exited := make(chan error, 1)
	go func() { exited <- cmd.Wait() }()
	t.Cleanup(func() {
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
	})

	baseURL := "http://" + listen
	waitForHealthz(t, ctx, baseURL, logPath)

	// Authenticated surface: the model catalog.
	doAuthGET(t, ctx, baseURL+"/v1/models", token, logPath)

	// Golden path: a streamed chat completion through the mock upstream.
	status, streamed := doStream(t, ctx, baseURL+"/v1/chat/completions", token)
	if status != http.StatusOK {
		t.Fatalf("POST /v1/chat/completions = %d, want 200\n%s", status, tailFile(logPath))
	}
	if !strings.Contains(streamed, "hello-e2e") {
		t.Fatalf("stream did not contain the upstream content:\n%s", streamed)
	}

	// Feature-detect the contract version (vacuous on pre-contract releases).
	assertContractVersion(ctx, t, binPath, configDir, logPath)

	// Graceful shutdown per CONTRACT.md §8.3.
	if err := cmd.Process.Signal(syscall.SIGTERM); err != nil {
		t.Fatalf("SIGTERM: %v", err)
	}
	select {
	case err := <-exited:
		if err != nil {
			t.Fatalf("nenya exited non-zero after SIGTERM: %v\n%s", err, tailFile(logPath))
		}
	case <-time.After(35 * time.Second):
		t.Fatalf("nenya did not exit after SIGTERM\n%s", tailFile(logPath))
	}
}

// mockUpstream is an OpenAI-compatible SSE endpoint that asserts the request
// shape so a nenya regression (wrong method/path/body) fails the test.
func mockUpstream() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1/chat/completions" {
			http.Error(w, "unexpected request", http.StatusBadRequest)
			return
		}
		var body struct {
			Model  string `json:"model"`
			Stream bool   `json:"stream"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, "bad body", http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		_, _ = fmt.Fprint(w, "data: {\"id\":\"1\",\"object\":\"chat.completion.chunk\",\"choices\":[{\"index\":0,\"delta\":{\"role\":\"assistant\",\"content\":\"hello-e2e\"}}]}\n\n")
		_, _ = fmt.Fprint(w, "data: [DONE]\n\n")
	})
}

func writeConfig(t *testing.T, path, listen, upstreamURL string) {
	t.Helper()
	config := fmt.Sprintf(`{
  "server": {"listen_addr": %q},
  "discovery": {"enabled": false},
  "providers": {"mock": {"url": %q, "auth_style": "none"}},
  "agents": {"e2e": {"strategy": "fallback", "models": [{"provider": "mock", "model": "e2e-model", "max_context": 8192}]}}
}`, listen, upstreamURL)
	if err := os.WriteFile(path, []byte(config), 0o644); err != nil {
		t.Fatal(err)
	}
}

func readClientToken(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var s struct {
		ClientToken string `json:"client_token"`
	}
	if err := json.Unmarshal(data, &s); err != nil {
		t.Fatalf("parse secrets: %v", err)
	}
	if s.ClientToken == "" {
		t.Fatal("install generated an empty client_token")
	}
	return s.ClientToken
}

func doAuthGET(t *testing.T, ctx context.Context, url, token, logPath string) {
	t.Helper()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET %s: %v", url, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET %s = %s, want 200\n%s", url, resp.Status, tailFile(logPath))
	}
}

func doStream(t *testing.T, ctx context.Context, url, token string) (int, string) {
	t.Helper()
	body := `{"model":"e2e","messages":[{"role":"user","content":"hi"}],"stream":true}`
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, url, strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST %s: %v", url, err)
	}
	defer func() { _ = resp.Body.Close() }()
	out, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		t.Fatalf("read stream: %v", err)
	}
	return resp.StatusCode, string(out)
}

// assertContractVersion probes `nenya describe --json` with a bounded timeout
// so an older binary ignores the command instead of starting a server. It is
// vacuous on releases that predate the command and enforces the range once it
// ships.
func assertContractVersion(ctx context.Context, t *testing.T, binPath, configDir, logPath string) {
	t.Helper()
	probeCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	cmd := exec.CommandContext(probeCtx, binPath, "describe", "--json")
	cmd.Env = append(os.Environ(), "NENYA_SECRETS_DIR="+configDir)
	out, err := cmd.Output()
	if err != nil {
		t.Logf("describe --json not available yet (contract target): %v", err)
		return
	}
	var described struct {
		ContractVersion int `json:"contract_version"`
	}
	if err := json.Unmarshal(out, &described); err != nil {
		t.Fatalf("describe --json returned unparseable JSON: %v\n%s", err, tailFile(logPath))
	}
	if err := contract.Check(described.ContractVersion); err != nil {
		t.Fatalf("contract mismatch: %v", err)
	}
}

func latestTag(ctx context.Context) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		fmt.Sprintf("https://api.github.com/repos/%s/%s/releases/latest", owner, repo), nil)
	if err != nil {
		return "", err
	}
	if token := os.Getenv("GITHUB_TOKEN"); token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("unexpected status %s", resp.Status)
	}
	var rel struct {
		TagName string `json:"tag_name"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&rel); err != nil {
		return "", err
	}
	if rel.TagName == "" {
		return "", fmt.Errorf("latest release has no tag_name")
	}
	return rel.TagName, nil
}

func waitForHealthz(t *testing.T, ctx context.Context, baseURL, logPath string) {
	t.Helper()
	client := &http.Client{Timeout: 3 * time.Second}
	deadline := time.Now().Add(60 * time.Second)
	for time.Now().Before(deadline) {
		reqCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
		req, _ := http.NewRequestWithContext(reqCtx, http.MethodGet, baseURL+"/healthz", nil)
		resp, err := client.Do(req)
		cancel()
		if err == nil {
			_ = resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return
			}
		}
		time.Sleep(500 * time.Millisecond)
	}
	t.Fatalf("nenya never became healthy at %s/healthz\n%s", baseURL, tailFile(logPath))
}

func freeAddr(t *testing.T) string {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = l.Close() }()
	return l.Addr().String()
}

func tailFile(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	if len(data) > 8000 {
		data = data[len(data)-8000:]
	}
	return string(data)
}
