package install

// This package deliberately embeds no copy of Nenya's example config:
// CONTRACT.md §4.4 reserves `nenya example-config` for that, and the boundary
// rule forbids re-encoding it. The one shim is BootstrapConfigContent below.
// internal/containers/setup.go keeps a second minimal shim (it cannot assume a
// nenya binary); collapse both once example-config ships.

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
// prefers `nenya paths --json` (contract target) for the config root/file and
// falls back to platform defaults until that command ships.
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
const probeTimeout = 5 * time.Second

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

// bootstrapSecrets creates secrets.json with a fresh client token when absent.
// Existing secrets are never overwritten, so an install cannot rotate a token.
// The token is deliberately not printed.
func bootstrapSecrets(p installPaths) (bool, error) {
	if _, err := os.Stat(p.secretsFile); err == nil {
		return false, nil
	} else if !os.IsNotExist(err) {
		return false, fmt.Errorf("stat %s: %w", p.secretsFile, err)
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
