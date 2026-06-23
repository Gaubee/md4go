---
name: compatibility-flags-alignment
overview: 基于DIFF_REPORT的top差异分析，新增模块化可插拔的兼容性flag体系。默认行为保持markdown标准合规（md4go的改进行为），通过独立flag开关让用户可选opt-in到md4c或goldmark的兼容行为。覆盖S-01(`||`单元格保护)、S-02/S-04(紧凑列表分隔)、S-05(脚注引用编号)、S-06(NULL代码跨度)、goldmark维度B(表格列数校验)、HTML实体解码、CDATA识别共7个兼容开关。
todos:
  - id: add-compat-flags
    content: 在 parser/flags.go 中新增 7 个 compat flag 常量和 Md4cCompat/GoldmarkCompat 便利组合，每个 flag 附详细注释
    status: completed
  - id: parser-level-compat
    content: 实现 parser 级 compat：table.go skipSpoilers gating(S-01)、mark.go NULL code span gating(S-06)、block.go 列数校验(维度B)、html_block.go CDATA 识别
    status: completed
    dependencies:
      - add-compat-flags
  - id: renderer-level-compat
    content: 实现 renderer 级 compat：plaintext.go 添加 flags 字段及 3 处 gating(S-02/S-04,S-05,实体解码)，text.go 传递 flags 到 PlainText
    status: completed
    dependencies:
      - add-compat-flags
  - id: compat-tests
    content: Use [skill:test-driven-development] 编写 compat_flags_test.go，覆盖每个 flag 正向/反向用例和组合测试
    status: completed
    dependencies:
      - parser-level-compat
      - renderer-level-compat
  - id: docs-and-cli
    content: 更新 README.md/PROGRESS.md/docs 新增 compat flag 文档，cmd/md4go/main.go 新增 -compat CLI 选项，Use [skill:verification-before-completion] 运行全量测试验证无回归
    status: completed
    dependencies:
      - compat-tests
---

## 用户需求

分析 DIFF_REPORT.md 中的 top 差异，在**默认行为符合 Markdown 标准**的前提下，通过**清晰合理易理解的 flag 选项开关**提供兼容性能力对齐。要求：

1. **Flag 按具体特性设计**——每个 flag 控制一个具体行为特征，而非按目标库命名
2. **通过组合提供兼容性**——`Md4cCompat`/`GoldmarkCompat` 是具体 flag 的组合，不作为主接口
3. **Flag 管理统一清晰**——集中定义，分类组织
4. **实现模块化可插拔**——compat 逻辑封装在独立模块，最小化对主流程复杂度的影响
5. **允许重构设计**

## 产品概述

为 md4go 新增 6 个**行为特性 flag**。默认（unset）= GFM/CommonMark 标准合规行为；set = opt-in 到非标准行为（md4c 兼容或 md4go 改进）。compat 逻辑封装在 `parser/compat.go` 和 `text/compat.go` 两个集中模块中，主流程通过 compat helper 函数调用。

**关键修正（审查确认）**：

- S-01: md4go 的 `||` 保护是**非标准改进**（GFM 规范中 `|` 是单元格分隔符）。默认改为拆分 `||`（GFM 标准=对齐 md4c），flag opt-in 保护。
- 维度B: md4go 的宽松列数校验是**非标准**（GFM 规范要求列数匹配）。默认改为严格校验（GFM 标准=对齐 goldmark），flag opt-in 宽松。
- 这两个修正使默认行为直接消除 21+44=65 条 diff，无需用户设置任何 flag。

> **CDATA 已排除**：代码审查确认 `parser/html_block.go:88-91` 已将 `<![CDATA[` 检测为 htmlBlockType5，DIFF_REPORT §5.5 的 7 条差异来自 goldmark 的 CDATA 内容处理方式不同，不是识别问题，无需新增 flag。

## Flag 设计（6 个行为特性 flag）

### 命名原则

Flag 名描述 **set 时的行为**。按功能域分类：

