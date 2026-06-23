package parser

// Flags is a bitmask of parser behavior switches.
//
// # Design Principles
//
//  1. Default (Flags=0) = CommonMark 0.31 standard compliance — no extensions, no behavior modifications.
//  2. Extension flags enable non-standard syntax extensions (opt-in).
//  3. Behavior flags modify standard parsing behavior (opt-in).
//  4. Compatibility flags enable non-standard improvement points or align with other implementations (opt-in).
//  5. Dialect presets combine feature flags for well-known markdown flavors.
//  6. All flags are modular and pluggable — combine freely via bitwise OR.
//
// Mirrors MD_FLAG_* values from md4c.h for 1:1 compatibility.
// Bit values 0x1–0x200000 match md4c exactly; 0x400000+ are md4go-specific.
type Flags uint32

const (
	// ═══════════════════════════════════════════════════════════════
	// Behavior Modification Flags (md4c MD_FLAG_* aligned)
	// ═══════════════════════════════════════════════════════════════
	// These flags modify CommonMark standard parsing behavior.
	// Default (unset) = CommonMark standard behavior.

	// FlagCollapseWhitespace collapses non-trivial whitespace in
	// MD_TEXT_NORMAL into a single space. Mirrors MD_FLAG_COLLAPSEWHITESPACE.
	FlagCollapseWhitespace Flags = 0x1

	// FlagPermissiveATXHeaders allows ATX headers without a space
	// after # (e.g. "###header"). Mirrors MD_FLAG_PERMISSIVEATXHEADERS.
	FlagPermissiveATXHeaders Flags = 0x2

	// FlagNoIndentedCodeBlocks disables indented code blocks.
	// Only fenced code blocks are recognized. Mirrors MD_FLAG_NOINDENTEDCODEBLOCKS.
	FlagNoIndentedCodeBlocks Flags = 0x10

	// FlagNoHTMLBlocks disables raw HTML blocks. Mirrors MD_FLAG_NOHTMLBLOCKS.
	FlagNoHTMLBlocks Flags = 0x20

	// FlagNoHTMLSpans disables raw HTML inline spans. Mirrors MD_FLAG_NOHTMLSPANS.
	FlagNoHTMLSpans Flags = 0x40

	// FlagUnderline makes _ produce <u> tags instead of <em> emphasis.
	// Also disables _ for normal emphasis. Mirrors MD_FLAG_UNDERLINE.
	FlagUnderline Flags = 0x4000

	// FlagHardSoftBreaks forces all soft breaks to act as hard breaks.
	// Mirrors MD_FLAG_HARD_SOFT_BREAKS.
	FlagHardSoftBreaks Flags = 0x8000

	// ═══════════════════════════════════════════════════════════════
	// Extension Syntax Flags (md4c MD_FLAG_* aligned)
	// ═══════════════════════════════════════════════════════════════
	// These flags enable non-standard syntax extensions.
	// Default (unset) = extension disabled (standard CommonMark only).

	// FlagPermissiveURLAutolinks recognizes URLs as autolinks even
	// without <, >. Mirrors MD_FLAG_PERMISSIVEURLAUTOLINKS.
	FlagPermissiveURLAutolinks Flags = 0x4

	// FlagPermissiveEmailAutolinks recognizes emails as autolinks even
	// without <, > and "mailto:". Mirrors MD_FLAG_PERMISSIVEEMAILAUTOLINKS.
	FlagPermissiveEmailAutolinks Flags = 0x8

	// FlagPermissiveWWWAutolinks recognizes "www." URLs as autolinks
	// without a scheme prefix. Mirrors MD_FLAG_PERMISSIVEWWWAUTOLINKS.
	FlagPermissiveWWWAutolinks Flags = 0x400

	// FlagTables enables GFM table syntax. Mirrors MD_FLAG_TABLES.
	FlagTables Flags = 0x100

	// FlagStrikethrough enables GFM strikethrough syntax (~~text~~).
	// Mirrors MD_FLAG_STRIKETHROUGH.
	FlagStrikethrough Flags = 0x200

	// FlagTasklists enables GFM task list syntax (- [x] and - [ ]).
	// Mirrors MD_FLAG_TASKLISTS.
	FlagTasklists Flags = 0x800

	// FlagLatexMathSpans enables $...$ and $$...$$ LaTeX math spans.
	// Mirrors MD_FLAG_LATEXMATHSPANS.
	FlagLatexMathSpans Flags = 0x1000

	// FlagWikilinks enables [[target]] and [[target|label]] wiki links.
	// Mirrors MD_FLAG_WIKILINKS.
	FlagWikilinks Flags = 0x2000

	// FlagSpoilers enables ||spoiler|| spans. Mirrors MD_FLAG_SPOILERS.
	FlagSpoilers Flags = 0x10000

	// FlagSuperscripts enables ^superscript^ spans. Mirrors MD_FLAG_SUPERSCRIPTS.
	FlagSuperscripts Flags = 0x20000

	// FlagSubscripts enables ~subscript~ spans. Mirrors MD_FLAG_SUBSCRIPTS.
	FlagSubscripts Flags = 0x40000

	// FlagAdmonitions enables admonition blocks (> [!type]).
	// Mirrors MD_FLAG_ADMONITIONS.
	FlagAdmonitions Flags = 0x80000

	// FlagFootnotes enables [^label] footnote references.
	// Mirrors MD_FLAG_FOOTNOTES.
	FlagFootnotes Flags = 0x100000

	// FlagHighlight enables ==highlight== spans. Mirrors MD_FLAG_HIGHLIGHT.
	FlagHighlight Flags = 0x200000

	// ═══════════════════════════════════════════════════════════════
	// Compatibility Flags (md4go-specific, not in md4c)
	// ═══════════════════════════════════════════════════════════════
	// These flags enable non-standard improvement points or align output
	// with other implementations. Default (unset) = GFM/CommonMark standard
	// compliance. Not included in any Dialect preset — must be explicitly set.

	// FlagTableInterruptParagraph allows a table delimiter row to interrupt
	// a multi-line paragraph. When enabled, the last line of the paragraph
	// is promoted to the table header row; preceding lines remain as paragraph.
	//
	// This aligns with goldmark's Table extension behavior but diverges from
	// the GFM spec and md4c, which require a blank line before the table.
	FlagTableInterruptParagraph Flags = 0x400000

	// FlagProtectDoublePipe protects || from being split as a table cell
	// boundary. This is an md4go improvement — GFM spec treats | as the cell
	// delimiter, so || means an empty cell followed by content.
	// Default: split || per GFM standard (aligns with md4c).
	FlagProtectDoublePipe Flags = 0x800000

	// FlagStrictTableColumns requires table header and delimiter rows to
	// have matching column counts (GFM standard, aligns with goldmark).
	// Default: lenient — does not validate column count match (md4c behavior).
	FlagStrictTableColumns Flags = 0x1000000

	// FlagDecodeEntities decodes HTML entities to Unicode in plain text
	// output (goldmark behavior). Default: preserves entity text verbatim
	// (e.g. &amp; stays &amp;). No plain-text rendering standard; both
	// behaviors are reasonable.
	FlagDecodeEntities Flags = 0x10000000
)

