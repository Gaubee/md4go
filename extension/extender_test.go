package extension

import (
	"bytes"
	"strings"
	"testing"

	"md4go/html"
	"md4go/parser"
)

// --- Extender mechanism tests ---

func TestExtenderFlags(t *testing.T) {
	p := parser.New(0, &Strikethrough{})
	if p.Flags()&parser.FlagStrikethrough == 0 {
		t.Error("FlagStrikethrough not set after Strikethrough.Extend()")
	}
}

func TestExtenderMarkChars(t *testing.T) {
	// Default markChars should not contain '|'
	p0 := parser.New(0)
	if p0.HasMarkChar('|') {
		t.Error("'|' should not be a mark char by default")
	}

	// Table extender adds '|'
	p1 := parser.New(0, &Table{})
	if !p1.HasMarkChar('|') {
		t.Error("'|' should be a mark char after Table.Extend()")
	}
}

func TestExtenderPermissiveAutolinks(t *testing.T) {
	p := parser.New(0, &PermissiveAutolinks{})

	expectedFlags := parser.PermissiveAutolinks
	if p.Flags()&expectedFlags != expectedFlags {
		t.Errorf("flags = %b, want %b", p.Flags(), expectedFlags)
	}

	for _, c := range []byte{'@', ':', '.'} {
		if !p.HasMarkChar(c) {
			t.Errorf("%q should be a mark char after PermissiveAutolinks.Extend()", c)
		}
	}
}

func TestExtenderTaskList(t *testing.T) {
	p := parser.New(0, &TaskList{})
	if p.Flags()&parser.FlagTasklists == 0 {
		t.Error("FlagTasklists not set after TaskList.Extend()")
	}
}

func TestExtenderAdmonition(t *testing.T) {
	p := parser.New(0, &Admonition{})
	if p.Flags()&parser.FlagAdmonitions == 0 {
		t.Error("FlagAdmonitions not set after Admonition.Extend()")
	}
}

func TestExtenderFootnote(t *testing.T) {
	p := parser.New(0, &Footnote{})
	if p.Flags()&parser.FlagFootnotes == 0 {
		t.Error("FlagFootnotes not set after Footnote.Extend()")
	}
}

func TestGFMPresetFlags(t *testing.T) {
	p := parser.New(0, GFM...)

	expectedFlags := parser.DialectGitHub
	if p.Flags()&expectedFlags != expectedFlags {
		t.Errorf("GFM flags = %b, want %b", p.Flags(), expectedFlags)
	}
}

func TestGFMPresetMarkChars(t *testing.T) {
	p := parser.New(0, GFM...)

	// '|' should be added by Table extender
	if !p.HasMarkChar('|') {
		t.Error("'|' should be a mark char in GFM preset")
	}

	// '@', ':', '.' should be added by PermissiveAutolinks extender
	for _, c := range []byte{'@', ':', '.'} {
		if !p.HasMarkChar(c) {
			t.Errorf("%q should be a mark char in GFM preset", c)
		}
	}

	// '~' should be added by Strikethrough extender
	if !p.HasMarkChar('~') {
		t.Error("'~' should be a mark char (added by Strikethrough extender)")
	}
}

func TestExtenderMultipleCombined(t *testing.T) {
	// Test that multiple extensions can be combined
	p := parser.New(0,
		&Strikethrough{},
		&Table{},
		&TaskList{},
	)

	expected := parser.FlagStrikethrough | parser.FlagTables | parser.FlagTasklists
	if p.Flags()&expected != expected {
		t.Errorf("combined flags = %b, want %b", p.Flags(), expected)
	}

	if !p.HasMarkChar('|') {
		t.Error("'|' should be a mark char after combining extensions")
	}
}

func TestExtenderCustomExtender(t *testing.T) {
	// Test that a custom extender can be written by the user
	custom := &customExtender{}
	p := parser.New(0, custom)

	if p.Flags()&parser.FlagHardSoftBreaks == 0 {
		t.Error("custom extender should set FlagHardSoftBreaks")
	}
}

type customExtender struct{}

func (e *customExtender) Extend(r parser.Registrar) {
	r.SetFlags(parser.FlagHardSoftBreaks)
}

