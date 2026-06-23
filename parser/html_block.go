package parser

import "bytes"

// HTML block types per CommonMark §4.6, mirroring md4c's 7-type system.
// Types 1-5 have specific end conditions; types 6-7 end at blank lines.
const (
	htmlBlockNone uint8 = 0
	// Type 1: <script, <pre, <style, <textarea — ends with closing tag.
	htmlBlockType1 uint8 = 1
	// Type 2: <!-- — ends with -->.
	htmlBlockType2 uint8 = 2
	// Type 3: <? — ends with ?>.
	htmlBlockType3 uint8 = 3
	// Type 4: <! followed by A-Z — ends with >.
	htmlBlockType4 uint8 = 4
	// Type 5: <![CDATA[ — ends with ]]>.
	htmlBlockType5 uint8 = 5
	// Type 6: <tag or </tag where tag is a known block-level element — ends at blank line.
	htmlBlockType6 uint8 = 6
	// Type 7: any other < tag at start of line (with conditions) — ends at blank line.
	htmlBlockType7 uint8 = 7
)

// htmlBlockType1Tags are tags that end with their closing counterpart.
var htmlBlockType1Tags = [][]byte{
	[]byte("pre"),
	[]byte("script"),
	[]byte("style"),
	[]byte("textarea"),
}

// htmlBlockType6Tags are block-level HTML elements for type 6/7 detection.
// Mirrors md4c's t6[] and t7[] tag tables.
var htmlBlockType6Tags = [][]byte{
	[]byte("address"), []byte("article"), []byte("aside"),
	[]byte("base"), []byte("basefont"), []byte("blockquote"), []byte("body"),
	[]byte("caption"), []byte("center"), []byte("col"), []byte("colgroup"),
	[]byte("dd"), []byte("details"), []byte("dialog"), []byte("dir"),
	[]byte("div"), []byte("dl"), []byte("dt"),
	[]byte("fieldset"), []byte("figcaption"), []byte("figure"), []byte("footer"),
	[]byte("form"),
	[]byte("frame"), []byte("frameset"),
	[]byte("h1"), []byte("h2"), []byte("h3"), []byte("h4"), []byte("h5"), []byte("h6"),
	[]byte("head"), []byte("header"), []byte("hr"), []byte("html"),
	[]byte("iframe"),
	[]byte("legend"), []byte("li"), []byte("link"),
	[]byte("main"), []byte("menu"), []byte("menuitem"),
	[]byte("nav"), []byte("noframes"),
	[]byte("ol"), []byte("optgroup"), []byte("option"),
	[]byte("p"), []byte("param"), []byte("section"), []byte("source"), []byte("summary"),
	[]byte("table"), []byte("tbody"), []byte("td"), []byte("tfoot"),
	[]byte("th"), []byte("thead"), []byte("title"), []byte("tr"), []byte("track"),
	[]byte("ul"),
}

// detectHTMLBlockStart checks if line starts an HTML block.
// Returns the HTML block type (1-7) and true if it does.
// Mirrors md4c md_is_html_block_start_condition().
func detectHTMLBlockStart(line []byte) (uint8, bool) {
	if len(line) == 0 || line[0] != '<' {
		return 0, false
	}
	off := 1

	// Type 1: <pre, <script, <style, <textarea
	if off < len(line) {
		if matchTag(line, off, htmlBlockType1Tags) {
			return htmlBlockType1, true
		}
	}

	// Type 2: <!--
	if off+2 < len(line) && line[off] == '!' && line[off+1] == '-' && line[off+2] == '-' {
		return htmlBlockType2, true
	}

	// Type 3: <?
	if off < len(line) && line[off] == '?' {
		return htmlBlockType3, true
	}

	// Type 4: <! followed by uppercase letter
	if off+1 < len(line) && line[off] == '!' && line[off+1] >= 'A' && line[off+1] <= 'Z' {
		return htmlBlockType4, true
	}

	// Type 5: <![CDATA[
	if off+7 < len(line) && string(line[off:off+8]) == "![CDATA[" {
		return htmlBlockType5, true
	}

	// Type 6: <tag or </tag where tag is a known block-level element
	// followed by space, tab, >, />, or end-of-line.
	if off < len(line) {
		isClose := false
		if line[off] == '/' {
			isClose = true
			off++
		}
		if off < len(line) && (line[off] >= 'a' && line[off] <= 'z' || line[off] >= 'A' && line[off] <= 'Z') {
			tagEnd := off
			for tagEnd < len(line) {
				c := line[tagEnd]
				if c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' {
					tagEnd++
				} else {
					break
				}
			}
			tag := line[off:tagEnd]
			if matchTagName(tag, htmlBlockType6Tags) {
				// After the tag name, must be followed by space, tab, >, />, or EOL
				if tagEnd >= len(line) {
					return htmlBlockType6, true
				}
				c := line[tagEnd]
				if c == ' ' || c == '\t' || c == '>' {
					return htmlBlockType6, true
				}
				if c == '/' && tagEnd+1 < len(line) && line[tagEnd+1] == '>' {
					return htmlBlockType6, true
				}
			}
		}
		_ = isClose // used for future type 7 distinction
	}

	// Type 7: Opening or closing tag where the tag name is NOT a type-6
	// block-level element. Per CommonMark §4.6, type 7 requires a VALID
	// HTML tag structure — meaning proper attribute syntax (same state
	// machine as isInlineHTMLTag in autolink.go).
	//
	// This excludes autolinks like <http://...> and <foo@bar.com> because
	// those are not valid HTML tags (invalid characters after tag name).
	// Mirrors md4c's approach where type 7 uses the same tag validation
	// as inline HTML detection.
	//
	// Note: we use off=0 (the '<' position) because the type 6 check
	// above may have modified off. isHTMLBlockTag expects off to point
	// to the '<' character.
	if len(line) > 1 {
		c := line[1] // character after '<'
		if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || c == '/' {
			if _, ok := isHTMLBlockTag(line, 0); ok {
				return htmlBlockType7, true
			}
		}
	}

	return 0, false
}

