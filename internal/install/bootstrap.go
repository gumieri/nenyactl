package install

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

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
	unitDir     string
}

// resolveInstallPaths determines where config, secrets, and units live. It
// prefers `nenya paths --json` (contract target) for the config root and falls
// back to nenyactl's platform defaults until that command ships.
func resolveInstallPaths(ctx context.Context, cfg Config, runner CommandRunner, execPath string) installPaths {
	if cfg.UserInstall {
		dir := cfg.configDirOverride
		if dir == "" {
			d, err := userConfigDir()
			if err != nil {
				d = filepath.Join(".nenyactl", "nenya")
			}
			dir = d
		}
		return installPaths{
			configDir:   dir,
			configFile:  filepath.Join(dir, "config.json"),
			secretsFile: filepath.Join(dir, "secrets.json"),
		}
	}

	dir := cfg.configDirOverride
	if dir == "" {
		dir = systemConfigDir()
	}
	if p, ok := queryNenyaPaths(ctx, runner, execPath); ok && p.ConfigDir != "" {
		dir = p.ConfigDir
	}

	unitDir := cfg.unitDirOverride
	if unitDir == "" {
		unitDir = systemUnitDir()
	}

	return installPaths{
		configDir:   dir,
		configFile:  filepath.Join(dir, "config.json"),
		secretsFile: filepath.Join(dir, "secrets.json"),
		unitDir:     unitDir,
	}
}

// nenyaPaths mirrors the `nenya paths --json` shape (CONTRACT.md §4.2).
type nenyaPaths struct {
	Mode       string  `json:"mode"`
	ConfigDir  string  `json:"config_dir"`
	ConfigFile string  `json:"config_file"`
	ConfigD    string  `json:"config_d"`
	SecretsDir string  `json:"secrets_dir"`
	SocketPath *string `json:"socket_path"`
	Platform   string  `json:"platform"`
}

// queryNenyaPaths feature-detects `nenya paths --json`. ok is false when the
// command does not exist or does not produce parseable output.
func queryNenyaPaths(ctx context.Context, runner CommandRunner, execPath string) (nenyaPaths, bool) {
	out, err := runner.Output(ctx, execPath, "paths", "--json")
	if err != nil || len(out) == 0 {
		return nenyaPaths{}, false
	}
	var p nenyaPaths
	if err := json.Unmarshal(out, &p); err != nil {
		return nenyaPaths{}, false
	}
	return p, true
}

// bootstrapConfig creates configDir and config.json when absent. Content comes
// from `nenya example-config` when available, else from minimalConfig.
func bootstrapConfig(ctx context.Context, runner CommandRunner, execPath string, p installPaths) (bool, error) {
	if _, err := os.Stat(p.configFile); err == nil {
		return false, nil
	}

	content := []byte(minimalConfig)
	if out, err := runner.Output(ctx, execPath, "example-config"); err == nil && len(strings.TrimSpace(string(out))) > 0 {
		content = out
	}

	if err := os.MkdirAll(p.configDir, 0o755); err != nil {
		return false, fmt.Errorf("create config dir %s: %w", p.configDir, err)
	}
	if err := os.WriteFile(p.configFile, content, 0o644); err != nil {
		return false, fmt.Errorf("write %s: %w", p.configFile, err)
	}
	fmt.Printf("Created %s\n", p.configFile)
	return true, nil
}

// bootstrapSecrets creates secrets.json with a fresh client token when absent.
// Existing secrets are never overwritten, so an install cannot rotate a token.
// The token is not printed.
func bootstrapSecrets(p installPaths) (bool, error) {
	if _, err := os.Stat(p.secretsFile); err == nil {
		return false, nil
	}

	if err := os.MkdirAll(filepath.Dir(p.secretsFile), 0o755); err != nil {
		return false, fmt.Errorf("create secrets dir %s: %w", filepath.Dir(p.secretsFile), err)
	}

	content, err := json.MarshalIndent(map[string]string{
		"client_token": secrets.GenerateClientToken(),
	}, "", "  ")
	if err != nil {
		return false, fmt.Errorf("marshal secrets: %w", err)
	}
	content = append(content, '\n')

	if err := os.WriteFile(p.secretsFile, content, 0o600); err != nil {
		return false, fmt.Errorf("write %s: %w", p.secretsFile, err)
	}
	fmt.Printf("Created %s (mode 0600)\n", p.secretsFile)
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
			warnService("load %s: %v", plist, err)
		}
	case isSystemd(unitDir):
		if _, err := runner.Output(ctx, systemctlBin, "daemon-reload"); err != nil {
			warnService("systemctl daemon-reload: %v", err)
		}
		if _, err := runner.Output(ctx, systemctlBin, "enable", "--now", "nenya.socket"); err != nil {
			warnService("systemctl enable --now nenya.socket: %v", err)
		}
	}
}

func warnService(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "Warning: could not enable the nenya service: "+format+"\n", args...)
	fmt.Fprintln(os.Stderr, "Enable it later with: sudo systemctl enable --now nenya.socket")
}

func isSystemd(unitDir string) bool {
	return strings.Contains(unitDir, "systemd")
}

func isLaunchd(unitDir string) bool {
	return strings.Contains(unitDir, "LaunchDaemons") || strings.Contains(unitDir, "LaunchAgents")
}
