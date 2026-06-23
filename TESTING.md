# md4go 测试体系说明

> 本文档说明 md4go 项目的测试方向、功能点、文件组织、如何运行，以及 diffcheck 对拍工具的使用。

---

## 1. 测试方向总览

md4go 的测试体系覆盖以下五大方向，对应 SOP §6.1 测试矩阵：

| # | 测试方向 | 验证目标 | 验收门槛 |
|---|---------|---------|---------|
| 1 | **CommonMark 合规** | 652 条 CommonMark 0.31 spec example 100% 通过 | `TestCommonMarkSpec` |
| 2 | **GFM/扩展合规** | md4c spec-*.txt 全扩展 spec + regressions 100% 通过 | `TestGFMAndExtensionSpec` |
| 3 | **稳定性与健壮性** | 并发安全、空/深层/海量输入无 panic、flag 组合无崩溃 | `TestConcurrent*`, `TestEmpty*`, `TestDeepNesting` |
| 4 | **增量管线一致性** | `Convert`([]byte) vs `ConvertStream`(io.Reader) 输出一致 + 内存有界 | `TestStreamEqualsFull*`, `TestStreamingMemory` |
| 5 | **病态输入线性** | O(n²) 防护、线性时间、内存有界 | `TestPathological*`, `BenchmarkPathological` |
| 6 | **Fuzz 安全** | 任意字节流无 panic、双渲染器确定性 | `FuzzParseNoPanicNoLeak` 等 |
| 7 | **兼容性 Flag** | 25 个 flag 正交性、Dialect 预设组合、Flag 值唯一性 | `compat_flags_test.go` |
| 8 | **回归钉死** | I45–I62 修复的特定 bug 不复发 | `regression_*.go` |
| 9 | **扩展集成** | `WithExtensions`/`WithFlags` API 正确组合 | `integration/extension_integration_test.go` |

---

## 2. 测试文件组织

### 2.1 测试分层架构

测试按 **被测模块** 和 **测试层级** 归属到各自目录：

- **单元测试**：与被测代码同目录（`parser/`、`html/`、`text/`、`extension/`、`stream/`），直接测试模块内部逻辑
- **集成测试**：统一放在 `integration/` 目录（`package integration_test`），通过导入 `md4go` 及子包测试全链路行为（含 `WithExtensions`/`WithFlags` 选项 API，经公共 API 行为断言）

```
md4go/
├── integration/                    # ══ 集成测试 (package integration_test) ══
│   ├── spec_test.go                #   CommonMark 0.31 spec 合规 (652 example)
│   ├── extension_spec_test.go      #   GFM/扩展 spec 合规 (spec-*.txt + regressions)
│   ├── extension_integration_test.go # WithExtensions/WithFlags API 组合 (公共 API 行为断言)
│   ├── pathological_test.go        #   病态输入: 无 panic + 线性时间 + 内存有界
│   ├── fuzz_test.go                #   Fuzz: 无 panic + 双渲染器确定性
│   ├── concurrency_test.go         #   并发安全: 多 goroutine + HTML/Plain 混合
│   ├── edgecase_test.go            #   健壮性: 空/海量输入 + 渲染器接口合规
│   └── testhelpers_test.go         #   共享辅助 (mdConvertPlain/HTML, normalizeHTML)
│
├── parser/                         # ══ parser 单元测试 (package parser_test) ══
│   ├── compat_flags_test.go        #   兼容性 Flag: 正交性 + 预设组合 + 值唯一
│   ├── regression_test.go          #   解析器逻辑 bug 钉死 (I45–I62)
│   ├── edgecase_test.go            #   深层嵌套 + flag 组合无 panic
│   ├── flags_test.go               #   单个 flag 行为验证
│   ├── parser_test.go              #   解析器核心逻辑
│   ├── emphasis_test.go            #   Rule-of-3 强调解析
│   ├── mark_test.go                #   Mark 系统
│   ├── link_test.go                #   链接/图片解析
│   └── ...
│
├── html/                           # ══ html 单元测试 (package html_test) ══
│   ├── regression_test.go          #   HTML 渲染/实体处理 bug 钉死 (I45–I62)
│   ├── edgecase_test.go            #   HTML 渲染器边界用例
│   └── render_flags_test.go        #   HTML 渲染器 flag
│
├── text/                           # ══ text 单元测试 (package text_test) ══
│   ├── streaming_test.go           #   流式 vs 全量一致性 + 内存有界
│   └── edgecase_test.go            #   流式一致性 + tight list 渲染
│
├── extension/*_test.go             # [extension] 扩展语法单元测试
├── stream/*_test.go                # [stream] LineSource 单元测试
└── diffcheck/                      # 独立对拍工具 (三方引擎 diff)
```

