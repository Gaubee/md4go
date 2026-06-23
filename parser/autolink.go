package parser

// autolink.go implements autolink and inline HTML detection.
// Mirrors md4c md_is_autolink() + md_is_autolink_uri() + md_is_autolink_email()
// and the inline HTML detection in md_collect_marks() case '<'.
//
// In md4c, when '<' is encountered in collectMarks:
//   1. Check for raw HTML inline (md_is_html_any) — resolved as HTML text
//   2. Check for autolink (md_is_autolink) — resolved as link with AUTOLINK flag
//   3. Neither — skip (the '<' is literal text)

// isAutolinkURI checks if text[off] starts a URI autolink like <http://...>.
// Returns (end, true) if matched, where end points past the closing '>'.
// Mirrors md4c md_is_autolink_uri() (md4c.c:3129-3164).
func isAutolinkURI(text []byte, off int) (int, bool) {
	size := len(text)
	if off >= size || text[off] != '<' {
		return off, false
	}

	i := off + 1

	// Check for scheme: 1+ alnum followed by [+.-] and then ':'
	// Scheme must be at least 2 chars after '<', max 32 chars from '<'
	if i >= size || !isAlnum(text[i]) {
		return off, false
	}
	i++

	for i < size && i-off <= 32 {
		if text[i] == ':' && i-off >= 3 {
			// Found scheme — break out to check path
			break
		}
		if !isAlnum(text[i]) && text[i] != '+' && text[i] != '-' && text[i] != '.' {
			return off, false
		}
		i++
	}

	// Must have found ':'
	if i >= size || text[i] != ':' || i-off < 3 {
		return off, false
	}
	i++ // skip ':'

	// Check the path after the scheme: no whitespace, control chars, or '<'
	// Mirrors md4c ISCNTRL which includes DEL (0x7F) (md4c.c:349).
	for i < size && text[i] != '>' {
		if isWhitespace(text[i]) || isCtrl(text[i]) || text[i] == '<' {
			return off, false
		}
		i++
	}

	if i >= size {
		return off, false
	}

	// Found '>'
	return i + 1, true
}

// isAutolinkEmail checks if text[off] starts an email autolink like <user@domain>.
// Returns (end, true) if matched, where end points past the closing '>'.
// Mirrors md4c md_is_autolink_email() (md4c.c:3167-3215).
func isAutolinkEmail(text []byte, off int) (int, bool) {
	size := len(text)
	if off >= size || text[off] != '<' {
		return off, false
	}

	i := off + 1

	// Username (before '@'): alnum or .!#$%&'*+/=?^_`{|}~-
	for i < size && (isAlnum(text[i]) || isEmailUserChar(text[i])) {
		i++
	}
	if i <= off+1 {
		return off, false
	}

	// '@'
	if i >= size || text[i] != '@' {
		return off, false
	}
	i++

	// Domain labels delimited with '.'
	// Each label: 1-63 alnum chars or '-', but '-' not allowed as first or last
	labelLen := 0
	for i < size {
		if isAlnum(text[i]) {
			labelLen++
		} else if text[i] == '-' && labelLen > 0 {
			labelLen++
		} else if text[i] == '.' && labelLen > 0 && text[i-1] != '-' {
			labelLen = 0
		} else {
			break
		}
		if labelLen > 63 {
			return off, false
		}
		i++
	}

	// Must end with '>' and last char before it must not be '-'
	if labelLen <= 0 || i >= size || text[i] != '>' || text[i-1] == '-' {
		return off, false
	}

	return i + 1, true
}

// isEmailUserChar returns true if c is allowed in the username part of an email.
func isEmailUserChar(c byte) bool {
	switch c {
	case '.', '!', '#', '$', '%', '&', '\'', '*', '+', '/', '=', '?', '^', '_', '`', '{', '|', '}', '~', '-':
		return true
	}
	return false
}

// autolinkResult holds the result of autolink detection.
type autolinkResult struct {
	end           int  // position after the closing '>'
	missingMailto bool // true for email autolinks (need to prepend "mailto:")
}

// isAutolink checks if text[off] starts an autolink (URI or email).
// Mirrors md4c md_is_autolink() (md4c.c:3217-3231).
func isAutolink(text []byte, off int) (autolinkResult, bool) {
	if end, ok := isAutolinkURI(text, off); ok {
		return autolinkResult{end: end, missingMailto: false}, true
	}
	if end, ok := isAutolinkEmail(text, off); ok {
		return autolinkResult{end: end, missingMailto: true}, true
	}
	return autolinkResult{}, false
}

