package parser

// emphasis.go implements the Rule-of-3 emphasis resolution algorithm.
// Mirrors md4c md_analyze_emph() + md_split_emph_mark() + md_analyze_marks().
//
// The Rule-of-3 algorithm uses 12 opener stacks (6 per character * and _)
// to ensure O(n) emphasis pairing. Each stack is indexed by:
//   - Character: * (stacks 0-5) or _ (stacks 6-11)
//   - OC flag: opener-only (oo, stacks 0-2/6-8) or opener+closer (oc, stacks 3-5/9-11)
//   - MOD3: length % 3 == 0, 1, or 2
//
// When a closer is found, it searches the allowed opener stacks for the
// most recent (rightmost) opener, respecting the Rule-of-3 priority:
//  1. Same character, OC stacks with different MOD3
//  2. Same character, OO stacks
//  3. Rightmost opener wins

// splitEmphMark splits a longer emphasis mark into two marks.
// The original mark keeps the first (origLen - n) characters;
// the new mark (carved from a dummy mark) takes the last n characters.
// Returns the index of the new (split-off) mark.
//
// Mirrors md4c md_split_emph_mark() (md4c.c:4145-4160).
func (ms *markStacks) splitEmphMark(markIndex int, n int) int {
	mark := &ms.marks[markIndex]
	newIndex := markIndex + (mark.End - mark.Beg - n)
	dummy := &ms.marks[newIndex]

	// Copy the original mark's fields into the dummy position
	*dummy = *mark
	// Shorten the original mark
	mark.End -= n
	// The new mark starts where the original now ends
	dummy.Beg = mark.End

	return newIndex
}

// analyzeEmph resolves emphasis marks using the Rule-of-3 algorithm.
// Mirrors md4c md_analyze_emph() (md4c.c:4163-4224).
//
// When a closer is encountered, we search the 6 allowed opener stacks
// (based on Rule-of-3 priority) for the most recent opener, then resolve
// the pair. If opener and closer have different lengths, the longer one
// is split using splitEmphMark.
func (ms *markStacks) analyzeEmph(markIndex int) {
	mark := &ms.marks[markIndex]

	// If we can be a closer, try to resolve with a preceding opener.
	if mark.Flags&markPotentialCloser != 0 {
		var opener *Mark
		openerIndex := -1
		flags := mark.Flags

		// Build list of allowed opener stacks per Rule-of-3.
		// Priority:
		//   1. OC stacks with MOD3 different from closer's MOD3
		//   2. OO stacks (if closer is not OC or MOD3 allows)
		// Mirrors md4c md_analyze_emph() opener_stacks[6] construction.
		var openerStacks [6]int
		nOpenerStacks := 0

		// OC stacks: always allowed, prefer different MOD3
		openerStacks[nOpenerStacks] = emphStack(mark.Ch, markEmphOC|markEmphMod3_0)
		nOpenerStacks++
		if (flags & markEmphMod3Mask) != markEmphMod3_2 {
			openerStacks[nOpenerStacks] = emphStack(mark.Ch, markEmphOC|markEmphMod3_1)
			nOpenerStacks++
		}
		if (flags & markEmphMod3Mask) != markEmphMod3_1 {
			openerStacks[nOpenerStacks] = emphStack(mark.Ch, markEmphOC|markEmphMod3_2)
			nOpenerStacks++
		}

		// OO stacks: allowed unless closer is OC with matching MOD3
		openerStacks[nOpenerStacks] = emphStack(mark.Ch, markEmphMod3_0)
		nOpenerStacks++
		if flags&markEmphOC == 0 || (flags&markEmphMod3Mask) != markEmphMod3_2 {
			openerStacks[nOpenerStacks] = emphStack(mark.Ch, markEmphMod3_1)
			nOpenerStacks++
		}
		if flags&markEmphOC == 0 || (flags&markEmphMod3Mask) != markEmphMod3_1 {
			openerStacks[nOpenerStacks] = emphStack(mark.Ch, markEmphMod3_2)
			nOpenerStacks++
		}

		// Find the most recent (rightmost) opener from the allowed stacks.
		// Mirrors md4c: "Opener is the most recent mark from the allowed stacks."
		for i := 0; i < nOpenerStacks; i++ {
			top := ms.stacks[openerStacks[i]]
			if top >= 0 {
				m := &ms.marks[top]
				if opener == nil || m.End > opener.End {
					openerIndex = top
					opener = m
				}
			}
		}

		// Resolve if we found a matching opener.
		if opener != nil {
			openerSize := opener.End - opener.Beg
			closerSize := mark.End - mark.Beg
			stack := ms.openerStack(openerIndex)

			if openerSize > closerSize {
				// Split opener: keep closerSize chars, push remainder
				openerIndex = ms.splitEmphMark(openerIndex, closerSize)
				ms.push(stack, openerIndex)
			} else if openerSize < closerSize {
				// Split closer: keep openerSize chars, remainder stays on closer
				ms.splitEmphMark(markIndex, closerSize-openerSize)
			}

			ms.popOpeners(openerIndex)
			ms.resolveRange(openerIndex, markIndex)
			return
		}
	}

	// If we could not resolve as closer, we may yet be an opener.
	if mark.Flags&markPotentialOpener != 0 {
		stack := emphStack(mark.Ch, mark.Flags)
		ms.push(stack, markIndex)
	}
}