### 2.2 测试归属原则

| 目录 | 包名 | 测试层级 | 测试方向 |
|------|------|---------|---------|
| **`integration/`** | `integration_test` | 集成 | spec 合规、扩展 API 组合、病态输入、Fuzz、并发安全、全链路边界 |
| **`parser/`** | `parser_test` | 单元 | flag 行为/正交性、bug 回归、深层嵌套、强调/链接/Mark |
| **`html/`** | `html_test` | 单元 | 实体处理、嵌套图片、highlight/spoiler 守卫、渲染 flag |
| **`text/`** | `text_test` | 单元 | 流式一致性、内存有界、tight list 渲染 |
| **`extension/`** | `extension` | 单元 | Extender 接口 |
| **`stream/`** | `stream` | 单元 | SliceSource/ReaderSource |

> SOP §1.4 约束：文件行数 ≤ 800 行。所有文件均满足（`integration/` 最大 599 行）。

---

## 3. 功能点覆盖

### 3.1 CommonMark 合规 (`integration/spec_test.go`)

- `TestCommonMarkSpec` — 652 条 CommonMark 0.31 spec example 全量对拍（主合规门禁）
- `TestBlockLevelSpec` — 块级 section 增量对拍（M2 调试用）
- `TestInlineSpec` — 行内 section 增量对拍（M3 调试用）
- HTML 归一化辅助函数（`normalizeHTML`, `parseHTMLTag`, `decodeHTMLEntities` 等）

### 3.2 扩展 spec 合规 (`integration/extension_spec_test.go`)

- `TestGFMAndExtensionSpec` — 运行所有 `testdata/spec/spec-*.txt`
- `TestRegressions` — 运行 `testdata/spec/regressions.txt`
- `buildConfigFromOptions` — md4c CLI flag → Go parser flags + extenders 映射

### 3.3 增量管线 (`text/streaming_test.go`)

- `TestStreamEqualsFull` — 33 种输入 `Convert` vs `ConvertStream` PlainText 输出一致
- `TestStreamEqualsFull_HTML` — 9 种输入 `Parse` vs `ParseStream` HTML 输出一致
- `TestStreamingMemory` — 100 万行文档流式解析 HeapInuse < 256MB
- `TestStreamingDoesNotBufferAll` — 验证增量处理（非全量缓冲）
- `TestConvertVsConvertStream_ForwardRefdef` — 前向引用降级行为验证

### 3.4 病态输入 (`integration/pathological_test.go`)

- `TestPathologicalNoPanic` — 26 种病态输入无 panic（30s 超时检测 O(n²)）
- `TestPathologicalOutput` — 26 种输出匹配正则模式（对齐 md4c behavior，无 skip）
- `BenchmarkPathological` — 性能基准
- `TestPathologicalLinear` — 线性时间验证（500 vs 2000 规模，比率 ≤ 2.5× 线性比）
- `TestPathologicalMemory` — 内存有界验证（HeapInuse < 256MB）
- `TestPathologicalStreamEqualsFull` — 病态输入流式 vs 全量一致
- `TestPathologicalWithExtensions` — GFM 扩展模式病态输入

### 3.5 Fuzz 测试 (`integration/fuzz_test.go`)

- `FuzzParseNoPanicNoLeak` — 任意字节流 → HTML/Plain/GFM/Streaming 四路无 panic
- `FuzzParseWithFlags` — 7 种 flag 组合下无 panic
- `FuzzTightLooseListSeparator` — I62 tight/loose 修复的定向 fuzz（确定性 + 无多余空行）
- `FuzzTightLooseListDualRender` — 跨渲染器确定性验证

