package diffcheck

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"
)

const testTimeout = 10 * time.Second

// TestDiffCheckFuzzMd4cAlignment verifies md4go aligns with md4c (the reference
// implementation) on fuzz seeds in loose normalization mode.
//
// Known differences where md4go is MORE correct than md4c (documented as S-cases):
//   - S-02: tight list paragraph separator (md4go preserves word boundaries)
//   - S-05: footnote reference [N] output (md4go outputs [N])
//   - S-06: NULL character handling (md4go follows CommonMark: NULL→U+FFFD)
//   - S-07: code span backtick limit (md4go supports 1024, md4c limited to 32 — non-standard)
//
// These are tracked via the maxMd4cDiffs threshold. The test fails only if
// the diff count EXCEEDS the known baseline, catching regressions.
//
// Goldmark diffs are logged as diagnostics but do not cause test failure
// (goldmark has known architectural differences: DOM traversal side effects,
// CDATA handling, image text stripping).
func TestDiffCheckFuzzMd4cAlignment(t *testing.T) {
	cases := LoadFuzzSeeds()
	runMd4cAlignmentCheck(t, cases, NormalizeLoose, 4, "default GFM")
}

// TestDiffCheckConstructedMd4cAlignment verifies md4go aligns with md4c on
// programmatically constructed test cases (nesting sweeps, delimiter lengths,
// structural boundaries). Must pass with zero md4go≠md4c diffs.
func TestDiffCheckConstructedMd4cAlignment(t *testing.T) {
	cases := LoadFuzzConstructed()
	runMd4cAlignmentCheck(t, cases, NormalizeLoose, 0, "constructed")
}

// TestDiffCheckCommonMarkAlignment verifies md4go aligns with md4c in
// CommonMark-only mode (no extensions) on fuzz seeds. Ensures the core
// CommonMark parsing is aligned without extension interference.
func TestDiffCheckCommonMarkAlignment(t *testing.T) {
	cases := LoadFuzzSeeds()
	engines := []Engine{
		NewMd4goEngine(testTimeout, WithMd4goFlags(DialectCommonMarkFlags)),
	}
	if md4cEng, err := NewMd4cEngine(testTimeout, WithMd4cCommonMark()); err == nil {
		engines = append(engines, md4cEng)
	} else {
		t.Logf("md4c engine unavailable: %v", err)
	}
	runMd4cAlignmentCheckWithEngines(t, cases, NormalizeLoose, engines, 3, "CommonMark")
}

// TestDiffCheckStrictDiagnostics runs fuzz seeds in strict normalization mode
// and reports all diffs as diagnostics. Strict mode captures whitespace/linebreak
// differences which are expected between implementations. This test does NOT fail
// — it only logs diffs for diagnostic purposes.
func TestDiffCheckStrictDiagnostics(t *testing.T) {
	cases := LoadFuzzSeeds()
	runDiagnosticCheck(t, cases, NormalizeStrict)
}

// TestDiffCheckGoldmarkDiagnostics runs fuzz seeds with GoldmarkCompat flags
// and reports md4go≠goldmark diffs as diagnostics. This test does NOT fail —
// it logs remaining goldmark diffs for diagnostic purposes.
func TestDiffCheckGoldmarkDiagnostics(t *testing.T) {
	cases := LoadFuzzSeeds()
	engines := []Engine{
		NewMd4goEngine(testTimeout, WithMd4goFlags(DialectGitHubFlags|GoldmarkCompatFlags)),
		NewGoldmarkEngine(testTimeout),
	}
	runDiagnosticCheckWithEngines(t, cases, NormalizeLoose, engines)
}

// TestDiffCheckJSONLAlignment runs the full JSONL dataset (10350 cases) against
// md4go and md4c in loose mode. Skipped in short mode or if data is missing.
// The known diff baseline is 60 (S-01~S-07 documented differences).
func TestDiffCheckJSONLAlignment(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping JSONL test in short mode")
	}

	path := DefaultJSONLPath()
	if _, err := os.Stat(path); err != nil {
		t.Skipf("JSONL data not found at %s: %v", path, err)
	}

	cases, err := LoadJSONL(path)
	if err != nil {
		t.Fatalf("load jsonl: %v", err)
	}
	if len(cases) == 0 {
		t.Fatal("no cases loaded from JSONL")
	}

	runMd4cAlignmentCheck(t, cases, NormalizeLoose, 64, "JSONL default")
}

