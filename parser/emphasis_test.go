package parser

import (
	"testing"

	"github.com/userpro/md4go/ast"
)

// --- splitEmphMark tests ---

func TestSplitEmphMark(t *testing.T) {
	ms := &markStacks{}
	ms.reset()

	// Create: *** (mark at 0-3) + 2 dummy marks at [1] and [2]
	ms.addMark(0, 3, '*', markPotentialOpener|markPotentialCloser|markEmphMod3_0)
	ms.addMark(0, 0, 'D', 0)
	ms.addMark(0, 0, 'D', 0)

	// Split: take 1 char from the 3-char mark
	newIdx := ms.splitEmphMark(0, 1)

	if newIdx != 2 {
		t.Errorf("splitEmphMark(0,1) returned %d, want 2", newIdx)
	}
	// Original mark should be shortened to 0-2 (2 chars)
	if ms.marks[0].End != 2 {
		t.Errorf("original mark end = %d, want 2", ms.marks[0].End)
	}
	// New mark should be at 2-3 (1 char)
	if ms.marks[2].Beg != 2 || ms.marks[2].End != 3 {
		t.Errorf("new mark = {%d,%d}, want {2,3}", ms.marks[2].Beg, ms.marks[2].End)
	}
	// New mark should inherit the original flags
	if ms.marks[2].Ch != '*' {
		t.Errorf("new mark ch = %q, want '*'", ms.marks[2].Ch)
	}
}

func TestSplitEmphMarkTwoChars(t *testing.T) {
	ms := &markStacks{}
	ms.reset()

	// Create: **** (mark at 0-4) + 3 dummy marks
	ms.addMark(0, 4, '*', markPotentialOpener|markEmphMod3_1)
	ms.addMark(0, 0, 'D', 0)
	ms.addMark(0, 0, 'D', 0)
	ms.addMark(0, 0, 'D', 0)

	// Split: take 2 chars
	newIdx := ms.splitEmphMark(0, 2)

	if newIdx != 2 {
		t.Errorf("splitEmphMark(0,2) returned %d, want 2", newIdx)
	}
	// Original: 0-2 (2 chars)
	if ms.marks[0].End != 2 {
		t.Errorf("original mark end = %d, want 2", ms.marks[0].End)
	}
	// New: 2-4 (2 chars)
	if ms.marks[2].Beg != 2 || ms.marks[2].End != 4 {
		t.Errorf("new mark = {%d,%d}, want {2,4}", ms.marks[2].Beg, ms.marks[2].End)
	}
}

// --- analyzeEmph tests (Rule-of-3) ---

func TestAnalyzeEmphSimpleEm(t *testing.T) {
	// *foo* → <em>foo</em>
	ms := &markStacks{}
	ms.reset()
	collectMarks(ms, []byte("*foo*"), &markCharMap, 0)
	ms.analyzeMarks([]byte("*foo*"), []byte("*_"))

	// First * should be resolved as opener, second as closer
	opener := findMarkByCh(ms, '*', 0)
	closer := findMarkByCh(ms, '*', 1)

	if opener == nil || closer == nil {
		t.Fatal("missing emphasis marks")
	}
	if opener.Flags&markOpener == 0 || opener.Flags&markResolved == 0 {
		t.Errorf("opener flags = %08b, missing OPENER|RESOLVED", opener.Flags)
	}
	if closer.Flags&markCloser == 0 || closer.Flags&markResolved == 0 {
		t.Errorf("closer flags = %08b, missing CLOSER|RESOLVED", closer.Flags)
	}
	if opener.Next != closerIdx(ms, '*', 1) {
		t.Errorf("opener.Next = %d, want closer index", opener.Next)
	}
}

func TestAnalyzeEmphStrong(t *testing.T) {
	// **foo** → <strong>foo</strong>
	ms := &markStacks{}
	ms.reset()
	collectMarks(ms, []byte("**foo**"), &markCharMap, 0)
	ms.analyzeMarks([]byte("**foo**"), []byte("*_"))

	opener := findMarkByCh(ms, '*', 0)
	closer := findMarkByCh(ms, '*', 1)

	if opener == nil || closer == nil {
		t.Fatal("missing emphasis marks")
	}
	if opener.End-opener.Beg != 2 {
		t.Errorf("opener length = %d, want 2", opener.End-opener.Beg)
	}
	if closer.End-closer.Beg != 2 {
		t.Errorf("closer length = %d, want 2", closer.End-closer.Beg)
	}
}

