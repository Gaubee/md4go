# md4go — A Markdown Parser for Go

[中文](README.zh.md) | English

md4go is a Markdown parser for Go that uses a push-based, event-driven model and does not build an AST. It is **CommonMark 0.31 compliant (652/652)** with full support for GFM extensions (tables / strikethrough / task lists / autolinks).

## Quick Start

### Installation

```bash
go get github.com/userpro/md4go
```

### Minimal Example: Markdown → Plain Text

```go
package main

import (
    "os"
    "md4go/text"
    "md4go/parser"
)

func main() {
    src := []byte("# Hello\n\n- item1\n- item2\n")
    text.Convert(src, os.Stdout, text.WithFlags(parser.DialectGitHub))
}
// Output:
// Hello
//
// item1
// item2
```

### Minimal Example: Markdown → HTML

```go
package main

import (
    "os"
    "md4go/html"
    "md4go/parser"
)

func main() {
    src := []byte("# Hello\n\n- item1\n- item2\n")
    html.Convert(src, os.Stdout, html.WithFlags(parser.DialectGitHub))
}
// Output:
// <h1>Hello</h1>
// <ul>
// <li>item1</li>
// <li>item2</li>
// </ul>
```

### Command Line

```bash
# Build
go build -o md4go ./cmd/md4go

# Markdown → plain text (GFM mode by default)
echo "# Hello" | ./md4go

# Markdown → HTML
echo "# Hello" | ./md4go -html

# Streaming input (low memory, plain text mode only)
cat large.md | ./md4go -stream

# goldmark compatibility mode
echo "| a | b |" | ./md4go -compat goldmark
```

## Three-Layer Architecture

```
┌─────────────────────────────────────────────┐
│  Convenience Layer (one-line wrappers)       │
│  text.Convert()    html.Convert()            │
├─────────────────────────────────────────────┤
│  Core Layer (parsing API)                    │
│  md4go.Parser.Parse(src, renderer)           │
│  renderer.Renderer interface                 │
├─────────────────────────────────────────────┤
│  Custom Layer (user-defined renderers)       │
│  Implement the 5 methods of Renderer         │
└─────────────────────────────────────────────┘
```

- **Convenience Layer** (`text` / `html` packages): convert in a single call
- **Core Layer** (`md4go` root package): full parsing API, pushes events to any Renderer
- **Custom Layer**: implement the `renderer.Renderer` interface for custom output formats

## Use Cases

### Scenario 1: Markdown Text Extraction (RAG / Search Indexing / Content Cleaning)

```go
// Extract plain text from Markdown, stripping all formatting markers
var buf bytes.Buffer
text.Convert(markdownBytes, &buf, text.WithFlags(parser.DialectGitHub))
plainText := buf.String()
```

Typical uses:
- Document preprocessing for RAG systems
- Content indexing for full-text search engines
- Plain-text versions of Markdown emails / notifications
- Text summaries of chat messages

### Scenario 2: Markdown → HTML Rendering

```go
// Generate HTML in XHTML mode
var buf bytes.Buffer
html.Convert(markdownBytes, &buf,
    html.WithFlags(parser.DialectGitHub),
    html.WithRendererFlags(html.FlagXHTML),
)
```

### Scenario 3: Streaming Large Files

```go
// Read line by line with constant memory usage
file, _ := os.Open("large.md")
defer file.Close()
text.ConvertStream(file, os.Stdout, text.WithFlags(parser.DialectGitHub))
```

> **Note**: In streaming mode, reference link definitions (refdefs) follow a "first-seen-first-served" rule — forward references degrade to literal text. One-shot parsing (`Convert`) has no such limitation.

### Scenario 3b: Streaming with State Preservation (Continuation)

`ConvertStream`/`ParseStream` reset the parser context on each call — they treat
every batch as an independent document. That is fine for a one-pass file
conversion, but it breaks when a structure spans multiple calls (a code block
opened in one chunk and closed in another, or a paragraph fed line-by-line).

`ParseStreamContinue` / `ParseStreamEnd` expose md4c's native continuation
semantics: a single parser context is fed lines incrementally, so block and
container state survives chunk boundaries. This is what a front-of-pipe guard
(e.g. a custom-syntax detector) uses to query `InProtectedBlock()` mid-stream and
decide whether a marker at the current cursor sits inside a verbatim block.