// runMd4cAlignmentCheck runs md4go and md4c engines and fails if md4go≠md4c
// diff count exceeds maxDiffs. Goldmark is included for diagnostic comparison
// but its diffs don't cause failure.
func runMd4cAlignmentCheck(t *testing.T, cases []TestCase, mode NormalizeMode, maxDiffs int, label string) {
	t.Helper()
	engines := initEngines(t)
	runMd4cAlignmentCheckWithEngines(t, cases, mode, engines, maxDiffs, label)
}

// runMd4cAlignmentCheckWithEngines runs the given engines and fails only if
// md4go≠md4c diffs exceed maxDiffs. Other diffs are logged as diagnostics.
func runMd4cAlignmentCheckWithEngines(t *testing.T, cases []TestCase, mode NormalizeMode, engines []Engine, maxDiffs int, label string) {
	t.Helper()
	if len(engines) < 2 {
		t.Fatal("need at least 2 engines")
	}

	md4cDiffCount := 0
	goldmarkDiffCount := 0
	for _, tc := range cases {
		cr := RunCase(context.Background(), engines, tc, mode)

		for engName, err := range cr.Errors {
			if err != nil {
				if IsTimeout(err) {
					t.Logf("Case #%d [%s] %s: timeout", tc.Index, tc.Source, engName)
				} else {
					t.Logf("Case #%d [%s] %s: %v", tc.Index, tc.Source, engName, err)
				}
			}
		}

		// Check for md4go≠md4c diffs — these are the reference alignment
		for _, p := range cr.Pairs {
			if p == nil || p.Same {
				continue
			}
			if isMd4goMd4cPair(p.NameA, p.NameB) {
				md4cDiffCount++
				// Log all md4go≠md4c diffs for diagnostic purposes
				t.Logf("Case #%d [%s]: md4go≠md4c diff (normalize=%v)\n%s",
					tc.Index, tc.Source, mode, FormatCaseReport(cr))
			} else {
				goldmarkDiffCount++
			}
		}
	}

	t.Logf("[%s] %d cases, md4go≠md4c: %d (max allowed: %d), goldmark diffs: %d (normalize=%v)",
		label, len(cases), md4cDiffCount, maxDiffs, goldmarkDiffCount, mode)

	if md4cDiffCount > maxDiffs {
		t.Errorf("[%s] md4go≠md4c diff count %d exceeds maximum %d — new regression detected",
			label, md4cDiffCount, maxDiffs)
	}
}

// runDiagnosticCheck runs all engines and logs all diffs without failing.
func runDiagnosticCheck(t *testing.T, cases []TestCase, mode NormalizeMode) {
	t.Helper()
	engines := initEngines(t)
	runDiagnosticCheckWithEngines(t, cases, mode, engines)
}

// runDiagnosticCheckWithEngines runs the given engines and logs all diffs
// without failing. Used for diagnostic/informational tests.
func runDiagnosticCheckWithEngines(t *testing.T, cases []TestCase, mode NormalizeMode, engines []Engine) {
	t.Helper()
	if len(engines) < 2 {
		t.Fatal("need at least 2 engines")
	}

	diffCount := 0
	for _, tc := range cases {
		cr := RunCase(context.Background(), engines, tc, mode)

		for engName, err := range cr.Errors {
			if err != nil {
				if IsTimeout(err) {
					t.Logf("Case #%d [%s] %s: timeout", tc.Index, tc.Source, engName)
				} else {
					t.Logf("Case #%d [%s] %s: %v", tc.Index, tc.Source, engName, err)
				}
			}
		}

		if cr.HasDiff() {
			diffCount++
			// Only log first 10 diffs to avoid flooding output
			if diffCount <= 10 {
				t.Logf("Case #%d [%s]: diff (diagnostic)\n%s",
					tc.Index, tc.Source, FormatCaseReport(cr))
			}
		}
	}

	t.Logf("%d cases, %d diffs (diagnostic, normalize=%v)", len(cases), diffCount, mode)
}

// isMd4goMd4cPair returns true if the pair is between md4go and md4c engines.
func isMd4goMd4cPair(nameA, nameB string) bool {
	return (strings.HasPrefix(nameA, "md4go") && strings.HasPrefix(nameB, "md4c")) ||
		(strings.HasPrefix(nameA, "md4c") && strings.HasPrefix(nameB, "md4go"))
}

func initEngines(t *testing.T) []Engine {
	t.Helper()

	var engines []Engine

	engines = append(engines, NewMd4goEngine(testTimeout))

	md4cEng, err := NewMd4cEngine(testTimeout)
	if err != nil {
		t.Logf("md4c engine unavailable: %v", err)
	} else {
		engines = append(engines, md4cEng)
	}

	engines = append(engines, NewGoldmarkEngine(testTimeout))

	return engines
}
