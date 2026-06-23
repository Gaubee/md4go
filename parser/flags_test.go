package parser_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/userpro/md4go/html"
	"github.com/userpro/md4go/parser"
)

// renderHTML converts markdown to HTML using the given flags.
func renderHTML(md string, flags parser.Flags) string {
	p := parser.New(flags)
	var buf bytes.Buffer
	h := html.NewHTML(&buf)
	if err := p.Parse([]byte(md), h); err != nil {
		return "ERROR:" + err.Error()
	}
	_ = h.Flush()
	// Trim trailing newline added by the HTML renderer
	return strings.TrimRight(buf.String(), "\n")
}

func TestFlagNoHTMLBlocks(t *testing.T) {
	// With parser.FlagNoHTMLBlocks, HTML blocks should be treated as paragraphs
	md := "<div>\nhello\n</div>\n"
	got := renderHTML(md, parser.FlagNoHTMLBlocks)
	if got == "<div>\nhello\n</div>" {
		t.Errorf("parser.FlagNoHTMLBlocks should not produce raw HTML block, got: %q", got)
	}

	// Without the flag, HTML blocks are rendered as raw HTML
	got = renderHTML(md, 0)
	want := "<div>\nhello\n</div>"
	if got != want {
		t.Errorf("No flag (HTML blocks allowed):\ninput:  %q\nwant:   %q\ngot:    %q", md, want, got)
	}
}

func TestFlagNoHTMLSpans(t *testing.T) {
	// With parser.FlagNoHTMLSpans, inline HTML should be escaped
	md := "hello <b>world</b>\n"
	got := renderHTML(md, parser.FlagNoHTMLSpans)
	if got == "<p>hello <b>world</b></p>" {
		t.Errorf("parser.FlagNoHTMLSpans should escape inline HTML, got: %q", got)
	}

	// Without the flag, inline HTML is preserved
	got = renderHTML(md, 0)
	want := "<p>hello <b>world</b></p>"
	if got != want {
		t.Errorf("No flag (HTML spans allowed):\ninput:  %q\nwant:   %q\ngot:    %q", md, want, got)
	}

	// Autolinks should still work with parser.FlagNoHTMLSpans
	md = "<http://example.com>\n"
	got = renderHTML(md, parser.FlagNoHTMLSpans)
	want = `<p><a href="http://example.com">http://example.com</a></p>`
	if got != want {
		t.Errorf("parser.FlagNoHTMLSpans with autolink:\ninput:  %q\nwant:   %q\ngot:    %q", md, want, got)
	}
}

func TestFlagNoIndentedCodeBlocks(t *testing.T) {
	// With parser.FlagNoIndentedCodeBlocks, indented code is not detected
	md := "    code block\n"
	got := renderHTML(md, parser.FlagNoIndentedCodeBlocks)
	if got == "<pre><code>code block</code></pre>" {
		t.Errorf("parser.FlagNoIndentedCodeBlocks should not produce indented code, got: %q", got)
	}

	// Without the flag, indented code is detected
	got = renderHTML(md, 0)
	want := "<pre><code>code block\n</code></pre>"
	if got != want {
		t.Errorf("No flag (indented code allowed):\ninput:  %q\nwant:   %q\ngot:    %q", md, want, got)
	}

	// Fenced code should still work with parser.FlagNoIndentedCodeBlocks
	md = "```\ncode\n```\n"
	got = renderHTML(md, parser.FlagNoIndentedCodeBlocks)
	want = "<pre><code>code\n</code></pre>"
	if got != want {
		t.Errorf("parser.FlagNoIndentedCodeBlocks with fenced code:\ninput:  %q\nwant:   %q\ngot:    %q", md, want, got)
	}
}

func TestFlagPermissiveATXHeaders(t *testing.T) {
	// With parser.FlagPermissiveATXHeaders, no space needed after #
	md := "###header\n"
	got := renderHTML(md, parser.FlagPermissiveATXHeaders)
	want := "<h3>header</h3>"
	if got != want {
		t.Errorf("parser.FlagPermissiveATXHeaders:\ninput:  %q\nwant:   %q\ngot:    %q", md, want, got)
	}

	// Without the flag, ###header is not a header
	got = renderHTML(md, 0)
	want = "<p>###header</p>"
	if got != want {
		t.Errorf("No flag (space required):\ninput:  %q\nwant:   %q\ngot:    %q", md, want, got)
	}

	// Normal ATX headers should still work with the flag
	md = "### header\n"
	got = renderHTML(md, parser.FlagPermissiveATXHeaders)
	want = "<h3>header</h3>"
	if got != want {
		t.Errorf("parser.FlagPermissiveATXHeaders with space:\ninput:  %q\nwant:   %q\ngot:    %q", md, want, got)
	}
}

func TestFlagHardSoftBreaks(t *testing.T) {
	// With parser.FlagHardSoftBreaks, all soft breaks become hard breaks.
	// HTML renderer outputs <br />\n for TextBR.
	md := "foo\nbar\n"
	got := renderHTML(md, parser.FlagHardSoftBreaks)
	want := "<p>foo<br />\nbar</p>"
	if got != want {
		t.Errorf("parser.FlagHardSoftBreaks:\ninput:  %q\nwant:   %q\ngot:    %q", md, want, got)
	}

	// Without the flag, soft breaks are preserved as newlines
	got = renderHTML(md, 0)
	want = "<p>foo\nbar</p>"
	if got != want {
		t.Errorf("No flag (soft breaks):\ninput:  %q\nwant:   %q\ngot:    %q", md, want, got)
	}
}

