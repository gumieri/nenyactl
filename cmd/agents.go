package cmd

import (
	"encoding/json"
	"fmt"

	"github.com/gumieri/nenyactl/internal/agents"
	"github.com/gumieri/nenyactl/internal/detect"
	"github.com/gumieri/nenyactl/internal/nenya"
	"github.com/spf13/cobra"
)

var agentsCmd = &cobra.Command{
	Use:   "agents",
	Short: "Configure Nenya agents",
	Long: `Configure how Nenya routes requests to models.

Choose between auto-generated agents (recommended) or custom agent configuration.
The model list comes from the installed nenya (` + "`nenya describe --json`" + `), so it
matches what the gateway actually supports; this command requires that contract
surface.

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
		res, err = resolveDir(cmd.Context(), agentsDir, dirAttach, false)
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

	desc, err := client.Describe(cmd.Context())
	if err != nil {
		return fmt.Errorf("agents requires `nenya describe --json` (nenya v0.16.0 or newer): %w", err)
	}
	catalog := agents.CatalogFromDescribe(catalogModels(desc.Providers.Catalog))
	if len(catalog.Models) == 0 {
		return fmt.Errorf("nenya reported no providers or models; configure a provider key first (nenyactl secret set --provider <name> <key>)")
	}

	useAuto, cfg, err := agents.RunAgentEditor(catalog)
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

	// Write the custom agents first, then disable auto-agents. The safer
	// partial state is custom agents present with auto still on (the custom
	// set is already in the config) rather than auto off with the previous
	// agents list. Each write reports its own path.
	path, err := client.SetConfig(cmd.Context(), "agents", string(agentsJSON))
	if err != nil {
		return fmt.Errorf("write agents: %w", err)
	}
	fmt.Println(successStyle.Render("✓"), "Custom agents saved →", path)

	discoveryPath, err := client.SetConfig(cmd.Context(), "discovery.auto_agents", "false")
	if err != nil {
		return fmt.Errorf("disable auto-agents: %w", err)
	}
	fmt.Println(successStyle.Render("✓"), "Auto-agents disabled →", discoveryPath)

	return nil
}

// catalogModels converts the describe provider catalog into the agents picker's
// CatalogModel list, so the picker stays independent of the contract client's
// types.
func catalogModels(catalog []nenya.ProviderCatalogEntry) []agents.CatalogModel {
	models := make([]agents.CatalogModel, 0, len(catalog))
	for _, e := range catalog {
		models = append(models, agents.CatalogModel{Provider: e.Provider, Model: e.Model})
	}
	return models
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
