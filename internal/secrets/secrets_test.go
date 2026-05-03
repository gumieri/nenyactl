package secrets

import (
	"strings"
	"testing"
)

func TestGenerateClientToken(t *testing.T) {
	token := GenerateClientToken()
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
		token := GenerateClientToken()
		if seen[token] {
			t.Errorf("duplicate token generated: %q", token)
		}
		seen[token] = true
	}
}

func TestGenerateAPIKey(t *testing.T) {
	key, name := GenerateAPIKey()
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
		key, _ := GenerateAPIKey()
		if seen[key] {
			t.Errorf("duplicate API key generated: %q", key)
		}
		seen[key] = true
	}
}