func TestFlagCollapseWhitespace(t *testing.T) {
	// With parser.FlagCollapseWhitespace, multiple spaces collapse to one
	md := "foo  bar\n"
	got := renderHTML(md, parser.FlagCollapseWhitespace)
	want := "<p>foo bar</p>"
	if got != want {
		t.Errorf("parser.FlagCollapseWhitespace:\ninput:  %q\nwant:   %q\ngot:    %q", md, want, got)
	}

	// Without the flag, spaces are preserved
	got = renderHTML(md, 0)
	want = "<p>foo  bar</p>"
	if got != want {
		t.Errorf("No flag (spaces preserved):\ninput:  %q\nwant:   %q\ngot:    %q", md, want, got)
	}

	// Tab collapse
	md = "foo\tbar\n"
	got = renderHTML(md, parser.FlagCollapseWhitespace)
	want = "<p>foo bar</p>"
	if got != want {
		t.Errorf("parser.FlagCollapseWhitespace with tab:\ninput:  %q\nwant:   %q\ngot:    %q", md, want, got)
	}
}

func TestFlagUnderline(t *testing.T) {
	// With parser.FlagUnderline, _ marks become <u> tags
	md := "_underline_\n"
	got := renderHTML(md, parser.FlagUnderline)
	want := "<p><u>underline</u></p>"
	if got != want {
		t.Errorf("parser.FlagUnderline:\ninput:  %q\nwant:   %q\ngot:    %q", md, want, got)
	}

	// Without the flag, _ is emphasis
	got = renderHTML(md, 0)
	want = "<p><em>underline</em></p>"
	if got != want {
		t.Errorf("No flag (emphasis):\ninput:  %q\nwant:   %q\ngot:    %q", md, want, got)
	}

	// Double _ should produce nested <u> tags
	md = "__double__\n"
	got = renderHTML(md, parser.FlagUnderline)
	want = "<p><u><u>double</u></u></p>"
	if got != want {
		t.Errorf("parser.FlagUnderline double:\ninput:  %q\nwant:   %q\ngot:    %q", md, want, got)
	}

	// Asterisk emphasis should still work with parser.FlagUnderline
	md = "*emphasis*\n"
	got = renderHTML(md, parser.FlagUnderline)
	want = "<p><em>emphasis</em></p>"
	if got != want {
		t.Errorf("parser.FlagUnderline with *:\ninput:  %q\nwant:   %q\ngot:    %q", md, want, got)
	}
}

func TestFlagNoHTML(t *testing.T) {
	// parser.NoHTML = NoHTMLBlocks | NoHTMLSpans
	md := "<div>\n<b>bold</b>\n</div>\n"
	got := renderHTML(md, parser.NoHTML)
	if got == "<div>\n<b>bold</b>\n</div>" {
		t.Errorf("parser.NoHTML should escape HTML, got: %q", got)
	}
}

func TestFlagsCombined(t *testing.T) {
	// Multiple flags can be combined
	md := "###header\n\nfoo  bar\n"
	got := renderHTML(md, parser.FlagPermissiveATXHeaders|parser.FlagCollapseWhitespace)
	want := "<h3>header</h3>\n<p>foo bar</p>"
	if got != want {
		t.Errorf("Combined flags:\ninput:  %q\nwant:   %q\ngot:    %q", md, want, got)
	}
}

func TestFlagPermissiveEmailAutolinks(t *testing.T) {
	// With parser.FlagPermissiveEmailAutolinks, user@domain.com becomes an autolink
	md := "Contact user@example.com for info\n"
	got := renderHTML(md, parser.FlagPermissiveEmailAutolinks)
	want := `<p>Contact <a href="mailto:user@example.com">user@example.com</a> for info</p>`
	if got != want {
		t.Errorf("parser.FlagPermissiveEmailAutolinks:\ninput:  %q\nwant:   %q\ngot:    %q", md, want, got)
	}

	// Without the flag, no autolink
	got = renderHTML(md, 0)
	want = "<p>Contact user@example.com for info</p>"
	if got != want {
		t.Errorf("No flag (no email autolink):\ninput:  %q\nwant:   %q\ngot:    %q", md, want, got)
	}

	// Email with subdomain
	md = "admin@mail.example.com\n"
	got = renderHTML(md, parser.FlagPermissiveEmailAutolinks)
	want = `<p><a href="mailto:admin@mail.example.com">admin@mail.example.com</a></p>`
	if got != want {
		t.Errorf("Email with subdomain:\ninput:  %q\nwant:   %q\ngot:    %q", md, want, got)
	}

	// Email with plus and dot in username
	md = "first.last+tag@example.com\n"
	got = renderHTML(md, parser.FlagPermissiveEmailAutolinks)
	want = `<p><a href="mailto:first.last+tag@example.com">first.last+tag@example.com</a></p>`
	if got != want {
		t.Errorf("Email with plus and dot:\ninput:  %q\nwant:   %q\ngot:    %q", md, want, got)
	}
}

func TestFlagPermissiveURLAutolinks(t *testing.T) {
	// With parser.FlagPermissiveURLAutolinks, http:// URLs become autolinks
	md := "Visit http://example.com for info\n"
	got := renderHTML(md, parser.FlagPermissiveURLAutolinks)
	want := `<p>Visit <a href="http://example.com">http://example.com</a> for info</p>`
	if got != want {
		t.Errorf("parser.FlagPermissiveURLAutolinks:\ninput:  %q\nwant:   %q\ngot:    %q", md, want, got)
	}

	// https://
	md = "Visit https://example.com/path for info\n"
	got = renderHTML(md, parser.FlagPermissiveURLAutolinks)
	want = `<p>Visit <a href="https://example.com/path">https://example.com/path</a> for info</p>`
	if got != want {
		t.Errorf("HTTPS URL:\ninput:  %q\nwant:   %q\ngot:    %q", md, want, got)
	}

	// ftp://
	md = "Download ftp://files.example.com/pub/file.txt\n"
	got = renderHTML(md, parser.FlagPermissiveURLAutolinks)
	want = `<p>Download <a href="ftp://files.example.com/pub/file.txt">ftp://files.example.com/pub/file.txt</a></p>`
	if got != want {
		t.Errorf("FTP URL:\ninput:  %q\nwant:   %q\ngot:    %q", md, want, got)
	}

	// URL with query and fragment (raw & in input, not HTML entity)
	md = "See http://example.com/path?q=1&r=2#section\n"
	got = renderHTML(md, parser.FlagPermissiveURLAutolinks)
	if !strings.Contains(got, `<a href="http://example.com/path?q=1`) {
		t.Errorf("URL with query/fragment:\ninput:  %q\ngot:    %q", md, got)
	}

	// Without the flag, no autolink
	md = "Visit http://example.com\n"
	got = renderHTML(md, 0)
	want = "<p>Visit http://example.com</p>"
	if got != want {
		t.Errorf("No flag (no URL autolink):\ninput:  %q\nwant:   %q\ngot:    %q", md, want, got)
	}
}

