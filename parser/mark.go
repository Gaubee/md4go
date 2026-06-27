package parser

import (
	"unicode/utf8"
)

// Mark represents an inline marker in the text, mirroring md4c's MD_MARK.
// Marks are stored in a flat slice; Prev/Next serve as both linked-list
// pointers (for opener stacks) and resolved-pair cross-references.
//
// Mark characters and their roles (from md4c md4c.c §2727-2746):
//
//	'\\' — backslash escape
//	'\0' — NULL character (replaced with U+FFFD)
//	'*'  — emphasis/strong opener or closer
//	'_'  — emphasis/strong opener or closer
//	'`'  — code span opener or closer
//	'&'  — HTML entity opener
//	';'  — HTML entity closer
//	'<'  — raw HTML / autolink opener
//	'>'  — raw HTML / autolink closer
//	'['  — link/image text opener
//	'!'  — image opener (treated like '[' with CANBEIMAGE flag)
//	']'  — link/image text closer
//	'~'  — strikethrough/subscript opener/closer
//	'^'  — superscript opener/closer
//	'$'  — LaTeX math opener/closer
//	'|'  — spoiler opener/closer or table cell boundary
//	'='  — highlight (==) opener/closer
//	'@'  — permissive email autolink
//	':'  — permissive URL autolink
//	'.'  — permissive WWW autolink
//	'D'  — dummy mark (stores link dest/title, or split remainder)
type Mark struct {
	Beg   int  // start offset in block text
	End   int  // end offset in block text
	Prev  int  // previous mark index / resolved opener index
	Next  int  // next mark index / resolved closer index
	Ch    byte // mark character (see list above)
	Flags markFlags
}

// markFlags holds bit flags for a Mark, mirroring md4c's MD_MARK flags.
// Bits 0x01–0x10 are generic; bits 0x20–0x80 are type-specific
// (reused across different mark characters for different meanings).
type markFlags uint8

const (
	// Generic flags (apply to all mark types)
	markPotentialOpener markFlags = 1 << iota // 0x01 — may be an opener
	markPotentialCloser                       // 0x02 — may be a closer
	markOpener                                // 0x04 — confirmed opener (resolved)
	markCloser                                // 0x08 — confirmed closer (resolved)
	markResolved                              // 0x10 — resolved in any definitive way

	// Emphasis-specific flags (reuse bits 0x20–0x80)
	markEmphOC       markFlags = 0x20        // opener+closer hybrid candidate
	markEmphMod3_0   markFlags = 0x40        // length % 3 == 0
	markEmphMod3_1   markFlags = 0x80        // length % 3 == 1
	markEmphMod3_2   markFlags = 0x40 | 0x80 // length % 3 == 2
	markEmphMod3Mask markFlags = 0x40 | 0x80

	// Bracket-specific flags (reuse bits 0x20–0x80)
	markBracketCanBeImage markFlags = 0x20 // '[' preceded by '!'
	markBracketHasNested  markFlags = 0x40 // '[' contains resolved link
	markBracketFootnote   markFlags = 0x80 // '[^' footnote reference

	// Autolink flags (reuse bit 0x20)
	markAutolink              markFlags = 0x20 // for '<','>': is an autolink
	markAutolinkMissingMailto markFlags = 0x40 // for '@': missing mailto:

	// Permissive autolink validity flag (reuse bit 0x20)
	markValidPermissiveAutolink markFlags = 0x20 // for '@', ':', '.'
)

// markSentinelIndex is used as an invalid/empty mark index.
const markSentinelIndex = -1

// markStacks holds the 19 opener stacks for the Rule-of-3 emphasis algorithm
// and other inline mark pairing. Mirrors md4c's opener_stacks[19].
//
// Stack layout:
//
//	[0-5]   Asterisk: oo_mod3_0, oo_mod3_1, oo_mod3_2, oc_mod3_0, oc_mod3_1, oc_mod3_2
//	[6-11]  Underscore: oo_mod3_0, oo_mod3_1, oo_mod3_2, oc_mod3_0, oc_mod3_1, oc_mod3_2
//	[12]    Tilde 1 (subscript ~)
//	[13]    Tilde 2 (strikethrough ~~)
//	[14]    Bracket [ (links/images)
//	[15]    Dollar $ (LaTeX math)
//	[16]    Pipe || (spoiler)
//	[17]    Caret ^ (superscript)
//	[18]    Equal = (highlight ==)
type markStacks struct {
	stacks [19]int // top index for each stack; -1 = empty

	// marks slice holds all marks for the current block.
	// After a block is processed, the caller resets marks to [:0].
	marks []Mark

	// Unresolved link list (linked via opener.Prev), for resolveBrackets.
	// Mirrors md4c unresolved_link_head/tail.
	unresolvedLinkHead int
	unresolvedLinkTail int

	// linkAttrMap stores resolved link/image href+title by opener mark index.
	// Populated during resolveBracketLink, consumed during processInlines.
	linkAttrMap map[int]linkAttrs

	// refDefs is the reference definition map (shared across blocks).
	// Key: normalized label (string), Value: *RefDef.
	refDefs map[string]*RefDef

	// footnoteDefs is the footnote definition map (shared across blocks).
	// Key: normalized label (string), Value: *FootnoteDef.
	footnoteDefs      map[string]*FootnoteDef
	nextFootnoteIndex uint // 1-based counter for sequential numbering

	// labelNormBuf points to ctx.labelNormBuf — a per-parse reusable buffer
	// for normalizeLinkLabel. Set in context.reset(). Used inside markStacks
	// methods (resolveBracketFootnote, isLinkReference) to avoid per-link
	// allocation of the normalization buffer.
	labelNormBuf *[]byte

	// I36: C-34 — HTML horizon tracking for raw HTML inline detection optimization.
	// Mirrors md4c ctx.html_comment_horizon etc. (md4c.c:261-264).
	// When a scan for a closer fails, the horizon records how far the scan
	// went, allowing future scans to skip already-scanned ranges.
	htmlCommentHorizon   int
	htmlProcInstrHorizon int
	htmlDeclHorizon      int
	htmlCdataHorizon     int

	// Code span closer cache: avoids O(n²) re-scans for backtick runs.
	// Indexed by backtick run length; resized to match codespanMaxLen.
	// Initialized lazily per block via codeSpanCloserInit.
	lastCodeSpanClosers  []int
	codeSpanCloserInit   bool // false = cache needs per-block init
	codeSpanParagraphEnd bool // true = block fully scanned, no closer found

	// linkAttrSlots is a fixed-size inline array for link href/title storage.
	// 99%+ of leaf blocks have 0-1 links, and 4 slots cover virtually all cases.
	// When the block has ≤4 links, storeLinkAttrs writes directly to a slot
	// (no allocation). Only when >4 links per block does it fall back to
	// linkAttrMap (which is allocated lazily and cleared per-block).
	linkAttrSlots [4]linkAttrSlot
	linkAttrCount int
}

