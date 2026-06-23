// Package text provides PlainText rendering for the md4go parser.
//
// This is a business-layer convenience wrapper. Users who need custom rendering
// should implement the renderer.Renderer interface directly and use md4go.Parser.
package text

import (
	"io"

	"md4go/parser"
	"md4go/stream"
)

// Option configures the text conversion.
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
func WithExtensions(exts ...parser.Extender) Option {
	return func(c *config) { c.extenders = append(c.extenders, exts...) }
}

// Convert parses src and writes plain text to w.
// Uses SliceSource internally — refdef is fully visible, forward references work.
func Convert(src []byte, w io.Writer, opts ...Option) error {
	cfg := &config{}
	for _, o := range opts {
		o(cfg)
	}
	p := parser.New(cfg.flags, cfg.extenders...)
	r := NewPlainTextWithFlags(w, cfg.flags)
	if err := p.Parse(src, r); err != nil {
		return err
	}
	return r.Flush()
}

// ConvertStream parses from r (io.Reader) and writes plain text to w.
// Uses ReaderSource — incremental, low memory. Refdef is first-seen-first.
func ConvertStream(r io.Reader, w io.Writer, opts ...Option) error {
	cfg := &config{}
	for _, o := range opts {
		o(cfg)
	}
	p := parser.New(cfg.flags, cfg.extenders...)
	rs := stream.NewReaderSource(r)
	pt := NewPlainTextWithFlags(w, cfg.flags)
	if err := p.ParseStream(rs, pt); err != nil {
		return err
	}
	return pt.Flush()
}
