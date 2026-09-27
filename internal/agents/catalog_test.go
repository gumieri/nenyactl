package agents

import (
	"sort"
	"testing"
)

func TestCatalogFromDescribe(t *testing.T) {
	t.Run("uses the provider catalog and sorts", func(t *testing.T) {
		c := CatalogFromDescribe([][2]string{
			{"gemini", "gemini-2.5-flash"},
			{"anthropic", "claude-sonnet-4-5"},
			{"anthropic", "claude-opus-4-7"},
		})

		want := []CatalogModel{
			{Provider: "anthropic", Model: "claude-opus-4-7"},
			{Provider: "anthropic", Model: "claude-sonnet-4-5"},
			{Provider: "gemini", Model: "gemini-2.5-flash"},
		}
		if len(c.Models) != len(want) {
			t.Fatalf("models = %d, want %d: %v", len(c.Models), len(want), c.Models)
		}
		for i := range want {
			if c.Models[i] != want[i] {
				t.Errorf("models[%d] = %v, want %v", i, c.Models[i], want[i])
			}
		}
	})

	t.Run("de-duplicates entries", func(t *testing.T) {
		c := CatalogFromDescribe([][2]string{
			{"anthropic", "claude-sonnet-4-5"},
			{"anthropic", "claude-sonnet-4-5"},
		})
		if len(c.Models) != 1 {
			t.Fatalf("models = %v, want one entry", c.Models)
		}
	})

	t.Run("ignores configured providers with no catalog entry", func(t *testing.T) {
		// nenya owns the catalog: a configured provider without a catalog
		// entry must not become a fabricated model row.
		c := CatalogFromDescribe([][2]string{{"anthropic", "claude-sonnet-4-5"}})
		if len(c.Models) != 1 {
			t.Fatalf("models = %v, want only the real catalog entry", c.Models)
		}
		if c.Models[0].Provider != "anthropic" || c.Models[0].Model != "claude-sonnet-4-5" {
			t.Errorf("models = %v", c.Models)
		}
	})

	t.Run("empty describe yields an empty catalog", func(t *testing.T) {
		if c := CatalogFromDescribe(nil); len(c.Models) != 0 {
			t.Errorf("models = %v, want empty", c.Models)
		}
	})

	t.Run("ignores blank provider or model", func(t *testing.T) {
		c := CatalogFromDescribe([][2]string{{"", "x"}, {"y", ""}, {"", ""}})
		if len(c.Models) != 0 {
			t.Errorf("models = %v, want empty", c.Models)
		}
	})
}

func TestStrategies(t *testing.T) {
	want := map[string]bool{"fallback": true, "round-robin": true}
	for _, s := range Strategies {
		if !want[s] {
			t.Errorf("unexpected strategy %q", s)
		}
		delete(want, s)
	}
	if len(want) != 0 {
		t.Errorf("missing strategies: %v", want)
	}
	if !sort.StringsAreSorted(append([]string{}, Strategies...)) {
		t.Errorf("Strategies should be sorted: %v", Strategies)
	}
}
