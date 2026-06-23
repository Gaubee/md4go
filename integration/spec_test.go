package integration_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/userpro/md4go"
	"github.com/userpro/md4go/html"
)

// specCase represents a single CommonMark spec example.
type specCase struct {
	Markdown  string `json:"markdown"`
	HTML      string `json:"html"`
	Example   int    `json:"example"`
	Section   string `json:"section"`
	StartLine int    `json:"start_line"`
	EndLine   int    `json:"end_line"`
}

// loadSpecJSON loads spec cases from the CommonMark JSON file.
func loadSpecJSON(t *testing.T, path string) []specCase {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read spec %s: %v", path, err)
	}
	var cases []specCase
	if err := json.Unmarshal(data, &cases); err != nil {
		t.Fatalf("parse spec JSON: %v", err)
	}
	return cases
}

// blockLevelSections defines CommonMark sections that test block-level parsing.
var blockLevelSections = map[string]bool{
	"Tabs":              true,
	"Backslash escapes": true,
	"Entity and numeric character references": true,
	"Precedence":                 true,
	"Thematic breaks":            true,
	"ATX headings":               true,
	"Setext headings":            true,
	"Indented code blocks":       true,
	"Fenced code blocks":         true,
	"HTML blocks":                true,
	"Link reference definitions": true,
	"Paragraphs":                 true,
	"Blank lines":                true,
	"Block quotes":               true,
	"List items":                 true,
	"Lists":                      true,
}

// inlineSections defines CommonMark sections that require inline parsing (M3).
var inlineSections = map[string]bool{
	"Inlines":                      true,
	"Code spans":                   true,
	"Emphasis and strong emphasis": true,
	"Links":                        true,
	"Images":                       true,
	"Autolinks":                    true,
	"Raw HTML":                     true,
	"Hard line breaks":             true,
	"Soft line breaks":             true,
	"Textual content":              true,
}

// TestCommonMarkSpec runs ALL 652 CommonMark 0.31 spec examples against the
// HTML renderer in a single unified test. This is the primary compliance gate
// for CommonMark 0.31 (M6验收). Reports per-section and overall pass rate.
func TestCommonMarkSpec(t *testing.T) {
	cases := loadSpecJSON(t, "../testdata/spec/commonmark-0.31.json")
	if len(cases) != 652 {
		t.Fatalf("expected 652 CommonMark 0.31 examples, got %d", len(cases))
	}

	passed := 0
	failed := 0
	sectionStats := make(map[string]*struct{ passed, failed int })

	md := md4go.New()

	for _, c := range cases {
		if sectionStats[c.Section] == nil {
			sectionStats[c.Section] = &struct{ passed, failed int }{}
		}
		st := sectionStats[c.Section]

		t.Run(fmt.Sprintf("%s/Example_%d", c.Section, c.Example), func(t *testing.T) {
			var got bytes.Buffer
			h := html.NewHTML(&got)
			if err := md.Parse([]byte(c.Markdown), h); err != nil {
				st.failed++
				failed++
				t.Fatalf("parse error: %v", err)
			}
			_ = h.Flush()

			gotNorm := normalizeHTML(got.String())
			wantNorm := normalizeHTML(c.HTML)

			if gotNorm != wantNorm {
				st.failed++
				failed++
				t.Errorf("section=%s example=%d\n--- input ---\n%q\n--- want ---\n%q\n--- got ---\n%q",
					c.Section, c.Example, c.Markdown, wantNorm, gotNorm)
			} else {
				st.passed++
				passed++
			}
		})
	}

	t.Logf("CommonMark 0.31 spec: %d/%d passed, %d failed", passed, len(cases), failed)

	// Report per-section statistics
	for sec, st := range sectionStats {
		total := st.passed + st.failed
		t.Logf("  %-45s %d/%d", sec, st.passed, total)
	}
}

// TestBlockLevelSpec runs block-level CommonMark spec examples (§1-§16)
// against the HTML renderer. This is the M2 incremental spec validation.
// Note: TestCommonMarkSpec is the primary unified compliance gate (all 652 examples).
// This test is kept for backward compatibility and incremental debugging.
func TestBlockLevelSpec(t *testing.T) {
	cases := loadSpecJSON(t, "../testdata/spec/commonmark-0.31.json")

	passed := 0
	failed := 0
	total := 0

	md := md4go.New()

	for _, c := range cases {
		if !blockLevelSections[c.Section] {
			continue
		}
		total++

		t.Run(fmt.Sprintf("Example_%d", c.Example), func(t *testing.T) {
			var got bytes.Buffer
			h := html.NewHTML(&got)
			if err := md.Parse([]byte(c.Markdown), h); err != nil {
				t.Fatalf("parse error: %v", err)
			}
			_ = h.Flush()

			gotNorm := normalizeHTML(got.String())
			wantNorm := normalizeHTML(c.HTML)

			if gotNorm != wantNorm {
				failed++
				t.Errorf("section=%s example=%d\n--- input ---\n%q\n--- want ---\n%q\n--- got ---\n%q",
					c.Section, c.Example, c.Markdown, wantNorm, gotNorm)
			} else {
				passed++
			}
		})
	}

	t.Logf("Block-level spec: %d/%d passed, %d failed",
		passed, total, failed)
}

