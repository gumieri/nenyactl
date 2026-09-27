package cmd

import (
	"encoding/json"
	"fmt"

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
	agentsCmd.Flags().StringVar(&agentsDir, "dir", "", "Config root or container directory (auto-detected; skips installation detection)")
	agentsCmd.Flags().StringVar(&agentsMode, "mode", "", "Deployment mode for --dir: bare-metal or container (default: auto-detect)")
}

var (
	agentsDir  string
	agentsMode string
)

func runAgents(cmd *cobra.Command, args []string) error {
	if agentsMode != "" && agentsDir == "" {
		return fmt.Errorf("--mode requires --dir")
	}

	var res dirResolution
	if agentsDir != "" {
		var err error
		res, err = resolveDir(agentsDir, dirAttach, false)
		if err != nil {
			return err
		}
		if agentsMode != "" {
			mode, modeErr := parseAgentsMode(agentsMode)
			if modeErr != nil {
				return modeErr
			}
			info, detectErr := detect.DetectFromDir(agentsDir, mode)
			if detectErr != nil {
				return detectErr
			}
			res = detectedResolution(info)
		}
	} else {
		info, err := detect.Detect()
		if err != nil {
			return err
		}
		res = detectedResolution(info)
	}

	client := res.Contract()

	useAuto, cfg, err := agents.RunAgentEditor()
	if err != nil {
		return err
	}

	if useAuto {
		path, err := client.SetConfig(cmd.Context(), "discovery.auto_agents", "true")
		if err != nil {
			return fmt.Errorf("enable auto-agents: %w", err)
		}
		fmt.Println(successStyle.Render("✓"), "Auto-agents enabled →", path)
		return nil
	}

	if cfg == nil {
		return nil
	}

	// An absent or nil agents value would marshal to null and wipe the
	// configured agents, so refuse rather than send `config set agents null`.
	agentsCfg, ok := cfg["agents"]
	if !ok || agentsCfg == nil {
		return fmt.Errorf("no agents configured to save")
	}
	agentsJSON, err := json.Marshal(agentsCfg)
	if err != nil {
		return fmt.Errorf("encode agents: %w", err)
	}
	path, err := client.SetConfig(cmd.Context(), "agents", string(agentsJSON))
	if err != nil {
		return fmt.Errorf("write agents: %w", err)
	}
	fmt.Println(successStyle.Render("✓"), "Custom agents saved →", path)

	if path, err := client.SetConfig(cmd.Context(), "discovery.auto_agents", "false"); err != nil {
		return fmt.Errorf("disable auto-agents: %w", err)
	} else {
		fmt.Println(successStyle.Render("✓"), "Auto-agents disabled →", path)
	}

	return nil
}

// parseAgentsMode maps an explicit --mode value to a detect.Mode.
func parseAgentsMode(mode string) (detect.Mode, error) {
	switch mode {
	case "bare-metal":
		return detect.ModeBareMetal, nil
	case "container":
		return detect.ModeContainer, nil
	default:
		return detect.ModeNone, fmt.Errorf("--mode: invalid value %q (use bare-metal or container)", mode)
	}
}
