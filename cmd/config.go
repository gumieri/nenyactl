package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode"

	"github.com/gumieri/nenyactl/internal/config"
	"github.com/gumieri/nenyactl/internal/detect"
	"github.com/gumieri/nenyactl/internal/install"
	"github.com/gumieri/nenyactl/internal/nenya"
	"github.com/gumieri/nenyactl/internal/paths"
	"github.com/gumieri/nenyactl/internal/secrets"
	"github.com/spf13/cobra"
)

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Manage Nenya configuration",
	Long:  `Bootstrap, edit, and manage the Nenya configuration.`,
}

func init() {
	rootCmd.AddCommand(configCmd)
	configCmd.AddCommand(configInitCmd)
	configCmd.AddCommand(configEditCmd)
}

var configInitCmd = &cobra.Command{
	Use:   "init",
	Short: "Create initial configuration",
	Long: `Create the Nenya configuration directory and write
the example config file.

Default location: ` + paths.SystemConfigDir() + `
Use --dir to specify a custom path.`,
	RunE: runConfigInit,
}

var configDir string

func init() {
	configInitCmd.Flags().StringVar(&configDir, "dir", "", "Config root or container directory (default: system config root)")
}

func runConfigInit(cmd *cobra.Command, args []string) error {
	res, err := resolveDir(cmd.Context(), configDir, dirCreate, false)
	if err != nil {
		return err
	}

	if err := bootstrapConfig(res.ConfigDir()); err != nil {
		return fmt.Errorf("config init: %w", err)
	}
	fmt.Println(successStyle.Render("✓"), "Config directory created:", res.ConfigDir())
	return nil
}

// bootstrapContentProbe sources the bootstrap config content. Package var so
// tests can stub the host `nenya example-config` probe; the default delegates
// to install's feature-detected probe. Mutating it is not safe for parallel
// tests.
var bootstrapContentProbe = func() []byte {
	return install.BootstrapConfigContent(context.Background(), install.NewExecRunner(), "nenya")
}

// bootstrapConfig is the documented bootstrap shim: it writes
// `<config-root>/config.json` directly because `config init` must work before a
// nenya binary exists. The content comes from `nenya example-config` when
// available, else install's minimal shim, so nenyactl never embeds a copy.
func bootstrapConfig(dir string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create %s: %w", dir, err)
	}

	path := filepath.Join(dir, "config.json")
	if _, err := os.Stat(path); err == nil {
		fmt.Println(dimStyle.Render("  ∃"), "Skipping existing", path)
		return nil
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("stat %s: %w", path, err)
	}

	content := bootstrapContentProbe()
	if err := os.WriteFile(path, content, 0o644); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	fmt.Println(successStyle.Render("✓"), "Wrote", path)
	return nil
}

var configEditCmd = &cobra.Command{
	Use:   "edit",
	Short: "Edit Nenya configuration",
	Long: `Open an interactive TUI editor for the Nenya configuration file, or
apply changes non-interactively with --set (repeatable; dotted keys;
values use the same literal rules as the TUI — bare strings are
quoted, true/false/null and numbers pass through).

--print-diff renders the pending change set without writing anything.
With --set it shows what would be applied; after an interactive
session it shows what the session changed and skips the write.

Detects whether Nenya is installed as bare-metal or as a container.
Use --dir to override automatic detection.`,
	RunE: runConfigEdit,
}

var (
	configEditDir  string
	configEditSet  []string
	configEditDiff bool
)

func init() {
	configEditCmd.Flags().StringVar(&configEditDir, "dir", "", "Config root or container directory (auto-detected)")
	configEditCmd.Flags().StringArrayVar(&configEditSet, "set", nil, "set a config key non-interactively: --set key=value (dotted key; repeatable, last wins; same literal rules as the TUI plus local validation: malformed JSON rejected, non-JSON numbers quoted)")
	configEditCmd.Flags().BoolVar(&configEditDiff, "print-diff", false, "print the pending change set without writing")
}

// describeForEdit is the Describe seam runConfigEdit uses, stubbed in tests
// to prove the --set path never requires a live daemon (NCTL-48 review).
var describeForEdit = func(ctx context.Context, client *nenya.Client) (nenya.Description, error) {
	return client.Describe(ctx)
}

