# 表达式生成器

根据给定的点生成分段线性函数的数学表达式的 Python 工具。

## 功能特性

- 使用的运算符: `+`, `-`, `*`, `/`, `()`, `max`, `min`
- 支持自定义变量名
- 处理所有类型的分段线性函数:
  - 线性函数(恒定斜率)
  - 凸函数(斜率递增)
  - 凹函数(斜率递减)
  - 混合/锯齿形(斜率交替变化)
- **自动验证**生成的表达式(可通过 `--no-verify` 禁用)
- 简洁的命令行界面

## 环境配置

本项目使用 [uv](https://github.com/astral-sh/uv) 管理 Python 环境和依赖。

### 安装 uv

```bash
# macOS/Linux
curl -LsSf https://astral.sh/uv/install.sh | sh

# 或使用 pip
pip install uv
```

### 创建并激活虚拟环境

```bash
# 创建 Python 虚拟环境 (Python 3.8+)
uv venv

# 激活环境
# macOS/Linux:
source .venv/bin/activate

# Windows:
.venv\Scripts\activate
```

### 安装依赖

本工具仅使用 Python 标准库,无需额外依赖。

## 使用方法

### 基础用法

```bash
python3 generate_expression.py "[[0,0], [1,1], [2,2], [3,3]]"
```

输出: `x`

### 自定义变量名

```bash
python3 generate_expression.py "[[0,0], [1,1], [2,3]]" --var x
```

输出: `max(x,2*x-1.0)`

### 更多示例

```bash
# 分段线性函数
python3 generate_expression.py "[[0,0], [1,1], [2,3]]"

# 自定义变量名
python3 generate_expression.py "[[0,0], [10,100], [20,150]]" -v mem

# 设置脚本为可执行
chmod +x generate_expression.py
./generate_expression.py "[[0,0], [1,1], [2,2]]"

# 跳过自动验证
python3 generate_expression.py "[[0,0], [1,1], [2,2]]" --no-verify
```

## 输入格式

点应该以 JSON 数组的格式提供,每个点为 [x, y] 对:
```
"[[x1,y1], [x2,y2], [x3,y3], ...]"
```

## 输出

工具输出一个数学表达式字符串,该表达式:
- 通过所有输入点
- 仅使用允许的运算符: `+`, `-`, `*`, `/`, `()`, `max`, `min`
- 仅包含指定的变量(其他值均为常量)
- 表示分段线性函数
- **自动验证**确保表达式正确

## 算法说明

工具执行以下步骤:
1. 按 x 坐标排序点
2. 计算每个线段的线性方程
3. 判断函数类型:
   - **凸函数**(斜率递增): 使用所有线段的 `max()`
   - **凹函数**(斜率递减): 使用所有线段的 `min()`
   - **混合函数**(斜率交替): 根据斜率变化构建嵌套的 `max()`/`min()` 表达式
4. 自动验证生成的表达式是否通过所有点

## 命令行选项

- `points`: (必需) JSON 数组格式的点, 如 `"[[0,0], [1,1], [2,2]]"`
- `-v, --var`: 变量名 (默认: `x`)
- `--no-verify`: 跳过生成表达式的自动验证

## 使用示例

### 示例 1: 线性函数
```bash
$ python3 generate_expression.py "[[0,0], [1,1], [2,2], [3,3]]"
x
```

### 示例 2: 分段线性函数(凸)
```bash
$ python3 generate_expression.py "[[0,0], [1,1], [2,3]]"
max(x,2*x-1.0)
```

验证:
- 当 x=0: max(0, 0-1) = max(0, -1) = 0 ✓
- 当 x=1: max(1, 2-1) = max(1, 1) = 1 ✓
- 当 x=2: max(2, 4-1) = max(2, 3) = 3 ✓

### 示例 3: 凹函数
```bash
$ python3 generate_expression.py "[[0,10], [5,20], [10,15]]"
min(2*x+10.0,-x+25.0)
```

### 示例 4: 混合斜率(锯齿形)
```bash
$ python3 generate_expression.py "[[0,0], [1,2], [2,1], [3,3]]"
max(min(2*x,-x+3.0),2*x-3.0)
```

验证:
- 当 x=0: max(min(0,3),−3) = max(0,−3) = 0 ✓
- 当 x=1: max(min(2,2),−1) = max(2,−1) = 2 ✓
- 当 x=2: max(min(4,1),1) = max(1,1) = 1 ✓
- 当 x=3: max(min(6,0),3) = max(0,3) = 3 ✓

## 自动验证

默认情况下,工具会自动验证生成的表达式:

```bash
$ python3 generate_expression.py "[[0,0], [1,1], [2,2]]"
x
# 自动验证通过,静默输出
```

如果验证失败,会输出错误信息:
```bash
$ python3 generate_expression.py "[[invalid points]]"
Verification FAILED at x=1: expected=1, actual=0.9

WARNING: Generated expression failed verification!
Expression: incorrect_expression
```

可以使用 `--no-verify` 跳过验证:
```bash
$ python3 generate_expression.py "[[0,0], [1,1]]" --no-verify
x
```

## 测试

运行测试脚本验证工具功能:

```bash
chmod +x test_advanced.sh
./test_advanced.sh
```

测试包括:
- 线性函数
- 凸函数(斜率递增)
- 凹函数(斜率递减)
- 混合斜率(上-下-上模式)
- 复杂锯齿形
- 负值
- 大数值
- 自定义变量名

## 退出虚拟环境

完成后,退出虚拟环境:

```bash
deactivate
```

## 故障排除

### 问题: "Invalid points format"
**原因**: 输入格式不正确
**解决**: 确保使用 JSON 数组格式,如 `"[[0,0], [1,1]]"`

### 问题: "Points have same x-coordinate"
**原因**: 两个点的 x 坐标相同
**解决**: 确保所有点的 x 坐标唯一

### 问题: "Verification FAILED"
**原因**: 生成的表达式未通过验证(这是一个 bug)
**解决**: 请报告此问题,包含输入的点

## 技术细节

- Python 版本: 3.8+
- 依赖: 仅标准库
- 支持的运算符: `+`, `-`, `*`, `/`, `()`, `max`, `min`
- 数值精度: 浮点数比较容差为 1e-10
