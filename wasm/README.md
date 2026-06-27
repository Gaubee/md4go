# md4go WebAssembly

将 md4go 编译为 WebAssembly，在浏览器端直接解析 Markdown。

## 构建

```bash
# 1. 编译 WASM 二进制
GOOS=js GOARCH=wasm go build -o md4go.wasm ./wasm

# 2. 复制 Go WASM 运行时
cp "$(go env GOROOT)/lib/wasm/wasm_exec.js" .
```

## 快速上手

```html
<script type="module">
  import { initMd4go } from './wasm/md4go.js';

  const md4go = await initMd4go();

  // Markdown → HTML (GFM, XHTML)
  md4go.parseToHTML("# Hello **world**");  // → <h1>Hello <strong>world</strong></h1>

  // Markdown → 纯文本
  md4go.parseToText("- item 1\n- item 2");  // → item 1\nitem 2
</script>
```

## API

### 一次性转换

#### `parseToHTML(md, flagsOrOptions?)`

| 参数 | 类型 | 默认值 | 说明 |
|---|---|---|---|
| `md` | `string` | — | Markdown 源文本 |
| `flagsOrOptions` | `number \| ParseOptions` | `1` | 解析 flags，或 `{flags, rendererFlags}` 对象 |

返回 HTML 字符串（默认 XHTML）。传入 options 对象时自动委托给 `parseToHTMLWithOptions`。

```javascript
// 快速方式
md4go.parseToHTML("# Hi");                       // GFM + XHTML
md4go.parseToHTML("## T", md4go.Flags.CommonMark); // CommonMark + XHTML

// 完整 options
md4go.parseToHTML("# Hi", {
  flags: md4go.Flags.GitHub,
  rendererFlags: 0,  // 关闭 XHTML → <br> 而非 <br />
});
```

#### `parseToText(md, flags?)`

| 参数 | 类型 | 默认值 | 说明 |
|---|---|---|---|
| `md` | `string` | — | Markdown 源文本 |
| `flags` | `number` | `1` | 解析 flags |

返回纯文本字符串。

#### `parseToHTMLWithOptions(md, options?)`

| 参数 | 类型 | 默认值 | 说明 |
|---|---|---|---|
| `md` | `string` | — | Markdown 源文本 |
| `options.flags` | `number` | `1` | 解析 flags |
| `options.rendererFlags` | `number` | `0x0008` | HTML 渲染器 flags |

### Parser 复用 —— 场景 1

复用同一个 Parser 实例解析多段 Markdown，避免重复创建。适合编辑器实时预览、批量处理等场景。

```javascript
const p = md4go.createParser(md4go.Flags.GitHub);

// 多次解析复用同一个 parser
inputEl.addEventListener('input', () => {
  document.getElementById('preview').innerHTML =
    p.parseToHTML(inputEl.value);
});

// 释放
p.dispose();
```

**Parser 对象方法**：

| 方法 | 签名 | 说明 |
|---|---|---|
| `parseToHTML(md, rendererFlags?)` | `string → string` | 解析为 HTML |
| `parseToText(md)` | `string → string` | 解析为纯文本 |
| `parseWithRenderer(md, callbacks)` | `string, object → null\|{error}` | 自定义渲染 |
| `dispose()` | `() → void` | 释放资源 |

### Stream 分块 —— 场景 3

为超大文档提供分块累积模式：逐块 `write()`，最后一次性 `finishHTML()` / `finishText()` 解析。

```javascript
// 从 fetch Response stream 读取
const s = md4go.createStreamParser(md4go.Flags.GitHub);

const response = await fetch('/large-doc.md');
const reader = response.body.getReader();
const decoder = new TextDecoder();

while (true) {
  const { done, value } = await reader.read();
  if (done) break;
  s.write(decoder.decode(value, { stream: true }));
}

const html = s.finishHTML();
s.dispose();
```

```javascript
// 从 FileReader 分块读取
const file = input.files[0];
const s = md4go.createStreamParser(md4go.Flags.GitHub);

const chunkSize = 64 * 1024; // 64KB
for (let offset = 0; offset < file.size; offset += chunkSize) {
  const blob = file.slice(offset, offset + chunkSize);
  const text = await blob.text();
  s.write(text);
}

const text = s.finishText();
s.dispose();
```

