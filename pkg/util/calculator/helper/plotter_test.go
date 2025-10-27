package helper

import (
	"os"
	"testing"

	"k8s.io/apimachinery/pkg/api/resource"
)

func TestEvaluateExpression(t *testing.T) {
	tests := []struct {
		name     string
		expr     string
		varName  string
		varValue float64
		want     float64
		wantErr  bool
	}{
		{
			name:     "simple cpu multiplication",
			expr:     "cpu * 2",
			varName:  "cpu",
			varValue: 4,
			want:     8,
			wantErr:  false,
		},
		{
			name:     "cpu with max function",
			expr:     "max(cpu, 2)",
			varName:  "cpu",
			varValue: 1,
			want:     2,
			wantErr:  false,
		},
		{
			name:     "complex expression",
			expr:     "0.5*cpu - 0.3*max(0, cpu-4) + 0.3*max(0, cpu-8)",
			varName:  "cpu",
			varValue: 0,
			want:     0,
			wantErr:  false,
		},
		{
			name:     "complex expression at inflection point",
			expr:     "0.5*cpu - 0.3*max(0, cpu-4) + 0.3*max(0, cpu-8)",
			varName:  "cpu",
			varValue: 4,
			want:     2.0,
			wantErr:  false,
		},
		{
			name:     "complex expression at another inflection point",
			expr:     "0.5*cpu - 0.3*max(0, cpu-4) + 0.3*max(0, cpu-8)",
			varName:  "cpu",
			varValue: 8,
			want:     2.8,
			wantErr:  false,
		},
		{
			name:     "memory expression",
			expr:     "memory * 0.5",
			varName:  "memory",
			varValue: 2,          // 2 Gi
			want:     1073741824, // Result: 1 Gi in bytes (2Gi * 0.5 = 1Gi = 1073741824 bytes)
			wantErr:  false,
		},
		{
			name:     "memory with max",
			expr:     "max(memory, 1)",
			varName:  "memory",
			varValue: 0.5,       // 0.5 Gi
			want:     536870912, // max(0.5Gi, 1core) = 0.5Gi in bytes
			wantErr:  false,
		},
		{
			name:     "cpu with quantity in expression",
			expr:     "cpu + 100m",
			varName:  "cpu",
			varValue: 1,
			want:     1.1,
			wantErr:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := evaluateExpression(tt.expr, tt.varName, tt.varValue)
			if (err != nil) != tt.wantErr {
				t.Errorf("evaluateExpression() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && got != tt.want {
				t.Errorf("evaluateExpression() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestPlotExpression(t *testing.T) {
	tmpFile := "/tmp/test_plot.png"
	defer os.Remove(tmpFile)

	tests := []struct {
		name    string
		config  PlotConfig
		wantErr bool
	}{
		{
			name: "valid cpu plot",
			config: PlotConfig{
				Expression: "0.5*cpu",
				Variable:   "cpu",
				MinValue:   resource.MustParse("0"),
				MaxValue:   resource.MustParse("10"),
				NumPoints:  100,
				OutputFile: tmpFile,
			},
			wantErr: false,
		},
		{
			name: "invalid variable",
			config: PlotConfig{
				Expression: "0.5*cpu",
				Variable:   "disk",
				MinValue:   resource.MustParse("0"),
				MaxValue:   resource.MustParse("10"),
				OutputFile: tmpFile,
			},
			wantErr: true,
		},
		{
			name: "invalid expression",
			config: PlotConfig{
				Expression: "disk * 2",
				Variable:   "cpu",
				MinValue:   resource.MustParse("0"),
				MaxValue:   resource.MustParse("10"),
				OutputFile: tmpFile,
			},
			wantErr: true,
		},
		{
			name: "valid memory plot",
			config: PlotConfig{
				Expression: "0.5*memory + max(0, memory-2)",
				Variable:   "memory",
				MinValue:   resource.MustParse("0Gi"),
				MaxValue:   resource.MustParse("8Gi"),
				NumPoints:  100,
				OutputFile: tmpFile,
			},
			wantErr: false,
		},
		{
			name: "cpu with millicores",
			config: PlotConfig{
				Expression: "cpu * 0.5",
				Variable:   "cpu",
				MinValue:   resource.MustParse("0m"),
				MaxValue:   resource.MustParse("8000m"),
				NumPoints:  100,
				OutputFile: tmpFile,
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := PlotExpression(&tt.config)
			if (err != nil) != tt.wantErr {
				t.Errorf("PlotExpression() error = %v, wantErr %v", err, tt.wantErr)
			}
			if !tt.wantErr {
				// Check if file was created
				if _, err := os.Stat(tmpFile); os.IsNotExist(err) {
					t.Errorf("PlotExpression() did not create output file")
				}
			}
		})
	}
}
