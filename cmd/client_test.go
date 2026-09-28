package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gumieri/nenyactl/internal/clients"
)

func TestResolvedEndpoint(t *testing.T) {
	// contractDispatch makes the recording runner answer per command: describe
	// returns describeJSON, paths errors (local fallback), and secret get
	// returns tokenJSON (empty token falls back to the file shim).
	contractDispatch := func(rr *recordingRunner, describeJSON, tokenJSON string) {
		rr.rec.onCall = func(args []string) {
			rr.err = nil
			switch args[0] {
			case "describe":
				rr.out = []byte(describeJSON)
			case "paths":
				rr.err = errors.New("nenya paths: unknown command")
			case "secret":
				rr.out = []byte(tokenJSON)
			}
		}
	}

	t.Run("reads the published port and token from a container dir", func(t *testing.T) {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, "compose.yml"), []byte("services:\n  nenya:\n    ports:\n      - \"127.0.0.1:9090:8080\"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(filepath.Join(dir, "secrets"), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "secrets", "01-client.json"), []byte(`{"client_token":"nk-container"}`), 0o600); err != nil {
			t.Fatal(err)
		}

		// describe reports no listen_addr, so the compose mapping is used.
		rr := newRecordingRunner()
		rr.out = []byte(`{"contract_version":1,"config":{}}`)
		fakeContract(t, rr)

		res, err := resolveDir(context.Background(), dir, dirAttach, false)
		if err != nil {
			t.Fatal(err)
		}
		ep, err := resolvedEndpoint(context.Background(), res)
		if err != nil {
			t.Fatal(err)
		}
		if ep.BaseURL != "http://localhost:9090" {
			t.Errorf("BaseURL = %s", ep.BaseURL)
		}
		if ep.Token != "nk-container" {
			t.Errorf("Token = %s", ep.Token)
		}
	})

	t.Run("uses the compose published port even when describe reports the internal port", func(t *testing.T) {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, "compose.yml"), []byte("services:\n  nenya:\n    ports:\n      - \"127.0.0.1:9090:8080\"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(filepath.Join(dir, "secrets"), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "secrets", "01-client.json"), []byte(`{"client_token":"nk-container"}`), 0o600); err != nil {
			t.Fatal(err)
		}

		// describe reports the container-internal :8080; the host port is the
		// compose mapping (9090).
		rr := newRecordingRunner()
		rr.out = []byte(`{"contract_version":1,"config":{"server":{"listen_addr":":8080"}}}`)
		fakeContract(t, rr)

		res, err := resolveDir(context.Background(), dir, dirAttach, false)
		if err != nil {
			t.Fatal(err)
		}
		ep, err := resolvedEndpoint(context.Background(), res)
		if err != nil {
			t.Fatal(err)
		}
		if ep.BaseURL != "http://localhost:9090" {
			t.Errorf("BaseURL = %s, want the compose published port", ep.BaseURL)
		}
	})

	t.Run("reads a bare-metal secrets.json", func(t *testing.T) {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, "config.json"), []byte("{}"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "secrets.json"), []byte(`{"client_token":"nk-bare"}`), 0o600); err != nil {
			t.Fatal(err)
		}

		rr := newRecordingRunner()
		contractDispatch(rr, `{"contract_version":1,"config":{"server":{"listen_addr":":8080"}}}`, "")
		fakeContract(t, rr)

		res, err := resolveDir(context.Background(), dir, dirAttach, false)
		if err != nil {
			t.Fatal(err)
		}
		ep, err := resolvedEndpoint(context.Background(), res)
		if err != nil {
			t.Fatal(err)
		}
		// secret get is unavailable (contractDispatch errors it), so the token
		// comes from the documented file shim.
		if ep.Token != "nk-bare" {
			t.Errorf("Token = %s", ep.Token)
		}
		if ep.BaseURL != "http://localhost:8080" {
			t.Errorf("BaseURL = %s", ep.BaseURL)
		}
	})

	t.Run("bare-metal reads the token through nenya secret get when available", func(t *testing.T) {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, "config.json"), []byte("{}"), 0o644); err != nil {
			t.Fatal(err)
		}
		// A stale file exists, but the contract's answer is authoritative and
		// different: the file shim must not win.
		if err := os.WriteFile(filepath.Join(dir, "secrets.json"), []byte(`{"client_token":"nk-stale"}`), 0o600); err != nil {
			t.Fatal(err)
		}

		rr := newRecordingRunner()
		contractDispatch(rr, `{"contract_version":1,"config":{"server":{"listen_addr":":8080"}}}`, "nk-contract")
		fakeContract(t, rr)

		res, err := resolveDir(context.Background(), dir, dirAttach, false)
		if err != nil {
			t.Fatal(err)
		}
		ep, err := resolvedEndpoint(context.Background(), res)
		if err != nil {
			t.Fatal(err)
		}
		if ep.Token != "nk-contract" {
			t.Errorf("Token = %s, want the contract-resolved token", ep.Token)
		}
	})

	t.Run("bare-metal reads the port from the effective describe config", func(t *testing.T) {
		dir := t.TempDir()
		// The raw config.json has no listen_addr; the effective config from
		// describe does (e.g. from a config.d overlay), and must win.
		if err := os.WriteFile(filepath.Join(dir, "config.json"), []byte("{}"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "secrets.json"), []byte(`{"client_token":"nk-bare"}`), 0o600); err != nil {
			t.Fatal(err)
		}

		rr := newRecordingRunner()
		contractDispatch(rr, `{"contract_version":1,"config":{"server":{"listen_addr":":9090"}}}`, "nk-contract")
		fakeContract(t, rr)

		res, err := resolveDir(context.Background(), dir, dirAttach, false)
		if err != nil {
			t.Fatal(err)
		}
		ep, err := resolvedEndpoint(context.Background(), res)
		if err != nil {
			t.Fatal(err)
		}
		if ep.BaseURL != "http://localhost:9090" {
			t.Errorf("BaseURL = %s, want the describe-effective :9090", ep.BaseURL)
		}
	})

	t.Run("errors when no token exists", func(t *testing.T) {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, "config.json"), []byte("{}"), 0o644); err != nil {
			t.Fatal(err)
		}
		rr := newRecordingRunner()
		contractDispatch(rr, `{"contract_version":1,"config":{"server":{"listen_addr":":8080"}}}`, "")
		fakeContract(t, rr)

		res, err := resolveDir(context.Background(), dir, dirAttach, false)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := resolvedEndpoint(context.Background(), res); err == nil {
			t.Fatal("expected error without a token")
		}
	})

	t.Run("errors when no port can be resolved", func(t *testing.T) {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, "config.json"), []byte("{}"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "secrets.json"), []byte(`{"client_token":"nk-bare"}`), 0o600); err != nil {
			t.Fatal(err)
		}
		rr := newRecordingRunner()
		rr.out = []byte(`{"contract_version":1,"config":{}}`)
		fakeContract(t, rr)

		res, err := resolveDir(context.Background(), dir, dirAttach, false)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := resolvedEndpoint(context.Background(), res); err == nil {
			t.Fatal("expected error without a resolvable port")
		}
	})
}

