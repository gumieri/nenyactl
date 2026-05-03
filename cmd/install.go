package cmd

import (
	"context"
	"fmt"

	"github.com/gumieri/nenyactl/internal/install"
	"github.com/gumieri/nenyactl/internal/paths"
	"github.com/spf13/cobra"
)

var installCmd = &cobra.Command{
	Use:   "install [version]",
	Short: "Download and install Nenya binary",
	Long: `Download the Nenya binary from GitHub releases.

For running Nenya as a service, use:
  nenyactl containers setup --start

The binary is installed to ` + paths.SystemBinDir() + ` (system)
or ~/.local/bin (user).`,
	Args: cobra.MaximumNArgs(1),
	RunE: runInstall,
}

var installUser bool

func init() {
	rootCmd.AddCommand(installCmd)
	installCmd.Flags().BoolVar(&installUser, "user", false, "Install to user bin dir instead of system-wide")
}

func runInstall(cmd *cobra.Command, args []string) error {
	ctx := context.Background()

	version := ""
	if len(args) > 0 {
		version = args[0]
	}

	cfg := install.Config{
		UserInstall: installUser,
		Version:     version,
	}

	if err := install.Install(ctx, cfg); err != nil {
		return fmt.Errorf("install: %w", err)
	}

	fmt.Println(successStyle.Render("✓"), "nenya installed successfully")
	fmt.Println(dimStyle.Render("  To run as a service: nenyactl containers setup --start"))

	return nil
}