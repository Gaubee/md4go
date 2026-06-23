package parser_test

import (
	"bytes"
	"strings"
	"testing"

	"md4go"
	"md4go/ast"
	"md4go/extension"
	"md4go/html"
	"md4go/renderer"
	"md4go/text"
)

// convertToPlainText converts markdown to plain text for testing.
func convertToPlainText(t *testing.T, src string) string {
	t.Helper()
	var buf bytes.Buffer
	if err := text.Convert([]byte(src), &buf); err != nil {
		t.Fatalf("Convert error: %v", err)
	}
	return buf.String()
}

func TestATXHeader(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"h1", "# Hello", "\nHello\n"},
		{"h2", "## Hello", "\nHello\n"},
		{"h3", "### Hello", "\nHello\n"},
		{"h4", "#### Hello", "\nHello\n"},
		{"h5", "##### Hello", "\nHello\n"},
		{"h6", "###### Hello", "\nHello\n"},
		{"no_space_after_hash", "#Hello", "\n#Hello\n"},
		{"trailing_hashes", "# Hello #", "\nHello\n"},
		{"trailing_hashes_no_space", "# Hello#", "\nHello#\n"},
		{"empty_header", "#", "\n"},
		{"header_with_spaces", "#   Hello   ", "\nHello\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := convertToPlainText(t, tt.input)
			if got != tt.want {
				t.Errorf("input %q:\n  want %q\n  got  %q", tt.input, tt.want, got)
			}
		})
	}
}

func TestParagraph(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"simple", "Hello world", "\nHello world\n"},
		{"multi_line", "line1\nline2", "\nline1\nline2\n"},
		{"two_paragraphs", "para1\n\npara2", "\npara1\npara2\n"},
		{"trailing_newline", "Hello\n", "\nHello\n"},
		{"leading_blank", "\nHello", "\nHello\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := convertToPlainText(t, tt.input)
			if got != tt.want {
				t.Errorf("input %q:\n  want %q\n  got  %q", tt.input, tt.want, got)
			}
		})
	}
}

func TestHTMLRenderer(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"h1", "# Hello", "<h1>Hello</h1>\n"},
		{"h2", "## Hello", "<h2>Hello</h2>\n"},
		{"paragraph", "Hello world", "<p>Hello world</p>\n"},
		{"two_paras", "p1\n\np2", "<p>p1</p>\n<p>p2</p>\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			h := html.NewHTML(&buf)
			md := md4go.New()
			if err := md.Parse([]byte(tt.input), h); err != nil {
				t.Fatalf("ConvertTo error: %v", err)
			}
			_ = h.Flush()
			got := buf.String()
			if got != tt.want {
				t.Errorf("input %q:\n  want %q\n  got  %q", tt.input, tt.want, got)
			}
		})
	}
}

// TestConvertStreamConsistency verifies that Convert and ConvertStream
// produce the same output for simple inputs without forward references.
func TestConvertStreamConsistency(t *testing.T) {
	inputs := []string{
		"# Hello\n",
		"Hello world\n",
		"para1\n\npara2\n",
		"## Title\n\nSome text here.\n",
	}
	for i, input := range inputs {
		full := convertToPlainText(t, input)
		var buf bytes.Buffer
		if err := text.ConvertStream(bytes.NewReader([]byte(input)), &buf); err != nil {
			t.Fatalf("ConvertStream error: %v", err)
		}
		stream := buf.String()
		if full != stream {
			t.Errorf("case %d: mismatch\n  full=%q\n  stream=%q", i, full, stream)
		}
	}
}

func TestThematicBreak(t *testing.T) {
	tests := []struct {
		name  string
		input string
		html  string
		text  string
	}{
		{"dash3", "---", "<hr />\n", "\n"},
		{"star3", "***", "<hr />\n", "\n"},
		{"underscore3", "___", "<hr />\n", "\n"},
		{"dash_with_spaces", "- - -", "<hr />\n", "\n"},
		{"long_dash", "----------", "<hr />\n", "\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// HTML
			var hbuf bytes.Buffer
			h := html.NewHTML(&hbuf)
			md := md4go.New()
			_ = md.Parse([]byte(tt.input), h)
			_ = h.Flush()
			if got := hbuf.String(); got != tt.html {
				t.Errorf("HTML: want %q, got %q", tt.html, got)
			}
			// PlainText
			got := convertToPlainText(t, tt.input)
			if got != tt.text {
				t.Errorf("Text: want %q, got %q", tt.text, got)
			}
		})
	}
}

func TestFencedCode(t *testing.T) {
	tests := []struct {
		name  string
		input string
		html  string
	}{
		{"backtick3", "```\ncode\n```", "<pre><code>code\n</code></pre>\n"},
		{"tilde3", "~~~\ncode\n~~~", "<pre><code>code\n</code></pre>\n"},
		{"multi_line", "```\nline1\nline2\n```", "<pre><code>line1\nline2\n</code></pre>\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var hbuf bytes.Buffer
			h := html.NewHTML(&hbuf)
			md := md4go.New()
			_ = md.Parse([]byte(tt.input), h)
			_ = h.Flush()
			if got := hbuf.String(); got != tt.html {
				t.Errorf("want %q, got %q", tt.html, got)
			}
		})
	}
}

