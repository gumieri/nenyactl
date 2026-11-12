package cmd

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"

	"github.com/gumieri/nenyactl/internal/contract"
	"github.com/gumieri/nenyactl/internal/detect"
	"github.com/gumieri/nenyactl/internal/install"
	"github.com/gumieri/nenyactl/internal/nenya"
	"github.com/spf13/cobra"
)

var doctorCmd = &cobra.Command{
	Use:   "doctor",
	Short: "Diagnose the nenya seam",
	// RunE errors (failed checks, interrupted watches) are results, not
	// usage mistakes — the usage block would only add noise.
	SilenceUsage: true,
	Long: `Check the installed nenya binary, contract compatibility, the resolved
config and secrets, provider keys, port availability, and service state, and
report an actionable fix for anything that is wrong.

Checks that need a running gateway or a provider key are reported as warnings,
not failures, so doctor is safe to run before the service is started. Exits
non-zero when a check fails, so it can gate scripts.

With --watch, the checks re-run every --interval until every check passes or
you interrupt with ctrl-c — a live repair loop instead of a static report
(--interval applies only with --watch; minimum 1s).

Checks whose fix is a nenyactl-owned command (currently 'nenyactl up') offer
to run it directly with a [y/N] prompt (one-shot mode only — watch never
prompts; EOF on stdin counts as "no"). Doctor never shells out to arbitrary
commands.`,
	Args: cobra.NoArgs,
	RunE: runDoctor,
}

var (
	doctorDir      string
	doctorWatch    bool
	doctorInterval time.Duration
)

func init() {
	rootCmd.AddCommand(doctorCmd)
	doctorCmd.Flags().StringVar(&doctorDir, "dir", "", "Deployment root (default: auto-detect)")
	doctorCmd.Flags().BoolVar(&doctorWatch, "watch", false, "re-run the checks on an interval until all pass or ctrl-c")
	doctorCmd.Flags().DurationVar(&doctorInterval, "interval", 5*time.Second, "re-run interval for --watch (minimum 1s)")
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

// checkResult is one diagnosed item. Fix is the human suggestion; Action, when
// set, is a nenyactl-owned command the doctor can offer to run itself (NCTL-47)
// — doctor never shells out to arbitrary commands.
type checkResult struct {
	Name   string
	Status checkStatus
	Detail string
	Fix    string
	Action doctorAction
}

// doctorAction enumerates the internal fixes doctor can offer to run.
type doctorAction int

const (
	actionNone doctorAction = iota
	actionUp                // `nenyactl up`: ensure the deployment and wait for /healthz
)

// actionCommand is the user-visible command each action corresponds to.
func actionCommand(a doctorAction) string {
	switch a {
	case actionUp:
		return "nenyactl up"
	default:
		return ""
	}
}

// runDoctorAction executes an internal fix by invoking the real command path,
// so the doctor's fix and the standalone command share one implementation
// (NCTL-47). dir and wait come from the caller's flag state.
func runDoctorAction(ctx context.Context, dir string, wait time.Duration, a doctorAction) error {
	switch a {
	case actionUp:
		return runUpWith(ctx, dir, wait)
	default:
		return fmt.Errorf("no runnable action %d", int(a))
	}
}

// printResults prints every check and returns the failure count.
func printResults(results []checkResult) int {
	for _, r := range results {
		printCheck(r)
	}
	return countFailed(results)
}

// resolveLifecycleDirFn and diagnoseFn are the doctor's host-interaction
// seams, overridden in tests so the repair flow can run hermetically
// (NCTL-47 review).
var (
	resolveLifecycleDirFn = resolveLifecycleDir
	diagnoseFn            = diagnose
	// runDoctorActionFn is the fix-execution seam (tests intercept the repair
	// without touching the host).
	runDoctorActionFn = runDoctorAction
)

func runDoctor(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()
	if !doctorWatch && cmd.Flags().Changed("interval") {
		_, _ = fmt.Fprintln(cmd.ErrOrStderr(), "note: --interval applies only with --watch")
	}
	res, err := resolveLifecycleDirFn(ctx, doctorDir)
	if err != nil {
		// The deployment-failure fixes span binary install (network,
		// state-changing) and manual steps; they stay display-only —
		// only idempotent internal fixes (actionUp) are ever offered.
		printCheck(checkResult{Name: "deployment", Status: checkFail, Detail: err.Error(), Fix: deploymentFix(err)})
		return fmt.Errorf("deployment: %w", err)
	}

	if doctorWatch {
		if effectiveDoctorInterval() != doctorInterval {
			_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "note: --interval %s is below the 1s floor; using %s\n", doctorInterval, effectiveDoctorInterval())
		}
		return runDoctorWatch(ctx, res)
	}

	refresh := func() []checkResult { return diagnoseFn(ctx, res, statusHTTPClient) }
	results := refresh()
	failed := printResults(results)

	// NCTL-47: actionable checks (fails, and warnings whose fix is
	// nenyactl-owned) are offered interactively; each accepted fix re-runs
	// the real command.
	repaired := false
	if failed > 0 || anyActionable(results) {
		var fixed []checkResult
		repaired, fixed, err = offerFixes(ctx, cmd.InOrStdin(), cmd.ErrOrStderr(),
			func(ctx context.Context, a doctorAction) error {
				// res.Path, not doctorDir: the fix must act on the exact
				// installation doctor diagnosed (no re-detection window).
				return runDoctorActionFn(ctx, res.Path, upWait, a)
			}, results, refresh)
		if err != nil {
			return err
		}
		if repaired {
			// Reuse offerFixes' post-fix diagnosis: no second describe/health
			// round-trip.
			results = fixed
		}
	}

	// After an accepted repair, the exit status reflects post-fix reality
	// instead of the pre-fix snapshot (results came from offerFixes' final
	// refresh — no extra diagnose run).
	if repaired {
		fmt.Println()
		failed = printResults(results)
		if failed == 0 {
			fmt.Println("all checks passed")
			return nil
		}
	}

	fmt.Println()
	if failed > 0 {
		return fmt.Errorf("%d check(s) failed", failed)
	}
	fmt.Println("all checks passed")
	return nil
}

