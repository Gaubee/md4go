package parser

import (
	"bytes"

	"github.com/userpro/md4go/ast"
	"github.com/userpro/md4go/renderer"
)

// processBlockInlines runs the full inline pipeline on blockText:
// collectMarks → analyzeMarks → resolveBrackets → analyzeLinkContents → processInlines.
//
// blockText is assembled once by the caller, eliminating the previous double
// allocation where analyzeInlines and processInlines each called assembleBlockText.
//
// Mirrors md4c md_analyze_inlines() + md_process_inlines() — the three-phase
// inline pipeline (md4c §5.4). The order is critical and must not be changed.
func (p *Parser) processBlockInlines(ctx *context, blockText []byte, r renderer.Renderer) {
	// Phase 1: collect marks
	collectMarks(&ctx.stk, blockText, &p.markChars, p.compat, p.flags)

	// Phase 2: Bracket spans — links, images, footnotes
	// Mirrors md4c md_analyze_marks("[]!") + md_resolve_brackets()
	ctx.stk.analyzeMarks(blockText)
	ctx.stk.resolveBrackets(blockText, p.flags)

	// Phase 3: Emphasis, entities, and extension marks (link contents analysis)
	// Mirrors md4c md_analyze_link_contents() (md4c.c:4656-4699).
	ctx.stk.analyzeLinkContents(blockText, 0, len(ctx.stk.marks), p.flags)

	// Phase 4: Emit inline events by traversing resolved marks.
	// Mirrors md4c md_process_inlines().
	p.processInlines(ctx, blockText, r)
}

