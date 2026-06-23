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

### Scenario 4: Custom Renderer (Structured Data Extraction)

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

| Mode | API | Memory | Forward References |
|---|---|---|---|
| One-shot `[]byte` | `Parse` / `Convert` | O(n) | ✅ Fully supported |
| Streaming `io.Reader` | `ParseStream` / `ConvertStream` | O(line) | ❌ First-seen-first-served |

**Recommendation**: use `Convert` for documents < 10 MB; use `ConvertStream` for very large documents.

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
| Strict table column count validation | `FlagStrictTableColumns` | Header 3 cols, delimiter 2 cols: loosely recognized by default; not recognized as a table with the flag |
| Inline span / bracket extra spaces | — | Side effect of goldmark's DOM traversal, should not be replicated |

> See `DIFF_REPORT.md` for the full comparison report.

## Project Documentation

| Document | Location | Description |
|---|---|---|
| README.md | root | Quick start + API reference (English) |
| README.zh.md | root | 快速上手 + API 参考（中文） |
| ARCHITECTURE.md | root | Architecture design |
| DESIGN.md | root | Algorithm design details |
| TESTING.md | root | Testing system overview |

## Acknowledgments

This project was originally ported to Go based on the algorithm design of [md4c](https://github.com/mity/md4c) v0.5.3, with subsequent engineering improvements and standards-compliance enhancements on top.
