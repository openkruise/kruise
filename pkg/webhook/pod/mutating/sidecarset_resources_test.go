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

package mutating

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"

	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	"github.com/openkruise/kruise/pkg/control/sidecarcontrol"
)

func TestApplyResourcesPolicy(t *testing.T) {
	tests := []struct {
		name                  string
		pod                   *corev1.Pod
		sidecarContainer      *appsv1alpha1.SidecarContainer
		matchedSidecarSets    []sidecarcontrol.SidecarControl
		expectError           bool
		expectedCPULimit      string
		expectedMemoryLimit   string
		expectedCPURequest    string
		expectedMemoryRequest string
		expectNoCPULimit      bool // CPU limit should NOT be set (unlimited)
		expectNoMemoryLimit   bool // Memory limit should NOT be set (unlimited)
	}{
		{
			name: "sum mode with percentage - basic",
			pod: &corev1.Pod{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  "app1",
							Image: "nginx:1.14.2",
							Resources: corev1.ResourceRequirements{
								Limits: corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse("200m"),
									corev1.ResourceMemory: resource.MustParse("200Mi"),
								},
								Requests: corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse("50m"),
									corev1.ResourceMemory: resource.MustParse("100Mi"),
								},
							},
						},
						{
							Name:  "app2",
							Image: "nginx:1.14.2",
							Resources: corev1.ResourceRequirements{
								Limits: corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse("400m"),
									corev1.ResourceMemory: resource.MustParse("400Mi"),
								},
								Requests: corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse("100m"),
									corev1.ResourceMemory: resource.MustParse("200Mi"),
								},
							},
						},
					},
				},
			},
			sidecarContainer: &appsv1alpha1.SidecarContainer{
				Container: corev1.Container{
					Name:  "sidecar1",
					Image: "sidecar:latest",
				},
				ResourcesPolicy: &appsv1alpha1.ResourcesPolicy{
					TargetContainerMode:       appsv1alpha1.TargetContainerModeSum,
					TargetContainersNameRegex: ".*",
					ResourceExpr: appsv1alpha1.ResourceExpr{
						Limits: &appsv1alpha1.ResourceExprLimits{
							CPU:    "cpu*50%",
							Memory: "200Mi",
						},
						Requests: &appsv1alpha1.ResourceExprRequests{
							CPU:    "cpu*50%",
							Memory: "100Mi",
						},
					},
				},
			},
			matchedSidecarSets:    []sidecarcontrol.SidecarControl{},
			expectError:           false,
			expectedCPULimit:      "300m",  // (200m + 400m) * 50% = 300m
			expectedMemoryLimit:   "200Mi", // constant
			expectedCPURequest:    "75m",   // (50m + 100m) * 50% = 75m
			expectedMemoryRequest: "100Mi", // constant
		},
		{
			name: "max mode with expression",
			pod: &corev1.Pod{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  "large.engine.v4",
							Image: "nginx:1.14.2",
							Resources: corev1.ResourceRequirements{
								Limits: corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse("200m"),
									corev1.ResourceMemory: resource.MustParse("200Mi"),
								},
								Requests: corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse("50m"),
									corev1.ResourceMemory: resource.MustParse("100Mi"),
								},
							},
						},
						{
							Name:  "large.engine.v8",
							Image: "nginx:1.14.2",
							Resources: corev1.ResourceRequirements{
								Limits: corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse("400m"),
									corev1.ResourceMemory: resource.MustParse("400Mi"),
								},
								Requests: corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse("100m"),
									corev1.ResourceMemory: resource.MustParse("200Mi"),
								},
							},
						},
					},
				},
			},
			sidecarContainer: &appsv1alpha1.SidecarContainer{
				Container: corev1.Container{
					Name:  "sidecar1",
					Image: "sidecar:latest",
				},
				ResourcesPolicy: &appsv1alpha1.ResourcesPolicy{
					TargetContainerMode:       appsv1alpha1.TargetContainerModeMax,
					TargetContainersNameRegex: "^large.engine.v.*$",
					ResourceExpr: appsv1alpha1.ResourceExpr{
						Limits: &appsv1alpha1.ResourceExprLimits{
							CPU:    "max(cpu*50%, 50m)",
							Memory: "200Mi",
						},
						Requests: &appsv1alpha1.ResourceExprRequests{
							CPU:    "max(cpu*50%, 50m)",
							Memory: "100Mi",
						},
					},
				},
			},
			matchedSidecarSets:    []sidecarcontrol.SidecarControl{},
			expectError:           false,
			expectedCPULimit:      "200m", // max(max(200m, 400m) * 50%, 50m) = max(200m, 50m) = 200m
			expectedMemoryLimit:   "200Mi",
			expectedCPURequest:    "50m", // max(max(50m, 100m) * 50%, 50m) = max(50m, 50m) = 50m
			expectedMemoryRequest: "100Mi",
		},
		{
			name: "regex filter - specific container",
			pod: &corev1.Pod{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  "large.engine.v4",
							Image: "nginx:1.14.2",
							Resources: corev1.ResourceRequirements{
								Limits: corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse("200m"),
									corev1.ResourceMemory: resource.MustParse("200Mi"),
								},
								Requests: corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse("50m"),
									corev1.ResourceMemory: resource.MustParse("100Mi"),
								},
							},
						},
						{
							Name:  "large.engine.v8",
							Image: "nginx:1.14.2",
							Resources: corev1.ResourceRequirements{
								Limits: corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse("400m"),
									corev1.ResourceMemory: resource.MustParse("400Mi"),
								},
								Requests: corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse("100m"),
									corev1.ResourceMemory: resource.MustParse("200Mi"),
								},
							},
						},
					},
				},
			},
			sidecarContainer: &appsv1alpha1.SidecarContainer{
				Container: corev1.Container{
					Name:  "sidecar1",
					Image: "sidecar:latest",
				},
				ResourcesPolicy: &appsv1alpha1.ResourcesPolicy{
					TargetContainerMode:       appsv1alpha1.TargetContainerModeSum,
					TargetContainersNameRegex: "^large.engine.v4$",
					ResourceExpr: appsv1alpha1.ResourceExpr{
						Limits: &appsv1alpha1.ResourceExprLimits{
							CPU:    "max(cpu*50%, 50m)",
							Memory: "200Mi",
						},
						Requests: &appsv1alpha1.ResourceExprRequests{
							CPU:    "max(cpu*50%, 50m)",
							Memory: "100Mi",
						},
					},
				},
			},
			matchedSidecarSets:    []sidecarcontrol.SidecarControl{},
			expectError:           false,
			expectedCPULimit:      "100m", // max(sum(200m) * 50%, 50m) = 100m
			expectedMemoryLimit:   "200Mi",
			expectedCPURequest:    "50m", // max(sum(50m) * 50%, 50m) = 50m
			expectedMemoryRequest: "100Mi",
		},
		{
			name: "no matching containers - should error",
			pod: &corev1.Pod{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  "app1",
							Image: "nginx:1.14.2",
							Resources: corev1.ResourceRequirements{
								Limits: corev1.ResourceList{
									corev1.ResourceCPU: resource.MustParse("200m"),
								},
							},
						},
					},
				},
			},
			sidecarContainer: &appsv1alpha1.SidecarContainer{
				Container: corev1.Container{
					Name:  "sidecar1",
					Image: "sidecar:latest",
				},
				ResourcesPolicy: &appsv1alpha1.ResourcesPolicy{
					TargetContainerMode:       appsv1alpha1.TargetContainerModeSum,
					TargetContainersNameRegex: "^nonexistent$",
					ResourceExpr: appsv1alpha1.ResourceExpr{
						Limits: &appsv1alpha1.ResourceExprLimits{
							CPU: "cpu*50%",
						},
					},
				},
			},
			matchedSidecarSets: []sidecarcontrol.SidecarControl{},
			expectError:        true,
		},
		{
			name: "sum mode - one container without CPU limit (unlimited)",
			pod: &corev1.Pod{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  "app1",
							Image: "nginx:1.14.2",
							Resources: corev1.ResourceRequirements{
								Limits: corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse("200m"),
									corev1.ResourceMemory: resource.MustParse("200Mi"),
								},
								Requests: corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse("100m"),
									corev1.ResourceMemory: resource.MustParse("100Mi"),
								},
							},
						},
						{
							Name:  "app2",
							Image: "nginx:1.14.2",
							Resources: corev1.ResourceRequirements{
								Limits: corev1.ResourceList{
									// No CPU limit - unlimited
									corev1.ResourceMemory: resource.MustParse("400Mi"),
								},
								Requests: corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse("50m"),
									corev1.ResourceMemory: resource.MustParse("200Mi"),
								},
							},
						},
					},
				},
			},
			sidecarContainer: &appsv1alpha1.SidecarContainer{
				Container: corev1.Container{
					Name:  "sidecar1",
					Image: "sidecar:latest",
				},
				ResourcesPolicy: &appsv1alpha1.ResourcesPolicy{
					TargetContainerMode:       appsv1alpha1.TargetContainerModeSum,
					TargetContainersNameRegex: ".*",
					ResourceExpr: appsv1alpha1.ResourceExpr{
						Limits: &appsv1alpha1.ResourceExprLimits{
							CPU:    "cpu*50%",
							Memory: "memory*30%",
						},
						Requests: &appsv1alpha1.ResourceExprRequests{
							CPU:    "cpu*30%",
							Memory: "memory*20%",
						},
					},
				},
			},
			matchedSidecarSets: []sidecarcontrol.SidecarControl{},
			expectError:        false,
			// CPU limit should NOT be set (unlimited) because app2 has no CPU limit
			expectNoCPULimit: true,
			// Memory limit should be set: (200Mi + 400Mi) * 30% = 180Mi
			expectedMemoryLimit: "180Mi",
			// CPU request should be set: (100m + 50m) * 30% = 45m
			expectedCPURequest: "45m",
			// Memory request should be set: (100Mi + 200Mi) * 20% = 60Mi
			expectedMemoryRequest: "60Mi",
		},
		{
			name: "max mode - one container without Memory limit (unlimited)",
			pod: &corev1.Pod{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  "app1",
							Image: "nginx:1.14.2",
							Resources: corev1.ResourceRequirements{
								Limits: corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse("200m"),
									corev1.ResourceMemory: resource.MustParse("200Mi"),
								},
								Requests: corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse("100m"),
									corev1.ResourceMemory: resource.MustParse("100Mi"),
								},
							},
						},
						{
							Name:  "app2",
							Image: "nginx:1.14.2",
							Resources: corev1.ResourceRequirements{
								Limits: corev1.ResourceList{
									corev1.ResourceCPU: resource.MustParse("400m"),
									// No Memory limit - unlimited
								},
								Requests: corev1.ResourceList{
									corev1.ResourceCPU: resource.MustParse("200m"),
									// No Memory request - treated as 0
								},
							},
						},
					},
				},
			},
			sidecarContainer: &appsv1alpha1.SidecarContainer{
				Container: corev1.Container{
					Name:  "sidecar1",
					Image: "sidecar:latest",
				},
				ResourcesPolicy: &appsv1alpha1.ResourcesPolicy{
					TargetContainerMode:       appsv1alpha1.TargetContainerModeMax,
					TargetContainersNameRegex: ".*",
					ResourceExpr: appsv1alpha1.ResourceExpr{
						Limits: &appsv1alpha1.ResourceExprLimits{
							CPU:    "cpu*40%",
							Memory: "memory*30%",
						},
						Requests: &appsv1alpha1.ResourceExprRequests{
							CPU:    "cpu*25%",
							Memory: "memory*20%",
						},
					},
				},
			},
			matchedSidecarSets: []sidecarcontrol.SidecarControl{},
			expectError:        false,
			// CPU limit should be set: max(200m, 400m) * 40% = 160m
			expectedCPULimit: "160m",
			// Memory limit should NOT be set (unlimited) because app2 has no Memory limit
			expectNoMemoryLimit: true,
			// CPU request should be set: max(100m, 200m) * 25% = 50m
			expectedCPURequest: "50m",
			// Memory request should be set: max(100Mi, 0) * 20% = 20Mi
			expectedMemoryRequest: "20Mi",
		},
		{
			name: "sum mode - container without requests (treated as 0)",
			pod: &corev1.Pod{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  "app1",
							Image: "nginx:1.14.2",
							Resources: corev1.ResourceRequirements{
								Limits: corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse("200m"),
									corev1.ResourceMemory: resource.MustParse("200Mi"),
								},
								Requests: corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse("100m"),
									corev1.ResourceMemory: resource.MustParse("100Mi"),
								},
							},
						},
						{
							Name:  "app2",
							Image: "nginx:1.14.2",
							Resources: corev1.ResourceRequirements{
								Limits: corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse("400m"),
									corev1.ResourceMemory: resource.MustParse("400Mi"),
								},
								// No requests - treated as 0
							},
						},
					},
				},
			},
			sidecarContainer: &appsv1alpha1.SidecarContainer{
				Container: corev1.Container{
					Name:  "sidecar1",
					Image: "sidecar:latest",
				},
				ResourcesPolicy: &appsv1alpha1.ResourcesPolicy{
					TargetContainerMode:       appsv1alpha1.TargetContainerModeSum,
					TargetContainersNameRegex: ".*",
					ResourceExpr: appsv1alpha1.ResourceExpr{
						Limits: &appsv1alpha1.ResourceExprLimits{
							CPU:    "cpu*50%",
							Memory: "memory*30%",
						},
						Requests: &appsv1alpha1.ResourceExprRequests{
							CPU:    "cpu*30%",
							Memory: "memory*20%",
						},
					},
				},
			},
			matchedSidecarSets: []sidecarcontrol.SidecarControl{},
			expectError:        false,
			// Limits should be set normally: sum(200m, 400m) * 50% = 300m
			expectedCPULimit:    "300m",
			expectedMemoryLimit: "180Mi", // sum(200Mi, 400Mi) * 30% = 180Mi
			// Requests: app2 has no requests, treated as 0
			expectedCPURequest:    "30m",  // sum(100m, 0) * 30% = 30m
			expectedMemoryRequest: "20Mi", // sum(100Mi, 0) * 20% = 20Mi
		},
		{
			name: "init-container with resource policy (native sidecar)",
			pod: &corev1.Pod{
				Spec: corev1.PodSpec{
					InitContainers: []corev1.Container{
						{
							Name:  "app1",
							Image: "nginx:1.14.2",
							Resources: corev1.ResourceRequirements{
								Limits: corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse("400m"),
									corev1.ResourceMemory: resource.MustParse("800Mi"),
								},
								Requests: corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse("200m"),
									corev1.ResourceMemory: resource.MustParse("400Mi"),
								},
							},
							RestartPolicy: ptr.To(corev1.ContainerRestartPolicyAlways),
						},
						{
							Name:  "app2",
							Image: "nginx:1.14.2",
							Resources: corev1.ResourceRequirements{
								Limits: corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse("400m"),
									corev1.ResourceMemory: resource.MustParse("800Mi"),
								},
								Requests: corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse("200m"),
									corev1.ResourceMemory: resource.MustParse("400Mi"),
								},
							},
						},
					},
					Containers: []corev1.Container{
						{
							Name:  "app2",
							Image: "nginx:1.14.2",
							Resources: corev1.ResourceRequirements{
								Limits: corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse("600m"),
									corev1.ResourceMemory: resource.MustParse("1200Mi"),
								},
								Requests: corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse("300m"),
									corev1.ResourceMemory: resource.MustParse("600Mi"),
								},
							},
						},
					},
				},
			},
			sidecarContainer: &appsv1alpha1.SidecarContainer{
				Container: corev1.Container{
					Name:  "init-sidecar",
					Image: "sidecar:latest",
				},
				ResourcesPolicy: &appsv1alpha1.ResourcesPolicy{
					TargetContainerMode:       appsv1alpha1.TargetContainerModeSum,
					TargetContainersNameRegex: "^app.*$",
					ResourceExpr: appsv1alpha1.ResourceExpr{
						Limits: &appsv1alpha1.ResourceExprLimits{
							CPU:    "max(cpu*30%, 50m)",
							Memory: "max(memory*25%, 100Mi)",
						},
						Requests: &appsv1alpha1.ResourceExprRequests{
							CPU:    "cpu*20%",
							Memory: "memory*15%",
						},
					},
				},
			},
			matchedSidecarSets: []sidecarcontrol.SidecarControl{},
			expectError:        false,
			// CPU limit: max((400m + 600m) * 30%, 50m) = max(300m, 50m) = 300m
			expectedCPULimit: "300m",
			// Memory limit: max((800Mi + 1200Mi) * 25%, 100Mi) = max(500Mi, 100Mi) = 500Mi
			expectedMemoryLimit: "500Mi",
			// CPU request: (200m + 300m) * 20% = 100m
			expectedCPURequest: "100m",
			// Memory request: (400Mi + 600Mi) * 15% = 150Mi
			expectedMemoryRequest: "150Mi",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := applyResourcesPolicy(tt.pod, tt.sidecarContainer, tt.matchedSidecarSets)

			if tt.expectError {
				if err == nil {
					t.Errorf("Expected error but got none")
				}
				return
			}

			if err != nil {
				t.Errorf("Unexpected error: %v", err)
				return
			}

			// Check CPU limit
			if tt.expectNoCPULimit {
				if _, ok := tt.sidecarContainer.Resources.Limits[corev1.ResourceCPU]; ok {
					t.Errorf("Expected CPU limit NOT to be set (unlimited), but it was set")
				}
			} else if tt.expectedCPULimit != "" {
				if cpuLimit, ok := tt.sidecarContainer.Resources.Limits[corev1.ResourceCPU]; !ok {
					t.Errorf("Expected CPU limit %s, but it was not set", tt.expectedCPULimit)
				} else if cpuLimit.String() != tt.expectedCPULimit {
					t.Errorf("Expected CPU limit %s, got %s", tt.expectedCPULimit, cpuLimit.String())
				}
			}

			// Check Memory limit
			if tt.expectNoMemoryLimit {
				if _, ok := tt.sidecarContainer.Resources.Limits[corev1.ResourceMemory]; ok {
					t.Errorf("Expected Memory limit NOT to be set (unlimited), but it was set")
				}
			} else if tt.expectedMemoryLimit != "" {
				if memLimit, ok := tt.sidecarContainer.Resources.Limits[corev1.ResourceMemory]; !ok {
					t.Errorf("Expected Memory limit %s, but it was not set", tt.expectedMemoryLimit)
				} else if memLimit.String() != tt.expectedMemoryLimit {
					t.Errorf("Expected Memory limit %s, got %s", tt.expectedMemoryLimit, memLimit.String())
				}
			}

			// Check CPU request
			if tt.expectedCPURequest != "" {
				if cpuRequest, ok := tt.sidecarContainer.Resources.Requests[corev1.ResourceCPU]; !ok {
					t.Errorf("Expected CPU request %s, but it was not set", tt.expectedCPURequest)
				} else if cpuRequest.String() != tt.expectedCPURequest {
					t.Errorf("Expected CPU request %s, got %s", tt.expectedCPURequest, cpuRequest.String())
				}
			}

			// Check Memory request
			if tt.expectedMemoryRequest != "" {
				if memRequest, ok := tt.sidecarContainer.Resources.Requests[corev1.ResourceMemory]; !ok {
					t.Errorf("Expected Memory request %s, but it was not set", tt.expectedMemoryRequest)
				} else if memRequest.String() != tt.expectedMemoryRequest {
					t.Errorf("Expected Memory request %s, got %s", tt.expectedMemoryRequest, memRequest.String())
				}
			}
		})
	}
}

