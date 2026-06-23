package parser

import (
	"bytes"

	"md4go/ast"
)

// refdef.go implements link reference definition and footnote definition detection.
// Mirrors md4c md_is_link_reference_definition() (md4c.c:2399-2510),
// md_is_footnote_definition() (md4c.c:2027-2125), and
// md_consume_link_reference_definitions() (md4c.c:5726-5777).

// FootnoteDef stores a footnote definition, analogous to md4c's MD_FOOTNOTE_DEF.
// Mirrors md4c.c:2013-2019 struct MD_FOOTNOTE_DEF_tag.
type FootnoteDef struct {
	Label         string // normalized label
	Index         uint   // 0=unreferenced; 1-based order of first reference
	RefCount      uint   // number of references to this footnote
	ContentLines  []Line // content lines of the footnote definition
	NContentLines int    // number of content lines
}

// consumeLinkRefDefs tries to consume link reference definitions from the
// beginning of a block's lines. Returns the number of lines consumed.
// Mirrors md4c md_consume_link_reference_definitions() (md4c.c:5726-5777).
func (p *Parser) consumeLinkRefDefs(ctx *context, b *Block) int {
	if b.NLines == 0 {
		return 0
	}
	// Only paragraphs and setext headings may contain link reference definitions.
	// ATX headings are excluded — they cannot consume refdefs.
	// Mirrors md4c md_consume_link_reference_definitions() (md4c.c:5790-5791):
	//   if(ctx->current_block->type == MD_BLOCK_P ||
	//      (ctx->current_block->type == MD_BLOCK_H && (ctx->current_block->flags & MD_BLOCK_SETEXT_HEADER)))
	if b.Type != ast.BlockP && !(b.Type == ast.BlockH && b.Flags&blockSetextHeader != 0) {
		return 0
	}
	firstLine := ctx.blk.lines[b.LineIdx].Text
	if len(firstLine) == 0 || firstLine[0] != '[' {
		return 0
	}

	nConsumed := 0
	for nConsumed < b.NLines {
		n := 0

		// I33: When footnotes are enabled, try footnote definition first for lines
		// starting with [^, so they are not accidentally consumed as link ref
		// definitions (which would also match [^label]: url).
		// Mirrors md4c md_consume_link_reference_definitions() (md4c.c:5736-5746).
		if p.flags&FlagFootnotes != 0 {
			lines := ctx.blk.lines[b.LineIdx : b.LineIdx+b.NLines]
			if nConsumed < len(lines) {
				line := lines[nConsumed].Text
				if len(line) >= 2 && line[0] == '[' && line[1] == '^' {
					n = p.isFootnoteDefinition(ctx, b, nConsumed)
				}
			}
		}

		if n == 0 {
			n = p.isLinkReferenceDefinition(ctx, b, nConsumed)
		}
		if n == 0 {
			break
		}
		nConsumed += n
	}

	if nConsumed > 0 {
		if nConsumed >= b.NLines {
			// All lines consumed — mark block as empty
			b.NLines = 0
		} else {
			// Partial consumption: advance LineIdx past consumed lines
			// instead of copying, because new lines may have been appended
			// after the block's original range (e.g., setext underline added
			// before endBlock, then refdef consumes the first line).
			b.LineIdx += nConsumed
			b.NLines -= nConsumed
		}
	}
	return nConsumed
}

