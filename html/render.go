package html

import (
	"bytes"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/userpro/md4go/ast"
	"github.com/userpro/md4go/renderer"
)

// HTML is the HTML renderer — used for spec compliance validation.
// It mirrors md4c-html.c's output tag mapping.
// I17: Full entity translation, URL percent-encoding, tight lists, missing spans.
// I37: List content buffering for correct tight/loose determination.
type HTML struct {
	w          *renderer.BufWriter
	escape     [256]bool // text content escape table
	escapeAttr [256]bool // attribute value escape table (always includes ")
	urlEsc     [256]bool // URL percent-encoding map
	imgDepth   int
	xhtml      bool
	flags      RenderFlags

	// I37: List content buffering for tight/loose detection.
	listBufs []*bytes.Buffer

	// imgDetail holds the image detail for the current image span.
	imgDetail *ast.ImgDetail
}

// out returns the current write target: the top list buffer if buffering,
// or the main BufWriter otherwise.
func (h *HTML) out() *bytes.Buffer {
	if len(h.listBufs) > 0 {
		return h.listBufs[len(h.listBufs)-1]
	}
	return nil
}

// emitStr writes a string to the current output (list buffer or main writer).
func (h *HTML) emitStr(s string) {
	if buf := h.out(); buf != nil {
		buf.WriteString(s)
	} else {
		_ = h.w.WriteString(s)
	}
}

// emitBytes writes bytes to the current output.
func (h *HTML) emitBytes(b []byte) {
	if buf := h.out(); buf != nil {
		buf.Write(b)
	} else {
		_, _ = h.w.Write(b)
	}
}

// emitByte writes a single byte to the current output.
func (h *HTML) emitByte(c byte) {
	if buf := h.out(); buf != nil {
		buf.WriteByte(c)
	} else {
		_ = h.w.WriteByte(c)
	}
}

// pushListBuffer starts buffering list content.
func (h *HTML) pushListBuffer() {
	h.listBufs = append(h.listBufs, &bytes.Buffer{})
}

// popListBuffer removes the top buffer, strips <p> tags if tight,
// and writes the result to the parent buffer or main writer.
func (h *HTML) popListBuffer(isTight bool) {
	buf := h.listBufs[len(h.listBufs)-1]
	h.listBufs = h.listBufs[:len(h.listBufs)-1]

	content := buf.String()
	if isTight {
		content = stripPTagsInLI(content)
	}

	if parent := h.out(); parent != nil {
		parent.WriteString(content)
	} else {
		_ = h.w.WriteString(content)
	}
}

