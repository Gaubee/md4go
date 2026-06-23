---
name: diff-test-suite
overview: 在 tools/ 目录下构建基于 goldmark、md4c、本项目（md2text）三个 Markdown 实现的 plaintext 对拍测试体系，含 JSONL 和 fuzz 输入、超时保护、4目录分离架构。
todos:
  - id: init-diffcheck-module
    content: 创建 tools/diffcheck/ 目录、go.mod、Engine 接口和超时执行框架
    status: completed
  - id: impl-md2text-engine
    content: 实现 impl_md2text/engine.go：md2text 转换引擎
    status: completed
    dependencies:
      - init-diffcheck-module
  - id: impl-md4c-engine
    content: 实现 impl_md4c/engine.go：md4c-plain 子进程调用引擎
    status: completed
    dependencies:
      - init-diffcheck-module
  - id: impl-goldmark-engine
    content: 实现 impl_goldmark/engine.go：移植 strip.go 的 goldmark→HTML→goquery 方式
    status: completed
    dependencies:
      - init-diffcheck-module
  - id: impl-diff-logic
    content: 实现 diff/ 目录：归一化 normalizer + 逐行 differ + 报告 report
    status: completed
    dependencies:
      - init-diffcheck-module
  - id: impl-loader
    content: 实现 loader.go：JSONL 解析 + fuzz 种子加载
    status: completed
    dependencies:
      - init-diffcheck-module
  - id: impl-main-cli
    content: 实现 main.go：CLI 入口整合所有模块
    status: completed
    dependencies:
      - impl-md2text-engine
      - impl-md4c-engine
      - impl-goldmark-engine
      - impl-diff-logic
      - impl-loader
  - id: impl-test-integration
    content: 实现 diffcheck_test.go：go test 集成对拍测试
    status: completed
    dependencies:
      - impl-main-cli
---

## 产品概述

在 tools/ 目录下构建三方可拍测试体系，对 goldmark、md4c、本项目 md2text 三个 Markdown 解析实现的 plaintext 输出进行交叉验证，并输出辅助定位的 diff 信息。

## 核心功能

- **测试用例加载**：从 `testdata.jsonl` 加载 JSONL 文件输入（1001条），提取每行 JSON 的 `data.content` 字段作为 markdown 输入；从 fuzz 种子生成合法与不合法 markdown 输入
- **三方引擎调用**：调用 md2text（Go API 直接调用）、md4c-plain（通过 os/exec 调用 C 二进制）、goldmark（复用 `common_test/markdown/strip.go` 的 goldmark→HTML→goquery 提取文本方式）
- **超时保护**：每个测试用例执行加时间限制（默认 10s），避免 pathological 输入卡死
- **输出归一化与比较**：对三方 plaintext 输出做空白/换行归一化后进行两两对比
- **差异报告**：输出包含不一致的用例索引、输入摘要、三方输出对比、逐行 diff 定位信息
- **目录清晰分组**：3 个实现各自转换逻辑目录 + 1 个 diff 逻辑目录

## Tech Stack

- 语言：Go（与项目一致）
- 依赖：`github.com/yuin/goldmark`（本地 `goldmark/` 目录）、`github.com/PuerkitoBio/goquery`（复用 strip.go 方式需要）
- md4c 调用：`os/exec` 调用已编译的 `tools/md4c-plain/md4c-plain` 二进制
- 无其他新外部依赖

## Implementation Approach

### 整体策略

在 `tools/diffcheck/` 下创建独立的 Go 模块，按 4 个子目录组织：3 个实现的转换逻辑 + 1 个 diff 对比逻辑。每个测试用例分别调用三个引擎获取 plaintext 输出，归一化后两两对比，差异用逐行 diff 展示。每个用例加 `context.WithTimeout` 防止卡死。

### 三方引擎集成

1. **md2text**（`impl_md2text/`）：直接 Go API 调用 `md2text.New(md2text.WithFlags(parser.DialectGitHub)).Convert(src, &buf)`，使用 `context.WithTimeout` 包裹
2. **md4c**（`impl_md4c/`）：通过 `exec.CommandContext(ctx, path, "--gfm")` 调用 md4c-plain 二进制，stdin/stdout 管道通信，context 超时自动 kill 子进程
3. **goldmark**（`impl_goldmark/`）：复用 `common_test/markdown/strip.go` 的方式 — goldmark→HTML→goquery 提取文本节点。从 strip.go 移植核心逻辑（goldmark engine 初始化 + ConvertMarkdownToPlainText + walkNodes），去掉对 `common_test` 模块的直接依赖，同时引入 goquery 依赖

### 超时控制

- 默认每个用例 10s 超时（可通过 `--timeout` 参数调整）
- md2text/goldmark：使用 `context.WithTimeout` + goroutine + channel 模式（参考 `pathological_test.go` 的 done channel 模式）
- md4c：使用 `exec.CommandContext`，超时自动发送 SIGKILL

### 输出归一化

三个实现的空白处理存在差异：

- md2text/md4c-plain：保留换行和段落空行，表格用 tab 分隔
- goldmark→HTML→goquery：文本用空格连接，换行/空行语义丢失（`strings.Join(textParts, " ")` + `strings.Fields`）

归一化策略（两套模式）：

1. **strict**：仅去除末尾空白行，保留原始换行/空行语义。适合 md2text vs md4c 精确对比
2. **loose**（默认）：合并连续空白为单个空格，`strings.Fields` 重组。适合三方宽松对比（因为 goldmark strip 方式天然丢失换行语义）

### diff 报告格式

对不一致的用例，输出辅助定位信息：

