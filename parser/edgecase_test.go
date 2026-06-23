package parser_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/userpro/md4go/parser"
	"github.com/userpro/md4go/text"
)

// convertPlainText converts markdown to plain text with given flags.
func convertPlainText(t *testing.T, src string, flags parser.Flags) string {
	t.Helper()
	var buf bytes.Buffer
	if err := text.Convert([]byte(src), &buf, text.WithFlags(flags)); err != nil {
		t.Fatalf("Convert failed: %v", err)
	}
	return buf.String()
}

// TestDeepNesting verifies that deeply nested containers don't cause
// stack overflow or excessive memory usage.
func TestDeepNesting(t *testing.T) {
	tests := []struct {
		name  string
		gen   func(n int) []byte
		depth int
	}{
		{
			name:  "nested_blockquotes",
			gen:   func(n int) []byte { return []byte(strings.Repeat("> ", n) + "text\n") },
			depth: 50,
		},
		{
			name: "nested_lists",
			gen: func(n int) []byte {
				var buf bytes.Buffer
				for i := 0; i < n; i++ {
					for j := 0; j < i; j++ {
						buf.WriteString("  ")
					}
					buf.WriteString("- item\n")
				}
				return buf.Bytes()
			},
			depth: 30,
		},
		{
			name: "deep_emphasis",
			gen: func(n int) []byte {
				return []byte(strings.Repeat("*", n) + "text" + strings.Repeat("*", n) + "\n")
			},
			depth: 16,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			src := tt.gen(tt.depth)
			func() {
				defer func() {
					if r := recover(); r != nil {
						t.Fatalf("panic at depth %d: %v", tt.depth, r)
					}
				}()
				convertPlainText(t, string(src), 0)
			}()
		})
	}
}

// TestAllFlagsCombination verifies that all flag combinations don't panic.
func TestAllFlagsCombination(t *testing.T) {
	input := "# Header\n\n**bold** _italic_\n\n- item\n\n    code\n\n<div>html</div>\n"

	flagSets := []struct {
		name  string
		flags parser.Flags
	}{
		{"default", 0},
		{"collapse_whitespace", parser.FlagCollapseWhitespace},
		{"permissive_atx", parser.FlagPermissiveATXHeaders},
		{"no_indented_code", parser.FlagNoIndentedCodeBlocks},
		{"no_html_blocks", parser.FlagNoHTMLBlocks},
		{"no_html_spans", parser.FlagNoHTMLSpans},
		{"no_html", parser.NoHTML},
		{"hard_soft_breaks", parser.FlagHardSoftBreaks},
		{"underline", parser.FlagUnderline},
		{"permissive_autolinks", parser.PermissiveAutolinks},
		{"all_basic", parser.FlagCollapseWhitespace | parser.FlagPermissiveATXHeaders |
			parser.FlagNoHTMLBlocks | parser.FlagNoHTMLSpans},
	}

	for _, fs := range flagSets {
		t.Run(fs.name, func(t *testing.T) {
			func() {
				defer func() {
					if r := recover(); r != nil {
						t.Fatalf("panic with flags %s: %v", fs.name, r)
					}
				}()
				convertPlainText(t, input, fs.flags)
			}()
		})
	}
}
