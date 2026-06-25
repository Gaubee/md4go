package parser

import (
	"github.com/userpro/md4go/ast"
	"unicode"
	"unicode/utf8"
)

// link.go implements bracket-based link/image resolution.
// Mirrors md4c md_analyze_bracket() + md_resolve_brackets() + md_resolve_bracket_link().
//
// The bracket resolution happens in Phase 2 of the inline analysis pipeline:
//   Phase 1: collectMarks — scan block text for potential marks (including [ ] ! brackets)
//   Phase 2: analyzeMarks("[]!") + resolveBrackets — resolve links/images from inside out
//   Phase 3: analyzeMarks("*_&~^$|=") — emphasis/entity/extension resolution
//
// Key design (from md4c):
//   - analyzeBracket: when ']' is found, pair it with the most recent '[' on bracketStack
//     and add the pair to the unresolved_link list (linked via opener.prev)
//   - resolveBrackets: walk the unresolved_link list from inside out, trying to resolve
//     each bracket pair as footnote → wikilink → link/image
//   - Links cannot nest: if inner bracket is a link, outer cannot be a link (but can be image)
//   - Reference definitions are looked up in ctx.refdefs map (I20 will add full refdef tracking)

// analyzeBracket handles '[' and ']' marks during analyzeMarks("[]!").
// Mirrors md4c md_analyze_bracket() (md4c.c:3696-3724).
//
// For '[': push onto bracketStack, set HASNESTED on previous top.
// For ']': pop from bracketStack, pair with opener, add to unresolved_link list.
func (ms *markStacks) analyzeBracket(markIndex int) {
	mark := &ms.marks[markIndex]

	if mark.Flags&markPotentialOpener != 0 {
		// '[' opener: push onto bracket stack
		// Set HASNESTED on the previous top (if any)
		if ms.stacks[bracketStack] >= 0 {
			ms.marks[ms.stacks[bracketStack]].Flags |= markBracketHasNested
		}
		ms.push(bracketStack, markIndex)
		return
	}

	// ']' closer: try to pair with the most recent '[' opener
	if ms.stacks[bracketStack] >= 0 {
		openerIndex := ms.pop(bracketStack)
		opener := &ms.marks[openerIndex]

		// Interconnect opener and closer
		opener.Next = markIndex
		mark.Prev = openerIndex

		// Add the pair to the unresolved_link list.
		// We misuse opener.Prev for the list link (since opener.Next points to closer).
		// The list is ordered by closer position (inside-out).
		if ms.unresolvedLinkTail >= 0 {
			ms.marks[ms.unresolvedLinkTail].Prev = openerIndex
		} else {
			ms.unresolvedLinkHead = openerIndex
		}
		ms.unresolvedLinkTail = openerIndex
		opener.Prev = -1
	}
}