func TestFlagPermissiveWWWAutolinks(t *testing.T) {
	// With parser.FlagPermissiveWWWAutolinks, www.example.com becomes an autolink
	md := "Visit www.example.com for info\n"
	got := renderHTML(md, parser.FlagPermissiveWWWAutolinks)
	want := `<p>Visit <a href="http://www.example.com">www.example.com</a> for info</p>`
	if got != want {
		t.Errorf("parser.FlagPermissiveWWWAutolinks:\ninput:  %q\nwant:   %q\ngot:    %q", md, want, got)
	}

	// www with path
	md = "See www.example.com/path\n"
	got = renderHTML(md, parser.FlagPermissiveWWWAutolinks)
	want = `<p>See <a href="http://www.example.com/path">www.example.com/path</a></p>`
	if got != want {
		t.Errorf("WWW with path:\ninput:  %q\nwant:   %q\ngot:    %q", md, want, got)
	}

	// Without the flag, no autolink
	md = "Visit www.example.com\n"
	got = renderHTML(md, 0)
	want = "<p>Visit www.example.com</p>"
	if got != want {
		t.Errorf("No flag (no WWW autolink):\ninput:  %q\nwant:   %q\ngot:    %q", md, want, got)
	}
}

func TestPermissiveAutolinksBoundary(t *testing.T) {
	flags := parser.PermissiveAutolinks

	// Punctuation after URL is excluded
	md := "Visit http://example.com.\n"
	got := renderHTML(md, flags)
	if !strings.Contains(got, `<a href="http://example.com">http://example.com</a>.`) {
		t.Errorf("Punctuation after URL:\ninput:  %q\ngot:    %q", md, got)
	}

	// Parentheses: URL in parens
	md = "Visit (http://example.com) now\n"
	got = renderHTML(md, flags)
	if !strings.Contains(got, `<a href="http://example.com">http://example.com</a>`) {
		t.Errorf("URL in parens:\ninput:  %q\ngot:    %q", md, got)
	}

	// URL followed by comma
	md = "See http://example.com, and more\n"
	got = renderHTML(md, flags)
	if !strings.Contains(got, `<a href="http://example.com">http://example.com</a>,`) {
		t.Errorf("URL followed by comma:\ninput:  %q\ngot:    %q", md, got)
	}

	// No autolink without scheme suffix
	md = "http:not-a-link\n"
	got = renderHTML(md, flags)
	if strings.Contains(got, "<a href") {
		t.Errorf("http without // should not autolink:\ninput:  %q\ngot:    %q", md, got)
	}

	// Email at line start
	md = "user@example.com\n"
	got = renderHTML(md, flags)
	if !strings.Contains(got, `<a href="mailto:user@example.com">user@example.com</a>`) {
		t.Errorf("Email at line start:\ninput:  %q\ngot:    %q", md, got)
	}

	// www after punctuation
	md = "See: www.example.com\n"
	got = renderHTML(md, flags)
	if !strings.Contains(got, `<a href="http://www.example.com">www.example.com</a>`) {
		t.Errorf("www after punctuation:\ninput:  %q\ngot:    %q", md, got)
	}

	// www not preceded by boundary (should NOT autolink)
	md = "xwww.example.com\n"
	got = renderHTML(md, flags)
	if strings.Contains(got, "<a href") {
		t.Errorf("www without boundary should NOT autolink:\ninput:  %q\ngot:    %q", md, got)
	}
}

func TestPermissiveAutolinksInsideEmphasis(t *testing.T) {
	flags := parser.PermissiveAutolinks

	// Permissive autolink inside emphasis should not cross emphasis marks
	md := "*Visit http://example.com for info*\n"
	got := renderHTML(md, flags)
	if !strings.Contains(got, `<em>Visit <a href="http://example.com">http://example.com</a> for info</em>`) {
		t.Errorf("Autolink inside emphasis:\ninput:  %q\ngot:    %q", md, got)
	}
}

// --- I32: Subscript + LaTeX math tests ---

func TestFlagSubscripts(t *testing.T) {
	// With parser.FlagSubscripts, single ~ creates <sub> tags
	md := "H~2~O is water\n"
	got := renderHTML(md, parser.FlagSubscripts)
	want := "<p>H<sub>2</sub>O is water</p>"
	if got != want {
		t.Errorf("parser.FlagSubscripts:\ninput:  %q\nwant:   %q\ngot:    %q", md, want, got)
	}

	// Without the flag, single ~ is literal text
	got = renderHTML(md, 0)
	want = "<p>H~2~O is water</p>"
	if got != want {
		t.Errorf("No flag (literal tilde):\ninput:  %q\nwant:   %q\ngot:    %q", md, want, got)
	}

	// Subscript at word boundaries
	md = "x~sub~y\n"
	got = renderHTML(md, parser.FlagSubscripts)
	want = "<p>x<sub>sub</sub>y</p>"
	if got != want {
		t.Errorf("Subscript mid-word:\ninput:  %q\nwant:   %q\ngot:    %q", md, want, got)
	}

	// Subscript cannot open before whitespace
	md = "~ subscript\n"
	got = renderHTML(md, parser.FlagSubscripts)
	want = "<p>~ subscript</p>"
	if got != want {
		t.Errorf("Subscript cannot open before whitespace:\ninput:  %q\nwant:   %q\ngot:    %q", md, want, got)
	}
}

