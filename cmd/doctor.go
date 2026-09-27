package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/gumieri/nenyactl/internal/contract"
	"github.com/gumieri/nenyactl/internal/detect"
	"github.com/gumieri/nenyactl/internal/install"
	"github.com/gumieri/nenyactl/internal/nenya"
	"github.com/gumieri/nenyactl/internal/secrets"
	"github.com/spf13/cobra"
)

var doctorCmd = &cobra.Command{
	Use:   "doctor",
	Short: "Diagnose the nenya seam",
	Long: `Check the installed nenya binary, contract compatibility, the resolved
config and secrets, provider keys, port availability, and service state, and
report an actionable fix for anything that is wrong.

Checks that need a running gateway or a provider key are reported as warnings,
not failures, so doctor is safe to run before the service is started. Exits
non-zero when a check fails, so it can gate scripts.`,
	Args: cobra.NoArgs,
	RunE: runDoctor,
}

var doctorDir string

func init() {
	rootCmd.AddCommand(doctorCmd)
	doctorCmd.Flags().StringVar(&doctorDir, "dir", "", "Deployment root (default: auto-detect)")
}

// checkStatus is the outcome of a single doctor check.
type checkStatus int

const (
	checkOK checkStatus = iota
	checkWarn
	checkFail
)

func (s checkStatus) String() string {
	switch s {
	case checkOK:
		return "ok"
	case checkWarn:
		return "warn"
	default:
		return "fail"
	}
}

// checkResult is one diagnosed item.
type checkResult struct {
	Name   string
	Status checkStatus
	Detail string
	Fix    string
}

func runDoctor(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()
	res, err := resolveLifecycleDir(doctorDir)
	if err != nil {
		printCheck(checkResult{Name: "deployment", Status: checkFail, Detail: err.Error(), Fix: deploymentFix(err)})
		return fmt.Errorf("deployment: %w", err)
	}

	results := diagnose(ctx, res, statusHTTPClient)
	var failed int
	for _, r := range results {
		printCheck(r)
		if r.Status == checkFail {
			failed++
		}
	}
	fmt.Println()
	if failed > 0 {
		return fmt.Errorf("%d check(s) failed", failed)
	}
	fmt.Println("all checks passed")
	return nil
}

func printCheck(r checkResult) {
	style := successStyle
	switch r.Status {
	case checkWarn:
		style = warnStyle
	case checkFail:
		style = errorStyle
	}
	fmt.Printf("%s %-12s %s\n", style.Render(r.Status.String()), r.Name, r.Detail)
	if r.Fix != "" {
		fmt.Printf("  fix: %s\n", r.Fix)
	}
}

// deploymentFix chooses the suggested action from the detection error.
func deploymentFix(err error) string {
	var perm *detect.PermissionError
	if errors.As(err, &perm) {
		return "check file permissions; re-run with sudo if the config is root-owned"
	}
	return "nenyactl up"
}

// diagnose runs every check against the resolved deployment. It fetches the
// effective description once and shares it, so doctor spawns `nenya` once.
func diagnose(ctx context.Context, res dirResolution, doer healthDoer) []checkResult {
	client := res.Contract()
	desc, descErr := client.Describe(ctx)
	haveDesc := descErr == nil

	return []checkResult{
		checkConfig(res, descErr, haveDesc),
		checkSecrets(res, desc, haveDesc),
		checkContract(desc, descErr),
		checkProviders(desc, descErr),
		checkPort(ctx, doer, res, desc, haveDesc),
		checkService(res),
	}
}

func checkConfig(res dirResolution, descErr error, haveDesc bool) checkResult {
	r := checkResult{Name: "config"}
	info, err := os.Stat(res.Info.ConfigFile)
	switch {
	case err == nil && info.IsDir():
		return checkResult{Name: "config", Status: checkFail, Detail: res.Info.ConfigFile + " is a directory", Fix: "remove it and run `nenyactl up`"}
	case err == nil:
		r.Detail = res.Info.ConfigFile
	case len(configAddonDirs(res)) > 0:
		// Directory-mode config.d-only deployments have no config.json.
		r.Detail = strings.Join(configAddonDirs(res), ", ")
	default:
		return checkResult{Name: "config", Status: checkFail, Detail: "no config at " + res.Info.ConfigFile, Fix: "run `nenyactl up`"}
	}

	if !haveDesc {
		if v, ok := contract.UnsupportedContractVersion(descErr); ok {
			return checkResult{Name: "config", Status: checkFail, Detail: fmt.Sprintf("nenya reports unsupported contract_version %d", v), Fix: "update nenyactl or install a compatible nenya"}
		}
		r.Status = checkWarn
		r.Detail += " (cannot read effective config: " + descErr.Error() + ")"
		r.Fix = "install a nenya release that ships `describe --json`"
	}
	return r
}

// configAddonDirs returns the config.d drop-ins present for the deployment.
func configAddonDirs(res dirResolution) []string {
	entries, err := os.ReadDir(res.Info.ConfigD)
	if err != nil {
		return nil
	}
	var files []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".json") && e.Name() != "secrets.json" {
			files = append(files, filepath.Join(res.Info.ConfigD, e.Name()))
		}
	}
	return files
}

