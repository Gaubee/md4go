# md4go / md4c / goldmark 性能对比

> 环境: AMD EPYC 7K62 48-Core · linux/amd64 · Go 1.25.4 · 2026-06-27

| 引擎 | 实现 |
|------|------|
| **md4go** | Go 纯 |
| **md4c** | C (CGo + libmd4c.a) |
| **goldmark** | Go |

## 一、吞吐量 (CommonMark 652 例 ~23.8KB → HTML5)

| 引擎 | ns/op | B/op | allocs/op |
|------|-------|------|-----------|
| **md4c** | **1,782,568** | **84,456** | **7,357** |
| md4go | 2,084,098 | 272,834 | 4,444 |
| goldmark | 2,803,813 | 1,284,142 | 9,524 |

## 二、纯解析 (Null 渲染器)

| 引擎 | ns/op | B/op | allocs/op |
|------|-------|------|-----------|
| **md4c** | **465,467** | **0** | **0** |
| md4go | 1,788,230 | 95,769 | 4,336 |
| goldmark | 2,834,265 | 1,109,894 | 9,524 |

### 二-A、块级解析 ParseBlocksOnly（跳过内联管线）

| 引擎 | ns/op | B/op | allocs/op |
|------|-------|------|-----------|
| **md4go** | **1,306,742** | **63,950** | **3,614** |

## 三、GFM 扩展

| 引擎 | ns/op | B/op | allocs/op |
|------|-------|------|-----------|
| **md4c** | **1,491,764** | 153,169 | 7,440 |
| md4go | 1,676,256 | 705,391 | **5,845** |
| goldmark | 3,016,582 | 1,435,827 | 12,105 |

## 四、真实场景 (JSONL)

| 数据 | md4c | md4go | goldmark |
|------|------|-------|----------|
| example (13条) | 112µs / 5.4KB | 163µs / 88.0KB | 160µs / 125.7KB |
| testdata1 (1,000条) | 97.6ms / 16.0MB | 113ms / 41.2MB | 280ms / 119.4MB |
| testdata2 (9,999条) | 1.06s / 155.6MB | 1.24s / 412.5MB | 2.80s / 1.25GB |

## 五、病态输入

| 用例 | md4go | md4c | goldmark |
|------|-------|------|----------|
| **nested_strong_emph** | **451µs / 86.0KB** | 818µs / 42.4KB | 1,534µs / 1.03MB |
| **backticks** | 164µs / 88.7KB | **48.3µs / 41.0KB** | 2,003µs / 182.8KB |
| **deeply_nested_lists** | 4,123µs / 1.02MB | **276µs / 11.3KB** | 19,304µs / 5.66MB |
| **huge_table** (500列) | 236µs / 13.9KB | **201µs / 8.7KB** | 💀 113ms / 63.4MB |
| **permissive_autolinks** | 67.2µs / 8.8KB | **36.2µs / 3.1KB** | 💀 5,231µs / 130.8KB |
| **many_link_ref_instances** | 658µs / 630.6KB | **187µs / 55.7KB** | 1,028µs / 636.5KB |
| **nested_invalid_link_refs** | 88.7µs / 7.0KB | 23.3µs / 1.3KB | 💀 817µs / 335.2KB |

## 六、综合排名

```
              速度排名              内存效率            分配次数
吞吐量:     md4c > md4go > g    md4c > md4go > g    md4go > md4c > g
纯解析:     md4c > bs > md4go>g  md4c > bs > md4go>g  md4c > bs > md4go>g
GFM:        md4c > md4go > g    md4c > md4go > g    md4go > md4c > g
真实场景:    md4c > md4go > g    md4c > md4go > g    md4go > md4c > g
```

| 引擎 | 定位 |
|------|------|
| **md4c** | C 引擎零 GC，速度/内存双冠军 |
| **md4go** | 纯 Go 最优，分配次数已低于 CGo 桥接开销 |
| **goldmark** | huge_table 退化 562× |
