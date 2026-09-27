// Package nenya is a typed client for the nenya consumer contract
// (CONTRACT.md). Every fact about configuration, paths, secrets layout, release
// artifacts, and the effective merged config comes from these commands — nenyactl
// never reproduces nenya's loader, merge, or path resolution itself.
package nenya

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

// Runner executes a nenya command and returns its standard output. It is
// injectable so contract calls can be tested without the binary installed.
type Runner interface {
	Output(ctx context.Context, args ...string) ([]byte, error)
}

// Binary is the Runner backed by an installed nenya binary.
type Binary struct {
	// Path is the nenya executable. Defaults to "nenya" on PATH.
	Path string
	// Env holds extra environment entries ("K=V") appended to the process
	// environment. Used to target a secrets directory for `secret set`.
	Env []string
}

// Output runs `nenya <args>` and returns stdout.
func (b Binary) Output(ctx context.Context, args ...string) ([]byte, error) {
	path := b.Path
	if path == "" {
		path = "nenya"
	}
	cmd := exec.CommandContext(ctx, path, args...)
	if len(b.Env) > 0 {
		cmd.Env = append(os.Environ(), b.Env...)
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		if msg := strings.TrimSpace(stderr.String()); msg != "" {
			return out, fmt.Errorf("nenya %s: %w: %s", strings.Join(args, " "), err, msg)
		}
		return out, fmt.Errorf("nenya %s: %w", strings.Join(args, " "), err)
	}
	return out, nil
}

// probeTimeout bounds each contract call so a misbehaving binary cannot hang
// nenyactl.
const probeTimeout = 10 * time.Second

// Client is a contract client bound to a binary and a working directory of
// context (config dir or file), so every call sees the same target.
type Client struct {
	runner Runner
	// configDir/configFile pin the target; either may be empty to use nenya's
	// own defaults.
	configDir  string
	configFile string
}

// New returns a Client for the given runner with no target pinned.
func New(runner Runner) *Client { return &Client{runner: runner} }

// WithConfigDir returns a copy of the client pinned to a config directory.
func (c *Client) WithConfigDir(dir string) *Client {
	cp := *c
	cp.configDir = dir
	cp.configFile = ""
	return &cp
}

// WithConfigFile returns a copy of the client pinned to a single config file.
func (c *Client) WithConfigFile(file string) *Client {
	cp := *c
	cp.configFile = file
	cp.configDir = ""
	return &cp
}

// target returns the flags that select the config root/file for a command.
func (c *Client) target() []string {
	switch {
	case c.configFile != "":
		return []string{"--config", c.configFile}
	case c.configDir != "":
		return []string{"--config-dir", c.configDir}
	default:
		return nil
	}
}

func (c *Client) output(ctx context.Context, args ...string) ([]byte, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	ctx, cancel := context.WithTimeout(ctx, probeTimeout)
	defer cancel()
	return c.runner.Output(ctx, args...)
}

// Paths mirrors `nenya paths --json` (CONTRACT.md §4.2, Appendix A.2).
type Paths struct {
	Mode       string  `json:"mode"`
	ConfigDir  string  `json:"config_dir"`
	ConfigFile string  `json:"config_file"`
	ConfigD    string  `json:"config_d"`
	SecretsDir string  `json:"secrets_dir"`
	SocketPath *string `json:"socket_path"`
	Platform   string  `json:"platform"`
}

// Paths resolves the filesystem contract.
func (c *Client) Paths(ctx context.Context) (Paths, error) {
	out, err := c.output(ctx, append([]string{"paths", "--json"}, c.target()...)...)
	if err != nil {
		return Paths{}, fmt.Errorf("nenya paths --json: %w", err)
	}
	var p Paths
	if err := json.Unmarshal(out, &p); err != nil {
		return Paths{}, fmt.Errorf("parse paths --json: %w", err)
	}
	return p, nil
}

// SecretsSource reports which secrets source won and which were searched.
type SecretsSource struct {
	ActiveSource string   `json:"active_source"`
	Searched     []string `json:"searched"`
}

// Diagnostic is a load-time diagnostic (CONTRACT.md Appendix A.3).
type Diagnostic struct {
	Level   string `json:"level"`
	Code    string `json:"code"`
	Message string `json:"message"`
	Source  string `json:"source"`
}

// Description mirrors `nenya describe --json` (CONTRACT.md §4.3). Config is the
// authoritative effective document; nenyactl must render from it and never
// recompute the merge.
type Description struct {
	ContractVersion int             `json:"contract_version"`
	Version         Version         `json:"version"`
	Paths           Paths           `json:"paths"`
	Secrets         SecretsSource   `json:"secrets"`
	Config          json.RawMessage `json:"config"`
	Providers       struct {
		Configured []string `json:"configured"`
		Catalog    []struct {
			Provider      string `json:"provider"`
			Model         string `json:"model"`
			ContextWindow int    `json:"context_window"`
			MaxOutput     int    `json:"max_output"`
		} `json:"catalog"`
	} `json:"providers"`
	Diagnostics []Diagnostic `json:"diagnostics"`
}