// analyzeTilde resolves tilde marks (strikethrough ~~ / subscript ~).
// GFM requires opener and closer to have matching length (1 or 2).
// Mirrors md4c md_analyze_tilde() (md4c.c:4227-4246).
func (ms *markStacks) analyzeTilde(markIndex int) {
	mark := &ms.marks[markIndex]
	stack := ms.openerStack(markIndex)

	if mark.Flags&markPotentialCloser != 0 && ms.stacks[stack] >= 0 {
		openerIndex := ms.stacks[stack]
		ms.popOpeners(openerIndex)
		ms.resolveRange(openerIndex, markIndex)
		return
	}

	if mark.Flags&markPotentialOpener != 0 {
		ms.push(stack, markIndex)
	}
}

// analyzeCaret resolves caret marks (superscript ^).
// Mirrors md4c md_analyze_caret() (md4c.c:4249-4263).
func (ms *markStacks) analyzeCaret(markIndex int) {
	mark := &ms.marks[markIndex]

	if mark.Flags&markPotentialCloser != 0 && ms.stacks[caretStack] >= 0 {
		openerIndex := ms.stacks[caretStack]
		ms.popOpeners(openerIndex)
		ms.resolveRange(openerIndex, markIndex)
		return
	}

	if mark.Flags&markPotentialOpener != 0 {
		ms.push(caretStack, markIndex)
	}
}

// analyzeHighlight resolves highlight marks (==).
// Simple pairing: most recent opener with most recent closer.
// Mirrors md4c md_analyze_highlight() (md4c.c:4320-4342).
func (ms *markStacks) analyzeHighlight(markIndex int) {
	mark := &ms.marks[markIndex]

	// Guard: only exactly "==" (length 2) can be a highlight pair.
	// Mirrors md4c: if(mark->end - mark->beg != 2) return;
	if mark.End-mark.Beg != 2 {
		return
	}

	if mark.Flags&markPotentialCloser != 0 && ms.stacks[equalStack] >= 0 {
		openerIndex := ms.stacks[equalStack]
		ms.popOpeners(openerIndex)
		ms.resolveRange(openerIndex, markIndex)
		return
	}

	if mark.Flags&markPotentialOpener != 0 {
		ms.push(equalStack, markIndex)
	}
}

// analyzeSpoiler resolves spoiler marks (||).
// Simple pairing: most recent opener with most recent closer.
// Mirrors md4c md_analyze_spoiler() (md4c.c:4294-4313).
func (ms *markStacks) analyzeSpoiler(markIndex int) {
	mark := &ms.marks[markIndex]

	// Guard: only exactly "||" (length 2) can be a spoiler pair.
	// Mirrors md4c: if(mark->end - mark->beg != 2) return;
	if mark.End-mark.Beg != 2 {
		return
	}

	if mark.Flags&markPotentialCloser != 0 && ms.stacks[pipeStack] >= 0 {
		openerIndex := ms.stacks[pipeStack]
		ms.popOpeners(openerIndex)
		ms.resolveRange(openerIndex, markIndex)
		return
	}

	if mark.Flags&markPotentialOpener != 0 {
		ms.push(pipeStack, markIndex)
	}
}

