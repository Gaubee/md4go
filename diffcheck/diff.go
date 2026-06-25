package diffcheck

import (
	"fmt"
	"strings"
)

// ─── Normalize ───────────────────────────────────────────────────────────────

// NormalizeMode controls how output is normalized before comparison.
type NormalizeMode int

const (
	// NormalizeStrict only trims trailing whitespace lines.
	// Preserves line break / paragraph semantics.
	NormalizeStrict NormalizeMode = iota
	// NormalizeLoose collapses all whitespace into single spaces.
	// Suitable for comparing goldmark output which loses line-break semantics.
	NormalizeLoose
)

// Normalize applies the given normalization mode to an engine's output.
func Normalize(output string, mode NormalizeMode) string {
	switch mode {
	case NormalizeStrict:
		lines := strings.Split(output, "\n")
		end := len(lines)
		for end > 0 && strings.TrimSpace(lines[end-1]) == "" {
			end--
		}
		if end == 0 {
			return ""
		}
		return strings.Join(lines[:end], "\n")
	case NormalizeLoose:
		return strings.Join(strings.Fields(output), " ")
	default:
		return output
	}
}

// ─── LineDiff ────────────────────────────────────────────────────────────────

// DiffLine represents one line in a unified diff.
type DiffLine struct {
	Kind  byte   // ' ' (common), '-' (only in A), '+' (only in B)
	LineA int    // 1-based line number in A (0 if not in A)
	LineB int    // 1-based line number in B (0 if not in B)
	Text  string // line content
}

// DiffResult holds the pairwise diff between two engine outputs.
type DiffResult struct {
	NameA   string
	NameB   string
	Lines   []DiffLine
	Same    bool
	OutputA string // normalized output of A
	OutputB string // normalized output of B
}

// Compare normalizes both outputs and produces a line-by-line diff.
func Compare(nameA, outputA, nameB, outputB string, mode NormalizeMode) *DiffResult {
	normA := Normalize(outputA, mode)
	normB := Normalize(outputB, mode)

	dr := &DiffResult{
		NameA:   nameA,
		NameB:   nameB,
		OutputA: normA,
		OutputB: normB,
		Same:    normA == normB,
	}
	if dr.Same {
		return dr
	}

	linesA := splitLines(normA)
	linesB := splitLines(normB)
	dr.Lines = lcsDiff(linesA, linesB)
	return dr
}

// splitLines splits text into lines, keeping content without newlines.
func splitLines(s string) []string {
	if s == "" {
		return nil
	}
	return strings.Split(s, "\n")
}

// lcsDiff computes an LCS-based unified diff.
// For the scale we deal with (plaintext output, typically <1000 lines),
// O(n*m) DP is perfectly fine.
func lcsDiff(a, b []string) []DiffLine {
	m, n := len(a), len(b)

	// Build LCS table
	dp := make([][]int, m+1)
	for i := range dp {
		dp[i] = make([]int, n+1)
	}
	for i := 1; i <= m; i++ {
		for j := 1; j <= n; j++ {
			if a[i-1] == b[j-1] {
				dp[i][j] = dp[i-1][j-1] + 1
			} else if dp[i-1][j] >= dp[i][j-1] {
				dp[i][j] = dp[i-1][j]
			} else {
				dp[i][j] = dp[i][j-1]
			}
		}
	}

	// Backtrack to produce diff lines
	type op struct {
		kind  byte
		text  string
		lineA int
		lineB int
	}
	var ops []op

	i, j := m, n
	for i > 0 || j > 0 {
		if i > 0 && j > 0 && a[i-1] == b[j-1] {
			ops = append(ops, op{' ', a[i-1], i, j})
			i--
			j--
		} else if j > 0 && (i == 0 || dp[i][j-1] >= dp[i-1][j]) {
			ops = append(ops, op{'+', b[j-1], 0, j})
			j--
		} else {
			ops = append(ops, op{'-', a[i-1], i, 0})
			i--
		}
	}

	// Reverse into forward order
	result := make([]DiffLine, len(ops))
	for k := 0; k < len(ops); k++ {
		o := ops[len(ops)-1-k]
		result[k] = DiffLine{Kind: o.kind, LineA: o.lineA, LineB: o.lineB, Text: o.text}
	}
	return result
}

// ─── Report ──────────────────────────────────────────────────────────────────

// CaseResult holds the results for a single test case.
type CaseResult struct {
	Index  int    // global 0-based case index
	Source string // "jsonl" or "fuzz"
	Input  []byte // original markdown input

	Outputs map[string]string // engine name → normalized output

	// Pairwise diffs (populated when len(engines) >= 2)
	Pairs []*DiffResult

	// Error info per engine
	Errors map[string]error // engine name → error (nil if success)
}

