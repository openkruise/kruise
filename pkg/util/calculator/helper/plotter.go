package helper

import (
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"os"

	"github.com/openkruise/kruise/pkg/util/calculator"
	"golang.org/x/image/font"
	"golang.org/x/image/font/basicfont"
	"golang.org/x/image/math/fixed"
	"k8s.io/apimachinery/pkg/api/resource"
)

// PlotConfig contains configuration for plotting
type PlotConfig struct {
	Expression string            // Expression to plot (e.g., "0.5*cpu - 0.3*max(0, cpu-4) + 0.3*max(0, cpu-8)")
	Variable   string            // Variable to plot against (cpu or memory)
	MinValue   resource.Quantity // Minimum value for the variable (k8s quantity)
	MaxValue   resource.Quantity // Maximum value for the variable (k8s quantity)
	NumPoints  int               // Number of points to evaluate
	OutputFile string            // Output file path (e.g., "docs/proposals/20250913-story5.png")
	Title      string            // Plot title
}

// PlotExpression plots the expression and saves to a file
func PlotExpression(config *PlotConfig) error {
	// Validate expression
	if err := ValidateExpression(config); err != nil {
		return fmt.Errorf("invalid expression: %w", err)
	}

	// Validate variable
	if config.Variable != "cpu" && config.Variable != "memory" {
		return fmt.Errorf("variable must be 'cpu' or 'memory', got '%s'", config.Variable)
	}

	// Set default values
	if config.NumPoints <= 0 {
		config.NumPoints = 1000
	}
	if config.Title == "" {
		config.Title = fmt.Sprintf("Expression: %s", config.Expression)
	}

	// Convert quantities to float64 for calculation
	// For CPU: use cores (not millicores)
	// For memory: use Gi (not bytes)
	var minVal, maxVal float64
	var xLabel, yLabel string

	if config.Variable == "cpu" {
		// Convert to cores
		minVal = float64(config.MinValue.MilliValue()) / 1000.0
		maxVal = float64(config.MaxValue.MilliValue()) / 1000.0
		xLabel = "CPU (cores)"
		yLabel = "Result (cores)"
	} else { // memory
		// Convert to Gi
		minVal = float64(config.MinValue.Value()) / (1024.0 * 1024.0 * 1024.0)
		maxVal = float64(config.MaxValue.Value()) / (1024.0 * 1024.0 * 1024.0)
		xLabel = "Memory (Gi)"
		yLabel = "Result (Gi)"
	}

	// Generate data points
	var xValues []float64
	var yValues []float64
	step := (maxVal - minVal) / float64(config.NumPoints-1)

	for i := 0; i < config.NumPoints; i++ {
		x := minVal + float64(i)*step
		y, err := evaluateExpression(config.Expression, config.Variable, x)
		if err != nil {
			// If evaluation fails, use 0
			y = 0
		}
		if config.Variable == "memory" {
			// convert value to Gi
			y = y / 1024 / 1024 / 1024
		}
		xValues = append(xValues, x)
		yValues = append(yValues, y)
	}

	// Create image
	width, height := 800, 600
	img := image.NewRGBA(image.Rect(0, 0, width, height))

	// Fill background with white
	draw.Draw(img, img.Bounds(), &image.Uniform{color.White}, image.Point{}, draw.Src)

	// Define plot area
	margin := 80
	plotX := margin
	plotY := margin
	plotWidth := width - 2*margin
	plotHeight := height - 2*margin

	// Find min/max Y for scaling
	minY, maxY := yValues[0], yValues[0]
	for _, y := range yValues {
		if y < minY {
			minY = y
		}
		if y > maxY {
			maxY = y
		}
	}

	// Add some padding to Y range
	yRange := maxY - minY
	if yRange < 0.01 {
		yRange = 1.0
	}
	minY -= yRange * 0.1
	maxY += yRange * 0.1

	// Draw axes
	drawLine(img, plotX, plotY+plotHeight, plotX+plotWidth, plotY+plotHeight, color.Black) // X-axis
	drawLine(img, plotX, plotY, plotX, plotY+plotHeight, color.Black)                      // Y-axis

	// Draw grid and labels
	numXTicks := 10
	numYTicks := 10

	for i := 0; i <= numXTicks; i++ {
		x := plotX + (plotWidth * i / numXTicks)
		y := plotY + plotHeight
		drawLine(img, x, y, x, y+5, color.Black)

		// Draw grid line
		gridColor := color.RGBA{200, 200, 200, 255}
		drawLine(img, x, plotY, x, y, gridColor)

		// Draw label
		xVal := minVal + (maxVal-minVal)*float64(i)/float64(numXTicks)
		label := fmt.Sprintf("%.1f", xVal)
		addLabel(img, x-15, y+20, label, color.Black)
	}

	for i := 0; i <= numYTicks; i++ {
		x := plotX
		y := plotY + plotHeight - (plotHeight * i / numYTicks)
		drawLine(img, x-5, y, x, y, color.Black)

		// Draw grid line
		gridColor := color.RGBA{200, 200, 200, 255}
		drawLine(img, x, y, plotX+plotWidth, y, gridColor)

		// Draw label
		yVal := minY + (maxY-minY)*float64(i)/float64(numYTicks)
		label := fmt.Sprintf("%.2f", yVal)
		addLabel(img, x-60, y+5, label, color.Black)
	}

	// Draw the curve
	blue := color.RGBA{0, 0, 255, 255}
	for i := 0; i < len(xValues)-1; i++ {
		x1 := plotX + int(float64(plotWidth)*(xValues[i]-minVal)/(maxVal-minVal))
		y1 := plotY + plotHeight - int(float64(plotHeight)*(yValues[i]-minY)/(maxY-minY))
		x2 := plotX + int(float64(plotWidth)*(xValues[i+1]-minVal)/(maxVal-minVal))
		y2 := plotY + plotHeight - int(float64(plotHeight)*(yValues[i+1]-minY)/(maxY-minY))
		drawLine(img, x1, y1, x2, y2, blue)
	}

	// Draw title
	addLabel(img, width/2-100, 30, config.Title, color.Black)

	// Draw axis labels
	addLabel(img, width/2-50, height-20, xLabel, color.Black)
	addLabel(img, 20, height/2, yLabel, color.Black)

	// Save image
	file, err := os.Create(config.OutputFile)
	if err != nil {
		return fmt.Errorf("failed to create output file: %w", err)
	}
	defer file.Close()

	if err := png.Encode(file, img); err != nil {
		return fmt.Errorf("failed to encode PNG: %w", err)
	}

	return nil
}

