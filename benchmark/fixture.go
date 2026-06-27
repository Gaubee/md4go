package benchmark

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/userpro/md4go/ast"
	"github.com/userpro/md4go/parser"
	"github.com/userpro/md4go/renderer"
)

// ─── NullRenderer: sink renderer for parse-only benchmarks ───

// NullRenderer counts bytes without producing output.
// Implements renderer.Renderer.
type NullRenderer struct {
	n int64 // total text bytes seen (prevents dead-code elimination)
}

func (nr *NullRenderer) EnterBlock(ast.BlockType, any) error { return nil }
func (nr *NullRenderer) LeaveBlock(ast.BlockType, any) error { return nil }
func (nr *NullRenderer) EnterSpan(ast.SpanType, any) error   { return nil }
func (nr *NullRenderer) LeaveSpan(ast.SpanType, any) error   { return nil }
func (nr *NullRenderer) Text(_ ast.TextType, text []byte) error {
	nr.n += int64(len(text))
	return nil
}

// ─── CommonMark spec document builder ───

type specCase struct {
	Markdown string `json:"markdown"`
}

var (
	cmDocCache  string
	cmDocOnce   sync.Once
	cmDocErr    error
)

// BuildCommonMarkDoc loads CommonMark 0.31 spec examples and concatenates them.
func BuildCommonMarkDoc() string {
	cmDocOnce.Do(func() {
		cmDocCache, cmDocErr = buildCommonMarkDoc()
	})
	if cmDocErr != nil {
		panic("failed to build CommonMark doc: " + cmDocErr.Error())
	}
	return cmDocCache
}

func buildCommonMarkDoc() (string, error) {
	data, err := os.ReadFile("../testdata/spec/commonmark-0.31.json")
	if err != nil {
		return "", fmt.Errorf("read spec: %w", err)
	}
	var cases []specCase
	if err := json.Unmarshal(data, &cases); err != nil {
		return "", fmt.Errorf("parse spec: %w", err)
	}

	var sb strings.Builder
	for i, tc := range cases {
		if i > 0 {
			sb.WriteString("\n\n\u2014\u2014\u2014\n\n") // separator
		}
		sb.WriteString(tc.Markdown)
	}
	return sb.String(), nil
}

// ─── Pathological cases (mirror integration/pathological_test.go) ───

// PathCase is a pathological benchmark input.
type PathCase struct {
	Name  string
	Input func() string // lazy generation
	Flags parser.Flags  // parser flags
}

// BuildPathologicalCases returns 26 pathological test cases.
// N=500 for benchmark (large enough to measure without O(n²) hangs).
func BuildPathologicalCases() []PathCase {
	const N = 500
	const M = 300

	cases := []PathCase{
		{Name: "U+0000", Input: func() string { return "abc\x00de\x00" }},
		{Name: "nested_strong_emph", Input: func() string {
			return strings.Repeat("*a **a ", N) + "b" + strings.Repeat(" a** a*", N)
		}},
		{Name: "many_emph_closers", Input: func() string {
			return strings.Repeat("a_ ", N)
		}},
		{Name: "many_emph_openers", Input: func() string {
			return strings.Repeat("_a ", N)
		}},
		{Name: "many_3_emph_openers", Input: func() string {
			return strings.Repeat("a***", N)
		}},
		{Name: "many_link_closers", Input: func() string {
			return strings.Repeat("a]", N)
		}},
		{Name: "many_link_openers", Input: func() string {
			return strings.Repeat("[a", N)
		}},
		{Name: "mismatched_openers_closers", Input: func() string {
			return strings.Repeat("*a_ ", M)
		}},
		{Name: "openers_closers_multiple_3", Input: func() string {
			return "a**b" + strings.Repeat("c* ", M)
		}},
		{Name: "link_openers_emph_closers", Input: func() string {
			return strings.Repeat("[ a_", M)
		}},
		{Name: "hard_link_emph", Input: func() string {
			return "**x [a*b**c*](d)"
		}},
		{Name: "nested_brackets", Input: func() string {
			return strings.Repeat("[", M) + "a" + strings.Repeat("]", M)
		}},
		{Name: "nested_block_quotes", Input: func() string {
			return strings.Repeat("> ", M) + "a"
		}},
		{Name: "backticks", Input: func() string {
			var b strings.Builder
			for i := 1; i < 200; i++ {
				b.WriteString("e")
				b.WriteString(strings.Repeat("`", i))
			}
			return b.String()
		}},
		{Name: "many_links", Input: func() string {
			return strings.Repeat("[t](/u) ", M)
		}},
		{Name: "deeply_nested_lists", Input: func() string {
			var b strings.Builder
			for i := range 200 {
				b.WriteString(strings.Repeat("  ", i))
				b.WriteString("* a\n")
			}
			return b.String()
		}},
		{Name: "many_html_openers", Input: func() string {
			return strings.Repeat("<>", M)
		}},
		{Name: "many_html_proc_inst", Input: func() string {
			return "x" + strings.Repeat("<?", M)
		}},
		{Name: "many_html_CDATA", Input: func() string {
			return "x" + strings.Repeat("<![CDATA[", M)
		}},
		{Name: "many_backticks_escapes", Input: func() string {
			return strings.Repeat("\\``", M)
		}},
		{Name: "many_broken_link_titles", Input: func() string {
			return strings.Repeat("[ (](", M)
		}},
		{Name: "nested_invalid_link_refs", Input: func() string {
			return strings.Repeat("[", M) + strings.Repeat("]", M) + "\n\n[a]: /b"
		}},
		{Name: "permissive_autolinks", Input: func() string {
			return strings.Repeat("www._", M) + "x"
		}, Flags: parser.FlagPermissiveWWWAutolinks},
		{Name: "huge_table", Input: func() string {
			const T = 500
			return strings.Repeat("th|", T) + "\n" + strings.Repeat("-|", T) + "\n" + strings.Repeat("td\n", T)
		}, Flags: parser.FlagTables},
		{Name: "many_broken_links", Input: func() string {
			return strings.Repeat("]([\n", M)
		}},
		{Name: "many_link_ref_instances", Input: func() string {
			return "[x]: " + strings.Repeat("x", M) + "\n" + strings.Repeat("[x] ", M)
		}},
	}
	return cases
}

