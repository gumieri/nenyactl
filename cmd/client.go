package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/gumieri/nenyactl/internal/clients"
	"github.com/gumieri/nenyactl/internal/containers"
	"github.com/spf13/cobra"
)

var clientCmd = &cobra.Command{
	Use:   "client",
	Short: "Generate client configuration for the gateway",
	Long: `Generate ready-to-paste configuration for clients that speak the
OpenAI-compatible API: OpenCode, Cursor, Claude Code, and Aider.

The endpoint (published port) and token come from the resolved deployment,
so the output matches what is actually running.`,
}

var clientAddCmd = &cobra.Command{
	Use:   "add <name>",
	Short: "Print or write client configuration",
	Long: `Render client configuration from the resolved endpoint and token.

With --write, a JSON client config (opencode) is merged into its file,
preserving unrelated keys. Other clients print a snippet only.`,
	Args: cobra.ExactArgs(1),
	RunE: runClientAdd,
}

var (
	clientDir    string
	clientWrite  bool
	clientOutput string
)

func init() {
	rootCmd.AddCommand(clientCmd)
	clientCmd.AddCommand(clientAddCmd)
	f := clientAddCmd.Flags()
	f.StringVar(&clientDir, "dir", "", "Config root or container directory (default: system config root)")
	f.BoolVar(&clientWrite, "write", false, "Write the client config file instead of printing (JSON clients only)")
	f.StringVarP(&clientOutput, "output", "o", "", "Write the snippet to this file")
}

func runClientAdd(cmd *cobra.Command, args []string) error {
	name, err := clients.ParseName(args[0])
	if err != nil {
		return err
	}

	res, err := resolveDir(clientDir, false)
	if err != nil {
		return err
	}
	ep, err := resolvedEndpoint(res)
	if err != nil {
		return err
	}

	snippet, err := clients.Render(name, ep)
	if err != nil {
		return err
	}

	switch {
	case clientWrite:
		if snippet.MergeKey == "" {
			return fmt.Errorf("--write is only supported for JSON clients that nenyactl can merge (opencode); use -o to save the snippet")
		}
		return writeClientConfig(snippet, ep)
	case clientOutput != "":
		if err := os.WriteFile(clientOutput, []byte(snippet.Body+"\n"), 0o600); err != nil {
			return fmt.Errorf("write %s: %w", clientOutput, err)
		}
		fmt.Println(successStyle.Render("✓"), "Wrote", clientOutput)
		return nil
	default:
		fmt.Println(dimStyle.Render("  " + snippet.Description))
		fmt.Println()
		fmt.Println(snippet.Body)
		return nil
	}
}

// resolvedEndpoint reads the published port and client token from a resolved
// deployment. It never hardcodes 8080 or a specific secrets filename.
func resolvedEndpoint(res dirResolution) (clients.Endpoint, error) {
	base := "http://localhost:" + containers.PublishedPort(res.Path)

	token := containers.ClientToken(res.Path)
	if token == "" {
		token = readSecretsToken(res.Info.SecretsFile())
	}
	if token == "" {
		return clients.Endpoint{}, fmt.Errorf("no client token found for %s; create one with 'nenyactl secret bootstrap --dir %s'", res.Path, res.Path)
	}
	return clients.Endpoint{BaseURL: base, Token: token}, nil
}

func readSecretsToken(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	var s struct {
		ClientToken string `json:"client_token"`
	}
	if err := json.Unmarshal(data, &s); err != nil {
		return ""
	}
	return s.ClientToken
}

// writeClientConfig merges the snippet into the client's config file. It
// expands a leading ~ and preserves every unrelated key.
func writeClientConfig(snippet clients.Snippet, ep clients.Endpoint) error {
	path := expandHome(snippet.ConfigPath)
	if path == "" {
		return fmt.Errorf("client %s has no known config path", snippet.Name)
	}

	var existing []byte
	if data, err := os.ReadFile(path); err == nil {
		existing = data
	}

	var merged []byte
	var err error
	switch snippet.Name {
	case clients.OpenCode:
		merged, err = clients.MergeOpenCodeProvider(existing, ep)
	default:
		return fmt.Errorf("no merge writer for client %s", snippet.Name)
	}
	if err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}
	if err := os.WriteFile(path, merged, 0o644); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	fmt.Println(successStyle.Render("✓"), "Updated", path)
	return nil
}

func expandHome(path string) string {
	if path == "" {
		return ""
	}
	if strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return ""
		}
		return filepath.Join(home, path[2:])
	}
	return path
}

// printClientSnippets prints a short client-setup block with the resolved
// endpoint and token. Callers use it at the end of a successful install or
// `up` so the next action is copy-paste. It is best-effort: a missing token is
// reported, not fatal.
func printClientSnippets(res dirResolution) {
	ep, err := resolvedEndpoint(res)
	if err != nil {
		return
	}
	fmt.Println()
	fmt.Println(infoStyle.Render("›"), "Connect a client to", ep.BaseURL)
	fmt.Println(dimStyle.Render("   Authorization: Bearer " + ep.Token))
	for _, name := range clients.Supported() {
		snippet, rerr := clients.Render(name, ep)
		if rerr != nil {
			continue
		}
		fmt.Println()
		fmt.Println(dimStyle.Render("  " + string(name) + ": " + snippet.Description))
		fmt.Println(indent(snippet.Body, "    "))
	}
	fmt.Println()
	fmt.Println(dimStyle.Render("  Full config: nenyactl client add <opencode|cursor|claude|aider>"))
}

func indent(s, prefix string) string {
	lines := strings.Split(strings.TrimRight(s, "\n"), "\n")
	for i, l := range lines {
		lines[i] = prefix + l
	}
	return strings.Join(lines, "\n")
}
