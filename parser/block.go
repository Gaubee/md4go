package parser

import (
	"bytes"

	"github.com/userpro/md4go/ast"
	"github.com/userpro/md4go/renderer"
)

// LineType identifies the type of a line during first-pass analysis.
// Mirrors MD_LINETYPE in md4c.c.
type LineType uint8

const (
	LineBlank LineType = iota
	LineHR
	LineATXHeader
	LineSetextHeader
	LineSetextUnderline
	LineIndentedCode
	LineFencedCode
	LineHTML
	LineText
	LineTable
	LineTableUnderline
	LineBlockquote
	LineULItem
	LineOLItem
)

// lineAnalysis holds the result of analyzing a single line.
// Mirrors MD_LINE_ANALYSIS in md4c.c.
type lineAnalysis struct {
	lineType        LineType
	data            uint16 // ATX level, setext level, fence char, HTML block type, etc.
	fenceLen        int    // I36: C-35 — opening fence length (mirrors md4c.c:287 ctx->code_fence_length)
	enforceNewBlock bool
	content         []byte // the meaningful content of the line
	indent          int    // indentation level (for verbatim blocks)

	// Container analysis results (I28, mirrors md4c md_analyze_line):
	// nParents: how many existing containers this line belongs to
	// nBrothers: 1 if this is a new item in an existing list (sibling)
	// nChildren: how many new child containers to enter
	nParents  int
	nBrothers int
	nChildren int

	// newContainers holds info for brother/child containers being entered.
	// Multiple containers can be entered on a single line (e.g. `> - foo`
	// enters both a blockquote and a list). Mirrors md4c pushing containers
	// during md_analyze_line().
	newContainers []Container

	// isTask/taskMark are set when a task list marker [x]/[X]/[ ] is detected.
	isTask   bool
	taskMark byte

	// I37: Saved value of ctx.lastLineHasListLooseningEffect from the PREVIOUS
	// line, saved at the start of analyzeLine (before we reset/set it for the
	// current line). Mirrors md4c prev_line_has_list_loosening_effect (md4c.c:6497).
	prevLineHasListLooseningEffect bool
}

// dummyBlankLine is the default pivot line.
var dummyBlankLine = lineAnalysis{lineType: LineBlank}

// codeIndentOffset is the minimum indentation for an indented code block.
const codeIndentOffset = 4

// measureIndent measures the visual column offset of leading whitespace in line,
// expanding tabs per CommonMark rule (tab stop every 4 columns).
// Returns (visualColumn, byteOffset) where byteOffset points to the first
// non-whitespace character (or end of line).
// Mirrors md4c md_line_indent_measure() with total_indent=0.
func measureIndent(line []byte) (col int, off int) {
	return measureIndentFrom(line, 0)
}

// measureIndentFrom measures the visual column offset of leading whitespace in
// line, expanding tabs starting from startCol. This mirrors md4c's
// md_line_indentation(ctx, total_indent, beg, &off) which uses total_indent
// as the starting column for tab expansion.
//
// C-03b: This is critical for tab+container interactions. When a tab follows
// a '>' or list mark, it must be expanded from the column AFTER the mark,
// not from column 0. Otherwise tab stops are misaligned.
//
// Returns (relativeIndent, byteOffset) where relativeIndent is the number of
// visual columns added by the whitespace, and byteOffset points past the
// whitespace.
func measureIndentFrom(line []byte, startCol int) (relIndent int, off int) {
	indent := startCol
	for off < len(line) {
		if line[off] == ' ' {
			indent++
			off++
		} else if line[off] == '\t' {
			indent = (indent + 4) & ^3 // next tab stop at 4-column boundary
			off++
		} else {
			break
		}
	}
	return indent - startCol, off
}

// stripIndent removes the first n visual columns of indentation from a line,
// respecting tab stops. Used for indented code blocks where we strip 4 columns.
// Mirrors md4c md_resolve_line_max_indent().
func stripIndent(line []byte, nCols int) []byte {
	col := 0
	off := 0
	for off < len(line) && col < nCols {
		if line[off] == ' ' {
			col++
			off++
		} else if line[off] == '\t' {
			nextTab := (col + 4) & ^3
			if nextTab > nCols {
				// Tab spans past the strip point — replace with remaining spaces
				remaining := nCols - col
				result := make([]byte, 0, len(line)+remaining)
				for i := 0; i < remaining; i++ {
					result = append(result, ' ')
				}
				result = append(result, line[off+1:]...)
				return result
			}
			col = nextTab
			off++
		} else {
			break
		}
	}
	return line[off:]
}