// HasDiff returns true if any pair differs.
func (cr *CaseResult) HasDiff() bool {
	for _, p := range cr.Pairs {
		if p != nil && !p.Same {
			return true
		}
	}
	return false
}

// HasError returns true if any engine produced an error.
func (cr *CaseResult) HasError() bool {
	for _, err := range cr.Errors {
		if err != nil {
			return true
		}
	}
	return false
}

// escapePreview produces a readable preview of raw input, showing \n, \t etc.
func escapePreview(s string, maxLen int) string {
	s = strings.ReplaceAll(s, "\n", `\n`)
	s = strings.ReplaceAll(s, "\r", `\r`)
	s = strings.ReplaceAll(s, "\t", `\t`)
	if len(s) > maxLen {
		return s[:maxLen] + "..."
	}
	return s
}

// FormatCaseReport produces a human-readable diff report for a single case.
func FormatCaseReport(cr *CaseResult) string {
	var b strings.Builder

	// Header
	fmt.Fprintf(&b, "━━━ Case #%d [%s] ━━━\n", cr.Index, cr.Source)

	// Input preview (escaped, first 200 chars)
	inputStr := string(cr.Input)
	fmt.Fprintf(&b, "Input: %s\n", escapePreview(inputStr, 200))

	// Each engine's output with line numbers
	engineOrder := []string{"goldmark", "md4go", "md4go-html", "md4go-html(goldmark-compat)", "md4c"}
	for _, name := range engineOrder {
		output, ok := cr.Outputs[name]
		if !ok {
			continue
		}
		fmt.Fprintf(&b, "\n--- %s ---\n", name)
		if err := cr.Errors[name]; err != nil {
			fmt.Fprintf(&b, "[ERROR: %v]\n", err)
			continue
		}
		if output == "" {
			b.WriteString("(empty)\n")
			continue
		}
		for i, line := range strings.Split(output, "\n") {
			fmt.Fprintf(&b, "%d│ %s\n", i+1, line)
		}
	}

	// Pairwise diffs with context
	for _, p := range cr.Pairs {
		if p == nil {
			continue
		}
		fmt.Fprintf(&b, "\n△ %s vs %s", p.NameA, p.NameB)
		if p.Same {
			b.WriteString(": identical\n")
			continue
		}
		b.WriteString("\n")
		formatDiffLines(&b, p.Lines)
	}

	return b.String()
}

const contextLines = 1 // number of context lines around changes

// formatDiffLines formats diff lines with minimal context around changes.
func formatDiffLines(b *strings.Builder, lines []DiffLine) {
	// Find which lines are changes or adjacent to changes
	changed := make([]bool, len(lines))
	for i, dl := range lines {
		if dl.Kind != ' ' {
			for j := i - contextLines; j <= i+contextLines; j++ {
				if j >= 0 && j < len(lines) {
					changed[j] = true
				}
			}
		}
	}

	prevShown := false
	for i, dl := range lines {
		if !changed[i] {
			prevShown = false
			continue
		}
		// Insert ellipsis gap between non-adjacent shown regions
		if prevShown && i > 0 && !changed[i-1] {
			b.WriteString(" ⋮\n")
		} else if !prevShown && i > 0 && changed[i] {
			// Starting a new region after a gap
			if i > 0 {
				b.WriteString(" ⋮\n")
			}
		}
		switch dl.Kind {
		case ' ':
			fmt.Fprintf(b, " %d│ %s\n", dl.LineA, dl.Text)
		case '-':
			fmt.Fprintf(b, "-%d│ %s\n", dl.LineA, dl.Text)
		case '+':
			fmt.Fprintf(b, "+%d│ %s\n", dl.LineB, dl.Text)
		}
		prevShown = true
	}
}

// Summary holds aggregate statistics.
type Summary struct {
	TotalCases   int
	DiffCases    int
	TimeoutCases int
	ErrorCases   int
	PairStats    map[string]int // "goldmark≠md4go" → count
}

// FormatSummary produces a structured summary.
func (s *Summary) FormatSummary() string {
	var b strings.Builder
	fmt.Fprintf(&b, "Total: %d | Diff: %d | Timeout: %d | Error: %d",
		s.TotalCases, s.DiffCases, s.TimeoutCases, s.ErrorCases)
	if len(s.PairStats) > 0 {
		b.WriteString("\n")
		for k, v := range s.PairStats {
			fmt.Fprintf(&b, "  %s: %d\n", k, v)
		}
	}
	return b.String()
}
