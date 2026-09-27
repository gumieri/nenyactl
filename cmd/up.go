package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/gumieri/nenyactl/internal/containers"
	"github.com/gumieri/nenyactl/internal/detect"
	"github.com/gumieri/nenyactl/internal/install"
	"github.com/gumieri/nenyactl/internal/secrets"
	"github.com/spf13/cobra"
)

var upCmd = &cobra.Command{
	Use:   "up",
	Short: "Bring Nenya up from any state",
	Long: `Single entry point: detect the deployment (installing a bare-metal
service if none exists), create config and a client token if missing, prompt
for provider keys when none are configured, start the service or containers,
wait for /healthz, and print a copy-pasteable client snippet.

Use --dir to act on a specific deployment root instead of auto-detecting.`,
	RunE: runUp,
}

var (
	upDir         string
	upHealthWait  = 30 * time.Second
	upInstallFunc = install.Install
	// upServiceStart is a seam so up can be tested without touching the host.
	upServiceStart = func() error { return runServiceStartWithExec(defaultExec) }
)

func init() {
	rootCmd.AddCommand(upCmd)
	upCmd.Flags().StringVar(&upDir, "dir", "", "Deployment root (default: auto-detect, installing if missing)")
}

func runUp(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()

	res, err := ensureDeployment(ctx, upDir)
	if err != nil {
		return err
	}

	if err := upDeployment(ctx, res, statusHTTPClient, upHealthWait); err != nil {
		return err
	}

	printClientSnippets(res)
	return nil
}

// ensureDeployment resolves the deployment, installing a bare-metal service
// when --dir was not given and nothing is detected.
func ensureDeployment(ctx context.Context, dir string) (dirResolution, error) {
	if dir != "" {
		return resolveDir(dir, dirAttach, false)
	}

	info, err := detect.Detect()
	if err == nil {
		return detectedResolution(info), nil
	}

	fmt.Println(infoStyle.Render("›"), "No Nenya installation detected; installing the service")
	if installErr := upInstallFunc(ctx, install.Config{}); installErr != nil {
		return dirResolution{}, fmt.Errorf("install: %w", installErr)
	}

	info, err = detect.Detect()
	if err != nil {
		return dirResolution{}, err
	}
	return detectedResolution(info), nil
}

// upDeployment drives a resolved deployment to a healthy, reachable state. It
// is separated from the command so it can be tested with a fake contract,
// health doer, and start function.
func upDeployment(ctx context.Context, res dirResolution, doer healthDoer, healthWait time.Duration) error {
	if err := ensureConfig(res); err != nil {
		return err
	}
	if err := ensureClientToken(ctx, res); err != nil {
		return err
	}
	ensureProviderKeys(ctx, res)

	if err := startDeployment(res); err != nil {
		return err
	}

	desc, _ := res.Contract().Describe(ctx)
	port := statusPort(res, desc, len(desc.Config) > 0)
	if err := waitForHealth(ctx, doer, port, healthWait); err != nil {
		return err
	}
	fmt.Println(successStyle.Render("✓"), "Nenya is healthy on port", port)
	return nil
}

func ensureConfig(res dirResolution) error {
	if _, err := os.Stat(res.Info.ConfigFile); err == nil {
		return nil
	}
	fmt.Println(infoStyle.Render("›"), "Creating config at", res.Info.ConfigFile)
	if err := bootstrapConfig(res.ConfigDir()); err != nil {
		return fmt.Errorf("create config: %w", err)
	}
	return nil
}

func ensureClientToken(ctx context.Context, res dirResolution) error {
	if secrets.ExistingTokenFile(res.Info.SecretsDir()) != "" {
		return nil
	}
	fmt.Println(infoStyle.Render("›"), "Creating a client token")
	writer := res.Contract().SecretWriterFor(res.Info.SecretsDir())
	if _, err := writer.SetClientToken(ctx, ""); err != nil {
		return fmt.Errorf("create client token: %w", err)
	}
	return nil
}

// ensureProviderKeys prompts for provider keys when none are configured. The
// prompt is best-effort: without a terminal it is skipped.
func ensureProviderKeys(ctx context.Context, res dirResolution) {
	desc, err := res.Contract().Describe(ctx)
	if err == nil && len(desc.Providers.Configured) > 0 {
		return
	}
	keys, err := containers.CollectProviderKeys()
	if err != nil || len(keys) == 0 {
		return
	}
	writer := res.Contract().SecretWriterFor(res.Info.SecretsDir())
	for _, provider := range sortedStringKeys(keys) {
		if _, err := writer.SetProviderKey(ctx, provider, keys[provider]); err != nil {
			fmt.Println(errorStyle.Render("✗"), "Could not set provider key", provider+":", err)
			continue
		}
		fmt.Println(successStyle.Render("✓"), "Saved provider key", provider)
	}
}

// startDeployment starts the resolved deployment: compose for a container root,
// the service unit for bare-metal.
func startDeployment(res dirResolution) error {
	if res.Kind == dirContainerRoot {
		fmt.Println(infoStyle.Render("›"), "Starting container deployment at", res.Path)
		return runContainerStartWithExec(defaultExec, res.Path)
	}
	fmt.Println(infoStyle.Render("›"), "Starting the Nenya service")
	return upServiceStart()
}

// waitForHealth polls /healthz until it returns 200 or the wait elapses.
func waitForHealth(ctx context.Context, doer healthDoer, port string, wait time.Duration) error {
	deadline := time.Now().Add(wait)
	var last string
	for {
		last = healthStatus(ctx, doer, port)
		if last == "healthy" {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("nenya did not become healthy on port %s within %s (last: %s)", port, wait, strings.TrimPrefix(last, "unreachable "))
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(500 * time.Millisecond):
		}
	}
}
