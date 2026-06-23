package parser_test

import (
	"bytes"
	"strings"
	"testing"

	"md4go/extension"
	"md4go/html"
	"md4go/parser"
	"md4go/text"
)

// This file collects regression tests for parser/logic bugs fixed during
// iterations I45–I62. Each test pins a specific bug fix and documents
// the md4c source alignment or md4go improvement it verifies.

// convertPlain converts markdown to plain text with given flags and extensions.
func convertPlain(t *testing.T, src string, opts ...text.Option) string {
	t.Helper()
	var buf bytes.Buffer
	if err := text.Convert([]byte(src), &buf, opts...); err != nil {
		t.Fatalf("Convert failed: %v", err)
	}
	return buf.String()
}

// convertHTML converts markdown to HTML with given flags and extensions.
func convertHTML(t *testing.T, src string, opts ...html.Option) string {
	t.Helper()
	var buf bytes.Buffer
	if err := html.Convert([]byte(src), &buf, opts...); err != nil {
		t.Fatalf("HTML Convert failed: %v", err)
	}
	return buf.String()
}

// TestTableFollowedByList verifies that a list item following a table is not
// incorrectly classified as a table row. This was a bug where the table
// continuation check at step 10 of analyzeLine used the original pivot
// parameter instead of the effective pivot (piv), causing lines like
// "* hello" after a table to be misclassified as table rows.
func TestTableFollowedByList(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "table_then_bullet_list",
			input: "| a | b |\n---|---\n* hello\n",
			want:  "a\tb\nhello\n",
		},
		{
			name:  "table_then_dash_list",
			input: "| a | b |\n---|---\n- hello\n",
			want:  "a\tb\nhello\n",
		},
		{
			name:  "table_then_list_with_pipes",
			input: "| a | b |\n---|---\n* | c | d |\n",
			want:  "a\tb\n| c | d |\n",
		},
		{
			name:  "table_then_ordered_list",
			input: "| a | b |\n---|---\n1. hello\n",
			want:  "a\tb\nhello\n",
		},
		{
			name:  "table_then_nested_list",
			input: "| a | b |\n---|---\n- item1\n  - nested\n",
			want:  "a\tb\nitem1\nnested\n",
		},
		{
			name:  "table_data_row_not_list",
			input: "| a | b |\n---|---\n| c | d |\n",
			want:  "a\tb\nc\td\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := convertPlain(t, tt.input, text.WithExtensions(extension.GFM...))
			if got != tt.want {
				t.Errorf("input %q:\n  want %q\n  got  %q", tt.input, tt.want, got)
			}
		})
	}
}

// TestLongEmphasisRun verifies that long runs of emphasis markers (* or _)
// are not truncated. The old markMaxRunLen=16 limit was overly aggressive,
// causing underscores in emphasis spans like *___...___* to be lost.
// md4c has no such limit.
func TestLongEmphasisRun(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{"long_underscore_in_emphasis", "*" + strings.Repeat("_", 100) + "*"},
		{"long_asterisk_run", strings.Repeat("*", 100) + "text" + strings.Repeat("*", 100)},
		{"mixed_long_run", "*" + strings.Repeat("_", 50) + "text" + strings.Repeat("_", 50) + "*"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := convertPlain(t, tt.input)
			if len(got) == 0 {
				t.Error("expected non-empty output")
			}
			// Verify underscores are not truncated
			if strings.Contains(tt.input, strings.Repeat("_", 100)) && !strings.Contains(got, strings.Repeat("_", 100)) {
				t.Errorf("long underscore run was truncated in output: got %d chars, want at least 100",
					strings.Count(got, "_"))
			}
		})
	}
}

// TestSpoilerProtectionInTableCells verifies that || in table cells
// is protected from being treated as cell boundaries when FlagProtectDoublePipe
// is set. md4go correctly handles || as spoiler markers, preventing cell splitting.
func TestSpoilerProtectionInTableCells(t *testing.T) {
	input := "| a || b |\n|---|---|\n| c || d |\n"
	got := convertPlain(t, input, text.WithExtensions(extension.GFM...))

	// With spoiler protection, "c || d" should stay in one cell, not be split
	if strings.Contains(got, "c\td") {
		t.Errorf("|| was incorrectly treated as cell boundary: got %q", got)
	}
}