> **注意**：WASM 为单线程模型，`write()` 仅累积数据到 Go 侧缓冲区，`finishHTML()` 时一次性解析全部。不会降低峰值内存，但提供了分块输入模式。

**Stream 对象方法**：

| 方法 | 签名 | 说明 |
|---|---|---|
| `write(chunk)` | `string → void` | 追加数据块 |
| `finishHTML(rendererFlags?)` | `number? → string` | 解析并返回 HTML，重置缓冲区 |
| `finishText()` | `() → string` | 解析并返回纯文本，重置缓冲区 |
| `dispose()` | `() → void` | 释放资源 |

### 自定义渲染 —— 场景 5

通过 JavaScript 回调直接接收解析事件，实现结构化数据提取或自定义输出格式。对应 Go 侧 `renderer.Renderer` 接口的 5 个方法。

**回调函数**：

| 回调 | Go 接口方法 | 签名 |
|---|---|---|
| `enterBlock` | `EnterBlock(BlockType, detail)` | `(type: number, detail: Object\|null) => void` |
| `leaveBlock` | `LeaveBlock(BlockType, detail)` | `(type: number, detail: Object\|null) => void` |
| `enterSpan` | `EnterSpan(SpanType, detail)` | `(type: number, detail: Object\|null) => void` |
| `leaveSpan` | `LeaveSpan(SpanType, detail)` | `(type: number, detail: Object\|null) => void` |
| `text` | `Text(TextType, text)` | `(type: number, text: string) => void` |

所有回调均可选，未提供的不会产生开销。

#### 适配模式 1：提取结构化数据

```javascript
// 提取所有链接（等价于 README.zh.md 场景5 示例）
const links = [];
md4go.parseWithRenderer("see [link](https://e.com) and [more](https://m.org)", {
  enterSpan(type, detail) {
    if (type === md4go.SpanType.SpanLink) {
      links.push({ href: detail.href, title: detail.title });
    }
  },
});
console.log(links);
// [{ href: "https://e.com", title: "" }, { href: "https://m.org", title: "" }]
```

```javascript
// 提取文档大纲（所有标题及层级）
const outline = [];
md4go.parseWithRenderer(doc, {
  enterBlock(type, detail) {
    if (type === md4go.BlockType.BlockH) {
      outline.push({ level: detail.level, text: '...pending...' });
    }
  },
  text(type, text) {
    if (type === md4go.TextType.TextNormal && outline.length > 0) {
      outline[outline.length - 1].text += text;
    }
  },
});
```

#### 适配模式 2：构建自定义输出格式

```javascript
// 将 Markdown 转换为纯文本（逐步输出，等价于 parseToText）
const chunks = [];
md4go.parseWithRenderer(md, {
  enterBlock(type, detail) {
    if (type === md4go.BlockType.BlockP) {
      chunks.push('\n\n'); // 段落间空行
    }
  },
  text(type, text) {
    if (type === md4go.TextType.TextNormal ||
        type === md4go.TextType.TextEntity) {
      chunks.push(text);
    }
  },
});
```

#### 适配模式 3：带状态的解析

```javascript
// 统计各类 block/span 数量
const counts = { blocks: {}, spans: {}, chars: 0 };
md4go.parseWithRenderer(md, {
  enterBlock(type) { counts.blocks[type] = (counts.blocks[type] || 0) + 1; },
  enterSpan(type) { counts.spans[type] = (counts.spans[type] || 0) + 1; },
  text(type, text) {
    if (type === md4go.TextType.TextNormal) counts.chars += text.length;
  },
});
```

**关键技巧**：
- 利用 JS 闭包在回调之间共享状态（如上例的 `counts`、`links`、`outline`）
- 事件按文档顺序推送：先 `EnterBlock(P)`，再内联 span 事件，最后 `LeaveBlock(P)`
- detail 为 `null` 的类型通过类型常量区分（如 `SpanEm`、`SpanStrong` 无 detail）
- `TextType` 可用于区分普通文本和特殊内容（entity、code、HTML 等）

