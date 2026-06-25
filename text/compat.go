package text

import (
	"bytes"

	// stdhtml aliases the standard library html package to avoid
	// collision with the md4go/html package.
	stdhtml "html"

	"github.com/userpro/md4go/parser"
)

// compatConfig pre-computes renderer-level behavior decisions from parser
// Flags, avoiding repeated bitmask checks in hot paths.
type compatConfig struct {
	decodeEntities bool // FlagDecodeEntities: decode HTML entities to Unicode
	stripHTMLTags  bool // FlagStripHTMLTags: strip HTML tags from raw HTML content
}

// newCompatConfig builds a compatConfig from parser flags.
func newCompatConfig(flags parser.Flags) compatConfig {
	return compatConfig{
		decodeEntities: flags&parser.FlagDecodeEntities != 0,
		stripHTMLTags:  flags&parser.FlagStripHTMLTags != 0,
	}
}

// decodeEntity decodes an HTML entity to Unicode or returns it verbatim.
func decodeEntity(text []byte, decode bool) []byte {
	if !decode {
		return text
	}
	return []byte(stdhtml.UnescapeString(string(text)))
}

// extractHTMLText strips HTML tags and constructs from raw HTML, keeping
// only the visible text content. It uses parser.ScanHTMLTag to identify and
// skip HTML constructs (tags, comments, CDATA, PIs, declarations).
//
// For non-visible elements (script, style, etc.), the entire element content
// is removed — not just the tags — matching the behavior of DOM-based text
// extraction (goquery, browsers). This is handled by parser.SkipNonVisibleContent.
//
// A '<' not starting a valid HTML construct is preserved as literal text.
func extractHTMLText(html []byte) []byte {
	out, _ := extractHTMLTextTracked(html)
	return out
}

// extractHTMLTextTracked is like extractHTMLText but additionally returns
// the tag name of an unclosed non-visible element if one is found at the end
// of the fragment (i.e., an opening tag like <script> with no matching
// </script> in the same fragment). Callers that process HTML content across
// multiple fragments (e.g., line-by-line HTML block events) use this to
// skip subsequent fragments until the closing tag is found.
//
// Returns (extractedText, unclosedNonVisibleTag). The tag name is empty
// when no unclosed non-visible element was found.
func extractHTMLTextTracked(html []byte) (extracted []byte, unclosedNV string) {
	if len(html) == 0 {
		return nil, ""
	}
	if bytes.IndexByte(html, '<') < 0 {
		return html, ""
	}
	var out []byte
	i := 0
	for i < len(html) {
		if html[i] != '<' {
			out = append(out, html[i])
			i++
			continue
		}
		end := parser.ScanHTMLTag(html, i)
		if end <= i {
			// Not a valid HTML construct — preserve '<' as literal text.
			out = append(out, '<')
			i++
			continue
		}
		// Check if this is an opening tag for a non-visible element.
		// If so, skip the entire element content until the closing tag.
		name, isClosing, isSelfClosing := parser.ParseHTMLTagName(html[i:end])
		if !isClosing && !isSelfClosing && parser.IsNonVisibleElement(name) {
			skipEnd := parser.SkipNonVisibleContent(html, end, name)
			if skipEnd < 0 {
				// No closing tag in this fragment — enter skip mode.
				return out, name
			}
			i = skipEnd
		} else {
			i = end
		}
	}
	return out, ""
}