// isLinkReferenceDefinition parses lines starting at startLine as a refdef.
// Returns the number of lines consumed (0 if not a refdef).
// Mirrors md4c md_is_link_reference_definition() (md4c.c:2399-2510).
func (p *Parser) isLinkReferenceDefinition(ctx *context, b *Block, startLine int) int {
	lines := ctx.blk.lines[b.LineIdx : b.LineIdx+b.NLines]
	if startLine >= len(lines) {
		return 0
	}

	// C-38: A link reference definition cannot start with 4+ spaces of indentation.
	// Per CommonMark §4.7: "The link reference definition does not correspond to
	// a paragraph start when it begins a line with 4 or more spaces."
	// Mirrors md4c.c:1612-1614: check label indentation < 4.
	firstLine := lines[startLine].Text
	indentLen := 0
	for indentLen < len(firstLine) && (firstLine[indentLen] == ' ' || firstLine[indentLen] == '\t') {
		indentLen++
	}
	if indentLen >= 4 {
		return 0
	}

	// Step 1: Parse link label [label]
	// labelBeg = offset of first char after '[', labelEnd = offset of ']'
	labelBeg, labelEnd, labelLines, ok := parseRefdefLabel(lines, startLine)
	if !ok {
		return 0
	}

	// lineIdx points to the line containing ']'
	lineIdx := startLine + labelLines - 1
	line := lines[lineIdx].Text
	off := labelEnd + 1 // skip ']'

	// Step 2: Parse colon ':' right after ']'
	if off >= len(line) {
		// Colon could be on next line (with optional whitespace before it)
		lineIdx++
		if lineIdx >= len(lines) {
			return 0
		}
		line = lines[lineIdx].Text
		off = 0
		for off < len(line) && (line[off] == ' ' || line[off] == '\t') {
			off++
		}
	}
	if off >= len(line) || line[off] != ':' {
		return 0
	}
	off++ // skip ':'

	// Step 3: Skip optional whitespace (at most one line break)
	off = skipRefdefWhitespace(lines, &lineIdx, off)
	if lineIdx >= len(lines) {
		return 0
	}
	line = lines[lineIdx].Text

	// Step 4: Parse link destination
	// C-38: Destination can be on the next line after colon, with optional
	// whitespace. Mirrors md4c md_is_link_reference_definition() (md4c.c:1596-1910).
	// If destination is not found on the current line, try the next line.
	destBeg, destEnd, destOff, destOk := parseRefdefDestination(line, off)
	if !destOk {
		// Try next line for destination
		if lineIdx+1 < len(lines) {
			lineIdx++
			line = lines[lineIdx].Text
			off = 0
			for off < len(line) && (line[off] == ' ' || line[off] == '\t') {
				off++
			}
			destBeg, destEnd, destOff, destOk = parseRefdefDestination(line, off)
		}
		if !destOk {
			return 0
		}
	}
	off = destOff

	// Save the line index where destination was parsed — needed later for href extraction.
	// After title parsing, lineIdx may advance past the destination line.
	destLineIdx := lineIdx

	// Step 5: Parse optional link title
	titleText, titleEndLineIdx, titleOk := parseRefdefTitle(lines, lineIdx, off)
	if titleOk {
		lineIdx = titleEndLineIdx
		if lineIdx < len(lines) {
			line = lines[lineIdx].Text
		}
	} else {
		// No title — rest of line after dest must be whitespace only.
		if off < len(line) {
			remaining := line[off:]
			if len(bytes.TrimRight(remaining, " \t")) > 0 {
				return 0
			}
		}
	}

	// Step 6: After title, verify remaining lines are consumed properly
	// C-38: After the title closing quote, only whitespace should follow on that line.
	// Also, after a refdef is consumed, the remaining lines should not form
	// unexpected block structures.
	// Per CommonMark: a refdef must be followed by a blank line or end of block.
	// Actually, the refdef can be followed by more content on subsequent lines
	// (that's handled by the loop in consumeLinkRefDefs).

	// Step 7: Check trailing content after the title closer.
	// The rest of the line after the title closer must be whitespace only.
	// (Already handled inside parseRefdefTitle.)

	// Step 8: Extract label text and normalize
	label := extractRefdefLabelText(lines, startLine, labelBeg, labelEnd, labelLines)
	if len(label) == 0 {
		return 0
	}
	normalized := normalizeLinkLabel(label)
	if len(normalized) == 0 {
		return 0
	}

	// Extract destination from the line where it was actually parsed.
	var href []byte
	if destLineIdx < len(lines) {
		href = lines[destLineIdx].Text[destBeg:destEnd]
		if len(href) >= 2 && href[0] == '<' && href[len(href)-1] == '>' {
			href = href[1 : len(href)-1]
		}
	}

	// Resolve backslash escapes in href and title per CommonMark
	href = resolveBackslashEscapes(href)
	resolvedTitle := resolveBackslashEscapes(titleText)

	// Store in refDefs (first definition wins per CommonMark)
	if _, exists := ctx.refDefs[string(normalized)]; !exists {
		ctx.refDefs[string(normalized)] = &RefDef{
			href:  append([]byte(nil), href...),
			title: append([]byte(nil), resolvedTitle...),
		}
	}

	// Return number of lines consumed
	return lineIdx - startLine + 1
}