// stripPTagsInLI removes <p> and </p> tags that are direct children of <li> tags,
// converting loose list output to tight list output.
// For tight lists, CommonMark expects <li>foo</li> not <li><p>foo</p></li>.
//
// I37: The content includes the current list's <ul>/<ol> wrapper tags.
// We track listNestDepth:
//   - 0: before entering the current list's <ul>/<ol>
//   - 1: inside the current list — strip <p> from <li> at blockDepth 0
//   - 2+: inside a nested list — don't strip <p> (nested list already resolved)
func stripPTagsInLI(content string) string {
	var buf strings.Builder
	buf.Grow(len(content))
	i := 0
	n := len(content)
	inLI := false
	blockDepth := 0    // block nesting depth inside current <li>
	listNestDepth := 0 // 0=outside list, 1=current list, 2+=nested list

	for i < n {
		// Track <ul>/<ol> nesting
		if content[i] == '<' {
			if strings.HasPrefix(content[i:], "<ul") || strings.HasPrefix(content[i:], "<ol") {
				listNestDepth++
				buf.WriteByte(content[i])
				i++
				continue
			}
			if strings.HasPrefix(content[i:], "</ul>") || strings.HasPrefix(content[i:], "</ol>") {
				if listNestDepth == 1 {
					inLI = false // leaving current list
				}
				listNestDepth--
				if listNestDepth < 0 {
					listNestDepth = 0
				}
				buf.WriteByte(content[i])
				i++
				continue
			}
		}

		// Check for <li (start of list item) — only process in current list (depth 1)
		if listNestDepth == 1 && i+3 < n && content[i] == '<' && content[i+1] == 'l' && content[i+2] == 'i' && (content[i+3] == '>' || content[i+3] == ' ') {
			inLI = true
			blockDepth = 0
			// Copy everything up to and including the closing >
			end := strings.IndexByte(content[i:], '>')
			if end < 0 {
				buf.WriteByte(content[i])
				i++
				continue
			}
			buf.WriteString(content[i : i+end+1])
			i += end + 1
			continue
		}

		// Check for </li> — only process in current list (depth 1)
		if listNestDepth == 1 && i+4 < n && content[i] == '<' && content[i+1] == '/' && content[i+2] == 'l' && content[i+3] == 'i' && content[i+4] == '>' {
			inLI = false
			buf.WriteString("</li>")
			i += 5
			continue
		}

		// Strip <p> only in current list (depth 1), at blockDepth 0 inside <li>
		if inLI && blockDepth == 0 && listNestDepth == 1 && i+2 < n && content[i] == '<' && content[i+1] == 'p' && content[i+2] == '>' {
			i += 3 // skip <p>
			continue
		}

		// Strip </p> only in current list (depth 1), at blockDepth 0 inside <li>
		if inLI && blockDepth == 0 && listNestDepth == 1 && i+3 < n && content[i] == '<' && content[i+1] == '/' && content[i+2] == 'p' && content[i+3] == '>' {
			i += 4 // skip </p>
			// Also skip optional trailing newline
			if i < n && content[i] == '\n' {
				i++
			}
			continue
		}

		// Track block nesting: opening block tags increase blockDepth
		// (only in current list, depth 1)
		if inLI && content[i] == '<' && listNestDepth == 1 {
			if startsWithBlockTag(content, i+1) {
				blockDepth++
			} else if i+1 < n && content[i+1] == '/' && startsWithBlockTag(content, i+2) {
				blockDepth--
				if blockDepth < 0 {
					blockDepth = 0
				}
			}
		}

		buf.WriteByte(content[i])
		i++
	}

	return buf.String()
}

// startsWithBlockTag checks if content at position i starts with a block-level tag name.
func startsWithBlockTag(content string, i int) bool {
	tags := []string{"ul", "ol", "blockquote", "pre", "div", "dl", "table", "section"}
	for _, tag := range tags {
		if strings.HasPrefix(content[i:], tag) {
			// Make sure it's a tag name boundary (followed by > or space)
			end := i + len(tag)
			if end < len(content) && (content[end] == '>' || content[end] == ' ') {
				return true
			}
		}
	}
	return false
}

// newHTMLWithWriter creates an HTML renderer with an existing BufWriter.
func NewHTMLWithWriter(w *renderer.BufWriter) *HTML {
	h := &HTML{w: w}
	h.initEscapeMaps()
	return h
}

// initEscapeMaps precomputes the HTML and URL escape character tables.
func (h *HTML) initEscapeMaps() {
	if h.flags&FlagNoXHTMLEscaping != 0 {
		// goldmark-compatible: text content escapes only & < >
		for _, c := range []byte("&<>") {
			h.escape[c] = true
		}
		// Attribute values also escape " (needed for "-delimited attrs)
		for _, c := range []byte("\"&<>") {
			h.escapeAttr[c] = true
		}
	} else {
		// Default: XHTML-safe — escapes " & ' < > (matches md4c-html.c)
		for _, c := range []byte("\"&'<>") {
			h.escape[c] = true
			h.escapeAttr[c] = true
		}
	}

	// URL percent-encoding: non-alphanumeric chars not in safe set.
	safeURL := "~-_.+!*(),%#@?=;:/$"
	for i := 0; i < 256; i++ {
		c := byte(i)
		if isAlnum(c) {
			continue
		}
		isSafe := false
		for j := 0; j < len(safeURL); j++ {
			if c == safeURL[j] {
				isSafe = true
				break
			}
		}
		if !isSafe {
			h.urlEsc[c] = true
		}
	}
}

func isAlnum(c byte) bool {
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9')
}

// Flush flushes buffered output.
func (h *HTML) Flush() error {
	return h.w.Flush()
}