// isInlineHTML checks if text[off] starts an inline HTML span.
// This is a simplified version of md4c's md_is_html_any() for inline HTML.
// It detects common inline HTML patterns: tags, comments, processing instructions.
// Returns (end, true) if matched.
//
// We handle these cases (mirroring md4c's priority):
//  1. HTML comments: <!-- ... -->
//  2. Processing instructions: <? ... ?>
//  3. CDATA sections: <![CDATA[ ... ]]>
//  4. Opening/closing tags: <tag ...> or </tag ...>
//  5. Self-closing tags: <tag ... />
//
// I36: C-34 — Now accepts *markStacks for HTML horizon tracking.
// Mirrors md4c md_scan_for_html_closer() (md4c.c:1229-1262) which stores
// per-type horizons to skip already-scanned ranges on future calls.
func isInlineHTML(ms *markStacks, text []byte, off int) (int, bool) {
	size := len(text)
	if off >= size || text[off] != '<' {
		return off, false
	}

	if off+3 < size && text[off+1] == '!' && text[off+2] == '-' && text[off+3] == '-' {
		// HTML comment: <!-- ... -->
		return scanForHTMLCloser(ms, text, off, "-->", &ms.htmlCommentHorizon)
	}

	if off+1 < size && text[off+1] == '?' {
		// Processing instruction: <? ... ?>
		return scanForHTMLCloser(ms, text, off, "?>", &ms.htmlProcInstrHorizon)
	}

	// HTML declaration: <!alpha...>
	// Mirrors md4c md_is_html_declaration() (md4c.c:1299-1319).
	// Requires declaration name starting with alpha after '<!'.
	if off+2 < size && text[off+1] == '!' && isAlpha(text[off+2]) {
		return scanForHTMLCloser(ms, text, off, ">", &ms.htmlDeclHorizon)
	}

	// CDATA section: <![CDATA[ ... ]]>
	if off+8 < size && text[off+1] == '!' && text[off+2] == '[' &&
		text[off+3] == 'C' && text[off+4] == 'D' && text[off+5] == 'A' &&
		text[off+6] == 'T' && text[off+7] == 'A' && text[off+8] == '[' {
		return scanForHTMLCloser(ms, text, off, "]]>", &ms.htmlCdataHorizon)
	}

	// HTML tag (opening or closing)
	return isInlineHTMLTag(text, off)
}

// scanForHTMLCloser scans for a closer string in the text starting from off+2.
// Uses horizon tracking to skip already-scanned ranges.
// Mirrors md4c md_scan_for_html_closer() (md4c.c:1229-1262).
func scanForHTMLCloser(ms *markStacks, text []byte, off int, closer string, horizon *int) (int, bool) {
	size := len(text)
	i := off + 2 // skip past opening delimiter (e.g., "<!" for comments)

	// I36: C-34 — Horizon optimization: if we've already scanned past the
	// end of the text without finding a closer, skip.
	// Mirrors md4c.c:1237-1241.
	if i < *horizon && *horizon >= size-len(closer) {
		return off, false
	}

	for i+len(closer) <= size {
		found := true
		for j := 0; j < len(closer); j++ {
			if text[i+j] != closer[j] {
				found = false
				break
			}
		}
		if found {
			return i + len(closer), true
		}
		i++
	}

	// I36: C-34 — Record horizon for future scans.
	// Mirrors md4c.c:1256.
	*horizon = i
	return off, false
}

