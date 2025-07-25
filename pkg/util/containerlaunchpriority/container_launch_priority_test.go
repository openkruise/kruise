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

package containerlaunchpriority

import (
	"testing"

	"github.com/stretchr/testify/assert"
	v1 "k8s.io/api/core/v1"

	appspub "github.com/openkruise/kruise/apis/apps/pub"
)

func TestGetKey(t *testing.T) {
	tests := []struct {
		name     string
		priority int
		expected string
	}{
		{
			name:     "positive priority",
			priority: 5,
			expected: "p_5",
		},
		{
			name:     "negative priority",
			priority: -3,
			expected: "p_-3",
		},
		{
			name:     "zero priority",
			priority: 0,
			expected: "p_0",
		},
		{
			name:     "large positive priority",
			priority: 2147483647,
			expected: "p_2147483647",
		},
		{
			name:     "large negative priority",
			priority: -2147483647,
			expected: "p_-2147483647",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := GetKey(tt.priority)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestGeneratePriorityEnv(t *testing.T) {
	tests := []struct {
		name     string
		priority int
		podName  string
		expected v1.EnvVar
	}{
		{
			name:     "positive priority with simple pod name",
			priority: 3,
			podName:  "test-pod",
			expected: v1.EnvVar{
				Name: appspub.ContainerLaunchBarrierEnvName,
				ValueFrom: &v1.EnvVarSource{
					ConfigMapKeyRef: &v1.ConfigMapKeySelector{
						LocalObjectReference: v1.LocalObjectReference{Name: "test-pod-barrier"},
						Key:                  "p_3",
					},
				},
			},
		},
		{
			name:     "negative priority with complex pod name",
			priority: -1,
			podName:  "my-app-deployment-abc123",
			expected: v1.EnvVar{
				Name: appspub.ContainerLaunchBarrierEnvName,
				ValueFrom: &v1.EnvVarSource{
					ConfigMapKeyRef: &v1.ConfigMapKeySelector{
						LocalObjectReference: v1.LocalObjectReference{Name: "my-app-deployment-abc123-barrier"},
						Key:                  "p_-1",
					},
				},
			},
		},
		{
			name:     "zero priority",
			priority: 0,
			podName:  "zero-pod",
			expected: v1.EnvVar{
				Name: appspub.ContainerLaunchBarrierEnvName,
				ValueFrom: &v1.EnvVarSource{
					ConfigMapKeyRef: &v1.ConfigMapKeySelector{
						LocalObjectReference: v1.LocalObjectReference{Name: "zero-pod-barrier"},
						Key:                  "p_0",
					},
				},
			},
		},
		{
			name:     "empty pod name",
			priority: 1,
			podName:  "",
			expected: v1.EnvVar{
				Name: appspub.ContainerLaunchBarrierEnvName,
				ValueFrom: &v1.EnvVarSource{
					ConfigMapKeyRef: &v1.ConfigMapKeySelector{
						LocalObjectReference: v1.LocalObjectReference{Name: "-barrier"},
						Key:                  "p_1",
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := GeneratePriorityEnv(tt.priority, tt.podName)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestGetContainerPriority(t *testing.T) {
	tests := []struct {
		name      string
		container *v1.Container
		expected  *int
	}{
		{
			name: "container with valid priority env",
			container: &v1.Container{
				Name: "test-container",
				Env: []v1.EnvVar{
					{
						Name: appspub.ContainerLaunchBarrierEnvName,
						ValueFrom: &v1.EnvVarSource{
							ConfigMapKeyRef: &v1.ConfigMapKeySelector{
								Key: "p_5",
							},
						},
					},
				},
			},
			expected: intPtr(5),
		},
		{
			name: "container with negative priority",
			container: &v1.Container{
				Name: "test-container",
				Env: []v1.EnvVar{
					{
						Name: appspub.ContainerLaunchBarrierEnvName,
						ValueFrom: &v1.EnvVarSource{
							ConfigMapKeyRef: &v1.ConfigMapKeySelector{
								Key: "p_-3",
							},
						},
					},
				},
			},
			expected: intPtr(-3),
		},
		{
			name: "container with zero priority",
			container: &v1.Container{
				Name: "test-container",
				Env: []v1.EnvVar{
					{
						Name: appspub.ContainerLaunchBarrierEnvName,
						ValueFrom: &v1.EnvVarSource{
							ConfigMapKeyRef: &v1.ConfigMapKeySelector{
								Key: "p_0",
							},
						},
					},
				},
			},
			expected: intPtr(0),
		},
		{
			name: "container with no priority env",
			container: &v1.Container{
				Name: "test-container",
				Env: []v1.EnvVar{
					{
						Name:  "OTHER_ENV",
						Value: "some-value",
					},
				},
			},
			expected: nil,
		},
		{
			name: "container with no env vars",
			container: &v1.Container{
				Name: "test-container",
			},
			expected: nil,
		},
		{
			name: "container with multiple env vars including priority",
			container: &v1.Container{
				Name: "test-container",
				Env: []v1.EnvVar{
					{
						Name:  "FIRST_ENV",
						Value: "value1",
					},
					{
						Name: appspub.ContainerLaunchBarrierEnvName,
						ValueFrom: &v1.EnvVarSource{
							ConfigMapKeyRef: &v1.ConfigMapKeySelector{
								Key: "p_10",
							},
						},
					},
					{
						Name:  "LAST_ENV",
						Value: "value2",
					},
				},
			},
			expected: intPtr(10),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := GetContainerPriority(tt.container)
			if tt.expected == nil {
				assert.Nil(t, result)
			} else {
				assert.NotNil(t, result)
				assert.Equal(t, *tt.expected, *result)
			}
		})
	}
}

func TestGetContainerPriority_EdgeCases(t *testing.T) {
	tests := []struct {
		name      string
		container *v1.Container
		expected  *int
	}{
		{
			name: "container with invalid key format (too short)",
			container: &v1.Container{
				Name: "test-container",
				Env: []v1.EnvVar{
					{
						Name: appspub.ContainerLaunchBarrierEnvName,
						ValueFrom: &v1.EnvVarSource{
							ConfigMapKeyRef: &v1.ConfigMapKeySelector{
								Key: "p",
							},
						},
					},
				},
			},
			expected: nil,
		},
		{
			name: "container with invalid key format (wrong prefix)",
			container: &v1.Container{
				Name: "test-container",
				Env: []v1.EnvVar{
					{
						Name: appspub.ContainerLaunchBarrierEnvName,
						ValueFrom: &v1.EnvVarSource{
							ConfigMapKeyRef: &v1.ConfigMapKeySelector{
								Key: "x_5",
							},
						},
					},
				},
			},
			expected: intPtr(5),
		},
		{
			name: "container with non-numeric priority",
			container: &v1.Container{
				Name: "test-container",
				Env: []v1.EnvVar{
					{
						Name: appspub.ContainerLaunchBarrierEnvName,
						ValueFrom: &v1.EnvVarSource{
							ConfigMapKeyRef: &v1.ConfigMapKeySelector{
								Key: "p_abc",
							},
						},
					},
				},
			},
			expected: intPtr(0),
		},
		{
			name: "container with very large priority",
			container: &v1.Container{
				Name: "test-container",
				Env: []v1.EnvVar{
					{
						Name: appspub.ContainerLaunchBarrierEnvName,
						ValueFrom: &v1.EnvVarSource{
							ConfigMapKeyRef: &v1.ConfigMapKeySelector{
								Key: "p_2147483647",
							},
						},
					},
				},
			},
			expected: intPtr(2147483647),
		},
		{
			name: "container with env var but no ValueFrom",
			container: &v1.Container{
				Name: "test-container",
				Env: []v1.EnvVar{
					{
						Name:  appspub.ContainerLaunchBarrierEnvName,
						Value: "direct-value",
					},
				},
			},
			expected: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// For cases that would panic, we need to handle them specially
			if tt.name == "container with env var but no ValueFrom" ||
			   tt.name == "container with invalid key format (too short)" {
				assert.Panics(t, func() {
					GetContainerPriority(tt.container)
				})
				return
			}

			result := GetContainerPriority(tt.container)
			if tt.expected == nil {
				assert.Nil(t, result)
			} else {
				assert.NotNil(t, result)
				assert.Equal(t, *tt.expected, *result)
			}
		})
	}
}

func TestIntegration_GenerateAndParse(t *testing.T) {
	tests := []struct {
		name     string
		priority int
		podName  string
	}{
		{
			name:     "round trip positive priority",
			priority: 42,
			podName:  "test-pod",
		},
		{
			name:     "round trip negative priority",
			priority: -15,
			podName:  "negative-pod",
		},
		{
			name:     "round trip zero priority",
			priority: 0,
			podName:  "zero-pod",
		},
		{
			name:     "round trip max int priority",
			priority: 2147483647,
			podName:  "max-pod",
		},
		{
			name:     "round trip min int priority",
			priority: -2147483647,
			podName:  "min-pod",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Generate environment variable
			envVar := GeneratePriorityEnv(tt.priority, tt.podName)

			// Create container with the generated env var
			container := &v1.Container{
				Name: "test-container",
				Env:  []v1.EnvVar{envVar},
			}

			// Parse priority back from container
			parsedPriority := GetContainerPriority(container)

			// Verify round trip
			assert.NotNil(t, parsedPriority)
			assert.Equal(t, tt.priority, *parsedPriority)

			// Verify env var structure
			assert.Equal(t, appspub.ContainerLaunchBarrierEnvName, envVar.Name)
			assert.NotNil(t, envVar.ValueFrom)
			assert.NotNil(t, envVar.ValueFrom.ConfigMapKeyRef)
			assert.Equal(t, tt.podName+"-barrier", envVar.ValueFrom.ConfigMapKeyRef.Name)
			assert.Equal(t, GetKey(tt.priority), envVar.ValueFrom.ConfigMapKeyRef.Key)
		})
	}
}

func TestConstants(t *testing.T) {
	// Verify that the priority start index constant is correct
	assert.Equal(t, 2, priorityStartIndex)

	// Verify that GetKey format matches expected pattern for parsing
	key := GetKey(123)
	assert.Equal(t, "p_123", key)

	// Verify that parsing logic works with the constant
	assert.Equal(t, "123", key[priorityStartIndex:])
}

func intPtr(i int) *int {
	return &i
}
