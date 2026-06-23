package integration_test

import (
	"bytes"
	"strings"
	"testing"

	"md4go"
	"md4go/extension"
	"md4go/html"
	"md4go/parser"
)

func TestBehaviorExperiment(t *testing.T) {
	// 1) table without extension
	p := md4go.New()
	var b1 bytes.Buffer
	h := html.NewHTML(&b1)
	p.Parse([]byte("| a | b |\n|---|---|\n| c | d |"), h)
	h.Flush()
	t.Logf("no-ext table: %q", b1.String())

	// 2) table WITH extension
	p2 := md4go.New(md4go.WithExtensions(&extension.Table{}), md4go.WithFlags(parser.FlagCollapseWhitespace))
	var b2 bytes.Buffer
	h2 := html.NewHTML(&b2)
	p2.Parse([]byte("| a | b |\n|---|---|\n| c | d |"), h2)
	h2.Flush()
	t.Logf("ext table: %q contains<table>=%v", b2.String(), strings.Contains(b2.String(), "<table>"))

	// 3) strikethrough
	var b3 bytes.Buffer
	h3 := html.NewHTML(&b3)
	p2.Parse([]byte("~~del~~"), h3)
	h3.Flush()
	t.Logf("strike: %q contains<del>=%v", b3.String(), strings.Contains(b3.String(), "<del>"))

	// 4) FlagNoHTMLBlocks + FlagNoHTMLSpans + strikethrough ext
	p4 := md4go.New(
		md4go.WithFlags(parser.FlagNoHTMLBlocks),
		md4go.WithExtensions(&extension.Strikethrough{}),
		md4go.WithFlags(parser.FlagNoHTMLSpans),
	)
	var b4 bytes.Buffer
	h4 := html.NewHTML(&b4)
	p4.Parse([]byte("<div>raw</div>\n\n~~strike~~\n\n<b>inline</b>"), h4)
	h4.Flush()
	out4 := b4.String()
	t.Logf("noflags: %q", out4)
	t.Logf("  contains<div>=%v contains<del>=%v contains<b>=%v contains&lt;b&gt;=%v",
		strings.Contains(out4, "<div>"), strings.Contains(out4, "<del>"),
		strings.Contains(out4, "<b>"), strings.Contains(out4, "&lt;b&gt;"))
}
