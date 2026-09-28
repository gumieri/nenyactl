package install

// This package deliberately embeds no copy of Nenya's example config:
// CONTRACT.md §4.4 reserves `nenya example-config` for that, and the boundary
// rule forbids re-encoding it. Two documented shims remain for released
// binaries without the writer commands: BootstrapConfigContent (example-config)
// and the hand-written secrets.json in bootstrapSecrets (secret set). Both are
// feature-detected fallbacks, retired once those binaries are gone.
// internal/containers/setup.go keeps the config shim too (it cannot assume a
// nenya binary at all).

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gumieri/nenyactl/internal/nenya"
	"github.com/gumieri/nenyactl/internal/secrets"
)

// minimalConfig is a documented bootstrap shim used only until nenya ships the
// `example-config` contract command (CONTRACT.md §4.4). It is intentionally
// tiny and is NOT a copy of nenya's example config: when `nenya example-config`
// exists, that output is written instead. Delete this once the command ships.
const minimalConfig = `{
  "server": {
    "listen_addr": ":8080"
  }
}
`

// installPaths is the resolved set of writable locations for an install.
type installPaths struct {
	configDir   string
	configFile  string
	secretsFile string
	// secretsDir is nenya's wildcard secrets merge directory (CONTRACT §6.1),
	// used only to detect an existing token.
	secretsDir string
	unitDir    string
}

// resolveInstallPaths determines where config, secrets, and units live. It
// prefers `nenya paths --json` (CONTRACT.md §4.2, stable) for the config
// root/file and falls back to platform defaults when the command is absent
// (released binaries without it).
func resolveInstallPaths(ctx context.Context, cfg Config, runner CommandRunner, execPath string) (installPaths, error) {
	if cfg.UserInstall {
		dir := cfg.configDirOverride
		if dir == "" {
			d, err := userConfigDir()
			if err != nil {
				return installPaths{}, fmt.Errorf("resolve user config dir: %w", err)
			}
			dir = d
		}
		return installPaths{
			configDir:   dir,
			configFile:  filepath.Join(dir, "config.json"),
			secretsFile: filepath.Join(dir, "secrets.json"),
		}, nil
	}

	dir := cfg.configDirOverride
	configFile := ""
	secretsDir := ""
	if dir == "" {
		dir = systemConfigDir()
		// nenya paths --json is the authority for the config root. Its
		// secrets_dir is a wildcard *merge directory* (CONTRACT §6.1), not the
		// single LoadCredential file the shipped unit expects, so it is used
		// only to locate an existing token, never to place a fresh one. A
		// fresh install writes <config-root>/secrets.json, which is what the
		// unit wires via `LoadCredential=secrets:/etc/nenya/secrets.json`.
		// Regenerating the unit for another secrets path needs
		// `nenya service-unit --secrets-file`.
		if p, ok := queryNenyaPaths(ctx, runner, execPath); ok {
			if p.ConfigDir != "" {
				dir = p.ConfigDir
			}
			if p.ConfigFile != "" {
				configFile = p.ConfigFile
			}
			if p.SecretsDir != "" {
				secretsDir = p.SecretsDir
			}
		}
	}
	if configFile == "" {
		configFile = filepath.Join(dir, "config.json")
	}

	unitDir := cfg.unitDirOverride
	if unitDir == "" {
		unitDir = systemUnitDir()
	}

	return installPaths{
		configDir:   dir,
		configFile:  configFile,
		secretsFile: filepath.Join(dir, "secrets.json"),
		secretsDir:  secretsDir,
		unitDir:     unitDir,
	}, nil
}

// probeTimeout bounds feature-detection probes. A released nenya that ignores
// unknown subcommands would otherwise fall through to server startup and block
// forever, so every probe is time-bounded and a timeout is treated as "absent".
// It is a var so tests can shrink it instead of sleeping through the real
// bound; mutating it is not safe for parallel tests.
var probeTimeout = 5 * time.Second

// probeOutput runs a feature-detection command with a bounded context.
func probeOutput(ctx context.Context, runner CommandRunner, name string, args ...string) ([]byte, error) {
	probeCtx, cancel := context.WithTimeout(ctx, probeTimeout)
	defer cancel()
	return runner.Output(probeCtx, name, args...)
}