// resolveEditDirFn is the resolution seam runConfigEdit uses, stubbed in
// tests to avoid host detection (NCTL-48 review).
var resolveEditDirFn = resolveEditDir

func runConfigEdit(cmd *cobra.Command, args []string) error {
	res, err := resolveEditDirFn(cmd.Context())
	if err != nil {
		return err
	}

	client := res.Contract()

	// Non-interactive --set path runs before Describe: a pure --print-diff
	// dry run must not require a live daemon (NCTL-48 review).
	if len(configEditSet) > 0 {
		return runConfigSetFlags(cmd, client)
	}

	desc, err := describeForEdit(cmd.Context(), client)
	if err != nil {
		return fmt.Errorf("read effective config via `nenya describe --json`: %w", err)
	}
	if len(desc.Config) == 0 {
		return fmt.Errorf("nenya describe returned no config for %s", res.ConfigDir())
	}

	fmt.Println(infoStyle.Render("›"), "Editing:", res.ConfigDir())

	secretsDir := secretsDirFor(res, desc)
	result, changed, err := config.RunConfigEditor(desc.Config, providerKeyStatus(secretsDir))
	if err != nil {
		return fmt.Errorf("editor: %w", err)
	}

	if !changed || (len(result.Changes) == 0 && len(result.KeyChanges) == 0) {
		fmt.Println(infoStyle.Render("›"), "No changes made")
		return nil
	}

	// --print-diff after an interactive session shows what the session
	// changed and deliberately skips the write (NCTL-48).
	if configEditDiff {
		if len(result.Changes) > 0 {
			fmt.Print(config.RenderChanges(result.Changes))
		}
		for _, op := range result.ProviderKeys() {
			fmt.Printf("provider key %s: set/rotate via nenya secret writer\n", op.Provider)
		}
		fmt.Println(dimStyle.Render("Dry run: nothing written (--print-diff)"))
		return nil
	}

	// Provider keys go through the single writer, which needs the secrets
	// directory — fail before any write when it cannot be resolved, so the
	// run is all-or-nothing at the gate level.
	if len(result.KeyChanges) > 0 && secretsDir == "" {
		return fmt.Errorf("cannot determine the secrets directory for %s", res.Path)
	}

	if err := applyConfigChanges(cmd, client, result); err != nil {
		return err
	}
	return applyProviderKeyChanges(cmd, client, secretsDir, result)
}

// resolveEditDir resolves the deployment the edit targets: --dir when given,
// auto-detection otherwise.
func resolveEditDir(ctx context.Context) (dirResolution, error) {
	if configEditDir != "" {
		return resolveDir(ctx, configEditDir, dirAttach, false)
	}
	info, err := detect.Detect()
	if err != nil {
		return dirResolution{}, err
	}
	return detectedResolution(info), nil
}

// parseSetFlags parses repeatable --set key=value flags into a change set.
// Values go through config.ParseConfigValue, the same literal rules as the
// TUI editor, so flags and TUI produce identical records (NCTL-48). Dotted
// keys must have only non-empty segments — the TUI can only produce real
// config keys, and the flag path should not be looser.
func parseSetFlags(sets []string) (map[string]string, error) {
	changes := make(map[string]string, len(sets))
	for _, s := range sets {
		key, value, found := strings.Cut(s, "=")
		if !found {
			return nil, fmt.Errorf("--set expects key=value, got %q (missing =)", s)
		}
		key = strings.TrimSpace(key)
		if err := validateDottedKey(key); err != nil {
			return nil, fmt.Errorf("--set %q: %w", s, err)
		}
		literal, err := config.ParseConfigValue(value)
		if err != nil {
			return nil, fmt.Errorf("--set %q: %w", s, err)
		}
		changes[key] = literal
	}
	return changes, nil
}

// validateDottedKey rejects keys with empty dot-separated segments (a..b, .a,
// a.) and any whitespace inside a segment, which nenya's writer would
// otherwise persist or reject opaquely (NCTL-48 review). Surrounding
// whitespace is already trimmed by the caller.
func validateDottedKey(key string) error {
	if key == "" {
		return fmt.Errorf("empty key")
	}
	for _, seg := range strings.Split(key, ".") {
		if seg == "" {
			return fmt.Errorf("key %q has an empty dotted segment", key)
		}
		if strings.IndexFunc(seg, unicode.IsSpace) >= 0 {
			return fmt.Errorf("key %q contains whitespace inside a segment", key)
		}
	}
	return nil
}

