package cmd

import (
	"fmt"

	"github.com/gumieri/nenyactl/internal/install"
	"github.com/spf13/cobra"
)

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Show version information",
	Args:  cobra.NoArgs,
	RunE:  runVersion,
}

func init() {
	rootCmd.AddCommand(versionCmd)
}

func runVersion(cmd *cobra.Command, args []string) error {
	fmt.Println(infoStyle.Render("›"), "nenyactl v0.1.0")

	ctx := cmd.Context()
	latest, err := install.CheckLatestVersion(ctx)
	if err != nil {
		fmt.Println(dimStyle.Render("  Unable to check for nenya updates"))
		return nil
	}

	fmt.Println(dimStyle.Render("  Latest nenya release:"), latest)
	return nil
}
