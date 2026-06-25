package parser_test

import (
	"strings"
	"testing"

	"github.com/userpro/md4go/parser"
)

// --- processFootnoteDefs multi-line content tests ---
// Verifies that footnote definitions with multi-line content are correctly
// assembled into blockText and processed through the inline pipeline.

func TestFootnoteMultiLineContent(t *testing.T) {
	// Footnote with multiple content lines including emphasis
	src := "Text[^1]\n\n[^1]: First line\n*emphasized* content\n\nNext para"

	got := renderHTMLStr(t, src, parser.DialectGitHub)
	// Verify footnote content is present and emphasis is processed
	if !strings.Contains(got, "First line") {
		t.Errorf("expected 'First line' in output, got %q", got)
	}
	if !strings.Contains(got, "emphasized") {
		t.Errorf("expected 'emphasized' in output, got %q", got)
	}
	if !strings.Contains(got, "<em>") {
		t.Errorf("expected <em> tag for emphasis in footnote, got %q", got)
	}
}

func TestFootnoteMultiLineWithLink(t *testing.T) {
	// Footnote with multiple content lines including a link
	src := "Text[^1]\n\n[^1]: See [link](http://example.com)\nfor more"

	got := renderHTMLStr(t, src, parser.DialectGitHub)
	if !strings.Contains(got, "link") {
		t.Errorf("expected 'link' text in output, got %q", got)
	}
	if !strings.Contains(got, "http://example.com") {
		t.Errorf("expected href in output, got %q", got)
	}
}

func TestFootnoteSingleLineContent(t *testing.T) {
	// Simple single-line footnote — verify still works after refactor
	src := "Text[^1]\n\n[^1]: Simple note"

	got := renderHTMLStr(t, src, parser.DialectGitHub)
	if !strings.Contains(got, "Simple note") {
		t.Errorf("expected 'Simple note' in output, got %q", got)
	}
}
