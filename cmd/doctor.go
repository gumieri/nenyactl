package cmd

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/gumieri/nenyactl/internal/contract"
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
not failures, so doctor is safe to run before the service is started.`,
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
		fmt.Printf("%s %s: %v\n", checkFail, "deployment", err)
		fmt.Println("  fix: nenyactl up")
		return nil
	}

	results := diagnose(ctx, res)
	var failed int
	for _, r := range results {
		fmt.Printf("%-10s %-16s %s\n", r.Status, r.Name, r.Detail)
		if r.Fix != "" {
			fmt.Printf("  fix: %s\n", r.Fix)
		}
		if r.Status == checkFail {
			failed++
		}
	}
	fmt.Println()
	if failed > 0 {
		fmt.Printf("%d check(s) failed\n", failed)
	} else {
		fmt.Println("all checks passed")
	}
	return nil
}

// diagnose runs every check against the resolved deployment. It is separated
// from the command so each check can be unit-tested.
func diagnose(ctx context.Context, res dirResolution) []checkResult {
	client := res.Contract()

	var results []checkResult
	results = append(results, checkConfig(client, ctx, res))
	results = append(results, checkSecrets(res))
	results = append(results, checkContract(client, ctx))
	results = append(results, checkProviders(client, ctx))
	results = append(results, checkPort(client, ctx, res))
	results = append(results, checkService())
	return results
}

func checkConfig(client *nenya.Client, ctx context.Context, res dirResolution) checkResult {
	r := checkResult{Name: "config"}
	info, err := os.Stat(res.Info.ConfigFile)
	switch {
	case err == nil && info.IsDir():
		r.Status, r.Detail = checkFail, res.Info.ConfigFile+" is a directory"
		r.Fix = "remove it and run `nenyactl config init`"
		return r
	case err == nil:
		r.Detail = res.Info.ConfigFile
	default:
		r.Status, r.Detail = checkFail, "no config at "+res.Info.ConfigFile
		r.Fix = "run `nenyactl config init --dir " + res.Path + "`"
		return r
	}

	if _, err := client.Describe(ctx); err != nil {
		r.Status, r.Detail = checkWarn, "cannot read effective config ("+err.Error()+")"
		r.Fix = "install a nenya release that ships `describe --json`"
		return r
	}
	return r
}

func checkSecrets(res dirResolution) checkResult {
	dir := res.Info.SecretsDir()
	if dir == "" {
		return checkResult{Name: "secrets", Status: checkFail, Detail: "cannot resolve the secrets directory"}
	}
	found := secrets.ExistingTokenFile(dir)
	if found == "" {
		return checkResult{
			Name:   "secrets",
			Status: checkFail,
			Detail: "no client token in " + dir,
			Fix:    "run `nenyactl secret bootstrap --dir " + res.Path + "`",
		}
	}
	info, err := os.Stat(found)
	if err != nil {
		return checkResult{Name: "secrets", Status: checkWarn, Detail: err.Error()}
	}
	if info.Mode().Perm()&0o077 != 0 {
		return checkResult{
			Name:   "secrets",
			Status: checkFail,
			Detail: fmt.Sprintf("%s is mode %04o, want 0600", found, info.Mode().Perm()),
			Fix:    "chmod 600 " + found,
		}
	}
	return checkResult{Name: "secrets", Status: checkOK, Detail: found}
}

func checkContract(client *nenya.Client, ctx context.Context) checkResult {
	desc, err := client.Describe(ctx)
	if err != nil {
		var unsupported *contract.UnsupportedError
		if errors.As(err, &unsupported) {
			return checkResult{Name: "contract", Status: checkFail, Detail: unsupported.Error(), Fix: "update nenyactl or install a compatible nenya"}
		}
		return checkResult{
			Name:   "contract",
			Status: checkWarn,
			Detail: "not reported (describe unavailable)",
			Fix:    "nenya releases that ship `describe --json` report contract_version",
		}
	}
	if err := contract.Check(desc.ContractVersion); err != nil {
		return checkResult{Name: "contract", Status: checkFail, Detail: err.Error(), Fix: "update nenyactl or install a compatible nenya"}
	}
	return checkResult{
		Name:   "contract",
		Status: checkOK,
		Detail: fmt.Sprintf("v%d (nenyactl supports %s)", desc.ContractVersion, contract.Range()),
	}
}

func checkProviders(client *nenya.Client, ctx context.Context) checkResult {
	desc, err := client.Describe(ctx)
	if err != nil {
		return checkResult{Name: "providers", Status: checkWarn, Detail: "not reported (describe unavailable)"}
	}
	if len(desc.Providers.Configured) == 0 {
		return checkResult{
			Name:   "providers",
			Status: checkWarn,
			Detail: "no provider keys configured",
			Fix:    "nenyactl secret set --provider <name> <api-key>",
		}
	}
	return checkResult{Name: "providers", Status: checkOK, Detail: strings.Join(desc.Providers.Configured, ", ")}
}

func checkPort(client *nenya.Client, ctx context.Context, res dirResolution) checkResult {
	desc, _ := client.Describe(ctx)
	port := statusPort(res, desc, len(desc.Config) > 0)
	if port == "" {
		return checkResult{Name: "port", Status: checkWarn, Detail: "could not resolve a port"}
	}

	addr := "localhost:" + port
	conn, err := net.DialTimeout("tcp", addr, time.Second)
	if err != nil {
		return checkResult{Name: "port", Status: checkWarn, Detail: addr + " is free (service not running)", Fix: "nenyactl up"}
	}
	_ = conn.Close()
	return checkResult{Name: "port", Status: checkOK, Detail: addr + " is listening"}
}

func checkService() checkResult {
	path := filepath.Join(defaultUnitDir(), "nenya.service")
	if _, err := os.Stat(path); err == nil {
		return checkResult{Name: "service", Status: checkOK, Detail: path}
	}
	return checkResult{
		Name:   "service",
		Status: checkWarn,
		Detail: "no systemd unit found",
		Fix:    "run `nenyactl up` to install and enable it",
	}
}

func defaultUnitDir() string {
	if runtime.GOOS == "darwin" {
		return "/Library/LaunchDaemons"
	}
	return "/etc/systemd/system"
}
