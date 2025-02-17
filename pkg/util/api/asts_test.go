/*
Copyright 2025 The Kruise Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package api

import (
	"testing"

	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/util/sets"
)

func TestGetIntSet(t *testing.T) {
	tests := []struct {
		name     string
		input    []intstr.IntOrString
		expected sets.Set[int]
		wantErr  bool
	}{
		{
			name: "integer elements",
			input: []intstr.IntOrString{
				intstr.FromInt32(1),
				intstr.FromInt32(2),
				intstr.FromInt32(3),
			},
			expected: sets.New(1, 2, 3),
		},
		{
			name: "string range",
			input: []intstr.IntOrString{
				intstr.FromString("1-3"),
			},
			expected: sets.New(1, 2, 3),
		},
		{
			name: "invalid range end",
			input: []intstr.IntOrString{
				intstr.FromString("1-%"),
			},
			wantErr: true,
		},
		{
			name: "invalid range start",
			input: []intstr.IntOrString{
				intstr.FromString("%-2"),
			},
			wantErr: true,
		},
		{
			name: "invalid range split",
			input: []intstr.IntOrString{
				intstr.FromString("1-2-3"),
			},
			wantErr: true,
		},
		{
			name: "mixed input",
			input: []intstr.IntOrString{
				intstr.FromInt32(1),
				intstr.FromString("2-3"),
			},
			expected: sets.New(1, 2, 3),
		},
		{
			name:     "empty input",
			input:    []intstr.IntOrString{},
			expected: sets.New[int](),
		},
		{
			name: "duplicate numbers",
			input: []intstr.IntOrString{
				intstr.FromInt32(1),
				intstr.FromInt32(1),
				intstr.FromInt32(2),
				intstr.FromInt32(3),
			},
			expected: sets.New(1, 2, 3),
		},
		{
			name: "overlapping ranges",
			input: []intstr.IntOrString{
				intstr.FromString("1-3"),
				intstr.FromString("2-4"),
			},
			expected: sets.New(1, 2, 3, 4),
		},
		{
			name: "range with number",
			input: []intstr.IntOrString{
				intstr.FromInt32(2),
				intstr.FromString("1-3"),
			},
			expected: sets.New(1, 2, 3),
		},
		{
			name: "unsorted number",
			input: []intstr.IntOrString{
				intstr.FromInt32(2),
				intstr.FromInt32(1),
			},
			expected: sets.New(1, 2),
		},
		{
			name: "bad strange range",
			input: []intstr.IntOrString{
				intstr.FromString("03-001"),
			},
			wantErr: true,
		},
		{
			name: "good strange range",
			input: []intstr.IntOrString{
				intstr.FromString("0003-005"),
			},
			expected: sets.New(3, 4, 5),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			actual := GetReserveOrdinalIntSet(tt.input)
			if tt.wantErr {
				if actual != nil {
					t.Errorf("Expected error (nil set), but got %v", actual)
				}
				return
			}
			if !actual.Equal(tt.expected) {
				t.Errorf("For case %q, expected set %v, but got %v", tt.name, tt.expected, actual)
			}
		})
	}
}