func TestAggregateResourcesBySum(t *testing.T) {
	t.Run("all containers have limits", func(t *testing.T) {
		containers := []corev1.Container{
			{
				Name: "app1",
				Resources: corev1.ResourceRequirements{
					Limits: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("200m"),
						corev1.ResourceMemory: resource.MustParse("200Mi"),
					},
					Requests: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("50m"),
						corev1.ResourceMemory: resource.MustParse("100Mi"),
					},
				},
			},
			{
				Name: "app2",
				Resources: corev1.ResourceRequirements{
					Limits: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("400m"),
						corev1.ResourceMemory: resource.MustParse("400Mi"),
					},
					Requests: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("100m"),
						corev1.ResourceMemory: resource.MustParse("200Mi"),
					},
				},
			},
		}

		limits, requests := aggregateResourcesBySum(containers)

		// Check limits
		if cpuLimit := limits[corev1.ResourceCPU]; cpuLimit.String() != "600m" {
			t.Errorf("Expected CPU limit 600m, got %s", cpuLimit.String())
		}
		if memLimit := limits[corev1.ResourceMemory]; memLimit.String() != "600Mi" {
			t.Errorf("Expected Memory limit 600Mi, got %s", memLimit.String())
		}

		// Check requests
		if cpuRequest := requests[corev1.ResourceCPU]; cpuRequest.String() != "150m" {
			t.Errorf("Expected CPU request 150m, got %s", cpuRequest.String())
		}
		if memRequest := requests[corev1.ResourceMemory]; memRequest.String() != "300Mi" {
			t.Errorf("Expected Memory request 300Mi, got %s", memRequest.String())
		}
	})

	t.Run("one container without CPU limit - should be unlimited", func(t *testing.T) {
		containers := []corev1.Container{
			{
				Name: "app1",
				Resources: corev1.ResourceRequirements{
					Limits: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("200m"),
						corev1.ResourceMemory: resource.MustParse("200Mi"),
					},
					Requests: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("50m"),
						corev1.ResourceMemory: resource.MustParse("100Mi"),
					},
				},
			},
			{
				Name: "app2",
				Resources: corev1.ResourceRequirements{
					Limits: corev1.ResourceList{
						// No CPU limit - unlimited
						corev1.ResourceMemory: resource.MustParse("400Mi"),
					},
					Requests: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("100m"),
						corev1.ResourceMemory: resource.MustParse("200Mi"),
					},
				},
			},
		}

		limits, requests := aggregateResourcesBySum(containers)

		// CPU limit should NOT be set (unlimited)
		if _, ok := limits[corev1.ResourceCPU]; ok {
			t.Errorf("Expected CPU limit NOT to be set (unlimited), but it was set to %v", limits[corev1.ResourceCPU])
		}

		// Memory limit should be set: 200Mi + 400Mi = 600Mi
		if memLimit, ok := limits[corev1.ResourceMemory]; !ok {
			t.Errorf("Expected Memory limit to be set, but it was not")
		} else if memLimit.String() != "600Mi" {
			t.Errorf("Expected Memory limit 600Mi, got %s", memLimit.String())
		}

		// Requests should still be calculated (treat missing as 0)
		if cpuRequest := requests[corev1.ResourceCPU]; cpuRequest.String() != "150m" {
			t.Errorf("Expected CPU request 150m, got %s", cpuRequest.String())
		}
		if memRequest := requests[corev1.ResourceMemory]; memRequest.String() != "300Mi" {
			t.Errorf("Expected Memory request 300Mi, got %s", memRequest.String())
		}
	})

	t.Run("container without requests - treated as 0", func(t *testing.T) {
		containers := []corev1.Container{
			{
				Name: "app1",
				Resources: corev1.ResourceRequirements{
					Limits: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("200m"),
						corev1.ResourceMemory: resource.MustParse("200Mi"),
					},
					Requests: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("50m"),
						corev1.ResourceMemory: resource.MustParse("100Mi"),
					},
				},
			},
			{
				Name: "app2",
				Resources: corev1.ResourceRequirements{
					Limits: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("400m"),
						corev1.ResourceMemory: resource.MustParse("400Mi"),
					},
					// No requests - treated as 0
				},
			},
		}

		limits, requests := aggregateResourcesBySum(containers)

		// Limits should be summed normally
		if cpuLimit := limits[corev1.ResourceCPU]; cpuLimit.String() != "600m" {
			t.Errorf("Expected CPU limit 600m, got %s", cpuLimit.String())
		}
		if memLimit := limits[corev1.ResourceMemory]; memLimit.String() != "600Mi" {
			t.Errorf("Expected Memory limit 600Mi, got %s", memLimit.String())
		}

		// Requests: app2 has no requests, treated as 0
		// sum(50m, 0) = 50m
		if cpuRequest := requests[corev1.ResourceCPU]; cpuRequest.String() != "50m" {
			t.Errorf("Expected CPU request 50m (sum(50m, 0)), got %s", cpuRequest.String())
		}
		// sum(100Mi, 0) = 100Mi
		if memRequest := requests[corev1.ResourceMemory]; memRequest.String() != "100Mi" {
			t.Errorf("Expected Memory request 100Mi (sum(100Mi, 0)), got %s", memRequest.String())
		}
	})
}

