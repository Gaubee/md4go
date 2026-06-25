package parser_test

import (
	"testing"

	"github.com/userpro/md4go/parser"
)

// TestParseHTMLTagName verifies parser.ParseHTMLTagName correctly extracts
// tag name, closing tag, and self-closing flag from HTML tag fragments.
func TestParseHTMLTagName(t *testing.T) {
	tests := []struct {
		name          string
		tag           string
		wantName      string
		wantClosing   bool
		wantSelfClose bool
	}{
		// Opening tags
		{"simple", "<span>", "span", false, false},
		{"with_attrs", `<a href="url">`, "a", false, false},
		{"with_spaces", `<div class="x" id="y">`, "div", false, false},
		{"uppercase", "<DIV>", "div", false, false},
		{"mixed_case", "<ScRiPt>", "script", false, false},
		{"dashed_name", "<my-element>", "my-element", false, false},

		// Closing tags
		{"closing", "</span>", "span", true, false},
		{"closing_uppercase", "</SCRIPT>", "script", true, false},
		{"closing_with_space", "</style >", "style", true, false},

		// Self-closing tags
		{"self_closing", "<br/>", "br", false, true},
		{"self_closing_space", "<br />", "br", false, true},
		{"self_closing_attrs", `<img src="x"/>`, "img", false, true},
		{"self_closing_space_attrs", `<img src="x" />`, "img", false, true},

		// Tags with '/' in attribute values (NOT self-closing)
		{"attr_with_slash", `<script type="text/javascript">`, "script", false, false},
		{"attr_url_with_slash", `<a href="http://example.com/path">`, "a", false, false},

		// Non-tag constructs (comments, CDATA, PI, declaration) — name is empty
		{"comment", "<!-- c -->", "", false, false},
		{"cdata", "<![CDATA[x]]>", "", false, false},
		{"pi", "<?php ?>", "", false, false},
		{"declaration", "<!DOCTYPE html>", "", false, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			name, closing, selfClose := parser.ParseHTMLTagName([]byte(tt.tag))
			if name != tt.wantName {
				t.Errorf("ParseHTMLTagName(%q) name = %q, want %q", tt.tag, name, tt.wantName)
			}
			if closing != tt.wantClosing {
				t.Errorf("ParseHTMLTagName(%q) isClosing = %v, want %v", tt.tag, closing, tt.wantClosing)
			}
			if selfClose != tt.wantSelfClose {
				t.Errorf("ParseHTMLTagName(%q) isSelfClosing = %v, want %v", tt.tag, selfClose, tt.wantSelfClose)
			}
		})
	}
}

// TestIsNonVisibleElement verifies the non-visible element set.
func TestIsNonVisibleElement(t *testing.T) {
	visible := []string{"div", "span", "p", "a", "pre", "textarea", "h1", "ul", "li"}
	invisible := []string{"script", "style", "head", "title", "noscript", "template"}

	for _, tag := range visible {
		if parser.IsNonVisibleElement(tag) {
			t.Errorf("IsNonVisibleElement(%q) = true, want false", tag)
		}
	}
	for _, tag := range invisible {
		if !parser.IsNonVisibleElement(tag) {
			t.Errorf("IsNonVisibleElement(%q) = false, want true", tag)
		}
	}
}

// TestSkipNonVisibleContent verifies that content between non-visible element
// tags is correctly skipped.
func TestSkipNonVisibleContent(t *testing.T) {
	tests := []struct {
		name  string
		html  string
		start int
		tag   string
		want  int // offset after closing tag
	}{
		{
			name:  "simple_script",
			html:  "<script>alert(1)</script>",
			start: 8, // after <script>
			tag:   "script",
			want:  25, // len(html) = 25
		},
		{
			name:  "style_with_nested_tags",
			html:  "<style>a < b { color: red; }</style>",
			start: 7,
			tag:   "style",
			want:  36, // len(html) = 36
		},
		{
			name:  "nested_same_tag",
			html:  "<script>var x = '<script>';</script>",
			start: 8,
			tag:   "script",
			want:  36, // after </script> at end
		},
		{
			name:  "no_closing_tag",
			html:  "<script>alert(1)",
			start: 8,
			tag:   "script",
			want:  -1, // not found
		},
		{
			name:  "uppercase_closing",
			html:  "<script>content</SCRIPT>",
			start: 8,
			tag:   "script",
			want:  24, // len = 24
		},
		{
			name:  "empty_content",
			html:  "<script></script>",
			start: 8,
			tag:   "script",
			want:  17, // len = 17
		},
		{
			name:  "content_with_text_before_close",
			html:  "<style>body { }</style>after",
			start: 7,
			tag:   "style",
			want:  23, // after </style>
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parser.SkipNonVisibleContent([]byte(tt.html), tt.start, tt.tag)
			if got != tt.want {
				t.Errorf("SkipNonVisibleContent(%q, %d, %q) = %d, want %d",
					tt.html, tt.start, tt.tag, got, tt.want)
			}
		})
	}
}