// analyzeLine determines the type of a line by checking the priority chain.
// Mirrors md4c md_analyze_line() (md4c.c:6489-6975) — §5.2 priority chain.
//
// I28: Now uses n_parents/n_brothers/n_children for nested container support,
// matching md4c's algorithm exactly.
//
// Flow:
//  1. Measure visual indentation (expanding tabs)
//  2. Determine nParents by matching against existing container stack
//  3. Pivot-line inheritance (fenced code, HTML block continuation)
//  4. Blank line check (after stripping container marks)
//  5. Setext underline / HR / brother container / indented code / new child container
//  6. Table continuation / trigger table dispatch (ATX, fenced code, HTML block)
//  7. Table underline / indented code fallback / text fallback
//  8. Lazy continuation (if text and no brother/child)
func (p *Parser) analyzeLine(ctx *context, line []byte, pivot *lineAnalysis) lineAnalysis {
	la := lineAnalysis{}

	// I37: Save the loosening effect flag from the previous line BEFORE we
	// reset/set it for the current line. Mirrors md4c.c:6497:
	//   int prev_line_has_list_loosening_effect = ctx->last_line_has_list_loosening_effect;
	la.prevLineHasListLooseningEffect = ctx.lastLineHasListLooseningEffect

	// I29: FlagNoIndentedCodeBlocks — set codeIndent to effectively infinite
	// so indented code blocks are never detected.
	// Mirrors md4c.c:7207: ctx.code_indent_offset = (flags & NOINDENTEDCODE) ? (OFF)(-1) : 4
	codeIndent := codeIndentOffset
	if p.flags&FlagNoIndentedCodeBlocks != 0 {
		codeIndent = 1 << 30
	}

	// C-03b: Track total_indent for proper tab expansion inside containers.
	// Mirrors md4c.c:6492: unsigned total_indent = 0;
	// totalIndent accumulates the visual columns consumed by container marks,
	// so that tabs following container marks expand from the correct column.
	totalIndent := 0

	// Step 1: Measure initial indentation.
	// Mirrors md4c.c:6502-6503.
	relIndent, off := measureIndentFrom(line, totalIndent)
	indentCol := relIndent
	totalIndent += relIndent
	la.indent = indentCol

	// 1. Determine nParents: how many existing containers this line belongs to.
	// Mirrors md4c.c:6509-6535.
	// containerOff tracks the byte offset after all container marks,
	// before content indentation stripping — needed for HTML block verbatim output.
	containerOff := 0
	for la.nParents < ctx.nContainers() {
		c := &ctx.containers[la.nParents]

		if c.Ch == '>' && indentCol < codeIndent && off < len(line) && line[off] == '>' {
			// Blockquote mark — strip it.
			// Mirrors md4c.c:6516-6525.
			off++
			totalIndent++
			// Measure all whitespace after >, using totalIndent as starting column.
			// C-03b: This is critical — tabs after > must expand from the column
			// after >, not from column 0. Mirrors md4c.c:6518-6519.
			relIndent, relOff := measureIndentFrom(line[off:], totalIndent)
			off += relOff
			totalIndent += relIndent
			indentCol = relIndent
			// The optional 1st space after '>' is part of the block quote mark.
			// Mirrors md4c.c:6522-6523.
			if indentCol > 0 {
				indentCol--
			}
			containerOff = off
			la.nParents++
		} else if c.Ch != '>' && indentCol >= c.ContentsIndent {
			// List container — reduce indent by contents_indent.
			// Note: total_indent is NOT updated for list containers (md4c.c:6527-6529).
			containerOff = off
			indentCol -= c.ContentsIndent
			la.nParents++
		} else {
			break
		}
	}
	// If no container marks were consumed, containerOff stays at 0.
	// For HTML block types 1-5, we want the content from containerOff onward
	// (preserving indentation), not from off (which strips indentation).

	// Blank line: list containers stay open even without indentation.
	// Mirrors md4c.c:6537-6544.
	if off >= len(line) {
		if la.nBrothers+la.nChildren == 0 {
			// Keep list containers as parents even for blank lines
			for la.nParents < ctx.nContainers() && ctx.containers[la.nParents].Ch != '>' {
				la.nParents++
			}
		}
	}

	// 2. Pivot-line inheritance: fenced code continuation
	// Mirrors md4c.c:6548-6571.
	// Key: md4c checks closing fence BEFORE checking n_parents == n_containers.
	// When n_parents < n_containers (container ended), the closing fence
	// is still detected and the line becomes BLANK, not a new fenced code.
	if pivot.lineType == LineFencedCode {
		la.data = pivot.data
		la.fenceLen = pivot.fenceLen // I36: C-35 — carry fence length

		// Check closing fence regardless of n_parents (mirrors md4c.c:6553-6558).
		if indentCol < codeIndent {
			if isClosingCodeFence(line[off:], byte(pivot.data), pivot.fenceLen) {
				la.lineType = LineBlank
				// I37: Closing fence does NOT set list loosening effect.
				// Mirrors md4c.c:6556: ctx->last_line_has_list_loosening_effect = FALSE;
				ctx.lastLineHasListLooseningEffect = false
				return la
			}
		}

		// Then check if we're still inside the container (mirrors md4c.c:6562-6569).
		if la.nParents == ctx.nContainers() {
			la.lineType = LineFencedCode
			la.content = line[off:]
			// Store the current line's actual indentation (indentCol).
			// The opening fence's indentation is stored in the first verbatim line's Indent field.
			// Stripping is done in processLine by comparing against the fence indent.
			// Mirrors md4c.c:6561-6567.
			la.indent = indentCol
			return la
		}
		// n_parents < n_containers and no closing fence: fall through to other checks.
	}

	// 3. Pivot-line inheritance: HTML block continuation
	// Mirrors md4c.c:6573-6601.
	// I37: Now uses ctx.htmlBlockType (mirrors md4c ctx->html_block_type) to
	// track whether we're inside an HTML block. This is critical because:
	// - For types 1-5, blank lines are LineHTML continuation (not LineBlank)
	// - When the end condition is found on the start line, htmlBlockType is 0,
	//   so subsequent lines are analyzed normally (not forced to LineHTML)
	if pivot.lineType == LineHTML && ctx.htmlBlockType > 0 {
		if la.nParents < ctx.nContainers() {
			// HTML block is implicitly ended if the enclosing container ends.
			// Mirrors md4c.c:6575-6578.
			ctx.htmlBlockType = 0
			// Fall through to normal analysis
		} else {
			// Check end condition on this line.
			// Mirrors md4c.c:6582-6594.
			ht := ctx.htmlBlockType
			if htmlBlockEndCondition(line[off:], ht) {
				// End condition found — make sure this is the last line.
				// Mirrors md4c.c:6587: ctx->html_block_type = 0;
				ctx.htmlBlockType = 0

				// Types 6/7 end conditions serve as blank lines.
				// Mirrors md4c.c:6589-6594.
				if htmlBlockEndsAtBlank(ht) {
					la.lineType = LineBlank
					la.indent = 0
					return la
				}
				// Types 1-5: end condition found, but line is still LineHTML.
				// The block will be ended in processLine when the NEXT line
				// has a different type (since htmlBlockType is now 0).
			}

			// Line is HTML continuation (or end line for types 1-5).
			// Mirrors md4c.c:6597-6598.
			la.lineType = LineHTML
			la.data = uint16(ht)
			// For all HTML block types, preserve the raw content including
			// indentation. Use containerOff (byte offset after container marks
			// but before whitespace stripping) to preserve verbatim content.
			// This matches CommonMark spec: HTML block content is verbatim.
			co := containerOff
			if co < len(line) {
				la.content = line[co:]
			} else {
				la.content = line[off:]
			}
			la.nParents = ctx.nContainers()
			return la
		}
	}

	// 4. Blank line check
	if off >= len(line) {
		if pivot.lineType == LineIndentedCode && la.nParents == ctx.nContainers() {
			// Indented code block continuation: blank lines within code blocks
			// preserve their remaining whitespace after stripping 4 columns.
			// Mirrors md4c.c:6605-6611.
			la.lineType = LineIndentedCode
			la.content = line[off:] // empty for blank lines
			if indentCol > codeIndentOffset {
				la.indent = indentCol - codeIndentOffset
			} else {
				la.indent = 0
			}
			// Mirrors md4c.c:6611: blank lines inside indented code blocks do NOT
			// contribute to list loosening effect. Without this, a blank line inside
			// an indented code block nested in a list could incorrectly make the
			// list loose.
			ctx.lastLineHasListLooseningEffect = false
		} else {
			la.lineType = LineBlank
			// Set list loosening effect
			if la.nParents > 0 && la.nBrothers+la.nChildren == 0 {
				if ctx.containers[la.nParents-1].Ch != '>' {
					ctx.lastLineHasListLooseningEffect = true
				}
			}

			// C-04: Detect list item starting with two blank lines.
			// Mirrors md4c.c:6618-6637: when a blank line is inside a list item
			// (top block is LI) and the list item has no content block yet,
			// set the flag. This means the list item was followed immediately
			// by a blank line (i.e., the list item starts with blank lines).
			if la.nParents > 0 && ctx.containers[la.nParents-1].Ch != '>' &&
				la.nBrothers+la.nChildren == 0 &&
				ctx.blk.current == nil &&
				!ctx.containers[la.nParents-1].HasContent {
				ctx.lastListItemStartsWithTwoBlankLines = true
			}
		}
		return la
	}

	// C-04: Apply "list item starts with two blank lines" rule.
	// Mirrors md4c.c:6642-6664: when a non-blank line is encountered and
	// the flag is set, if we're still at the top of the list item, end it
	// by decrementing nParents (the line no longer belongs to this list item).
	if ctx.lastListItemStartsWithTwoBlankLines {
		if la.nParents > 0 && la.nParents == ctx.nContainers() &&
			ctx.containers[la.nParents-1].Ch != '>' &&
			la.nBrothers+la.nChildren == 0 &&
			ctx.blk.current == nil &&
			!ctx.containers[la.nParents-1].HasContent {
			// Leave the list item — this line goes back to the outer context
			la.nParents--
			// Re-adjust indent: add back the container's contents_indent
			// since we're no longer inside this container
			if la.nParents > 0 {
				ci := ctx.containers[la.nParents-1].ContentsIndent
				if la.indent > ci {
					la.indent -= ci
				} else {
					la.indent = 0
				}
			}
			// Rebuild indentCol from la.indent for subsequent analysis
			indentCol = la.indent
		}
		ctx.lastListItemStartsWithTwoBlankLines = false
	}

	// Reset list loosening effect for non-blank lines
	ctx.lastLineHasListLooseningEffect = false

	// Main analysis loop — mirrors md4c's while(TRUE) at md4c.c:6546.
	// After brother/child container detection, md4c uses `continue` to go
	// back to the top of the loop, re-checking HR, setext, etc. on the
	// remaining content. This is critical for cases like `- * * *` where
	// the HR check must run AFTER the list mark is consumed.
	//
	// piv tracks the effective pivot line. After a brother/child container
	// is detected, md4c sets pivot_line = &md_dummy_blank_line (md4c.c:6702,
	// 6771) so that setext/paragraph-interruption checks don't match.
	piv := pivot
	for {
		// 5. Setext underline (only if current block is paragraph at top container level)
		// Mirrors md4c.c:6670-6681.
		// Note: md4c does NOT check for potential refdef here. Instead, md4c allows
		// the setext underline to be detected, adds it to the block, and then handles
		// the priority in md_end_current_block() via refdef consumption (which runs
		// first and can suppress the entire block if all lines are consumed as refdefs).
		if indentCol < codeIndent && piv.lineType == LineText &&
			off < len(line) && (line[off] == '=' || line[off] == '-') &&
			la.nParents == ctx.nContainers() {
			c := line[off]
			level := 1
			if c == '-' {
				level = 2
			}
			if isSetextUnderline(line[off:], level) {
				la.lineType = LineSetextUnderline
				la.data = uint16(level)
				la.content = line[off:] // store underline text for refdef consumption
				return la
			}
		}

		// 6. HR (thematic break) — checked before container marks per md4c §5.2.
		// Mirrors md4c.c:6684-6692.
		if indentCol < codeIndent && off < len(line) && isHR(line[off:]) {
			la.lineType = LineHR
			return la
		}

		// 7. Check for "brother" container (new item in existing list).
		// Mirrors md4c.c:6696-6728.
		if la.nParents < ctx.nContainers() && la.nBrothers+la.nChildren == 0 {
			if c, contentOff, ok := isContainerMark(line, off, indentCol); ok {
				if isContainerCompatible(&ctx.containers[la.nParents], &c) {
					// Brother container found.
					// Mirrors md4c.c:6702: pivot_line = &md_dummy_blank_line
					piv = &dummyBlankLine
					off = contentOff
					// C-03b: Update totalIndent with the mark width.
					totalIndent += c.ContentsIndent - c.MarkIndent
					relIndent, relOff := measureIndentFrom(line[off:], totalIndent)
					off += relOff
					totalIndent += relIndent
					indentCol = relIndent

					if off >= len(line) {
						c.ContentsIndent++
					} else if indentCol <= codeIndent {
						c.ContentsIndent += indentCol
						indentCol = 0
					} else {
						c.ContentsIndent += 1
						indentCol--
					}

					ctx.containers[la.nParents].MarkIndent = c.MarkIndent
					ctx.containers[la.nParents].ContentsIndent = c.ContentsIndent
					// Update containerOff after indent stripping so HTML block
					// types 1-5 get content starting after the container mark + whitespace.
					ctx.containerOff = off
					la.newContainers = append(la.newContainers, c)
					la.nBrothers++
					continue // re-check HR, setext, etc. on remaining content
				}
			}
		}

		// 8. Check for indented code (cannot interrupt paragraph).
		// Mirrors md4c.c:6731-6737.
		if indentCol >= codeIndent && piv.lineType != LineText {
			la.lineType = LineIndentedCode
			la.content = line[off:]
			la.indent = indentCol - codeIndentOffset
			return la
		}

		// 9. Check for new child container (single per iteration).
		// Mirrors md4c.c:6740-6780 — md4c uses continue to loop for
		// multiple container marks, re-checking HR between each.
		if indentCol < codeIndent && off < len(line) {
			c, contentOff, ok := isContainerMark(line, off, indentCol)
			if ok {
				// Check paragraph interruption rules (only for first child container)
				if la.nChildren == 0 && piv.lineType == LineText && la.nParents == ctx.nContainers() {
					if contentOff >= len(line) && c.Ch != '>' {
						break // List mark followed by blank line cannot interrupt paragraph
					}
					if (c.Ch == '.' || c.Ch == ')') && c.Start != 1 {
						break // Ordered list with start != 1 cannot interrupt paragraph
					}
				}

				// Mirrors md4c.c:6770-6771: if n_brothers+n_children == 0, set pivot to dummy
				if la.nBrothers+la.nChildren == 0 {
					piv = &dummyBlankLine
				}
				// Mirrors md4c.c:6773-6774: leave excess containers before pushing child
				// (handled later in handleContainerTransitions via leaveContainers)

				off = contentOff
				totalIndent += c.ContentsIndent - c.MarkIndent
				relIndent, relOff := measureIndentFrom(line[off:], totalIndent)
				off += relOff
				totalIndent += relIndent
				indentCol = relIndent

				if off >= len(line) {
					c.ContentsIndent++
				} else if indentCol <= codeIndent {
					c.ContentsIndent += indentCol
					indentCol = 0
				} else {
					c.ContentsIndent += 1
					indentCol--
				}

				// Update containerOff after indent stripping so HTML block
				// types 1-5 get content starting after the container mark + whitespace.
				ctx.containerOff = off
				la.newContainers = append(la.newContainers, c)
				la.nChildren++
				continue // re-check HR, setext, etc. on remaining content
			}
		}

		break // no more container marks detected
	}

	// 9b. If content is blank after container mark detection, treat as blank.
	// This handles cases like `>` alone or `- ` alone where the container mark
	// consumes all content. Mirrors md4c behavior where such lines are blank
	// within the container (no paragraph is created).
	if off >= len(line) {
		la.lineType = LineBlank
		return la
	}

	// 10. Table continuation.
	tableContMatch := la.nParents == ctx.nContainers()
	if !tableContMatch && p.flags&FlagTableInterruptParagraph != 0 &&
		piv.lineType == LineTable && la.nBrothers+la.nChildren == 0 {
		tableContMatch = true
	}

	if piv.lineType == LineTable && tableContMatch {
		// FlagTableInterruptByHeaders: per GFM spec, a table is broken at
		// the start of any block-level structure (ATX heading, fenced code,
		// HTML block). HR and container marks are handled in steps 5-9.
		interrupted := p.flags&FlagTableInterruptByHeaders != 0 &&
			indentCol < codeIndent && off < len(line) &&
			canInterruptTable(line[off:], p.flags)

		if !interrupted {
			la.lineType = LineTable
			la.content = line[off:]
			if la.nParents != ctx.nContainers() {
				la.nParents = ctx.nContainers()
			}
			return la
		}
	}

	// 11. Trigger table dispatch for leaf block types (ATX, fenced code, HTML)
	// I37: Removed la.nParents == ctx.nContainers() condition — md4c checks these
	// block types regardless of container nesting level (md4c.c:6788-6833 has no
	// such condition). When nParents < nContainers, excess containers are left
	// in handleContainerTransitions before the block is processed.
	if indentCol < codeIndent && off < len(line) {
		for _, trigger := range p.triggers[line[off]] {
			if result, ok := trigger.Check(ctx, line, off, pivot); ok {
				result.nParents = la.nParents
				result.nBrothers = la.nBrothers
				result.nChildren = la.nChildren
				result.newContainers = la.newContainers
				// Preserve the indentation calculated so far.
				// For fenced code blocks, this is the opening fence's indentation,
				// which is used to strip continuation lines (md4c.c:6567-6571).
				if result.indent == 0 {
					result.indent = indentCol
				}
				return result
			}
		}
	}

	// 12. Table underline check
	//
	// C-05: When FlagTableInterruptParagraph is set (part of GoldmarkCompat),
	// also allow table detection during lazy continuation — i.e., when the
	// paragraph is continuing inside a container (list/blockquote) without
	// re-indentation. goldmark's paragraph transformer detects tables
	// regardless of container nesting, so we must relax the n_parents check
	// for lazy continuation cases.
	//
	// At this point, la.nParents has NOT yet been updated for lazy
	// continuation (that happens in step 15 below). So we check the lazy
	// continuation precondition here: pivot is text, and no brother/child
	// container was found in steps 7/9. If so, treat the line as if it's at
	// the current container level (matching what step 15 would set).
	parentMatch := la.nParents == ctx.nContainers()
	if !parentMatch && p.flags&FlagTableInterruptParagraph != 0 &&
		pivot.lineType == LineText && la.nBrothers+la.nChildren == 0 {
		parentMatch = true
	}

	if p.flags&FlagTables != 0 && pivot.lineType == LineText &&
		indentCol < codeIndent && off < len(line) &&
		(line[off] == '|' || line[off] == '-' || line[off] == ':') &&
		parentMatch &&
		ctx.blk.current != nil && ctx.blk.current.Type == ast.BlockP &&
		(ctx.blk.current.NLines == 1 || p.flags&FlagTableInterruptParagraph != 0) {
		if colCount, ok := isTableUnderline(line[off:]); ok {
			// Compat: validate column count match.
			// Default (lenient): skip validation (aligns with md4c).
			// FlagStrictTableColumns set = require header cell count <=
			// delimiter cell count (mirrors goldmark's extension/table.go
			// Transform() which rejects when header has MORE cells than
			// delimiter, but pads when header has fewer).
			headerLineIdx := ctx.blk.current.LineIdx + ctx.blk.current.NLines - 1
			headerLine := ctx.blk.lines[headerLineIdx].Text
			if validateTableColumns(headerLine, colCount, p.compat) {
				la.lineType = LineTableUnderline
				la.data = uint16(colCount)
				la.content = line[off:]
				// C-05: For lazy continuation, set nParents to match the
				// container level (same as step 15 would). This ensures
				// handleContainerTransitions treats the line as staying
				// inside the current container, so the table is emitted
				// inside the list/blockquote — matching goldmark.
				if la.nParents != ctx.nContainers() {
					la.nParents = ctx.nContainers()
				}
				return la
			}
			// Column mismatch: not a table, falls through to text continuation
		}
	}

	// 13. Indented code — paragraph continuation takes priority.
	// Mirrors md4c.c:6732 (indented code cannot interrupt paragraph).
	// C-03b: With totalIndent tracking, indentCol is correctly calculated.
	if indentCol >= codeIndent {
		if ctx.blk.current != nil && ctx.blk.current.Type == ast.BlockP {
			la.lineType = LineText
			la.content = line[off:]
			// Lazy continuation: if pivot is text and no brother/child found,
			// keep all containers as parents.
			// Mirrors md4c.c:6854-6857 (goto done reaches this after early return).
			if la.nBrothers+la.nChildren == 0 {
				la.nParents = ctx.nContainers()
			}
			return la
		}
		la.lineType = LineIndentedCode
		la.content = line[off:]
		la.indent = indentCol - codeIndentOffset
		return la
	}

	// 14. Normal text — default
	la.lineType = LineText
	la.content = line[off:]

	// 15. Lazy continuation: if pivot is text and no brother/child found,
	// keep all containers as parents.
	// Mirrors md4c.c:6854-6857.
	if pivot.lineType == LineText && la.nBrothers+la.nChildren == 0 {
		la.nParents = ctx.nContainers()
	}

	// 16. Task list detection (for brother/child containers)
	// Mirrors md4c.c:6860-6879. Uses the last container in newContainers.
	if p.flags&FlagTasklists != 0 && la.nBrothers+la.nChildren > 0 && len(la.newContainers) > 0 {
		tc := &la.newContainers[len(la.newContainers)-1]
		if tc.Ch != '>' {
			if tm, newOff, ok := detectTaskMark(line, off); ok {
				tc.IsTask = true
				tc.TaskMark = tm
				tc.TaskMarkOff = off + 1 // approximate
				la.isTask = true
				la.taskMark = tm
				off = newOff
				la.content = line[off:]
			}
		}
	}

	// 17. Admonition detection (for child blockquote containers).
	// Mirrors md4c.c:6948-6967: when n_children > 0, check if the last
	// new container is a blockquote and the line content is [!TYPE].
	// If matched, the blockquote becomes an admonition and the line is
	// treated as blank (the [!TYPE] tag doesn't produce content).
	if p.flags&FlagAdmonitions != 0 && la.nChildren > 0 && la.lineType == LineText {
		lastContainer := &la.newContainers[len(la.newContainers)-1]
		if lastContainer.Ch == '>' {
			if admType, ok := detectAdmonitionTag(la.content); ok {
				lastContainer.IsAdmonition = true
				lastContainer.AdmonitionType = admType
				la.lineType = LineBlank
				la.content = nil
			}
		}
	}

	return la
}