// ─── GFM document builder ───

var (
	gfmDocCache string
	gfmDocOnce  sync.Once
)

// BuildGFMDoc returns a Markdown document exercising GFM extensions.
func BuildGFMDoc() string {
	gfmDocOnce.Do(func() {
		var sb strings.Builder
		// Tables
		sb.WriteString("## Tables\n\n")
		sb.WriteString("| Name | Age | City |\n|------|-----|------|\n")
		sb.WriteString(strings.Repeat("| Alice | 30 | Beijing |\n", 200))
		sb.WriteString("\n")
		// Task lists
		sb.WriteString("## Task Lists\n\n")
		for i := 0; i < 200; i++ {
			sb.WriteString("- [x] done task\n- [ ] pending task\n")
		}
		sb.WriteString("\n")
		// Strikethrough
		sb.WriteString("## Strikethrough\n\n")
		sb.WriteString(strings.Repeat("~~deleted~~ ", 200))
		sb.WriteString("\n\n")
		// Autolinks
		sb.WriteString("## Autolinks\n\n")
		for i := 0; i < 100; i++ {
			sb.WriteString("Visit http://example.com/path and user@example.com.\n")
		}
		gfmDocCache = sb.String()
	})
	return gfmDocCache
}

// ─── JSONL Loader (compatible with diffcheck/data/ format) ───

// TestCase is a single Markdown input.
type TestCase struct {
	Index  int    // 0-based index
	Input  []byte // markdown content
	Source string // filename
}

type jsonlRecord struct {
	Data struct {
		Content string `json:"content"`
	} `json:"data"`
}

// LoadJSONL reads a JSONL file, extracting data.content from each line.
func LoadJSONL(path string) ([]TestCase, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open jsonl: %w", err)
	}
	defer f.Close()

	var cases []TestCase
	scanner := bufio.NewScanner(f)
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
			continue
		}
		if rec.Data.Content == "" {
			continue
		}
		cases = append(cases, TestCase{
			Index:  lineNum - 1,
			Input:  []byte(rec.Data.Content),
			Source: filepath.Base(path),
		})
	}
	return cases, scanner.Err()
}

// LoadDataDir loads all .jsonl files from directory dir.
func LoadDataDir(dir string) (map[string][]TestCase, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("read data dir: %w", err)
	}
	files := make(map[string][]TestCase)
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".jsonl") {
			continue
		}
		cases, err := LoadJSONL(filepath.Join(dir, e.Name()))
		if err != nil {
			return nil, fmt.Errorf("load %s: %w", e.Name(), err)
		}
		if len(cases) > 0 {
			files[e.Name()] = cases
		}
	}
	return files, nil
}

// ─── HTML normalization ───

// normalizeHTML trims trailing whitespace for basic output comparison.
func normalizeHTML(s string) string { return strings.TrimSpace(s) }

// ─── Compile-time interface check ───
var _ renderer.Renderer = (*NullRenderer)(nil)
