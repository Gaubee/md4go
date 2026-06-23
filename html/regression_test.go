package html_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/userpro/md4go/extension"
	"github.com/userpro/md4go/html"
	"github.com/userpro/md4go/parser"
	"github.com/userpro/md4go/text"
)

// This file collects regression tests for HTML rendering and entity
// handling bugs fixed during iterations I45–I62.

// renderHTML converts markdown to HTML with given options.
func renderHTML(t *testing.T, src string, opts ...html.Option) string {
	t.Helper()
	var buf bytes.Buffer
	if err := html.Convert([]byte(src), &buf, opts...); err != nil {
		t.Fatalf("HTML Convert failed: %v", err)
	}
	return buf.String()
}

// renderPlain converts markdown to plain text with given options.
// Used by tests that verify both HTML and plain text rendering.
func renderPlain(t *testing.T, src string, opts ...text.Option) string {
	t.Helper()
	var buf bytes.Buffer
	if err := text.Convert([]byte(src), &buf, opts...); err != nil {
		t.Fatalf("Convert failed: %v", err)
	}
	return buf.String()
}

// TestHTMLNestedImageSuppression verifies that nested images in HTML output
// are fully suppressed (no <img> tag emitted inside another image's alt text).
// Mirrors md4c-html.c: all spans including nested IMG are blocked when
// inside an image (image_nesting_level > 0).
func TestHTMLNestedImageSuppression(t *testing.T) {
	input := `![outer ![inner](inner.png)](outer.png)`
	got := renderHTML(t, input, html.WithFlags(parser.DialectGitHub))
	imgCount := bytes.Count([]byte(got), []byte("<img"))
	if imgCount != 1 {
		t.Errorf("expected exactly 1 <img> tag, got %d in: %s", imgCount, got)
	}
	if !bytes.Contains([]byte(got), []byte(`alt="outer inner"`)) {
		t.Errorf("expected alt=\"outer inner\" in output, got: %s", got)
	}
}

// TestEmptyLinkDestination verifies that an empty inline link []()
// produces an HTML link with empty href, not a nil href.
// CommonMark spec: []() produces a link with empty-string href.
func TestEmptyLinkDestination(t *testing.T) {
	got := renderHTML(t, "[]()\n")
	if !bytes.Contains([]byte(got), []byte(`href=""`)) {
		t.Errorf("expected href=\"\" in output, got: %s", got)
	}
}

// TestHighlightLengthGuard verifies that highlight marks must be exactly "==".
// Single "=" is not a highlight opener/closer. Mirrors md4c (md4c.c:4320-4322).
func TestHighlightLengthGuard(t *testing.T) {
	opts := []html.Option{
		html.WithFlags(parser.FlagHighlight),
		html.WithExtensions(&extension.Highlight{}),
	}

	got := renderHTML(t, "=not highlight=\n", opts...)
	if bytes.Contains([]byte(got), []byte("<mark>")) {
		t.Errorf("single = should not produce <mark>, got: %s", got)
	}

	got2 := renderHTML(t, "==highlight==\n", opts...)
	if !bytes.Contains([]byte(got2), []byte("<mark>")) {
		t.Errorf("== should produce <mark>, got: %s", got2)
	}
}

// TestSpoilerLengthGuard verifies that spoiler marks must be exactly "||".
// Single "|" is not a spoiler opener/closer. Mirrors md4c (md4c.c:4294-4313).
func TestSpoilerLengthGuard(t *testing.T) {
	opts := []html.Option{
		html.WithFlags(parser.FlagSpoilers),
		html.WithExtensions(&extension.Spoiler{}),
	}

	got := renderHTML(t, "|not spoiler|\n", opts...)
	if bytes.Contains([]byte(got), []byte("<x-spoiler>")) {
		t.Errorf("single | should not produce <x-spoiler>, got: %s", got)
	}

	got2 := renderHTML(t, "||spoiler||\n", opts...)
	if !bytes.Contains([]byte(got2), []byte("<x-spoiler>")) {
		t.Errorf("|| should produce <x-spoiler>, got: %s", got2)
	}
}

