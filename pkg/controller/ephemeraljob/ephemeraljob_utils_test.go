/*
Copyright 2026 The Kruise Authors.

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

package ephemeraljob

import "testing"

func TestGetCreateCount(t *testing.T) {
	tests := []struct {
		name        string
		toCreate    int
		parallelism int
		running     int
		expected    int
	}{
		{name: "no running pods, more pods than parallelism", toCreate: 10, parallelism: 5, running: 0, expected: 5},
		{name: "no running pods, fewer pods than parallelism", toCreate: 3, parallelism: 5, running: 0, expected: 3},
		{name: "some running, more pods than free slots", toCreate: 10, parallelism: 5, running: 3, expected: 2},
		{name: "some running, one pod left", toCreate: 1, parallelism: 5, running: 2, expected: 1},
		{name: "some running, fewer pods than parallelism", toCreate: 3, parallelism: 5, running: 2, expected: 3},
		{name: "parallelism full", toCreate: 2, parallelism: 3, running: 3, expected: 0},
		{name: "more running than parallelism", toCreate: 2, parallelism: 1, running: 2, expected: 0},
		{name: "nothing to create", toCreate: 0, parallelism: 5, running: 1, expected: 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := getCreateCount(tt.toCreate, tt.parallelism, tt.running); got != tt.expected {
				t.Errorf("getCreateCount(%d, %d, %d) = %d, expected %d", tt.toCreate, tt.parallelism, tt.running, got, tt.expected)
			}
		})
	}
}
