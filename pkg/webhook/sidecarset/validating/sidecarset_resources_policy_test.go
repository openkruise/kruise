/*
Copyright 2019 The Kruise Authors.

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

package validating

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/validation/field"

	appsv1beta1 "github.com/openkruise/kruise/apis/apps/v1beta1"
)

func init() {
	testScheme = runtime.NewScheme()
	utilruntime.Must(appsv1beta1.AddToScheme(testScheme))
	utilruntime.Must(corev1.AddToScheme(testScheme))
	handler = &SidecarSetCreateUpdateHandler{}
}

func TestValidateResourcesPolicy(t *testing.T) {
	tests := []struct {
		name          string
		container     appsv1beta1.SidecarContainer
		expectErrors  int
		errorContains []string
	}{
		{
			name: "valid resources policy with sum mode and both limits and requests",
			container: appsv1beta1.SidecarContainer{
				Container: corev1.Container{
					Name:                     "test-sidecar",
					Image:                    "test-image",
					ImagePullPolicy:          corev1.PullIfNotPresent,
					TerminationMessagePolicy: corev1.TerminationMessageReadFile,
				},
				PodInjectPolicy: appsv1beta1.BeforeAppContainerType,
				ShareVolumePolicy: appsv1beta1.ShareVolumePolicy{
					Type: appsv1beta1.ShareVolumePolicyDisabled,
				},
				ResourcesPolicy: &appsv1beta1.ResourcesPolicy{
					TargetContainersMode:      appsv1beta1.TargetContainersModeSum,
					TargetContainersNameRegex: "^app.*$",
					ResourcesExpr: appsv1beta1.ResourcesExpr{
						Limits: &appsv1beta1.ResourceExprLimits{
							CPU:    "max(cpu*50%, 50m)",
							Memory: "max(memory*50%, 100Mi)",
						},
						Requests: &appsv1beta1.ResourceExprRequests{
							CPU:    "max(cpu*50%, 50m)",
							Memory: "max(memory*50%, 100Mi)",
						},
					},
				},
			},
			expectErrors: 0,
		},
		{
			name: "valid resources policy with max mode and only limits",
			container: appsv1beta1.SidecarContainer{
				Container: corev1.Container{
					Name:                     "test-sidecar",
					Image:                    "test-image",
					ImagePullPolicy:          corev1.PullIfNotPresent,
					TerminationMessagePolicy: corev1.TerminationMessageReadFile,
				},
				PodInjectPolicy: appsv1beta1.BeforeAppContainerType,
				ShareVolumePolicy: appsv1beta1.ShareVolumePolicy{
					Type: appsv1beta1.ShareVolumePolicyDisabled,
				},
				ResourcesPolicy: &appsv1beta1.ResourcesPolicy{
					TargetContainersMode:      appsv1beta1.TargetContainersModeMax,
					TargetContainersNameRegex: "^app.*$",
					ResourcesExpr: appsv1beta1.ResourcesExpr{
						Limits: &appsv1beta1.ResourceExprLimits{
							CPU: "cpu*50%",
						},
					},
				},
			},
			expectErrors: 0,
		},
		{
			name: "valid resources policy with only requests",
			container: appsv1beta1.SidecarContainer{
				Container: corev1.Container{
					Name:                     "test-sidecar",
					Image:                    "test-image",
					ImagePullPolicy:          corev1.PullIfNotPresent,
					TerminationMessagePolicy: corev1.TerminationMessageReadFile,
				},
				PodInjectPolicy: appsv1beta1.BeforeAppContainerType,
				ShareVolumePolicy: appsv1beta1.ShareVolumePolicy{
					Type: appsv1beta1.ShareVolumePolicyDisabled,
				},
				ResourcesPolicy: &appsv1beta1.ResourcesPolicy{
					TargetContainersMode:      appsv1beta1.TargetContainersModeSum,
					TargetContainersNameRegex: ".*",
					ResourcesExpr: appsv1beta1.ResourcesExpr{
						Requests: &appsv1beta1.ResourceExprRequests{
							CPU:    "max(cpu*50%, 50m)",
							Memory: "100Mi",
						},
					},
				},
			},
			expectErrors: 0,
		},
		{
			name: "valid resources policy with empty regex",
			container: appsv1beta1.SidecarContainer{
				Container: corev1.Container{
					Name:                     "test-sidecar",
					Image:                    "test-image",
					ImagePullPolicy:          corev1.PullIfNotPresent,
					TerminationMessagePolicy: corev1.TerminationMessageReadFile,
				},
				PodInjectPolicy: appsv1beta1.BeforeAppContainerType,
				ShareVolumePolicy: appsv1beta1.ShareVolumePolicy{
					Type: appsv1beta1.ShareVolumePolicyDisabled,
				},
				ResourcesPolicy: &appsv1beta1.ResourcesPolicy{
					TargetContainersMode:      appsv1beta1.TargetContainersModeSum,
					TargetContainersNameRegex: "",
					ResourcesExpr: appsv1beta1.ResourcesExpr{
						Limits: &appsv1beta1.ResourceExprLimits{
							CPU: "cpu*50%",
						},
					},
				},
			},
			expectErrors: 1,
		},
		{
			name: "resources policy and resources.limits both configured - should fail",
			container: appsv1beta1.SidecarContainer{
				Container: corev1.Container{
					Name:                     "test-sidecar",
					Image:                    "test-image",
					ImagePullPolicy:          corev1.PullIfNotPresent,
					TerminationMessagePolicy: corev1.TerminationMessageReadFile,
					Resources: corev1.ResourceRequirements{
						Limits: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse("100m"),
							corev1.ResourceMemory: resource.MustParse("100Mi"),
						},
					},
				},
				PodInjectPolicy: appsv1beta1.BeforeAppContainerType,
				ShareVolumePolicy: appsv1beta1.ShareVolumePolicy{
					Type: appsv1beta1.ShareVolumePolicyDisabled,
				},
				ResourcesPolicy: &appsv1beta1.ResourcesPolicy{
					TargetContainersMode: appsv1beta1.TargetContainersModeSum,
					ResourcesExpr: appsv1beta1.ResourcesExpr{
						Limits: &appsv1beta1.ResourceExprLimits{
							CPU: "cpu*50%",
						},
					},
				},
			},
			expectErrors:  1,
			errorContains: []string{"resourcesPolicy and resources cannot be configured together"},
		},
		{
			name: "resources policy and resources.requests both configured - should fail",
			container: appsv1beta1.SidecarContainer{
				Container: corev1.Container{
					Name:                     "test-sidecar",
					Image:                    "test-image",
					ImagePullPolicy:          corev1.PullIfNotPresent,
					TerminationMessagePolicy: corev1.TerminationMessageReadFile,
					Resources: corev1.ResourceRequirements{
						Requests: corev1.ResourceList{
							corev1.ResourceCPU: resource.MustParse("50m"),
						},
					},
				},
				PodInjectPolicy: appsv1beta1.BeforeAppContainerType,
				ShareVolumePolicy: appsv1beta1.ShareVolumePolicy{
					Type: appsv1beta1.ShareVolumePolicyDisabled,
				},
				ResourcesPolicy: &appsv1beta1.ResourcesPolicy{
					TargetContainersMode: appsv1beta1.TargetContainersModeSum,
					ResourcesExpr: appsv1beta1.ResourcesExpr{
						Limits: &appsv1beta1.ResourceExprLimits{
							CPU: "cpu*50%",
						},
					},
				},
			},
			expectErrors:  1,
			errorContains: []string{"resourcesPolicy and resources cannot be configured together"},
		},
		{
			name: "invalid regex pattern - should fail",
			container: appsv1beta1.SidecarContainer{
				Container: corev1.Container{
					Name:                     "test-sidecar",
					Image:                    "test-image",
					ImagePullPolicy:          corev1.PullIfNotPresent,
					TerminationMessagePolicy: corev1.TerminationMessageReadFile,
				},
				PodInjectPolicy: appsv1beta1.BeforeAppContainerType,
				ShareVolumePolicy: appsv1beta1.ShareVolumePolicy{
					Type: appsv1beta1.ShareVolumePolicyDisabled,
				},
				ResourcesPolicy: &appsv1beta1.ResourcesPolicy{
					TargetContainersMode:      appsv1beta1.TargetContainersModeSum,
					TargetContainersNameRegex: "[invalid(regex",
					ResourcesExpr: appsv1beta1.ResourcesExpr{
						Limits: &appsv1beta1.ResourceExprLimits{
							CPU: "cpu*50%",
						},
					},
				},
			},
			expectErrors:  1,
			errorContains: []string{"invalid regex pattern"},
		},
		{
			name: "no limits and requests - should fail",
			container: appsv1beta1.SidecarContainer{
				Container: corev1.Container{
					Name:                     "test-sidecar",
					Image:                    "test-image",
					ImagePullPolicy:          corev1.PullIfNotPresent,
					TerminationMessagePolicy: corev1.TerminationMessageReadFile,
				},
				PodInjectPolicy: appsv1beta1.BeforeAppContainerType,
				ShareVolumePolicy: appsv1beta1.ShareVolumePolicy{
					Type: appsv1beta1.ShareVolumePolicyDisabled,
				},
				ResourcesPolicy: &appsv1beta1.ResourcesPolicy{
					TargetContainersMode:      appsv1beta1.TargetContainersModeSum,
					TargetContainersNameRegex: "^app.*$",
					ResourcesExpr:             appsv1beta1.ResourcesExpr{},
				},
			},
			expectErrors:  1,
			errorContains: []string{"at least one of limits or requests must be configured"},
		},
		{
			name: "invalid cpu expression - should fail",
			container: appsv1beta1.SidecarContainer{
				Container: corev1.Container{
					Name:                     "test-sidecar",
					Image:                    "test-image",
					ImagePullPolicy:          corev1.PullIfNotPresent,
					TerminationMessagePolicy: corev1.TerminationMessageReadFile,
				},
				PodInjectPolicy: appsv1beta1.BeforeAppContainerType,
				ShareVolumePolicy: appsv1beta1.ShareVolumePolicy{
					Type: appsv1beta1.ShareVolumePolicyDisabled,
				},
				ResourcesPolicy: &appsv1beta1.ResourcesPolicy{
					TargetContainersMode:      appsv1beta1.TargetContainersModeSum,
					TargetContainersNameRegex: "^app.*$",
					ResourcesExpr: appsv1beta1.ResourcesExpr{
						Limits: &appsv1beta1.ResourceExprLimits{
							CPU: "invalid expression @#$",
						},
					},
				},
			},
			expectErrors:  1,
			errorContains: []string{"invalid expression"},
		},
		{
			name: "unbalanced parentheses - should fail",
			container: appsv1beta1.SidecarContainer{
				Container: corev1.Container{
					Name:                     "test-sidecar",
					Image:                    "test-image",
					ImagePullPolicy:          corev1.PullIfNotPresent,
					TerminationMessagePolicy: corev1.TerminationMessageReadFile,
				},
				PodInjectPolicy: appsv1beta1.BeforeAppContainerType,
				ShareVolumePolicy: appsv1beta1.ShareVolumePolicy{
					Type: appsv1beta1.ShareVolumePolicyDisabled,
				},
				ResourcesPolicy: &appsv1beta1.ResourcesPolicy{
					TargetContainersMode:      appsv1beta1.TargetContainersModeSum,
					TargetContainersNameRegex: "^app.*$",
					ResourcesExpr: appsv1beta1.ResourcesExpr{
						Limits: &appsv1beta1.ResourceExprLimits{
							CPU: "max(cpu*50%, 50m",
						},
					},
				},
			},
			expectErrors:  1,
			errorContains: []string{"invalid expression"},
		},
		{
			name: "invalid function syntax - should fail",
			container: appsv1beta1.SidecarContainer{
				Container: corev1.Container{
					Name:                     "test-sidecar",
					Image:                    "test-image",
					ImagePullPolicy:          corev1.PullIfNotPresent,
					TerminationMessagePolicy: corev1.TerminationMessageReadFile,
				},
				PodInjectPolicy: appsv1beta1.BeforeAppContainerType,
				ShareVolumePolicy: appsv1beta1.ShareVolumePolicy{
					Type: appsv1beta1.ShareVolumePolicyDisabled,
				},
				ResourcesPolicy: &appsv1beta1.ResourcesPolicy{
					TargetContainersMode:      appsv1beta1.TargetContainersModeSum,
					TargetContainersNameRegex: "^app.*$",
					ResourcesExpr: appsv1beta1.ResourcesExpr{
						Limits: &appsv1beta1.ResourceExprLimits{
							CPU: "max(cpu*50%)",
						},
					},
				},
			},
			expectErrors:  1,
			errorContains: []string{"invalid expression"},
		},
		{
			name: "invalid variable name - should fail",
			container: appsv1beta1.SidecarContainer{
				Container: corev1.Container{
					Name:                     "test-sidecar",
					Image:                    "test-image",
					ImagePullPolicy:          corev1.PullIfNotPresent,
					TerminationMessagePolicy: corev1.TerminationMessageReadFile,
				},
				PodInjectPolicy: appsv1beta1.BeforeAppContainerType,
				ShareVolumePolicy: appsv1beta1.ShareVolumePolicy{
					Type: appsv1beta1.ShareVolumePolicyDisabled,
				},
				ResourcesPolicy: &appsv1beta1.ResourcesPolicy{
					TargetContainersMode:      appsv1beta1.TargetContainersModeSum,
					TargetContainersNameRegex: "^app.*$",
					ResourcesExpr: appsv1beta1.ResourcesExpr{
						Limits: &appsv1beta1.ResourceExprLimits{
							CPU: "invalidvar * 50%",
						},
					},
				},
			},
			expectErrors:  1,
			errorContains: []string{"invalid expression"},
		},
		{
			name: "valid complex expression from story 5",
			container: appsv1beta1.SidecarContainer{
				Container: corev1.Container{
					Name:                     "test-sidecar",
					Image:                    "test-image",
					ImagePullPolicy:          corev1.PullIfNotPresent,
					TerminationMessagePolicy: corev1.TerminationMessageReadFile,
				},
				PodInjectPolicy: appsv1beta1.BeforeAppContainerType,
				ShareVolumePolicy: appsv1beta1.ShareVolumePolicy{
					Type: appsv1beta1.ShareVolumePolicyDisabled,
				},
				ResourcesPolicy: &appsv1beta1.ResourcesPolicy{
					TargetContainersMode:      appsv1beta1.TargetContainersModeSum,
					TargetContainersNameRegex: "^app.*$",
					ResourcesExpr: appsv1beta1.ResourcesExpr{
						Limits: &appsv1beta1.ResourceExprLimits{
							CPU:    "0.5*cpu - 0.3*max(0, cpu-4) + 0.3*max(0, cpu-8)",
							Memory: "max(memory*50%, 100Mi)",
						},
					},
				},
			},
			expectErrors: 0,
		},
		{
			name: "valid with only cpu in limits",
			container: appsv1beta1.SidecarContainer{
				Container: corev1.Container{
					Name:                     "test-sidecar",
					Image:                    "test-image",
					ImagePullPolicy:          corev1.PullIfNotPresent,
					TerminationMessagePolicy: corev1.TerminationMessageReadFile,
				},
				PodInjectPolicy: appsv1beta1.BeforeAppContainerType,
				ShareVolumePolicy: appsv1beta1.ShareVolumePolicy{
					Type: appsv1beta1.ShareVolumePolicyDisabled,
				},
				ResourcesPolicy: &appsv1beta1.ResourcesPolicy{
					TargetContainersMode:      appsv1beta1.TargetContainersModeMax,
					TargetContainersNameRegex: "^.*",
					ResourcesExpr: appsv1beta1.ResourcesExpr{
						Limits: &appsv1beta1.ResourceExprLimits{
							CPU: "cpu*50%",
						},
					},
				},
			},
			expectErrors: 0,
		},
		{
			name: "valid with only memory in limits",
			container: appsv1beta1.SidecarContainer{
				Container: corev1.Container{
					Name:                     "test-sidecar",
					Image:                    "test-image",
					ImagePullPolicy:          corev1.PullIfNotPresent,
					TerminationMessagePolicy: corev1.TerminationMessageReadFile,
				},
				PodInjectPolicy: appsv1beta1.BeforeAppContainerType,
				ShareVolumePolicy: appsv1beta1.ShareVolumePolicy{
					Type: appsv1beta1.ShareVolumePolicyDisabled,
				},
				ResourcesPolicy: &appsv1beta1.ResourcesPolicy{
					TargetContainersMode:      appsv1beta1.TargetContainersModeMax,
					TargetContainersNameRegex: "^.*",
					ResourcesExpr: appsv1beta1.ResourcesExpr{
						Limits: &appsv1beta1.ResourceExprLimits{
							Memory: "memory*50%",
						},
					},
				},
			},
			expectErrors: 0,
		},
		{
			name: "empty cpu expression in requests is valid",
			container: appsv1beta1.SidecarContainer{
				Container: corev1.Container{
					Name:                     "test-sidecar",
					Image:                    "test-image",
					ImagePullPolicy:          corev1.PullIfNotPresent,
					TerminationMessagePolicy: corev1.TerminationMessageReadFile,
				},
				PodInjectPolicy: appsv1beta1.BeforeAppContainerType,
				ShareVolumePolicy: appsv1beta1.ShareVolumePolicy{
					Type: appsv1beta1.ShareVolumePolicyDisabled,
				},
				ResourcesPolicy: &appsv1beta1.ResourcesPolicy{
					TargetContainersMode:      appsv1beta1.TargetContainersModeSum,
					TargetContainersNameRegex: "^.*",
					ResourcesExpr: appsv1beta1.ResourcesExpr{
						Requests: &appsv1beta1.ResourceExprRequests{
							Memory: "100Mi",
						},
					},
				},
			},
			expectErrors: 0,
		},
		{
			name: "invalid cpu expression in requests - should fail",
			container: appsv1beta1.SidecarContainer{
				Container: corev1.Container{
					Name:                     "test-sidecar",
					Image:                    "test-image",
					ImagePullPolicy:          corev1.PullIfNotPresent,
					TerminationMessagePolicy: corev1.TerminationMessageReadFile,
				},
				PodInjectPolicy: appsv1beta1.BeforeAppContainerType,
				ShareVolumePolicy: appsv1beta1.ShareVolumePolicy{
					Type: appsv1beta1.ShareVolumePolicyDisabled,
				},
				ResourcesPolicy: &appsv1beta1.ResourcesPolicy{
					TargetContainersMode:      appsv1beta1.TargetContainersModeSum,
					TargetContainersNameRegex: "^.*",
					ResourcesExpr: appsv1beta1.ResourcesExpr{
						Requests: &appsv1beta1.ResourceExprRequests{
							CPU: "invalid_cpu_expr",
						},
					},
				},
			},
			expectErrors:  1,
			errorContains: []string{"invalid expression"},
		},
		{
			name: "invalid memory expression in limits - should fail",
			container: appsv1beta1.SidecarContainer{
				Container: corev1.Container{
					Name:                     "test-sidecar",
					Image:                    "test-image",
					ImagePullPolicy:          corev1.PullIfNotPresent,
					TerminationMessagePolicy: corev1.TerminationMessageReadFile,
				},
				PodInjectPolicy: appsv1beta1.BeforeAppContainerType,
				ShareVolumePolicy: appsv1beta1.ShareVolumePolicy{
					Type: appsv1beta1.ShareVolumePolicyDisabled,
				},
				ResourcesPolicy: &appsv1beta1.ResourcesPolicy{
					TargetContainersMode:      appsv1beta1.TargetContainersModeSum,
					TargetContainersNameRegex: "^.*",
					ResourcesExpr: appsv1beta1.ResourcesExpr{
						Limits: &appsv1beta1.ResourceExprLimits{
							Memory: "bad_memory_expr",
						},
					},
				},
			},
			expectErrors:  1,
			errorContains: []string{"invalid expression"},
		},
		{
			name: "invalid memory expression in requests - should fail",
			container: appsv1beta1.SidecarContainer{
				Container: corev1.Container{
					Name:                     "test-sidecar",
					Image:                    "test-image",
					ImagePullPolicy:          corev1.PullIfNotPresent,
					TerminationMessagePolicy: corev1.TerminationMessageReadFile,
				},
				PodInjectPolicy: appsv1beta1.BeforeAppContainerType,
				ShareVolumePolicy: appsv1beta1.ShareVolumePolicy{
					Type: appsv1beta1.ShareVolumePolicyDisabled,
				},
				ResourcesPolicy: &appsv1beta1.ResourcesPolicy{
					TargetContainersMode:      appsv1beta1.TargetContainersModeSum,
					TargetContainersNameRegex: "^.*",
					ResourcesExpr: appsv1beta1.ResourcesExpr{
						Requests: &appsv1beta1.ResourceExprRequests{
							Memory: "bad_memory_expr",
						},
					},
				},
			},
			expectErrors:  1,
			errorContains: []string{"invalid expression"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Directly test validateResourcesPolicy to avoid needing full scheme setup
			var allErrs field.ErrorList
			if tt.container.ResourcesPolicy != nil {
				allErrs = validateResourcesPolicy(tt.container, field.NewPath("spec").Index(0))
			}

			if len(allErrs) != tt.expectErrors {
				t.Errorf("expected %d errors, got %d: %v", tt.expectErrors, len(allErrs), allErrs)
			}

			if len(tt.errorContains) > 0 && len(allErrs) > 0 {
				errorStr := allErrs.ToAggregate().Error()
				for _, expected := range tt.errorContains {
					if !contains(errorStr, expected) {
						t.Errorf("expected error to contain '%s', got: %s", expected, errorStr)
					}
				}
			}
		})
	}
}

func TestValidateResourceExpression(t *testing.T) {
	tests := []struct {
		name        string
		expr        string
		variable    string
		expectError bool
		errorMsg    string
	}{
		{
			name:        "valid simple expression",
			expr:        "cpu*50%",
			variable:    "cpu",
			expectError: false,
		},
		{
			name:        "valid max function",
			expr:        "max(cpu*50%, 50m)",
			variable:    "cpu",
			expectError: false,
		},
		{
			name:        "valid min function",
			expr:        "min(cpu*50%, 100m)",
			variable:    "cpu",
			expectError: false,
		},
		{
			name:        "valid complex expression",
			expr:        "0.5*cpu - 0.3*max(0, cpu-4) + 0.3*max(0, cpu-8)",
			variable:    "cpu",
			expectError: false,
		},
		{
			name:        "valid with spaces",
			expr:        "max( cpu * 0.5 , 50m )",
			variable:    "cpu",
			expectError: false,
		},
		{
			name:        "invalid variable name",
			expr:        "invalidvar * 50%",
			variable:    "cpu",
			expectError: true,
			errorMsg:    "invalid",
		},
		{
			name:        "valid memory expression",
			expr:        "memory*50%",
			variable:    "memory",
			expectError: false,
		},
		{
			name:        "valid nested functions",
			expr:        "max(min(cpu*50%, 100m), 50m)",
			variable:    "cpu",
			expectError: false,
		},
		{
			name:        "empty expression",
			expr:        "",
			variable:    "cpu",
			expectError: false,
		},
		{
			name:        "invalid syntax",
			expr:        "cpu +*/ 50%",
			variable:    "cpu",
			expectError: true,
			errorMsg:    "invalid",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateResourceExpression(tt.expr, tt.variable)

			if tt.expectError {
				if err == nil {
					t.Errorf("expected error but got none")
				} else if tt.errorMsg != "" && !contains(err.Error(), tt.errorMsg) {
					t.Errorf("expected error to contain '%s', got: %s", tt.errorMsg, err.Error())
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
			}
		})
	}
}

// Helper function to check if a string contains a substring
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && (s[0:len(substr)] == substr || contains(s[1:], substr)))
}
