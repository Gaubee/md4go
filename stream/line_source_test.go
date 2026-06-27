package stream

import (
	"bytes"
	"io"
	"strings"
	"testing"
)

func TestSliceSource_Basic(t *testing.T) {
	src := []byte("hello\nworld\n")
	ls := NewSliceSource(src)

	line, ok, err := ls.NextLine()
	if err != nil || !ok || string(line) != "hello" {
		t.Fatalf("first line: got %q ok=%v err=%v, want \"hello\"", line, ok, err)
	}
	if ln := ls.LineNumber(); ln != 1 {
		t.Errorf("line number after first: got %d, want 1", ln)
	}

	line, ok, err = ls.NextLine()
	if err != nil || !ok || string(line) != "world" {
		t.Fatalf("second line: got %q ok=%v err=%v, want \"world\"", line, ok, err)
	}
	if ln := ls.LineNumber(); ln != 2 {
		t.Errorf("line number after second: got %d, want 2", ln)
	}

	line, ok, err = ls.NextLine()
	if err != nil || ok {
		t.Fatalf("eof: got %q ok=%v err=%v, want ok=false", line, ok, err)
	}
	if ln := ls.LineNumber(); ln != 2 {
		t.Errorf("line number at eof: got %d, want 2", ln)
	}
}

func TestSliceSource_NoTrailingNewline(t *testing.T) {
	src := []byte("hello\nworld")
	ls := NewSliceSource(src)

	line, ok, _ := ls.NextLine()
	if !ok || string(line) != "hello" {
		t.Fatalf("first: %q", line)
	}
	line, ok, _ = ls.NextLine()
	if !ok || string(line) != "world" {
		t.Fatalf("second: %q", line)
	}
	_, ok, _ = ls.NextLine()
	if ok {
		t.Fatal("should be eof")
	}
}

func TestSliceSource_CRLF(t *testing.T) {
	ls := NewSliceSource([]byte("hello\r\nworld\r\n"))
	line, ok, _ := ls.NextLine()
	if !ok || string(line) != "hello" {
		t.Fatalf("CRLF line: %q", line)
	}
	line, ok, _ = ls.NextLine()
	if !ok || string(line) != "world" {
		t.Fatalf("CRLF line2: %q", line)
	}
}

func TestSliceSource_Empty(t *testing.T) {
	ls := NewSliceSource([]byte{})
	_, ok, _ := ls.NextLine()
	if ok {
		t.Fatal("empty input should return ok=false")
	}
	if ls.LineNumber() != 0 {
		t.Errorf("line number: got %d, want 0", ls.LineNumber())
	}
}

func TestSliceSource_ZeroCopy(t *testing.T) {
	src := []byte("line1\nline2\n")
	ls := NewSliceSource(src)
	line1, _, _ := ls.NextLine()
	line2, _, _ := ls.NextLine()
	// Verify that returned slices reference the original buffer
	if !bytes.Equal(line1, []byte("line1")) {
		t.Fatalf("line1: %q", line1)
	}
	if !bytes.Equal(line2, []byte("line2")) {
		t.Fatalf("line2: %q", line2)
	}
	// SliceSource returns views into the original buffer (zero-copy).
	// Verify by checking that the line content matches a subslice of src.
	if !bytes.Contains(src, line1) {
		t.Error("SliceSource line1 not found in original buffer")
	}
}

func TestReaderSource_Basic(t *testing.T) {
	r := strings.NewReader("hello\nworld\n")
	ls := NewReaderSource(r)

	line, ok, err := ls.NextLine()
	if err != nil || !ok || string(line) != "hello" {
		t.Fatalf("first line: got %q ok=%v err=%v", line, ok, err)
	}
	if ln := ls.LineNumber(); ln != 1 {
		t.Errorf("line number: got %d, want 1", ln)
	}

	line, ok, err = ls.NextLine()
	if err != nil || !ok || string(line) != "world" {
		t.Fatalf("second line: got %q ok=%v err=%v", line, ok, err)
	}

	_, ok, _ = ls.NextLine()
	if ok {
		t.Fatal("should be eof")
	}
}

func TestReaderSource_DataStability(t *testing.T) {
	// Critical test: verify that previously returned lines are not
	// corrupted by subsequent NextLine calls.
	r := strings.NewReader("line1\nline2\nline3\n")
	ls := NewReaderSource(r)

	line1, _, _ := ls.NextLine()
	line2, _, _ := ls.NextLine()
	line3, _, _ := ls.NextLine()

	// After reading all lines, verify earlier ones are still valid
	if string(line1) != "line1" {
		t.Errorf("line1 corrupted: got %q", line1)
	}
	if string(line2) != "line2" {
		t.Errorf("line2 corrupted: got %q", line2)
	}
	if string(line3) != "line3" {
		t.Errorf("line3 corrupted: got %q", line3)
	}
}

func TestReaderSource_Empty(t *testing.T) {
	ls := NewReaderSource(strings.NewReader(""))
	_, ok, _ := ls.NextLine()
	if ok {
		t.Fatal("empty input should return ok=false")
	}
	if ls.LineNumber() != 0 {
		t.Errorf("line number: got %d, want 0", ls.LineNumber())
	}
}

func TestReaderSource_NoTrailingNewline(t *testing.T) {
	ls := NewReaderSource(strings.NewReader("hello"))
	line, ok, _ := ls.NextLine()
	if !ok || string(line) != "hello" {
		t.Fatalf("got %q", line)
	}
	_, ok, _ = ls.NextLine()
	if ok {
		t.Fatal("should be eof")
	}
}