// processLine builds/closes blocks based on the line analysis.
// Mirrors md4c md_process_line() (md4c.c:6978-7100).
//
// I28: Container transitions are now handled via the container stack,
// using nParents/nBrothers/nChildren from lineAnalysis.
func (p *Parser) processLine(ctx *context, la *lineAnalysis, r renderer.Renderer) error {
	// Handle container (blockquote/list) transitions first
	if err := p.handleContainerTransitions(ctx, la, r); err != nil {
		return err
	}

	// Blank line ends current leaf block (but containers stay open if la.container != 0)
	if la.lineType == LineBlank {
		if ctx.blk.current != nil {
			p.endBlock(ctx, r)
		}
		return nil
	}

	// enforceNewBlock: HTML block end condition on this line, or fenced code start.
	// Mirrors md4c.c:6990-6991: if(line->enforce_new_block) md_end_current_block(ctx);
	if la.enforceNewBlock && ctx.blk.current != nil {
		p.endBlock(ctx, r)
	}

	// Setext underline: convert current paragraph to header
	// Mirrors md4c md_process_line() MD_LINE_SETEXTUNDERLINE (md4c.c:7005-7022):
	//   1. Change block type to H
	//   2. Add the underline line to the block (so refdef consumption can use it)
	//   3. Call endBlock (which runs consumeLinkRefDefs first)
	//   4. If all lines consumed as refdefs, pivot becomes blank
	//   5. If some lines remain, the underline is "downgraded" to text
	if la.lineType == LineSetextUnderline {
		if ctx.blk.current != nil && ctx.blk.current.Type == ast.BlockP {
			ctx.blk.current.Type = ast.BlockH
			ctx.blk.current.Data = la.data
			ctx.blk.current.Flags |= blockSetextHeader
		}
		// Add the underline line to the block (mirrors md4c.c:7011)
		// This allows consumeLinkRefDefs to use the underline as a potential
		// link destination line in a multi-line refdef like [f]:\n-
		if ctx.blk.current != nil {
			ctx.blk.addLine(la.content)
		}
		p.endBlock(ctx, r)
		// After endBlock, if block still exists (not all lines consumed by refdefs),
		// the setext underline is "downgraded" to start a new paragraph.
		// Mirrors md4c.c:7015-7020.
		if ctx.blk.current != nil {
			// The remaining lines form a paragraph (the setext heading was
			// partially consumed by refdefs). The current line acts as text.
			la.lineType = LineText
			// The block was already ended in endBlock, but current_block was
			// preserved (downgraded to paragraph). We just need to continue
			// with the current line as text for the next iteration.
		}
		return nil
	}

	// Table underline: convert current paragraph to table block.
	// Mirrors md4c.c:7024-7034. The first paragraph line becomes the header
	// row, and this underline becomes line 1. The pivot's type is changed to
	// LineTable so subsequent lines are treated as table rows.
	//
	// With FlagTableInterruptParagraph, a multi-line paragraph is split:
	// the first N-1 lines are emitted as a paragraph, and the last line is
	// promoted to the table header row. This mirrors goldmark's behavior.
	if la.lineType == LineTableUnderline {
		if ctx.blk.current != nil && ctx.blk.current.Type == ast.BlockP {
			if ctx.blk.current.NLines > 1 {
				// Split: emit first N-1 lines as paragraph,
				// promote last line to table header.
				// Pattern mirrors setext heading underline removal (endBlock).
				headerIdx := ctx.blk.current.LineIdx + ctx.blk.current.NLines - 1
				headerLine := ctx.blk.lines[headerIdx].Text

				ctx.blk.current.NLines--
				p.endBlock(ctx, r)

				ctx.blk.startNewBlock(LineTable, la.data, 0)
				ctx.blk.addLine(headerLine)
			} else {
				ctx.blk.current.Type = ast.BlockTable
				ctx.blk.current.Data = la.data
			}
		}
		ctx.blk.addLine(la.content)
		la.lineType = LineTable // so next line sees pivot as LineTable
		return nil
	}

	// HR and ATX header form single-line blocks
	if la.lineType == LineHR || la.lineType == LineATXHeader {
		if ctx.blk.current != nil {
			p.endBlock(ctx, r)
		}
		p.markListContent(ctx)
		ctx.blk.startNewBlock(la.lineType, la.data, la.fenceLen)
		ctx.blk.addLine(la.content)
		p.endBlock(ctx, r)
		return nil
	}

	// HTML block — verbatim block
	if la.lineType == LineHTML {
		if ctx.blk.current != nil && ctx.blk.current.Type != ast.BlockHTML {
			p.endBlock(ctx, r)
		}
		if ctx.blk.current == nil {
			p.markListContent(ctx)
			ctx.blk.startNewBlock(la.lineType, la.data, la.fenceLen)
		}
		ctx.blk.addVerbatimLine(la.content, 0)
		// I37: When htmlBlockType is 0, the block has ended (end condition was
		// found on this line or the start line). Close it immediately.
		// Mirrors md4c: when enforce_new_block or type change ends the block.
		if ctx.htmlBlockType == 0 {
			p.endBlock(ctx, r)
		}
		return nil
	}

	// Fenced code — verbatim block
	if la.lineType == LineFencedCode {
		if ctx.blk.current != nil && ctx.blk.current.Type != ast.BlockCode {
			p.endBlock(ctx, r)
		}
		if ctx.blk.current == nil {
			// First line of fenced code block (the opening fence).
			// la.indent is the fence indentation; store it for later stripping.
			p.markListContent(ctx)
			ctx.blk.startNewBlock(la.lineType, la.data, la.fenceLen)
			ctx.blk.addVerbatimLine(la.content, la.indent)
		} else {
			// Continuation line: strip indentation by the opening fence's indent.
			// Mirrors md4c.c:6567-6571:
			//   if(line->indent > pivot_line->indent) line->indent -= pivot_line->indent;
			//   else line->indent = 0;
			fenceIndent := ctx.blk.vlines[ctx.blk.current.LineIdx].Indent
			strippedIndent := 0
			if la.indent > fenceIndent {
				strippedIndent = la.indent - fenceIndent
			}
			ctx.blk.addVerbatimLine(la.content, strippedIndent)
		}
		return nil
	}

	// Indented code — verbatim block (content already stripped of 4-col indent)
	if la.lineType == LineIndentedCode {
		if ctx.blk.current != nil && ctx.blk.current.Type != ast.BlockCode {
			p.endBlock(ctx, r)
		}
		if ctx.blk.current == nil {
			p.markListContent(ctx)
			ctx.blk.startNewBlock(la.lineType, la.data, la.fenceLen)
		}
		ctx.blk.addVerbatimLine(la.content, la.indent)
		return nil
	}

	// Different type ends current block
	if ctx.blk.current != nil {
		if lineTypeToBlockType(la.lineType) != ctx.blk.current.Type {
			// I37: When leaving an HTML block via type change, reset htmlBlockType.
			// This handles the case where a non-HTML line follows an HTML block
			// that hasn't been closed yet (types 1-5 without end condition).
			if ctx.blk.current.Type == ast.BlockHTML {
				ctx.htmlBlockType = 0
			}
			p.endBlock(ctx, r)
		}
	}

	// Start new block if needed
	if ctx.blk.current == nil {
		p.markListContent(ctx)
		ctx.blk.startNewBlock(la.lineType, la.data, la.fenceLen)
	}

	// Add line to current block
	// Note: Fenced code lines are handled in the fenced code branch above,
	// so this section only handles indented code and normal text lines.
	if la.lineType == LineIndentedCode {
		ctx.blk.addVerbatimLine(la.content, la.indent)
	} else {
		ctx.blk.addLine(la.content)
	}

	return nil
}

