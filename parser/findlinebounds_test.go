package parser

import (
	"testing"
)

// --- findLineBounds cache tests ---
//
// These tests verify that collectMarks correctly detects line boundaries
// for ^, $, =, @, :, . marks. The findLineBounds cache in collectMarks must
// produce the same results as uncached findLineBounds calls.
//
// The cache is especially critical for permissive autolinks (@/:/.): the
// dummy mark stores lineBeg/lineEnd for analyzePermissiveAutolink, and a
// stale cache could cause autolinks to expand across line boundaries.

// markFlagNames maps flag bits to readable names for error messages.
func markFlagName(f markFlags) string {
	switch f {
	case 0:
		return "none"
	case markPotentialOpener:
		return "opener"
	case markPotentialCloser:
		return "closer"
	case markPotentialOpener | markPotentialCloser:
		return "opener+closer"
	default:
		return "other"
	}
}

// findMarksByCh returns all marks with the given character, excluding
// dummy ('D') and sentinel marks.
func findMarksByCh(ms *markStacks, ch byte) []*Mark {
	var result []*Mark
	for i := range ms.marks {
		m := &ms.marks[i]
		if m.Ch == ch {
			result = append(result, m)
		}
	}
	return result
}

// fullMarkChars builds the standard set of mark characters for testing.
func fullMarkChars() [256]bool {
	var mc [256]bool
	mc['*'] = true
	mc['_'] = true
	mc['`'] = true
	mc['\\'] = true
	mc['&'] = true
	mc[';'] = true
	mc['<'] = true
	mc['>'] = true
	mc['['] = true
	mc[']'] = true
	mc['!'] = true
	mc[0] = true
	return mc
}

// --- ^ (superscript) line boundary tests ---

func TestCollectMarksSuperscriptLineBoundary(t *testing.T) {
	tests := []struct {
		name       string
		input      string
		wantMarks  int  // expected number of ^ marks
		wantCloser bool // first ^ should have markPotentialCloser
		wantOpener bool // first ^ should have markPotentialOpener
	}{
		// ^foo^ at line start: first ^ cannot be closer (at line start),
		// CAN be opener (followed by non-ws). Second ^ CAN be closer
		// (preceded by non-ws), cannot be opener (at end of input).
		{"caret at line start", "^foo^", 2, false, true},
		// x^2^ mid-line: first ^ can be both (preceded by x, followed by 2).
		// Second ^ can only be closer (at end).
		{"caret mid-line", "x^2^", 2, true, true},
		// Multi-line: each line's marks are independent.
		// Line 1 "x^2^": same as mid-line case.
		// Line 2 "y^3^": ^ at pos 5, lb=4 (y). off > lb so closer NOT cleared
		// by line-start check. Preceded by 'y' (non-ws) so closer kept.
		{"multi-line carets", "x^2^\ny^3^", 4, true, true},
		// ^ at line start after newline: off=5, lb=5. off <= lb → closer cleared.
		// isWhitespaceBefore also true (\n before). Opener kept (followed by 'f').
		{"caret after newline only", "text\n^foo^", 2, false, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ms := &markStacks{}
			ms.reset()
			mc := fullMarkChars()
			mc['^'] = true
			collectMarks(ms, []byte(tt.input), &mc, newCompatConfig(FlagSuperscripts), FlagSuperscripts)

			carets := findMarksByCh(ms, '^')
			if len(carets) != tt.wantMarks {
				t.Fatalf("expected %d ^ marks, got %d (marks: %+v)", tt.wantMarks, len(carets), carets)
			}
			first := carets[0]
			hasCloser := first.Flags&markPotentialCloser != 0
			hasOpener := first.Flags&markPotentialOpener != 0
			if hasCloser != tt.wantCloser {
				t.Errorf("first ^ closer flag: got %v, want %v (flags=%s)",
					hasCloser, tt.wantCloser, markFlagName(first.Flags))
			}
			if hasOpener != tt.wantOpener {
				t.Errorf("first ^ opener flag: got %v, want %v (flags=%s)",
					hasOpener, tt.wantOpener, markFlagName(first.Flags))
			}
		})
	}
}

