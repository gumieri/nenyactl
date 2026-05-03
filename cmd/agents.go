package cmd

import (
	"fmt"

	"github.com/gumieri/nenyactl/internal/agents"
	"github.com/gumieri/nenyactl/internal/paths"
	"github.com/spf13/cobra"
)

var agentsCmd = &cobra.Command{
	Use:   "agents",
	Short: "Configure Nenya agents",
	Long:  `Configure how Nenya routes requests to models.

Choose between auto-generated agents (recommended) or custom agent configuration.
`,
}

func init() {
	rootCmd.AddCommand(agentsCmd)
	agentsCmd.RunE = runAgents
}

func runAgents(cmd *cobra.Command, args []string) error {
	// Get the container directory (where setup created config/secrets)
	dir, err := paths.ContainerDir()
	if err != nil {
		return fmt.Errorf("cannot determine container directory: %w", err)
	}

	// Run the agent TUI
	useAuto, cfg, err := agents.RunAgentEditor()
	if err != nil {
		return err
	}

	if useAuto {
		// Just ensure discovery.auto_agents is true (it is by default in base config)
		if err := agents.UpdateConfigDiscovery(dir, true); err != nil {
			return fmt.Errorf("update discovery: %w", err)
		}
		fmt.Println(successStyle.Render("✓"), "Auto-agents enabled")
		return nil
	}

	if cfg == nil {
		// User cancelled or no changes
		return nil
	}

	// Write custom agents config
	if err := agents.WriteAgentsConfig(dir, cfg); err != nil {
		return fmt.Errorf("write agents config: %w", err)
	}
	fmt.Println(successStyle.Render("✓"), "Custom agents saved")

	// Ensure auto_agents is false
	if err := agents.UpdateConfigDiscovery(dir, false); err != nil {
		return fmt.Errorf("update discovery: %w", err)
	}
	fmt.Println(successStyle.Render("✓"), "Auto-agents disabled")

	return nil
}