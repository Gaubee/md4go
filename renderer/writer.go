package renderer

import (
	"bufio"
	"io"
)

// BufWriter wraps a bufio.Writer with convenience methods.
// Inspired by goldmark's util.BufWriter pattern — buffers writes to minimise
// syscall overhead, while exposing direct []byte writes for zero-copy.
type BufWriter struct {
	bw *bufio.Writer
}

// NewBufWriter creates a BufWriter wrapping w with a 4 KB buffer.
func NewBufWriter(w io.Writer) *BufWriter {
	return &BufWriter{bw: bufio.NewWriterSize(w, 4096)}
}

// Write writes raw bytes — zero-copy path for renderer output.
func (b *BufWriter) Write(p []byte) (int, error) {
	return b.bw.Write(p)
}

// WriteByte writes a single byte.
func (b *BufWriter) WriteByte(c byte) error {
	return b.bw.WriteByte(c)
}

// WriteString writes a string literal.
func (b *BufWriter) WriteString(s string) error {
	_, err := b.bw.WriteString(s)
	return err
}

// Flush flushes the internal buffer to the underlying writer.
func (b *BufWriter) Flush() error {
	return b.bw.Flush()
}
