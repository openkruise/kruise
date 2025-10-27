package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/openkruise/kruise/pkg/util/calculator/helper"
	"k8s.io/apimachinery/pkg/api/resource"
)

func main() {
	// Command-line flags
	expression := flag.String("expr", "", "Expression to validate and plot (required)")
	variable := flag.String("var", "cpu", "Variable to plot against (cpu or memory)")
	minValue := flag.String("min", "0", "Minimum value for the variable (k8s quantity, e.g., 0, 100m, 1Gi)")
	maxValue := flag.String("max", "16", "Maximum value for the variable (k8s quantity, e.g., 16, 8000m, 4Gi)")
	output := flag.String("output", "", "Output file path for the plot (e.g., plot.png)")
	validateOnly := flag.Bool("validate", false, "Only validate the expression without plotting")

	flag.Parse()

	// Check if expression is provided
	if *expression == "" {
		fmt.Println("Error: expression is required")
		flag.Usage()
		os.Exit(1)
	}

	minQty, err := resource.ParseQuantity(*minValue)
	if err != nil {
		fmt.Printf("Invalid min value '%s': %v\n", *minValue, err)
		os.Exit(1)
	}

	maxQty, err := resource.ParseQuantity(*maxValue)
	if err != nil {
		fmt.Printf("Invalid max value '%s': %v\n", *maxValue, err)
		os.Exit(1)
	}

	// Plot expression
	fmt.Printf("Plotting expression to %s\n", *output)
	config := &helper.PlotConfig{
		Expression: *expression,
		Variable:   *variable,
		MinValue:   minQty,
		MaxValue:   maxQty,
		NumPoints:  1000,
		OutputFile: *output,
		Title:      fmt.Sprintf("Expression: %s", *expression),
	}

	// Validate expression
	fmt.Printf("Validating expression: %s\n", *expression)
	if err := helper.ValidateExpression(config); err != nil {
		fmt.Printf("Validation failed: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("✓ Expression is valid")

	// If validate-only mode, exit here
	if *validateOnly {
		os.Exit(0)
	}

	// Check if output file is provided
	if *output == "" {
		fmt.Println("No output file specified, skipping plot generation")
		os.Exit(0)
	}

	if err := helper.PlotExpression(config); err != nil {
		fmt.Printf("Plot generation failed: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("✓ Plot generated successfully: %s\n", *output)
}