// --- $ (LaTeX math) line boundary tests ---

func TestCollectMarksDollarLineBoundary(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantMarks int
		// For the first $ mark, check opener flag.
		// $ at line start (off == lb): off > lb is false → opener check
		// NOT applied → opener kept.
		// $ mid-line preceded by alnum (off > lb, not ws, not punct):
		// opener cleared. If also followed by alnum: closer cleared → no mark.
		// $ preceded by punct: opener kept (punct doesn't block opener).
		// $ preceded by ws: opener kept (ws doesn't block opener).
		wantOpener bool
	}{
		// $x$ at line start: first $ at pos 0, lb=0. off > lb? No → opener kept.
		// Followed by 'x' (alnum) → closer cleared. f = opener only.
		{"dollar at line start", "$x$", 2, true},
		// x$y$z mid-line: $ at pos 1 preceded by 'x' (alnum) → opener cleared.
		// Followed by 'y' (alnum) → closer cleared. f = 0 → NO mark created.
		{"dollar mid-line alnum (no mark)", "x$y$z", 0, false},
		// ($x$) — $ preceded by '(' (punct) → opener kept.
		// Followed by 'x' (alnum) → closer cleared. f = opener only.
		{"dollar after punct", "($x$)", 2, true},
		// Multi-line: $x$\n$y$
		// Line 2: $ at pos 4, lb=4. off > lb? No → opener kept.
		{"multi-line dollar", "$x$\n$y$", 4, true},
		// text $x$: $ at pos 5, lb=0. off > lb? Yes.
		// Preceded by ' ' (ws) → !isWhitespaceBefore is false → opener kept.
		{"dollar after space", "text $x$", 2, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ms := &markStacks{}
			ms.reset()
			mc := fullMarkChars()
			mc['$'] = true
			collectMarks(ms, []byte(tt.input), &mc, newCompatConfig(FlagLatexMathSpans), FlagLatexMathSpans)

			dollars := findMarksByCh(ms, '$')
			if len(dollars) != tt.wantMarks {
				t.Fatalf("expected %d $ marks, got %d", tt.wantMarks, len(dollars))
			}
			if tt.wantMarks == 0 {
				return
			}
			first := dollars[0]
			hasOpener := first.Flags&markPotentialOpener != 0
			if hasOpener != tt.wantOpener {
				t.Errorf("first $ opener flag: got %v, want %v (flags=%s)",
					hasOpener, tt.wantOpener, markFlagName(first.Flags))
			}
		})
	}
}

// --- = (highlight) line boundary tests ---

func TestCollectMarksHighlightLineBoundary(t *testing.T) {
	tests := []struct {
		name       string
		input      string
		wantMarks  int
		wantCloser bool // first == should NOT be closer (at line start or preceded by ws)
		wantOpener bool // first == should be opener (followed by non-ws)
	}{
		// ==hl== at line start: first == at pos 0, lb=0.
		// off <= lb → closer cleared. Followed by 'h' → opener kept.
		{"highlight at line start", "==hl==", 2, false, true},
		// x==hl==z mid-line: first == at pos 1, lb=0. off > lb.
		// Preceded by 'x' (not ws) → closer kept. Followed by 'h' → opener kept.
		{"highlight mid-line", "x==hl==z", 2, true, true},
		// Multi-line: ==a==\n==b==
		// Line 2: == at pos 5, lb=5. off <= lb → closer cleared.
		{"multi-line highlight", "==a==\n==b==", 4, false, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ms := &markStacks{}
			ms.reset()
			mc := fullMarkChars()
			mc['='] = true
			collectMarks(ms, []byte(tt.input), &mc, newCompatConfig(FlagHighlight), FlagHighlight)

			highlights := findMarksByCh(ms, '=')
			if len(highlights) != tt.wantMarks {
				t.Fatalf("expected %d = marks, got %d", tt.wantMarks, len(highlights))
			}
			first := highlights[0]
			hasCloser := first.Flags&markPotentialCloser != 0
			hasOpener := first.Flags&markPotentialOpener != 0
			if hasCloser != tt.wantCloser {
				t.Errorf("first = closer flag: got %v, want %v (flags=%s)",
					hasCloser, tt.wantCloser, markFlagName(first.Flags))
			}
			if hasOpener != tt.wantOpener {
				t.Errorf("first = opener flag: got %v, want %v (flags=%s)",
					hasOpener, tt.wantOpener, markFlagName(first.Flags))
			}
		})
	}
}