// queryNenyaPaths feature-detects `nenya paths --json`. ok is false when the
// command does not exist or does not produce parseable output. The shape is
// shared with the contract client so the two cannot drift.
func queryNenyaPaths(ctx context.Context, runner CommandRunner, execPath string) (nenya.Paths, bool) {
	out, err := probeOutput(ctx, runner, execPath, "paths", "--json")
	if err != nil || len(out) == 0 {
		return nenya.Paths{}, false
	}
	var p nenya.Paths
	if err := json.Unmarshal(out, &p); err != nil {
		return nenya.Paths{}, false
	}
	return p, true
}

// BootstrapConfigContent returns the config content to write for a fresh
// install: `nenya example-config` (CONTRACT.md §4.4) when available, else a
// documented minimal shim. It is the single source for both install and
// `config init`, so the two paths cannot diverge.
func BootstrapConfigContent(ctx context.Context, runner CommandRunner, execPath string) []byte {
	if out, err := probeOutput(ctx, runner, execPath, "example-config"); err == nil && len(strings.TrimSpace(string(out))) > 0 {
		return out
	}
	return []byte(minimalConfig)
}

// bootstrapConfig creates configDir and config.json when absent. Content comes
// from `nenya example-config` when available, else from minimalConfig.
func bootstrapConfig(ctx context.Context, runner CommandRunner, execPath string, p installPaths) (bool, error) {
	if _, err := os.Stat(p.configFile); err == nil {
		return false, nil
	} else if !os.IsNotExist(err) {
		return false, fmt.Errorf("stat %s: %w", p.configFile, err)
	}

	content := BootstrapConfigContent(ctx, runner, execPath)

	if err := os.MkdirAll(p.configDir, 0o755); err != nil {
		return false, fmt.Errorf("create config dir %s: %w", p.configDir, err)
	}
	return writeNewFile(p.configFile, content, 0o644)
}

// bootstrapSecrets creates the deployment's client token when absent. On a
// binary with the secret writer (feature-detected via `secret set -h`,
// CONTRACT.md §4.7) it delegates: `nenya secret get` first answers "is there a
// token" for sources a local scan cannot see (systemd credentials, env), then
// `nenya secret set --client-token --config-dir <root>` writes — nenya resolves
// the file (the config root since NENYA-102, the path the shipped unit wires
// via LoadCredential), writes atomically with mode 0600, generates a compliant
// token, and fails closed when a credential source would shadow the write; the
// reported location is verified against the nominal target so a misdirected
// write is an install error, not a silent gap. Released binaries without the
// writer fall back to hand-writing the config-root secrets.json: the documented
// shim, retired once `secret set` is everywhere.
// Existing secrets are never overwritten, so an install cannot rotate a token.
// The token is deliberately not printed.
func bootstrapSecrets(ctx context.Context, runner CommandRunner, execPath string, p installPaths) (bool, error) {
	if _, err := os.Stat(p.secretsFile); err == nil {
		return false, nil
	} else if !os.IsNotExist(err) {
		return false, fmt.Errorf("stat %s: %w", p.secretsFile, err)
	}

	if secretWriterSupported(ctx, runner, execPath) {
		// Pin the install's config root for BOTH the guard and the write: an
		// unpinned call would resolve nenya's §3.3 default (the system root)
		// and inspect — or rotate — another deployment's token.
		client := nenya.New(nenyaRunner{runner: runner, execPath: execPath}).WithConfigDir(p.configDir)
		if client.SecretGetSupported(ctx) {
			// The reader sees credential-dir and env sources a local scan
			// cannot; if it resolves a token, this install must not create
			// (or shadow) one.
			if tok, err := client.SecretGet(ctx, "client-token"); err == nil && tok != "" {
				fmt.Fprintf(os.Stderr, "Warning: the deployment already resolves a client token; not creating %s.\n", p.secretsFile)
				return false, nil
			}
		} else if found := secrets.ExistingTokenFile(p.secretsDir); found != "" {
			// Writer without reader: complement with the local scan.
			fmt.Fprintf(os.Stderr, "Warning: a client token already exists in %s; not creating %s.\n", found, p.secretsFile)
			return false, nil
		}
		path, err := client.SetClientTokenInRoot(ctx, p.configDir, "")
		if err != nil {
			return false, fmt.Errorf("secret set: %w", err)
		}
		if path == "" {
			return false, fmt.Errorf("secret set wrote the token but reported no target path")
		}
		if filepath.Clean(path) != filepath.Clean(p.secretsFile) {
			return false, fmt.Errorf("secret set wrote %s, expected %s (check NENYA_SECRETS_DIR / NENYA_CONFIG_DIR in the install environment)", path, p.secretsFile)
		}
		fmt.Printf("Wrote client token to %s\n", path)
		return true, nil
	}

	if found := secrets.ExistingTokenFile(p.secretsDir); found != "" {
		fmt.Fprintf(os.Stderr, "Warning: a client token already exists in %s; not creating %s.\n", found, p.secretsFile)
		fmt.Fprintln(os.Stderr, "Delete it (or set NENYA_SECRETS_DIR) before creating a new one from the install.")
		return false, nil
	}

	if err := os.MkdirAll(filepath.Dir(p.secretsFile), 0o700); err != nil {
		return false, fmt.Errorf("create secrets dir %s: %w", filepath.Dir(p.secretsFile), err)
	}

	token, err := secrets.GenerateClientToken()
	if err != nil {
		return false, err
	}
	content, err := json.MarshalIndent(map[string]string{
		"client_token": token,
	}, "", "  ")
	if err != nil {
		return false, fmt.Errorf("marshal secrets: %w", err)
	}
	content = append(content, '\n')
	return writeNewFile(p.secretsFile, content, 0o600)
}

