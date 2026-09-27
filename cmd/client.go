package cmd

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"

	"github.com/gumieri/nenyactl/internal/clients"
	"github.com/gumieri/nenyactl/internal/containers"
	"github.com/gumieri/nenyactl/internal/jsonc"
	"github.com/gumieri/nenyactl/internal/paths"
	"github.com/spf13/cobra"
)

var clientCmd = &cobra.Command{
	Use:   "client",
	Short: "Generate client configuration for the gateway",
	Long: `Generate ready-to-paste configuration for clients that speak the
OpenAI-compatible API: OpenCode, Cursor, Claude Code, and Aider.

The endpoint (published port) and token come from the resolved deployment,
so the output matches what is actually running. Tokens are only printed when
you ask for them.`,
}

var clientAddCmd = &cobra.Command{
	Use:   "add <name>",
	Short: "Print or write client configuration",
	Long: `Render client configuration from the resolved endpoint and token.

By default the snippet is printed with the token redacted. Pass
--show-token to include the real token. With --write, a JSON client config
(opencode) is merged into its file, preserving unrelated keys; other
clients print a snippet only.`,
	Args: cobra.ExactArgs(1),
	RunE: runClientAdd,
}

var (
	clientDir       string
	clientWrite     bool
	clientOutput    string
	clientShowToken bool
)

func init() {
	rootCmd.AddCommand(clientCmd)
	clientCmd.AddCommand(clientAddCmd)
	f := clientAddCmd.Flags()
	f.StringVar(&clientDir, "dir", "", "Config root or container directory (default: system config root)")
	f.BoolVar(&clientWrite, "write", false, "Write the client config file instead of printing (JSON clients only)")
	f.StringVarP(&clientOutput, "output", "o", "", "Write the snippet to this file (mode 0600)")
	f.BoolVar(&clientShowToken, "show-token", false, "Include the real token in printed output")
	clientAddCmd.MarkFlagsMutuallyExclusive("write", "output")
}

func runClientAdd(cmd *cobra.Command, args []string) error {
	name, err := clients.ParseName(args[0])
	if err != nil {
		return err
	}
	if clientShowToken && (clientWrite || clientOutput != "") {
		return fmt.Errorf("--show-token only applies to printed output")
	}

	res, err := resolveDir(clientDir, dirAttach, false)
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
		if !snippet.Mergeable {
			return fmt.Errorf("--write is only supported for JSON clients that nenyactl can merge (opencode); use -o to save the snippet")
		}
		return writeClientConfig(snippet, ep)
	case clientOutput != "":
		if err := os.MkdirAll(filepath.Dir(clientOutput), 0o755); err != nil {
			return fmt.Errorf("create output dir: %w", err)
		}
		if err := os.WriteFile(clientOutput, []byte(snippet.Body+"\n"), 0o600); err != nil {
			return fmt.Errorf("write %s: %w", clientOutput, err)
		}
		if err := os.Chmod(clientOutput, 0o600); err != nil {
			return fmt.Errorf("chmod %s: %w", clientOutput, err)
		}
		fmt.Println(successStyle.Render("✓"), "Wrote", clientOutput)
		return nil
	default:
		body := snippet.Body
		if !clientShowToken {
			body = redactToken(body, ep.Token)
		}
		fmt.Println(dimStyle.Render("  " + snippet.Description))
		fmt.Println()
		fmt.Println(body)
		if !clientShowToken {
			fmt.Println()
			fmt.Println(dimStyle.Render("  Token redacted; pass --show-token to print it."))
		}
		return nil
	}
}

// resolvedEndpoint reads the published port and client token from a resolved
// deployment. It never hardcodes a token location; for bare-metal it reads the
// effective listen address from the config file when present.
func resolvedEndpoint(res dirResolution) (clients.Endpoint, error) {
	port := containers.PublishedPort(res.Path)
	if p := listenPort(res.Info.ConfigFile); p != "" {
		port = p
	}
	base := "http://localhost:" + port

	token := containers.ClientToken(res.Path)
	if token == "" {
		token = readSecretsToken(res.Info.SecretsFile())
	}
	if token == "" {
		return clients.Endpoint{}, fmt.Errorf("no client token found for %s; create one with 'nenyactl secret bootstrap --dir %s'", res.Path, res.Path)
	}
	return clients.Endpoint{BaseURL: base, Token: token}, nil
}

