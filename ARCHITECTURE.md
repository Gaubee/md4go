# md4go 架构文档

> 按图索骥审查代码的参考文档。涵盖包结构、依赖方向、核心接口、数据流和关键实现映射。

## 1. 包结构总览

```
md4go/                          ← Go module root
├── md4go.go                    ← 根包：Parser API（底层解析入口）
├── ast/
│   └── events.go               ← 事件类型定义（BlockType/SpanType/TextType + Detail 结构体）
├── renderer/
│   ├── renderer.go             ← Renderer 接口（5 个回调方法）
│   └── writer.go               ← BufWriter（带缓冲的零拷贝写入器）
├── parser/                     ← 解析器核心（最大包，~28 个源文件）
│   ├── parser.go               ← Parser 结构体 + Parse/ParseStream
│   ├── context.go              ← 解析上下文（mark 栈、line 状态）
│   ├── block.go                ← 块级解析（标题/代码/引用/列表/表格/脚注）
│   ├── container.go            ← 容器块处理（blockquote/list 嵌套）
│   ├── emphasis.go             ← 强调/加粗 mark 匹配
│   ├── codespan.go             ← 行内代码 span
│   ├── autolink.go             ← 自动链接检测
│   ├── html_block.go           ← HTML 块类型判定
│   ├── attribute.go            ← 属性解析（link destination/title）
│   ├── flags.go                ← Flags 位掩码 + 常量
│   ├── extender.go             ← Extender 接口 + Registrar
│   └── ...                     ← 其他内部模块
├── stream/
│   └── line_source.go          ← LineSource 接口 + SliceSource/ReaderSource
├── text/
│   ├── text.go                 ← 业务层封装：Convert/ConvertStream/NewPlainText
│   └── plaintext.go            ← PlainText 渲染器实现
├── html/
│   ├── convert.go              ← 业务层封装：Convert/NewHTML/NewWithFlags/Flags
│   ├── render.go               ← HTML 渲染器实现
│   └── entity.go               ← HTML 实体查找表
├── extension/
│   ├── extender.go             ← 包文档 + 通用说明
│   ├── gfm.go                  ← GFM 扩展集（Strikethrough/Table/Tasklist/Autolink）
│   └── extra.go                ← 额外扩展（Footnote/Spoiler/Math/Wikilink/Sup/Sub/Mark/Admonition）
├── cmd/
│   └── md4go/
│       └── main.go             ← CLI 入口
└── diffcheck/                  ← 对拍测试工具（独立 go.mod）
```

## 2. 依赖方向（无环）

```
                    ast (零依赖，纯类型定义)
                     ↑
        renderer ────┘          stream (仅 io 依赖)
          ↑                        ↑
          │                        │
        parser ─── ast, renderer, stream
          ↑
    ┌─────┼─────┐
    │     │     │
  md4go  html   text     extension
  (根包)  │     │          │
         └─┬───┘          │
           ↑              │
        cmd/md4go ←───────┘
```

**关键约束**：
- `html/` 和 `text/` **不导入**根包 `md4go`（避免循环），直接使用 `parser.New()`
- `parser/` **不导入** `html/` 和 `text/`
- `ast/` 零外部依赖，所有包均可安全导入
- `renderer/` 仅依赖 `ast`，仅含接口定义

## 3. 三层架构

### 3.1 底层：解析 API（`md4go` 根包 + `parser/` + `renderer/` + `ast/` + `stream/`）

这是用户实现自定义渲染器的入口。核心流程：

```
用户代码 → md4go.New(opts...) → *Parser
         → parser.Parse(src, renderer) → 事件流
```

**关键类型**：

| 类型 | 位置 | 职责 |
|---|---|---|
| `md4go.Parser` | `md4go.go` | 用户可见的解析器，持有 `*parser.Parser` |
| `md4go.Option` | `md4go.go` | `func(*config)` — 配置 flags 和 extensions |
| `parser.Parser` | `parser/parser.go` | 内部解析器，持有解析上下文 |
| `parser.Flags` | `parser/flags.go` | 位掩码常量（DialectCommonMark/DialectGitHub 等） |
| `renderer.Renderer` | `renderer/renderer.go` | 5 回调接口 |
| `ast.BlockType` | `ast/events.go` | 块级事件类型枚举 |
| `ast.SpanType` | `ast/events.go` | 行内事件类型枚举 |
| `ast.TextType` | `ast/events.go` | 文本事件类型枚举 |

**事件流顺序**：