// htmlBlockEndsAtBlank returns true if the HTML block type ends at a blank line.
// Types 6 and 7 end at blank lines; types 1-5 have specific end conditions.
func htmlBlockEndsAtBlank(ht uint8) bool {
	return ht == htmlBlockType6 || ht == htmlBlockType7
}

// htmlBlockEndCondition checks if the line ends an HTML block of the given type.
// Mirrors md4c md_is_html_block_end_condition().
func htmlBlockEndCondition(line []byte, ht uint8) bool {
	switch ht {
	case htmlBlockType1:
		// Ends with </tag> for any type-1 tag
		return findClosingTag(line, htmlBlockType1Tags)
	case htmlBlockType2:
		// Ends with -->
		return containsBytes(line, []byte("-->"))
	case htmlBlockType3:
		// Ends with ?>
		return containsBytes(line, []byte("?>"))
	case htmlBlockType4:
		// Ends with >
		return containsByte(line, '>')
	case htmlBlockType5:
		// Ends with ]]>
		return containsBytes(line, []byte("]]>"))
	case htmlBlockType6, htmlBlockType7:
		// Ends at blank line
		return isBlankLine(line)
	}
	return false
}

// --- HTML block helper functions ---

func matchTag(line []byte, off int, tags [][]byte) bool {
	for _, tag := range tags {
		tagEnd := off + len(tag)
		if tagEnd > len(line) {
			continue
		}
		if !equalFold(line[off:tagEnd], tag) {
			continue
		}
		// After tag, must be space, tab, >, />, or end-of-line
		if tagEnd >= len(line) {
			return true
		}
		c := line[tagEnd]
		if c == ' ' || c == '\t' || c == '>' {
			return true
		}
		if c == '/' && tagEnd+1 < len(line) && line[tagEnd+1] == '>' {
			return true
		}
	}
	return false
}

func matchTagName(tag []byte, tags [][]byte) bool {
	for _, t := range tags {
		if len(tag) != len(t) {
			continue
		}
		if equalFold(tag, t) {
			return true
		}
	}
	return false
}

func equalFold(a, b []byte) bool {
	return bytes.EqualFold(a, b)
}

