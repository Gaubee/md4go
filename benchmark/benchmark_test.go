package benchmark

import (
	"bytes"
	"fmt"
	"io"
	"strings"
	"sync"
	"testing"

	"github.com/userpro/md4go"
	"github.com/userpro/md4go/ast"
	"github.com/userpro/md4go/html"
	"github.com/userpro/md4go/parser"
	"github.com/yuin/goldmark"
	goldmarkExt "github.com/yuin/goldmark/extension"
)

// ─── Pre-built parsers (one-shot creation, reused across iterations) ───
//
// Creating new parsers per iteration is ~500-2000ns overhead for md4go
// and ~5000-15000ns for goldmark (extension registration).  Reusing removes
// this bias, giving a fair comparison of *parse speed* between Go and C.

var (
	md4goCM     *md4go.Parser
	md4goCMOnce sync.Once

	md4goGFM     *md4go.Parser
	md4goGFMOnce sync.Once

	gmCM     goldmark.Markdown
	gmCMOnce sync.Once

	gmGFM     goldmark.Markdown
	gmGFMOnce sync.Once
)

func getMd4goCM() *md4go.Parser {
	md4goCMOnce.Do(func() { md4goCM = md4go.New(md4go.WithFlags(parser.DialectCommonMark)) })
	return md4goCM
}

func getMd4goGFM() *md4go.Parser {
	md4goGFMOnce.Do(func() { md4goGFM = md4go.New(md4go.WithFlags(parser.DialectGitHub)) })
	return md4goGFM
}

func getGoldmarkCM() goldmark.Markdown {
	gmCMOnce.Do(func() { gmCM = goldmark.New() })
	return gmCM
}

func getGoldmarkGFM() goldmark.Markdown {
	gmGFMOnce.Do(func() { gmGFM = goldmark.New(goldmark.WithExtensions(goldmarkExt.GFM)) })
	return gmGFM
}

// ─── On-demand parser cache for arbitrary flag combinations ───

var (
	parserCacheMu sync.Mutex
	parserCache   = map[parser.Flags]*md4go.Parser{}
	gmCacheMu     sync.Mutex
	gmCache       = map[bool]goldmark.Markdown{}
)

func getMd4goParser(flags parser.Flags) *md4go.Parser {
	parserCacheMu.Lock()
	p, ok := parserCache[flags]
	if !ok {
		p = md4go.New(md4go.WithFlags(flags))
		parserCache[flags] = p
	}
	parserCacheMu.Unlock()
	return p
}

func getGoldmarkParser(gfm bool) goldmark.Markdown {
	gmCacheMu.Lock()
	gm, ok := gmCache[gfm]
	if !ok {
		if gfm {
			gm = goldmark.New(goldmark.WithExtensions(goldmarkExt.GFM))
		} else {
			gm = goldmark.New()
		}
		gmCache[gfm] = gm
	}
	gmCacheMu.Unlock()
	return gm
}

// ─── Setup-cost benchmarks (isolated measurement) ───

func BenchmarkSetup_Md4goCM(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = md4go.New(md4go.WithFlags(parser.DialectCommonMark))
	}
}

func BenchmarkSetup_Md4goGFM(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = md4go.New(md4go.WithFlags(parser.DialectGitHub))
	}
}

func BenchmarkSetup_GoldmarkCM(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = goldmark.New()
	}
}

func BenchmarkSetup_GoldmarkGFM(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = goldmark.New(goldmark.WithExtensions(goldmarkExt.GFM))
	}
}

// ─── Helpers (reuse parsers, create fresh renderers) ───

func md4goHTML(p *md4go.Parser, input []byte) string {
	var buf bytes.Buffer
	h := html.NewWithFlags(&buf, 0) // HTML5 mode
	p.Parse(input, h)
	h.Flush()
	return buf.String()
}

func md4goNull(p *md4go.Parser, input []byte) {
	var nr NullRenderer
	p.Parse(input, &nr)
}

