package parser

import (
	"sync"

	"github.com/userpro/md4go/ast"
)

var contextPool = sync.Pool{New: func() any { return &context{} }}

// getContext returns a pooled context.
func getContext() *context { return contextPool.Get().(*context) }

// putContext returns a context to the pool, dropping oversized maps.
func putContext(ctx *context) {
	if ctx == nil {
		return
	}

	const maxPoolMapSize = 1024
	if len(ctx.refDefs) > maxPoolMapSize {
		ctx.refDefs = nil
	}
	if len(ctx.footnoteDefs) > maxPoolMapSize {
		ctx.footnoteDefs = nil
	}
	// Prevent unbounded labelNormBuf growth from pathological labels
	// (e.g. a crafted document with a 1MB link label). 4KB is more than
	// adequate for any real-world link label.
	const maxLabelNormBufSize = 4096
	if cap(ctx.labelNormBuf) > maxLabelNormBufSize {
		ctx.labelNormBuf = nil
	}
	// Prevent unbounded blockTextBuf growth — 64KB handles any realistic block.
	const maxBlockTextBufSize = 65536
	if cap(ctx.blockTextBuf) > maxBlockTextBufSize {
		ctx.blockTextBuf = nil
	}
	contextPool.Put(ctx)
}

// context holds per-parse mutable state, analogous to md4c's MD_CTX.
//
// Key design choice (M4/I12): context does NOT hold a reference to the full
// input document. The unified incremental pipeline feeds lines via LineSource,
// and block content is stored in blockStack as independent []byte copies.
// This ensures Convert ([]byte) and ConvertStream (io.Reader) share the same
// code path with no dependency on the original input buffer.
//
// I28: Container state is now a stack ([]Container), mirroring md4c's
// ctx->containers[] + ctx->n_containers. This supports arbitrary nesting depth.
type context struct {
	blk        blockStack  // flat block storage (blocks + lines + vlines)
	containers []Container // stack of open containers (blockquote/list nesting)
	flags      Flags       // parser flags (mirrors p.flags, set per-parse)

	// Tight/loose list tracking (mirrors md4c ctx.last_line_has_list_loosening_effect).
	lastLineHasListLooseningEffect      bool
	lastListItemStartsWithTwoBlankLines bool

	// I37: HTML block type tracking (mirrors md4c ctx->html_block_type).
	// When > 0, we are inside an HTML block of the given type (1-7).
	// Set when an HTML block starts, cleared when it ends.
	htmlBlockType uint8

	// I39: Container mark offset — byte position after container marks
	// (blockquote >, list marks) but before content indentation stripping.
	// Needed for HTML block types 1-5 verbatim output to preserve indentation.
	// Set in analyzeLine step 1, read by htmlBlockTrigger and step 3.
	containerOff int

	// noInline: when true, emitBlock skips the entire inline analysis pipeline
	// (processBlockInlines). Used by ParseBlocksOnly for fast structural parsing:
	// only EnterBlock/LeaveBlock events are emitted, no span or text events.
	// This saves ~8,000 renderer interface dispatches per parse plus the full
	// collectMarks→analyzeMarks→resolveBrackets→analyzeLinkContents→processInlines
	// chain, cutting ParseOnly wall-clock time by ~40-60%.
	noInline bool

	// labelNormBuf is a reusable buffer for normalizeLinkLabel.
	// Every reference link resolution allocates make([]byte, 0, end-start)
	// for label normalization. Pooling this buffer avoids per-link allocation.
	// The buffer is shared across blocks within a single parse call;
	// the normalized result is always consumed immediately (converted to
	// string for map lookup), so reuse is safe.
	labelNormBuf []byte

	// blockTextBuf is a reusable buffer for assembleTextFromLines.
	// Each leaf block's text is assembled into this buffer and consumed
	// synchronously by processBlockInlines. After ctx.stk.reset(), no
	// references to the buffer remain, so reuse across blocks is safe.
	// Mirrors the md4c philosophy of a flat per-block text buffer.
	blockTextBuf []byte

	// Mark system (M3): inline marks for current block
	stk markStacks // marks + 19 opener stacks

	// Reference definitions (M3/I09): map[string]*RefDef — shared across blocks
	// Go map replaces md4c's md_build_ref_def_hashtable(), supports incremental insert
	refDefs map[string]*RefDef

	// Footnote definitions (I33): map[string]*FootnoteDef
	// Mirrors md4c's footnote_hashtable + next_footnote_index.
	footnoteDefs      map[string]*FootnoteDef
	nextFootnoteIndex uint // 1-based counter for sequential numbering
}