// resolveBrackets walks the unresolved_link list from inside out and
// resolves each bracket pair as a footnote, wikilink, link, or image.
// Mirrors md4c md_resolve_brackets() (md4c.c:4016-4094).
func (ms *markStacks) resolveBrackets(blockText []byte, flags Flags) {
	openerIndex := ms.unresolvedLinkHead
	var lastLinkBeg, lastLinkEnd int
	var lastImgBeg, lastImgEnd int

	for openerIndex >= 0 {
		opener := &ms.marks[openerIndex]
		closerIndex := opener.Next
		closer := &ms.marks[closerIndex]
		nextIndex := opener.Prev

		// Skip disabled marks from previous iterations
		if opener.Ch == 'D' {
			openerIndex = nextIndex
			continue
		}

		// Determine next_opener/next_closer (outer bracket pair, if any)
		var nextOpener, nextCloser *Mark
		if nextIndex >= 0 {
			nextOpener = &ms.marks[nextIndex]
			nextCloser = &ms.marks[nextOpener.Next]
		}

		// Can we be an image? If CANBEIMAGE, expand opener to eat the '!'
		if opener.Flags&markBracketCanBeImage != 0 {
			opener.Ch = '!'
			if opener.Beg > 0 {
				opener.Beg--
			}
		}

		// Check nesting rules (md4c:4058-4067):
		//   - Outer cannot end inside (...) of inner link/image
		//   - Outer cannot be link if inner is link (but can be image)
		if (opener.Beg < lastLinkBeg && closer.End < lastLinkEnd) ||
			(opener.Beg < lastImgBeg && closer.End < lastImgEnd) ||
			(opener.Beg < lastLinkEnd && opener.Ch != '!') {
			openerIndex = nextIndex
			continue
		}

		// I33: Try footnote reference first (md4c:4069-4074).
		// Footnote refs [^label] have higher priority than wikilinks and links.
		if ms.resolveBracketFootnote(blockText, openerIndex, closerIndex, opener, closer,
			&lastLinkBeg, &lastLinkEnd, &nextIndex, flags) {
			openerIndex = nextIndex
			continue
		}

		// I35: Try wikilink next (md4c:4076-4082).
		// Wikilinks [[target]] or [[target|label]] have higher priority than regular links.
		if ms.resolveBracketWikilink(blockText, openerIndex, closerIndex, opener, closer,
			nextOpener, nextCloser, &lastLinkBeg, &lastLinkEnd, &nextIndex, flags) {
			openerIndex = nextIndex
			continue
		}

		// Try to resolve as a link or image
		if ms.resolveBracketLink(blockText, openerIndex, closerIndex, opener, closer,
			nextOpener, nextCloser, &nextIndex,
			&lastLinkBeg, &lastLinkEnd, &lastImgBeg, &lastImgEnd, flags) {
			// Recursively analyze link contents (emphasis, entities, etc.)
			ms.analyzeLinkContents(blockText, openerIndex+1, closerIndex, flags)

			// If the link text is formed by nothing but permissive autolink,
			// suppress the autolink.
			// Mirrors md4c.c:3984-4010 (issue #152).
			if flags&(FlagPermissiveURLAutolinks|FlagPermissiveEmailAutolinks|FlagPermissiveWWWAutolinks) != 0 {
				ms.suppressPermissiveAutolinkInLink(openerIndex, closerIndex)
			}
		}

		openerIndex = nextIndex
	}

	// Clear bracket stack and unresolved list
	ms.stacks[bracketStack] = markSentinelIndex
	ms.unresolvedLinkHead = -1
	ms.unresolvedLinkTail = -1
}

// resolveBracketFootnote tries to resolve a bracket pair as a footnote reference [^label].
// Returns true if resolved as a footnote reference.
// Mirrors md4c md_resolve_bracket_footnote() (md4c.c:3817-3868).
func (ms *markStacks) resolveBracketFootnote(blockText []byte,
	openerIndex, closerIndex int,
	opener, closer *Mark,
	lastLinkBeg, lastLinkEnd *int,
	nextIndex *int,
	flags Flags) bool {

	// Must have FOOTNOTES flag enabled
	if flags&FlagFootnotes == 0 {
		return false
	}

	// Must be a '[' opener (not '!') and the next char must be '^'
	if opener.Ch != '[' {
		return false
	}
	if opener.End >= len(blockText) || blockText[opener.End] != '^' {
		return false
	}

	// Expand the opener to eat the '^'
	// Mirrors md4c.c:3832-3833: opener->end++.
	opener.End++

	// Recalculate closer since we may have changed the opener
	closer = &ms.marks[opener.Next]

	// Label is the raw text between the opener end and the closer begin.
	// opener.End points past [^, closer.Beg points to ].
	labelBeg := opener.End
	labelEnd := closer.Beg

	if labelBeg >= labelEnd {
		return false // empty label
	}

	// Look up the footnote definition
	label := blockText[labelBeg:labelEnd]
	normalized := normalizeLinkLabel(label)
	if len(normalized) == 0 {
		return false
	}

	// Check footnoteDefs map stored in markStacks
	footnoteDef := ms.lookupFootnoteDef(normalized)
	if footnoteDef == nil {
		return false
	}

	// Assign index on first reference
	// Mirrors md4c.c:3849-3852.
	if footnoteDef.Index == 0 {
		ms.nextFootnoteIndex++
		footnoteDef.Index = ms.nextFootnoteIndex
	}
	footnoteDef.RefCount++

	// Store the footnote reference details in the dummy mark after the opener.
	// Mirrors md4c.c:3854-3858: index_mark->beg = def->index; index_mark->end = def->ref_count.
	// We store these in linkAttrMap instead.
	ms.storeLinkAttrs(openerIndex, nil, nil)
	if ms.linkAttrMap != nil {
		attrs := ms.linkAttrMap[openerIndex]
		attrs.footnoteID = footnoteDef.Index
		attrs.footnoteRefID = footnoteDef.RefCount
		ms.linkAttrMap[openerIndex] = attrs
	}

	// Mark as resolved with FOOTNOTE flag
	opener.Flags |= markOpener | markResolved | markBracketFootnote
	closer.Flags |= markCloser | markResolved | markBracketFootnote
	*lastLinkBeg = opener.Beg
	*lastLinkEnd = closer.End
	*nextIndex = opener.Prev

	return true
}

