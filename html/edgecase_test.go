package html_test

import (
	"testing"
)

// TestHTMLRendererEdgeCases verifies HTML renderer doesn't panic on edge cases.
func TestHTMLRendererEdgeCases(t *testing.T) {
	tests := []struct {
		name string
		src  string
	}{
		{"empty", ""},
		{"null_char", "a\x00b"},
		{"entity_in_title", "![alt](src.png 'foo &quot; bar')"},
		{"nested_emphasis", "***bold italic***"},
		{"empty_link", "[]()"},
		{"incomplete_code", "`unclosed"},
		{"incomplete_link", "[text](url"},
		{"many_brackets", "[[[a]]]"},
		{"unicode", "Hello 世界 🎉"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			func() {
				defer func() {
					if r := recover(); r != nil {
						t.Fatalf("panic: %v", r)
					}
				}()
				renderHTML(t, tt.src)
			}()
		})
	}
}
