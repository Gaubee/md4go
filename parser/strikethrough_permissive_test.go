package parser_test

import (
	"strings"
	"testing"

	"github.com/userpro/md4go/parser"
)

// ═══════════════════════════════════════════════════════════════
// Strikethrough flanking: md4c strict (default) vs permissive
// (cmark-gfm / goldmark). See DIFFCHECK_REPORT.md §3 R7a.
// ═══════════════════════════════════════════════════════════════

// TestStrikethroughDefaultStrict verifies md4c's default strikethrough
// flanking: ~~ cannot open when preceded by "other" (alphanumeric) and
// cannot close when followed by "other". This blocks intraword strikes.
func TestStrikethroughDefaultStrict(t *testing.T) {
	base := parser.FlagStrikethrough
	cases := []struct {
		name, input, wantHTML string
	}{
		{"word boundary", "~~strike~~", "<p><del>strike</del></p>"},
		{"after space", "foo ~~bar~~", "<p>foo <del>bar</del></p>"},
		{"before punct", "foo.~~bar~~", "<p>foo.<del>bar</del></p>"},
		// Intraword (other~~other): blocked by md4c strict rule.
		{"intraword blocked", "foo~~bar~~baz", "<p>foo~~bar~~baz</p>"},
		{"intraword odd count", "a~~b~~c~~d", "<p>a~~b~~c~~d</p>"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := renderHTML(c.input, base)
			if got != c.wantHTML {
				t.Errorf("input %q:\nwant %q\ngot  %q", c.input, c.wantHTML, got)
			}
		})
	}
}

// TestStrikethroughPermissive verifies the permissive flag makes ~~ behave
// like * emphasis (CommonMark left/right-flanking, no intraword restriction),
// matching cmark-gfm (the GFM reference implementation) and goldmark.
func TestStrikethroughPermissive(t *testing.T) {
	flags := parser.FlagStrikethrough | parser.FlagStrikethroughPermissive
	cases := []struct {
		name, input, wantHTML string
	}{
		// Word-boundary cases still work (unchanged from default).
		{"word boundary", "~~strike~~", "<p><del>strike</del></p>"},
		{"after space", "foo ~~bar~~", "<p>foo <del>bar</del></p>"},
		{"before punct", "foo.~~bar~~", "<p>foo.<del>bar</del></p>"},

		// Intraword strikes now recognized (cmark-gfm/goldmark behavior).
		{"intraword pair", "foo~~bar~~baz", "<p>foo<del>bar</del>baz</p>"},
		// Odd number: first two pair, third has no closer -> literal.
		{"intraword odd", "a~~b~~c~~d", "<p>a<del>b</del>c~~d</p>"},

		// R7a case from DIFFCHECK_REPORT.md (testdata1.jsonl #522):
		// First two ~~ pair across the span; the third is unpaired.
		{"r7a multi", "Tordera~~Located x The~~house is~~surrounded",
			"<p>Tordera<del>Located x The</del>house is~~surrounded</p>"},

		// Whitespace-only content between ~~ must NOT pair (matches cmark-gfm).
		// Permissive uses *-style flanking: ~~ followed by whitespace cannot
		// open, so no strikethrough is produced.
		{"ws only no strike", "~~ ~~", "<p>~~ ~~</p>"},
		// Leading/trailing whitespace: opener must be followed by non-ws,
		// closer must be preceded by non-ws.
		{"leading ws", "~~ foo~~", "<p>~~ foo~~</p>"},
		{"trailing ws", "~~foo ~~", "<p>~~foo ~~</p>"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := renderHTML(c.input, flags)
			if got != c.wantHTML {
				t.Errorf("input %q:\nwant %q\ngot  %q", c.input, c.wantHTML, got)
			}
		})
	}
}

// TestStrikethroughPermissiveNested verifies nesting still works under the
// permissive flag (matches the GFM spec nested example).
func TestStrikethroughPermissiveNested(t *testing.T) {
	flags := parser.FlagStrikethrough | parser.FlagStrikethroughPermissive
	got := renderHTML("~~foo ~~bar~~ baz~~", flags)
	want := "<p><del>foo <del>bar</del> baz</del></p>"
	if got != want {
		t.Errorf("nested:\nwant %q\ngot  %q", want, got)
	}
}

// TestStrikethroughPermissiveDoesNotAffectSubscript verifies the permissive
// flag only loosens ~~ flanking and does not change single-~ subscript behavior.
func TestStrikethroughPermissiveDoesNotAffectSubscript(t *testing.T) {
	src := "H~2~O\n"
	sub := renderHTML(src, parser.FlagSubscripts)
	subPerm := renderHTML(src, parser.FlagSubscripts|parser.FlagStrikethroughPermissive)
	if sub != subPerm {
		t.Errorf("FlagStrikethroughPermissive should not affect subscript: %q vs %q", sub, subPerm)
	}
	if !strings.Contains(sub, "<sub>2</sub>") {
		t.Errorf("subscript not recognized: %q", sub)
	}
}
