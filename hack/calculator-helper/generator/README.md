# Expression Generator

[中文文档](README_zh.md)

A Python tool that generates mathematical expressions representing piecewise linear functions passing through given points.

## Features

- Generates expressions using operators: `+`, `-`, `*`, `/`, `()`, `max`, `min`
- Supports custom variable names
- Handles all types of piecewise linear functions:
  - Linear (constant slope)
  - Convex (increasing slopes)
  - Concave (decreasing slopes)
  - Mixed/zigzag (alternating slopes)
- **Automatic verification** of generated expressions (can be disabled with `--no-verify`)
- Simple command-line interface

## Setup

This project uses [uv](https://github.com/astral-sh/uv) for Python environment management.

### Install uv

```bash
# macOS/Linux
curl -LsSf https://astral.sh/uv/install.sh | sh

# Or using pip
pip install uv
```

### Create and Activate Environment

```bash
# Create virtual environment (Python 3.8+)
uv venv

# Activate the environment
# On macOS/Linux:
source .venv/bin/activate

# On Windows:
.venv\Scripts\activate
```

### Dependencies

This tool uses only Python standard library. No additional dependencies required.

## Usage

### Basic Usage

```bash
python3 generate_expression.py "[[0,0], [1,1], [2,2], [3,3]]"
```

Output: `x`

### Custom Variable Name

```bash
python3 generate_expression.py "[[0,0], [1,1], [2,3]]" --var mem
```

Output: `max(mem,2*mem-1.0)`

### Skip Verification

```bash
python3 generate_expression.py "[[0,0], [1,1], [2,2]]" --no-verify
```

## Input Format

Points should be provided as a JSON array of [x, y] pairs:
```
"[[x1,y1], [x2,y2], [x3,y3], ...]"
```

## Command-line Options

- `points`: (required) Points as JSON array, e.g., `"[[0,0], [1,1], [2,2]]"`
- `-v, --var`: Variable name (default: `x`)
- `--no-verify`: Skip automatic verification of generated expression

## Examples

### Example 1: Linear Function
```bash
$ python3 generate_expression.py "[[0,0], [1,1], [2,2], [3,3]]"
x
```

### Example 2: Convex Function
```bash
$ python3 generate_expression.py "[[0,0], [1,1], [2,3]]"
max(x,2*x-1.0)
```

### Example 3: Concave Function
```bash
$ python3 generate_expression.py "[[0,10], [5,20], [10,15]]"
min(2*x+10.0,-x+25.0)
```

### Example 4: Mixed Slopes (Zigzag)
```bash
$ python3 generate_expression.py "[[0,0], [1,2], [2,1], [3,3]]"
max(min(2*x,-x+3.0),2*x-3.0)
```

## Algorithm

The tool:
1. Sorts points by x-coordinate
2. Calculates linear equations for each segment
3. Determines if the function is convex, concave, or mixed:
   - **Convex** (slopes increasing): Uses `max()` of all segments
   - **Concave** (slopes decreasing): Uses `min()` of all segments
   - **Mixed** (slopes alternating): Builds nested `max()`/`min()` expressions
4. Automatically verifies the generated expression passes through all points

## Testing

Run the test script to verify functionality:

```bash
chmod +x test_advanced.sh
./test_advanced.sh
```

## Help

```bash
python3 generate_expression.py --help
```

## Deactivate Environment

When done, deactivate the virtual environment:

```bash
deactivate
```
