package parser

import "unicode/utf8"

// permissive_autolink.go implements permissive autolink resolution.
// Mirrors md4c md_analyze_permissive_autolink() + md_analyze_permissive_autolink_segment()
// (md4c.c:4380-4545).
//
// Permissive autolinks detect URLs, emails, and WWW links without requiring
// angle brackets (<...>). They are analyzed LAST in the inline pipeline,
// after emphasis/entity/extension marks, because they are greedy and expand
// from their original mark position.
//
// Three types:
//   - '@' → permissive email autolink (e.g. user@domain.com)
//   - ':' → permissive URL autolink (e.g. http://example.com)
//   - '.' → permissive WWW autolink (e.g. www.example.com)

// analyzePermissiveAutolink tries to resolve a permissive autolink mark.
// Mirrors md4c md_analyze_permissive_autolink() (md4c.c:4463-4545).
//
// The opener mark (@/:/)  has beg/end set to the trigger range (e.g. "http://"
// for ':', "@" for '@', "www." for '.'). The next mark (dummy) stores line
// boundaries: beg=lineBeg, end=lineEnd.
//
// On success, the opener is expanded to cover the full autolink text range,
// the dummy becomes the closer, and the pair is resolved.
func (ms *markStacks) analyzePermissiveAutolink(blockText []byte, markIndex int, flags Flags) {
	opener := &ms.marks[markIndex]
	closer := &ms.marks[markIndex+1] // the dummy mark

	if closer.Ch != 'D' {
		return
	}

	// Dummy mark stores line boundaries (from collectMarks)
	lineBeg := closer.Beg
	lineEnd := closer.End

	beg := opener.Beg
	end := opener.End

	// For '@': scan backwards for email username
	// Mirrors md4c.c:4477-4483
	if opener.Ch == '@' {
		nComponents := ms.analyzePermissiveAutolinkSegment(blockText, beg, lineBeg, &beg, true,
			0, "", ".-+_", markIndex)
		if nComponents < 1 {
			return
		}
	}

	// Verify left boundary: must be line start, whitespace, allowed punctuation,
	// or a resolved opener mark.
	// Mirrors md4c.c:4485-4493: ISUNICODEWHITESPACEBEFORE(beg) + ISANYOF(beg-1, "({[")
	if beg > lineBeg {
		c := blockText[beg-1]
		// Use isWhitespaceBefore for correct multi-byte Unicode whitespace handling.
		// Mirrors md4c ISUNICODEWHITESPACEBEFORE which uses md_decode_utf8_before__.
		if !isWhitespaceBefore(blockText, beg) &&
			c != '(' && c != '{' && c != '[' {
			// Check if preceded by a resolved opener mark
			if !ms.scanLeftForResolvedOpener(markIndex, beg-1) {
				return
			}
		}
	}

	// Scan for hostname segment. Hostname requires at least 2 dot-delimited components.
	// Mirrors md4c.c:4495-4499
	nComponents := ms.analyzePermissiveAutolinkSegment(blockText, end, lineEnd, &end, false,
		'.', "", "-_", markIndex)
	if nComponents < 2 {
		return
	}

	// For URL/WWW (not email): scan for path, query, fragment segments
	// Mirrors md4c.c:4501-4526
	if opener.Ch != '@' {
		// Scan for path segment
		if end < lineEnd && blockText[end] == '/' {
			n := ms.analyzePermissiveAutolinkSegment(blockText, end+1, lineEnd, &end, false,
				'/', ".+-_~", "", markIndex)
			if n < 0 {
				return
			}
			// Path can also end with additional '/' (directory)
			if end < lineEnd && blockText[end] == '/' {
				end++
			}
		}

		// Scan for query segment
		if end < lineEnd && blockText[end] == '?' {
			n := ms.analyzePermissiveAutolinkSegment(blockText, end+1, lineEnd, &end, false,
				'&', "._=()", "+-", markIndex)
			if n < 0 {
				return
			}
		}

		// Scan for fragment segment
		if end < lineEnd && blockText[end] == '#' {
			n := ms.analyzePermissiveAutolinkSegment(blockText, end+1, lineEnd, &end, false,
				0, "", ".-+_", markIndex)
			if n < 0 {
				return
			}
		}
	}

	// Verify right boundary: must be line end, whitespace, allowed punctuation,
	// or a resolved closer mark.
	// Mirrors md4c.c:4528-4536
	if end < lineEnd {
		c := blockText[end]
		if !isWhitespace(c) && !isUnicodeWhitespaceAt(blockText, end) &&
			c != ')' && c != '}' && c != ']' && c != '.' && c != '!' &&
			c != '?' && c != ',' && c != ';' {
			// Check if followed by a resolved closer mark
			if !ms.scanRightForResolvedCloser(markIndex, end) {
				return
			}
		}
	}

	// Success — resolve as permissive autolink.
	// Mirrors md4c.c:4538-4544
	opener.Beg = beg
	opener.End = beg
	closer.Beg = end
	closer.End = end
	closer.Ch = opener.Ch
	ms.resolveRange(markIndex, markIndex+1)
}

