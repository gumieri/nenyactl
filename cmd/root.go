package cmd

import (
	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "nenyactl",
	Short: "Manage Nenya AI Gateway",
	Long: `nenyactl installs and manages Nenya AI Gateway.

The primary flow is linear and mode-agnostic:

  nenyactl up        bring Nenya up from any state (installs if missing)
  nenyactl status    show the resolved deployment, contract, and health
  nenyactl doctor    diagnose the seam and suggest fixes
  nenyactl down      stop the resolved deployment

nenyactl chooses bare-metal (systemd/launchd) or container (Podman/Docker)
internally; the service, containers, and install commands remain for power
users and advanced layouts.`,
}

func Execute() error {
	return rootCmd.Execute()
}
