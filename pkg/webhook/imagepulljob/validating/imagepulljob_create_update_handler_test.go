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
	"context"
	"encoding/json"
	"strings"
	"testing"

	admissionv1 "k8s.io/api/admission/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	"github.com/openkruise/kruise/apis"
	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	appsv1beta1 "github.com/openkruise/kruise/apis/apps/v1beta1"
	"github.com/openkruise/kruise/pkg/features"
	utilfeature "github.com/openkruise/kruise/pkg/util/feature"
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

func TestPullSecretsImmutability(t *testing.T) {
	utilruntime.Must(apis.AddToScheme(scheme.Scheme))
	defer utilfeature.SetFeatureGateDuringTest(t, utilfeature.DefaultMutableFeatureGate, features.KruiseDaemon, true)()
	defer utilfeature.SetFeatureGateDuringTest(t, utilfeature.DefaultMutableFeatureGate, features.ImagePullJobGate, true)()

	baseJob := func(pullSecrets []string) *appsv1beta1.ImagePullJob {
		return &appsv1beta1.ImagePullJob{
			ObjectMeta: metav1.ObjectMeta{Name: "test-job", Namespace: "default"},
			Spec: appsv1beta1.ImagePullJobSpec{
				Image: "nginx:latest",
				ImagePullJobTemplate: appsv1beta1.ImagePullJobTemplate{
					PullSecrets:      pullSecrets,
					CompletionPolicy: appsv1beta1.CompletionPolicy{Type: appsv1beta1.Always},
					PullPolicy:       &appsv1beta1.PullPolicy{TimeoutSeconds: ptr.To[int32](600)},
				},
			},
		}
	}
	marshal := func(t *testing.T, obj interface{}) []byte {
		t.Helper()
		data, err := json.Marshal(obj)
		if err != nil {
			t.Fatal(err)
		}
		return data
	}

	tests := []struct {
		name      string
		newObj    *appsv1beta1.ImagePullJob
		oldObj    *appsv1beta1.ImagePullJob
		operation admissionv1.Operation
		allowed   bool
	}{
		{
			name:      "create with pullSecrets is allowed",
			newObj:    baseJob([]string{"secret1"}),
			operation: admissionv1.Create,
			allowed:   true,
		},
		{
			name:      "update with unchanged pullSecrets is allowed",
			newObj:    baseJob([]string{"secret1"}),
			oldObj:    baseJob([]string{"secret1"}),
			operation: admissionv1.Update,
			allowed:   true,
		},
		{
			name:      "update with changed pullSecrets is denied",
			newObj:    baseJob([]string{"secret2"}),
			oldObj:    baseJob([]string{"secret1"}),
			operation: admissionv1.Update,
			allowed:   false,
		},
		{
			name:      "update adding pullSecrets is denied",
			newObj:    baseJob([]string{"secret1"}),
			oldObj:    baseJob(nil),
			operation: admissionv1.Update,
			allowed:   false,
		},
		{
			name:      "update removing pullSecrets is denied",
			newObj:    baseJob(nil),
			oldObj:    baseJob([]string{"secret1"}),
			operation: admissionv1.Update,
			allowed:   false,
		},
		{
			name:      "update with both nil pullSecrets is allowed",
			newObj:    baseJob(nil),
			oldObj:    baseJob(nil),
			operation: admissionv1.Update,
			allowed:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			decoder := admission.NewDecoder(scheme.Scheme)
			handler := &ImagePullJobCreateUpdateHandler{Decoder: decoder}

			req := admission.Request{
				AdmissionRequest: admissionv1.AdmissionRequest{
					Operation: tt.operation,
					Resource: metav1.GroupVersionResource{
						Group:    appsv1beta1.GroupVersion.Group,
						Version:  appsv1beta1.GroupVersion.Version,
						Resource: "imagepulljobs",
					},
					Object: runtime.RawExtension{Raw: marshal(t, tt.newObj)},
				},
			}
			if tt.oldObj != nil {
				req.AdmissionRequest.OldObject = runtime.RawExtension{Raw: marshal(t, tt.oldObj)}
			}

			resp := handler.Handle(context.Background(), req)
			if resp.Allowed != tt.allowed {
				t.Errorf("expected allowed=%v, got allowed=%v, reason=%s", tt.allowed, resp.Allowed, resp.Result.Message)
			}
			if !tt.allowed && resp.Result != nil && !strings.Contains(resp.Result.Message, "pullSecrets") {
				t.Errorf("expected error about pullSecrets, got: %s", resp.Result.Message)
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