// markListContent sets HasContent=true on the innermost list container.
// Mirrors md4c pushing a block onto block_bytes, which makes
// top_block->type != MD_BLOCK_LI for the C-04 check.
func (p *Parser) markListContent(ctx *context) {
	for i := len(ctx.containers) - 1; i >= 0; i-- {
		if ctx.containers[i].Ch != '>' {
			ctx.containers[i].HasContent = true
			break
		}
	}
}

// endBlock closes the current block and emits events.
// Mirrors md4c md_end_current_block() (md4c.c:5779-5821):
//   - First, try to consume link reference definitions from paragraph/setext blocks
//   - If all lines consumed, suppress block emission entirely
//   - For setext headers, remove the underline line (or downgrade to paragraph)
//   - Otherwise, emit the block normally
func (p *Parser) endBlock(ctx *context, r renderer.Renderer) {
	if ctx.blk.current == nil {
		return
	}
	b := ctx.blk.current

	// Try to consume link reference definitions before emitting.
	// Mirrors md4c md_end_current_block() lines 5790-5798:
	// only check paragraphs and setext headings whose first line starts with '['.
	nConsumed := p.consumeLinkRefDefs(ctx, b)
	if nConsumed > 0 && b.NLines == 0 {
		// All lines consumed by refdefs — suppress block emission entirely.
		// Mirrors md4c: "if(n == n_lines) { ctx->current_block = NULL; }"
		ctx.blk.endCurrentBlock()
		ctx.blk.compact()
		return
	}

	// Handle setext heading underline removal.
	// Mirrors md4c md_end_current_block() lines 5801-5813:
	// After refdef consumption, for setext headers:
	//   - If n_lines > 1: remove the last line (the underline)
	//   - If n_lines == 1: downgrade the block to a paragraph and return
	//     (the underline line becomes the start of a new paragraph)
	if b.Type == ast.BlockH && b.Flags&blockSetextHeader != 0 {
		if b.NLines > 1 {
			// Remove the underline (last line).
			// Mirrors md4c.c:5805-5807.
			b.NLines--
		} else {
			// Only the underline remains after eating the refdefs.
			// Downgrade the block to a paragraph and keep the line.
			// Mirrors md4c.c:5808-5812.
			b.Type = ast.BlockP
			// Don't emit yet — the current block stays open as a paragraph.
			// md4c returns early here, keeping current_block non-NULL.
			return
		}
	}

	ctx.blk.endCurrentBlock()

	// Suppress too sparse tables.
	// Mirrors md4c md_process_table_block_contents() (md4c.c:5440-5461):
	//   if(n_cols > 32 && n_rows > 4096/n_cols) {
	//       if(table_input_size / n_cols < n_rows / 4)
	//           block->type = MD_BLOCK_P;
	//   }
	// https://github.com/mity/md4c/issues/345
	if b.Type == ast.BlockTable {
		nCols := int(b.Data)
		nRows := b.NLines
		if nCols > 32 && nRows > 4096/nCols {
			tableInputSize := nRows
			for i := 0; i < nRows; i++ {
				tableInputSize += len(ctx.blk.lines[b.LineIdx+i].Text)
			}
			if tableInputSize/nCols < nRows/4 {
				b.Type = ast.BlockP
			}
		}
	}

	p.emitBlock(ctx, b, r)
	// Release line data after emission — prevents memory growth
	// in the streaming pipeline. Only compacts when no blocks are open.
	ctx.blk.compact()
}

