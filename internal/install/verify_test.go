package install

import (
	"context"
	"crypto/sha256"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestParseChecksums(t *testing.T) {
	data := []byte("ABC123  nenya_0.15.0_linux_arm64.tar.gz\n\nnot-a-line\ndef456  other.tar.gz\n")
	sums := parseChecksums(data)
	if sums["nenya_0.15.0_linux_arm64.tar.gz"] != "abc123" {
		t.Errorf("got %q", sums["nenya_0.15.0_linux_arm64.tar.gz"])
	}
	if sums["other.tar.gz"] != "def456" {
		t.Errorf("got %q", sums["other.tar.gz"])
	}
	if len(sums) != 2 {
		t.Errorf("expected 2 entries, got %d", len(sums))
	}
}

func TestChecksumFor(t *testing.T) {
	sums := map[string]string{"a.tar.gz": "aa"}
	if _, err := checksumFor(sums, "missing.tar.gz"); err == nil {
		t.Fatal("expected error for missing entry")
	}
	if got, err := checksumFor(sums, "a.tar.gz"); err != nil || got != "aa" {
		t.Fatalf("got %q, %v", got, err)
	}
}

func TestVerifyFileChecksum(t *testing.T) {
	p := filepath.Join(t.TempDir(), "f")
	if err := os.WriteFile(p, []byte("payload"), 0o644); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256([]byte("payload"))
	if err := verifyFileChecksum(p, fmt.Sprintf("%x", sum)); err != nil {
		t.Fatalf("valid checksum rejected: %v", err)
	}
	if err := verifyFileChecksum(p, strings.Repeat("0", 64)); err == nil {
		t.Fatal("expected mismatch error")
	}
}

// cosignRunner records commands and succeeds for cosign.
type cosignRunner struct{ calls *[]string }

func (r cosignRunner) Output(_ context.Context, name string, args ...string) ([]byte, error) {
	*r.calls = append(*r.calls, strings.Join(append([]string{name}, args...), " "))
	return []byte("ok"), nil
}

func TestVerifySigstoreBundle(t *testing.T) {
	tmp := t.TempDir()
	artifact := filepath.Join(tmp, "checksums.txt")
	bundle := filepath.Join(tmp, "checksums.txt.sigstore.json")
	_ = os.WriteFile(artifact, []byte("x"), 0o644)
	_ = os.WriteFile(bundle, []byte("{}"), 0o644)

	t.Run("succeeds and passes identity and issuer", func(t *testing.T) {
		var calls []string
		if err := verifySigstoreBundle(context.Background(), cosignRunner{&calls}, artifact, bundle, "id-re", "issuer"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		joined := strings.Join(calls, "\n")
		if !strings.Contains(joined, "cosign version") {
			t.Errorf("expected cosign version probe, got: %s", joined)
		}
		if !strings.Contains(joined, "--certificate-identity-regexp id-re") {
			t.Errorf("expected identity flag, got: %s", joined)
		}
		if !strings.Contains(joined, "--certificate-oidc-issuer issuer") {
			t.Errorf("expected issuer flag, got: %s", joined)
		}
	})

	t.Run("fails closed when cosign is missing", func(t *testing.T) {
		err := verifySigstoreBundle(context.Background(), fakeRunner{}, artifact, bundle, "", "")
		if err == nil {
			t.Fatal("expected error when cosign is unavailable")
		}
		if !strings.Contains(err.Error(), "cosign is required") {
			t.Errorf("unexpected error: %v", err)
		}
	})

	t.Run("propagates verification failure", func(t *testing.T) {
		var calls []string
		err := verifySigstoreBundle(context.Background(), fakeRunner{available: map[string]bool{"cosign version": true}}, artifact, bundle, "", "")
		if err == nil {
			t.Fatal("expected verify-blob failure")
		}
		_ = calls
	})
}

func TestInstallVerification(t *testing.T) {
	archive := containTarGz(t, map[string]string{"nenya": "fake-binary-content"})
	name := archiveFilename("v0.1.0", runtime.GOOS, runtime.GOARCH)
	sum := sha256.Sum256(archive)
	good := fmt.Sprintf("%x  %s\n", sum, name)

	tests := []struct {
		name        string
		checksums   string
		serveBundle bool
		runner      CommandRunner
		skipVerify  bool
		wantErr     string
	}{
		{
			name:        "valid checksum and cosign installs",
			checksums:   good,
			serveBundle: true,
			runner:      cosignRunner{new([]string)},
		},
		{
			name:        "tampered checksum aborts",
			checksums:   strings.Repeat("0", 64) + "  " + name + "\n",
			serveBundle: true,
			runner:      cosignRunner{new([]string)},
			wantErr:     "checksum mismatch",
		},
		{
			name:        "missing checksum entry aborts",
			checksums:   "abc  something-else.tar.gz\n",
			serveBundle: true,
			runner:      cosignRunner{new([]string)},
			wantErr:     "no entry",
		},
		{
			name:        "missing bundle aborts when verifying",
			checksums:   good,
			serveBundle: false,
			runner:      cosignRunner{new([]string)},
			wantErr:     "checksums.txt.sigstore.json",
		},
		{
			name:        "missing cosign aborts",
			checksums:   good,
			serveBundle: true,
			runner:      fakeRunner{},
			wantErr:     "cosign is required",
		},
		{
			name:        "skip-verify bypasses bundle but still checks sha",
			checksums:   good,
			serveBundle: false,
			runner:      fakeRunner{},
			skipVerify:  true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch {
				case strings.HasSuffix(r.URL.Path, "checksums.txt.sigstore.json"):
					if !tc.serveBundle {
						w.WriteHeader(http.StatusNotFound)
						return
					}
					_, _ = w.Write([]byte("{}"))
				case strings.HasSuffix(r.URL.Path, "checksums.txt"):
					_, _ = w.Write([]byte(tc.checksums))
				case strings.HasSuffix(r.URL.Path, ".tar.gz"):
					_, _ = w.Write(archive)
				default:
					w.WriteHeader(http.StatusNotFound)
				}
			}))
			defer server.Close()
			pointAtServer(t, server)

			tmp := t.TempDir()
			t.Setenv("HOME", tmp)
			cfg := Config{
				UserInstall: true,
				Version:     "v0.1.0",
				SkipService: true,
				SkipVerify:  tc.skipVerify,
			}
			err := InstallWithHTTPAndRunner(context.Background(), cfg, server.Client(), tc.runner)
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if _, statErr := os.Stat(filepath.Join(tmp, ".local", "bin", "nenya")); statErr != nil {
					t.Fatalf("binary not installed: %v", statErr)
				}
				return
			}
			if err == nil {
				t.Fatalf("expected error containing %q", tc.wantErr)
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("error %q does not contain %q", err.Error(), tc.wantErr)
			}
			if _, statErr := os.Stat(filepath.Join(tmp, ".local", "bin", "nenya")); statErr == nil {
				t.Fatal("binary must not be installed when verification fails")
			}
		})
	}
}
