---
name: diff-test-suite
overview: 在 tools/ 目录下构建基于 goldmark、md4c、本项目（md2text）三个 Markdown 实现的 plaintext 对拍测试体系，包含 JSONL 文件输入和 fuzz 生成输入，校验三实现输出一致并提供辅助定位 diff 信息。
todos:
  - id: init-diffcheck-module
    content: 创建 tools/diffcheck/ 目录和 go.mod，配置 replace directive 指向主项目
    status: pending
  - id: implement-engine
    content: 实现 engine.go：Engine 接口、Md2TextEngine、Md4cEngine、GoldmarkEngine
    status: pending
    dependencies:
      - init-diffcheck-module
  - id: implement-goldmark-plaintext
    content: 实现 goldmark_plaintext.go：goldmark plaintext renderer，覆盖核心节点和 GFM 扩展节点
    status: pending
    dependencies:
      - init-diffcheck-module
  - id: implement-loader-normalizer-differ
    content: 实现 loader.go（JSONL 解析 + fuzz 种子）、normalizer.go（输出归一化）、differ.go（逐行 diff + 报告）
    status: pending
    dependencies:
      - init-diffcheck-module
  - id: implement-main-cli
    content: 实现 main.go：CLI 入口，整合 loader/engine/differ，支持 --jsonl/--fuzz/--all 参数
    status: pending
    dependencies:
      - implement-engine
      - implement-goldmark-plaintext
      - implement-loader-normalizer-differ
  - id: implement-test-integration
    content: 实现 diffcheck_test.go：go test 集成，验证对拍测试可运行并输出差异报告
    status: pending
    dependencies:
      - implement-main-cli
---

## 产品概述

在 `tools/` 目录下构建三方可拍测试体系，对 goldmark、md4c、本项目 md2text 三个 markdown 解析实现的 plaintext 输出进行交叉验证，并输出辅助定位的 diff 信息。

## 核心功能

- **测试用例加载**：从 `testdata.jsonl` 加载 JSONL 文件输入，提取 `data.content` 作为 markdown；从 fuzz 种子/语料库生成合法与不合法 markdown 输入
- **三方引擎调用**：调用 md2text（Go API 直接调用）、md4c-plain（通过 `os/exec` 调用 C 二进制）、goldmark（Go API + 自定义 plaintext renderer）
- **输出归一化与比较**：对三方 plaintext 输出做空白/换行归一化后进行两两对比
- **差异报告**：输出包含不一致的用例索引、输入摘要、三方输出对比、逐行 diff 定位信息
- **目录重构**：重构 tools/ 目录，将对拍测试相关代码组织在 `tools/diffcheck/` 下

## 技术栈

- 语言：Go（与项目一致）
- 依赖：`github.com/yuin/goldmark`（本地已存在）、`os/exec` 调用 md4c-plain 二进制
- 无需引入新的外部依赖

## 实现方案

### 整体策略

在 `tools/diffcheck/` 下创建独立的 Go 模块，实现三方可拍测试。每个测试用例分别调用三个引擎获取 plaintext 输出，归一化后两两对比，差异用逐行 diff 展示。

### 三方引擎集成

1. **md2text**：`md2text.New(md2text.WithFlags(parser.DialectGitHub)).Convert(src, &buf)` — 直接 Go API 调用
2. **md4c**：`exec.Command("./tools/md4c-plain/md4c-plain", "--gfm").Stdin = bytes.NewReader(src)` — 通过 stdin/stdout 管道与 C 二进制通信
3. **goldmark**：需要编写 plaintext renderer — 遍历 AST，文本节点直接输出，表格用 tab 分隔，代码块原样输出，列表逐项输出，链接/图片仅输出文本内容。对齐 md4c-plain/main.c 的渲染逻辑

### goldmark plaintext renderer 设计

goldmark 使用 `ast.Walk` 遍历 AST 节点，核心策略：

- **文本节点**（`ast.Text`, `ast.String`）：直接输出文本内容
- **代码块**（`ast.CodeBlock`, `ast.FencedCodeBlock`）：原样输出代码，前后加空行
- **标题**（`ast.Heading`）：输出文本，前后加空行
- **段落**（`ast.Paragraph`）：输出文本，前后加空行（表内除外）
- **列表**（`ast.List`）：逐项输出，tight list 不加段间空行
- **表格**（extension `ast.Table`）：tab 分隔单元格，每行换行
- **链接**（`ast.Link`）：仅输出子节点文本
- **图片**（`ast.Image`）：仅输出子节点文本（alt）
- **删除线**（`ast.Strikethrough`）：仅输出子节点文本
- **任务列表**（`ast.TaskCheckBox`）：输出 `[x]` 或 `[ ]` 前缀