// full reference link, collapsed reference link, or shortcut reference link.
// Mirrors md4c md_resolve_bracket_link() (md4c.c:3872-4014).
//
// Returns true if resolved as a link/image.
func (ms *markStacks) resolveBracketLink(blockText []byte,
	openerIndex, closerIndex int,
	opener, closer *Mark,
	nextOpener, nextCloser *Mark,
	nextIndex *int,
	lastLinkBeg, lastLinkEnd, lastImgBeg, lastImgEnd *int,
	flags Flags) bool {

	var href, title []byte
	isLink := false

	// Strategy 1: If there's a next bracket pair right after the closer,
	// it might be a full reference link [text][ref] or collapsed reference [text][].
	// Mirrors md4c md_resolve_bracket_link() (md4c.c:3924-3967).
	if nextOpener != nil && nextOpener.Beg == closer.End {
		if nextCloser != nil && nextCloser.Beg > closer.End+1 {
			// Full reference link: [text][ref]
			if nextOpener.Flags&markBracketHasNested == 0 {
				href, title, isLink = ms.isLinkReference(blockText,
					nextOpener.Beg, nextCloser.End)
			}
		} else {
			// Collapsed reference link: [text][]
			// The label is the same as the link text.
			// Try looking up the label from [text] in refdefs.
			if opener.Flags&markBracketHasNested == 0 {
				href, title, isLink = ms.isLinkReference(blockText,
					opener.Beg, closer.End)
			}
		}

		if isLink && nextCloser != nil {
			// Eat the 2nd [...]  — expand closer to cover the reference part.
			// Mirrors md4c: closer->end = next_closer->end
			closer.End = nextCloser.End
			// Mark the 2nd bracket pair as dummy so they don't produce output.
			// Mirrors md4c: next_opener->ch = 'D'; next_closer->ch = 'D';
			nextOpener.Ch = 'D'
			nextOpener.Flags |= markResolved
			nextCloser.Ch = 'D'
			nextCloser.Flags |= markResolved
			// Skip the next pair in the outer loop
			*nextIndex = ms.marks[*nextIndex].Prev
		}
	} else {
		// Strategy 2: Check for inline link (...)
		if closer.End < len(blockText) && blockText[closer.End] == '(' {
			var inlineEnd int
			href, title, inlineEnd, isLink = ms.isInlineLinkSpec(blockText, closer.End)
			if isLink && inlineEnd > closer.End {
				// Check the closing ')' is not inside an already resolved range
				// (i.e. a range with a higher priority), e.g. a code span.
				// Mirrors md4c.c:3916-3937.
				followingMarkIdx := closerIndex + 1
				for followingMarkIdx < len(ms.marks) {
					m := &ms.marks[followingMarkIdx]
					if m.Beg >= inlineEnd {
						break
					}
					if m.Flags&(markOpener|markResolved) == (markOpener | markResolved) {
						if m.Next >= 0 && m.Next < len(ms.marks) && ms.marks[m.Next].Beg >= inlineEnd {
							// Cancel the link status — the ')' is inside a resolved span
							isLink = false
							break
						}
						followingMarkIdx = m.Next + 1
					} else {
						followingMarkIdx++
					}
				}

				if isLink {
					// Disable marks in the range [closerIndex+1, followingMarkIdx)
					// that fall within the inline link spec (dest/title).
					ms.disableMarksInRange(closerIndex+1, followingMarkIdx)
					closer.End = inlineEnd
				}
			}
		}

		// Strategy 3: Collapsed reference link [text][]
		if !isLink && opener.Flags&markBracketHasNested == 0 {
			href, title, isLink = ms.isLinkReference(blockText,
				opener.Beg, closer.End)
		}
	}

	if !isLink {
		return false
	}

	// Resolve as link/image
	opener.Flags |= markOpener | markResolved
	closer.Flags |= markCloser | markResolved

	// Store href/title in linkAttrMap (keyed by opener mark index).
	// The two dummy marks after the opener retain their original Beg/End
	// (text offsets) so that analyzeLinkContents's lastEnd tracking works
	// correctly. Previously, setting Beg/End to sentinel mark-index values
	// caused marks within the link content to be incorrectly skipped when
	// their text position was less than the sentinel value.
	ms.storeLinkAttrs(openerIndex, href, title)

	if opener.Ch == '[' {
		*lastLinkBeg = opener.Beg
		*lastLinkEnd = closer.End
	} else {
		*lastImgBeg = opener.Beg
		*lastImgEnd = closer.End
	}

	return true
}

