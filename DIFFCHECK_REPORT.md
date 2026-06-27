# md4go Diffcheck 差异根因分析报告

> **数据集**: fuzz 175 + constructed 141 + JSONL 10,349 = **10,665** 条
> **配置**: Part 1: DialectGitHub (no compat)。Part 2: DialectGitHub + `GoldmarkCompat`
> **日期**: 2026-06-27（刷新）
> **工具**: `diffcheck/` text + goquery 对拍 + HTML 字节级对比

---

## Part 1: md4go vs md4c

### 1.1 Text 直接对比 (plain-text renderer → loose normalize)

| 指标 | 数值 |
|---|---|
| Total | 10,665 |
| Diffs | **57** |
| Match | **99.5%** |

**57 条差异分类：**

| 类别 | 数量 | 根因 | 案例 |
|---|---|---|---|
| **tight-list 分隔** | ~44 | md4go 在 tight list items 间加空格；md4c 直接拼接 | `- one\n  - nested` → `one nested` vs `onenested` |
| **footnote 渲染** | ~4 | footnote ref 文本输出差异 | `[^1]` → `[1]` vs 不渲染 |
| **backtick 上限** | 1 | md4go max=1024 (CommonMark)；md4c max=32 (C 约束) | 33+ backtick code span 识别差异 |
| **NULL→U+FFFD** | 1 | md4go 替换 U+FFFD；md4c 跳过 | `` `\x00` `` → `�` vs 空白 |
| **其他** | ~7 | 空白/换行微小差异 | 非语义 |

### 1.2 HTML 字节级直接对比

| 指标 | 数值 |
|---|---|
| Total | 10,665 |
| Raw diffs | **1,071** |
| 实质差异 | **<20** |
| 结构一致率 | **>99.8%** |

**1,071 raw diffs 深度分析（逐字节对比 + 逐案 structual classify）：**

| 类别 | 数量 | 占比 | 结论 |
|---|---|---|---|
| **whitespace_only** | 1,051 | 98.1% | normalize 函数不处理 tag 内空格导致的 false positive；逐案验证两者 HTML **结构+文本完全一致** |
| **NULL→U+FFFD** | 1 | 0.1% | md4go 有意改进（`\x00`→`&#xfffd;`，md4c skip） |
| **backtick 上限** | 1 | 0.1% | md4go: 1024 (CommonMark)，md4c: 32 (C 约束) |
| **text_content** | 18 | 1.7% | 极端嵌套 / `\|` 语义 / setext-table 优先级微小差异 |

> **结论**: md4go 与 md4c 的 HTML 输出在结构与语义层面一致率 >99.8%。1,071 raw diffs 中 98% 是 normalize 函数在比较 tag 间空白时的误报（如 `<h2>text </h2>` vs `<h2>text</h2>`）。**无需修改渲染逻辑**——md4go 实现合理正确，不破坏 Markdown 标准。

### 1.3 HTML→goquery 同管道对比 (纯解析器差异)

| 指标 | 数值 |
|---|---|
| Total | 10,665 |
| Diffs | **26** |
| Match | **99.75%** |

> goquery 管道消除了 table 渲染中的 tags 差异，仅暴露语义层面分歧（与 text 管道重叠）。

---

## Part 2: md4go vs goldmark

> ⚠️ md4go 默认对齐 md4c，不使用 raw default 对比 goldmark。**本节全部使用 `GoldmarkCompat` flag**，验证 md4go 开启兼容后的最大对齐能力。
> `GoldmarkCompat` = FlagTableInterruptParagraph | FlagDecodeEntities | FlagStripBOM | FlagStrikethroughPermissive | FlagStripHTMLTags | FlagStrictTableColumns | FlagTableInterruptByHeaders | FlagNoXHTMLEntityEncoding

### 2.1 Text 直接对比 (md4go text vs goldmark→goquery text)

| 指标 | 数值 | vs 无 compat |
|---|---|---|
| Total | 10,665 | — |
| Diffs | **2,680** | ↓ 3,822 |
| Match | **74.3%** | ↑ 36.8pp |

**差异根因：**

