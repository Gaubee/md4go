package extension

import "md4go/parser"

// Strikethrough extension enables ~~strikethrough~~ syntax.
// Corresponds to md4c MD_FLAG_STRIKETHROUGH.
//
// Adds '~' to mark characters for strikethrough detection.
// Mirrors md4c md_setup_mark_char_map():
//
//	if(flags & MD_FLAG_STRIKETHROUGH) mark_char_map['~'] = 1;
type Strikethrough struct{}

// Extend implements parser.Extender.
func (e *Strikethrough) Extend(r parser.Registrar) {
	r.SetFlags(parser.FlagStrikethrough)
	r.AddMarkChar('~')
}

// Table extension enables GFM table syntax.
// Corresponds to md4c MD_FLAG_TABLES.
//
// Adds '|' to mark characters (for cell boundary detection) and registers
// a table block trigger. Mirrors md4c md_setup_mark_char_map():
//
//	if(flags & MD_FLAG_TABLES) mark_char_map['|'] = 1;
type Table struct{}

// Extend implements parser.Extender.
func (e *Table) Extend(r parser.Registrar) {
	r.SetFlags(parser.FlagTables)
	r.AddMarkChar('|')
}

// TaskList extension enables GFM task list syntax: - [ ] and - [x].
// Corresponds to md4c MD_FLAG_TASKLISTS.
//
// No new mark characters needed — task list detection happens during
// container mark processing (list item parsing), checking for [ ] / [x]
// after the list mark.
type TaskList struct{}

// Extend implements parser.Extender.
func (e *TaskList) Extend(r parser.Registrar) {
	r.SetFlags(parser.FlagTasklists)
}

// PermissiveAutolinks extension enables URL/email autolinks without angle brackets.
// Corresponds to md4c MD_FLAG_PERMISSIVEAUTOLINKS (URL + Email + WWW).
//
// Adds '@', ':', '.' to mark characters for permissive autolink detection.
// Mirrors md4c md_setup_mark_char_map() which conditionally adds these chars.
type PermissiveAutolinks struct{}

// Extend implements parser.Extender.
func (e *PermissiveAutolinks) Extend(r parser.Registrar) {
	r.SetFlags(parser.PermissiveAutolinks)
	r.AddMarkChar('@')
	r.AddMarkChar(':')
	r.AddMarkChar('.')
}

// Admonition extension enables admonition blocks (::: note ...).
// Corresponds to md4c MD_FLAG_ADMONITIONS.
type Admonition struct{}

// Extend implements parser.Extender.
func (e *Admonition) Extend(r parser.Registrar) {
	r.SetFlags(parser.FlagAdmonitions)
}

// Footnote extension enables footnote syntax [^label].
// Corresponds to md4c MD_FLAG_FOOTNOTES.
type Footnote struct{}

// Extend implements parser.Extender.
func (e *Footnote) Extend(r parser.Registrar) {
	r.SetFlags(parser.FlagFootnotes)
}

// LatexMath extension enables LaTeX math spans ($...$ and $$...$$).
// Corresponds to md4c MD_FLAG_LATEXMATHSPANS.
//
// Adds '$' to mark characters for LaTeX math detection.
// Mirrors md4c md_setup_mark_char_map():
//
//	if(flags & MD_FLAG_LATEXMATHSPANS) mark_char_map['$'] = 1;
type LatexMath struct{}

// Extend implements parser.Extender.
func (e *LatexMath) Extend(r parser.Registrar) {
	r.SetFlags(parser.FlagLatexMathSpans)
	r.AddMarkChar('$')
}

// Subscript extension enables subscript syntax (~sub~).
// Corresponds to md4c MD_FLAG_SUBSCRIPTS.
//
// Adds '~' to mark characters for subscript detection.
// Mirrors md4c md_setup_mark_char_map():
//
//	if(flags & MD_FLAG_SUBSCRIPTS) mark_char_map['~'] = 1;
type Subscript struct{}

// Extend implements parser.Extender.
func (e *Subscript) Extend(r parser.Registrar) {
	r.SetFlags(parser.FlagSubscripts)
	r.AddMarkChar('~')
}

// Superscript extension enables superscript syntax (^super^).
// Corresponds to md4c MD_FLAG_SUPERSCRIPTS.
//
// Adds '^' to mark characters for superscript detection.
// Mirrors md4c md_setup_mark_char_map():
//
//	if(flags & MD_FLAG_SUPERSCRIPTS) mark_char_map['^'] = 1;
type Superscript struct{}

// Extend implements parser.Extender.
func (e *Superscript) Extend(r parser.Registrar) {
	r.SetFlags(parser.FlagSuperscripts)
	r.AddMarkChar('^')
}

// Highlight extension enables highlight syntax (==highlight==).
// Corresponds to md4c MD_FLAG_HIGHLIGHT.
//
// Adds '=' to mark characters for highlight detection.
// Mirrors md4c md_setup_mark_char_map():
//
//	if(flags & MD_FLAG_HIGHLIGHT) mark_char_map['='] = 1;
type Highlight struct{}

// Extend implements parser.Extender.
func (e *Highlight) Extend(r parser.Registrar) {
	r.SetFlags(parser.FlagHighlight)
	r.AddMarkChar('=')
}

// Spoiler extension enables spoiler syntax (||spoiler||).
// Corresponds to md4c MD_FLAG_SPOILERS.
//
// The '|' mark character may already be registered by the Table extender.
// This extender sets the flag and ensures '|' is a mark char.
type Spoiler struct{}

// Extend implements parser.Extender.
func (e *Spoiler) Extend(r parser.Registrar) {
	r.SetFlags(parser.FlagSpoilers)
	r.AddMarkChar('|')
}

// Wikilink extension enables wikilink syntax ([[target]] and [[target|label]]).
// Corresponds to md4c MD_FLAG_WIKILINKS.
//
// Adds '|' to mark characters (for label delimiter detection) if not already
// registered by the Table or Spoiler extenders.
// Mirrors md4c md_setup_mark_char_map():
//
//	if(flags & MD_FLAG_WIKILINKS) mark_char_map['|'] = 1;
type Wikilink struct{}

// Extend implements parser.Extender.
func (e *Wikilink) Extend(r parser.Registrar) {
	r.SetFlags(parser.FlagWikilinks)
	r.AddMarkChar('|')
}

// GFM is a preset of extensions matching md4c's MD_DIALECT_GITHUB.
// It enables: Permissive Autolinks, Tables, Strikethrough, Task Lists,
// Admonitions, and Footnotes.
//
// Usage:
//
//	md := md4go.New(md4go.WithExtensions(extension.GFM...))
var GFM = []parser.Extender{
	&PermissiveAutolinks{},
	&Table{},
	&Strikethrough{},
	&TaskList{},
	&Admonition{},
	&Footnote{},
}