// analyzePermissiveAutolinkSegment scans a segment of a permissive autolink.
// Mirrors md4c md_analyze_permissive_autolink_segment() (md4c.c:4380-4461).
//
// Scans forward (scanBackwards=false) or backward (scanBackwards=true) from
// 'off' towards 'boundary', looking for word components delimited by
// componentDelim (e.g. '.' for hostname) and word_delims (e.g. "-_").
//
// Returns the number of components found. If a componentDelim is provided,
// each occurrence of it increments the component count. The final segment
// after the last delimiter also counts as a component.
//
// On return, *pOff is updated to the new position.
func (ms *markStacks) analyzePermissiveAutolinkSegment(blockText []byte, off, boundary int, pOff *int,
	scanBackwards bool, componentDelim byte, wordExtra, wordDelims string,
	markIndex int) int {

	nComponents := 0
	nOpenBrackets := 0
	seenWordDelim := true
	seenComponentDelim := true
	componentBeg := off

	for off != boundary {
		if scanBackwards {
			off--
		}

		c := blockText[off]

		// Only accept extra and delimiter characters if they're not part of a
		// resolved mark. Mirrors md4c.c:4400-4408.
		if !isAlnum(c) && !isWhitespace(c) {
			if (!scanBackwards && ms.scanRightForResolvedMark(markIndex, off)) ||
				(scanBackwards && ms.scanLeftForResolvedMark(markIndex, off)) {
				if scanBackwards {
					off++
				}
				break
			}
		}

		// Track parentheses balance (not for email)
		if !scanBackwards {
			if c == '(' {
				nOpenBrackets++
			} else if c == ')' {
				if nOpenBrackets <= 0 {
					break
				}
				nOpenBrackets--
			}
		}

		if isAlnum(c) || isInString(c, wordExtra) {
			seenWordDelim = false
			seenComponentDelim = false
		} else {
			if seenWordDelim {
				if scanBackwards {
					off++
				}
				break
			}

			if isInString(c, wordDelims) {
				seenWordDelim = true
			} else if componentDelim != 0 && c == componentDelim {
				if seenComponentDelim {
					if scanBackwards {
						off++
					}
					break
				}
				seenComponentDelim = true
				componentBeg = off
				nComponents++
			} else {
				if scanBackwards {
					off++
				}
				break
			}
		}

		if !scanBackwards {
			off++
		}
	}

	// Rollback falsely consumed delimiter
	if seenWordDelim || seenComponentDelim {
		if scanBackwards {
			off++
		} else {
			off--
		}
	}

	if off != componentBeg {
		nComponents++
	}

	if nOpenBrackets != 0 {
		return -1
	}

	*pOff = off
	return nComponents
}

// scanLeftForResolvedOpener checks if position pos is within a resolved mark's
// span (beg <= pos < end). If the mark found is an opener, returns true.
// Mirrors md4c md_scan_left_for_resolved_mark() (md4c.c:4336-4356).
func (ms *markStacks) scanLeftForResolvedOpener(markIndex int, pos int) bool {
	for i := markIndex - 1; i >= 0; i-- {
		m := &ms.marks[i]
		if m.Ch == 'D' || m.Beg > pos {
			continue
		}
		if m.Beg <= pos && pos < m.End && m.Flags&markResolved != 0 {
			return m.Flags&markOpener != 0
		}
		if m.End <= pos {
			break
		}
	}
	return false
}

// scanRightForResolvedCloser checks if position pos is within a resolved mark's
// span (beg <= pos < end). If the mark found is a closer, returns true.
// Mirrors md4c md_scan_right_for_resolved_mark() (md4c.c:4358-4378).
func (ms *markStacks) scanRightForResolvedCloser(markIndex int, pos int) bool {
	for i := markIndex + 2; i < len(ms.marks); i++ {
		m := &ms.marks[i]
		if m.Ch == 'D' || m.End <= pos {
			continue
		}
		if m.Beg <= pos && pos < m.End && m.Flags&markResolved != 0 {
			return m.Flags&markCloser != 0
		}
		if m.Beg > pos {
			break
		}
	}
	return false
}

// scanLeftForResolvedMark checks if position pos is within any resolved mark's span.
// Used in segment scanning to stop at resolved mark boundaries.
// Mirrors md4c md_scan_left_for_resolved_mark() (md4c.c:4336-4356).
func (ms *markStacks) scanLeftForResolvedMark(markIndex int, pos int) bool {
	for i := markIndex - 1; i >= 0; i-- {
		m := &ms.marks[i]
		if m.Ch == 'D' || m.Beg > pos {
			continue
		}
		if m.Beg <= pos && pos < m.End && m.Flags&markResolved != 0 {
			return true
		}
		if m.End <= pos {
			break
		}
	}
	return false
}

// scanRightForResolvedMark checks if position pos is within any resolved mark's span.
// Used in segment scanning to stop at resolved mark boundaries.
// Mirrors md4c md_scan_right_for_resolved_mark() (md4c.c:4358-4378).
func (ms *markStacks) scanRightForResolvedMark(markIndex int, pos int) bool {
	for i := markIndex + 1; i < len(ms.marks); i++ {
		m := &ms.marks[i]
		if m.Ch == 'D' || m.End <= pos {
			continue
		}
		if m.Beg <= pos && pos < m.End && m.Flags&markResolved != 0 {
			return true
		}
		if m.Beg > pos {
			break
		}
	}
	return false
}

// isInString checks if byte c is in the string s.
func isInString(c byte, s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] == c {
			return true
		}
	}
	return false
}

// isUnicodeWhitespaceAt checks if the byte at position off in blockText
// starts a Unicode whitespace character. Uses md4c's WHITESPACE_MAP.
func isUnicodeWhitespaceAt(blockText []byte, off int) bool {
	if off < 0 || off >= len(blockText) {
		return false
	}
	c := blockText[off]
	if c == ' ' || c == '\t' || c == '\n' || c == '\r' {
		return true
	}
	// Check for multi-byte Unicode whitespace
	if c >= 0xC0 {
		r, _ := utf8.DecodeRune(blockText[off:])
		if r > 0 {
			return isMd4cUnicodeWhitespace(r)
		}
	}
	return false
}
