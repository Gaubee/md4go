//go:build js && wasm

// Package wasm exposes md4go's Markdown APIs to the browser via WebAssembly.
//
// Build:
//
//	GOOS=js GOARCH=wasm go build -o md4go.wasm ./wasm
//
// Global JS functions:
//
//	// One-shot conversion
//	md4goParseToHTML(md, flags?)                           → string
//	md4goParseToText(md, flags?)                           → string
//	md4goParseToHTMLWithOptions(md, options?)               → string
//	md4goParseWithRenderer(md, flags?, callbacks?)          → null | {error}
//
//	// Parser reuse (CREATE/DISPOSE lifecycle)
//	md4goCreateParser(flags?)                              → ParserObj
//	md4goCreateStreamParser(flags?)                        → StreamObj
package main

import (
	"bytes"
	"syscall/js"

	"github.com/userpro/md4go/ast"
	"github.com/userpro/md4go/html"
	"github.com/userpro/md4go/parser"
	"github.com/userpro/md4go/renderer"
	"github.com/userpro/md4go/text"
)

func main() {
	js.Global().Set("md4goParseToHTML", js.FuncOf(parseToHTML))
	js.Global().Set("md4goParseToText", js.FuncOf(parseToText))
	js.Global().Set("md4goParseToHTMLWithOptions", js.FuncOf(parseToHTMLWithOptions))
	js.Global().Set("md4goParseWithRenderer", js.FuncOf(parseWithRenderer))
	js.Global().Set("md4goCreateParser", js.FuncOf(createParser))
	js.Global().Set("md4goCreateStreamParser", js.FuncOf(createStreamParser))
	<-make(chan struct{})
}

// ─── One-shot APIs ───────────────────────────────────────────────────

func parseToHTML(this js.Value, args []js.Value) any {
	md := getMarkdown(args)
	flags := getFlags(args)
	return convertToHTML(md, flags, html.FlagXHTML)
}

func parseToText(this js.Value, args []js.Value) any {
	md := getMarkdown(args)
	flags := getFlags(args)
	return convertToText(md, flags)
}

func parseToHTMLWithOptions(this js.Value, args []js.Value) any {
	md := getMarkdown(args)
	flags := parser.DialectGitHub
	rendererFlags := html.FlagXHTML
	if len(args) >= 2 && args[1].Type() == js.TypeObject {
		opts := args[1]
		if f := opts.Get("flags"); f.Type() == js.TypeNumber {
			flags = parser.Flags(f.Int())
		}
		if rf := opts.Get("rendererFlags"); rf.Type() == js.TypeNumber {
			rendererFlags = html.RenderFlags(rf.Int())
		}
	}
	return convertToHTML(md, flags, rendererFlags)
}

func parseWithRenderer(this js.Value, args []js.Value) any {
	md, flags, cbs := extractRendererArgs(args)
	r := newJSRenderer(cbs)
	p := parser.New(flags)
	if err := p.Parse([]byte(md), r); err != nil {
		return map[string]any{"error": err.Error()}
	}
	return nil
}

// ─── Parser reuse ────────────────────────────────────────────────────

// createParser returns a reusable parser object.
//
//	> const p = md4goCreateParser(1)
//	> p.parseToHTML("# A")   // "<h1>A</h1>"
//	> p.parseToHTML("## B")  // "<h2>B</h2>"
//	> p.parseToText("# C")   // "C"
//	> p.parseWithRenderer("D", {enterSpan(t,d){…}})
//	> p.dispose()
func createParser(this js.Value, args []js.Value) any {
	flags := parser.DialectGitHub
	if len(args) >= 1 && args[0].Type() == js.TypeNumber {
		flags = parser.Flags(args[0].Int())
	}
	p := parser.New(flags)

	obj := js.ValueOf(map[string]any{})
	obj.Set("parseToHTML", js.FuncOf(func(this js.Value, args []js.Value) any {
		md := getMarkdown(args)
		rf := html.FlagXHTML
		if len(args) >= 2 && args[1].Type() == js.TypeNumber {
			rf = html.RenderFlags(args[1].Int())
		}
		var buf bytes.Buffer
		h := html.NewWithFlags(&buf, rf)
		if err := p.Parse([]byte(md), h); err != nil {
			return ""
		}
		_ = h.Flush()
		return buf.String()
	}))
	obj.Set("parseToText", js.FuncOf(func(this js.Value, args []js.Value) any {
		md := getMarkdown(args)
		var buf bytes.Buffer
		t := text.NewPlainText(&buf)
		if err := p.Parse([]byte(md), t); err != nil {
			return ""
		}
		_ = t.Flush()
		return buf.String()
	}))
	obj.Set("parseWithRenderer", js.FuncOf(func(this js.Value, args []js.Value) any {
		md, _, cbs := extractRendererArgs(args)
		r := newJSRenderer(cbs)
		if err := p.Parse([]byte(md), r); err != nil {
			return map[string]any{"error": err.Error()}
		}
		return nil
	}))
	obj.Set("dispose", js.FuncOf(func(this js.Value, args []js.Value) any {
		// Release parser reference so GC can collect it.
		// js.FuncOf closures capture p; dropping the obj
		// reference allows both Go and JS GC to proceed.
		return nil
	}))
	return obj
}

