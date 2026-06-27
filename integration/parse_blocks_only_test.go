package integration_test

import (
	"testing"

	"github.com/userpro/md4go"
	"github.com/userpro/md4go/ast"
	"github.com/userpro/md4go/parser"
)

// blockCollector collects block-type and counts for ParseBlocksOnly testing.
type blockCollector struct {
	blocks []ast.BlockType
	spans  int // should remain 0 in blocks-only mode
	texts  int // TextCode and TextHTML may appear; span text should not
}

func (c *blockCollector) EnterBlock(t ast.BlockType, _ any) error {
	c.blocks = append(c.blocks, t)
	return nil
}
func (c *blockCollector) LeaveBlock(_ ast.BlockType, _ any) error { return nil }
func (c *blockCollector) EnterSpan(_ ast.SpanType, _ any) error   { c.spans++; return nil }
func (c *blockCollector) LeaveSpan(_ ast.SpanType, _ any) error   { return nil }
func (c *blockCollector) Text(_ ast.TextType, _ []byte) error     { c.texts++; return nil }

// TestParseBlocksOnlyBasic verifies that ParseBlocksOnly emits only block-level
// events for basic markdown structures: headings, paragraphs, code blocks.
func TestParseBlocksOnlyBasic(t *testing.T) {
	p := md4go.New(md4go.WithFlags(parser.DialectGitHub))

	src := []byte("# Heading\n\nA paragraph.\n\n```go\nfmt.Println(\"hi\")\n```\n")

	c := &blockCollector{}
	if err := p.ParseBlocksOnly(src, c); err != nil {
		t.Fatal(err)
	}

	// Verify no Span events emitted
	if c.spans != 0 {
		t.Errorf("expected 0 Span events in blocks-only mode, got %d", c.spans)
	}

	// Expected EnterBlock sequence: Doc, H, P, Code (4 blocks)
	if len(c.blocks) != 4 {
		t.Errorf("expected 4 EnterBlock events (Doc,H,P,Code), got %d: %v", len(c.blocks), c.blocks)
	}
}

// TestParseBlocksOnlyHeadings verifies heading extraction use case.
func TestParseBlocksOnlyHeadings(t *testing.T) {
	p := md4go.New()

	src := []byte("# Intro\n\n## Section 1\n\nText here.\n\n### Sub 1.1\n\n## Section 2\n")

	type heading struct {
		level int
	}
	var headings []heading
	r := &headingCollector{
		onHeading: func(d *ast.HeadingDetail) {
			headings = append(headings, heading{level: d.Level})
		},
	}

	if err := p.ParseBlocksOnly(src, r); err != nil {
		t.Fatal(err)
	}

	expected := []int{1, 2, 3, 2}
	if len(headings) != len(expected) {
		t.Fatalf("expected %d headings, got %d", len(expected), len(headings))
	}
	for i, h := range headings {
		if h.level != expected[i] {
			t.Errorf("heading[%d]: expected level %d, got %d", i, expected[i], h.level)
		}
	}
}

// headingCollector collects heading blocks via a callback.
type headingCollector struct {
	onHeading func(d *ast.HeadingDetail)
}

func (h *headingCollector) EnterBlock(t ast.BlockType, d any) error {
	if t == ast.BlockH && h.onHeading != nil {
		if hd, ok := d.(*ast.HeadingDetail); ok {
			h.onHeading(hd)
		}
	}
	return nil
}
func (h *headingCollector) LeaveBlock(_ ast.BlockType, _ any) error { return nil }
func (h *headingCollector) EnterSpan(_ ast.SpanType, _ any) error   { return nil }
func (h *headingCollector) LeaveSpan(_ ast.SpanType, _ any) error   { return nil }
func (h *headingCollector) Text(_ ast.TextType, _ []byte) error     { return nil }

// TestParseBlocksOnlyWithRefdefs verifies that reference definitions are
// correctly consumed and not emitted as paragraphs.
func TestParseBlocksOnlyWithRefdefs(t *testing.T) {
	p := md4go.New()

	src := []byte("[label]: /url\n\nA paragraph.\n")

	c := &blockCollector{}
	if err := p.ParseBlocksOnly(src, c); err != nil {
		t.Fatal(err)
	}

	// Should be: Doc, P, Doc (the refdef paragraph is suppressed)
	// Only 1 paragraph "A paragraph." — the [label]: /url line is consumed as refdef.
	pCount := 0
	for _, b := range c.blocks {
		if b == ast.BlockP {
			pCount++
		}
	}
	if pCount != 1 {
		t.Errorf("expected exactly 1 paragraph block, got %d (refdef may not have been consumed)", pCount)
	}
}

// TestParseBlocksOnlyEmpty verifies empty input is handled correctly.
func TestParseBlocksOnlyEmpty(t *testing.T) {
	p := md4go.New()

	for _, input := range [][]byte{nil, {}, []byte("\n")} {
		c := &blockCollector{}
		if err := p.ParseBlocksOnly(input, c); err != nil {
			t.Errorf("ParseBlocksOnly(%q): unexpected error: %v", input, err)
		}
	}
}

// TestParseBlocksOnlyNoInlineConflict verifies Parse and ParseBlocksOnly don't
// interfere (noInline flag is properly reset per-context).
func TestParseBlocksOnlyNoInlineConflict(t *testing.T) {
	p := md4go.New()

	// ParseBlocksOnly first
	c1 := &blockCollector{}
	if err := p.ParseBlocksOnly([]byte("**bold**"), c1); err != nil {
		t.Fatal(err)
	}
	// Span events should be 0
	if c1.spans != 0 {
		t.Errorf("ParseBlocksOnly: expected 0 spans, got %d", c1.spans)
	}

	// Then normal parse — span events should appear
	c2 := &blockCollector{}
	if err := p.Parse([]byte("**bold**"), c2); err != nil {
		t.Fatal(err)
	}
	if c2.spans == 0 {
		t.Error("Parse after ParseBlocksOnly: expected span events, got 0 — noInline may be leaking")
	}
}

// TestParseBlocksOnlyConsistency verifies ParseBlocksOnly and Parse emit the
// same block types (just different inline content).
func TestParseBlocksOnlyConsistency(t *testing.T) {
	p := md4go.New(md4go.WithFlags(parser.DialectGitHub))

	src := []byte("# H1\n\nPara.\n\n> Quote\n\n- Item\n\n```go\ncode\n```\n\n---\n")

	c1 := &blockCollector{}
	if err := p.ParseBlocksOnly(src, c1); err != nil {
		t.Fatal(err)
	}

	c2 := &blockCollector{}
	if err := p.Parse(src, c2); err != nil {
		t.Fatal(err)
	}

	// Both should have the same number of EnterBlock events
	if len(c1.blocks) != len(c2.blocks) {
		t.Errorf("block count mismatch: ParseBlocksOnly=%d, Parse=%d", len(c1.blocks), len(c2.blocks))
	}

	// Block sequence should be identical
	for i := 0; i < len(c1.blocks) && i < len(c2.blocks); i++ {
		if c1.blocks[i] != c2.blocks[i] {
			t.Errorf("block[%d] mismatch: ParseBlocksOnly=%v, Parse=%v",
				i, c1.blocks[i], c2.blocks[i])
		}
	}
}