// nContainers returns the number of open containers.
// Mirrors md4c ctx->n_containers.
func (ctx *context) nContainers() int { return len(ctx.containers) }

// inContainer returns true if any container is open.
func (ctx *context) inContainer() bool { return len(ctx.containers) > 0 }

// topContainer returns the innermost open container, or nil if none.
func (ctx *context) topContainer() *Container {
	if len(ctx.containers) == 0 {
		return nil
	}
	return &ctx.containers[len(ctx.containers)-1]
}

// inBlockquote returns true if the innermost container is a blockquote.
func (ctx *context) inBlockquote() bool {
	c := ctx.topContainer()
	return c != nil && c.Ch == '>'
}

// inListItem returns true if the innermost container is a list item.
func (ctx *context) inListItem() bool {
	c := ctx.topContainer()
	return c != nil && c.Ch != '>'
}

// inTightList returns true if the nearest list container is tight.
// Walks the container stack from innermost to outermost.
func (ctx *context) inTightList() bool {
	for i := len(ctx.containers) - 1; i >= 0; i-- {
		c := &ctx.containers[i]
		if c.Ch != '>' {
			// This is a list container
			return !c.IsLoose
		}
	}
	return false
}

// listType returns the BlockType of the nearest list container, or 0 if none.
func (ctx *context) listType() ast.BlockType {
	for i := len(ctx.containers) - 1; i >= 0; i-- {
		c := &ctx.containers[i]
		if c.Ch == '.' || c.Ch == ')' {
			return ast.BlockOL
		}
		if c.Ch == '-' || c.Ch == '*' || c.Ch == '+' {
			return ast.BlockUL
		}
	}
	return 0
}

// reset clears the context for reuse.
func (ctx *context) reset() {
	ctx.blk.reset()
	ctx.containers = ctx.containers[:0]
	ctx.stk.reset()
	// Initialize refDefs if needed, and share with markStacks
	if ctx.refDefs == nil {
		ctx.refDefs = make(map[string]*RefDef)
	} else {
		// Clear existing entries for reuse
		for k := range ctx.refDefs {
			delete(ctx.refDefs, k)
		}
	}
	ctx.stk.refDefs = ctx.refDefs
	// Initialize footnote defs (before sharing with markStacks)
	if ctx.footnoteDefs == nil {
		ctx.footnoteDefs = make(map[string]*FootnoteDef)
	} else {
		for k := range ctx.footnoteDefs {
			delete(ctx.footnoteDefs, k)
		}
	}
	// Share footnoteDefs with markStacks (after initialization)
	ctx.stk.footnoteDefs = ctx.footnoteDefs
	ctx.stk.nextFootnoteIndex = 0 // will be set during parse
	ctx.nextFootnoteIndex = 0
	ctx.lastLineHasListLooseningEffect = false
	ctx.lastListItemStartsWithTwoBlankLines = false
	ctx.htmlBlockType = 0
	ctx.containerOff = 0
	ctx.noInline = false
	ctx.labelNormBuf = ctx.labelNormBuf[:0]
	ctx.blockTextBuf = ctx.blockTextBuf[:0]
	// Wire markStacks.labelNormBuf to ctx.labelNormBuf so normalizeLinkLabel
	// inside markStacks methods can reuse the context-level buffer.
	ctx.stk.labelNormBuf = &ctx.labelNormBuf
}
