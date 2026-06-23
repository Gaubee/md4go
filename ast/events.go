// Package ast defines event types for the push-based parsing model.
//
// This package deliberately contains NO node tree — only type enums and
// detail structs. This is the physical guarantee of the "no AST" constraint
// inherited from md4c's architecture.
package ast

// BlockType identifies block-level document elements.
// Mirrors MD_BLOCKTYPE in md4c.h.
type BlockType uint8

const (
	BlockDoc BlockType = iota
	BlockQuote
	BlockUL
	BlockOL
	BlockLI
	BlockHR
	BlockH
	BlockCode
	BlockHTML
	BlockP
	BlockTable
	BlockTHead
	BlockTBody
	BlockTR
	BlockTH
	BlockTD
	BlockFootnoteDefSection
	BlockFootnoteDef
	BlockAdmonition
)

// SpanType identifies inline span elements.
// Mirrors MD_SPANTYPE in md4c.h.
type SpanType uint8

const (
	SpanEm SpanType = iota
	SpanStrong
	SpanLink
	SpanImg
	SpanCode
	SpanDel
	SpanLatexMath
	SpanLatexMathDisplay
	SpanWikilink
	SpanU
	SpanSpoiler
	SpanSuperscript
	SpanSubscript
	SpanFootnoteRef
	SpanMark
)

// TextType classifies text content within a span.
// Mirrors MD_TEXTTYPE in md4c.h.
type TextType uint8

const (
	TextNormal TextType = iota
	TextNullChar
	TextBR
	TextSoftBR
	TextEntity
	TextCode
	TextHTML
	TextLatexMath
)

// Align specifies table cell alignment.
type Align uint8

const (
	AlignDefault Align = iota
	AlignLeft
	AlignCenter
	AlignRight
)

// --- Attribute type (mirrors MD_ATTRIBUTE in md4c.h:277-282) ---

// SubstrType identifies the type of a substring within an attribute value.
// Mirrors MD_TEXTTYPE usage in MD_ATTRIBUTE substr_types.
// Only SubstrNormal, SubstrEntity, and SubstrNullChar can appear in attributes.
type SubstrType uint8

const (
	SubstrNormal SubstrType = iota
	SubstrEntity
	SubstrNullChar
)

// Attribute represents a link/image attribute (href, title, src, lang, etc.)
// with substring type information for entity and NULL char handling.
//
// The Text field contains the processed text (backslash escapes resolved).
// SubTypes and SubOffsets describe how to split Text into typed substrings:
//   - SubstrNormal: pass through the appropriate escaping function
//   - SubstrEntity: translate the entity (e.g. &amp; → &) then escape
//   - SubstrNullChar: render as U+0000 (verbatim) or U+FFFD (HTML text)
//
// Invariant: len(SubOffsets) == len(SubTypes) + 1
//
//	SubOffsets[0] == 0
//	SubOffsets[last] == len(Text)
//
// Mirrors MD_ATTRIBUTE in md4c.h:277-282.
type Attribute struct {
	Text       []byte
	SubTypes   []SubstrType
	SubOffsets []int
}

// IsTrivial reports whether the attribute has no entities or NULL chars
// (i.e., a single NORMAL substring covering the entire text).
func (a Attribute) IsTrivial() bool {
	return len(a.SubTypes) <= 1
}

// NewAttribute creates a trivial attribute (single NORMAL substring) from raw text.
// Used when no entity/NULL char processing is needed.
func NewAttribute(text []byte) Attribute {
	return Attribute{
		Text:       text,
		SubTypes:   []SubstrType{SubstrNormal},
		SubOffsets: []int{0, len(text)},
	}
}

// EmptyAttribute is a zero-value attribute for use when no data is available.
var EmptyAttribute = Attribute{
	SubTypes:   []SubstrType{SubstrNormal},
	SubOffsets: []int{0, 0},
}

// --- Detail structs (mirrors MD_*_DETAIL in md4c.h) ---

// HeadingDetail carries the heading level (1–6) for BlockH.
type HeadingDetail struct {
	Level int
}

// CodeDetail carries fenced-code metadata for BlockCode.
type CodeDetail struct {
	Info      Attribute
	Lang      Attribute
	FenceChar byte
}

// ULDetail carries unordered-list metadata for BlockUL.
type ULDetail struct {
	IsTight bool
	Mark    byte
}

// OLDetail carries ordered-list metadata for BlockOL.
type OLDetail struct {
	Start   int
	IsTight bool
	Mark    byte
}

// LIDetail carries list-item metadata for BlockLI.
type LIDetail struct {
	IsTask      bool
	TaskMark    byte
	TaskMarkOff int
}

// TableDetail carries table dimensions for BlockTable.
type TableDetail struct {
	ColCount     int
	HeadRowCount int
	BodyRowCount int
}

// TDDetail carries alignment for BlockTH / BlockTD.
type TDDetail struct {
	Align Align
}

// LinkDetail carries link metadata for SpanLink.
type LinkDetail struct {
	Href       Attribute
	Title      Attribute
	IsAutolink bool
}

// ImgDetail carries image metadata for SpanImg.
type ImgDetail struct {
	Src   Attribute
	Title Attribute
}

// AdmonitionDetail carries admonition type for BlockAdmonition.
type AdmonitionDetail struct {
	Type Attribute
}

// FootnoteRefDetail carries footnote reference metadata for SpanFootnoteRef.
// Mirrors MD_SPAN_FOOTNOTE_REF_DETAIL in md4c.h:352-357.
type FootnoteRefDetail struct {
	ID    uint // 1-based identifier of the referenced footnote
	RefID uint // 1-based identifier of this reference among references to the same footnote
	Label Attribute
}

// FootnoteDefDetail carries footnote definition metadata for BlockFootnoteDef.
// Mirrors MD_BLOCK_FOOTNOTE_DEF_DETAIL in md4c.h:359-364.
type FootnoteDefDetail struct {
	ID       uint // 1-based identifier of this footnote
	RefCount uint // Number of references to this footnote
	Label    Attribute
}

// WikilinkDetail carries wikilink metadata for SpanWikilink.
// Mirrors MD_SPAN_WIKILINK_DETAIL in md4c.h:348-350.
type WikilinkDetail struct {
	Target Attribute // wikilink target (the part before |)
}
