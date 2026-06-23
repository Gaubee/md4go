// Package stream provides the LineSource abstraction for incremental line input.
//
// md4c's first pass is a while(off < size) line loop — inherently incremental.
// LineSource abstracts "give me the next line" so that Convert ([]byte) and
// ConvertStream (io.Reader) share the same parsing core.
package stream

import (
	"bufio"
	"bytes"
	"io"
)

// LineSource provides lines one at a time without loading the entire document.
// Mirrors md4c's while(off < size) loop, but abstracted as an interface.
type LineSource interface {
	// NextLine returns the next line WITHOUT trailing newline.
	// ok is false when no more lines are available.
	// The returned slice must remain valid across subsequent NextLine calls.
	NextLine() (line []byte, ok bool, err error)

	// LineNumber returns the 1-based line number of the last line returned
	// by NextLine, or 0 if no lines have been read yet.
	LineNumber() int

	// DocEndsWithNewline reports whether the input ends with a newline.
	// Mirrors md4c's ctx.doc_ends_with_newline.
	// Only reliable after all lines have been read (ok == false).
	DocEndsWithNewline() bool
}

// SliceSource implements LineSource over a []byte — used by Convert.
// The backing []byte persists for the full parse, so returned slices
// can be direct references (zero-copy).
type SliceSource struct {
	src                []byte
	off                int
	lineNum            int
	docEndsWithNewline bool
}

// NewSliceSource creates a LineSource backed by a []byte.
func NewSliceSource(src []byte) *SliceSource {
	ends := len(src) > 0 && (src[len(src)-1] == '\n' || src[len(src)-1] == '\r')
	return &SliceSource{src: src, docEndsWithNewline: ends}
}

// NextLine returns the next line from the byte slice.
// The returned slice references the original []byte (zero-copy).
func (s *SliceSource) NextLine() (line []byte, ok bool, err error) {
	if s.off >= len(s.src) {
		return nil, false, nil
	}
	end := bytes.IndexByte(s.src[s.off:], '\n')
	if end < 0 {
		line := s.src[s.off:]
		s.off = len(s.src)
		s.lineNum++
		return line, true, nil
	}
	line = s.src[s.off : s.off+end]
	s.off += end + 1
	// Trim trailing \r for CRLF compatibility
	if len(line) > 0 && line[len(line)-1] == '\r' {
		line = line[:len(line)-1]
	}
	s.lineNum++
	return line, true, nil
}

// LineNumber returns the 1-based number of the last line read.
func (s *SliceSource) LineNumber() int { return s.lineNum }

// DocEndsWithNewline reports whether the input ended with a newline.
func (s *SliceSource) DocEndsWithNewline() bool { return s.docEndsWithNewline }

// ReaderSource implements LineSource over an io.Reader — used by ConvertStream.
// Each NextLine call returns a stable copy (not a view into the Scanner buffer).
type ReaderSource struct {
	scanner            *bufio.Scanner
	lineNum            int
	lastLine           []byte // last line returned, for trailing-newline detection
	docEndsWithNewline bool
	done               bool
}

// NewReaderSource creates a LineSource backed by an io.Reader.
// It uses a large buffer (1 MB) to handle long lines.
func NewReaderSource(r io.Reader) *ReaderSource {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024) // up to 1 MB per line
	scanner.Split(bufio.ScanLines)
	return &ReaderSource{scanner: scanner}
}

// NextLine returns the next line from the reader.
// The returned slice is a copy of the Scanner's internal buffer,
// so it remains valid after subsequent NextLine calls.
// This is critical: bufio.Scanner.Bytes() reuses its internal buffer,
// but the parser stores line content in block stacks that outlive one
// iteration — without copying, stored lines would be corrupted.
func (rs *ReaderSource) NextLine() (line []byte, ok bool, err error) {
	if !rs.scanner.Scan() {
		rs.done = true
		// If the last line was non-empty and didn't end with newline,
		// the document doesn't end with newline.
		// bufio.Scanner strips the trailing \n, so if the last call
		// returned a line, it was newline-terminated (or last line of file).
		// We track via the last line's content — if it was empty, the
		// previous line had a trailing newline.
		return nil, false, rs.scanner.Err()
	}
	raw := rs.scanner.Bytes()
	// Must copy: scanner.Bytes() is only valid until next Scan().
	line = append([]byte(nil), raw...)
	rs.lineNum++
	rs.lastLine = line
	return line, true, nil
}

// LineNumber returns the 1-based number of the last line read.
func (rs *ReaderSource) LineNumber() int { return rs.lineNum }

// DocEndsWithNewline reports whether the input ended with a newline.
// For ReaderSource, this is a best-effort heuristic: if the last line
// returned by Scanner was empty (meaning the file ended with \n\n or \n),
// the document ends with a newline. Otherwise, it may or may not.
// This is less precise than SliceSource, but sufficient for plaintext
// rendering where trailing newline handling is a minor concern.
func (rs *ReaderSource) DocEndsWithNewline() bool {
	if !rs.done {
		return false
	}
	// If the last line was empty, the input ended with \n
	return len(rs.lastLine) == 0
}