func goldmarkHTML(gm goldmark.Markdown, input []byte) string {
	var buf bytes.Buffer
	gm.Convert(input, &buf)
	return buf.String()
}

func goldmarkNull(gm goldmark.Markdown, input []byte) {
	gm.Convert(input, io.Discard)
}

// ─── Throughput (CommonMark 652 examples → HTML5) ───

func BenchmarkThroughput_Md4go(b *testing.B) {
	doc := BuildCommonMarkDoc()
	src := []byte(doc)
	p := getMd4goCM()
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		md4goHTML(p, src)
	}
	b.ReportMetric(float64(len(src))/1e6, "MB-input")
}

func BenchmarkThroughput_Md4cCgo(b *testing.B) {
	doc := BuildCommonMarkDoc()
	src := []byte(doc)
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		Md4cConvertHTML(src, 0, 0)
	}
	b.ReportMetric(float64(len(src))/1e6, "MB-input")
}

func BenchmarkThroughput_Goldmark(b *testing.B) {
	doc := BuildCommonMarkDoc()
	src := []byte(doc)
	gm := getGoldmarkCM()
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		goldmarkHTML(gm, src)
	}
	b.ReportMetric(float64(len(src))/1e6, "MB-input")
}

// ─── Parse-only (null/sink renderer, CommonMark doc) ───

func BenchmarkParseOnly_Md4go(b *testing.B) {
	doc := BuildCommonMarkDoc()
	src := []byte(doc)
	p := getMd4goCM()
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		md4goNull(p, src)
	}
}

func BenchmarkParseOnly_Md4cCgo(b *testing.B) {
	doc := BuildCommonMarkDoc()
	src := []byte(doc)
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		Md4cParseNull(src, 0)
	}
}

func BenchmarkParseOnly_Goldmark(b *testing.B) {
	doc := BuildCommonMarkDoc()
	src := []byte(doc)
	gm := getGoldmarkCM()
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		goldmarkNull(gm, src)
	}
}

// ParseBlocksOnly — block structure only, inline pipeline skipped entirely.
// Uses a renderer that counts blocks but ignores all span/text events.
type blocksOnlyRenderer struct {
	blocks int64
}

func (r *blocksOnlyRenderer) EnterBlock(ast.BlockType, any) error { r.blocks++; return nil }
func (r *blocksOnlyRenderer) LeaveBlock(ast.BlockType, any) error { return nil }
func (r *blocksOnlyRenderer) EnterSpan(ast.SpanType, any) error   { return nil }
func (r *blocksOnlyRenderer) LeaveSpan(ast.SpanType, any) error   { return nil }
func (r *blocksOnlyRenderer) Text(ast.TextType, []byte) error     { return nil }

// sink for dead-code elimination prevention
var benchBlockSink int64

func BenchmarkParseOnly_Md4goBlocksOnly(b *testing.B) {
	doc := BuildCommonMarkDoc()
	src := []byte(doc)
	p := getMd4goCM()
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		var r blocksOnlyRenderer
		_ = p.ParseBlocksOnly(src, &r)
		benchBlockSink = r.blocks
	}
}

// ─── GFM extensions ───

// md4go parser.Flags and md4c MD_FLAG_* share identical bit values by design.
const mdGfmFlags uint = uint(parser.DialectGitHub)

func BenchmarkGFM_Md4go(b *testing.B) {
	src := []byte(BuildGFMDoc())
	p := getMd4goGFM()
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		md4goHTML(p, src)
	}
}

func BenchmarkGFM_Md4cCgo(b *testing.B) {
	src := []byte(BuildGFMDoc())
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		Md4cConvertHTML(src, mdGfmFlags, 0)
	}
}

func BenchmarkGFM_Goldmark(b *testing.B) {
	src := []byte(BuildGFMDoc())
	gm := getGoldmarkGFM()
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		goldmarkHTML(gm, src)
	}
}

