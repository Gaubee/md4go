# md4go 算法设计文档

> 本文档解释 md4go 解析器的核心算法设计细节，作为 [ARCHITECTURE.md](ARCHITECTURE.md) 的补充。
> 架构文档侧重"包结构和依赖方向"，本文档侧重"算法如何工作"。

## 目录

1. [解析总览：两遍架构与增量管线](#1-解析总览两遍架构与增量管线)
2. [行分析算法（analyzeLine）](#2-行分析算法analyzeline)
3. [容器栈模型（nParents / nBrothers / nChildren）](#3-容器栈模型nparents--nbrothers--nchildren)
4. [块构建与关闭（processLine）](#4-块构建与关闭processline)
5. [Mark 系统（collectMarks）](#5-mark-系统collectmarks)
6. [行内三阶段管线](#6-行内三阶段管线)
7. [Rule-of-3 强调匹配算法](#7-rule-of-3-强调匹配算法)
8. [Bracket 链接解析算法](#8-bracket-链接解析算法)
9. [行内事件发射（processInlines）](#9-行内事件发射processinlines)
10. [扩展注入机制](#10-扩展注入机制)
11. [流式解析与前向引用降级](#11-流式解析与前向引用降级)

---

## 1. 解析总览：两遍架构与增量管线

### 1.1 核心设计选择

md4go 继承 md4c 的核心设计：**不构建 AST 树**。解析器直接将解析结果以事件流推送给渲染器，事件产生即消费即释放。这带来两个关键优势：

- **零内存开销**：不需要为整个文档构建节点树
- **天然增量**：块关闭即可触发行内处理和事件推送

### 1.2 两种输入模式

```
                    ┌──────────────────────────────────────┐
                    │       parseLinesInternal()           │  ← 唯一核心
                    │  (行循环 → analyzeLine → processLine) │
                    └──────────────┬───────────────────────┘
                                   │
                    ┌──────────────┴───────────────┐
                    │                              │
              Parse ([]byte)                ParseStream (LineSource)
                    │                              │
     ┌──────────────┴──────────────┐               │
     │                             │               │
  Pass 1                    Pass 2            单遍
  discardRenderer           真实 renderer      真实 renderer
  收集 refdef/footnote      预填充 refdef      先见先得
  (前向引用可用)            完整渲染            (前向引用降级)
```

**`Parse`（两遍）**：第一遍用 `discardRenderer`（空渲染器）扫描全文收集所有引用链接定义和脚注定义到 `refDefs`/`footnoteDefs` map；第二遍预填充这些 map 后完整渲染。这保证了前向引用（`[link][ref]` 出现在 `[ref]: url` 之前）能正确解析。

**`ParseStream`（单遍）**：直接调用 `parseLinesInternal`，refdef 在首次遇到时加入 map。前向引用因 map 中尚无对应条目而退化为字面文本。

### 1.3 parseLinesInternal 主循环

```
EnterBlock(DOC)
pivot = dummyBlankLine          ← 双缓冲：pivot 指向上一行分析结果
while line := src.NextLine():
    la = analyzeLine(ctx, line, pivot)   ← 行类型分析
    processLine(ctx, la, renderer)       ← 块构建/关闭 + 事件发射
    pivot = la
endCurrentBlock()                        ← 关闭尾部未关闭的叶子块
leaveContainers(ctx, 0)                  ← 关闭剩余容器（从内到外）
processFootnoteDefs()                    ← 发射已引用的脚注定义
LeaveBlock(DOC)
```

双缓冲设计（`lineBufs[2]`）确保 `analyzeLine` 可以同时访问当前行和前一行的分析结果（`pivot`），这对 setext 标题检测（需判断上一行是否为段落文本）等场景至关重要。

---

## 2. 行分析算法（analyzeLine）

`analyzeLine` 是块级解析的入口，负责判断每一行的类型。它镜像 md4c 的 `md_analyze_line()`，按固定优先级链依次检测。

### 2.1 检测优先级链

```
1. 空行检测（LineBlank）
2. 容器标记处理（blockquote '>', list -+*digits）  ← 步骤 1
3. 缩进计算 + 内容提取
4. Setext 下划线检测（仅当 pivot 为段落文本）      ← 直接检测，非触发表
5. 水平线检测（HR: --- *** ___）                   ← 直接检测，非触发表
6. 触发表分发（ATX # / 围栏代码 `~ / HTML <）      ← 字符索引触发表
7. 缩进代码块检测（缩进 ≥ 4）
8. 表格行检测（含分隔行检测）
9. 默认 → LineText（段落文本）
10. Admonition 检测（blockquote 内 [!TYPE] 标记）  ← 后处理
```

### 2.2 缩进计算与 Tab 展开

CommonMark 规定 Tab 每 4 列为一个制表位。`measureIndentFrom` 从指定列开始展开 Tab：

```go
func measureIndentFrom(line []byte, startCol int) (relIndent int, off int) {
    indent := startCol
    for off < len(line) {
        if line[off] == ' ' { indent++; off++ }
        else if line[off] == '\t' { indent = (indent + 4) & ^3; off++ }  // 下一个 4 列边界
        else { break }
    }
    return indent - startCol, off
}
```

关键细节：当 Tab 紧跟在容器标记（如 `>` 或 `-`）之后时，必须从标记后的列开始展开，而非从列 0 开始。这确保了 `>\ttext`（blockquote + Tab + 内容）的正确处理。

### 2.3 字符索引触发表

ATX 标题、围栏代码、HTML 块通过 `[256][]BlockTrigger` 触发表分发。触发器按注册顺序（即优先级）检查：

```
'#'  → atxTrigger{}        ← ATX 标题 (# ~ ######)
'`'  → fencedCodeTrigger{}  ← 围栏代码（``` 开头）
'~'  → fencedCodeTrigger{}  ← 围栏代码（~~~ 开头）
'<'  → htmlBlockTrigger{}   ← HTML 块（7 种类型）
```

Setext 和 HR 不通过触发表，因为它们的检测需要在容器标记处理之前进行（CommonMark 规定 setext/HR 优先于容器嵌套）。

### 2.4 lineAnalysis 结构体

每次 `analyzeLine` 返回一个 `lineAnalysis`，包含：

| 字段 | 说明 |
|---|---|
| `lineType` | 行类型（LineBlank/LineATXHeader/LineFencedCode/...） |
| `data` | 类型相关数据（ATX 级别、围栏字符、HTML 块类型等） |
| `content` | 行的有效内容（已剥离容器标记和缩进） |
| `nParents` | 此行属于多少个已有容器 |
| `nBrothers` | 1 表示是已有列表的新兄弟项 |
| `nChildren` | 需要进入多少个新子容器 |
| `newContainers` | 新容器的信息列表 |

---

## 3. 容器栈模型（nParents / nBrothers / nChildren）

容器块（blockquote、列表/列表项）可以任意嵌套。md4c 用三值模型描述每行与容器栈的关系：

### 3.1 三值语义

```
> > - item          ← 容器栈深度 = 3（blockquote > blockquote > list）
> >   more          ← 属于同一列表项的续行

行 "> > - item":
  nParents  = 0  (不属于任何已有容器，因为上一行是列表项的续行)
  nBrothers = 1  (是已有列表的新兄弟项)
  nChildren = 0  (不需要进入新容器)

行 "> >   more":
  nParents  = 3  (属于全部 3 个已有容器)
  nBrothers = 0
  nChildren = 0
```

### 3.2 容器栈操作

`processLine` 根据 nParents/nBrothers/nChildren 决定容器的进入/退出：

```
1. 离开容器：从最内层退出，直到栈深度 == nParents
   （如果 nBrothers == 1，多退一层，因为兄弟项需要新列表项）

2. 进入新容器：nChildren 个新容器依次 push 到栈
   newContainers[] 携带每个容器的类型（'>' 或列表标记）

3. 容器标记处理完成后，剩余内容交给叶子块分析
```

### 3.3 紧/松列表跟踪

CommonMark 区分紧列表（tight）和松列表（loose）：

- **紧列表**：列表项之间无空行 → `<li>` 直接包含内容
- **松列表**：列表项之间有空行 → `<li>` 内包含 `<p>` 段落

跟踪通过 `lastLineHasListLooseningEffect` 标志实现：当一行是空行且当前在列表内时，设置此标志，后续列表项检测时会据此将列表标记为 loose。

### 3.4 Admonition 检测

Admonition 是 blockquote 的特殊形式。当 blockquote 容器的第一行内容匹配 `[!TYPE]`（TYPE 为 note/tip/important/warning/caution 之一）时，该 blockquote 被标记为 admonition，`[!TYPE]` 行被视为空行（不产生内容）：

```
> [!NOTE]
> This is a note.
```

检测发生在 `analyzeLine` 的后处理阶段（步骤 17），当新容器是 blockquote 且内容匹配 `[!TYPE]` 模式时触发。

---

## 4. 块构建与关闭（processLine）

`processLine` 接收 `lineAnalysis`，构建新块或关闭已有块，并在块关闭时触发行内处理和事件发射。

### 4.1 块的扁平存储

md4go 不构建块树。所有块存储在 `blockStack` 的扁平切片中：

```go
type blockStack struct {
    blocks  []Block         // 所有块（扁平存储）
    lines   []Line          // 普通文本行
    vlines  []VerbatimLine  // 原样行（代码/HTML）
    current *Block          // 正在构建的块
}
```

每个 `Block` 通过 `LineIdx` 和 `NLines` 引用其行数据。容器块不持有行（它们的"内容"是子块），叶子块持有行。

### 4.2 块生命周期

```
startNewBlock() → current 指向新块
  addLine() / addVerbatimLine() → 追加行到 current
endBlock() → 关闭 current，触发行内处理：
  1. analyzeInlines(ctx, block)   ← 收集 mark + 解析配对
  2. processInlines(ctx, block, r) ← 遍历 mark，发射事件
  3. EnterBlock(...) / Text(...) / LeaveBlock(...)
  4. compact() → 如果无其他块打开，释放行数据
```

### 4.3 内存压缩

`compact()` 在无块打开时清空 `lines`/`vlines`/`blocks` 切片。这对流式解析至关重要——已发射块的行数据不再需要，清空后内存占用与文档大小无关（仅与当前打开的块相关）。

---

## 5. Mark 系统（collectMarks）

Mark 是行内解析的基本单元。每个 mark 记录文本中的一个潜在行内结构标记（`*`、`_`、`[`、`]`、`~` 等）。

### 5.1 Mark 结构体

```go
type Mark struct {
    Beg   int        // 在块文本中的起始偏移
    End   int        // 在块文本中的结束偏移
    Prev  int        // 前一个 mark 索引 / 已解析的 opener 索引
    Next  int        // 下一个 mark 索引 / 已解析的 closer 索引
    Ch    byte       // mark 字符（'*', '_', '[', ...）
    Flags markFlags  // 状态标志
}
```

`Prev`/`Next` 在不同阶段有不同用途：
- **配对前**：用于 opener 栈的链表链接
- **配对后**：opener.Next 指向 closer，closer.Prev 指向 opener

### 5.2 Mark 字符集

CommonMark 基础 mark 字符：`\ * _ \` & ; < > [ ] ! \0`

扩展通过 `AddMarkChar()` 追加额外字符：
- Strikethrough/Subscript → `~`
- Table → `|`
- LatexMath → `$`
- Superscript → `^`
- Highlight → `=`
- Spoiler/Wikilink → `|`
- PermissiveAutolinks → `@ : .`

### 5.3 collectMarks 扫描

`collectMarks` 线性扫描块文本，遇到 mark 字符时创建 mark 并设置初始 flags。关键处理包括：

- **反斜杠转义**：`\` + ASCII 标点 → 标记为 `markResolved`（已解决，不参与配对）
- **NULL 字符**：`\0` → 标记为 NULL mark，发射时替换为 U+FFFD
- **强调 mark**：`*`/`_` 连续运行合并为一个 mark，根据左右字符判断 opener/closer 属性
- **Bracket**：`[` 标记为 opener，`]` 标记为 closer；`!` 前缀的 `[` 设置 `markBracketCanBeImage`
- **代码 span**：连续 `` ` `` 合并为一个 mark

### 5.4 强调 mark 的 opener/closer 判定

CommonMark 规定 `*` 和 `_` 的 opener/closer 属性取决于相邻字符的 Unicode 类别：

```
左侧字符        右侧字符        判定
─────────────────────────────────────────
空格/空白       非空格          → 仅 opener
非空格          空格/空白      → 仅 closer
非空格          非标点          → 仅 closer（*）/ opener+closer（_ 特殊规则）
标点            标点            → opener+closer
```

`_` 比 `*` 更严格：`_` 在单词内部（两侧均为非标点非空白）不能作为 emphasis，而 `*` 可以。这通过 `markEmphOC`（opener+closer）flag 区分。

---

## 6. 行内三阶段管线

行内解析在 `analyzeInlines` 中分三个阶段执行，**顺序不可更改**：

```
Phase 1: collectMarks(blockText)
    ↓ 扫描文本，收集所有 mark 到 marks[]

Phase 2: analyzeMarks("[]!") + resolveBrackets()
    ↓ 配对 bracket，解析链接/图片/脚注/wikilink
    ↓ 链接内容递归调用 analyzeLinkContents

Phase 3: analyzeLinkContents("*_&~^$|=")
    ↓ 对顶层和每个链接内容区间执行 mark 分析
    ↓ 强调匹配、实体解析、扩展 mark 配对
```

### 6.1 为什么是这个顺序

1. **Phase 2 先于 Phase 3**：链接/图片的文本内容可能包含强调（如 `[**bold** link](url)`），必须先确定 bracket 边界，再在 bracket 内部解析强调
2. **链接不可嵌套**：CommonMark 规定链接文本内不能再有链接（但可以有图片）。Phase 2 的 `resolveBrackets` 从内到外处理，内部链接解析后外部 bracket 如果检测到内部已解析为链接，则放弃链接解析
3. **Phase 3 递归**：`resolveBracketLink` 成功后调用 `analyzeLinkContents` 对 `[opener+1, closer)` 区间递归执行 Phase 3，确保链接文本内的强调被正确解析

### 6.2 analyzeMarks 遍历

```go
for i := 0; i < len(marks); i++ {
    mark := &marks[i]
    if mark.Flags&markResolved != 0 {
        // 跳过已解析的 span（opener → 跳到 closer 之后）
        if mark.Flags&markOpener != 0 && mark.Next >= 0 {
            i = mark.Next
        }
        continue
    }
    if !isInMarkChars(mark.Ch, markChars) { continue }
    if mark.Beg < lastEnd { continue }  // 被前一个 span 展开覆盖

    switch mark.Ch {
    case '&':  analyzeEntity(...)
    case '*', '_': analyzeEmph(...)
    case '~':  analyzeTilde(...)
    case '^':  analyzeCaret(...)
    case '$':  analyzeDollar(...)
    case '|':  analyzeSpoiler(...)
    case '=':  analyzeHighlight(...)
    case '[', ']': analyzeBracket(...)
    }
}
```

---

## 7. Rule-of-3 强调匹配算法

这是 md4c/md4go 最精巧的算法，确保 `*`/`_` 强调配对在 **O(n)** 时间内完成，且严格符合 CommonMark 规范。

### 7.1 问题背景

CommonMark 的强调规则有一个反直觉的特性：`***` 可以解析为 `<strong><em>` 或 `<em><strong>` 或 `***`（字面），取决于上下文。关键是 **Rule-of-3**：

> 当一个 mark 的长度 mod 3 == 0 时，它不能与同样 mod 3 == 0 的 mark 配对（除非其中一个是 opener+closer 双重身份）。

这防止了 `***foo***bar***baz***` 被错误地解析为 `***foo` + `***bar` + `***baz`（应该解析为 `***foo***` + `bar` + `***baz***`）。

### 7.2 12 个 opener 栈

为 `*` 和 `_` 各维护 6 个栈，共 12 个：

```
栈索引    字符    类型              MOD3
────────────────────────────────────────
0        *       opener-only       0
1        *       opener-only       1
2        *       opener-only       2
3        *       opener+closer     0
4        *       opener+closer     1
5        *       opener+closer     2
6        _       opener-only       0
7        _       opener-only       1
8        _       opener-only       2
9        _       opener+closer     0
10       _       opener+closer     1
11       _       opener+closer     2
```

另加 7 个扩展栈（索引 12-18）：tilde1/tilde2/bracket/dollar/pipe/caret/equal。

### 7.3 配对搜索策略

当遇到一个 closer 时，按以下优先级搜索 opener：

```
1. OC 栈（opener+closer），MOD3 与 closer 不同
   → 这些 opener 总是允许的，优先选择 MOD3 不同的
2. OO 栈（opener-only）
   → 除非 closer 是 OC 且 MOD3 匹配，否则允许
```

具体代码逻辑：

```go
// 构建允许的 opener 栈列表（最多 6 个）
openerStacks[0] = OC_MOD3_0                        // 总是允许
if closer.MOD3 != 2 { openerStacks[n++] = OC_MOD3_1 }  // MOD3 不同才允许
if closer.MOD3 != 1 { openerStacks[n++] = OC_MOD3_2 }

openerStacks[n++] = OO_MOD3_0                      // 总是允许
if closer.OC == 0 || closer.MOD3 != 2 { openerStacks[n++] = OO_MOD3_1 }
if closer.OC == 0 || closer.MOD3 != 1 { openerStacks[n++] = OO_MOD3_2 }

// 从所有允许的栈中找 End 最大的（最近的）opener
for each allowed stack:
    if stack.top >= 0:
        if marks[stack.top].End > best.End:
            best = marks[stack.top]
```

**关键**：选择 End 最大的 opener（即最接近 closer 的），这保证了"最近匹配"原则。

### 7.4 长度不匹配的分裂

当 opener 和 closer 长度不同时（如 `**foo*`），通过 `splitEmphMark` 分裂较长的一方：

```
Opener ***  +  Closer *  →  Opener **  +  新 mark * (作为 closer)

splitEmphMark(openerIndex, closerSize):
    mark = marks[openerIndex]
    newIndex = openerIndex + (mark.End - mark.Beg - n)
    marks[newIndex] = *mark          // 复制
    mark.End -= n                    // 缩短原 mark
    marks[newIndex].Beg = mark.End   // 新 mark 从原 mark 结束处开始
```

分裂后的新 mark 被推回 opener 栈，留待后续 closer 使用。

### 7.5 其他 mark 类型的配对

| 字符 | 算法 | 特殊规则 |
|---|---|---|
| `~` | `analyzeTilde` | 简单栈配对，1 个 `~` = subscript，2 个 `~~` = strikethrough |
| `^` | `analyzeCaret` | 简单栈配对（superscript） |
| `=` | `analyzeHighlight` | 仅 `==`（长度 2）配对 |
| `\|` | `analyzeSpoiler` | 仅 `\|\|`（长度 2）配对 |
| `$` | `analyzeDollar` | 长度必须匹配（`$` 配 `$`，`$$` 配 `$$`），配对后禁用区间内所有 mark（不嵌套） |

---

## 8. Bracket 链接解析算法

链接和图片解析分两步：`analyzeBracket` 配对 `[`/`]`，`resolveBrackets` 解析配对内容。

### 8.1 Bracket 配对（analyzeBracket）

```
遇到 '[': push 到 bracketStack，标记前一个栈顶为 HASNESTED
遇到 ']': pop bracketStack，与 opener 互连（Next/Prev），
          加入 unresolved_link 链表（通过 opener.Prev 链接）
```

`unresolved_link` 链表按 closer 位置从内到外排序，确保 `resolveBrackets` 从最内层的 bracket 开始处理。

### 8.2 resolveBrackets 从内到外解析

```go
for openerIndex := unresolvedLinkHead; openerIndex >= 0; openerIndex = nextIndex {
    // 1. 嵌套规则检查
    //    - 外部 bracket 不能结束在内部 link 的 (...) 内
    //    - 外部 bracket 不能是 link（如果内部已是 link），但可以是 image

    // 2. 尝试脚注引用 [^label]（优先级最高）
    if resolveBracketFootnote(...) { continue }

    // 3. 尝试 wikilink [[target]]（优先级次高）
    if resolveBracketWikilink(...) { continue }

    // 4. 尝试链接/图片
    if resolveBracketLink(...) {
        // 递归分析链接文本内容（Phase 3）
        analyzeLinkContents(blockText, openerIndex+1, closerIndex)
    }
}
```

### 8.3 链接类型解析（resolveBracketLink）

按优先级尝试四种链接形式：

```
1. 行内链接: [text](url "title")
   → 检测 closer 之后紧跟 '('，解析 destination 和 title

2. 引用链接（完整形式）: [text][ref]
   → 检测 closer 之后紧跟 '['，提取 ref label，查 refDefs map

3. 引用链接（简写形式）: [text]
   → 用 text 本身作为 ref label，查 refDefs map

4. 自动链接: <url>
   → 在 Phase 3 的 '<' mark 处理中完成
```

### 8.4 CANBEIMAGE 扩展

`![alt](url)` 的 `!` 前缀通过 `markBracketCanBeImage` flag 处理。当 `resolveBrackets` 发现 opener 有此 flag 时，将 opener 的 `Ch` 改为 `'!'` 并将 `Beg` 前移一位（吃掉 `!`），从而使该 bracket 对在事件发射时被识别为图片而非链接。

### 8.5 嵌套约束

CommonMark 规定：
- 链接文本内不能再有链接
- 但链接文本内可以有图片

实现方式：`resolveBrackets` 维护 `lastLinkBeg/End` 和 `lastImgBeg/End`。当外部 bracket 的范围包含内部已解析的 link/image 时，根据规则决定是否放弃解析：

```go
if (opener.Beg < lastLinkBeg && closer.End < lastLinkEnd) ||
   (opener.Beg < lastLinkBeg && opener.Ch != '!') {
    // 外部 bracket 在内部 link 内部，或外部 bracket 是 link 但内部已有 link → 放弃
    continue
}
```

---

## 9. 行内事件发射（processInlines）

`processInlines` 遍历已解析的 marks，在 mark 之间发射文本事件，在 resolved mark 处发射 span 进入/离开事件。

### 9.1 遍历逻辑

```go
off := 0
for i := 0; i < len(marks); i++ {
    m := &marks[i]

    // 跳过 dummy mark（splitEmphMark 的产物）
    if m.Ch == 'D' { continue }

    // 跳过被前一个 span 展开覆盖的 mark
    if m.Beg < off { continue }

    // 发射 mark 之前的文本
    if m.Beg > off {
        emitTextWithBreaks(blockText[off:m.Beg])
        off = m.Beg
    }

    if m.Flags & markResolved != 0 {
        switch m.Ch {
        case '*': case '_':  // 发射 Em/Strong span
        case '~':            // 发射 Del/Subscript span
        case '`':            // 发射 Code span
        case '[': case '!':  // 发射 Link/Image span
        case '$':            // 发射 LatexMath span
        case '&':            // 发射 Entity text
        case '<':            // 发射 Autolink/HTML text
        // ...
        }
    } else {
        // 未解析的 mark → 作为字面文本发射
        emitTextWithBreaks(blockText[m.Beg:m.End])
    }
    off = m.End
}
// 发射最后一段文本
if off < len(blockText) {
    emitTextWithBreaks(blockText[off:])
}
```

### 9.2 强调 span 类型推导

一个 `***` mark（长度 3）需要发射 `<em><strong>`（opener）或 `</strong></em>`（closer）。`resolveEmphSpanType` 根据 mark 长度和 opener/closer 身份推导 span 序列：

```
Opener（从内到外）:           Closer（从外到内）:
  len=1 → [Em]                  len=1 → [Em]
  len=2 → [Strong]              len=2 → [Strong]
  len=3 → [Em, Strong]          len=3 → [Strong, Em]
  len=4 → [Strong, Strong]      len=4 → [Strong, Strong]
  len=5 → [Em, Strong, Strong]  len=5 → [Strong, Strong, Em]
```

Opener 先发射 `Em`（奇数长度时），再发射 `Strong` 对；Closer 先发射 `Strong` 对，最后发射 `Em`。这保证了嵌套顺序正确。

### 9.3 换行处理

`emitTextWithBreaks` 将文本按 `\n` 分割，在每行末尾判断换行类型：

```
行末两个空格 + \n  → TextBR（硬换行）
FlagHardSoftBreaks → TextBR（所有软换行变硬换行）
其他               → TextSoftBR（软换行）
```

行尾空格在发射前被剥离（CommonMark 规定行尾空格无意义），但段落中间的行间空格保留（用于硬换行检测）。

---

## 10. 扩展注入机制

### 10.1 Extender 接口

```go
type Extender interface {
    Extend(r Registrar)
}

type Registrar interface {
    RegisterBlockTrigger(c byte, trigger BlockTrigger)
    AddMarkChar(c byte)
    Flags() Flags
    SetFlags(Flags)
}
```

`*Parser` 实现 `Registrar`。扩展在 `New()` 构造时调用 `Extend()`，向 parser 注册自己的语法处理器。

### 10.2 三种注册方式

1. **SetFlags**：设置 flag 位，控制 parser 内部的条件分支（如 `FlagStrikethrough` 使 `~` mark 在 `analyzeTilde` 中被处理）

2. **AddMarkChar**：将字符加入 mark 字符映射，使 `collectMarks` 在扫描时识别该字符为 mark

3. **RegisterBlockTrigger**：注册块级触发器，使 `analyzeLine` 在遇到特定首字符时调用扩展的检测逻辑

### 10.3 扩展示例

```go
// Strikethrough 扩展
func (e *Strikethrough) Extend(r parser.Registrar) {
    r.SetFlags(parser.FlagStrikethrough)  // 使 parser 启用 ~ 处理
    r.AddMarkChar('~')                     // 使 collectMarks 识别 ~ 为 mark
}

// Table 扩展
func (e *Table) Extend(r parser.Registrar) {
    r.SetFlags(parser.FlagTables)
    r.AddMarkChar('|')                     // | 用于单元格边界检测
    // Table 的块级检测在 table.go 内部通过 flag 条件分支实现，
    // 而非通过 RegisterBlockTrigger（因为表格行检测需要段落上下文）
}
```

### 10.4 GFM 预设

```go
var GFM = []parser.Extender{
    &PermissiveAutolinks{},  // URL+Email+WWW 自动链接
    &Table{},                // GFM 表格
    &Strikethrough{},        // ~~删除线~~
    &TaskList{},             // - [x] 任务列表
    &Admonition{},           // > [!NOTE] 告诫块
    &Footnote{},             // [^1] 脚注
}
```

---

## 11. 流式解析与前向引用降级

### 11.1 统一管线

`Convert` 和 `ConvertStream` 共享 `parseLinesInternal` 核心，仅 `LineSource` 不同：

| | SliceSource | ReaderSource |
|---|---|---|
| 输入 | `[]byte` | `io.Reader` |
| 行引用 | 零拷贝（直接引用原切片） | 拷贝（Scanner 缓冲区复用） |
| 内存 | O(n)（持有全文） | O(行)（逐行读取） |
| 前向引用 | 支持（两遍扫描） | 不支持（单遍，先见先得） |

### 11.2 前向引用降级

引用链接定义 `[ref]: url` 可以出现在引用 `[link][ref]` 之后的任意位置。两遍模式下，Pass 1 先收集所有 refdef，Pass 2 完整渲染时所有引用都能解析。

单遍模式下，refdef 在首次遇到时加入 `refDefs` map。如果引用出现在 refdef 之前，map 中尚无对应条目，引用退化为字面文本 `[link][ref]`。

```
流式输入:
  [link][ref]        ← refDefs 中无 "ref"，退化为字面文本
  ...
  [ref]: https://...  ← 此时才加入 refDefs

一次性输入:
  [link][ref]        ← Pass 1 已收集 refdef，Pass 2 正常解析为链接
  ...
  [ref]: https://...
```

### 11.3 blockStack.compact 的作用

流式模式下，已关闭并发射的块的行数据不再需要。`compact()` 在无块打开时清空行切片，防止内存随文档增长：

```go
func (bs *blockStack) compact() {
    if bs.current != nil { return }  // 有块打开时不压缩
    bs.lines = bs.lines[:0]
    bs.vlines = bs.vlines[:0]
    bs.blocks = bs.blocks[:0]
}
```

安全性保证：仅在 `current == nil`（无块打开）时调用。新块的 `LineIdx` 基于当前切片长度，压缩后获得正确的索引。

---

## 附录：关键数据结构速查

### context（解析上下文）

| 字段 | 类型 | 说明 |
|---|---|---|
| `blk` | `blockStack` | 扁平块存储（blocks + lines + vlines） |
| `containers` | `[]Container` | 开放容器栈 |
| `stk` | `markStacks` | mark 切片 + 19 个 opener 栈 |
| `refDefs` | `map[string]*RefDef` | 引用链接定义 |
| `footnoteDefs` | `map[string]*FootnoteDef` | 脚注定义 |
| `htmlBlockType` | `uint8` | 当前 HTML 块类型（1-7，0=不在 HTML 块内） |

### markStacks

| 字段 | 说明 |
|---|---|
| `stacks[19]` | 19 个 opener 栈的栈顶索引（-1=空） |
| `marks []Mark` | 当前块的所有 mark（扁平存储） |
| `unresolvedLinkHead/Tail` | 未解析 bracket 链表头尾 |
| `linkAttrMap` | 已解析链接的 href/title（按 opener 索引） |

### Mark flags

| Flag | 值 | 说明 |
|---|---|---|
| `markPotentialOpener` | 0x01 | 可能是 opener |
| `markPotentialCloser` | 0x02 | 可能是 closer |
| `markOpener` | 0x04 | 已确认为 opener |
| `markCloser` | 0x08 | 已确认为 closer |
| `markResolved` | 0x10 | 已解析（配对/转义/禁用） |
| `markEmphOC` | 0x20 | 强调：opener+closer 双重身份 |
| `markEmphMod3_0/1/2` | 0x40/0x80/0xC0 | 强调：长度 mod 3 |
| `markBracketCanBeImage` | 0x20 | bracket：`!` 前缀 |
| `markBracketFootnote` | 0x80 | bracket：`[^` 脚注引用 |