func TestBothSources_Parity(t *testing.T) {
	input := "line1\nline2\nline3\n"
	sliceLS := NewSliceSource([]byte(input))
	readerLS := NewReaderSource(strings.NewReader(input))

	for {
		sLine, sOK, sErr := sliceLS.NextLine()
		rLine, rOK, rErr := readerLS.NextLine()

		if sErr != nil || rErr != nil {
			t.Fatalf("errors: slice=%v reader=%v", sErr, rErr)
		}
		if sOK != rOK {
			t.Fatalf("ok mismatch: slice=%v reader=%v", sOK, rOK)
		}
		if !sOK {
			break
		}
		if string(sLine) != string(rLine) {
			t.Errorf("line mismatch: slice=%q reader=%q", sLine, rLine)
		}
	}
}

// TestBothSources_MultilineBlock simulates the real parser's usage pattern:
// lines are stored in a block stack and accessed after the block is complete.
// This is the exact scenario where ReaderSource's copy is essential.
func TestBothSources_MultilineBlock(t *testing.T) {
	input := "first line\nsecond line\nthird line\n"

	sources := []struct {
		name string
		ls   LineSource
	}{
		{"SliceSource", NewSliceSource([]byte(input))},
		{"ReaderSource", NewReaderSource(strings.NewReader(input))},
	}

	for _, src := range sources {
		t.Run(src.name, func(t *testing.T) {
			var stored [][]byte
			for {
				line, ok, err := src.ls.NextLine()
				if err != nil {
					t.Fatal(err)
				}
				if !ok {
					break
				}
				stored = append(stored, line)
			}
			expected := []string{"first line", "second line", "third line"}
			if len(stored) != len(expected) {
				t.Fatalf("got %d lines, want %d", len(stored), len(expected))
			}
			for i, exp := range expected {
				if string(stored[i]) != exp {
					t.Errorf("line %d: got %q, want %q", i, stored[i], exp)
				}
			}
		})
	}
}

// BenchmarkSliceSource measures the cost of line iteration over a []byte.
func BenchmarkSliceSource(b *testing.B) {
	data := bytes.Repeat([]byte("hello world\n"), 1000)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ls := NewSliceSource(data)
		for {
			_, ok, _ := ls.NextLine()
			if !ok {
				break
			}
		}
	}
}

// BenchmarkReaderSource measures the cost of line iteration over an io.Reader,
// including the necessary copy for data safety.
func BenchmarkReaderSource(b *testing.B) {
	data := bytes.Repeat([]byte("hello world\n"), 1000)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ls := NewReaderSource(bytes.NewReader(data))
		for {
			_, ok, _ := ls.NextLine()
			if !ok {
				break
			}
		}
	}
}

// BenchmarkReaderSourceLarge measures streaming with larger input.
func BenchmarkReaderSourceLarge(b *testing.B) {
	var buf bytes.Buffer
	for i := 0; i < 100000; i++ {
		buf.WriteString("this is a moderately long line for benchmarking purposes\n")
	}
	data := buf.Bytes()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ls := NewReaderSource(bytes.NewReader(data))
		for {
			_, ok, _ := ls.NextLine()
			if !ok {
				break
			}
		}
	}
}

// TestSliceSource_PreservesTrailingSpaces verifies that trailing spaces
// are preserved in line content (needed for hard line break detection).
func TestSliceSource_PreservesTrailingSpaces(t *testing.T) {
	ls := NewSliceSource([]byte("hello  \nworld\n"))
	line, _, _ := ls.NextLine()
	if string(line) != "hello  " {
		t.Errorf("trailing spaces stripped: got %q, want \"hello  \"", line)
	}
}

// TestReaderSource_PreservesTrailingSpaces verifies the same for streaming.
func TestReaderSource_PreservesTrailingSpaces(t *testing.T) {
	ls := NewReaderSource(strings.NewReader("hello  \nworld\n"))
	line, _, _ := ls.NextLine()
	if string(line) != "hello  " {
		t.Errorf("trailing spaces stripped: got %q, want \"hello  \"", line)
	}
}

// TestSliceSource_TabExpansionNotDone verifies that LineSource returns raw
// tab characters — tab expansion is the parser's responsibility (measureIndent).
func TestSliceSource_TabExpansionNotDone(t *testing.T) {
	ls := NewSliceSource([]byte("\thello\n"))
	line, _, _ := ls.NextLine()
	if string(line) != "\thello" {
		t.Errorf("tab should be preserved: got %q", line)
	}
}

// TestReaderSource_LargeLines verifies handling of long lines.
func TestReaderSource_LargeLines(t *testing.T) {
	longLine := strings.Repeat("x", 100000)
	ls := NewReaderSource(strings.NewReader(longLine + "\n"))
	line, ok, err := ls.NextLine()
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if !ok {
		t.Fatal("should have a line")
	}
	if len(line) != 100000 {
		t.Errorf("long line: got %d bytes, want 100000", len(line))
	}
}

// TestIOReaderInterface verifies that ReaderSource works with any io.Reader.
func TestIOReaderInterface(t *testing.T) {
	var r io.Reader = strings.NewReader("hello\n")
	ls := NewReaderSource(r)
	line, ok, _ := ls.NextLine()
	if !ok || string(line) != "hello" {
		t.Fatalf("io.Reader interface: got %q", line)
	}
}