```
// === 兼容性 Flag ===
//
// 每个 flag 控制一个具体行为特性。默认（unset）= GFM/CommonMark 标准合规；
// set = opt-in 到非标准行为（md4c 兼容或 md4go 改进）。
// 不包含在任何 Dialect 预设中。

// --- 表格特性 ---
// FlagProtectDoublePipe: set 时保护 || 不被拆分为单元格边界（md4go 改进）。
// 默认: 按 GFM 标准拆分 || 为单元格边界（对齐 md4c）。
// 覆盖: S-01 (21条)，默认即消除差异。
FlagProtectDoublePipe Flags = 0x800000

// FlagLenientTableColumns: set 时宽松校验表格列数（md4go 行为）。
// 默认: 按 GFM 标准严格要求列数匹配（对齐 goldmark）。
// 覆盖: 维度B (44条)，默认即消除差异。
FlagLenientTableColumns Flags = 0x1000000

// --- 列表特性 ---
// FlagConcatTightList: set 时紧凑列表无分隔拼接（md4c 行为）。
// 默认: 保留词界分隔（更正确）。
// 覆盖: S-02/S-04 (37条)。
FlagConcatTightList Flags = 0x2000000

// --- 脚注特性 ---
// FlagDropFootnoteRef: set 时丢弃脚注引用编号（md4c 行为）。
// 默认: 输出 [N]（更正确）。
// 覆盖: S-05 (1条)。
FlagDropFootnoteRef Flags = 0x4000000

// --- 代码跨度特性 ---
// FlagSkipNullCodeSpan: set 时不识别含NULL代码跨度（md4c 行为）。
// 默认: 识别并替换NULL为U+FFFD（CommonMark 标准）。
// 覆盖: S-06 (1条)。
FlagSkipNullCodeSpan Flags = 0x8000000

// --- 实体特性 ---
// FlagDecodeEntities: set 时解码HTML实体为Unicode（goldmark 行为）。
// 默认: 保留实体原始文本。
// 覆盖: ~620条。
FlagDecodeEntities Flags = 0x10000000

// --- 文档化组合（非主接口，仅便利） ---
// Md4cCompat: 对齐 md4c 行为。S-01 默认已对齐，仅需 opt-in md4c 非标准行为。
Md4cCompat = FlagConcatTightList | FlagDropFootnoteRef | FlagSkipNullCodeSpan
// GoldmarkCompat: 对齐 goldmark 行为。维度B 默认已对齐，仅需 opt-in goldmark 非标准行为。
GoldmarkCompat = FlagTableInterruptParagraph | FlagDecodeEntities
```

## 架构设计：Compat 模块模式

### 核心原则

1. **主流程不内联 flag 检查**——通过 compat helper 函数调用
2. **compat 逻辑集中封装**——每个包一个 `compat.go` 文件
3. **CompatConfig 预计算**——避免热路径重复 bitmask 检查
4. **helper 函数自文档化**——函数名描述行为

### 模块结构

```
parser/
├── flags.go          ← 新增 6 个 flag 常量 + 文档化组合
├── compat.go         ← [新增] parser 级 compatConfig + helper 函数
├── table.go          ← splitTableCells 接受 compatConfig，调用 helper
├── mark.go           ← collectCodeSpanMark 接受 compatConfig，调用 helper
├── block.go          ← analyzeLine step 12 调用列数校验 helper
└── ...               ← 其他文件不变

text/
├── compat.go         ← [新增] renderer 级 compatConfig + helper 函数
├── plaintext.go      ← PlainText 添加 compat 字段，调用 helper
├── text.go           ← Convert 传递 flags 到 PlainText
└── ...
```

### parser/compat.go（新增）

