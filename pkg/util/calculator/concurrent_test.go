package calculator

import (
	"fmt"
	"sync"
	"testing"

	"k8s.io/apimachinery/pkg/api/resource"
)

func TestConcurrentParsing(t *testing.T) {
	// Test concurrent parsing with different expressions
	expressions := []string{
		"2 + 3 * 4",
		"50% * 200",
		"max(40m, 20)",
		"100m + 50m",
		"(2 + 3) * 4",
		"40m * 2",
		"max(100m, 200m)",
		"min(100m, 200m)",
	}

	var wg sync.WaitGroup
	errors := make(chan error, len(expressions))

	for i, expr := range expressions {
		wg.Add(1)
		go func(index int, expression string) {
			defer wg.Done()

			calc, err := Parse(expression)
			if err != nil {
				errors <- fmt.Errorf("goroutine %d: failed to parse %s: %v", index, expression, err)
				return
			}

			result := calc.GetResult()
			if result == nil {
				errors <- fmt.Errorf("goroutine %d: no result for %s", index, expression)
				return
			}

			t.Logf("Goroutine %d: %s = %s", index, expression, result.String())
		}(i, expr)
	}

	wg.Wait()
	close(errors)

	// Check for errors
	for err := range errors {
		if err != nil {
			t.Error(err)
		}
	}
}

func TestConcurrentParsingWithVariables(t *testing.T) {
	// Test concurrent parsing with variables
	variables := map[string]*Value{
		"cpu":    {IsQuantity: false, Number: 100},
		"memory": {IsQuantity: true, Quantity: resource.MustParse("512Mi")},
	}

	expressions := []string{
		"cpu * 2",
		"memory * 50%",
		"max(cpu, 50)",
		"min(memory, 1Gi)",
		"cpu + 50",
		"memory - 100Mi",
	}

	var wg sync.WaitGroup
	errors := make(chan error, len(expressions))

	for i, expr := range expressions {
		wg.Add(1)
		go func(index int, expression string) {
			defer wg.Done()

			calc, err := ParseWithVariables(expression, variables)
			if err != nil {
				errors <- fmt.Errorf("goroutine %d: failed to parse %s: %v", index, expression, err)
				return
			}

			result := calc.GetResult()
			if result == nil {
				errors <- fmt.Errorf("goroutine %d: no result for %s", index, expression)
				return
			}

			t.Logf("Goroutine %d: %s = %s", index, expression, result.String())
		}(i, expr)
	}

	wg.Wait()
	close(errors)

	// Check for errors
	for err := range errors {
		if err != nil {
			t.Error(err)
		}
	}
}

func TestResourceIsolation(t *testing.T) {
	// Test that different calculator instances are isolated
	calc1, err := Parse("2 + 3")
	if err != nil {
		t.Fatalf("Failed to create calc1: %v", err)
	}

	calc2, err := Parse("5 * 4")
	if err != nil {
		t.Fatalf("Failed to create calc2: %v", err)
	}

	// Verify results are different and correct
	result1 := calc1.GetResult()
	result2 := calc2.GetResult()

	if result1.String() != "5" {
		t.Errorf("calc1 result: got %s, want 5", result1.String())
	}

	if result2.String() != "20" {
		t.Errorf("calc2 result: got %s, want 20", result2.String())
	}

	// Verify expressions are stored correctly
	if calc1.expr != "2 + 3" {
		t.Errorf("calc1 expr: got %s, want 2 + 3", calc1.expr)
	}

	if calc2.expr != "5 * 4" {
		t.Errorf("calc2 expr: got %s, want 5 * 4", calc2.expr)
	}
}
