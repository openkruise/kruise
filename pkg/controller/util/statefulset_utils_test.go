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

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestGetOrdinal(t *testing.T) {
	tests := []struct {
		name            string
		podName         string
		expectedOrdinal int32
	}{
		{
			name:            "valid statefulset pod",
			podName:         "web-0",
			expectedOrdinal: 0,
		},
		{
			name:            "valid statefulset pod with higher ordinal",
			podName:         "web-42",
			expectedOrdinal: 42,
		},
		{
			name:            "valid statefulset pod with multi-digit ordinal",
			podName:         "mysql-server-999",
			expectedOrdinal: 999,
		},
		{
			name:            "pod with hyphenated name",
			podName:         "my-app-service-5",
			expectedOrdinal: 5,
		},
		{
			name:            "invalid pod name without ordinal",
			podName:         "regular-pod",
			expectedOrdinal: -1,
		},
		{
			name:            "invalid pod name with non-numeric suffix",
			podName:         "pod-abc",
			expectedOrdinal: -1,
		},
		{
			name:            "pod name with only number",
			podName:         "123",
			expectedOrdinal: -1,
		},
		{
			name:            "empty pod name",
			podName:         "",
			expectedOrdinal: -1,
		},
		{
			name:            "pod name ending with hyphen",
			podName:         "pod-",
			expectedOrdinal: -1,
		},
		{
			name:            "valid pod with ordinal 0",
			podName:         "kafka-0",
			expectedOrdinal: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pod := &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name: tt.podName,
				},
			}

			ordinal := GetOrdinal(pod)
			assert.Equal(t, tt.expectedOrdinal, ordinal)
		})
	}
}

func TestGetParentNameAndOrdinal(t *testing.T) {
	tests := []struct {
		name           string
		podName        string
		expectedParent string
		expectedOrd    int32
	}{
		{
			name:           "simple statefulset pod",
			podName:        "web-0",
			expectedParent: "web",
			expectedOrd:    0,
		},
		{
			name:           "complex name with multiple hyphens",
			podName:        "my-stateful-app-15",
			expectedParent: "my-stateful-app",
			expectedOrd:    15,
		},
		{
			name:           "large ordinal number",
			podName:        "redis-cluster-1000",
			expectedParent: "redis-cluster",
			expectedOrd:    1000,
		},
		{
			name:           "invalid format - no ordinal",
			podName:        "regular-pod-name",
			expectedParent: "",
			expectedOrd:    -1,
		},
		{
			name:           "invalid format - non-numeric suffix",
			podName:        "pod-xyz",
			expectedParent: "",
			expectedOrd:    -1,
		},
		{
			name:           "single character prefix",
			podName:        "a-7",
			expectedParent: "a",
			expectedOrd:    7,
		},
		{
			name:           "ordinal with leading zeros",
			podName:        "app-007",
			expectedParent: "app",
			expectedOrd:    7,
		},
		{
			name:           "empty string",
			podName:        "",
			expectedParent: "",
			expectedOrd:    -1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pod := &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name: tt.podName,
				},
			}

			parent, ordinal := getParentNameAndOrdinal(pod)
			assert.Equal(t, tt.expectedParent, parent, "parent name mismatch")
			assert.Equal(t, tt.expectedOrd, ordinal, "ordinal mismatch")
		})
	}
}