func TestFlagSubscriptsWithStrikethrough(t *testing.T) {
	// Both flags enabled: single ~ = subscript, double ~~ = strikethrough
	flags := parser.FlagSubscripts | parser.FlagStrikethrough

	md := "H~2~O and ~~deleted~~\n"
	got := renderHTML(md, flags)
	want := "<p>H<sub>2</sub>O and <del>deleted</del></p>"
	if got != want {
		t.Errorf("Subscript + Strikethrough:\ninput:  %q\nwant:   %q\ngot:    %q", md, want, got)
	}

	// Only strikethrough flag: single ~ is literal
	md = "H~2~O\n"
	got = renderHTML(md, parser.FlagStrikethrough)
	want = "<p>H~2~O</p>"
	if got != want {
		t.Errorf("Only Strikethrough (no subscript):\ninput:  %q\nwant:   %q\ngot:    %q", md, want, got)
	}
}

func TestFlagLatexMathSpans(t *testing.T) {
	// With parser.FlagLatexMathSpans, $...$ creates inline LaTeX math
	md := "The $x^2$ equation\n"
	got := renderHTML(md, parser.FlagLatexMathSpans)
	want := "<p>The <x-equation>x^2</x-equation> equation</p>"
	if got != want {
		t.Errorf("parser.FlagLatexMathSpans inline:\ninput:  %q\nwant:   %q\ngot:    %q", md, want, got)
	}

	// Without the flag, $ is literal text
	got = renderHTML(md, 0)
	want = "<p>The $x^2$ equation</p>"
	if got != want {
		t.Errorf("No flag (literal dollar):\ninput:  %q\nwant:   %q\ngot:    %q", md, want, got)
	}
}

func TestFlagLatexMathDisplay(t *testing.T) {
	// $$...$$ creates display-mode LaTeX math
	md := "$$E=mc^2$$\n"
	got := renderHTML(md, parser.FlagLatexMathSpans)
	want := "<p><x-equation type=\"display\">E=mc^2</x-equation></p>"
	if got != want {
		t.Errorf("LaTeX display mode:\ninput:  %q\nwant:   %q\ngot:    %q", md, want, got)
	}
}

func TestLatexMathLengthMatching(t *testing.T) {
	// $...$$ should NOT match (length mismatch)
	md := "The $x^2$$ text\n"
	got := renderHTML(md, parser.FlagLatexMathSpans)
	// The $ should match with the first $ of $$, and the remaining $ is literal
	// Actually in md4c: $x^2$$ — opener $ (len 1), closer candidates: first $ of $$ (len 2)
	// Since lengths don't match, the opener stays. Then second $$ can match as opener+closer.
	// Let's verify with md4c behavior: $x^2$$ → <x-equation>x^2</x-equation>$
	// Wait, actually in md4c, the mark for $$ has beg/end covering both $ chars,
	// so it's a single mark with length 2. The single $ has length 1.
	// Length 1 opener can't match length 2 closer, and length 2 closer can't match length 1 opener.
	// So: $ (opener len 1, no matching closer), $$ (opener+closer len 2, self-paired? No, $$ is one mark)
	// Actually, $$ is a single mark with POTENTIAL_OPENER|POTENTIAL_CLOSER.
	// It can pair with another $$ mark. The single $ can pair with another $ mark.
	// In "$x^2$$", we have marks: $ (off=4, len=1), $$ (off=8, len=2).
	// $ mark is potential opener, $$ mark is potential closer.
	// analyzeDollar for $$: it's a potential closer, checks dollarStack top which is $ (len 1).
	// Lengths don't match (1 != 2), so no resolution. $$ is then a potential opener, pushed.
	// No closer for $, no closer for $$. Both stay unresolved.
	// Result: literal "$x^2$$"
	want := "<p>The $x^2$$ text</p>"
	if got != want {
		t.Errorf("LaTeX length mismatch ($...$$):\ninput:  %q\nwant:   %q\ngot:    %q", md, want, got)
	}

	// $$...$ should also NOT match
	md = "$$x^2$ text\n"
	got = renderHTML(md, parser.FlagLatexMathSpans)
	want = "<p>$$x^2$ text</p>"
	if got != want {
		t.Errorf("LaTeX length mismatch ($$...$):\ninput:  %q\nwant:   %q\ngot:    %q", md, want, got)
	}

	// $...$ matches correctly
	md = "The $x^2$ is inline\n"
	got = renderHTML(md, parser.FlagLatexMathSpans)
	want = "<p>The <x-equation>x^2</x-equation> is inline</p>"
	if got != want {
		t.Errorf("LaTeX $...$ matching:\ninput:  %q\nwant:   %q\ngot:    %q", md, want, got)
	}

	// $$...$$ matches correctly
	md = "$$x^2$$ is display\n"
	got = renderHTML(md, parser.FlagLatexMathSpans)
	want = "<p><x-equation type=\"display\">x^2</x-equation> is display</p>"
	if got != want {
		t.Errorf("LaTeX $$...$$ matching:\ninput:  %q\nwant:   %q\ngot:    %q", md, want, got)
	}
}