// processInlines emits inline events for a leaf block by traversing
// the resolved marks. Mirrors md4c md_process_inlines().
func (p *Parser) processInlines(ctx *context, blockText []byte, r renderer.Renderer) {
	marks := ctx.stk.marks

	// If no marks (or only the sentinel), emit raw text.
	if len(marks) <= 1 {
		p.emitTextWithBreaks(r, blockText, false, false, true)
		return
	}

	// latexDepth tracks nesting of LaTeX math spans.
	// When > 0, text is emitted as TextLatexMath instead of TextNormal.
	// Mirrors md4c's text_type state variable (md4c.c:4908-4914).
	latexDepth := 0

	// Walk through marks and emit text segments between them.
	off := 0
	for i := 0; i < len(marks); i++ {
		m := &marks[i]

		// Skip dummy marks entirely — they exist only for splitEmphMark
		if m.Ch == 'D' {
			continue
		}

		// Skip marks that fall before the current position — consumed by a
		// previous expanded span (e.g. [text][ref] whose closer.End expanded).
		// Exception: permissive autolink openers may have expanded Beg < off
		// (backward username scan); still process them to render the link.
		isAutoOpener := m.Flags&markOpener != 0 && m.Flags&markResolved != 0 &&
			(m.Ch == '@' || m.Ch == ':' || m.Ch == '.')
		if m.Beg < off && m.Ch != 127 && !isAutoOpener {
			continue
		}

		// Emit text before this mark (handling newlines as SoftBR/HardBR)
		if m.Beg > off {
			p.emitTextWithBreaks(r, blockText[off:m.Beg], false, latexDepth > 0)
			off = m.Beg
		}

		if m.Flags&markResolved != 0 {
			switch m.Ch {
			case '\\':
				// Backslash escape.
				// Mirrors md4c md_process_inlines() case '\\' (md4c.c:4808-4813).
				if m.End-m.Beg >= 2 && m.Beg+1 < len(blockText) && blockText[m.Beg+1] == '\n' {
					// Backslash + newline → hard line break
					_ = r.Text(ast.TextBR, nil)
				} else if m.End-m.Beg >= 2 {
					// Backslash + punctuation → emit the escaped character
					p.emitTextWithBreaks(r, blockText[m.Beg+1:m.End], false, latexDepth > 0)
				}
				off = m.End

			case ' ':
				// I29: COLLAPSEWHITESPACE — emit a single space for collapsed whitespace.
				// Mirrors md4c md_process_inlines() case ' ' (md4c.c:4815-4817).
				_ = r.Text(ast.TextNormal, []byte{' '})
				off = m.End

			case '`':
				// Code span: emit content between opener and closer
				if m.Flags&markOpener != 0 {
					closerIdx := m.Next
					if closerIdx >= 0 && closerIdx < len(marks) {
						closer := &marks[closerIdx]
						codeText := blockText[m.End:closer.Beg]
						codeText = trimCodeSpanContent(codeText)
						_ = r.EnterSpan(ast.SpanCode, nil)
						// I36: C-31 — use textWithNullReplacement for code span content,
						// mirroring md4c md_text_with_null_replacement() (md4c.c:410-437).
						p.textWithNullReplacement(r, ast.TextCode, codeText)
						_ = r.LeaveSpan(ast.SpanCode, nil)
						off = closer.End
						i = closerIdx // skip to after closer
						continue
					}
				}
				off = m.End

			case '*':
				fallthrough
			case '_':
				// I29: FlagUnderline — when set, '_' marks emit SpanU per character
				// instead of SpanEm/SpanStrong.
				// Mirrors md4c.c:4829-4843: case '_' with UNDERLINE flag.
				if m.Ch == '_' && p.flags&FlagUnderline != 0 {
					markLen := m.End - m.Beg
					if m.Flags&markOpener != 0 {
						for j := 0; j < markLen; j++ {
							_ = r.EnterSpan(ast.SpanU, nil)
						}
					} else if m.Flags&markCloser != 0 {
						for j := 0; j < markLen; j++ {
							_ = r.LeaveSpan(ast.SpanU, nil)
						}
					}
					off = m.End
					continue
				}
				// Emphasis/strong emphasis.
				// Mirrors md4c md_process_inlines() case '*','_' (md4c.c:4829-4866).
				// Inlined from resolveEmphSpanType to eliminate heap allocation.
				// Opener: odd → <em> first, then <strong> pairs.
				// Closer: <strong> pairs first, then <em> if odd.
				markLen := m.End - m.Beg
				if m.Flags&markOpener != 0 {
					if markLen%2 == 1 {
						_ = r.EnterSpan(ast.SpanEm, nil)
						markLen--
					}
					for markLen >= 2 {
						_ = r.EnterSpan(ast.SpanStrong, nil)
						markLen -= 2
					}
				} else if m.Flags&markCloser != 0 {
					for markLen >= 2 {
						_ = r.LeaveSpan(ast.SpanStrong, nil)
						markLen -= 2
					}
					if markLen == 1 {
						_ = r.LeaveSpan(ast.SpanEm, nil)
					}
				}
				off = m.End

			case '~':
				// Strikethrough (~~) or subscript (~), depending on mark length and flags.
				// Mirrors md4c md_process_inlines() case '~' (md4c.c:4864-4879):
				//   if(mark->end - mark->beg == 1 && SUBSCRIPTS flag) → MD_SPAN_SUBSCRIPT
				//   else → MD_SPAN_DEL
				markLen := m.End - m.Beg
				if markLen == 1 && p.flags&FlagSubscripts != 0 {
					// Subscript: single ~ with FlagSubscripts
					if m.Flags&markOpener != 0 {
						_ = r.EnterSpan(ast.SpanSubscript, nil)
					} else if m.Flags&markCloser != 0 {
						_ = r.LeaveSpan(ast.SpanSubscript, nil)
					}
				} else {
					// Strikethrough: double ~~, or single ~ without FlagSubscripts
					if m.Flags&markOpener != 0 {
						_ = r.EnterSpan(ast.SpanDel, nil)
					} else if m.Flags&markCloser != 0 {
						_ = r.LeaveSpan(ast.SpanDel, nil)
					}
				}
				off = m.End

			case '^':
				// Superscript
				if m.Flags&markOpener != 0 {
					_ = r.EnterSpan(ast.SpanSuperscript, nil)
				} else if m.Flags&markCloser != 0 {
					_ = r.LeaveSpan(ast.SpanSuperscript, nil)
				}
				off = m.End

			case '=':
				// Highlight (==). Only double = produces <mark>; single = is literal.
				// Mirrors md4c md_process_inlines() case '=' (md4c.c:4898-4904):
				//   if(mark->end - mark->beg == 2) → MD_SPAN_MARK
				if m.End-m.Beg == 2 {
					if m.Flags&markOpener != 0 {
						_ = r.EnterSpan(ast.SpanMark, nil)
					} else if m.Flags&markCloser != 0 {
						_ = r.LeaveSpan(ast.SpanMark, nil)
					}
				}
				off = m.End

			case '|':
				// Spoiler (||)
				if m.Flags&markOpener != 0 {
					_ = r.EnterSpan(ast.SpanSpoiler, nil)
				} else if m.Flags&markCloser != 0 {
					_ = r.LeaveSpan(ast.SpanSpoiler, nil)
				}
				off = m.End

			case '$':
				// LaTeX math: inline ($...$) or display ($$...$$).
				// Mirrors md4c md_process_inlines() case '$' (md4c.c:4907-4914).
				if m.Flags&markOpener != 0 {
					spanType := ast.SpanLatexMath
					if m.End-m.Beg == 2 {
						spanType = ast.SpanLatexMathDisplay
					}
					_ = r.EnterSpan(spanType, nil)
					latexDepth++
				} else if m.Flags&markCloser != 0 {
					spanType := ast.SpanLatexMath
					if m.End-m.Beg == 2 {
						spanType = ast.SpanLatexMathDisplay
					}
					_ = r.LeaveSpan(spanType, nil)
					if latexDepth > 0 {
						latexDepth--
					}
				}
				off = m.End

			case '&':
				// Entity: emit as TextEntity
				if m.Flags&markOpener != 0 && m.Next >= 0 && m.Next < len(marks) {
					closerIdx := m.Next
					entityText := blockText[m.Beg:marks[closerIdx].End]
					_ = r.Text(ast.TextEntity, entityText)
					off = marks[closerIdx].End
					i = closerIdx
					continue
				}
				off = m.End

			case 0:
				// NULL character → U+FFFD
				_ = r.Text(ast.TextNullChar, []byte("\uFFFD"))
				off = m.End

			case '[':
				fallthrough
			case '!':
				// Footnote reference, wikilink, link, or image opener.
				// Mirrors md4c md_process_inlines() case '['/'!' (md4c.c:4917-4961).

				// Footnote reference: self-contained span, no text emitted.
				if m.Flags&markBracketFootnote != 0 {
					// I33: Footnote reference — self-contained span, no text emitted.
					// Only the opener is processed; the closer is skipped by the
					// off-advance below. Mirrors md4c.c:4926-4941.
					opener := m
					closerIdx := opener.Next
					if closerIdx >= 0 && closerIdx < len(marks) {
						closer := &marks[closerIdx]
						attrs, ok := ctx.stk.getLinkAttrsFull(i)
						if ok {
							// Build label attribute from the text between opener.End and closer.Beg
							label := blockText[opener.End:closer.Beg]
							labelAttr := BuildAttribute(label, 0)
							detail := &ast.FootnoteRefDetail{
								ID:    attrs.footnoteID,
								RefID: attrs.footnoteRefID,
								Label: labelAttr,
							}
							_ = r.EnterSpan(ast.SpanFootnoteRef, detail)
							_ = r.LeaveSpan(ast.SpanFootnoteRef, nil)
						}
						// Skip past the entire [^label] — redirect opener's end past closer
						off = closer.End
						i = closerIdx
						continue
					}
					off = m.End
					continue
				}

				// I35: Wikilink: [[target]] or [[target|label]].
				// Mirrors md4c md_process_inlines() (md4c.c:4944-4961):
				// When opener.ch == '[' && closer.ch == ']' &&
				//   opener.end - opener.beg >= 2 && closer.end - closer.beg >= 2,
				// this is a wikilink.
				if m.Flags&markOpener != 0 && m.Flags&markResolved != 0 {
					closerIdx := m.Next
					if closerIdx >= 0 && closerIdx < len(marks) {
						closer := &marks[closerIdx]
						// CRITICAL: Must check m.Ch == '[' to distinguish from
						// CANBEIMAGE expansion where Ch becomes '!' with Beg--.
						if m.Ch == '[' && closer.Ch == ']' {
							if openerEndMinusBeg := m.End - m.Beg; openerEndMinusBeg >= 2 {
								closerEndMinusBeg := closer.End - closer.Beg
								if closerEndMinusBeg >= 2 {
									// This is a wikilink [[...]]
									// Determine target and whether there's a label.
									// Mirrors md4c.c:4948-4958:
									//   has_label = (opener->end - opener->beg > 2)
									//   if(has_label) target = opener->beg+2 ... opener->end
									//   else target = opener->end ... closer->beg
									hasLabel := openerEndMinusBeg > 2
									var target []byte
									if hasLabel {
										target = blockText[m.Beg+2 : m.End]
									} else {
										target = blockText[m.End:closer.Beg]
									}

									targetAttr := BuildAttribute(target, 0)
									detail := &ast.WikilinkDetail{Target: targetAttr}
									_ = r.EnterSpan(ast.SpanWikilink, detail)
									off = m.End
									continue
								}
							}
						}

						// Regular link or image
						spanType := resolveLinkSpanType(m.Ch)
						href, title := ctx.stk.getLinkAttrs(i)
						hrefAttr := BuildAttribute(href, buildAttrNoEscapes)
						titleAttr := BuildAttribute(title, buildAttrNoEscapes)
						var detail any
						if spanType == ast.SpanLink {
							detail = &ast.LinkDetail{Href: hrefAttr, Title: titleAttr}
						} else {
							detail = &ast.ImgDetail{Src: hrefAttr, Title: titleAttr}
						}
						_ = r.EnterSpan(spanType, detail)
						off = m.End
						continue
					}
				}
				off = m.End

			case ']':
				// Link, image, or wikilink closer.
				if m.Flags&markCloser != 0 && m.Flags&markResolved != 0 {
					openerIdx := m.Prev
					if openerIdx >= 0 && openerIdx < len(marks) {
						opener := &marks[openerIdx]
						// I35: Wikilink closer — if opener.Ch=='[' and both have end-beg >= 2.
						// Must check opener.Ch=='[' to avoid false positive with CANBEIMAGE.
						if opener.Ch == '[' && opener.End-opener.Beg >= 2 && m.End-m.Beg >= 2 {
							_ = r.LeaveSpan(ast.SpanWikilink, nil)
							off = m.End
							continue
						}
						spanType := resolveLinkSpanType(opener.Ch)
						_ = r.LeaveSpan(spanType, nil)
						off = m.End
						continue
					}
				}
				off = m.End

			case '<':
				// Autolink or raw HTML.
				// Mirrors md4c md_process_inlines() case '<'/'>' (md4c.c:4983-5030).
				if m.Flags&markAutolink != 0 {
					// Autolink: emit as a link span.
					// The opener '<' has end = off+1, and the closer '>' has beg = end-1.
					// The link destination is the text between opener.end and closer.beg.
					if m.Flags&markOpener != 0 {
						closerIdx := m.Next
						if closerIdx >= 0 && closerIdx < len(marks) {
							closer := &marks[closerIdx]
							dest := blockText[m.End:closer.Beg]

							// Prepend "mailto:" for email autolinks, per md4c
							href := dest
							if m.Flags&markAutolinkMissingMailto != 0 {
								href = append([]byte("mailto:"), dest...)
							}

							// Build attribute with noEscapes (autolinks don't resolve backslashes).
							// Mirrors md4c: MD_BUILD_ATTR_NO_ESCAPES for autolinks (md4c.c:4715).
							hrefAttr := BuildAttribute(href, buildAttrNoEscapes)
							detail := &ast.LinkDetail{Href: hrefAttr, IsAutolink: true}
							_ = r.EnterSpan(ast.SpanLink, detail)
							// Emit the destination text as visible link text
							p.emitTextWithBreaks(r, dest, false, latexDepth > 0)
							_ = r.LeaveSpan(ast.SpanLink, nil)
							off = closer.End
							i = closerIdx // skip the closer mark
							continue
						}
					}
					// Autolink closer should have been handled above
					off = m.End
				} else {
					// Raw HTML inline: emit the text between opener and closer as TextHTML.
					// In md4c, the opener sets text_type = MD_TEXT_HTML until the closer.
					if m.Flags&markOpener != 0 {
						closerIdx := m.Next
						if closerIdx >= 0 && closerIdx < len(marks) {
							closer := &marks[closerIdx]
							// Emit the entire HTML span from '<' to '>'
							htmlText := blockText[m.Beg:closer.End]
							_ = r.Text(ast.TextHTML, htmlText)
							off = closer.End
							i = closerIdx
							continue
						}
					}
					off = m.End
				}

			case 127:
				// Sentinel — emit any remaining text after last mark
				if off < len(blockText) {
					p.emitTextWithBreaks(r, blockText[off:], false, latexDepth > 0)
				}
				off = len(blockText)

			case '@', ':', '.':
				// Permissive autolink (email/URL/WWW).
				// The opener/closer are cross-linked via Next/Prev.
				// opener.Beg == opener.End (start of expanded range)
				// closer.Beg == closer.End (end of range)
				// Visible text: blockText[max(m.Beg,off):closer.Beg]
				// — when Beg was expanded backward past off (email username
				//   scan), only emit text not already emitted by prior marks).
				if m.Flags&markOpener != 0 {
					closerIdx := m.Next
					if closerIdx >= 0 && closerIdx < len(marks) {
						closer := &marks[closerIdx]
						closer.Flags |= markValidPermissiveAutolink

						// Full dest for href (complete URL/email)
						fullDest := blockText[m.Beg:closer.Beg]

						// Only emit visible text portion not yet emitted
						destStart := m.Beg
						if destStart < off {
							destStart = off
						}
						dest := blockText[destStart:closer.Beg]

						href := fullDest
						if m.Ch == '@' || m.Ch == '.' {
							prefix := "mailto:"
							if m.Ch == '.' {
								prefix = "http://"
							}
							href = make([]byte, 0, len(prefix)+len(fullDest))
							href = append(href, prefix...)
							href = append(href, fullDest...)
						}

						hrefAttr := BuildAttribute(href, buildAttrNoEscapes)
						detail := &ast.LinkDetail{Href: hrefAttr, IsAutolink: true}
						_ = r.EnterSpan(ast.SpanLink, detail)
						p.emitTextWithBreaks(r, dest, false, latexDepth > 0)
						_ = r.LeaveSpan(ast.SpanLink, nil)
						off = closer.End
						i = closerIdx
						continue
					}
				} else if m.Flags&markCloser != 0 {
					if m.Flags&markValidPermissiveAutolink != 0 {
						// Handled in opener case above
					}
				}
				off = m.End

			default:
				off = m.End
			}
		} else {
			// Unresolved mark — emit as literal text
			if m.End > m.Beg {
				p.emitTextWithBreaks(r, blockText[m.Beg:m.End], false, latexDepth > 0)
			}
			off = m.End
		}
	}

	// Emit any remaining text after the last mark.
	if off < len(blockText) {
		p.emitTextWithBreaks(r, blockText[off:], false, latexDepth > 0, true)
	}
}

