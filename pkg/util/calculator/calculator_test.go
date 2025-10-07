package calculator

import (
	"fmt"
	"strings"
	"testing"

	"k8s.io/apimachinery/pkg/api/resource"
)

func TestParseNumber(t *testing.T) {
	tests := []struct {
		input    string
		expected float64
		wantErr  bool
	}{
		{"42", 42, false},
		{"3.14", 3.14, false},
		{"50%", 0.5, false},
		{"100.5%", 1.005, false},
		{"invalid", 0, true},
	}

	calc := NewCalculator()
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, err := calc.parseNumber(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("parseNumber() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && got != tt.expected {
				t.Errorf("parseNumber() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestBasicArithmetic(t *testing.T) {
	tests := []struct {
		expr     string
		expected string
		wantErr  bool
	}{
		// Basic arithmetic operations
		{"2 + 3", "5", false},
		{"10 - 4", "6", false},
		{"3 * 4", "12", false},
		{"15 / 3", "5", false},
		{"(2 + 3) * 4", "20", false},

		// Percentages
		{"50%", "0.5", false},
		{"100% * 2", "2", false},

		// Function calls - only max and min are supported
		{"max(10, 20)", "20", false},
		{"min(10, 20)", "10", false},

		// Error cases
		{"10 / 0", "", true},
		{"unknown(1, 2)", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.expr, func(t *testing.T) {
			result, err := Parse(tt.expr)
			fmt.Printf("tt.expr: %v\n", tt.expr)
			fmt.Printf("result: %v\n", result)
			if err != nil {
				fmt.Printf("error: %v\n", err)
			} else if result == nil {
				fmt.Printf("error: result is nil\n")
			}

			if (err != nil) != tt.wantErr {
				t.Errorf("Parse() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && result.GetResult().String() != tt.expected {
				t.Errorf("Parse() = %v, want %v", result.GetResult().String(), tt.expected)
			}
		})
	}
}

func TestQuantityOperations(t *testing.T) {
	tests := []struct {
		expr     string
		expected string
		wantErr  bool
	}{
		// Basic Quantity operations
		{"40m + 20m", "60m", false},
		{"100Mi - 50Mi", "50Mi", false},
		{"2 * 40m", "80m", false},
		{"100m / 2", "50m", false},

		// Mixed operations
		{"max(40m, 20)", "20", false}, // 40m (0.04) < 20 (base unit)
		{"min(40m, 20m)", "20m", false},

		// Error cases
		{"40m * 2m", "", true}, // Two Quantities multiplied, invalid
		{"2 / 40m", "", true},  // Quantity as divisor, invalid
	}

	for _, tt := range tests {
		t.Run(tt.expr, func(t *testing.T) {
			result, err := Parse(tt.expr)
			if (err != nil) != tt.wantErr {
				t.Errorf("Parse() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && result != nil && result.GetResult().String() != tt.expected {
				t.Errorf("Parse() = %v, want %v", result.GetResult().String(), tt.expected)
			}
		})
	}
}

func TestValueString(t *testing.T) {
	tests := []struct {
		name     string
		value    *Value
		expected string
	}{
		{
			name:     "number value",
			value:    &Value{IsQuantity: false, Number: 42.5},
			expected: "42.5",
		},
		{
			name: "quantity value",
			value: func() *Value {
				calc := NewCalculator()
				q, _ := calc.parseQuantity("100m")
				return &Value{IsQuantity: true, Quantity: q}
			}(),
			expected: "100m",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.value.String(); got != tt.expected {
				t.Errorf("Value.String() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestToQuantity(t *testing.T) {
	tests := []struct {
		name  string
		value *Value
	}{
		{
			name:  "number to quantity",
			value: &Value{IsQuantity: false, Number: 100},
		},
		{
			name: "quantity to quantity",
			value: func() *Value {
				calc := NewCalculator()
				q, _ := calc.parseQuantity("50m")
				return &Value{IsQuantity: true, Quantity: q}
			}(),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			calc := NewCalculator()
			q := calc.toQuantity(tt.value)
			// Verify the returned Quantity is valid
			if q.IsZero() && tt.value.Number != 0 && !tt.value.IsQuantity {
				t.Error("toQuantity() returned zero for non-zero input")
			}
		})
	}
}

func TestArithmeticFunctions(t *testing.T) {
	// Test addition
	t.Run("add function", func(t *testing.T) {
		calc := NewCalculator()
		left := &Value{IsQuantity: false, Number: 10}
		right := &Value{IsQuantity: false, Number: 20}
		result := calc.add(left, right)
		if result.Number != 30 {
			t.Errorf("add() = %v, want 30", result.Number)
		}
	})

	// Test subtraction
	t.Run("sub function", func(t *testing.T) {
		calc := NewCalculator()
		left := &Value{IsQuantity: false, Number: 20}
		right := &Value{IsQuantity: false, Number: 10}
		result := calc.sub(left, right)
		if result.Number != 10 {
			t.Errorf("sub() = %v, want 10", result.Number)
		}
	})

	// Test multiplication
	t.Run("mul function", func(t *testing.T) {
		calc := NewCalculator()
		left := &Value{IsQuantity: false, Number: 5}
		right := &Value{IsQuantity: false, Number: 3}
		result := calc.mul(left, right)
		if result.Number != 15 {
			t.Errorf("mul() = %v, want 15", result.Number)
		}
	})

	// Test division
	t.Run("div function", func(t *testing.T) {
		calc := NewCalculator()
		left := &Value{IsQuantity: false, Number: 20}
		right := &Value{IsQuantity: false, Number: 4}
		result := calc.div(left, right)
		if result.Number != 5 {
			t.Errorf("div() = %v, want 5", result.Number)
		}
	})
}

func TestFunctionCalls(t *testing.T) {
	// Test max function
	t.Run("max function", func(t *testing.T) {
		calc := NewCalculator()
		args := []*Value{
			{IsQuantity: false, Number: 10},
			{IsQuantity: false, Number: 20},
		}
		result := calc.maxFunc(args)
		if result.Number != 20 {
			t.Errorf("maxFunc() = %v, want 20", result.Number)
		}
	})

	// Test min function
	t.Run("min function", func(t *testing.T) {
		calc := NewCalculator()
		args := []*Value{
			{IsQuantity: false, Number: 10},
			{IsQuantity: false, Number: 20},
		}
		result := calc.minFunc(args)
		if result.Number != 10 {
			t.Errorf("minFunc() = %v, want 10", result.Number)
		}
	})
}

func TestErrorCases(t *testing.T) {
	// Test division by zero
	t.Run("division by zero", func(t *testing.T) {
		_, err := Parse("10 / 0")
		if err == nil {
			t.Error("expected error for division by zero")
		}
		if err != nil && !strings.Contains(err.Error(), "division by zero") {
			t.Errorf("expected division by zero error, got: %v", err)
		}
	})

	// Test multiplication of two Quantities
	t.Run("quantity multiplication", func(t *testing.T) {
		_, err := Parse("40m * 2m")
		if err == nil {
			t.Error("expected error for quantity multiplication")
		}
		if err != nil && !strings.Contains(err.Error(), "multiplication of two quantities") {
			t.Errorf("expected quantity multiplication error, got: %v", err)
		}
	})

	// Test division by Quantity
	t.Run("division by quantity", func(t *testing.T) {
		_, err := Parse("2 / 40m")
		if err == nil {
			t.Error("expected error for division by quantity")
		}
		if err != nil && !strings.Contains(err.Error(), "division by quantity") {
			t.Errorf("expected division by quantity error, got: %v", err)
		}
	})
}

func TestVariables(t *testing.T) {
	tests := []struct {
		name      string
		expr      string
		variables map[string]*Value
		expected  string
		wantErr   bool
	}{
		{
			name: "simple variable",
			expr: "cpu",
			variables: map[string]*Value{
				"cpu": &Value{IsQuantity: true, Quantity: func() resource.Quantity { calc := NewCalculator(); q, _ := calc.parseQuantity("100m"); return q }()},
			},
			expected: "100m",
			wantErr:  false,
		},
		{
			name: "variable in expression",
			expr: "cpu * 2",
			variables: map[string]*Value{
				"cpu": &Value{IsQuantity: true, Quantity: func() resource.Quantity { calc := NewCalculator(); q, _ := calc.parseQuantity("50m"); return q }()},
			},
			expected: "100m",
			wantErr:  false,
		},
		{
			name: "max with variable",
			expr: "max(cpu, 200m)",
			variables: map[string]*Value{
				"cpu": &Value{IsQuantity: true, Quantity: func() resource.Quantity { calc := NewCalculator(); q, _ := calc.parseQuantity("100m"); return q }()},
			},
			expected: "200m",
			wantErr:  false,
		},
		{
			name:      "undefined variable",
			expr:      "undefined_var",
			variables: map[string]*Value{},
			expected:  "",
			wantErr:   true,
		},
		{
			name: "memory variable",
			expr: "memory + 100Mi",
			variables: map[string]*Value{
				"memory": &Value{IsQuantity: true, Quantity: func() resource.Quantity { calc := NewCalculator(); q, _ := calc.parseQuantity("200Mi"); return q }()},
			},
			expected: "300Mi",
			wantErr:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ParseWithVariables(tt.expr, tt.variables)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseWithVariables() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && result != nil && result.GetResult().String() != tt.expected {
				t.Errorf("ParseWithVariables() = %v, want %v", result.GetResult().String(), tt.expected)
			}
		})
	}
}

func TestNegativeValues(t *testing.T) {
	tests := []struct {
		name     string
		expr     string
		expected string
		wantErr  bool
		errMsg   string
	}{
		{
			name:     "negative number",
			expr:     "-10",
			expected: "",
			wantErr:  true,
			errMsg:   "negative",
		},
		{
			name:     "negative quantity",
			expr:     "-100m",
			expected: "",
			wantErr:  true,
			errMsg:   "negative",
		},
		{
			name:     "subtraction resulting in negative",
			expr:     "10 - 20",
			expected: "",
			wantErr:  true,
			errMsg:   "negative",
		},
		{
			name:     "quantity subtraction resulting in negative",
			expr:     "50m - 100m",
			expected: "",
			wantErr:  true,
			errMsg:   "negative",
		},
		{
			name:     "positive result",
			expr:     "20 - 10",
			expected: "10",
			wantErr:  false,
			errMsg:   "",
		},
		{
			name:     "zero result",
			expr:     "10 - 10",
			expected: "0",
			wantErr:  false,
			errMsg:   "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := Parse(tt.expr)
			if (err != nil) != tt.wantErr {
				t.Errorf("Parse() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr && err != nil && !strings.Contains(err.Error(), tt.errMsg) {
				t.Errorf("Parse() error = %v, expected error containing '%s'", err, tt.errMsg)
			}
			if !tt.wantErr && result != nil && result.GetResult().String() != tt.expected {
				t.Errorf("Parse() = %v, want %v", result.GetResult().String(), tt.expected)
			}
		})
	}
}

func TestProposalScenarios(t *testing.T) {
	tests := []struct {
		name      string
		expr      string
		variables map[string]*Value
		expected  string
		wantErr   bool
	}{
		{
			name: "Story 1 - max(cpu*50%, 50m)",
			expr: "max(cpu * 50%, 50m)",
			variables: map[string]*Value{
				"cpu": &Value{IsQuantity: true, Quantity: resource.MustParse("200m")},
			},
			expected: "100m",
			wantErr:  false,
		},
		{
			name: "Story 2 - sum of container resources",
			expr: "(cpu1 + cpu2) * 50%",
			variables: map[string]*Value{
				"cpu1": &Value{IsQuantity: true, Quantity: resource.MustParse("200m")},
				"cpu2": &Value{IsQuantity: true, Quantity: resource.MustParse("400m")},
			},
			expected: "300m",
			wantErr:  false,
		},
		{
			name: "Story 3 - max of container resources",
			expr: "max(cpu1, cpu2) * 50%",
			variables: map[string]*Value{
				"cpu1": &Value{IsQuantity: true, Quantity: resource.MustParse("200m")},
				"cpu2": &Value{IsQuantity: true, Quantity: resource.MustParse("400m")},
			},
			expected: "200m",
			wantErr:  false,
		},
		{
			name: "Negative result should fail",
			expr: "cpu - 100m",
			variables: map[string]*Value{
				"cpu": &Value{IsQuantity: true, Quantity: resource.MustParse("50m")},
			},
			expected: "",
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ParseWithVariables(tt.expr, tt.variables)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseWithVariables() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && result != nil && result.GetResult().String() != tt.expected {
				t.Errorf("ParseWithVariables() = %v, want %v", result.GetResult().String(), tt.expected)
			}
		})
	}
}

func TestSetVariables(t *testing.T) {
	calc := NewCalculator()

	// Test setting variables
	variables := map[string]*Value{
		"cpu":    {IsQuantity: true, Quantity: resource.MustParse("100m")},
		"memory": {IsQuantity: true, Quantity: resource.MustParse("512Mi")},
	}

	calc.SetVariables(variables)

	// Verify variables are set correctly
	if len(calc.variables) != 2 {
		t.Errorf("Expected 2 variables, got %d", len(calc.variables))
	}

	// Test setting nil variables
	calc.SetVariables(nil)
	if len(calc.variables) != 0 {
		t.Errorf("Expected 0 variables after setting nil, got %d", len(calc.variables))
	}

	// Test setting empty variables
	calc.SetVariables(make(map[string]*Value))
	if len(calc.variables) != 0 {
		t.Errorf("Expected 0 variables after setting empty map, got %d", len(calc.variables))
	}
}

func TestCalculatorInstanceMethods(t *testing.T) {
	// Test GetExpression
	calc, err := Parse("2 + 3")
	if err != nil {
		t.Fatalf("Failed to parse: %v", err)
	}

	if calc.GetExpression() != "2 + 3" {
		t.Errorf("GetExpression: got %s, want 2 + 3", calc.GetExpression())
	}

	// Test GetVariables with no variables
	if len(calc.GetVariables()) != 0 {
		t.Errorf("GetVariables: expected empty map, got %v", calc.GetVariables())
	}

	// Test GetVariables with variables
	variables := map[string]*Value{
		"cpu": {IsQuantity: true, Quantity: resource.MustParse("100m")},
	}
	calc2, err := ParseWithVariables("cpu", variables)
	if err != nil {
		t.Fatalf("Failed to parse with variables: %v", err)
	}

	vars := calc2.GetVariables()
	if len(vars) != 1 {
		t.Errorf("GetVariables: expected 1 variable, got %d", len(vars))
	}

	if val, exists := vars["cpu"]; !exists || val.Quantity.String() != "100m" {
		t.Errorf("GetVariables: cpu variable not found or incorrect")
	}
}

func TestEdgeCases(t *testing.T) {
	tests := []struct {
		name    string
		expr    string
		wantErr bool
		errMsg  string
	}{
		{
			name:    "empty expression",
			expr:    "",
			wantErr: true,
		},
		{
			name:    "whitespace only",
			expr:    "   ",
			wantErr: true,
		},
		{
			name:    "invalid character",
			expr:    "2 + @",
			wantErr: true,
		},
		{
			name:    "unclosed parenthesis",
			expr:    "(2 + 3",
			wantErr: true,
		},
		{
			name:    "empty parenthesis",
			expr:    "()",
			wantErr: true,
		},
		{
			name:    "nested functions",
			expr:    "max(min(10, 20), 15)",
			wantErr: false,
		},
		{
			name:    "scientific notation",
			expr:    "1e3 + 500",
			wantErr: false,
		},
		{
			name:    "scientific notation with decimal",
			expr:    "1.5e2 * 2",
			wantErr: false,
		},
		{
			name:    "very large number",
			expr:    "999999999999999999 + 1",
			wantErr: false,
		},
		{
			name:    "very small number",
			expr:    "0.000000000000000001 + 0.000000000000000002",
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			calc, err := Parse(tt.expr)
			if (err != nil) != tt.wantErr {
				t.Errorf("Parse() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && calc != nil {
				result := calc.GetResult()
				if result == nil {
					t.Errorf("Parse() returned nil result for valid expression")
				}
			}
		})
	}
}

func TestParserEdgeCases(t *testing.T) {
	tests := []struct {
		name    string
		expr    string
		want    string
		wantErr bool
	}{
		{
			name: "multiple spaces",
			expr: "2   +   3",
			want: "5",
		},
		{
			name: "tabs and spaces",
			expr: "2\t+\t3",
			want: "5",
		},
		{
			name: "newlines",
			expr: "2\n+\n3",
			want: "5",
		},
		{
			name: "mixed whitespace",
			expr: "  2  \t  +  \n  3  ",
			want: "5",
		},
		{
			name: "function with extra spaces",
			expr: "max(  10  ,  20  )",
			want: "20",
		},
		{
			name: "parentheses with spaces",
			expr: "( 2 + 3 ) * 4",
			want: "20",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			calc, err := Parse(tt.expr)
			if (err != nil) != tt.wantErr {
				t.Errorf("Parse() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr {
				result := calc.GetResult()
				if result.String() != tt.want {
					t.Errorf("Parse() = %v, want %v", result.String(), tt.want)
				}
			}
		})
	}
}

func TestQuantityEdgeCases(t *testing.T) {
	tests := []struct {
		name    string
		expr    string
		want    string
		wantErr bool
	}{
		{
			name: "zero quantity",
			expr: "0m + 0m",
			want: "0",
		},
		{
			name: "large quantity",
			expr: "1000Gi + 500Gi",
			want: "1500Gi",
		},
		{
			name: "decimal quantity",
			expr: "1.5Gi * 2",
			want: "3Gi",
		},
		{
			name: "quantity with different units",
			expr: "1Gi + 100Mi",
			want: "1124Mi",
		},
		{
			name: "max with same quantities",
			expr: "max(100m, 100m)",
			want: "100m",
		},
		{
			name: "min with same quantities",
			expr: "min(100m, 100m)",
			want: "100m",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			calc, err := Parse(tt.expr)
			if (err != nil) != tt.wantErr {
				t.Errorf("Parse() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr {
				result := calc.GetResult()
				if result.String() != tt.want {
					t.Errorf("Parse() = %v, want %v", result.String(), tt.want)
				}
			}
		})
	}
}

func TestFunctionArgumentErrors(t *testing.T) {
	tests := []struct {
		name    string
		expr    string
		wantErr bool
		errMsg  string
	}{
		{
			name:    "function with no arguments",
			expr:    "max()",
			wantErr: true,
		},
		{
			name:    "function with one argument",
			expr:    "max(10)",
			wantErr: true,
		},
		{
			name:    "function with three arguments",
			expr:    "max(10, 20, 30)",
			wantErr: true,
		},
		{
			name:    "function with nested error",
			expr:    "max(10, 20 / 0)",
			wantErr: true,
		},
		{
			name:    "function with invalid nested function",
			expr:    "max(10, unknown(20, 30))",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Parse(tt.expr)
			if (err != nil) != tt.wantErr {
				t.Errorf("Parse() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr && tt.errMsg != "" && err != nil {
				if !strings.Contains(err.Error(), tt.errMsg) {
					t.Errorf("Expected error containing '%s', got '%s'", tt.errMsg, err.Error())
				}
			}
		})
	}
}

func TestComplexNestedExpressions(t *testing.T) {
	tests := []struct {
		name    string
		expr    string
		want    string
		wantErr bool
	}{
		{
			name: "deeply nested max/min",
			expr: "max(min(max(10, 20), 15), 25)",
			want: "25",
		},
		{
			name: "complex arithmetic with resources",
			expr: "(100m + 50m) * 2 - 50m",
			want: "250m",
		},
		{
			name: "percentage in function",
			expr: "max(100m * 50%, 30m)",
			want: "50m",
		},
		{
			name: "multiple operations",
			expr: "max(10 + 5, 20 - 5) * 2",
			want: "30",
		},
		{
			name: "resource chain operations",
			expr: "max(100m, min(200m, 150m)) + 50m",
			want: "200m",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			calc, err := Parse(tt.expr)
			if (err != nil) != tt.wantErr {
				t.Errorf("Parse() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr {
				result := calc.GetResult()
				if result.String() != tt.want {
					t.Errorf("Parse() = %v, want %v", result.String(), tt.want)
				}
			}
		})
	}
}
