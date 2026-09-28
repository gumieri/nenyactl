package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/gumieri/nenyactl/internal/containers"
	"github.com/gumieri/nenyactl/internal/detect"
	"github.com/gumieri/nenyactl/internal/jsonc"
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
func resolveLifecycleDir(ctx context.Context, dir string) (dirResolution, error) {
	if dir != "" {
		return resolveDir(ctx, dir, dirAttach, false)
	}
	info, err := detect.Detect()
	if err != nil {
		return dirResolution{}, err
	}
	return detectedResolution(info), nil
}

func runDown(cmd *cobra.Command, args []string) error {
	res, err := resolveLifecycleDir(cmd.Context(), downDir)
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
	res, err := resolveLifecycleDir(cmd.Context(), statusDir)
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
	pad := strings.Repeat(" ", max(1, 12-len(label)))
	fmt.Fprintf(&b, "%s:%s%s\n", label, pad, res.Path)

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
	// The secrets location is the contract's answer, not ours: the effective
	// source when describe resolved one (it names the winning file or
	// directory, e.g. the systemd credential), else the preferred file from
	// paths --json, else the resolved secrets dir.
	secretsValue := secretsDir
	if haveDesc {
		switch {
		case desc.Secrets.ActiveSource != "":
			secretsValue = desc.Secrets.ActiveSource
		case desc.Paths.SecretsFile != nil && *desc.Paths.SecretsFile != "":
			secretsValue = *desc.Paths.SecretsFile
		case desc.Paths.SecretsDir != "":
			secretsValue = desc.Paths.SecretsDir
		}
	}
	fmt.Fprintf(&b, "Config dir:  %s\n", configDir)
	fmt.Fprintf(&b, "Secrets:     %s\n", secretsValue)
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

// statusPort resolves the published/effective port from the deployment. For a
// container the compose published port is authoritative (describe reports the
// container-internal listen address, e.g. :8080, not the host mapping). For
// bare-metal it prefers the effective config from `nenya describe --json`; when
// that surface is unavailable (a released nenya without it) it falls back to the
// deployment's config file, mirroring nenya's directory layout. It returns ""
// when no port can be determined.
func statusPort(res dirResolution, desc nenya.Description, haveDesc bool) string {
	if res.Kind == dirContainerRoot {
		if p, ok := containers.HostPort(res.Path); ok && p != "" {
			return p
		}
	}
	if haveDesc && len(desc.Config) > 0 {
		if p := portFromEffective(desc.Config); p != "" {
			return p
		}
	}
	// Port resolution reads the deployment's config.json and config.d drop-ins
	// only as a shim for a released nenya without `describe --json` (NENYA-103).
	// On released nenya ≤0.15 a config.d drop-in makes nenya ignore config.json
	// (the XOR hazard, AGENTS.md §4), so drop-ins are preferred when present;
	// delete this reader once `describe` is stable everywhere.
	if addon := portFromConfigDropIns(res.Info.ConfigD); addon != "" {
		return addon
	}
	if p := portFromConfigFile(res.Info.ConfigFile); p != "" {
		return p
	}
	if res.Kind == dirContainerRoot {
		return containers.DefaultPort
	}
	return ""
}

// portFromEffective extracts the listen port from the effective config JSON
// (server.listen_addr).
func portFromEffective(effective []byte) string {
	var cfg struct {
		Server struct {
			ListenAddr string `json:"listen_addr"`
		} `json:"server"`
	}
	if err := json.Unmarshal(effective, &cfg); err != nil {
		return ""
	}
	return splitPort(cfg.Server.ListenAddr)
}

// portFromConfigFile reads server.listen_addr from a JSONC config file.
func portFromConfigFile(path string) string {
	if path == "" {
		return ""
	}
	v, err := jsonc.ReadFile(path)
	if err != nil {
		return ""
	}
	field, ok := jsonc.GetNestedField(v, []string{"server", "listen_addr"})
	if !ok {
		return ""
	}
	return splitPort(strings.Trim(jsonc.FieldValueString(field), `"`))
}

// portFromConfigDropIns returns the last listen port found in config.d, in
// ascending filename order (later file wins), skipping secrets.json.
func portFromConfigDropIns(dir string) string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return ""
	}
	var files []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".json") && e.Name() != "secrets.json" {
			files = append(files, filepath.Join(dir, e.Name()))
		}
	}
	sort.Strings(files)
	port := ""
	for _, f := range files {
		if p := portFromConfigFile(f); p != "" {
			port = p
		}
	}
	return port
}

// splitPort returns the port from a host:port address, or "" when it cannot be
// parsed.
func splitPort(addr string) string {
	if addr == "" {
		return ""
	}
	if _, port, err := net.SplitHostPort(addr); err == nil {
		return port
	}
	return ""
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
	if resp == nil {
		return false, "no response"
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode == http.StatusOK {
		return true, "healthy"
	}
	return false, resp.Status
}