// linkAttrs stores link destination and title for a resolved link/image.
type linkAttrs struct {
	href          []byte
	title         []byte
	footnoteID    uint // I33: for footnote references, the 1-based id
	footnoteRefID uint // I33: for footnote references, the 1-based ref_id among same footnote
}

// storeLinkAttrs stores the href and title for a resolved link at the given opener index.
func (ms *markStacks) storeLinkAttrs(openerIndex int, href, title []byte) {
	if ms.linkAttrMap == nil {
		ms.linkAttrMap = make(map[int]linkAttrs)
	}
	ms.linkAttrMap[openerIndex] = linkAttrs{href: href, title: title}
}

// getLinkAttrs retrieves the stored href and title for a resolved link.
func (ms *markStacks) getLinkAttrs(openerIndex int) (href, title []byte) {
	if ms.linkAttrMap == nil {
		return nil, nil
	}
	attr, ok := ms.linkAttrMap[openerIndex]
	if !ok {
		return nil, nil
	}
	return attr.href, attr.title
}

// isInlineLinkSpec parses an inline link specification starting at off
// (which points to '(').
// Returns (href, title, endOff, ok).
// Mirrors md4c md_is_inline_link_spec() (md4c.c:2572-2666).
func (ms *markStacks) isInlineLinkSpec(text []byte, off int) ([]byte, []byte, int, bool) {
	if off >= len(text) || text[off] != '(' {
		return nil, nil, off, false
	}
	off++ // skip '('

	// Skip optional whitespace (with up to one line break)
	off = skipInlineWhitespace(text, off)

	// Empty link: ()
	// CommonMark spec: () produces a link with empty-string href.
	// Mirrors md4c: sets dest_beg = dest_end = off (empty string, not NULL).
	if off < len(text) && text[off] == ')' {
		return []byte{}, nil, off + 1, true
	}

	// Parse link destination
	destBeg, destEnd, newOff, ok := parseLinkDestination(text, off)
	if !ok {
		return nil, nil, off, false
	}
	off = newOff

	// Parse optional title (md_is_link_title handles its own leading whitespace)
	titleBeg, titleEnd, newOff, titleOk := parseLinkTitleFromInline(text, off)
	if titleOk && titleEnd > titleBeg {
		off = newOff
	}

	// Skip optional whitespace before ')'
	off = skipInlineWhitespace(text, off)

	// Must end with ')'
	if off >= len(text) || text[off] != ')' {
		return nil, nil, off, false
	}
	endOff := off + 1

	href := resolveBackslashEscapes(text[destBeg:destEnd])
	var title []byte
	if titleEnd > titleBeg {
		title = resolveBackslashEscapes(text[titleBeg:titleEnd])
	}
	return href, title, endOff, true
}

// isLinkReference checks whether the bracket range [beg, end) in blockText
// corresponds to a known reference definition.
// Returns (href, title, ok).
// Mirrors md4c md_is_link_reference() (md4c.c:2512-2570).
func (ms *markStacks) isLinkReference(blockText []byte, beg, end int) ([]byte, []byte, bool) {
	if ms.refDefs == nil {
		return nil, nil, false
	}

	// Extract label from [label] or ![label]
	label := extractLinkLabel(blockText, beg, end)
	if len(label) == 0 {
		return nil, nil, false
	}

	// Normalize label: collapse whitespace, case-fold
	normalized := normalizeLinkLabel(label)
	if len(normalized) == 0 {
		return nil, nil, false
	}

	def, ok := ms.refDefs[string(normalized)]
	if !ok {
		return nil, nil, false
	}
	return def.href, def.title, true
}

// extractLinkLabel extracts the label text from a bracket range [label] or ![label].
func extractLinkLabel(text []byte, beg, end int) []byte {
	if beg >= end {
		return nil
	}
	// Skip opening '[' or '!'
	start := beg
	if start < len(text) && text[start] == '!' {
		start++
	}
	if start < len(text) && text[start] == '[' {
		start++
	}
	// Skip closing ']'
	finish := end
	if finish > 0 && finish <= len(text) && text[finish-1] == ']' {
		finish--
	}
	if start >= finish {
		return nil
	}
	return text[start:finish]
}