func TestRedactToken(t *testing.T) {
	body := "apiKey: nk-abc\nbaseURL: http://x"
	if got := redactToken(body, "nk-abc"); strings.Contains(got, "nk-abc") || !strings.Contains(got, "<client-token>") {
		t.Errorf("redactToken = %q", got)
	}
	if got := redactToken(body, ""); got != body {
		t.Errorf("empty token should not alter body: %q", got)
	}
}

func TestExpandHome(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	got, err := expandHome("~/.config/opencode/opencode.json")
	if err != nil {
		t.Fatal(err)
	}
	if got != filepath.Join(home, ".config", "opencode", "opencode.json") {
		t.Errorf("expandHome = %s", got)
	}
	if got, err := expandHome("/abs/path"); err != nil || got != "/abs/path" {
		t.Errorf("absolute path altered: %s, %v", got, err)
	}
}

func TestWriteClientConfigMerges(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	cfgPath := filepath.Join(home, ".config", "opencode", "opencode.json")
	if err := os.MkdirAll(filepath.Dir(cfgPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cfgPath, []byte(`{"theme":"dark"}`), 0o644); err != nil {
		t.Fatal(err)
	}

	endpoint := clients.Endpoint{BaseURL: "http://localhost:9090", Token: "nk-test"}
	snippet, err := clients.Render(clients.OpenCode, endpoint)
	if err != nil {
		t.Fatal(err)
	}

	if err := writeClientConfig(snippet, endpoint); err != nil {
		t.Fatalf("writeClientConfig: %v", err)
	}

	data, err := os.ReadFile(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	var root map[string]any
	if err := json.Unmarshal(data, &root); err != nil {
		t.Fatal(err)
	}
	if root["theme"] != "dark" {
		t.Error("unrelated key not preserved")
	}
	providers, ok := root["providers"].(map[string]any)
	if !ok {
		t.Fatal("providers object missing")
	}
	nenya, ok := providers["nenya"].(map[string]any)
	if !ok {
		t.Fatal("nenya provider not added")
	}
	if nenya["package"] != "@opencode/ai/providers/openai-compatible" {
		t.Errorf("package = %v", nenya["package"])
	}
	settings, ok := nenya["settings"].(map[string]any)
	if !ok {
		t.Fatal("nenya provider has no settings")
	}
	if settings["apiKey"] != "nk-test" {
		t.Errorf("apiKey = %v", settings["apiKey"])
	}
	if _, ok := root["provider"]; ok {
		t.Error("V1 provider key should not be written")
	}
	info, err := os.Stat(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Errorf("config mode = %o, want 0600", info.Mode().Perm())
	}
}
