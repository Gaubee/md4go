package extension

import "github.com/userpro/md4go/parser"

// Strikethrough extension enables ~~strikethrough~~ syntax.
//
// Adds '~' to mark characters for strikethrough detection.
type Strikethrough struct{}

// Extend implements parser.Extender.
func (e *Strikethrough) Extend(r parser.Registrar) {
	r.SetFlags(parser.FlagStrikethrough)
	r.AddMarkChar('~')
}

// Table extension enables GFM table syntax.
//
// Adds '|' to mark characters (for cell boundary detection) and registers
// a table block trigger.
type Table struct{}

// Extend implements parser.Extender.
func (e *Table) Extend(r parser.Registrar) {
	r.SetFlags(parser.FlagTables)
	r.AddMarkChar('|')
}

// TaskList extension enables GFM task list syntax: - [ ] and - [x].
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
//
// Adds '@', ':', '.' to mark characters for permissive autolink detection.
type PermissiveAutolinks struct{}

// Extend implements parser.Extender.
func (e *PermissiveAutolinks) Extend(r parser.Registrar) {
	r.SetFlags(parser.PermissiveAutolinks)
	r.AddMarkChar('@')
	r.AddMarkChar(':')
	r.AddMarkChar('.')
}

// Admonition extension enables admonition blocks (> [!TYPE] ...).
// Recognized types: note, tip, important, warning, caution.
type Admonition struct{}

// Extend implements parser.Extender.
func (e *Admonition) Extend(r parser.Registrar) {
	r.SetFlags(parser.FlagAdmonitions)
}

// Footnote extension enables footnote syntax [^label].
type Footnote struct{}

// Extend implements parser.Extender.
func (e *Footnote) Extend(r parser.Registrar) {
	r.SetFlags(parser.FlagFootnotes)
}

// LatexMath extension enables LaTeX math spans ($...$ and $$...$$).
//
// Adds '$' to mark characters for LaTeX math detection.
type LatexMath struct{}

// Extend implements parser.Extender.
func (e *LatexMath) Extend(r parser.Registrar) {
	r.SetFlags(parser.FlagLatexMathSpans)
	r.AddMarkChar('$')
}

// Subscript extension enables subscript syntax (~sub~).
//
// Adds '~' to mark characters for subscript detection.
type Subscript struct{}

// Extend implements parser.Extender.
func (e *Subscript) Extend(r parser.Registrar) {
	r.SetFlags(parser.FlagSubscripts)
	r.AddMarkChar('~')
}

// Superscript extension enables superscript syntax (^super^).
//
// Adds '^' to mark characters for superscript detection.
type Superscript struct{}

// Extend implements parser.Extender.
func (e *Superscript) Extend(r parser.Registrar) {
	r.SetFlags(parser.FlagSuperscripts)
	r.AddMarkChar('^')
}

// Highlight extension enables highlight syntax (==highlight==).
//
// Adds '=' to mark characters for highlight detection.
type Highlight struct{}

// Extend implements parser.Extender.
func (e *Highlight) Extend(r parser.Registrar) {
	r.SetFlags(parser.FlagHighlight)
	r.AddMarkChar('=')
}

// Spoiler extension enables spoiler syntax (||spoiler||).
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
//
// Adds '|' to mark characters (for label delimiter detection) if not already
// registered by the Table or Spoiler extenders.
type Wikilink struct{}

// Extend implements parser.Extender.
func (e *Wikilink) Extend(r parser.Registrar) {
	r.SetFlags(parser.FlagWikilinks)
	r.AddMarkChar('|')
}

// GFM is a preset of extensions for GitHub Flavored Markdown.
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
