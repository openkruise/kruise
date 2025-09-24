package main

import (
	"fmt"
	"log"

	"github.com/openkruise/kruise/pkg/util/calculator"
	"k8s.io/apimachinery/pkg/api/resource"
)

func main() {
	fmt.Println("=== Kruise Calculator Demo ===")
	fmt.Println()

	// Basic arithmetic operations examples
	fmt.Println("1. Basic arithmetic operations:")
	examples := []string{
		"2 + 3 * 4",
		"10 - 4",
		"3 * 4",
		"12 / 3",
		"(2 + 3) * 4",
		"5 / 2",
	}

	for _, expr := range examples {
		result, err := calculator.Parse(expr)
		if err != nil {
			log.Printf("Error parsing %s: %v", expr, err)
		} else {
			fmt.Printf("  %s = %s\n", expr, result.GetResult().String())
		}
	}
	fmt.Println()

	// Percentage operations
	fmt.Println("2. Percentage operations:")
	percentExamples := []string{
		"50%",
		"50% * 200",
		"100% + 50%",
		"50% / 3",
	}

	for _, expr := range percentExamples {
		result, err := calculator.Parse(expr)
		if err != nil {
			log.Printf("Error parsing %s: %v", expr, err)
		} else {
			fmt.Printf("  %s = %s\n", expr, result.GetResult().String())
		}
	}
	fmt.Println()

	// Function calls - only max and min are supported
	fmt.Println("3. Function calls:")
	funcExamples := []string{
		"max(40, 20)",
		"min(40, 20)",
		"max(100m, 200m)",
		"min(100m, 200m)",
		"max(100m, 2)",
		"min(100m, 2)",
		"max(1Gi, 2Gi)",
		"min(1Gi, 2Gi)",
		"max(1Gi, 1Mi)",
		"min(1Gi, 1Mi)",
		"max(1Gi, 1)",
		"min(1Gi, 1)",
	}

	for _, expr := range funcExamples {
		result, err := calculator.Parse(expr)
		if err != nil {
			log.Printf("Error parsing %s: %v", expr, err)
		} else if result != nil {
			fmt.Printf("  %s = %s\n", expr, result.GetResult().String())
		} else {
			fmt.Printf("  %s: no result\n", expr)
		}
	}
	fmt.Println()

	// Kubernetes resource operations
	fmt.Println("4. Kubernetes resource operations:")
	resourceExamples := []string{
		"max(40m, 20)",      // max(40m, 20) -> 40m
		"40m * 2",           // 40m * 2 -> 80m
		"100m + 50m",        // 100m + 50m -> 150m
		"100m - 50m",        // 100m - 50m -> 50m
		"100m / 2",          // 100m / 2 -> 50m
		"max(100Mi, 200Mi)", // max(100Mi, 200Mi) -> 200Mi
		"max(max(min(100m, 200m), 50m) * 50% + 1, 550m) / 4 ", // == 1050m / 4 -> 262.5m
		"(1Gi + 1Mi) / 2 * 200%",                              // (1Gi + 1Mi) / 2 * 200% -> 1025Mi
	}

	for _, expr := range resourceExamples {
		result, err := calculator.Parse(expr)
		if err != nil {
			log.Printf("Error parsing %s: %v", expr, err)
		} else if result != nil {
			fmt.Printf("  %s = %s\n", expr, result.GetResult().String())
		} else {
			fmt.Printf("  %s: no result\n", expr)
		}
	}
	fmt.Println()

	// Variable support
	fmt.Println("5. Variable support:")
	variables := map[string]*calculator.Value{
		"cpu":    {IsQuantity: true, Quantity: resource.MustParse("200")},
		"memory": {IsQuantity: true, Quantity: resource.MustParse("512Mi")},
		"other":  {IsQuantity: true, Quantity: resource.MustParse("1")},
	}

	varExamples := []string{
		"50",
		"50 + other",
		"cpu",
		"cpu * 50%",
		"max(cpu, 100m)",
		"min(cpu, 100m)",
		"max(cpu * 50%, 50m)",
		"min(cpu * 50%, 50m)",
		"memory",
		"memory * 50%",
		"max(memory * 50%, 256Mi)",
		"min(memory * 50%, 256Mi)",
		"max(cpu * 50%, 500m) * 200% / 200% - 100m",
	}

	for _, expr := range varExamples {
		result, err := calculator.ParseWithVariables(expr, variables)
		if err != nil {
			log.Printf("Error parsing %s: %v", expr, err)
		} else if result != nil {
			fmt.Printf("  %s = %s,isQuantity=%v, Value = %v\n", expr, result.GetResult().String(), result.GetResult().IsQuantity, result.GetResult().Quantity.Value())
		} else {
			fmt.Printf("  %s: no result\n", expr)
		}
	}
	fmt.Println()

	// Negative value check
	fmt.Println("6. Negative value check:")
	negativeExamples := []string{
		"10 - 20",                // Should report error
		"50m - 100m",             // Should report error
		"20 - 10",                // Should succeed
		"max(100m, 200m) - 100m", // Should succeed
		"min(100m, 200m) - 100m", // Should succeed
		"100 + min(100, -50)",    // Should succeed
	}

	for _, expr := range negativeExamples {
		result, err := calculator.Parse(expr)
		if err != nil {
			fmt.Printf("  %s: ✅ Correctly reported error - %v\n", expr, err)
		} else if result != nil {
			fmt.Printf("  %s = %s\n", expr, result.GetResult().String())
		} else {
			fmt.Printf("  %s: no result\n", expr)
		}
	}
	fmt.Println()

	// Error examples
	fmt.Println("7. Error handling:")
	errorExamples := []string{
		"40m * 2m",       // Two Quantities multiplied, invalid
		"40m * 40m * 1m", // Three Quantities multiplied, invalid
		" 40m * 1m / 1m", // Quantity as divisor, invalid
		"2 / 40m",        // Quantity as divisor, invalid
		"10 / 0",         // Division by zero, invalid
	}

	for _, expr := range errorExamples {
		result, err := calculator.Parse(expr)
		if err != nil {
			fmt.Printf("  %s: ✅ Correctly reported error - %v\n", expr, err)
		} else if result != nil {
			fmt.Printf("  %s: ❌ Should have reported error but got result: %s\n", expr, result.GetResult().String())
		} else {
			fmt.Printf("  %s: ❌ Should have reported error but got nil result\n", expr)
		}
	}
	fmt.Println()

	fmt.Println("=== Demo Complete ===")
}