func TestLatexMathNoNesting(t *testing.T) {
	// LaTeX math does not allow nesting.
	// md4c behavior: analyzeDollar processes marks in order; when a closer finds
	// a matching opener on the stack, they pair and the stack is cleared.
	// For "$a $b$ c$": marks at positions 3 and 5 ($b$) pair first,
	// leaving the outer $ at pos 0 and $ at pos 8 unpaired (literal text).
	// Verified: echo '$a $b$ c$' | md2html --flatex-math
	md := "$a $b$ c$\n"
	got := renderHTML(md, parser.FlagLatexMathSpans)
	want := "<p>$a <x-equation>b</x-equation> c$</p>"
	if got != want {
		t.Errorf("LaTeX no nesting ($a $b$ c$):\ninput:  %q\nwant:   %q\ngot:    %q", md, want, got)
	}
}

func TestLatexMathThreeDollars(t *testing.T) {
	// Three or more $ signs are not treated as marks (literal text)
	md := "$$$ text\n"
	got := renderHTML(md, parser.FlagLatexMathSpans)
	want := "<p>$$$ text</p>"
	if got != want {
		t.Errorf("Three dollar signs (literal):\ninput:  %q\nwant:   %q\ngot:    %q", md, want, got)
	}
}

func TestFlagSuperscripts(t *testing.T) {
	// With parser.FlagSuperscripts, ^ creates <sup> tags
	md := "x^2^ is squared\n"
	got := renderHTML(md, parser.FlagSuperscripts)
	want := "<p>x<sup>2</sup> is squared</p>"
	if got != want {
		t.Errorf("parser.FlagSuperscripts:\ninput:  %q\nwant:   %q\ngot:    %q", md, want, got)
	}

	// Without the flag, ^ is literal
	got = renderHTML(md, 0)
	want = "<p>x^2^ is squared</p>"
	if got != want {
		t.Errorf("No flag (literal caret):\ninput:  %q\nwant:   %q\ngot:    %q", md, want, got)
	}
}

func TestFlagHighlight(t *testing.T) {
	// With parser.FlagHighlight, == creates <mark> tags
	md := "This is ==highlighted== text\n"
	got := renderHTML(md, parser.FlagHighlight)
	want := "<p>This is <mark>highlighted</mark> text</p>"
	if got != want {
		t.Errorf("parser.FlagHighlight:\ninput:  %q\nwant:   %q\ngot:    %q", md, want, got)
	}

	// Without the flag, == is literal
	got = renderHTML(md, 0)
	want = "<p>This is ==highlighted== text</p>"
	if got != want {
		t.Errorf("No flag (literal equal):\ninput:  %q\nwant:   %q\ngot:    %q", md, want, got)
	}
}

// --- I33: Footnotes tests ---

func TestFlagFootnotes(t *testing.T) {
	// Basic footnote reference and definition
	md := "Here is a footnote[^1]\n\n[^1]: The footnote text\n"
	got := renderHTML(md, parser.FlagFootnotes)
	if !strings.Contains(got, `<sup><a href="#fn-1" id="fnref-1-1">1</a></sup>`) {
		t.Errorf("Footnote ref HTML:\ninput:  %q\ngot:    %q", md, got)
	}
	if !strings.Contains(got, `<section class="footnotes">`) {
		t.Errorf("Footnote section:\ninput:  %q\ngot:    %q", md, got)
	}
	if !strings.Contains(got, `<li id="fn-1">`) {
		t.Errorf("Footnote def li:\ninput:  %q\ngot:    %q", md, got)
	}
	if !strings.Contains(got, `class="footnote-backref"`) {
		t.Errorf("Footnote backref:\ninput:  %q\ngot:    %q", md, got)
	}
	if !strings.Contains(got, `</section>`) {
		t.Errorf("Footnote section close:\ninput:  %q\ngot:    %q", md, got)
	}
}

func TestFootnotesMultiple(t *testing.T) {
	// Multiple footnotes — numbered in reference order
	md := "First[^b] then second[^a]\n\n[^a]: Alpha\n[^b]: Beta\n"
	got := renderHTML(md, parser.FlagFootnotes)
	// First referenced is [^b] → id=1, second is [^a] → id=2
	if !strings.Contains(got, `<sup><a href="#fn-1" id="fnref-1-1">1</a></sup>`) {
		t.Errorf("First ref (fn-1):\ngot: %q", got)
	}
	if !strings.Contains(got, `<sup><a href="#fn-2" id="fnref-2-1">2</a></sup>`) {
		t.Errorf("Second ref (fn-2):\ngot: %q", got)
	}
}

func TestFootnotesMultipleReferences(t *testing.T) {
	// Same footnote referenced multiple times — backrefs for each
	md := "Ref[^1] again[^1]\n\n[^1]: The note\n"
	got := renderHTML(md, parser.FlagFootnotes)
	// First ref has fnref-1-1, second has fnref-1-2
	if !strings.Contains(got, `fnref-1-1`) {
		t.Errorf("First ref fnref-1-1:\ngot: %q", got)
	}
	if !strings.Contains(got, `fnref-1-2`) {
		t.Errorf("Second ref fnref-1-2:\ngot: %q", got)
	}
	// Backref link should have both references
	if !strings.Contains(got, `href="#fnref-1-1" class="footnote-backref"`) {
		t.Errorf("First backref:\ngot: %q", got)
	}
	if !strings.Contains(got, `href="#fnref-1-2" class="footnote-backref"`) {
		t.Errorf("Second backref:\ngot: %q", got)
	}
}

func TestFootnotesNoFlag(t *testing.T) {
	// Without parser.FlagFootnotes, [^1] is literal text
	md := "Here is a footnote[^1]\n\n[^1]: The footnote text\n"
	got := renderHTML(md, 0)
	if strings.Contains(got, `<sup>`) {
		t.Errorf("Without parser.FlagFootnotes, no footnote HTML:\ngot: %q", got)
	}
	if strings.Contains(got, `<section class="footnotes">`) {
		t.Errorf("Without parser.FlagFootnotes, no footnote section:\ngot: %q", got)
	}
}

