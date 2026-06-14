package cmd

import (
	"strings"
	"testing"
)

// withEvalFlags swaps the package-level eval flag values for the duration of a
// test and restores them via t.Cleanup. runEval reads these globals directly
// (cobra binds them in init()), so tests drive it the same way the CLI does.
func withEvalFlags(t *testing.T, ci bool, threshold float64, topN int) {
	t.Helper()
	prevCI, prevThreshold, prevTopN := evalCI, evalThreshold, evalTopN
	evalCI = ci
	evalThreshold = threshold
	evalTopN = topN
	t.Cleanup(func() {
		evalCI = prevCI
		evalThreshold = prevThreshold
		evalTopN = prevTopN
	})
}

// TestRunEvalRequiresCIMode covers the early guard: without --ci the command is
// a no-op that returns an instructive error and never touches the store.
func TestRunEvalRequiresCIMode(t *testing.T) {
	withEvalFlags(t, false, 0.70, 5)

	err := runEval(evalCmd, nil)
	if err == nil {
		t.Fatal("runEval without --ci should return an error, got nil")
	}
	if !strings.Contains(err.Error(), "--ci") {
		t.Errorf("runEval non-CI error = %q, want substring '--ci'", err.Error())
	}
}

// TestRunEvalCIPassesAtDefaultThreshold drives the full --ci happy path: temp
// DB, fixture seed, FTS run, per-result rendering, and the summary line. The
// embedded fixture corpus is designed to clear the default 0.70 threshold, so
// runEval must return nil. Requires the sqlite_fts5 build tag (the Makefile and
// CI both set it); SearchFTS needs the FTS5 virtual table.
func TestRunEvalCIPassesAtDefaultThreshold(t *testing.T) {
	withEvalFlags(t, true, 0.70, 5)

	finish := captureStdout(t)
	err := runEval(evalCmd, nil)
	out := finish()

	if err != nil {
		t.Fatalf("runEval --ci at default threshold = %v, want nil", err)
	}
	if !strings.Contains(out, "passed") {
		t.Errorf("runEval --ci output missing summary 'passed' line.\nGot:\n%s", out)
	}
	if !strings.Contains(out, "threshold") {
		t.Errorf("runEval --ci output missing 'threshold' in summary.\nGot:\n%s", out)
	}
}

// TestRunEvalCIFailsAboveAchievableThreshold forces the below-threshold branch
// by demanding a pass rate the fixture corpus cannot reach (>100%). This drives
// the rep.Meets(threshold) == false return path and the FAIL rendering for any
// non-passing question, without depending on the exact fixture pass rate.
func TestRunEvalCIFailsAboveAchievableThreshold(t *testing.T) {
	withEvalFlags(t, true, 1.01, 5)

	finish := captureStdout(t)
	err := runEval(evalCmd, nil)
	out := finish()

	if err == nil {
		t.Fatal("runEval --ci with an unreachable threshold should return a below-threshold error, got nil")
	}
	if !strings.Contains(err.Error(), "below threshold") {
		t.Errorf("runEval error = %q, want substring 'below threshold'", err.Error())
	}
	if !strings.Contains(out, "passed") {
		t.Errorf("runEval should still print the summary line before failing.\nGot:\n%s", out)
	}
}