// Stack index constants for emphasis stacks.
const (
	asteriskOO0   = 0 // asterisk opener-only,  len%3==0
	asteriskOO1   = 1
	asteriskOO2   = 2
	asteriskOC0   = 3 // asterisk opener+closer, len%3==0
	asteriskOC1   = 4
	asteriskOC2   = 5
	underscoreOO0 = 6
	underscoreOO1 = 7
	underscoreOO2 = 8
	underscoreOC0 = 9
	underscoreOC1 = 10
	underscoreOC2 = 11
	tilde1        = 12 // single ~ (subscript)
	tilde2        = 13 // double ~~ (strikethrough)
	bracketStack  = 14 // [ and ![
	dollarStack   = 15 // $ (LaTeX)
	pipeStack     = 16 // || (spoiler)
	caretStack    = 17 // ^ (superscript)
	equalStack    = 18 // == (highlight)
)

// reset clears all stacks and marks for a new block.
func (ms *markStacks) reset() {
	for i := range ms.stacks {
		ms.stacks[i] = markSentinelIndex
	}
	ms.marks = ms.marks[:0]
	ms.unresolvedLinkHead = -1
	ms.unresolvedLinkTail = -1
	ms.linkAttrCount = 0
	ms.linkAttrMap = nil
	ms.codeSpanCloserInit = false
	ms.codeSpanParagraphEnd = false
	// Note: refDefs and footnoteDefs are NOT reset per-block — they persist across blocks
}

// lookupFootnoteDef looks up a footnote definition by normalized label.
func (ms *markStacks) lookupFootnoteDef(normalized []byte) *FootnoteDef {
	if ms.footnoteDefs == nil {
		return nil
	}
	return ms.footnoteDefs[string(normalized)]
}

// push pushes markIndex onto the given stack.
// The mark's Next field is used as the stack link.
func (ms *markStacks) push(stackIdx, markIndex int) {
	ms.marks[markIndex].Next = ms.stacks[stackIdx]
	ms.stacks[stackIdx] = markIndex
}

// pop pops the top mark index from the given stack.
// Returns markSentinelIndex if the stack is empty.
func (ms *markStacks) pop(stackIdx int) int {
	top := ms.stacks[stackIdx]
	if top >= 0 {
		ms.stacks[stackIdx] = ms.marks[top].Next
	}
	return top
}

// emphStack returns the opener stack index for an emphasis mark (* or _)
// based on its character and flags (OC, MOD3).
// Mirrors md4c md_emph_stack().
func emphStack(ch byte, flags markFlags) int {
	base := 0
	if ch == '_' {
		base = 6
	}
	if flags&markEmphOC != 0 {
		base += 3
	}
	switch flags & markEmphMod3Mask {
	case markEmphMod3_0:
		// base + 0
	case markEmphMod3_1:
		base++
	case markEmphMod3_2:
		base += 2
	}
	return base
}

// openerStack returns the opener stack index for the given mark.
// Mirrors md4c md_opener_stack().
func (ms *markStacks) openerStack(markIndex int) int {
	m := &ms.marks[markIndex]
	switch m.Ch {
	case '*', '_':
		return emphStack(m.Ch, m.Flags)
	case '~':
		if m.End-m.Beg == 1 {
			return tilde1
		}
		return tilde2
	case '!', '[':
		return bracketStack
	}
	return -1
}

// popOpeners removes all opener entries >= markIndex from every stack.
// Mirrors md4c md_pop_openers().
func (ms *markStacks) popOpeners(markIndex int) {
	for i := range ms.stacks {
		for ms.stacks[i] >= markIndex {
			ms.pop(i)
		}
	}
}

// addMark appends a new mark and returns its index.
func (ms *markStacks) addMark(beg, end int, ch byte, flags markFlags) int {
	idx := len(ms.marks)
	ms.marks = append(ms.marks, Mark{
		Beg:   beg,
		End:   end,
		Prev:  markSentinelIndex,
		Next:  markSentinelIndex,
		Ch:    ch,
		Flags: flags,
	})
	return idx
}

// resolveRange pairs opener and closer, setting resolved/opener/closer flags
// and cross-linking via Prev/Next. Mirrors md4c md_resolve_range().
func (ms *markStacks) resolveRange(openerIndex, closerIndex int) {
	o := &ms.marks[openerIndex]
	c := &ms.marks[closerIndex]
	o.Next = closerIndex
	c.Prev = openerIndex
	o.Flags |= markOpener | markResolved
	c.Flags |= markCloser | markResolved
}

// disableMarksInRange sets all marks in [from, to) to dummy marks,
// preventing them from participating in pairing.
// Mirrors md4c md_disable_marks().
func (ms *markStacks) disableMarksInRange(from, to int) {
	for i := from; i < to; i++ {
		ms.marks[i].Ch = 'D'
		ms.marks[i].Flags = 0
	}
}

// --- mark_char_map ---

// markCharMap is the default 256-entry lookup table for fast mark character
// detection. Mirrors md4c md_setup_mark_char_map() (md4c.c:2970-2983).
//
// This is the CommonMark base set: \ * _ ` & ; < > [ ] ! \0
// Note: space and newline are NOT in the base set (unlike previous versions).
// Space is only added when FlagCollapseWhitespace is set (md4c.c:2996-3003).
// Newline is not a mark char — it's handled as raw text between marks.
// Extensions add their characters via Registrar.AddMarkChar() at parser construction time.
var markCharMap [256]bool

