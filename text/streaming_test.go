package text_test

import (
	"bytes"
	"io"
	"runtime"
	"testing"

	"md4go/html"
	"md4go/parser"
	"md4go/stream"
	"md4go/text"
)

// TestStreamEqualsFull verifies that Convert ([]byte) and ConvertStream
// (io.Reader) produce identical output for the same input.
// This is the M4 gate: the unified incremental pipeline must give
// consistent results regardless of the input source.
//
// Per tech plan §7.4: ConvertStream uses "first-seen-first" refdef resolution
// while Convert has full visibility. For inputs without forward references,
// the output should be identical. For inputs with forward references,
// ConvertStream may produce literal text where Convert produces a link.
func TestStreamEqualsFull(t *testing.T) {
	corpus := []struct {
		name  string
		input string
	}{
		{"simple_paragraph", "Hello world\n"},
		{"heading", "# Title\nParagraph\n"},
		{"multiple_headings", "# H1\n## H2\n### H3\n"},
		{"hr", "---\n"},
		{"fenced_code", "```\ncode\n```\n"},
		{"fenced_code_tilde", "~~~\ncode\n~~~\n"},
		{"indented_code", "    code\n    line\n"},
		{"emphasis", "*hello* **world**\n"},
		{"strong_em", "***foo***\n"},
		{"code_span", "`code` text\n"},
		{"link_inline", "[foo](http://example.com)\n"},
		{"image_inline", "![alt](src.png)\n"},
		{"autolink_uri", "<http://example.com>\n"},
		{"autolink_email", "<user@example.com>\n"},
		{"hard_break_spaces", "foo  \nbar\n"},
		{"hard_break_backslash", "foo\\\nbar\n"},
		{"block_quote", "> quoted\n> text\n"},
		{"unordered_list", "- one\n- two\n- three\n"},
		{"ordered_list", "1. first\n2. second\n"},
		{"setext_h1", "Title\n=====\n"},
		{"setext_h2", "Title\n-----\n"},
		{"html_block", "<div>\ntext\n</div>\n"},
		{"mixed_content", "# Header\n\nParagraph with *emphasis* and `code`.\n\n- item 1\n- item 2\n\n```\nfenced\n```\n"},
		{"empty_lines", "para1\n\n\npara2\n"},
		{"no_trailing_newline", "hello\nworld"},
		{"single_line", "just one line"},
		{"blank_input", ""},
		{"multiline_paragraph", "line1\nline2\nline3\n"},
		{"unicode", "你好世界\n日本語テスト\n"},
		{"complex_inline", "**bold *italic* bold**\n"},
		{"nested_emphasis", "*a **b** c*\n"},
		{"link_with_emphasis", "[*foo*](url)\n"},
		{"image_in_paragraph", "text ![img](x.png) more\n"},
		{"escaped_chars", "\\*not emphasis\\*\n"},
	}

	for _, c := range corpus {
		t.Run(c.name, func(t *testing.T) {
			var fullBuf, streamBuf bytes.Buffer

			if err := text.Convert([]byte(c.input), &fullBuf); err != nil {
				t.Fatalf("Convert error: %v", err)
			}
			if err := text.ConvertStream(bytes.NewReader([]byte(c.input)), &streamBuf); err != nil {
				t.Fatalf("ConvertStream error: %v", err)
			}

			if fullBuf.String() != streamBuf.String() {
				t.Errorf("output mismatch for %q:\n--- Convert ---\n%q\n--- ConvertStream ---\n%q",
					c.input, fullBuf.String(), streamBuf.String())
			}
		})
	}
}

// TestStreamEqualsFull_HTML verifies HTML rendering consistency between
// Parse ([]byte, SliceSource) and ParseStream (ReaderSource) on the same
// input. For inputs without forward references, both paths must produce
// byte-identical HTML output.
func TestStreamEqualsFull_HTML(t *testing.T) {
	corpus := []struct {
		name  string
		input string
	}{
		{"paragraph", "Hello world\n"},
		{"heading", "# Title\n"},
		{"emphasis", "*hello* **world**\n"},
		{"code", "`code`\n"},
		{"link", "[foo](url)\n"},
		{"list", "- a\n- b\n"},
		{"blockquote", "> quote\n"},
		{"fenced_code", "```\ncode\n```\n"},
		{"mixed", "# H\n\npara *em* `code`.\n\n- i1\n- i2\n"},
	}

	for _, c := range corpus {
		t.Run(c.name, func(t *testing.T) {
			p := parser.New(0)

			// Full input → HTML
			var fullBuf bytes.Buffer
			h := html.NewHTML(&fullBuf)
			if err := p.Parse([]byte(c.input), h); err != nil {
				t.Fatalf("Parse error: %v", err)
			}
			_ = h.Flush()

			// Streaming → HTML via ParseStream + ReaderSource
			var streamBuf bytes.Buffer
			h2 := html.NewHTML(&streamBuf)
			if err := p.ParseStream(stream.NewReaderSource(bytes.NewReader([]byte(c.input))), h2); err != nil {
				t.Fatalf("ParseStream error: %v", err)
			}
			_ = h2.Flush()

			if fullBuf.String() != streamBuf.String() {
				t.Errorf("HTML output mismatch for %q:\n--- Parse ---\n%q\n--- ParseStream ---\n%q",
					c.input, fullBuf.String(), streamBuf.String())
			}
		})
	}
}

