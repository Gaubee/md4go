package parser

import (
	"bytes"

	"github.com/userpro/md4go/ast"
	"github.com/userpro/md4go/renderer"
)

// isTableUnderline checks if a line is a table underline (e.g., | --- | :--- | ---: | :---: |).
// Returns the column count and true if it is.
// Mirrors md4c md_is_table_underline() (md4c.c:5954-6007).
func isTableUnderline(line []byte) (colCount int, ok bool) {
	off := 0
	foundPipe := false

	// Optional leading pipe
	if off < len(line) && line[off] == '|' {
		foundPipe = true
		off++
		for off < len(line) && (line[off] == ' ' || line[off] == '\t') {
			off++
		}
	}

	for {
		delimited := false

		// Optional leading ':'
		if off < len(line) && line[off] == ':' {
			off++
		}
		// Required '-'
		if off >= len(line) || line[off] != '-' {
			return 0, false
		}
		for off < len(line) && line[off] == '-' {
			off++
		}
		// Optional trailing ':'
		if off < len(line) && line[off] == ':' {
			off++
		}

		colCount++

		// Skip whitespace
		for off < len(line) && (line[off] == ' ' || line[off] == '\t') {
			off++
		}

		// Optional pipe delimiter
		if off < len(line) && line[off] == '|' {
			delimited = true
			foundPipe = true
			off++
			for off < len(line) && (line[off] == ' ' || line[off] == '\t') {
				off++
			}
		}

		// End of line?
		if off >= len(line) {
			break
		}

		if !delimited {
			return 0, false
		}
	}

	if !foundPipe {
		return 0, false
	}

	return colCount, true
}

// analyzeTableAlignment extracts column alignments from a table underline.
// Mirrors md4c md_analyze_table_alignment() (md4c.c:5119-5142).
//
// index → align mapping (mirrors md4c's align_map):
//
//	0 = default (no colons)
//	1 = left (leading colon only)
//	2 = right (trailing colon only)
//	3 = center (both colons)
func analyzeTableAlignment(line []byte, nAlign int) []ast.Align {
	alignMap := []ast.Align{ast.AlignDefault, ast.AlignLeft, ast.AlignRight, ast.AlignCenter}
	aligns := make([]ast.Align, nAlign)
	off := 0

	for i := 0; i < nAlign && off < len(line); i++ {
		index := 0

		// Find next '-'
		for off < len(line) && line[off] != '-' {
			off++
		}
		if off >= len(line) {
			break
		}
		// Check for leading ':'
		if off > 0 && line[off-1] == ':' {
			index |= 1
		}
		// Skip dashes
		for off < len(line) && line[off] == '-' {
			off++
		}
		// Check for trailing ':'
		if off < len(line) && line[off] == ':' {
			index |= 2
		}

		aligns[i] = alignMap[index]
	}

	return aligns
}

// splitTableCells splits a table row by unescaped pipe characters,
// respecting backtick code spans and wikilink brackets.
// Leading/trailing pipes are stripped, and each cell is trimmed.
// Mirrors md4c's table_mode mark analysis which only treats unresolved
// single-| marks as cell boundaries.
//
// The cc parameter controls || protection behavior:
//   - cc.protectDoublePipe=false (default): || is split per GFM standard
//   - cc.protectDoublePipe=true: || is protected (md4go improvement)
func splitTableCells(line []byte, cc compatConfig) [][]byte {
	line = bytes.TrimSpace(line)

	// Build a set of byte positions that are "protected" from pipe splitting:
	// inside code spans and wikilinks.
	protected := make([]bool, len(line))

	// 1. Mark positions inside backtick code spans.
	skipCodeSpans(line, protected)

	// 2. Mark positions inside wikilinks [[...]].
	skipWikilinks(line, protected)

	// 3. Compat: protect || from cell splitting when FlagProtectDoublePipe is set.
	protectDoublePipeInCells(line, protected, cc.protectDoublePipe)

	var cells [][]byte
	cellStart := 0

	for i := 0; i < len(line); i++ {
		if protected[i] {
			continue
		}
		if line[i] == '\\' && i+1 < len(line) {
			i++ // skip escaped char
			continue
		}
		if line[i] == '|' {
			cells = append(cells, bytes.TrimSpace(line[cellStart:i]))
			cellStart = i + 1
		}
	}
	cells = append(cells, bytes.TrimSpace(line[cellStart:]))

	// If first cell is empty and line starts with |, it's a leading pipe
	if len(cells) > 1 && len(cells[0]) == 0 && len(line) > 0 && line[0] == '|' {
		cells = cells[1:]
	}

	// If last cell is empty and line ends with |, it's a trailing pipe
	if len(cells) > 1 && len(cells[len(cells)-1]) == 0 && len(line) > 0 && line[len(line)-1] == '|' {
		cells = cells[:len(cells)-1]
	}

	return cells
}

