# Calculator 表达式验证工具

一个用于验证 calculator 表达式并生成可视化图表的小工具。

## 功能

- 验证 kruise calculator 包的表达式语法
- 生成可视化图表,展示表达式在不同输入值下的行为
- 仅支持 `cpu` 和 `memory` 变量

## 快速开始

### 编译

```bash
go build -o validator .
```

### 使用

**验证表达式:**

```bash
./validator -expr "cpu * 0.5" -validate
```

**验证并绘图:**

```bash
./validator -expr "0.5*cpu - 0.3*max(0, cpu-4)" -var cpu -min 0 -max 16 -output plot.png
```

## 命令行参数

| 参数 | 说明 | 默认值 |
|------|------|--------|
| `-expr` | 要验证的表达式(必需) | - |
| `-var` | 变量名: `cpu` 或 `memory` | `cpu` |
| `-min` | 最小值(k8s quantity 格式) | `0` |
| `-max` | 最大值(k8s quantity 格式) | `16` |
| `-output` | 输出 PNG 文件路径 | - |
| `-validate` | 仅验证,不绘图 | `false` |

## 示例

**CPU 表达式:**
```bash
./validator -expr "max(cpu*50%, 50m)" -var cpu -min 0 -max 16 -output cpu.png
```

**Memory 表达式:**
```bash
./validator -expr "0.5*memory + max(0, memory-2)" -var memory -min 0Gi -max 8Gi -output mem.png
```

**仅验证:**
```bash
./validator -expr "cpu + 100m" -validate
```

## 表达式语法

**运算符:** `+`, `-`, `*`, `/`
**函数:** `max()`, `min()`
**变量:** 仅支持 `cpu`, `memory`
**Quantity:** `100m`, `2`, `1Gi`, `100Mi` 等
**数字:** `0.5`, `50%`, `100` 等

## 依赖说明

本工具使用主仓库的 `github.com/openkruise/kruise/pkg/util/calculator` 包。

为了在本地开发环境中使用,`go.mod` 中包含了 `replace` 指令:

```go
replace github.com/openkruise/kruise => ../../..
```

这样工具就可以引用父仓库中的 calculator 包了。
