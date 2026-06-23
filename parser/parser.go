// Package parser implements the Markdown parser core.
//
// Architecture: two-pass + push model.
//   - Pass 1 (Parse): collect reference definitions with a discard renderer
//   - Pass 2 (Parse): full rendering with pre-populated refdefs
//   - Block close: emit events to renderer (unified incremental pipeline)
//
// Convert ([]byte) and ConvertStream (io.Reader) share the same
// parseLinesInternal core, only the LineSource differs. Blocks are closed
// and emitted as soon as they're complete.
package parser

import (
	"github.com/userpro/md4go/ast"
	"github.com/userpro/md4go/renderer"
	"github.com/userpro/md4go/stream"
)

// Parser is the reusable parser, analogous to md4c's MD_CTX.
type Parser struct {
	flags     Flags
	compat    compatConfig // pre-computed compatibility behavior decisions
	triggers  *triggerTable
	markChars [256]bool // per-parser mark character map (extensions can add chars)
}

// New creates a Parser with the given flags and optional extenders.
// Extenders are applied immediately, configuring triggers, mark characters,
// and flags before any Parse() call.
func New(flags Flags, extenders ...Extender) *Parser {
	p := &Parser{
		flags:     flags,
		compat:    newCompatConfig(flags),
		triggers:  newTriggerTable(),
		markChars: buildMarkChars(flags), // per-parser mark char map
	}

	for _, e := range extenders {
		e.Extend(p)
	}
	return p
}

// --- Registrar implementation (for Extender.Extend) ---

// RegisterBlockTrigger adds a block trigger for the given leading character.
func (p *Parser) RegisterBlockTrigger(c byte, trigger BlockTrigger) {
	p.triggers.register(c, trigger)
}

// AddMarkChar marks a byte as a potential inline mark character.
func (p *Parser) AddMarkChar(c byte) {
	p.markChars[c] = true
}

// Flags returns the current parser flags.
func (p *Parser) Flags() Flags { return p.flags }

// SetFlags adds the given flags to the parser's flag set.
func (p *Parser) SetFlags(f Flags) { p.flags |= f }

// HasMarkChar returns true if the byte is a registered mark character.
func (p *Parser) HasMarkChar(c byte) bool { return p.markChars[c] }

// discardRenderer absorbs all events, producing no output.
// Used in the first pass of two-pass parsing to collect forward refdefs.
type discardRenderer struct{}

func (discardRenderer) EnterBlock(ast.BlockType, any) error { return nil }
func (discardRenderer) LeaveBlock(ast.BlockType, any) error { return nil }
func (discardRenderer) EnterSpan(ast.SpanType, any) error   { return nil }
func (discardRenderer) LeaveSpan(ast.SpanType, any) error   { return nil }
func (discardRenderer) Text(ast.TextType, []byte) error     { return nil }
func (discardRenderer) Flush() error                        { return nil }

// Parse parses src ([]byte) and pushes events to r.
// Uses SliceSource internally — refdefs are fully visible in document order,
// so forward references work. Mirrors md4c md_parse() two-pass approach:
//   - Pass 1: scan all blocks, collect refdefs (mirrors md4c first pass)
//   - Pass 2: full rendering with pre-populated refDefs (mirrors md_process_all_blocks)
func (p *Parser) Parse(src []byte, r renderer.Renderer) error {
	// Pass 1: collect all refdefs (including forward references)
	// Mirrors md4c md_process_doc() first pass which builds the refdef hashtable
	// before md_process_all_blocks() does inline processing.
	preCtx := &context{}
	preCtx.reset()
	p.parseLinesInternal(stream.NewSliceSource(src), discardRenderer{}, preCtx)

	// Pass 2: full rendering with pre-collected refdefs
	ctx := &context{}
	ctx.reset()
	// Pre-populate refDefs from first pass so forward references resolve
	if len(preCtx.refDefs) > 0 {
		for k, v := range preCtx.refDefs {
			ctx.refDefs[k] = v
		}
		ctx.stk.refDefs = ctx.refDefs
	}
	// I33: Pre-populate footnoteDefs from first pass
	if len(preCtx.footnoteDefs) > 0 {
		for k, v := range preCtx.footnoteDefs {
			ctx.footnoteDefs[k] = v
		}
		ctx.stk.footnoteDefs = ctx.footnoteDefs
	}
	return p.parseLinesInternal(stream.NewSliceSource(src), r, ctx)
}

// ParseStream parses from a LineSource and pushes events to r.
// Uses any LineSource — typically ReaderSource for streaming.
// Refdefs are first-seen-first; forward references degrade to literal text.
// Mirrors md4c's incremental line-by-line architecture.
func (p *Parser) ParseStream(src stream.LineSource, r renderer.Renderer) error {
	ctx := &context{}
	ctx.reset()
	return p.parseLinesInternal(src, r, ctx)
}

