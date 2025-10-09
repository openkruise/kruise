/*
Copyright 2025 The Kruise Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

	http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package calculator

import (
	"strings"
	"testing"

	"k8s.io/apimachinery/pkg/api/resource"
)

// BenchmarkParseSimpleArithmetic benchmarks simple arithmetic expression parsing
func BenchmarkParseSimpleArithmetic(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, _ = Parse("2 + 3")
	}
}

// BenchmarkParseComplexArithmetic benchmarks complex arithmetic with parentheses
func BenchmarkParseComplexArithmetic(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, _ = Parse("(10 + 20) * 3 - 5")
	}
}

// BenchmarkParsePercentage benchmarks percentage calculations
func BenchmarkParsePercentage(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, _ = Parse("100 * 50%")
	}
}

// BenchmarkParseQuantity benchmarks Kubernetes quantity operations
func BenchmarkParseQuantity(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, _ = Parse("100m + 50m")
	}
}

// BenchmarkParseQuantityComplex benchmarks complex quantity expressions
func BenchmarkParseQuantityComplex(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, _ = Parse("(100m + 50m) * 2")
	}
}

// BenchmarkParseMaxFunction benchmarks max function calls
func BenchmarkParseMaxFunction(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, _ = Parse("max(100m, 200m)")
	}
}

// BenchmarkParseMinFunction benchmarks min function calls
func BenchmarkParseMinFunction(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, _ = Parse("min(100m, 200m)")
	}
}

// BenchmarkParseNestedFunctions benchmarks deeply nested function calls
func BenchmarkParseNestedFunctions(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, _ = Parse("max(min(100m, 200m), 150m)")
	}
}

// BenchmarkParseWithVariablesSimple benchmarks simple variable substitution
func BenchmarkParseWithVariablesSimple(b *testing.B) {
	variables := map[string]*Value{
		"cpu": {IsQuantity: true, Quantity: resource.MustParse("100m")},
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = ParseWithVariables("cpu * 2", variables)
	}
}

// BenchmarkParseWithVariablesComplex benchmarks complex expressions with variables
func BenchmarkParseWithVariablesComplex(b *testing.B) {
	variables := map[string]*Value{
		"cpu":    {IsQuantity: true, Quantity: resource.MustParse("200m")},
		"memory": {IsQuantity: true, Quantity: resource.MustParse("512Mi")},
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = ParseWithVariables("max(cpu * 50%, 50m)", variables)
	}
}

// BenchmarkParseProposalScenario benchmarks real-world SidecarSet resource calculation
// This represents Story 1 from the proposal: max(cpu*50%, 50m)
func BenchmarkParseProposalScenario(b *testing.B) {
	variables := map[string]*Value{
		"cpu": {IsQuantity: true, Quantity: resource.MustParse("200m")},
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = ParseWithVariables("max(cpu * 50%, 50m)", variables)
	}
}

// BenchmarkParseLargeQuantity benchmarks parsing large memory quantities
func BenchmarkParseLargeQuantity(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, _ = Parse("1000Gi + 500Gi")
	}
}

// BenchmarkParseScientificNotation benchmarks scientific notation
func BenchmarkParseScientificNotation(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, _ = Parse("1e3 + 500")
	}
}

// BenchmarkCalculatorReuse benchmarks reusing calculator instances
func BenchmarkCalculatorReuse(b *testing.B) {
	calc := NewCalculator()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = calc.Parse("2 + 3")
	}
}

// BenchmarkCalculatorWithVariablesReuse benchmarks reusing calculator with variables
func BenchmarkCalculatorWithVariablesReuse(b *testing.B) {
	variables := map[string]*Value{
		"cpu": {IsQuantity: true, Quantity: resource.MustParse("100m")},
	}
	calc := NewCalculatorWithVariables(variables)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = calc.Parse("cpu * 2")
	}
}

// BenchmarkArithmeticOperations benchmarks individual arithmetic operations
func BenchmarkArithmeticOperations(b *testing.B) {
	calc := NewCalculator()
	left := &Value{IsQuantity: true, Quantity: resource.MustParse("100m")}
	right := &Value{IsQuantity: true, Quantity: resource.MustParse("50m")}

	b.Run("Add", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			_ = calc.add(left, right)
		}
	})

	b.Run("Sub", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			_ = calc.sub(left, right)
		}
	})

	b.Run("Mul", func(b *testing.B) {
		b.ReportAllocs()
		numValue := &Value{IsQuantity: false, Number: 2}
		for i := 0; i < b.N; i++ {
			_ = calc.mul(left, numValue)
		}
	})

	b.Run("Div", func(b *testing.B) {
		b.ReportAllocs()
		numValue := &Value{IsQuantity: false, Number: 2}
		for i := 0; i < b.N; i++ {
			_ = calc.div(left, numValue)
		}
	})
}

// BenchmarkFunctionCalls benchmarks max and min function performance
func BenchmarkFunctionCalls(b *testing.B) {
	calc := NewCalculator()
	args := []*Value{
		{IsQuantity: true, Quantity: resource.MustParse("100m")},
		{IsQuantity: true, Quantity: resource.MustParse("200m")},
	}

	b.Run("Max", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			_ = calc.maxFunc(args)
		}
	})

	b.Run("Min", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			_ = calc.minFunc(args)
		}
	})
}

// BenchmarkParseMemoryAllocation benchmarks memory allocation patterns
func BenchmarkParseMemoryAllocation(b *testing.B) {
	expressions := []string{
		"2 + 3",
		"100m + 50m",
		"max(cpu * 50%, 50m)",
		"(100m + 50m) * 2 - 50m",
		"max(min(100m, 200m), 150m)",
	}

	variables := map[string]*Value{
		"cpu": {IsQuantity: true, Quantity: resource.MustParse("200m")},
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		expr := expressions[i%len(expressions)]
		if strings.Contains(expr, "cpu") {
			_, _ = ParseWithVariables(expr, variables)
		} else {
			_, _ = Parse(expr)
		}
	}
}

// BenchmarkConcurrentParsing benchmarks concurrent parsing performance
func BenchmarkConcurrentParsing(b *testing.B) {
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_, _ = Parse("max(100m * 50%, 50m)")
		}
	})
}
