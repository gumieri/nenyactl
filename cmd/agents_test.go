package cmd

import (
	"testing"

	"github.com/gumieri/nenyactl/internal/agents"
	"github.com/gumieri/nenyactl/internal/detect"
	"github.com/gumieri/nenyactl/internal/nenya"
)

func TestCatalogModels(t *testing.T) {
	got := catalogModels([]nenya.ProviderCatalogEntry{
		{Provider: "anthropic", Model: "claude-sonnet-4-5"},
		{Provider: "gemini", Model: "gemini-2.5-flash"},
	})
	want := []agents.CatalogModel{
		{Provider: "anthropic", Model: "claude-sonnet-4-5"},
		{Provider: "gemini", Model: "gemini-2.5-flash"},
	}
	if len(got) != len(want) {
		t.Fatalf("catalogModels len = %d, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("catalogModels[%d] = %v, want %v", i, got[i], want[i])
		}
	}
}

func TestProviderDefs(t *testing.T) {
	t.Run("uses the catalog providers", func(t *testing.T) {
		defs := providerDefs([]nenya.ProviderCatalogEntry{
			{Provider: "anthropic", Model: "claude-sonnet-4-5"},
			{Provider: "anthropic", Model: "claude-opus-4-7"},
			{Provider: "gemini", Model: "gemini-2.5-flash"},
		}, nil)
		if len(defs) != 2 {
			t.Fatalf("defs = %d, want 2: %+v", len(defs), defs)
		}
		for _, d := range defs {
			if !d.NeedsKey {
				t.Errorf("provider %s should need a key", d.Name)
			}
		}
	})

	t.Run("adds configured providers missing from the catalog", func(t *testing.T) {
		defs := providerDefs([]nenya.ProviderCatalogEntry{{Provider: "anthropic", Model: "m"}}, []string{"openai"})
		found := false
		for _, d := range defs {
			if d.Name == "openai" {
				found = true
			}
		}
		if !found {
			t.Errorf("configured provider openai missing: %+v", defs)
		}
	})

	t.Run("falls back to the builtin shim when empty", func(t *testing.T) {
		defs := providerDefs(nil, nil)
		if len(defs) == 0 {
			t.Fatal("expected the builtin provider shim")
		}
	})
}

func TestParseAgentsMode(t *testing.T) {
	cases := []struct {
		in   string
		want detect.Mode
		ok   bool
	}{
		{"bare-metal", detect.ModeBareMetal, true},
		{"container", detect.ModeContainer, true},
		{"bogus", detect.ModeNone, false},
	}
	for _, c := range cases {
		got, err := parseAgentsMode(c.in)
		if c.ok {
			if err != nil || got != c.want {
				t.Errorf("parseAgentsMode(%q) = %v, %v; want %v", c.in, got, err, c.want)
			}
			continue
		}
		if err == nil {
			t.Errorf("parseAgentsMode(%q) expected error", c.in)
		}
	}
}
