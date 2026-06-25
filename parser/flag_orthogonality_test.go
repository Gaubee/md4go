package parser_test

import (
	"strings"
	"testing"

	"github.com/userpro/md4go/parser"
)

// ═══════════════════════════════════════════════════════════════
// Flag Orthogonality — each flag independently changes behavior
// ═══════════════════════════════════════════════════════════════

// TestFlagCollapseWhitespaceOrthogonal verifies FlagCollapseWhitespace independently changes behavior.
func TestFlagCollapseWhitespaceOrthogonal(t *testing.T) {
	src := "foo  bar\n"
	off := convert(t, src, 0)
	on := convert(t, src, parser.FlagCollapseWhitespace)
	if off == on {
		t.Errorf("FlagCollapseWhitespace should change output: off=%q on=%q", off, on)
	}
	if !strings.Contains(on, "foo bar") {
		t.Errorf("FlagCollapseWhitespace: expected 'foo bar', got %q", on)
	}
}

// TestFlagPermissiveATXHeadersOrthogonal verifies FlagPermissiveATXHeaders independently changes behavior.
func TestFlagPermissiveATXHeadersOrthogonal(t *testing.T) {
	src := "###header\n"
	off := convert(t, src, 0)
	on := convert(t, src, parser.FlagPermissiveATXHeaders)
	if off == on {
		t.Errorf("FlagPermissiveATXHeaders should change output: off=%q on=%q", off, on)
	}
}

// TestFlagNoIndentedCodeBlocksOrthogonal verifies FlagNoIndentedCodeBlocks independently changes behavior.
func TestFlagNoIndentedCodeBlocksOrthogonal(t *testing.T) {
	src := "    code\n"
	off := convert(t, src, 0)
	on := convert(t, src, parser.FlagNoIndentedCodeBlocks)
	if off == on {
		t.Errorf("FlagNoIndentedCodeBlocks should change output: off=%q on=%q", off, on)
	}
}

// TestFlagHardSoftBreaksOrthogonal verifies FlagHardSoftBreaks independently changes behavior.
// In plain text, hard/soft breaks both produce \n, so we use HTML to verify the difference.
// Note: flags_test.go TestFlagHardSoftBreaks tests the exact HTML output;
// this test only verifies orthogonality (on != off) to avoid redundancy.
func TestFlagHardSoftBreaksOrthogonal(t *testing.T) {
	src := "foo\nbar\n"
	off := renderHTMLStr(t, src, 0)
	on := renderHTMLStr(t, src, parser.FlagHardSoftBreaks)
	if off == on {
		t.Errorf("FlagHardSoftBreaks should change HTML output: off=%q on=%q", off, on)
	}
	if !strings.Contains(on, "<br") {
		t.Errorf("FlagHardSoftBreaks: expected <br> tag, got %q", on)
	}
}

// TestFlagUnderlineOrthogonal verifies FlagUnderline independently changes behavior.
// In plain text, <u> and <em> both render as the inner text, so we use the HTML
// renderer to verify the flag changes the output tag.
func TestFlagUnderlineOrthogonal(t *testing.T) {
	src := "_text_\n"
	off := renderHTMLStr(t, src, 0)
	on := renderHTMLStr(t, src, parser.FlagUnderline)
	if off == on {
		t.Errorf("FlagUnderline should change HTML output: off=%q on=%q", off, on)
	}
	if !strings.Contains(on, "<u>") {
		t.Errorf("FlagUnderline: expected <u> tag, got %q", on)
	}
	if !strings.Contains(off, "<em>") {
		t.Errorf("default: expected <em> tag, got %q", off)
	}
}

// TestFlagTablesOrthogonal verifies FlagTables independently changes behavior.
func TestFlagTablesOrthogonal(t *testing.T) {
	src := "| a | b |\n|---|---|\n| c | d |"
	off := convert(t, src, 0)
	on := convert(t, src, parser.FlagTables)
	if off == on {
		t.Errorf("FlagTables should change output: off=%q on=%q", off, on)
	}
}

// TestFlagStrikethroughOrthogonal verifies FlagStrikethrough independently changes behavior.
func TestFlagStrikethroughOrthogonal(t *testing.T) {
	src := "~~strike~~\n"
	off := convert(t, src, 0)
	on := convert(t, src, parser.FlagStrikethrough)
	if off == on {
		t.Errorf("FlagStrikethrough should change output: off=%q on=%q", off, on)
	}
}