// assembleTextFromLines joins lines with '\n', writing into the reusable buffer
// pointed to by buf. The caller must ensure buf is not read after the next call
// to assembleTextFromLines with the same buf pointer.
// The returned slice is a view into *buf.
func assembleTextFromLines(lines []Line, buf *[]byte) []byte {
	if len(lines) == 0 {
		return nil
	}
	// Calculate required capacity
	size := 0
	for _, ln := range lines {
		size += len(ln.Text)
	}
	size += len(lines) - 1 // newlines between lines

	*buf = (*buf)[:0]
	if cap(*buf) < size {
		*buf = make([]byte, 0, size)
	}
	for i, ln := range lines {
		if i > 0 {
			*buf = append(*buf, '\n')
		}
		*buf = append(*buf, ln.Text...)
	}
	return *buf
}

// assembleBlockText joins the lines of a leaf block with '\n' into
// ctx.blockTextBuf. The returned slice is a view into ctx.blockTextBuf
// and is valid only until the next call to assembleBlockText.
//
// IMPORTANT: Trailing spaces are preserved in the line text so that
// hard line break detection (two+ trailing spaces + newline) works.
// This mirrors md4c where line content retains trailing spaces for
// hard break detection in md_process_inlines().
func (p *Parser) assembleBlockText(ctx *context, b *Block) []byte {
	if b.NLines == 0 {
		return nil
	}
	return assembleTextFromLines(ctx.blk.lines[b.LineIdx:b.LineIdx+b.NLines], &ctx.blockTextBuf)
}

