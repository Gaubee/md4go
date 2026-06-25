package parser_test

import (
	"testing"
)

// --- resolveEmphSpanType tests (now inlined in processInlines) ---
// Verifies emphasis span nesting order through the full parse→HTML pipeline.
// Opener: odd length → <em> first, then <strong> pairs.
// Closer: <strong> pairs first, then <em> if odd.

func TestEmphSpanNestingOrder(t *testing.T) {
	tests := []struct {
		md   string
		want string
	}{
		{"*foo*\n", "<p><em>foo</em></p>"},
		{"**foo**\n", "<p><strong>foo</strong></p>"},
		{"***foo***\n", "<p><em><strong>foo</strong></em></p>"},
		{"****foo****\n", "<p><strong><strong>foo</strong></strong></p>"},
		{"*****foo*****\n", "<p><em><strong><strong>foo</strong></strong></em></p>"},
	}
	for _, tt := range tests {
		got := renderHTML(tt.md, 0)
		if got != tt.want {
			t.Errorf("emphasis nesting:\ninput: %q\nwant:  %q\ngot:   %q", tt.md, tt.want, got)
		}
	}
}

// TestEmphSpanNestingOrderLongMarks verifies markLen > 7 (extreme emphasis).
// Go compiler may escape very long mark runs to heap, but the inlined logic
// must still produce correct nesting.
func TestEmphSpanNestingOrderLongMarks(t *testing.T) {
	// 8 asterisks = 4 strong pairs
	got := renderHTML("********foo********\n", 0)
	want := "<p><strong><strong><strong><strong>foo</strong></strong></strong></strong></p>"
	if got != want {
		t.Errorf("8-asterisk emphasis:\nwant:  %q\ngot:   %q", want, got)
	}
}
