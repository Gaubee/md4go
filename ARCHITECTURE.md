# md4go 架构文档

> 按图索骥审查代码的参考文档。涵盖包结构、依赖方向、核心接口、数据流和关键设计决策。

## 1. 包结构总览

```
md4go/                          ← Go module root
├── md4go.go                    ← 根包：Parser API（底层解析入口）
├── ast/
│   └── events.go               ← 事件类型定义（BlockType/SpanType/TextType + Detail 结构体）
├── renderer/
│   ├── renderer.go             ← Renderer 接口（5 个回调方法）
│   └── writer.go               ← BufWriter（带缓冲的零拷贝写入器）
├── parser/                     ← 解析器核心（最大包，21 个源文件 + 12 个测试文件）
│   ├── parser.go               ← Parser 结构体 + Parse/ParseStream/parseLinesInternal
│   ├── context.go              ← 解析上下文（mark 栈、line 状态）
│   ├── block.go                ← 行分析 analyzeLine + 行处理 processLine
│   ├── block_stack.go          ← Block/Line/VerbatimLine/Container + blockStack
│   ├── container.go            ← 容器块处理（blockquote/list 嵌套、admonition 检测）
│   ├── inline.go               ← 行内处理三阶段管线 + processInlines 事件发射
│   ├── mark.go                 ← Mark 结构体 + markStacks（19 个 opener 栈）+ collectMarks
│   ├── emphasis.go             ← 强调/加粗 Rule-of-3 算法 + 各 mark 类型分析器
│   ├── link.go                 ← 链接/图片/脚注引用 bracket 解析 + resolveBrackets
│   ├── codespan.go             ← 行内代码 span
│   ├── autolink.go             ← 尖括号自动链接 + raw HTML inline 检测
│   ├── permissive_autolink.go  ← 无尖括号 URL/email/WWW 自动链接
│   ├── html_block.go           ← HTML 块 7 种类型判定
│   ├── attribute.go            ← 属性解析（link destination/title → Attribute）
│   ├── refdef.go               ← 引用链接定义 + 脚注定义检测/收集
│   ├── table.go                ← GFM 表格行解析
│   ├── trigger.go              ← BlockTrigger 接口 + 字符索引触发表 + 内置触发器
│   ├── flags.go                ← Flags 位掩码 + 常量 + Dialect 预设
│   ├── extender.go             ← Extender 接口 + Registrar
│   ├── compat.go               ← 兼容性 flag 预计算决策
│   └── md4c_punct.go           ← Unicode 标点分类表（opener/closer 判定用）
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
│   ├── extender.go             ← 包文档（Extender 机制说明）
│   └── gfm.go                  ← 全部 12 个扩展类型 + GFM 预设集
├── cmd/
│   └── md4go/
│       └── main.go             ← CLI 入口
├── integration/                ← 集成测试（package integration_test，导入 md4go 及子包）
├── testdata/                   ← 测试数据（CommonMark spec / 扩展 spec / fuzz 语料）
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

## 4. 核心类型与函数索引

### 4.1 解析器核心

| 文件 | 关键类型/函数 | 职责 |
|---|---|---|
| `parser/parser.go` | `Parser`, `Parse()`, `ParseStream()`, `parseLinesInternal()` | 解析器主体 + 行循环主流程 |
| `parser/context.go` | `context` | 解析上下文（mark 栈、容器栈、refdef map） |
| `parser/block.go` | `analyzeLine()`, `processLine()`, `lineAnalysis`, `LineType` | 行类型分析 + 块级构建/关闭 |
| `parser/block_stack.go` | `Block`, `Line`, `VerbatimLine`, `Container`, `blockStack` | 扁平块存储 + 容器状态 |
| `parser/inline.go` | `analyzeInlines()`, `processInlines()` | 行内三阶段管线 + 事件发射 |
| `parser/mark.go` | `Mark`, `markStacks`, `collectMarks()` | Mark 结构体 + 19 个 opener 栈 + mark 收集 |
| `parser/emphasis.go` | `analyzeMarks()`, `analyzeEmph()`, `analyzeTilde()` 等 | Rule-of-3 强调匹配 + 各 mark 类型分析器 |
| `parser/link.go` | `analyzeBracket()`, `resolveBrackets()` | 链接/图片/脚注/wikilink bracket 解析 |
| `parser/refdef.go` | `consumeLinkRefDefs()`, `RefDef`, `FootnoteDef` | 引用链接定义 + 脚注定义检测/收集 |
| `parser/flags.go` | `Flags`, `DialectCommonMark`, `DialectGitHub` | 位掩码标志 + Dialect 预设 |
| `parser/trigger.go` | `BlockTrigger`, `triggerTable` | 字符索引块触发器分发 |
| `parser/extender.go` | `Extender`, `Registrar` | 扩展注入接口 |
| `ast/events.go` | `BlockType`, `SpanType`, `TextType`, 各 `Detail` 结构体 | 事件类型枚举 + 详情结构 |

### 4.2 渲染器

| 文件 | 关键类型/函数 | 职责 |
|---|---|---|
| `renderer/renderer.go` | `Renderer` 接口 | 5 回调契约（EnterBlock/LeaveBlock/EnterSpan/LeaveSpan/Text） |
| `renderer/writer.go` | `BufWriter` | 4KB 带缓冲写入器 |
| `text/plaintext.go` | `PlainText` | 纯文本渲染器实现 |
| `text/text.go` | `Convert()`, `ConvertStream()`, `NewPlainText()` | 纯文本业务层封装 |
| `html/render.go` | `HTML`, `writeEscaped()`, `renderEntity()`, `writeURLEscaped()` | HTML 渲染器实现 |
| `html/convert.go` | `Convert()`, `NewHTML()`, `NewWithFlags()`, `Flags` | HTML 业务层封装 + 渲染器标志 |
| `html/entity.go` | HTML 实体查找表 | 命名实体 → Unicode 翻译 |

### 4.3 扩展系统

| 扩展 | 语法 | 注册内容 |
|---|---|---|
| `extension.Strikethrough` | `~~删除线~~` | `FlagStrikethrough` + mark char `~` |
| `extension.Table` | GFM 表格 | `FlagTables` + mark char `\|` |
| `extension.TaskList` | `- [x]` 任务列表 | `FlagTasklists` |
| `extension.PermissiveAutolinks` | URL/Email/WWW 自动链接 | `PermissiveAutolinks` + mark chars `@ : .` |
| `extension.LatexMath` | `$...$` / `$$...$$` | `FlagLatexMathSpans` + mark char `$` |
| `extension.Wikilink` | `[[wikilink]]` | `FlagWikilinks` + mark char `\|` |
| `extension.Footnote` | `[^1]` 脚注 | `FlagFootnotes` |
| `extension.Admonition` | `> [!NOTE]` 告诫块 | `FlagAdmonitions` |
| `extension.Superscript` | `^上标^` | `FlagSuperscripts` + mark char `^` |
| `extension.Subscript` | `~下标~` | `FlagSubscripts` + mark char `~` |
| `extension.Spoiler` | `\|\|剧透\|\|` | `FlagSpoilers` + mark char `\|` |
| `extension.Highlight` | `==高亮==` | `FlagHighlight` + mark char `=` |
| `extension.GFM` | GFM 预设 | 6 个扩展的组合切片 |

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
  ├── stream.NewSliceSource(src)     ← 将 []byte 切分为行（零拷贝）
  │
  ├── Pass 1: parseLinesInternal()   ← 块级构建 + refdef 收集（用 discardRenderer）
  │     ├── analyzeLine()
  │     ├── processLine()
  │     └── refDefs / footnoteDefs 收集
  │
  ├── Pass 2: parseLinesInternal()   ← 完整渲染（预填充 refdef 后）
  │     ├── analyzeLine()
  │     ├── processLine() → 块关闭时触发 analyzeInlines + processInlines
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
        └── 单遍 parseLinesInternal()，行来源为 LineSource
            （refdef 先见先得，前向引用退化为字面文本）
```