func init() {
	for _, c := range []byte("\\*_`&;<>[]!") {
		markCharMap[c] = true
	}
	// NULL character
	markCharMap[0] = true
}

// buildMarkChars returns a per-parser mark character map for the given flags.
// Mirrors md4c md_setup_mark_char_map() (md4c.c:2970-2994).
// Used by New() and tests to ensure consistent mark char registration.
func buildMarkChars(flags Flags) [256]bool {
	mc := markCharMap
	if flags&FlagCollapseWhitespace != 0 {
		for _, c := range []byte(" \t\v\f") {
			mc[c] = true
		}
	}
	if flags&FlagPermissiveEmailAutolinks != 0 {
		mc['@'] = true
	}
	if flags&FlagPermissiveURLAutolinks != 0 {
		mc[':'] = true
	}
	if flags&FlagPermissiveWWWAutolinks != 0 {
		mc['.'] = true
	}
	if flags&FlagStrikethrough != 0 || flags&FlagSubscripts != 0 {
		mc['~'] = true
	}
	if flags&FlagLatexMathSpans != 0 {
		mc['$'] = true
	}
	if flags&FlagSuperscripts != 0 {
		mc['^'] = true
	}
	if flags&FlagHighlight != 0 {
		mc['='] = true
	}
	if flags&FlagWikilinks != 0 || flags&FlagTables != 0 || flags&FlagSpoilers != 0 {
		mc['|'] = true
	}
	return mc
}

// --- collectMarks ---

