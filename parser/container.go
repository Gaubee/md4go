package parser

import (
	"github.com/userpro/md4go/ast"
	"github.com/userpro/md4go/renderer"
)

// admonitionTags lists the recognized admonition type labels.
// Mirrors md4c.c:5535 MD_ADMONITION_TAGS[].
var admonitionTags = []string{"note", "tip", "important", "warning", "caution"}

// detectAdmonitionTag checks if content matches the [!TYPE] admonition pattern.
// Returns the tag index and true if matched, or 0 and false otherwise.
//
// Mirrors md4c.c:6948-6967: the content must be exactly [!TYPE] where TYPE
// is one of the recognized tags (case-insensitive). The total length must be
// between 4 and 15 characters (3 < len < 16 in md4c).
func detectAdmonitionTag(content []byte) (int, bool) {
	n := len(content)
	if n < 4 || n > 15 {
		return 0, false
	}
	if content[0] != '[' || content[1] != '!' || content[n-1] != ']' {
		return 0, false
	}
	// Extract the tag label between [! and ]
	label := string(content[2 : n-1])
	for i, tag := range admonitionTags {
		if len(label) == len(tag) && asciiCaseEqual(label, tag) {
			return i, true
		}
	}
	return 0, false
}

// asciiCaseEqual compares two ASCII strings case-insensitively.
// Mirrors md4c md_ascii_case_eq().
func asciiCaseEqual(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := 0; i < len(a); i++ {
		ca, cb := a[i], b[i]
		if ca >= 'A' && ca <= 'Z' {
			ca += 0x20
		}
		if cb >= 'A' && cb <= 'Z' {
			cb += 0x20
		}
		if ca != cb {
			return false
		}
	}
	return true
}

// admonitionTagString returns the admonition type string for the given index.
func admonitionTagString(idx int) []byte {
	if idx >= 0 && idx < len(admonitionTags) {
		return []byte(admonitionTags[idx])
	}
	return []byte("note")
}

// isContainerMark checks if the character at off is a container mark
// (blockquote >, unordered list -/+/*, or ordered list digits + ./)).
// Returns the container info, the offset after the mark, and true.
// Mirrors md4c md_is_container_mark() (md4c.c:6409-6464).
func isContainerMark(line []byte, off int, indent int) (Container, int, bool) {
	if off >= len(line) || indent >= codeIndentOffset {
		return Container{}, off, false
	}

	c := line[off]

	// Block quote mark
	if c == '>' {
		return Container{
			Ch:             '>',
			MarkIndent:     indent,
			ContentsIndent: indent + 1,
		}, off + 1, true
	}

	// Unordered list bullet mark
	if c == '-' || c == '+' || c == '*' {
		if off+1 >= len(line) || line[off+1] == ' ' || line[off+1] == '\t' || line[off+1] == '\n' || line[off+1] == '\r' {
			return Container{
				Ch:             c,
				MarkIndent:     indent,
				ContentsIndent: indent + 1,
			}, off + 1, true
		}
		return Container{}, off, false
	}

	// Ordered list item mark
	if c >= '0' && c <= '9' {
		maxEnd := off + 9
		if maxEnd > len(line) {
			maxEnd = len(line)
		}
		numOff := off
		start := 0
		for numOff < maxEnd && line[numOff] >= '0' && line[numOff] <= '9' {
			start = start*10 + int(line[numOff]-'0')
			numOff++
		}
		if numOff > off && numOff < len(line) && (line[numOff] == '.' || line[numOff] == ')') {
			if numOff+1 >= len(line) || line[numOff+1] == ' ' || line[numOff+1] == '\t' || line[numOff+1] == '\n' || line[numOff+1] == '\r' {
				return Container{
					Ch:             line[numOff],
					Start:          start,
					MarkIndent:     indent,
					ContentsIndent: indent + (numOff - off) + 1,
				}, numOff + 1, true
			}
		}
	}

	return Container{}, off, false
}

// isContainerCompatible checks if a new container is compatible with an
// existing container (i.e., same list type for "brother" detection).
// Mirrors md4c md_is_container_compatible() (md4c.c:6278-6291).
//
// Key differences from previous implementation (aligned with md4c):
//   - Blockquote is NEVER compatible as a brother (md4c returns FALSE for '>')
//   - New container's mark_indent must be <= existing container's contents_indent
func isContainerCompatible(existing, new *Container) bool {
	// Mirrors md4c.c:6280: if(container->ch == '>') return FALSE;
	if new.Ch == '>' {
		return false
	}
	// Mirrors md4c.c:6282: if(container->ch != pivot->ch) return FALSE;
	if new.Ch != existing.Ch {
		return false
	}
	// Mirrors md4c.c:6284: if(container->mark_indent > pivot->contents_indent) return FALSE;
	if new.MarkIndent > existing.ContentsIndent {
		return false
	}
	return true
}

