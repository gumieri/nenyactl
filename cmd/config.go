package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

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
	Long: `Open an interactive TUI editor for the Nenya configuration file.

Detects whether Nenya is installed as bare-metal or as a container.
Use --dir to override automatic detection.`,
	RunE: runConfigEdit,
}

var configEditDir string

func init() {
	configEditCmd.Flags().StringVar(&configEditDir, "dir", "", "Config root or container directory (auto-detected)")
}

func runConfigEdit(cmd *cobra.Command, args []string) error {
	res, err := resolveEditDir(cmd.Context())
	if err != nil {
		return err
	}

	client := res.Contract()

	desc, err := client.Describe(cmd.Context())
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
