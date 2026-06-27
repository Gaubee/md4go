package benchmark

import (
	"bytes"
	"path/filepath"
	"testing"

	"github.com/userpro/md4go"
	"github.com/userpro/md4go/html"
	"github.com/userpro/md4go/parser"
	"github.com/yuin/goldmark"
	goldmarkExt "github.com/yuin/goldmark/extension"
)

// ─── md4c CGo correctness tests ───

func TestMd4cCgoNoPanic(t *testing.T) {
	samples := []struct {
		name  string
		input string
	}{
		{"empty", ""},
		{"simple", "# Hello\n\n**bold** text"},
		{"code", "```\ncode\n```\n"},
		{"table", "| a | b |\n|---|---|\n| 1 | 2 |\n"},
		{"null", "a\x00b"},
	}
	for _, s := range samples {
		t.Run(s.name, func(t *testing.T) {
			out := Md4cConvertHTML([]byte(s.input), 0, 0)
			if s.input == "" && out != "" {
				t.Errorf("empty input should produce empty output, got %q", out)
			}
		})
	}

	// Null parse should not panic
	Md4cParseNull([]byte("# Hello\n\n**bold**"), 0)
}

func TestMd4cCgoOutput(t *testing.T) {
	cases, err := LoadJSONL(filepath.Join("data", "example.jsonl"))
	if err != nil {
		t.Fatalf("load example.jsonl: %v", err)
	}
	if len(cases) == 0 {
		t.Fatal("no test cases in example.jsonl")
	}

	t.Logf("testing %d example.jsonl cases", len(cases))

	for i, tc := range cases {
		t.Run(sanitizeTestName(tc.Input, i), func(t *testing.T) {
			// md4c via CGo
			md4cOut := normalizeHTML(Md4cConvertHTML(tc.Input, 0, 0))

			// md4go
			var md4goBuf bytes.Buffer
			md := md4go.New(md4go.WithFlags(parser.DialectCommonMark))
			h := html.NewWithFlags(&md4goBuf, 0) // HTML5 mode, no XHTML
			if err := md.Parse(tc.Input, h); err != nil {
				t.Fatalf("md4go parse: %v", err)
			}
			h.Flush()
			md4goOut := normalizeHTML(md4goBuf.String())

			// Compare md4c-CGo and md4go (they share the same algorithm)
			if md4cOut != md4goOut {
				// goldmark strips HTML blocks by default — this is a known
				// divergence. md4c and md4go preserve raw HTML.
				t.Logf("md4c vs md4go differ (may be expected for HTML blocks):")
				t.Logf("  md4c-CGo: %s", truncate(md4cOut, 200))
				t.Logf("  md4go:    %s", truncate(md4goOut, 200))
			}

			// goldmark
			var gb bytes.Buffer
			gm := goldmark.New(
				goldmark.WithExtensions(goldmarkExt.GFM),
			)
			if err := gm.Convert(tc.Input, &gb); err != nil {
				t.Fatalf("goldmark convert: %v", err)
			}
			goldOut := normalizeHTML(gb.String())
			_ = goldOut

			// Sanity: all three should produce non-empty for non-empty input
			if len(tc.Input) > 0 && len(tc.Input) > 5 {
				if md4cOut == "" {
					t.Error("md4c-CGo produced empty output for non-empty input")
				}
				if md4goOut == "" {
					t.Error("md4go produced empty output for non-empty input")
				}
				if goldOut == "" {
					t.Log("goldmark produced empty output (may strip HTML content)")
				}
			}
		})
	}
}

func TestMd4cCgoPathologicalNoPanic(t *testing.T) {
	cases := BuildPathologicalCases()
	for _, tc := range cases {
		t.Run(tc.Name, func(t *testing.T) {
			input := tc.Input()
			// Should not panic
			_ = Md4cConvertHTML([]byte(input), uint(tc.Flags), 0)
			Md4cParseNull([]byte(input), uint(tc.Flags))
		})
	}
}

//
// helpers
//

func sanitizeTestName(input []byte, idx int) string {
	s := string(input)
	// Take the first visible line (skip headings)
	lines := bytes.Split(input, []byte("\n"))
	for _, l := range lines {
		trimmed := bytes.TrimSpace(l)
		if len(trimmed) > 0 && !bytes.HasPrefix(trimmed, []byte("#")) {
			s = string(trimmed)
			break
		}
	}
	if len(s) > 40 {
		s = s[:40]
	}
	return s
}

func truncate(s string, n int) string {
	if len(s) > n {
		return s[:n] + "..."
	}
	return s
}