| 类别 | 数量 | 根因 |
|---|---|---|
| **goquery 管道噪声** | ~1,620 | goldmark DOM traversal 在节点边界插入空格；md4go text 直渲无此行为 — **非解析器差异，管线架构不同所致** |
| **块级状态机** | ~640 | goldmark ParagraphTransformer (两阶段) vs md4go analyzeLine (单 pass) — 容器嵌套+段落跨结构场景 |
| **表格行为** | ~270 | 列数校验、表格中断、`\|` 处理残余差异（已有 FlagStrictTableColumns 等大幅消除） |
| **行内边界** | ~51 | 行尾空格、句点后空格等 |
| **扩展集** | ~8 | admonition `> [!NOTE]`、footnote `[^1]` — md4go 支持，goldmark 不支持 |
| **HTML block + 图片 alt** | ~210 | goldmark safe 丢弃 HTML block；goquery 不输出 img alt |

> **text 管线差异 60% 是管线架构噪声**（goquery 空格插入），非解析器层面。

### 2.2 HTML→goquery 同管道对比 (纯解析器差异)

| 指标 | 数值 | vs 无 compat |
|---|---|---|
| Total | 10,665 | — |
| Diffs | **1,057** | ↓ 3,985 |
| Match | **89.8%** | ↑ 38.2pp |

> goquery 统一管道消除了 ~1,620 条架构噪声。**1057 条差异是 md4go（GoldmarkCompat）与 goldmark 的纯解析器层面分歧。**

**1057 条差异根因：**

| 类别 | 数量 | 根因 |
|---|---|---|
| **块级状态机** | ~640 | goldmark 两阶段 vs md4go 单 pass — 最大残余差异 |
| **表格行为** | ~270 | 列数校验、表格中断 |
| **行内边界** | ~51 | 行尾空格、句点后空格 |
| **扩展集** | ~8 | Admonition/Footnote |
| **其他** | ~88 | HTML block、entity 等边界 |

**代表性案例：**

```
∘ Admonition (扩展集，不可消除):
  INPUT:    > [!NOTE]\n> note content
  md4go:    note content                       ← 识别为 admonition
  goldmark: [!NOTE] content                    ← 当作普通引用文本
  
∘ Table (GoldmarkCompat 已大幅消除):
  通过 FlagStrictTableColumns + FlagTableInterruptParagraph +
  FlagTableInterruptByHeaders 三个 flag 消灭了 ~600 条表格差异。
  残余 ~270 条来自极端 column count 和 | 语义边界。
  
∘ 块级状态机 (最大残余，占 60%):
  单 pass analyzeLine vs 两阶段 ParagraphTransformer 的架构级差异，
  在容器嵌套+段落跨结构场景触发。需要 ParagraphTransformer 模式（可选启用）。
```

---

## 综合结论

| 对比维度 | md4go vs md4c (default) | md4go vs goldmark (GoldmarkCompat) |
|---|---|---|
| **Text 语义对齐** | 99.5% (57/10,665) | 74.3% (2,680/10,665) |
| **HTML 结构对齐** | >99.8% (<20/10,665 real) | — |
| **HTML→goquery 对齐** | 99.75% (26/10,665) | 89.8% (1,057/10,665) |
| **核心差异** | cosmetic + table tags | 块级状态机 (60%) + 表格 (25%) + 管线噪声 |

### GoldmarkCompat 效果

| Flag | 消除差异 | 类别 |
|---|---|---|
| FlagStrictTableColumns + FlagTableInterruptParagraph + FlagTableInterruptByHeaders | ~600 | 表格行为 |
| FlagDecodeEntities + FlagStripHTMLTags + FlagStripBOM | ~210 | HTML block + entity |
| FlagStrikethroughPermissive | — | ~~ flanking |
| FlagNoXHTMLEntityEncoding | — | XHTML 编码 |

### 设计原则

- **默认 (flags=0)** = CommonMark / GFM 标准 → 与 md4c 天然对齐 (99.5%)
- **`GoldmarkCompat`** = opt-in 一揽子兼容 flag → 与 goldmark 对齐至 89.8%