```
package parser

// compatConfig 预计算 parser 级兼容性行为决策。
type compatConfig struct {
    protectDoublePipe  bool // FlagProtectDoublePipe: 保护 || 不被拆分
    lenientTableCols   bool // FlagLenientTableColumns: 宽松列数校验
    skipNullCodeSpan   bool // FlagSkipNullCodeSpan: 跳过含 NULL 的代码跨度
}

func newCompatConfig(flags Flags) compatConfig {
    return compatConfig{
        protectDoublePipe: flags&FlagProtectDoublePipe != 0,
        lenientTableCols:  flags&FlagLenientTableColumns != 0,
        skipNullCodeSpan:  flags&FlagSkipNullCodeSpan != 0,
    }
}

// protectDoublePipeInCells 保护 || 不被拆分为单元格边界。
// protect=true 时保护（md4go 改进），false 时按 GFM 标准拆分。
func protectDoublePipeInCells(line []byte, protected []bool, protect bool) {
    if !protect {
        return // GFM 标准: 不保护，|| 被拆分为单元格边界
    }
    skipSpoilers(line, protected) // md4go 改进: 保护 ||
}

// shouldBreakOnNull 返回遇到 NULL 时是否终止 backtick 扫描。
// skip=true 时终止（md4c 兼容），false 时继续（CommonMark 标准）。
func shouldBreakOnNull(skip bool) bool {
    return skip
}

// validateTableColumns 校验标题行与分隔行列数是否匹配。
// lenient=true 时跳过校验（md4go 行为），false 时严格匹配（GFM 标准）。
func validateTableColumns(headerLine []byte, underlineCols int, lenient bool) bool {
    if lenient {
        return true
    }
    headerCols := len(splitTableCellsRaw(headerLine))
    return headerCols == underlineCols
}
```

### text/compat.go（新增）

```
package text

import "md4go/parser"
import stdhtml "html"

type compatConfig struct {
    concatTightList bool
    dropFootnoteRef bool
    decodeEntities  bool
}

func newCompatConfig(flags parser.Flags) compatConfig {
    return compatConfig{
        concatTightList: flags&parser.FlagConcatTightList != 0,
        dropFootnoteRef: flags&parser.FlagDropFootnoteRef != 0,
        decodeEntities:  flags&parser.FlagDecodeEntities != 0,
    }
}

func shouldEmitTightListSeparator(cc compatConfig) bool {
    return !cc.concatTightList
}

func shouldEmitFootnoteRef(cc compatConfig) bool {
    return !cc.dropFootnoteRef
}

func decodeEntity(text []byte, decode bool) []byte {
    if !decode {
        return text
    }
    return []byte(stdhtml.UnescapeString(string(text)))
}
```

## 主流程接入点

### S-01: parser/table.go — splitTableCells

```
// 修改后: splitTableCells 接受 cc，默认(protect=false)不保护 ||
func splitTableCells(line []byte, cc compatConfig) [][]byte {
    ...
    protectDoublePipeInCells(line, protected, cc.protectDoublePipe)
    ...
}
```

### S-06: parser/mark.go — collectCodeSpanMark

```
// 修改后: 接受 cc，skipNullCodeSpan=true 时遇到 NULL 终止扫描
func collectCodeSpanMark(ms *markStacks, text []byte, off int, cc compatConfig) int {
    ...
    if shouldBreakOnNull(cc.skipNullCodeSpan) && off+actualLen < len(text) && text[off+actualLen] == 0 {
        return off + actualLen
    }
    ...
}
```

### 维度B: parser/block.go — analyzeLine step 12

```
// 修改后: lenient=false(默认)时严格校验列数
if colCount, ok := isTableUnderline(line[off:]); ok {
    headerLine := ctx.blk.lines[ctx.blk.current.LineIdx + ctx.blk.current.NLines - 1].Text
    if validateTableColumns(headerLine, colCount, p.compat.lenientTableCols) {
        la.lineType = LineTableUnderline
        ...
    }
}
```

### S-02/S-04, S-05, 实体解码: text/plaintext.go

```
// BlockP: shouldEmitTightListSeparator(pt.compat)
// EnterSpan: shouldEmitFootnoteRef(pt.compat)
// Text: decodeEntity(text, pt.compat.decodeEntities)
```

### Parser/Renderer 初始化

- parser/parser.go: Parser 添加 compat 字段
- text/text.go: Convert 传递 flags 到 PlainText
- text/plaintext.go: PlainText 添加 compat 字段 + NewPlainTextWithFlags

## 实现计划

### 1. Flag 定义 (parser/flags.go)

### 2. Parser 级 compat (parser/compat.go + 接入)

### 3. Renderer 级 compat (text/compat.go + 接入)

### 4. 测试

### 5. 文档 + CLI