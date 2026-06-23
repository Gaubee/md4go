package text

import (
	"io"

	"strconv"

	"github.com/userpro/md4go/ast"
	"github.com/userpro/md4go/parser"
	"github.com/userpro/md4go/renderer"
)

// PlainText renders parse events as plain text.
// Emphasis, links, and other spans are stripped to their textual content.
//
// The separator state machine (needBlank/needNL) controls block-level
// separators between paragraphs, list items, and other block elements.
type PlainText struct {
	w           *renderer.BufWriter
	compat      compatConfig // pre-computed compatibility behavior decisions
	col         int          // current table column (for tab separation)
	inCodeBlock bool         // inside <code> block
	inHTMLBlock bool         // inside <html> block
	inTD        bool         // inside TH or TD
	inLI        bool         // inside LI
	inTR        bool         // inside TR
	needBlank   bool         // need blank line before next text
	needNL      bool         // need newline before next text

	// tightListDepth tracks nesting depth of tight lists.
	tightListDepth int

	// tightListEntered tracks IsTight at EnterBlock time for each nested list.
	// A list may start tight (IsTight=true at EnterBlock) but become loose
	// (IsTight=false at LeaveBlock). We must decrement tightListDepth based
	// on the EnterBlock value, not the final LeaveBlock value, to avoid leaks.
	tightListEntered []bool
}

// NewPlainText creates a PlainText renderer with zero flags (backward compatible).
func NewPlainText(w io.Writer) *PlainText {
	return NewPlainTextWithFlags(w, 0)
}

// NewPlainTextWithFlags creates a PlainText renderer with the given parser flags,
// enabling compatibility behavior toggles.
func NewPlainTextWithFlags(w io.Writer, flags parser.Flags) *PlainText {
	return &PlainText{
		w:      renderer.NewBufWriter(w),
		compat: newCompatConfig(flags),
	}
}

// flushPending outputs any pending separator.
func (pt *PlainText) flushPending() error {
	if pt.needBlank {
		if err := pt.w.WriteByte('\n'); err != nil {
			return err
		}
		pt.needBlank = false
		pt.needNL = false
	} else if pt.needNL {
		if err := pt.w.WriteByte('\n'); err != nil {
			return err
		}
		pt.needNL = false
	}
	return nil
}

// Text writes textual content according to its type.
func (pt *PlainText) Text(t ast.TextType, text []byte) error {
	switch t {
	case ast.TextNullChar:
		if err := pt.flushPending(); err != nil {
			return err
		}
		return pt.w.WriteString("\uFFFD")
	case ast.TextBR, ast.TextSoftBR:
		if err := pt.w.WriteByte('\n'); err != nil {
			return err
		}
		pt.needNL = false
		return nil
	case ast.TextEntity:
		// Compat: FlagDecodeEntities controls whether entities are decoded
		// to Unicode (goldmark behavior) or preserved verbatim (default).
		if err := pt.flushPending(); err != nil {
			return err
		}
		_, err := pt.w.Write(decodeEntity(text, pt.compat.decodeEntities))
		return err
	default:
		if err := pt.flushPending(); err != nil {
			return err
		}
		_, err := pt.w.Write(text)
		return err
	}
}

// EnterBlock handles block-level enter events.
func (pt *PlainText) EnterBlock(b ast.BlockType, detail any) error {
	switch b {
	case ast.BlockQuote:
		pt.needBlank = true
	case ast.BlockUL:
		isTight := true
		if d, ok := detail.(*ast.ULDetail); ok {
			isTight = d.IsTight
		}
		pt.tightListEntered = append(pt.tightListEntered, isTight)
		if isTight {
			pt.tightListDepth++
		}
	case ast.BlockOL:
		isTight := true
		if d, ok := detail.(*ast.OLDetail); ok {
			isTight = d.IsTight
		}
		pt.tightListEntered = append(pt.tightListEntered, isTight)
		if isTight {
			pt.tightListDepth++
		}
	case ast.BlockLI:
		pt.inLI = true
	case ast.BlockH:
		pt.needBlank = true
	case ast.BlockCode:
		pt.inCodeBlock = true
		pt.needBlank = true
	case ast.BlockHTML:
		pt.inHTMLBlock = true
		pt.needBlank = true
	case ast.BlockHR:
		pt.needBlank = true
	case ast.BlockP:
		if pt.inTD {
		} else if pt.inLI && pt.tightListDepth > 0 {
			// Tight list paragraph separator.
			// Emit \n to preserve word boundaries between paragraphs in a
			// tight list (CommonMark does not define plain-text rendering;
			// this default is more correct than md4c's no-separator concat).
			//
			// Downgrade any pending needBlank (e.g. from a blockquote or code
			// block that just closed inside the list item) to a single \n —
			// in a tight list, block boundaries within items are separated by
			// \n, not blank lines.
			if pt.needBlank {
				pt.needBlank = false
				pt.needNL = true
			}
			if pt.needNL {
				if err := pt.w.WriteByte('\n'); err != nil {
					return err
				}
				pt.needNL = false
			}
		} else {
			pt.needBlank = true
		}
	case ast.BlockTR:
		pt.inTR = true
		pt.col = 0
		if err := pt.flushPending(); err != nil {
			return err
		}
	case ast.BlockTH, ast.BlockTD:
		pt.inTD = true
		if pt.col > 0 {
			if err := pt.w.WriteByte('\t'); err != nil {
				return err
			}
		}
		pt.col++
	case ast.BlockFootnoteDefSection, ast.BlockFootnoteDef, ast.BlockAdmonition:
		pt.needBlank = true
	}
	return nil
}

