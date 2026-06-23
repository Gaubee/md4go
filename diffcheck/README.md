# diffcheck — Markdown 引擎对拍测试工具

三方 Markdown→纯文本 引擎的 diff 对比工具，用于发现渲染行为差异。

## 引擎

| 引擎 | 实现方式 | 说明 |
|---|---|---|
| **md4go** | Go API 直接调用 | 本项目主引擎 |
| **md4c** | `exec.CommandContext` 调用 C 子进程 | 参考实现（GFM 模式） |
| **goldmark** | goldmark→HTML→goquery 提取文本 | 第三方 Go 库，会丢失 HTML 块和图片文本 |

## 前置条件

编译 md4c-plain 二进制 + Go CLI（首次运行前执行一次）：

```bash
cd diffcheck
./build.sh
```

或手动分步编译：

```bash
# C 二进制
cd diffcheck/csrc
gcc -O2 -I../../md4c/src -o md4c-plain main.c ../../md4c/src/md4c.c

# Go CLI
cd diffcheck
go build -o diffcheck ./cmd/diffcheck
```

## 运行方式

### CLI — 用户数据

```bash
cd diffcheck

# 单个 markdown 文件
./diffcheck --file=path/to/markdown.md

# stdin 输入
echo "# Hello **world**" | ./diffcheck --stdin

# 报告写入文件（而非 stdout）
./diffcheck --file=input.md --output=report.txt

# 与 --normalize/--dialect/--compat 组合
./diffcheck --file=input.md --normalize=strict --dialect=commonmark
```

### CLI — 内置数据源

```bash
cd diffcheck

# 默认：运行全部测试（JSONL + fuzz 种子）
go run ./cmd/diffcheck

# 指定数据源
go run ./cmd/diffcheck --src=fuzz        # 仅 fuzz 种子（175 条，秒级完成）
go run ./cmd/diffcheck --src=jsonl       # 仅 JSONL 数据（10350 条）
go run ./cmd/diffcheck --src=constructed # 仅构造用例（141 条）
go run ./cmd/diffcheck --src=all         # 全部

# 调整参数
go run ./cmd/diffcheck --normalize=strict   # strict 模式（保留换行语义）
go run ./cmd/diffcheck --normalize=loose    # loose 模式（折叠空白，默认）
go run ./cmd/diffcheck --timeout=30s        # 加大超时（长输入场景）
go run ./cmd/diffcheck -v                   # 显示一致的用例
```

退出码：0 = 全部一致，1 = 存在差异。

### go test

```bash
cd diffcheck

go test -run TestDiffCheckFuzz -v              # fuzz 种子测试
go test -run TestDiffCheckJSONL -v              # JSONL 测试（较慢）
go test -run TestDiffCheckJSONLStrict -v        # strict 归一化
go test -short -run TestDiffCheckJSONL          # short 模式跳过 JSONL
```

## 测试数据格式与构造

diffcheck 支持三种测试数据输入方式，以及三类内置数据源。

### 输入方式

| 方式 | 命令 | 适用场景 |
|---|---|---|
| 单个 Markdown 文件 | `./diffcheck --file=path/to/markdown.md` | 快速验证某个 `.md` 文件 |
| stdin | `echo "# Hello" \| ./diffcheck --stdin` | 管道输入、脚本集成 |
| JSONL 数据集 | `./diffcheck --src=jsonl` | 批量回归测试（自动加载 `data/` 目录） |

### JSONL 格式规范

每行一个 JSON 对象，工具仅读取 `data.content` 字段作为 Markdown 输入，其余字段（如 `url`、`status_code`）会被忽略。

```jsonl
{"data":{"content":"# 标题\n\n正文 **加粗**。\n"}}
{"data":{"content":"- 列表项一\n- 列表项二\n"}}
```

**格式要点：**

- 每行一个完整的 JSON 对象（JSONL，非 JSON 数组）
- 必须包含 `data.content` 字符串字段；空 `content` 的行会被跳过
- 换行符在 JSON 中转义为 `\n`，缩进用 `\t` 或空格
- 非法 JSON 行会被静默跳过（不会中断加载）
- 单行最大 10MB（加载器内置缓冲区上限）
- 文件编码须为 UTF-8

**构造示例（Python）：**

```python
import json

cases = [
    "# Hello\n\nParagraph.\n",
    "- item 1\n- item 2\n",
]
with open("mydata.jsonl", "w", encoding="utf-8") as f:
    for md in cases:
        f.write(json.dumps({"data": {"content": md}}, ensure_ascii=False) + "\n")
```

**构造示例（Go）：**

```go
import "encoding/json"

cases := []string{"# Hello\n", "- item\n"}
for _, md := range cases {
    rec := struct {
        Data struct{ Content string `json:"content"` } `json:"data"`
    }{}
    rec.Data.Content = md
    b, _ := json.Marshal(rec)
    fmt.Println(string(b)) // 每行写入文件
}
```

### 格式演示文件

[`data/example.jsonl`](data/example.jsonl) 包含 13 条样例，演示各类 Markdown 构造：

| 行 | 演示内容 |
|---|---|
| 1 | 基础标题 + 行内强调 |
| 2 | 行内代码 + 围栏代码块（带语言标注） |
| 3 | 有序/无序列表 + 嵌套列表 |
| 4 | GFM 表格 |
| 5 | 链接、图片、引用式链接定义 |
| 6 | 引用块（含嵌套） |
| 7 | HTML 块（goldmark 会 strip） |
| 8 | GFM 任务列表 + 删除线 |
| 9 | 实体引用 + 反斜杠转义 |
| 10 | 硬换行（尾随空格 / 反斜杠）+ 空结构 |
| 11 | 中文内容 |
| 12 | 混合内容综合用例 |
| 13 | 未闭合结构（测试解析器鲁棒性） |