// Describe returns the effective state. It is the single source of truth for
// "what config is actually in effect".
func (c *Client) Describe(ctx context.Context) (Description, error) {
	out, err := c.output(ctx, append([]string{"describe", "--json"}, c.target()...)...)
	if err != nil {
		return Description{}, fmt.Errorf("nenya describe --json: %w", err)
	}
	var d Description
	if err := json.Unmarshal(out, &d); err != nil {
		return Description{}, fmt.Errorf("parse describe --json: %w", err)
	}
	return d, nil
}

// Version mirrors `nenya version --json` (CONTRACT.md §4.1).
type Version struct {
	Version         string `json:"version"`
	Commit          string `json:"commit"`
	BuildTime       string `json:"build_time"`
	ContractVersion int    `json:"contract_version"`
}

// Version returns the version surface.
func (c *Client) Version(ctx context.Context) (Version, error) {
	out, err := c.output(ctx, "version", "--json")
	if err != nil {
		return Version{}, fmt.Errorf("nenya version --json: %w", err)
	}
	var v Version
	if err := json.Unmarshal(out, &v); err != nil {
		return Version{}, fmt.Errorf("parse version --json: %w", err)
	}
	return v, nil
}

// ExampleConfig returns the canonical example config JSONC (CONTRACT.md §4.4).
func (c *Client) ExampleConfig(ctx context.Context) ([]byte, error) {
	out, err := c.output(ctx, "example-config")
	if err != nil {
		return nil, fmt.Errorf("nenya example-config: %w", err)
	}
	return out, nil
}

// ServiceUnitOptions selects the emitted service unit (CONTRACT.md §4.5).
type ServiceUnitOptions struct {
	Init        string
	ExecPath    string
	ConfigDir   string
	SecretsFile string
}

// ServiceUnit returns the shipped unit with the supplied paths substituted.
func (c *Client) ServiceUnit(ctx context.Context, opts ServiceUnitOptions) ([]byte, error) {
	args := []string{"service-unit"}
	if opts.Init != "" {
		args = append(args, "--init", opts.Init)
	}
	if opts.ExecPath != "" {
		args = append(args, "--exec-path", opts.ExecPath)
	}
	if opts.ConfigDir != "" {
		args = append(args, "--config-dir", opts.ConfigDir)
	}
	if opts.SecretsFile != "" {
		args = append(args, "--secrets-file", opts.SecretsFile)
	}
	out, err := c.output(ctx, args...)
	if err != nil {
		return nil, fmt.Errorf("nenya service-unit: %w", err)
	}
	return out, nil
}

// SetConfig sets a dotted config key through nenya's single writer. The value is
// passed verbatim; nenya parses it as JSON when valid, else as a string. It
// returns the path nenya wrote (printed by `config set`).
func (c *Client) SetConfig(ctx context.Context, dottedKey, value string) (string, error) {
	out, err := c.output(ctx, append([]string{"config", "set"}, append(c.target(), dottedKey, value)...)...)
	if err != nil {
		return "", fmt.Errorf("nenya config set %s: %w", dottedKey, err)
	}
	return strings.TrimSpace(string(out)), nil
}

// SecretWriter is a Client whose secret commands target a specific secrets
// directory via NENYA_SECRETS_DIR. nenya's `secret set` resolves the file
// itself (and fails closed on systemd credentials), so nenyactl only supplies
// the directory the deployment uses.
type SecretWriter struct {
	runner Runner
	dir    string
}

// SecretWriterFor returns a SecretWriter targeting dir.
func (c *Client) SecretWriterFor(dir string) SecretWriter {
	return SecretWriter{runner: secretsEnvRunner{base: c.runner, dir: dir}, dir: dir}
}

// Dir returns the secrets directory the writer targets.
func (w SecretWriter) Dir() string { return w.dir }

// SetClientToken sets (or generates, when token is empty) the client token.
func (w SecretWriter) SetClientToken(ctx context.Context, token string) (string, error) {
	args := []string{"secret", "set", "--client-token"}
	if token != "" {
		args = append(args, token)
	}
	return w.output(ctx, args...)
}

// SetProviderKey sets provider_keys[provider].
func (w SecretWriter) SetProviderKey(ctx context.Context, provider, apiKey string) (string, error) {
	return w.output(ctx, "secret", "set", "--provider", provider, apiKey)
}

func (w SecretWriter) output(ctx context.Context, args ...string) (string, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	ctx, cancel := context.WithTimeout(ctx, probeTimeout)
	defer cancel()
	out, err := w.runner.Output(ctx, args...)
	if err != nil {
		return "", fmt.Errorf("nenya %s: %w", strings.Join(args, " "), err)
	}
	return strings.TrimSpace(string(out)), nil
}

// secretsEnvRunner injects NENYA_SECRETS_DIR into command invocations.
type secretsEnvRunner struct {
	base Runner
	dir  string
}

func (r secretsEnvRunner) Output(ctx context.Context, args ...string) ([]byte, error) {
	if b, ok := r.base.(Binary); ok {
		b.Env = append(append([]string{}, b.Env...), "NENYA_SECRETS_DIR="+r.dir)
		return b.Output(ctx, args...)
	}
	return r.base.Output(ctx, args...)
}