// listenPort extracts the port from server.listen_addr in a JSONC config file.
// It returns "" when the file or field is absent/unparseable.
func listenPort(configFile string) string {
	v, err := jsonc.ReadFile(configFile)
	if err != nil {
		return ""
	}
	field, ok := jsonc.GetNestedField(v, []string{"server", "listen_addr"})
	if !ok {
		return ""
	}
	addr := jsonc.FieldValueString(field)
	addr = strings.Trim(addr, `"`)
	_, port, err := net.SplitHostPort(addr)
	if err != nil {
		return ""
	}
	return port
}

func readSecretsToken(path string) string {
	if path == "" {
		return ""
	}
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

// redactToken replaces the token with a placeholder in printed output.
func redactToken(body, token string) string {
	if token == "" {
		return body
	}
	return strings.ReplaceAll(body, token, "<client-token>")
}

// writeClientConfig merges the snippet into the client's config file. It
// expands a leading ~ and preserves every unrelated key. The file holds a
// credential, so it is always written mode 0600.
func writeClientConfig(snippet clients.Snippet, ep clients.Endpoint) error {
	path, err := expandHome(snippet.ConfigPath)
	if err != nil {
		return err
	}
	if path == "" {
		return fmt.Errorf("client %s has no known config path", snippet.Name)
	}

	var existing []byte
	if data, readErr := os.ReadFile(path); readErr == nil {
		existing = data
	} else if !os.IsNotExist(readErr) {
		return fmt.Errorf("read %s: %w", path, readErr)
	}

	var merged []byte
	switch snippet.Name {
	case clients.OpenCode:
		merged, err = clients.MergeOpenCodeProvider(existing, ep)
	default:
		return fmt.Errorf("no merge writer for client %s", snippet.Name)
	}
	if err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}
	if err := os.WriteFile(path, merged, 0o600); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	if err := os.Chmod(path, 0o600); err != nil {
		return fmt.Errorf("chmod %s: %w", path, err)
	}
	fmt.Println(successStyle.Render("✓"), "Updated", path, "(mode 0600)")
	return nil
}

// expandHome resolves a leading ~ using the user's home directory. It returns
// an error when the home directory cannot be determined.
func expandHome(path string) (string, error) {
	if path == "" {
		return "", nil
	}
	if path == "~" {
		return os.UserHomeDir()
	}
	if strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("resolve home dir: %w", err)
		}
		return filepath.Join(home, path[2:]), nil
	}
	return path, nil
}

// printClientSnippets prints a short connect block with the resolved endpoint
// and a redacted token, so the next action is copy-paste. It is best-effort and
// never prints the token itself.
func printClientSnippets(res dirResolution) {
	ep, err := resolvedEndpoint(res)
	if err != nil {
		return
	}
	fmt.Println()
	fmt.Println(infoStyle.Render("›"), "Connect a client to", ep.BaseURL)
	for _, name := range clients.Supported() {
		snippet, rerr := clients.Render(name, ep)
		if rerr != nil {
			continue
		}
		fmt.Println(dimStyle.Render("  " + string(name) + ": " + snippet.Description))
	}
	fmt.Println()
	fmt.Println(dimStyle.Render("  Full config: nenyactl client add <opencode|cursor|claude|aider> --show-token"))
}

// installResolution resolves the deployment created by an install so callers
// can print the connect block for the right root (--user vs system).
func installResolution(userInstall bool, systemConfigDir string) dirResolution {
	if userInstall {
		if dir, err := paths.UserConfigDir(); err == nil {
			return configRoot(dir)
		}
	}
	return configRoot(systemConfigDir)
}
