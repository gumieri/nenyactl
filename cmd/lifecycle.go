package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/gumieri/nenyactl/internal/containers"
	"github.com/gumieri/nenyactl/internal/detect"
	"github.com/gumieri/nenyactl/internal/nenya"
	"github.com/spf13/cobra"
)

var downCmd = &cobra.Command{
	Use:   "down",
	Short: "Stop the resolved Nenya deployment",
	Long: `Stop the Nenya deployment nenyactl resolves for this machine.

Bare-metal stops the nenya service (systemd/launchd); a container
deployment runs the compose down in its directory. Use --dir to point at a
specific deployment root instead of auto-detecting.`,
	RunE: runDown,
}

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show the resolved Nenya deployment status",
	Long: `Print the resolved mode, paths, contract version, health, and the
configured agents and models for the deployment nenyactl resolves for this
machine. Port and token come from the resolved deployment, never hardcoded.`,
	RunE: runStatus,
}

var (
	downDir   string
	statusDir string
)

func init() {
	rootCmd.AddCommand(downCmd)
	rootCmd.AddCommand(statusCmd)
	downCmd.Flags().StringVar(&downDir, "dir", "", "Deployment root (default: auto-detect)")
	statusCmd.Flags().StringVar(&statusDir, "dir", "", "Deployment root (default: auto-detect)")
}

// resolveLifecycleDir resolves the deployment for a lifecycle command: an
// explicit --dir wins, otherwise the machine's installation is auto-detected.
func resolveLifecycleDir(dir string) (dirResolution, error) {
	if dir != "" {
		return resolveDir(dir, dirAttach, false)
	}
	info, err := detect.Detect()
	if err != nil {
		return dirResolution{}, err
	}
	return detectedResolution(info), nil
}

func runDown(cmd *cobra.Command, args []string) error {
	res, err := resolveLifecycleDir(downDir)
	if err != nil {
		return err
	}
	if res.Kind == dirContainerRoot {
		fmt.Println(infoStyle.Render("›"), "Stopping container deployment at", res.Path)
		return runContainerStopWithExec(defaultExec, res.Path)
	}
	fmt.Println(infoStyle.Render("›"), "Stopping Nenya service")
	return runServiceStopWithExec(defaultExec)
}

// healthDoer is the minimal HTTP surface status needs, so health checks are
// testable without a live gateway.
type healthDoer interface {
	Do(*http.Request) (*http.Response, error)
}

var statusHTTPClient healthDoer = &http.Client{Timeout: 3 * time.Second}

func runStatus(cmd *cobra.Command, args []string) error {
	res, err := resolveLifecycleDir(statusDir)
	if err != nil {
		return err
	}
	fmt.Print(buildStatus(cmd.Context(), res, statusHTTPClient))
	return nil
}

// statusAgents is the subset of the effective config status reports.
type statusAgents struct {
	Discovery struct {
		AutoAgents bool `json:"auto_agents"`
	} `json:"discovery"`
	Agents map[string]struct {
		Strategy string   `json:"strategy"`
		Models   []string `json:"models"`
	} `json:"agents"`
}

// buildStatus renders the deployment report. It is separated from the command
// so it can be tested with a fake contract and health doer.
func buildStatus(ctx context.Context, res dirResolution, doer healthDoer) string {
	var b strings.Builder

	mode := "bare-metal"
	if res.Kind == dirContainerRoot {
		mode = "container"
	}
	fmt.Fprintf(&b, "Mode:        %s\n", mode)
	fmt.Fprintf(&b, "Deployment:  %s\n", res.Path)
	fmt.Fprintf(&b, "Config dir:  %s\n", res.ConfigDir())
	fmt.Fprintf(&b, "Secrets dir: %s\n", res.Info.SecretsDir())

	var desc nenya.Description
	haveDesc := false
	if d, err := res.Contract().Describe(ctx); err != nil {
		fmt.Fprintf(&b, "Contract:    unavailable (%v)\n", err)
	} else {
		desc, haveDesc = d, true
		fmt.Fprintf(&b, "Contract:    v%d\n", d.ContractVersion)
		if d.Version.Version != "" {
			fmt.Fprintf(&b, "Nenya:       %s\n", d.Version.Version)
		}
		if len(d.Providers.Configured) > 0 {
			fmt.Fprintf(&b, "Providers:   %s\n", strings.Join(d.Providers.Configured, ", "))
		}
		if len(d.Diagnostics) > 0 {
			fmt.Fprintf(&b, "Diagnostics: %d (run `nenyactl config edit` to review)\n", len(d.Diagnostics))
		}
	}

	port := statusPort(res, desc, haveDesc)
	fmt.Fprintf(&b, "Port:        %s\n", port)
	fmt.Fprintf(&b, "Health:      %s\n", healthStatus(ctx, doer, port))

	if haveDesc {
		writeStatusAgents(&b, desc.Config)
	}
	return b.String()
}

// writeStatusAgents lists the configured agents and their models from the
// effective config.
func writeStatusAgents(b *strings.Builder, effective []byte) {
	if len(effective) == 0 {
		return
	}
	var cfg statusAgents
	if err := json.Unmarshal(effective, &cfg); err != nil {
		return
	}
	fmt.Fprintf(b, "Auto-agents: %t\n", cfg.Discovery.AutoAgents)
	if len(cfg.Agents) == 0 {
		return
	}
	names := make([]string, 0, len(cfg.Agents))
	for name := range cfg.Agents {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		a := cfg.Agents[name]
		models := strings.Join(a.Models, ", ")
		if models == "" {
			models = "(no models)"
		}
		fmt.Fprintf(b, "Agent %-12s %s: %s\n", name, a.Strategy, models)
	}
}

// statusPort resolves the published/effective port from the deployment, never a
// hardcoded default.
func statusPort(res dirResolution, desc nenya.Description, haveDesc bool) string {
	if res.Kind == dirContainerRoot {
		if p := containers.PublishedPort(res.Path); p != "" {
			return p
		}
	}
	if haveDesc && len(desc.Config) > 0 {
		var cfg struct {
			Server struct {
				ListenAddr string `json:"listen_addr"`
			} `json:"server"`
		}
		if err := json.Unmarshal(desc.Config, &cfg); err == nil {
			if _, port, err := net.SplitHostPort(cfg.Server.ListenAddr); err == nil && port != "" {
				return port
			}
		}
	}
	if p := listenPort(res.Info.ConfigFile); p != "" {
		return p
	}
	return containers.DefaultPort
}

func healthStatus(ctx context.Context, doer healthDoer, port string) string {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://localhost:"+port+"/healthz", nil)
	if err != nil {
		return "unknown"
	}
	resp, err := doer.Do(req)
	if err != nil {
		return "unreachable (" + err.Error() + ")"
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode == http.StatusOK {
		return "healthy"
	}
	return resp.Status
}