func TestAnalyzeEmphStrongEm(t *testing.T) {
	// ***foo*** → <em><strong>foo</strong></em>
	ms := &markStacks{}
	ms.reset()
	collectMarks(ms, []byte("***foo***"), &markCharMap, 0)
	ms.analyzeMarks([]byte("***foo***"), []byte("*_"))

	opener := findMarkByCh(ms, '*', 0)
	closer := findMarkByCh(ms, '*', 1)

	if opener == nil || closer == nil {
		t.Fatal("missing emphasis marks")
	}
	if opener.End-opener.Beg != 3 {
		t.Errorf("opener length = %d, want 3", opener.End-opener.Beg)
	}
	if closer.End-closer.Beg != 3 {
		t.Errorf("closer length = %d, want 3", closer.End-closer.Beg)
	}
}

func TestAnalyzeEmphSplit(t *testing.T) {
	// ***foo** → <em>*foo</em><strong></strong>...
	// Actually: ***foo** → the first ** pairs with the closing **,
	// and the remaining * is left as an unresolved opener.
	// Expected: <strong>*foo</strong> (md4c behavior: rightmost 2 chars match)
	ms := &markStacks{}
	ms.reset()
	collectMarks(ms, []byte("***foo**"), &markCharMap, 0)
	ms.analyzeMarks([]byte("***foo**"), []byte("*_"))

	// The closer ** should be resolved
	closer := findMarkByCh(ms, '*', 1)
	if closer == nil {
		t.Fatal("missing closer mark")
	}
	if closer.Flags&markResolved == 0 {
		t.Error("closer should be resolved")
	}
}

func TestAnalyzeEmphUnderscore(t *testing.T) {
	// _foo_ → <em>foo</em>
	ms := &markStacks{}
	ms.reset()
	collectMarks(ms, []byte("_foo_"), &markCharMap, 0)
	ms.analyzeMarks([]byte("_foo_"), []byte("*_"))

	opener := findMarkByCh(ms, '_', 0)
	closer := findMarkByCh(ms, '_', 1)

	if opener == nil || closer == nil {
		t.Fatal("missing emphasis marks")
	}
	if opener.Flags&markOpener == 0 || opener.Flags&markResolved == 0 {
		t.Errorf("opener flags = %08b, missing OPENER|RESOLVED", opener.Flags)
	}
	if closer.Flags&markCloser == 0 || closer.Flags&markResolved == 0 {
		t.Errorf("closer flags = %08b, missing CLOSER|RESOLVED", closer.Flags)
	}
}

func TestAnalyzeEmphIntraWordUnderscore(t *testing.T) {
	// foo_bar → not emphasis (intra-word underscore)
	ms := &markStacks{}
	ms.reset()
	collectMarks(ms, []byte("foo_bar"), &markCharMap, 0)
	ms.analyzeMarks([]byte("foo_bar"), []byte("*_"))

	// No marks should be resolved as emphasis
	for _, m := range ms.marks {
		if m.Ch == '_' && m.Flags&markResolved != 0 && (m.Flags&markOpener != 0 || m.Flags&markCloser != 0) {
			t.Errorf("intra-word underscore should not be resolved: flags=%08b", m.Flags)
		}
	}
}

func TestAnalyzeEmphRuleOf3(t *testing.T) {
	// Rule-of-3 test: ***bar*** should produce <em><strong>bar</strong></em>
	// because *** length=3, mod3=0, and the opener/closer have matching MOD3.
	// This is the basic Rule-of-3 case.
	ms := &markStacks{}
	ms.reset()
	collectMarks(ms, []byte("***bar***"), &markCharMap, 0)
	ms.analyzeMarks([]byte("***bar***"), []byte("*_"))

	opener := findMarkByCh(ms, '*', 0)
	closer := findMarkByCh(ms, '*', 1)
	if opener == nil || closer == nil {
		t.Fatal("missing emphasis marks")
	}
	if opener.Flags&markResolved == 0 || closer.Flags&markResolved == 0 {
		t.Error("both marks should be resolved")
	}
}

func TestAnalyzeEmphUnresolved(t *testing.T) {
	// *foo → no closer, opener stays unresolved
	ms := &markStacks{}
	ms.reset()
	collectMarks(ms, []byte("*foo"), &markCharMap, 0)
	ms.analyzeMarks([]byte("*foo"), []byte("*_"))

	opener := findMarkByCh(ms, '*', 0)
	if opener == nil {
		t.Fatal("missing opener mark")
	}
	if opener.Flags&markResolved != 0 {
		t.Error("unpaired opener should not be resolved")
	}
}