// runConfigSetFlags applies --set changes non-interactively; --print-diff
// renders the change set instead of writing (NCTL-48).
func runConfigSetFlags(cmd *cobra.Command, client *nenya.Client) error {
	changes, err := parseSetFlags(configEditSet)
	if err != nil {
		return err
	}
	if configEditDiff {
		fmt.Print(config.RenderChanges(changes))
		fmt.Println(dimStyle.Render("Dry run: nothing written (--print-diff)"))
		return nil
	}
	return applyConfigChanges(cmd, client, &config.EditorResult{Changes: changes})
}

// secretsDirFor resolves the secrets directory. For a bare-metal config root,
// nenya's contract answer (paths --json / describe) wins — it honors
// NENYA_SECRETS_DIR outside the config root. For container deployments,
// describe runs host-side and its answer is the host config root's layout,
// while the compose file mounts the real secrets dir into the container —
// there the local resolution (the mounted dir nenyactl manages) is correct,
// mirroring every sibling writer call site.
func secretsDirFor(res dirResolution, desc nenya.Description) string {
	if res.Kind == dirConfigRoot {
		if desc.Paths.SecretsDir != "" {
			return desc.Paths.SecretsDir
		}
		if c := res.contractSecretsDir(); c != "" {
			return c
		}
	}
	return res.Info.SecretsDir()
}

// providerKeyStatus gathers secrets-file key presence for the providers
// screen: names only, never values (NCTL-45).
func providerKeyStatus(secretsDir string) []config.ProviderKeyStatus {
	var status []config.ProviderKeyStatus
	for _, name := range secrets.ProviderKeyNames(secretsDir) {
		// The merge spans several *.json documents; the label stays generic.
		status = append(status, config.ProviderKeyStatus{Name: name, KeySet: true, Source: "secrets dir"})
	}
	return status
}

// applyConfigChanges applies every config change through nenya's single
// writer. nenya chooses the target file (managed drop-in or config file),
// merges, and writes atomically, so nenyactl never reproduces the loader or
// the merge. Each key is one call, so a mid-way failure is reported with what
// already landed rather than pretending the edit was atomic.
func applyConfigChanges(cmd *cobra.Command, client *nenya.Client, result *config.EditorResult) error {
	keys := make([]string, 0, len(result.Changes))
	for k := range result.Changes {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var applied []string
	for _, key := range keys {
		path, err := client.SetConfig(cmd.Context(), key, result.Changes[key])
		if err != nil {
			if len(applied) > 0 {
				return fmt.Errorf("apply %s: %w (already applied: %s)", key, err, strings.Join(applied, ", "))
			}
			return fmt.Errorf("apply %s: %w", key, err)
		}
		applied = append(applied, key)
		fmt.Println(successStyle.Render("✓"), key, "→", path)
	}
	return nil
}

// applyProviderKeyChanges applies provider-key changes through nenya's single
// writer — the same path as `nenyactl secret set --provider <name>` (NCTL-45).
// A mid-way failure reports what already landed (config keys and prior
// provider keys), mirroring applyConfigChanges.
func applyProviderKeyChanges(cmd *cobra.Command, client *nenya.Client, secretsDir string, result *config.EditorResult) error {
	writer := client.SecretWriterFor(secretsDir)
	ops := result.ProviderKeys()
	var applied []string
	for _, op := range ops {
		path, err := writer.SetProviderKey(cmd.Context(), op.Provider, op.Value)
		if err != nil {
			if len(applied) > 0 {
				return fmt.Errorf("apply provider key %s: %w (already applied: %s)", op.Provider, err, strings.Join(applied, ", "))
			}
			return fmt.Errorf("apply provider key %s: %w", op.Provider, err)
		}
		applied = append(applied, op.Provider)
		fmt.Println(successStyle.Render("✓"), "provider key", op.Provider, "→", path)
	}
	return nil
}
