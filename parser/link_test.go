package parser_test

import (
	"bytes"
	"testing"

	"md4go/html"
	"md4go/parser"
	"md4go/renderer"
	"md4go/text"
)

// --- Code span tests ---

func TestCodeSpanBasic(t *testing.T) {
	tests := []struct {
		input    string
		wantCode string // expected code content
	}{
		{"`foo`", "foo"},
		{"``foo``", "foo"},
		{"`foo bar`", "foo bar"},
		{"` foo `", "foo"},        // leading/trailing space stripped
		{"`  foo  `", " foo "},    // one space stripped from each side
		{"`  `", "  "},            // all spaces → no stripping
		{"`a``b`", "a``b"},        // single backtick, internal double
		{"``a`b``", "a`b"},        // double backtick, internal single
		{"`foo\nbar`", "foo bar"}, // newline → space
	}

	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			var buf bytes.Buffer
			r := html.NewHTMLWithWriter(renderer.NewBufWriter(&buf))
			p := parser.New(0)
			if err := p.Parse([]byte(tc.input), r); err != nil {
				t.Fatalf("Parse error: %v", err)
			}
			r.Flush()
			got := buf.String()
			if !bytes.Contains([]byte(got), []byte("<code>")) {
				t.Errorf("expected <code> in output, got: %s", got)
			}
			if !bytes.Contains([]byte(got), []byte("</code>")) {
				t.Errorf("expected </code> in output, got: %s", got)
			}
		})
	}
}

func TestCodeSpanUnmatched(t *testing.T) {
	// Unmatched backtick should be literal
	input := "`foo"
	var buf bytes.Buffer
	r := html.NewHTMLWithWriter(renderer.NewBufWriter(&buf))
	p := parser.New(0)
	if err := p.Parse([]byte(input), r); err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	r.Flush()
	got := buf.String()
	if bytes.Contains([]byte(got), []byte("<code>")) {
		t.Errorf("unmatched backtick should not produce <code>, got: %s", got)
	}
}

// --- Inline link tests ---

func TestInlineLinkBasic(t *testing.T) {
	tests := []struct {
		input string
		href  string
	}{
		{"[foo](http://example.com)", "http://example.com"},
		{"[foo](/url)", "/url"},
		{"[foo]()", ""},
		{"[link](<http://example.com>)", "http://example.com"},
	}

	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			var buf bytes.Buffer
			r := html.NewHTMLWithWriter(renderer.NewBufWriter(&buf))
			p := parser.New(0)
			if err := p.Parse([]byte(tc.input), r); err != nil {
				t.Fatalf("Parse error: %v", err)
			}
			r.Flush()
			got := buf.String()
			if !bytes.Contains([]byte(got), []byte("<a href=\""+tc.href+"\"")) {
				t.Errorf("expected <a href=%q in output, got: %s", tc.href, got)
			}
		})
	}
}

func TestInlineLinkWithTitle(t *testing.T) {
	input := `[foo](http://example.com "title")`
	var buf bytes.Buffer
	r := html.NewHTMLWithWriter(renderer.NewBufWriter(&buf))
	p := parser.New(0)
	if err := p.Parse([]byte(input), r); err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	r.Flush()
	got := buf.String()
	if !bytes.Contains([]byte(got), []byte(`title="title"`)) {
		t.Errorf("expected title attribute in output, got: %s", got)
	}
}

func TestInlineImage(t *testing.T) {
	input := `![foo](http://example.com/img.png)`
	var buf bytes.Buffer
	r := html.NewHTMLWithWriter(renderer.NewBufWriter(&buf))
	p := parser.New(0)
	if err := p.Parse([]byte(input), r); err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	r.Flush()
	got := buf.String()
	if !bytes.Contains([]byte(got), []byte("<img")) {
		t.Errorf("expected <img> in output, got: %s", got)
	}
	if !bytes.Contains([]byte(got), []byte(`src="http://example.com/img.png"`)) {
		t.Errorf("expected src attribute in output, got: %s", got)
	}
}

// --- Integration tests: full parse → HTML ---

func TestParseInlineLinkIntegration(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string // substring expected in HTML output
	}{
		{"simple inline link", "[foo](http://example.com)", `<a href="http://example.com">foo</a>`},
		{"inline link with title", `[bar](/url "title")`, `<a href="/url" title="title">bar</a>`},
		{"empty inline link", "[baz]()", `<a href="">baz</a>`},
		{"code span in paragraph", "`code`", "<code>code</code>"},
		{"emphasis + link", "*[foo](url)*", `<em><a href="url">foo</a></em>`},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var buf bytes.Buffer
			r := html.NewHTMLWithWriter(renderer.NewBufWriter(&buf))
			p := parser.New(0)
			if err := p.Parse([]byte(tc.input), r); err != nil {
				t.Fatalf("Parse error: %v", err)
			}
			r.Flush()
			got := buf.String()
			if !bytes.Contains([]byte(got), []byte(tc.want)) {
				t.Errorf("expected %q in output, got: %s", tc.want, got)
			}
		})
	}
}

func TestImageIntegration(t *testing.T) {
	input := `![alt text](http://example.com/image.png "title")`
	var buf bytes.Buffer
	r := html.NewHTMLWithWriter(renderer.NewBufWriter(&buf))
	p := parser.New(0)
	if err := p.Parse([]byte(input), r); err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	r.Flush()
	got := buf.String()
	if !bytes.Contains([]byte(got), []byte(`<img src="http://example.com/image.png"`)) {
		t.Errorf("expected <img> with src in output, got: %s", got)
	}
	if !bytes.Contains([]byte(got), []byte(`title="title"`)) {
		t.Errorf("expected title attribute in output, got: %s", got)
	}
}