// TestFlagStrikethroughPermissiveOrthogonal verifies FlagStrikethroughPermissive
// independently changes behavior. The discriminating input is intraword
// strikethrough (foo~~bar~~baz), which the default (md4c strict) leaves literal
// but the permissive flag (cmark-gfm/goldmark) recognizes.
func TestFlagStrikethroughPermissiveOrthogonal(t *testing.T) {
	src := "foo~~bar~~baz\n"
	base := parser.FlagStrikethrough
	off := convert(t, src, base)
	on := convert(t, src, base|parser.FlagStrikethroughPermissive)
	if off == on {
		t.Errorf("FlagStrikethroughPermissive should change output: off=%q on=%q", off, on)
	}
	if !strings.Contains(on, "bar") || strings.Contains(on, "~~") {
		t.Errorf("FlagStrikethroughPermissive: expected intraword strike, got %q", on)
	}
}

// TestFlagTasklistsOrthogonal verifies FlagTasklists independently changes behavior.
func TestFlagTasklistsOrthogonal(t *testing.T) {
	src := "- [x] done\n"
	off := convert(t, src, 0)
	on := convert(t, src, parser.FlagTasklists)
	if off == on {
		t.Errorf("FlagTasklists should change output: off=%q on=%q", off, on)
	}
}

// TestFlagFootnotesOrthogonal verifies FlagFootnotes independently changes behavior.
func TestFlagFootnotesOrthogonal(t *testing.T) {
	src := "Text[^1]\n\n[^1]: Note\n"
	off := convert(t, src, 0)
	on := convert(t, src, parser.FlagFootnotes)
	if off == on {
		t.Errorf("FlagFootnotes should change output: off=%q on=%q", off, on)
	}
}

// TestFlagHighlightOrthogonal verifies FlagHighlight independently changes behavior.
func TestFlagHighlightOrthogonal(t *testing.T) {
	src := "==hl==\n"
	off := convert(t, src, 0)
	on := convert(t, src, parser.FlagHighlight)
	if off == on {
		t.Errorf("FlagHighlight should change output: off=%q on=%q", off, on)
	}
}

// TestFlagSpoilersOrthogonal verifies FlagSpoilers independently changes behavior.
func TestFlagSpoilersOrthogonal(t *testing.T) {
	src := "||secret||\n"
	off := convert(t, src, 0)
	on := convert(t, src, parser.FlagSpoilers)
	if off == on {
		t.Errorf("FlagSpoilers should change output: off=%q on=%q", off, on)
	}
}

// TestFlagSuperscriptsOrthogonal verifies FlagSuperscripts independently changes behavior.
func TestFlagSuperscriptsOrthogonal(t *testing.T) {
	src := "x^2^\n"
	off := convert(t, src, 0)
	on := convert(t, src, parser.FlagSuperscripts)
	if off == on {
		t.Errorf("FlagSuperscripts should change output: off=%q on=%q", off, on)
	}
}

// TestFlagSubscriptsOrthogonal verifies FlagSubscripts independently changes behavior.
func TestFlagSubscriptsOrthogonal(t *testing.T) {
	src := "H~2~O\n"
	off := convert(t, src, 0)
	on := convert(t, src, parser.FlagSubscripts)
	if off == on {
		t.Errorf("FlagSubscripts should change output: off=%q on=%q", off, on)
	}
}

// TestFlagLatexMathSpansOrthogonal verifies FlagLatexMathSpans independently changes behavior.
func TestFlagLatexMathSpansOrthogonal(t *testing.T) {
	src := "$x^2$\n"
	off := convert(t, src, 0)
	on := convert(t, src, parser.FlagLatexMathSpans)
	if off == on {
		t.Errorf("FlagLatexMathSpans should change output: off=%q on=%q", off, on)
	}
}

// TestFlagWikilinksOrthogonal verifies FlagWikilinks independently changes behavior.
func TestFlagWikilinksOrthogonal(t *testing.T) {
	src := "[[target]]\n"
	off := convert(t, src, 0)
	on := convert(t, src, parser.FlagWikilinks)
	if off == on {
		t.Errorf("FlagWikilinks should change output: off=%q on=%q", off, on)
	}
}

// TestFlagAdmonitionsOrthogonal verifies FlagAdmonitions independently changes behavior.
func TestFlagAdmonitionsOrthogonal(t *testing.T) {
	src := "> [!note]\n> Content\n"
	off := convert(t, src, 0)
	on := convert(t, src, parser.FlagAdmonitions)
	if off == on {
		t.Errorf("FlagAdmonitions should change output: off=%q on=%q", off, on)
	}
}

// TestFlagStripBOMOrthogonal verifies FlagStripBOM independently changes behavior.
func TestFlagStripBOMOrthogonal(t *testing.T) {
	src := "\xEF\xBB\xBFHello\n"
	off := convert(t, src, 0)
	on := convert(t, src, parser.FlagStripBOM)
	if off == on {
		t.Errorf("FlagStripBOM should change output: off=%q on=%q", off, on)
	}
	if !strings.Contains(off, "\ufeff") {
		t.Errorf("default: expected BOM preserved, got %q", off)
	}
	if strings.Contains(on, "\ufeff") {
		t.Errorf("FlagStripBOM: expected BOM stripped, got %q", on)
	}
}