### 输出归一化

三个实现的空白处理差异可能存在，归一化策略：

1. 统一换行符为 `\n`
2. 去除末尾空白行（保留中间空白行语义）
3. 可选：合并连续空行为单个空行（宽松模式）

### diff 报告格式

对不一致的用例，输出：

```
=== Case #42 (source: jsonl) ===
Input (first 200 chars): # Hello **world**...
--- goldmark vs md2text ---
  1 | Hello
+ 1 | Hello world
--- goldmark vs md4c ---
(same)
--- md2text vs md4c ---
  1 | Hello world
- 1 | Hello
```

使用 Go 标准库 `strings` 实现简单的逐行 diff，无需引入 diff 库。

## 目录结构

```
tools/
├── md4c-plain/                  # [KEEP] 现有 md4c plaintext 工具，不变
│   ├── main.c
│   ├── md4c-plain               # 编译产物
│   ├── testdata.jsonl      # JSONL 测试输入
│   ├── README.md
│   └── output.txt
├── diffcheck/                    # [NEW] 对拍测试主目录，独立 Go 模块
│   ├── go.mod                    # [NEW] 独立模块，replace directive 指向 ../../
│   ├── main.go                   # [NEW] CLI 入口，支持 --jsonl / --fuzz / --all / --verbose
│   ├── engine.go                 # [NEW] Engine 接口 + md2text/md4c 引擎实现
│   ├── goldmark_plaintext.go     # [NEW] goldmark plaintext renderer
│   ├── normalizer.go             # [NEW] 输出归一化逻辑
│   ├── differ.go                 # [NEW] 逐行 diff + 报告格式化
│   ├── loader.go                # [NEW] 测试用例加载：JSONL 解析 + fuzz 种子生成
│   └── diffcheck_test.go         # [NEW] Go test 对拍测试，可直接 go test 运行
```

### 文件功能详述

- **go.mod**: `module diffcheck`，replace `md2text => ../../`，require `github.com/yuin/goldmark`（本地路径）
- **main.go**: CLI 入口，flag 解析（`--jsonl`, `--fuzz`, `--all`, `--verbose`, `--normalize=strict|loose`），调用 loader 加载用例，调用 engine 获取三方输出，调用 differ 生成报告
- **engine.go**: 定义 `Engine` 接口（`Name() string`, `Convert(input []byte) (string, error)`），实现 `Md2TextEngine`（直接 Go API）、`Md4cEngine`（`os/exec` 调用二进制）、`GoldmarkEngine`（使用 goldmark_plaintext renderer）
- **goldmark_plaintext.go**: 实现 `goldmark.renderer.NodeRenderer` 接口，遍历 AST 输出纯文本，对齐 md4c-plain 的渲染规则
- **normalizer.go**: `Normalize(output string, mode string) string`，处理换行、尾部空白、连续空行等
- **differ.go**: `Compare(nameA, outputA, nameB, outputB string) *DiffResult`，逐行对比，生成带行号的 diff 报告
- **loader.go**: `LoadJSONL(path string) ([]TestCase, error)` 解析 JSONL 提取 data.content；`LoadFuzzSeeds() []TestCase` 生成 fuzz 种子用例
- **diffcheck_test.go**: `TestDiffCheckJSONL` / `TestDiffCheckFuzz` / `TestDiffCheckAll`，集成 `go test` 框架

## 实现注意事项

- **性能**：JSONL 有 1001 条记录，md4c 每次调用需 fork+exec，考虑为 md4c 实现批量输入管道（单次启动进程，多行分隔输入），或在测试中标注为慢测试用 `-short` 跳过
- **安全性**：md4c 二进制路径使用相对路径，不涉及内部网络请求；JSONL 数据来源不可信，解析时需防御性处理（空 content、非法 JSON 等）
- **blast radius**：tools/diffcheck/ 为独立模块，不影响主项目代码
- **goldmark renderer 注册**：需要同时注册 core AST 节点和 extension AST 节点（Table, Strikethrough, TaskCheckBox）的渲染函数
- **md4c-plain 编译**：测试前需确保 md4c-plain 已编译，main.go 中可加编译检查