// --- analyzeTilde tests ---

func TestAnalyzeTildeStrikethrough(t *testing.T) {
	// ~~strike~~ → <del>strike</del>
	ms := &markStacks{}
	ms.reset()
	mc := buildMarkChars(FlagStrikethrough)
	collectMarks(ms, []byte("~~strike~~"), &mc, FlagStrikethrough)
	ms.analyzeMarks([]byte("~~strike~~"), []byte("~"))

	opener := findMarkByCh(ms, '~', 0)
	closer := findMarkByCh(ms, '~', 1)
	if opener == nil || closer == nil {
		t.Fatal("missing tilde marks")
	}
	if opener.Flags&markOpener == 0 || closer.Flags&markCloser == 0 {
		t.Error("tilde marks should be resolved as opener/closer")
	}
}

// --- analyzeEntity tests ---

func TestAnalyzeEntity(t *testing.T) {
	// &amp; → entity
	ms := &markStacks{}
	ms.reset()
	collectMarks(ms, []byte("&amp;"), &markCharMap, 0)
	ms.analyzeMarks([]byte("&amp;"), []byte("&"))

	ampMark := findMarkByCh(ms, '&', 0)
	if ampMark == nil {
		t.Fatal("missing & mark")
	}
	if ampMark.Flags&markResolved == 0 || ampMark.Flags&markOpener == 0 {
		t.Errorf("& mark flags = %08b, expected OPENER|RESOLVED", ampMark.Flags)
	}
}

func TestAnalyzeEntityInvalid(t *testing.T) {
	// &invalid (no semicolon) → not resolved as entity
	ms := &markStacks{}
	ms.reset()
	collectMarks(ms, []byte("&invalid"), &markCharMap, 0)
	ms.analyzeMarks([]byte("&invalid"), []byte("&"))

	ampMark := findMarkByCh(ms, '&', 0)
	if ampMark == nil {
		t.Fatal("missing & mark")
	}
	if ampMark.Flags&markResolved != 0 {
		t.Error("invalid entity should not be resolved")
	}
}

// --- resolveEmphSpanType tests ---

func TestResolveEmphSpanType(t *testing.T) {
	tests := []struct {
		markLen  int
		isOpener bool
		want     []ast.SpanType
	}{
		{1, true, []ast.SpanType{ast.SpanEm}},
		{2, true, []ast.SpanType{ast.SpanStrong}},
		{3, true, []ast.SpanType{ast.SpanEm, ast.SpanStrong}},
		{4, true, []ast.SpanType{ast.SpanStrong, ast.SpanStrong}},
		{5, true, []ast.SpanType{ast.SpanEm, ast.SpanStrong, ast.SpanStrong}},
		{1, false, []ast.SpanType{ast.SpanEm}},
		{2, false, []ast.SpanType{ast.SpanStrong}},
		{3, false, []ast.SpanType{ast.SpanStrong, ast.SpanEm}},
		{4, false, []ast.SpanType{ast.SpanStrong, ast.SpanStrong}},
		{5, false, []ast.SpanType{ast.SpanStrong, ast.SpanStrong, ast.SpanEm}},
	}
	for _, tt := range tests {
		got := resolveEmphSpanType(tt.markLen, tt.isOpener)
		if len(got) != len(tt.want) {
			t.Errorf("resolveEmphSpanType(%d, %v) = %v, want %v", tt.markLen, tt.isOpener, got, tt.want)
			continue
		}
		for i := range got {
			if got[i] != tt.want[i] {
				t.Errorf("resolveEmphSpanType(%d, %v)[%d] = %v, want %v", tt.markLen, tt.isOpener, i, got[i], tt.want[i])
			}
		}
	}
}

// --- analyzeMarks integration tests ---