// analyzeDollar resolves dollar marks ($ / $$ for LaTeX math).
// Mirrors md4c md_analyze_dollar() (md4c.c:4265-4292).
//
// Key rules (aligned with md4c):
//   - Opener and closer must have matching length ($ matches $, $$ matches $$)
//   - All marks between opener and closer are disabled (no nesting, no other inline parsing)
//   - After resolving, all pending openers are discarded (LaTeX math does not nest)
func (ms *markStacks) analyzeDollar(markIndex int) {
	mark := &ms.marks[markIndex]

	if mark.Flags&markPotentialCloser != 0 && ms.stacks[dollarStack] >= 0 {
		openerIndex := ms.stacks[dollarStack]
		opener := &ms.marks[openerIndex]

		// Length matching: opener and closer must have the same number of '$'.
		// Mirrors md4c: if(opener->end - opener->beg == closer->end - closer->beg)
		if opener.End-opener.Beg == mark.End-mark.Beg {
			// Matching closer — resolve the pair.
			// Mirrors md4c md_analyze_dollar() (md4c.c:4279-4286).
			ms.popOpeners(openerIndex)
			ms.disableMarksInRange(openerIndex+1, markIndex)
			ms.resolveRange(openerIndex, markIndex)

			// Discard all pending openers: LaTeX math span does not allow nesting.
			// Mirrors md4c: DOLLAR_OPENERS.top = -1
			ms.stacks[dollarStack] = markSentinelIndex
			return
		}
		// Non-matching length: this closer doesn't match the top opener.
		// Fall through to check if this mark can be an opener.
	}

	if mark.Flags&markPotentialOpener != 0 {
		ms.push(dollarStack, markIndex)
	}
}

// analyzeEntity resolves HTML entity marks (&...;).
// Mirrors md4c md_analyze_entity() which calls md_is_entity_str() to validate.
func (ms *markStacks) analyzeEntity(blockText []byte, markIndex int) {
	mark := &ms.marks[markIndex]
	if mark.Flags&markPotentialOpener == 0 {
		return
	}
	// Look for the closest ';' closer after this '&'
	for i := markIndex + 1; i < len(ms.marks); i++ {
		closer := &ms.marks[i]
		if closer.Ch == ';' && closer.Flags&markPotentialCloser != 0 {
			// Validate entity contents between & and ;
			// Mirrors md4c md_is_entity_str() (md4c.c:1412-1434)
			contentBeg := mark.End   // position after '&'
			contentEnd := closer.Beg // position of ';'
			if isValidEntityContent(blockText, contentBeg, contentEnd) {
				ms.resolveRange(markIndex, i)
			}
			return
		}
		// Don't cross resolved spans or other '&' marks
		if closer.Ch == '&' || closer.Flags&markResolved != 0 {
			break
		}
	}
}

// isValidEntityContent validates the content between & and ; of an entity.
// Mirrors md4c md_is_entity_str() (md4c.c:1412-1434) which delegates to
// md_is_hex_entity_contents, md_is_dec_entity_contents, md_is_named_entity_contents.
func isValidEntityContent(text []byte, beg, end int) bool {
	if beg >= end {
		return false
	}

	off := beg

	// Check for numeric entity: &#...; or &#x...;
	if off < end && text[off] == '#' {
		off++
		if off < end && (text[off] == 'x' || text[off] == 'X') {
			// Hexadecimal entity: &#xHHH;
			// Mirrors md4c md_is_hex_entity_contents() (md4c.c:1356-1370)
			// 1-6 hex digits
			digitStart := off + 1
			digitEnd := digitStart
			for digitEnd < end && isHexDigit(text[digitEnd]) && digitEnd-digitStart <= 8 {
				digitEnd++
			}
			nDigits := digitEnd - digitStart
			return nDigits >= 1 && nDigits <= 6 && digitEnd == end
		}
		// Decimal entity: &#DDD;
		// Mirrors md4c md_is_dec_entity_contents() (md4c.c:1373-1387)
		// 1-7 digits
		digitStart := off
		digitEnd := digitStart
		for digitEnd < end && isDigit(text[digitEnd]) && digitEnd-digitStart <= 8 {
			digitEnd++
		}
		nDigits := digitEnd - digitStart
		return nDigits >= 1 && nDigits <= 7 && digitEnd == end
	}

	// Named entity: &name;
	// Mirrors md4c md_is_named_entity_contents() (md4c.c:1390-1410)
	// First char must be alpha, rest alphanumeric, total 2-48 chars
	if off >= end || !isAlpha(text[off]) {
		return false
	}
	off++
	nameStart := beg
	for off < end && isAlnum(text[off]) && off-nameStart <= 48 {
		off++
	}
	nameLen := off - nameStart
	return nameLen >= 2 && nameLen <= 48 && off == end
}

