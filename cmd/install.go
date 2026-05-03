package cmd

import (
	"context"
	"fmt"

	"github.com/gumieri/nenyactl/internal/install"
	"github.com/spf13/cobra"
)

var installCmd = &cobra.Command{
	Use:   "install [version]",
	Short: "Download and install Nenya binary and service",
	Long: `Download and install the Nenya binary and configure it as a service.

Linux:   Installs systemd units (nenya.service + nenya.socket)
macOS:   Installs launchd plist
Windows: Not supported (use 'nenyactl containers setup' instead)

The binary is installed to /usr/local/bin (system) or ~/.local/bin (user).`,
	Args: cobra.MaximumNArgs(1),
	RunE: runInstall,
}

var (
	installUser      bool
	installSkipSvc  bool
)

func init() {
	rootCmd.AddCommand(installCmd)
	installCmd.Flags().BoolVar(&installUser, "user", false, "Install to user bin dir instead of system-wide")
	installCmd.Flags().BoolVar(&installSkipSvc, "skip-service", false, "Install binary only, skip service configuration")
}

func runInstall(cmd *cobra.Command, args []string) error {
	ctx := context.Background()

	version := ""
	if len(args) > 0 {
		version = args[0]
	}

	cfg := install.Config{
		UserInstall:  installUser,
		Version:      version,
		SkipService:  installSkipSvc,
	}

	if err := install.Install(ctx, cfg); err != nil {
		return fmt.Errorf("install: %w", err)
	}

	fmt.Println(successStyle.Render("✓"), "nenya installed successfully")
	return nil
}
