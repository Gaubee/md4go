package parser_test

import (
	"strings"
	"testing"

	"github.com/userpro/md4go/parser"
)

// ═══════════════════════════════════════════════════════════════
// Dialect Preset Composition Verification
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
// Compatibility Preset Composition Verification
// ═══════════════════════════════════════════════════════════════

func TestGoldmarkCompatComposition(t *testing.T) {
	want := parser.FlagTableInterruptParagraph | parser.FlagDecodeEntities |
		parser.FlagStripBOM | parser.FlagStrikethroughPermissive | parser.FlagStripHTMLTags |
		parser.FlagStrictTableColumns | parser.FlagTableInterruptByHeaders |
		parser.FlagNoXHTMLEntityEncoding
	if parser.GoldmarkCompat != want {
		t.Errorf("GoldmarkCompat = 0x%x, want 0x%x",
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
// Flag Value Non-Overlap Verification
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
		{"FlagStripBOM", parser.FlagStripBOM},
		{"FlagStrikethroughPermissive", parser.FlagStrikethroughPermissive},
		{"FlagStripHTMLTags", parser.FlagStripHTMLTags},
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
// Convenience Combinations
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