// runDoctorWatch re-renders the diagnosis every interval until every check
// passes (success exit) or the context is canceled (ctrl-c/SIGTERM), then
// reports the last run's status (NCTL-47).
func runDoctorWatch(ctx context.Context, res dirResolution) error {
	interval := effectiveDoctorInterval()
	// ctrl-c and SIGTERM end the loop cleanly so the final status prints.
	ctx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()

	clear := stdoutIsTerminal()
	paint := func(results []checkResult, failed int) {
		if clear {
			fmt.Print("\x1b[2J\x1b[H") // clear screen, cursor home
		}
		fmt.Printf("doctor --watch · every %s · %s\n\n", interval, time.Now().Format("15:04:05"))
		for _, r := range results {
			printCheck(r)
		}
		if failed == 0 {
			fmt.Println("\nall checks passed")
		} else {
			fmt.Printf("\n%d check(s) failing — re-running in %s (ctrl-c to stop)\n", failed, interval)
		}
	}
	failed, canceled := doctorWatchLoop(ctx, interval, func() []checkResult {
		return diagnose(ctx, res, statusHTTPClient)
	}, paint, waitInterval)
	if canceled {
		return fmt.Errorf("interrupted with %d check(s) still failing", failed)
	}
	if failed > 0 {
		return fmt.Errorf("%d check(s) failed", failed)
	}
	return nil
}

// effectiveDoctorInterval clamps the configured watch interval to the 1s
// floor (NCTL-47).
func effectiveDoctorInterval() time.Duration {
	if doctorInterval < time.Second {
		return time.Second
	}
	return doctorInterval
}

// stdoutIsTerminal reports whether stdout is a character device (a terminal);
// ANSI clear-screen codes are only emitted then.
func stdoutIsTerminal() bool {
	info, err := os.Stdout.Stat()
	return err == nil && info.Mode()&os.ModeCharDevice != 0
}