// ═══════════════════════════════════════════════════════════════
// Convenience Combinations
// ═══════════════════════════════════════════════════════════════

const (
	// PermissiveAutolinks enables all three permissive autolink types
	// (URL + Email + WWW). Mirrors MD_FLAG_PERMISSIVEAUTOLINKS.
	PermissiveAutolinks = FlagPermissiveURLAutolinks | FlagPermissiveEmailAutolinks | FlagPermissiveWWWAutolinks

	// NoHTML disables both HTML blocks and inline spans.
	// Mirrors MD_FLAG_NOHTML.
	NoHTML = FlagNoHTMLBlocks | FlagNoHTMLSpans
)

// ═══════════════════════════════════════════════════════════════
// Dialect Presets
// ═══════════════════════════════════════════════════════════════
// Dialect presets combine feature flags for well-known markdown flavors.
// Each preset is composed of base, readable feature flags.
// Default (Flags=0) = DialectCommonMark (pure CommonMark standard).

const (
	// DialectCommonMark is pure CommonMark 0.31 with no extensions.
	// Equivalent to md4c's MD_DIALECT_COMMONMARK (0).
	// Use this for strict CommonMark compliance.
	DialectCommonMark Flags = 0

	// DialectGitHub enables the GitHub Flavored Markdown feature set.
	// Equivalent to md4c's MD_DIALECT_GITHUB.
	// Composed of: permissive autolinks + tables + strikethrough +
	// tasklists + admonitions + footnotes.
	DialectGitHub = PermissiveAutolinks | FlagTables | FlagStrikethrough | FlagTasklists | FlagAdmonitions | FlagFootnotes
)

// ═══════════════════════════════════════════════════════════════
// Compatibility Presets
// ═══════════════════════════════════════════════════════════════
// Compatibility presets combine compatibility flags for alignment with
// specific implementations. These are NOT dialect presets — they modify
// rendering behavior to match other engines, not to enable features.
// Combine with a Dialect preset: DialectGitHub | GoldmarkCompat.
//
// Note: md4go's defaults already follow CommonMark/GFM standards, so no
// preset is needed to align with md4c's standard behavior. Only non-standard
// improvement points (e.g. FlagProtectDoublePipe) and goldmark alignment
// flags are provided as opt-in.

const (
	// GoldmarkCompat aligns output with goldmark's behavior.
	// Includes FlagStrictTableColumns (goldmark enforces GFM column count match).
	GoldmarkCompat = FlagTableInterruptParagraph | FlagStrictTableColumns | FlagDecodeEntities
)
