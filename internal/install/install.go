package install

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/gumieri/nenyactl/internal/paths"
)

// HTTPDoer is the interface for making HTTP requests.
type HTTPDoer interface {
	Do(*http.Request) (*http.Response, error)
}

const (
	owner = "gumieri"
	repo  = "nenya"
	// binDir matches the shipped unit files' ExecStart (/usr/bin/nenya) and
	// nenya's own installer. Installing elsewhere would enable a unit whose
	// ExecStart does not exist.
	binDir = "/usr/bin"
	// defaultUnitConfigDir is the config root hardcoded by the shipped units
	// (systemd LoadCredential and the launchd NENYA_CONFIG_DIR). Regenerate
	// with `nenya service-unit` (contract target) for a different root.
	defaultUnitConfigDir = "/etc/nenya"
)

// Config controls an installation.
type Config struct {
	UserInstall bool
	Version     string
	SkipService bool
	// SkipVerify disables SHA-256 + cosign verification. It exists only for
	// offline/air-gapped workflows that verify out of band; installing an
	// unverified binary is a contract violation (CONTRACT.md §7.2).
	SkipVerify bool
	// CosignIdentityRegexp overrides the expected cosign keyless identity.
	CosignIdentityRegexp string
	// CosignIssuer overrides the expected cosign OIDC issuer.
	CosignIssuer string

	// Test-only overrides (unexported; settable from same-package tests).
	binDirOverride    string
	configDirOverride string
	unitDirOverride   string
}

// Platform hooks, overridable in tests.
var (
	systemConfigDir = func() string { return paths.SystemConfigDir() }
	userConfigDir   = func() (string, error) { return paths.UserConfigDir() }
	systemUnitDir   = func() string {
		if runtime.GOOS == "darwin" {
			return "/Library/LaunchDaemons"
		}
		return "/etc/systemd/system"
	}
	systemctlBin = "systemctl"
	launchctlBin = "launchctl"
)

// Install downloads and installs nenya using http.DefaultClient.
func Install(ctx context.Context, cfg Config) error {
	return InstallWithHTTP(ctx, cfg, http.DefaultClient)
}

// InstallWithHTTP installs nenya using the supplied HTTP client.
func InstallWithHTTP(ctx context.Context, cfg Config, hc HTTPDoer) error {
	return InstallWithHTTPAndRunner(ctx, cfg, hc, defaultRunner)
}

