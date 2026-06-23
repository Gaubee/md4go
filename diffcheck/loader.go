package diffcheck

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// TestCase represents one test input.
type TestCase struct {
	Index  int    // global 0-based index
	Source string // "jsonl" or "fuzz"
	Input  []byte // markdown content
}

// jsonlRecord matches the structure of testdata.jsonl.
type jsonlRecord struct {
	Data struct {
		Content string `json:"content"`
	} `json:"data"`
}

// LoadJSONL reads a JSONL file and extracts data.content from each line.
// Skips lines with empty or missing content.
func LoadJSONL(path string) ([]TestCase, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open jsonl: %w", err)
	}
	defer f.Close()

	var cases []TestCase
	scanner := bufio.NewScanner(f)
	// Allow larger lines (some content can be big)
	scanner.Buffer(make([]byte, 0, 1024*1024), 10*1024*1024)

	lineNum := 0
	for scanner.Scan() {
		lineNum++
		line := scanner.Text()
		if strings.TrimSpace(line) == "" {
			continue
		}

		var rec jsonlRecord
		if err := json.Unmarshal([]byte(line), &rec); err != nil {
			// Skip invalid JSON lines gracefully
			continue
		}
		if rec.Data.Content == "" {
			continue
		}

		cases = append(cases, TestCase{
			Index:  lineNum - 1, // 0-based
			Source: "jsonl",
			Input:  []byte(rec.Data.Content),
		})
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read jsonl: %w", err)
	}
	return cases, nil
}

