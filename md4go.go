// Package md4go provides a Markdown parser with a push-based, event-driven API.
//
// md4go is the base parsing layer — it parses Markdown and emits structured events
// (EnterBlock/LeaveBlock/EnterSpan/LeaveSpan/Text) to any renderer.Renderer
// implementation. HTML and plain-text rendering are provided as separate business-layer
// packages (md4go/html, md4go/text), but users can implement custom renderers
// using just this package and the renderer.Renderer interface.
//
// Basic usage — parse with a custom renderer:
//
//	p := md4go.New(md4go.WithFlags(parser.DialectGitHub))
//	p.Parse([]byte("# Hello"), myRenderer)
//
// Convenience — use the text or html packages for one-shot conversion:
//
//	text.Convert([]byte("# Hello"), os.Stdout, md4go.WithFlags(parser.DialectGitHub))
//	html.Convert([]byte("# Hello"), os.Stdout, md4go.WithFlags(parser.DialectGitHub))
package md4go

import (
	"github.com/userpro/md4go/parser"
	"github.com/userpro/md4go/renderer"
	"github.com/userpro/md4go/stream"
)

// Parser is the main entry point. It holds a reusable parse context.
type Parser struct {
	p *parser.Parser
}

// Option configures the Parser instance.
type Option func(*config)

type config struct {
	flags     parser.Flags
	extenders []parser.Extender
}

// WithFlags sets parser behavior flags.
func WithFlags(f parser.Flags) Option {
	return func(c *config) { c.flags |= f }
}

// WithExtensions registers runtime extensions with the parser.
// Mirrors goldmark's WithExtensions option — extensions are applied
// during Parser construction, before any Parse() call.
func WithExtensions(exts ...parser.Extender) Option {
	return func(c *config) { c.extenders = append(c.extenders, exts...) }
}

// New creates a Parser with the given options.
func New(opts ...Option) *Parser {
	cfg := &config{}
	for _, o := range opts {
		o(cfg)
	}
	return &Parser{p: parser.New(cfg.flags, cfg.extenders...)}
}

// Parse parses src ([]byte) and pushes events to r.
// Uses SliceSource internally — refdefs are fully visible in document order,
// so forward references work.
func (p *Parser) Parse(src []byte, r renderer.Renderer) error {
	return p.p.Parse(src, r)
}

// ParseStream parses from a LineSource and pushes events to r.
// Uses any LineSource — typically ReaderSource for streaming.
// Refdefs are first-seen-first; forward references degrade to literal text.
func (p *Parser) ParseStream(src stream.LineSource, r renderer.Renderer) error {
	return p.p.ParseStream(src, r)
}