// normalizeLinkLabel normalizes a link label per CommonMark §4.7:
// strip leading/trailing whitespace, collapse internal whitespace to single space,
// Unicode case-fold. Mirrors md4c md_link_label_cmp() which uses Unicode fold map.
func normalizeLinkLabel(label []byte) []byte {
	// Strip leading/trailing whitespace
	start, end := 0, len(label)
	for start < end && isWhitespace(label[start]) {
		start++
	}
	for end > start && isWhitespace(label[end-1]) {
		end--
	}
	if start >= end {
		return nil
	}

	// Collapse internal whitespace and case-fold using Unicode case folding
	result := make([]byte, 0, end-start)
	inSpace := false
	for i := start; i < end; {
		c := label[i]
		if isWhitespace(c) {
			if !inSpace {
				result = append(result, ' ')
				inSpace = true
			}
			i++
		} else if c < 0x80 {
			// ASCII: simple lower-case
			if c >= 'A' && c <= 'Z' {
				c += 32
			}
			result = append(result, c)
			inSpace = false
			i++
		} else {
			// Multi-byte UTF-8: decode rune, apply Unicode case folding
			r, size := utf8.DecodeRune(label[i:])
			if r > 0 {
				folded := unicodeFold(r)
				result = append(result, folded...)
				inSpace = false
			}
			i += size
		}
	}
	return result
}

// unicodeFold applies Unicode case folding to a rune, mirroring md4c's md_unicode_fold__.
// Uses Go's unicode.ToLower for single-character folding, plus multi-character fold
// handling for ẞ→ss, ß→ss, ﬁ→fi, ﬂ→fl to match md4c's behavior.
func unicodeFold(r rune) []byte {
	// Multi-character folds (md4c alignment: md_unicode_fold__ returns 2-char sequences)
	switch r {
	case 0x1E9E, 0x00DF: // ẞ, ß → "ss"
		return []byte("ss")
	case 0xFB01: // ﬁ → "fi"
		return []byte("fi")
	case 0xFB02: // ﬂ → "fl"
		return []byte("fl")
	}
	// Single-character fold via unicode.ToLower (matches md4c for all other cases)
	lower := unicode.ToLower(r)
	var buf [4]byte
	n := utf8.EncodeRune(buf[:], lower)
	return buf[:n]
}

// isWhitespace returns true for CommonMark whitespace characters.
func isWhitespace(c byte) bool {
	return c == ' ' || c == '\t' || c == '\n' || c == '\r'
}

// suppressPermissiveAutolinkInLink checks if the link text is formed by nothing
// but a permissive autolink, and if so, suppresses (un-resolves) the autolink.
// This prevents double-nested <a> tags when a permissive autolink is the sole
// content of a link.
// Mirrors md4c.c:3984-4010 (issue #152).
func (ms *markStacks) suppressPermissiveAutolinkInLink(openerIndex, closerIndex int) {
	// Find first non-dummy mark after opener
	firstNested := openerIndex + 1
	for firstNested < closerIndex && ms.marks[firstNested].Ch == 'D' {
		firstNested++
	}
	if firstNested >= closerIndex {
		return
	}

	// Find last non-dummy mark before closer
	lastNested := closerIndex - 1
	for lastNested > openerIndex && ms.marks[lastNested].Ch == 'D' {
		lastNested--
	}
	if lastNested <= openerIndex {
		return
	}

	first := &ms.marks[firstNested]
	last := &ms.marks[lastNested]

	// Check if the link content is nothing but a permissive autolink:
	// 1. first mark is resolved
	// 2. first mark's ch is '@', ':', or '.' (permissive autolink marks)
	// 3. first mark starts at the opener's end
	// 4. first mark's next is the last mark
	// 5. last mark's end is at the closer's begin
	if first.Flags&markResolved == 0 {
		return
	}
	if first.Ch != '@' && first.Ch != ':' && first.Ch != '.' {
		return
	}
	if first.Beg != ms.marks[openerIndex].End {
		return
	}
	if first.Next != lastNested {
		return
	}
	if last.End != ms.marks[closerIndex].Beg {
		return
	}

	// Suppress the autolink by un-resolving both marks
	first.Ch = 'D'
	first.Flags &^= markResolved
	last.Ch = 'D'
	last.Flags &^= markResolved
}

