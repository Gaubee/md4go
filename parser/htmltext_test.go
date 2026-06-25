package parser_test

import (
	"testing"

	"github.com/userpro/md4go/parser"
)

// TestScanHTMLTag verifies parser.ScanHTMLTag returns the correct end offset
// for each HTML construct type, and returns i for non-construct '<'.
func TestScanHTMLTag(t *testing.T) {
	tests := []struct {
		name string
		html string
		want int // end offset after the construct at position 0
	}{
		{"tag", "<span>", 6},
		{"tag_with_attrs", `<a href="url">`, 14},
		{"closing_tag", "</span>", 7},
		{"self_closing", "<br />", 6},
		{"comment", "<!-- c -->", 10},
		{"comment_with_gt", "<!-- a > b -->", 14},
		{"cdata", "<![CDATA[x]]>", 13},
		{"pi", "<?php ?>", 8},
		{"doctype", "<!DOCTYPE html>", 15},
		{"not_tag_digit", "<3", 0},
		{"not_tag_space", "< text", 0},
		{"not_tag_eol", "<", 0},
		{"unterminated", "<span", 0},
		{"unterminated_attr", `<a href="x`, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parser.ScanHTMLTag([]byte(tt.html), 0)
			if got != tt.want {
				t.Errorf("ScanHTMLTag(%q, 0):\n  want %d\n  got  %d", tt.html, tt.want, got)
			}
		})
	}
}
