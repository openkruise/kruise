# Calculator Benchmark Tests

This document describes the benchmark tests for the calculator package.

## Running Benchmarks

### Run all benchmarks
```bash
go test -bench=. -benchmem ./pkg/util/calculator/
```

### Run specific benchmark
```bash
# Arithmetic benchmarks
go test -bench=BenchmarkParseSimpleArithmetic -benchmem ./pkg/util/calculator/

# Variable substitution benchmarks
go test -bench=BenchmarkParseWithVariables -benchmem ./pkg/util/calculator/

# Function call benchmarks
go test -bench=BenchmarkFunctionCalls -benchmem ./pkg/util/calculator/
```

### Run with custom duration or iterations
```bash
# Run for 5 seconds
go test -bench=BenchmarkParseSimpleArithmetic -benchtime=5s ./pkg/util/calculator/

# Run 10000 iterations
go test -bench=BenchmarkParseSimpleArithmetic -benchtime=10000x ./pkg/util/calculator/
```

### Generate profiling data
```bash
# CPU profile
go test -bench=. -cpuprofile=cpu.prof ./pkg/util/calculator/
go tool pprof cpu.prof

# Memory profile
go test -bench=. -memprofile=mem.prof ./pkg/util/calculator/
go tool pprof mem.prof
```

### Compare benchmark results
```bash
# Save baseline
go test -bench=. -benchmem ./pkg/util/calculator/ > old.txt

# Make changes and test again
go test -bench=. -benchmem ./pkg/util/calculator/ > new.txt

# Compare (requires benchstat tool)
benchstat old.txt new.txt
```

## Benchmark Categories

### 1. Basic Parsing
- **BenchmarkParseSimpleArithmetic**: Simple expression `2 + 3`
- **BenchmarkParseComplexArithmetic**: Complex expression `(10 + 20) * 3 - 5`
- **BenchmarkParsePercentage**: Percentage `100 * 50%`
- **BenchmarkParseScientificNotation**: Scientific notation `1e3 + 500`

### 2. Kubernetes Quantity Operations
- **BenchmarkParseQuantity**: Basic quantity `100m + 50m`
- **BenchmarkParseQuantityComplex**: Complex quantity `(100m + 50m) * 2`
- **BenchmarkParseLargeQuantity**: Large memory `1000Gi + 500Gi`

### 3. Function Calls
- **BenchmarkParseMaxFunction**: Max function `max(100m, 200m)`
- **BenchmarkParseMinFunction**: Min function `min(100m, 200m)`
- **BenchmarkParseNestedFunctions**: Nested functions `max(min(100m, 200m), 150m)`

### 4. Variable Substitution
- **BenchmarkParseWithVariablesSimple**: Simple variable `cpu * 2`
- **BenchmarkParseWithVariablesComplex**: Complex with variables `max(cpu * 50%, 50m)`
- **BenchmarkParseProposalScenario**: Real-world SidecarSet scenario

### 5. Instance Reuse
- **BenchmarkCalculatorReuse**: Reusing Calculator instances
- **BenchmarkCalculatorWithVariablesReuse**: Reusing Calculator with variables

### 6. Low-Level Operations
- **BenchmarkArithmeticOperations**: Individual operations (Add, Sub, Mul, Div)
- **BenchmarkFunctionCalls**: Function implementations (Max, Min)

### 7. Additional Tests
- **BenchmarkParseMemoryAllocation**: Memory allocation patterns
- **BenchmarkConcurrentParsing**: Concurrent parsing performance

## Reference Results

The following benchmark results were obtained on:
- **CPU**: Apple M1 Pro (10 cores)
- **OS**: macOS Darwin 24.6.0
- **Architecture**: arm64

```
BenchmarkParseSimpleArithmetic-10                2219050        627.0 ns/op     1152 B/op      7 allocs/op
BenchmarkParseComplexArithmetic-10               1252183       1753 ns/op       1472 B/op     11 allocs/op
BenchmarkParsePercentage-10                      1834174        697.7 ns/op     1152 B/op      7 allocs/op
BenchmarkParseQuantity-10                        1385220        780.4 ns/op     1280 B/op      9 allocs/op
BenchmarkParseQuantityComplex-10                 1000000       1594 ns/op       1440 B/op     11 allocs/op
BenchmarkParseMaxFunction-10                     1218528        956.3 ns/op     1152 B/op      7 allocs/op
BenchmarkParseMinFunction-10                     1655571        850.6 ns/op     1152 B/op      7 allocs/op
BenchmarkParseNestedFunctions-10                 1000000       1110 ns/op       1312 B/op      9 allocs/op
BenchmarkParseWithVariablesSimple-10             2165596        562.3 ns/op     1072 B/op      6 allocs/op
BenchmarkParseWithVariablesComplex-10            1366978       1102 ns/op       1232 B/op      8 allocs/op
BenchmarkParseProposalScenario-10                1000000       1873 ns/op       1232 B/op      8 allocs/op
BenchmarkParseLargeQuantity-10                    873884       1196 ns/op       1280 B/op      9 allocs/op
BenchmarkParseScientificNotation-10              1778199        718.7 ns/op     1152 B/op      7 allocs/op
BenchmarkCalculatorReuse-10                      1869799        536.8 ns/op     1056 B/op      5 allocs/op
BenchmarkCalculatorWithVariablesReuse-10         2612859        560.3 ns/op      976 B/op      4 allocs/op
BenchmarkArithmeticOperations/Add-10            10246407        144.1 ns/op      208 B/op      3 allocs/op
BenchmarkArithmeticOperations/Sub-10            10507390        152.1 ns/op      208 B/op      3 allocs/op
BenchmarkArithmeticOperations/Mul-10            24176366         46.42 ns/op      80 B/op      1 allocs/op
BenchmarkArithmeticOperations/Div-10            30410781         41.39 ns/op      80 B/op      1 allocs/op
BenchmarkFunctionCalls/Max-10                   32819502         48.07 ns/op      80 B/op      1 allocs/op
BenchmarkFunctionCalls/Min-10                   27980316         43.60 ns/op      80 B/op      1 allocs/op
BenchmarkParseMemoryAllocation-10                1000000       1402 ns/op       1340 B/op      9 allocs/op
BenchmarkConcurrentParsing-10                    1000000       1699 ns/op       1312 B/op      9 allocs/op
```

### Metrics Explanation

- **ns/op**: Nanoseconds per operation (lower is better)
- **B/op**: Bytes allocated per operation (lower is better)
- **allocs/op**: Number of allocations per operation (lower is better)

## Performance Summary

| Category | Average Time | Memory Usage | Allocations |
|----------|-------------|--------------|-------------|
| Simple expressions | ~600-800 ns | ~1.1 KB | 5-7 |
| Complex expressions | ~1500-1800 ns | ~1.4 KB | 9-11 |
| Variable substitution | ~600-1100 ns | ~1.0-1.2 KB | 4-8 |
| Instance reuse | ~540-560 ns | ~1.0 KB | 4-5 |
| Low-level operations | ~40-150 ns | 80-208 B | 1-3 |
| Concurrent parsing | ~1700 ns | ~1.3 KB | 9 |

## Use Cases

The calculator's performance is well-suited for:
- **SidecarSet dynamic resource configuration**: Expression parsing takes 1-2 microseconds, which has minimal impact on Pod creation latency
- **High-throughput scenarios**: Can handle thousands of expression evaluations per second
- **Concurrent processing**: Thread-safe design supports parallel Pod creation requests
- **Webhook operations**: Low memory footprint suitable for webhook environments