// EnterBlock handles block-level enter events.
// Maps directly to md4c-html.c enter_block_callback.
// I37: UL/OL start buffering; P inside lists always emits <p>.
func (h *HTML) EnterBlock(b ast.BlockType, detail any) error {
	switch b {
	case ast.BlockDoc:
		// no-op
	case ast.BlockQuote:
		h.emitStr("<blockquote>\n")
	case ast.BlockUL:
		h.pushListBuffer()
		h.emitStr("<ul>\n")
	case ast.BlockOL:
		h.pushListBuffer()
		h.renderOpenOL(detail)
	case ast.BlockLI:
		h.renderOpenLI(detail)
	case ast.BlockHR:
		if h.xhtml {
			h.emitStr("<hr />\n")
		} else {
			h.emitStr("<hr>\n")
		}
	case ast.BlockH:
		h.renderOpenH(detail)
	case ast.BlockCode:
		h.renderOpenCode(detail)
	case ast.BlockHTML:
		// no-op
	case ast.BlockP:
		// I37: Always emit <p> tags. Tight list <p> stripping is handled
		// by popListBuffer when the list ends.
		h.emitStr("<p>")
	case ast.BlockTable:
		h.emitStr("<table>\n")
	case ast.BlockTHead:
		h.emitStr("<thead>\n")
	case ast.BlockTBody:
		h.emitStr("<tbody>\n")
	case ast.BlockTR:
		h.emitStr("<tr>\n")
	case ast.BlockTH:
		h.renderOpenTD("th", detail)
	case ast.BlockTD:
		h.renderOpenTD("td", detail)
	case ast.BlockFootnoteDefSection:
		h.emitStr("<section class=\"footnotes\">\n<ol>\n")
	case ast.BlockFootnoteDef:
		h.renderOpenFootnoteDef(detail)
	case ast.BlockAdmonition:
		h.renderOpenAdmonition(detail)
	}
	return nil
}

// LeaveBlock handles block-level leave events.
// I37: UL/OL pop buffer with tight/loose processing.
func (h *HTML) LeaveBlock(b ast.BlockType, detail any) error {
	switch b {
	case ast.BlockDoc:
		// no-op
	case ast.BlockQuote:
		h.emitStr("</blockquote>\n")
	case ast.BlockUL:
		h.emitStr("</ul>\n")
		isTight := false
		if d, ok := detail.(*ast.ULDetail); ok {
			isTight = d.IsTight
		}
		h.popListBuffer(isTight)
	case ast.BlockOL:
		h.emitStr("</ol>\n")
		isTight := false
		if d, ok := detail.(*ast.OLDetail); ok {
			isTight = d.IsTight
		}
		h.popListBuffer(isTight)
	case ast.BlockLI:
		h.emitStr("</li>\n")
	case ast.BlockHR:
		// no-op
	case ast.BlockH:
		h.renderCloseH(detail)
	case ast.BlockCode:
		h.emitStr("</code></pre>\n")
	case ast.BlockHTML:
		// no-op
	case ast.BlockP:
		// I37: Always emit </p>. Tight list stripping is handled at list end.
		h.emitStr("</p>\n")
	case ast.BlockTable:
		h.emitStr("</table>\n")
	case ast.BlockTHead:
		h.emitStr("</thead>\n")
	case ast.BlockTBody:
		h.emitStr("</tbody>\n")
	case ast.BlockTR:
		h.emitStr("</tr>\n")
	case ast.BlockTH:
		h.emitStr("</th>\n")
	case ast.BlockTD:
		h.emitStr("</td>\n")
	case ast.BlockFootnoteDefSection:
		h.emitStr("</ol>\n</section>\n")
	case ast.BlockFootnoteDef:
		h.renderCloseFootnoteDef(detail)
	case ast.BlockAdmonition:
		h.emitStr("</div>\n")
	}
	return nil
}

