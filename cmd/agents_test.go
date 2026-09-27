package cmd

import (
	"testing"

	"github.com/gumieri/nenyactl/internal/detect"
	"github.com/gumieri/nenyactl/internal/nenya"
)

func TestCatalogPairs(t *testing.T) {
	got := catalogPairs([]nenya.ProviderCatalogEntry{
		{Provider: "anthropic", Model: "claude-sonnet-4-5"},
		{Provider: "gemini", Model: "gemini-2.5-flash"},
	})
	want := [][2]string{
		{"anthropic", "claude-sonnet-4-5"},
		{"gemini", "gemini-2.5-flash"},
	}
	if len(got) != len(want) {
		t.Fatalf("catalogPairs len = %d, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("catalogPairs[%d] = %v, want %v", i, got[i], want[i])
		}
	}
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
