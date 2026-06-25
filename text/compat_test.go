package text

import (
	"bytes"
	"strings"
	"testing"

	"github.com/userpro/md4go/parser"
)

// TestExtractHTMLText verifies that extractHTMLText correctly strips HTML
// tags, handles non-visible elements (script/style), and preserves literal
// '<' that doesn't start a valid HTML construct.
func TestExtractHTMLText(t *testing.T) {
	tests := []struct {
		name string
		html string
		want string
	}{
		// Regular tags — strip tags, keep text
		{"simple_tag", "<span>hello</span>", "hello"},
		{"nested_tags", "<div><p>text</p></div>", "text"},
		{"with_attrs", `<a href="url">link</a>`, "link"},
		{"self_closing", "<br/>", ""},
		{"multiple_tags", "<b>bold</b> and <i>italic</i>", "bold and italic"},

		// Non-visible elements — strip entire content
		{"script", "<script>alert(1)</script>", ""},
		{"style", "<style>body { }</style>", ""},
		{"script_with_nested_gt", "<script>if (a > b) {}</script>", ""},
		{"script_with_nested_tag", "<script>var x = '<div>';</script>", ""},
		{"title", "<title>Page Title</title>", ""},
		{"noscript", "<noscript>fallback</noscript>", ""},

		// Mixed visible and non-visible
		{"mixed", "before <script>bad</script> after", "before  after"},
		{"script_between_text", "a<script>x</script>b", "ab"},

		// HTML comments — stripped
		{"comment", "<!-- comment -->", ""},
		{"comment_with_text", "before <!-- c --> after", "before  after"},

		// CDATA, PI, declarations — stripped
		{"cdata", "<![CDATA[data]]>", ""},
		{"pi", "<?php ?>", ""},
		{"declaration", "<!DOCTYPE html>", ""},

		// Literal '<' (not a valid HTML construct) — preserved
		{"literal_lt", "a < b", "a < b"},
		{"literal_lt_digit", "if (x <3) {}", "if (x <3) {}"},
		{"literal_lt_space", "< text", "< text"},
		{"math_expression", "1 < 2 > 0", "1 < 2 > 0"},

		// Empty input
		{"empty", "", ""},
		{"no_html", "plain text", "plain text"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := string(extractHTMLText([]byte(tt.html)))
			if got != tt.want {
				t.Errorf("extractHTMLText(%q):\n  want %q\n  got  %q", tt.html, tt.want, got)
			}
		})
	}
}

// TestExtractHTMLTextTracked verifies that extractHTMLTextTracked correctly
// returns the unclosed non-visible element tag name when an opening tag
// has no matching closing tag in the same fragment.
func TestExtractHTMLTextTracked(t *testing.T) {
	tests := []struct {
		name         string
		html         string
		wantText     string
		wantUnclosed string
	}{
		// Closed non-visible element — no unclosed tag
		{"script_closed", "<script>x</script>", "", ""},
		{"style_closed", "<style>a</style>", "", ""},
		{"script_with_attrs_closed", "<script type=\"text/javascript\">x</script>", "", ""},
		{"mixed_closed", "before <script>x</script> after", "before  after", ""},

		// Unclosed non-visible element — returns tag name, content skipped
		{"script_unclosed", "<script>alert(1)", "", "script"},
		{"script_unclosed_multiline", "<script>\nvar x = 1;", "", "script"},
		{"style_unclosed", "<style>body {", "", "style"},

		// No non-visible elements
		{"visible_only", "<span>html</span>", "html", ""},
		{"no_html", "plain text", "plain text", ""},
		{"literal_lt", "a < b", "a < b", ""},

		// Self-closing non-visible element — not unclosed
		{"script_self_closing", "<script/>", "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotText, gotUnclosed := extractHTMLTextTracked([]byte(tt.html))
			if string(gotText) != tt.wantText {
				t.Errorf("extractHTMLTextTracked(%q) text = %q, want %q", tt.html, gotText, tt.wantText)
			}
			if gotUnclosed != tt.wantUnclosed {
				t.Errorf("extractHTMLTextTracked(%q) unclosed = %q, want %q", tt.html, gotUnclosed, tt.wantUnclosed)
			}
		})
	}
}

// TestExtractHTMLTextWithFlags verifies the full text pipeline with
// FlagStripHTMLTags enabled, testing integration with the PlainText renderer.
func TestExtractHTMLTextWithFlags(t *testing.T) {
	flags := parser.DialectGitHub | parser.GoldmarkCompat

	tests := []struct {
		name  string
		input string
		want  string
	}{
		// Single-line HTML blocks
		{
			name:  "script_block_single_line",
			input: "<script>content</script>\n",
			want:  "", // HTML block type 1 — full content stripped
		},
		{
			name:  "script_block_with_attrs",
			input: "<script type=\"text/javascript\">alert(1)</script>\n",
			want:  "", // attributes don't affect non-visible element detection
		},
		{
			name:  "span_inline",
			input: "<span>html</span>\n",
			want:  "html", // tags stripped, text kept
		},
		{
			name:  "inline_html_tags",
			input: "text <b>bold</b> end\n",
			want:  "text bold end", // inline tags stripped, text kept
		},

		// Multi-line HTML blocks — non-visible element content stripped
		{
			name:  "script_block_multiline",
			input: "<script>\nvar x = 1;\n</script>\n",
			want:  "", // multi-line script content stripped
		},
		{
			name:  "script_block_multiline_with_text_after",
			input: "<script>\nvar x = 1;\n</script>\nafter\n",
			want:  "after", // script content stripped, text after block kept
		},
		{
			name:  "style_block_multiline",
			input: "<style>\nbody { color: red; }\n</style>\n",
			want:  "", // multi-line style content stripped
		},
		{
			name:  "script_block_with_attrs_multiline",
			input: "<script type=\"text/javascript\">\nalert(1)\n</script>\n",
			want:  "", // multi-line script with attrs stripped
		},

		// Visible HTML blocks — text extracted
		{
			name:  "div_block_multiline",
			input: "<div>\nvisible\n</div>\n",
			want:  "visible", // visible element text kept
		},
		{
			name:  "pre_block",
			input: "<pre>content</pre>\n",
			want:  "content", // pre is visible, text kept
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			_ = Convert([]byte(tt.input), &buf, WithFlags(flags))
			got := strings.TrimSpace(buf.String())
			if got != tt.want {
				t.Errorf("Convert(%q):\n  want %q\n  got  %q", tt.input, tt.want, got)
			}
		})
	}
}