```
=== Case #42 (source: jsonl) ===
Input (first 200 chars): # Hello **world**...
--- goldmark vs md2text ---
  1 | Hello world
+ 1 | Hello
  2 | world
--- md2text vs md4c ---
(same)
--- goldmark vs md4c ---
  1 | Hello world
- 1 | Hello
  2 | world
```

使用 Go 标准库实现简单逐行 diff，无需引入 diff 库。

## Directory Structure

```
tools/
├── md4c-plain/                  # [KEEP] 现有 md4c plaintext 工具，不变
│   ├── main.c
│   ├── md4c-plain               # 编译产物
│   ├── testdata.jsonl      # JSONL 测试输入
│   ├── README.md
│   └── output.txt
└── diffcheck/                    # [NEW] 对拍测试主目录，独立 Go 模块
    ├── go.mod                    # [NEW] 独立模块，replace 指向 ../../ 和 ../../goldmark
    ├── main.go                   # [NEW] CLI 入口，支持 --jsonl/--fuzz/--all/--timeout/--normalize
    ├── impl_md2text/             # [NEW] md2text 转换逻辑
    │   └── engine.go             # [NEW] Md2TextEngine：调用 md2text.Convert，context 超时
    ├── impl_md4c/                # [NEW] md4c 转换逻辑
    │   └── engine.go             # [NEW] Md4cEngine：exec.CommandContext 调用 md4c-plain
    ├── impl_goldmark/            # [NEW] goldmark 转换逻辑（复用 strip.go 方式）
    │   └── engine.go             # [NEW] GoldmarkEngine：goldmark→HTML→goquery 提取文本
    ├── diff/                     # [NEW] diff 对比逻辑
    │   ├── normalizer.go         # [NEW] 输出归一化（strict/loose 两种模式）
    │   ├── differ.go             # [NEW] 逐行 diff 算法 + 报告格式化
    │   └── report.go            # [NEW] 报告结构体 + 输出格式化
    ├── loader.go                # [NEW] 测试用例加载：JSONL 解析 + fuzz 种子
    ├── engine.go                # [NEW] Engine 接口定义 + 通用超时执行逻辑
    └── diffcheck_test.go        # [NEW] Go test 对拍测试集成
```

### 文件功能详述

- **go.mod**: `module diffcheck`，replace `md2text => ../../`、`github.com/yuin/goldmark => ../../goldmark`，require `github.com/PuerkitoBio/goquery`
- **main.go**: CLI 入口，flag 解析（`--jsonl`, `--fuzz`, `--all`, `--timeout`, `--normalize=loose|strict`, `--verbose`），调用 loader 加载用例，初始化三引擎，调用 differ 对比并输出报告
- **engine.go**: 定义 `Engine` 接口（`Name() string`, `Convert(ctx context.Context, input []byte) (string, error)`），实现通用 `RunWithTimeout(ctx context.Context, fn func() (string, error)) (string, error)` 函数
- **impl_md2text/engine.go**: 实现 `Md2TextEngine`，调用 `md2text.New(md2text.WithFlags(parser.DialectGitHub)).Convert()`，带 context 超时
- **impl_md4c/engine.go**: 实现 `Md4cEngine`，使用 `exec.CommandContext` 调用 `md4c-plain --gfm`，通过 stdin 写入输入、stdout 读取输出，超时自动 kill 进程；增加二进制路径检测和编译提示
- **impl_goldmark/engine.go**: 实现 `GoldmarkEngine`，移植 `common_test/markdown/strip.go` 的 goldmark→HTML→goquery 逻辑：初始化 goldmark 引擎（带 GFM + Table 扩展），预处理去除图片语法，转换为 HTML，通过 goquery 递归遍历 DOM 提取文本节点，`strings.Fields` 重组
- **diff/normalizer.go**: `Normalize(output string, mode Mode) string`，strict 模式仅去除末尾空白行；loose 模式合并连续空白为单个空格 + `strings.Fields` 重组
- **diff/differ.go**: `Compare(nameA, outputA, nameB, outputB string) *DiffResult`，逐行对比，标记增/删/不变行，生成带行号的 diff
- **diff/report.go**: `Report` 结构体（case 索引、来源、输入摘要、三对 DiffResult），`Format() string` 格式化为可读报告，`Summary() string` 输出统计摘要
- **loader.go**: `LoadJSONL(path string) ([]TestCase, error)` 解析 JSONL 提取 `data.content`（防御空/非法行）；`LoadFuzzSeeds() []TestCase` 生成 fuzz 种子用例（复用 `fuzz_test.go` 的种子 + 扩展）
- **diffcheck_test.go**: `TestDiffCheckJSONL` / `TestDiffCheckFuzz` / `TestDiffCheckAll`，集成 `go test`，每个用例带超时，差异 case 用 `t.Errorf` 报告，可通过 `-short` 跳过慢测试

## Implementation Notes

- **性能**：JSONL 有 1001 条记录，md4c 每次调用需 fork+exec，考虑实现批量管道模式（单次启动 md4c-plain，用特殊分隔符分割多输入）或标注为慢测试用 `-short` 跳过
- **安全性**：md4c 二进制路径使用相对路径，不涉及网络请求；JSONL 数据来源不可信，解析时需防御空 content、非法 JSON 等
- **blast radius**：tools/diffcheck/ 为独立模块，不影响主项目代码
- **goquery 依赖**：goldmark 引擎需要 goquery，在 diffcheck 的 go.mod 中添加
- **md4c-plain 编译**：测试前需确保 md4c-plain 已编译，engine 初始化时检测并给出编译提示
- **context 传播**：md4c 使用 `exec.CommandContext` 原生支持超时 kill；md2text/goldmark 使用 goroutine + done channel 模式
- **goldmark strip 逻辑移植**：不直接 import `common_test` 模块（依赖过重），而是将 strip.go 的核心逻辑移植到 impl_goldmark/engine.go，保持逻辑一致