// parseRefdefLabel parses [label] starting from lines[startLine].
// Returns (labelBeg, labelEnd, nLines, ok) where:
//   - labelBeg = offset of first char after '[' in lines[startLine]
//   - labelEnd = offset of ']' in lines[startLine+nLines-1]
//   - nLines = number of lines the label spans
func parseRefdefLabel(lines []Line, startLine int) (labelBeg, labelEnd, nLines int, ok bool) {
	if startLine >= len(lines) {
		return 0, 0, 0, false
	}
	line := lines[startLine].Text
	off := 0
	// Skip whitespace before [
	for off < len(line) && (line[off] == ' ' || line[off] == '\t') {
		off++
	}
	if off >= len(line) || line[off] != '[' {
		return 0, 0, 0, false
	}
	off++ // skip '['

	beg := off
	labelLen := 0
	nLines = 1

	for {
		if off >= len(line) {
			if startLine+nLines >= len(lines) {
				return 0, 0, 0, false
			}
			line = lines[startLine+nLines].Text
			off = 0
			nLines++
			// Mirrors md4c.c:2236: len++ for line break in label.
			// Line breaks count as 1 character toward the 999 limit.
			labelLen++
			if labelLen > 999 {
				return 0, 0, 0, false
			}
			continue
		}
		c := line[off]
		if c == ']' {
			end := off
			if labelLen == 0 || labelLen > 999 {
				return 0, 0, 0, false
			}
			return beg, end, nLines, true
		}
		if c == '[' {
			return 0, 0, 0, false
		}
		// Mirrors md4c.c:2193: backslash escape in labels supports both
		// ASCII punctuation AND newlines (ISPUNCT(off+1) || ISNEWLINE(off+1)).
		// All characters — including escape sequences — count as 1 toward
		// the 999 limit (md4c.c:2230: len++ is unconditional per iteration).
		if c == '\\' && off+1 < len(line) && (isASCIIPunctRefdef(line[off+1]) || line[off+1] == '\n') {
			off += 2
		} else {
			off++
		}
		labelLen++
		if labelLen > 999 {
			return 0, 0, 0, false
		}
	}
}

