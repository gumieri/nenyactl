package cmd

import (
	"testing"

	"github.com/gumieri/nenyactl/internal/detect"
)

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