// isHTMLBlockTag validates HTML tag structure for HTML block type 7 detection.
// It reuses the same attribute state machine as isInlineHTMLTag() in autolink.go,
// but takes the full line with '<' at line[off] (off points to the '<' character).
// Returns (end, true) if a valid HTML tag is found.
//
// This ensures that lines like <foo@bar.com> or <http://...> are NOT matched
// as type 7 HTML blocks, because they don't have valid HTML attribute syntax.
func isHTMLBlockTag(line []byte, off int) (int, bool) {
	size := len(line)
	if off < 0 || off >= size || line[off] != '<' {
		return off, false
	}

	i := off + 1 // skip '<'

	if i >= size {
		return off, false
	}

	// Check for closing tag
	isClose := false
	if line[i] == '/' {
		isClose = true
		i++
	}

	// Tag name must start with alpha
	if i >= size || !isAlpha(line[i]) {
		return off, false
	}

	// Tag name: alpha, digit, or '-'
	i++
	for i < size && (isAlphaDigit(line[i]) || line[i] == '-') {
		i++
	}

	if isClose {
		// Closing tags: optional whitespace then '>'
		for i < size && (line[i] == ' ' || line[i] == '\t' || line[i] == '\n') {
			i++
		}
		if i < size && line[i] == '>' {
			// Type 7: after the closing tag, only whitespace or EOL allowed
			end := i + 1
			for end < size && (line[end] == ' ' || line[end] == '\t' || line[end] == '\n' || line[end] == '\r') {
				end++
			}
			if end >= size {
				return i + 1, true
			}
			return off, false
		}
		return off, false
	}

	// Opening tags: proper attribute state machine
	// Same states as isInlineHTMLTag() in autolink.go.
	attrState := 0

	for i < size {
		c := line[i]
		switch attrState {
		case 0:
			if c == ' ' || c == '\t' || c == '\n' {
				attrState = 1
				i++
			} else if c == '>' {
				// Type 7: after '>', only whitespace or EOL allowed
				// Per CommonMark §4.6: "line begins with an open tag or closing
				// tag, followed only by whitespace or the end of the line."
				end := i + 1
				for end < size && (line[end] == ' ' || line[end] == '\t' || line[end] == '\n' || line[end] == '\r') {
					end++
				}
				if end >= size {
					return i + 1, true
				}
				return off, false
			} else if c == '/' {
				if i+1 < size && line[i+1] == '>' {
					// Self-closing tag: type 7 allows '/>' at end
					end := i + 2
					for end < size && (line[end] == ' ' || line[end] == '\t' || line[end] == '\n' || line[end] == '\r') {
						end++
					}
					if end >= size {
						return i + 2, true
					}
					return off, false
				}
				return off, false
			} else {
				return off, false
			}
		case 1:
			if c == '>' {
				end := i + 1
				for end < size && (line[end] == ' ' || line[end] == '\t' || line[end] == '\n' || line[end] == '\r') {
					end++
				}
				if end >= size {
					return i + 1, true
				}
				return off, false
			} else if c == '/' {
				if i+1 < size && line[i+1] == '>' {
					end := i + 2
					for end < size && (line[end] == ' ' || line[end] == '\t' || line[end] == '\n' || line[end] == '\r') {
						end++
					}
					if end >= size {
						return i + 2, true
					}
					return off, false
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
			if isAlphaDigit(c) || c == '-' || c == '_' || c == '.' || c == ':' {
				i++
			} else if c == ' ' || c == '\t' || c == '\n' {
				// md4c: ISWHITESPACE only transitions state 0→1; state 2 stays.
				i++
			} else if c == '=' {
				attrState = 3
				i++
			} else if c == '>' {
				end := i + 1
				for end < size && (line[end] == ' ' || line[end] == '\t' || line[end] == '\n' || line[end] == '\r') {
					end++
				}
				if end >= size {
					return i + 1, true
				}
				return off, false
			} else if c == '/' {
				if i+1 < size && line[i+1] == '>' {
					end := i + 2
					for end < size && (line[end] == ' ' || line[end] == '\t' || line[end] == '\n' || line[end] == '\r') {
						end++
					}
					if end >= size {
						return i + 2, true
					}
					return off, false
				}
				return off, false
			} else {
				return off, false
			}
		case 3:
			if c == ' ' || c == '\t' || c == '\n' {
				i++
			} else if c == '\'' {
				attrState = 42
				i++
			} else if c == '"' {
				attrState = 43
				i++
			} else if c == '>' {
				return off, false
			} else {
				attrState = 41
				i++
			}
		case 41:
			if c == ' ' || c == '\t' || c == '\n' {
				// md4c: unquoted value ends on whitespace → state 1
				attrState = 1
				i++
			} else if c == '>' {
				end := i + 1
				for end < size && (line[end] == ' ' || line[end] == '\t' || line[end] == '\n' || line[end] == '\r') {
					end++
				}
				if end >= size {
					return i + 1, true
				}
				return off, false
			} else if c == '"' || c == '\'' || c == '=' || c == '<' || c == '`' {
				return off, false
			} else {
				i++
			}
		case 42:
			if c == '\'' {
				attrState = 0
				i++
			} else if c == '>' {
				return off, false
			} else {
				i++
			}
		case 43:
			if c == '"' {
				attrState = 0
				i++
			} else if c == '>' {
				return off, false
			} else {
				i++
			}
		default:
			return off, false
		}
	}

	return off, false
}

func findClosingTag(line []byte, tags [][]byte) bool {
	for _, tag := range tags {
		closeTag := make([]byte, 0, len(tag)+3)
		closeTag = append(closeTag, '<', '/')
		closeTag = append(closeTag, tag...)
		closeTag = append(closeTag, '>')
		if containsBytes(line, closeTag) {
			return true
		}
	}
	return false
}

func containsBytes(s, sub []byte) bool {
	return bytes.Contains(s, sub)
}

func containsByte(s []byte, c byte) bool {
	for _, b := range s {
		if b == c {
			return true
		}
	}
	return false
}

// isSetextUnderline checks if line is a setext header underline.
// level 1 expects '=' characters, level 2 expects '-' characters.
// Mirrors md4c md_is_setext_underline(): contiguous run of the expected
// character, optionally followed by whitespace, then end-of-line.
func isSetextUnderline(line []byte, level int) bool {
	if len(line) == 0 {
		return false
	}
	var expected byte
	if level == 1 {
		expected = '='
	} else {
		expected = '-'
	}
	if line[0] != expected {
		return false
	}
	// Count contiguous expected chars
	off := 0
	for off < len(line) && line[off] == expected {
		off++
	}
	// Optionally, space(s) or tabs can follow
	for off < len(line) && (line[off] == ' ' || line[off] == '\t') {
		off++
	}
	// But nothing more is allowed on the line
	return off >= len(line)
}