// emitBlock emits enter/text/leave events for a completed block.
// Mirrors md4c md_process_leaf_block() — in tight lists, paragraph EnterBlock/LeaveBlock
// are suppressed (mirrors md4c.c:5494-5495, 5524-5525).
func (p *Parser) emitBlock(ctx *context, b *Block, r renderer.Renderer) {
	var detail any

	switch b.Type {
	case ast.BlockH:
		detail = &ast.HeadingDetail{Level: int(b.Data)}
	case ast.BlockCode:
		detail = p.buildCodeDetail(ctx, b)
	case ast.BlockTable:
		detail = &ast.TableDetail{
			ColCount:     int(b.Data),
			HeadRowCount: 1,
			BodyRowCount: b.NLines - 2,
		}
	}

	// I37: Always emit EnterBlock/LeaveBlock for paragraphs.
	// Tight list <p> tag suppression is now handled by the HTML renderer's
	// list content buffering mechanism (popListBuffer strips <p> for tight lists).
	// This replaces the old approach of skipping EnterBlock/LeaveBlock entirely,
	// which was incorrect because the list's tight/loose status might change
	// after paragraphs were already emitted.
	if err := r.EnterBlock(b.Type, detail); err != nil {
		return
	}

	switch b.Type {
	case ast.BlockH, ast.BlockP:
		// Inline-capable blocks: use the mark pipeline for inline analysis.
		// Mirrors md4c md_process_all_blocks() → md_analyze_inlines + md_process_inlines.
		// blockText is assembled once and passed to processBlockInlines, which runs
		// the full pipeline (collectMarks → resolve → emit) without re-assembling.
		blockText := p.assembleBlockText(ctx, b)
		p.processBlockInlines(ctx, blockText, r)
		// Reset marks for next block (memory does not accumulate across blocks)
		ctx.stk.reset()
	case ast.BlockTable:
		// Table block: process rows and cells.
		// Mirrors md4c md_process_table_block_contents().
		p.emitTable(ctx, b, r)
	case ast.BlockCode:
		// For fenced code (b.Data != 0), skip the first line (the fence).
		// Mirrors md4c md_process_code_block_contents().
		startLine := 0
		if b.Data != 0 { // fenced code
			startLine = 1
		}
		// Trim trailing blank lines from indented code blocks.
		// Per CommonMark: trailing blank lines of an indented code block
		// do not contribute to the output. Mirrors md4c behavior.
		endLine := b.NLines
		if b.Data == 0 { // indented code block
			for endLine > startLine {
				vl := ctx.blk.vlines[b.LineIdx+endLine-1]
				if len(bytes.TrimSpace(vl.Text)) > 0 {
					break
				}
				endLine--
			}
		}
		for i := startLine; i < endLine; i++ {
			vl := ctx.blk.vlines[b.LineIdx+i]
			for j := 0; j < vl.Indent; j++ {
				_ = r.Text(ast.TextCode, []byte{' '})
			}
			// I36: C-31 — use textWithNullReplacement for code block content,
			// mirroring md4c md_text_with_null_replacement() (md4c.c:410-437).
			p.textWithNullReplacement(r, ast.TextCode, vl.Text)
			_ = r.Text(ast.TextCode, []byte{'\n'})
		}
	case ast.BlockHTML:
		// Emit raw HTML content
		for i := 0; i < b.NLines; i++ {
			vl := ctx.blk.vlines[b.LineIdx+i]
			// I36: C-31 — use textWithNullReplacement for HTML block content.
			p.textWithNullReplacement(r, ast.TextHTML, vl.Text)
			_ = r.Text(ast.TextHTML, []byte{'\n'})
		}
	case ast.BlockHR:
		// no text content
	}

	_ = r.LeaveBlock(b.Type, detail)
}

