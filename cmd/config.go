package cmd

import (
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
	configInitCmd.Flags().StringVar(&configDir, "dir", "", "Configuration directory (default: auto-detect)")
}

func runConfigInit(cmd *cobra.Command, args []string) error {
	dir := configDir
	if dir == "" {
		info, err := detect.Detect()
		if err != nil {
			dir = paths.SystemConfigDir()
			fmt.Println(dimStyle.Render("  ⚠ auto-detection failed, using default:"), dir)
		} else {
			switch info.Mode {
			case detect.ModeBareMetal:
				dir = paths.SystemConfigDir()
			case detect.ModeContainer:
				dir = info.DataDir + "/config"
			}
		}
	}

	if err := bootstrapConfig(dir); err != nil {
		return fmt.Errorf("config init: %w", err)
	}
	fmt.Println(successStyle.Render("✓"), "Config directory created:", dir)
	return nil
}

func bootstrapConfig(dir string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create %s: %w", dir, err)
	}

	files := map[string]string{
		"config.json": install.ExampleConfig,
	}

	for name, content := range files {
		path := filepath.Join(dir, name)
		if _, err := os.Stat(path); err == nil {
			fmt.Println(dimStyle.Render("  ∃"), "Skipping existing", path)
			continue
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			return fmt.Errorf("write %s: %w", path, err)
		}
		fmt.Println(successStyle.Render("✓"), "Wrote", path)
	}

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
	configEditCmd.Flags().StringVar(&configEditDir, "dir", "", "Configuration directory (skips auto-detection)")
}

func runConfigEdit(cmd *cobra.Command, args []string) error {
	var configFile string

	if configEditDir != "" {
		if _, statErr := os.Stat(configEditDir); statErr != nil {
			return fmt.Errorf("--dir: %w", statErr)
		}
		configFile = configEditDir + "/config.json"
	} else {
		info, err := detect.Detect()
		if err != nil {
			return err
		}
		configFile = info.ConfigFile
	}

	if _, err := os.Stat(configFile); err != nil {
		return fmt.Errorf("config file not found: %s\n\n  Create with: nenyactl config init", configFile)
	}

	fmt.Println(infoStyle.Render("›"), "Editing:", configFile)

	edited, changed, err := config.RunConfigEditor(configFile)
	if err != nil {
		return fmt.Errorf("editor: %w", err)
	}

	if !changed {
		fmt.Println(infoStyle.Render("›"), "No changes made")
		return nil
	}

	if err := jsonc.WriteFile(configFile, edited, 0o644); err != nil {
		return fmt.Errorf("save config: %w", err)
	}

	fmt.Println(successStyle.Render("✓"), "Config saved:", configFile)
	return nil
}