// EnterSpan — inline span tags, mirroring md4c-html.c enter_span_callback.
func (h *HTML) EnterSpan(s ast.SpanType, detail any) error {
	// When inside an image span, suppress ALL span tag output.
	// Mirrors md4c-html.c enter_span_callback: "if(r->image_nesting_level > 0) return 0;"
	// Note: md4c blocks all spans including nested IMG; imgDepth is incremented
	// inside the SpanImg case below.
	if h.imgDepth > 0 {
		if s == ast.SpanImg {
			h.imgDepth++
		}
		return nil
	}

	switch s {
	case ast.SpanEm:
		h.emitStr("<em>")
	case ast.SpanStrong:
		h.emitStr("<strong>")
	case ast.SpanCode:
		h.emitStr("<code>")
	case ast.SpanDel:
		h.emitStr("<del>")
	case ast.SpanLink:
		h.renderOpenLink(detail)
	case ast.SpanImg:
		h.renderOpenImg(detail)
	case ast.SpanSuperscript:
		h.emitStr("<sup>")
	case ast.SpanSubscript:
		h.emitStr("<sub>")
	case ast.SpanLatexMath:
		h.emitStr("<x-equation>")
	case ast.SpanLatexMathDisplay:
		h.emitStr(`<x-equation type="display">`)
	case ast.SpanSpoiler:
		h.emitStr("<x-spoiler>")
	case ast.SpanMark:
		h.emitStr("<mark>")
	case ast.SpanU:
		h.emitStr("<u>")
	case ast.SpanWikilink:
		h.renderOpenWikilink(detail)
	case ast.SpanFootnoteRef:
		h.renderOpenFootnoteRef(detail)
	}
	return nil
}

// LeaveSpan — inline span closing tags.
func (h *HTML) LeaveSpan(s ast.SpanType, detail any) error {
	// When inside an image span, suppress all span tag output except
	// for the image itself (which may close the outermost image).
	if h.imgDepth > 0 && s != ast.SpanImg {
		return nil
	}

	switch s {
	case ast.SpanEm:
		h.emitStr("</em>")
	case ast.SpanStrong:
		h.emitStr("</strong>")
	case ast.SpanCode:
		h.emitStr("</code>")
	case ast.SpanDel:
		h.emitStr("</del>")
	case ast.SpanLink:
		h.emitStr("</a>")
	case ast.SpanImg:
		h.imgDepth--
		if h.imgDepth > 0 {
			return nil
		}
		// First-level IMG close: output title + closing.
		// Mirrors md4c-html.c leave_span_callback IMG branch.
		if h.imgDetail != nil && h.imgDetail.Title.Text != nil {
			h.emitStr("\" title=\"")
			h.renderAttribute(h.imgDetail.Title, h.writeEscapedBytes)
		}
		if h.xhtml {
			h.emitStr("\" />")
		} else {
			h.emitStr("\">")
		}
		h.imgDetail = nil
	case ast.SpanSuperscript:
		h.emitStr("</sup>")
	case ast.SpanSubscript:
		h.emitStr("</sub>")
	case ast.SpanLatexMath, ast.SpanLatexMathDisplay:
		h.emitStr("</x-equation>")
	case ast.SpanSpoiler:
		h.emitStr("</x-spoiler>")
	case ast.SpanMark:
		h.emitStr("</mark>")
	case ast.SpanU:
		h.emitStr("</u>")
	case ast.SpanWikilink:
		h.emitStr("</x-wikilink>")
	case ast.SpanFootnoteRef:
		// noop: enter_span already emitted full HTML
	}
	return nil
}

