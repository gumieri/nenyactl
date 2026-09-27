package containers

import (
	"os"
	"path/filepath"
	"testing"
)

func TestHostPortMapping(t *testing.T) {
	cases := []struct {
		listen string
		want   string
	}{
		{"", "8080:8080"},
		{":8080", "8080:8080"},
		{"8080", "8080:8080"},
		{"0.0.0.0:8080", "8080:8080"},
		{":9090", "9090:8080"},
		{"9090", "9090:8080"},
		{"127.0.0.1:9090", "127.0.0.1:9090:8080"},
		{"localhost:8080", "localhost:8080:8080"},
	}
	for _, c := range cases {
		got, err := HostPortMapping(c.listen)
		if err != nil {
			t.Errorf("HostPortMapping(%q) error: %v", c.listen, err)
			continue
		}
		if got != c.want {
			t.Errorf("HostPortMapping(%q) = %q, want %q", c.listen, got, c.want)
		}
	}

	if _, err := HostPortMapping("not:a:valid:listen:addr"); err == nil {
		t.Error("expected error for invalid listen address")
	}
}

func TestPublishedPort(t *testing.T) {
	t.Run("reads host port from compose", func(t *testing.T) {
		dir := t.TempDir()
		compose := "services:\n  nenya:\n    ports:\n      - \"127.0.0.1:9090:8080\"\n    volumes:\n      - ./config:/etc/nenya:ro\n"
		if err := os.WriteFile(filepath.Join(dir, "compose.yml"), []byte(compose), 0o644); err != nil {
			t.Fatal(err)
		}
		if got := PublishedPort(dir); got != "9090" {
			t.Errorf("PublishedPort = %q, want 9090", got)
		}
	})

	t.Run("falls back to .env PORT", func(t *testing.T) {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, ".env"), []byte("NENYA_IMAGE=x\nPORT=7070\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		if got := PublishedPort(dir); got != "7070" {
			t.Errorf("PublishedPort = %q, want 7070", got)
		}
	})

	t.Run("defaults to 8080", func(t *testing.T) {
		if got := PublishedPort(t.TempDir()); got != "8080" {
			t.Errorf("PublishedPort = %q, want 8080", got)
		}
	})
}

func TestClientToken(t *testing.T) {
	t.Run("merges secrets in name order, last non-empty wins", func(t *testing.T) {
		dir := t.TempDir()
		secretsDir := filepath.Join(dir, "secrets")
		if err := os.MkdirAll(secretsDir, 0o700); err != nil {
			t.Fatal(err)
		}
		_ = os.WriteFile(filepath.Join(secretsDir, "01-client.json"), []byte(`{"client_token":"nk-first"}`), 0o600)
		_ = os.WriteFile(filepath.Join(secretsDir, "02-providers.json"), []byte(`{"provider_keys":{}}`), 0o600)
		_ = os.WriteFile(filepath.Join(secretsDir, "03-override.json"), []byte(`{"client_token":"nk-last"}`), 0o600)

		if got := ClientToken(dir); got != "nk-last" {
			t.Errorf("ClientToken = %q, want nk-last", got)
		}
	})

	t.Run("falls back to secrets.json", func(t *testing.T) {
		dir := t.TempDir()
		_ = os.WriteFile(filepath.Join(dir, "secrets.json"), []byte(`{"client_token":"nk-single"}`), 0o600)
		if got := ClientToken(dir); got != "nk-single" {
			t.Errorf("ClientToken = %q, want nk-single", got)
		}
	})

	t.Run("missing secrets returns empty", func(t *testing.T) {
		if got := ClientToken(t.TempDir()); got != "" {
			t.Errorf("ClientToken = %q, want empty", got)
		}
	})
}
