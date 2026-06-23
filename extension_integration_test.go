package md4go

import (
	"bytes"
	"testing"

	"md4go/extension"
	"md4go/parser"
	"md4go/text"
)

func TestWithExtensionsGFM(t *testing.T) {
	p := New(WithExtensions(extension.GFM...))
	var buf bytes.Buffer
	r := text.NewPlainText(&buf)
	if err := p.Parse([]byte("# Hello\n\nworld"), r); err != nil {
		t.Fatalf("Parse failed: %v", err)
	}
	r.Flush()
	if buf.String() == "" {
		t.Error("output should not be empty")
	}
}

func TestWithExtensionsIndividual(t *testing.T) {
	p := New(
		WithExtensions(&extension.Strikethrough{}, &extension.Table{}),
		WithFlags(parser.FlagCollapseWhitespace),
	)
	if p.p.Flags()&parser.FlagStrikethrough == 0 {
		t.Error("FlagStrikethrough not set")
	}
	if p.p.Flags()&parser.FlagTables == 0 {
		t.Error("FlagTables not set")
	}
	if p.p.Flags()&parser.FlagCollapseWhitespace == 0 {
		t.Error("FlagCollapseWhitespace not set")
	}
	if !p.p.HasMarkChar('|') {
		t.Error("'|' should be a mark char after Table extension")
	}
}

func TestWithExtensionsAndFlags(t *testing.T) {
	p := New(
		WithFlags(parser.FlagNoHTMLBlocks),
		WithExtensions(&extension.Strikethrough{}),
		WithFlags(parser.FlagNoHTMLSpans),
	)
	if p.p.Flags()&parser.FlagNoHTMLBlocks == 0 {
		t.Error("FlagNoHTMLBlocks not set")
	}
	if p.p.Flags()&parser.FlagNoHTMLSpans == 0 {
		t.Error("FlagNoHTMLSpans not set")
	}
	if p.p.Flags()&parser.FlagStrikethrough == 0 {
		t.Error("FlagStrikethrough not set")
	}
}

func TestNoExtensionsDefaultBehavior(t *testing.T) {
	p := New()
	var buf bytes.Buffer
	r := text.NewPlainText(&buf)
	if err := p.Parse([]byte("# Title\n\nParagraph."), r); err != nil {
		t.Fatalf("Parse failed: %v", err)
	}
	r.Flush()
	if buf.String() == "" {
		t.Error("output should not be empty")
	}
	if p.p.HasMarkChar('|') {
		t.Error("'|' should not be a mark char without extensions")
	}
}
