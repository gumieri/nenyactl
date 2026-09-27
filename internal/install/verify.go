package install

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
)

// CommandRunner executes an external command and returns its standard output.
// It is injectable so signature verification can be tested without cosign or
// the nenya binary installed.
type CommandRunner interface {
	Output(ctx context.Context, name string, args ...string) ([]byte, error)
}

// execRunner is the production CommandRunner backed by os/exec.
type execRunner struct{}

func (execRunner) Output(ctx context.Context, name string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg != "" {
			return out, fmt.Errorf("%w: %s", err, msg)
		}
		return out, err
	}
	return out, nil
}

// defaultRunner is used when no runner is injected.
var defaultRunner CommandRunner = execRunner{}

// Cosign keyless identity defaults for nenya's release signing. The signature
// bundle is produced by cosign `sign-blob` in nenya's release workflow, so the
// certificate identity is that workflow at a version tag.
const (
	// DefaultCosignIdentityRegexp matches nenya's release workflow identity.
	DefaultCosignIdentityRegexp = `^https://github\.com/gumieri/nenya/\.github/workflows/release\.yml@refs/tags/v.*$`
	// DefaultCosignIssuer is the GitHub Actions OIDC issuer.
	DefaultCosignIssuer = "https://token.actions.githubusercontent.com"
)

// parseChecksums parses a goreleaser checksums.txt. Each non-empty line is
// "<sha256>  <filename>"; lines that do not match are ignored.
func parseChecksums(data []byte) map[string]string {
	sums := make(map[string]string)
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) != 2 {
			continue
		}
		sums[fields[1]] = strings.ToLower(fields[0])
	}
	return sums
}

// checksumFor returns the expected lowercase hex SHA-256 for name.
func checksumFor(sums map[string]string, name string) (string, error) {
	want, ok := sums[name]
	if !ok || want == "" {
		return "", fmt.Errorf("checksums.txt has no entry for %s", name)
	}
	return want, nil
}

// verifyFileChecksum checks the SHA-256 of path against want (hex).
func verifyFileChecksum(path, want string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return err
	}
	got := hex.EncodeToString(h.Sum(nil))
	if !strings.EqualFold(got, want) {
		return fmt.Errorf("checksum mismatch for %s:\n  expected: %s\n  actual:   %s", path, want, got)
	}
	return nil
}

// verifySigstoreBundle verifies artifactPath against bundlePath using cosign
// keyless verification. A missing cosign binary is a hard error: installing an
// unverified binary is a contract violation (CONTRACT.md §7.2).
func verifySigstoreBundle(ctx context.Context, runner CommandRunner, artifactPath, bundlePath, identityRegexp, issuer string) error {
	if identityRegexp == "" {
		identityRegexp = DefaultCosignIdentityRegexp
	}
	if issuer == "" {
		issuer = DefaultCosignIssuer
	}

	if _, err := runner.Output(ctx, "cosign", "version"); err != nil {
		return fmt.Errorf("cosign is required to verify release signatures but is not available: %w", err)
	}

	if _, err := runner.Output(ctx, "cosign",
		"verify-blob",
		"--bundle", bundlePath,
		"--certificate-identity-regexp", identityRegexp,
		"--certificate-oidc-issuer", issuer,
		artifactPath,
	); err != nil {
		return fmt.Errorf("cosign verification failed: %w", err)
	}
	return nil
}