// Text writes text content, HTML-escaping where needed.
// Mirrors md4c-html.c text_callback.
// I47: Streaming alt rendering — when inside an image, text goes through the
// same rendering pipeline as normal text (writeEscaped/renderEntity), matching
// md4c's text_callback which does NOT check inside_img for text rendering.
func (h *HTML) Text(t ast.TextType, text []byte) error {
	// If inside an image span, render text directly (streaming approach).
	// md4c outputs text between alt=" and " directly through render_html_escaped
	// or render_verbatim, not into a buffer. This matches that behavior.
	if h.imgDepth > 0 {
		switch t {
		case ast.TextNullChar:
			h.emitStr("\uFFFD")
		case ast.TextBR, ast.TextSoftBR:
			// Mirrors md4c-html.c:575-579: inside image, BR/SOFTBR → " "
			h.emitStr(" ")
		case ast.TextHTML:
			// Mirrors md4c-html.c:580: render_verbatim (raw output, no escaping)
			h.emitBytes(text)
		case ast.TextCode:
			// Code span text inside alt: HTML-escaped (mirrors md4c render_html_escaped)
			_ = h.writeEscaped(text)
		case ast.TextEntity:
			// Mirrors md4c-html.c:581: render_entity with render_html_escaped
			_ = h.renderEntity(text)
		case ast.TextLatexMath:
			// Mirrors md4c-html.c: MD_TEXT_LATEXMATH→render_html_escaped (even inside image)
			_ = h.writeEscaped(text)
		default:
			// TextNormal and others: HTML-escaped (mirrors md4c render_html_escaped)
			_ = h.writeEscaped(text)
		}
		return nil
	}

	switch t {
	case ast.TextNullChar:
		h.emitStr("\uFFFD")
	case ast.TextBR:
		if h.xhtml {
			h.emitStr("<br />\n")
		} else {
			h.emitStr("<br>\n")
		}
	case ast.TextSoftBR:
		h.emitByte('\n')
	case ast.TextHTML:
		h.emitBytes(text)
	case ast.TextCode:
		return h.writeEscaped(text)
	case ast.TextLatexMath:
		// Mirrors md4c-html.c: MD_TEXT_LATEXMATH falls through to default→render_html_escaped.
		// LaTeX math content must be HTML-escaped, not verbatim, to produce valid HTML.
		return h.writeEscaped(text)
	case ast.TextEntity:
		return h.renderEntity(text)
	default:
		return h.writeEscaped(text)
	}
	return nil
}

// --- Entity Translation (mirrors md4c-html.c render_entity + render_utf8_codepoint) ---

// renderEntity translates an HTML entity to UTF-8, then passes through HTML escaping.
// I36: C-40 — When FlagVerbatimEntities is set, outputs the raw entity text
// verbatim (no HTML escaping). Mirrors md4c-html.c:211-213:
//
//	if(flags & MD_HTML_FLAG_VERBATIM_ENTITIES) { render_verbatim(r, text, size); return; }
func (h *HTML) renderEntity(text []byte) error {
	if h.flags&FlagVerbatimEntities != 0 {
		// Mirrors md4c-html.c:211-213 render_verbatim.
		// Use emitBytes to go through list buffer when inside a list.
		h.emitBytes(text)
		return nil
	}

	if len(text) > 3 && text[1] == '#' {
		codepoint := h.parseNumericEntity(text)
		if codepoint != 0xFFFD || h.isValidNumericEntity(text) {
			return h.writeUTF8CodepointEscaped(codepoint)
		}
		return h.writeEscaped(text)
	}

	if e, ok := entityLookup(string(text)); ok {
		if err := h.writeUTF8CodepointEscaped(e.codepoints[0]); err != nil {
			return err
		}
		if e.codepoints[1] != 0 {
			return h.writeUTF8CodepointEscaped(e.codepoints[1])
		}
		return nil
	}

	return h.writeEscaped(text)
}

func (h *HTML) parseNumericEntity(text []byte) rune {
	if len(text) < 4 || text[1] != '#' {
		return 0xFFFD
	}

	var codepoint uint64
	var err error
	if text[2] == 'x' || text[2] == 'X' {
		codepoint, err = strconv.ParseUint(string(text[3:len(text)-1]), 16, 32)
	} else {
		codepoint, err = strconv.ParseUint(string(text[2:len(text)-1]), 10, 32)
	}

	if err != nil {
		return 0xFFFD
	}

	if codepoint == 0 || codepoint > 0x10FFFF || (codepoint >= 0xD800 && codepoint <= 0xDFFF) {
		return 0xFFFD
	}
	return rune(codepoint)
}

func (h *HTML) isValidNumericEntity(text []byte) bool {
	if len(text) < 4 || text[1] != '#' || text[len(text)-1] != ';' {
		return false
	}

	if text[2] == 'x' || text[2] == 'X' {
		if len(text) < 5 {
			return false
		}
		for _, c := range text[3 : len(text)-1] {
			if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')) {
				return false
			}
		}
		return len(text) > 4
	}

	for _, c := range text[2 : len(text)-1] {
		if c < '0' || c > '9' {
			return false
		}
	}
	return len(text) > 3
}

