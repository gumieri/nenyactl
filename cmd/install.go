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

The binary is installed to /usr/bin (system) or ~/.local/bin (user).
A full install also bootstraps config.json and secrets.json (mode 0600) when
missing, then loads and enables the service. Use --skip-service for a
binary-only install; --user bootstraps user config without system units.`,
	Args: cobra.MaximumNArgs(1),
	RunE: runInstall,
}

var (
	installUser       bool
	installSkipSvc    bool
	installSkipVerify bool
)

func init() {
	rootCmd.AddCommand(installCmd)
	installCmd.Flags().BoolVar(&installUser, "user", false, "Install to user bin dir instead of system-wide")
	installCmd.Flags().BoolVar(&installSkipSvc, "skip-service", false, "Install binary only, skip service configuration")
	installCmd.Flags().BoolVar(&installSkipVerify, "skip-verify", false, "Skip cosign signature verification (SHA-256 is still enforced)")
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
		SkipService: installSkipSvc,
		SkipVerify:  installSkipVerify,
	}

	if err := install.Install(ctx, cfg); err != nil {
		return fmt.Errorf("install: %w", err)
	}

	fmt.Println(successStyle.Render("✓"), "nenya installed successfully")
	return nil
}
