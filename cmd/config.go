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
	"github.com/gumieri/nenyactl/internal/paths"
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
	res, err := resolveDir(configDir, dirCreate, false)
	if err != nil {
		return err
	}

	if err := bootstrapConfig(res.ConfigDir()); err != nil {
		return fmt.Errorf("config init: %w", err)
	}
	fmt.Println(successStyle.Render("✓"), "Config directory created:", res.ConfigDir())
	return nil
}

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

	content := install.BootstrapConfigContent(context.Background(), install.NewExecRunner(), "nenya")
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
	var res dirResolution
	if configEditDir != "" {
		var err error
		res, err = resolveDir(configEditDir, dirAttach, false)
		if err != nil {
			return err
		}
	} else {
		info, err := detect.Detect()
		if err != nil {
			return err
		}
		res = detectedResolution(info)
	}

	client := res.Contract()

	desc, err := client.Describe(cmd.Context())
	if err != nil {
		return fmt.Errorf("config edit requires a nenya release that ships the `describe --json` contract command: %w", err)
	}
	if len(desc.Config) == 0 {
		return fmt.Errorf("nenya describe returned no config for %s", res.ConfigDir())
	}

	fmt.Println(infoStyle.Render("›"), "Editing:", res.ConfigDir())

	result, changed, err := config.RunConfigEditor(desc.Config)
	if err != nil {
		return fmt.Errorf("editor: %w", err)
	}

	if !changed || len(result.Changes) == 0 {
		fmt.Println(infoStyle.Render("›"), "No changes made")
		return nil
	}

	// Apply every change through nenya's single writer. nenya chooses the
	// target file (managed drop-in or config file), merges, and writes
	// atomically, so nenyactl never reproduces the loader or the merge. Each
	// key is one call, so a mid-way failure is reported with what already
	// landed rather than pretending the edit was atomic.
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