func TestAggregateResourcesByMax(t *testing.T) {
	t.Run("all containers have limits", func(t *testing.T) {
		containers := []corev1.Container{
			{
				Name: "app1",
				Resources: corev1.ResourceRequirements{
					Limits: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("200m"),
						corev1.ResourceMemory: resource.MustParse("200Mi"),
					},
					Requests: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("50m"),
						corev1.ResourceMemory: resource.MustParse("100Mi"),
					},
				},
			},
			{
				Name: "app2",
				Resources: corev1.ResourceRequirements{
					Limits: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("400m"),
						corev1.ResourceMemory: resource.MustParse("400Mi"),
					},
					Requests: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("100m"),
						corev1.ResourceMemory: resource.MustParse("200Mi"),
					},
				},
			},
		}

		limits, requests := aggregateResourcesByMax(containers)

		// Check limits
		if cpuLimit := limits[corev1.ResourceCPU]; cpuLimit.String() != "400m" {
			t.Errorf("Expected CPU limit 400m, got %s", cpuLimit.String())
		}
		if memLimit := limits[corev1.ResourceMemory]; memLimit.String() != "400Mi" {
			t.Errorf("Expected Memory limit 400Mi, got %s", memLimit.String())
		}

		// Check requests
		if cpuRequest := requests[corev1.ResourceCPU]; cpuRequest.String() != "100m" {
			t.Errorf("Expected CPU request 100m, got %s", cpuRequest.String())
		}
		if memRequest := requests[corev1.ResourceMemory]; memRequest.String() != "200Mi" {
			t.Errorf("Expected Memory request 200Mi, got %s", memRequest.String())
		}
	})

	t.Run("one container without Memory limit - should be unlimited", func(t *testing.T) {
		containers := []corev1.Container{
			{
				Name: "app1",
				Resources: corev1.ResourceRequirements{
					Limits: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("200m"),
						corev1.ResourceMemory: resource.MustParse("200Mi"),
					},
					Requests: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("50m"),
						corev1.ResourceMemory: resource.MustParse("100Mi"),
					},
				},
			},
			{
				Name: "app2",
				Resources: corev1.ResourceRequirements{
					Limits: corev1.ResourceList{
						corev1.ResourceCPU: resource.MustParse("400m"),
						// No Memory limit - unlimited
					},
					Requests: corev1.ResourceList{
						corev1.ResourceCPU: resource.MustParse("100m"),
						// No Memory request
					},
				},
			},
		}

		limits, requests := aggregateResourcesByMax(containers)

		// CPU limit should be set: max(200m, 400m) = 400m
		if cpuLimit, ok := limits[corev1.ResourceCPU]; !ok {
			t.Errorf("Expected CPU limit to be set, but it was not")
		} else if cpuLimit.String() != "400m" {
			t.Errorf("Expected CPU limit 400m, got %s", cpuLimit.String())
		}

		// Memory limit should NOT be set (unlimited)
		if _, ok := limits[corev1.ResourceMemory]; ok {
			t.Errorf("Expected Memory limit NOT to be set (unlimited), but it was set to %v", limits[corev1.ResourceMemory])
		}

		// Requests: max(50m, 100m) = 100m
		if cpuRequest := requests[corev1.ResourceCPU]; cpuRequest.String() != "100m" {
			t.Errorf("Expected CPU request 100m, got %s", cpuRequest.String())
		}

		// Memory request: max(100Mi, 0) = 100Mi (missing treated as 0)
		if memRequest := requests[corev1.ResourceMemory]; memRequest.String() != "100Mi" {
			t.Errorf("Expected Memory request 100Mi (max(100Mi, 0)), got %s", memRequest.String())
		}
	})

	t.Run("one container without Cpu limit - should be unlimited", func(t *testing.T) {
		containers := []corev1.Container{
			{
				Name: "app1",
				Resources: corev1.ResourceRequirements{
					Limits: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("200m"),
						corev1.ResourceMemory: resource.MustParse("200Mi"),
					},
					Requests: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("50m"),
						corev1.ResourceMemory: resource.MustParse("100Mi"),
					},
				},
			},
			{
				Name: "app2",
				Resources: corev1.ResourceRequirements{
					Limits: corev1.ResourceList{
						// No CPU limit - unlimited
						corev1.ResourceMemory: resource.MustParse("400Mi"),
					},
					Requests: corev1.ResourceList{
						// No CPU request
						corev1.ResourceMemory: resource.MustParse("200Mi"),
					},
				},
			},
		}

		limits, requests := aggregateResourcesByMax(containers)

		// Memory limit should be set: max(200Mi, 400Mi) = 400Mi
		if memLimit, ok := limits[corev1.ResourceMemory]; !ok {
			t.Errorf("Expected Memory limit to be set, but it was not")
		} else if memLimit.String() != "400Mi" {
			t.Errorf("Expected Memory limit 400Mi, got %s", memLimit.String())
		}

		// Cpu limit should NOT be set (unlimited)
		if _, ok := limits[corev1.ResourceCPU]; ok {
			t.Errorf("Expected CPU limit NOT to be set (unlimited), but it was set to %v", limits[corev1.ResourceCPU])
		}

		// Requests: max(50m, 0) = 50m
		if cpuRequest := requests[corev1.ResourceCPU]; cpuRequest.String() != "50m" {
			t.Errorf("Expected CPU request 50m, got %s", cpuRequest.String())
		}

		// Memory request: max(100Mi, 200Mi) = 200Mi (missing treated as 0)
		if memRequest := requests[corev1.ResourceMemory]; memRequest.String() != "200Mi" {
			t.Errorf("Expected Memory request 200Mi (max(100Mi, 200Mi)), got %s", memRequest.String())
		}
	})

	t.Run("container without requests - treated as 0", func(t *testing.T) {
		containers := []corev1.Container{
			{
				Name: "app1",
				Resources: corev1.ResourceRequirements{
					Limits: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("200m"),
						corev1.ResourceMemory: resource.MustParse("200Mi"),
					},
					Requests: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("50m"),
						corev1.ResourceMemory: resource.MustParse("100Mi"),
					},
				},
			},
			{
				Name: "app2",
				Resources: corev1.ResourceRequirements{
					Limits: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("100m"),
						corev1.ResourceMemory: resource.MustParse("150Mi"),
					},
					// No requests - treated as 0
				},
			},
		}

		limits, requests := aggregateResourcesByMax(containers)

		// Limits should use max
		if cpuLimit := limits[corev1.ResourceCPU]; cpuLimit.String() != "200m" {
			t.Errorf("Expected CPU limit 200m (max(200m, 100m)), got %s", cpuLimit.String())
		}
		if memLimit := limits[corev1.ResourceMemory]; memLimit.String() != "200Mi" {
			t.Errorf("Expected Memory limit 200Mi (max(200Mi, 150Mi)), got %s", memLimit.String())
		}

		// Requests: app2 has no requests, treated as 0
		// max(50m, 0) = 50m
		if cpuRequest := requests[corev1.ResourceCPU]; cpuRequest.String() != "50m" {
			t.Errorf("Expected CPU request 50m (max(50m, 0)), got %s", cpuRequest.String())
		}
		// max(100Mi, 0) = 100Mi
		if memRequest := requests[corev1.ResourceMemory]; memRequest.String() != "100Mi" {
			t.Errorf("Expected Memory request 100Mi (max(100Mi, 0)), got %s", memRequest.String())
		}
	})
}