// processFootnoteDefs emits footnote definitions that were referenced,
// in the order they were first referenced.
// Mirrors md4c md_process_footnote_defs() (md4c.c:7094-7119).
func (p *Parser) processFootnoteDefs(ctx *context, r renderer.Renderer) error {
	if len(ctx.footnoteDefs) == 0 {
		return nil
	}

	// Sort footnote defs by index (reference order) — mirrors md4c qsort by index.
	// Only emit footnotes that were actually referenced (index > 0).
	type indexedDef struct {
		index int
		def   *FootnoteDef
	}
	var sorted []indexedDef
	maxIndex := uint(0)
	for _, def := range ctx.footnoteDefs {
		if def.Index > 0 {
			sorted = append(sorted, indexedDef{index: int(def.Index), def: def})
			if def.Index > maxIndex {
				maxIndex = def.Index
			}
		}
	}

	if len(sorted) == 0 {
		return nil
	}

	// Simple insertion sort (small N expected)
	for i := 1; i < len(sorted); i++ {
		for j := i; j > 0 && sorted[j].index < sorted[j-1].index; j-- {
			sorted[j], sorted[j-1] = sorted[j-1], sorted[j]
		}
	}

	if err := r.EnterBlock(ast.BlockFootnoteDefSection, nil); err != nil {
		return err
	}

	for _, sd := range sorted {
		def := sd.def
		labelAttr := BuildAttribute([]byte(def.Label), 0)
		detail := &ast.FootnoteDefDetail{
			ID:       def.Index,
			RefCount: def.RefCount,
			Label:    labelAttr,
		}

		if err := r.EnterBlock(ast.BlockFootnoteDef, detail); err != nil {
			return err
		}

		// Process the footnote content lines as inline content.
		// Mirrors md4c md_process_footnote_def() (md4c.c:7069-7089):
		//   MD_ENTER_BLOCK(FOOTNOTE_DEF, &det)
		//   md_process_normal_block_contents(ctx, def->content_lines, def->n_content_lines)
		//   MD_LEAVE_BLOCK(FOOTNOTE_DEF, &det)
		if def.NContentLines > 0 {
			// Create a temporary block for the footnote content and process inlines
			tempBlock := &Block{
				Type:    ast.BlockP,
				NLines:  def.NContentLines,
				LineIdx: 0,
			}
			// Store content lines in a temporary line slice
			tempLines := make([]Line, def.NContentLines)
			copy(tempLines, def.ContentLines[:def.NContentLines])

			// Save and restore context's block/lines state
			savedBlk := ctx.blk
			ctx.blk = blockStack{}
			ctx.blk.current = tempBlock
			ctx.blk.lines = tempLines

			p.analyzeInlines(ctx, tempBlock)
			p.processInlines(ctx, tempBlock, r)

			// Restore
			ctx.blk = savedBlk
		}

		if err := r.LeaveBlock(ast.BlockFootnoteDef, detail); err != nil {
			return err
		}
	}

	return r.LeaveBlock(ast.BlockFootnoteDefSection, nil)
}

// parseLinesInternal is the unified core — reads lines from src, emits events to r.
// Mirrors md4c md_process_doc() (md4c.c:7122-7181).
//
// Flow:
//
//	MD_ENTER_BLOCK(DOC)
//	while(line := src.NextLine()):
//	    analyzeLine → processLine
//	    (processLine emits block events on close → inline analysis → text events)
//	endCurrentBlock         ← close any trailing open block
//	closeAllContainers      ← close remaining open containers (md_leave_child_containers)
//	MD_LEAVE_BLOCK(DOC)
func (p *Parser) parseLinesInternal(src stream.LineSource, r renderer.Renderer, ctx *context) error {

	// I29: Set parser flags on context for use in analyzeLine/triggers.
	ctx.flags = p.flags

	if err := r.EnterBlock(ast.BlockDoc, nil); err != nil {
		return err
	}

	// Two lineAnalysis buffers for alternating pivot/current (md4c pattern)
	var lineBufs [2]lineAnalysis
	pivot := &dummyBlankLine

	for {
		line, ok, err := src.NextLine()
		if err != nil {
			return err
		}
		if !ok {
			break
		}

		// Alternate between two buffers so pivot stays valid
		la := &lineBufs[0]
		if la == pivot {
			la = &lineBufs[1]
		}

		// First pass: analyze line type (md4c md_analyze_line)
		*la = p.analyzeLine(ctx, line, pivot)

		// Build/close blocks and emit events (md4c md_process_line)
		if err := p.processLine(ctx, la, r); err != nil {
			return err
		}

		pivot = la
	}

	// Close any remaining open leaf block.
	// Mirrors md4c md_end_current_block() after the main loop.
	if ctx.blk.current != nil {
		p.endBlock(ctx, r)
	}

	// Close any remaining open containers (innermost first).
	// Mirrors md4c md_leave_child_containers(ctx, 0).
	// I28: Now uses the general container stack (in container.go).
	p.leaveContainers(ctx, 0, r)

	// I33: Emit footnote definitions that were referenced, in reference order.
	// Mirrors md4c md_process_footnote_defs() (md4c.c:7094-7119).
	if p.flags&FlagFootnotes != 0 {
		if err := p.processFootnoteDefs(ctx, r); err != nil {
			return err
		}
	}

	return r.LeaveBlock(ast.BlockDoc, nil)
}