// isInlineHTMLTag detects <tag ...> or </tag ...> or <tag ... />.
// Uses a proper state machine for attribute validation, mirroring md4c's
// md_is_html_tag() (md4c.c:1111-1229).
func isInlineHTMLTag(text []byte, off int) (int, bool) {
	size := len(text)
	if off >= size || text[off] != '<' {
		return off, false
	}

	i := off + 1 // skip '<'

	if i >= size {
		return off, false
	}

	// Check for closing tag
	isClose := false
	if text[i] == '/' {
		isClose = true
		i++
	}

	// Tag name must start with alpha
	if i >= size || !isAlpha(text[i]) {
		return off, false
	}

	// Tag name: alpha, digit, or '-'
	i++
	for i < size && (isAlphaDigit(text[i]) || text[i] == '-') {
		i++
	}

	if isClose {
		// Closing tags: optional whitespace then '>'
		for i < size && (text[i] == ' ' || text[i] == '\t' || text[i] == '\n') {
			i++
		}
		if i < size && text[i] == '>' {
			return i + 1, true
		}
		return off, false
	}

	// Opening tags: proper attribute state machine
	// Mirrors md4c md_is_html_tag() attr_state logic.
	// State 0: attribute could follow after whitespace
	// State 1: after whitespace (attribute name may follow)
	// State 2: after attribute name ('=' MAY follow)
	// State 3: after '=' (value specification MUST follow)
	// State 41: in unquoted attribute value
	// State 42: in single-quoted attribute value
	// State 43: in double-quoted attribute value
	attrState := 0

	for i < size {
		c := text[i]
		switch attrState {
		case 0:
			// Whitespace → state 1; '/' or '>' → handled
			if c == ' ' || c == '\t' || c == '\n' {
				attrState = 1
				i++
			} else if c == '>' {
				return i + 1, true
			} else if c == '/' {
				if i+1 < size && text[i+1] == '>' {
					return i + 2, true
				}
				return off, false
			} else {
				return off, false
			}
		case 1:
			// Attribute name start
			if c == '>' {
				return i + 1, true
			} else if c == '/' {
				if i+1 < size && text[i+1] == '>' {
					return i + 2, true
				}
				return off, false
			} else if c == ' ' || c == '\t' || c == '\n' {
				i++
			} else if isAlpha(c) || c == '_' || c == ':' {
				attrState = 2
				i++
			} else {
				return off, false
			}
		case 2:
			// Inside attribute name
			if isAlphaDigit(c) || c == '-' || c == '_' || c == '.' || c == ':' {
				i++
			} else if c == ' ' || c == '\t' || c == '\n' {
				// md4c: ISWHITESPACE only transitions state 0→1; state 2 stays.
				i++
			} else if c == '=' {
				attrState = 3
				i++
			} else if c == '>' {
				return i + 1, true
			} else if c == '/' {
				if i+1 < size && text[i+1] == '>' {
					return i + 2, true
				}
				return off, false
			} else {
				return off, false
			}
		case 3:
			// After '=': value must follow
			if c == ' ' || c == '\t' || c == '\n' {
				i++
			} else if c == '\'' {
				attrState = 42
				i++
			} else if c == '"' {
				attrState = 43
				i++
			} else if c == '>' {
				return off, false // value expected
			} else {
				attrState = 41 // unquoted value
				i++
			}
		case 41:
			// Unquoted attribute value
			if c == ' ' || c == '\t' || c == '\n' {
				// md4c: unquoted value ends on whitespace → state 1
				// (state 0→1 via ISWHITESPACE re-inspection, state 41→1 via line advance).
				attrState = 1
				i++
			} else if c == '>' {
				return i + 1, true
			} else if c == '"' || c == '\'' || c == '=' || c == '<' || c == '`' {
				return off, false
			} else {
				i++
			}
		case 42:
			// Single-quoted attribute value: only '\'' closes the quote.
			// All other chars (including '>') are part of the value.
			if c == '\'' {
				attrState = 0
				i++
			} else {
				i++
			}
		case 43:
			// Double-quoted attribute value: only '"' closes the quote.
			// All other chars (including '>') are part of the value.
			if c == '"' {
				attrState = 0
				i++
			} else {
				i++
			}
		default:
			return off, false
		}
	}

	return off, false
}

// isAlnum returns true if c is an ASCII letter or digit.
func isAlnum(c byte) bool {
	return isAlpha(c) || (c >= '0' && c <= '9')
}

// isCtrl returns true if c is an ASCII control character.
// Mirrors md4c ISCNTRL_ macro (md4c.c:349): (unsigned)(ch) <= 31 || (unsigned)(ch) == 127
func isCtrl(c byte) bool {
	return c < 0x20 || c == 0x7F
}

// isAlpha returns true if c is an ASCII letter.
func isAlpha(c byte) bool {
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}

// isAlphaDigit returns true if c is an ASCII letter, digit, or hyphen.
func isAlphaDigit(c byte) bool {
	return isAlpha(c) || (c >= '0' && c <= '9') || c == '-'
}