## 直接使用全局函数

```html
<script src="wasm_exec.js"></script>
<script>
  const go = new Go();
  WebAssembly.instantiateStreaming(fetch("md4go.wasm"), go.importObject)
    .then(result => { go.run(result.instance); })
    .then(() => {
      // One-shot
      console.log(md4goParseToHTML("# Hello", 1));
      console.log(md4goParseToText("Hello"));

      // Parser reuse
      const p = md4goCreateParser(1);
      console.log(p.parseToHTML("## Second"));

      // Stream
      const s = md4goCreateStreamParser(1);
      s.write("# part1\n");
      s.write("## part2\n");
      console.log(s.finishHTML());
    });
</script>
```

## 常量

### 解析 Flags

| 常量 | 值 | 说明 |
|---|---|---|
| `md4go.Flags.CommonMark` | `0` | 标准 CommonMark（无扩展） |
| `md4go.Flags.GitHub` | `0x180F0C` | GFM 扩展（表格/删除线/任务列表/脚注等） |

高阶用户可直接传入任意 bitmask 组合。

### HTML Renderer Flags

| 常量 | 值 | 说明 |
|---|---|---|
| `md4go.RendererFlags.FlagDebug` | `0x0001` | 调试输出 |
| `md4go.RendererFlags.FlagVerbatimEntities` | `0x0002` | 实体原样输出 |
| `md4go.RendererFlags.FlagSkipUTF8BOM` | `0x0004` | 跳过 UTF-8 BOM |
| `md4go.RendererFlags.FlagXHTML` | `0x0008` | XHTML 自闭合标签 |
| `md4go.RendererFlags.FlagNoXHTMLEscaping` | `0x0010` | 不编码 `'` `"` |

### BlockType / SpanType / TextType

参见 [`md4go.js`](md4go.js) 中 `BlockType`、`SpanType`、`TextType` 常量对象。

## Detail 参考

回调函数的 `detail` 参数按类型携带不同字段（`null` 表示无 detail）：

| Block/Span | detail 字段 |
|---|---|
| BlockH | `{level}` |
| BlockCode | `{info, lang, fenceChar}` |
| BlockUL | `{isTight, mark}` |
| BlockOL | `{start, isTight, mark}` |
| BlockLI | `{isTask, taskMark, taskMarkOff}` |
| BlockTable | `{colCount, headRowCount, bodyRowCount}` |
| BlockTH/TD | `{align}` |
| BlockAdmonition | `{type}` |
| BlockFootnoteDef | `{id, refCount, label}` |
| SpanLink | `{href, title, isAutolink}` |
| SpanImg | `{src, title}` |
| SpanFootnoteRef | `{id, refId, label}` |
| SpanWikilink | `{target}` |
| 其他 (Em,Strong,Code,Del,…) | `null` |

> 完整 API 参考见 [README.zh.md](../README.zh.md)。

## 测试

```bash
# 一键运行 E2E 测试
go test ./integration/ -run TestWASME2E -v

# 直接运行 JS 测试（需先手动构建 WASM）
GOOS=js GOARCH=wasm go build -o wasm/md4go.wasm ./wasm
cp "$(go env GOROOT)/lib/wasm/wasm_exec.js" wasm/
node --test --test-reporter spec wasm/md4go_e2e.test.js
```

**测试覆盖**：parseToHTML、parseToText、parseToHTMLWithOptions、parseWithRenderer、createParser、createStreamParser、常量导出、边界用例，共 48 条用例。

**前置条件**：Node.js 18+（Node.js 不可用时 `go test` 自动跳过）。

## 文件

| 文件 | 说明 |
|---|---|
| `main.go` | WASM 入口，注册全局 JS 函数 |
| `md4go.js` | ES module loader，封装 `initMd4go()` |
| `md4go.wasm` | 编译产物（已 gitignore） |
| `wasm_exec.js` | Go WASM 运行时（从 `$GOROOT/lib/wasm/` 复制） |
| `md4go_e2e.test.js` | E2E 测试（Node.js `node:test`，零依赖） |
