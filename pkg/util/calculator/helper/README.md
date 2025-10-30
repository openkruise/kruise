# Calculator Expression Helper Tool

Expression validation and visualization plotting tool based on the calculator package, used for SidecarSet dynamic resource configuration feature.

## Overview

This tool provides two core functionalities:

1. **Expression Validation**: Validates expression syntax and ensures only allowed variables (`cpu` and `memory`) are used
2. **Expression Plotting**: Generates visualization curves to help understand expression behavior across different input values

## Key Features

- ✅ Expression syntax validation (based on calculator package)
- ✅ Supports only `cpu` and `memory` variables
- ✅ Accepts k8s Quantity format inputs (e.g., `100m`, `4Gi`)
- ✅ Automatic unit conversion:
  - CPU: Display unit is **cores**
  - Memory: Display unit is **Gi**
- ✅ Fixed generation of 1000 data points for smooth curves
- ✅ PNG format output

## Usage

### Command-Line Tool

#### Basic Usage

```bash
go run pkg/util/calculator/helper/cmd/main.go \
  -expr "0.5*cpu - 0.3*max(0, cpu-4) + 0.3*max(0, cpu-8)" \
  -var cpu \
  -min 0 \
  -max 16 \
  -output docs/proposals/20250913-story5.png
```

#### Command-Line Parameters

| Parameter | Description | Example | Default |
|-----------|-------------|---------|---------|
| `-expr` | Expression (required) | `"0.5*cpu + 100m"` | None |
| `-var` | Variable name | `cpu` or `memory` | `cpu` |
| `-min` | Minimum value (k8s quantity) | `0`, `100m`, `1Gi` | `0` |
| `-max` | Maximum value (k8s quantity) | `16`, `8000m`, `4Gi` | `16` |
| `-output` | Output file path | `plot.png` | None |
| `-validate` | Validate only without plotting | - | `false` |

### Usage Examples

#### 1. Validate Expression Only

```bash
go run pkg/util/calculator/helper/cmd/main.go \
  -expr "max(cpu*50%, 50m)" \
  -validate
```

Output:
```
Validating expression: max(cpu*50%, 50m)
✓ Expression is valid
```

#### 2. CPU Expression Plotting (Using Cores)

```bash
go run pkg/util/calculator/helper/cmd/main.go \
  -expr "0.5*cpu - 0.3*max(0, cpu-4) + 0.3*max(0, cpu-8)" \
  -var cpu \
  -min 0 \
  -max 16 \
  -output cpu_plot.png
```

- X-axis: CPU (cores) - displays 0, 1.6, 3.2, ..., 16
- Y-axis: Result (cores)

#### 3. CPU Expression Plotting (Using Millicores Input)

```bash
go run pkg/util/calculator/helper/cmd/main.go \
  -expr "0.5*cpu - 0.3*max(0, cpu-4) + 0.3*max(0, cpu-8)" \
  -var cpu \
  -min 0m \
  -max 16000m \
  -output cpu_plot.png
```

- Input: `0m` to `16000m` (millicores)
- Display: 0 to 16 cores (automatically converted)

#### 4. Memory Expression Plotting

```bash
go run pkg/util/calculator/helper/cmd/main.go \
  -expr "0.5*memory + max(0, memory-2)" \
  -var memory \
  -min 0Gi \
  -max 8Gi \
  -output memory_plot.png
```

- X-axis: Memory (Gi) - displays 0, 0.8, 1.6, ..., 8
- Y-axis: Result (Gi)

#### 5. Complex Expression with Quantities

```bash
go run pkg/util/calculator/helper/cmd/main.go \
  -expr "0.3*max(0, 2*cpu-200m)" \
  -var cpu \
  -min 0 \
  -max 1 \
  -output complex_plot.png
```

### Programmatic Usage

#### Validate Expression

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

#### Generate Plot

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

## Supported Expression Syntax

### Operators and Functions

- **Arithmetic operators**: `+`, `-`, `*`, `/`
- **Functions**: `max()`, `min()`
- **Parentheses**: `(`, `)`
- **Numbers**: Integers, floats, percentages (e.g., `50%`)
- **Quantities**: CPU (e.g., `100m`, `2`) and Memory (e.g., `100Mi`, `1Gi`)
- **Variables**: Only `cpu` and `memory` are supported

### Valid Expression Examples

```
cpu * 0.5
max(cpu*50%, 50m)
memory + 100Mi
0.5*cpu - 0.3*max(0, cpu-4) + 0.3*max(0, cpu-8)
min(cpu*2, 4)
(cpu + 2) * 0.5
0.3*max(0, 2*cpu-200m)
```

