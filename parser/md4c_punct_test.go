package parser

import (
	"testing"
)

// TestMd4cUnicodeWhitespaceAlignment verifies that our isMd4cUnicodeWhitespace
// matches md4c's WHITESPACE_MAP exactly.
func TestMd4cUnicodeWhitespaceAlignment(t *testing.T) {
	// md4c WHITESPACE_MAP: S(0x0020), S(0x00a0), S(0x1680), R(0x2000,0x200a),
	//   S(0x202f), S(0x205f), S(0x3000)
	mustBeWhitespace := []rune{
		0x0020, 0x00A0, 0x1680,
		0x2000, 0x2001, 0x2002, 0x2003, 0x2004, 0x2005, 0x2006,
		0x2007, 0x2008, 0x2009, 0x200A,
		0x202F, 0x205F, 0x3000,
	}
	for _, cp := range mustBeWhitespace {
		if !isMd4cUnicodeWhitespace(cp) {
			t.Errorf("U+%04X should be whitespace but isMd4cUnicodeWhitespace returned false", cp)
		}
	}

	// Verify some non-whitespace codepoints
	mustNotBeWhitespace := []rune{
		0x0021, // !
		0x0041, // A
		0x200B, // ZERO WIDTH SPACE (not in Zs)
		0x00AD, // SOFT HYPHEN (not in Zs)
		0xFEFF, // BOM (not in Zs)
	}
	for _, cp := range mustNotBeWhitespace {
		if isMd4cUnicodeWhitespace(cp) {
			t.Errorf("U+%04X should NOT be whitespace but isMd4cUnicodeWhitespace returned true", cp)
		}
	}
}

// TestMd4cUnicodePunctAlignment verifies that our isMd4cUnicodePunct
// matches md4c's PUNCT_MAP for key codepoints.
func TestMd4cUnicodePunctAlignment(t *testing.T) {
	// Codepoints that MUST be punctuation in md4c's PUNCT_MAP
	mustBePunct := []rune{
		// ASCII punctuation (handled separately by isASCIIPunct)
		0x0021, 0x002F, 0x003A, 0x0040, 0x005B, 0x0060, 0x007B, 0x007E,
		// Latin-1 Supplement
		0x00A1, 0x00AB, 0x00B4, 0x00B7, // ¡ « ´ ·
		0x00D7, 0x00F7, // × ÷
		// General Punctuation
		0x2010, 0x2013, 0x2014, // ‐ – —
		0x2018, 0x2019, 0x201C, 0x201D, // ' ' " "
		0x2026, // …
		0x2030, // ‰
		0x20AC, // € (in PUNCT_MAP range 0x20A0-0x20C4)
		// CJK
		0x3001, 0x3002, // 、 。
		0x30A0, 0x30FB, // ・
		// Halfwidth and Fullwidth
		0xFF01, 0xFF0F, // ！
		// Supplementary: emoji (in PUNCT_MAP range 0x1F300-0x1F6D9)
		0x1F600, // 😀
		0x1F4A9, // 💩
		// Supplementary: arrows (in PUNCT_MAP range 0x2190-0x2429)
		0x2190, 0x2192, // ← →
		// Supplementary: box drawing (in PUNCT_MAP range 0x2500-0x2775)
		0x2500, 0x2502, // ─ │
	}
	for _, cp := range mustBePunct {
		if cp <= 0x7F {
			// ASCII is handled by isASCIIPunct, skip
			continue
		}
		if !isMd4cUnicodePunct(cp) {
			t.Errorf("U+%04X should be punct but isMd4cUnicodePunct returned false", cp)
		}
	}

	// Codepoints that must NOT be punctuation
	mustNotBePunct := []rune{
		0x0041, // A (letter)
		0x0030, // 0 (digit)
		0x0100, // Ā (letter)
		0x4E00, // 一 (CJK ideograph)
	}
	for _, cp := range mustNotBePunct {
		if cp <= 0x7F {
			continue
		}
		if isMd4cUnicodePunct(cp) {
			t.Errorf("U+%04X should NOT be punct but isMd4cUnicodePunct returned true", cp)
		}
	}
}