func TestAnalyzeMarksMixedEmphasis(t *testing.T) {
	tests := []struct {
		name  string
		input string
		// Check that we get the expected number of resolved opener/closer pairs
		// Each * or _ run (including split marks) counts as one mark;
		// a "resolved pair" = one resolved opener mark.
		// Note: Rule-of-3 prevents * from matching with ** (different MOD3).
		wantResolvedPairs int
	}{
		{"em", "*foo*", 1},            // *opener + *closer → 1 pair
		{"strong", "**foo**", 1},      // **opener + **closer → 1 pair
		{"em+strong", "***foo***", 1}, // ***opener + ***closer → 1 pair (3 chars)
		// Rule-of-3: * (mod3=1) does NOT match ** (mod3=2) as opener/closer.
		// So * stays unresolved and ** pairs with **.
		{"star then 2star", "*foo**bar**", 1},
		// Similarly, ** (mod3=2) doesn't match * closer; * pairs with * only.
		{"2star then star", "**foo*bar*", 1},
		// Same-length ems do pair
		{"two em", "*foo*bar*baz*", 2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ms := &markStacks{}
			ms.reset()
			collectMarks(ms, []byte(tt.input), &markCharMap, 0)
			ms.analyzeMarks([]byte(tt.input), []byte("*_"))

			resolvedPairs := 0
			for i := range ms.marks {
				m := &ms.marks[i]
				if m.Flags&markOpener != 0 && m.Flags&markResolved != 0 && (m.Ch == '*' || m.Ch == '_') {
					resolvedPairs++
				}
			}
			if resolvedPairs != tt.wantResolvedPairs {
				t.Errorf("resolved pairs = %d, want %d", resolvedPairs, tt.wantResolvedPairs)
			}
		})
	}
}

// --- Helper functions ---

// findMarkByCh returns the nth mark with the given character (0-indexed).
func findMarkByCh(ms *markStacks, ch byte, n int) *Mark {
	count := 0
	for i := range ms.marks {
		if ms.marks[i].Ch == ch {
			if count == n {
				return &ms.marks[i]
			}
			count++
		}
	}
	return nil
}

// closerIdx returns the index of the nth mark with the given character.
func closerIdx(ms *markStacks, ch byte, n int) int {
	count := 0
	for i := range ms.marks {
		if ms.marks[i].Ch == ch {
			if count == n {
				return i
			}
			count++
		}
	}
	return -1
}

// --- Unicode flank detection tests ---
// Mirrors CommonMark spec examples 353-354: Unicode whitespace (NBSP)
// and Unicode punctuation (£, €) must be detected for emphasis flanking.

func TestUnicodeFlankWhitespace(t *testing.T) {
	// *\u00a0a\u00a0* — non-breaking space is Unicode whitespace
	// The * should NOT be left/right flanking → no emphasis
	ms := &markStacks{}
	ms.reset()
	collectMarks(ms, []byte("*\u00a0a\u00a0*"), &markCharMap, 0)

	// Find the * marks
	var starMarks []int
	for i := range ms.marks {
		if ms.marks[i].Ch == '*' {
			starMarks = append(starMarks, i)
		}
	}
	if len(starMarks) < 2 {
		t.Fatalf("expected at least 2 * marks, got %d", len(starMarks))
	}
	// Neither * should be a potential opener or closer
	// (NBSP is Unicode whitespace → both sides are whitespace → no flags)
	for _, idx := range starMarks {
		m := &ms.marks[idx]
		if m.Flags&markPotentialOpener != 0 || m.Flags&markPotentialCloser != 0 {
			t.Errorf("* at pos %d has opener/closer flags (%08b), expected none (Unicode whitespace flank)",
				m.Beg, m.Flags)
		}
	}
}

func TestUnicodeFlankPunctuation(t *testing.T) {
	// *$*alpha — $ is ASCII punctuation → no emphasis (both * are openers)
	// *£*bravo — £ is Unicode punctuation (Po) → no emphasis (both * are openers)
	// *€*charlie — € is Unicode symbol (Sc, included by md4c PUNCT_MAP) → no emphasis
	inputs := []string{
		"*$*alpha",
		"*\u00a3*\u0062ravo",   // £ (U+00A3)
		"*\u20ac*\u0063harlie", // € (U+20AC)
	}

	for _, input := range inputs {
		ms := &markStacks{}
		ms.reset()
		collectMarks(ms, []byte(input), &markCharMap, 0)
		ms.analyzeMarks([]byte(input), []byte("*"))

		// No * should be resolved as both opener and closer
		for i := range ms.marks {
			m := &ms.marks[i]
			if m.Ch == '*' && m.Flags&markOpener != 0 && m.Flags&markCloser != 0 {
				t.Errorf("input %q: * at pos %d is both opener and closer (should not pair)",
					input, m.Beg)
			}
		}
	}
}