// collectMarks scans the lines of a leaf block and populates ms.marks
// with all potential inline marks. Mirrors md4c md_collect_marks().
//
// This is Phase 1 of the inline analysis pipeline (see md4c_ARCHITECTURE.md §5.4).
// Phases 2-3 (analyzeMarks/resolveBrackets) are in separate files.
//
// The blockText is assembled by joining the block's lines with '\n'.
// Each mark's Beg/End are offsets into blockText.
//
// markChars is the per-parser mark character map — extensions may have
// added characters beyond the CommonMark default set.
func collectMarks(ms *markStacks, blockText []byte, markChars *[256]bool, cc compatConfig, flags Flags) {
	ms.reset()

	// Pre-allocate mark slice capacity to avoid repeated growth during
	// append. One mark per ~8 bytes of text is a safe upper bound for
	// typical documents (most chars are not mark chars).
	if n := len(blockText) / 8; cap(ms.marks) < n {
		ms.marks = make([]Mark, 0, n)
	}

	off := 0
	size := len(blockText)

	// Line boundary cache: since collectMarks scans sequentially, marks on
	// the same line reuse cached boundaries without repeated backward scans.
	// curLineBeg/curLineEnd are invalidated when off moves outside [beg, end).
	curLineBeg, curLineEnd := 0, 0

	for off < size {
		// Fast skip: 4 characters at a time (loop unrolling, mirrors md4c)
		for off+3 < size &&
			!markChars[blockText[off]] &&
			!markChars[blockText[off+1]] &&
			!markChars[blockText[off+2]] &&
			!markChars[blockText[off+3]] {
			off += 4
		}
		for off < size && !markChars[blockText[off]] {
			off++
		}
		if off >= size {
			break
		}

		c := blockText[off]

		switch c {
		case '\\':
			// Backslash escape: if followed by ASCII punctuation, mark as resolved.
			// Mirrors md4c md_collect_marks() case '\\' (md4c.c:3273-3278).
			//
			// md4c has a guard: "Hard-break cannot be on the last line of the block."
			// In Go, blockText does NOT include a trailing \n for the last line,
			// so a backslash at the end of the last line would not match the
			// blockText[off+1]=='\n' check, naturally preventing this case.
			if off+1 < size && isASCIIPunct(blockText[off+1]) {
				ms.addMark(off, off+2, '\\', markResolved)
				off += 2
			} else if off+1 < size && blockText[off+1] == '\n' {
				// Hard line break via backslash + newline.
				// The mark covers both '\' and '\n' so that the newline
				// is consumed and won't also emit a SoftBR.
				ms.addMark(off, off+2, '\\', markResolved)
				off += 2
			} else {
				off++
			}

		case '*':
			fallthrough
		case '_':
			off = collectEmphMark(ms, blockText, off)

		case '`':
			off = collectCodeSpanMark(ms, blockText, off, cc)

		case '&':
			// Entity opener — will be matched with ';' closer.
			// Mirrors md4c md_collect_marks() case '&'.
			ms.addMark(off, off+1, '&', markPotentialOpener)
			off++

		case ';':
			// Entity closer — only if preceded by an '&' mark.
			if len(ms.marks) > 0 {
				last := &ms.marks[len(ms.marks)-1]
				if last.Ch == '&' {
					ms.addMark(off, off+1, ';', markPotentialCloser)
					off++
					continue
				}
			}
			off++

		case '<':
			// A potential autolink or raw HTML inline start/end.
			// Mirrors md4c md_collect_marks() case '<' (md4c.c:3390-3436).
			//
			// Priority:
			//   1. Raw HTML inline (isInlineHTML) — resolved immediately
			//   2. Autolink (isAutolink) — resolved immediately with AUTOLINK flag
			//   3. Neither — skip (literal '<')

			// I29: FlagNoHTMLSpans — skip raw HTML inline detection when flag set.
			// Mirrors md4c.c:3396: if(!(ctx->parser.flags & MD_FLAG_NOHTMLSPANS))
			if flags&FlagNoHTMLSpans == 0 {
				// 1. Check for raw HTML inline
				if htmlEnd, ok := isInlineHTML(ms, blockText, off); ok {
					// Raw HTML: create a resolved opener/closer pair.
					// Mirrors md4c.c:3406-3407: ADD_MARK('<',off,off,...) + ADD_MARK('>',html_end,html_end,...)
					openerIdx := ms.addMark(off, off, '<', markOpener|markResolved)
					closerIdx := ms.addMark(htmlEnd, htmlEnd, '>', markCloser|markResolved)
					ms.marks[openerIdx].Next = closerIdx
					ms.marks[closerIdx].Prev = openerIdx
					off = htmlEnd
					continue
				}
			}

			// 2. Check for autolink (always checked, even with NOHTMLSPANS)
			if result, ok := isAutolink(blockText, off); ok {
				flags := markOpener | markResolved | markAutolink
				if result.missingMailto {
					flags |= markAutolinkMissingMailto
				}
				// Autolink opener: beg=off, end=off+1 (the '<' character)
				// Autolink closer: beg=end-1, end=end (the '>' character)
				// Text between opener.end and closer.beg is the link destination.
				openerIdx := ms.addMark(off, off+1, '<', flags)
				closerIdx := ms.addMark(result.end-1, result.end, '>', markCloser|flags)
				ms.marks[openerIdx].Next = closerIdx
				ms.marks[closerIdx].Prev = openerIdx
				off = result.end
				continue
			}

			// 3. Neither — literal '<', skip
			off++

		case '>':
			// '>' is only consumed as part of autolink/HTML above.
			// A standalone '>' is literal text.
			off++

		case '[':
			// Link/image text opener.
			// Mirrors md4c md_collect_marks() case '[' and '!'.
			flags := markPotentialOpener
			// Check for '!' before '[' (image), but only if the '!' is not
			// part of a backslash escape. Mirrors md4c where backslash escape
			// is processed before the '!' check.
			if off > 0 && blockText[off-1] == '!' {
				// Check if this '!' was consumed by a preceding backslash escape
				isEscapedBang := false
				for j := len(ms.marks) - 1; j >= 0; j-- {
					if ms.marks[j].Ch == '\\' && ms.marks[j].End == off && ms.marks[j].Flags&markResolved != 0 {
						isEscapedBang = true
						break
					}
				}
				if !isEscapedBang {
					flags |= markBracketCanBeImage
				}
			}
			ms.addMark(off, off+1, '[', flags)
			// Add two dummy marks for link data (dest/title), mirrors md4c
			ms.addMark(off, off, 'D', 0)
			ms.addMark(off, off, 'D', 0)
			off++

		case '!':
			// Only interesting if followed by '[' (image).
			// We handle it here: when we see '![', the '[' handler above
			// will check for preceding '!'. The '!' itself doesn't get a mark.
			if off+1 < size && blockText[off+1] == '[' {
				// The '[' will be handled on next iteration and will
				// see the '!' at off-1 (relative to its position).
				// But we need to handle it now since the '[' is at off+1.
				off++ // skip '!'
				// Now blockText[off] == '['
				flags := markPotentialOpener | markBracketCanBeImage
				ms.addMark(off, off+1, '[', flags)
				ms.addMark(off, off, 'D', 0)
				ms.addMark(off, off, 'D', 0)
				off++
			} else {
				off++
			}

		case ']':
			// Link/image closer
			ms.addMark(off, off+1, ']', markPotentialCloser)
			off++

		case '~':
			if off < curLineBeg || off >= curLineEnd {
				curLineBeg, curLineEnd = findLineBounds(blockText, off)
			}
			off = collectTildeMark(ms, blockText, off, curLineBeg, flags)

		case '^':
			// Superscript: only single '^' is a delimiter; longer runs are literal.
			// Mirrors md4c md_collect_marks() case '^' (md4c.c:3550-3567):
			//   Cannot open before whitespace; cannot close after whitespace.
			tmp := off + 1
			for tmp < size && blockText[tmp] == '^' {
				tmp++
			}
			if tmp-off == 1 {
				if off < curLineBeg || off >= curLineEnd {
					curLineBeg, curLineEnd = findLineBounds(blockText, off)
				}
				lb := curLineBeg
				f := markPotentialOpener | markPotentialCloser
				// Cannot open if followed by whitespace or end of line
				// (md4c: off + 1 >= line->end || ISUNICODEWHITESPACE(off + 1))
				if tmp >= size || isWhitespaceAt(blockText, tmp) {
					f &^= markPotentialOpener
				}
				// Cannot close if preceded by whitespace or start of line
				// (md4c: off == line->beg || ISUNICODEWHITESPACEBEFORE(off))
				if off <= lb || isWhitespaceBefore(blockText, off) {
					f &^= markPotentialCloser
				}
				if f != 0 {
					ms.addMark(off, off+1, '^', f)
				}
			}
			off = tmp

		case '$':
			// LaTeX math: 1 or 2 '$' marks.
			// Mirrors md4c md_collect_marks() case '$' (md4c.c:3588-3600):
			//   Follows emphasis-like left/right-flanking rules.
			end := off + 1
			for end < size && blockText[end] == '$' {
				end++
			}
			if end-off > 2 {
				off = end
				continue
			}
			if off < curLineBeg || off >= curLineEnd {
				curLineBeg, curLineEnd = findLineBounds(blockText, off)
			}
			lb := curLineBeg
			f := markPotentialOpener | markPotentialCloser
			// Cannot open if preceded by non-whitespace non-punct (alphanumeric)
			// (md4c: off > line->beg && !ISUNICODEWHITESPACEBEFORE(off) && !ISUNICODEPUNCTBEFORE(off))
			if off > lb && !isWhitespaceBefore(blockText, off) && !isPunctBefore(blockText, off) {
				f &^= markPotentialOpener
			}
			// Cannot close if followed by non-whitespace non-punct (alphanumeric)
			// (md4c: tmp < line->end && !ISUNICODEWHITESPACE(tmp) && !ISUNICODEPUNCT(tmp))
			if end < size && !isWhitespaceAt(blockText, end) && !isPunctAt(blockText, end) {
				f &^= markPotentialCloser
			}
			if f != 0 {
				ms.addMark(off, end, '$', f)
			}
			off = end

		case '=':
			// Highlight: only exactly 2 '=' form a delimiter.
			// Mirrors md4c md_collect_marks() case '=' (md4c.c:3575-3590):
			//   Cannot open before whitespace; cannot close after whitespace.
			end := off + 1
			for end < size && blockText[end] == '=' {
				end++
			}
			if end-off == 2 {
				if off < curLineBeg || off >= curLineEnd {
					curLineBeg, curLineEnd = findLineBounds(blockText, off)
				}
				lb := curLineBeg
				f := markPotentialOpener | markPotentialCloser
				// Cannot open if followed by whitespace or end of line
				// (md4c: tmp >= line->end || ISUNICODEWHITESPACE(tmp))
				if end >= size || isWhitespaceAt(blockText, end) {
					f &^= markPotentialOpener
				}
				// Cannot close if preceded by whitespace or start of line
				// (md4c: off == line->beg || ISUNICODEWHITESPACEBEFORE(off))
				if off <= lb || isWhitespaceBefore(blockText, off) {
					f &^= markPotentialCloser
				}
				if f != 0 {
					ms.addMark(off, end, '=', f)
				}
			}
			off = end

		case '|':
			// Spoiler: '||' opener/closer, or table cell boundary, or wikilink label delimiter.
			// Mirrors md4c md_collect_marks() case '|' (md4c.c:3520-3542).
			if off+1 < size && blockText[off+1] == '|' {
				ms.addMark(off, off+2, '|', markPotentialOpener|markPotentialCloser)
				off += 2
			} else if flags&FlagWikilinks != 0 || flags&FlagTables != 0 || flags&FlagSpoilers != 0 {
				// Single '|': table cell boundary or wikilink label delimiter.
				// Mirrors md4c.c:3538: if((table_mode || (flags & MD_FLAG_WIKILINKS)) && ch == '|')
				// No potential opener/closer flags — this mark is used by resolveBracketWikilink
				// to find the | delimiter inside [[...]].
				ms.addMark(off, off+1, '|', 0)
				off++
			} else {
				off++
			}

		case '@':
			// Permissive email autolink candidate.
			// Mirrors md4c md_collect_marks() case '@' (md4c.c:3460-3472).
			// Only create mark if there's an alnum before '@' and at least 3
			// more characters after it (md4c requires off+3 < line->end).
			if off < curLineBeg || off >= curLineEnd {
				curLineBeg, curLineEnd = findLineBounds(blockText, off)
			}
			lineBeg, lineEnd := curLineBeg, curLineEnd
			if off > lineBeg && isAlnum(blockText[off-1]) && off+3 < lineEnd && isAlnum(blockText[off+1]) {
				ms.addMark(off, off+1, '@', markPotentialOpener)
				// Dummy stores line boundaries for analyzePermissiveAutolink.
				// Mirrors md4c: ADD_MARK('D', line->beg, line->end, 0)
				ms.addMark(lineBeg, lineEnd, 'D', 0)
			}
			off++

		case ':':
			// Permissive URL autolink candidate (http:// https:// ftp://).
			// Mirrors md4c md_collect_marks() case ':' (md4c.c:3474-3508).
			// Must verify both scheme prefix AND "//" suffix after ':'.
			// C checks scheme/suffix within line boundaries (line->beg/line->end).
			if off < curLineBeg || off >= curLineEnd {
				curLineBeg, curLineEnd = findLineBounds(blockText, off)
			}
			lineBeg, lineEnd := curLineBeg, curLineEnd
			if schemeInfo, ok := isSchemeWithSuffix(blockText, off, lineBeg, lineEnd); ok {
				// opener covers "scheme://" (from scheme start to after "//")
				ms.addMark(schemeInfo.beg, schemeInfo.end, ':', markPotentialOpener)
				// Dummy stores line boundaries
				ms.addMark(lineBeg, lineEnd, 'D', 0)
				off = schemeInfo.end
			}
			off++

		case '.':
			// Permissive WWW autolink candidate.
			// Mirrors md4c md_collect_marks() case '.' (md4c.c:3510-3524).
			// Must verify "www" prefix and boundary before it.
			if wwwInfo, ok := isWWWPrefixWithBoundary(blockText, off); ok {
				if off < curLineBeg || off >= curLineEnd {
					curLineBeg, curLineEnd = findLineBounds(blockText, off)
				}
				lineBeg, lineEnd := curLineBeg, curLineEnd
				// opener covers "www." (from 'w' start to after '.')
				ms.addMark(wwwInfo.beg, wwwInfo.end, '.', markPotentialOpener)
				// Dummy stores line boundaries
				ms.addMark(lineBeg, lineEnd, 'D', 0)
				off = wwwInfo.end
			}
			off++

		case ' ', '\t', '\v', '\f':
			// I29: COLLAPSEWHITESPACE — collapse runs of whitespace into single space.
			// Only active when the flag is set; otherwise space (which is in the
			// default mark char map) just falls through to skip.
			// Mirrors md4c md_collect_marks() (md4c.c:3648-3660).
			if flags&FlagCollapseWhitespace == 0 {
				off++
				continue
			}
			end := off + 1
			for end < size && isWhitespaceChar(blockText[end]) {
				end++
			}
			if end-off > 1 || c != ' ' {
				ms.addMark(off, end, ' ', markResolved)
			}
			off = end

		case 0:
			// NULL character — replaced with U+FFFD.
			ms.addMark(off, off+1, 0, markResolved)
			off++

		default:
			off++
		}
	}

	// Sentinel mark at the end — simplifies processInlines bounds checking.
	// Mirrors md4c's trailing sentinel ADD_MARK(127, ctx->size, ctx->size, MD_MARK_RESOLVED).
	ms.addMark(size, size, 127, markResolved)
}

