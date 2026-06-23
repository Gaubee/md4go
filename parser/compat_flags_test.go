package parser_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/userpro/md4go/html"
	"github.com/userpro/md4go/parser"
	"github.com/userpro/md4go/text"
)

// convert is a test helper that converts markdown to plain text with given flags.
func convert(t *testing.T, src string, flags parser.Flags) string {
	t.Helper()
	var buf bytes.Buffer
	if err := text.Convert([]byte(src), &buf, text.WithFlags(flags)); err != nil {
		t.Fatalf("Convert failed: %v", err)
	}
	return buf.String()
}

// convertExt is like convert but also enables GFM extensions.
func convertExt(t *testing.T, src string, flags parser.Flags) string {
	t.Helper()
	var buf bytes.Buffer
	if err := text.Convert([]byte(src), &buf,
		text.WithFlags(parser.DialectGitHub|flags),
	); err != nil {
		t.Fatalf("Convert failed: %v", err)
	}
	return buf.String()
}

// renderHTMLStr is a test helper that converts markdown to HTML with given flags.
func renderHTMLStr(t *testing.T, src string, flags parser.Flags) string {
	t.Helper()
	var buf bytes.Buffer
	if err := html.Convert([]byte(src), &buf, html.WithFlags(flags)); err != nil {
		t.Fatalf("HTML Convert failed: %v", err)
	}
	return strings.TrimRight(buf.String(), "\n")
}

// --- S-01: FlagProtectDoublePipe ---

func TestFlagProtectDoublePipe(t *testing.T) {
	// Table with || inside a cell.
	// GFM standard (default): || splits into empty cell + content.
	// FlagProtectDoublePipe: || is protected, kept as literal text in one cell.
	src := "| a | b |\n|---|---|\n| x || y |"

	// Default: || splits → 3 cells in body row → "x\t\ty" (empty middle cell)
	gotDefault := convertExt(t, src, 0)
	if !strings.Contains(gotDefault, "x") {
		t.Errorf("default: expected 'x' in output, got %q", gotDefault)
	}

	// With FlagProtectDoublePipe: || protected → 2 cells → "x || y" in one cell
	gotProtected := convertExt(t, src, parser.FlagProtectDoublePipe)
	if !strings.Contains(gotProtected, "||") {
		t.Errorf("FlagProtectDoublePipe: expected '||' preserved, got %q", gotProtected)
	}

	// Outputs should differ
	if gotDefault == gotProtected {
		t.Errorf("expected different output with/without flag, both: %q", gotDefault)
	}
}

// --- S-02/S-04: tight list paragraph separator (default behavior) ---

func TestTightListSeparatorDefaultNoFlag(t *testing.T) {
	// Tight list with multiple paragraphs in a single list item.
	// Default (no compat flag): \n separator preserves word boundaries between paragraphs.
	// List-item separators are always preserved.
	src := "- para1a\n\n  para1b\n- second"

	got := convert(t, src, 0)
	if !strings.Contains(got, "para1a") || !strings.Contains(got, "para1b") {
		t.Errorf("expected 'para1a' and 'para1b' in output, got %q", got)
	}
	// Default: "para1a\npara1b" (paragraph separator preserved)
	if !strings.Contains(got, "para1a\npara1b") {
		t.Errorf("expected paragraph separator, got %q", got)
	}
	// List-item separator should be present
	if !strings.Contains(got, "para1b\nsecond") && !strings.Contains(got, "para1b second") {
		t.Errorf("expected list-item separator before 'second', got %q", got)
	}
}

// --- S-05: footnote reference [N] output (default behavior) ---

func TestFootnoteRefOutput(t *testing.T) {
	src := "Text[^1]\n\n[^1]: Footnote content"

	// Default: [1] is output for footnote reference
	got := convertExt(t, src, 0)
	if !strings.Contains(got, "[1]") {
		t.Errorf("expected '[1]' in output, got %q", got)
	}
}

// --- S-06: NULL code span recognition (default behavior) ---

func TestNullCodeSpanRecognition(t *testing.T) {
	// Code span containing NULL character.
	// Default: NULL replaced with U+FFFD, code span recognized (CommonMark standard).
	src := "text `" + string([]byte{0}) + "` more"

	got := convert(t, src, 0)
	if !strings.Contains(got, "\uFFFD") {
		t.Errorf("expected U+FFFD in output, got %q", got)
	}
}