// emitTextWithBreaks emits text segments, splitting at '\n' boundaries.
// For each newline, it checks if the preceding text ends with two or more
// spaces — if so, it emits a hard line break (TextBR); otherwise a soft
// line break (TextSoftBR).
//
// When latexMath is true, text is emitted as TextLatexMath instead of
// TextNormal, mirroring md4c's text_type state variable for $ spans.
//
// This mirrors md4c md_process_inlines() hard break detection (md4c.c:5085-5096):
//   - Backslash + newline: already handled in the '\\' mark case above
//   - Two trailing spaces + newline: checked here
//   - Flag HARD_SOFT_BREAKS: all soft breaks become hard
func (p *Parser) emitTextWithBreaks(r renderer.Renderer, text []byte, enforceHardBreak bool, latexMath bool, isParaEnd ...bool) {
	textType := ast.TextNormal
	if latexMath {
		textType = ast.TextLatexMath
	}
	start := 0
	for i := 0; i <= len(text); i++ {
		if i == len(text) || text[i] == '\n' {
			lineText := text[start:i]
			if len(lineText) > 0 {
				// Strip trailing spaces at line boundaries per md4c.c:6906-6910.
				isLineEnd := i < len(text)
				paraEnd := len(isParaEnd) > 0 && isParaEnd[0] && i == len(text)

				trailingSpaces := countTrailingSpaces(lineText)
				if (isLineEnd || paraEnd) && trailingSpaces > 0 {
					trimmedLen := len(lineText) - trailingSpaces
					if trimmedLen > 0 {
						_ = r.Text(textType, lineText[:trimmedLen])
					}
				} else {
					_ = r.Text(textType, lineText)
				}
			}
			if i < len(text) {
				// Determine break type.
				// Mirrors md4c md_process_inlines() (md4c.c:5085-5096):
				// Hard breaks are only checked for MD_TEXT_NORMAL (not code or
				// LaTeX math), and require the last two trailing chars to be
				// spaces (tabs don't count). Direct byte comparison replaces
				// bytes.HasSuffix to avoid per-newline slice allocation.
				// I29: FlagHardSoftBreaks — all soft breaks become hard.
				isHardBreak := false
				if !latexMath {
					segLen := i - start
					isHardBreak = enforceHardBreak ||
						(segLen >= 2 && text[i-2] == ' ' && text[i-1] == ' ') ||
						p.flags&FlagHardSoftBreaks != 0
				}
				if isHardBreak {
					_ = r.Text(ast.TextBR, nil)
				} else {
					_ = r.Text(ast.TextSoftBR, nil)
				}
			}
			start = i + 1
		}
	}
}