// detectTaskMark checks for a GFM task list marker [x], [X], or [ ] after
// the list mark. Returns the task mark character, the offset after the marker,
// and true if found.
//
// Mirrors md4c md4c.c:6859-6874.
func detectTaskMark(line []byte, off int) (taskMark byte, newOff int, ok bool) {
	tmp := off
	// Skip up to 3 blanks (md4c allows up to 3 spaces before [)
	for tmp < len(line) && tmp < off+3 && (line[tmp] == ' ' || line[tmp] == '\t') {
		tmp++
	}
	// Need at least 3 more characters: [x] or [ ]
	if tmp+2 >= len(line) {
		return 0, 0, false
	}
	if line[tmp] != '[' {
		return 0, 0, false
	}
	c := line[tmp+1]
	if c != 'x' && c != 'X' && c != ' ' {
		return 0, 0, false
	}
	if line[tmp+2] != ']' {
		return 0, 0, false
	}
	// After ] must be whitespace or EOL
	if tmp+3 < len(line) {
		after := line[tmp+3]
		if after != ' ' && after != '\t' && after != '\n' && after != '\r' {
			return 0, 0, false
		}
	}
	// Skip past [x] and following whitespace
	newOff = tmp + 3
	for newOff < len(line) && (line[newOff] == ' ' || line[newOff] == '\t') {
		newOff++
	}
	return c, newOff, true
}

// buildLIDetail creates the detail for a list item, including task list info.
func buildLIDetail(container *Container) any {
	if container.IsTask {
		return &ast.LIDetail{
			IsTask:      true,
			TaskMark:    container.TaskMark,
			TaskMarkOff: container.TaskMarkOff,
		}
	}
	return nil
}

// handleContainerTransitions manages container (blockquote/list) open/close
// transitions based on the line analysis results (nParents/nBrothers/nChildren).
// Called at the start of processLine before leaf block processing.
//
// Mirrors md4c md_analyze_line() container management section (md4c.c:6920-6971)
// + md_enter_child_containers() + md_leave_child_containers().
func (p *Parser) handleContainerTransitions(ctx *context, la *lineAnalysis, r renderer.Renderer) error {
	// Tight/loose list detection (mirrors md4c.c:6920-6927):
	// I37: Use la.prevLineHasListLooseningEffect (saved from previous line)
	// instead of ctx.lastLineHasListLooseningEffect (which may have been
	// reset by analyzeLine for the current non-blank line).
	if la.prevLineHasListLooseningEffect && la.lineType != LineBlank {
		if la.nParents+la.nBrothers > 0 {
			c := &ctx.containers[la.nParents+la.nBrothers-1]
			if c.Ch != '>' {
				c.IsLoose = true
			}
		}
	}

	// 1. Leave containers we are no longer part of.
	// Mirrors md4c.c:6930-6931: if n_children == 0 and n_parents + n_brothers < n_containers.
	// I37: Also mirrors md4c.c:6773-6774 — when n_children > 0 (child container detected
	// in step 9 of analyzeLine), md4c calls md_leave_child_containers(ctx, n_parents+n_brothers)
	// before pushing the new child container. In our code, this means we should leave containers
	// whenever n_parents + n_brothers < n_containers, regardless of n_children.
	if la.nParents+la.nBrothers < ctx.nContainers() {
		if err := p.leaveContainers(ctx, la.nParents+la.nBrothers, r); err != nil {
			return err
		}
	}

	// 2. Handle brother container (new list item in existing list).
	// Mirrors md4c.c:6934-6945.
	if la.nBrothers > 0 {
		// Close current list item (LI) if any
		if ctx.blk.current != nil {
			p.endBlock(ctx, r)
		}
		// Emit LeaveBlock(LI) for the old item
		c := &ctx.containers[la.nParents]
		_ = r.LeaveBlock(ast.BlockLI, nil)

		// Update container task info from newContainers[0]
		if len(la.newContainers) > 0 {
			bro := &la.newContainers[0]
			c.IsTask = bro.IsTask
			c.TaskMark = bro.TaskMark
			c.TaskMarkOff = bro.TaskMarkOff
		}

		// Emit EnterBlock(LI) for the new item
		if err := r.EnterBlock(ast.BlockLI, buildLIDetail(c)); err != nil {
			return err
		}
	}

	// 3. Enter new child containers.
	// Mirrors md4c.c:6948-6971 (md_enter_child_containers).
	// Enter each child container from newContainers[nBrothers:].
	for i := la.nBrothers; i < len(la.newContainers); i++ {
		if err := p.enterContainer(ctx, &la.newContainers[i], r); err != nil {
			return err
		}
	}

	return nil
}