// TestNullCharInCodeSpan verifies that NULL characters inside code spans
// are replaced with U+FFFD in both HTML and PlainText output.
// This is an intentional improvement over md4c (S-06).
func TestNullCharInCodeSpan(t *testing.T) {
	input := []byte("`\x00`\n")
	opts := []text.Option{text.WithFlags(parser.DialectGitHub)}

	plainOut := renderPlain(t, string(input), opts...)
	if bytes.Contains([]byte(plainOut), []byte{0x00}) {
		t.Errorf("plaintext output contains raw NULL byte: %q", plainOut)
	}
	if !bytes.Contains([]byte(plainOut), []byte("\xEF\xBF\xBD")) {
		t.Errorf("plaintext output missing U+FFFD: %q", plainOut)
	}

	htmlOpts := []html.Option{html.WithFlags(parser.DialectGitHub)}
	htmlOut := renderHTML(t, string(input), htmlOpts...)
	if bytes.Contains([]byte(htmlOut), []byte{0x00}) {
		t.Errorf("HTML output contains raw NULL byte: %q", htmlOut)
	}
}

// TestHTMLEntityCodepointZero verifies that the HTML entity &#0;
// renders as U+FFFD, not as a NULL byte. This aligns with md4c's
// render_utf8_codepoint which checks "0 < codepoint" and outputs
// U+FFFD for codepoint 0.
func TestHTMLEntityCodepointZero(t *testing.T) {
	got := renderHTML(t, "&#0;\n")
	if bytes.Contains([]byte(got), []byte{0x00}) {
		t.Errorf("HTML output contains raw NULL byte: %v", got)
	}
	if !bytes.Contains([]byte(got), []byte("\xEF\xBF\xBD")) {
		t.Errorf("HTML output missing U+FFFD for &#0;: %s", got)
	}
}

// TestHTMLEntitySurrogatePair verifies that HTML entities for surrogate
// codepoints (U+D800-U+DFFF) render as U+FFFD, matching md4c's
// render_utf8_codepoint behavior.
func TestHTMLEntitySurrogatePair(t *testing.T) {
	tests := []struct {
		name           string
		input          string
		hasReplacement bool
	}{
		{"low_surrogate", "&#xD800;\n", true},
		{"high_surrogate", "&#xDFFF;\n", true},
		{"valid_codepoint", "&#x4E2D;\n", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := renderHTML(t, tt.input)
			hasReplacement := bytes.Contains([]byte(got), []byte("\xEF\xBF\xBD"))
			if hasReplacement != tt.hasReplacement {
				t.Errorf("U+FFFD presence=%v, want=%v, output: %s",
					hasReplacement, tt.hasReplacement, got)
			}
		})
	}
}

// TestHTMLInvalidNumericEntity verifies that invalid numeric entities
// (containing non-hex/non-decimal characters) are output as raw text,
// matching md4c's behavior.
func TestHTMLInvalidNumericEntity(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"invalid_hex_chars", "&#xZZ;\n", "&amp;#xZZ;"},
		{"invalid_dec_chars", "&#abc;\n", "&amp;#abc;"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := renderHTML(t, tt.input)
			if !strings.Contains(got, tt.want) {
				t.Errorf("expected %q in output, got: %s", tt.want, got)
			}
		})
	}
}

// TestWikilinkExtension verifies that [[target|label]] wikilink syntax
// is correctly parsed by md4go. This is an intentional improvement
// over md4c (S-03) which does not support wikilinks.
func TestWikilinkExtension(t *testing.T) {
	opts := []html.Option{
		html.WithFlags(parser.FlagWikilinks),
		html.WithExtensions(&extension.Wikilink{}),
	}
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"simple", "[[target]]\n", "target"},
		{"with_label", "[[target|label]]\n", "label"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := renderHTML(t, tt.input, opts...)
			if !strings.Contains(got, tt.want) {
				t.Errorf("expected %q in output, got: %s", tt.want, got)
			}
		})
	}
}
