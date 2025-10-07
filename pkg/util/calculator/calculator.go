package calculator

import (
	"fmt"
	"sync"

	"k8s.io/apimachinery/pkg/api/resource"
)

// Global variable for yacc parser - use mutex for thread safety
var (
	currentCalc   *Calculator
	currentCalcMu sync.Mutex
)

// Value represents the calculator's value type, can be number or Quantity
type Value struct {
	IsQuantity bool
	Number     float64
	Quantity   resource.Quantity
}

func (v *Value) String() string {
	if v.IsQuantity {
		return v.Quantity.String()
	}
	return fmt.Sprintf("%g", v.Number)
}

// Calculator represents a calculator instance with its own state
type Calculator struct {
	expr      string
	variables map[string]*Value
	result    *Value
	lastError error
}

// NewCalculator creates a new calculator instance
func NewCalculator() *Calculator {
	return &Calculator{
		expr:      "",
		variables: make(map[string]*Value),
		result:    nil,
	}
}

// NewCalculatorWithVariables creates a new calculator instance with variables
func NewCalculatorWithVariables(vars map[string]*Value) *Calculator {
	calc := &Calculator{
		expr:      "",
		variables: make(map[string]*Value),
		result:    nil,
	}
	if vars != nil {
		calc.variables = vars
	}
	return calc
}

// SetVariables sets variables for this calculator instance
func (c *Calculator) SetVariables(vars map[string]*Value) {
	if vars != nil {
		c.variables = vars
	} else {
		c.variables = make(map[string]*Value)
	}
}

// GetResult gets the result of the last calculation
func (c *Calculator) GetResult() *Value {
	return c.result
}

// GetExpression returns the original expression
func (c *Calculator) GetExpression() string {
	return c.expr
}

// GetVariables returns the variable bindings
func (c *Calculator) GetVariables() map[string]*Value {
	return c.variables
}

// Parse parses an expression and returns the result
func (c *Calculator) Parse(input string) (*Value, error) {
	c.expr = input
	c.lastError = nil

	// Use mutex for thread safety
	currentCalcMu.Lock()
	defer currentCalcMu.Unlock()

	// Use yacc-generated parser with lexer
	lexer := &yyLex{}
	lexer.init(input)

	// Set current calculator for yacc parser
	currentCalc = c

	// Parse using yacc
	parseResult := yyParse(lexer)

	if parseResult != 0 {
		// Check for specific errors first
		if c.lastError != nil {
			return nil, c.lastError
		}
		if lexer.err != nil {
			return nil, lexer.err
		}
		// Generic parse error
		return nil, fmt.Errorf("parse error")
	}

	// Get result from yacc parser
	result := c.result
	if result == nil {
		// This shouldn't happen if parse succeeded, but check for errors anyway
		if c.lastError != nil {
			return nil, c.lastError
		}
		return nil, fmt.Errorf("no result from parser")
	}

	// Check for negative values
	if result.IsQuantity {
		if result.Quantity.Sign() < 0 {
			return nil, fmt.Errorf("calculation result is negative: %s", result.Quantity.String())
		}
	} else {
		if result.Number < 0 {
			return nil, fmt.Errorf("calculation result is negative: %g", result.Number)
		}
	}

	// all value must be quantity, convert number to quantity
	// if !result.IsQuantity {
	// 	result.Quantity = resource.MustParse(fmt.Sprintf("%g", result.Number))
	// 	result.IsQuantity = true
	// }

	return result, nil
}

// Parse creates a new calculator instance and parses the expression
func Parse(input string) (*Calculator, error) {
	calc := NewCalculator()
	_, err := calc.Parse(input)
	if err != nil {
		return nil, err
	}
	return calc, nil
}

// ParseWithVariables creates a new calculator instance with variables and parses the expression
func ParseWithVariables(input string, vars map[string]*Value) (*Calculator, error) {
	calc := NewCalculatorWithVariables(vars)
	_, err := calc.Parse(input)
	if err != nil {
		return nil, err
	}
	return calc, nil
}
