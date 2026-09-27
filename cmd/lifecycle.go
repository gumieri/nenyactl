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
	Args: cobra.NoArgs,
	RunE: runDown,
}

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show the resolved Nenya deployment status",
	Long: `Print the resolved mode, paths, contract version, health, and the
configured agents and models for the deployment nenyactl resolves for this
machine. Port and token come from the resolved deployment, never hardcoded.`,
	Args: cobra.NoArgs,
	RunE: runStatus,
}

var (
	downDir   string
	statusDir string
	// downServiceRun is a seam so down can be tested without stopping the host.
	downServiceRun = func() error { return runServiceStopWithExec(defaultExec) }
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
	return downServiceRun()
}

// healthDoer is the minimal HTTP surface lifecycle needs, so health checks are
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
	label := "Config root"
	if res.Kind == dirContainerRoot {
		label = "Deployment"
	}
	fmt.Fprintf(&b, "Mode:        %s\n", mode)
	fmt.Fprintf(&b, "%s:%s%s\n", label, strings.Repeat(" ", max(1, 12-len(label))), res.Path)

	// nenya's resolved paths are authoritative once describe succeeds.
	configDir := res.ConfigDir()
	secretsDir := res.Info.SecretsDir()
	var desc nenya.Description
	haveDesc := false
	if d, err := res.Contract().Describe(ctx); err != nil {
		fmt.Fprintf(&b, "Contract:    unavailable (%v)\n", err)
	} else {
		desc, haveDesc = d, true
		if d.Paths.ConfigDir != "" {
			configDir = d.Paths.ConfigDir
		}
		if d.Paths.SecretsDir != "" {
			secretsDir = d.Paths.SecretsDir
		}
		fmt.Fprintf(&b, "Contract:    v%d\n", d.ContractVersion)
		if d.Version.Version != "" {
			fmt.Fprintf(&b, "Nenya:       %s\n", d.Version.Version)
		}
		if len(d.Providers.Configured) > 0 {
			fmt.Fprintf(&b, "Providers:   %s\n", strings.Join(d.Providers.Configured, ", "))
		}
		if len(d.Diagnostics) > 0 {
			fmt.Fprintf(&b, "Diagnostics: %d (run `nenyactl doctor`)\n", len(d.Diagnostics))
		}
	}
	fmt.Fprintf(&b, "Config dir:  %s\n", configDir)
	fmt.Fprintf(&b, "Secrets dir: %s\n", secretsDir)

	port := statusPort(res, desc, haveDesc)
	if port == "" {
		fmt.Fprintf(&b, "Port:        unknown\n")
	} else {
		fmt.Fprintf(&b, "Port:        %s\n", port)
	}
	if ok, detail := healthStatus(ctx, doer, port); ok {
		fmt.Fprintf(&b, "Health:      healthy\n")
	} else {
		fmt.Fprintf(&b, "Health:      %s\n", detail)
	}

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
		fmt.Fprintln(b, "Agents:      present but unparseable")
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
// hardcoded default. It returns "" when no port can be determined.
func statusPort(res dirResolution, desc nenya.Description, haveDesc bool) string {
	if res.Kind == dirContainerRoot {
		if p, ok := containers.HostPort(res.Path); ok && p != "" {
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
	return listenPort(res.Info.ConfigFile)
}

// healthStatus polls /healthz and reports whether the gateway is healthy along
// with a human-readable detail.
func healthStatus(ctx context.Context, doer healthDoer, port string) (bool, string) {
	if port == "" {
		return false, "no port resolved"
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://localhost:"+port+"/healthz", nil)
	if err != nil {
		return false, "invalid request"
	}
	resp, err := doer.Do(req)
	if err != nil {
		return false, "unreachable (" + err.Error() + ")"
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode == http.StatusOK {
		return true, "healthy"
	}
	return false, resp.Status
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