// analyzeLinkContents analyzes emphasis/entity/extension marks within a resolved
// link span (between openerIndex+1 and closerIndex).
// Mirrors md4c md_analyze_link_contents() (md4c.c:4656-4699).
func (ms *markStacks) analyzeLinkContents(blockText []byte, markBeg, markEnd int, flags Flags) {
	// Build mark character filter based on flags
	var filter [256]bool
	filter['*'] = true
	filter['_'] = true
	filter['&'] = true
	if flags&FlagStrikethrough != 0 || flags&FlagSubscripts != 0 {
		filter['~'] = true
	}
	if flags&FlagSuperscripts != 0 {
		filter['^'] = true
	}
	if flags&FlagLatexMathSpans != 0 {
		filter['$'] = true
	}
	if flags&FlagSpoilers != 0 {
		filter['|'] = true
	}
	if flags&FlagHighlight != 0 {
		filter['='] = true
	}

	// Analyze marks within the link content range
	ms.analyzeMarksRange(blockText, markBeg, markEnd, filter, [256]bool{}, false, 0)

	// Clear opener stacks after link contents analysis (md4c:4697-4698)
	for i := range ms.stacks {
		if i != bracketStack { // don't clear bracket stack yet
			ms.stacks[i] = markSentinelIndex
		}
	}

	// I30: Permissive autolinks are processed LAST, as they may be greedy and
	// expand from their original mark. Also their implementation must be careful
	// not to cross any (previously) resolved marks when doing so.
	// Mirrors md4c md_analyze_link_contents() (md4c.c:4680-4695).
	if flags&(FlagPermissiveURLAutolinks|FlagPermissiveEmailAutolinks|FlagPermissiveWWWAutolinks) != 0 {
		var autolinkFilter [256]bool
		if flags&FlagPermissiveEmailAutolinks != 0 {
			autolinkFilter['@'] = true
		}
		if flags&FlagPermissiveURLAutolinks != 0 {
			autolinkFilter[':'] = true
		}
		if flags&FlagPermissiveWWWAutolinks != 0 {
			autolinkFilter['.'] = true
		}

		// noskip: the emphasis/entity chars so we don't expand into resolved spans.
		// The `filter` set from above serves as the noskip set.
		ms.analyzeMarksRange(blockText, markBeg, markEnd, autolinkFilter, filter, true, flags)
	}
}

// --- Inline link parsing helpers ---
// These mirror md4c's md_is_link_destination() and md_is_link_title().

// parseLinkDestination parses a link destination starting at off.
// Returns (destBeg, destEnd, newOff, ok).
// Mirrors md4c md_is_link_destination_A/B (md4c.c:2271-2329).
func parseLinkDestination(text []byte, off int) (int, int, int, bool) {
	if off >= len(text) {
		return 0, 0, off, false
	}

	if text[off] == '<' {
		// Angle-bracket destination: <...>
		return parseLinkDestinationAngle(text, off)
	}
	// Bare destination
	return parseLinkDestinationBare(text, off)
}

// parseLinkDestinationAngle parses <...> link destination.
// Mirrors md4c md_is_link_destination_A() (md4c.c:2247-2277).
func parseLinkDestinationAngle(text []byte, off int) (int, int, int, bool) {
	if off >= len(text) || text[off] != '<' {
		return 0, 0, off, false
	}
	off++ // skip '<'
	destBeg := off
	for off < len(text) {
		c := text[off]
		if c == '\\' && off+1 < len(text) && isASCIIPunct(text[off+1]) {
			off += 2
			continue
		}
		// Mirrors md4c.c:2262: ISNEWLINE(off) || CH(off) == '<' → FALSE.
		// c < 0x20 covers newlines/control chars; '<' (0x3C) is explicitly rejected.
		// DEL (0x7F) is also a control char per md4c ISCNTRL (md4c.c:349).
		if c == '<' || isCtrl(c) {
			return 0, 0, off, false
		}
		if c == '>' {
			destEnd := off
			off++ // skip '>'
			return destBeg, destEnd, off, true
		}
		off++
	}
	return 0, 0, off, false
}

// parseLinkDestinationBare parses a bare link destination (no angle brackets).
// Mirrors md4c md_is_link_destination_B().
func parseLinkDestinationBare(text []byte, off int) (int, int, int, bool) {
	destBeg := off
	parenDepth := 0
	for off < len(text) {
		c := text[off]
		if c == '\\' && off+1 < len(text) && isASCIIPunct(text[off+1]) {
			off += 2
			continue
		}
		if c == '(' {
			parenDepth++
			if parenDepth > 32 {
				return 0, 0, off, false
			}
		} else if c == ')' {
			if parenDepth == 0 {
				break
			}
			parenDepth--
		} else if isWhitespace(c) {
			break
		}
		// Control characters not allowed (ISCNTRL in md4c, includes DEL 0x7F)
		if isCtrl(c) {
			return 0, 0, off, false
		}
		off++
	}
	// Mirrors md4c.c:2311: if(parenthesis_level != 0 || off == beg) return FALSE.
	if parenDepth != 0 || off == destBeg {
		return 0, 0, off, false
	}
	return destBeg, off, off, true
}