// LeaveBlock handles block-level leave events.
func (pt *PlainText) LeaveBlock(b ast.BlockType, detail any) error {
	switch b {
	case ast.BlockQuote, ast.BlockH, ast.BlockCode, ast.BlockHTML, ast.BlockHR,
		ast.BlockTable, ast.BlockFootnoteDef:
		pt.needNL = true
	case ast.BlockP:
		if !pt.inTD {
			// Set needNL to preserve word boundaries between paragraphs.
			// List-item separators (from BlockLI leave) are unaffected.
			pt.needNL = true
		}
	case ast.BlockTR:
		pt.inTR = false
		pt.needNL = true
	case ast.BlockTH, ast.BlockTD:
		pt.inTD = false
	case ast.BlockUL:
		// Decrement based on EnterBlock's IsTight (from stack), not the final
		// IsTight — a list may start tight but become loose mid-stream.
		wasTight := true
		if n := len(pt.tightListEntered); n > 0 {
			wasTight = pt.tightListEntered[n-1]
			pt.tightListEntered = pt.tightListEntered[:n-1]
		}
		if wasTight {
			pt.tightListDepth--
			if pt.tightListDepth < 0 {
				pt.tightListDepth = 0
			}
			// Tight list: blocks within items are separated by \n, not blank lines.
			pt.needNL = true
		} else {
			// Loose list: blocks are separated by blank lines.
			pt.needBlank = true
		}
	case ast.BlockOL:
		wasTight := true
		if n := len(pt.tightListEntered); n > 0 {
			wasTight = pt.tightListEntered[n-1]
			pt.tightListEntered = pt.tightListEntered[:n-1]
		}
		if wasTight {
			pt.tightListDepth--
			if pt.tightListDepth < 0 {
				pt.tightListDepth = 0
			}
			pt.needNL = true
		} else {
			pt.needBlank = true
		}
	case ast.BlockLI:
		pt.inLI = false
		pt.needNL = true
	case ast.BlockFootnoteDefSection:
		pt.needNL = true
	default:
		pt.needNL = true
	}
	return nil
}

// EnterSpan — plain text mode: most spans are no-op.
func (pt *PlainText) EnterSpan(s ast.SpanType, detail any) error {
	// Footnote reference: output [N] to mark the reference position.
	// (md4c's plain renderer drops it; md4go's default is more useful.)
	if s == ast.SpanFootnoteRef {
		if d, ok := detail.(*ast.FootnoteRefDetail); ok {
			if err := pt.w.WriteByte('['); err != nil {
				return err
			}
			if err := pt.w.WriteString(itoa(int(d.ID))); err != nil {
				return err
			}
			if err := pt.w.WriteByte(']'); err != nil {
				return err
			}
		}
	}
	return nil
}

// LeaveSpan — plain text mode: most spans are no-op.
func (pt *PlainText) LeaveSpan(ast.SpanType, any) error { return nil }

func itoa(n int) string { return strconv.Itoa(n) }

// Flush flushes buffered output to the underlying writer.
func (pt *PlainText) Flush() error {
	if pt.needNL {
		if err := pt.w.WriteByte('\n'); err != nil {
			return err
		}
		pt.needNL = false
	}
	return pt.w.Flush()
}