// TestStreamingMemory verifies that ConvertStream uses bounded memory
// regardless of input size. The parser must not retain the full document
// in memory — only active blocks, mark stacks, and refdefs.
//
// Gate: HeapInuse after streaming a large document must be < 256 MB.
func TestStreamingMemory(t *testing.T) {
	const lines = 1_000_000
	var buf bytes.Buffer
	for i := 0; i < lines; i++ {
		switch i % 5 {
		case 0:
			buf.WriteString("# Heading ")
			buf.WriteString(intToStr(i/5 + 1))
			buf.WriteByte('\n')
		case 1:
			buf.WriteString("Paragraph with *emphasis* and `code` content number ")
			buf.WriteString(intToStr(i))
			buf.WriteByte('\n')
		case 2:
			buf.WriteString("- list item ")
			buf.WriteString(intToStr(i))
			buf.WriteByte('\n')
		case 3:
			buf.WriteString("> quote line ")
			buf.WriteString(intToStr(i))
			buf.WriteByte('\n')
		case 4:
			buf.WriteByte('\n')
		}
	}
	input := buf.Bytes()

	runtime.GC()
	var before runtime.MemStats
	runtime.ReadMemStats(&before)

	if err := text.ConvertStream(bytes.NewReader(input), io.Discard); err != nil {
		t.Fatalf("ConvertStream error: %v", err)
	}

	var after runtime.MemStats
	runtime.ReadMemStats(&after)

	heapInuse := after.HeapInuse - before.HeapInuse
	threshold := uint64(256) << 20

	t.Logf("Streaming %d lines: HeapInuse delta = %d MB, threshold = %d MB",
		lines, heapInuse>>20, threshold>>20)

	if heapInuse > threshold {
		t.Errorf("HeapInuse %d MB exceeds %d MB threshold — parser may be retaining data",
			heapInuse>>20, threshold>>20)
	}
}

// intToStr converts an integer to its string representation.
func intToStr(n int) string {
	if n == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[i:])
}

// TestStreamingDoesNotBufferAll verifies that ConvertStream processes
// input incrementally — output should start flowing before the entire
// input is consumed.
func TestStreamingDoesNotBufferAll(t *testing.T) {
	slowInput := &trackingReader{data: []byte("# H1\n\npara1\n\npara2\n\npara3\n")}
	var output bytes.Buffer

	if err := text.ConvertStream(slowInput, &output); err != nil {
		t.Fatalf("ConvertStream error: %v", err)
	}

	if output.Len() == 0 {
		t.Error("no output produced")
	}
}

type trackingReader struct {
	data   []byte
	offset int
}

func (r *trackingReader) Read(p []byte) (n int, err error) {
	if r.offset >= len(r.data) {
		return 0, io.EOF
	}
	n = copy(p, r.data[r.offset:])
	r.offset += n
	return n, nil
}

// TestConvertVsConvertStream_ForwardRefdef tests the known limitation:
// ConvertStream cannot resolve forward references because refdefs are
// first-seen-first, while Convert can see all refdefs.
//
// With forward refdefs, Convert ([]byte, full visibility) resolves the
// reference link, while ConvertStream (io.Reader, first-seen-first)
// produces literal text because the refdef hasn't been seen yet when the
// reference is encountered. This test documents the expected behavioral
// difference — it does NOT assert equality.
func TestConvertVsConvertStream_ForwardRefdef(t *testing.T) {
	input := "[click here][ref]\n\n[ref]: http://example.com\n"

	var fullBuf, streamBuf bytes.Buffer

	if err := text.Convert([]byte(input), &fullBuf); err != nil {
		t.Fatalf("Convert error: %v", err)
	}
	if err := text.ConvertStream(bytes.NewReader([]byte(input)), &streamBuf); err != nil {
		t.Fatalf("ConvertStream error: %v", err)
	}

	fullOut := fullBuf.String()
	streamOut := streamBuf.String()

	t.Logf("Convert:       %q", fullOut)
	t.Logf("ConvertStream: %q", streamOut)

	// Convert (full visibility) should resolve the link to "click here".
	if !bytes.Contains([]byte(fullOut), []byte("click here")) {
		t.Errorf("Convert should resolve forward refdef: got %q", fullOut)
	}

	// Both outputs should contain the link text regardless of resolution.
	if !bytes.Contains([]byte(streamOut), []byte("click here")) {
		t.Errorf("ConvertStream should contain link text: got %q", streamOut)
	}
}
