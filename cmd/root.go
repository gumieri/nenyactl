package cmd

import (
	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "nenyactl",
	Short: "Manage Nenya AI Gateway",
	Long: `nenyactl installs and manages Nenya AI Gateway.

Supports bare-metal (systemd/launchd) and container (Podman/Docker) deployments.
Auto-detects the installation type for config editing.`,
}

func Execute() error {
	return rootCmd.Execute()
}
