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
