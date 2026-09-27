// Package nenya is a typed client for the nenya consumer contract
// (CONTRACT.md). Every fact about configuration, paths, secrets layout, release
// artifacts, and the effective merged config comes from these commands, so
// nenyactl never reproduces nenya's loader, merge, or path resolution itself.
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

	"github.com/gumieri/nenyactl/internal/contract"
)

// Runner executes a nenya command and returns its standard output. It is
// injectable so contract calls can be tested without the binary installed.
type Runner interface {
	Output(ctx context.Context, args ...string) ([]byte, error)
}

// EnvRunner is a Runner that can carry extra environment entries. Binary
// implements it, which is how SecretWriter targets a secrets directory.
type EnvRunner interface {
	Runner
	// WithEnv returns a Runner whose environment is extended with env entries
	// ("K=V"), leaving the receiver unchanged.
	WithEnv(env ...string) Runner
}

// Binary is the Runner backed by an installed nenya binary.
type Binary struct {
	// Path is the nenya executable. Defaults to "nenya" on PATH.
	Path string
	// Env holds extra environment entries ("K=V") appended to the process
	// environment. Empty means the process inherits its environment.
	Env []string
}

// WithEnv returns a copy of the Binary with env appended.
func (b Binary) WithEnv(env ...string) Runner {
	b.Env = append(append([]string{}, b.Env...), env...)
	return b
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
		return out, commandError(args, err, stderr.String())
	}
	return out, nil
}

// Call budgets. Reads get a short bound; writes get a longer one because a
// mid-write timeout cannot be retried safely by nenyactl.
const (
	probeTimeout = 10 * time.Second
	writeTimeout = 30 * time.Second
)

// Client is a contract client bound to a Runner and, optionally, to a config
// directory, so every call sees the same target. nenyactl manages directory-mode
// deployments (CONTRACT.md §5.1), so the target is always a config root.
type Client struct {
	runner    Runner
	configDir string
}

// New returns a Client for the given runner with no target pinned.
func New(runner Runner) *Client { return &Client{runner: runner} }

// WithConfigDir returns a copy of the client pinned to a config directory.
func (c *Client) WithConfigDir(dir string) *Client {
	cp := *c
	cp.configDir = dir
	return &cp
}

// target returns the flags that select the config root for a command.
func (c *Client) target() []string {
	if c.configDir != "" {
		return []string{"--config-dir", c.configDir}
	}
	return nil
}

// output runs a read-only contract call under probeTimeout.
func (c *Client) output(ctx context.Context, args ...string) ([]byte, error) {
	return runBounded(ctx, c.runner, probeTimeout, args...)
}

// runBounded executes a contract call under a bounded context. A nil context is
// normalized so a caller that bypasses cobra cannot panic the process, and a
// cancelled caller context is surfaced as cancellation rather than as the
// command's own exit error.
func runBounded(ctx context.Context, runner Runner, timeout time.Duration, args ...string) ([]byte, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	out, err := runner.Output(ctx, args...)
	if err != nil && ctx.Err() != nil {
		return out, fmt.Errorf("%w: %w", ctx.Err(), err)
	}
	return out, err
}

// commandError builds a non-leaking error for a failed command. Secret commands
// carry an API key or client token as an argument and may echo it on stderr, so
// neither the argument values nor stderr are included. `config set` can also
// carry a secret in its value, so the value is scrubbed from stderr.
func commandError(args []string, err error, stderr string) error {
	msg := strings.TrimSpace(stderr)
	if isSecretCommand(args) {
		return fmt.Errorf("nenya %s: %w", redactedArgs(args), err)
	}
	if isConfigSet(args) && msg != "" {
		msg = strings.ReplaceAll(msg, args[len(args)-1], "<value>")
	}
	if msg != "" {
		return fmt.Errorf("nenya %s: %w: %s", redactedArgs(args), err, msg)
	}
	return fmt.Errorf("nenya %s: %w", redactedArgs(args), err)
}

// isSecretCommand reports whether args invokes `nenya secret ...`.
func isSecretCommand(args []string) bool {
	return len(args) > 0 && args[0] == "secret"
}

