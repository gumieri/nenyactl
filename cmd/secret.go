package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/gumieri/nenyactl/internal/paths"
	secrets "github.com/gumieri/nenyactl/internal/secrets"
	"github.com/spf13/cobra"
)

var secretCmd = &cobra.Command{
	Use:   "secret",
	Short: "Manage Nenya secrets",
	Long:  `Generate and manage API keys and secrets for Nenya.`,
}

func init() {
	rootCmd.AddCommand(secretCmd)
	secretCmd.AddCommand(secretGenCmd)
	secretCmd.AddCommand(secretBootstrapCmd)
}

var secretGenCmd = &cobra.Command{
	Use:   "generate",
	Short: "Generate a client token or API key",
	Long: `Generate a random client token or API key.

Client tokens are used for /v1/* endpoint authentication.
API keys are used for client RBAC access.`,
	RunE: runSecretGenerate,
}

var (
	secretType      string
	secretOutput    string
	secretForClient string
)

func init() {
	secretGenCmd.Flags().StringVarP(&secretType, "type", "t", "client", "Secret type: client or apikey")
	secretGenCmd.Flags().StringVarP(&secretOutput, "output", "o", "", "Write to secrets.json file instead of stdout")
	secretGenCmd.Flags().StringVar(&secretForClient, "name", "", "Client name for API key (required with --type apikey)")
}

func runSecretGenerate(cmd *cobra.Command, args []string) error {
	switch secretType {
	case "client":
		token := secrets.GenerateClientToken()
		if secretOutput != "" {
			return fmt.Errorf("write to file not implemented")
		}
		fmt.Println(token)
		return nil

	case "apikey":
		if secretForClient == "" {
			return fmt.Errorf("--name is required for --type apikey")
		}
		id, token := secrets.GenerateAPIKey()
		if secretOutput != "" {
			return fmt.Errorf("write to file not implemented")
		}
		fmt.Printf("ID:    %s\n", id)
		fmt.Printf("Name:  %s\n", secretForClient)
		fmt.Printf("Token: %s\n", token)
		return nil

	default:
		return fmt.Errorf("unknown secret type: %s (use: client, apikey)", secretType)
	}
}

var secretBootstrapCmd = &cobra.Command{
	Use:   "bootstrap",
	Short: "Create initial secrets file",
	Long: `Create a secrets.json file with a generated client token
and placeholder provider keys.

Default location: ` + paths.SystemConfigDir() + `/secrets.json
Existing secrets files are NOT overwritten.`,
	RunE: runSecretBootstrap,
}

var bootstrapDir string

func init() {
	secretBootstrapCmd.Flags().StringVar(&bootstrapDir, "dir", paths.SystemConfigDir(), "Secrets directory")
}

func runSecretBootstrap(cmd *cobra.Command, args []string) error {
	secretsPath := filepath.Join(bootstrapDir, "secrets.json")

	if _, err := os.Stat(secretsPath); err == nil {
		return fmt.Errorf("%s already exists, refusing to overwrite", secretsPath)
	}

	if err := os.MkdirAll(bootstrapDir, 0o755); err != nil {
		return fmt.Errorf("create directory %s: %w", bootstrapDir, err)
	}

	token := secrets.GenerateClientToken()
	content := fmt.Sprintf(`{
  "client_token": "%s",
  "provider_keys": {
    "gemini": "AIza...",
    "deepseek": "sk-..."
  }
}
`, token)

	if err := os.WriteFile(secretsPath, []byte(content), 0o600); err != nil {
		return fmt.Errorf("write %s: %w", secretsPath, err)
	}

	fmt.Println(successStyle.Render("✓"), "Wrote", secretsPath)
	fmt.Println(dimStyle.Render("  → Set your provider API keys before starting nenya"))

	return nil
}