func isHexDigit(c byte) bool {
	return (c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')
}

func isDigit(c byte) bool {
	return c >= '0' && c <= '9'
}

// analyzeMarksRange is the unified mark analysis loop.
// It iterates through marks[markBeg:markEnd], resolving those whose character
// is in the filter set. Mirrors md4c md_analyze_marks() (md4c.c:4549-4614).
//
// When hasNoskip is true, resolved openers whose character is in the noskip set
// are NOT skipped past (used by permissive autolinks to avoid expanding into
// resolved emphasis spans).
func (ms *markStacks) analyzeMarksRange(
	blockText []byte, markBeg, markEnd int,
	filter [256]bool, noskip [256]bool, hasNoskip bool, flags Flags,
) {
	lastEnd := 0
	if markBeg < len(ms.marks) {
		lastEnd = ms.marks[markBeg].Beg
	}

	for i := markBeg; i < markEnd && i < len(ms.marks); i++ {
		mark := &ms.marks[i]

		// Skip resolved spans
		if mark.Flags&markResolved != 0 {
			if mark.Flags&markOpener != 0 && mark.Next >= 0 {
				// If this mark is in the noskip set, don't skip past it
				if !hasNoskip || !noskip[mark.Ch] {
					i = mark.Next
				}
			}
			continue
		}

		// Skip marks not in the filter set
		if !filter[mark.Ch] {
			continue
		}

		// The resolving in previous step could have expanded a mark.
		if mark.Beg < lastEnd {
			continue
		}

		// Analyze the mark by type
		switch mark.Ch {
		case '&':
			ms.analyzeEntity(blockText, i)
		case '*', '_':
			ms.analyzeEmph(i)
		case '~':
			ms.analyzeTilde(i)
		case '^':
			ms.analyzeCaret(i)
		case '$':
			ms.analyzeDollar(i)
		case '|':
			ms.analyzeSpoiler(i)
		case '=':
			ms.analyzeHighlight(i)
		case '[', ']':
			ms.analyzeBracket(i)
		case '@', ':', '.':
			ms.analyzePermissiveAutolink(blockText, i, flags)
		}

		// Update lastEnd if the mark was resolved
		if mark.Flags&markResolved != 0 {
			if mark.Flags&markOpener != 0 && mark.Next >= 0 {
				lastEnd = ms.marks[mark.Next].End
			} else {
				lastEnd = mark.End
			}
		}
	}
}

// analyzeMarks resolves bracket spans (links, images, footnotes).
// Thin wrapper around analyzeMarksRange with the "[]!" filter.
//
// flags is passed as 0 because the filter only includes bracket characters;
// the '@', ':', '.' cases (which use flags for permissive autolinks) are
// never reached. Permissive autolinks are handled separately in
// analyzeLinkContents, which passes the real flags.
func (ms *markStacks) analyzeMarks(blockText []byte) {
	var filter [256]bool
	filter['['] = true
	filter[']'] = true
	filter['!'] = true
	ms.analyzeMarksRange(blockText, 0, len(ms.marks), filter, [256]bool{}, false, 0)
}

// resolveEmphSpanType logic is inlined in processInlines (inline.go)
// case '*','_' to eliminate per-mark heap allocation of []ast.SpanType.