// ─── CGo bridge overhead (isolated) ───
//
// md4c HTML path calls goProcessOutput() via CGo for each output chunk.
// Each call does C.GoBytes() → alloc+copy from C heap → Go heap.
// This benchmark measures that bridge cost for a known output size,
// so users can subtract it from md4c HTML numbers to estimate pure-C cost.

func BenchmarkCGoBridgeOverhead(b *testing.B) {
	// Produce a known output size that matches a typical CommonMark doc render.
	// We use Md4cConvertHTML's actual output length as the target.
	doc := BuildCommonMarkDoc()
	src := []byte(doc)
	outLen := len(Md4cConvertHTML(src, 0, 0))
	outStr := strings.Repeat("x", outLen) // ~26 KB
	outBytes := []byte(outStr)

	b.Run("PureGo_WriteString", func(b *testing.B) {
		var buf bytes.Buffer
		b.ResetTimer()
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			buf.Reset()
			buf.Write(outBytes)
		}
	})

	b.Run("CGo_GoBytesCopy", func(b *testing.B) {
		// Simulate what goProcessOutput does per chunk.
		// Real md_html() calls this ~600-700 times for a 24 KB input.
		// We approximate: 700 chunks of ~37 bytes each = ~26 KB.
		const chunks = 700
		chunkSize := outLen / chunks
		var cBuf [64]byte // typical chunk size
		for i := range chunkSize {
			cBuf[i] = 'x'
		}
		var md4cBuf bytes.Buffer
		b.ResetTimer()
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			md4cBuf.Reset()
			for j := 0; j < chunks; j++ {
				// Mimics: md4cBuf.Write(C.GoBytes(unsafe.Pointer(text), C.int(size)))
				md4cBuf.Write(bytes.Clone(cBuf[:chunkSize]))
			}
		}
	})
}

// ─── Real-world (JSONL data files) ───

func BenchmarkRealWorld(b *testing.B) {
	files, err := LoadDataDir("data")
	if err != nil {
		b.Fatalf("load data dir: %v", err)
	}
	if len(files) == 0 {
		b.Skip("no .jsonl files in data/")
	}

	type flatCase struct {
		file string
		idx  int
		raw  []byte
	}
	var allCases []flatCase
	for name, cases := range files {
		for _, tc := range cases {
			allCases = append(allCases, flatCase{name, tc.Index, tc.Input})
		}
	}
	if len(allCases) == 0 {
		b.Skip("no test cases loaded")
	}

	p := getMd4goGFM()
	gm := getGoldmarkGFM()

	b.Run("Md4go", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			for _, c := range allCases {
				md4goHTML(p, c.raw)
			}
		}
		b.ReportMetric(float64(len(allCases)), "cases")
	})

	b.Run("Md4cCgo", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			for _, c := range allCases {
				Md4cConvertHTML(c.raw, mdGfmFlags, 0)
			}
		}
		b.ReportMetric(float64(len(allCases)), "cases")
	})

	b.Run("Goldmark", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			for _, c := range allCases {
				goldmarkHTML(gm, c.raw)
			}
		}
		b.ReportMetric(float64(len(allCases)), "cases")
	})

	// Per-file breakdown
	for name, cases := range files {
		b.Run(fmt.Sprintf("File_%s", strings.TrimSuffix(name, ".jsonl")), func(b *testing.B) {
			b.Run("Md4go", func(b *testing.B) {
				b.ReportAllocs()
				for i := 0; i < b.N; i++ {
					for _, c := range cases {
						md4goHTML(p, c.Input)
					}
				}
			})
			b.Run("Md4cCgo", func(b *testing.B) {
				b.ReportAllocs()
				for i := 0; i < b.N; i++ {
					for _, c := range cases {
						Md4cConvertHTML(c.Input, mdGfmFlags, 0)
					}
				}
			})
			b.Run("Goldmark", func(b *testing.B) {
				b.ReportAllocs()
				for i := 0; i < b.N; i++ {
					for _, c := range cases {
						goldmarkHTML(gm, c.Input)
					}
				}
			})
		})
	}
}
