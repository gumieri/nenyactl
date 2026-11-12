package cmd

import (
	"bytes"
	"context"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/gumieri/nenyactl/internal/detect"
)

// Tests for the doctor watch loop and the interactive fix prompt (NCTL-47).

func TestDoctorWatchLoop(t *testing.T) {
	t.Run("loops until every check passes", func(t *testing.T) {
		runs := [][]checkResult{
			{{Name: "config", Status: checkFail}},
			{{Name: "config", Status: checkFail}},
			{{Name: "config", Status: checkOK}},
		}
		runIdx := 0
		var paintedCounts []int
		var waited []time.Duration
		ctx := context.Background()
		failed, canceled := doctorWatchLoop(ctx, 5*time.Second,
			func() []checkResult {
				r := runs[runIdx]
				runIdx++
				return r
			},
			func(_ []checkResult, failed int) { paintedCounts = append(paintedCounts, failed) },
			func(ctx context.Context, d time.Duration) bool {
				waited = append(waited, d)
				return true
			})
		if failed != 0 || canceled {
			t.Errorf("failed=%d canceled=%v, want 0/false (success exit)", failed, canceled)
		}
		if len(paintedCounts) != 3 {
			t.Errorf("paints = %d, want 3 (one per run)", len(paintedCounts))
		}
		if len(waited) != 2 {
			t.Errorf("waits = %d, want 2 (no wait after the passing run)", len(waited))
		}
		if waited[0] != 5*time.Second {
			t.Errorf("wait = %s, want the interval", waited[0])
		}
	})

	t.Run("canceled context exits with the last failure count", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		paints := 0
		failed, canceled := doctorWatchLoop(ctx, time.Second,
			func() []checkResult {
				return []checkResult{{Name: "secrets", Status: checkFail}}
			},
			func([]checkResult, int) { paints++ },
			func(ctx context.Context, _ time.Duration) bool {
				cancel() // simulate ctrl-c during the wait
				return false
			})
		if failed != 1 || !canceled {
			t.Errorf("failed=%d canceled=%v, want 1/true", failed, canceled)
		}
		if paints != 1 {
			t.Errorf("paints = %d, want 1 (exit after cancellation)", paints)
		}
	})

	t.Run("cancellation during diagnose is reported as interrupted", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel() // canceled before the run: diagnose failures are artifacts
		failed, canceled := doctorWatchLoop(ctx, time.Second,
			func() []checkResult {
				return []checkResult{{Name: "config", Status: checkFail}}
			},
			func([]checkResult, int) {},
			func(ctx context.Context, _ time.Duration) bool { return true })
		if failed != 1 || !canceled {
			t.Errorf("failed=%d canceled=%v, want 1/true (artifact failures flagged)", failed, canceled)
		}
	})
}

func TestEffectiveDoctorInterval(t *testing.T) {
	cases := []struct {
		name string
		in   time.Duration
		want time.Duration
	}{
		{"zero clamps to the floor", 0, time.Second},
		{"below floor clamps", 500 * time.Millisecond, time.Second},
		{"at floor passes", time.Second, time.Second},
		{"above floor passes", 5 * time.Second, 5 * time.Second},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			saved := doctorInterval
			doctorInterval = tc.in
			t.Cleanup(func() { doctorInterval = saved })
			if got := effectiveDoctorInterval(); got != tc.want {
				t.Errorf("effectiveDoctorInterval(%s) = %s, want %s", tc.in, got, tc.want)
			}
		})
	}
}

