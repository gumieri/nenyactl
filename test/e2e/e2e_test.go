// Package e2e exercises the real nenyactl <-> nenya seam against a real nenya
// release. It is gated on NENYACTL_E2E=1 so it never runs in the unit job.
//
// It downloads the latest (or NENYA_E2E_VERSION) release, verifies its SHA-256,
// extracts the real archive, then starts the real binary against a mock
// OpenAI-compatible upstream and asserts the golden path: health, an
// authenticated request, and a streamed chat completion.
package e2e

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
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
	version := strings.TrimPrefix(tag, "v")
	t.Logf("nenya release under test: %s (%s/%s)", tag, runtime.GOOS, runtime.GOARCH)

	dir := t.TempDir()
	binPath := filepath.Join(dir, "nenya")
	members, err := downloadAndExtract(ctx, tag, version, binPath)
	if err != nil {
		t.Fatalf("download and extract release: %v", err)
	}
	assertArchiveLayout(t, members)

	// The mock upstream is an OpenAI-compatible SSE endpoint.
	mock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		_, _ = fmt.Fprint(w, "data: {\"id\":\"1\",\"object\":\"chat.completion.chunk\",\"choices\":[{\"index\":0,\"delta\":{\"role\":\"assistant\",\"content\":\"hello-e2e\"}}]}\n\n")
		_, _ = fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer mock.Close()

	configDir := filepath.Join(dir, "config")
	secretsDir := filepath.Join(dir, "secrets")
	if err := os.MkdirAll(secretsDir, 0o700); err != nil {
		t.Fatal(err)
	}
	listen := freeAddr(t)

	config := fmt.Sprintf(`{
  "server": {"listen_addr": %q},
  "discovery": {"enabled": false},
  "providers": {"mock": {"url": %q, "auth_style": "none"}},
  "agents": {"e2e": {"strategy": "fallback", "models": [{"provider": "mock", "model": "e2e-model", "max_context": 8192}]}}
}`, listen, mock.URL+"/v1/chat/completions")
	configFile := filepath.Join(configDir, "config.json")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(configFile, []byte(config), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(secretsDir, "secrets.json"), []byte(`{"client_token":"nk-e2e-test"}`), 0o600); err != nil {
		t.Fatal(err)
	}

	logPath := filepath.Join(dir, "nenya.log")
	logFile, err := os.Create(logPath)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = logFile.Close() }()

	cmd := exec.CommandContext(ctx, binPath, "-config", configFile)
	cmd.Env = append(os.Environ(), "NENYA_SECRETS_DIR="+secretsDir)
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
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/v1/models", nil)
	req.Header.Set("Authorization", "Bearer nk-e2e-test")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET /v1/models: %v", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /v1/models = %s, want 200", resp.Status)
	}

	// Golden path: a streamed chat completion through the mock upstream.
	body := `{"model":"e2e","messages":[{"role":"user","content":"hi"}],"stream":true}`
	req, _ = http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/v1/chat/completions", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer nk-e2e-test")
	req.Header.Set("Content-Type", "application/json")
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST /v1/chat/completions: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("POST /v1/chat/completions = %s, want 200", resp.Status)
	}
	streamed, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		t.Fatalf("read stream: %v", err)
	}
	if !bytes.Contains(streamed, []byte("hello-e2e")) {
		t.Fatalf("stream did not contain the upstream content:\n%s", streamed)
	}

	// Feature-detect the target contract surface; assert version when present.
	assertContractVersion(ctx, t, binPath)

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