// --- Emphasis mark collection ---

// collectEmphMark handles * and _ marks, computing left/right flank status
// and EMPH_MOD3 classification. Mirrors md4c md_collect_marks() case '*','_'.
func collectEmphMark(ms *markStacks, text []byte, off int) int {
	ch := text[off]

	// Count run length
	end := off + 1
	for end < len(text) && text[end] == ch {
		end++
	}
	runLen := end - off

	// Determine left-flanking and right-flanking per CommonMark §6.2.
	// left_level:  0=whitespace, 1=punctuation, 2=other
	// right_level: 0=whitespace, 1=punctuation, 2=other
	leftLevel := charFlankLevel(text, off, -1)   // char before the run
	rightLevel := charFlankLevel(text, end-1, 1) // char after the run

	// Intra-word underscore doesn't have special meaning.
	// md4c (md4c.c:3305-3308): if ch=='_' and both levels are 2, set both to 0.
	// This prevents marks like foo_bar_baz from being treated as emphasis.
	if ch == '_' && leftLevel == 2 && rightLevel == 2 {
		leftLevel = 0
		rightLevel = 0
	}

	var flags markFlags

	// Potentially an opener if left-flanking
	// CommonMark: left-flanking = not followed by whitespace AND
	//   (not followed by punctuation OR preceded by punctuation or whitespace)
	// md4c (md4c.c:3313-3316): right_level > 0 && right_level >= left_level
	if rightLevel != 0 { // not followed by whitespace
		if rightLevel == 1 { // followed by punctuation
			// Left-flanking if also preceded by punctuation or whitespace
			if leftLevel == 0 || leftLevel == 1 {
				flags |= markPotentialOpener
			}
		} else { // followed by other
			flags |= markPotentialOpener
		}
	}

	// Potentially a closer if right-flanking
	// CommonMark: right-flanking = not preceded by whitespace AND
	//   (not preceded by punctuation OR followed by punctuation or whitespace)
	// md4c (md4c.c:3313-3314): left_level > 0 && left_level >= right_level
	if leftLevel != 0 { // not preceded by whitespace
		if leftLevel == 1 { // preceded by punctuation
			// Right-flanking if also followed by punctuation or whitespace
			if rightLevel == 0 || rightLevel == 1 {
				flags |= markPotentialCloser
			}
		} else { // preceded by other
			flags |= markPotentialCloser
		}
	}

	// Set EMPH_OC if both opener and closer
	if flags&markPotentialOpener != 0 && flags&markPotentialCloser != 0 {
		flags |= markEmphOC
	}

	// Set MOD3 flags
	switch runLen % 3 {
	case 0:
		flags |= markEmphMod3_0
	case 1:
		flags |= markEmphMod3_1
	case 2:
		flags |= markEmphMod3_2
	}

	// Mark run length: md4c has NO limit on emphasis mark run length.
	// The old markMaxRunLen=16 was overly aggressive and caused underscores
	// inside emphasis spans (e.g., *___...___* in table cells) to be
	// truncated, losing content. The Rule-of-3 algorithm is already O(n)
	// and doesn't need truncation. md4c creates marks for the full run.
	// Pathological input protection is handled by the codespanMaxLen=1024
	// limit (for backtick runs) and the overall linear-time guarantees
	// of the emphasis algorithm.

	// Add the emphasis mark for the whole run, followed by (runLen-1) dummy
	// marks. The dummy marks are used by splitEmphMark() when the opener
	// and closer have different lengths (e.g., ***foo** → ** paired + * pushed).
	// Mirrors md4c md_collect_marks() case '*','_': ADD_MARK() + dummy marks.
	ms.addMark(off, end, ch, flags)
	for j := 1; j < runLen; j++ {
		ms.addMark(off, off, 'D', 0)
	}
	off = end

	return off
}

