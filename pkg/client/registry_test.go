package client

import (
	"testing"

	"k8s.io/apimachinery/pkg/version"
)

func TestShouldUpdateResourceByResize(t *testing.T) {
	tests := []struct {
		name     string
		major    string
		minor    string
		expected bool
	}{
		{
			name:     "standard minor below 1.32",
			major:    "1",
			minor:    "28",
			expected: false,
		},
		{
			name:     "standard minor at 1.32",
			major:    "1",
			minor:    "32",
			expected: true,
		},
		{
			name:     "standard minor above 1.32",
			major:    "1",
			minor:    "33",
			expected: true,
		},
		{
			name:     "private build minor with plus suffix below 1.32",
			major:    "1",
			minor:    "19+",
			expected: false,
		},
		{
			name:     "private build minor with plus suffix below 1.32 (28+)",
			major:    "1",
			minor:    "28+",
			expected: false,
		},
		{
			name:     "private build minor with plus suffix at 1.32",
			major:    "1",
			minor:    "32+",
			expected: true,
		},
		{
			name:     "private build minor with plus suffix above 1.32",
			major:    "1",
			minor:    "35+",
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Temporarily override curVersion for the test.
			orig := curVersion
			curVersion = &version.Info{Major: tt.major, Minor: tt.minor}
			defer func() { curVersion = orig }()

			got := ShouldUpdateResourceByResize()
			if got != tt.expected {
				t.Errorf("ShouldUpdateResourceByResize() with Major=%q Minor=%q = %v, want %v",
					tt.major, tt.minor, got, tt.expected)
			}
		})
	}
}