// --- Test PlainText output with links ---

func TestPlainTextLink(t *testing.T) {
	input := "[foo](http://example.com)"
	var buf bytes.Buffer
	r := text.NewPlainText(&buf)
	p := parser.New(0)
	if err := p.Parse([]byte(input), r); err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	r.Flush()
	got := buf.String()
	// PlainText should just output the link text
	if got != "\nfoo\n" {
		t.Errorf("PlainText link: got %q, want %q", got, "\nfoo\n")
	}
}

// --- Test bracket nesting rules ---

func TestBracketNestingRules(t *testing.T) {
	// Nested links: inner link resolves, outer does not
	input := "[[inner](url)]"
	var buf bytes.Buffer
	r := html.NewHTMLWithWriter(renderer.NewBufWriter(&buf))
	p := parser.New(0)
	if err := p.Parse([]byte(input), r); err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	r.Flush()
	got := buf.String()
	// Should contain the inner link
	if !bytes.Contains([]byte(got), []byte(`<a href="url">inner</a>`)) {
		t.Errorf("expected inner link in output, got: %s", got)
	}
}

// --- Code span backtick length pairing tests ---

func TestCodeSpanExactBacktickMatch(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string // substring expected in HTML output
	}{
		{"single wraps double content", "` `` `", "<code>``</code>"},
		{"single wraps double with spaces", "`  ``  `", "<code> `` </code>"},
		{"single wraps double mid-text", "` foo `` bar `", "<code>foo `` bar</code>"},
		{"triple no match", "```foo``", "```foo``"},
		{"single literal double code", "`foo``bar``", "`foo<code>bar</code>"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var buf bytes.Buffer
			r := html.NewHTMLWithWriter(renderer.NewBufWriter(&buf))
			p := parser.New(0)
			if err := p.Parse([]byte(tc.input), r); err != nil {
				t.Fatalf("Parse error: %v", err)
			}
			r.Flush()
			got := buf.String()
			if !bytes.Contains([]byte(got), []byte(tc.want)) {
				t.Errorf("expected %q in output, got: %s", tc.want, got)
			}
		})
	}
}

// --- Image alt text with nested links/images ---

func TestImageAltWithNestedLink(t *testing.T) {
	input := `![foo [bar](/url)](/url2)`
	var buf bytes.Buffer
	r := html.NewHTMLWithWriter(renderer.NewBufWriter(&buf))
	p := parser.New(0)
	if err := p.Parse([]byte(input), r); err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	r.Flush()
	got := buf.String()
	if !bytes.Contains([]byte(got), []byte(`alt="foo bar"`)) {
		t.Errorf("expected alt=\"foo bar\" in output, got: %s", got)
	}
	if bytes.Contains([]byte(got), []byte("<a ")) {
		t.Errorf("should not contain <a> tag inside image, got: %s", got)
	}
}

func TestImageAltWithNestedImage(t *testing.T) {
	input := `![foo ![bar](/url)](/url2)`
	var buf bytes.Buffer
	r := html.NewHTMLWithWriter(renderer.NewBufWriter(&buf))
	p := parser.New(0)
	if err := p.Parse([]byte(input), r); err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	r.Flush()
	got := buf.String()
	if !bytes.Contains([]byte(got), []byte(`alt="foo bar"`)) {
		t.Errorf("expected alt=\"foo bar\" in output, got: %s", got)
	}
	if bytes.Count([]byte(got), []byte("<img")) != 1 {
		t.Errorf("expected exactly 1 <img> tag, got: %s", got)
	}
}

// --- Permissive autolink-in-link suppression test ---

func TestPermissiveAutolinkSuppressedInLink(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string // substring expected in HTML output
	}{
		{
			"http autolink inside link text suppressed",
			"[http://example.com](/url)",
			`<a href="/url">http://example.com</a>`,
		},
		{
			"www autolink inside link text suppressed",
			"[www.example.com](/url)",
			`<a href="/url">www.example.com</a>`,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var buf bytes.Buffer
			r := html.NewHTMLWithWriter(renderer.NewBufWriter(&buf))
			p := parser.New(parser.FlagPermissiveURLAutolinks | parser.FlagPermissiveWWWAutolinks)
			if err := p.Parse([]byte(tc.input), r); err != nil {
				t.Fatalf("Parse error: %v", err)
			}
			r.Flush()
			got := buf.String()
			if !bytes.Contains([]byte(got), []byte(tc.want)) {
				t.Errorf("expected %q in output, got: %s", tc.want, got)
			}
			aCount := bytes.Count([]byte(got), []byte("<a "))
			if aCount != 1 {
				t.Errorf("expected exactly 1 <a> tag, got %d in: %s", aCount, got)
			}
		})
	}
}

// --- PlainText entity output alignment test ---

func TestPlainTextEntityVerbatim(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"named entity ouml", "&ouml;", "&ouml;"},
		{"named entity amp", "&amp;", "&amp;"},
		{"named entity nbsp", "&nbsp;", "&nbsp;"},
		{"numeric decimal entity", "&#246;", "&#246;"},
		{"numeric hex entity", "&#xF6;", "&#xF6;"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var buf bytes.Buffer
			r := text.NewPlainText(&buf)
			p := parser.New(0)
			if err := p.Parse([]byte(tc.input), r); err != nil {
				t.Fatalf("Parse error: %v", err)
			}
			r.Flush()
			got := buf.String()
			if !bytes.Contains([]byte(got), []byte(tc.want)) {
				t.Errorf("expected %q in output, got: %q", tc.want, got)
			}
		})
	}
}
