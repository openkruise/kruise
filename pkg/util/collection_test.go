package util

import (
	"reflect"
	"testing"
)

// TestFilterInt tests the Filter function with integer slices
func TestFilterInt(t *testing.T) {
	tests := []struct {
		name     string
		input    []int
		remove   func(int) bool
		expected []int
	}{
		{
			name:     "Empty slice",
			input:    []int{},
			remove:   func(x int) bool { return x > 0 },
			expected: []int{},
		},
		{
			name:     "All elements filtered out",
			input:    []int{1, 2, 3, 4, 5},
			remove:   func(x int) bool { return x > 0 },
			expected: []int{},
		},
		{
			name:     "No elements filtered out",
			input:    []int{1, 2, 3, 4, 5},
			remove:   func(x int) bool { return x < 0 },
			expected: []int{1, 2, 3, 4, 5},
		},
		{
			name:     "Partial filtering - integers",
			input:    []int{1, -2, 3, -4, 5},
			remove:   func(x int) bool { return x < 0 },
			expected: []int{1, 3, 5},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := Filter(tt.input, tt.remove)
			if !reflect.DeepEqual(result, tt.expected) {
				t.Errorf("Filter() = %v, want %v", result, tt.expected)
			}
		})
	}
}

// TestFilterString tests the Filter function with string slices
func TestFilterString(t *testing.T) {
	tests := []struct {
		name     string
		input    []string
		remove   func(string) bool
		expected []string
	}{
		{
			name:     "String filtering by length",
			input:    []string{"a", "bb", "ccc", "dd"},
			remove:   func(x string) bool { return len(x) > 2 },
			expected: []string{"a", "bb", "dd"},
		},
		{
			name:     "Empty string slice",
			input:    []string{},
			remove:   func(x string) bool { return len(x) > 0 },
			expected: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := Filter(tt.input, tt.remove)
			if !reflect.DeepEqual(result, tt.expected) {
				t.Errorf("Filter() = %v, want %v", result, tt.expected)
			}
		})
	}
}

// TestFilterFloat64 tests the Filter function with float64 slices
func TestFilterFloat64(t *testing.T) {
	tests := []struct {
		name     string
		input    []float64
		remove   func(float64) bool
		expected []float64
	}{
		{
			name:     "Float filtering",
			input:    []float64{1.1, -2.2, 3.3, 0.0, -1.1},
			remove:   func(x float64) bool { return x <= 0 },
			expected: []float64{1.1, 3.3},
		},
		{
			name:     "All positive floats",
			input:    []float64{1.1, 2.2, 3.3},
			remove:   func(x float64) bool { return x < 0 },
			expected: []float64{1.1, 2.2, 3.3},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := Filter(tt.input, tt.remove)
			if !reflect.DeepEqual(result, tt.expected) {
				t.Errorf("Filter() = %v, want %v", result, tt.expected)
			}
		})
	}
}

// TestFilterWithStruct tests filtering with struct types
func TestFilterWithStruct(t *testing.T) {
	// Define a test struct
	type Person struct {
		Name string
		Age  int
	}

	// Test data
	people := []Person{
		{Name: "Alice", Age: 25},
		{Name: "Bob", Age: 17},
		{Name: "Charlie", Age: 30},
		{Name: "David", Age: 15},
	}

	// Filter out minors (age < 18)
	adults := Filter(people, func(p Person) bool {
		return p.Age < 18
	})

	// Expected result
	expected := []Person{
		{Name: "Alice", Age: 25},
		{Name: "Charlie", Age: 30},
	}

	// Verify result
	if !reflect.DeepEqual(adults, expected) {
		t.Errorf("Filter() = %v, want %v", adults, expected)
	}
}

// TestFilterNilSlice tests behavior with nil slice
func TestFilterNilSlice(t *testing.T) {
	var nilSlice []int

	result := Filter(nilSlice, func(x int) bool { return x > 0 })

	if len(result) != 0 {
		t.Errorf("Filter() returned slice with length %d, want 0", len(result))
	}
}

// BenchmarkFilter benchmarks the Filter function performance
func BenchmarkFilter(b *testing.B) {
	// Create test data
	data := make([]int, 10000)
	for i := 0; i < len(data); i++ {
		data[i] = i
	}

	// Reset timer and run benchmark
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		Filter(data, func(x int) bool { return x%2 == 0 }) // Filter even numbers
	}
}