```
EnterBlock(Doc)
  EnterBlock(H, {Level:1})     Text(Normal, "Hello")     LeaveBlock(H, {Level:1})
  EnterBlock(P)                Text(Normal, "world")     LeaveBlock(P)
  EnterBlock(UL, {IsTight})
    EnterBlock(LI)             Text(Normal, "item")      LeaveBlock(LI)
  LeaveBlock(UL, {IsTight})
LeaveBlock(Doc)
```

### 3.2 业务层：PlainText（`text/`）和 HTML（`html/`）

对底层的简单封装，各自定义 `Option` 避免循环导入。

**`text` 包**：

| 符号 | 位置 | 用途 |
|---|---|---|
| `text.Convert(src, w, opts...)` | `text/text.go` | 一次性 Markdown → PlainText |
| `text.ConvertStream(r, w, opts...)` | `text/text.go` | 流式 Markdown → PlainText |
| `text.NewPlainText(w)` | `text/text.go` | 创建 PlainText 渲染器 |
| `text.Option` | `text/text.go` | `func(*config)` — WithFlags/WithExtensions |
| `PlainText` | `text/plaintext.go` | 实现 `renderer.Renderer` |

**`html` 包**：

| 符号 | 位置 | 用途 |
|---|---|---|
| `html.Convert(src, w, opts...)` | `html/convert.go` | 一次性 Markdown → HTML |
| `html.NewHTML(w)` | `html/convert.go` | 创建 XHTML 模式 HTML 渲染器 |
| `html.NewWithFlags(w, flags)` | `html/convert.go` | 创建指定 flags 的 HTML 渲染器 |
| `html.NewHTMLWithWriter(bw)` | `html/render.go` | 从 BufWriter 创建（测试用） |
| `html.Flags` | `html/convert.go` | 渲染器标志位（Debug/VerbatimEntities/SkipUTF8BOM/XHTML） |
| `html.Option` | `html/convert.go` | `func(*config)` — WithFlags/WithExtensions/WithRendererFlags |

### 3.3 用户层：自定义渲染器

用户只需实现 `renderer.Renderer` 接口的 5 个方法：

```go
type MyRenderer struct{}

func (r *MyRenderer) EnterBlock(t ast.BlockType, detail any) error { ... }
func (r *MyRenderer) LeaveBlock(t ast.BlockType, detail any) error { ... }
func (r *MyRenderer) EnterSpan(t ast.SpanType, detail any) error  { ... }
func (r *MyRenderer) LeaveSpan(t ast.SpanType, detail any) error  { ... }
func (r *MyRenderer) Text(t ast.TextType, text []byte) error       { ... }
```

使用方式：

```go
p := md4go.New(md4go.WithFlags(parser.DialectGitHub))
p.Parse([]byte("# Hello"), &MyRenderer{})
```

## 4. 核心实现映射（md4c C → md4go Go）

### 4.1 解析器核心

| md4c C 文件/函数 | md4go Go 文件 | 说明 |
|---|---|---|
| `md4c.c: md_parse()` | `parser/parser.go: Parse()` | 主解析入口 |
| `md4c.c: MD_CTX` | `parser/context.go: context` | 解析上下文 |
| `md4c.c: md_process_line()` | `parser/block.go` | 行处理 → 块级构建 |
| `md4c.c: md_process_inlines()` | `parser/emphasis.go` | 行内处理 → mark 匹配 |
| `md4c.c: md_analyze_marks()` | `parser/emphasis.go: analyzeMarks()` | mark 栈分析 |
| `md4c.c: md_resolve_links()` | `parser/autolink.go` | 链接解析 |
| `md4c.h: MD_BLOCKTYPE` | `ast/events.go: BlockType` | 块级类型枚举 |
| `md4c.h: MD_SPANTYPE` | `ast/events.go: SpanType` | 行内类型枚举 |
| `md4c.h: MD_TEXTTYPE` | `ast/events.go: TextType` | 文本类型枚举 |

### 4.2 渲染器

| md4c C | md4go Go | 说明 |
|---|---|---|
| `md4c-html.c` | `html/render.go` | HTML 渲染器 |
| `md4c-plain/main.c` | `text/plaintext.go` | PlainText 渲染器 |
| `md4c-html.c: MD_HTML_FLAG_*` | `html/convert.go: Flags` | 渲染器标志位 |
| `md4c-html.c: render_html_escaped()` | `html/render.go: writeEscaped()` | HTML 转义 |
| `md4c-html.c: render_entity()` | `html/render.go: renderEntity()` | 实体翻译 |
| `md4c-html.c: render_url_escaped()` | `html/render.go: writeURLEscaped()` | URL 编码 |

