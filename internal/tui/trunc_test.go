package tui

import "testing"

func TestTrunc(t *testing.T) {
	cases := []struct {
		name string
		in   string
		max  int
		want string
	}{
		{"short fits", "short", 10, "short"},
		{"exactly max", "exactly-10", 10, "exactly-10"},
		{"cut appends dots", "a-very-long-agent-name", 10, "a-very-..."},
		{"empty", "", 5, ""},
		{"max below ellipsis width", "abcdef", 2, "ab"},
		{"max 3 keeps bare runes", "abcdef", 3, "abc"},
		{"max 4 keeps 1 rune + dots", "abcdef", 4, "a..."},
		{"max zero", "abcdef", 0, ""},
		{"max negative", "abcdef", -1, ""},
		{"multibyte runes are never split", "héllo-wörld", 7, "héll..."},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := Trunc(tc.in, tc.max); got != tc.want {
				t.Errorf("Trunc(%q, %d) = %q, want %q", tc.in, tc.max, got, tc.want)
			}
		})
	}
}