// ─── Stream accumulator ──────────────────────────────────────────────

// createStreamParser returns a chunk-accumulating stream object for
// large documents. Call write() repeatedly, then finishHTML() or
// finishText() to parse the complete document.
//
// Streaming in WASM is single-threaded; chunks are accumulated and
// parsed as a whole on finish(). The benefit is the feeding pattern
// (e.g. from fetch / FileReader streams) and parser reuse.
func createStreamParser(this js.Value, args []js.Value) any {
	flags := parser.DialectGitHub
	if len(args) >= 1 && args[0].Type() == js.TypeNumber {
		flags = parser.Flags(args[0].Int())
	}

	buf := &bytes.Buffer{}
	p := parser.New(flags)

	obj := js.ValueOf(map[string]any{})
	obj.Set("write", js.FuncOf(func(this js.Value, args []js.Value) any {
		if len(args) > 0 {
			buf.WriteString(args[0].String())
		}
		return nil
	}))
	obj.Set("finishHTML", js.FuncOf(func(this js.Value, args []js.Value) any {
		rf := html.FlagXHTML
		if len(args) >= 1 && args[0].Type() == js.TypeNumber {
			rf = html.RenderFlags(args[0].Int())
		}
		var out bytes.Buffer
		h := html.NewWithFlags(&out, rf)
		if err := p.Parse(buf.Bytes(), h); err != nil {
			return ""
		}
		_ = h.Flush()
		buf.Reset()
		return out.String()
	}))
	obj.Set("finishText", js.FuncOf(func(this js.Value, args []js.Value) any {
		var out bytes.Buffer
		t := text.NewPlainText(&out)
		if err := p.Parse(buf.Bytes(), t); err != nil {
			return ""
		}
		_ = t.Flush()
		buf.Reset()
		return out.String()
	}))
	obj.Set("dispose", js.FuncOf(func(this js.Value, args []js.Value) any {
		return nil
	}))
	return obj
}

// ─── Helpers ─────────────────────────────────────────────────────────

func getMarkdown(args []js.Value) string {
	if len(args) == 0 {
		return ""
	}
	return args[0].String()
}

func getFlags(args []js.Value) parser.Flags {
	if len(args) < 2 {
		return parser.DialectGitHub
	}
	return parser.Flags(args[1].Int())
}

// extractRendererArgs pulls (md, flags, callbacksObject) from args.
// Callback detection: positional (md, flags, callbacksObj) or (md, callbacksObj).
func extractRendererArgs(args []js.Value) (md string, flags parser.Flags, cbs js.Value) {
	md = getMarkdown(args)
	flags = parser.DialectGitHub
	cbIdx := 1
	if len(args) >= 2 && args[1].Type() == js.TypeNumber {
		flags = parser.Flags(args[1].Int())
		cbIdx = 2
	}
	if cbIdx < len(args) && args[cbIdx].Type() == js.TypeObject {
		cbs = args[cbIdx]
	}
	return
}

func getCallback(obj js.Value, name string) js.Value {
	if obj.IsNull() || obj.IsUndefined() {
		return js.Null()
	}
	fn := obj.Get(name)
	if fn.Type() != js.TypeFunction {
		return js.Null()
	}
	return fn
}

func convertToHTML(md string, flags parser.Flags, rendererFlags html.RenderFlags) string {
	var buf bytes.Buffer
	if err := html.Convert([]byte(md), &buf,
		html.WithFlags(flags),
		html.WithRendererFlags(rendererFlags),
	); err != nil {
		return ""
	}
	return buf.String()
}

func convertToText(md string, flags parser.Flags) string {
	var buf bytes.Buffer
	if err := text.Convert([]byte(md), &buf,
		text.WithFlags(flags),
	); err != nil {
		return ""
	}
	return buf.String()
}

// ─── JS Renderer Bridge ──────────────────────────────────────────────