// --- Code span mark collection ---

// Maximum backtick run length for code spans. Guards against O(n²) pathological inputs.
const codespanMaxLen = 1024

// collectCodeSpanMark handles backtick marks for code spans.
// Immediately pairs opener with the closest matching closer. Caches
// intermediate backtick runs of other lengths to avoid O(n²) re-scans.
func collectCodeSpanMark(ms *markStacks, text []byte, off int, cc compatConfig) int {
	actualLen := 0
	for off+actualLen < len(text) && text[off+actualLen] == '`' {
		actualLen++
	}
	maxLen := cc.codespanMaxLen
	if maxLen == 0 {
		maxLen = codespanMaxLen
	}
	openerLen := actualLen
	if openerLen > maxLen {
		openerLen = maxLen
	}

	actualEnd := off + actualLen
	openerIdx := ms.addMark(off, off+openerLen, '`', markPotentialOpener|markPotentialCloser)

	// Lazy-init the closer cache for this block (once).
	if cap(ms.lastCodeSpanClosers) <= maxLen {
		ms.lastCodeSpanClosers = make([]int, maxLen+1)
	}
	cache := ms.lastCodeSpanClosers[:maxLen+1]
	if !ms.codeSpanCloserInit {
		for i := range cache {
			cache[i] = 0
		}
		ms.codeSpanCloserInit = true
	}

	// If paragraph was fully scanned and the cached closer is before us, skip.
	if ms.codeSpanParagraphEnd && openerLen < len(cache) && cache[openerLen] > 0 && cache[openerLen] < actualEnd {
		return actualEnd
	}

	for closerOff := actualEnd; closerOff < len(text); {
		if text[closerOff] != '`' {
			closerOff++
			continue
		}
		closerLen := 0
		for closerOff+closerLen < len(text) && text[closerOff+closerLen] == '`' {
			closerLen++
		}
		if closerLen == openerLen {
			closerEnd := closerOff + closerLen
			closerIdx := ms.addMark(closerOff, closerEnd, '`', markPotentialCloser)
			ms.resolveRange(openerIdx, closerIdx)
			return closerEnd
		}
		if closerLen > 0 && closerLen < len(cache) && closerOff > cache[closerLen] {
			cache[closerLen] = closerOff
		}
		closerOff += closerLen
	}

	ms.codeSpanParagraphEnd = true
	return actualEnd
}

// --- Tilde mark collection ---

