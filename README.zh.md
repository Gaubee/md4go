# md4go — Go 语言的 Markdown 解析器

中文 | [English](README.md)

md4go 是一个 Go 语言的 Markdown 解析器，采用 push-based 事件驱动模型，不构建 AST。**CommonMark 0.31 合规 652/652**，GFM 扩展（表格/删除线/任务列表/自动链接）全部支持。

## 快速上手

### 安装

```bash
go get github.com/userpro/md4go
```

### 最简用法：Markdown → 纯文本

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
// 输出:
// Hello
//
// item1
// item2
```

### 最简用法：Markdown → HTML

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
// 输出:
// <h1>Hello</h1>
// <ul>
// <li>item1</li>
// <li>item2</li>
// </ul>
```

### 命令行

```bash
# 编译
go build -o md4go ./cmd/md4go

# Markdown → 纯文本（默认 GFM 模式）
echo "# Hello" | ./md4go

# Markdown → HTML
echo "# Hello" | ./md4go -html

# 流式输入（低内存，仅纯文本模式）
cat large.md | ./md4go -stream

# goldmark 兼容模式
echo "| a | b |" | ./md4go -compat goldmark
```

## 三层架构

```
┌─────────────────────────────────────────────┐
│  业务层（一站式便捷封装）                      │
│  text.Convert()    html.Convert()            │
├─────────────────────────────────────────────┤
│  底层（解析 API）                             │
│  md4go.Parser.Parse(src, renderer)           │
│  renderer.Renderer 接口                      │
├─────────────────────────────────────────────┤
│  用户层（自定义渲染器）                        │
│  实现 Renderer 接口的 5 个方法                 │
└─────────────────────────────────────────────┘
```

- **业务层**（`text`/`html` 包）：一行代码完成转换
- **底层**（`md4go` 根包）：完整解析 API，事件推送到任意 Renderer
- **用户层**：实现 `renderer.Renderer` 接口自定义输出格式

## 使用场景

### 场景 1：Markdown 文本提取（RAG/搜索索引/内容清洗）

```go
// 从 Markdown 中提取纯文本，去除所有格式标记
var buf bytes.Buffer
text.Convert(markdownBytes, &buf, text.WithFlags(parser.DialectGitHub))
plainText := buf.String()
```

典型用途：
- RAG 系统的文档预处理
- 全文搜索引擎的内容索引
- Markdown 邮件/通知的纯文本版本
- 聊天消息的文本摘要

### 场景 2：Markdown → HTML 渲染

```go
// 生成 HTML，兼容 XHTML 模式
var buf bytes.Buffer
html.Convert(markdownBytes, &buf,
    html.WithFlags(parser.DialectGitHub),
    html.WithRendererFlags(html.FlagXHTML),
)
```

### 场景 3：流式处理大文件

```go
// 逐行读取，内存占用恒定
file, _ := os.Open("large.md")
defer file.Close()
text.ConvertStream(file, os.Stdout, text.WithFlags(parser.DialectGitHub))
```

> **注意**：流式模式下 refdef（引用链接定义）遵循"先见先得"规则，前向引用会退化为字面文本。一次性解析（`Convert`）无此限制。

### 场景 4：自定义渲染器（结构化数据提取）

```go
// 提取所有链接
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

// 使用
p := md4go.New(md4go.WithFlags(parser.DialectGitHub))
ext := &LinkExtractor{}
p.Parse(src, ext)
fmt.Println(ext.links) // ["https://example.com", ...]
```

## API 参考

### 根包 `md4go` — 解析 API

```go
// 创建解析器
p := md4go.New(
    md4go.WithFlags(parser.DialectGitHub),       // 设置解析标志
    md4go.WithExtensions(&extension.Table{}),     // 注册扩展
)

// 解析 []byte → 推送事件到 renderer
p.Parse(src, myRenderer)

// 流式解析 io.Reader → 推送事件到 renderer
p.ParseStream(lineSource, myRenderer)
```

### `text` 包 — 纯文本

```go
// 一站式转换
text.Convert(src, writer, text.WithFlags(...), text.WithExtensions(...))

// 流式转换
text.ConvertStream(reader, writer, text.WithFlags(...))

// 获取渲染器实例（高级用法）
pt := text.NewPlainText(writer)
p.Parse(src, pt)
pt.Flush()
```

### `html` 包 — HTML

```go
// 一站式转换
html.Convert(src, writer, html.WithFlags(...), html.WithExtensions(...), html.WithRendererFlags(...))

// XHTML 模式（默认）
h := html.NewHTML(writer)

// 指定渲染器标志
h := html.NewWithFlags(writer, html.FlagXHTML|html.FlagVerbatimEntities)

// 高级用法
h := html.NewHTMLWithWriter(renderer.NewBufWriter(writer))
```

**HTML 渲染器标志**：

| 标志 | 值 | 说明 |
|---|---|---|
| `FlagDebug` | 0x0001 | 调试输出 |
| `FlagVerbatimEntities` | 0x0002 | 实体原样输出（不翻译为 UTF-8） |
| `FlagSkipUTF8BOM` | 0x0004 | 跳过输入开头的 UTF-8 BOM |
| `FlagXHTML` | 0x0008 | XHTML 自闭合标签（`<br />`） |

### `renderer` 包 — 接口定义

```go
type Renderer interface {
    EnterBlock(t ast.BlockType, detail any) error
    LeaveBlock(t ast.BlockType, detail any) error
    EnterSpan(t ast.SpanType, detail any) error
    LeaveSpan(t ast.SpanType, detail any) error
    Text(t ast.TextType, text []byte) error
}
```

