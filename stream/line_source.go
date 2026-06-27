// Package stream provides the LineSource abstraction for incremental line input.
//
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
}

// SliceSource implements LineSource over a []byte — used by Convert.
// The backing []byte persists for the full parse, so returned slices
// can be direct references (zero-copy).
type SliceSource struct {
	src     []byte
	off     int
	lineNum int
}

// NewSliceSource creates a LineSource backed by a []byte.
func NewSliceSource(src []byte) *SliceSource {
	return &SliceSource{src: src}
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

// ReaderSource implements LineSource over an io.Reader — used by ConvertStream.
// Each NextLine call returns a stable copy (not a view into the Scanner buffer).
type ReaderSource struct {
	scanner *bufio.Scanner
	lineNum int
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
		return nil, false, rs.scanner.Err()
	}
	raw := rs.scanner.Bytes()
	// Must copy: scanner.Bytes() is only valid until next Scan().
	line = append([]byte(nil), raw...)
	rs.lineNum++
	return line, true, nil
}

// LineNumber returns the 1-based number of the last line read.
func (rs *ReaderSource) LineNumber() int { return rs.lineNum }
