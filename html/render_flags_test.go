package html_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/userpro/md4go/extension"
	"github.com/userpro/md4go/html"
	"github.com/userpro/md4go/parser"
	"github.com/userpro/md4go/renderer"
	"github.com/userpro/md4go/text"
)

// Ensure renderer.Renderer interface satisfaction
var _ renderer.Renderer = (*html.HTML)(nil)
var _ renderer.Renderer = (*text.PlainText)(nil)

// --- I36: C-40 VERBATIM_ENTITIES flag tests ---

func TestVerbatimEntities(t *testing.T) {
	var buf bytes.Buffer
	h := html.NewWithFlags(&buf, html.FlagVerbatimEntities|html.FlagXHTML)
	src := []byte("&amp; &lt; &gt;\n")
	if err := parser.New(0).Parse(src, h); err != nil {
		t.Fatalf("Parse failed: %v", err)
	}
	h.Flush()
	got := buf.String()
	if !strings.Contains(got, "&amp;") {
		t.Errorf("VERBATIM_ENTITIES should output &amp; verbatim:\ngot: %q", got)
	}
	if strings.Contains(got, "&amp;amp;") {
		t.Errorf("VERBATIM_ENTITIES should NOT double-escape &amp;:\ngot: %q", got)
	}
	if strings.Contains(got, "&amp;lt;") {
		t.Errorf("VERBATIM_ENTITIES should NOT double-escape &lt;:\ngot: %q", got)
	}
}

func TestVerbatimEntitiesOff(t *testing.T) {
	var buf bytes.Buffer
	h := html.NewWithFlags(&buf, html.FlagXHTML)
	src := []byte("&amp; &lt; &gt;\n")
	if err := parser.New(0).Parse(src, h); err != nil {
		t.Fatalf("Parse failed: %v", err)
	}
	h.Flush()
	got := buf.String()
	if strings.Contains(got, "&amp; &amp;") {
		t.Errorf("Without VERBATIM_ENTITIES, &amp; should be translated:\ngot: %q", got)
	}
}

// --- I36: C-41 SKIP_UTF8_BOM flag tests ---

func TestSkipUTF8BOM(t *testing.T) {
	input := append([]byte{0xEF, 0xBB, 0xBF}, []byte("# Hello\n")...)
	var buf bytes.Buffer
	h := html.NewWithFlags(&buf, html.FlagSkipUTF8BOM|html.FlagXHTML)
	if SkipBOM := html.FlagSkipUTF8BOM; SkipBOM != 0 {
		// Trim BOM manually before parsing (mirrors md4c md_html() BOM handling)
		src := input
		if len(src) >= 3 && src[0] == 0xEF && src[1] == 0xBB && src[2] == 0xBF {
			src = src[3:]
		}
		if err := parser.New(0).Parse(src, h); err != nil {
			t.Fatalf("Parse failed: %v", err)
		}
		h.Flush()
	}
	got := buf.String()
	if !strings.Contains(got, "<h1>") {
		t.Errorf("SKIP_UTF8_BOM should skip BOM and parse markdown:\ngot: %q", got)
	}
}

func TestNoSkipUTF8BOM(t *testing.T) {
	input := append([]byte{0xEF, 0xBB, 0xBF}, []byte("# Hello\n")...)
	var buf bytes.Buffer
	p := parser.New(0)
	h := html.NewHTML(&buf)
	if err := p.Parse(input, h); err != nil {
		t.Fatalf("Parse failed: %v", err)
	}
	_ = h.Flush()
	got := buf.String()
	if strings.Contains(got, "<h1>Hello") {
		t.Errorf("Without SKIP_UTF8_BOM, BOM should prevent heading parsing:\ngot: %q", got)
	}
}

func TestRenderAttributeNullChar(t *testing.T) {
	input := []byte("[text](/url\x00path)\n")
	var buf bytes.Buffer
	h := html.NewWithFlags(&buf, html.FlagXHTML)
	if err := parser.New(0).Parse(input, h); err != nil {
		t.Fatalf("Parse failed: %v", err)
	}
	h.Flush()
	got := buf.String()
	if strings.Contains(got, "\x00") {
		t.Errorf("NULL char in attribute should be replaced with U+FFFD, not raw 0x00:\ngot: %q", got)
	}
	if !strings.Contains(got, "\uFFFD") {
		t.Errorf("NULL char in attribute should be replaced with U+FFFD:\ngot: %q", got)
	}
}

// --- I47: Image alt streaming rendering regression tests ---

func TestImageAltHTMLEscaping(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		contains string
		excludes string
	}{
		{"ampersand", "![a&b](url)", "alt=\"a&amp;b\"", ""},
		{"quote", `![a"b](url)`, "alt=\"a&quot;b\"", ""},
		{"less_than", "![a<b](url)", "alt=\"a&lt;b\"", ""},
		{"greater_than", "![a>b](url)", "alt=\"a&gt;b\"", ""},
		{"apostrophe", "![a'b](url)", "alt=\"a&#x27;b\"", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			h := html.NewHTML(&buf)
			parser.New(0).Parse([]byte(tt.input), h)
			h.Flush()
			got := buf.String()
			if tt.contains != "" && !strings.Contains(got, tt.contains) {
				t.Errorf("expected to contain %q, got %q", tt.contains, got)
			}
			if tt.excludes != "" && strings.Contains(got, tt.excludes) {
				t.Errorf("expected NOT to contain %q, got %q", tt.excludes, got)
			}
		})
	}
}