// --- Dimension B: FlagStrictTableColumns ---

func TestFlagStrictTableColumns(t *testing.T) {
	// Table with mismatched header/delimiter column counts.
	// Default (lenient): recognized as table (aligns with md4c).
	// FlagStrictTableColumns: not recognized as table (strict column match, GFM standard/goldmark).
	src := "| a | b | c |\n|---|---|\n| d | e | f |"

	// Default: lenient → recognized as table (pipes stripped)
	gotDefault := convertExt(t, src, 0)
	if strings.Contains(gotDefault, "---") {
		t.Errorf("default (lenient): expected table recognized (no '---'), got %q", gotDefault)
	}

	// With FlagStrictTableColumns: strict → 3 header cols ≠ 2 delimiter cols → not a table
	gotStrict := convertExt(t, src, parser.FlagStrictTableColumns)
	// When not a table, pipes are preserved as text
	if !strings.Contains(gotStrict, "|") {
		t.Errorf("strict: expected pipes preserved (not a table), got %q", gotStrict)
	}

	// Outputs should differ
	if gotDefault == gotStrict {
		t.Errorf("expected different output with/without flag, both: %q", gotDefault)
	}
}

// --- FlagDecodeEntities ---

func TestFlagDecodeEntities(t *testing.T) {
	src := "&amp; &copy; &#65;"

	// Default: entities preserved verbatim
	gotDefault := convert(t, src, 0)
	if !strings.Contains(gotDefault, "&amp;") {
		t.Errorf("default: expected '&amp;' preserved, got %q", gotDefault)
	}

	// With FlagDecodeEntities: entities decoded to Unicode
	gotDecoded := convert(t, src, parser.FlagDecodeEntities)
	if !strings.Contains(gotDecoded, "&") || strings.Contains(gotDecoded, "&amp;") {
		t.Errorf("decode: expected '&' decoded, got %q", gotDecoded)
	}
	if !strings.Contains(gotDecoded, "©") {
		t.Errorf("decode: expected '©' decoded, got %q", gotDecoded)
	}
	if !strings.Contains(gotDecoded, "A") {
		t.Errorf("decode: expected 'A' decoded from &#65;, got %q", gotDecoded)
	}
}

// --- FlagTableInterruptParagraph (existing, verify still works) ---

func TestFlagTableInterruptParagraph(t *testing.T) {
	src := "Para\n| a | b |\n|---|---|\n| c | d |"

	// Default: table can't interrupt paragraph → not recognized as table
	gotDefault := convertExt(t, src, 0)
	if !strings.Contains(gotDefault, "---") {
		t.Errorf("default: expected '---' preserved (not a table), got %q", gotDefault)
	}

	// With FlagTableInterruptParagraph: table interrupts paragraph
	gotInterrupt := convertExt(t, src, parser.FlagTableInterruptParagraph)
	if strings.Contains(gotInterrupt, "---") {
		t.Errorf("interrupt: expected table recognized (no '---'), got %q", gotInterrupt)
	}
}

// --- Combination tests ---

func TestGoldmarkCompat(t *testing.T) {
	// GoldmarkCompat = FlagTableInterruptParagraph | FlagStrictTableColumns | FlagDecodeEntities
	src := "&amp; test\n\n| a | b |\n|---|---|\n| c | d |"

	got := convertExt(t, src, parser.GoldmarkCompat)
	// With GoldmarkCompat: entity decoded, table can interrupt paragraph
	if strings.Contains(got, "&amp;") {
		t.Errorf("GoldmarkCompat: expected entity decoded, got %q", got)
	}
}

// --- Regression: default behavior unchanged ---

func TestCompatFlagsDefaultNoChange(t *testing.T) {
	// Ensure default (no compat flags) still passes basic markdown
	cases := []struct {
		name string
		src  string
		want string
	}{
		{"heading", "# Hello", "Hello"},
		{"paragraph", "Hello world", "Hello world"},
		{"code", "`code`", "code"},
		{"emphasis", "*em*", "em"},
		{"link", "[text](url)", "text"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := convert(t, c.src, 0)
			if !strings.Contains(got, c.want) {
				t.Errorf("default %s: expected %q in output, got %q", c.name, c.want, got)
			}
		})
	}
}