### 3.6 兼容性 Flag (`parser/compat_flags_test.go`)

- 行为修改 flag 正交性（CollapseWhitespace, PermissiveATX, NoIndentedCode, HardSoftBreaks, Underline 等）
- 扩展语法 flag 正交性（Tables, Strikethrough, Tasklists, Footnotes, Highlight, Spoilers 等）
- Dialect 预设组合验证（DialectCommonMark=0, DialectGitHub 组成, GoldmarkCompat 组成）
- Flag 值唯一性（25 个 flag 均为不重叠的 2 的幂）
- S-01~S-06 默认行为验证（ProtectDoublePipe, tight list 分隔符, footnote ref, NULL code span 等）

### 3.7 回归测试 (`parser/regression_test.go` + `html/regression_test.go`)

| 测试 | 钉死的 Bug | 迭代 |
|------|-----------|------|
| `TestTableFollowedByList` | 表格续行 pivot 变量 bug | I56 |
| `TestTableContinuationAfterContainer` | 容器标记后表格续行误判 | I56 |
| `TestLongEmphasisRun` | markMaxRunLen=16 移除 | I56 |
| `TestSpoilerProtectionInTableCells` | `\|\|` spoiler 保护 | I57 |
| `TestSpoilerProtectionInTableCellsRoundTrip` | FlagProtectDoublePipe 行为 | I60 |
| `TestFencedCodeInfoStringTab` | fence info string tab 对齐 | I57 |
| `TestNextCloserNilSafety` | resolveBracketLink 空指针守卫 | I58 |
| `TestFootnoteReferenceNumber` | footnote ref [N] 输出 (S-05) | I59 |
| `TestTightListParagraphSeparator` | tight list 段落分隔符 (S-02) | I60 |
| `TestHTMLNestedImageSuppression` | 嵌套图片抑制 (image_nesting_level) | I57 |
| `TestEmptyLinkDestination` | 空链接目标 href="" | I58 |
| `TestHighlightLengthGuard` | `==` vs `=` 长度守卫 | I58 |
| `TestSpoilerLengthGuard` | `\|\|` vs `\|` 长度守卫 | I58 |
| `TestNullCharInCodeSpan` | NULL → U+FFFD (S-06) | I59 |
| `TestHTMLEntityCodepointZero` | `&#0;` → U+FFFD | I59 |
| `TestHTMLEntitySurrogatePair` | 代理对 → U+FFFD | I59 |
| `TestHTMLInvalidNumericEntity` | 无效数字实体原样输出 | I59 |
| `TestWikilinkExtension` | wikilink 扩展支持 (S-03) | I59 |
| `TestPlainTextNestedTightList` | tightListDepth 栈修复 | I62 |

---

## 4. 如何运行

### 4.1 全量测试

```bash
go test ./...
```

### 4.2 按方向运行

```bash
# CommonMark 合规 (652 example) — 集成测试
go test ./integration/ -run TestCommonMarkSpec -v

# GFM/扩展 spec — 集成测试
go test ./integration/ -run TestGFMAndExtensionSpec -v

# 回归测试 — parser + html 单元测试
go test ./parser/ -run "TestTable|TestLong|TestSpoiler|TestFenced|TestNext|TestFootnote|TestTight" -v
go test ./html/ -run "TestHTML|TestHighlight|TestSpoiler|TestNull|TestWiki|TestEmpty" -v

# 并发安全 — 集成测试
go test ./integration/ -run TestConcurrent -v -race

# 流式管线 — text 包
go test ./text/ -run "TestStream|TestStreaming|TestConvert" -v

# 兼容性 Flag — parser 包
go test ./parser/ -run "TestFlag|TestDialect|TestGoldmark|TestCompat|TestPermissive|TestNoHTML|TestDefault" -v

# 病态输入 — 集成测试
go test ./integration/ -run TestPathological -v -timeout 120s
go test ./integration/ -bench BenchmarkPathological -benchmem

# Fuzz (短时) — 集成测试
go test ./integration/ -fuzz=FuzzParseNoPanicNoLeak -fuzztime=5m
```