// isConfigSet reports whether args invokes `nenya config set ...`.
func isConfigSet(args []string) bool {
	return len(args) >= 2 && args[0] == "config" && args[1] == "set"
}

// redactedArgs renders an invocation for an error message, omitting the values
// of secret commands and of `config set` (which can carry a secret in the value).
func redactedArgs(args []string) string {
	if isSecretCommand(args) {
		return "secret set …"
	}
	if isConfigSet(args) {
		return strings.Join(args[:len(args)-1], " ") + " <value>"
	}
	return strings.Join(args, " ")
}

// Paths mirrors `nenya paths --json` (CONTRACT.md §4.2, Appendix A.2).
type Paths struct {
	Mode       string `json:"mode"`
	ConfigDir  string `json:"config_dir"`
	ConfigFile string `json:"config_file"`
	ConfigD    string `json:"config_d"`
	SecretsDir string `json:"secrets_dir"`
	// SocketPath is null unless a Unix-domain socket is configured.
	SocketPath *string `json:"socket_path"`
	Platform   string  `json:"platform"`
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

// ProviderCatalogEntry is one row of the model catalog.
type ProviderCatalogEntry struct {
	Provider      string `json:"provider"`
	Model         string `json:"model"`
	ContextWindow int    `json:"context_window"`
	MaxOutput     int    `json:"max_output"`
}

// Version mirrors `nenya version --json` (CONTRACT.md §4.1).
type Version struct {
	Version         string `json:"version"`
	Commit          string `json:"commit"`
	BuildTime       string `json:"build_time"`
	ContractVersion int    `json:"contract_version"`
}

// Description mirrors `nenya describe --json` (CONTRACT.md §4.3). Config is the
// authoritative effective document; nenyactl must render from it and never
// recompute the merge. Paths/Secrets/Providers/Diagnostics mirror the contract
// shape so the client can carry the whole document as the seam grows.
type Description struct {
	ContractVersion int             `json:"contract_version"`
	Version         Version         `json:"version"`
	Paths           Paths           `json:"paths"`
	Secrets         SecretsSource   `json:"secrets"`
	Config          json.RawMessage `json:"config"`
	Providers       struct {
		Configured []string               `json:"configured"`
		Catalog    []ProviderCatalogEntry `json:"catalog"`
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
	if d.ContractVersion != 0 {
		if err := contract.Check(d.ContractVersion); err != nil {
			return Description{}, err
		}
	}
	if string(d.Config) == "null" {
		d.Config = nil
	}
	return d, nil
}

// SetConfig sets a dotted config key through nenya's single writer. The value is
// passed verbatim; nenya parses it as JSON when valid, else as a string. It
// returns the target path nenya reports on stdout.
func (c *Client) SetConfig(ctx context.Context, dottedKey, value string) (string, error) {
	args := append([]string{"config", "set"}, c.target()...)
	args = append(args, dottedKey, value)
	out, err := runBounded(ctx, c.runner, writeTimeout, args...)
	if err != nil {
		return "", fmt.Errorf("nenya config set %s: %w", dottedKey, err)
	}
	return strings.TrimSpace(string(out)), nil
}

// SecretWriter writes secrets through nenya's single writer, targeting a
// specific secrets directory via NENYA_SECRETS_DIR (CONTRACT.md §3.3). nenya's
// `secret set` resolves the file itself (and fails closed on systemd
// credentials), so nenyactl only supplies the directory the deployment uses.
type SecretWriter struct {
	runner Runner
	dir    string
}

// SecretWriterFor returns a SecretWriter targeting dir. It injects
// NENYA_SECRETS_DIR when the runner implements EnvRunner (Binary does); a runner
// that cannot carry environment writes through nenya's default source, so callers
// that need targeting must supply an EnvRunner.
func (c *Client) SecretWriterFor(dir string) SecretWriter {
	runner := c.runner
	if er, ok := runner.(EnvRunner); ok {
		runner = er.WithEnv("NENYA_SECRETS_DIR=" + dir)
	}
	return SecretWriter{runner: runner, dir: dir}
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
	out, err := runBounded(ctx, w.runner, writeTimeout, args...)
	if err != nil {
		return "", commandError(args, err, "")
	}
	return strings.TrimSpace(string(out)), nil
}
