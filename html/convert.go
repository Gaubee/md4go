// Package html provides HTML rendering for the md4go parser.
//
// This is a business-layer convenience wrapper. Users who need custom rendering
// should implement the renderer.Renderer interface directly and use md4go.Parser.
package html

import (
	"io"

	"github.com/userpro/md4go/parser"
	"github.com/userpro/md4go/renderer"
)

// Option configures the HTML conversion.
type Option func(*config)

type config struct {
	flags     parser.Flags
	extenders []parser.Extender
	rFlags    RenderFlags
	skipBOM   bool
}

// WithFlags sets parser behavior flags.
func WithFlags(f parser.Flags) Option {
	return func(c *config) { c.flags |= f }
}

// WithExtensions registers runtime extensions with the parser.
func WithExtensions(exts ...parser.Extender) Option {
	return func(c *config) { c.extenders = append(c.extenders, exts...) }
}

// WithRendererFlags sets HTML renderer flags.
func WithRendererFlags(f RenderFlags) Option {
	return func(c *config) { c.rFlags |= f }
}

// Convert parses src and writes HTML to w.
// Uses XHTML-style self-closing tags by default for CommonMark spec compliance.
// If WithRendererFlags(FlagSkipUTF8BOM) is set, UTF-8 BOM at input start is stripped.
func Convert(src []byte, w io.Writer, opts ...Option) error {
	cfg := &config{}
	for _, o := range opts {
		o(cfg)
	}
	// Handle BOM skip
	if cfg.rFlags&FlagSkipUTF8BOM != 0 && len(src) >= 3 {
		if src[0] == 0xEF && src[1] == 0xBB && src[2] == 0xBF {
			src = src[3:]
		}
	}
	// Map parser-level flag to renderer flag
	if cfg.flags&parser.FlagNoXHTMLEntityEncoding != 0 {
		cfg.rFlags |= FlagNoXHTMLEscaping
	}
	p := parser.New(cfg.flags, cfg.extenders...)
	h := NewWithFlags(w, cfg.rFlags)
	if err := p.Parse(src, h); err != nil {
		return err
	}
	return h.Flush()
}

// NewHTML creates an HTML renderer writing to w.
// Uses XHTML-style self-closing tags (<img />, <br />, <hr />)
// by default for CommonMark spec compliance.
func NewHTML(w io.Writer) *HTML {
	h := &HTML{w: renderer.NewBufWriter(w), xhtml: true, flags: FlagXHTML}
	h.initEscapeMaps()
	return h
}

// NewWithFlags creates an HTML renderer with specific renderer flags.
func NewWithFlags(w io.Writer, flags RenderFlags) *HTML {
	h := &HTML{w: renderer.NewBufWriter(w), flags: flags}
	h.xhtml = flags&FlagXHTML != 0
	h.initEscapeMaps()
	return h
}

// RenderFlags is a bitmask of HTML renderer behavior switches.
type RenderFlags uint32

const (
	FlagDebug            RenderFlags = 0x0001
	FlagVerbatimEntities RenderFlags = 0x0002
	FlagSkipUTF8BOM      RenderFlags = 0x0004
	FlagXHTML            RenderFlags = 0x0008
	FlagNoXHTMLEscaping  RenderFlags = 0x0010
)