// TestInlineSpec runs inline CommonMark spec examples (§8-§13)
// against the HTML renderer. This is the M3 incremental spec validation.
// Note: TestCommonMarkSpec is the primary unified compliance gate (all 652 examples).
// This test is kept for backward compatibility and incremental debugging.
func TestInlineSpec(t *testing.T) {
	cases := loadSpecJSON(t, "../testdata/spec/commonmark-0.31.json")

	passed := 0
	failed := 0
	total := 0

	md := md4go.New()

	for _, c := range cases {
		if !inlineSections[c.Section] {
			continue
		}
		total++

		t.Run(fmt.Sprintf("Example_%d", c.Example), func(t *testing.T) {
			var got bytes.Buffer
			h := html.NewHTML(&got)
			if err := md.Parse([]byte(c.Markdown), h); err != nil {
				t.Fatalf("parse error: %v", err)
			}
			_ = h.Flush()

			gotNorm := normalizeHTML(got.String())
			wantNorm := normalizeHTML(c.HTML)

			if gotNorm != wantNorm {
				failed++
				t.Errorf("section=%s example=%d\n--- input ---\n%q\n--- want ---\n%q\n--- got ---\n%q",
					c.Section, c.Example, c.Markdown, wantNorm, gotNorm)
			} else {
				passed++
			}
		})
	}

	t.Logf("Inline spec: %d/%d passed, %d failed",
		passed, total, failed)
}

// --- HTML Normalization (ported from md4c normalize.py) ---

var whitespaceRe = regexp.MustCompile(`\s+`)

var blockTags = map[string]bool{
	"article": true, "header": true, "aside": true, "hgroup": true,
	"blockquote": true, "hr": true, "iframe": true, "body": true,
	"li": true, "map": true, "button": true, "object": true, "canvas": true,
	"ol": true, "caption": true, "output": true, "col": true, "p": true,
	"colgroup": true, "pre": true, "dd": true, "progress": true,
	"div": true, "section": true, "dl": true, "table": true, "td": true,
	"dt": true, "tbody": true, "embed": true, "textarea": true,
	"fieldset": true, "tfoot": true, "figcaption": true, "th": true,
	"figure": true, "thead": true, "footer": true, "tr": true,
	"form": true, "ul": true, "h1": true, "h2": true, "h3": true,
	"h4": true, "h5": true, "h6": true, "video": true, "script": true,
	"style": true,
}

// htmlChunkRe splits HTML into tags and text chunks.
var htmlChunkRe = regexp.MustCompile(`(<!\[CDATA\[.*?\]\]>|<[^>]*>|[^<]+)`)

// normalizeHTML normalizes HTML output for comparison, following md4c's normalize.py.
func normalizeHTML(html string) string {
	var buf strings.Builder
	buf.Grow(len(html))

	inPre := false
	last := "starttag"
	lastTag := ""

	for _, chunk := range htmlChunkRe.FindAllString(html, -1) {
		if len(chunk) == 0 {
			continue
		}

		// CDATA — pass through
		if strings.HasPrefix(chunk, "<![CDATA") {
			buf.WriteString(chunk)
			last = "cdata"
			continue
		}

		// Tag
		if chunk[0] == '<' {
			tag, attrs, isEnd, _ := parseHTMLTag(chunk)

			if isEnd {
				if tag == "pre" {
					inPre = false
				}
				if blockTags[tag] {
					trimTrailingWhitespace(&buf)
				}
				buf.WriteString("</")
				buf.WriteString(tag)
				buf.WriteString(">")
				lastTag = tag
				last = "endtag"
			} else {
				if tag == "pre" {
					inPre = true
				}
				if blockTags[tag] {
					trimTrailingWhitespace(&buf)
				}
				buf.WriteString("<")
				buf.WriteString(tag)
				writeSortedAttrs(&buf, attrs)
				buf.WriteString(">")
				lastTag = tag
				last = "starttag"
			}
			continue
		}

		// Text content
		data := chunk
		afterTag := last == "endtag" || last == "starttag"
		afterBlockTag := afterTag && blockTags[lastTag]

		if afterTag && lastTag == "br" {
			data = strings.TrimLeft(data, "\n")
		}
		if !inPre {
			data = whitespaceRe.ReplaceAllString(data, " ")
		}
		if afterBlockTag && !inPre {
			if last == "starttag" {
				data = strings.TrimLeft(data, " ")
			} else if last == "endtag" {
				data = strings.TrimSpace(data)
			}
		}
		// Decode HTML entities in text for comparison (mirrors md4c normalize.py html.unescape)
		data = decodeHTMLEntities(data)
		buf.WriteString(data)
		last = "data"
	}

	return buf.String()
}

// trimTrailingWhitespace removes trailing whitespace from buf.
func trimTrailingWhitespace(buf *strings.Builder) {
	s := buf.String()
	trimmed := strings.TrimRight(s, " \t\n\r")
	buf.Reset()
	buf.WriteString(trimmed)
}

