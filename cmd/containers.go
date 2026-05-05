package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/gumieri/nenyactl/internal/containers"
	ctpaths "github.com/gumieri/nenyactl/internal/paths"
	"github.com/gumieri/nenyactl/internal/secrets"
	"github.com/spf13/cobra"
)

// execer is the interface for running external commands.
type execer interface {
	Command(name string, args ...string) cmdRunner
}

// cmdRunner is the interface for a command that can be run.
type cmdRunner interface {
	Dir(string) cmdRunner
	Stdout(io.Writer) cmdRunner
	Stderr(io.Writer) cmdRunner
	Run() error
}

// realCmd wraps exec.Cmd to implement cmdRunner.
type realCmd struct{ *exec.Cmd }

func (r realCmd) Dir(dir string) cmdRunner {
	r.Cmd.Dir = dir
	return r
}

func (r realCmd) Stdout(w io.Writer) cmdRunner {
	r.Cmd.Stdout = w
	return r
}

func (r realCmd) Stderr(w io.Writer) cmdRunner {
	r.Cmd.Stderr = w
	return r
}

func (r realCmd) Run() error {
	return r.Cmd.Run()
}

// realExec implements execer using os/exec.
type realExec struct{}

func (realExec) Command(name string, args ...string) cmdRunner {
	return realCmd{Cmd: exec.Command(name, args...)}
}

var defaultExec execer = realExec{}

var containerCmd = &cobra.Command{
	Use:   "containers",
	Short: "Manage Nenya container deployment",
	Long:  `Create and manage Nenya container deployments using Podman or Docker.`,
}

func init() {
	rootCmd.AddCommand(containerCmd)
	containerCmd.AddCommand(containerSetupCmd)
	containerCmd.AddCommand(containerStartCmd)
	containerCmd.AddCommand(containerStopCmd)
	containerCmd.AddCommand(containerStatusCmd)
}

var containerSetupCfg struct {
	dir        string
	listenAddr string
	start      bool
}

var containerSetupCmd = &cobra.Command{
	Use:   "setup",
	Short: "Create a Nenya container deployment",
	Long: `Scaffold a Nenya container deployment.

Creates a directory with config, secrets, and compose.yml.
Prompts for provider API keys via TUI.

Default directory: ` + containerDefaultDir() + `
`,
	RunE: runContainerSetup,
}

func init() {
	f := containerSetupCmd.Flags()
	f.StringVar(&containerSetupCfg.dir, "dir", "", "Output directory (default: XDG_DATA_HOME/nenyactl/nenya)")
	f.StringVar(&containerSetupCfg.listenAddr, "listen", ":8080", "Listen address")
	f.BoolVar(&containerSetupCfg.start, "start", false, "Auto-start containers after setup")
}

func containerDefaultDir() string {
	if d, err := ctpaths.ContainerDir(); err == nil {
		return d
	}
	return "./nenya-data"
}

func runContainerSetup(cmd *cobra.Command, args []string) error {
	return runContainerSetupWithExec(defaultExec, containerSetupCfg.dir, containerSetupCfg.listenAddr, containerSetupCfg.start)
}

func runContainerSetupWithExec(ex execer, dir, listenAddr string, startAfter bool) error {
	if dir == "" {
		d, err := ctpaths.ContainerDir()
		if err != nil {
			return fmt.Errorf("cannot determine default directory: %w", err)
		}
		dir = d
	}

	fmt.Println(infoStyle.Render("›"), "Setting up Nenya in:", dir)

	if err := containers.Setup(containers.SetupConfig{
		ListenAddr: listenAddr,
		Dir:        dir,
	}); err != nil {
		return fmt.Errorf("setup: %w", err)
	}
	fmt.Println(successStyle.Render("✓"), "Created config and compose.yml")

	token := secrets.GenerateClientToken()
	clientPath := filepath.Join(dir, "secrets", "01-client.json")
	if err := os.WriteFile(clientPath, []byte(fmt.Sprintf(`{
  "client_token": "%s"
}`, token)), 0o600); err != nil {
		return fmt.Errorf("write client secrets: %w", err)
	}
	fmt.Println(successStyle.Render("✓"), "Generated client token")

	providersPath := filepath.Join(dir, "secrets", "02-providers.json")
	keys, err := containers.CollectProviderKeys()
	if err != nil {
		fmt.Println(dimStyle.Render("  TUI skipped or cancelled"))
		keys = nil
	}
	if len(keys) > 0 {
		content := "{\n  \"provider_keys\": {\n"
		i := 0
		for k, v := range keys {
			content += fmt.Sprintf(`    "%s": "%s"`, k, v)
			i++
			if i < len(keys) {
				content += ","
			}
			content += "\n"
		}
		content += "  }\n}\n"
		if err := os.WriteFile(providersPath, []byte(content), 0o600); err != nil {
			return fmt.Errorf("write providers secrets: %w", err)
		}
		fmt.Println(successStyle.Render("✓"), "Saved", len(keys), "provider keys")
	}

	if startAfter {
		return runContainerStartWithExec(ex, dir)
	}

	fmt.Println()
	fmt.Println(dimStyle.Render("  Next steps:"))
	fmt.Println(dimStyle.Render("  cd " + dir))
	fmt.Println(dimStyle.Render("  podman compose up -d  # or: docker compose up -d"))
	fmt.Println(dimStyle.Render("  nenyactl containers status --dir " + dir))
	return nil
}