### 4.3 扩展系统

| md4c C | md4go Go | 说明 |
|---|---|---|
| `MD_FLAG_STRIKETHROUGH` | `extension/Strikethrough` | ~~删除线~~ |
| `MD_FLAG_TABLES` | `extension/Table` | GFM 表格 |
| `MD_FLAG_TASKLISTS` | `extension.Tasklist` | 任务列表 |
| `MD_FLAG_PERMISSIVE*` | `extension.PermissiveAutolinks` | 自动链接 |
| `MD_FLAG_LATEXMATHSPANS` | `extension.LatexMath` | LaTeX 数学 |
| `MD_FLAG_WIKILINKS` | `extension.Wikilink` | `[[wikilink]]` |
| `MD_FLAG_FOOTNOTES` | `extension.Footnote` | 脚注 |

## 5. 关键设计决策

### 5.1 为什么 html/text 不导入根包

根包 `md4go` 导入了 `parser`、`renderer`、`stream`。如果 `html` 或 `text` 导入 `md4go`，则形成 `md4go → parser → renderer` 和 `html → md4go → parser` 的环。解法：`html/text` 直接导入 `parser` 和 `renderer`，各自定义 `Option` 类型。

### 5.2 为什么 renderer 包只含接口

`renderer.Renderer` 是解析器和渲染器之间的契约。把它从具体实现（PlainText/HTML）中剥离出来，使得 `parser` 只需依赖接口定义而不依赖任何渲染实现，保证依赖方向单向。

### 5.3 BufWriter 为什么在 renderer 包

`text.PlainText` 和 `html.HTML` 都需要带缓冲的写入器。把 `BufWriter` 放在 `renderer/` 而非新建 `internal/` 包，是因为 `renderer` 已经是两端的共同依赖，无需引入新包。

### 5.4 测试文件循环依赖的处理

`parser/` 内部的测试如果需要 `html/text` 渲染器来验证输出，会遇到循环导入。解法：
- 使用 `html/text` 的测试：`package parser_test`（外部测试包）
- 使用 `parser` 内部未导出符号的测试：`package parser`（内部测试包）
- 拆分文件：`link_test.go`（外部）+ `link_internal_test.go`（内部）

## 6. 数据流详解

### 6.1 一次性解析（[]byte 输入）

```
md4go.New()  →  Parser{p: *parser.Parser}
  │
  ▼
Parser.Parse(src, renderer)
  │
  ├── parser.newSliceSource(src)      ← 将 []byte 切分为行
  │
  ├── parser.firstPass()              ← 块级构建 + refdef 收集
  │     ├── md_process_line()
  │     └── mark 栈积累
  │
  ├── parser.secondPass()             ← 行内解析 + 事件发射
  │     ├── md_process_inlines()
  │     ├── mark 匹配 → EnterSpan/LeaveSpan
  │     └── Text 事件
  │
  └── renderer.EnterBlock/LeaveBlock/EnterSpan/LeaveSpan/Text
```

### 6.2 流式解析（io.Reader 输入）

```
text.ConvertStream(reader, writer, opts...)
  │
  ├── parser.New()
  ├── stream.NewReaderSource(reader)   ← 逐行读取
  ├── text.NewPlainText(writer)
  │
  └── parser.ParseStream(lineSource, renderer)
        │
        └── 同 firstPass + secondPass，但行来源为 LineSource
```

## 7. 审查清单

按以下顺序审查代码可快速理解全局：

1. **`ast/events.go`** — 理解所有事件类型，这是所有代码的"词汇表"
2. **`renderer/renderer.go`** — 理解 Renderer 接口，这是解析器和渲染器的"契约"
3. **`md4go.go`** — 理解用户层 API，最简单的入口
4. **`parser/parser.go`** — 理解 Parse/ParseStream 主流程
5. **`parser/context.go`** — 理解解析上下文，所有状态都在这里
6. **`parser/flags.go`** — 理解 flags 常量，控制解析行为
7. **`text/plaintext.go`** — 最简单的 Renderer 实现，理解事件消费
8. **`html/render.go`** — 最完整的 Renderer 实现，理解所有事件类型
9. **`extension/gfm.go`** — 理解扩展注入机制
10. **`parser/block.go`** — 理解块级解析核心逻辑