// InstallWithHTTPAndRunner installs nenya, injecting the HTTP client and the
// command runner used for signature verification and service management.
func InstallWithHTTPAndRunner(ctx context.Context, cfg Config, hc HTTPDoer, runner CommandRunner) error {
	if runtime.GOOS == "windows" {
		return fmt.Errorf("bare-metal installation is not supported on Windows; use 'nenyactl containers setup' instead")
	}

	tag := normalizeTag(cfg.Version)
	if tag == "" {
		var err error
		tag, err = FetchLatestVersionWithHTTP(ctx, hc)
		if err != nil {
			return fmt.Errorf("fetch latest version: %w", err)
		}
		tag = normalizeTag(tag)
	}

	archiveName := archiveFilename(tag, runtime.GOOS, runtime.GOARCH)

	tmpDir, err := os.MkdirTemp("", "nenyactl-*")
	if err != nil {
		return fmt.Errorf("create temp dir: %w", err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	archivePath := filepath.Join(tmpDir, archiveName)
	if err := downloadWith(ctx, downloadURL(tag), archivePath, hc); err != nil {
		return fmt.Errorf("download nenya: %w", err)
	}

	if err := verifyDownload(ctx, cfg, hc, runner, tag, archiveName, archivePath, tmpDir); err != nil {
		return err
	}

	extractDir := filepath.Join(tmpDir, "extract")
	if err := os.MkdirAll(extractDir, 0o755); err != nil {
		return fmt.Errorf("create extract dir: %w", err)
	}

	if err := untar(archivePath, extractDir); err != nil {
		return fmt.Errorf("extract archive: %w", err)
	}

	// Consumers MUST extract the `nenya` member by exact name (CONTRACT.md §7.1).
	binaryPath := filepath.Join(extractDir, "nenya")
	if _, err := os.Stat(binaryPath); os.IsNotExist(err) {
		return fmt.Errorf("nenya binary not found in archive at expected path %s", binaryPath)
	}

	if err := os.Chmod(binaryPath, 0o755); err != nil {
		return fmt.Errorf("set executable: %w", err)
	}

	dest := filepath.Join(binDir, "nenya")
	if cfg.binDirOverride != "" {
		dest = filepath.Join(cfg.binDirOverride, "nenya")
	} else if cfg.UserInstall {
		home, err := os.UserHomeDir()
		if err != nil {
			return fmt.Errorf("get home dir: %w", err)
		}
		dest = filepath.Join(home, ".local", "bin", "nenya")
	}

	parent := filepath.Dir(dest)
	if err := os.MkdirAll(parent, 0o755); err != nil {
		return fmt.Errorf("create bin dir: %w", err)
	}

	if err := copyFile(binaryPath, dest, 0o755); err != nil {
		return fmt.Errorf("install binary: %w", err)
	}

	fmt.Printf("Installed nenya %s to %s\n", tag, dest)

	if cfg.UserInstall && !cfg.SkipService {
		fmt.Fprintln(os.Stderr, "Note: --user installs the binary and user config only; no system service is written.")
	}

	// Full installs bootstrap config + secrets so the service can start, then
	// load/enable the unit. --skip-service keeps this binary-only; --user skips
	// the service but still bootstraps user config.
	doService := !cfg.SkipService && !cfg.UserInstall
	doBootstrap := !cfg.SkipService
	if doBootstrap {
		p, pathErr := resolveInstallPaths(ctx, cfg, runner, dest)
		if pathErr != nil {
			return pathErr
		}

		if doService && p.configDir != defaultUnitConfigDir {
			fmt.Fprintf(os.Stderr, "Warning: config root is %s but the shipped unit expects %s.\n", p.configDir, defaultUnitConfigDir)
			fmt.Fprintln(os.Stderr, "Regenerate the unit with 'nenya service-unit --config-dir "+p.configDir+"' once that command ships.")
		}

		serviceReady := true
		if doService {
			if err := installServiceFilesTo(extractDir, p.unitDir); err != nil {
				serviceReady = false
				fmt.Fprintf(os.Stderr, "Warning: failed to install service files: %v\n", err)
				fmt.Fprintln(os.Stderr, "You can run nenya directly from the command line.")
			}
		}

		bootstrapReady := true
		if _, err := bootstrapConfig(ctx, runner, dest, p); err != nil {
			bootstrapReady = false
			fmt.Fprintf(os.Stderr, "Warning: could not create config: %v\n", err)
		}
		if _, err := bootstrapSecrets(p); err != nil {
			bootstrapReady = false
			fmt.Fprintf(os.Stderr, "Warning: could not create secrets: %v\n", err)
		}

		if cfg.UserInstall && bootstrapReady {
			fmt.Fprintf(os.Stderr, "User install is not a service. Run it with:\n  NENYA_CONFIG_DIR=%s NENYA_SECRETS_DIR=%s %s\n", p.configDir, p.configDir, dest)
		}

		switch {
		case doService && serviceReady && bootstrapReady:
			enableService(ctx, runner, p.unitDir)
		case doService && !bootstrapReady:
			fmt.Fprintln(os.Stderr, "Warning: service not enabled because config/secrets bootstrap failed.")
		}
	}

	return nil
}

// verifyDownload verifies the release before extraction or installation. The
// checksums file is first authenticated with cosign (CONTRACT.md §7.2), then
// the archive SHA-256 is compared against it. Any mismatch aborts.
func verifyDownload(ctx context.Context, cfg Config, hc HTTPDoer, runner CommandRunner, tag, archiveName, archivePath, workDir string) error {
	checksumsPath := filepath.Join(workDir, "checksums.txt")
	if err := downloadWith(ctx, checksumsURL(tag), checksumsPath, hc); err != nil {
		return fmt.Errorf("download checksums.txt: %w", err)
	}

	if cfg.SkipVerify {
		fmt.Fprintln(os.Stderr, "Warning: cosign signature verification skipped (--skip-verify); SHA-256 is still enforced")
	} else {
		bundlePath := filepath.Join(workDir, "checksums.txt.sigstore.json")
		if err := downloadWith(ctx, sigstoreBundleURL(tag), bundlePath, hc); err != nil {
			return fmt.Errorf("download checksums.txt.sigstore.json: %w", err)
		}
		if err := verifySigstoreBundle(ctx, runner, checksumsPath, bundlePath, cfg.CosignIdentityRegexp, cfg.CosignIssuer); err != nil {
			return err
		}
	}

	data, err := os.ReadFile(checksumsPath)
	if err != nil {
		return fmt.Errorf("read checksums.txt: %w", err)
	}
	want, err := checksumFor(parseChecksums(data), archiveName)
	if err != nil {
		return err
	}
	if err := verifyFileChecksum(archivePath, want); err != nil {
		return fmt.Errorf("verify %s: %w", archiveName, err)
	}
	return nil
}

// normalizeTag ensures a release tag keeps its conventional leading `v`, since
// download URLs are /releases/download/vX.Y.Z/ while artifact names omit it.
func normalizeTag(v string) string {
	v = strings.TrimSpace(v)
	if v == "" || strings.HasPrefix(v, "v") {
		return v
	}
	return "v" + v
}

func installServiceFiles(extractDir string) error {
	return installServiceFilesTo(extractDir, systemUnitDir())
}

// extractFile is an ordered archive member -> destination copy.
type extractFile struct {
	src string
	dst string
}

// installServiceFilesTo copies the shipped unit files from the archive into
// unitDir. Unit contents are NOT generated here: the shipped units are the
// contract source until `nenya service-unit` ships (CONTRACT.md §7.1/§4.5).
func installServiceFilesTo(extractDir, unitDir string) error {
	switch runtime.GOOS {
	case "linux":
		return copyFromExtract(extractDir, []extractFile{
			{"deploy/nenya.service", filepath.Join(unitDir, "nenya.service")},
			{"deploy/nenya.socket", filepath.Join(unitDir, "nenya.socket")},
		})
	case "darwin":
		return copyFromExtract(extractDir, []extractFile{
			{"deploy/nenya.plist", filepath.Join(unitDir, "com.gumieri.nenya.plist")},
		})
	}
	return nil
}

func copyFromExtract(extractDir string, files []extractFile) error {
	for _, f := range files {
		srcPath := filepath.Join(extractDir, f.src)
		if _, err := os.Stat(srcPath); os.IsNotExist(err) {
			return fmt.Errorf("file not found in archive: %s", f.src)
		}
		dstDir := filepath.Dir(f.dst)
		if err := os.MkdirAll(dstDir, 0o755); err != nil {
			return fmt.Errorf("create directory %s: %w", dstDir, err)
		}
		if err := copyFile(srcPath, f.dst, 0o644); err != nil {
			return fmt.Errorf("copy %s to %s: %w", f.src, f.dst, err)
		}
		fmt.Printf("Installed %s\n", f.dst)
	}
	return nil
}

// archiveFilename builds the release archive name for a tag. The tag keeps its
// leading `v`; the artifact name does not (e.g. tag v0.15.0 ->
// nenya_0.15.0_linux_amd64.tar.gz).
func archiveFilename(tag, osName, arch string) string {
	return fmt.Sprintf("nenya_%s_%s_%s.tar.gz", strings.TrimPrefix(tag, "v"), osName, arch)
}

// assetBaseURL is the release download prefix for a tag.
var assetBaseURL = func(tag string) string {
	return fmt.Sprintf("https://github.com/%s/%s/releases/download/%s/", owner, repo, tag)
}

var (
	downloadURL = func(tag string) string {
		return assetBaseURL(tag) + archiveFilename(tag, runtime.GOOS, runtime.GOARCH)
	}
	checksumsURL = func(tag string) string {
		return assetBaseURL(tag) + "checksums.txt"
	}
	sigstoreBundleURL = func(tag string) string {
		return assetBaseURL(tag) + "checksums.txt.sigstore.json"
	}
)

func download(ctx context.Context, url, dest string) error {
	return downloadWith(ctx, url, dest, http.DefaultClient)
}

func downloadWith(ctx context.Context, url, dest string, hc HTTPDoer) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	resp, err := hc.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status %s for %s", resp.Status, url)
	}

	out, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer func() { _ = out.Close() }()

	_, err = io.Copy(out, resp.Body)
	return err
}

func copyFile(src, dst string, mode os.FileMode) error {
	s, err := os.Open(src)
	if err != nil {
		return err
	}
	defer func() { _ = s.Close() }()

	d, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer func() { _ = d.Close() }()

	if _, err := io.Copy(d, s); err != nil {
		return err
	}

	return os.Chmod(dst, mode)
}