## 7. 审查清单

按以下顺序审查代码可快速理解全局：

1. **`ast/events.go`** — 理解所有事件类型，这是所有代码的"词汇表"
2. **`renderer/renderer.go`** — 理解 Renderer 接口，这是解析器和渲染器的"契约"
3. **`md4go.go`** — 理解用户层 API，最简单的入口
4. **`parser/parser.go`** — 理解 Parse/ParseStream/parseLinesInternal 主流程
5. **`parser/context.go`** — 理解解析上下文，所有状态都在这里
6. **`parser/flags.go`** — 理解 flags 常量，控制解析行为
7. **`parser/block.go`** — 理解 analyzeLine 行类型分析 + processLine 块构建/关闭
8. **`parser/mark.go`** — 理解 Mark 结构体和 19 个 opener 栈（Rule-of-3 基础）
9. **`parser/emphasis.go`** — 理解 Rule-of-3 强调匹配算法
10. **`parser/inline.go`** — 理解行内三阶段管线和事件发射
11. **`parser/link.go`** — 理解 bracket 配对和链接/图片解析
12. **`text/plaintext.go`** — 最简单的 Renderer 实现，理解事件消费
13. **`html/render.go`** — 最完整的 Renderer 实现，理解所有事件类型
14. **`extension/gfm.go`** — 理解扩展注入机制和全部 12 个扩展