// doctorWatchLoop runs the diagnosis, paints each run, and waits interval
// between runs until ctx is canceled or a run has zero failures. It returns
// the last run's failure count and whether cancellation ended the loop
// (NCTL-47): failures at cancellation may be diagnose artifacts, so the
// caller reports them as interrupted. run and wait are injectable for tests;
// wait reports whether the full interval elapsed (false = canceled).
func doctorWatchLoop(ctx context.Context, interval time.Duration, run func() []checkResult, paint func([]checkResult, int), wait func(context.Context, time.Duration) bool) (int, bool) {
	for {
		results := run()
		failed := countFailed(results)
		paint(results, failed)
		if failed == 0 {
			return 0, false
		}
		if ctx.Err() != nil {
			return failed, true
		}
		if !wait(ctx, interval) {
			return failed, true
		}
	}
}

// countFailed returns the number of checkFail entries.
func countFailed(results []checkResult) int {
	var failed int
	for _, r := range results {
		if r.Status == checkFail {
			failed++
		}
	}
	return failed
}

// anyActionable reports whether any check carries a nenyactl-owned fix worth
// offering, even when nothing hard-fails (e.g. the pre-`up` state where the
// actionable checks are warnings) (NCTL-47).
func anyActionable(results []checkResult) bool {
	for _, r := range results {
		if isActionable(r) {
			return true
		}
	}
	return false
}

// waitInterval sleeps interval or until ctx is canceled; reports whether the
// full interval elapsed.
func waitInterval(ctx context.Context, interval time.Duration) bool {
	timer := time.NewTimer(interval)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}

// offerFixes prompts for checks whose fix is a nenyactl-owned command and
// runs accepted fixes through run (NCTL-47). Only internal actions are
// offered — doctor never shells out to arbitrary commands. Each action is
// offered at most once per invocation; after an accepted fix the diagnosis is
// refreshed via refresh, so later prompts reflect post-fix state and a fix
// that repaired several checks never re-prompts. A declined check drops out
// of consideration; EOF or a read error declines and stops prompting (doctor's
// exit code still gates scripts). It reports whether any fix ran and returns
// the final diagnosis (refreshed after the last accepted fix) so callers can
// reuse it instead of re-running the checks.
func offerFixes(ctx context.Context, in io.Reader, out io.Writer, run func(context.Context, doctorAction) error, initial []checkResult, refresh func() []checkResult) (bool, []checkResult, error) {
	// Work on a copy: declines mutate the working set, and the caller's
	// initial slice must stay intact.
	results := make([]checkResult, len(initial))
	copy(results, initial)
	reader := bufio.NewReader(in)
	repaired := false
	offered := map[doctorAction]bool{}
	for {
		var pending *checkResult
		var pendingIdx int
		for i := range results {
			r := &results[i]
			if !isActionable(*r) || offered[r.Action] {
				continue
			}
			pending = r
			pendingIdx = i
			break
		}
		if pending == nil {
			return repaired, results, nil
		}
		offered[pending.Action] = true

		_, _ = fmt.Fprintf(out, "run: %s? [y/N] ", actionCommand(pending.Action))
		line, err := reader.ReadString('\n')
		if err != nil {
			// EOF or a read failure (including a partial line with no
			// newline): decline and stop prompting.
			return repaired, results, nil
		}
		answer := strings.ToLower(strings.TrimSpace(line))
		if answer != "y" && answer != "yes" {
			// Declined: drop this check so other pending actions still
			// prompt.
			results = append(results[:pendingIdx], results[pendingIdx+1:]...)
			continue
		}
		if err := run(ctx, pending.Action); err != nil {
			return repaired, results, fmt.Errorf("fix (%s): %w", actionCommand(pending.Action), err)
		}
		repaired = true
		results = refresh()
	}
}

// isActionable reports whether a check's fix is a nenyactl-owned command worth
// offering: actionable status (fail or warn) plus a known internal action
// (NCTL-47). Shared by the offer gates so they stay in lockstep.
func isActionable(r checkResult) bool {
	return r.Action != actionNone && (r.Status == checkFail || r.Status == checkWarn)
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
	var ambiguous *detect.AmbiguousError
	if errors.As(err, &ambiguous) {
		return "pass --dir to choose which installation to act on"
	}
	return "nenyactl up"
}

