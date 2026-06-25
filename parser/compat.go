package parser

// compatConfig pre-computes parser-level compatibility behavior decisions
// from Flags, avoiding repeated bitmask checks in hot paths.
type compatConfig struct {
	protectDoublePipe bool // FlagProtectDoublePipe OR FlagSpoilers: protect || from cell splitting
	strictTableCols   bool // FlagStrictTableColumns: reject tables whose header has MORE cells than the delimiter (mirrors goldmark's extension/table.go Transform reject path)
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

// validateTableColumns validates the header row's column count against
// the delimiter row's column count.
//
// When strict=false (default), always returns true (lenient — accepts any
// mismatch; aligns with md4c).
//
// When strict=true (FlagStrictTableColumns set), mirrors goldmark's
// extension/table.go Transform() behavior:
//
//	header.ChildCount() > len(alignments) → REJECT (header has more cells than
//	  the delimiter → table is not recognized; lines fall back to paragraph)
//	header.ChildCount() <= len(alignments) → ACCEPT (goldmark's parseRow pads
//	  the header with empty cells to match the delimiter count; md4go's
//	  emitTableRow also pads shorter rows with empty cells, so outputs match)
//
// Empirical verification (2026-06-25): this asymmetric rule eliminates
// ~640 md4go≠goldmark diffs in the column-mismatch class without introducing
// new ones (the previous symmetric `==` check rejected tables that goldmark
// accepts via padding, causing +700 regressions).
//
// Uses splitTableCells with the same compatConfig to ensure consistent
// column counting between validation and rendering.
func validateTableColumns(headerLine []byte, underlineCols int, cc compatConfig) bool {
	if !cc.strictTableCols {
		return true
	}
	headerCols := len(splitTableCells(headerLine, cc))
	return headerCols <= underlineCols
}