// --- FlagProtectDoublePipe with FlagSpoilers ---

func TestFlagProtectDoublePipeWithSpoilers(t *testing.T) {
	// When FlagSpoilers is enabled, || is always protected (spoiler syntax).
	// FlagProtectDoublePipe should not be needed when spoilers are on.
	src := "| A | B |\n|---|---|\n| ||spoiler|| | baz |"

	// With FlagSpoilers only (no FlagProtectDoublePipe): || protected for spoiler syntax
	got := convertExt(t, src, parser.FlagSpoilers)
	if !strings.Contains(got, "spoiler") {
		t.Errorf("FlagSpoilers: expected 'spoiler' in output, got %q", got)
	}
}

// ═══════════════════════════════════════════════════════════════
// Flag Audit: Dialect Preset Composition Verification
// ═══════════════════════════════════════════════════════════════

// TestDialectCommonMarkIsZero verifies DialectCommonMark = 0 (pure CommonMark, no extensions).
func TestDialectCommonMarkIsZero(t *testing.T) {
	if parser.DialectCommonMark != 0 {
		t.Errorf("DialectCommonMark must be 0 (pure CommonMark), got 0x%x", parser.DialectCommonMark)
	}
}

// TestDialectGitHubComposition verifies DialectGitHub is composed of the correct base flags.
func TestDialectGitHubComposition(t *testing.T) {
	want := parser.PermissiveAutolinks | parser.FlagTables | parser.FlagStrikethrough |
		parser.FlagTasklists | parser.FlagAdmonitions | parser.FlagFootnotes
	if parser.DialectGitHub != want {
		t.Errorf("DialectGitHub = 0x%x, want 0x%x (PermissiveAutolinks|Tables|Strikethrough|Tasklists|Admonitions|Footnotes)",
			parser.DialectGitHub, want)
	}
}

// TestDialectGitHubMatchesMd4c verifies DialectGitHub matches md4c's MD_DIALECT_GITHUB.
func TestDialectGitHubMatchesMd4c(t *testing.T) {
	// md4c: MD_DIALECT_GITHUB = MD_FLAG_PERMISSIVEAUTOLINKS | MD_FLAG_TABLES | MD_FLAG_STRIKETHROUGH | MD_FLAG_TASKLISTS | MD_FLAG_ADMONITIONS | MD_FLAG_FOOTNOTES
	// All flag values must match md4c.h exactly.
	if parser.FlagPermissiveURLAutolinks != 0x4 {
		t.Errorf("FlagPermissiveURLAutolinks = 0x%x, md4c expects 0x4", parser.FlagPermissiveURLAutolinks)
	}
	if parser.FlagPermissiveEmailAutolinks != 0x8 {
		t.Errorf("FlagPermissiveEmailAutolinks = 0x%x, md4c expects 0x8", parser.FlagPermissiveEmailAutolinks)
	}
	if parser.FlagPermissiveWWWAutolinks != 0x400 {
		t.Errorf("FlagPermissiveWWWAutolinks = 0x%x, md4c expects 0x400", parser.FlagPermissiveWWWAutolinks)
	}
	if parser.FlagTables != 0x100 {
		t.Errorf("FlagTables = 0x%x, md4c expects 0x100", parser.FlagTables)
	}
	if parser.FlagStrikethrough != 0x200 {
		t.Errorf("FlagStrikethrough = 0x%x, md4c expects 0x200", parser.FlagStrikethrough)
	}
	if parser.FlagTasklists != 0x800 {
		t.Errorf("FlagTasklists = 0x%x, md4c expects 0x800", parser.FlagTasklists)
	}
	if parser.FlagAdmonitions != 0x80000 {
		t.Errorf("FlagAdmonitions = 0x%x, md4c expects 0x80000", parser.FlagAdmonitions)
	}
	if parser.FlagFootnotes != 0x100000 {
		t.Errorf("FlagFootnotes = 0x%x, md4c expects 0x100000", parser.FlagFootnotes)
	}
}

