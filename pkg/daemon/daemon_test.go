/*
Copyright 2021 The Kruise Authors.

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

package daemon

import (
	"testing"

	"k8s.io/client-go/rest"
)

func TestValidateDaemonFlags(t *testing.T) {
	testCases := []struct {
		name        string
		cfg         *rest.Config
		crrWorkers  int
		expectError bool
		errorMsg    string
	}{
		{
			name:        "nil config",
			cfg:         nil,
			crrWorkers:  32,
			expectError: true,
			errorMsg:    "cfg can not be nil",
		},
		{
			name:        "invalid crr workers (0)",
			cfg:         &rest.Config{},
			crrWorkers:  0,
			expectError: true,
			errorMsg:    "crr-workers must be greater than 0",
		},
		{
			name:        "invalid crr workers (-1)",
			cfg:         &rest.Config{},
			crrWorkers:  -1,
			expectError: true,
			errorMsg:    "crr-workers must be greater than 0",
		},
		{
			name:        "valid crr workers",
			cfg:         &rest.Config{},
			crrWorkers:  32,
			expectError: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateDaemonFlags(tc.cfg, tc.crrWorkers)
			if tc.expectError {
				if err == nil {
					t.Fatal("expected error, but got nil")
				}
				if err.Error() != tc.errorMsg {
					t.Fatalf("expected error message %q, but got %q", tc.errorMsg, err.Error())
				}
			} else {
				if err != nil {
					t.Fatalf("expected no error, but got %v", err)
				}
			}
		})
	}
}
