package parser

import (
	"md4go/ast"
)

// Block represents a block-level element in the document structure.
// Mirrors MD_BLOCK in md4c.c — stored in a flat slice, NOT an object tree.
type Block struct {
	Type     ast.BlockType
	Flags    uint8
	Data     uint16 // header level, fence char, etc.
	FenceLen int    // I36: C-35 — opening fence length for precise closing detection (md4c.c:287)
	NLines   int
	// Index into blockStack.lines or blockStack.vlines for the first line.
	LineIdx int
}

// Block flags — mirrors md4c MD_BLOCK_* flags.
const (
	blockContainer       uint8 = 1 << 0
	blockContainerCloser uint8 = 1 << 1
	blockContainerOpener uint8 = 1 << 2
	blockLooseList       uint8 = 1 << 3
	blockSetextHeader    uint8 = 1 << 4
)

// Line represents a normal text line within a block.
// Stores the line bytes directly — works for both streaming and full-input.
type Line struct {
	Text []byte
}

// VerbatimLine represents a line in a code/HTML block.
type VerbatimLine struct {
	Text   []byte
	Indent int
}

// Container tracks blockquote/list nesting during line analysis.
// Mirrors MD_CONTAINER in md4c.c (md4c.c:5295-5306).
type Container struct {
	Ch             byte // '>' for blockquote, '-', '+', '*', '.', ')' for lists
	IsLoose        bool // true if list is loose (has blank lines between items)
	IsTask         bool // true if task list item [x]/[X]/[ ]
	Start          int  // OL start number
	MarkIndent     int  // indentation of the container mark
	ContentsIndent int  // indentation of content after the mark
	TaskMark       byte // task mark character: 'x', 'X', or ' '
	TaskMarkOff    int  // offset in input of the task mark char (between [ and ])
	IsAdmonition   bool // true if this is an admonition block (not a regular blockquote)
	AdmonitionType int  // 0=note, 1=tip, 2=important, 3=warning, 4=caution
	HasContent     bool // C-04: true if any leaf block was started inside this container
}

// blockStack holds all blocks and their lines in flat slices.
// Go equivalent of md4c's block_bytes contiguous memory region.
//
// Key M4 optimization: in the unified incremental pipeline, blocks are
// emitted as soon as they close. After emission, the block's line data
// is no longer needed. When no blocks are open, we compact the line
// storage to prevent memory from growing with document size.
type blockStack struct {
	blocks  []Block
	lines   []Line         // normal text lines
	vlines  []VerbatimLine // verbatim (code/HTML) lines
	current *Block         // block being built (nil if none)
}

// reset clears the block stack for reuse.
func (bs *blockStack) reset() {
	bs.blocks = bs.blocks[:0]
	bs.lines = bs.lines[:0]
	bs.vlines = bs.vlines[:0]
	bs.current = nil
}

// startNewBlock pushes a new Block and marks it as current.
// Mirrors md4c md_start_new_block().
func (bs *blockStack) startNewBlock(lineType LineType, data uint16, fenceLen int) {
	idx := len(bs.blocks)
	bs.blocks = append(bs.blocks, Block{
		Type:     lineTypeToBlockType(lineType),
		Data:     data,
		FenceLen: fenceLen, // I36: C-35
		NLines:   0,
		LineIdx:  len(bs.lines), // default to lines; vlines adjust on first add
	})
	bs.current = &bs.blocks[idx]
}

// addLine appends a normal text line to the current block.
func (bs *blockStack) addLine(text []byte) {
	bs.lines = append(bs.lines, Line{Text: text})
	if bs.current != nil {
		bs.current.NLines++
	}
}

// addVerbatimLine appends a verbatim line (code/HTML) to the current block.
func (bs *blockStack) addVerbatimLine(text []byte, indent int) {
	bs.vlines = append(bs.vlines, VerbatimLine{Text: text, Indent: indent})
	if bs.current != nil {
		if bs.current.NLines == 0 {
			bs.current.LineIdx = len(bs.vlines) - 1
		}
		bs.current.NLines++
	}
}

// endCurrentBlock marks the current block as complete.
func (bs *blockStack) endCurrentBlock() {
	bs.current = nil
}

// compact releases line data from already-emitted blocks.
// Called after a block is emitted when no other blocks are open.
// This is critical for the streaming pipeline: without compaction,
// line data accumulates in the slices and memory grows with document size.
//
// Safety: this is only safe to call when no blocks reference the
// existing line data (i.e., current is nil and all previous blocks
// have been emitted). New blocks start with LineIdx based on the
// current slice length, so after compaction they get correct indices.
func (bs *blockStack) compact() {
	// Only compact when no blocks are open
	if bs.current != nil {
		return
	}
	// Reset line slices — all previous block data has been consumed
	bs.lines = bs.lines[:0]
	bs.vlines = bs.vlines[:0]
	bs.blocks = bs.blocks[:0]
}

// lineTypeToBlockType maps a LineType to the corresponding BlockType.
func lineTypeToBlockType(lt LineType) ast.BlockType {
	switch lt {
	case LineHR:
		return ast.BlockHR
	case LineATXHeader, LineSetextHeader, LineSetextUnderline:
		return ast.BlockH
	case LineFencedCode, LineIndentedCode:
		return ast.BlockCode
	case LineHTML:
		return ast.BlockHTML
	case LineBlockquote:
		return ast.BlockQuote
	case LineULItem:
		return ast.BlockUL
	case LineOLItem:
		return ast.BlockOL
	case LineText:
		return ast.BlockP
	case LineTable, LineTableUnderline:
		return ast.BlockTable
	default:
		return ast.BlockP
	}
}
