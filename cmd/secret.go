package cmd

import (
	"context"
	"fmt"

	"github.com/gumieri/nenyactl/internal/containers"
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
	secretForClient string
)

func init() {
	secretGenCmd.Flags().StringVarP(&secretType, "type", "t", "client", "Secret type: client or apikey")
	secretGenCmd.Flags().StringVar(&secretForClient, "name", "", "Client name for API key (required with --type apikey)")
}

func runSecretGenerate(cmd *cobra.Command, args []string) error {
	switch secretType {
	case "client":
		token, err := secrets.GenerateClientToken()
		if err != nil {
			return err
		}
		fmt.Println(token)
		return nil

	case "apikey":
		if secretForClient == "" {
			return fmt.Errorf("--name is required for --type apikey")
		}
		id, token, err := secrets.GenerateAPIKey()
		if err != nil {
			return err
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
	Short: "Create the initial client token",
	Long: `Create the deployment's client token through nenya's single writer
(nenya secret set), which chooses the secrets file, writes atomically, and
fails closed when a systemd credential source is active.

Existing tokens are overwritten only with --force; --force cannot override a
systemd credential source, because nenya fails closed in that case.`,
	RunE: runSecretBootstrap,
}

var (
	bootstrapDir   string
	bootstrapForce bool
)

func init() {
	secretBootstrapCmd.Flags().StringVar(&bootstrapDir, "dir", "", "Config root or container directory (default: system config root)")
	secretBootstrapCmd.Flags().BoolVar(&bootstrapForce, "force", false, "Replace an existing client token")
}

func runSecretBootstrap(cmd *cobra.Command, args []string) error {
	res, err := resolveDir(cmd.Context(), bootstrapDir, dirCreate, false)
	if err != nil {
		return err
	}

	secretsDir := res.Info.SecretsDir()
	if secretsDir == "" {
		return fmt.Errorf("cannot determine the secrets directory for %s", res.Path)
	}

	if !bootstrapForce {
		// The guard must see what nenya sees: a token in a systemd credential
		// or a non-local NENYA_SECRETS_DIR is invisible to a local file scan,
		// yet `secret set` would overwrite it.
		tok, _, _ := clientToken(cmd.Context(), res)
		existing := secrets.ExistingTokenFile(secretsDir)
		if tok != "" || existing != "" {
			detail := res.Path
			if existing != "" {
				detail = existing
			}
			return fmt.Errorf("client token already exists (%s); use --force to replace it", shellQuoteAll([]string{detail}))
		}
	}

	writer := res.Contract().SecretWriterFor(secretsDir)
	path, err := writer.SetClientToken(cmd.Context(), "")
	if err != nil {
		return fmt.Errorf("secret bootstrap: %w", err)
	}

	fmt.Println(successStyle.Render("✓"), "Wrote client token to", path)
	fmt.Println(dimStyle.Render("  → Set your provider API keys with: nenyactl secret set --provider <name> <key>"))

	return nil
}

// secretSetCmd sets a provider key through nenya's single writer.
var secretSetCmd = &cobra.Command{
	Use:   "set <api-key>",
	Short: "Set a provider API key",
	Args:  cobra.ExactArgs(1),
	Long: `Set a provider API key through nenya's single writer (nenya secret set),
which chooses the secrets file, writes atomically, and fails closed when a
systemd credential source is active.

The key is passed to nenya as a command argument, so it is visible in the
process list; prefix the command with a space to keep it out of shell history.`,
	RunE: runSecretSet,
}

var (
	secretSetDir      string
	secretSetProvider string
)

func init() {
	secretCmd.AddCommand(secretSetCmd)
	secretSetCmd.Flags().StringVar(&secretSetDir, "dir", "", "Config root or container directory (default: system config root)")
	secretSetCmd.Flags().StringVar(&secretSetProvider, "provider", "", "Provider name whose key is set (required)")
}

// clientToken resolves the deployment's effective client token. Bare-metal
// reads it through nenya's single reader (`nenya secret get --client-token`,
// CONTRACT.md §4.8), so the §6.1 source precedence is nenya's, not ours;
// released binaries without that command fall back to the documented file shim
// over the deployment's secrets directory. Container deployments read the merge
// directory the compose file mounts at /run/secrets/nenya. viaContract reports
// that the token came from the contract rather than the shim; contractErr is
// the reader's error (unsupported command, or a real failure) for callers that
// must distinguish "shim found it" from "nenya agreed".
func clientToken(ctx context.Context, res dirResolution) (token string, viaContract bool, contractErr error) {
	if res.Kind == dirContainerRoot {
		return containers.ClientToken(res.Path), false, nil
	}
	tok, err := res.Contract().SecretGet(ctx, "client-token")
	if err == nil && tok != "" {
		return tok, true, nil
	}
	return secrets.ClientTokenInDir(res.Info.SecretsDir()), false, err
}

func runSecretSet(cmd *cobra.Command, args []string) error {
	if secretSetProvider == "" {
		return fmt.Errorf("--provider is required")
	}
	if len(args) != 1 {
		return fmt.Errorf("usage: nenyactl secret set --provider <name> <api-key>")
	}

	res, err := resolveDir(cmd.Context(), secretSetDir, dirAttach, false)
	if err != nil {
		return err
	}
	secretsDir := res.Info.SecretsDir()
	if secretsDir == "" {
		return fmt.Errorf("cannot determine the secrets directory for %s", res.Path)
	}

	writer := res.Contract().SecretWriterFor(secretsDir)
	path, err := writer.SetProviderKey(cmd.Context(), secretSetProvider, args[0])
	if err != nil {
		return fmt.Errorf("secret set: %w", err)
	}

	fmt.Println(successStyle.Render("✓"), "Provider key", secretSetProvider, "→", path)
	return nil
}
