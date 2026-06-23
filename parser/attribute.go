package parser

import "md4go/ast"

// attribute.go implements attribute building with substring type tracking.
// Mirrors md4c md_build_attribute() (md4c.c:1508-1591).
//
// md4c splits attribute values (href, title, src, lang, etc.) into typed
// substrings: NORMAL, ENTITY, and NULLCHAR. The renderer then handles each
// substring type differently:
//   - NORMAL: pass through escaping function (HTML-escape or URL-escape)
//   - ENTITY: translate entity to UTF-8, then pass through escaping function
//   - NULLCHAR: render as U+0000 (verbatim) in attributes
//
// This is the MD_ATTRIBUTE substr_types mechanism (md4c.h:277-282, §10 C-30).

// buildAttrNoEscapes is the flag for BuildAttribute to skip backslash escape
// resolution. Mirrors md4c MD_BUILD_ATTR_NO_ESCAPES (md4c.c:1458).
// Used for autolink destinations where backslash escapes are not resolved.
const buildAttrNoEscapes = 1

// BuildAttribute processes raw text into an ast.Attribute, resolving backslash
// escapes (unless noEscapes is set) and identifying entities and NULL characters.
//
// The resulting Attribute.Text has backslash escapes resolved (when enabled),
// with entities preserved as-is (e.g. &amp; stays in the text). The SubTypes
// and SubOffsets arrays record where entities and NULL chars are located so the
// renderer can handle them specially.
//
// Mirrors md4c md_build_attribute() (md4c.c:1508-1591).
func BuildAttribute(rawText []byte, flags int) ast.Attribute {
	rawSize := len(rawText)
	if rawSize == 0 {
		return ast.EmptyAttribute
	}

	// Fast path: if no backslash, ampersand, or NULL, build trivial attribute
	// without any allocation. Mirrors md4c is_trivial check (md4c.c:1520-1526).
	isTrivial := true
	for i := 0; i < rawSize; i++ {
		c := rawText[i]
		if c == '\\' || c == '&' || c == 0 {
			isTrivial = false
			break
		}
	}

	if isTrivial {
		return ast.NewAttribute(rawText)
	}

	// Non-trivial: scan and build substrings
	text := make([]byte, 0, rawSize)
	var subTypes []ast.SubstrType
	var subOffsets []int

	off := 0
	rawOff := 0

	for rawOff < rawSize {
		c := rawText[rawOff]

		// NULL character → NULLCHAR substring
		// Mirrors md4c.c:1549-1555.
		if c == 0 {
			subTypes = append(subTypes, ast.SubstrNullChar)
			subOffsets = append(subOffsets, off)
			text = append(text, c)
			off++
			rawOff++
			continue
		}

		// Entity (&...) → ENTITY substring if valid entity
		// Mirrors md4c.c:1557-1567.
		if c == '&' {
			entEnd := findEntityEnd(rawText, rawOff, rawSize)
			if entEnd > rawOff {
				subTypes = append(subTypes, ast.SubstrEntity)
				subOffsets = append(subOffsets, off)
				text = append(text, rawText[rawOff:entEnd]...)
				off += entEnd - rawOff
				rawOff = entEnd
				continue
			}
		}

		// Start or continue NORMAL substring
		// Mirrors md4c.c:1569-1570.
		if len(subTypes) == 0 || subTypes[len(subTypes)-1] != ast.SubstrNormal {
			subTypes = append(subTypes, ast.SubstrNormal)
			subOffsets = append(subOffsets, off)
		}

		// Backslash escape resolution (unless noEscapes flag)
		// Mirrors md4c.c:1572-1575.
		if flags&buildAttrNoEscapes == 0 &&
			c == '\\' && rawOff+1 < rawSize &&
			(isASCIIPunct(rawText[rawOff+1]) || rawText[rawOff+1] == '\n') {
			rawOff++ // skip backslash, keep escaped char
		}

		text = append(text, rawText[rawOff])
		off++
		rawOff++
	}

	// Final offset (mirrors md4c.c:1579)
	subOffsets = append(subOffsets, off)

	return ast.Attribute{
		Text:       text,
		SubTypes:   subTypes,
		SubOffsets: subOffsets,
	}
}

// findEntityEnd checks if text[beg:] starts with a valid entity (&...;).
// Returns the offset after the ';' if valid, or beg if not.
// Mirrors md4c md_is_entity_str() (md4c.c:1411-1433).
func findEntityEnd(text []byte, beg, maxEnd int) int {
	off := beg
	if off >= maxEnd || text[off] != '&' {
		return beg
	}
	off++

	if off+2 < maxEnd && text[off] == '#' && (text[off+1] == 'x' || text[off+1] == 'X') {
		// Hex entity: &#xHHH;
		if !isValidHexEntityContents(text, off+2, maxEnd, &off) {
			return beg
		}
	} else if off+1 < maxEnd && text[off] == '#' {
		// Decimal entity: &#DDD;
		if !isValidDecEntityContents(text, off+1, maxEnd, &off) {
			return beg
		}
	} else {
		// Named entity: &name;
		if !isValidNamedEntityContents(text, off, maxEnd, &off) {
			return beg
		}
	}

	if off < maxEnd && text[off] == ';' {
		return off + 1
	}
	return beg
}

// isValidHexEntityContents validates hex digits between &#x and ;.
// Mirrors md4c md_is_hex_entity_contents() (md4c.c:1356-1370).
// 1-6 hex digits allowed.
func isValidHexEntityContents(text []byte, beg, maxEnd int, off *int) bool {
	n := 0
	o := beg
	for o < maxEnd {
		c := text[o]
		if (c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F') {
			n++
			o++
			if n > 6 {
				return false
			}
		} else {
			break
		}
	}
	*off = o
	return n >= 1
}

// isValidDecEntityContents validates decimal digits between &# and ;.
// Mirrors md4c md_is_dec_entity_contents() (md4c.c:1372-1386).
// 1-7 decimal digits allowed.
func isValidDecEntityContents(text []byte, beg, maxEnd int, off *int) bool {
	n := 0
	o := beg
	for o < maxEnd {
		c := text[o]
		if c >= '0' && c <= '9' {
			n++
			o++
			if n > 7 {
				return false
			}
		} else {
			break
		}
	}
	*off = o
	return n >= 1
}

// isValidNamedEntityContents validates named entity content between & and ;.
// Mirrors md4c md_is_named_entity_contents() (md4c.c:1388-1410).
// 2-48 alphanumeric characters (first must be letter).
func isValidNamedEntityContents(text []byte, beg, maxEnd int, off *int) bool {
	n := 0
	o := beg
	for o < maxEnd {
		c := text[o]
		if isAlnumByte(c) {
			if n == 0 && !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')) {
				return false
			}
			n++
			o++
			if n > 48 {
				return false
			}
		} else {
			break
		}
	}
	*off = o
	return n >= 2
}

// isAlnumByte returns true for ASCII alphanumeric characters.
func isAlnumByte(c byte) bool {
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9')
}
