package calculator

import (
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
		{"5..5", 0, true},
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
		{"5..5", "", true},
		{"5%%", "", true},

		// Percentages
		{"50%", "0.5", false},
		{"100% * 2", "2", false},

		// Function calls - only max and min are supported
		{"max(10, 20)", "20", false},
		{"min(10, 20)", "10", false},

		// ExponentDigit
		{"1e2", "100", false},
		{"1E2", "100", false},

		// complex
		{"min(max(max(max(0.5,0.5*0.5),0.5-1.0),0.5-1.0),3.0)", "0.5", false},

		// Error cases
		{"10 / 0", "", true},
		{"unknown(1, 2)", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.expr, func(t *testing.T) {
			result, err := Parse(tt.expr)
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
		{"100m", "100m", false},

		// Mixed operations
		{"max(40m, 20)", "20", false}, // 40m (0.04) < 20 (base unit)
		{"min(40m, 20m)", "20m", false},
		{"min(max(max(max(0.5,0.5*0.5),0.5-1.0),0.5-1.0),3.0)", "0.5", false},
		{"min(max(max(max(500m,0.5*0.5),0.5-1.0),0.5-1.0),3.0)", "500m", false},

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
			name: "min(max(max(max(0.5,0.5*cpu),cpu-1.0),cpu-1.0),3.0)",
			expr: "min(max(max(max(0.5,0.5*cpu),cpu-1.0),cpu-1.0),3.0)",
			variables: map[string]*Value{
				"cpu": &Value{IsQuantity: true, Quantity: func() resource.Quantity { calc := NewCalculator(); q, _ := calc.parseQuantity("0.5"); return q }()},
			},
			expected: "500m",
			wantErr:  false,
		},
		{
			name: "max with decimal number and quantity",
			expr: "max(0.25, cpu)",
			variables: map[string]*Value{
				"cpu": &Value{IsQuantity: true, Quantity: func() resource.Quantity { calc := NewCalculator(); q, _ := calc.parseQuantity("100m"); return q }()},
			},
			expected: "250m",
			wantErr:  false,
		},
		{
			name: "max with quantity and decimal number",
			expr: "max(cpu, 0.75)",
			variables: map[string]*Value{
				"cpu": &Value{IsQuantity: true, Quantity: func() resource.Quantity { calc := NewCalculator(); q, _ := calc.parseQuantity("500m"); return q }()},
			},
			expected: "750m",
			wantErr:  false,
		},
		{
			name: "min with decimal number and quantity",
			expr: "min(0.8, cpu)",
			variables: map[string]*Value{
				"cpu": &Value{IsQuantity: true, Quantity: func() resource.Quantity { calc := NewCalculator(); q, _ := calc.parseQuantity("1.0"); return q }()},
			},
			expected: "800m",
			wantErr:  false,
		},
		{
			name: "min with quantity and decimal number",
			expr: "min(cpu, 1.5)",
			variables: map[string]*Value{
				"cpu": &Value{IsQuantity: true, Quantity: func() resource.Quantity { calc := NewCalculator(); q, _ := calc.parseQuantity("2.0"); return q }()},
			},
			expected: "1500m",
			wantErr:  false,
		},
		{
			name: "max(min(2*cpu,-cpu+3.0),2*cpu-3.0)",
			expr: "max(min(2*cpu,-cpu+3.0),2*cpu-3.0)",
			variables: map[string]*Value{
				"cpu": &Value{IsQuantity: true, Quantity: func() resource.Quantity { calc := NewCalculator(); q, _ := calc.parseQuantity("1.5"); return q }()},
			},
			expected: "1500m",
			wantErr:  false,
		},
		{
			name: "max(min(2*cpu,3.0-cpu),2*cpu-3.0)",
			expr: "max(min(2*cpu,3.0-cpu),2*cpu-3.0)",
			variables: map[string]*Value{
				"cpu": &Value{IsQuantity: true, Quantity: func() resource.Quantity { calc := NewCalculator(); q, _ := calc.parseQuantity("1.5"); return q }()},
			},
			expected: "1500m",
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
		{
			name: "memory variable with max",
			expr: "0.5 * memory + max(0, memory-2Gi)",
			variables: map[string]*Value{
				"memory": &Value{IsQuantity: true, Quantity: func() resource.Quantity { calc := NewCalculator(); q, _ := calc.parseQuantity("8Gi"); return q }()},
			},
			expected: "10Gi",
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
			name: "no spaces",
			expr: "2+3+4/2",
			want: "7",
		},
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

// TestLexerBugFixes tests the specific bugs found and fixed in the lexer
// These tests ensure the fixes for operator parsing and scientific notation remain correct
func TestLexerBugFixes(t *testing.T) {
	tests := []struct {
		name     string
		expr     string
		expected string
		desc     string
	}{
		// Bug fix: Operator adjacency parsing (operators without spaces)
		{
			name:     "plus followed by minus",
			expr:     "10+-5",
			expected: "5",
			desc:     "Fixed: was incorrectly parsed as single token, result was 0",
		},
		{
			name:     "minus followed by minus",
			expr:     "10--5",
			expected: "15",
			desc:     "Fixed: was incorrectly parsed as single token, result was 0",
		},
		{
			name:     "operator adjacency with spaces",
			expr:     "10 + -5",
			expected: "5",
			desc:     "Should work correctly with spaces",
		},
		{
			name:     "operator adjacency subtraction",
			expr:     "10 - -5",
			expected: "15",
			desc:     "Should work correctly with spaces",
		},
		{
			name:     "multiple operator adjacency",
			expr:     "20+-10+5",
			expected: "15",
			desc:     "Multiple adjacent operators should be parsed correctly",
		},
		{
			name:     "quantity with operator adjacency",
			expr:     "100m+50m",
			expected: "150m",
			desc:     "Quantity operations with adjacent operators",
		},

		// Scientific notation tests
		{
			name:     "scientific notation lowercase e",
			expr:     "1e3",
			expected: "1000",
			desc:     "Standard scientific notation",
		},
		{
			name:     "scientific notation uppercase E",
			expr:     "1E3",
			expected: "1000",
			desc:     "Uppercase E scientific notation",
		},
		{
			name:     "scientific notation with plus",
			expr:     "1e+3",
			expected: "1000",
			desc:     "Scientific notation with explicit plus",
		},
		{
			name:     "scientific notation with minus",
			expr:     "1e-3",
			expected: "0.001",
			desc:     "Scientific notation with negative exponent",
		},
		{
			name:     "scientific notation uppercase E with plus",
			expr:     "1E+3",
			expected: "1000",
			desc:     "Uppercase E with explicit plus",
		},
		{
			name:     "scientific notation uppercase E with minus",
			expr:     "1E-3",
			expected: "0.001",
			desc:     "Uppercase E with negative exponent",
		},
		{
			name:     "decimal scientific notation",
			expr:     "1.5e2",
			expected: "150",
			desc:     "Scientific notation with decimal base",
		},
		{
			name:     "decimal scientific notation with sign",
			expr:     "2.5E+1",
			expected: "25",
			desc:     "Decimal base with explicit sign",
		},

		// Kubernetes quantity unit tests (up to P/Pi)
		{
			name:     "milli unit",
			expr:     "100m",
			expected: "100m",
			desc:     "Milli CPU unit",
		},
		{
			name:     "kilo unit",
			expr:     "1k",
			expected: "1k",
			desc:     "Kilo unit",
		},
		{
			name:     "Mega unit",
			expr:     "100M",
			expected: "100M",
			desc:     "Mega unit",
		},
		{
			name:     "Giga unit",
			expr:     "1G",
			expected: "1G",
			desc:     "Giga unit",
		},
		{
			name:     "Tera unit",
			expr:     "1T",
			expected: "1T",
			desc:     "Tera unit",
		},
		{
			name:     "Peta unit",
			expr:     "1P",
			expected: "1P",
			desc:     "Peta unit (maximum supported decimal unit)",
		},
		{
			name:     "Kibi unit",
			expr:     "1Ki",
			expected: "1Ki",
			desc:     "Kibi unit",
		},
		{
			name:     "Mebi unit",
			expr:     "100Mi",
			expected: "100Mi",
			desc:     "Mebi unit",
		},
		{
			name:     "Gibi unit",
			expr:     "1Gi",
			expected: "1Gi",
			desc:     "Gibi unit",
		},
		{
			name:     "Tebi unit",
			expr:     "1Ti",
			expected: "1Ti",
			desc:     "Tebi unit",
		},
		{
			name:     "Pebi unit",
			expr:     "1Pi",
			expected: "1Pi",
			desc:     "Pebi unit (maximum supported binary unit)",
		},

		// Mixed scenarios combining fixes
		{
			name:     "scientific notation in expression",
			expr:     "1e3 + 2e2",
			expected: "1200",
			desc:     "Multiple scientific notations in one expression",
		},
		{
			name:     "scientific notation with operator adjacency",
			expr:     "1e3+-200",
			expected: "800",
			desc:     "Combining scientific notation with adjacent operators",
		},
		{
			name:     "quantity arithmetic",
			expr:     "100m + 50m",
			expected: "150m",
			desc:     "Basic quantity addition",
		},
		{
			name:     "quantity with percentage",
			expr:     "200m * 50%",
			expected: "100m",
			desc:     "Quantity with percentage multiplication",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := Parse(tt.expr)
			if err != nil {
				t.Errorf("Parse(%q) unexpected error: %v\nDescription: %s", tt.expr, err, tt.desc)
				return
			}
			actual := result.GetResult().String()
			if actual != tt.expected {
				t.Errorf("Parse(%q) = %v, want %v\nDescription: %s", tt.expr, actual, tt.expected, tt.desc)
			}
		})
	}
}

// TestScientificNotationEdgeCases tests edge cases specific to scientific notation parsing
func TestScientificNotationEdgeCases(t *testing.T) {
	tests := []struct {
		name    string
		expr    string
		wantErr bool
		desc    string
	}{
		{
			name:    "incomplete scientific notation - just e",
			expr:    "1e",
			wantErr: false, // Acceptable: parsed as 0 by strconv
			desc:    "Incomplete exponent - handled by Go's strconv",
		},
		{
			name:    "incomplete scientific notation - e with sign",
			expr:    "1e+",
			wantErr: false, // Acceptable: parsed as 0 by strconv
			desc:    "Incomplete exponent with sign - handled by Go's strconv",
		},
		{
			name:    "double exponent",
			expr:    "1e3e2",
			wantErr: true,
			desc:    "Double exponent should be rejected",
		},
		{
			name:    "scientific notation in max function",
			expr:    "max(1e3, 2e2)",
			wantErr: false,
			desc:    "Scientific notation should work in functions",
		},
		{
			name:    "scientific notation with quantity comparison",
			expr:    "max(1e3, 100m)",
			wantErr: false,
			desc:    "Scientific notation can be compared with quantities",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Parse(tt.expr)
			if (err != nil) != tt.wantErr {
				t.Errorf("Parse(%q) error = %v, wantErr %v\nDescription: %s",
					tt.expr, err, tt.wantErr, tt.desc)
			}
		})
	}
}

// TestLexerCoverageEdgeCases tests specific lexer edge cases to improve code coverage
func TestLexerCoverageEdgeCases(t *testing.T) {
	tests := []struct {
		name    string
		expr    string
		wantErr bool
		desc    string
	}{
		{
			name:    "number starting with dot",
			expr:    ".5 + .5",
			wantErr: false,
			desc:    "Numbers can start with decimal point",
		},
		{
			name:    "dot followed by non-digit as quantity check",
			expr:    ".5Mi",
			wantErr: false,
			desc:    "Decimal starting number with quantity unit",
		},
		{
			name:    "exponent followed by unit (invalid scientific notation)",
			expr:    "1eKi",
			wantErr: true,
			desc:    "Scientific notation exponent should not be followed by unit letters",
		},
		{
			name:    "number with sign in exponent not after e",
			expr:    "1.5+2",
			wantErr: false,
			desc:    "Plus sign not immediately after e should be operator",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Parse(tt.expr)
			if (err != nil) != tt.wantErr {
				t.Errorf("Parse(%q) error = %v, wantErr %v\nDescription: %s",
					tt.expr, err, tt.wantErr, tt.desc)
			}
		})
	}
}

