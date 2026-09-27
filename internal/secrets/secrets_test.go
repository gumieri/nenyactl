package secrets

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGenerateClientToken(t *testing.T) {
	token, err := GenerateClientToken()
	if err != nil {
		t.Fatalf("GenerateClientToken() error = %v", err)
	}
	if token == "" {
		t.Fatal("GenerateClientToken() returned empty string")
	}

	if !strings.HasPrefix(token, "nk-") {
		t.Errorf("GenerateClientToken() = %q, want prefix nk-", token)
	}

	if len(token) < 20 {
		t.Errorf("GenerateClientToken() length = %d, want >= 20", len(token))
	}
}

func TestGenerateClientTokenUniqueness(t *testing.T) {
	seen := make(map[string]bool)
	for i := 0; i < 100; i++ {
		token, err := GenerateClientToken()
		if err != nil {
			t.Fatalf("GenerateClientToken() error = %v", err)
		}
		if seen[token] {
			t.Errorf("duplicate token generated: %q", token)
		}
		seen[token] = true
	}
}

func TestGenerateAPIKey(t *testing.T) {
	key, name, err := GenerateAPIKey()
	if err != nil {
		t.Fatalf("GenerateAPIKey() error = %v", err)
	}
	if key == "" {
		t.Error("GenerateAPIKey() returned empty key")
	}
	if name == "" {
		t.Error("GenerateAPIKey() returned empty name")
	}

	if len(key) < 8 {
		t.Errorf("GenerateAPIKey() key length = %d, want >= 8", len(key))
	}
}

func TestGenerateAPIKeyUniqueness(t *testing.T) {
	seen := make(map[string]bool)
	for i := 0; i < 100; i++ {
		key, _, err := GenerateAPIKey()
		if err != nil {
			t.Fatalf("GenerateAPIKey() error = %v", err)
		}
		if seen[key] {
			t.Errorf("duplicate API key generated: %q", key)
		}
		seen[key] = true
	}
}

func TestExistingTokenFile(t *testing.T) {
	t.Run("empty dir", func(t *testing.T) {
		if got := ExistingTokenFile(t.TempDir()); got != "" {
			t.Errorf("got %q, want empty", got)
		}
	})

	t.Run("missing dir", func(t *testing.T) {
		if got := ExistingTokenFile("/nonexistent/secrets"); got != "" {
			t.Errorf("got %q, want empty", got)
		}
	})

	t.Run("empty dir argument", func(t *testing.T) {
		if got := ExistingTokenFile(""); got != "" {
			t.Errorf("got %q, want empty", got)
		}
	})

	t.Run("secrets.json", func(t *testing.T) {
		dir := t.TempDir()
		writeFile(t, dir, "secrets.json", `{"client_token":"nk-a"}`)
		if got := ExistingTokenFile(dir); got != filepath.Join(dir, "secrets.json") {
			t.Errorf("got %q", got)
		}
	})

	t.Run("single secrets file", func(t *testing.T) {
		dir := t.TempDir()
		writeFile(t, dir, "secrets", `{"client_token":"nk-b"}`)
		if got := ExistingTokenFile(dir); got != filepath.Join(dir, "secrets") {
			t.Errorf("got %q", got)
		}
	})

	t.Run("arbitrary json drop-in", func(t *testing.T) {
		dir := t.TempDir()
		writeFile(t, dir, "01-client.json", `{"client_token":"nk-c"}`)
		if got := ExistingTokenFile(dir); got != filepath.Join(dir, "01-client.json") {
			t.Errorf("got %q", got)
		}
	})

	t.Run("json without a token is ignored", func(t *testing.T) {
		dir := t.TempDir()
		writeFile(t, dir, "providers.json", `{"provider_keys":{}}`)
		if got := ExistingTokenFile(dir); got != "" {
			t.Errorf("got %q, want empty", got)
		}
	})

	t.Run("unparseable file is ignored", func(t *testing.T) {
		dir := t.TempDir()
		writeFile(t, dir, "secrets.json", `not json`)
		if got := ExistingTokenFile(dir); got != "" {
			t.Errorf("got %q, want empty", got)
		}
	})
}

func TestHasClientToken(t *testing.T) {
	if !HasClientToken([]byte(`{"client_token":"nk-x"}`)) {
		t.Error("expected true for a token")
	}
	if HasClientToken([]byte(`{"client_token":""}`)) {
		t.Error("expected false for an empty token")
	}
	if HasClientToken([]byte(`{"other":1}`)) {
		t.Error("expected false without a token")
	}
	if HasClientToken([]byte(`not json`)) {
		t.Error("expected false for unparseable data")
	}
}

func TestClientTokenFromJSON(t *testing.T) {
	if got := ClientTokenFromJSON([]byte(`{"client_token":"nk-x"}`)); got != "nk-x" {
		t.Errorf("ClientTokenFromJSON = %q, want nk-x", got)
	}
	for _, bad := range []string{`{}`, `{"client_token":""}`, `not json`} {
		if got := ClientTokenFromJSON([]byte(bad)); got != "" {
			t.Errorf("ClientTokenFromJSON(%q) = %q, want empty", bad, got)
		}
	}
}

func TestClientTokenInDir(t *testing.T) {
	t.Run("last non-empty token wins in name order", func(t *testing.T) {
		dir := t.TempDir()
		writeFile(t, dir, "01-client.json", `{"client_token":"nk-first"}`)
		writeFile(t, dir, "02-providers.json", `{"provider_keys":{}}`)
		writeFile(t, dir, "03-override.json", `{"client_token":"nk-last"}`)
		if got := ClientTokenInDir(dir); got != "nk-last" {
			t.Errorf("ClientTokenInDir = %q, want nk-last", got)
		}
	})

	t.Run("falls back to secrets.json", func(t *testing.T) {
		dir := t.TempDir()
		writeFile(t, dir, "secrets.json", `{"client_token":"nk-single"}`)
		if got := ClientTokenInDir(dir); got != "nk-single" {
			t.Errorf("ClientTokenInDir = %q, want nk-single", got)
		}
	})

	t.Run("missing dir returns empty", func(t *testing.T) {
		if got := ClientTokenInDir(filepath.Join(t.TempDir(), "nope")); got != "" {
			t.Errorf("ClientTokenInDir = %q, want empty", got)
		}
	})
}

func writeFile(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}