// collectTildeMark handles ~ marks for strikethrough (~~ or ~) and subscript (~).
// Mirrors md4c md_collect_marks() case '~' (md4c.c:3592-3622).
//
// GFM Strikethrough flanking rules (default, md4c):
//   - Remove OPENER if preceded by non-whitespace AND non-punctuation ("other")
//   - Remove CLOSER if followed by non-whitespace AND non-punctuation ("other")
//
// This is more permissive than emphasis flanking: strikethrough can open/close
// adjacent to punctuation, while emphasis has stricter rules.
//
// When FlagStrikethroughPermissive is set, ~~ instead follows the CommonMark
// *-style flanking (no intraword restriction), matching cmark-gfm (the GFM
// reference implementation) and goldmark.
func collectTildeMark(ms *markStacks, text []byte, off int, lineBeg int, flags Flags) int {
	end := off + 1
	for end < len(text) && text[end] == '~' {
		end++
	}
	runLen := end - off

	if runLen == 1 && flags&FlagSubscripts != 0 {
		// Subscript: single ~ with whitespace-based flanking.
		// Mirrors md4c md4c.c:3602-3612: cannot open before whitespace; cannot close after whitespace.
		mFlags := markPotentialOpener | markPotentialCloser
		// Cannot open if followed by whitespace or end of line
		if end >= len(text) || isWhitespaceAt(text, end) {
			mFlags &^= markPotentialOpener
		}
		// Cannot close if preceded by whitespace or start of line
		if off <= lineBeg || isWhitespaceBefore(text, off) {
			mFlags &^= markPotentialCloser
		}
		if mFlags != 0 {
			ms.addMark(off, end, '~', mFlags)
		}
	} else if runLen <= 2 && flags&FlagStrikethrough != 0 {
		// Strikethrough: ~~ delimiters.
		leftLevel := charFlankLevel(text, off, -1)
		rightLevel := charFlankLevel(text, end-1, 1)

		var mFlags markFlags
		if flags&FlagStrikethroughPermissive != 0 {
			// Permissive flanking: ~~ behaves like * emphasis (CommonMark
			// left/right-flanking, NO intraword restriction). Matches the
			// GFM reference implementation (cmark-gfm) and goldmark, which
			// treat ~ as a *-style delimiter: canOpen = left-flanking,
			// canClose = right-flanking. This allows intraword strikethrough
			// such as "foo~~bar~~baz" -> "foo<del>bar</del>baz".
			//
			// The GFM spec text (§6.5) only says strikethrough is "delimited
			// by two tildes" and does not specify flanking for the intraword
			// case; both cmark-gfm and goldmark are permissive, while md4c
			// (the default below) applies a stricter intraword restriction.
			if rightLevel != 0 { // not followed by whitespace
				if rightLevel == 1 { // followed by punctuation
					if leftLevel == 0 || leftLevel == 1 { // preceded by ws/punct
						mFlags |= markPotentialOpener
					}
				} else { // followed by other
					mFlags |= markPotentialOpener
				}
			}
			if leftLevel != 0 { // not preceded by whitespace
				if leftLevel == 1 { // preceded by punctuation
					if rightLevel == 0 || rightLevel == 1 { // followed by ws/punct
						mFlags |= markPotentialCloser
					}
				} else { // preceded by other
					mFlags |= markPotentialCloser
				}
			}
		} else {
			// md4c strikethrough flanking (default): stricter than emphasis.
			// Mirrors md4c md4c.c:3610-3619.
			// Remove OPENER if preceded by non-whitespace non-punct ("other")
			// Remove CLOSER if followed by non-whitespace non-punct ("other")
			mFlags = markPotentialOpener | markPotentialCloser
			if leftLevel == 2 {
				mFlags &^= markPotentialOpener
			}
			if rightLevel == 2 {
				mFlags &^= markPotentialCloser
			}
		}
		if mFlags != 0 {
			ms.addMark(off, end, '~', mFlags)
		}
	}
	// runLen > 2 or no flags: no mark created (literal text)

	return end
}

// --- Helper functions ---

// charFlankLevel returns the "flank level" of the character adjacent to
// an emphasis run. Used to determine left-flanking / right-flanking status.
//
//	dir=-1: character before position off
//	dir=+1: character after position off
//
// Returns: 0=whitespace/eol, 1=punctuation, 2=other (alphanumeric, CJK, etc.)
// Mirrors md4c md_collect_marks() left_level / right_level logic (md4c.c:3290-3302).
//
// Unicode whitespace (Zs category + ASCII whitespace) and Unicode punctuation
// (P + S categories, matching md4c's md_is_unicode_punct__ / md_is_unicode_whitespace__).
func charFlankLevel(text []byte, off int, dir int) int {
	var pos int
	if dir < 0 {
		if off <= 0 {
			return 0 // boundary → whitespace
		}
		pos = off - 1
		// For multi-byte UTF-8, walk back to the lead byte
		for pos > 0 && !utf8.RuneStart(text[pos]) {
			pos--
		}
	} else {
		pos = off + 1
		if pos >= len(text) {
			return 0 // boundary → whitespace
		}
	}

	c := text[pos]
	// Fast path: ASCII
	if c < 0x80 {
		if c == ' ' || c == '\t' || c == '\n' || c == '\r' {
			return 0
		}
		if isASCIIPunct(c) {
			return 1
		}
		return 2
	}

	// Multi-byte UTF-8: decode the codepoint
	r, _ := utf8.DecodeRune(text[pos:])
	// Unicode whitespace: Zs category (matches md4c WHITESPACE_MAP)
	if isMd4cUnicodeWhitespace(r) {
		return 0
	}
	// Unicode punctuation: P + S categories (matches md4c PUNCT_MAP)
	if isMd4cUnicodePunct(r) {
		return 1
	}
	return 2
}

// isASCIIPunct returns true if c is an ASCII punctuation character.
// CommonMark defines punctuation as the set: !"#$%&'()*+,-./:;<=>?@[\]^_`{|}~
func isASCIIPunct(c byte) bool {
	return (c >= 0x21 && c <= 0x2F) || // !"#$%&'()*+,-./
		(c >= 0x3A && c <= 0x40) || // :;<=>?@
		(c >= 0x5B && c <= 0x60) || // [\]^_`
		(c >= 0x7B && c <= 0x7E) // {|}~
}

// schemeResult holds the result of permissive URL autolink scheme detection.
type schemeResult struct {
	beg int // start of scheme (e.g. position of 'h' in "http://")
	end int // position after "//" (e.g. position after "http://")
}

// isSchemeWithSuffix checks if the ':' at position off is preceded by
// http/https/ftp scheme AND followed by "//".
// Mirrors md4c md_collect_marks() case ':' (md4c.c:3474-3508).
// lineBeg/lineEnd are the current line boundaries (mirrors C's line->beg/line->end).
func isSchemeWithSuffix(text []byte, off, lineBeg, lineEnd int) (schemeResult, bool) {
	type schemeInfo struct {
		prefix string
		suffix string // must follow the ':'
	}
	schemes := []schemeInfo{
		{"http", "//"},
		{"https", "//"},
		{"ftp", "//"},
	}
	for _, s := range schemes {
		prefixLen := len(s.prefix)
		start := off - prefixLen
		// C checks: line->beg + scheme_size <= off (md4c.c:3495)
		if start < lineBeg {
			continue
		}
		// Check scheme prefix (case-sensitive, matching md4c md_ascii_eq)
		match := true
		for i := 0; i < prefixLen; i++ {
			if text[start+i] != s.prefix[i] {
				match = false
				break
			}
		}
		if !match {
			continue
		}
		// Check suffix "//" after ':'
		suffixStart := off + 1 // position after ':'
		// C checks: off + 1 + suffix_size < line->end (md4c.c:3496)
		if suffixStart+len(s.suffix) >= lineEnd {
			continue
		}
		suffixMatch := true
		for i := 0; i < len(s.suffix); i++ {
			if text[suffixStart+i] != s.suffix[i] {
				suffixMatch = false
				break
			}
		}
		if suffixMatch {
			return schemeResult{
				beg: start,
				end: suffixStart + len(s.suffix),
			}, true
		}
	}
	return schemeResult{}, false
}