type jsRenderer struct {
	enterBlockFn js.Value
	leaveBlockFn js.Value
	enterSpanFn  js.Value
	leaveSpanFn  js.Value
	textFn       js.Value
}

var _ renderer.Renderer = (*jsRenderer)(nil)

func newJSRenderer(cbs js.Value) *jsRenderer {
	return &jsRenderer{
		enterBlockFn: getCallback(cbs, "enterBlock"),
		leaveBlockFn: getCallback(cbs, "leaveBlock"),
		enterSpanFn:  getCallback(cbs, "enterSpan"),
		leaveSpanFn:  getCallback(cbs, "leaveSpan"),
		textFn:       getCallback(cbs, "text"),
	}
}

func (r *jsRenderer) EnterBlock(t ast.BlockType, detail any) error {
	if r.enterBlockFn.IsNull() {
		return nil
	}
	r.enterBlockFn.Invoke(js.ValueOf(int(t)), blockDetailToJS(detail))
	return nil
}

func (r *jsRenderer) LeaveBlock(t ast.BlockType, detail any) error {
	if r.leaveBlockFn.IsNull() {
		return nil
	}
	r.leaveBlockFn.Invoke(js.ValueOf(int(t)), blockDetailToJS(detail))
	return nil
}

func (r *jsRenderer) EnterSpan(t ast.SpanType, detail any) error {
	if r.enterSpanFn.IsNull() {
		return nil
	}
	r.enterSpanFn.Invoke(js.ValueOf(int(t)), spanDetailToJS(detail))
	return nil
}

func (r *jsRenderer) LeaveSpan(t ast.SpanType, detail any) error {
	if r.leaveSpanFn.IsNull() {
		return nil
	}
	r.leaveSpanFn.Invoke(js.ValueOf(int(t)), spanDetailToJS(detail))
	return nil
}

func (r *jsRenderer) Text(t ast.TextType, text []byte) error {
	if r.textFn.IsNull() {
		return nil
	}
	r.textFn.Invoke(js.ValueOf(int(t)), string(text))
	return nil
}

// ─── Detail serialization ────────────────────────────────────────────

func blockDetailToJS(d any) js.Value {
	if d == nil {
		return js.Null()
	}
	switch v := d.(type) {
	case *ast.HeadingDetail:
		return js.ValueOf(map[string]any{"level": v.Level})
	case *ast.CodeDetail:
		return js.ValueOf(map[string]any{
			"info":      string(v.Info.Text),
			"lang":      string(v.Lang.Text),
			"fenceChar": int(v.FenceChar),
		})
	case *ast.ULDetail:
		return js.ValueOf(map[string]any{
			"isTight": v.IsTight,
			"mark":    int(v.Mark),
		})
	case *ast.OLDetail:
		return js.ValueOf(map[string]any{
			"start":   v.Start,
			"isTight": v.IsTight,
			"mark":    int(v.Mark),
		})
	case *ast.LIDetail:
		return js.ValueOf(map[string]any{
			"isTask":      v.IsTask,
			"taskMark":    int(v.TaskMark),
			"taskMarkOff": v.TaskMarkOff,
		})
	case *ast.TableDetail:
		return js.ValueOf(map[string]any{
			"colCount":     v.ColCount,
			"headRowCount": v.HeadRowCount,
			"bodyRowCount": v.BodyRowCount,
		})
	case *ast.TDDetail:
		return js.ValueOf(map[string]any{"align": int(v.Align)})
	case *ast.AdmonitionDetail:
		return js.ValueOf(map[string]any{"type": string(v.Type.Text)})
	case *ast.FootnoteDefDetail:
		return js.ValueOf(map[string]any{
			"id":       int(v.ID),
			"refCount": int(v.RefCount),
			"label":    string(v.Label.Text),
		})
	default:
		return js.Null()
	}
}

func spanDetailToJS(d any) js.Value {
	if d == nil {
		return js.Null()
	}
	switch v := d.(type) {
	case *ast.LinkDetail:
		return js.ValueOf(map[string]any{
			"href":       string(v.Href.Text),
			"title":      string(v.Title.Text),
			"isAutolink": v.IsAutolink,
		})
	case *ast.ImgDetail:
		return js.ValueOf(map[string]any{
			"src":   string(v.Src.Text),
			"title": string(v.Title.Text),
		})
	case *ast.FootnoteRefDetail:
		return js.ValueOf(map[string]any{
			"id":    int(v.ID),
			"refId": int(v.RefID),
			"label": string(v.Label.Text),
		})
	case *ast.WikilinkDetail:
		return js.ValueOf(map[string]any{"target": string(v.Target.Text)})
	default:
		return js.Null()
	}
}