func TestGetTargetContainers(t *testing.T) {
	// Mock sidecarset
	mockSidecarSet := &appsv1alpha1.SidecarSet{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-sidecarset",
		},
		Spec: appsv1alpha1.SidecarSetSpec{
			InitContainers: []appsv1alpha1.SidecarContainer{
				{
					Container: corev1.Container{
						Name: "init-sidecar1",
					},
				},
			},
			Containers: []appsv1alpha1.SidecarContainer{
				{
					Container: corev1.Container{
						Name: "sidecar1",
					},
				},
			},
		},
	}

	restartAlways := corev1.ContainerRestartPolicyAlways
	pod := &corev1.Pod{
		Spec: corev1.PodSpec{
			InitContainers: []corev1.Container{
				{Name: "init-sidecar1"},        // This is a Kruise sidecar, should be excluded
				{Name: "plain-init-container"}, // This is a plain init-container, should be excluded
				{ // This is a native sidecar container, should be included
					Name:          "sidecar-container",
					RestartPolicy: &restartAlways},
			},
			Containers: []corev1.Container{
				{Name: "app1"},
				{Name: "app2"},
				{Name: "sidecar1"}, // This is a Kruise sidecar, should be excluded
			},
		},
	}

	matchedSidecarSets := []sidecarcontrol.SidecarControl{
		sidecarcontrol.New(mockSidecarSet),
	}

	containers, err := getTargetContainers(pod, "^.*$", matchedSidecarSets)
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}

	if len(containers) != 3 {
		t.Errorf("Expected 3 containers, got %d", len(containers))
	}

	// Verify that sidecar1 is excluded
	for _, c := range containers {
		if c.Name == "sidecar1" {
			t.Errorf("Kruise sidecar container should be excluded")
		}
		if c.Name == "init-sidecar1" {
			t.Errorf("Kruise init-container should be excluded")
		}
		if c.Name == "plain-init-container" {
			t.Errorf("Plain init-container should be excluded")
		}

	}
}

