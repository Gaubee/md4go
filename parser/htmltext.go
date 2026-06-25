package parser

import "bytes"

// ScanHTMLTag returns the byte offset immediately after the HTML construct
// starting at text[i] (where text[i] == '<'). Returns i if text[i] does not
// start a valid HTML construct, so the caller can treat '<' as literal text.
//
// Recognizes all HTML constructs per the HTML5 and CommonMark §4.6/§4.7
// parsing model:
//   - Comments: <!-- ... -->
//   - CDATA sections: <![CDATA[ ... ]]>
//   - Processing instructions: <? ... ?>
//   - Declarations: <! ... >
//   - Tags: <tag ...>, </tag>, <br/>
//
// For tags, attribute values in single or double quotes are respected so '>'
// inside quotes does not prematurely close the tag. An unterminated tag (no
// closing '>') is not a valid construct — i is returned.
func ScanHTMLTag(text []byte, i int) int {
	switch {
	case bytes.HasPrefix(text[i:], []byte("<!--")):
		return indexAfterCloser(text, i+4, "-->")
	case bytes.HasPrefix(text[i:], []byte("<![CDATA[")):
		return indexAfterCloser(text, i+9, "]]>")
	case i+1 < len(text) && text[i+1] == '?':
		return indexAfterCloser(text, i+2, "?>")
	case i+1 < len(text) && text[i+1] == '!':
		return indexAfterCloser(text, i+2, ">")
	}
	j := i + 1
	if j < len(text) && text[j] == '/' {
		j++
	}
	if j >= len(text) || !isAlpha(text[j]) {
		return i
	}
	return scanTagClose(text, i)
}

// scanTagClose scans from text[i] (at '<') for the closing '>'. Returns the
// position after '>', or i if no '>' is found (not a valid tag). Respects
// single and double quoted attribute values so '>' inside quotes does not
// prematurely close the tag.
func scanTagClose(text []byte, i int) int {
	n := len(text)
	start := i
	for i++; i < n; {
		switch c := text[i]; {
		case c == '>':
			return i + 1
		case c == '\'' || c == '"':
			i++
			for i < n && text[i] != c {
				i++
			}
			if i < n {
				i++
			}
		default:
			i++
		}
	}
	return start
}

// indexAfterCloser returns the position after the first occurrence of closer
// in text[start:]. If not found, returns len(text).
func indexAfterCloser(text []byte, start int, closer string) int {
	if idx := bytes.Index(text[start:], []byte(closer)); idx >= 0 {
		return start + idx + len(closer)
	}
	return len(text)
}

// ─── Tag name extraction ────────────────────────────────────────────────────

// ParseHTMLTagName extracts the tag name from an HTML tag fragment identified
// by ScanHTMLTag. The fragment must start with '<' and end with '>'
// (e.g. "<script>", "</style>", `<div class="x">`, "<br/>").
//
// Returns:
//   - name: lowercased tag name (e.g. "script", "div")
//   - isClosing: true if this is a closing tag (</tag>)
//   - isSelfClosing: true if this is a self-closing tag (<tag/>)
//
// For non-tag constructs (comments, CDATA, PIs, declarations), name is empty.
func ParseHTMLTagName(tag []byte) (name string, isClosing, isSelfClosing bool) {
	if len(tag) < 2 || tag[0] != '<' {
		return
	}
	// Non-tag constructs: comments (<!--), CDATA (<![CDATA[), PI (<?), declarations (<!)
	// These start with '!','?' after '<' and are not HTML tags.
	c := tag[1]
	if c == '!' || c == '?' {
		return
	}
	j := 1
	if j < len(tag) && tag[j] == '/' {
		isClosing = true
		j++
	}
	// Tag name must start with alpha (validated by ScanHTMLTag, but check again).
	if j >= len(tag) || !isAlpha(tag[j]) {
		return
	}
	start := j
	for j < len(tag) {
		c := tag[j]
		if c == '>' || c == ' ' || c == '\t' || c == '\n' || c == '\r' || c == '/' {
			break
		}
		j++
	}
	name = string(bytes.ToLower(tag[start:j]))
	// Detect self-closing: tag ends with '/>'.
	// We check the character(s) before the final '>'. This is safe because
	// ScanHTMLTag already validated the tag structure and returned a
	// well-formed fragment ending with '>'.
	if len(tag) >= 2 && tag[len(tag)-1] == '>' && tag[len(tag)-2] == '/' {
		isSelfClosing = true
	}
	return
}

// ─── Non-visible element handling ───────────────────────────────────────────

// nonVisibleElements are HTML elements whose text content is not rendered
// by browsers or goquery's DOM text extraction. These elements contain
// script, style, or metadata — not user-visible content.
//
// Reference: HTML5 spec §4.3 (head, title), §4.12 (script, style, noscript,
// template). goquery's Text() method uses the DOM textContent property,
// which does not include content from these elements.
var nonVisibleElements = map[string]bool{
	"script":   true,
	"style":    true,
	"head":     true,
	"title":    true,
	"noscript": true,
	"template": true,
}

// IsNonVisibleElement returns true if the given lowercased tag name
// represents an HTML element whose content is not visible text
// (e.g. script, style, head).
func IsNonVisibleElement(tagName string) bool {
	return nonVisibleElements[tagName]
}

// SkipNonVisibleContent scans text starting at offset `tagEnd` (the position
// immediately after an opening tag) for the closing tag </name> of a
// non-visible element. Returns the offset after the closing tag, or -1 if
// no closing tag is found.
//
// This is used by text renderers to skip the content of non-visible HTML
// elements (script, style, etc.), matching the behavior of DOM-based text
// extraction (goquery, browsers) which does not include content from these
// elements.
func SkipNonVisibleContent(text []byte, tagEnd int, name string) int {
	for pos := tagEnd; pos < len(text); {
		lt := bytes.IndexByte(text[pos:], '<')
		if lt < 0 {
			break
		}
		p := pos + lt
		end := ScanHTMLTag(text, p)
		if end > p {
			n, closing, _ := ParseHTMLTagName(text[p:end])
			if closing && n == name {
				return end
			}
			pos = end
		} else {
			pos = p + 1
		}
	}
	return -1
}
