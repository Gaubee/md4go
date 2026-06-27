# md4go Benchmark Suite

Cross-engine benchmarks: **md4go** (pure Go), **md4c** (C via CGo), **goldmark**.

## Quick Start (one command)

```bash
cd benchmark
make bench
```

This automatically clones md4c (if missing), creates CGo symlinks, and runs all benchmarks.

## Prerequisites

- Go 1.25+
- GCC (for CGo)
- `make` (or see manual steps below)

## Commands

```bash
make setup        # clone md4c + create CGo symlinks (one-time)
make test         # run correctness verification (43 test cases)
make bench        # full benchmark suite (3s per benchmark)
make bench-quick  # quick mode: throughput + parse-only + GFM + setup
make clean        # remove symlinks only (md4c checkout preserved)
```

## Manual Setup (no make)

```bash
# Clone md4c
git clone --depth 1 https://github.com/mity/md4c.git ../md4c

# Create CGo symlinks
ln -sf ../md4c/src/md4c.c md4c.c
ln -sf ../md4c/src/md4c-html.c md4c_html.c
ln -sf ../md4c/src/entity.c entity.c

# Run benchmarks
go test -bench=. -benchtime=3s -benchmem | tee result.txt
```

## Benchmark Dimensions

| Name | Input | Output |
|------|-------|--------|
| `Throughput` | CommonMark 652 examples concatenated (~24KB) | HTML5 |
| `ParseOnly` | Same, null/sink renderer | None |
| `GFM` | Tables, tasklists, strikethrough, autolinks | HTML5 |
| `Pathological` | 26 stress-test cases (backticks, nesting, links) | HTML5 |
| `RealWorld` | JSONL files from `data/` | HTML5 |
| `Setup` | Parser creation cost (isolated) | — |
| `CGoBridge` | CGo `GoBytes` copy overhead | — |

## Custom Data

Add `.jsonl` files to `data/` in the format:

```json
{"data":{"content":"# Your markdown\n\n**bold** and *italic*."}}
```

Auto-discovered by `BenchmarkRealWorld`.

## Result Columns

| Column | Meaning |
|--------|---------|
| `ns/op` | Nanoseconds per operation (lower is better) |
| `B/op` | Bytes allocated per operation |
| `allocs/op` | Heap allocations per operation |
