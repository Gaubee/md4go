package text

import (
	// stdhtml aliases the standard library html package to avoid
	// collision with the md4go/html package.
	stdhtml "html"

	"github.com/userpro/md4go/parser"
)

// compatConfig pre-computes renderer-level compatibility behavior decisions
// from parser Flags, avoiding repeated bitmask checks in hot paths.
type compatConfig struct {
	decodeEntities bool // FlagDecodeEntities: decode HTML entities to Unicode
}

// newCompatConfig builds a compatConfig from parser flags.
func newCompatConfig(flags parser.Flags) compatConfig {
	return compatConfig{
		decodeEntities: flags&parser.FlagDecodeEntities != 0,
	}
}

// decodeEntity decodes an HTML entity to Unicode or returns it verbatim.
// When decode=true (FlagDecodeEntities set), decode (goldmark behavior).
// When decode=false (default), return raw entity text.
func decodeEntity(text []byte, decode bool) []byte {
	if !decode {
		return text
	}
	return []byte(stdhtml.UnescapeString(string(text)))
}
