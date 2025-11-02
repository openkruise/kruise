package main

import (
	"testing"

	"k8s.io/apimachinery/pkg/api/resource"
)

func TestValidateExpression(t *testing.T) {
	tests := []struct {
		name    string
		config  PlotConfig
		wantErr bool
	}{
		{
			name: "valid cpu expression",
			config: PlotConfig{
				Expression: "0.5*cpu - 0.3*max(0, cpu-4) + 0.3*max(0, cpu-8)",
				MinValue:   resource.MustParse("1"),
				Variable:   "cpu",
			},
			wantErr: false,
		},
		{
			name: "valid memory expression",
			config: PlotConfig{
				Expression: "max(memory*50%, 100Mi)",
				MinValue:   resource.MustParse("1Gi"),
				Variable:   "memory",
			},
			wantErr: false,
		},
		{
			name: "valid cpu with min function",
			config: PlotConfig{
				Expression: "min(cpu*2, 4)",
				MinValue:   resource.MustParse("1"),
				Variable:   "cpu",
			},
			wantErr: false,
		},
		{
			name: "empty expression",
			config: PlotConfig{
				Expression: "",
				MinValue:   resource.MustParse("1"),
				Variable:   "cpu",
			},
			wantErr: true,
		},
		{
			name: "invalid variable",
			config: PlotConfig{
				Expression: "disk * 2",
				MinValue:   resource.MustParse("1"),
				Variable:   "cpu",
			},
			wantErr: true,
		},
		{
			name: "unmatched variable",
			config: PlotConfig{
				Expression: "cpu * 2",
				MinValue:   resource.MustParse("1Gi"),
				Variable:   "memory",
			},
			wantErr: true,
		},
		{
			name: "un-supported variable",
			config: PlotConfig{
				Expression: "disk * 2",
				MinValue:   resource.MustParse("1Gi"),
				Variable:   "disk",
			},
			wantErr: true,
		},
		{
			name: "multiple invalid variables",
			config: PlotConfig{
				Expression: "cpu + memory + disk",
				MinValue:   resource.MustParse("1"),
				Variable:   "cpu",
			},
			wantErr: true,
		},
		{
			name: "syntax error",
			config: PlotConfig{
				Expression: "cpu + +",
				MinValue:   resource.MustParse("1"),
				Variable:   "cpu",
			},
			wantErr: true,
		},
		{
			name: "division by zero",
			config: PlotConfig{
				Expression: "cpu / 0",
				MinValue:   resource.MustParse("1"),
				Variable:   "cpu",
			},
			wantErr: true,
		},
		{
			name: "valid complex expression",
			config: PlotConfig{
				Expression: "(cpu + 2) * 0.5 + max(0, cpu-4)",
				MinValue:   resource.MustParse("1"),
				Variable:   "cpu",
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateExpression(&tt.config)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateExpression() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestCheckAllowedVariables(t *testing.T) {
	tests := []struct {
		name      string
		variables string
		wantErr   bool
	}{
		{
			name:      "only cpu",
			variables: "cpu",
			wantErr:   false,
		},
		{
			name:      "only memory",
			variables: "memory",
			wantErr:   false,
		},
		{
			name:      "cpu and memory",
			variables: "cpu + memory",
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := checkAllowedVariables(tt.variables)
			if (err != nil) != tt.wantErr {
				t.Errorf("checkAllowedVariables() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
