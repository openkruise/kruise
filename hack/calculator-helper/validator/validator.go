package main

import (
	"fmt"

	"github.com/openkruise/kruise/pkg/util/calculator"
)

// ValidateExpression validates if the expression is legal
// Only cpu and memory variables are allowed
func ValidateExpression(config *PlotConfig) error {
	if config.Expression == "" {
		return fmt.Errorf("expression cannot be empty")
	}

	// Check if expression contains only allowed variables (cpu, memory)
	// and allowed operators/functions
	if err := checkAllowedVariables(config.Variable); err != nil {
		return err
	}

	// Try to parse the expression with dummy values to check syntax
	var testVars map[string]*calculator.Value
	if config.Variable == "cpu" {
		testVars = map[string]*calculator.Value{
			"cpu": {
				IsQuantity: true,
				Quantity:   config.MinValue,
			},
		}
	} else {
		testVars = map[string]*calculator.Value{
			"memory": {
				IsQuantity: true,
				Quantity:   config.MinValue,
			},
		}
	}

	calc := calculator.NewCalculatorWithVariables(testVars)
	_, err := calc.Parse(config.Expression)
	if err != nil {
		return fmt.Errorf("expression syntax error when %s=%s: %w", config.Variable, config.MinValue.String(), err)
	}

	return nil
}

// checkAllowedVariables ensures only cpu and memory variables are used
func checkAllowedVariables(variable string) error {
	if variable != "cpu" && variable != "memory" {
		return fmt.Errorf("invalid variable %s, only support cpu or memory", variable)
	}
	return nil
}