// parseLinkTitle parses an optional link title starting at off.
// Returns (titleBeg, titleEnd, newOff, ok).
// Mirrors md4c md_is_link_title() (md4c.c:2331-2389).
func parseLinkTitle(text []byte, off int) (int, int, int, bool) {
	if off >= len(text) {
		return 0, 0, off, false
	}

	// Skip whitespace first
	wsOff := skipInlineWhitespace(text, off)
	if wsOff == off {
		// No whitespace before title → no title
		return 0, 0, off, false
	}
	off = wsOff

	return parseLinkTitleContent(text, off)
}

// parseLinkTitleFromInline parses an optional link title in an inline link spec.
// Unlike parseLinkTitle, it expects to be called right after the destination,
// and handles whitespace + title detection in one go.
func parseLinkTitleFromInline(text []byte, off int) (int, int, int, bool) {
	if off >= len(text) {
		return 0, 0, off, false
	}

	// Must have whitespace before title
	wsOff := skipInlineWhitespace(text, off)
	if wsOff == off {
		// No whitespace → no title
		return 0, 0, off, false
	}

	titleBeg, titleEnd, newOff, ok := parseLinkTitleContent(text, wsOff)
	if !ok {
		return 0, 0, off, false
	}
	return titleBeg, titleEnd, newOff, true
}

// parseLinkTitleContent parses the actual title content after whitespace has been skipped.
func parseLinkTitleContent(text []byte, off int) (int, int, int, bool) {
	if off >= len(text) {
		return 0, 0, off, false
	}

	var closerChar byte
	switch text[off] {
	case '"':
		closerChar = '"'
	case '\'':
		closerChar = '\''
	case '(':
		closerChar = ')'
	default:
		return 0, 0, off, false
	}
	off++ // skip opening quote

	titleBeg := off
	for off < len(text) {
		c := text[off]
		if c == '\\' && off+1 < len(text) && isASCIIPunct(text[off+1]) {
			off += 2
			continue
		}
		if c == closerChar {
			titleEnd := off
			off++ // skip closing quote
			return titleBeg, titleEnd, off, true
		}
		if closerChar == ')' && c == '(' {
			return 0, 0, off, false
		}
		off++
	}
	return 0, 0, off, false
}

// skipInlineWhitespace skips whitespace in inline link parsing.
func skipInlineWhitespace(text []byte, off int) int {
	for off < len(text) {
		c := text[off]
		if c == ' ' || c == '\t' || c == '\n' || c == '\r' {
			off++
		} else {
			break
		}
	}
	return off
}

// resolveLinkSpanType determines the span type for a resolved bracket pair.
func resolveLinkSpanType(openerCh byte) ast.SpanType {
	if openerCh == '!' {
		return ast.SpanImg
	}
	return ast.SpanLink
}

// RefDef represents a reference definition, stored in the refdefs map.
type RefDef struct {
	href  []byte
	title []byte
}

// resolveBackslashEscapes removes backslash escapes from a byte slice.
// CommonMark requires that backslash escapes in link destinations and titles
// be resolved: \! → !, \( → (, etc. The backslash before ASCII punctuation
// is removed; all other characters are kept as-is.
// Mirrors md4c's approach of processing escapes during extraction.
func resolveBackslashEscapes(text []byte) []byte {
	result := make([]byte, 0, len(text))
	for i := 0; i < len(text); i++ {
		if text[i] == '\\' && i+1 < len(text) && (isASCIIPunct(text[i+1]) || text[i+1] == '\n') {
			// Backslash + ASCII punct → just the punct
			// Backslash + newline → just the newline (mirrors md4c hard-break mark)
			result = append(result, text[i+1])
			i++
		} else {
			result = append(result, text[i])
		}
	}
	return result
}