// normalizeHTML does minimal HTML normalization for test comparison.
func normalizeHTML(s string) string {
	// Remove whitespace between tags
	for strings.Contains(s, "> <") || strings.Contains(s, ">\n<") || strings.Contains(s, ">\t<") {
		s = strings.ReplaceAll(s, "> <", "><")
		s = strings.ReplaceAll(s, ">\n<", "><")
		s = strings.ReplaceAll(s, ">\t<", "><")
	}
	// Collapse remaining whitespace
	fields := strings.Fields(s)
	return strings.Join(fields, " ")
}

// --- Task List end-to-end tests ---

func TestTaskListEndToEnd(t *testing.T) {
	// Tight list support (I18): lists without blank lines between items are tight,
	// so <p> tags are suppressed inside list items. Mirrors md4c behavior.
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			"basic unchecked",
			"- [ ] foo",
			`<ul><li class="task-list-item"><input type="checkbox" class="task-list-item-checkbox" disabled="true" />foo</li></ul>`,
		},
		{
			"basic checked",
			"- [x] foo",
			`<ul><li class="task-list-item"><input type="checkbox" class="task-list-item-checkbox" disabled="true" checked="true" />foo</li></ul>`,
		},
		{
			"checked uppercase",
			"- [X] bar",
			`<ul><li class="task-list-item"><input type="checkbox" class="task-list-item-checkbox" disabled="true" checked="true" />bar</li></ul>`,
		},
		{
			"multiple items",
			"- [x] foo\n- [ ] bar\n- [x] baz",
			`<ul><li class="task-list-item"><input type="checkbox" class="task-list-item-checkbox" disabled="true" checked="true" />foo</li><li class="task-list-item"><input type="checkbox" class="task-list-item-checkbox" disabled="true" />bar</li><li class="task-list-item"><input type="checkbox" class="task-list-item-checkbox" disabled="true" checked="true" />baz</li></ul>`,
		},
		{
			"ordered list task",
			"1. [x] foo",
			`<ol><li class="task-list-item"><input type="checkbox" class="task-list-item-checkbox" disabled="true" checked="true" />foo</li></ol>`,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var buf bytes.Buffer
			h := html.NewHTML(&buf)
			p := parser.New(0, &TaskList{})
			if err := p.Parse([]byte(tc.input), h); err != nil {
				t.Fatalf("Parse error: %v", err)
			}
			_ = h.Flush()
			got := normalizeHTML(buf.String())
			want := normalizeHTML(tc.want)
			if got != want {
				t.Errorf("input: %q\nwant: %q\ngot:  %q", tc.input, tc.want, buf.String())
			}
		})
	}
}

func TestTaskListNotEnabledByDefault(t *testing.T) {
	var buf bytes.Buffer
	h := html.NewHTML(&buf)
	p := parser.New(0)
	_ = p.Parse([]byte("- [x] foo"), h)
	_ = h.Flush()
	got := buf.String()
	if strings.Contains(got, "task-list-item") {
		t.Errorf("task list should not be enabled by default, got: %s", got)
	}
}

func TestStrikethroughEndToEnd(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string // expected HTML output
	}{
		{"basic double", "~~strike~~", "<p><del>strike</del></p>"},
		{"basic single", "~Hi~ Hello", "<p><del>Hi</del> Hello</p>"},
		{"no match length", "This ~text~~ is curious.", "<p>This ~text~~ is curious.</p>"},
		{"no match too long", "foo ~~~bar~~~", "<p>foo ~~~bar~~~</p>"},
		{"no match whitespace", "~foo ~bar", "<p>~foo ~bar</p>"},
		{"nested", "~~foo ~~bar~~ baz~~", "<p><del>foo <del>bar</del> baz</del></p>"},
		{"with emphasis", "~~*foo*~~", "<p><del><em>foo</em></del></p>"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var buf bytes.Buffer
			h := html.NewHTML(&buf)
			p := parser.New(0, &Strikethrough{})
			if err := p.Parse([]byte(tc.input), h); err != nil {
				t.Fatalf("Parse error: %v", err)
			}
			_ = h.Flush()
			got := normalizeHTML(buf.String())
			want := normalizeHTML(tc.want)
			if got != want {
				t.Errorf("input: %q\nwant: %q\ngot:  %q", tc.input, tc.want, buf.String())
			}
		})
	}
}

