package benchmark

/*
#cgo CFLAGS: -I../../md4c/src
#cgo LDFLAGS: -L${SRCDIR}/../md4c/src -lmd4c

#include "md4c-html.h"
#include <string.h>
#include <stdlib.h>

// ─── Inline C null callbacks for md_parse() (zero CGo overhead) ───

static int n_enter_block(MD_BLOCKTYPE t, void* d, void* u) { (void)t;(void)d;(void)u; return 0; }
static int n_leave_block(MD_BLOCKTYPE t, void* d, void* u) { (void)t;(void)d;(void)u; return 0; }
static int n_enter_span(MD_SPANTYPE t, void* d, void* u) { (void)t;(void)d;(void)u; return 0; }
static int n_leave_span(MD_SPANTYPE t, void* d, void* u) { (void)t;(void)d;(void)u; return 0; }
static int n_text(MD_TEXTTYPE t, const MD_CHAR* s, MD_SIZE n, void* u) { (void)t;(void)s;(void)n;(void)u; return 0; }

// ─── C helper: call md_parse() with inline null callbacks ───

static int md_parse_null(const MD_CHAR* input, MD_SIZE size, unsigned flags) {
	MD_PARSER parser = {
		0,          // abi_version
		flags,      // flags
		n_enter_block,
		n_leave_block,
		n_enter_span,
		n_leave_span,
		n_text,
		NULL,       // debug_log
		NULL,       // syntax
	};
	return md_parse(input, size, &parser, NULL);
}

// ─── C helper: call md_html() with Go process_output callback ───

// NOTE: goProcessOutput takes non-const MD_CHAR* because Go //export cannot express const.
// The resulting -Wincompatible-pointer-types warning from md_html() is harmless —
// the Go side never writes to text.
extern void goProcessOutput(MD_CHAR* text, MD_SIZE size, void* userdata);

static int md_html_go(const MD_CHAR* input, MD_SIZE size,
                       unsigned parser_flags, unsigned renderer_flags) {
	return md_html(input, size, goProcessOutput, NULL, parser_flags, renderer_flags);
}
*/
import "C"

import (
	"bytes"
	"unsafe"
)

// ─── Package-level buffer for md4c HTML output ───
// Benchmarks run sequentially; no concurrent access.

var md4cBuf bytes.Buffer

//export goProcessOutput
func goProcessOutput(text *C.char, size C.uint, _ unsafe.Pointer) {
	md4cBuf.Write(C.GoBytes(unsafe.Pointer(text), C.int(size)))
}

// ─── Public API ───

// Md4cConvertHTML parses Markdown and returns HTML5 via md4c's md_html().
// parserFlags: MD_DIALECT_COMMONMARK (0) or MD_DIALECT_GITHUB.
// rendererFlags: 0 for HTML5 (no XHTML).
func Md4cConvertHTML(input []byte, parserFlags, rendererFlags uint) string {
	if len(input) == 0 {
		return ""
	}
	md4cBuf.Reset()
	C.md_html_go(
		(*C.char)(unsafe.Pointer(&input[0])),
		C.uint(len(input)),
		C.uint(parserFlags),
		C.uint(rendererFlags),
	)
	md4cBuf.WriteByte('\n')
	return md4cBuf.String()
}

// Md4cParseNull parses Markdown with inline-C null callbacks.
// Zero CGo boundary crosses per block/span/text event — pure C speed.
func Md4cParseNull(input []byte, parserFlags uint) {
	if len(input) == 0 {
		return
	}
	C.md_parse_null(
		(*C.char)(unsafe.Pointer(&input[0])),
		C.uint(len(input)),
		C.uint(parserFlags),
	)
}
