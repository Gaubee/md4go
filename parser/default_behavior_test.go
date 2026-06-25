package parser_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/userpro/md4go/html"
	"github.com/userpro/md4go/parser"
	"github.com/userpro/md4go/text"
)

// --- Shared test helpers (used by default_behavior_test, dialect_test, flag_orthogonality_test) ---

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

// --- S-07: code span backtick limit (md4go improvement over md4c) ---

func TestLongCodeSpanRecognition(t *testing.T) {
	// CommonMark 0.31 §6.1: no maximum on backtick string length.
	// md4c limits to 32 (CODESPAN_MARK_MAXLEN) — non-standard.
	// md4go raised limit to 1024, covering all practical inputs.
	backticks33 := strings.Repeat("`", 33)
	src := backticks33 + "code" + backticks33

	got := convert(t, src, 0)
	if strings.Contains(got, "`") {
		t.Errorf("expected 33-backtick code span recognized (no backticks in output), got %q", got)
	}
	if !strings.Contains(got, "code") {
		t.Errorf("expected 'code' in output, got %q", got)
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

// --- FlagStripBOM ---

func TestFlagStripBOM(t *testing.T) {
	// UTF-8 BOM is EF BB BF (U+FEFF)
	src := "\xEF\xBB\xBFHello world"

	// Default: BOM preserved in output
	gotDefault := convert(t, src, 0)
	if !strings.Contains(gotDefault, "\ufeff") {
		t.Errorf("default: expected BOM preserved, got %q", gotDefault)
	}

	// With FlagStripBOM: BOM stripped
	gotStripped := convert(t, src, parser.FlagStripBOM)
	if strings.Contains(gotStripped, "\ufeff") {
		t.Errorf("strip: expected BOM removed, got %q", gotStripped)
	}
	if !strings.Contains(gotStripped, "Hello world") || strings.Contains(gotStripped, "\ufeff") {
		t.Errorf("strip: expected 'Hello world' without BOM, got %q", gotStripped)
	}

	// BOM not at start should NOT be stripped (only leading BOM)
	srcMid := "text\xEF\xBB\xBFmore"
	gotMid := convert(t, srcMid, parser.FlagStripBOM)
	if !strings.Contains(gotMid, "\ufeff") {
		t.Errorf("mid-text BOM should NOT be stripped, got %q", gotMid)
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
	// GoldmarkCompat = FlagTableInterruptParagraph | FlagDecodeEntities | FlagStripBOM | FlagStrikethroughPermissive
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

// --- Default = CommonMark Standard Verification ---

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
