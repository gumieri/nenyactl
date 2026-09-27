package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/gumieri/nenyactl/internal/containers"
	"github.com/gumieri/nenyactl/internal/detect"
	"github.com/gumieri/nenyactl/internal/install"
	"github.com/gumieri/nenyactl/internal/nenya"
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
	Args: cobra.NoArgs,
	RunE: runUp,
}

var (
	upDir              string
	upWait             time.Duration
	upInstall          = install.Install
	upServiceRun       = func() error { return runServiceStartWithExec(defaultExec) }
	upServiceReload    = func() error { return runServiceReloadWithExec(defaultExec) }
	upContainerRestart = func(dir string) error { return runContainerRestartWithExec(defaultExec, dir) }
	upCollectKeys      = containers.CollectProviderKeys
	upDetect           = detect.Detect
)

func init() {
	rootCmd.AddCommand(upCmd)
	upCmd.Flags().StringVar(&upDir, "dir", "", "Deployment root (default: auto-detect, installing if missing)")
	upCmd.Flags().DurationVar(&upWait, "wait", 30*time.Second, "How long to wait for /healthz")
}

func runUp(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()

	res, err := ensureDeployment(ctx, upDir)
	if err != nil {
		return err
	}

	if err := upDeployment(ctx, res, statusHTTPClient, upWait); err != nil {
		return err
	}

	printClientSnippets(res)
	return nil
}

// ensureDeployment resolves the deployment, installing a bare-metal service
// when --dir was not given and nothing is detected. A binary that is present
// but whose config is missing (or unreadable) is a state problem, not a reason
// to reinstall, so those errors are surfaced rather than retried.
func ensureDeployment(ctx context.Context, dir string) (dirResolution, error) {
	if dir != "" {
		return resolveDir(dir, dirAttach, false)
	}

	info, err := upDetect()
	if err == nil {
		return detectedResolution(info), nil
	}
	if isInstallationStateError(err) {
		return dirResolution{}, err
	}
	if isAmbiguousInstall(err) {
		return dirResolution{}, fmt.Errorf("%w\n\npass --dir to choose which installation to act on", err)
	}

	fmt.Println(infoStyle.Render("›"), "No Nenya installation detected; installing the service")
	if installErr := upInstall(ctx, install.Config{}); installErr != nil {
		return dirResolution{}, fmt.Errorf("install: %w", installErr)
	}

	info, err = upDetect()
	if err != nil {
		return dirResolution{}, err
	}
	return detectedResolution(info), nil
}

// isInstallationStateError reports whether a detection error describes a
// present-but-incomplete installation, which installing again cannot fix.
func isInstallationStateError(err error) bool {
	var cfg *detect.ConfigNotFoundError
	var perm *detect.PermissionError
	return errors.As(err, &cfg) || errors.As(err, &perm)
}

// isAmbiguousInstall reports whether detection found more than one installation.
// Installing again would be wrong, so up asks for --dir instead.
func isAmbiguousInstall(err error) bool {
	var ambiguous *detect.AmbiguousError
	return errors.As(err, &ambiguous)
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

	desc, descErr := res.Contract().Describe(ctx)
	if descErr != nil {
		fmt.Println(dimStyle.Render("  (could not read the effective config: " + descErr.Error() + ")"))
	}
	wroteKeys := ensureProviderKeys(ctx, res, desc, descErr)

	if err := startDeployment(res); err != nil {
		return err
	}
	// Keys saved above are not loaded by an already-running process. A
	// bare-metal service reloads (SIGHUP); a running container only re-reads
	// the bind-mounted secrets on restart, since compose up does not recreate
	// an unchanged container.
	if wroteKeys {
		switch res.Kind {
		case dirContainerRoot:
			if err := upContainerRestart(res.Path); err != nil {
				fmt.Println(dimStyle.Render("  (could not restart the containers: " + err.Error() + ")"))
			}
		default:
			if err := upServiceReload(); err != nil {
				fmt.Println(dimStyle.Render("  (could not reload the service: " + err.Error() + ")"))
			}
		}
	}

	port := statusPort(res, desc, descErr == nil)
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
	// A config.d-only root is already configured in nenya main; do not shadow
	// it with a shim config.json.
	if len(configAddonDirs(res)) > 0 {
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

// ensureProviderKeys prompts for provider keys when none are configured and
// reports whether it saved any. The prompt is best-effort: without a terminal
// it is skipped, and it is skipped entirely when the contract could not be
// read, since we cannot know what the running nenya understands.
func ensureProviderKeys(ctx context.Context, res dirResolution, desc nenya.Description, descErr error) bool {
	if descErr != nil {
		return false
	}
	if len(desc.Providers.Configured) > 0 {
		return false
	}
	keys, err := upCollectKeys()
	if err != nil || len(keys) == 0 {
		return false
	}
	writer := res.Contract().SecretWriterFor(res.Info.SecretsDir())
	wrote := false
	for _, provider := range sortedStringKeys(keys) {
		if _, err := writer.SetProviderKey(ctx, provider, keys[provider]); err != nil {
			fmt.Println(errorStyle.Render("✗"), "Could not set provider key", provider+":", err)
			continue
		}
		wrote = true
		fmt.Println(successStyle.Render("✓"), "Saved provider key", provider)
	}
	return wrote
}

// startDeployment starts the resolved deployment: compose for a container root,
// the service unit for bare-metal.
func startDeployment(res dirResolution) error {
	if res.Kind == dirContainerRoot {
		fmt.Println(infoStyle.Render("›"), "Starting container deployment at", res.Path)
		return runContainerStartWithExec(defaultExec, res.Path)
	}
	fmt.Println(infoStyle.Render("›"), "Starting the Nenya service")
	return upServiceRun()
}

// waitForHealth polls /healthz until it returns 200 or the wait elapses.
func waitForHealth(ctx context.Context, doer healthDoer, port string, wait time.Duration) error {
	if port == "" {
		return fmt.Errorf("could not resolve the gateway port")
	}
	deadline := time.Now().Add(wait)
	var detail string
	for {
		var ok bool
		ok, detail = healthStatus(ctx, doer, port)
		if ok {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("nenya did not become healthy on port %s within %s (%s)", port, wait, detail)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(500 * time.Millisecond):
		}
	}
}