// evaluateExpression evaluates the expression with the given variable value
func evaluateExpression(expr string, varName string, varValue float64) (float64, error) {
	var x resource.Quantity
	if varName == "cpu" {
		x = resource.MustParse(fmt.Sprintf("%v", varValue))
	} else {
		x = resource.MustParse(fmt.Sprintf("%vGi", varValue))
	}
	vars := map[string]*calculator.Value{
		varName: {
			IsQuantity: true,
			Quantity:   x,
		},
	}

	calc := calculator.NewCalculatorWithVariables(vars)
	resValue, err := calc.Parse(expr)
	if err != nil {
		return 0, err
	}

	// Get the result value
	if resValue.IsQuantity {
		return float64(resValue.Quantity.MilliValue()) / 1000.0, nil
	}
	return resValue.Number, nil
}

// drawLine draws a line from (x1, y1) to (x2, y2)
func drawLine(img *image.RGBA, x1, y1, x2, y2 int, col color.Color) {
	// Simple Bresenham's line algorithm
	dx := abs(x2 - x1)
	dy := abs(y2 - y1)
	sx := 1
	if x1 >= x2 {
		sx = -1
	}
	sy := 1
	if y1 >= y2 {
		sy = -1
	}
	err := dx - dy

	for {
		img.Set(x1, y1, col)

		if x1 == x2 && y1 == y2 {
			break
		}

		e2 := 2 * err
		if e2 > -dy {
			err -= dy
			x1 += sx
		}
		if e2 < dx {
			err += dx
			y1 += sy
		}
	}
}

// addLabel adds text to the image
func addLabel(img *image.RGBA, x, y int, label string, col color.Color) {
	point := fixed.Point26_6{X: fixed.Int26_6(x * 64), Y: fixed.Int26_6(y * 64)}

	d := &font.Drawer{
		Dst:  img,
		Src:  image.NewUniform(col),
		Face: basicfont.Face7x13,
		Dot:  point,
	}
	d.DrawString(label)
}

// abs returns the absolute value of x
func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}