```go
p := md4go.New(md4go.WithFlags(parser.DialectCommonMark))
r := myRenderer{}

// Feed lines as they arrive (across network chunks, LLM tokens, etc.).
// Block/container state is preserved across Continue calls.
for _, line := range chunkedLines {
    if p.InProtectedBlock() {
        // cursor currently inside a code/html block — don't intercept markers here
    }
    p.ParseStreamContinue(lineSourceFor(line), r)
}
// Finalize once at end-of-stream: closes trailing blocks, emits footnotes,
// leaves the Doc block, and clears continuation state.
p.ParseStreamEnd(r)
```

`InProtectedBlock()` reports whether the cursor is currently inside a fenced or
indented code block or an HTML block, read live from the parser context. It is
accurate even though md4c's block events are deferred by a one-line lookahead —
the internal context state is current after each fed line, so the guard sees the
true state before emitting events for it.

### Scenario 4: WebAssembly (Browser)

md4go compiles to WebAssembly for browser-side Markdown parsing. See [`wasm/README.md`](wasm/README.md) for details.

```html
<script type="module">
  import { initMd4go } from './wasm/md4go.js';
  const { parseToHTML, parseToText } = await initMd4go();
  console.log(parseToHTML("# Hello **world**"));
</script>
```

```bash
# Build
GOOS=js GOARCH=wasm go build -o md4go.wasm ./wasm
cp "$(go env GOROOT)/lib/wasm/wasm_exec.js" .
```

### Scenario 5: Custom Renderer (Structured Data Extraction)

```go
// Extract all links
type LinkExtractor struct {
    links []string
    inLink bool
}

func (e *LinkExtractor) EnterBlock(ast.BlockType, any) error { return nil }
func (e *LinkExtractor) LeaveBlock(ast.BlockType, any) error { return nil }
func (e *LinkExtractor) EnterSpan(s ast.SpanType, d any) error {
    if s == ast.SpanLink {
        if detail, ok := d.(*ast.LinkDetail); ok {
            e.links = append(e.links, string(detail.Href.Text))
        }
        e.inLink = true
    }
    return nil
}
func (e *LinkExtractor) LeaveSpan(ast.SpanType, any) error { return nil }
func (e *LinkExtractor) Text(ast.TextType, []byte) error    { return nil }

// Usage
p := md4go.New(md4go.WithFlags(parser.DialectGitHub))
ext := &LinkExtractor{}
p.Parse(src, ext)
fmt.Println(ext.links) // ["https://example.com", ...]
```

## API Reference

### Root Package `md4go` — Parsing API

```go
// Create a parser
p := md4go.New(
    md4go.WithFlags(parser.DialectGitHub),       // set parse flags
    md4go.WithExtensions(&extension.Table{}),     // register extensions
)

// Parse []byte → push events to renderer
p.Parse(src, myRenderer)

// Stream-parse io.Reader → push events to renderer
p.ParseStream(lineSource, myRenderer)

// Stream-parse with state preserved across calls (continuation mode).
// First Continue opens the document; subsequent calls reuse the parser context
// so block/container state survives chunk boundaries. ParseStreamEnd finalizes.
p.ParseStreamContinue(lineSource, myRenderer)
p.ParseStreamEnd(myRenderer)   // call once at end-of-stream

// Live query: is the cursor currently inside a fenced/indented code block or
// HTML block? Only meaningful between ParseStreamContinue calls.
if p.InProtectedBlock() { /* cursor is in verbatim content */ }
```

### `text` Package — Plain Text

```go
// One-shot conversion
text.Convert(src, writer, text.WithFlags(...), text.WithExtensions(...))

// Streaming conversion
text.ConvertStream(reader, writer, text.WithFlags(...))

// Get a renderer instance (advanced)
pt := text.NewPlainText(writer)
p.Parse(src, pt)
pt.Flush()
```

### `html` Package — HTML