func checkSecrets(res dirResolution, desc nenya.Description, haveDesc bool) checkResult {
	dir := res.Info.SecretsDir()
	if dir == "" {
		return checkResult{Name: "secrets", Status: checkFail, Detail: "cannot resolve the secrets directory"}
	}

	// A systemd credential source is authoritative but invisible to us; defer
	// to the contract when it reports one.
	if haveDesc && strings.Contains(desc.Secrets.ActiveSource, "CREDENTIALS_DIRECTORY") {
		return checkResult{Name: "secrets", Status: checkOK, Detail: "systemd credential source: " + desc.Secrets.ActiveSource}
	}

	files := secretFilesIn(dir)
	if len(files) == 0 {
		return checkResult{Name: "secrets", Status: checkFail, Detail: "no client token in " + dir, Fix: "run `nenyactl up`"}
	}

	var loose []string
	for _, f := range files {
		if info, err := os.Stat(f); err == nil && info.Mode().Perm()&0o077 != 0 {
			loose = append(loose, fmt.Sprintf("%s (%04o)", f, info.Mode().Perm()))
		}
	}
	if len(loose) > 0 {
		return checkResult{Name: "secrets", Status: checkFail, Detail: "world/group-readable: " + strings.Join(loose, ", "), Fix: "chmod 600 " + shellQuoteAll(loose)}
	}
	return checkResult{Name: "secrets", Status: checkOK, Detail: fmt.Sprintf("%d file(s) in %s", len(files), dir)}
}

// secretFilesIn returns every secrets file (not directory) under dir that
// actually carries a client_token, so an unrelated config.json beside it is not
// mistaken for a secrets file.
func secretFilesIn(dir string) []string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var files []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if !strings.HasSuffix(e.Name(), ".json") && e.Name() != "secrets" {
			continue
		}
		p := filepath.Join(dir, e.Name())
		data, err := os.ReadFile(p)
		if err != nil || !secrets.HasClientToken(data) {
			continue
		}
		files = append(files, p)
	}
	return files
}

func checkContract(desc nenya.Description, descErr error) checkResult {
	if descErr != nil {
		if v, ok := contract.UnsupportedContractVersion(descErr); ok {
			return checkResult{Name: "contract", Status: checkFail, Detail: fmt.Sprintf("nenya contract_version %d is not supported (nenyactl supports %s)", v, contract.Range()), Fix: "update nenyactl or install a compatible nenya"}
		}
		return checkResult{Name: "contract", Status: checkWarn, Detail: "not reported (describe unavailable)", Fix: "nenya releases that ship `describe --json` report contract_version"}
	}
	if desc.ContractVersion == 0 {
		return checkResult{Name: "contract", Status: checkWarn, Detail: "not reported"}
	}
	return checkResult{Name: "contract", Status: checkOK, Detail: fmt.Sprintf("v%d (nenyactl supports %s)", desc.ContractVersion, contract.Range())}
}

func checkProviders(desc nenya.Description, descErr error) checkResult {
	if descErr != nil {
		return checkResult{Name: "providers", Status: checkWarn, Detail: "not reported (describe unavailable)"}
	}
	if len(desc.Providers.Configured) == 0 {
		return checkResult{Name: "providers", Status: checkWarn, Detail: "no provider keys configured", Fix: "nenyactl secret set --provider <name> <api-key>"}
	}
	return checkResult{Name: "providers", Status: checkOK, Detail: strings.Join(desc.Providers.Configured, ", ")}
}

func checkPort(ctx context.Context, doer healthDoer, res dirResolution, desc nenya.Description, haveDesc bool) checkResult {
	port := statusPort(res, desc, haveDesc)
	if port == "" {
		return checkResult{Name: "port", Status: checkWarn, Detail: "could not resolve a port"}
	}
	addr := "localhost:" + port
	ok, detail := healthStatus(ctx, doer, port)
	switch {
	case ok:
		return checkResult{Name: "port", Status: checkOK, Detail: addr + " is healthy"}
	case strings.HasPrefix(detail, "unreachable"):
		return checkResult{Name: "port", Status: checkWarn, Detail: addr + " is free (service not running)", Fix: "nenyactl up"}
	default:
		return checkResult{Name: "port", Status: checkWarn, Detail: addr + ": " + detail}
	}
}

func checkService(res dirResolution) checkResult {
	if res.Kind == dirContainerRoot {
		return checkResult{Name: "service", Status: checkOK, Detail: "container deployment (no unit)"}
	}
	name := "nenya.service"
	if runtime.GOOS == "darwin" {
		name = "com.gumieri.nenya.plist"
	}
	path := filepath.Join(install.SystemUnitDir(), name)
	if _, err := os.Stat(path); err == nil {
		return checkResult{Name: "service", Status: checkOK, Detail: path}
	}
	return checkResult{Name: "service", Status: checkWarn, Detail: "no unit found at " + path, Fix: "nenyactl up"}
}

func shellQuoteAll(paths []string) string {
	quoted := make([]string, len(paths))
	for i, p := range paths {
		quoted[i] = "'" + strings.ReplaceAll(p, "'", `'\''`) + "'"
	}
	return strings.Join(quoted, " ")
}