func (h *HTML) writeUTF8Codepoint(cp rune) error {
	var buf [4]byte
	n := encodeUTF8(cp, buf[:])
	if n == 0 {
		h.emitStr("\uFFFD")
		return nil
	}
	h.emitBytes(buf[:n])
	return nil
}

func (h *HTML) writeUTF8CodepointEscaped(cp rune) error {
	var buf [4]byte
	n := encodeUTF8(cp, buf[:])
	if n == 0 {
		h.emitStr("\uFFFD")
		return nil
	}
	return h.writeEscaped(buf[:n])
}

func encodeUTF8(cp rune, dst []byte) int {
	if cp == 0 {
		// Mirrors md4c render_utf8_codepoint: codepoint 0 → U+FFFD (not NULL byte).
		// md4c-html.c:198 checks "0 < codepoint" and falls to the replacement char.
		return 0
	} else if cp <= 0x7F {
		dst[0] = byte(cp)
		return 1
	} else if cp <= 0x7FF {
		dst[0] = 0xC0 | byte((cp>>6)&0x1F)
		dst[1] = 0x80 | byte(cp&0x3F)
		return 2
	} else if cp <= 0xFFFF {
		dst[0] = 0xE0 | byte((cp>>12)&0xF)
		dst[1] = 0x80 | byte((cp>>6)&0x3F)
		dst[2] = 0x80 | byte(cp&0x3F)
		return 3
	} else if cp <= 0x10FFFF {
		dst[0] = 0xF0 | byte((cp>>18)&0x7)
		dst[1] = 0x80 | byte((cp>>12)&0x3F)
		dst[2] = 0x80 | byte((cp>>6)&0x3F)
		dst[3] = 0x80 | byte(cp&0x3F)
		return 4
	}
	return 0
}

// --- Attribute rendering (mirrors md4c-html.c render_attribute) ---

func (h *HTML) renderAttribute(attr ast.Attribute, fn func([]byte)) {
	for i := 0; i < len(attr.SubTypes); i++ {
		if i+1 >= len(attr.SubOffsets) {
			break
		}
		if attr.SubOffsets[i] >= len(attr.Text) {
			break
		}
		off := attr.SubOffsets[i]
		end := attr.SubOffsets[i+1]
		if end > len(attr.Text) {
			end = len(attr.Text)
		}
		size := end - off
		if size <= 0 {
			continue
		}
		text := attr.Text[off:end]

		switch attr.SubTypes[i] {
		case ast.SubstrNullChar:
			// Mirrors md4c-html.c:263: render_utf8_codepoint(r, 0x0000, render_verbatim)
			// outputs U+FFFD replacement character for NULL in attributes.
			fn([]byte("\uFFFD"))
		case ast.SubstrEntity:
			h.renderEntityToFn(text, fn)
		default:
			fn(text)
		}
	}
}

// renderEntityToFn translates an HTML entity and passes the result through fn.
// I36: C-40 — When FlagVerbatimEntities is set, outputs verbatim via
// emitBytes (mirrors md4c render_verbatim, NOT fn_append which would escape).
func (h *HTML) renderEntityToFn(text []byte, fn func([]byte)) {
	if h.flags&FlagVerbatimEntities != 0 {
		h.emitBytes(text)
		return
	}

	if len(text) > 3 && text[1] == '#' {
		codepoint := h.parseNumericEntity(text)
		if codepoint != 0xFFFD || h.isValidNumericEntity(text) {
			var buf [4]byte
			n := encodeUTF8(codepoint, buf[:])
			if n > 0 {
				fn(buf[:n])
			} else {
				fn([]byte("\uFFFD"))
			}
			return
		}
		fn(text)
		return
	}

	if e, ok := entityLookup(string(text)); ok {
		var buf [4]byte
		n := encodeUTF8(e.codepoints[0], buf[:])
		if n > 0 {
			fn(buf[:n])
		}
		if e.codepoints[1] != 0 {
			n = encodeUTF8(e.codepoints[1], buf[:])
			if n > 0 {
				fn(buf[:n])
			}
		}
		return
	}

	fn(text)
}

