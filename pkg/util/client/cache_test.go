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

package client

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func TestIsDisableDeepCopy(t *testing.T) {
	tests := []struct {
		name     string
		opts     []client.ListOption
		expected bool
	}{
		{
			name:     "option is present alone",
			opts:     []client.ListOption{DisableDeepCopy},
			expected: true,
		},
		{
			name:     "only other options are present",
			opts:     []client.ListOption{client.Limit(10)},
			expected: false,
		},
		{
			name:     "slice is empty",
			opts:     []client.ListOption{},
			expected: false,
		},
		{
			name:     "slice is nil",
			opts:     nil,
			expected: false,
		},
		{
			name:     "option is present with others",
			opts:     []client.ListOption{DisableDeepCopy, &client.MatchingLabels{"foo": "bar"}},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, isDisableDeepCopy(tt.opts))
		})
	}
}