func TestFootnotesEmptyLabel(t *testing.T) {
	// [^] with empty label is NOT a footnote
	md := "Not a footnote[^]\n\n[^]: text\n"
	got := renderHTML(md, parser.FlagFootnotes)
	// Should be treated as regular text, not a footnote ref
	if strings.Contains(got, `<sup><a href`) {
		t.Errorf("Empty label should not be footnote:\ngot: %q", got)
	}
}

func TestFootnotesContinuation(t *testing.T) {
	// Footnote definition with continuation lines
	md := "A note[^1]\n\n[^1]: First line\nSecond line\n\nParagraph\n"
	got := renderHTML(md, parser.FlagFootnotes)
	// The footnote content should include both lines
	if !strings.Contains(got, "First line") {
		t.Errorf("Footnote first line:\ngot: %q", got)
	}
	if !strings.Contains(got, "Second line") {
		t.Errorf("Footnote second line:\ngot: %q", got)
	}
}

func TestFootnotesWithInline(t *testing.T) {
	// Footnote content with inline emphasis
	md := "Note[^1]\n\n[^1]: This is *emphasized*\n"
	got := renderHTML(md, parser.FlagFootnotes)
	if !strings.Contains(got, `<em>emphasized</em>`) {
		t.Errorf("Footnote with emphasis:\ngot: %q", got)
	}
}

func TestFootnotesUnreferenced(t *testing.T) {
	// Unreferenced footnote definitions should NOT be emitted
	md := "Just text\n\n[^1]: Unreferenced note\n"
	got := renderHTML(md, parser.FlagFootnotes)
	if strings.Contains(got, `<section class="footnotes">`) {
		t.Errorf("Unreferenced footnote should not be emitted:\ngot: %q", got)
	}
}

// --- I34: Admonition tests ---

func TestFlagAdmonitionNote(t *testing.T) {
	// Basic [!note] admonition
	md := "> [!note]\n> This is a note\n"
	got := renderHTML(md, parser.FlagAdmonitions)
	if !strings.Contains(got, `<div class="admonition-note">`) {
		t.Errorf("Admonition note div:\ninput:  %q\ngot:    %q", md, got)
	}
	if !strings.Contains(got, `<p class="admonition-title">note</p>`) {
		t.Errorf("Admonition note title:\ninput:  %q\ngot:    %q", md, got)
	}
	if !strings.Contains(got, "This is a note") {
		t.Errorf("Admonition note content:\ninput:  %q\ngot:    %q", md, got)
	}
	if !strings.Contains(got, `</div>`) {
		t.Errorf("Admonition close div:\ninput:  %q\ngot:    %q", md, got)
	}
}

func TestFlagAdmonitionAllTypes(t *testing.T) {
	types := []string{"note", "tip", "important", "warning", "caution"}
	for _, typ := range types {
		md := "> [!" + typ + "]\n> Content\n"
		got := renderHTML(md, parser.FlagAdmonitions)
		want := `<div class="admonition-` + typ + `">`
		if !strings.Contains(got, want) {
			t.Errorf("Admonition type %s:\ninput:  %q\nwant:   contains %q\ngot:    %q", typ, md, want, got)
		}
		wantTitle := `<p class="admonition-title">` + typ + `</p>`
		if !strings.Contains(got, wantTitle) {
			t.Errorf("Admonition title for %s:\ninput:  %q\nwant:   contains %q\ngot:    %q", typ, md, wantTitle, got)
		}
	}
}

func TestFlagAdmonitionCaseInsensitive(t *testing.T) {
	// [!NOTE] should match case-insensitively
	md := "> [!NOTE]\n> Content\n"
	got := renderHTML(md, parser.FlagAdmonitions)
	if !strings.Contains(got, `<div class="admonition-note">`) {
		t.Errorf("Admonition case insensitive:\ninput:  %q\ngot:    %q", md, got)
	}
	if !strings.Contains(got, `<p class="admonition-title">note</p>`) {
		t.Errorf("Admonition case insensitive title:\ninput:  %q\ngot:    %q", md, got)
	}
}

func TestFlagAdmonitionNoFlag(t *testing.T) {
	// Without parser.FlagAdmonitions, [!note] is regular blockquote text
	md := "> [!note]\n> Content\n"
	got := renderHTML(md, 0)
	if strings.Contains(got, `admonition`) {
		t.Errorf("Without parser.FlagAdmonitions, no admonition:\ngot: %q", got)
	}
	if !strings.Contains(got, `<blockquote>`) {
		t.Errorf("Without parser.FlagAdmonitions, should be blockquote:\ngot: %q", got)
	}
}

func TestFlagAdmonitionNoSpaceAfterMark(t *testing.T) {
	// >[!note] (no space after >) should still work
	md := ">[!note]\n>Content\n"
	got := renderHTML(md, parser.FlagAdmonitions)
	if !strings.Contains(got, `<div class="admonition-note">`) {
		t.Errorf("Admonition no space after >:\ninput:  %q\ngot:    %q", md, got)
	}
}

func TestFlagAdmonitionInvalidTag(t *testing.T) {
	// [!invalid] is not a recognized tag — should be regular blockquote
	md := "> [!invalid]\n> Content\n"
	got := renderHTML(md, parser.FlagAdmonitions)
	if strings.Contains(got, `admonition`) {
		t.Errorf("Invalid tag should not produce admonition:\ngot: %q", got)
	}
	if !strings.Contains(got, `<blockquote>`) {
		t.Errorf("Invalid tag should produce blockquote:\ngot: %q", got)
	}
}

func TestFlagAdmonitionExtraTextAfterTag(t *testing.T) {
	// [!note] extra text — should NOT be admonition (must be exact match)
	md := "> [!note] extra\n> Content\n"
	got := renderHTML(md, parser.FlagAdmonitions)
	if strings.Contains(got, `admonition`) {
		t.Errorf("Extra text after tag should not produce admonition:\ngot: %q", got)
	}
	if !strings.Contains(got, `<blockquote>`) {
		t.Errorf("Extra text should produce blockquote:\ngot: %q", got)
	}
}