// --- URL Escaping (mirrors md4c-html.c render_url_escaped) ---

func (h *HTML) writeURLEscaped(url []byte) {
	beg := 0
	for i, c := range url {
		if h.urlEsc[c] {
			if i > beg {
				h.emitBytes(url[beg:i])
			}
			if c == '&' {
				h.emitStr("&amp;")
			} else {
				const hex = "0123456789ABCDEF"
				h.emitByte('%')
				h.emitByte(hex[c>>4])
				h.emitByte(hex[c&0xF])
			}
			beg = i + 1
		}
	}
	if beg < len(url) {
		h.emitBytes(url[beg:])
	}
}

// --- HTML Escaping ---

// writeEscapedWith escapes text using the given escape table.
// Characters in the table are encoded as HTML entities.
func (h *HTML) writeEscapedWith(text []byte, table *[256]bool) error {
	beg := 0
	off := 0
	size := len(text)

	for {
		// Fast skip: 4 characters at a time (loop unrolling).
		for off+3 < size &&
			!table[text[off]] &&
			!table[text[off+1]] &&
			!table[text[off+2]] &&
			!table[text[off+3]] {
			off += 4
		}
		for off < size && !table[text[off]] {
			off++
		}

		if off > beg {
			h.emitBytes(text[beg:off])
		}

		if off < size {
			switch text[off] {
			case '"':
				h.emitStr("&quot;")
			case '&':
				h.emitStr("&amp;")
			case '\'':
				h.emitStr("&#x27;")
			case '<':
				h.emitStr("&lt;")
			case '>':
				h.emitStr("&gt;")
			}
			off++
		} else {
			break
		}
		beg = off
	}
	return nil
}

// writeEscaped escapes text for HTML text content.
func (h *HTML) writeEscaped(text []byte) error {
	return h.writeEscapedWith(text, &h.escape)
}

// --- Block rendering helpers ---

func (h *HTML) renderOpenH(detail any) {
	if d, ok := detail.(*ast.HeadingDetail); ok {
		h.emitStr(fmt.Sprintf("<h%d>", d.Level))
	} else {
		h.emitStr("<h1>")
	}
}

func (h *HTML) renderCloseH(detail any) {
	if d, ok := detail.(*ast.HeadingDetail); ok {
		h.emitStr(fmt.Sprintf("</h%d>\n", d.Level))
	} else {
		h.emitStr("</h1>\n")
	}
}

func (h *HTML) renderOpenOL(detail any) {
	if d, ok := detail.(*ast.OLDetail); ok && d.Start != 1 {
		h.emitStr(fmt.Sprintf("<ol start=\"%d\">\n", d.Start))
	} else {
		h.emitStr("<ol>\n")
	}
}

func (h *HTML) renderOpenLI(detail any) {
	if d, ok := detail.(*ast.LIDetail); ok && d.IsTask {
		// Align with md4c-html.c:322-324.
		// XHTML mode: disabled="true" checked="true" />
		// HTML5 mode: disabled checked>
		h.emitStr("<li class=\"task-list-item\"><input type=\"checkbox\" class=\"task-list-item-checkbox\"")
		if h.xhtml {
			h.emitStr(" disabled=\"true\"")
			if d.TaskMark == 'x' || d.TaskMark == 'X' {
				h.emitStr(" checked=\"true\"")
			}
			h.emitStr(" />")
		} else {
			h.emitStr(" disabled")
			if d.TaskMark == 'x' || d.TaskMark == 'X' {
				h.emitStr(" checked")
			}
			h.emitByte('>')
		}
		return
	}
	h.emitStr("<li>")
}

func (h *HTML) renderOpenCode(detail any) {
	h.emitStr("<pre><code")
	if d, ok := detail.(*ast.CodeDetail); ok && d.Lang.Text != nil {
		h.emitStr(" class=\"")
		langText := d.Lang.Text
		if len(langText) < 9 || string(langText[:9]) != "language-" {
			h.emitStr("language-")
		}
		h.renderAttribute(d.Lang, h.writeEscapedBytes)
		h.emitStr("\"")
	}
	h.emitByte('>')
}

