package parser

import "github.com/userpro/md4go/ast"

// BlockTrigger checks whether a line at the given offset starts a specific
// block type. Triggers are registered in a [256] indexed table and dispatched
// by the first non-whitespace character.
//
// This provides goldmark-style extensibility: extensions (M5) can register
// new triggers without modifying analyzeLine. Mirrors md4c's md_is_*()
// detection functions but with character-indexed dispatch.
type BlockTrigger interface {
	// Check returns a populated lineAnalysis and true if the line matches
	// this trigger. ctx provides access to the current block state (needed
	// by setext, which depends on whether the current block is a paragraph).
	Check(ctx *context, line []byte, off int, pivot *lineAnalysis) (lineAnalysis, bool)
}

// triggerTable is a character-indexed dispatch table for block detection.
// When a line's first non-whitespace character matches an entry, the
// registered triggers are checked in registration order (priority).
type triggerTable [256][]BlockTrigger

// register adds a trigger for a specific leading character.
func (t *triggerTable) register(c byte, trigger BlockTrigger) {
	t[c] = append(t[c], trigger)
}

// registerRange adds a trigger for a range of characters (inclusive).
func (t *triggerTable) registerRange(lo, hi byte, trigger BlockTrigger) {
	for c := int(lo); c <= int(hi); c++ {
		t[byte(c)] = append(t[byte(c)], trigger)
	}
}

// newTriggerTable creates the default trigger table with all built-in
// block type triggers registered in md4c priority order.
//
// Per md4c md_analyze_line() priority chain:
//   - Setext underline and HR are checked BEFORE container marks (directly
//     in analyzeLine, not through this table)
//   - Container marks (blockquote >, list -+* digits) are checked after HR
//     (directly in analyzeLine)
//   - The trigger table handles leaf block types checked AFTER container
//     marks: ATX header, fenced code, HTML block
//
// Priority per character (registration order = priority):
//   - '#' → ATX header
//   - '`' → fenced code
//   - '~' → fenced code
//   - '<' → HTML block
func newTriggerTable() *triggerTable {
	var t triggerTable

	// ATX header.
	t.register('#', &atxTrigger{})

	// Fenced code.
	t.register('`', &fencedCodeTrigger{})
	t.register('~', &fencedCodeTrigger{})

	// HTML block.
	t.register('<', &htmlBlockTrigger{})

	return &t
}

// --- Trigger implementations ---

// atxTrigger detects ATX headers (# ... ######).
type atxTrigger struct{}

func (tr *atxTrigger) Check(ctx *context, line []byte, off int, _ *lineAnalysis) (lineAnalysis, bool) {
	if level, content, ok := parseATXHeaderContent(line[off:], ctx.flags); ok {
		return lineAnalysis{
			lineType: LineATXHeader,
			data:     uint16(level),
			content:  content,
		}, true
	}
	return lineAnalysis{}, false
}

// hrTrigger detects thematic breaks (---, ***, ___ with optional spaces).
type hrTrigger struct{}

func (tr *hrTrigger) Check(_ *context, line []byte, off int, _ *lineAnalysis) (lineAnalysis, bool) {
	if isHR(line[off:]) {
		return lineAnalysis{lineType: LineHR}, true
	}
	return lineAnalysis{}, false
}

// fencedCodeTrigger detects fenced code block starts (``` or ~~~).
type fencedCodeTrigger struct{}

func (tr *fencedCodeTrigger) Check(_ *context, line []byte, off int, _ *lineAnalysis) (lineAnalysis, bool) {
	if fc, fl, info, ok := isFencedCodeStart(line[off:]); ok {
		return lineAnalysis{
			lineType: LineFencedCode,
			data:     uint16(fc),
			fenceLen: fl, // I36: C-35 — store fence length
			content:  info,
		}, true
	}
	return lineAnalysis{}, false
}

// setextTrigger detects setext header underlines (=== for h1, --- for h2).
// Only matches when the current block is a paragraph — mirrors md4c's
// md_is_setext_underline() which is called only from the paragraph context.
type setextTrigger struct {
	level int // 1 for '=', 2 for '-'
}

func (tr *setextTrigger) Check(ctx *context, line []byte, off int, _ *lineAnalysis) (lineAnalysis, bool) {
	// Only match if the current block is a paragraph.
	if ctx.blk.current == nil || ctx.blk.current.Type != ast.BlockP {
		return lineAnalysis{}, false
	}
	if isSetextUnderline(line[off:], tr.level) {
		return lineAnalysis{
			lineType: LineSetextUnderline,
			data:     uint16(tr.level),
		}, true
	}
	return lineAnalysis{}, false
}

// htmlBlockTrigger detects HTML block starts.
// Mirrors md4c md_is_html_block_start_condition() — 7 types.
type htmlBlockTrigger struct{}

func (tr *htmlBlockTrigger) Check(ctx *context, line []byte, off int, pivot *lineAnalysis) (lineAnalysis, bool) {
	// I29: FlagNoHTMLBlocks — skip HTML block detection when flag is set.
	// Mirrors md4c.c:6814-6815: if(CH(off) == '<' && !(flags & MD_FLAG_NOHTMLBLOCKS))
	if ctx.flags&FlagNoHTMLBlocks != 0 {
		return lineAnalysis{}, false
	}
	if ht, ok := detectHTMLBlockStart(line[off:]); ok {
		// HTML block type 7 cannot interrupt a paragraph.
		// Mirrors md4c md4c.c:6819-6821:
		//   if(ctx->html_block_type == 7 && pivot_line->type == MD_LINE_TEXT)
		//       ctx->html_block_type = 0;
		if ht == htmlBlockType7 && pivot.lineType == LineText {
			return lineAnalysis{}, false
		}

		// I37: Set ctx.htmlBlockType (mirrors md4c.c:6817)
		ctx.htmlBlockType = ht

		// Check if the line itself immediately closes the block.
		// Mirrors md4c.c:6823-6828:
		//   if(md_is_html_block_end_condition(ctx, off, &off) == ctx->html_block_type) {
		//       ctx->html_block_type = 0;
		//   }
		if htmlBlockEndCondition(line[off:], ht) {
			ctx.htmlBlockType = 0
		}

		// enforceNewBlock is always TRUE for HTML block starts.
		// Mirrors md4c.c:6830: line->enforce_new_block = TRUE;
		// I39: For types 1-5, preserve indentation in content (verbatim output).
		// Use ctx.containerOff (after container marks, before indent stripping)
		// instead of off (after indent stripping).
		content := line[off:]
		if ht <= htmlBlockType5 && ctx.containerOff < off {
			content = line[ctx.containerOff:]
		}
		return lineAnalysis{
			lineType:        LineHTML,
			data:            uint16(ht),
			content:         content,
			enforceNewBlock: true,
		}, true
	}
	return lineAnalysis{}, false
}