func TestFlagAdmonitionMultiLine(t *testing.T) {
	// Multi-line admonition content
	md := "> [!warning]\n> First line\n> Second line\n"
	got := renderHTML(md, parser.FlagAdmonitions)
	if !strings.Contains(got, `<div class="admonition-warning">`) {
		t.Errorf("Multi-line admonition div:\ngot: %q", got)
	}
	if !strings.Contains(got, "First line") {
		t.Errorf("Multi-line admonition first line:\ngot: %q", got)
	}
	if !strings.Contains(got, "Second line") {
		t.Errorf("Multi-line admonition second line:\ngot: %q", got)
	}
}

func TestFlagAdmonitionEmptyContent(t *testing.T) {
	// Admonition with no content after tag
	md := "> [!tip]\n"
	got := renderHTML(md, parser.FlagAdmonitions)
	if !strings.Contains(got, `<div class="admonition-tip">`) {
		t.Errorf("Empty admonition div:\ngot: %q", got)
	}
	if !strings.Contains(got, `<p class="admonition-title">tip</p>`) {
		t.Errorf("Empty admonition title:\ngot: %q", got)
	}
}

func TestFlagAdmonitionNestedInList(t *testing.T) {
	// Admonition nested inside a list item
	md := "- item\n  > [!note]\n  > Content\n"
	got := renderHTML(md, parser.FlagAdmonitions|parser.FlagTasklists)
	if !strings.Contains(got, `<div class="admonition-note">`) {
		t.Errorf("Nested admonition div:\ngot: %q", got)
	}
}

// --- I35: Wikilinks tests ---

func TestFlagWikilinkBasic(t *testing.T) {
	// Basic wikilink [[target]]
	// Mirrors md4c: [[target]] → <x-wikilink data-target="target">target</x-wikilink>
	md := "[[target]]\n"
	got := renderHTML(md, parser.FlagWikilinks)
	if !strings.Contains(got, `<x-wikilink data-target="target">`) {
		t.Errorf("Wikilink open tag:\ngot: %q", got)
	}
	if !strings.Contains(got, `target</x-wikilink>`) {
		t.Errorf("Wikilink close tag:\ngot: %q", got)
	}
}

func TestFlagWikilinkWithLabel(t *testing.T) {
	// Wikilink with label: [[target|label]]
	// Mirrors md4c: [[target|label]] → <x-wikilink data-target="target">label</x-wikilink>
	md := "[[target|label]]\n"
	got := renderHTML(md, parser.FlagWikilinks)
	if !strings.Contains(got, `<x-wikilink data-target="target">`) {
		t.Errorf("Wikilink with label target:\ngot: %q", got)
	}
	if !strings.Contains(got, `label</x-wikilink>`) {
		t.Errorf("Wikilink with label text:\ngot: %q", got)
	}
}

func TestFlagWikilinkNoFlag(t *testing.T) {
	// Without parser.FlagWikilinks, [[...]] is literal text
	md := "[[target]]\n"
	got := renderHTML(md, 0)
	if strings.Contains(got, "x-wikilink") {
		t.Errorf("Wikilink without flag should be literal:\ngot: %q", got)
	}
	if !strings.Contains(got, "[[target]]") {
		t.Errorf("Literal brackets expected:\ngot: %q", got)
	}
}

func TestFlagWikilinkEmpty(t *testing.T) {
	// Empty wikilink [[]] is not a valid wikilink (destination must be > 0 chars)
	md := "[[]]\n"
	got := renderHTML(md, parser.FlagWikilinks)
	if strings.Contains(got, "x-wikilink") {
		t.Errorf("Empty wikilink should be literal:\ngot: %q", got)
	}
}

func TestFlagWikilinkPipeAtEnd(t *testing.T) {
	// [[foo|]] — pipe with no label after it
	// Mirrors md4c: [[foo|]] → <x-wikilink data-target="foo">
	md := "[[foo|]]\n"
	got := renderHTML(md, parser.FlagWikilinks)
	if !strings.Contains(got, `<x-wikilink data-target="foo">`) {
		t.Errorf("Wikilink pipe at end:\ngot: %q", got)
	}
}

func TestFlagWikilinkWithEmphasis(t *testing.T) {
	// Emphasis inside wikilink label
	md := "[[target|*label*]]\n"
	got := renderHTML(md, parser.FlagWikilinks)
	if !strings.Contains(got, `<x-wikilink data-target="target">`) {
		t.Errorf("Wikilink target with emphasis in label:\ngot: %q", got)
	}
	if !strings.Contains(got, `<em>label</em>`) {
		t.Errorf("Emphasis in wikilink label:\ngot: %q", got)
	}
}

func TestFlagWikilinkNotImage(t *testing.T) {
	// ![foo](/url) should NOT be a wikilink even with parser.FlagWikilinks
	md := "![foo](/url)\n"
	got := renderHTML(md, parser.FlagWikilinks)
	if strings.Contains(got, "x-wikilink") {
		t.Errorf("Image should not be wikilink:\ngot: %q", got)
	}
	if !strings.Contains(got, `<img`) {
		t.Errorf("Image tag expected:\ngot: %q", got)
	}
}

func TestFlagWikilinkNotLink(t *testing.T) {
	// [foo](/url) should NOT be a wikilink even with parser.FlagWikilinks
	md := "[foo](/url)\n"
	got := renderHTML(md, parser.FlagWikilinks)
	if strings.Contains(got, "x-wikilink") {
		t.Errorf("Link should not be wikilink:\ngot: %q", got)
	}
	if !strings.Contains(got, `<a href="/url">`) {
		t.Errorf("Link tag expected:\ngot: %q", got)
	}
}