func TestOfferFixes(t *testing.T) {
	upFail := checkResult{Name: "config", Status: checkFail, Detail: "no config", Fix: "run `nenyactl up`", Action: actionUp}
	manualFail := checkResult{Name: "secrets", Status: checkFail, Detail: "world-readable", Fix: "chmod 600 ..."}
	okResult := checkResult{Name: "port", Status: checkOK, Detail: "free"}

	var ran []doctorAction
	run := func(ctx context.Context, a doctorAction) error {
		ran = append(ran, a)
		return nil
	}

	t.Run("y runs the nenyactl-owned fix", func(t *testing.T) {
		ran = nil
		repaired, _, err := offerFixes(context.Background(), strings.NewReader("y\n"), &bytes.Buffer{}, run, []checkResult{upFail},
			func() []checkResult { return []checkResult{okResult} })
		if err != nil {
			t.Fatal(err)
		}
		if !repaired {
			t.Error("repaired = false, want true")
		}
		if len(ran) != 1 || ran[0] != actionUp {
			t.Errorf("ran = %v, want [actionUp]", ran)
		}
	})

	t.Run("default (empty answer) declines", func(t *testing.T) {
		ran = nil
		repaired, _, err := offerFixes(context.Background(), strings.NewReader("\n"), &bytes.Buffer{}, run, []checkResult{upFail}, nil)
		if err != nil {
			t.Fatal(err)
		}
		if repaired || len(ran) != 0 {
			t.Errorf("repaired=%v ran=%v, want false/none (default is N)", repaired, ran)
		}
	})

	t.Run("n, N, and no do not run", func(t *testing.T) {
		ran = nil
		for _, answer := range []string{"n\n", "N\n", "no\n"} {
			if _, _, err := offerFixes(context.Background(), strings.NewReader(answer), &bytes.Buffer{}, run, []checkResult{upFail}, nil); err != nil {
				t.Fatal(err)
			}
		}
		if len(ran) != 0 {
			t.Errorf("ran = %v, want none", ran)
		}
	})

	t.Run("EOF on stdin declines silently and stops", func(t *testing.T) {
		ran = nil
		repaired, _, err := offerFixes(context.Background(), strings.NewReader(""), &bytes.Buffer{}, run, []checkResult{upFail}, nil)
		if err != nil {
			t.Errorf("EOF must not error (scripts gate on the exit code): %v", err)
		}
		if repaired || len(ran) != 0 {
			t.Errorf("repaired=%v ran=%v, want false/none", repaired, ran)
		}
	})

	t.Run("checks without an internal action are never prompted", func(t *testing.T) {
		ran = nil
		in := strings.NewReader("y\ny\ny\n") // would accept everything if prompted
		if _, _, err := offerFixes(context.Background(), in, &bytes.Buffer{}, run, []checkResult{manualFail, okResult}, nil); err != nil {
			t.Fatal(err)
		}
		if len(ran) != 0 {
			t.Errorf("ran = %v, want none (chmod is not nenyactl-owned)", ran)
		}
	})

	t.Run("a failed fix aborts with the command named", func(t *testing.T) {
		_, _, err := offerFixes(context.Background(), strings.NewReader("y\n"), &bytes.Buffer{},
			func(ctx context.Context, a doctorAction) error { return context.DeadlineExceeded },
			[]checkResult{upFail}, nil)
		if err == nil {
			t.Fatal("expected an error when the fix fails")
		}
		if !strings.Contains(err.Error(), "nenyactl up") {
			t.Errorf("error = %v, want the command name", err)
		}
	})

	t.Run("the same action is offered at most once, and a repairing fix clears later prompts", func(t *testing.T) {
		// config-missing and no-client-token both carry actionUp; after the
		// accepted fix, the refreshed diagnosis is healthy — no second prompt.
		ran = nil
		calls := 0
		refresh := func() []checkResult {
			calls++
			if calls == 1 {
				return []checkResult{upFail, {Name: "secrets", Status: checkFail, Detail: "no token", Fix: "run `nenyactl up`", Action: actionUp}}
			}
			return []checkResult{okResult} // post-fix: healthy
		}
		repaired, _, err := offerFixes(context.Background(), strings.NewReader("y\n"), &bytes.Buffer{}, run,
			[]checkResult{upFail, {Name: "secrets", Status: checkFail, Detail: "no token", Fix: "run `nenyactl up`", Action: actionUp}}, refresh)
		if err != nil {
			t.Fatal(err)
		}
		if !repaired {
			t.Error("repaired = false, want true")
		}
		if len(ran) != 1 {
			t.Errorf("ran = %v, want exactly one actionUp (no duplicate prompt)", ran)
		}
	})

	t.Run("prompt and offers go to the injected writer", func(t *testing.T) {
		var out bytes.Buffer
		if _, _, err := offerFixes(context.Background(), strings.NewReader("n\n"), &out, run, []checkResult{upFail}, nil); err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(out.String(), "run: nenyactl up? [y/N]") {
			t.Errorf("prompt = %q, want the command offer", out.String())
		}
	})

	t.Run("warned checks with actions are offered too", func(t *testing.T) {
		ran = nil
		warn := checkResult{Name: "port", Status: checkWarn, Detail: "free", Fix: "nenyactl up", Action: actionUp}
		repaired, _, err := offerFixes(context.Background(), strings.NewReader("y\n"), &bytes.Buffer{}, run, []checkResult{warn},
			func() []checkResult { return []checkResult{okResult} })
		if err != nil {
			t.Fatal(err)
		}
		if !repaired || len(ran) != 1 {
			t.Errorf("repaired=%v ran=%v, want the warning's fix offered", repaired, ran)
		}
	})
}