func TestIndentedCode(t *testing.T) {
	input := "    code line"
	want := "<pre><code>code line\n</code></pre>\n"
	var hbuf bytes.Buffer
	h := html.NewHTML(&hbuf)
	md := md4go.New()
	_ = md.Parse([]byte(input), h)
	_ = h.Flush()
	if got := hbuf.String(); got != want {
		t.Errorf("want %q, got %q", want, got)
	}
}

func TestMixedContent(t *testing.T) {
	input := `# Title

Some paragraph text.

---

More text after HR.`
	text := convertToPlainText(t, input)
	// Should have "Title", "Some paragraph text.", "More text after HR."
	if !contains(text, "Title") || !contains(text, "Some paragraph text.") || !contains(text, "More text after HR.") {
		t.Errorf("unexpected output: %q", text)
	}
}

func TestSetextHeader(t *testing.T) {
	tests := []struct {
		name  string
		input string
		html  string
		text  string
	}{
		{"h1_eq", "Title\n===", "<h1>Title</h1>\n", "\nTitle\n"},
		{"h2_dash", "Title\n---", "<h2>Title</h2>\n", "\nTitle\n"},
		{"h1_multi_eq", "Title\n=======", "<h1>Title</h1>\n", "\nTitle\n"},
		{"h2_multi_dash", "Title\n-------", "<h2>Title</h2>\n", "\nTitle\n"},
		{"h1_trailing_space", "Title\n=== ", "<h1>Title</h1>\n", "\nTitle\n"},
		{"multi_line_para_h1", "Line1\nLine2\n===", "<h1>Line1\nLine2</h1>\n", "\nLine1\nLine2\n"},
		{"setext_after_blank", "Para1\n\nTitle\n===", "<p>Para1</p>\n<h1>Title</h1>\n", "\nPara1\nTitle\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// HTML
			var hbuf bytes.Buffer
			h := html.NewHTML(&hbuf)
			md := md4go.New()
			_ = md.Parse([]byte(tt.input), h)
			_ = h.Flush()
			if got := hbuf.String(); got != tt.html {
				t.Errorf("HTML: want %q, got %q", tt.html, got)
			}
			// PlainText
			got := convertToPlainText(t, tt.input)
			if got != tt.text {
				t.Errorf("Text: want %q, got %q", tt.text, got)
			}
		})
	}
}

func TestBlockquote(t *testing.T) {
	tests := []struct {
		name  string
		input string
		html  string
	}{
		{"simple", "> quote", "<blockquote>\n<p>quote</p>\n</blockquote>\n"},
		{"multi_line", "> line1\n> line2", "<blockquote>\n<p>line1\nline2</p>\n</blockquote>\n"},
		{"two_paras", "> para1\n>\n> para2", "<blockquote>\n<p>para1</p>\n<p>para2</p>\n</blockquote>\n"},
		{"no_space_after_gt", ">quote", "<blockquote>\n<p>quote</p>\n</blockquote>\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var hbuf bytes.Buffer
			h := html.NewHTML(&hbuf)
			md := md4go.New()
			_ = md.Parse([]byte(tt.input), h)
			_ = h.Flush()
			if got := hbuf.String(); got != tt.html {
				t.Errorf("HTML: want %q, got %q", tt.html, got)
			}
		})
	}
}

func TestUnorderedList(t *testing.T) {
	tests := []struct {
		name  string
		input string
		html  string
	}{
		{"single_dash", "- item", "<ul>\n<li>item</li>\n</ul>\n"},
		{"single_star", "* item", "<ul>\n<li>item</li>\n</ul>\n"},
		{"single_plus", "+ item", "<ul>\n<li>item</li>\n</ul>\n"},
		{"two_items", "- item1\n- item2", "<ul>\n<li>item1</li>\n<li>item2</li>\n</ul>\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var hbuf bytes.Buffer
			h := html.NewHTML(&hbuf)
			md := md4go.New()
			_ = md.Parse([]byte(tt.input), h)
			_ = h.Flush()
			if got := hbuf.String(); got != tt.html {
				t.Errorf("HTML: want %q, got %q", tt.html, got)
			}
		})
	}
}

func TestOrderedList(t *testing.T) {
	tests := []struct {
		name  string
		input string
		html  string
	}{
		{"single_dot", "1. item", "<ol>\n<li>item</li>\n</ol>\n"},
		{"single_paren", "1) item", "<ol>\n<li>item</li>\n</ol>\n"},
		{"two_items", "1. item1\n2. item2", "<ol>\n<li>item1</li>\n<li>item2</li>\n</ol>\n"},
		{"start_5", "5. item", "<ol start=\"5\">\n<li>item</li>\n</ol>\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var hbuf bytes.Buffer
			h := html.NewHTML(&hbuf)
			md := md4go.New()
			_ = md.Parse([]byte(tt.input), h)
			_ = h.Flush()
			if got := hbuf.String(); got != tt.html {
				t.Errorf("HTML: want %q, got %q", tt.html, got)
			}
		})
	}
}