// downloadAndExtract downloads the release archive + checksums, verifies the
// SHA-256, extracts the `nenya` member to binPath, and returns all member names.
func downloadAndExtract(ctx context.Context, tag, version, binPath string) ([]string, error) {
	base := fmt.Sprintf("https://github.com/%s/%s/releases/download/%s/", owner, repo, tag)
	archiveName := fmt.Sprintf("nenya_%s_%s_%s.tar.gz", version, runtime.GOOS, runtime.GOARCH)

	archive, err := httpGet(ctx, base+archiveName)
	if err != nil {
		return nil, fmt.Errorf("download %s: %w", archiveName, err)
	}
	checksums, err := httpGet(ctx, base+"checksums.txt")
	if err != nil {
		return nil, fmt.Errorf("download checksums.txt: %w", err)
	}

	want := ""
	for _, line := range strings.Split(string(checksums), "\n") {
		fields := strings.Fields(line)
		if len(fields) == 2 && fields[1] == archiveName {
			want = strings.ToLower(fields[0])
		}
	}
	if want == "" {
		return nil, fmt.Errorf("checksums.txt has no entry for %s", archiveName)
	}
	sum := sha256.Sum256(archive)
	if got := hex.EncodeToString(sum[:]); got != want {
		return nil, fmt.Errorf("checksum mismatch for %s: want %s got %s", archiveName, want, got)
	}

	gz, err := gzip.NewReader(bytes.NewReader(archive))
	if err != nil {
		return nil, err
	}
	defer func() { _ = gz.Close() }()

	var members []string
	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		members = append(members, hdr.Name)
		if hdr.Name != "nenya" {
			continue
		}
		out, err := os.OpenFile(binPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
		if err != nil {
			return nil, err
		}
		if _, err := io.Copy(out, tr); err != nil {
			_ = out.Close()
			return nil, err
		}
		if err := out.Close(); err != nil {
			return nil, err
		}
	}
	return members, nil
}

// assertArchiveLayout enforces the CONTRACT.md §7.1 member set.
func assertArchiveLayout(t *testing.T, members []string) {
	t.Helper()
	has := func(name string) bool {
		for _, m := range members {
			if m == name {
				return true
			}
		}
		return false
	}
	if !has("nenya") {
		t.Errorf("archive is missing the nenya member; got %v", members)
	}
	switch runtime.GOOS {
	case "linux":
		for _, m := range []string{"deploy/nenya.service", "deploy/nenya.socket"} {
			if !has(m) {
				t.Errorf("archive is missing %s; got %v", m, members)
			}
		}
	case "darwin":
		if !has("deploy/nenya.plist") {
			t.Errorf("archive is missing deploy/nenya.plist; got %v", members)
		}
	}
}

// assertContractVersion feature-detects `nenya describe --json`; once it ships,
// the reported contract_version must be within nenyactl's supported range.
func assertContractVersion(ctx context.Context, t *testing.T, binPath string) {
	t.Helper()
	out, err := exec.CommandContext(ctx, binPath, "describe", "--json").Output()
	if err != nil {
		t.Logf("describe --json not available yet (contract target): %v", err)
		return
	}
	var described struct {
		ContractVersion int `json:"contract_version"`
	}
	if err := json.Unmarshal(out, &described); err != nil {
		t.Fatalf("describe --json returned unparseable JSON: %v", err)
	}
	if err := contract.Check(described.ContractVersion); err != nil {
		t.Fatalf("contract mismatch: %v", err)
	}
}

func latestTag(ctx context.Context) (string, error) {
	body, err := httpGet(ctx, fmt.Sprintf("https://api.github.com/repos/%s/%s/releases/latest", owner, repo))
	if err != nil {
		return "", err
	}
	var rel struct {
		TagName string `json:"tag_name"`
	}
	if err := json.Unmarshal(body, &rel); err != nil {
		return "", err
	}
	if rel.TagName == "" {
		return "", fmt.Errorf("latest release has no tag_name")
	}
	return rel.TagName, nil
}

func httpGet(ctx context.Context, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status %s for %s", resp.Status, url)
	}
	return io.ReadAll(resp.Body)
}

func waitForHealthz(t *testing.T, ctx context.Context, baseURL, logPath string) {
	t.Helper()
	deadline := time.Now().Add(60 * time.Second)
	for time.Now().Before(deadline) {
		req, _ := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/healthz", nil)
		resp, err := http.DefaultClient.Do(req)
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