// nenyaRunner adapts the install CommandRunner to the nenya contract client's
// Runner: the binary path becomes the command name, matching how every other
// nenya invocation in this package runs (and how tests script them).
type nenyaRunner struct {
	runner   CommandRunner
	execPath string
}

// Output runs `<execPath> <args…>` through the install runner.
func (n nenyaRunner) Output(ctx context.Context, args ...string) ([]byte, error) {
	return n.runner.Output(ctx, n.execPath, args...)
}

// secretWriterSupported reports whether the installed binary implements the
// `secret` writer surface (CONTRACT.md §4.7): `secret set -h` prints usage and
// exits 0 on a supported binary. Anything else — an unknown command, or a
// pre-contract binary that treats unknown subcommands as a server start and
// blocks until the bounded probe fires — means "unsupported", so callers fall
// back rather than assume. The probe tests the subcommand actually used for
// the write, so a binary with the reader but not the writer is unsupported.
func secretWriterSupported(ctx context.Context, runner CommandRunner, execPath string) bool {
	_, err := probeOutput(ctx, runner, execPath, "secret", "set", "-h")
	return err == nil
}

// writeNewFile writes content to path only if it does not already exist, using
// O_EXCL to avoid racing or clobbering a file created concurrently.
func writeNewFile(path string, content []byte, mode os.FileMode) (bool, error) {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, mode)
	if err != nil {
		if os.IsExist(err) {
			return false, nil
		}
		return false, fmt.Errorf("create %s: %w", path, err)
	}
	if _, err := f.Write(content); err != nil {
		_ = f.Close()
		return false, fmt.Errorf("write %s: %w", path, err)
	}
	if err := f.Close(); err != nil {
		return false, fmt.Errorf("close %s: %w", path, err)
	}
	fmt.Printf("Created %s (mode %04o)\n", path, mode.Perm())
	return true, nil
}

// enableService loads and enables the service so a fresh install is reachable.
// Failures are warnings: install can succeed on hosts without an active init
// system (e.g. containers).
func enableService(ctx context.Context, runner CommandRunner, unitDir string) {
	switch {
	case unitDir == "":
		return
	case isLaunchd(unitDir):
		plist := filepath.Join(unitDir, "com.gumieri.nenya.plist")
		if _, err := runner.Output(ctx, launchctlBin, "load", "-w", plist); err != nil {
			warnService("sudo launchctl load -w "+plist, "load %s: %v", plist, err)
		}
	case isSystemd(unitDir):
		if _, err := runner.Output(ctx, systemctlBin, "daemon-reload"); err != nil {
			warnService("sudo systemctl daemon-reload", "systemctl daemon-reload: %v", err)
		}
		if _, err := runner.Output(ctx, systemctlBin, "enable", "--now", "nenya.socket"); err != nil {
			warnService("sudo systemctl enable --now nenya.socket", "systemctl enable --now nenya.socket: %v", err)
		}
	}
}

func warnService(hint, format string, args ...any) {
	fmt.Fprintf(os.Stderr, "Warning: could not enable the nenya service: "+format+"\n", args...)
	fmt.Fprintln(os.Stderr, "Enable it later with: "+hint)
}

func isSystemd(unitDir string) bool {
	return strings.Contains(unitDir, "systemd")
}

func isLaunchd(unitDir string) bool {
	return strings.Contains(unitDir, "LaunchDaemons") || strings.Contains(unitDir, "LaunchAgents")
}