// leaveContainers closes all containers above nKeep.
// Mirrors md4c md_leave_child_containers() (md4c.c:6365-6406).
// I37: Passes the final IsTight value (based on Container.IsLoose) at LeaveBlock time,
// enabling the renderer to determine tight/loose list processing.
func (p *Parser) leaveContainers(ctx *context, nKeep int, r renderer.Renderer) error {
	for ctx.nContainers() > nKeep {
		c := ctx.topContainer()
		if c == nil {
			break
		}

		// Close any open leaf block first
		if ctx.blk.current != nil {
			p.endBlock(ctx, r)
		}

		switch c.Ch {
		case '.', ')':
			// Ordered list: close LI then OL
			_ = r.LeaveBlock(ast.BlockLI, nil)
			_ = r.LeaveBlock(ast.BlockOL, &ast.OLDetail{
				Start:   c.Start,
				Mark:    c.Ch,
				IsTight: !c.IsLoose, // I37: final IsLoose determines IsTight
			})
		case '-', '+', '*':
			// Unordered list: close LI then UL
			_ = r.LeaveBlock(ast.BlockLI, nil)
			_ = r.LeaveBlock(ast.BlockUL, &ast.ULDetail{
				Mark:    c.Ch,
				IsTight: !c.IsLoose, // I37: final IsLoose determines IsTight
			})
		case '>':
			if c.IsAdmonition {
				detail := &ast.AdmonitionDetail{
					Type: ast.NewAttribute(admonitionTagString(c.AdmonitionType)),
				}
				_ = r.LeaveBlock(ast.BlockAdmonition, detail)
			} else {
				_ = r.LeaveBlock(ast.BlockQuote, nil)
			}
		}

		// Pop from stack
		ctx.containers = ctx.containers[:len(ctx.containers)-1]
	}
	return nil
}

// enterContainer pushes a new container onto the stack and emits EnterBlock.
// Mirrors md4c md_enter_child_containers() (md4c.c:6316-6362).
func (p *Parser) enterContainer(ctx *context, c *Container, r renderer.Renderer) error {
	// Close any open leaf block first
	if ctx.blk.current != nil {
		p.endBlock(ctx, r)
	}

	switch c.Ch {
	case '.', ')':
		// Ordered list
		detail := &ast.OLDetail{
			Start:   c.Start,
			Mark:    c.Ch,
			IsTight: !c.IsLoose,
		}
		if err := r.EnterBlock(ast.BlockOL, detail); err != nil {
			return err
		}
		if err := r.EnterBlock(ast.BlockLI, buildLIDetail(c)); err != nil {
			return err
		}
	case '-', '+', '*':
		// Unordered list
		detail := &ast.ULDetail{
			Mark:    c.Ch,
			IsTight: !c.IsLoose,
		}
		if err := r.EnterBlock(ast.BlockUL, detail); err != nil {
			return err
		}
		if err := r.EnterBlock(ast.BlockLI, buildLIDetail(c)); err != nil {
			return err
		}
	case '>':
		if c.IsAdmonition {
			detail := &ast.AdmonitionDetail{
				Type: ast.NewAttribute(admonitionTagString(c.AdmonitionType)),
			}
			if err := r.EnterBlock(ast.BlockAdmonition, detail); err != nil {
				return err
			}
		} else {
			if err := r.EnterBlock(ast.BlockQuote, nil); err != nil {
				return err
			}
		}
	}

	// Push onto stack
	ctx.containers = append(ctx.containers, *c)
	return nil
}

// closeAllContainers emits LeaveBlock for all open containers.
// Closes innermost first. Mirrors md4c md_leave_child_containers(ctx, 0).
func (p *Parser) closeAllContainers(ctx *context, r renderer.Renderer) {
	p.leaveContainers(ctx, 0, r)
}