// --- @ (permissive email autolink) line boundary tests ---
// These are the most critical: the dummy mark stores lineBeg/lineEnd
// for analyzePermissiveAutolink. A stale cache could cause autolinks
// to cross line boundaries.

func TestCollectMarksAtSignLineBoundary(t *testing.T) {
	// foo@bar.com on line 1, "baz" on line 2.
	// The @ mark should only be created if off+3 < lineEnd (enough room
	// on the same line). The dummy mark should store line 1's bounds.
	input := "foo@bar.com\nbaz"

	ms := &markStacks{}
	ms.reset()
	mc := fullMarkChars()
	mc['@'] = true
	collectMarks(ms, []byte(input), &mc, newCompatConfig(FlagPermissiveEmailAutolinks), FlagPermissiveEmailAutolinks)

	atMarks := findMarksByCh(ms, '@')
	if len(atMarks) != 1 {
		t.Fatalf("expected 1 @ mark, got %d", len(atMarks))
	}

	// The @ mark should be followed by a dummy ('D') mark storing
	// the correct line boundaries [0, 11) (line 1 is "foo@bar.com").
	at := atMarks[0]
	// Find the dummy mark right after @
	var dummy *Mark
	for i := range ms.marks {
		m := &ms.marks[i]
		if m.Ch == 'D' && m.Beg <= at.Beg {
			dummy = m
		}
	}
	if dummy == nil {
		t.Fatal("expected a dummy ('D') mark associated with @ mark")
	}
	// Dummy mark's Beg/End should be line boundaries: [0, 11)
	// (line 1 = "foo@bar.com", \n at position 11)
	if dummy.Beg != 0 {
		t.Errorf("dummy mark lineBeg: got %d, want 0", dummy.Beg)
	}
	if dummy.End != 11 {
		t.Errorf("dummy mark lineEnd: got %d, want 11 (position of \\n)", dummy.End)
	}
}

func TestCollectMarksAtSignDoesNotCrossLineBoundary(t *testing.T) {
	// a@b\ncom — @ at position 1, line 1 = "a@b" (len 3).
	// off+3 = 4, lineEnd = 3. 4 < 3 is false → NO mark should be created
	// (not enough room on the same line for a valid email autolink).
	input := "a@b\ncom"

	ms := &markStacks{}
	ms.reset()
	mc := fullMarkChars()
	mc['@'] = true
	collectMarks(ms, []byte(input), &mc, newCompatConfig(FlagPermissiveEmailAutolinks), FlagPermissiveEmailAutolinks)

	atMarks := findMarksByCh(ms, '@')
	if len(atMarks) != 0 {
		t.Errorf("expected 0 @ marks (not enough room on line), got %d", len(atMarks))
	}
}

// --- Cache consistency: multiple marks on same line and across lines ---