func TestActionCommand(t *testing.T) {
	if got := actionCommand(actionUp); got != "nenyactl up" {
		t.Errorf("actionCommand(actionUp) = %q, want nenyactl up", got)
	}
	if got := actionCommand(actionNone); got != "" {
		t.Errorf("actionCommand(actionNone) = %q, want empty", got)
	}
}

func TestRunDoctorActionUnknown(t *testing.T) {
	err := runDoctorAction(context.Background(), "", 0, doctorAction(42))
	if err == nil {
		t.Fatal("expected an error for an unknown action")
	}
	if !strings.Contains(err.Error(), "42") {
		t.Errorf("error = %v, want the action value identified", err)
	}
}

func TestCountFailed(t *testing.T) {
	results := []checkResult{
		{Name: "a", Status: checkOK},
		{Name: "b", Status: checkWarn},
		{Name: "c", Status: checkFail},
		{Name: "d", Status: checkFail},
	}
	if got := countFailed(results); got != 2 {
		t.Errorf("countFailed = %d, want 2 (warnings are not failures)", got)
	}
}

func TestRunDoctorRepairFlow(t *testing.T) {
	// The H-fix wiring: an accepted repair re-diagnoses and the exit status
	// reflects post-fix reality. Host interactions are stubbed hermetically.
	fails := 1
	var resolveCalls, diagnoseCalls int
	savedResolve := resolveLifecycleDirFn
	savedDiagnose := diagnoseFn
	t.Cleanup(func() { resolveLifecycleDirFn = savedResolve; diagnoseFn = savedDiagnose })
	resolveLifecycleDirFn = func(ctx context.Context, dir string) (dirResolution, error) {
		resolveCalls++
		return dirResolution{Kind: dirConfigRoot, Path: "/tmp/doctor-test", Info: &detect.Info{Mode: detect.ModeBareMetal, ConfigFile: "/tmp/doctor-test/config.json"}}, nil
	}
	diagnoseFn = func(ctx context.Context, res dirResolution, doer healthDoer) []checkResult {
		diagnoseCalls++
		if fails > 0 {
			return []checkResult{{Name: "config", Status: checkFail, Detail: "no config", Fix: "run `nenyactl up`", Action: actionUp}}
		}
		return []checkResult{{Name: "config", Status: checkOK, Detail: "present"}}
	}

	savedAction := runDoctorActionFn
	runDoctorActionFn = func(ctx context.Context, dir string, wait time.Duration, a doctorAction) error {
		fails = 0 // the "fix" repairs the deployment
		return nil
	}
	t.Cleanup(func() { runDoctorActionFn = savedAction })

	stdout := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w
	savedWatch := doctorWatch
	doctorWatch = false
	doctorCmd.SetIn(strings.NewReader("y\n"))
	t.Cleanup(func() {
		os.Stdout = stdout
		doctorCmd.SetIn(nil)
		doctorWatch = savedWatch
	})
	runErr := runDoctor(doctorCmd, nil)
	closeErr := w.Close()
	os.Stdout = stdout
	out, readErr := io.ReadAll(r)
	closeErr2 := r.Close()

	if runErr != nil {
		t.Fatalf("runDoctor = %v, want nil (repair succeeded)", runErr)
	}
	if closeErr != nil || readErr != nil || closeErr2 != nil {
		t.Fatalf("pipe handling: %v/%v/%v", closeErr, readErr, closeErr2)
	}
	if !strings.Contains(string(out), "all checks passed") {
		t.Errorf("output = %q, want the post-repair all-clear", out)
	}
	if diagnoseCalls != 2 {
		t.Errorf("diagnose calls = %d, want exactly 2 (initial + one post-fix refresh; no redundant re-diagnose)", diagnoseCalls)
	}
}