// diagnose runs every check against the resolved deployment. It fetches the
// effective description once and shares it, so the describe call is not
// repeated per check (resolution and the secrets check make their own
// contract probes).
func diagnose(ctx context.Context, res dirResolution, doer healthDoer) []checkResult {
	client := res.Contract()
	desc, descErr := client.Describe(ctx)
	haveDesc := descErr == nil

	return []checkResult{
		checkConfig(res, descErr, haveDesc),
		checkSecrets(ctx, res, desc, haveDesc),
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
		return checkResult{Name: "config", Status: checkFail, Detail: "no config at " + res.Info.ConfigFile, Fix: "run `nenyactl up`", Action: actionUp}
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

func checkSecrets(ctx context.Context, res dirResolution, desc nenya.Description, haveDesc bool) checkResult {
	dir := res.Info.SecretsDir()
	if dir == "" {
		return checkResult{Name: "secrets", Status: checkFail, Detail: "cannot resolve the secrets directory"}
	}

	// A systemd credential source is authoritative but invisible to us; defer
	// to the contract when it reports one.
	if haveDesc && strings.Contains(desc.Secrets.ActiveSource, "CREDENTIALS_DIRECTORY") {
		return checkResult{Name: "secrets", Status: checkOK, Detail: "systemd credential source: " + desc.Secrets.ActiveSource}
	}

	// Token presence through nenya's single reader first (§4.8): it resolves
	// the deployment's real source order, which a directory guess cannot. The
	// file shim covers released binaries without `secret get`; when the reader
	// ran and the shim disagrees, say so instead of picking a winner silently.
	token, viaContract, contractErr := clientToken(ctx, res)
	if token == "" {
		detail := "no client token in " + dir
		if contractErr != nil {
			detail += " (nenya secret get: " + contractErr.Error() + ")"
		}
		return checkResult{Name: "secrets", Status: checkFail, Detail: detail, Fix: "run `nenyactl up`", Action: actionUp}
	}

	detail := fmt.Sprintf("client token resolves (%s)", dir)
	switch {
	case viaContract:
		detail = "client token resolves via nenya secret get"
	case errors.Is(contractErr, errTokenNotResolved):
		// The reader exists, ran clean, and resolved nothing: the local token
		// is not the one nenya serves.
		return checkResult{Name: "secrets", Status: checkWarn, Detail: "nenya secret get resolved no client token, but a token exists in " + dir, Fix: "reconcile the secrets sources, then re-run `nenyactl doctor`"}
	case contractErr != nil:
		// The reader exists and failed: distinguish, don't hide.
		return checkResult{Name: "secrets", Status: checkWarn, Detail: "token in " + dir + ", but nenya secret get failed: " + contractErr.Error(), Fix: "reconcile the secrets sources, then re-run `nenyactl doctor`"}
	}

	// Check permissions on every secrets file we can see, not just the
	// token-bearing one; a credential-dir token has no local file to check.
	files := secretFilesIn(dir, res.Info.ConfigFile)
	var loose, looseModes []string
	for _, f := range files {
		if info, err := os.Stat(f); err == nil && info.Mode().Perm()&0o077 != 0 {
			loose = append(loose, f)
			looseModes = append(looseModes, fmt.Sprintf("%s (%04o)", f, info.Mode().Perm()))
		}
	}
	if len(loose) > 0 {
		return checkResult{Name: "secrets", Status: checkFail, Detail: "world/group-readable: " + strings.Join(looseModes, ", "), Fix: "chmod 600 " + shellQuoteAll(loose)}
	}
	return checkResult{Name: "secrets", Status: checkOK, Detail: detail}
}

// secretFilesIn returns every secrets file under dir (excluding the config file,
// which shares the bare-metal directory), regardless of whether it carries a
// token, so permissions are checked on all of them.
func secretFilesIn(dir, exclude string) []string {
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
		if p == exclude {
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
		return checkResult{Name: "port", Status: checkWarn, Detail: addr + " is free (service not running)", Fix: "nenyactl up", Action: actionUp}
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
	return checkResult{Name: "service", Status: checkWarn, Detail: "no unit found at " + path, Fix: "nenyactl up", Action: actionUp}
}

func shellQuoteAll(paths []string) string {
	quoted := make([]string, len(paths))
	for i, p := range paths {
		quoted[i] = "'" + strings.ReplaceAll(p, "'", `'\''`) + "'"
	}
	return strings.Join(quoted, " ")
}