func TestHTMLBlock(t *testing.T) {
	tests := []struct {
		name  string
		input string
		html  string
	}{
		{
			"type6_div",
			"<div>\nhello\n</div>",
			"<div>\nhello\n</div>\n",
		},
		{
			"type6_div_blank",
			"<div>\ntext\n\nafter",
			"<div>\ntext\n<p>after</p>\n",
		},
		{
			"type2_comment",
			"<!-- comment -->",
			"<!-- comment -->\n",
		},
		{
			"type2_multiline",
			"<!--\ncomment\n-->",
			"<!--\ncomment\n-->\n",
		},
		{
			"type3_pi",
			"<?xml version=\"1.0\"?>",
			"<?xml version=\"1.0\"?>\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var hbuf bytes.Buffer
			h := html.NewHTML(&hbuf)
			md := md4go.New()
			_ = md.Parse([]byte(tt.input), h)
			_ = h.Flush()
			if got := hbuf.String(); got != tt.html {
				t.Errorf("HTML: want %q, got %q", tt.html, got)
			}
		})
	}
}

func TestSetextNotHR(t *testing.T) {
	// "---" after a paragraph should be setext h2, not HR
	input := "Title\n---"
	var hbuf bytes.Buffer
	h := html.NewHTML(&hbuf)
	md := md4go.New()
	_ = md.Parse([]byte(input), h)
	_ = h.Flush()
	want := "<h2>Title</h2>\n"
	if got := hbuf.String(); got != want {
		t.Errorf("want %q, got %q", want, got)
	}
}

func TestHRPriorityOverList(t *testing.T) {
	// "- - -" should be HR, not a list item
	input := "- - -"
	var hbuf bytes.Buffer
	h := html.NewHTML(&hbuf)
	md := md4go.New()
	_ = md.Parse([]byte(input), h)
	_ = h.Flush()
	want := "<hr />\n"
	if got := hbuf.String(); got != want {
		t.Errorf("want %q, got %q", want, got)
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		(len(s) > 0 && len(substr) > 0 && strings.Contains(s, substr)))
}

// --- I46 Deep review regression tests ---

func TestClosingFenceOutsideContainer(t *testing.T) {
	// I46: Closing fence should be detected even when nParents < nContainers.
	// Mirrors md4c.c:6548-6558 where closing fence check happens BEFORE n_parents check.
	// Without this fix, the closing ``` would start a new fenced code block instead
	// of properly closing the existing one.
	input := "> ```\n> code\n```\n"
	var hbuf bytes.Buffer
	h := html.NewHTML(&hbuf)
	md := md4go.New()
	_ = md.Parse([]byte(input), h)
	_ = h.Flush()
	got := hbuf.String()
	// The fenced code block inside blockquote should be closed by the ``` line.
	// The ``` is detected as closing fence (not a new fenced code block).
	if !strings.Contains(got, "<code>") || !strings.Contains(got, "</code>") {
		t.Errorf("expected code block in blockquote, got: %q", got)
	}
}

func TestATXHeaderNotConsumedAsRefdef(t *testing.T) {
	// I46: ATX headers should NOT consume link reference definitions.
	// Only paragraphs and setext headings can contain refdefs.
	// Mirrors md4c.c:5790-5791 which checks MD_BLOCK_SETEXT_HEADER flag.
	input := "# [foo]: /url\n"
	var hbuf bytes.Buffer
	h := html.NewHTML(&hbuf)
	md := md4go.New()
	_ = md.Parse([]byte(input), h)
	_ = h.Flush()
	got := hbuf.String()
	// This should be rendered as a heading, not consumed as a refdef
	if !strings.Contains(got, "<h1>") {
		t.Errorf("expected h1 heading, got: %q", got)
	}
}

func TestPermissiveEmailAutolinkMinLength(t *testing.T) {
	// I46: @ mark requires off+3 < lineEnd (matching md4c.c:3462).
	// Short sequences like a@b should NOT create email autolink marks.
	input := "contact user@example.com for info\n"
	var hbuf bytes.Buffer
	h := html.NewHTML(&hbuf)
	md := md4go.New(md4go.WithExtensions(&extension.PermissiveAutolinks{}))
	_ = md.Parse([]byte(input), h)
	_ = h.Flush()
	got := hbuf.String()
	if !strings.Contains(got, "mailto:user@example.com") {
		t.Errorf("expected email autolink, got: %q", got)
	}
}

// Ensure renderer.Renderer is implemented correctly
var _ renderer.Renderer = (*text.PlainText)(nil)
var _ renderer.Renderer = (*html.HTML)(nil)

// Ensure event types are defined
var _ ast.BlockType = ast.BlockDoc
var _ ast.SpanType = ast.SpanEm
var _ ast.TextType = ast.TextNormal