// buildCodeDetail extracts the info string from the fence start line.
// Mirrors md4c md_setup_fenced_code_detail() (md4c.c:5386-5426).
func (p *Parser) buildCodeDetail(ctx *context, b *Block) *ast.CodeDetail {
	cd := &ast.CodeDetail{FenceChar: byte(b.Data)}
	if b.Data == 0 { // indented code
		return cd
	}
	if b.NLines == 0 {
		return cd
	}
	// First verbatim line is the fence line — extract info string
	fenceLine := ctx.blk.vlines[b.LineIdx]
	fenceChar := byte(b.Data)
	text := fenceLine.Text
	// Skip fence characters
	off := 0
	for off < len(text) && text[off] == fenceChar {
		off++
	}
	// Skip spaces
	for off < len(text) && (text[off] == ' ' || text[off] == '\t') {
		off++
	}
	info := text[off:]
	// Trim trailing whitespace
	info = bytes.TrimRight(info, " \t")
	// Extract language (first word)
	langEnd := 0
	for langEnd < len(info) && info[langEnd] != ' ' && info[langEnd] != '\t' {
		langEnd++
	}
	// Build attributes with entity/NULL char tracking.
	// Mirrors md4c md_build_attribute() calls in md_setup_fenced_code_detail()
	// (md4c.c:5409, 5425).
	langRaw := info[:langEnd]
	cd.Info = BuildAttribute(info, 0)
	if langEnd > 0 {
		cd.Lang = BuildAttribute(langRaw, 0)
	} else {
		cd.Lang = ast.EmptyAttribute
	}
	return cd
}

