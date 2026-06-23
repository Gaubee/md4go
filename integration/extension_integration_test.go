package integration_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/userpro/md4go"
	"github.com/userpro/md4go/extension"
	"github.com/userpro/md4go/html"
	"github.com/userpro/md4go/parser"
	"github.com/userpro/md4go/text"
)

// TestWithExtensionsGFM verifies the GFM extension bundle produces non-empty
// plain-text output for a basic document.
func TestWithExtensionsGFM(t *testing.T) {
	p := md4go.New(md4go.WithExtensions(extension.GFM...))
	var buf bytes.Buffer
	r := text.NewPlainText(&buf)
	if err := p.Parse([]byte("# Hello\n\nworld"), r); err != nil {
		t.Fatalf("Parse failed: %v", err)
	}
	r.Flush()
	if buf.String() == "" {
		t.Error("output should not be empty")
	}
}

// TestWithExtensionsIndividual verifies that individually registered
// extensions take observable effect through the public API:
//   - Table registers '|' as a mark char → table syntax is parsed.
//   - Strikethrough → ~~del~~ renders as <del>.
func TestWithExtensionsIndividual(t *testing.T) {
	p := md4go.New(
		md4go.WithExtensions(&extension.Strikethrough{}, &extension.Table{}),
		md4go.WithFlags(parser.FlagCollapseWhitespace),
	)

	// Table: '|'-delimited syntax becomes a <table>.
	var tb bytes.Buffer
	h := html.NewHTML(&tb)
	if err := p.Parse([]byte("| a | b |\n|---|---|\n| c | d |"), h); err != nil {
		t.Fatalf("table parse: %v", err)
	}
	h.Flush()
	if !strings.Contains(tb.String(), "<table>") {
		t.Errorf("Table extension not active: %q", tb.String())
	}

	// Strikethrough: ~~del~~ → <del>del</del>.
	var sb bytes.Buffer
	h2 := html.NewHTML(&sb)
	if err := p.Parse([]byte("~~del~~"), h2); err != nil {
		t.Fatalf("strike parse: %v", err)
	}
	h2.Flush()
	if !strings.Contains(sb.String(), "<del>") {
		t.Errorf("Strikethrough extension not active: %q", sb.String())
	}
}

// TestWithExtensionsAndFlags verifies that WithFlags may be called multiple
// times (flags OR together) and coexists with WithExtensions. Behaviors:
//   - FlagNoHTMLBlocks: raw <div> block is escaped (no literal <div>).
//   - FlagNoHTMLSpans: inline <b> is escaped (no literal <b>).
//   - Strikethrough extension: ~~strike~~ → <del>.
func TestWithExtensionsAndFlags(t *testing.T) {
	p := md4go.New(
		md4go.WithFlags(parser.FlagNoHTMLBlocks),
		md4go.WithExtensions(&extension.Strikethrough{}),
		md4go.WithFlags(parser.FlagNoHTMLSpans),
	)

	var buf bytes.Buffer
	h := html.NewHTML(&buf)
	if err := p.Parse([]byte("<div>raw</div>\n\n~~strike~~\n\n<b>inline</b>"), h); err != nil {
		t.Fatalf("parse: %v", err)
	}
	h.Flush()
	out := buf.String()

	if strings.Contains(out, "<div>") {
		t.Errorf("FlagNoHTMLBlocks not active: %q", out)
	}
	if !strings.Contains(out, "<del>") {
		t.Errorf("Strikethrough extension not active: %q", out)
	}
	if strings.Contains(out, "<b>") {
		t.Errorf("FlagNoHTMLSpans not active: %q", out)
	}
}

// TestNoExtensionsDefaultBehavior verifies that without extensions:
//   - plain-text output is non-empty for a basic document.
//   - '|' is not a mark char → table syntax is NOT parsed (stays a paragraph).
func TestNoExtensionsDefaultBehavior(t *testing.T) {
	p := md4go.New()

	var buf bytes.Buffer
	r := text.NewPlainText(&buf)
	if err := p.Parse([]byte("# Title\n\nParagraph."), r); err != nil {
		t.Fatalf("Parse failed: %v", err)
	}
	r.Flush()
	if buf.String() == "" {
		t.Error("output should not be empty")
	}

	// Without the Table extension, '|'-delimited syntax is not a table.
	var tb bytes.Buffer
	h := html.NewHTML(&tb)
	if err := p.Parse([]byte("| a | b |\n|---|---|\n| c | d |"), h); err != nil {
		t.Fatalf("parse: %v", err)
	}
	h.Flush()
	if strings.Contains(tb.String(), "<table>") {
		t.Errorf("table should not be parsed without extensions: %q", tb.String())
	}
}
