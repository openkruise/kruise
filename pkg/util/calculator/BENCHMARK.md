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
BenchmarkParseSimpleArithmetic-10                8082366        438.4 ns/op     1120 B/op      7 allocs/op
BenchmarkParseComplexArithmetic-10               4382671        830.1 ns/op     1440 B/op     11 allocs/op
BenchmarkParsePercentage-10                      7622686        481.6 ns/op     1120 B/op      7 allocs/op
BenchmarkParseQuantity-10                        6125442        608.2 ns/op     1248 B/op      9 allocs/op
BenchmarkParseQuantityComplex-10                 4242794        856.1 ns/op     1408 B/op     11 allocs/op
BenchmarkParseMaxFunction-10                     6317332        581.3 ns/op     1120 B/op      7 allocs/op
BenchmarkParseMinFunction-10                     6209354        661.1 ns/op     1120 B/op      7 allocs/op
BenchmarkParseNestedFunctions-10                 4048755        844.1 ns/op     1280 B/op      9 allocs/op
BenchmarkParseWithVariablesSimple-10             7972747        429.2 ns/op     1040 B/op      6 allocs/op
BenchmarkParseWithVariablesComplex-10            4969149        837.7 ns/op     1200 B/op      8 allocs/op
BenchmarkParseProposalScenario-10                4755639        858.4 ns/op     1200 B/op      8 allocs/op
BenchmarkParseLargeQuantity-10                   5339418        690.9 ns/op     1248 B/op      9 allocs/op
BenchmarkParseScientificNotation-10              8134534        478.5 ns/op     1120 B/op      7 allocs/op
BenchmarkCalculatorReuse-10                      9603499        392.1 ns/op     1024 B/op      5 allocs/op
BenchmarkCalculatorWithVariablesReuse-10        10385060        373.2 ns/op      944 B/op      4 allocs/op
BenchmarkArithmeticOperations/Add-10            36233281         96.75 ns/op      208 B/op      3 allocs/op
BenchmarkArithmeticOperations/Sub-10            40982226        109.3 ns/op      208 B/op      3 allocs/op
BenchmarkArithmeticOperations/Mul-10           100000000         36.16 ns/op      80 B/op      1 allocs/op
BenchmarkArithmeticOperations/Div-10           100000000         32.88 ns/op      80 B/op      1 allocs/op
BenchmarkFunctionCalls/Max-10                   94502450         33.36 ns/op      80 B/op      1 allocs/op
BenchmarkFunctionCalls/Min-10                  100000000         33.29 ns/op      80 B/op      1 allocs/op
BenchmarkParseMemoryAllocation-10                4568913        931.2 ns/op     1308 B/op      9 allocs/op
BenchmarkConcurrentParsing-10                    6013420        686.2 ns/op     1280 B/op      9 allocs/op
```

### Metrics Explanation

- **ns/op**: Nanoseconds per operation (lower is better)
- **B/op**: Bytes allocated per operation (lower is better)
- **allocs/op**: Number of allocations per operation (lower is better)

## Performance Summary

| Category | Average Time | Memory Usage | Allocations |
|----------|-------------|--------------|-------------|
| Simple expressions | ~430-610 ns | ~1.1 KB | 7-9 |
| Complex expressions | ~830-930 ns | ~1.3-1.4 KB | 9-11 |
| Variable substitution | ~370-840 ns | ~0.9-1.2 KB | 4-8 |
| Instance reuse | ~370-390 ns | ~0.9-1.0 KB | 4-5 |
| Low-level operations | ~33-110 ns | 80-208 B | 1-3 |
| Concurrent parsing | ~690 ns | ~1.3 KB | 9 |

## Use Cases

The calculator's performance is well-suited for:
- **SidecarSet dynamic resource configuration**: Expression parsing takes 1-2 microseconds, which has minimal impact on Pod creation latency
- **High-throughput scenarios**: Can handle thousands of expression evaluations per second
- **Concurrent processing**: Thread-safe design supports parallel Pod creation requests
- **Webhook operations**: Low memory footprint suitable for webhook environments
