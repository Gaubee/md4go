// Package renderer defines the Renderer interface — the contract between
// the parser and rendering targets.
//
// A Renderer consumes parse events (EnterBlock/LeaveBlock/EnterSpan/LeaveSpan/Text)
// and produces output in its target format. Multiple renderers (PlainText, HTML, …)
// share the same event stream.
//
// This package is part of the md4go base layer. Users implement this interface
// to consume parse events from the parser. Concrete renderer implementations
// (PlainText, HTML) live in separate packages (md4go/text, md4go/html).
package renderer

import (
	"github.com/userpro/md4go/ast"
)

// Renderer is the core abstraction between the parser and rendering targets.
// Every rendering target (PlainText, HTML, …) implements this interface.
//
// Users implement Renderer to consume parse events from md4go.Parser.
// See md4go/text and md4go/html for ready-made implementations.
type Renderer interface {
	// EnterBlock signals the start of a block-level element.
	EnterBlock(t ast.BlockType, detail any) error
	// LeaveBlock signals the end of a block-level element.
	LeaveBlock(t ast.BlockType, detail any) error
	// EnterSpan signals the start of an inline span.
	EnterSpan(t ast.SpanType, detail any) error
	// LeaveSpan signals the end of an inline span.
	LeaveSpan(t ast.SpanType, detail any) error
	// Text delivers textual content. The slice is NOT null-terminated;
	// use len(text). The caller must not retain the slice beyond the call.
	Text(t ast.TextType, text []byte) error
}