// parseRefdefDestination parses a link destination starting at off.
// Mirrors md4c md_is_link_destination_A/B.
func parseRefdefDestination(line []byte, off int) (int, int, int, bool) {
	if off >= len(line) {
		return 0, 0, off, false
	}
	for off < len(line) && (line[off] == ' ' || line[off] == '\t') {
		off++
	}
	if off >= len(line) {
		return 0, 0, off, false
	}

	if line[off] == '<' {
		off++
		destBeg := off
		for off < len(line) {
			c := line[off]
			if c == '\\' && off+1 < len(line) && isASCIIPunctRefdef(line[off+1]) {
				off += 2
				continue
			}
			// Mirrors md4c.c:2262: ISNEWLINE(off) || CH(off) == '<' → FALSE.
			// Control chars (including DEL 0x7F per md4c ISCNTRL) and '<' are rejected.
			if c == '<' || isCtrl(c) {
				return 0, 0, off, false
			}
			if c == '>' {
				destEnd := off
				off++
				return destBeg, destEnd, off, true
			}
			off++
		}
		return 0, 0, off, false
	}

	destBeg := off
	parenDepth := 0
	for off < len(line) {
		c := line[off]
		if c == '\\' && off+1 < len(line) && isASCIIPunctRefdef(line[off+1]) {
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
		} else if c == ' ' || c == '\t' {
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

// parseRefdefTitle parses an optional link title starting at off.
// Returns (titleText, finalLineIdx, ok) where finalLineIdx is the line index
// where the title ends (the line containing the closing quote).
func parseRefdefTitle(lines []Line, lineIdx int, off int) ([]byte, int, bool) {
	if lineIdx >= len(lines) {
		return nil, 0, false
	}
	line := lines[lineIdx].Text

	// Must have whitespace before title
	if off < len(line) && (line[off] == ' ' || line[off] == '\t') {
		for off < len(line) && (line[off] == ' ' || line[off] == '\t') {
			off++
		}
		// If we reached end of line after whitespace, try next line.
		// This handles cases like `      /url  \n           'the title'`
		// where trailing spaces on the destination line lead to EOL.
		if off >= len(line) {
			lineIdx++
			if lineIdx >= len(lines) {
				return nil, 0, false
			}
			line = lines[lineIdx].Text
			off = 0
			for off < len(line) && (line[off] == ' ' || line[off] == '\t') {
				off++
			}
		}
	} else if off >= len(line) {
		lineIdx++
		if lineIdx >= len(lines) {
			return nil, 0, false
		}
		line = lines[lineIdx].Text
		off = 0
		for off < len(line) && (line[off] == ' ' || line[off] == '\t') {
			off++
		}
	} else {
		return nil, 0, false
	}

	if off >= len(line) {
		return nil, 0, false
	}

	var closerChar byte
	switch line[off] {
	case '"':
		closerChar = '"'
	case '\'':
		closerChar = '\''
	case '(':
		closerChar = ')'
	default:
		return nil, 0, false
	}
	off++

	titleBeg := off
	startLineIdx := lineIdx
	nLines := 1

	for {
		if off >= len(line) {
			// C-38: Title cannot span blank lines.
			lineIdx++
			if lineIdx >= len(lines) {
				return nil, 0, false
			}
			nextLine := lines[lineIdx].Text
			if len(bytes.TrimRight(nextLine, " \t")) == 0 {
				return nil, 0, false
			}
			line = nextLine
			off = 0
			nLines = lineIdx - startLineIdx + 1
			continue
		}
		c := line[off]
		if c == '\\' && off+1 < len(line) && isASCIIPunctRefdef(line[off+1]) {
			off += 2
			continue
		}
		if c == closerChar {
			titleEnd := off
			off++
			// Check rest of line is whitespace only
			if off < len(line) {
				if len(bytes.TrimRight(line[off:], " \t")) > 0 {
					return nil, 0, false
				}
			}
			// Extract title text
			titleText := extractRefdefTitleText(lines, startLineIdx, titleBeg, titleEnd, nLines)
			return titleText, lineIdx, true
		}
		if closerChar == ')' && c == '(' {
			return nil, 0, false
		}
		off++
	}
}

// skipRefdefWhitespace skips whitespace and at most one line break.
func skipRefdefWhitespace(lines []Line, lineIdx *int, off int) int {
	line := lines[*lineIdx].Text
	for off < len(line) && (line[off] == ' ' || line[off] == '\t') {
		off++
	}
	if off >= len(line) && *lineIdx+1 < len(lines) {
		*lineIdx++
		line = lines[*lineIdx].Text
		off = 0
		for off < len(line) && (line[off] == ' ' || line[off] == '\t') {
			off++
		}
	}
	return off
}

// extractRefdefLabelText extracts the label text from the lines forming a refdef label.
func extractRefdefLabelText(lines []Line, startLine, beg, end, nLines int) []byte {
	if nLines == 1 {
		line := lines[startLine].Text
		if end > len(line) {
			end = len(line)
		}
		if beg >= end {
			return nil
		}
		return line[beg:end]
	}
	var buf []byte
	for i := 0; i < nLines; i++ {
		line := lines[startLine+i].Text
		if i == 0 {
			buf = append(buf, line[beg:]...)
		} else if i == nLines-1 {
			buf = append(buf, line[:end]...)
		} else {
			buf = append(buf, line...)
		}
		if i < nLines-1 {
			buf = append(buf, ' ')
		}
	}
	return buf
}

// extractRefdefTitleText extracts the title text from the lines forming a refdef title.
// C-38: Multi-line titles preserve newlines between lines (not spaces).
// Per CommonMark §4.7: "The title may contain whitespace and newlines"
// and the newline is included in the title text.
func extractRefdefTitleText(lines []Line, startLine, beg, end, nLines int) []byte {
	if nLines <= 1 {
		line := lines[startLine].Text
		if end > len(line) {
			end = len(line)
		}
		if beg >= end {
			return nil
		}
		return line[beg:end]
	}
	var buf []byte
	for i := 0; i < nLines; i++ {
		line := lines[startLine+i].Text
		if i == 0 {
			buf = append(buf, line[beg:]...)
		} else if i == nLines-1 {
			buf = append(buf, line[:end]...)
		} else {
			buf = append(buf, line...)
		}
		if i < nLines-1 {
			buf = append(buf, '\n') // preserve newline between lines
		}
	}
	return buf
}

// isASCIIPunctRefdef returns true for ASCII punctuation characters.
func isASCIIPunctRefdef(c byte) bool {
	return (c >= 0x21 && c <= 0x2F) || (c >= 0x3A && c <= 0x40) ||
		(c >= 0x5B && c <= 0x60) || (c >= 0x7B && c <= 0x7E)
}

// isFootnoteDefinition parses lines starting at startLine as a footnote definition.
// Returns the number of lines consumed (0 if not a footnote definition).
// Mirrors md4c md_is_footnote_definition() (md4c.c:2027-2125).
func (p *Parser) isFootnoteDefinition(ctx *context, b *Block, startLine int) int {
	lines := ctx.blk.lines[b.LineIdx : b.LineIdx+b.NLines]
	if startLine >= len(lines) {
		return 0
	}

	line := lines[startLine].Text
	off := 0

	// Must start with [^
	if len(line) < 3 || line[0] != '[' || line[1] != '^' {
		return 0
	}
	off = 2

	// Label: non-empty sequence of non-whitespace, non-bracket chars.
	// Mirrors md4c.c:2043-2049.
	labelBeg := off
	for off < len(line) && line[off] != ']' && !isWhitespace(line[off]) && line[off] != '[' {
		off++
	}
	labelEnd := off
	if labelEnd == labelBeg {
		return 0 // empty label
	}

	// Closing bracket.
	// Mirrors md4c.c:2051-2053.
	if off >= len(line) || line[off] != ']' {
		return 0
	}
	off++ // skip ']'

	// Colon.
	// Mirrors md4c.c:2056-2058.
	if off >= len(line) || line[off] != ':' {
		return 0
	}
	off++ // skip ':'

	// Skip optional whitespace after colon on the first line.
	// Mirrors md4c.c:2061-2063.
	for off < len(line) && (line[off] == ' ' || line[off] == '\t') {
		off++
	}

	// Count continuation lines. GitHub-style footnotes allow the rest of the
	// paragraph block to form the footnote body, including unindented lines.
	// Stop before a following line which itself starts a new footnote definition.
	// Mirrors md4c.c:2065-2088.
	n := startLine + 1
	for n < len(lines) {
		defLine := lines[n].Text
		if len(defLine) >= 3 && defLine[0] == '[' && defLine[1] == '^' {
			tmp := 2
			for tmp < len(defLine) && defLine[tmp] != ']' && !isWhitespace(defLine[tmp]) && defLine[tmp] != '[' {
				tmp++
			}
			if tmp > 2 && tmp+1 < len(defLine) && defLine[tmp] == ']' && defLine[tmp+1] == ':' {
				break
			}
		}
		n++
	}
	nLinesConsumed := n - startLine

	// Build content_lines array.
	// Line 0 content starts after the "[^label]: " prefix.
	// Lines 1..nLinesConsumed-1 are stored verbatim.
	// Mirrors md4c.c:2090-2109.
	var contentLines []Line
	if off >= len(line) && nLinesConsumed > 1 {
		// First line has no content after [^label]: — skip it
		contentLines = make([]Line, nLinesConsumed-1)
		copy(contentLines, lines[startLine+1:startLine+nLinesConsumed])
	} else {
		contentLines = make([]Line, nLinesConsumed)
		contentLines[0] = Line{Text: line[off:]}
		if nLinesConsumed > 1 {
			copy(contentLines[1:], lines[startLine+1:startLine+nLinesConsumed])
		}
	}

	// Extract and normalize label
	label := line[labelBeg:labelEnd]
	normalized := normalizeLinkLabel(label)
	if len(normalized) == 0 {
		return 0
	}

	// Store in footnoteDefs (first definition wins, matching md4c behavior)
	normalizedStr := string(normalized)
	if _, exists := ctx.footnoteDefs[normalizedStr]; !exists {
		ctx.footnoteDefs[normalizedStr] = &FootnoteDef{
			Label:         normalizedStr,
			ContentLines:  contentLines,
			NContentLines: len(contentLines),
		}
	}

	return nLinesConsumed
}