func TestFlagWikilinkMultiple(t *testing.T) {
	// Multiple wikilinks in same paragraph
	md := "[[foo]] and [[bar|baz]]\n"
	got := renderHTML(md, parser.FlagWikilinks)
	if strings.Count(got, "<x-wikilink") != 2 {
		t.Errorf("Expected 2 wikilinks:\ngot: %q", got)
	}
}

func TestFlagWikilinkWithTablePipe(t *testing.T) {
	// When both parser.FlagWikilinks and parser.FlagTables are set, | inside [[...]] is wikilink delimiter
	md := "[[target|label]]\n"
	got := renderHTML(md, parser.FlagWikilinks|parser.FlagTables)
	if !strings.Contains(got, `<x-wikilink data-target="target">`) {
		t.Errorf("Wikilink with table flag:\ngot: %q", got)
	}
}

// --- I36: C-31 NULL character replacement tests ---
// Mirrors md4c md_text_with_null_replacement() (md4c.c:410-437).

func TestNullCharInParagraph(t *testing.T) {
	// NULL in normal text → U+FFFD
	md := "foo\x00bar\n"
	got := renderHTML(md, 0)
	if !strings.Contains(got, "foo\uFFFDbar") {
		t.Errorf("NULL in paragraph:\ngot: %q", got)
	}
}

func TestNullCharInCodeSpan(t *testing.T) {
	// NULL inside code span → U+FFFD
	md := "foo `bar\x00baz` qux\n"
	got := renderHTML(md, 0)
	if !strings.Contains(got, "bar\uFFFDbaz") {
		t.Errorf("NULL in code span:\ngot: %q", got)
	}
}

func TestNullCharInCodeBlock(t *testing.T) {
	// NULL inside fenced code block → U+FFFD
	md := "```\nfoo\x00bar\n```\n"
	got := renderHTML(md, 0)
	if !strings.Contains(got, "foo\uFFFDbar") {
		t.Errorf("NULL in code block:\ngot: %q", got)
	}
}

func TestNullCharInHTMLBlock(t *testing.T) {
	// NULL inside HTML block → U+FFFD
	md := "<div>\nfoo\x00bar\n</div>\n"
	got := renderHTML(md, 0)
	if !strings.Contains(got, "foo\uFFFDbar") {
		t.Errorf("NULL in HTML block:\ngot: %q", got)
	}
}

func TestNullCharMultiple(t *testing.T) {
	// Multiple NULLs in text
	md := "a\x00b\x00c\n"
	got := renderHTML(md, 0)
	if !strings.Contains(got, "a\uFFFDb\uFFFDc") {
		t.Errorf("Multiple NULLs:\ngot: %q", got)
	}
}

// --- I36: C-35 code_fence_length tracking tests ---
// Mirrors md4c ctx->code_fence_length (md4c.c:287, 6022, 6050).

func TestFenceLengthClosingShorter(t *testing.T) {
	// Opening: 4 tildes. Closing: 3 tildes → NOT a valid close
	md := "~~~~\nfoo\n~~~\n"
	got := renderHTML(md, 0)
	if !strings.Contains(got, "~~~") {
		t.Errorf("3 tildes should not close 4-tilde fence:\ngot: %q", got)
	}
}

func TestFenceLengthClosingEqual(t *testing.T) {
	// Opening: 4 tildes. Closing: 4 tildes → valid close
	md := "~~~~\nfoo\n~~~~\n"
	got := renderHTML(md, 0)
	if !strings.Contains(got, "<pre><code>") {
		t.Errorf("4 tildes should close 4-tilde fence:\ngot: %q", got)
	}
	if strings.Contains(got, "~~~~") {
		t.Errorf("Closing fence should not appear in output:\ngot: %q", got)
	}
}

func TestFenceLengthClosingLonger(t *testing.T) {
	// Opening: 4 tildes. Closing: 7 tildes → valid close (CommonMark spec example 143)
	md := "~~~~    ruby startline=3 $%@#$\ndef foo(x)\n  return 3\nend\n~~~~~~~\n"
	got := renderHTML(md, 0)
	if !strings.Contains(got, `<code class="language-ruby">`) {
		t.Errorf("7 tildes should close 4-tilde fence:\ngot: %q", got)
	}
	if strings.Contains(got, "~~~~~~~") {
		t.Errorf("Closing fence should not appear in output:\ngot: %q", got)
	}
}

func TestFenceLengthBacktickFence(t *testing.T) {
	// Backtick fence: info string cannot contain backticks (md4c.c:6030-6032)
	// This input is NOT a fenced code block because ``` aa ``` contains backticks in info
	md := "``` aa ``` foo\nbar\n```\n"
	got := renderHTML(md, 0)
	// Should be parsed as paragraph with inline code, not fenced code block
	if strings.Contains(got, "<pre><code class=") {
		t.Errorf("Backtick fence with backtick in info string should be rejected:\ngot: %q", got)
	}
}

// --- I36: C-34 HTML horizon optimization tests ---

func TestHTMLHorizonMultipleComments(t *testing.T) {
	// Multiple HTML comment scans should use horizon optimization
	md := "<!-- comment1 --> text <!-- comment2 -->\n"
	got := renderHTML(md, 0)
	if !strings.Contains(got, "<!-- comment1 -->") || !strings.Contains(got, "<!-- comment2 -->") {
		t.Errorf("Multiple HTML comments:\ngot: %q", got)
	}
}

func TestHTMLHorizonUnclosedComment(t *testing.T) {
	// An unclosed comment should not prevent subsequent comment detection
	md := "<!-- no closer here\ntext\n<!-- comment -->\n"
	got := renderHTML(md, 0)
	// The first <!-- should not be detected as inline HTML (no closer)
	// The second <!-- should be detected
	if !strings.Contains(got, "<!-- comment -->") {
		t.Errorf("Second comment should be detected:\ngot: %q", got)
	}
}