// TestMd4cUnicodePunctBinarySearch tests the binary search implementation.
func TestMd4cUnicodePunctBinarySearch(t *testing.T) {
	// Test boundary codepoints of the first and last ranges in BMP table
	boundaryTests := []struct {
		cp       rune
		expected bool
	}{
		{0x00A0, false}, // just before first range (0x00A1-0x00A9)
		{0x00A1, true},  // first range start
		{0x00A9, true},  // first range end
		{0x00AA, false}, // just after first range
		{0xFFFC, true},  // last range in BMP (0xFFFC-0xFFFD)
		{0xFFFD, true},
		{0xFFFE, false}, // just after last BMP range
		// Supplementary plane boundaries
		{0x100FF, false}, // before first SMP range (0x10100)
		{0x10100, true},  // first SMP range start
		{0x10102, true},  // first SMP range end
		{0x1FBFA, true},  // last SMP range (0x1FBFA-0x1FBFA)
		{0x1FBFB, false}, // just after last SMP range
	}
	for _, tc := range boundaryTests {
		got := isMd4cUnicodePunct(tc.cp)
		if got != tc.expected {
			t.Errorf("U+%04X: expected %v, got %v", tc.cp, tc.expected, got)
		}
	}
}

// TestIsWhitespaceAtUnicode verifies that isWhitespaceAt correctly handles
// multi-byte Unicode whitespace characters.
func TestIsWhitespaceAtUnicode(t *testing.T) {
	tests := []struct {
		input string
		off   int
		want  bool
	}{
		// NBSP (U+00A0) — 2-byte UTF-8: C2 A0
		{"foo\xc2\xa0bar", 3, true},
		// EM SPACE (U+2003) — 3-byte UTF-8: E2 80 83
		{"foo\xe2\x80\x83bar", 3, true},
		// IDEOGRAPHIC SPACE (U+3000) — 3-byte UTF-8: E3 80 80
		{"foo\xe3\x80\x80bar", 3, true},
		// Non-whitespace: é (U+00E9) — 2-byte UTF-8: C3 A9
		{"foo\xc3\xa9bar", 3, false},
	}
	for _, tc := range tests {
		got := isWhitespaceAt([]byte(tc.input), tc.off)
		if got != tc.want {
			t.Errorf("isWhitespaceAt(%q, %d): expected %v, got %v", tc.input, tc.off, tc.want, got)
		}
	}
}

// TestIsPunctAtUnicode verifies that isPunctAt correctly handles
// multi-byte Unicode punctuation characters.
func TestIsPunctAtUnicode(t *testing.T) {
	tests := []struct {
		input string
		off   int
		want  bool
	}{
		// EM DASH (U+2014) — 3-byte UTF-8: E2 80 94
		{"foo\xe2\x80\x94bar", 3, true},
		// EURO SIGN (U+20AC) — 3-byte UTF-8: E2 82 AC
		{"foo\xe2\x82\xacbar", 3, true},
		// Non-punct: é (U+00E9) — 2-byte UTF-8: C3 A9
		{"foo\xc3\xa9bar", 3, false},
		// Non-punct: 中 (U+4E2D) — 3-byte UTF-8: E4 B8 AD
		{"foo\xe4\xb8\xadbar", 3, false},
	}
	for _, tc := range tests {
		got := isPunctAt([]byte(tc.input), tc.off)
		if got != tc.want {
			t.Errorf("isPunctAt(%q, %d): expected %v, got %v", tc.input, tc.off, tc.want, got)
		}
	}
}

// TestCharFlankLevelUnicode verifies that charFlankLevel correctly handles
// multi-byte Unicode characters adjacent to mark positions.
// Note: charFlankLevel is designed to be called with 'off' pointing to a
// single-byte ASCII mark character (e.g. *, _, ^). It checks the character
// before/after that mark.
func TestCharFlankLevelUnicode(t *testing.T) {
	// Test: EM DASH after a mark. Input: "*—b" where * is at position 0
	// charFlankLevel with dir=1 checks the character after position 0
	input := "*\xe2\x80\x94b" // "*—b"
	level := charFlankLevel([]byte(input), 0, 1)
	if level != 1 { // EM DASH is punctuation
		t.Errorf("charFlankLevel after '*' before EM DASH: expected 1 (punct), got %d", level)
	}

	// Test: NBSP after a mark. Input: "* b" (NBSP between * and b)
	input2 := "*\xc2\xa0b" // "* b" (with NBSP after *)
	level2 := charFlankLevel([]byte(input2), 0, 1)
	if level2 != 0 { // NBSP is whitespace
		t.Errorf("charFlankLevel after '*' before NBSP: expected 0 (whitespace), got %d", level2)
	}

	// Test: EM DASH before a mark. Input: "a—*" where * is at position 4
	input3 := "a\xe2\x80\x94*" // "a—*"
	level3 := charFlankLevel([]byte(input3), 4, -1)
	if level3 != 1 { // EM DASH is punctuation
		t.Errorf("charFlankLevel before '*' after EM DASH: expected 1 (punct), got %d", level3)
	}
}
