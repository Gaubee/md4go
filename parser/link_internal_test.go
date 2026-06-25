package parser

import (
	"github.com/userpro/md4go/ast"
	"testing"
)

// --- Tests that use unexported parser internals ---
// These must be in package parser to access markStacks, collectMarks, etc.

func TestShortcutReferenceLink(t *testing.T) {
	// Pre-populate the refDefs to test link resolution
	input := "[foo]"
	refDefs := map[string]*RefDef{
		"foo": {href: []byte("http://example.com"), title: nil},
	}

	var ms markStacks
	ms.reset()
	ms.refDefs = refDefs
	ms.unresolvedLinkHead = -1
	ms.unresolvedLinkTail = -1
	collectMarks(&ms, []byte(input), &markCharMap, 0)
	ms.analyzeMarks([]byte(input))
	ms.resolveBrackets([]byte(input), 0)

	// Check that the bracket was resolved as a link
	found := false
	for i, m := range ms.marks {
		if m.Ch == '[' && m.Flags&markResolved != 0 && m.Flags&markOpener != 0 {
			found = true
			href, _ := ms.getLinkAttrs(i)
			if string(href) != "http://example.com" {
				t.Errorf("expected href http://example.com, got %q", href)
			}
		}
	}
	if !found {
		t.Error("expected bracket to be resolved as link")
	}
}

func TestAnalyzeBracket(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		wantOpen int  // expected number of resolved opener brackets
		wantPair bool // expect at least one resolved pair
	}{
		{"simple link", "[foo](url)", 1, true},
		{"nested brackets", "[[foo]](url)", 1, true},
		{"unmatched opener", "[foo", 0, false},
		{"image", "![alt](src)", 1, true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var ms markStacks
			ms.reset()
			ms.refDefs = make(map[string]*RefDef)
			ms.unresolvedLinkHead = -1
			ms.unresolvedLinkTail = -1
			collectMarks(&ms, []byte(tc.input), &markCharMap, 0)
			ms.analyzeMarks([]byte(tc.input))
			ms.resolveBrackets([]byte(tc.input), 0)

			resolvedCount := 0
			for _, m := range ms.marks {
				if m.Flags&markResolved != 0 && (m.Ch == '[' || m.Ch == '!') && m.Flags&markOpener != 0 {
					resolvedCount++
				}
			}
			if tc.wantPair && resolvedCount < tc.wantOpen {
				t.Errorf("expected %d resolved opener(s), got %d", tc.wantOpen, resolvedCount)
			}
			if !tc.wantPair && resolvedCount > 0 {
				t.Errorf("expected no resolved pairs, got %d", resolvedCount)
			}
		})
	}
}

func TestParseLinkDestination(t *testing.T) {
	tests := []struct {
		input  string
		off    int
		wantOk bool
		want   string // expected destination text
	}{
		{"<http://foo.com>", 0, true, "http://foo.com"},
		{"/path/to/file", 0, true, "/path/to/file"},
		{"http://example.com", 0, true, "http://example.com"},
		{"", 0, false, ""},
		{"()", 0, true, "()"},
	}

	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			var ms markStacks
			ms.reset()
			beg, end, _, ok := parseLinkDestination([]byte(tc.input), tc.off)
			if ok != tc.wantOk {
				t.Errorf("parseLinkDestination(%q, %d): ok=%v, want %v", tc.input, tc.off, ok, tc.wantOk)
			}
			if ok && tc.input[beg:end] != tc.want {
				t.Errorf("parseLinkDestination(%q, %d): dest=%q, want %q", tc.input, tc.off, tc.input[beg:end], tc.want)
			}
		})
	}
}

func TestParseLinkTitle(t *testing.T) {
	tests := []struct {
		input  string
		wantOk bool
		want   string
	}{
		{" \"title\"", true, "title"},
		{" 'title'", true, "title"},
		{" (title)", true, "title"},
		{" notitle", false, ""},
		{"", false, ""},
	}

	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			beg, end, _, ok := parseLinkTitle([]byte(tc.input), 0)
			if ok != tc.wantOk {
				t.Errorf("parseLinkTitle(%q): ok=%v, want %v", tc.input, ok, tc.wantOk)
			}
			if ok {
				got := tc.input[beg:end]
				if got != tc.want {
					t.Errorf("parseLinkTitle(%q): title=%q, want %q", tc.input, got, tc.want)
				}
			}
		})
	}
}

func TestNormalizeLinkLabel(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"foo", "foo"},
		{"FOO", "foo"},
		{"Foo Bar", "foo bar"},
		{"Foo  Bar", "foo bar"},
		{"  Foo  ", "foo"},
		{"", ""},
		// Unicode case folding via unicode.ToLower (I52 regression)
		{"İ", "i"},     // U+0130 Latin Capital I with dot above → lowercase i
		{"ΑΒΓ", "αβγ"}, // Greek uppercase → lowercase
		{"ẞ", "ss"},    // U+1E9E sharp S → multi-char fold "ss"
		{"ß", "ss"},    // U+00DF eszett → multi-char fold "ss"
		{"ﬁ", "fi"},    // U+FB01 ligature fi → "fi"
		{"ﬂ", "fl"},    // U+FB02 ligature fl → "fl"
	}

	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			got := string(normalizeLinkLabel([]byte(tc.input)))
			if got != tc.want {
				t.Errorf("normalizeLinkLabel(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

func TestLinkAttrsStorage(t *testing.T) {
	var ms markStacks
	ms.reset()
	ms.storeLinkAttrs(5, []byte("http://example.com"), []byte("title"))
	href, title := ms.getLinkAttrs(5)
	if string(href) != "http://example.com" {
		t.Errorf("href: got %q, want %q", href, "http://example.com")
	}
	if string(title) != "title" {
		t.Errorf("title: got %q, want %q", title, "title")
	}
	// Non-existent key
	href, title = ms.getLinkAttrs(99)
	if href != nil || title != nil {
		t.Errorf("non-existent key: got href=%q title=%q, want nil", href, title)
	}
}

func TestIsLinkReference(t *testing.T) {
	var ms markStacks
	ms.reset()
	ms.refDefs = map[string]*RefDef{
		"foo": {href: []byte("http://example.com"), title: []byte("Example")},
	}

	href, title, ok := ms.isLinkReference([]byte("[foo]"), 0, 5)
	if !ok {
		t.Fatal("expected refdef to be found")
	}
	if string(href) != "http://example.com" {
		t.Errorf("href: got %q, want %q", href, "http://example.com")
	}
	if string(title) != "Example" {
		t.Errorf("title: got %q, want %q", title, "Example")
	}

	// Not found
	_, _, ok = ms.isLinkReference([]byte("[bar]"), 0, 5)
	if ok {
		t.Error("expected refdef not to be found")
	}
}

func TestResolveLinkSpanType(t *testing.T) {
	if resolveLinkSpanType('[') != ast.SpanLink {
		t.Error("expected SpanLink for '['")
	}
	if resolveLinkSpanType('!') != ast.SpanImg {
		t.Error("expected SpanImg for '!'")
	}
}