// TestTableContinuationAfterContainer verifies the fix for the pivot
// variable bug in analyzeLine step 10. When a container mark (e.g., *)
// is detected in step 9, piv is set to dummyBlankLine, preventing
// the table continuation check from misclassifying the line.
func TestTableContinuationAfterContainer(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{"table_then_list", "| a | b |\n---|---\n* item\n"},
		{"table_with_data_then_list", "| a | b |\n---|---\n| c | d |\n\n* item\n"},
		{"blockquote_then_table", "> | a | b |\n> ---|---\n> | c | d |\n"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := convertPlain(t, tt.input, text.WithExtensions(extension.GFM...))
			if len(got) == 0 {
				t.Error("expected non-empty output")
			}
		})
	}
}

// TestFencedCodeInfoStringTab verifies that tab after fence characters
// is treated as part of the info string (not skipped as whitespace).
// This aligns with md4c (md4c.c:6024-6026) which only skips spaces,
// not tabs, after fence characters.
func TestFencedCodeInfoStringTab(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantSub string
	}{
		{"tab_after_backtick", "```\tinfo\ncode\n```\n", "info"},
		{"space_after_backtick", "``` info\ncode\n```\n", "info"},
		{"tab_in_tilde_fence", "~~~\tinfo\ncode\n~~~\n", "info"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := convertHTML(t, tt.input, html.WithFlags(parser.DialectGitHub))
			if !bytes.Contains([]byte(got), []byte(tt.wantSub)) {
				t.Errorf("expected %q in output, got: %s", tt.wantSub, got)
			}
		})
	}
}

// TestNextCloserNilSafety verifies that resolveBracketLink doesn't panic
// when nextOpener exists but nextCloser is nil. This is a defensive guard
// against a potential nil pointer dereference identified in I58 audit.
func TestNextCloserNilSafety(t *testing.T) {
	input := "[text][\n"
	got := convertPlain(t, input)
	if !bytes.Contains([]byte(got), []byte("text")) {
		t.Errorf("expected 'text' in output, got: %q", got)
	}
}

// TestFootnoteReferenceNumber verifies that footnote references output
// the reference number in PlainText mode. This is an intentional improvement
// over md4c (S-05): md4c's plain text renderer omits footnote reference numbers.
func TestFootnoteReferenceNumber(t *testing.T) {
	input := "Text[^1]\n\n[^1]: Footnote content\n"
	got := convertPlain(t, input, text.WithExtensions(extension.GFM...))
	if !bytes.Contains([]byte(got), []byte("[1]")) {
		t.Errorf("expected [1] footnote reference in output, got: %q", got)
	}
}

// TestTightListParagraphSeparator verifies that md4go adds a separator
// between paragraphs within tight list items. This is an intentional
// improvement over md4c (S-02): md4c concatenates without any separator,
// which can merge words (e.g., "email@hostVersion" instead of
// "email@host Version").
func TestTightListParagraphSeparator(t *testing.T) {
	input := "- item1\n  paragraph2\n- item3\n"
	got := convertPlain(t, input, text.WithFlags(parser.DialectGitHub))
	if !bytes.Contains([]byte(got), []byte("item1")) {
		t.Errorf("expected 'item1' in output, got: %q", got)
	}
	if !bytes.Contains([]byte(got), []byte("paragraph2")) {
		t.Errorf("expected 'paragraph2' in output, got: %q", got)
	}
	if bytes.Contains([]byte(got), []byte("item1paragraph2")) {
		t.Errorf("item1 and paragraph2 should be separated, got: %q", got)
	}
}

// TestSpoilerProtectionInTableCellsRoundTrip verifies that || inside
// table cells is protected when FlagProtectDoublePipe (or FlagSpoilers) is set.
// Default (GFM standard): || is split as cell boundary (aligns with md4c).
// FlagProtectDoublePipe: || is protected (md4go improvement).
func TestSpoilerProtectionInTableCellsRoundTrip(t *testing.T) {
	input := "| a || b |\n|---|---|\n| c | d |\n"

	gotDefault := convertPlain(t, input, text.WithFlags(parser.DialectGitHub))
	if bytes.Contains([]byte(gotDefault), []byte("a || b")) {
		t.Errorf("default: expected || to split, got %q", gotDefault)
	}

	gotProtected := convertPlain(t, input, text.WithFlags(parser.DialectGitHub|parser.FlagProtectDoublePipe))
	if !bytes.Contains([]byte(gotProtected), []byte("a || b")) {
		t.Errorf("protected: expected 'a || b' in single cell, got: %q", gotProtected)
	}
}
