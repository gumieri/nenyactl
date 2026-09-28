package install

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/gumieri/nenyactl/internal/contract"
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
	// (systemd LoadCredential and the launchd NENYA_CONFIG_DIR). nenyactl
	// regenerates the unit with `nenya service-unit` (CONTRACT.md §4.5) for any
	// other root.
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

// SystemUnitDir returns the system directory the shipped service unit is
// installed to for the current platform.
func SystemUnitDir() string { return systemUnitDir() }

// Install downloads and installs nenya using http.DefaultClient.
func Install(ctx context.Context, cfg Config) error {
	return InstallWithHTTP(ctx, cfg, http.DefaultClient)
}

// InstallWithHTTP installs nenya using the supplied HTTP client.
func InstallWithHTTP(ctx context.Context, cfg Config, hc HTTPDoer) error {
	return InstallWithHTTPAndRunner(ctx, cfg, hc, defaultRunner)
}

// ResolveConfigRoot returns the config root an install with cfg would use,
// without performing the install. Callers (e.g. the --connect summary) use it
// so their output matches the paths install actually wrote.
func ResolveConfigRoot(ctx context.Context, cfg Config, runner CommandRunner, execPath string) (string, error) {
	p, err := resolveInstallPaths(ctx, cfg, runner, execPath)
	if err != nil {
		return "", err
	}
	return p.configDir, nil
}

