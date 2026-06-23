package diffcheck

import "md4go/parser"

// Flags is an alias for parser.Flags, re-exported for CLI convenience.
type Flags = parser.Flags

// Exported flag constants for CLI convenience.
var (
	DialectCommonMarkFlags = parser.DialectCommonMark
	DialectGitHubFlags     = parser.DialectGitHub
	GoldmarkCompatFlags    = parser.GoldmarkCompat
)
