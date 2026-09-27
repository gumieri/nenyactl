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
	secretBootstrapCmd.Flags().StringVar(&bootstrapDir, "dir", "", "Config root or container directory (default: system config root)")
}

func runSecretBootstrap(cmd *cobra.Command, args []string) error {
	res, err := resolveDir(bootstrapDir, dirCreate, false)
	if err != nil {
		return err
	}
	secretsDir := res.SecretsDir()
	secretsPath := filepath.Join(secretsDir, "01-client.json")

	newFile, err := writeNewFile0600(secretsPath, []byte(fmt.Sprintf(`{
  "client_token": "%s"
}
`, secrets.GenerateClientToken())))
	if err != nil {
		return err
	}
	if !newFile {
		return fmt.Errorf("%s already exists, refusing to overwrite", secretsPath)
	}

	fmt.Println(successStyle.Render("✓"), "Wrote", secretsPath)
	fmt.Println(dimStyle.Render("  → Set your provider API keys before starting nenya"))

	return nil
}

// writeNewFile0600 writes content with O_EXCL so an existing file is never
// clobbered, and reports whether a new file was created.
func writeNewFile0600(path string, content []byte) (bool, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return false, fmt.Errorf("create directory %s: %w", filepath.Dir(path), err)
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		if os.IsExist(err) {
			return false, nil
		}
		return false, fmt.Errorf("create %s: %w", path, err)
	}
	if _, err := f.Write(content); err != nil {
		_ = f.Close()
		return false, fmt.Errorf("write %s: %w", path, err)
	}
	return true, f.Close()
}