### 4.3 质量门禁

```bash
go build ./...       # 0 error
go test ./...        # 全 pass
go vet ./...         # 0 output
gofmt -l .           # 0 output (排除 vendored 模块)
```

### 4.4 Race Detector

```bash
go test -race ./...
```

### 4.5 短模式（跳过慢测试）

```bash
go test -short ./...
```

---

## 5. diffcheck 对拍工具

`diffcheck/` 是独立的 Go 模块（有自己的 `go.mod`），用于三方引擎（md4go / md4c / goldmark）的 Markdown→纯文本 diff 对比。

详见 [`diffcheck/README.md`](diffcheck/README.md)。

### 5.1 构建

```bash
# 编译 md4c-plain C 二进制（首次运行前执行一次）
cd diffcheck/csrc
gcc -O2 -I../../md4c/src -o md4c-plain main.c ../../md4c/src/md4c.c

# 构建 Go CLI
cd diffcheck
go build -o diffcheck ./cmd/diffcheck
```

### 5.2 运行

```bash
cd diffcheck

# 内置数据源
go run ./cmd/diffcheck --src=fuzz         # fuzz 种子 (175 条)
go run ./cmd/diffcheck --src=constructed  # 构造用例 (141 条)
go run ./cmd/diffcheck --src=jsonl        # JSONL 数据 (10350 条)
go run ./cmd/diffcheck --src=all          # 全部

# 用户数据
go run ./cmd/diffcheck --file=path/to/markdown.md    # 单个文件
echo "# Hello" | go run ./cmd/diffcheck --stdin      # stdin 输入

# 选项
go run ./cmd/diffcheck --normalize=strict   # strict 归一化
go run ./cmd/diffcheck --dialect=commonmark # CommonMark-only 模式
go run ./cmd/diffcheck --compat=goldmark    # GoldmarkCompat flag
go run ./cmd/diffcheck --output=report.txt  # 报告写入文件
go run ./cmd/diffcheck -v                   # 显示一致用例
```

### 5.3 go test 集成

```bash
cd diffcheck
go test -run TestDiffCheckFuzzMd4cAlignment -v       # fuzz 对拍
go test -run TestDiffCheckConstructedMd4cAlignment   # 构造用例对拍
go test -run TestDiffCheckCommonMarkAlignment -v     # CommonMark 对拍
go test -run TestDiffCheckJSONLAlignment -v          # JSONL 全量对拍 (较慢)
go test -short                                       # short 模式跳过 JSONL
```

---

## 6. 测试数据

| 数据 | 路径 | 说明 |
|------|------|------|
| CommonMark spec | `testdata/spec/commonmark-0.31.json` | 652 条 example (源自 goldmark) |
| 扩展 spec | `testdata/spec/spec-*.txt` | md4c spec 文件 (16 个) |
| 回归用例 | `testdata/spec/regressions.txt` | md4c 回归测试集 |
| Fuzz 种子 | `testdata/fuzz/` | fuzz 语料缓存 |
| JSONL 数据 | `diffcheck/data/testdata*.jsonl` | 真实文档语料 (10350 条) |

---

## 7. 已知差异 (S-cases)

md4go 在以下场景比 md4c 更正确（有意为之，非 bug）：

| 编号 | 差异 | 说明 |
|------|------|------|
| S-01 | `\|\|` 表格保护 | FlagProtectDoublePipe 保护 `\|\|` 不被分割 |
| S-02 | tight list 段落分隔符 | md4go 保留段落边界，md4c 直接拼接 |
| S-03 | Wikilink 支持 | md4go 支持 `[[target\|label]]`，md4c 不支持 |
| S-04 | tight list 段落分隔 | 同 S-02，plaintext 场景 |
| S-05 | footnote ref [N] 输出 | md4go 输出 [N]，md4c 省略 |
| S-06 | NULL code span 识别 | md4go 识别含 NULL 的 code span 并替换为 U+FFFD |

---

_本文档随测试体系演进更新。最后更新：I63 全量对拍验证 + 文档校正。_
