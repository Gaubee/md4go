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
	"maps"

	"github.com/userpro/md4go/ast"
	"github.com/userpro/md4go/renderer"
	"github.com/userpro/md4go/stream"
)

// Parser is the reusable parser, analogous to md4c's MD_CTX.
type Parser struct {
	flags             Flags
	compat            compatConfig // pre-computed compatibility behavior decisions
	triggers          *triggerTable
	markChars         [256]bool // per-parser mark character map (extensions can add chars)
	tableProtectedBuf []bool // reusable buffer for splitTableCells

	// Streaming-continuation state (ParseStreamContinue / ParseStreamEnd).
	// When streamStarted is true, streamCtx holds the parser context across
	// Continue calls so block/container state survives chunk boundaries, and
	// streamPivot holds the previous line's analysis (analyzeLine depends on it).
	// This restores md4c's native md_parse() continuation semantics: a single
	// context is fed lines incrementally instead of one-shot documents.
	streamCtx     *context
	streamPivot   lineAnalysis
	streamStarted bool
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

// Parse parses src ([]byte) and pushes events to r.
// Uses SliceSource internally — refdefs are fully visible in document order,
// so forward references work. Mirrors md4c md_parse() two-pass approach:
//   - Pass 1: scan all blocks, collect refdefs (mirrors md4c first pass)
//   - Pass 2: full rendering with pre-populated refDefs (mirrors md_process_all_blocks)
//
// Contexts are pooled via sync.Pool to preserve mark slice capacity across
// Parse() calls, reducing GC pressure in repeated parsing scenarios.
func (p *Parser) Parse(src []byte, r renderer.Renderer) error {
	// Pass 1: collect all refdefs (including forward references)
	preCtx := getContext()
	preCtx.reset()
	err := p.parseLinesInternal(stream.NewSliceSource(src), discardRenderer{}, preCtx)

	// Pass 2: full rendering with pre-collected refdefs
	ctx := getContext()
	ctx.reset()
	copyRefDefsFrom(ctx, preCtx)

	if err == nil {
		err = p.parseLinesInternal(stream.NewSliceSource(src), r, ctx)
	}

	putContext(preCtx)
	putContext(ctx)
	return err
}

// ParseStream parses from a LineSource and pushes events to r.
// Uses any LineSource — typically ReaderSource for streaming.
// Refdefs are first-seen-first; forward references degrade to literal text.
// Mirrors md4c's incremental line-by-line architecture.
func (p *Parser) ParseStream(src stream.LineSource, r renderer.Renderer) error {
	ctx := getContext()
	ctx.reset()
	err := p.parseLinesInternal(src, r, ctx)
	putContext(ctx)
	return err
}

// ParseBlocksOnly parses src and emits only block-level events (EnterBlock /
// LeaveBlock), skipping the entire inline analysis pipeline. No Span events are
// emitted; only block structural identity is communicated.
//
// This is significantly faster than Parse + NullRenderer for use cases that
// only need document structure:
//   - Heading extraction / TOC generation
//   - Block type counting and statistics
//   - Syntax validation (checking if a document is parseable)
//   - Bulk pre-check pipelines (thousands of documents)
//
// The two-pass approach is retained (Pass 1 collects refdefs so refdef-only
// paragraphs are correctly suppressed as block content). Pass 2 processes
// blocks with ctx.noInline=true, skipping assembleBlockText and the full
// collectMarks → analyzeMarks → resolveBrackets → analyzeLinkContents →
// processInlines chain inside emitBlock.
//
// Code block TextCode and HTML block TextHTML events are still emitted
// (they don't go through the inline pipeline).
func (p *Parser) ParseBlocksOnly(src []byte, r renderer.Renderer) error {
	// Pass 1: collect all refdefs (so refdef paragraphs are correctly suppressed)
	preCtx := getContext()
	preCtx.reset()
	err := p.parseLinesInternal(stream.NewSliceSource(src), discardRenderer{}, preCtx)

	// Pass 2: block-level only — skip the inline pipeline
	ctx := getContext()
	ctx.reset()
	ctx.noInline = true
	copyRefDefsFrom(ctx, preCtx)

	if err == nil {
		err = p.parseLinesInternal(stream.NewSliceSource(src), r, ctx)
	}

	putContext(preCtx)
	putContext(ctx)
	return err
}

// ParseBlocksOnlyStream parses from a LineSource and emits only block-level events.
// Stream variant of ParseBlocksOnly — single pass, no refdef pre-collection.
// Refdefs are first-seen-first; forward references degrade to literal text.
func (p *Parser) ParseBlocksOnlyStream(src stream.LineSource, r renderer.Renderer) error {
	ctx := getContext()
	ctx.reset()
	ctx.noInline = true
	err := p.parseLinesInternal(src, r, ctx)
	putContext(ctx)
	return err
}

// copyRefDefsFrom copies reference and footnote definitions from src to dst,
// including the markStacks pointers so that inline resolution can find them.
func copyRefDefsFrom(dst, src *context) {
	if len(src.refDefs) > 0 {
		maps.Copy(dst.refDefs, src.refDefs)
		dst.stk.refDefs = dst.refDefs
	}
	if len(src.footnoteDefs) > 0 {
		maps.Copy(dst.footnoteDefs, src.footnoteDefs)
		dst.stk.footnoteDefs = dst.footnoteDefs
	}
}

// ParseStreamContinue feeds lines from src to a streaming-continuation parse,
// emitting events for any blocks that close during this call. State (open
// containers, the in-progress leaf block, the previous line's analysis) is
// preserved across Continue calls on the same Parser, so a code block or
// paragraph split across chunk boundaries is parsed as a single structure.
//
// The first Continue call opens the document (EnterBlock Doc); subsequent calls
// reuse the existing context. The stream is finalized by ParseStreamEnd.
func (p *Parser) ParseStreamContinue(src stream.LineSource, r renderer.Renderer) error {
	firstCall := !p.streamStarted
	if firstCall {
		p.streamCtx = getContext()
		p.streamCtx.reset()
		p.streamPivot = dummyBlankLine
		p.streamStarted = true
		if err := r.EnterBlock(ast.BlockDoc, nil); err != nil {
			putContext(p.streamCtx)
			p.streamCtx = nil
			p.streamStarted = false
			p.streamPivot = dummyBlankLine
			return err
		}
	}
	p.streamCtx.flags = p.flags
	return p.runLineLoop(src, r, p.streamCtx, &p.streamPivot, firstCall)
}

// ParseStreamEnd finalizes a streaming-continuation parse started by
// ParseStreamContinue and clears the continuation state so the Parser can
// start a fresh stream.
func (p *Parser) ParseStreamEnd(r renderer.Renderer) error {
	if !p.streamStarted {
		return nil
	}
	ctx := p.streamCtx
	err := p.finalizeStream(ctx, r)
	putContext(ctx)
	p.streamCtx = nil
	p.streamStarted = false
	p.streamPivot = dummyBlankLine
	return err
}

// InProtectedBlock reports whether the streaming-continuation parse is
// currently inside a verbatim block (fenced/indented code block or HTML block).
func (p *Parser) InProtectedBlock() bool {
	if !p.streamStarted || p.streamCtx == nil {
		return false
	}
	ctx := p.streamCtx
	if ctx.htmlBlockType > 0 {
		return true
	}
	if cur := ctx.blk.current; cur != nil {
		switch cur.Type {
		case ast.BlockCode, ast.BlockHTML:
			return true
		}
	}
	return false
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
			// Assemble footnote content text directly from content lines,
			// without save/restore of ctx.blk. processBlockInlines accepts
			// pre-assembled blockText, so no temp block is needed.
			blockText := assembleTextFromLines(def.ContentLines[:def.NContentLines], &ctx.blockTextBuf)

			// Reset marks for footnote processing
			ctx.stk.reset()
			p.processBlockInlines(ctx, blockText, r)
			// Reset marks after processing
			ctx.stk.reset()
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
//	leaveContainers         ← close remaining open containers (md_leave_child_containers)
//	MD_LEAVE_BLOCK(DOC)
func (p *Parser) parseLinesInternal(src stream.LineSource, r renderer.Renderer, ctx *context) error {

	// I29: Set parser flags on context for use in analyzeLine/triggers.
	ctx.flags = p.flags

	if err := r.EnterBlock(ast.BlockDoc, nil); err != nil {
		return err
	}

	seed := dummyBlankLine
	if err := p.runLineLoop(src, r, ctx, &seed, true); err != nil {
		return err
	}

	return p.finalizeStream(ctx, r)
}

// runLineLoop is the per-line core shared by one-shot and streaming parses.
// It reads lines from src, runs analyzeLine + processLine, and emits events as
// blocks close. prev is the previous line's analysis (analyzeLine depends on
// it) at entry; its value at entry seeds the loop and it receives the final
// line's analysis at exit (so continuation streams can persist it). The loop
// uses two internal buffers and never writes through prev except at the end.
func (p *Parser) runLineLoop(src stream.LineSource, r renderer.Renderer, ctx *context, prev *lineAnalysis, isFirstCall bool) error {
	var lineBufs [2]lineAnalysis
	// Seed pivot with a copy of prev so we never mutate prev mid-loop (prev may
	// be dummyBlankLine, a package-level var).
	pivot := &lineBufs[0]
	*pivot = *prev
	firstLine := isFirstCall

	for {
		line, ok, err := src.NextLine()
		if err != nil {
			return err
		}
		if !ok {
			break
		}

		// Strip leading UTF-8 BOM (EF BB BF) from the first line when
		// FlagStripBOM is set. CommonMark does not specify BOM handling;
		// goldmark strips it, md4c preserves it. Mirrors no md4c code —
		// this is an md4go-specific compatibility flag.
		if firstLine {
			firstLine = false
			if p.flags&FlagStripBOM != 0 && len(line) >= 3 &&
				line[0] == 0xEF && line[1] == 0xBB && line[2] == 0xBF {
				line = line[3:]
			}
		}

		// Alternate between the two buffers so pivot stays valid.
		la := &lineBufs[1]
		if la == pivot {
			la = &lineBufs[0]
		}

		// First pass: analyze line type (md4c md_analyze_line)
		*la = p.analyzeLine(ctx, line, pivot)

		// Build/close blocks and emit events (md4c md_process_line)
		if err := p.processLine(ctx, la, r); err != nil {
			return err
		}

		pivot = la
	}
	// Persist the final line's analysis back to prev for continuation.
	*prev = *pivot
	return nil
}

// finalizeStream performs the document-end cleanup: close any trailing open
// leaf block, close remaining open containers, emit footnote defs, and leave
// the Doc block. Used by both one-shot parseLinesInternal and ParseStreamEnd.
func (p *Parser) finalizeStream(ctx *context, r renderer.Renderer) error {
	// Close any remaining open leaf block.
	// Mirrors md4c md_end_current_block() after the main loop.
	if ctx.blk.current != nil {
		p.endBlock(ctx, r)
	}

	// Close any remaining open containers (innermost first).
	// Mirrors md4c md_leave_child_containers(ctx, 0).
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

// ParseStreamContinue feeds lines from src to a streaming-continuation parse,
// emitting events for any blocks that close during this call. State (open
// containers, the in-progress leaf block, the previous line's analysis) is
// preserved across Continue calls on the same Parser, so a code block or
// paragraph split across chunk boundaries is parsed as a single structure.
//
// The first Continue call opens the document (EnterBlock Doc); subsequent calls
// reuse the existing context. The stream is finalized by ParseStreamEnd.
//
// This restores md4c's native md_parse() continuation semantics: md4c's C API
// is called once per line with a shared context, whereas md4go's one-shot
// Parse/ParseStream reset the context each call. Continue/End expose the
// incremental, state-preserving mode that md4c was designed around.
func (p *Parser) ParseStreamContinue(src stream.LineSource, r renderer.Renderer) error {
	firstCall := !p.streamStarted
	if firstCall {
		p.streamCtx = &context{}
		p.streamCtx.reset()
		p.streamPivot = dummyBlankLine
		p.streamStarted = true
		if err := r.EnterBlock(ast.BlockDoc, nil); err != nil {
			return err
		}
	}
	p.streamCtx.flags = p.flags
	return p.runLineLoop(src, r, p.streamCtx, &p.streamPivot, firstCall)
}

// ParseStreamEnd finalizes a streaming-continuation parse started by
// ParseStreamContinue: closes any trailing open block and containers, emits
// footnote definitions, leaves the Doc block, and clears the continuation
// state so the Parser can start a fresh stream. It must be called exactly once
// at end-of-stream.
func (p *Parser) ParseStreamEnd(r renderer.Renderer) error {
	if !p.streamStarted {
		return nil
	}
	ctx := p.streamCtx
	err := p.finalizeStream(ctx, r)
	p.streamCtx = nil
	p.streamStarted = false
	p.streamPivot = dummyBlankLine
	return err
}

// InProtectedBlock reports whether the streaming-continuation parse is
// currently inside a block whose content should be treated as verbatim
// (fenced/indented code block or HTML block) rather than as inline markdown.
// It reads the live parser context, so it reflects the state AFTER the most
// recently fed line(s) — including blocks whose EnterBlock/LeaveBlock events
// are deferred until the block closes (md4c's one-line lookahead).
//
// A front-of-pipe guard (e.g. a custom-syntax detector) queries this to decide
// whether a candidate marker at the current cursor is inside a protected
// region and should be passed through untouched.
//
// Only valid between ParseStreamContinue calls on a started stream; returns
// false otherwise.
func (p *Parser) InProtectedBlock() bool {
	if !p.streamStarted || p.streamCtx == nil {
		return false
	}
	ctx := p.streamCtx
	// An open HTML block (md4c html_block_type tracking).
	if ctx.htmlBlockType > 0 {
		return true
	}
	// The currently-accumulating leaf block is a code/html block.
	if cur := ctx.blk.current; cur != nil {
		switch cur.Type {
		case ast.BlockCode, ast.BlockHTML:
			return true
		}
	}
	return false
}
