package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/gumieri/nenyactl/internal/install"
	"github.com/gumieri/nenyactl/internal/paths"
	"github.com/spf13/cobra"
)

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Manage Nenya configuration",
	Long:  `Bootstrap and manage the Nenya configuration directory.`,
}

func init() {
	rootCmd.AddCommand(configCmd)
	configCmd.AddCommand(configInitCmd)
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
	configInitCmd.Flags().StringVar(&configDir, "dir", paths.SystemConfigDir(), "Configuration directory")
}

func runConfigInit(cmd *cobra.Command, args []string) error {
	if err := bootstrapConfig(configDir); err != nil {
		return fmt.Errorf("config init: %w", err)
	}
	fmt.Println(successStyle.Render("✓"), "Config directory created:", configDir)
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