### Invalid Expression Examples

```
disk * 2           # ❌ Invalid variable 'disk'
cpu + +            # ❌ Syntax error
cpu / 0            # ❌ Division by zero
storage + cpu      # ❌ Invalid variable 'storage'
```

## Technical Implementation Details

### Unit Conversion Mechanism

#### CPU Variable
```
Input (k8s quantity) → Internal Calculation (cores) → Display (cores)
   100m              →      0.1                     →    0.1
   1000m             →      1                       →    1
   4                 →      4                       →    4
```

#### Memory Variable
```
Input (k8s quantity) → Internal Calculation (Gi) → Display (Gi)
   1Gi               →     1                      →   1
   2048Mi            →     2                      →   2
   1073741824 bytes  →     1                      →   1
```

### Expression Evaluation Flow

1. **Parse Input**: Convert k8s quantity to internal representation
   ```go
   // CPU: "4" -> 4 cores
   x = resource.MustParse(fmt.Sprintf("%v", varValue))

   // Memory: "2" -> 2Gi
   x = resource.MustParse(fmt.Sprintf("%vGi", varValue))
   ```

2. **Expression Evaluation**: Use calculator package with variables as Quantity type
   ```go
   vars := map[string]*calculator.Value{
       varName: {
           IsQuantity: true,
           Quantity:   x,
       },
   }
   ```

3. **Result Conversion**:
   - CPU: `result.Quantity.MilliValue() / 1000` -> cores
   - Memory: `result.Quantity.Value() / (1024^3)` -> Gi

### Plotting Features

1. **Data Points**: Fixed 1000 points, calculated via `step = (max - min) / 999`
2. **Axes**:
   - X-axis: 10 tick marks
   - Y-axis: 10 tick marks, auto-scaled
3. **Grid Lines**: Gray grid lines for easier reading
4. **Auto Padding**: Y-axis range automatically adds 10% padding
5. **Curve Color**: Blue (RGB: 0, 0, 255)

## Test Coverage

### Unit Tests

```bash
go test ./pkg/util/calculator/helper -v
```

Test coverage:
- ✅ CPU expression evaluation
- ✅ Memory expression evaluation
- ✅ Expressions with Quantities (e.g., `cpu + 100m`)
- ✅ `max()` and `min()` functions
- ✅ Complex piecewise linear expressions
- ✅ Invalid variable detection
- ✅ Syntax error detection
- ✅ Division by zero detection
- ✅ CPU plotting (cores and millicores)
- ✅ Memory plotting (Gi)

## Dependencies

- **calculator**: `pkg/util/calculator` - Expression parsing and evaluation engine
- **image**: Go standard library - PNG image generation
- **font**: `golang.org/x/image/font` - Text label rendering
- **quantity**: `k8s.io/apimachinery/pkg/api/resource` - k8s Quantity handling

## Related Documentation

- [Proposal: SidecarSet Dynamic Resource Configuration](../../docs/proposals/20250913-sidecarset-dynamic-resources-when-creating.md)
- [Calculator Package](../calculator.go)

## Constraints and Limitations

1. **Variable Restriction**: Only `cpu` and `memory` variables are supported
2. **Data Points**: Fixed at 1000 points (not configurable)
3. **Fixed Units**:
   - CPU must be displayed in cores
   - Memory must be displayed in Gi
4. **No Inflection Point Annotation**: By design, does not automatically annotate slope change points
5. **Output Format**: Only supports PNG format

## Troubleshooting

### Common Errors

#### 1. Expression Validation Failed

```
Validation failed: invalid variable 'disk': only 'cpu' and 'memory' variables are allowed
```

**Solution**: Only use `cpu` or `memory` variables

#### 2. Quantity Parsing Failed

```
Invalid min value '100x': quantities must match the regular expression...
```

**Solution**: Use correct k8s quantity format, e.g., `100m`, `1Gi`

#### 3. Division by Zero

```
division by zero
```

**Solution**: Check division operations in the expression to ensure denominator is not zero

## Performance Considerations

- **Computational Complexity**: O(n), where n = 1000 (fixed)
- **Memory Usage**: ~8KB image + data point arrays
- **Generation Time**: Typically < 100ms (single plot)

## Example Output

Generated plot example (docs/proposals/20250913-story5.png):

- Title: "Expression: 0.5*cpu - 0.3*max(0, cpu-4) + 0.3*max(0, cpu-8)"
- X-axis: CPU (cores) - 0, 1.6, 3.2, ..., 16
- Y-axis: Result (cores) - auto-scaled
- Curve: Blue piecewise linear function
- Size: 800x600 pixels