var containerStartCmd = &cobra.Command{
	Use:   "start",
	Short: "Start Nenya containers",
	Long:  `Run podman/docker compose up -d in the container directory.`,
	RunE:  runContainerStart,
}

func init() {
	f := containerStartCmd.Flags()
	f.StringVar(&containerSetupCfg.dir, "dir", "", "Container directory")
}

func runContainerStart(cmd *cobra.Command, args []string) error {
	return runContainerStartWithExec(defaultExec, containerSetupCfg.dir)
}

func runContainerStartWithExec(ex execer, dir string) error {
	if dir == "" {
		d, err := ctpaths.ContainerDir()
		if err != nil {
			return fmt.Errorf("cannot determine default directory: %w", err)
		}
		dir = d
	}

	runtime, composeArgs, _ := ctpaths.ComposeCmd()
	composeArgs = append(composeArgs, "up", "-d")

	fmt.Printf("%s Running: %s compose %v\n", infoStyle.Render("›"), runtime, composeArgs)

	c := ex.Command(string(runtime), composeArgs...).Dir(dir).Stdout(os.Stdout).Stderr(os.Stderr)
	return c.Run()
}

var containerStopCmd = &cobra.Command{
	Use:   "stop",
	Short: "Stop Nenya containers",
	Long:  `Run podman/docker compose down in the container directory.`,
	RunE:  runContainerStop,
}

func init() {
	f := containerStopCmd.Flags()
	f.StringVar(&containerSetupCfg.dir, "dir", "", "Container directory")
}

func runContainerStop(cmd *cobra.Command, _ []string) error {
	return runContainerStopWithExec(defaultExec, containerSetupCfg.dir)
}

func runContainerStopWithExec(ex execer, dir string) error {
	if dir == "" {
		d, err := ctpaths.ContainerDir()
		if err != nil {
			return fmt.Errorf("cannot determine default directory: %w", err)
		}
		dir = d
	}

	runtime, composeArgs, _ := ctpaths.ComposeCmd()
	composeArgs = append(composeArgs, "down")

	fmt.Printf("%s Running: %s compose %v\n", infoStyle.Render("›"), runtime, composeArgs)

	c := ex.Command(string(runtime), composeArgs...).Dir(dir).Stdout(os.Stdout).Stderr(os.Stderr)
	return c.Run()
}

var containerStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show container status and health",
	Long:  `Show podman/docker compose ps and health check.`,
	RunE:  runContainerStatus,
}

func init() {
	f := containerStatusCmd.Flags()
	f.StringVar(&containerSetupCfg.dir, "dir", "", "Container directory")
}

func runContainerStatus(cmd *cobra.Command, _ []string) error {
	return runContainerStatusWithExec(defaultExec, containerSetupCfg.dir)
}

func runContainerStatusWithExec(ex execer, dir string) error {
	if dir == "" {
		d, err := ctpaths.ContainerDir()
		if err != nil {
			return fmt.Errorf("cannot determine default directory: %w", err)
		}
		dir = d
	}

	runtime, composeArgs, _ := ctpaths.ComposeCmd()
	composeArgs = append(composeArgs, "ps")

	fmt.Printf("%s Running: %s compose %v\n", infoStyle.Render("›"), runtime, composeArgs)
	c := ex.Command(string(runtime), composeArgs...).Dir(dir).Stdout(os.Stdout).Stderr(os.Stderr)
	if err := c.Run(); err != nil {
		return err
	}

	fmt.Println()
	fmt.Println(infoStyle.Render("›"), "Health check:")
	resp, err := http.Get("http://localhost:8080/healthz")
	if err != nil {
		fmt.Println(errorStyle.Render("✗"), "Connection failed:", err)
		return nil
	}
	defer resp.Body.Close()
	if resp.StatusCode == 200 {
		fmt.Println(successStyle.Render("✓"), "Nenya is healthy")
	} else {
		fmt.Println(errorStyle.Render("✗"), "Status:", resp.Status)
	}

	clientPath := filepath.Join(dir, "secrets", "01-client.json")
	data, err := os.ReadFile(clientPath)
	if err == nil {
		var secret struct {
			ClientToken string `json:"client_token"`
		}
		if json.Unmarshal(data, &secret) == nil && secret.ClientToken != "" {
			fmt.Println()
			fmt.Println(dimStyle.Render("  Client token:"))
			fmt.Println(successStyle.Render("  Authorization: Bearer ") + secret.ClientToken)
		}
	}

	return nil
}