# Calculator Expression Validator

A simple tool for validating calculator expressions and generating visualization plots.

## What it does

- Validates expression syntax for the kruise calculator package
- Generates visual plots showing how expressions behave across different input values
- Supports `cpu` and `memory` variables only

## Quick Start

### Build

```bash
go build -o validator .
```

### Usage

**Validate an expression:**

```bash
./validator -expr "cpu * 0.5" -validate
```

**Validate and plot:**

```bash
./validator -expr "0.5*cpu - 0.3*max(0, cpu-4)" -var cpu -min 0 -max 16 -output plot.png
```

## Command-Line Options

| Option | Description | Default |
|--------|-------------|---------|
| `-expr` | Expression to validate (required) | - |
| `-var` | Variable name: `cpu` or `memory` | `cpu` |
| `-min` | Minimum value (k8s quantity format) | `0` |
| `-max` | Maximum value (k8s quantity format) | `16` |
| `-output` | Output PNG file path | - |
| `-validate` | Validate only, skip plotting | `false` |

## Examples

**CPU expression:**
```bash
./validator -expr "max(cpu*50%, 50m)" -var cpu -min 0 -max 16 -output cpu.png
```

**Memory expression:**
```bash
./validator -expr "0.5*memory + max(0, memory-2)" -var memory -min 0Gi -max 8Gi -output mem.png
```

**Validation only:**
```bash
./validator -expr "cpu + 100m" -validate
```

## Expression Syntax

**Operators:** `+`, `-`, `*`, `/`
**Functions:** `max()`, `min()`
**Variables:** `cpu`, `memory` only
**Quantities:** `100m`, `2`, `1Gi`, `100Mi`, etc.
**Numbers:** `0.5`, `50%`, `100`, etc.

## Dependencies

This tool uses the `github.com/openkruise/kruise/pkg/util/calculator` package from the main repository.

To set up the local development environment, the `go.mod` includes a `replace` directive:

```go
replace github.com/openkruise/kruise => ../../..
```

This allows the tool to reference the calculator package from the parent repository.
