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

	warnAgentsDropIn(info)
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

// warnAgentsDropIn warns that writing config.d/20-agents.json makes nenya
// ignore an existing config.json entirely (CONTRACT.md §5.1; the XOR is tracked
// in NCTL "Fix config.d precedence inverted"). It does not change the write
// strategy, which belongs to that workstream.
func warnAgentsDropIn(info *detect.Info) {
	if info.ConfigFile == "" || info.ConfigD == "" {
		return
	}
	if _, err := os.Stat(info.ConfigFile); err != nil {
		return
	}
	if entries, err := os.ReadDir(info.ConfigD); err == nil && len(entries) > 0 {
		return // config.d is already the active layout
	}
	fmt.Fprintln(os.Stderr, "Warning: writing", filepath.Join(info.ConfigD, "20-agents.json"),
		"will make nenya ignore the existing", info.ConfigFile)
	fmt.Fprintln(os.Stderr, "until the config.d precedence issue is fixed. Back up", info.ConfigFile, "first.")
}
