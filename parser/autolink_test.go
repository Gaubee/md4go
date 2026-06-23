package parser

import "testing"

func TestIsAutolinkBasic(t *testing.T) {
	tests := []struct {
		input string
		want  bool
	}{
		{"<http://foo.bar.baz>", true},
		{"<https://foo.bar.baz/test>", true},
		{"<irc://foo.bar:2233/baz>", true},
		{"<MAILTO:FOO@BAR.BAZ>", true},
		{"<user@domain.com>", true},
		{"<http://foo.bar/baz bim>", false}, // space in URL
		{"<foo bar>", false},                // not a valid URL
		// DEL char (0x7F) is a control character per md4c ISCNTRL (md4c.c:349).
		// Autolinks with DEL in the path must be rejected.
		{"<http://foo\x7Fbar>", false},
	}
	for _, tt := range tests {
		_, ok := isAutolink([]byte(tt.input), 0)
		if ok != tt.want {
			t.Errorf("isAutolink(%q) = %v, want %v", tt.input, ok, tt.want)
		}
	}
}

func TestIsInlineHTMLTagStrict(t *testing.T) {
	tests := []struct {
		input string
		want  bool
	}{
		{"<b>", true},
		{"<br/>", true},
		{"<br />", true},
		{"<a href=\"foo\">", true},
		{"</b>", true},
		{"<http://foo.bar.baz>", false}, // should NOT match as HTML tag
		{"<foo bar=baz>", true},
		{"<foo@bar.example.com>", false}, // email, not HTML tag
		{"<a+b+c:d>", false},             // not a valid HTML tag
		{"<a h*#ref=\"hi\">", false},     // invalid attribute name
	}
	for _, tt := range tests {
		_, ok := isInlineHTMLTag([]byte(tt.input), 0)
		if ok != tt.want {
			t.Errorf("isInlineHTMLTag(%q) = %v, want %v", tt.input, ok, tt.want)
		}
	}
}

func TestIsHTMLBlockTagType7(t *testing.T) {
	tests := []struct {
		input string
		want  bool
	}{
		// Valid type 7 HTML block starts (tag followed only by whitespace or EOL)
		{"<ins>", true},
		{"</ins>", true},
		{"<a href=\"foo\">", true}, // tag with attributes, then EOL → type 7
		{"<a>", true},              // simple tag, then EOL → type 7
		{"<a><bab><c2c>", false},   // NOT type 7: after <a>, next char is '<' not whitespace/EOL
		{"<br/>", true},            // self-closing is OK
		{"<br />", true},           // self-closing with space
		{"<foo@bar.com>", false},   // not a valid HTML tag
		{"<a+b+c:d>", false},       // not a valid HTML tag
	}
	for _, tt := range tests {
		_, ok := isHTMLBlockTag([]byte(tt.input), 0)
		if ok != tt.want {
			t.Errorf("isHTMLBlockTag(%q) = %v, want %v", tt.input, ok, tt.want)
		}
	}
}

func TestDetectHTMLBlockStartType7(t *testing.T) {
	tests := []struct {
		input string
		want  uint8
		ok    bool
	}{
		{"<ins>", htmlBlockType7, true},
		{"</ins>", htmlBlockType7, true},
		{"<a><bab><c2c>", 0, false},                // not type 7 (multiple tags)
		{"<http://foo.bar.baz>", 0, false},         // not type 7 (autolink, not HTML tag)
		{"<foo@bar.example.com>", 0, false},        // not type 7 (not a valid tag)
		{"<!-- comment -->", htmlBlockType2, true}, // type 2, not 7
		{"<div>", htmlBlockType6, true},            // type 6 (block-level element)
	}
	for _, tt := range tests {
		ht, ok := detectHTMLBlockStart([]byte(tt.input))
		if ok != tt.ok || ht != tt.want {
			t.Errorf("detectHTMLBlockStart(%q) = (%d, %v), want (%d, %v)",
				tt.input, ht, ok, tt.want, tt.ok)
		}
	}
}

// --- HTML comment edge case tests ---
// Mirrors CommonMark spec example 626: <!--> and <!---> are valid
// HTML comments because md4c scans for --> starting after "<!" (not "<!--").
// I36: Updated to use isInlineHTML (which now includes horizon tracking).

func TestIsCtrl(t *testing.T) {
	tests := []struct {
		c    byte
		want bool
	}{
		{0x00, true},  // NUL
		{0x01, true},  // SOH
		{0x1F, true},  // US (last C0 control)
		{0x20, false}, // space
		{0x7E, false}, // ~
		{0x7F, true},  // DEL
		{0x80, false}, // high byte
		{'A', false},  // letter
	}
	for _, tt := range tests {
		if got := isCtrl(tt.c); got != tt.want {
			t.Errorf("isCtrl(0x%02X) = %v, want %v", tt.c, got, tt.want)
		}
	}
}

func TestInlineHTMLCommentEdgeCases(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  bool // whether isInlineHTML matches
		end   int  // expected end position if matched
	}{
		// <!--> is a complete comment: scan from off+2 finds --> immediately
		{"minimal comment <!-->", "<!-->", true, 5},
		// <!---> is a complete comment: scan from off+2 finds --> at pos 3-5
		{"minimal comment <!---->", "<!--->", true, 6},
		// <!-- foo --> is a normal comment
		{"normal comment", "<!-- foo -->", true, 12},
		// <!-- without --> is NOT a complete comment
		{"unclosed comment", "<!-- foo", false, 0},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ms := &markStacks{}
			ms.reset()
			end, ok := isInlineHTML(ms, []byte(tc.input), 0)
			if ok != tc.want {
				t.Errorf("isInlineHTML(%q) = (%d, %v), want (%d, %v)",
					tc.input, end, ok, tc.end, tc.want)
			}
			if ok && end != tc.end {
				t.Errorf("isInlineHTML(%q) end = %d, want %d",
					tc.input, end, tc.end)
			}
		})
	}
}