func TestStrikethroughNotEnabledByDefault(t *testing.T) {
	// Without the Strikethrough extension, ~~ should be literal text
	var buf bytes.Buffer
	h := html.NewHTML(&buf)
	p := parser.New(0)
	if err := p.Parse([]byte("~~strike~~"), h); err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	_ = h.Flush()
	got := buf.String()
	if strings.Contains(got, "<del>") {
		t.Errorf("strikethrough should not be enabled by default, got: %s", got)
	}
}

// --- Table end-to-end tests ---

func TestTableEndToEnd(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			"basic table with pipes",
			"| foo | bar |\n| --- | --- |\n| baz | qux |",
			`<table><thead><tr><th>foo</th><th>bar</th></tr></thead><tbody><tr><td>baz</td><td>qux</td></tr></tbody></table>`,
		},
		{
			"table without leading pipe",
			"foo | bar\n--- | ---\nbaz | qux",
			`<table><thead><tr><th>foo</th><th>bar</th></tr></thead><tbody><tr><td>baz</td><td>qux</td></tr></tbody></table>`,
		},
		{
			"alignment left",
			"| h |\n| :--- |\n| a |",
			`<table><thead><tr><th align="left">h</th></tr></thead><tbody><tr><td align="left">a</td></tr></tbody></table>`,
		},
		{
			"alignment right",
			"| h |\n| ---: |\n| a |",
			`<table><thead><tr><th align="right">h</th></tr></thead><tbody><tr><td align="right">a</td></tr></tbody></table>`,
		},
		{
			"alignment center",
			"| h |\n| :---: |\n| a |",
			`<table><thead><tr><th align="center">h</th></tr></thead><tbody><tr><td align="center">a</td></tr></tbody></table>`,
		},
		{
			"multiple body rows",
			"| a | b |\n| --- | --- |\n| 1 | 2 |\n| 3 | 4 |",
			`<table><thead><tr><th>a</th><th>b</th></tr></thead><tbody><tr><td>1</td><td>2</td></tr><tr><td>3</td><td>4</td></tr></tbody></table>`,
		},
		{
			"header only no body",
			"| a | b |\n| --- | --- |",
			`<table><thead><tr><th>a</th><th>b</th></tr></thead></table>`,
		},
		{
			"inline formatting in cells",
			"| *a* | **b** |\n| --- | --- |\n| `code` | ~~del~~ |",
			`<table><thead><tr><th><em>a</em></th><th><strong>b</strong></th></tr></thead><tbody><tr><td><code>code</code></td><td><del>del</del></td></tr></tbody></table>`,
		},
		{
			"escaped pipe in cell",
			`| a \| b | c |
| --- | --- |
| 1 | 2 |`,
			`<table><thead><tr><th>a | b</th><th>c</th></tr></thead><tbody><tr><td>1</td><td>2</td></tr></tbody></table>`,
		},
		{
			"fewer cells than columns",
			"| a | b | c |\n| --- | --- | --- |\n| 1 | 2 |",
			`<table><thead><tr><th>a</th><th>b</th><th>c</th></tr></thead><tbody><tr><td>1</td><td>2</td><td></td></tr></tbody></table>`,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var buf bytes.Buffer
			h := html.NewHTML(&buf)
			p := parser.New(0, &Table{}, &Strikethrough{})
			if err := p.Parse([]byte(tc.input), h); err != nil {
				t.Fatalf("Parse error: %v", err)
			}
			_ = h.Flush()
			got := normalizeHTML(buf.String())
			want := normalizeHTML(tc.want)
			if got != want {
				t.Errorf("input: %q\nwant: %q\ngot:  %q", tc.input, tc.want, buf.String())
			}
		})
	}
}

func TestTableNotEnabledByDefault(t *testing.T) {
	var buf bytes.Buffer
	h := html.NewHTML(&buf)
	p := parser.New(0)
	_ = p.Parse([]byte("| foo | bar |\n| --- | --- |\n| baz | qux |"), h)
	_ = h.Flush()
	got := buf.String()
	if strings.Contains(got, "<table>") {
		t.Errorf("table should not be enabled by default, got: %s", got)
	}
}
