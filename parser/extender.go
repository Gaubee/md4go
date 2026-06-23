package parser

// Extender is implemented by extensions to register their syntax handlers
// with the parser at construction time.
//
// Extender allows runtime injection of custom block triggers, inline mark
// characters, and behavior flags — enabling third-party extensions without
// modifying the parser core.
type Extender interface {
	// Extend registers this extension's handlers with the parser.
	// Called once during Parser construction, before any Parse() call.
	Extend(r Registrar)
}

// Registrar provides methods for extensions to configure the parser.
// Implemented by *Parser. Extensions receive a Registrar in Extend().
type Registrar interface {
	// RegisterBlockTrigger adds a block trigger for the given leading
	// character. Triggers are checked in registration order (priority).
	RegisterBlockTrigger(c byte, trigger BlockTrigger)

	// AddMarkChar marks a byte as a potential inline mark character.
	// collectMarks will stop at this character and dispatch to the
	// appropriate handler in its switch statement.
	AddMarkChar(c byte)

	// Flags returns the current parser flags.
	Flags() Flags

	// SetFlags adds the given flags to the parser's flag set.
	SetFlags(Flags)
}