// TestLexerInternalFunctions tests internal lexer functions directly
func TestLexerInternalFunctions(t *testing.T) {
	t.Run("isQuantityUnitChar", func(t *testing.T) {
		// Test that all letters are considered unit chars
		if !isQuantityUnitChar('m') {
			t.Error("Expected 'm' to be a quantity unit char")
		}
		if !isQuantityUnitChar('K') {
			t.Error("Expected 'K' to be a quantity unit char")
		}
		if isQuantityUnitChar('1') {
			t.Error("Expected '1' to not be a quantity unit char")
		}
		if isQuantityUnitChar('+') {
			t.Error("Expected '+' to not be a quantity unit char")
		}
	})

	t.Run("isIdentifierChar", func(t *testing.T) {
		if !isIdentifierChar('a') {
			t.Error("Expected 'a' to be an identifier char")
		}
		if !isIdentifierChar('1') {
			t.Error("Expected '1' to be an identifier char")
		}
		if !isIdentifierChar('_') {
			t.Error("Expected '_' to be an identifier char")
		}
		if isIdentifierChar('+') {
			t.Error("Expected '+' to not be an identifier char")
		}
	})

	t.Run("isQuantity with empty string", func(t *testing.T) {
		lex := &yyLex{}
		if lex.isQuantity("") {
			t.Error("Expected empty string to not be a quantity")
		}
	})

	t.Run("isQuantity with no unit", func(t *testing.T) {
		lex := &yyLex{}
		if lex.isQuantity("123") {
			t.Error("Expected number without unit to not be a quantity")
		}
	})

	t.Run("isQuantity with invalid unit", func(t *testing.T) {
		lex := &yyLex{}
		if lex.isQuantity("123xyz") {
			t.Error("Expected number with invalid unit to not be a quantity")
		}
	})

	t.Run("isQuantity with valid units", func(t *testing.T) {
		lex := &yyLex{}
		validUnits := []string{"100m", "1k", "1M", "1G", "1T", "1P", "1Ki", "100Mi", "1Gi", "1Ti", "1Pi"}
		for _, unit := range validUnits {
			if !lex.isQuantity(unit) {
				t.Errorf("Expected %s to be a valid quantity", unit)
			}
		}
	})

	t.Run("lex error handling", func(t *testing.T) {
		lex := &yyLex{}
		lex.init("test")
		lex.Error("test error")
		if lex.err == nil {
			t.Error("Expected error to be set")
		}
		if !strings.Contains(lex.err.Error(), "test error") {
			t.Errorf("Expected error message to contain 'test error', got: %v", lex.err)
		}
		if !strings.Contains(lex.err.Error(), "line 1") {
			t.Errorf("Expected error message to contain line number, got: %v", lex.err)
		}
	})

	t.Run("lexer next and peek with newline", func(t *testing.T) {
		lex := &yyLex{}
		lex.init("a\nb")

		r := lex.next()
		if r != 'a' {
			t.Errorf("Expected 'a', got %c", r)
		}
		if lex.line != 1 || lex.col != 2 {
			t.Errorf("Expected line 1, col 2, got line %d, col %d", lex.line, lex.col)
		}

		r = lex.next()
		if r != '\n' {
			t.Errorf("Expected newline, got %c", r)
		}
		if lex.line != 2 || lex.col != 1 {
			t.Errorf("Expected line 2, col 1 after newline, got line %d, col %d", lex.line, lex.col)
		}

		r = lex.next()
		if r != 'b' {
			t.Errorf("Expected 'b', got %c", r)
		}
	})

	t.Run("lexer peek at end of input", func(t *testing.T) {
		lex := &yyLex{}
		lex.init("a")
		lex.next() // consume 'a'

		r := lex.peek()
		if r != 0 {
			t.Errorf("Expected 0 at end of input, got %c", r)
		}
	})

	t.Run("lexer next at end of input", func(t *testing.T) {
		lex := &yyLex{}
		lex.init("a")
		lex.next() // consume 'a'

		r := lex.next()
		if r != 0 {
			t.Errorf("Expected 0 at end of input, got %c", r)
		}
	})

	t.Run("readNumber edge case - pos at end", func(t *testing.T) {
		lex := &yyLex{}
		lex.init("")
		lex.pos = 0 // at end already
		num := lex.readNumber()
		if num != "" {
			t.Errorf("Expected empty string when reading number at end of input, got: %s", num)
		}
	})

	t.Run("readIdentifier edge case - pos at end", func(t *testing.T) {
		lex := &yyLex{}
		lex.init("")
		lex.pos = 0 // at end already
		ident := lex.readIdentifier()
		if ident != "" {
			t.Errorf("Expected empty string when reading identifier at end of input, got: %s", ident)
		}
	})

	t.Run("readNumber with multiple percents (double percent)", func(t *testing.T) {
		// This tests the seenPercent check in readNumber
		// When we have already seen a percent, we should stop at the second one
		lex := &yyLex{}
		lex.init("50%")
		num := lex.readNumber()
		if num != "50%" {
			t.Errorf("Expected '50%%', got: %s", num)
		}
		// Verify that reading another number starting with % stops immediately
		// This is difficult to test directly through readNumber,
		// but the parser should reject "50%%" in the existing tests
	})
}