// --- Line type detection helpers ---

// parseATXHeaderContent checks if line is an ATX header.
// Returns (level, content, true) if it is.
// I29: flags controls PERMISSIVEATXHEADERS behavior (no space required after #).
func parseATXHeaderContent(line []byte, flags Flags) (level int, content []byte, ok bool) {
	i := 0
	for i < len(line) && line[i] == '#' && i < 6 {
		i++
	}
	if i == 0 || i > 6 {
		return 0, nil, false
	}
	level = i
	// I29: PERMISSIVEATXHEADERS — do not require space after # marks.
	// Mirrors md4c.c:5922-5924.
	if flags&FlagPermissiveATXHeaders == 0 {
		if i < len(line) && line[i] != ' ' && line[i] != '\t' {
			return 0, nil, false
		}
	}
	// Skip leading whitespace after # marks
	for i < len(line) && (line[i] == ' ' || line[i] == '\t') {
		i++
	}
	content = line[i:]
	// Strip trailing # sequence (must be preceded by space/tab)
	content = stripTrailingHashes(content, flags)
	return level, content, true
}

// stripTrailingHashes removes the optional closing '#' sequence.
// Per CommonMark §4.3: the closing '#' sequence must be preceded by
// one or more spaces/tabs. If the content becomes empty after stripping,
// return empty.
// I29: With PERMISSIVEATXHEADERS, trailing #s are always stripped (no whitespace
// required). Mirrors md4c.c:6902.
func stripTrailingHashes(content []byte, flags Flags) []byte {
	if len(content) == 0 {
		return content
	}
	// Trim trailing whitespace
	end := len(content)
	for end > 0 && (content[end-1] == ' ' || content[end-1] == '\t') {
		end--
	}
	if end == 0 {
		return content[:0]
	}
	// Check for trailing '#' run
	if content[end-1] == '#' {
		k := end
		for k > 0 && content[k-1] == '#' {
			k--
		}
		// The # run must be preceded by whitespace (or be at the very start
		// of content, meaning the header content is empty/only-#s)
		// With PERMISSIVEATXHEADERS, always strip trailing #s.
		if k == 0 || (content[k-1] == ' ' || content[k-1] == '\t') ||
			(flags&FlagPermissiveATXHeaders != 0) {
			// Strip the # run and any whitespace before it
			for k > 0 && (content[k-1] == ' ' || content[k-1] == '\t') {
				k--
			}
			return content[:k]
		}
	}
	return content[:end]
}