// textWithNullReplacement emits text with NULL characters (U+0000) replaced
// by TextNullChar events. Mirrors md4c md_text_with_null_replacement()
// (md4c.c:410-437): split text at NULL chars, emit the original text type
// for non-NULL segments and TextNullChar for each NULL byte.
//
// This is called for text that does NOT go through the mark pipeline:
// code block content, HTML block content, code span content, etc.
// (Normal text handles NULLs via the mark system in collectMarks case 0.)
func (p *Parser) textWithNullReplacement(r renderer.Renderer, textType ast.TextType, text []byte) {
	// Fast path: no NULL characters (common case for code/HTML blocks).
	// bytes.IndexByte is SIMD-optimized in Go runtime, far faster than
	// the byte-by-byte scan below for the common no-NULL case.
	if bytes.IndexByte(text, 0) < 0 {
		_ = r.Text(textType, text)
		return
	}
	// Slow path: split at NULL boundaries
	off := 0
	for off < len(text) {
		// Find next NULL or end
		end := off
		for end < len(text) && text[end] != 0 {
			end++
		}
		// Emit non-NULL segment
		if end > off {
			_ = r.Text(textType, text[off:end])
		}
		// Emit NULL replacement
		if end < len(text) && text[end] == 0 {
			_ = r.Text(ast.TextNullChar, []byte("\uFFFD"))
			end++
		}
		off = end
	}
}

// countTrailingSpaces counts trailing space/tab characters in text.
func countTrailingSpaces(text []byte) int {
	n := 0
	for i := len(text) - 1; i >= 0; i-- {
		if text[i] == ' ' || text[i] == '\t' {
			n++
		} else {
			break
		}
	}
	return n
}