```go
// One-shot conversion
html.Convert(src, writer, html.WithFlags(...), html.WithExtensions(...), html.WithRendererFlags(...))

// XHTML mode (default)
h := html.NewHTML(writer)

// Specify renderer flags
h := html.NewWithFlags(writer, html.FlagXHTML|html.FlagVerbatimEntities)

// Advanced usage
h := html.NewHTMLWithWriter(renderer.NewBufWriter(writer))
```

**HTML renderer flags**:

| Flag | Value | Description |
|---|---|---|
| `FlagDebug` | 0x0001 | Debug output |
| `FlagVerbatimEntities` | 0x0002 | Output entities verbatim (not translated to UTF-8) |
| `FlagSkipUTF8BOM` | 0x0004 | Skip a leading UTF-8 BOM in the input |
| `FlagXHTML` | 0x0008 | XHTML self-closing tags (`<br />`) |
| `FlagNoXHTMLEscaping` | 0x0010 | Escape only `& < >` (goldmark-compatible, no `'` `"`) |

### `renderer` Package — Interface Definition

```go
type Renderer interface {
    EnterBlock(t ast.BlockType, detail any) error
    LeaveBlock(t ast.BlockType, detail any) error
    EnterSpan(t ast.SpanType, detail any) error
    LeaveSpan(t ast.SpanType, detail any) error
    Text(t ast.TextType, text []byte) error
}
```

## Tuning Guide

### Choosing a Parse Mode

| Mode | Constant | Use Case |
|---|---|---|
| CommonMark | `parser.DialectCommonMark` | Standard Markdown, strict compliance |
| GitHub Flavored | `parser.DialectGitHub` | GFM extensions (tables / strikethrough / task lists / autolinks) |

`DialectGitHub` = `PermissiveAutolinks | FlagTables | FlagStrikethrough | FlagTasklists | FlagAdmonitions | FlagFootnotes`

### Choosing an Input Mode

| Mode | API | Memory | Forward References | Cross-chunk state |
|---|---|---|---|---|
| One-shot `[]byte` | `Parse` / `Convert` | O(n) | ✅ Fully supported | N/A |
| Streaming `io.Reader` | `ParseStream` / `ConvertStream` | O(line) | ❌ First-seen-first-served | ❌ Reset per call |
| Streaming continuation (incremental) | `ParseStreamContinue` / `End` | O(line) | ❌ First-seen-first-served | ✅ Preserved across calls |

**Recommendation**: use `Convert` for documents < 10 MB; use `ConvertStream` for very large documents; use `ParseStreamContinue`/`End` when block/container state must survive chunk boundaries (e.g. a front-of-pipe custom-syntax guard, or token-by-token LLM streaming).

### Choosing a Render Target

| Target | Package | Characteristics |
|---|---|---|
| Plain text | `text` | Strips all formatting, preserves text content and semantic boundaries |
| HTML | `html` | Full HTML output, XHTML / HTML5 selectable |
| Custom | `renderer` | Implement the Renderer interface |

### Performance Tips

1. **Reuse the Parser**: a `Parser` created by `md4go.New()` can be used for multiple `Parse()` calls
2. **Save memory with streaming**: `ConvertStream` reads line by line, with memory usage independent of document size
3. **Automatic BufWriter buffering**: `text.NewPlainText(w)` and `html.NewHTML(w)` use a 4 KB internal buffer
4. **Enable extensions on demand**: register only the extensions you need to reduce parsing overhead

### Extension Injection

```go
// Enable only tables and strikethrough
p := md4go.New(md4go.WithExtensions(
    &extension.Table{},
    &extension.Strikethrough{},
))

// All GFM extensions (shortcut)
p := md4go.New(md4go.WithFlags(parser.DialectGitHub))
// Equivalent to:
p := md4go.New(md4go.WithExtensions(extension.GFM...))
```

**Available extensions**:

| Extension | Syntax |
|---|---|
| `extension.Strikethrough` | `~~strikethrough~~` |
| `extension.Table` | GFM tables |
| `extension.Tasklist` | `- [x] task` |
| `extension.PermissiveAutolinks` | URL / email / WWW autolinks |
| `extension.Footnote` | `[^1]` footnotes |
| `extension.LatexMath` | `$inline$` / `$$block$$` |
| `extension.Wikilink` | `[[link]]` |
| `extension.Superscript` | `^superscript^` |
| `extension.Subscript` | `~subscript~` |
| `extension.Spoiler` | `||spoiler||` |
| `extension.Highlight` | `==highlight==` |
| `extension.Admonition` | `> [!NOTE]` admonition blocks |

