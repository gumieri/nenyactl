package secrets

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestProviderKeyNames(t *testing.T) {
	t.Run("returns names from secrets.json without values", func(t *testing.T) {
		dir := t.TempDir()
		doc := `{"client_token": "nk-x", "provider_keys": {"openai": "sk-a", "zai": "sk-b"}}`
		if err := os.WriteFile(filepath.Join(dir, "secrets.json"), []byte(doc), 0o600); err != nil {
			t.Fatal(err)
		}
		got := ProviderKeyNames(dir)
		want := []string{"openai", "zai"}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("ProviderKeyNames = %v, want %v", got, want)
		}
	})

	t.Run("merges additional json documents in name order", func(t *testing.T) {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, "secrets.json"), []byte(`{"provider_keys": {"openai": "sk-a"}}`), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "zz-extra.json"), []byte(`{"provider_keys": {"anthropic": "sk-b"}}`), 0o600); err != nil {
			t.Fatal(err)
		}
		got := ProviderKeyNames(dir)
		want := []string{"anthropic", "openai"}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("ProviderKeyNames = %v, want %v", got, want)
		}
	})

	t.Run("empty values do not count as set", func(t *testing.T) {
		dir := t.TempDir()
		doc := `{"provider_keys": {"openai": "", "zai": "   "}}`
		if err := os.WriteFile(filepath.Join(dir, "secrets.json"), []byte(doc), 0o600); err != nil {
			t.Fatal(err)
		}
		if got := ProviderKeyNames(dir); len(got) != 0 {
			t.Errorf("ProviderKeyNames = %v, want empty (blank values are unset)", got)
		}
	})

	t.Run("missing directory returns nil", func(t *testing.T) {
		if got := ProviderKeyNames(filepath.Join(t.TempDir(), "nope")); got != nil {
			t.Errorf("ProviderKeyNames = %v, want nil", got)
		}
		if got := ProviderKeyNames(""); got != nil {
			t.Errorf("ProviderKeyNames(\"\") = %v, want nil", got)
		}
	})
}
