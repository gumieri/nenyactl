package cmd

import (
	"fmt"
	"os"
	"path/filepath"

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

	var info *detect.Info
	if agentsDir != "" {
		res, err := resolveDir(agentsDir, dirAttach, false)
		if err != nil {
			return err
		}
		info = res.Info
		if agentsMode != "" {
			mode, modeErr := parseAgentsMode(agentsMode)
			if modeErr != nil {
				return modeErr
			}
			info, err = detect.DetectFromDir(agentsDir, mode)
			if err != nil {
				return err
			}
		}
	} else {
		var err error
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

	if dropInActive(info) {
		if err := agents.WriteAgentsConfig(info.ConfigD, cfg); err != nil {
			return fmt.Errorf("write agents config: %w", err)
		}
		fmt.Println(successStyle.Render("✓"), "Custom agents saved to", info.ConfigD)
	} else {
		// config.d is not the active layout: merging into config.json keeps
		// the user's config readable on every nenya version (CONTRACT.md §5.1).
		if err := agents.WriteAgentsIntoConfig(info.ConfigFile, cfg); err != nil {
			return fmt.Errorf("write agents into config: %w", err)
		}
		fmt.Println(successStyle.Render("✓"), "Custom agents merged into", info.ConfigFile)
	}

	if err := agents.UpdateConfigDiscovery(info.ConfigFile, false); err != nil {
		return fmt.Errorf("update discovery: %w", err)
	}
	fmt.Println(successStyle.Render("✓"), "Auto-agents disabled")

	return nil
}

// dropInActive reports whether config.d is already the active layout, i.e. it
// contains at least one *.json (excluding secrets.json). Writing a drop-in is
// only safe in that case; otherwise it would make nenya ignore config.json on
// released <=0.15 builds.
func dropInActive(info *detect.Info) bool {
	if info.ConfigD == "" {
		return false
	}
	entries, err := os.ReadDir(info.ConfigD)
	if err != nil {
		return false
	}
	for _, e := range entries {
		if !e.IsDir() && filepath.Ext(e.Name()) == ".json" && e.Name() != "secrets.json" {
			return true
		}
	}
	return false
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
