package cmd

import (
	"fmt"
	"os"

	"github.com/gumieri/nenyactl/internal/agents"
	"github.com/gumieri/nenyactl/internal/detect"
	"github.com/spf13/cobra"
)

var agentsCmd = &cobra.Command{
	Use:   "agents",
	Short: "Configure Nenya agents",
	Long: `Configure how Nenya routes requests to models.

Choose between auto-generated agents (recommended) or custom agent configuration.

Detects whether Nenya is installed as bare-metal or as a container.
Use --dir to override automatic detection.
`,
}

func init() {
	rootCmd.AddCommand(agentsCmd)
	agentsCmd.RunE = runAgents
	agentsCmd.Flags().StringVar(&agentsDir, "dir", "", "Configuration directory (skips auto-detection)")
}

var agentsDir string

func runAgents(cmd *cobra.Command, args []string) error {
	var info *detect.Info
	var err error

	if agentsDir != "" {
		if _, statErr := os.Stat(agentsDir); statErr != nil {
			return fmt.Errorf("--dir: %w", statErr)
		}
		info, err = detect.DetectFromDir(agentsDir, detect.ModeBareMetal)
		if err != nil {
			return err
		}
	} else {
		info, err = detect.Detect()
		if err != nil {
			return err
		}
	}

	useAuto, cfg, err := agents.RunAgentEditor()
	if err != nil {
		return err
	}

	if useAuto {
		if err := agents.UpdateConfigDiscovery(info.ConfigFile, true); err != nil {
			return fmt.Errorf("update discovery: %w", err)
		}
		fmt.Println(successStyle.Render("✓"), "Auto-agents enabled")
		return nil
	}

	if cfg == nil {
		return nil
	}

	if err := agents.WriteAgentsConfig(info.ConfigD, cfg); err != nil {
		return fmt.Errorf("write agents config: %w", err)
	}
	fmt.Println(successStyle.Render("✓"), "Custom agents saved")

	if err := agents.UpdateConfigDiscovery(info.ConfigFile, false); err != nil {
		return fmt.Errorf("update discovery: %w", err)
	}
	fmt.Println(successStyle.Render("✓"), "Auto-agents disabled")

	return nil
}