// canInterruptTable checks if the line starts a block-level structure that
// should interrupt table continuation per GFM spec. Only checks ATX headings,
// fenced code blocks, and HTML blocks — HR and container marks are already
// handled in steps 5-9 of analyzeLine.
func canInterruptTable(line []byte, flags Flags) bool {
	if len(line) == 0 {
		return false
	}
	switch line[0] {
	case '#':
		_, _, ok := parseATXHeaderContent(line, flags)
		return ok
	case '`', '~':
		_, _, _, ok := isFencedCodeStart(line)
		return ok
	case '<':
		_, ok := detectHTMLBlockStart(line)
		return ok
	}
	return false
}

// isHR checks if line is a thematic break.
// CommonMark §4.1: 3+ of -, *, _ with optional spaces.
func isHR(line []byte) bool {
	if len(line) == 0 {
		return false
	}
	c := line[0]
	if c != '-' && c != '*' && c != '_' {
		return false
	}
	count := 0
	for i := 0; i < len(line); i++ {
		if line[i] == c {
			count++
		} else if line[i] == ' ' || line[i] == '\t' {
			// spaces allowed
		} else {
			return false
		}
	}
	return count >= 3
}

// isFencedCodeStart checks if line starts a fenced code block.
// Returns (fenceChar, fenceLen, infoString, true) if it is.
// Mirrors md4c md_is_opening_code_fence() (md4c.c:6010-6038):
// for backtick fences, the info string must NOT contain any backticks.
// I36: C-35 — now returns fenceLen for precise closing fence detection,
// mirroring md4c ctx->code_fence_length (md4c.c:287, 6022).
func isFencedCodeStart(line []byte) (fenceChar byte, fenceLen int, info []byte, ok bool) {
	if len(line) == 0 {
		return 0, 0, nil, false
	}
	c := line[0]
	if c != '`' && c != '~' {
		return 0, 0, nil, false
	}
	off := 0
	for off < len(line) && line[off] == c {
		off++
	}
	if off < 3 {
		return 0, 0, nil, false
	}
	// I36: C-35 — fenceLen is the number of fence characters ONLY,
	// mirroring md4c ctx->code_fence_length (md4c.c:6022).
	fLen := off
	// Skip only spaces after fence characters (not tabs).
	// Mirrors md4c (md4c.c:6024-6026): tabs are part of the info string.
	for off < len(line) && line[off] == ' ' {
		off++
	}
	// Backtick-based fence must not contain '`' in the info string.
	// Mirrors md4c.c:6030-6032.
	if c == '`' {
		for i := off; i < len(line); i++ {
			if line[i] == '`' {
				return 0, 0, nil, false
			}
		}
	}
	return c, fLen, line[off:], true
}

// isClosingCodeFence checks if line is a closing code fence.
// I36: C-35 — now requires closing fence length >= openFenceLen,
// mirroring md4c (md4c.c:6050): if(off - beg < ctx->code_fence_length) goto out;
func isClosingCodeFence(line []byte, fenceChar byte, openFenceLen int) bool {
	if len(line) == 0 || line[0] != fenceChar {
		return false
	}
	// Count consecutive fence characters at the start of the line.
	// Mirrors md4c md_is_closing_code_fence() (md4c.c:6040-6064):
	//   while(off < ctx->size && CH(off) == ch) off++;
	count := 0
	for count < len(line) && line[count] == fenceChar {
		count++
	}
	// Closing fence must be at least as long as the opening fence.
	// Mirrors md4c (md4c.c:6050): if(off - beg < ctx->code_fence_length) goto out;
	if count < openFenceLen {
		return false
	}
	// Optionally, trailing spaces/tabs can follow.
	// Mirrors md4c (md4c.c:6053-6055):
	//   while(off < ctx->size && ISANYOF2(off, ' ', '\t')) off++;
	off := count
	for off < len(line) && (line[off] == ' ' || line[off] == '\t') {
		off++
	}
	// But nothing more is allowed on the line.
	// Mirrors md4c (md4c.c:6057-6059):
	//   if(off < ctx->size && !ISNEWLINE(off)) goto out;
	return off >= len(line)
}

// isBlankLine returns true if the line is empty or whitespace-only.
func isBlankLine(line []byte) bool {
	return len(bytes.TrimSpace(line)) == 0
}
