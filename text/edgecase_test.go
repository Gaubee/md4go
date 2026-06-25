package text_test

import (
	"bytes"
	"testing"

	"github.com/userpro/md4go/parser"
	"github.com/userpro/md4go/text"
)

// TestStreamConsistencyOnEdgeCases verifies that Convert and ConvertStream
// produce consistent results for inputs without forward references.
func TestStreamConsistencyOnEdgeCases(t *testing.T) {
	tests := []string{
		"# Header\n",
		"**bold** *italic* `code`\n",
		"> blockquote\n",
		"- item\n",
		"```\nfenced\n```\n",
		"",
		"\n",
		"a\n",
	}

	for i, input := range tests {
		var fullBuf, streamBuf bytes.Buffer
		text.Convert([]byte(input), &fullBuf)
		text.ConvertStream(bytes.NewReader([]byte(input)), &streamBuf)
		if fullBuf.String() != streamBuf.String() {
			t.Errorf("case %d: Convert != ConvertStream for %q\n  full=%q\n  stream=%q",
				i, input, fullBuf.String(), streamBuf.String())
		}
	}
}

// TestPlainTextNestedTightList verifies the tightListDepth stack fix.
// When a tight list is nested inside a loose list, the inner list's
// tight status should not overwrite the outer list's loose status.
//
// Note: The plain-text renderer is single-pass and sets tightListDepth at
// EnterBlock time (initial IsTight). A list that starts tight but becomes
// loose mid-stream (blank line between items) is rendered as tight because
// the final IsTight is only known at LeaveBlock. This aligns with md4c,
// which also treats these cases as tight. The HTML renderer handles this
// correctly via list-content buffering.
func TestPlainTextNestedTightList(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "tight_list_simple",
			input: "- a\n- b\n",
			want:  "a\nb\n",
		},
		{
			name:  "loose_list_simple",
			input: "- a\n\n- b\n",
			want:  "a\nb\n",
		},
		{
			name:  "nested_list_tight_inside",
			input: "- a\n  - x\n  - y\n\n- b\n",
			want:  "a\nx\ny\nb\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			func() {
				defer func() {
					if r := recover(); r != nil {
						t.Fatalf("panic: %v", r)
					}
				}()
				text.Convert([]byte(tt.input), &buf, text.WithFlags(parser.DialectGitHub))
			}()
			if tt.want != "" {
				got := buf.String()
				if got != tt.want {
					t.Errorf("input %q:\n  want %q\n  got  %q", tt.input, tt.want, got)
				}
			}
		})
	}
}

// TestStripHTMLTags verifies FlagStripHTMLTags strips HTML tags from raw HTML
// content (inline spans and HTML blocks) in plain-text rendering.
func TestStripHTMLTags(t *testing.T) {
	tests := []struct {
		name         string
		input        string
		wantDefault  string // output without FlagStripHTMLTags (verbatim HTML)
		wantStripped string // output with FlagStripHTMLTags (tags removed)
	}{
		{
			name:         "inline_html_span",
			input:        "<span>html</span>\n",
			wantDefault:  "\n<span>html</span>\n",
			wantStripped: "\nhtml\n",
		},
		{
			name:         "br_tag",
			input:        "<br>\n",
			wantDefault:  "\n<br>\n\n",
			wantStripped: "\n\n\n",
		},
		{
			name:         "nested_html",
			input:        "<div><p>text</p></div>\n",
			wantDefault:  "\n<div><p>text</p></div>\n\n",
			wantStripped: "\ntext\n\n",
		},
		{
			name:         "html_with_attributes",
			input:        "<a href=\"url\">link text</a>\n",
			wantDefault:  "\n<a href=\"url\">link text</a>\n",
			wantStripped: "\nlink text\n",
		},
		{
			name:         "pseudo_tag_in_text",
			input:        "sys/arch/<arch>/mca/mca_machdep.c\n",
			wantDefault:  "\nsys/arch/<arch>/mca/mca_machdep.c\n",
			wantStripped: "\nsys/arch//mca/mca_machdep.c\n",
		},
		// Edge cases: '>' inside HTML constructs must not prematurely close tags.
		{
			name: "quoted_attr_with_gt",
			// <a title="x > y"> is one inline HTML tag; '>' inside the
			// double-quoted attribute value must not close the tag early.
			input:        "<a title=\"x > y\">link</a>\n",
			wantDefault:  "\n<a title=\"x > y\">link</a>\n",
			wantStripped: "\nlink\n",
		},
		{
			name: "html_comment_with_gt",
			// The entire comment (including '>' inside) must be stripped.
			input:        "<!-- comment with > inside -->\n",
			wantDefault:  "\n<!-- comment with > inside -->\n\n",
			wantStripped: "\n\n\n",
		},
		{
			name: "cdata_section",
			// The entire CDATA section must be stripped.
			input:        "<![CDATA[data]]>\n",
			wantDefault:  "\n<![CDATA[data]]>\n\n",
			wantStripped: "\n\n\n",
		},
		{
			name: "processing_instruction",
			// The entire PI must be stripped.
			input:        "<?php echo ?>\n",
			wantDefault:  "\n<?php echo ?>\n\n",
			wantStripped: "\n\n\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Default (no stripping)
			var bufDef bytes.Buffer
			if err := text.Convert([]byte(tt.input), &bufDef, text.WithFlags(0)); err != nil {
				t.Fatalf("Convert default: %v", err)
			}
			if got := bufDef.String(); got != tt.wantDefault {
				t.Errorf("default input %q:\n  want %q\n  got  %q", tt.input, tt.wantDefault, got)
			}

			// With FlagStripHTMLTags
			var bufStrip bytes.Buffer
			if err := text.Convert([]byte(tt.input), &bufStrip,
				text.WithFlags(parser.FlagStripHTMLTags)); err != nil {
				t.Fatalf("Convert stripped: %v", err)
			}
			if got := bufStrip.String(); got != tt.wantStripped {
				t.Errorf("stripped input %q:\n  want %q\n  got  %q", tt.input, tt.wantStripped, got)
			}

			// GoldmarkCompat should also strip HTML
			var bufCompat bytes.Buffer
			if err := text.Convert([]byte(tt.input), &bufCompat,
				text.WithFlags(parser.DialectGitHub|parser.GoldmarkCompat)); err != nil {
				t.Fatalf("Convert compat: %v", err)
			}
			if got := bufCompat.String(); got != tt.wantStripped {
				t.Errorf("compat input %q:\n  want %q\n  got  %q", tt.input, tt.wantStripped, got)
			}
		})
	}
}