func TestFindLineBoundsCacheConsistency(t *testing.T) {
	// Input with multiple mark characters on the same line and across lines.
	// ^ at start, $ in middle, = at end — all on different lines.
	input := "^foo$ ==bar==\n^baz$\n==qux=="

	ms := &markStacks{}
	ms.reset()
	mc := fullMarkChars()
	mc['^'] = true
	mc['$'] = true
	mc['='] = true
	collectMarks(ms, []byte(input), &mc, newCompatConfig(FlagSuperscripts|FlagLatexMathSpans|FlagHighlight), FlagSuperscripts|FlagLatexMathSpans|FlagHighlight)

	// Verify that every mark (except dummy and sentinel) is within the
	// line bounds returned by a fresh findLineBounds call. If the cache
	// produced stale bounds, marks might have incorrect flanking flags
	// or dummy marks might store wrong line boundaries.
	for i := range ms.marks {
		m := &ms.marks[i]
		if m.Ch == 'D' || m.Ch == 127 {
			continue
		}
		lb, le := findLineBounds([]byte(input), m.Beg)
		if m.Beg < lb || m.Beg >= le {
			t.Errorf("mark %q at %d: findLineBounds returned [%d, %d), mark outside range",
				m.Ch, m.Beg, lb, le)
		}
	}

	// Also verify dummy marks store correct line boundaries.
	for i := range ms.marks {
		m := &ms.marks[i]
		if m.Ch != 'D' {
			continue
		}
		// Dummy mark Beg/End should match findLineBounds for any position
		// within that line.
		lb, le := findLineBounds([]byte(input), m.Beg)
		if m.Beg != lb {
			t.Errorf("dummy mark lineBeg: got %d, want %d", m.Beg, lb)
		}
		if m.End != le {
			t.Errorf("dummy mark lineEnd: got %d, want %d", m.End, le)
		}
	}
}

// TestFindLineBoundsCacheMultiLineAutolink verifies that permissive URL
// autolinks on different lines get correct line boundaries in their
// dummy marks. This is the key test for cache correctness: if the cache
// is stale, autolinks on line 2 might get line 1's boundaries.
func TestFindLineBoundsCacheMultiLineAutolink(t *testing.T) {
	// Two lines, each with a URL autolink candidate.
	// Line 1: "see http://a.com" → : mark at position 8
	// Line 2: "and http://b.com" → : mark at position 21
	input := "see http://a.com\nand http://b.com"

	ms := &markStacks{}
	ms.reset()
	mc := fullMarkChars()
	mc[':'] = true
	collectMarks(ms, []byte(input), &mc, newCompatConfig(FlagPermissiveURLAutolinks), FlagPermissiveURLAutolinks)

	colonMarks := findMarksByCh(ms, ':')
	if len(colonMarks) != 2 {
		t.Fatalf("expected 2 : marks (one per line), got %d", len(colonMarks))
	}

	// Each : mark should have an associated dummy mark with the correct
	// line boundaries for its line.
	// Line 1: [0, 16) (\n at position 16)
	// Line 2: [17, 33) (end of input at 33)
	wantLineBounds := []struct {
		lineBeg int
		lineEnd int
	}{
		{0, 16},  // "see http://a.com"
		{17, 33}, // "and http://b.com"
	}

	dummyIdx := 0
	for i := range ms.marks {
		m := &ms.marks[i]
		if m.Ch != 'D' {
			continue
		}
		if dummyIdx >= len(wantLineBounds) {
			t.Fatalf("unexpected extra dummy mark at index %d", dummyIdx)
		}
		want := wantLineBounds[dummyIdx]
		if m.Beg != want.lineBeg {
			t.Errorf("dummy mark %d lineBeg: got %d, want %d", dummyIdx, m.Beg, want.lineBeg)
		}
		if m.End != want.lineEnd {
			t.Errorf("dummy mark %d lineEnd: got %d, want %d", dummyIdx, m.End, want.lineEnd)
		}
		dummyIdx++
	}
	if dummyIdx != len(wantLineBounds) {
		t.Errorf("expected %d dummy marks, got %d", len(wantLineBounds), dummyIdx)
	}
}
