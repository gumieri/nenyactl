package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/gumieri/nenyactl/internal/config"
	"github.com/gumieri/nenyactl/internal/detect"
	"github.com/gumieri/nenyactl/internal/install"
	"github.com/gumieri/nenyactl/internal/jsonc"
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
	var configFile string
	var configD string

	if configEditDir != "" {
		res, err := resolveDir(configEditDir, dirAttach, false)
		if err != nil {
			return err
		}
		configFile = res.Info.ConfigFile
		configD = res.Info.ConfigD
	} else {
		info, err := detect.Detect()
		if err != nil {
			return err
		}
		configFile = info.ConfigFile
		configD = info.ConfigD
	}

	if _, err := os.Stat(configFile); err != nil {
		return fmt.Errorf("config file not found: %s\n\n  Create with: nenyactl config init", configFile)
	}

	fmt.Println(infoStyle.Render("›"), "Editing:", configFile)

	result, changed, err := config.RunConfigEditor(configFile, configD)
	if err != nil {
		return fmt.Errorf("editor: %w", err)
	}

	if !changed {
		fmt.Println(infoStyle.Render("›"), "No changes made")
		return nil
	}

	if err := jsonc.WriteFile(result.ConfigFile, result.Config, 0o644); err != nil {
		return fmt.Errorf("save config: %w", err)
	}
	fmt.Println(successStyle.Render("✓"), "Config saved:", result.ConfigFile)

	if dirty, ok := result.Dirty[result.AgentsFile]; ok && dirty {
		if err := config.WriteAgentsFile(result.AgentsFile, result.Agents); err != nil {
			return fmt.Errorf("save agents: %w", err)
		}
		fmt.Println(successStyle.Render("✓"), "Agents saved:", result.AgentsFile)
	}

	return nil
}
