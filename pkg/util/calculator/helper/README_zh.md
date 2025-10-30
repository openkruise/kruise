# Calculator Expression Helper Tool

基于 calculator 包的表达式验证和可视化绘图工具，用于 SidecarSet 动态资源配置功能。

## 功能概述

该工具提供两大核心功能：

1. **表达式验证**：验证表达式语法正确性，确保只使用允许的变量（`cpu` 和 `memory`）
2. **表达式绘图**：生成表达式的可视化曲线图，帮助理解表达式在不同输入值下的行为

## 核心特性

- ✅ 表达式语法验证（基于 calculator 包）
- ✅ 仅支持 `cpu` 和 `memory` 两个变量
- ✅ 接受 k8s Quantity 格式的输入（如 `100m`, `4Gi`）
- ✅ 自动单位转换：
  - CPU: 显示单位为**核数** (cores)
  - Memory: 显示单位为 **Gi**
- ✅ 固定生成 1000 个数据点，确保曲线平滑
- ✅ PNG 格式输出

## 使用方法

### 命令行工具

#### 基本用法

```bash
go run pkg/util/calculator/helper/cmd/main.go \
  -expr "0.5*cpu - 0.3*max(0, cpu-4) + 0.3*max(0, cpu-8)" \
  -var cpu \
  -min 0 \
  -max 16 \
  -output docs/proposals/20250913-story5.png
```

#### 命令行参数

| 参数 | 说明 | 示例 | 默认值 |
|------|------|------|--------|
| `-expr` | 表达式（必需） | `"0.5*cpu + 100m"` | 无 |
| `-var` | 变量名 | `cpu` 或 `memory` | `cpu` |
| `-min` | 最小值（k8s quantity） | `0`, `100m`, `1Gi` | `0` |
| `-max` | 最大值（k8s quantity） | `16`, `8000m`, `4Gi` | `16` |
| `-output` | 输出文件路径 | `plot.png` | 无 |
| `-validate` | 仅验证不绘图 | - | `false` |

### 使用示例

#### 1. 仅验证表达式

```bash
go run pkg/util/calculator/helper/cmd/main.go \
  -expr "max(cpu*50%, 50m)" \
  -validate
```

输出:
```
Validating expression: max(cpu*50%, 50m)
✓ Expression is valid
```

#### 2. CPU 表达式绘图（使用核数）

```bash
go run pkg/util/calculator/helper/cmd/main.go \
  -expr "0.5*cpu - 0.3*max(0, cpu-4) + 0.3*max(0, cpu-8)" \
  -var cpu \
  -min 0 \
  -max 16 \
  -output cpu_plot.png
```

- 横轴: CPU (cores) - 显示 0, 1.6, 3.2, ..., 16
- 纵轴: Result (cores)

#### 3. CPU 表达式绘图（使用 millicores 输入）

```bash
go run pkg/util/calculator/helper/cmd/main.go \
  -expr "0.5*cpu - 0.3*max(0, cpu-4) + 0.3*max(0, cpu-8)" \
  -var cpu \
  -min 0m \
  -max 16000m \
  -output cpu_plot.png
```

- 输入: `0m` 到 `16000m`（millicores）
- 显示: 0 到 16 cores（自动转换）

#### 4. Memory 表达式绘图

```bash
go run pkg/util/calculator/helper/cmd/main.go \
  -expr "0.5*memory + max(0, memory-2)" \
  -var memory \
  -min 0Gi \
  -max 8Gi \
  -output memory_plot.png
```

- 横轴: Memory (Gi) - 显示 0, 0.8, 1.6, ..., 8
- 纵轴: Result (Gi)

#### 5. 带 Quantity 的复杂表达式

```bash
go run pkg/util/calculator/helper/cmd/main.go \
  -expr "0.3*max(0, 2*cpu-200m)" \
  -var cpu \
  -min 0 \
  -max 1 \
  -output complex_plot.png
```

### 编程方式使用

#### 验证表达式

```go
import (
    "github.com/openkruise/kruise/pkg/util/calculator/helper"
    "k8s.io/apimachinery/pkg/api/resource"
)

config := &helper.PlotConfig{
    Expression: "0.5*cpu + 100m",
    Variable:   "cpu",
    MinValue:   resource.MustParse("0"),
    MaxValue:   resource.MustParse("16"),
    NumPoints:  1000,
    OutputFile: "output.png",
    Title:      "CPU Resource Expression",
}
err := helper.ValidateExpression(config)
if err != nil {
    fmt.Printf("Invalid expression: %v\n", err)
}
```

#### 生成绘图

```go
import (
    "github.com/openkruise/kruise/pkg/util/calculator/helper"
    "k8s.io/apimachinery/pkg/api/resource"
)

config := &helper.PlotConfig{
    Expression: "0.5*cpu - 0.3*max(0, cpu-4) + 0.3*max(0, cpu-8)",
    Variable:   "cpu",
    MinValue:   resource.MustParse("0"),
    MaxValue:   resource.MustParse("16"),
    NumPoints:  1000,
    OutputFile: "output.png",
    Title:      "CPU Resource Expression",
}

err := helper.PlotExpression(config)
if err != nil {
    fmt.Printf("Failed to generate plot: %v\n", err)
}
```

## 支持的表达式语法

### 运算符和函数