// TestDialectCommonMarkNoExtensions verifies that DialectCommonMark (flags=0) does not
// recognize any extension syntax (tables, strikethrough, tasklists, footnotes, etc.).
func TestDialectCommonMarkNoExtensions(t *testing.T) {
	cases := []struct {
		name  string
		src   string
		check func(string) bool // returns true if extension syntax is NOT recognized
	}{
		{"table", "| a | b |\n|---|---|\n| c | d |", func(got string) bool { return strings.Contains(got, "|") }},
		{"strikethrough", "~~strike~~\n", func(got string) bool { return strings.Contains(got, "~~") }},
		{"tasklist", "- [x] done\n", func(got string) bool { return strings.Contains(got, "[x]") }},
		{"footnote", "Text[^1]\n\n[^1]: Note\n", func(got string) bool { return !strings.Contains(got, "[1]") }},
		{"wikilink", "[[target]]\n", func(got string) bool { return strings.Contains(got, "[[") }},
		{"highlight", "==hl==\n", func(got string) bool { return strings.Contains(got, "==") }},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := convert(t, c.src, parser.DialectCommonMark)
			if !c.check(got) {
				t.Errorf("DialectCommonMark should not recognize %s extension, got %q", c.name, got)
			}
		})
	}
}

// TestDialectGitHubExtensionsEnabled verifies that DialectGitHub enables the expected extensions.
func TestDialectGitHubExtensionsEnabled(t *testing.T) {
	cases := []struct {
		name  string
		src   string
		check func(string) bool // returns true if extension syntax IS recognized
	}{
		{"table", "| a | b |\n|---|---|\n| c | d |", func(got string) bool { return !strings.Contains(got, "---") }},
		{"strikethrough", "~~strike~~\n", func(got string) bool { return !strings.Contains(got, "~~") }},
		{"tasklist", "- [x] done\n", func(got string) bool { return strings.Contains(got, "✓") || !strings.Contains(got, "[x]") }},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := convert(t, c.src, parser.DialectGitHub)
			if !c.check(got) {
				t.Errorf("DialectGitHub should recognize %s extension, got %q", c.name, got)
			}
		})
	}
}

// ═══════════════════════════════════════════════════════════════
// Flag Audit: Compatibility Preset Composition Verification
// ═══════════════════════════════════════════════════════════════

func TestGoldmarkCompatComposition(t *testing.T) {
	want := parser.FlagTableInterruptParagraph | parser.FlagStrictTableColumns | parser.FlagDecodeEntities
	if parser.GoldmarkCompat != want {
		t.Errorf("GoldmarkCompat = 0x%x, want 0x%x (TableInterruptParagraph|StrictTableColumns|DecodeEntities)",
			parser.GoldmarkCompat, want)
	}
}

// TestCompatFlagsNotInDialectGitHub verifies compat flags are NOT included in DialectGitHub.
func TestCompatFlagsNotInDialectGitHub(t *testing.T) {
	if parser.DialectGitHub&parser.GoldmarkCompat != 0 {
		t.Error("GoldmarkCompat flags should NOT be included in DialectGitHub")
	}
}

// ═══════════════════════════════════════════════════════════════
// Flag Audit: Default = CommonMark Standard Verification
// ═══════════════════════════════════════════════════════════════

// TestDefaultIsCommonMarkStandard verifies that flags=0 produces CommonMark-standard behavior:
// no extensions, no behavior modifications, no compat alignment.
func TestDefaultIsCommonMarkStandard(t *testing.T) {
	// Indented code blocks work (standard)
	got := convert(t, "    code\n", 0)
	if !strings.Contains(got, "code") {
		t.Errorf("default: indented code should work, got %q", got)
	}

	// ATX headers require space (standard)
	got = convert(t, "###NoSpace\n", 0)
	if !strings.Contains(got, "###NoSpace") {
		t.Errorf("default: ### without space should be literal, got %q", got)
	}

	// HTML blocks work (standard)
	got = convert(t, "<div>html</div>\n", 0)
	if !strings.Contains(got, "html") {
		t.Errorf("default: HTML blocks should work, got %q", got)
	}

	// Tables NOT recognized (standard — no extension)
	got = convert(t, "| a | b |\n|---|---|\n| c | d |", 0)
	if !strings.Contains(got, "|") {
		t.Errorf("default: tables should NOT be recognized (no extension), got %q", got)
	}

	// Strikethrough NOT recognized (standard — no extension)
	got = convert(t, "~~strike~~\n", 0)
	if !strings.Contains(got, "~~") {
		t.Errorf("default: strikethrough should NOT be recognized, got %q", got)
	}
}

