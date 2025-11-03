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
	"testing"

	fuzz "github.com/AdaLogics/go-fuzz-headers"
	"k8s.io/apimachinery/pkg/api/resource"
)

// FuzzParse tests the Parse function with random input expressions
// This fuzz test aims to discover crashes, panics, or unexpected errors in the parser
func FuzzParse(f *testing.F) {
	// Add seed inputs covering various expression types
	seeds := []string{
		// Basic arithmetic
		"2 + 3",
		"10 - 4",
		"3 * 4",
		"15 / 3",
		"(2 + 3) * 4",

		// Percentages
		"50%",
		"100% * 2",
		"cpu * 50%",

		// Functions
		"max(10, 20)",
		"min(10, 20)",
		"max(cpu, 200m)",
		"min(memory, 100Mi)",

		// Quantities
		"40m + 20m",
		"100Mi - 50Mi",
		"2 * 40m",
		"100m / 2",

		// Complex expressions
		"max(cpu * 50%, 50m)",
		"max(memory * 50%, 100Mi)",
		"(cpu1 + cpu2) * 50%",
		"max(cpu1, cpu2) * 50%",

		// Edge cases
		"",
		"   ",
		"0",
		"0m",
		"0Mi",

		// Scientific notation
		"1e3 + 500",
		"1.5e2 * 2",

		// Nested functions
		"max(min(10, 20), 15)",
		"max(min(max(10, 20), 15), 25)",

		// Large numbers
		"1000Gi + 500Gi",
		"999999999999999999 + 1",

		// Small numbers
		"0.000000000000000001 + 0.000000000000000002",
		"1m + 1m",

		// Mixed whitespace
		"2   +   3",
		"2\t+\t3",
		"2\n+\n3",

		// Potentially problematic inputs
		"10 / 0",
		"-10",
		"-100m",
		"unknown(1, 2)",
		"max()",
		"max(10)",
		"max(10, 20, 30)",
		"(2 + 3",
		"()",
		"2 + @",
	}

	for _, seed := range seeds {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, expr string) {
		// Simply call Parse and ensure it doesn't panic
		// We don't check for errors because fuzz testing is about finding crashes
		_, _ = Parse(expr)
	})
}

// FuzzParseWithVariables tests the ParseWithVariables function with random variables
// This tests the parser's handling of variable substitution
func FuzzParseWithVariables(f *testing.F) {
	// Add seed inputs with variables
	seeds := []struct {
		expr string
		vars []byte
	}{
		{"cpu", []byte{}},
		{"cpu * 2", []byte{}},
		{"max(cpu, 200m)", []byte{}},
		{"memory + 100Mi", []byte{}},
		{"cpu1 + cpu2", []byte{}},
		{"max(cpu1, cpu2) * 50%", []byte{}},
		{"undefined_var", []byte{}},
	}

	for _, seed := range seeds {
		f.Add(seed.expr, seed.vars)
	}

	f.Fuzz(func(t *testing.T, expr string, varsData []byte) {
		// Use go-fuzz-headers to generate structured variable maps
		cf := fuzz.NewConsumer(varsData)

		// Generate a map of variables
		variables := make(map[string]*Value)

		// Try to add some common variable names with fuzzed values
		variableNames := []string{"cpu", "memory", "cpu1", "cpu2", "mem1", "mem2"}

		for _, name := range variableNames {
			// Randomly decide whether to add this variable
			addVar, err := cf.GetBool()
			if err != nil {
				break
			}
			if !addVar {
				continue
			}

			// Randomly decide if this is a Quantity or Number
			isQuantity, err := cf.GetBool()
			if err != nil {
				break
			}

			if isQuantity {
				// Generate a random quantity string
				quantityStr, err := cf.GetString()
				if err != nil {
					break
				}

				// Try to parse it as a quantity
				calc := NewCalculator()
				q, err := calc.parseQuantity(quantityStr)
				if err != nil {
					// If parsing fails, use a default quantity
					q = resource.MustParse("100m")
				}

				variables[name] = &Value{
					IsQuantity: true,
					Quantity:   q,
				}
			} else {
				// Generate a random number
				num, err := cf.GetFloat64()
				if err != nil {
					break
				}

				variables[name] = &Value{
					IsQuantity: false,
					Number:     num,
				}
			}
		}

		// Call ParseWithVariables and ensure it doesn't panic
		_, _ = ParseWithVariables(expr, variables)
	})
}

// FuzzCalculatorParse tests the Calculator.Parse method
// This tests parsing with a pre-configured calculator instance
func FuzzCalculatorParse(f *testing.F) {
	// Add seed inputs
	seeds := []string{
		"2 + 3",
		"cpu * 2",
		"max(cpu, 200m)",
		"(10 + 20) * 3",
		"max(min(10, 20), 15)",
	}

	for _, seed := range seeds {
		f.Add(seed, []byte{})
	}

	f.Fuzz(func(t *testing.T, expr string, varsData []byte) {
		// Create a calculator with fuzzed variables
		cf := fuzz.NewConsumer(varsData)

		variables := make(map[string]*Value)
		variableNames := []string{"cpu", "memory"}

		for _, name := range variableNames {
			addVar, err := cf.GetBool()
			if err != nil {
				break
			}
			if !addVar {
				continue
			}

			isQuantity, err := cf.GetBool()
			if err != nil {
				break
			}

			if isQuantity {
				quantityStr, err := cf.GetString()
				if err != nil {
					break
				}

				calc := NewCalculator()
				q, err := calc.parseQuantity(quantityStr)
				if err != nil {
					q = resource.MustParse("100m")
				}

				variables[name] = &Value{
					IsQuantity: true,
					Quantity:   q,
				}
			} else {
				num, err := cf.GetFloat64()
				if err != nil {
					break
				}

				variables[name] = &Value{
					IsQuantity: false,
					Number:     num,
				}
			}
		}

		// Create calculator and parse
		calc := NewCalculatorWithVariables(variables)
		_, _ = calc.Parse(expr)
	})
}