// InstallWithHTTPAndRunner installs nenya, injecting the HTTP client and the
// command runner used for signature verification and service management.
func InstallWithHTTPAndRunner(ctx context.Context, cfg Config, hc HTTPDoer, runner CommandRunner) error {
	if runtime.GOOS == "windows" {
		return fmt.Errorf("bare-metal installation is not supported on Windows; use 'nenyactl containers setup' instead")
	}

	tag := normalizeTag(cfg.Version)
	if tag == "" && cfg.Version != "" {
		return fmt.Errorf("invalid version %q: expected a release tag like v0.15.0", cfg.Version)
	}
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

	if err := checkInstalledContract(ctx, runner, dest); err != nil {
		return err
	}

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

		serviceReady := true
		if doService {
			generated, err := installServiceUnitsAt(ctx, runner, dest, extractDir, p.unitDir, p.configDir, p.secretsFile)
			if err != nil {
				serviceReady = false
				fmt.Fprintf(os.Stderr, "Warning: failed to install service files: %v\n", err)
				fmt.Fprintln(os.Stderr, "You can run nenya directly from the command line.")
			} else {
				warnIfNonDefaultRoot(p, generated)
			}
		}

		bootstrapReady := true
		if _, err := bootstrapConfig(ctx, runner, dest, p); err != nil {
			bootstrapReady = false
			fmt.Fprintf(os.Stderr, "Warning: could not create config: %v\n", err)
		}
		if _, err := bootstrapSecrets(ctx, runner, dest, p); err != nil {
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
	return NormalizeTag(v)
}

// checkInstalledContract fails fast when the installed binary reports a
// nenya contract_version outside the range this build supports (CONTRACT.md §2).
// It feature-detects `describe --json`, then `version --json`; a binary that
// exposes neither (older releases) is accepted. It deliberately does not use
// internal/nenya's Client: install probes a freshly extracted binary by explicit
// path through CommandRunner, and only ever needs one field, so a partial
// decode cannot reject a binary just because another field drifted.
func checkInstalledContract(ctx context.Context, runner CommandRunner, execPath string) error {
	type versioned struct {
		ContractVersion int `json:"contract_version"`
	}

	probe := func(args ...string) (versioned, bool) {
		out, err := probeOutput(ctx, runner, execPath, args...)
		if err != nil || len(out) == 0 {
			return versioned{}, false
		}
		var v versioned
		if err := json.Unmarshal(out, &v); err != nil {
			return versioned{}, false
		}
		return v, true
	}

	v, ok := probe("describe", "--json")
	if !ok {
		v, ok = probe("version", "--json")
	}
	if !ok || v.ContractVersion == 0 {
		return nil
	}
	if err := contract.Check(v.ContractVersion); err != nil {
		return fmt.Errorf("installed nenya %s: %w", execPath, err)
	}
	return nil
}

// extractFile is an ordered archive member -> destination copy.
type extractFile struct {
	src string
	dst string
}

// serviceUnitSpec is one unit to install: the archive member that carries it
// (the fallback) and its destination name in the unit directory. generated is
// true when `nenya service-unit` can produce this unit; the contract emits the
// service unit only (the socket is a release-archive member, per CONTRACT.md
// §4.5).
type serviceUnitSpec struct {
	member      string // release-archive member
	destination string // filename under the unit directory
	generated   bool   // whether nenya service-unit emits this unit
	init        string // nenya service-unit --init value
}

// unitSpecs returns the units to install for the current platform.
func unitSpecs() []serviceUnitSpec { return unitSpecsFor(runtime.GOOS) }

// unitSpecsFor returns the units to install for a platform. It is
// parameterized so both platform tables are testable on any host.
func unitSpecsFor(goos string) []serviceUnitSpec {
	if goos == "darwin" {
		return []serviceUnitSpec{
			{member: "deploy/nenya.plist", destination: "com.gumieri.nenya.plist", generated: true, init: "launchd"},
		}
	}
	return []serviceUnitSpec{
		{member: "deploy/nenya.service", destination: "nenya.service", generated: true, init: "systemd"},
		// nenya service-unit emits the service unit only, so the socket is
		// always taken from the release archive.
		{member: "deploy/nenya.socket", destination: "nenya.socket"},
	}
}

// installServiceUnitsTo installs the platform's units and discards the
// generated flag. It is the test-facing wrapper around installServiceUnitsAt.
func installServiceUnitsTo(ctx context.Context, runner CommandRunner, execPath, extractDir, unitDir, configDir, secretsFile string) error {
	_, err := installServiceUnitsAt(ctx, runner, execPath, extractDir, unitDir, configDir, secretsFile)
	return err
}

// installServiceUnitsAt installs the units and reports whether the service unit
// was generated by nenya (as opposed to copied from the archive), so the caller
// can emit an honest note without re-reading the unit file.
func installServiceUnitsAt(ctx context.Context, runner CommandRunner, execPath, extractDir, unitDir, configDir, secretsFile string) (generated bool, err error) {
	for _, spec := range unitSpecs() {
		if spec.generated {
			if content, ok := serviceUnitContent(ctx, runner, execPath, spec.init, configDir, secretsFile); ok {
				if err := writeUnitFile(filepath.Join(unitDir, spec.destination), content); err != nil {
					return false, err
				}
				generated = true
				continue
			}
		}
		// Fall back to the archive member. It pins the default config root, so
		// callers warn when that differs from the install's root.
		if err := copyFromExtract(extractDir, []extractFile{{spec.member, filepath.Join(unitDir, spec.destination)}}); err != nil {
			return false, err
		}
	}
	return generated, nil
}

// warnIfNonDefaultRoot notes when the installed unit references the install's
// config root. When service-unit generated it, the unit does; when the archive
// fallback was used instead, the unit pins the default root and the user is
// warned with the regeneration command.
func warnIfNonDefaultRoot(p installPaths, generated bool) {
	if p.configDir == defaultUnitConfigDir {
		return
	}
	if generated {
		fmt.Fprintf(os.Stderr, "Note: generated the service unit for config root %s (nenya service-unit).\n", p.configDir)
		return
	}
	fmt.Fprintf(os.Stderr, "Warning: config root is %s but the installed unit references %s.\n", p.configDir, defaultUnitConfigDir)
	fmt.Fprintf(os.Stderr, "Regenerate it with 'nenya service-unit --config-dir %s --secrets-file %s'.\n", p.configDir, p.secretsFile)
}

// serviceUnitContent asks nenya to generate a unit for the given init system and
// paths. ok is false when `nenya service-unit` is unavailable, produced no
// output, or produced output that does not look like a unit, so callers can fall
// back rather than write a usage/log line as a unit file.
func serviceUnitContent(ctx context.Context, runner CommandRunner, execPath, init, configDir, secretsFile string) ([]byte, bool) {
	args := []string{"service-unit", "--init", init}
	if execPath != "" {
		args = append(args, "--exec-path", execPath)
	}
	if configDir != "" {
		args = append(args, "--config-dir", configDir)
	}
	if secretsFile != "" {
		args = append(args, "--secrets-file", secretsFile)
	}
	out, err := probeOutput(ctx, runner, execPath, args...)
	if err != nil || !looksLikeUnit(init, out) {
		return nil, false
	}
	return out, true
}

// looksLikeUnit reports whether out resembles a service unit for init. It guards
// against a binary that ignores the subcommand and prints something else.
func looksLikeUnit(init string, out []byte) bool {
	s := string(out)
	switch init {
	case "launchd":
		return strings.Contains(s, "<plist") || strings.Contains(s, "ProgramArguments")
	default:
		return strings.Contains(s, "[Unit]") || strings.Contains(s, "[Service]")
	}
}

// writeUnitFile writes a generated unit to unitDir with mode 0644.
func writeUnitFile(path string, content []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create directory %s: %w", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, content, 0o644); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	fmt.Printf("Installed %s\n", path)
	return nil
}

// installServiceFilesTo copies the shipped unit files from the archive into
// unitDir. It is the documented fallback used when `nenya service-unit` is not
// available; prefer installServiceUnitsAt, which reports generation.
func installServiceFilesTo(extractDir, unitDir string) error {
	var files []extractFile
	for _, spec := range unitSpecs() {
		files = append(files, extractFile{spec.member, filepath.Join(unitDir, spec.destination)})
	}
	return copyFromExtract(extractDir, files)
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

// NewExecRunner returns the production CommandRunner backed by os/exec.
func NewExecRunner() CommandRunner { return execRunner{} }

// NormalizeTag ensures a release tag keeps its conventional leading `v`, since
// download URLs are /releases/download/vX.Y.Z/ while artifact names omit it.
// It returns "" when the tag contains characters that could escape a path.
func NormalizeTag(v string) string {
	v = strings.TrimSpace(v)
	if v == "" {
		return ""
	}
	for _, r := range v {
		if !isTagRune(r) {
			return ""
		}
	}
	if strings.HasPrefix(v, "v") {
		return v
	}
	return "v" + v
}

// isTagRune reports whether r may appear in a release tag.
func isTagRune(r rune) bool {
	switch {
	case r >= '0' && r <= '9':
		return true
	case r >= 'a' && r <= 'z':
		return true
	case r >= 'A' && r <= 'Z':
		return true
	case r == '.' || r == '-' || r == '_' || r == 'v':
		return true
	default:
		return false
	}
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