可用其快速验证工具：

```bash
# 临时替换内置数据集进行测试
cp data/example.jsonl data/testdata1.jsonl
./diffcheck --src=jsonl
```

### 内置数据源

`--src` 控制 CLI 加载的内置数据源：

| 数据源 | `--src` 值 | 来源 | 说明 |
|---|---|---|---|
| JSONL | `jsonl` | `data/` 目录（`testdata1.jsonl`、`testdata2.jsonl`、`aidata_content.jsonl`） | 真实抓取数据，约 10350 条 |
| Fuzz 种子 | `fuzz` | `loader.go` 中 `LoadFuzzSeeds()` 硬编码 | 覆盖 CommonMark 0.31 各章节边界，175 条 |
| 构造用例 | `constructed` | `loader.go` 中 `LoadFuzzConstructed()` 程序生成 | 嵌套深度/定界符长度扫描，141 条 |
| 全部 | `all`（默认） | 上述三者合并 | — |

> **注意：** JSONL 自动加载仅识别 `data/` 目录下固定文件名（`aidata_content.jsonl`、`testdata1.jsonl`、`testdata2.jsonl`）。自定义 JSONL 文件需重命名为上述名称之一，或通过 `diffcheck.LoadJSONL(path)` 编程加载。

## 归一化模式

| 模式 | 行为 | 适用场景 |
|---|---|---|
| `loose`（默认） | 折叠所有空白为单空格 | 对比内容语义，忽略换行/缩进差异 |
| `strict` | 仅去除尾部空行，保留内部换行 | 对比输出结构，定位换行差异 |

loose 模式下 md4go vs md4c 仅 56 条差异（10350 条 JSONL）；strict 模式下 ~1091 条（多为换行/空白差异）。

完整 diff 对比报告见 [`../DIFF_REPORT.md`](../DIFF_REPORT.md)。

## 解读 Diff 报告

### 报告结构

```
━━━ Case #3 [fuzz] ━━━                     ← 用例编号 + 数据源
Input: ![img](src.png)\n                    ← 原始输入（\n\t 可见转义）

--- goldmark ---                            ← goldmark 归一化输出
(empty)

--- md4go ---                             ← md4go 归一化输出
1│ img                                      ← 行号│ 内容

--- md4c ---                                ← md4c 归一化输出
1│ img

△ md4go vs md4c: identical               ← 一致对，简写

△ md4go vs goldmark                      ← 差异对
-1│ img                                     ← - 行号：仅 md4go 有
                                            ← （goldmark 无对应行）
```

### 差异行标记

| 符号 | 含义 |
|---|---|
| ` 3│ text` | 上下文行（两侧共有） |
| `-3│ text` | 仅左侧引擎有（第 3 行） |
| `+3│ text` | 仅右侧引擎有（第 3 行） |
| ` ⋮` | 省略的相同行 |

### 错误标记

当引擎执行出错或超时时：

```
--- md4c ---
[ERROR: engine timeout: 30s]
```

### 汇总统计

```
━━━ Summary ━━━
Total: 934 | Diff: 568 | Timeout: 1 | Error: 0
  md4go≠goldmark: 566
  md4c≠goldmark: 567
  md4go≠md4c: 14
```

| 字段 | 含义 |
|---|---|
| Total | 总用例数 |
| Diff | 存在差异的用例数 |
| Timeout | 引擎超时的次数 |
| Error | 引擎出错的次数 |
| Pair Stats | 每对引擎的差异用例数 |

### 典型差异模式

| 差异对 | 原因 |
|---|---|
| `md4go≠goldmark` | goldmark strip 会丢失 HTML 块文本、图片 alt 文本、列表项句点后空格 |
| `md4go≠md4c`（loose 少量） | 实际内容差异，需关注 |
| `md4go≠md4c`（strict 较多） | 换行/空白处理差异，属预期 |

**重点关注 `md4go≠md4c` 在 loose 模式下的差异** — 这是两套实现间真正的内容差异。

## 目录结构

```
diffcheck/
├── go.mod
├── build.sh               # 一键构建脚本 (C + Go)
├── engine.go              # Engine 接口 + RunWithTimeout
├── engine_md4go.go        # md4go 引擎
├── engine_md4c.go         # md4c 子进程引擎
├── engine_goldmark.go     # goldmark strip 引擎
├── diff.go                # 归一化 + LCS diff + 报告格式化
├── flags.go               # 导出 parser.Flags 常量供 CLI 使用
├── loader.go              # JSONL 解析 + fuzz/constructed 种子
├── main.go                # RunCase + IsTimeout
├── diffcheck_test.go      # go test 集成
├── cmd/diffcheck/main.go  # CLI 入口 (--file/--stdin/--output/--dialect/--compat)
├── data/
│   ├── testdata1.jsonl        # 真实抓取数据（分片 1）
│   ├── testdata2.jsonl        # 真实抓取数据（分片 2）
│   └── example.jsonl          # 格式演示文件（13 条样例，见上文）
└── csrc/
    ├── main.c             # md4c-plain C 源码
    └── md4c-plain         # 编译产物
```