## 调优指南

### 选择解析模式

| 模式 | 常量 | 适用场景 |
|---|---|---|
| CommonMark | `parser.DialectCommonMark` | 标准 Markdown，严格合规 |
| GitHub Flavored | `parser.DialectGitHub` | GFM 扩展（表格/删除线/任务列表/自动链接） |

`DialectGitHub` = `PermissiveAutolinks | FlagTables | FlagStrikethrough | FlagTasklists | FlagAdmonitions | FlagFootnotes`

### 选择输入方式

| 方式 | API | 内存 | 前向引用 |
|---|---|---|---|
| 一次性 `[]byte` | `Parse` / `Convert` | O(n) | ✅ 完整支持 |
| 流式 `io.Reader` | `ParseStream` / `ConvertStream` | O(行) | ❌ 先见先得 |

**建议**：文档 < 10MB 用 `Convert`，超大文档用 `ConvertStream`。

### 选择渲染目标

| 目标 | 包 | 特点 |
|---|---|---|
| 纯文本 | `text` | 去除所有格式，保留文本内容和语义分隔 |
| HTML | `html` | 完整 HTML 输出，XHTML/HTML5 可选 |
| 自定义 | `renderer` | 实现 Renderer 接口 |

### 性能提示

1. **复用 Parser**：`md4go.New()` 创建的 `Parser` 可多次调用 `Parse()`
2. **流式节省内存**：`ConvertStream` 逐行读取，内存占用与文档大小无关
3. **BufWriter 自动缓冲**：`text.NewPlainText(w)` 和 `html.NewHTML(w)` 内部使用 4KB 缓冲区
4. **按需启用扩展**：只注册需要的扩展，减少解析开销

### 扩展注入

```go
// 仅启用表格和删除线
p := md4go.New(md4go.WithExtensions(
    &extension.Table{},
    &extension.Strikethrough{},
))

// GFM 全量扩展（快捷方式）
p := md4go.New(md4go.WithFlags(parser.DialectGitHub))
// 等价于:
p := md4go.New(md4go.WithExtensions(extension.GFM...))
```

**可用扩展**：

| 扩展 | 语法 |
|---|---|
| `extension.Strikethrough` | `~~删除线~~` |
| `extension.Table` | GFM 表格 |
| `extension.Tasklist` | `- [x] 任务` |
| `extension.PermissiveAutolinks` | URL/邮箱/WWW 自动链接 |
| `extension.Footnote` | `[^1]` 脚注 |
| `extension.LatexMath` | `$行内$` / `$$块级$$` |
| `extension.Wikilink` | `[[链接]]` |
| `extension.Superscript` | `^上标^` |
| `extension.Subscript` | `~下标~` |
| `extension.Spoiler` | `||剧透||` |
| `extension.Highlight` | `==高亮==` |
| `extension.Admonition` | `> [!NOTE]` 告诫块 |

## 合规性

| 标准 | 结果 |
|---|---|
| CommonMark 0.31 | 652/652 ✅ |
| GFM 表格/删除线/任务列表/自动链接 | 全部通过 ✅ |

## 已知差异

md4go 默认遵循 GFM/CommonMark 标准。与其他实现的差异分两类：**有意改进**（默认即生效，无需 flag）和**可通过兼容 flag 对齐**。

### 有意改进（默认行为）

| 编号 | 场景 | 默认行为 | 说明 |
|---|---|---|---|
| S-01 | `||` 表格单元格 | 拆分为单元格边界 | `FlagProtectDoublePipe` 可 opt-in 保护 `||` 不被分割 |
| S-02/04 | tight 列表段落分隔 | 保留 `\n` 词界、发出 P 事件 | 更利于文本提取 |
| S-03 | `[[target\|label]]` Wikilink | 识别为 wikilink | 支持带标签的 wikilink |
| S-05 | 脚注引用 | 输出 `[N]` | 保留引用编号 |
| S-06 | 含 NULL 的 code span | 识别并替换为 U+FFFD | 遵循 CommonMark |

### 与 goldmark 的差异

主要差异可通过 `GoldmarkCompat` 预设对齐：

| 场景 | 对齐 flag | 具体案例 |
|---|---|---|
| 表格不可中断段落（GFM 标准） | `FlagTableInterruptParagraph` | 段落后接表格：默认不识别为表格；设 flag 后段落末行提升为表头 |
| HTML 实体解码 | `FlagDecodeEntities` | `&amp; &copy;`：默认保留实体文本；设 flag 后解码为 `& ©` |
| 表格列数严格校验 | `FlagStrictTableColumns` | 标题 3 列、分隔行 2 列：默认宽松识别；设 flag 后不识别为表格 |
| 行内跨度/括号多余空格 | — | goldmark DOM 遍历副作用，不应复制 |

> 完整对比报告见 `DIFF_REPORT.md`。

## 项目文档

| 文档 | 位置 | 说明 |
|---|---|---|
| README.md | 根目录 | 英文版快速上手 + API 参考 |
| README.zh.md | 根目录 | 中文版快速上手 + API 参考 |
| ARCHITECTURE.md | 根目录 | 架构设计 |
| DESIGN.md | 根目录 | 算法设计细节 |
| TESTING.md | 根目录 | 测试体系说明 |

## 致谢

本项目最初基于 [md4c](https://github.com/mity/md4c) v0.5.3 的算法设计进行 Go 移植，在此基础上做了工程化改进和标准合规性增强。
