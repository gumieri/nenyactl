package cmd

import (
	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "nenyactl",
	Short: "Manage Nenya AI Gateway container deployments",
	Long: `nenyactl installs and manages Nenya AI Gateway using containers (Podman/Docker).

Handles container setup with interactive API key entry, configuration
bootstrapping, and secret generation.`,
}

func Execute() error {
	return rootCmd.Execute()
}