// resolveBracketWikilink tries to resolve a bracket pair as a wikilink
// [[destination]] or [[destination|label]].
// Returns true if resolved as a wikilink.
// Mirrors md4c md_resolve_bracket_wikilink() (md4c.c:3735-3815).
func (ms *markStacks) resolveBracketWikilink(blockText []byte,
	openerIndex, closerIndex int,
	opener, closer *Mark,
	nextOpener, nextCloser *Mark,
	lastLinkBeg, lastLinkEnd *int,
	nextIndex *int,
	flags Flags) bool {

	// Must have WIKILINKS flag enabled
	if flags&FlagWikilinks == 0 {
		return false
	}

	// Check basic conditions (md4c.c:3750-3752):
	//   - opener must be '[' with end-beg == 1 (single '[')
	//   - next_opener must be '[' with end-beg == 1
	//   - next_closer must be ']' with end-beg == 1
	if opener.Ch != '[' || opener.End-opener.Beg != 1 {
		return false
	}
	// Also reject if opener itself has CANBEIMAGE (e.g., ![[foo]])
	if opener.Flags&markBracketCanBeImage != 0 {
		return false
	}
	if nextOpener == nil || nextOpener.Ch != '[' || nextOpener.End-nextOpener.Beg != 1 {
		return false
	}
	// In md4c, ![[ creates a mark with Ch='!' (not '['), so next_opener->ch != '['
	// rejects it. In our code, ![ creates a '[' mark with CANBEIMAGE flag.
	// If nextOpener has CANBEIMAGE, it means ![[ which should NOT be a wikilink
	// (it should be an image instead).
	if nextOpener.Flags&markBracketCanBeImage != 0 {
		return false
	}
	if nextCloser == nil || nextCloser.Ch != ']' || nextCloser.End-nextCloser.Beg != 1 {
		return false
	}

	// Check that next_opener and next_closer are nested properly (md4c.c:3755-3757):
	//   - next_opener.beg == opener.beg - 1 (the second [ is immediately before the first)
	//   - next_closer.beg == closer.beg + 1 (the second ] is immediately after the first)
	if nextOpener.Beg != opener.Beg-1 || nextCloser.Beg != closer.Beg+1 {
		return false
	}

	// Scan for '|' delimiter within the brackets (md4c.c:3762-3776).
	// We don't allow destination to be longer than 100 characters.
	var delimIndex int = -1
	delimOff := openerIndex + 1
	for delimOff < closerIndex {
		m := &ms.marks[delimOff]
		if m.Ch == '|' {
			delimIndex = delimOff
			break
		}
		if m.Ch != 'D' {
			if m.Beg-opener.End > 100 {
				break
			}
			if m.Flags&markOpener != 0 && m.Next >= 0 {
				delimOff = m.Next
			}
		}
		delimOff++
	}

	// Compute destination range
	destBeg := opener.End
	destEnd := closer.Beg
	if delimIndex >= 0 {
		destEnd = ms.marks[delimIndex].Beg
	}

	// Validate destination (md4c.c:3780-3787)
	if destEnd-destBeg == 0 || destEnd-destBeg > 100 {
		return false
	}

	// No newlines in destination (md4c.c:3783-3787)
	for off := destBeg; off < destEnd; off++ {
		if blockText[off] == '\n' {
			return false
		}
	}

	// Pop openers (md4c.c:3789)
	ms.popOpeners(openerIndex)

	// Handle the pipe delimiter (md4c.c:3791-3801)
	if delimIndex >= 0 {
		delim := &ms.marks[delimIndex]
		if delim.End < closer.Beg {
			// Normal case: [[target|label]]
			ms.disableMarksInRange(openerIndex+1, delimIndex)
			delim.Flags |= markResolved
			opener.End = delim.Beg
		} else {
			// The pipe is just before the closer: [[foo|]]
			ms.disableMarksInRange(openerIndex+1, closerIndex)
			closer.Beg = delim.Beg
		}
	}

	// Expand opener and closer to cover the outer brackets (md4c.c:3803-3804)
	opener.Beg = nextOpener.Beg
	closer.End = nextCloser.End

	// Resolve the range (md4c.c:3805)
	opener.Flags |= markOpener | markResolved
	closer.Flags |= markCloser | markResolved
	opener.Next = closerIndex
	closer.Prev = openerIndex

	// Mark next_opener and next_closer as dummy (they were consumed by the wikilink)
	nextOpener.Ch = 'D'
	nextOpener.Flags |= markResolved
	nextCloser.Ch = 'D'
	nextCloser.Flags |= markResolved

	*lastLinkBeg = opener.Beg
	*lastLinkEnd = closer.End

	// If there's a pipe delimiter, analyze link contents (label) after it
	// (md4c.c:3810-3811)
	if delimIndex >= 0 {
		ms.analyzeLinkContents(blockText, delimIndex+1, closerIndex, flags)
	}

	*nextIndex = nextOpener.Prev
	return true
}
