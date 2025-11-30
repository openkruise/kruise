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

package validating

import (
	"strings"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/ptr"

	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	appsv1beta1 "github.com/openkruise/kruise/apis/apps/v1beta1"
)

func TestValidateParallelismV1alpha1(t *testing.T) {
	tests := []struct {
		name        string
		parallelism *intstr.IntOrString
		expectError bool
		errorMsg    string
	}{
		{
			name:        "valid integer parallelism",
			parallelism: &intstr.IntOrString{Type: intstr.Int, IntVal: 2},
			expectError: false,
		},
		{
			name:        "valid zero parallelism",
			parallelism: &intstr.IntOrString{Type: intstr.Int, IntVal: 0},
			expectError: false,
		},
		{
			name:        "invalid negative parallelism",
			parallelism: &intstr.IntOrString{Type: intstr.Int, IntVal: -1},
			expectError: true,
			errorMsg:    "parallelism must be non-negative",
		},
		{
			name:        "invalid percentage parallelism",
			parallelism: &intstr.IntOrString{Type: intstr.String, StrVal: "50%"},
			expectError: true,
			errorMsg:    "parallelism does not support percentage value",
		},
		{
			name:        "valid string integer parallelism",
			parallelism: &intstr.IntOrString{Type: intstr.String, StrVal: "50"},
			expectError: false,
		},
		{
			name:        "valid string zero parallelism",
			parallelism: &intstr.IntOrString{Type: intstr.String, StrVal: "0"},
			expectError: false,
		},
		{
			name:        "invalid string negative parallelism",
			parallelism: &intstr.IntOrString{Type: intstr.String, StrVal: "-1"},
			expectError: true,
			errorMsg:    "parallelism must be non-negative",
		},
		{
			name:        "invalid string non-numeric parallelism",
			parallelism: &intstr.IntOrString{Type: intstr.String, StrVal: "abc"},
			expectError: true,
			errorMsg:    "parallelism must be a valid integer",
		},
		{
			name:        "nil parallelism",
			parallelism: nil,
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			obj := &appsv1alpha1.ImagePullJob{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-job",
					Namespace: "default",
				},
				Spec: appsv1alpha1.ImagePullJobSpec{
					Image: "nginx:latest",
					ImagePullJobTemplate: appsv1alpha1.ImagePullJobTemplate{
						Parallelism: tt.parallelism,
						CompletionPolicy: appsv1alpha1.CompletionPolicy{
							Type: appsv1alpha1.Always,
						},
						PullPolicy: &appsv1alpha1.PullPolicy{
							TimeoutSeconds: ptr.To[int32](600),
						},
					},
				},
			}

			err := validate(obj)

			hasError := err != nil
			if hasError != tt.expectError {
				t.Errorf("expected error: %v, got error: %v, error: %v", tt.expectError, hasError, err)
				return
			}

			if tt.expectError && err != nil {
				if !strings.Contains(err.Error(), tt.errorMsg) {
					t.Errorf("expected error message containing '%s', got: %v", tt.errorMsg, err)
				}
			}
		})
	}
}

func TestValidateParallelismV1beta1(t *testing.T) {
	tests := []struct {
		name        string
		parallelism *intstr.IntOrString
		expectError bool
		errorMsg    string
	}{
		{
			name:        "valid integer parallelism",
			parallelism: &intstr.IntOrString{Type: intstr.Int, IntVal: 2},
			expectError: false,
		},
		{
			name:        "valid zero parallelism",
			parallelism: &intstr.IntOrString{Type: intstr.Int, IntVal: 0},
			expectError: false,
		},
		{
			name:        "invalid negative parallelism",
			parallelism: &intstr.IntOrString{Type: intstr.Int, IntVal: -1},
			expectError: true,
			errorMsg:    "parallelism must be non-negative",
		},
		{
			name:        "invalid percentage parallelism",
			parallelism: &intstr.IntOrString{Type: intstr.String, StrVal: "50%"},
			expectError: true,
			errorMsg:    "parallelism does not support percentage value",
		},
		{
			name:        "valid string integer parallelism",
			parallelism: &intstr.IntOrString{Type: intstr.String, StrVal: "50"},
			expectError: false,
		},
		{
			name:        "valid string zero parallelism",
			parallelism: &intstr.IntOrString{Type: intstr.String, StrVal: "0"},
			expectError: false,
		},
		{
			name:        "invalid string negative parallelism",
			parallelism: &intstr.IntOrString{Type: intstr.String, StrVal: "-1"},
			expectError: true,
			errorMsg:    "parallelism must be non-negative",
		},
		{
			name:        "invalid string non-numeric parallelism",
			parallelism: &intstr.IntOrString{Type: intstr.String, StrVal: "abc"},
			expectError: true,
			errorMsg:    "parallelism must be a valid integer",
		},
		{
			name:        "nil parallelism",
			parallelism: nil,
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			obj := &appsv1beta1.ImagePullJob{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-job",
					Namespace: "default",
				},
				Spec: appsv1beta1.ImagePullJobSpec{
					Image: "nginx:latest",
					ImagePullJobTemplate: appsv1beta1.ImagePullJobTemplate{
						Parallelism: tt.parallelism,
						CompletionPolicy: appsv1beta1.CompletionPolicy{
							Type: appsv1beta1.Always,
						},
						PullPolicy: &appsv1beta1.PullPolicy{
							TimeoutSeconds: ptr.To[int32](600),
						},
					},
				},
			}

			err := validateV1beta1(obj)

			hasError := err != nil
			if hasError != tt.expectError {
				t.Errorf("expected error: %v, got error: %v, error: %v", tt.expectError, hasError, err)
				return
			}

			if tt.expectError && err != nil {
				if !strings.Contains(err.Error(), tt.errorMsg) {
					t.Errorf("expected error message containing '%s', got: %v", tt.errorMsg, err)
				}
			}
		})
	}
}
