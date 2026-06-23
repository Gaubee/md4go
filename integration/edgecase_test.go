package integration_test

import (
	"bytes"
	"testing"

	"md4go"
	"md4go/renderer"
	"md4go/text"
)

// This file contains integration edge-case tests that exercise the root
// md4go.New() API across all three rendering modes (PlainText, HTML, Stream).
// Subpackage-specific edge cases live in their respective packages:
//   - parser/edgecase_test.go (flag combinations, deep nesting)
//   - html/edgecase_test.go    (HTML renderer edge cases)
//   - text/edgecase_test.go    (stream consistency, tight list rendering)

// TestEmptyInput verifies that empty or nil input doesn't panic across
// all rendering modes (PlainText, HTML, Stream).
func TestEmptyInput(t *testing.T) {
	tests := []struct {
		name string
		src  []byte
	}{
		{"nil", nil},
		{"empty", []byte{}},
		{"single_newline", []byte("\n")},
		{"multiple_newlines", []byte("\n\n\n\n")},
		{"spaces_only", []byte("   ")},
		{"tabs_only", []byte("\t\t")},
		{"single_char", []byte("a")},
		{"single_hash", []byte("#")},
		{"single_backtick", []byte("`")},
		{"single_asterisk", []byte("*")},
		{"single_underscore", []byte("_")},
		{"single_bracket", []byte("[")},
		{"single_angle", []byte("<")},
		{"single_ampersand", []byte("&")},
		{"null_byte", []byte{0x00}},
		{"bom_only", []byte{0xEF, 0xBB, 0xBF}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			md := md4go.New()

			// PlainText render
			var plainBuf bytes.Buffer
			func() {
				defer func() {
					if r := recover(); r != nil {
						t.Fatalf("panic in Convert: %v", r)
					}
				}()
				mdConvertPlain(md, tt.src, &plainBuf)
			}()

			// HTML render
			var htmlBuf bytes.Buffer
			func() {
				defer func() {
					if r := recover(); r != nil {
						t.Fatalf("panic in ConvertHTML: %v", r)
					}
				}()
				mdConvertHTML(md, tt.src, &htmlBuf)
			}()

			// Stream mode
			if tt.src != nil {
				func() {
					defer func() {
						if r := recover(); r != nil {
							t.Fatalf("panic in ConvertStream: %v", r)
						}
					}()
					var streamBuf bytes.Buffer
					text.ConvertStream(bytes.NewReader(tt.src), &streamBuf)
				}()
			}
		})
	}
}

// TestMassiveInput verifies that large inputs don't cause issues.
func TestMassiveInput(t *testing.T) {
	var buf bytes.Buffer
	for i := 0; i < 10000; i++ {
		buf.WriteString("word ")
	}
	buf.WriteString("\n")

	md := md4go.New()
	var out bytes.Buffer
	func() {
		defer func() {
			if r := recover(); r != nil {
				t.Fatalf("panic on large input: %v", r)
			}
		}()
		mdConvertPlain(md, buf.Bytes(), &out)
	}()

	if out.Len() == 0 {
		t.Error("expected non-empty output")
	}
}

// TestRendererInterfaceCompliance verifies that both renderers satisfy the interface.
func TestRendererInterfaceCompliance(t *testing.T) {
	var _ renderer.Renderer = (*text.PlainText)(nil)
	// HTML is also a Renderer — verify through usage
	md := md4go.New()
	var buf bytes.Buffer
	mdConvertHTML(md, []byte("test"), &buf)
}