func TestEvaluateResourceExpression(t *testing.T) {
	tests := []struct {
		name            string
		expr            string
		aggregatedValue resource.Quantity
		isLimit         bool
		expectNil       bool // Expected to return nil (unlimited)
		expectError     bool
		expectedValue   string
	}{
		{
			name:            "empty expression with isLimit=true should return nil (unlimited)",
			expr:            "",
			aggregatedValue: resource.MustParse("100m"),
			isLimit:         true,
			expectNil:       true,
			expectError:     false,
		},
		{
			name:            "empty expression with isLimit=false should return 0",
			expr:            "",
			aggregatedValue: resource.MustParse("100m"),
			isLimit:         false,
			expectNil:       false,
			expectError:     false,
			expectedValue:   "0",
		},
		{
			name:            "percentage expression - cpu*50%",
			expr:            "cpu*50%",
			aggregatedValue: resource.MustParse("200m"),
			isLimit:         true,
			expectNil:       false,
			expectError:     false,
			expectedValue:   "100m",
		},
		{
			name:            "percentage expression - memory*30%",
			expr:            "memory*30%",
			aggregatedValue: resource.MustParse("600Mi"),
			isLimit:         true,
			expectNil:       false,
			expectError:     false,
			expectedValue:   "180Mi",
		},
		{
			name:            "max expression - max(cpu*50%, 100m)",
			expr:            "max(cpu*50%, 100m)",
			aggregatedValue: resource.MustParse("150m"),
			isLimit:         true,
			expectNil:       false,
			expectError:     false,
			expectedValue:   "100m", // max(75m, 100m) = 100m
		},
		{
			name:            "max expression with higher percentage - max(cpu*50%, 100m)",
			expr:            "max(cpu*50%, 100m)",
			aggregatedValue: resource.MustParse("400m"),
			isLimit:         true,
			expectNil:       false,
			expectError:     false,
			expectedValue:   "200m", // max(200m, 100m) = 200m
		},
		{
			name:            "addition expression - cpu + 50m",
			expr:            "cpu + 50m",
			aggregatedValue: resource.MustParse("100m"),
			isLimit:         true,
			expectNil:       false,
			expectError:     false,
			expectedValue:   "150m",
		},
		{
			name:            "complex expression - max(memory*20% + 100Mi, 200Mi)",
			expr:            "max(memory*20% + 100Mi, 200Mi)",
			aggregatedValue: resource.MustParse("800Mi"),
			isLimit:         true,
			expectNil:       false,
			expectError:     false,
			expectedValue:   "260Mi", // max(160Mi + 100Mi, 200Mi) = max(260Mi, 200Mi) = 260Mi
		},
		{
			name:            "number result - constant value 500m",
			expr:            "500m",
			aggregatedValue: resource.MustParse("100m"),
			isLimit:         true,
			expectNil:       false,
			expectError:     false,
			expectedValue:   "500m",
		},
		{
			name:            "memory constant - 1Gi",
			expr:            "1Gi",
			aggregatedValue: resource.MustParse("500Mi"),
			isLimit:         true,
			expectNil:       false,
			expectError:     false,
			expectedValue:   "1Gi",
		},
		{
			name:            "invalid expression should return error",
			expr:            "cpu * invalid",
			aggregatedValue: resource.MustParse("100m"),
			isLimit:         true,
			expectNil:       false,
			expectError:     true,
		},
		{
			name:            "zero aggregated value with isLimit=false (request)",
			expr:            "cpu*50%",
			aggregatedValue: resource.MustParse("0"),
			isLimit:         false,
			expectNil:       false,
			expectError:     false,
			expectedValue:   "0",
		},
		{
			name:            "percentage of memory request",
			expr:            "memory*25%",
			aggregatedValue: resource.MustParse("400Mi"),
			isLimit:         false,
			expectNil:       false,
			expectError:     false,
			expectedValue:   "100Mi",
		},
		{
			name:            "number cpu",
			expr:            "1",
			aggregatedValue: resource.MustParse("2"),
			isLimit:         false,
			expectNil:       false,
			expectError:     false,
			expectedValue:   "1",
		},
		{
			name:            "number memory",
			expr:            "1",
			aggregatedValue: resource.MustParse("2Mi"),
			isLimit:         false,
			expectNil:       false,
			expectError:     false,
			expectedValue:   "1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := evaluateResourceExpression(tt.expr, tt.aggregatedValue, tt.isLimit)

			// Check error expectation
			if tt.expectError {
				if err == nil {
					t.Errorf("Expected error but got none")
				}
				return
			}

			if err != nil {
				t.Errorf("Unexpected error: %v", err)
				return
			}

			// Check nil expectation (unlimited)
			if tt.expectNil {
				if result != nil {
					t.Errorf("Expected nil (unlimited) but got: %v", result)
				}
				return
			}

			// Check value
			if result == nil {
				t.Errorf("Expected result %s but got nil", tt.expectedValue)
				return
			}

			if result.String() != tt.expectedValue {
				t.Errorf("Expected result %s, got %s", tt.expectedValue, result.String())
			}
		})
	}
}
