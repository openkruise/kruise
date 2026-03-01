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

package util

import (
	"testing"
)

func TestSetDefaultTtlForAlwaysNodeimage(t *testing.T) {
	tests := []struct {
		name      string
		input     int
		wantError bool
	}{
		{
			name:      "valid value",
			input:     1000,
			wantError: false,
		},
		{
			name:      "zero value",
			input:     0,
			wantError: false,
		},
		{
			name:      "negative value",
			input:     -1,
			wantError: true,
		},
		{
			name:      "max boundary value",
			input:     maxDefaultTtlsecondsForAlwaysNodeimage,
			wantError: false,
		},
		{
			name:      "exceeds max value",
			input:     maxDefaultTtlsecondsForAlwaysNodeimage + 1,
			wantError: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := SetDefaultTtlForAlwaysNodeimage(tt.input)
			if tt.wantError {
				if err == nil {
					t.Fatalf("expected error but got nil")
				}
			} else {
				if err != nil {
					t.Fatalf("got error: %s", err)
				}
				if int32(tt.input) != GetDefaultTtlsecondsForAlwaysNodeimage() {
					t.Fatalf("expected: %d, got: %d", tt.input, GetDefaultTtlsecondsForAlwaysNodeimage())
				}
			}
		})
	}
}