- **算术运算符**: `+`, `-`, `*`, `/`
- **函数**: `max()`, `min()`
- **括号**: `(`, `)`
- **数字**: 整数, 浮点数, 百分比 (如 `50%`)
- **Quantity**: CPU (如 `100m`, `2`) 和 Memory (如 `100Mi`, `1Gi`)
- **变量**: 仅支持 `cpu` 和 `memory`

### 合法表达式示例

```
cpu * 0.5
max(cpu*50%, 50m)
memory + 100Mi
0.5*cpu - 0.3*max(0, cpu-4) + 0.3*max(0, cpu-8)
min(cpu*2, 4)
(cpu + 2) * 0.5
0.3*max(0, 2*cpu-200m)
```

### 非法表达式示例

```
disk * 2           # ❌ 非法变量 'disk'
cpu + +            # ❌ 语法错误
cpu / 0            # ❌ 除零错误
storage + cpu      # ❌ 非法变量 'storage'
```

## 技术实现细节

### 单位转换机制

#### CPU 变量
```
输入 (k8s quantity) → 内部计算 (cores) → 显示 (cores)
   100m              →      0.1         →    0.1
   1000m             →      1           →    1
   4                 →      4           →    4
```

#### Memory 变量
```
输入 (k8s quantity) → 内部计算 (Gi) → 显示 (Gi)
   1Gi               →     1          →   1
   2048Mi            →     2          →   2
   1073741824 bytes  →     1          →   1
```

### 表达式计算流程

1. **解析输入**: 将 k8s quantity 转换为内部表示
   ```go
   // CPU: "4" -> 4 cores
   x = resource.MustParse(fmt.Sprintf("%v", varValue))

   // Memory: "2" -> 2Gi
   x = resource.MustParse(fmt.Sprintf("%vGi", varValue))
   ```

2. **表达式计算**: 使用 calculator 包，变量作为 Quantity 类型
   ```go
   vars := map[string]*calculator.Value{
       varName: {
           IsQuantity: true,
           Quantity:   x,
       },
   }
   ```

3. **结果转换**:
   - CPU: `result.Quantity.MilliValue() / 1000` -> cores
   - Memory: `result.Quantity.Value() / (1024^3)` -> Gi

### 绘图特性

1. **数据点数量**: 固定 1000 个点，通过 `step = (max - min) / 999` 计算
2. **坐标轴**:
   - X 轴: 10 个刻度标记
   - Y 轴: 10 个刻度标记，自动缩放
3. **网格线**: 灰色网格线辅助读数
4. **自动 padding**: Y 轴范围自动添加 10% 的 padding
5. **曲线颜色**: 蓝色 (RGB: 0, 0, 255)

## 测试覆盖

### 单元测试

```bash
go test ./pkg/util/calculator/helper -v
```

测试覆盖:
- ✅ CPU 表达式计算
- ✅ Memory 表达式计算
- ✅ 带 Quantity 的表达式（如 `cpu + 100m`）
- ✅ `max()` 和 `min()` 函数
- ✅ 复杂分段线性表达式
- ✅ 非法变量检测
- ✅ 语法错误检测
- ✅ 除零错误检测
- ✅ CPU 绘图（核数和 millicores）
- ✅ Memory 绘图（Gi）

## 依赖关系

- **calculator**: `pkg/util/calculator` - 表达式解析和计算引擎
- **image**: Go 标准库 - PNG 图像生成
- **font**: `golang.org/x/image/font` - 文本标签渲染
- **quantity**: `k8s.io/apimachinery/pkg/api/resource` - k8s Quantity 处理

## 相关文档

- [提案: SidecarSet 动态资源配置](../../docs/proposals/20250913-sidecarset-dynamic-resources-when-creating.md)
- [Calculator 包](../calculator.go)

## 约束和限制

1. **变量限制**: 仅支持 `cpu` 和 `memory` 两个变量
2. **数据点**: 固定 1000 个点（不可配置）
3. **单位固定**:
   - CPU 必须显示为 cores
   - Memory 必须显示为 Gi
4. **不支持拐点标注**: 按需求设计，不自动标注斜率变化点
5. **输出格式**: 仅支持 PNG 格式

## 故障排查

### 常见错误

#### 1. 表达式验证失败

```
Validation failed: invalid variable 'disk': only 'cpu' and 'memory' variables are allowed
```

**解决**: 仅使用 `cpu` 或 `memory` 变量

#### 2. Quantity 解析失败

```
Invalid min value '100x': quantities must match the regular expression...
```

**解决**: 使用正确的 k8s quantity 格式，如 `100m`, `1Gi`

#### 3. 除零错误

```
division by zero
```

**解决**: 检查表达式中的除法运算，确保分母不为零

## 性能考虑

- **计算复杂度**: O(n)，其中 n = 1000（固定）
- **内存使用**: 约 8KB 图像 + 数据点数组
- **生成时间**: 通常 < 100ms（单个图表）

## 示例输出

生成的图表 (docs/proposals/20250913-story5.png) 示例：

- 标题: "Expression: 0.5*cpu - 0.3*max(0, cpu-4) + 0.3*max(0, cpu-8)"
- X 轴: CPU (cores) - 0, 1.6, 3.2, ..., 16
- Y 轴: Result (cores) - 自动缩放
- 曲线: 蓝色分段线性函数
- 尺寸: 800x600 像素