// DefaultJSONLPath returns the default path to JSONL data files
// relative to this source file. Tries aidata_content.jsonl first,
// then testdata1.jsonl + testdata2.jsonl.
func DefaultJSONLPath() string {
	_, thisFile, _, _ := runtime.Caller(0)
	dir := filepath.Join(filepath.Dir(thisFile), "data")

	// Try original name first
	orig := filepath.Join(dir, "aidata_content.jsonl")
	if _, err := os.Stat(orig); err == nil {
		return orig
	}
	// Fallback: first available split file
	for _, name := range []string{"testdata1.jsonl", "testdata2.jsonl"} {
		p := filepath.Join(dir, name)
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return orig // return original path for error message
}

// LoadAllJSONL loads all available JSONL data files from the data directory.
func LoadAllJSONL() ([]TestCase, error) {
	_, thisFile, _, _ := runtime.Caller(0)
	dir := filepath.Join(filepath.Dir(thisFile), "data")

	var cases []TestCase
	for _, name := range []string{"aidata_content.jsonl", "testdata1.jsonl", "testdata2.jsonl"} {
		p := filepath.Join(dir, name)
		if _, err := os.Stat(p); err != nil {
			continue
		}
		loaded, err := LoadJSONL(p)
		if err != nil {
			return nil, fmt.Errorf("load %s: %w", name, err)
		}
		cases = append(cases, loaded...)
	}
	return cases, nil
}

// LoadFuzzSeeds returns hardcoded fuzz seed cases.
// Derived from fuzz_test.go seeds, plus additional edge/invalid inputs.
// Expanded in I58 to cover CommonMark 0.31 section boundaries,
// md4c C version alignment areas, and fuzz-constructed valid/invalid patterns.
func LoadFuzzSeeds() []TestCase {
	seeds := []string{
		// ── Basic markdown ──
		"# Hello\n",
		"**bold** and *italic*\n",
		"[link](http://example.com)\n",
		"![img](src.png)\n",
		"`code`\n",
		"```\nfenced\n```\n",
		"    indented code\n",
		"> blockquote\n",
		"- item 1\n- item 2\n",
		"1. first\n2. second\n",
		"---\n",
		"---\n\nfoo\n\n---\n",

		// ── HTML blocks ──
		"<div>html</div>\n",
		"<!-- comment -->\n",
		"<?php echo ?>\n",
		"<![CDATA[data]]>\n",

		// ── Entities ──
		"&amp; &copy; &#1234; &#xAbCD;\n",
		"&nonexistent;\n",

		// ── Escapes ──
		"\\*not bold\\*\n",
		"\\# not heading\n",

		// ── Links ──
		"[ref][]\n\n[ref]: /url\n",
		"[text][ref]\n\n[ref]: /url\n",

		// ── Extensions (GFM) ──
		"~~strikethrough~~\n",
		"| a | b |\n|---|---|\n| c | d |\n",
		"- [x] task\n",

		// ── Edge cases ──
		"\x00null\x00char\n",
		"",
		"\n",
		"\n\n\n",
		"   \n",
		"\t\ttab\n",
		"a\u200Bb\u200Cc\n",
		"emoji 🎉 test\n",

		// ── Pathological-style (reduced) ──
		"*a **a b a** a*\n",
		strings.Repeat("[", 100) + "a" + strings.Repeat("]", 100) + "\n",
		strings.Repeat("> ", 50) + "a\n",

		// ── Invalid/edge UTF-8 ──
		"\xff\xfe invalid utf8\n",
		"\xc0\x80 overlong null\n",

		// ── Deep nesting ──
		strings.Repeat("> ", 100) + "deep\n",

		// ── Very long line ──
		strings.Repeat("x", 10000) + "\n",

		// ── Empty-ish ──
		"   \n   \n",
		"# Only heading\n",
		"Paragraph with    extra   spaces.\n",

		// ── Mixed content ──
		"# Title\n\nParagraph with **bold** and *italic*.\n\n- item 1\n- item 2\n\n```\ncode block\n```\n",

		// ── I58: Fuzz-constructed valid markdown (CommonMark 0.31 section coverage) ──

		// Tabs (§1)
		"\tcode\n",
		"    \tindented with tab\n",
		"*\tlist item with tab\n",

		// Backslash escapes (§2)
		"\\!\\\"\\#\\$\\%\\&\\'\\(\\)\\*\\+\\,\\-\\.\\/\\:\\;\\<\\=\\>\\?\\@\\[\\\\\\]\\^\\_\\`\\{\\|\\}\\~\n",
		"\\*not emphasis\\*\n",
		"\\\n", // backslash at end of line → hard break

		// Entity references (§2)
		"&amp; &lt; &gt; &quot;\n",
		"&#34; &#x22;\n",
		"&#0; &#xD800;\n", // invalid codepoints

		// Precedence (§3)
		"- `one\n- two`\n", // code span vs list

		// Thematic breaks (§4)
		"***\n",
		"---\n",
		"___\n",
		" * * * \n",
		"  ***  \n",

		// ATX headings (§5)
		"## heading ##\n",
		"# heading # trailing\n",
		"###No space\n", // not a heading per spec? actually is
		"###### heading\n",
		"####### not heading\n", // 7 hashes

		// Setext headings (§6)
		"Heading\n=======\n",
		"Heading\n-------\n",

		// Indented code blocks (§7)
		"    code1\n    code2\n",
		"    code\n\nparagraph\n",

		// Fenced code blocks (§8)
		"```\ncode\n```\n",
		"~~~\ncode\n~~~\n",
		"```javascript\nconsole.log('hello');\n```\n",
		"```\tinfo\n```\n", // tab in info string (I57 fix)

		// HTML blocks (§9)
		"<pre>foo</pre>\n",
		"<script>alert(1)</script>\n",
		"<table>\n<tr>\n<td>\n</td>\n</tr>\n</table>\n",

		// Link reference definitions (§10)
		"[foo]: /url \"title\"\n",
		"[FOO]: /url\n\n[foo]\n", // case-insensitive

		// Paragraphs (§11)
		"aaa\n\nbbb\n",
		"aaa\nbbb\n", // soft break

		// Blank lines (§12)
		"\n\n\n",
		"  \n  \n",

		// Block quotes (§13)
		"> quote\n> continued\n",
		"> > nested\n",
		"> - list in quote\n",

		// List items (§14)
		"- one\n  - nested\n",
		"1. ordered\n   indented\n",
		"1. one\n2. two\n",

		// Lists (§15)
		"- one\n\n- two\n",  // loose list
		"- one\n- two\n",    // tight list
		"- a\n- b\n\n  c\n", // lazy continuation

		// Code spans (§8)
		"`code`\n",
		"``code``\n",
		"`code with space`\n",
		"`` `\n` ``\n", // backtick in code span

		// Emphasis and strong emphasis (§9)
		"*emphasis*\n",
		"**strong**\n",
		"***both***\n",
		"_emphasis_\n",
		"__strong__\n",
		"foo*bar*baz\n",
		"2*3*4\n",         // emphasis in digits
		"_foo_bar_baz_\n", // intra-word

		// Links (§10)
		"[link](/url)\n",
		"[link](/url \"title\")\n",
		"[link][/url]\n", // invalid link
		"[link](/url 'title')\n",
		"[link][]\n\n[link]: /url\n", // collapsed reference

		// Images (§11)
		"![alt](src.png)\n",
		"![alt](src.png \"title\")\n",
		"![outer ![inner](x.png)](y.png)\n", // nested image (I57 fix)

		// Autolinks (§12)
		"<http://example.com>\n",
		"<user@example.com>\n",

		// Raw HTML (§13)
		"<span>html</span>\n",
		"<br>\n",

		// Hard/soft line breaks (§14-15)
		"foo  \nbar\n", // hard break (2 spaces)
		"foo\\\nbar\n", // hard break (backslash)
		"foo\nbar\n",   // soft break

		// Textual content (§16)
		"hello\n",
		"Multiple   spaces\n",

		// ── I58: Fuzz-constructed invalid/truncated inputs ──

		// Truncated structures
		"```\nno closing fence",
		"~~~\nno closing",
		"> unclosed blockquote",
		"- unclosed list\n  still going",
		"[unclosed link](\n",
		"[unclosed reference][\n",
		"**unclosed emphasis",
		"*unclosed italic",
		"`unclosed code span",
		"~~unclosed strikethrough",

		// Mismatched delimiters
		"**close with *_\n",
		"__close with _*__\n",
		"~~wrong closer~~\n",
		"*mixed **delimiters***\n",

		// Empty/zero-length structures
		"[]\n",
		"[]()\n",
		"![]()\n",
		"**\n",
		"__\n",
		"~~\n",

		// NULL characters in various positions
		"\x00\n",
		"a\x00b\n",
		"# \x00heading\n",
		"[\x00](url)\n",
		"`\x00`\n",

		// Boundary: codespanMaxLen=32
		strings.Repeat("`", 32) + "code" + strings.Repeat("`", 32) + "\n",
		strings.Repeat("`", 33) + "no match" + strings.Repeat("`", 33) + "\n",
		strings.Repeat("`", 16) + "code" + strings.Repeat("`", 16) + "\n",

		// Deeply nested emphasis (Rule-of-3 stress)
		"***a***\n",
		"****a****\n",
		"*****a*****\n",
		"*a **b *c***\n",

		// Table edge cases (GFM)
		"| a |\n|---|\n| b |\n",
		"| a | b |\n|---|---|\n",
		"| a || b |\n",            // || in cell (S-01)
		"a | b\n---|---\nc | d\n", // no leading pipe
		"| a\n|---\n| b\n",        // no trailing pipe

		// Admonition (GFM)
		"> [!NOTE]\n> content\n",
		"> [!WARNING]\n> text\n",

		// Footnote (GFM)
		"Text[^1]\n\n[^1]: Footnote content\n",

		// Wikilink (GFM)
		"[[target]]\n",
		"[[target|label]]\n",

		// Mixed valid/invalid
		"# Heading\n\n**bold** and ~~strike~~\n\n```\ncode\n",
		"[link](url 'unclosed title\n",
		"![img](src.png \"title with \\\" quote\")\n",
		"<!-- unclosed comment\n",
		"<div>\nunclosed div\n",

		// Whitespace edge cases
		" \n \n \n",
		"\t\n",
		"   # indented heading\n",
		"    # code block with hash\n",
		"  * list with 2-space indent\n",

		// Unicode edge cases
		"Ünïcödé têxt\n",
		"中文标题\n",
		"🔗 emoji link\n",
		"café résumé naïve\n",
	}

	var cases []TestCase
	for i, s := range seeds {
		cases = append(cases, TestCase{
			Index:  i,
			Source: "fuzz",
			Input:  []byte(s),
		})
	}
	return cases
}

// LoadFuzzConstructed returns programmatically constructed test cases
// designed to stress parser boundaries and md4c alignment.
// These are generated rather than hardcoded to cover parameter spaces.
func LoadFuzzConstructed() []TestCase {
	var cases []TestCase
	idx := 0
	add := func(s string) {
		cases = append(cases, TestCase{Index: idx, Source: "constructed", Input: []byte(s)})
		idx++
	}

	// Nesting depth sweep: 1..30 levels
	for depth := 1; depth <= 30; depth++ {
		add(strings.Repeat("> ", depth) + "text\n")
		add(strings.Repeat("* ", depth) + "item\n")
	}

	// Emphasis length sweep: 1..8 asterisks
	for n := 1; n <= 8; n++ {
		add(strings.Repeat("*", n) + "text" + strings.Repeat("*", n) + "\n")
		add(strings.Repeat("_", n) + "text" + strings.Repeat("_", n) + "\n")
	}

	// Backtick length sweep: 1..10 backticks
	for n := 1; n <= 10; n++ {
		add(strings.Repeat("`", n) + "code" + strings.Repeat("`", n) + "\n")
	}

	// ATX heading level sweep: 1..7
	for level := 1; level <= 7; level++ {
		add(strings.Repeat("#", level) + " heading\n")
	}

	// Ordered list start number sweep
	for start := 0; start <= 10; start++ {
		add(fmt.Sprintf("%d. item\n", start))
	}

	// Indent level sweep for indented code (1..8 spaces)
	for indent := 1; indent <= 8; indent++ {
		add(strings.Repeat(" ", indent) + "code\n")
	}

	// Empty structure variations
	add("")
	add("\n")
	add("\r\n")
	add("\r")
	add(" \t \n")
	add("#\n")        // empty heading
	add(">\n")        // empty blockquote
	add("-\n")        // empty list item
	add("```\n```\n") // empty fenced code

	// Reference definition variations
	add("[label]: /url\n")
	add("[label]: /url 'title'\n")
	add("[label]: /url \"title\"\n")
	add("[label]: /url (title)\n")
	add("[LABEL]: /url\n\n[label]\n") // case folding

	// Link destination edge cases
	add("[t](<url with spaces>)\n")
	add("[t](url\\)with\\(parens)\n")
	add("[t]()\n")
	add("[t](#)\n")

	// HTML block type boundaries (7 types)
	add("<script>content</script>\n")
	add("<pre>content</pre>\n")
	add("<style>content</style>\n")
	add("<!-- comment -->\n")
	add("<?php ?>\n")
	add("<!DOCTYPE html>\n")
	add("<![CDATA[data]]>\n")

	// Hard break variations
	add("a  \nb\n")  // 2 spaces
	add("a   \nb\n") // 3 spaces
	add("a\\\nb\n")  // backslash
	add("a\nb\n")    // soft break

	return cases
}
