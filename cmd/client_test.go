package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/gumieri/nenyactl/internal/clients"
)

func TestResolvedEndpoint(t *testing.T) {
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

		res, err := resolveDir(dir, false)
		if err != nil {
			t.Fatal(err)
		}
		ep, err := resolvedEndpoint(res)
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

	t.Run("reads a bare-metal secrets.json", func(t *testing.T) {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, "config.json"), []byte("{}"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "secrets.json"), []byte(`{"client_token":"nk-bare"}`), 0o600); err != nil {
			t.Fatal(err)
		}

		res, err := resolveDir(dir, false)
		if err != nil {
			t.Fatal(err)
		}
		ep, err := resolvedEndpoint(res)
		if err != nil {
			t.Fatal(err)
		}
		if ep.Token != "nk-bare" {
			t.Errorf("Token = %s", ep.Token)
		}
		if ep.BaseURL != "http://localhost:8080" {
			t.Errorf("BaseURL = %s", ep.BaseURL)
		}
	})

	t.Run("errors when no token exists", func(t *testing.T) {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, "config.json"), []byte("{}"), 0o644); err != nil {
			t.Fatal(err)
		}
		res, err := resolveDir(dir, false)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := resolvedEndpoint(res); err == nil {
			t.Fatal("expected error without a token")
		}
	})
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
	if _, ok := root["provider"].(map[string]any)["nenya"]; !ok {
		t.Error("nenya provider not added")
	}
}