## Compliance

| Standard | Result |
|---|---|
| CommonMark 0.31 | 652/652 ✅ |
| GFM tables / strikethrough / task lists / autolinks | All passing ✅ |

## Known Differences

md4go follows the GFM / CommonMark standards by default. Differences from other implementations fall into two categories: **intentional improvements** (active by default, no flag needed) and **differences alignable via compatibility flags**.

### Intentional Improvements (Default Behavior)

| ID | Scenario | Default Behavior | Notes |
|---|---|---|---|
| S-01 | `||` in table cells | Split as a cell boundary | `FlagProtectDoublePipe` can opt in to keep `||` unsplit |
| S-02/04 | Tight list paragraph separation | Preserves `\n` word boundaries, emits P events | Better for text extraction |
| S-03 | `[[target\|label]]` wikilink | Recognized as a wikilink | Supports wikilinks with labels |
| S-05 | Footnote references | Outputs `[N]` | Preserves the reference number |
| S-06 | Code spans containing NULL | Recognized and replaced with U+FFFD | Follows CommonMark |

### Differences from goldmark

Major differences can be aligned via the `GoldmarkCompat` preset:

| Scenario | Alignment Flag | Example |
|---|---|---|
| Tables cannot interrupt a paragraph (GFM standard) | `FlagTableInterruptParagraph` | Paragraph followed by a table: not recognized as a table by default; with the flag, the last line of the paragraph is promoted to the table header |
| HTML entity decoding | `FlagDecodeEntities` | `&amp; &copy;`: entities kept as text by default; decoded to `& ©` with the flag |
| Leading UTF-8 BOM stripping | `FlagStripBOM` | `\ufeffHello`: BOM preserved by default; stripped with the flag (goldmark behavior) |
| Strikethrough `~~` intraword (md4c stricter than cmark-gfm) | `FlagStrikethroughPermissive` | `foo~~bar~~baz`: `~~` not recognized intraword by default (md4c behavior); recognized with the flag (cmark-gfm/goldmark behavior) |
| Inline HTML tag stripping (text renderer) | `FlagStripHTMLTags` | `<span>html</span>`: raw HTML preserved by default; tags stripped to `html` with the flag. Non-visible elements (`<script>`, `<style>`, etc.) have their entire content removed, matching goquery DOM text extraction |
| Strict table column count validation | `FlagStrictTableColumns` | Header 3 cols, delimiter 2 cols: loosely recognized by default; not recognized as a table with the flag |
| Table interrupted by adjacent header row | `FlagTableInterruptByHeaders` | Header row adjacent to another heading: recognized as heading by default; recognized as table header with the flag |
| XHTML entity encoding in HTML renderer | `FlagNoXHTMLEntityEncoding` | `"` and `'` encoded as `&quot;` `&#39;` by default; left as plain characters with the flag (goldmark behavior) |
| Inline span / bracket extra spaces | — | Side effect of goldmark's DOM traversal, should not be replicated |

> See `DIFFCHECK_REPORT.md` for the full comparison report.

## Project Documentation

| Document | Location | Description |
|---|---|---|
| README.md | root | Quick start + API reference (English) |
| README.zh.md | root | 快速上手 + API 参考（中文） |
| ARCHITECTURE.md | root | Architecture design |
| DESIGN.md | root | Algorithm design details |
| TESTING.md | root | Testing system overview |
| wasm/README.md | wasm/ | WebAssembly browser usage guide |
| diffcheck/README.md | diffcheck/ | Engine cross-comparison tool docs |
| DIFFCHECK_REPORT.md | root | Engine comparison report (md4go / md4c / goldmark) |
| benchmark/README.md | benchmark/ | Benchmark suite guide (md4go / md4c / goldmark) |
| benchmark/BENCHMARK_REPORT.md | benchmark/ | Latest benchmark results |

## Acknowledgments

This project was originally ported to Go based on the algorithm design of [md4c](https://github.com/mity/md4c) v0.5.3, with subsequent engineering improvements and standards-compliance enhancements on top.
