package tui

import "unicode/utf8"

// Trunc shortens s to at most max runes, ending with "..." when cut. It is
// rune-safe (never splits a multi-byte character) and never panics on small
// max values. When a max below the ellipsis width cuts the string, exactly
// max runes are kept without the ellipsis — the kept-prefix length is
// non-monotonic across that boundary by design. Shared by the agents and
// config editors' model lists.
func Trunc(s string, max int) string {
	if max <= 0 {
		return ""
	}
	if utf8.RuneCountInString(s) <= max {
		return s
	}
	if max <= 3 {
		return firstRunes(s, max)
	}
	return firstRunes(s, max-3) + "..."
}

// firstRunes copies at most n leading runes of s without materializing the
// whole string.
func firstRunes(s string, n int) string {
	out := make([]rune, 0, n)
	i := 0
	for _, r := range s {
		if i == n {
			break
		}
		out = append(out, r)
		i++
	}
	return string(out)
}