// wwwResult holds the result of permissive WWW autolink detection.
type wwwResult struct {
	beg int // start of "www" (position of first 'w')
	end int // position after "." (off+1)
}

// isWWWPrefixWithBoundary checks if the '.' at position off is preceded by "www"
// and has a valid boundary before it (line start, whitespace, or punctuation).
// Mirrors md4c md_collect_marks() case '.' (md4c.c:3510-3524).
// C uses ISUNICODEWHITESPACEBEFORE and ISUNICODEPUNCTBEFORE for boundary check.
func isWWWPrefixWithBoundary(text []byte, off int) (wwwResult, bool) {
	start := off - 3
	if start < 0 {
		return wwwResult{}, false
	}
	if text[start] != 'w' || text[start+1] != 'w' || text[start+2] != 'w' {
		return wwwResult{}, false
	}
	// Check boundary before "www": must be line start, Unicode whitespace, or
	// Unicode punctuation. Mirrors md4c.c:3513:
	//   (off-3 == line->beg || ISUNICODEWHITESPACEBEFORE(off-3) || ISUNICODEPUNCTBEFORE(off-3))
	if start > 0 {
		if !isWhitespaceBefore(text, start) && !isPunctBefore(text, start) {
			return wwwResult{}, false
		}
	}
	return wwwResult{beg: start, end: off + 1}, true
}

// findLineBounds returns the start and end positions of the line containing
// position off in blockText. Lines are delimited by '\n'.
// Used by permissive autolink mark collection to store line boundaries
// in the dummy mark, mirroring md4c's line->beg and line->end.
func findLineBounds(blockText []byte, off int) (lineBeg, lineEnd int) {
	// Find start of current line
	lineBeg = off
	for lineBeg > 0 && blockText[lineBeg-1] != '\n' {
		lineBeg--
	}
	// Find end of current line
	lineEnd = off
	for lineEnd < len(blockText) && blockText[lineEnd] != '\n' {
		lineEnd++
	}
	return lineBeg, lineEnd
}

// isWhitespaceChar returns true for ASCII whitespace chars used by
// COLLAPSEWHITESPACE. Mirrors md4c ISWHITESPACE_ macro (md4c.c:348):
// space, tab, vertical tab, form feed (NOT newline/carriage return).
func isWhitespaceChar(c byte) bool {
	return c == ' ' || c == '\t' || c == '\v' || c == '\f'
}

// isWhitespaceAt checks if the byte at position off in text is Unicode whitespace.
// Handles both ASCII whitespace and multi-byte Unicode Zs category characters.
// Mirrors md4c ISUNICODEWHITESPACE macro (forward decode from off).
func isWhitespaceAt(text []byte, off int) bool {
	if off < 0 || off >= len(text) {
		return true // boundary counts as whitespace
	}
	c := text[off]
	if c < 0x80 {
		return c == ' ' || c == '\t' || c == '\n' || c == '\r'
	}
	// Multi-byte UTF-8: decode and check md4c WHITESPACE_MAP
	r, _ := utf8.DecodeRune(text[off:])
	return isMd4cUnicodeWhitespace(r)
}

// isWhitespaceBefore checks if the character immediately before position off
// is Unicode whitespace. Walks backwards to find the start of a multi-byte
// UTF-8 character. Mirrors md4c ISUNICODEWHITESPACEBEFORE macro, which uses
// md_decode_utf8_before__ (md4c.c:954-974) to decode the character ending
// at off-1.
func isWhitespaceBefore(text []byte, off int) bool {
	if off <= 0 {
		return true // boundary counts as whitespace
	}
	pos := off - 1
	// Walk back to the start of a multi-byte UTF-8 character.
	for pos > 0 && !utf8.RuneStart(text[pos]) {
		pos--
	}
	if pos >= len(text) {
		return true
	}
	c := text[pos]
	if c < 0x80 {
		return c == ' ' || c == '\t' || c == '\n' || c == '\r'
	}
	r, _ := utf8.DecodeRune(text[pos:])
	return isMd4cUnicodeWhitespace(r)
}

// isPunctAt checks if the byte at position off in text is Unicode punctuation.
// Handles both ASCII punctuation and multi-byte Unicode P+S categories.
// Mirrors md4c ISUNICODEPUNCT macro (forward decode from off).
func isPunctAt(text []byte, off int) bool {
	if off < 0 || off >= len(text) {
		return false
	}
	c := text[off]
	if c < 0x80 {
		return isASCIIPunct(c)
	}
	r, _ := utf8.DecodeRune(text[off:])
	return isMd4cUnicodePunct(r)
}

// isPunctBefore checks if the character immediately before position off
// is Unicode punctuation. Walks backwards to find the start of a multi-byte
// UTF-8 character. Mirrors md4c ISUNICODEPUNCTBEFORE macro, which uses
// md_decode_utf8_before__ (md4c.c:954-974) to decode the character ending
// at off-1.
func isPunctBefore(text []byte, off int) bool {
	if off <= 0 {
		return false
	}
	pos := off - 1
	// Walk back to the start of a multi-byte UTF-8 character.
	for pos > 0 && !utf8.RuneStart(text[pos]) {
		pos--
	}
	if pos >= len(text) {
		return false
	}
	c := text[pos]
	if c < 0x80 {
		return isASCIIPunct(c)
	}
	r, _ := utf8.DecodeRune(text[pos:])
	return isMd4cUnicodePunct(r)
}
