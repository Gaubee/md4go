package parser

// codespan.go implements code span (backtick) parsing.
// Mirrors md4c md_is_code_span() + the backtick handling in md_collect_marks().
//
// Key design decisions (from md4c):
//   - Code spans are resolved IMMEDIATELY during collectMarks (Phase 1),
//     not deferred to later phases. This gives them highest priority:
//     their content is opaque — no other marks inside are interpreted.
//   - Shortest match: opener with n backticks matches closer with >= n backticks,
//     taking the shortest (first) matching closer.
//   - codespanMaxLen = 32: hard limit preventing O(n²) pathological inputs.
//
// The actual collectCodeSpanMark function lives in mark.go alongside other
// mark collection logic. This file contains the code span content trimming
// and processing helpers.

// trimCodeSpanContent strips leading/trailing whitespace from code span content
// per CommonMark §6.3:
// "If the resulting string both begins and ends with a space character,
// but does not consist entirely of space characters, a single space
// character is removed from both the beginning and the end."
//
// Also collapses internal newlines to spaces (CommonMark: line endings are
// treated as spaces).
func trimCodeSpanContent(content []byte) []byte {
	if len(content) == 0 {
		return content
	}

	// First, collapse internal newlines to spaces
	hasNewline := false
	for _, c := range content {
		if c == '\n' {
			hasNewline = true
			break
		}
	}

	var text []byte
	if hasNewline {
		text = make([]byte, len(content))
		for i, c := range content {
			if c == '\n' {
				text[i] = ' '
			} else {
				text[i] = c
			}
		}
	} else {
		text = content
	}

	// Strip one leading and one trailing space/newline if both ends have
	// whitespace AND content is not all whitespace.
	if len(text) > 0 && (text[0] == ' ') &&
		(text[len(text)-1] == ' ') {
		// Check not all spaces
		allSpaces := true
		for _, c := range text {
			if c != ' ' {
				allSpaces = false
				break
			}
		}
		if !allSpaces {
			return text[1 : len(text)-1]
		}
	}
	return text
}