// skipCodeSpans marks positions inside backtick code spans as protected.
// Code spans use either `...` or “...“ etc, matching the CommonMark spec.
func skipCodeSpans(line []byte, protected []bool) {
	i := 0
	for i < len(line) {
		if line[i] == '`' {
			// Count opening backtick run
			openerLen := 0
			j := i
			for j < len(line) && line[j] == '`' {
				j++
				openerLen++
			}
			// Find matching closing run
			closeStart := j
			for closeStart < len(line) {
				k := closeStart
				closeLen := 0
				for k < len(line) && line[k] == '`' {
					k++
					closeLen++
				}
				if closeLen == openerLen {
					// Mark everything from opener to closer end as protected
					for p := i; p < k; p++ {
						protected[p] = true
					}
					i = k
					goto nextCodeSpan
				}
				closeStart++
			}
			// No closing run found; not a code span
			i++
		} else {
			i++
		}
	nextCodeSpan:
	}
}

// skipWikilinks marks positions inside [[...]] as protected.
// The | delimiter inside wikilinks should not be treated as cell boundary.
func skipWikilinks(line []byte, protected []bool) {
	i := 0
	for i < len(line) {
		if i+1 < len(line) && line[i] == '[' && line[i+1] == '[' {
			// Find matching ]]
			j := i + 2 // skip the opening [[
			for j < len(line) {
				if j+1 < len(line) && line[j] == ']' && line[j+1] == ']' {
					// Found matching ]]
					for p := i; p < j+2 && p < len(protected); p++ {
						protected[p] = true
					}
					i = j + 2
					goto nextWikilink
				}
				j++
			}
			// No closing ]] found
			i++
		} else {
			i++
		}
	nextWikilink:
	}
}

// skipSpoilers marks || (double pipe) positions as protected so they are
// not treated as table cell boundaries.
func skipSpoilers(line []byte, protected []bool) {
	for i := 0; i+1 < len(line); i++ {
		if line[i] == '|' && line[i+1] == '|' && !protected[i] {
			protected[i] = true
			protected[i+1] = true
		}
	}
}

// emitTable processes a table block and emits table events to the renderer.
// Mirrors md4c md_process_table_block_contents() (md4c.c:5226-5263).
//
// Structure:
//
//	<table>
//	  <thead><tr><th>header cell</th>...</tr></thead>
//	  <tbody><tr><td>body cell</td>...</tr>...</tbody>
//	</table>
//
// Line 0 = header row, line 1 = underline, lines 2+ = body rows.
func (p *Parser) emitTable(ctx *context, b *Block, r renderer.Renderer) {
	if b.NLines < 2 {
		return
	}

	// Read all lines into a local slice to avoid issues if ctx.blk.lines
	// is reallocated during cell inline processing.
	lines := make([][]byte, b.NLines)
	for i := 0; i < b.NLines; i++ {
		lines[i] = ctx.blk.lines[b.LineIdx+i].Text
	}

	colCount := int(b.Data)

	// Analyze alignment from underline (line 1)
	aligns := analyzeTableAlignment(lines[1], colCount)

	// Note: the outer EnterBlock/LeaveBlock for BlockTable is handled by
	// emitBlock, so we only emit the inner elements (THead/TBody/TR/TH/TD).

	// Header (THead)
	_ = r.EnterBlock(ast.BlockTHead, nil)
	p.emitTableRow(ctx, lines[0], colCount, aligns, ast.BlockTH, r)
	_ = r.LeaveBlock(ast.BlockTHead, nil)

	// Body (TBody)
	if b.NLines > 2 {
		_ = r.EnterBlock(ast.BlockTBody, nil)
		for i := 2; i < b.NLines; i++ {
			p.emitTableRow(ctx, lines[i], colCount, aligns, ast.BlockTD, r)
		}
		_ = r.LeaveBlock(ast.BlockTBody, nil)
	}
}

// emitTableRow emits a single table row (TR with cells).
// Mirrors md4c md_process_table_row() (md4c.c:5171-5224).
func (p *Parser) emitTableRow(ctx *context, line []byte, colCount int, aligns []ast.Align, cellType ast.BlockType, r renderer.Renderer) {
	cells := splitTableCells(line, p.compat)

	_ = r.EnterBlock(ast.BlockTR, nil)

	for k := 0; k < colCount; k++ {
		var align ast.Align
		if k < len(aligns) {
			align = aligns[k]
		}
		detail := &ast.TDDetail{Align: align}

		var cellText []byte
		if k < len(cells) {
			cellText = cells[k]
		}

		_ = r.EnterBlock(cellType, detail)
		if len(cellText) > 0 {
			p.processCellInline(ctx, cellText, r)
		}
		_ = r.LeaveBlock(cellType, detail)
	}

	_ = r.LeaveBlock(ast.BlockTR, nil)
}

// processCellInline processes a table cell's text as inline content.
// Uses the mark pipeline (collectMarks + analyzeMarks + resolveBrackets +
// analyzeLinkContents + processInlines) on the cell text.
//
// The cell text is temporarily added to ctx.blk.lines so that
// assembleBlockText can read it. The line is cleaned up by compact()
// after the table block is fully emitted.
func (p *Parser) processCellInline(ctx *context, cellText []byte, r renderer.Renderer) {
	lineIdx := len(ctx.blk.lines)
	ctx.blk.lines = append(ctx.blk.lines, Line{Text: cellText})

	tempBlock := &Block{
		Type:    ast.BlockP,
		NLines:  1,
		LineIdx: lineIdx,
	}

	// Reset marks for this cell
	ctx.stk.reset()

	// Process inline content
	p.analyzeInlines(ctx, tempBlock)
	p.processInlines(ctx, tempBlock, r)

	// Reset marks after processing
	ctx.stk.reset()
}