func (h *HTML) renderOpenTD(cellType string, detail any) {
	h.emitByte('<')
	h.emitStr(cellType)
	if d, ok := detail.(*ast.TDDetail); ok {
		switch d.Align {
		case ast.AlignLeft:
			h.emitStr(" align=\"left\"")
		case ast.AlignCenter:
			h.emitStr(" align=\"center\"")
		case ast.AlignRight:
			h.emitStr(" align=\"right\"")
		}
	}
	h.emitByte('>')
}

func (h *HTML) renderOpenAdmonition(detail any) {
	h.emitStr("<div class=\"admonition-")
	if d, ok := detail.(*ast.AdmonitionDetail); ok {
		h.renderAttribute(d.Type, h.writeEscapedBytes)
	}
	h.emitStr("\"><p class=\"admonition-title\">")
	if d, ok := detail.(*ast.AdmonitionDetail); ok {
		h.renderAttribute(d.Type, h.writeEscapedBytes)
	}
	h.emitStr("</p>")
}

func (h *HTML) renderOpenFootnoteDef(detail any) {
	if d, ok := detail.(*ast.FootnoteDefDetail); ok {
		h.emitStr(fmt.Sprintf("<li id=\"fn-%d\">\n", d.ID))
	} else {
		h.emitStr("<li>\n")
	}
}

func (h *HTML) renderCloseFootnoteDef(detail any) {
	if d, ok := detail.(*ast.FootnoteDefDetail); ok {
		for refIndex := uint(1); refIndex <= d.RefCount; refIndex++ {
			if refIndex > 1 {
				h.emitStr(" ")
			}
			h.emitStr(fmt.Sprintf("<a href=\"#fnref-%d-%d\" class=\"footnote-backref\">&#8617;</a>", d.ID, refIndex))
		}
	}
	h.emitStr("\n</li>\n")
}

// WriteTo returns the HTML renderer's underlying writer for direct writes.
func (h *HTML) WriteTo(w io.Writer) (int64, error) {
	return 0, nil
}

// --- Link/Span rendering helpers ---

func (h *HTML) renderOpenLink(detail any) {
	h.emitStr("<a href=\"")
	if d, ok := detail.(*ast.LinkDetail); ok {
		h.renderAttribute(d.Href, h.writeURLEscaped)
		if d.Title.Text != nil {
			h.emitStr("\" title=\"")
			h.renderAttribute(d.Title, h.writeEscapedBytes)
		}
	}
	h.emitStr("\">")
}

func (h *HTML) renderOpenImg(detail any) {
	h.imgDepth++
	if h.imgDepth == 1 {
		h.emitStr("<img src=\"")
		if d, ok := detail.(*ast.ImgDetail); ok {
			h.renderAttribute(d.Src, h.writeURLEscaped)
			h.emitStr("\" alt=\"")
			// Store detail for title output in LeaveSpan
			h.imgDetail = d
		} else {
			h.imgDetail = &ast.ImgDetail{}
			h.emitStr("\" alt=\"")
		}
		// Alt text is now rendered directly by Text() between Enter and Leave
		// (streaming approach, matching md4c's text_callback behavior).
		// No buffering needed — imgAltBuf is no longer used.
	}
}

func (h *HTML) writeEscapedBytes(text []byte) {
	_ = h.writeEscapedWith(text, &h.escapeAttr)
}

func (h *HTML) renderOpenWikilink(detail any) {
	h.emitStr("<x-wikilink data-target=\"")
	if d, ok := detail.(*ast.WikilinkDetail); ok {
		h.renderAttribute(d.Target, h.writeEscapedBytes)
	}
	h.emitStr("\">")
}

func (h *HTML) renderOpenFootnoteRef(detail any) {
	if d, ok := detail.(*ast.FootnoteRefDetail); ok {
		h.emitStr(fmt.Sprintf("<sup><a href=\"#fn-%d\" id=\"fnref-%d-%d\">%d</a></sup>",
			d.ID, d.ID, d.RefID, d.ID))
	} else {
		h.emitStr("<sup><a href=\"#fn-1\" id=\"fnref-1-1\">1</a></sup>")
	}
}