// htmlAttr represents an HTML attribute.
type htmlAttr struct {
	key   string
	value string
	bare  bool // true if attribute had no =value (e.g. "disabled" vs "disabled=\"\"")
}

// parseHTMLTag parses an HTML tag string.
func parseHTMLTag(chunk string) (tag string, attrs []htmlAttr, isEnd bool, isSelfClose bool) {
	s := chunk

	if strings.HasPrefix(s, "</") {
		s = s[2:]
		if idx := strings.IndexByte(s, '>'); idx >= 0 {
			tag = strings.ToLower(strings.TrimSpace(s[:idx]))
		}
		isEnd = true
		return
	}

	s = strings.TrimPrefix(s, "<")
	isSelfClose = strings.HasSuffix(s, "/>")
	if isSelfClose {
		s = s[:len(s)-2]
	} else {
		s = strings.TrimSuffix(s, ">")
	}

	i := 0
	for i < len(s) && s[i] != ' ' && s[i] != '\t' && s[i] != '\n' && s[i] != '\r' && s[i] != '/' {
		i++
	}
	tag = strings.ToLower(s[:i])
	s = s[i:]

	attrs = parseAttrs(s)
	return
}

// parseAttrs parses HTML attributes from a string.
func parseAttrs(s string) []htmlAttr {
	var attrs []htmlAttr
	i := 0
	for i < len(s) {
		for i < len(s) && (s[i] == ' ' || s[i] == '\t' || s[i] == '\n' || s[i] == '\r') {
			i++
		}
		if i >= len(s) {
			break
		}
		nameStart := i
		for i < len(s) && s[i] != '=' && s[i] != ' ' && s[i] != '\t' && s[i] != '>' {
			i++
		}
		if i == nameStart {
			i++
			continue
		}
		name := strings.ToLower(s[nameStart:i])

		for i < len(s) && (s[i] == ' ' || s[i] == '\t') {
			i++
		}

		var value string
		bare := true // assume bare attribute until we find '='
		if i < len(s) && s[i] == '=' {
			bare = false
			i++
			for i < len(s) && (s[i] == ' ' || s[i] == '\t') {
				i++
			}
			if i < len(s) && s[i] == '"' {
				i++
				valStart := i
				for i < len(s) && s[i] != '"' {
					i++
				}
				value = s[valStart:i]
				if i < len(s) {
					i++
				}
			} else if i < len(s) && s[i] == '\'' {
				i++
				valStart := i
				for i < len(s) && s[i] != '\'' {
					i++
				}
				value = s[valStart:i]
				if i < len(s) {
					i++
				}
			} else {
				valStart := i
				for i < len(s) && s[i] != ' ' && s[i] != '\t' && s[i] != '>' {
					i++
				}
				value = s[valStart:i]
			}
		}

		attrs = append(attrs, htmlAttr{key: name, value: value, bare: bare})
	}
	return attrs
}

// writeSortedAttrs writes attributes sorted by key name.
// Mirrors md4c normalize.py: bare attributes output as just the key,
// attributes with values output as key="value".
func writeSortedAttrs(buf *strings.Builder, attrs []htmlAttr) {
	if len(attrs) == 0 {
		return
	}
	sort.Slice(attrs, func(i, j int) bool {
		return attrs[i].key < attrs[j].key
	})
	for _, a := range attrs {
		buf.WriteString(" ")
		buf.WriteString(a.key)
		if a.bare {
			// Bare attribute (e.g. "disabled" without ="") — match md4c normalize.py
		} else {
			buf.WriteString("=\"")
			buf.WriteString(escapeAttrValue(a.value))
			buf.WriteString("\"")
		}
	}
}

// escapeAttrValue escapes special characters in attribute values and decodes entities.
func escapeAttrValue(v string) string {
	// Decode HTML entities first for normalized comparison
	v = decodeHTMLEntities(v)
	var buf strings.Builder
	buf.Grow(len(v))
	for _, r := range v {
		switch r {
		case '<':
			buf.WriteString("&lt;")
		case '>':
			buf.WriteString("&gt;")
		case '"':
			buf.WriteString("&quot;")
		default:
			buf.WriteRune(r)
		}
	}
	return buf.String()
}

// decodeHTMLEntities decodes numeric HTML entities.
func decodeHTMLEntities(s string) string {
	result := entityRe.ReplaceAllStringFunc(s, func(match string) string {
		if len(match) < 4 {
			return match
		}
		if match[1] == '#' {
			if match[2] == 'x' || match[2] == 'X' {
				codeStr := match[3 : len(match)-1]
				if cp, err := strconv.ParseInt(codeStr, 16, 32); err == nil {
					return string(rune(cp))
				}
			} else {
				codeStr := match[2 : len(match)-1]
				if cp, err := strconv.ParseInt(codeStr, 10, 32); err == nil {
					return string(rune(cp))
				}
			}
		}
		return match
	})
	return result
}

var entityRe = regexp.MustCompile(`&#[xX]?[0-9a-fA-F]+;|&[A-Za-z][A-Za-z0-9]+;`)
