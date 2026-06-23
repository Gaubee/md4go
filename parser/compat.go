package parser

// compatConfig pre-computes parser-level compatibility behavior decisions
// from Flags, avoiding repeated bitmask checks in hot paths.
type compatConfig struct {
	protectDoublePipe bool // FlagProtectDoublePipe OR FlagSpoilers: protect || from cell splitting
	strictTableCols   bool // FlagStrictTableColumns: validate column count match
}

// newCompatConfig builds a compatConfig from parser flags.
func newCompatConfig(flags Flags) compatConfig {
	return compatConfig{
		// || protection is active when either FlagProtectDoublePipe (md4go improvement)
		// or FlagSpoilers (spoiler syntax requires || as delimiter, not cell boundary) is set.
		protectDoublePipe: flags&FlagProtectDoublePipe != 0 || flags&FlagSpoilers != 0,
		strictTableCols:   flags&FlagStrictTableColumns != 0,
	}
}

// protectDoublePipeInCells protects || from being split as table cell boundary.
// When protect=true (FlagProtectDoublePipe set), || is protected (md4go improvement).
// When protect=false (default), || is split per GFM standard (aligns with md4c).
func protectDoublePipeInCells(line []byte, protected []bool, protect bool) {
	if !protect {
		return // GFM standard: don't protect, || splits into cell boundaries
	}
	skipSpoilers(line, protected) // md4go improvement: protect ||
}

// validateTableColumns validates that header row column count matches underline.
// When strict=false (default), always returns true (lenient, aligns with md4c).
// When strict=true (FlagStrictTableColumns set), requires exact match (GFM standard, aligns with goldmark).
// Uses splitTableCells with the same compatConfig to ensure consistent column counting.
func validateTableColumns(headerLine []byte, underlineCols int, cc compatConfig) bool {
	if !cc.strictTableCols {
		return true
	}
	return len(splitTableCells(headerLine, cc)) == underlineCols
}