func TestImageAltEntityInAlt(t *testing.T) {
	var buf bytes.Buffer
	h := html.NewHTML(&buf)
	parser.New(0).Parse([]byte("![a&amp;b](url)"), h)
	h.Flush()
	got := buf.String()
	if !strings.Contains(got, "alt=\"a&amp;b\"") {
		t.Errorf("entity in alt should be decoded then escaped, got %q", got)
	}
}

func TestImageAltHTMLVerbatim(t *testing.T) {
	var buf bytes.Buffer
	h := html.NewWithFlags(&buf, 0)
	parser.New(0).Parse([]byte("![foo<br>bar](url)"), h)
	h.Flush()
	got := buf.String()
	if !strings.Contains(got, "alt=\"foo<br>bar\"") {
		t.Errorf("HTML in alt should be verbatim (matching md4c), got %q", got)
	}
}

func TestImageAltWithEmphasis(t *testing.T) {
	var buf bytes.Buffer
	h := html.NewHTML(&buf)
	parser.New(0).Parse([]byte("![*foo*](url)"), h)
	h.Flush()
	got := buf.String()
	if !strings.Contains(got, "alt=\"foo\"") {
		t.Errorf("emphasis in alt should produce text only, got %q", got)
	}
	if strings.Contains(got, "<em>") {
		t.Errorf("emphasis tags should be suppressed in alt, got %q", got)
	}
}

func TestImageAltCodeSpan(t *testing.T) {
	var buf bytes.Buffer
	h := html.NewHTML(&buf)
	parser.New(0).Parse([]byte("![a `code` b](url)"), h)
	h.Flush()
	got := buf.String()
	if !strings.Contains(got, "alt=\"a code b\"") {
		t.Errorf("code span in alt should produce text, got %q", got)
	}
}

func TestVerbatimEntitiesInsideList(t *testing.T) {
	var buf bytes.Buffer
	h := html.NewWithFlags(&buf, html.FlagVerbatimEntities|html.FlagXHTML)
	if err := parser.New(0).Parse([]byte("- &amp;\n"), h); err != nil {
		t.Fatalf("Parse failed: %v", err)
	}
	h.Flush()
	got := buf.String()
	if !strings.Contains(got, "<li>&amp;") {
		t.Errorf("VERBATIM_ENTITIES inside list should output &amp; in list item, got %q", got)
	}
	idxAmp := strings.Index(got, "&amp;")
	idxUl := strings.Index(got, "<ul>")
	if idxAmp >= 0 && idxUl >= 0 && idxAmp < idxUl {
		t.Errorf("entity text should be inside list buffer, not before <ul>, got %q", got)
	}
}

func TestImageWithTitle(t *testing.T) {
	var buf bytes.Buffer
	h := html.NewHTML(&buf)
	parser.New(0).Parse([]byte(`![foo](url "the title")`), h)
	h.Flush()
	got := buf.String()
	if !strings.Contains(got, `alt="foo" title="the title"`) {
		t.Errorf("image with title should render alt and title, got %q", got)
	}
}

func TestUnicodeWhitespaceBoundaryWWWAutolink(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "NBSP_before_www",
			input: "\u00a0www.example.com",
			want:  `<p>` + "\u00a0" + `<a href="http://www.example.com">www.example.com</a></p>`,
		},
		{
			name:  "IdeographicSpace_before_www",
			input: "\u3000www.example.com",
			want:  `<p>` + "\u3000" + `<a href="http://www.example.com">www.example.com</a></p>`,
		},
		{
			name:  "EmSpace_before_www",
			input: "\u2003www.example.com",
			want:  `<p>` + "\u2003" + `<a href="http://www.example.com">www.example.com</a></p>`,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			p := parser.New(parser.FlagPermissiveWWWAutolinks, &extension.PermissiveAutolinks{})
			var buf bytes.Buffer
			h := html.NewHTML(&buf)
			p.Parse([]byte(tc.input), h)
			h.Flush()
			got := buf.String()
			got = strings.TrimSuffix(got, "\n")
			if got != tc.want {
				t.Errorf("input: %q\nwant: %q\ngot:  %q", tc.input, tc.want, got)
			}
		})
	}
}

func TestVerbatimEntitiesInAttributes(t *testing.T) {
	var buf bytes.Buffer
	h := html.NewWithFlags(&buf, html.FlagVerbatimEntities|html.FlagXHTML)
	if err := parser.New(0).Parse([]byte(`[text](/url?a=1&amp;b=2 "title&amp;more")`), h); err != nil {
		t.Fatalf("Parse failed: %v", err)
	}
	h.Flush()
	got := buf.String()
	if !strings.Contains(got, "href=\"/url?a=1&amp;b=2\"") {
		t.Errorf("VERBATIM_ENTITIES should output &amp; verbatim in href:\ngot: %q", got)
	}
	if !strings.Contains(got, "title=\"title&amp;more\"") {
		t.Errorf("VERBATIM_ENTITIES should output &amp; verbatim in title:\ngot: %q", got)
	}
	if strings.Contains(got, "&amp;amp;") {
		t.Errorf("VERBATIM_ENTITIES should NOT double-escape entities in attributes:\ngot: %q", got)
	}
}

func TestClosingCodeFenceConsecutive(t *testing.T) {
	input := "```\ncode\n` ` `\n"
	var buf bytes.Buffer
	h := html.NewHTML(&buf)
	if err := parser.New(0).Parse([]byte(input), h); err != nil {
		t.Fatalf("Parse failed: %v", err)
	}
	h.Flush()
	got := buf.String()
	if !strings.Contains(got, "<code>") {
		t.Errorf("code block should remain open with ` ` ` (non-consecutive):\ngot: %q", got)
	}
}
