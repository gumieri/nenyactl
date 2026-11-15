// Package paths resolves nenyactl's filesystem locations: per-user and
// system config/data/bin directories across Linux, macOS, and Windows, plus
// the container deployment root. Platform dispatch is parameterized into
// testable For-variants; the exported shapes wire runtime.GOOS and the real
// environment through (NCTL-50).
package paths