// ═══════════════════════════════════════════════════════════════
// Flag Audit: Flag Orthogonality — each flag independently changes behavior
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

// ═══════════════════════════════════════════════════════════════
// Flag Audit: Flag Value Non-Overlap Verification
// ═══════════════════════════════════════════════════════════════

// TestFlagValuesNonOverlap verifies that all individual flag values are distinct powers of 2.
func TestFlagValuesNonOverlap(t *testing.T) {
	flags := []struct {
		name string
		val  parser.Flags
	}{
		{"FlagCollapseWhitespace", parser.FlagCollapseWhitespace},
		{"FlagPermissiveATXHeaders", parser.FlagPermissiveATXHeaders},
		{"FlagPermissiveURLAutolinks", parser.FlagPermissiveURLAutolinks},
		{"FlagPermissiveEmailAutolinks", parser.FlagPermissiveEmailAutolinks},
		{"FlagNoIndentedCodeBlocks", parser.FlagNoIndentedCodeBlocks},
		{"FlagNoHTMLBlocks", parser.FlagNoHTMLBlocks},
		{"FlagNoHTMLSpans", parser.FlagNoHTMLSpans},
		{"FlagTables", parser.FlagTables},
		{"FlagStrikethrough", parser.FlagStrikethrough},
		{"FlagPermissiveWWWAutolinks", parser.FlagPermissiveWWWAutolinks},
		{"FlagTasklists", parser.FlagTasklists},
		{"FlagLatexMathSpans", parser.FlagLatexMathSpans},
		{"FlagWikilinks", parser.FlagWikilinks},
		{"FlagUnderline", parser.FlagUnderline},
		{"FlagHardSoftBreaks", parser.FlagHardSoftBreaks},
		{"FlagSpoilers", parser.FlagSpoilers},
		{"FlagSuperscripts", parser.FlagSuperscripts},
		{"FlagSubscripts", parser.FlagSubscripts},
		{"FlagAdmonitions", parser.FlagAdmonitions},
		{"FlagFootnotes", parser.FlagFootnotes},
		{"FlagHighlight", parser.FlagHighlight},
		{"FlagTableInterruptParagraph", parser.FlagTableInterruptParagraph},
		{"FlagProtectDoublePipe", parser.FlagProtectDoublePipe},
		{"FlagStrictTableColumns", parser.FlagStrictTableColumns},
		{"FlagDecodeEntities", parser.FlagDecodeEntities},
	}

	seen := make(map[parser.Flags]string)
	for _, f := range flags {
		// Each flag must be a single power of 2
		if f.val == 0 || f.val&(f.val-1) != 0 {
			t.Errorf("%s = 0x%x is not a single power of 2", f.name, f.val)
		}
		// Each flag value must be unique
		if prev, ok := seen[f.val]; ok {
			t.Errorf("%s and %s have the same value 0x%x", f.name, prev, f.val)
		}
		seen[f.val] = f.name
	}
}

// ═══════════════════════════════════════════════════════════════
// Flag Audit: Convenience Combinations
// ═══════════════════════════════════════════════════════════════

func TestPermissiveAutolinksComposition(t *testing.T) {
	want := parser.FlagPermissiveURLAutolinks | parser.FlagPermissiveEmailAutolinks | parser.FlagPermissiveWWWAutolinks
	if parser.PermissiveAutolinks != want {
		t.Errorf("PermissiveAutolinks = 0x%x, want 0x%x", parser.PermissiveAutolinks, want)
	}
}

func TestNoHTMLComposition(t *testing.T) {
	want := parser.FlagNoHTMLBlocks | parser.FlagNoHTMLSpans
	if parser.NoHTML != want {
		t.Errorf("NoHTML = 0x%x, want 0x%x", parser.NoHTML, want)
	}
}
