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
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	admissionv1 "k8s.io/api/admission/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	"github.com/openkruise/kruise/apis"
	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	appsv1beta1 "github.com/openkruise/kruise/apis/apps/v1beta1"
)

var valInt32 int32 = 1
var valInt64 int64 = 2

func TestValidateBroadcastJobSpec(t *testing.T) {
	bjSpec1 := &appsv1beta1.BroadcastJobSpec{
		Parallelism: nil,
		Template: v1.PodTemplateSpec{
			ObjectMeta: metav1.ObjectMeta{
				Labels: map[string]string{
					"broadcastjob-name":           "bjtest",
					"broadcastjob-controller-uid": "1234",
				},
			},
			Spec: v1.PodSpec{
				RestartPolicy: v1.RestartPolicyAlways,
			},
		},
		CompletionPolicy: appsv1beta1.CompletionPolicy{
			Type:                    appsv1beta1.Never,
			TTLSecondsAfterFinished: &valInt32,
			ActiveDeadlineSeconds:   &valInt64,
		},
		Paused:        false,
		FailurePolicy: appsv1beta1.FailurePolicy{},
	}
	fieldErrorList := validateBroadcastJobSpec(bjSpec1, field.NewPath("spec"))
	assert.NotNil(t, fieldErrorList, nil)
	assert.Equal(t, fieldErrorList[0].Field, "spec.completionPolicy.ttlSecondsAfterFinished")
	assert.Equal(t, fieldErrorList[1].Field, "spec.completionPolicy.activeDeadlineSeconds")
	assert.Equal(t, fieldErrorList[2].Field, "spec.template.spec.restartPolicy")
	assert.Equal(t, fieldErrorList[3].Field, "spec.template.metadata.labels")
}

func TestBroadcastJobCreateUpdateHandler_Handle(t *testing.T) {
	utilruntime.Must(apis.AddToScheme(scheme.Scheme))

	tests := []struct {
		name           string
		request        admission.Request
		expectedResult bool
		expectedError  bool
	}{
		{
			name: "create valid v1beta1 BroadcastJob",
			request: admission.Request{
				AdmissionRequest: admissionv1.AdmissionRequest{
					Operation: admissionv1.Create,
					Resource: metav1.GroupVersionResource{
						Group:    appsv1beta1.GroupVersion.Group,
						Version:  appsv1beta1.GroupVersion.Version,
						Resource: "broadcastjobs",
					},
					Object: runtime.RawExtension{
						Raw: createBroadcastJobV1Beta1JSON(t, &appsv1beta1.BroadcastJob{
							ObjectMeta: metav1.ObjectMeta{
								Name:      "test-bj",
								Namespace: "default",
							},
							Spec: appsv1beta1.BroadcastJobSpec{
								Template: v1.PodTemplateSpec{
									Spec: v1.PodSpec{
										Containers: []v1.Container{
											{
												Name:  "test",
												Image: "nginx:latest",
											},
										},
										RestartPolicy: v1.RestartPolicyOnFailure,
									},
								},
							},
						}),
					},
				},
			},
			expectedResult: true,
			expectedError:  false,
		},
		{
			name: "create invalid v1beta1 BroadcastJob with Always restart policy",
			request: admission.Request{
				AdmissionRequest: admissionv1.AdmissionRequest{
					Operation: admissionv1.Create,
					Resource: metav1.GroupVersionResource{
						Group:    appsv1beta1.GroupVersion.Group,
						Version:  appsv1beta1.GroupVersion.Version,
						Resource: "broadcastjobs",
					},
					Object: runtime.RawExtension{
						Raw: createBroadcastJobV1Beta1JSON(t, &appsv1beta1.BroadcastJob{
							ObjectMeta: metav1.ObjectMeta{
								Name:      "test-bj-invalid",
								Namespace: "default",
							},
							Spec: appsv1beta1.BroadcastJobSpec{
								Template: v1.PodTemplateSpec{
									Spec: v1.PodSpec{
										Containers: []v1.Container{
											{
												Name:  "test",
												Image: "nginx:latest",
											},
										},
										RestartPolicy: v1.RestartPolicyAlways, // Invalid for BroadcastJob
									},
								},
							},
						}),
					},
				},
			},
			expectedResult: false,
			expectedError:  true,
		},
		{
			name: "create invalid v1beta1 BroadcastJob with forbidden labels",
			request: admission.Request{
				AdmissionRequest: admissionv1.AdmissionRequest{
					Operation: admissionv1.Create,
					Resource: metav1.GroupVersionResource{
						Group:    appsv1beta1.GroupVersion.Group,
						Version:  appsv1beta1.GroupVersion.Version,
						Resource: "broadcastjobs",
					},
					Object: runtime.RawExtension{
						Raw: createBroadcastJobV1Beta1JSON(t, &appsv1beta1.BroadcastJob{
							ObjectMeta: metav1.ObjectMeta{
								Name:      "test-bj-forbidden-labels",
								Namespace: "default",
							},
							Spec: appsv1beta1.BroadcastJobSpec{
								Template: v1.PodTemplateSpec{
									ObjectMeta: metav1.ObjectMeta{
										Labels: map[string]string{
											"broadcastjob-name": "forbidden", // This should be forbidden
										},
									},
									Spec: v1.PodSpec{
										Containers: []v1.Container{
											{
												Name:  "test",
												Image: "nginx:latest",
											},
										},
										RestartPolicy: v1.RestartPolicyOnFailure,
									},
								},
							},
						}),
					},
				},
			},
			expectedResult: false,
			expectedError:  true,
		},
		{
			name: "create invalid v1beta1 BroadcastJob with Never completion policy and TTL",
			request: admission.Request{
				AdmissionRequest: admissionv1.AdmissionRequest{
					Operation: admissionv1.Create,
					Resource: metav1.GroupVersionResource{
						Group:    appsv1beta1.GroupVersion.Group,
						Version:  appsv1beta1.GroupVersion.Version,
						Resource: "broadcastjobs",
					},
					Object: runtime.RawExtension{
						Raw: createBroadcastJobV1Beta1JSON(t, &appsv1beta1.BroadcastJob{
							ObjectMeta: metav1.ObjectMeta{
								Name:      "test-bj-invalid-completion",
								Namespace: "default",
							},
							Spec: appsv1beta1.BroadcastJobSpec{
								CompletionPolicy: appsv1beta1.CompletionPolicy{
									Type:                    appsv1beta1.Never,
									TTLSecondsAfterFinished: &[]int32{300}[0], // Invalid with Never
								},
								Template: v1.PodTemplateSpec{
									Spec: v1.PodSpec{
										Containers: []v1.Container{
											{
												Name:  "test",
												Image: "nginx:latest",
											},
										},
										RestartPolicy: v1.RestartPolicyOnFailure,
									},
								},
							},
						}),
					},
				},
			},
			expectedResult: false,
			expectedError:  true,
		},
		{
			name: "create v1alpha1 BroadcastJob",
			request: admission.Request{
				AdmissionRequest: admissionv1.AdmissionRequest{
					Operation: admissionv1.Create,
					Resource: metav1.GroupVersionResource{
						Group:    appsv1alpha1.GroupVersion.Group,
						Version:  appsv1alpha1.GroupVersion.Version,
						Resource: "broadcastjobs",
					},
					Object: runtime.RawExtension{
						Raw: createBroadcastJobV1Alpha1JSON(t, &appsv1alpha1.BroadcastJob{
							ObjectMeta: metav1.ObjectMeta{
								Name:      "test-bj-v1alpha1",
								Namespace: "default",
							},
							Spec: appsv1alpha1.BroadcastJobSpec{
								Template: v1.PodTemplateSpec{
									Spec: v1.PodSpec{
										Containers: []v1.Container{
											{
												Name:  "test",
												Image: "nginx:latest",
											},
										},
										RestartPolicy: v1.RestartPolicyOnFailure,
									},
								},
							},
						}),
					},
				},
			},
			expectedResult: true,
			expectedError:  false,
		},
		{
			name: "invalid JSON should return error",
			request: admission.Request{
				AdmissionRequest: admissionv1.AdmissionRequest{
					Operation: admissionv1.Create,
					Resource: metav1.GroupVersionResource{
						Group:    appsv1beta1.GroupVersion.Group,
						Version:  appsv1beta1.GroupVersion.Version,
						Resource: "broadcastjobs",
					},
					Object: runtime.RawExtension{
						Raw: []byte("invalid json"),
					},
				},
			},
			expectedResult: false,
			expectedError:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			decoder := admission.NewDecoder(scheme.Scheme)
			handler := BroadcastJobCreateUpdateHandler{
				Decoder: decoder,
			}

			response := handler.Handle(context.TODO(), tt.request)

			if tt.expectedError {
				if response.Allowed {
					t.Errorf("expected error but got allowed response")
				}
			} else {
				if !response.Allowed && !tt.expectedError {
					t.Errorf("expected allowed response but got error: %s", response.Result.Message)
				}
			}
		})
	}
}

func createBroadcastJobV1Beta1JSON(t *testing.T, bj *appsv1beta1.BroadcastJob) []byte {
	data, err := json.Marshal(bj)
	if err != nil {
		t.Fatalf("failed to marshal BroadcastJob v1beta1: %v", err)
	}
	return data
}

func createBroadcastJobV1Alpha1JSON(t *testing.T, bj *appsv1alpha1.BroadcastJob) []byte {
	data, err := json.Marshal(bj)
	if err != nil {
		t.Fatalf("failed to marshal BroadcastJob v1alpha1: %v", err)
	}
	return data
}

func TestValidatePodFailurePolicy(t *testing.T) {
	containerName := "main"

	tests := []struct {
		name          string
		policy        *appsv1beta1.PodFailurePolicy
		expectErrors  bool
		expectedCount int
	}{
		{
			name:          "nil policy is valid",
			policy:        nil,
			expectErrors:  false,
			expectedCount: 0,
		},
		{
			name: "valid policy with exit codes",
			policy: &appsv1beta1.PodFailurePolicy{
				Rules: []appsv1beta1.PodFailurePolicyRule{
					{
						Action: appsv1beta1.PodFailurePolicyActionFailJob,
						OnExitCodes: &appsv1beta1.PodFailurePolicyOnExitCodesRequirement{
							ContainerName: &containerName,
							Operator:      appsv1beta1.PodFailurePolicyOnExitCodesOpIn,
							Values:        []int32{42},
						},
					},
				},
			},
			expectErrors:  false,
			expectedCount: 0,
		},
		{
			name: "valid policy with pod conditions",
			policy: &appsv1beta1.PodFailurePolicy{
				Rules: []appsv1beta1.PodFailurePolicyRule{
					{
						Action: appsv1beta1.PodFailurePolicyActionIgnore,
						OnPodConditions: []appsv1beta1.PodFailurePolicyOnPodConditionsPattern{
							{
								Type:   v1.DisruptionTarget,
								Status: v1.ConditionTrue,
							},
						},
					},
				},
			},
			expectErrors:  false,
			expectedCount: 0,
		},
		{
			name: "empty rules",
			policy: &appsv1beta1.PodFailurePolicy{
				Rules: []appsv1beta1.PodFailurePolicyRule{},
			},
			expectErrors:  true,
			expectedCount: 1,
		},
		{
			name: "exit code 0 with In operator",
			policy: &appsv1beta1.PodFailurePolicy{
				Rules: []appsv1beta1.PodFailurePolicyRule{
					{
						Action: appsv1beta1.PodFailurePolicyActionFailJob,
						OnExitCodes: &appsv1beta1.PodFailurePolicyOnExitCodesRequirement{
							Operator: appsv1beta1.PodFailurePolicyOnExitCodesOpIn,
							Values:   []int32{0, 1}, // 0 is invalid with In
						},
					},
				},
			},
			expectErrors:  true,
			expectedCount: 1,
		},
		{
			name: "both onExitCodes and onPodConditions",
			policy: &appsv1beta1.PodFailurePolicy{
				Rules: []appsv1beta1.PodFailurePolicyRule{
					{
						Action: appsv1beta1.PodFailurePolicyActionFailJob,
						OnExitCodes: &appsv1beta1.PodFailurePolicyOnExitCodesRequirement{
							Operator: appsv1beta1.PodFailurePolicyOnExitCodesOpIn,
							Values:   []int32{42},
						},
						OnPodConditions: []appsv1beta1.PodFailurePolicyOnPodConditionsPattern{
							{
								Type:   v1.DisruptionTarget,
								Status: v1.ConditionTrue,
							},
						},
					},
				},
			},
			expectErrors:  true,
			expectedCount: 1,
		},
		{
			name: "empty values in exit codes",
			policy: &appsv1beta1.PodFailurePolicy{
				Rules: []appsv1beta1.PodFailurePolicyRule{
					{
						Action: appsv1beta1.PodFailurePolicyActionFailJob,
						OnExitCodes: &appsv1beta1.PodFailurePolicyOnExitCodesRequirement{
							Operator: appsv1beta1.PodFailurePolicyOnExitCodesOpIn,
							Values:   []int32{},
						},
					},
				},
			},
			expectErrors:  true,
			expectedCount: 1,
		},
		{
			name: "duplicate values in exit codes",
			policy: &appsv1beta1.PodFailurePolicy{
				Rules: []appsv1beta1.PodFailurePolicyRule{
					{
						Action: appsv1beta1.PodFailurePolicyActionFailJob,
						OnExitCodes: &appsv1beta1.PodFailurePolicyOnExitCodesRequirement{
							Operator: appsv1beta1.PodFailurePolicyOnExitCodesOpIn,
							Values:   []int32{42, 42}, // duplicate
						},
					},
				},
			},
			expectErrors:  true,
			expectedCount: 1,
		},
		{
			name: "invalid action",
			policy: &appsv1beta1.PodFailurePolicy{
				Rules: []appsv1beta1.PodFailurePolicyRule{
					{
						Action: appsv1beta1.PodFailurePolicyAction("Invalid"),
						OnExitCodes: &appsv1beta1.PodFailurePolicyOnExitCodesRequirement{
							Operator: appsv1beta1.PodFailurePolicyOnExitCodesOpIn,
							Values:   []int32{42},
						},
					},
				},
			},
			expectErrors:  true,
			expectedCount: 1,
		},
		{
			name: "invalid operator",
			policy: &appsv1beta1.PodFailurePolicy{
				Rules: []appsv1beta1.PodFailurePolicyRule{
					{
						Action: appsv1beta1.PodFailurePolicyActionFailJob,
						OnExitCodes: &appsv1beta1.PodFailurePolicyOnExitCodesRequirement{
							Operator: appsv1beta1.PodFailurePolicyOnExitCodesOperator("Invalid"),
							Values:   []int32{42},
						},
					},
				},
			},
			expectErrors:  true,
			expectedCount: 1,
		},
		{
			name: "neither onExitCodes nor onPodConditions",
			policy: &appsv1beta1.PodFailurePolicy{
				Rules: []appsv1beta1.PodFailurePolicyRule{
					{
						Action: appsv1beta1.PodFailurePolicyActionFailJob,
					},
				},
			},
			expectErrors:  true,
			expectedCount: 1,
		},
		{
			name: "empty condition type",
			policy: &appsv1beta1.PodFailurePolicy{
				Rules: []appsv1beta1.PodFailurePolicyRule{
					{
						Action: appsv1beta1.PodFailurePolicyActionIgnore,
						OnPodConditions: []appsv1beta1.PodFailurePolicyOnPodConditionsPattern{
							{
								Type:   "",
								Status: v1.ConditionTrue,
							},
						},
					},
				},
			},
			expectErrors:  true,
			expectedCount: 1,
		},
		{
			name: "invalid condition status",
			policy: &appsv1beta1.PodFailurePolicy{
				Rules: []appsv1beta1.PodFailurePolicyRule{
					{
						Action: appsv1beta1.PodFailurePolicyActionIgnore,
						OnPodConditions: []appsv1beta1.PodFailurePolicyOnPodConditionsPattern{
							{
								Type:   v1.DisruptionTarget,
								Status: v1.ConditionStatus("Invalid"),
							},
						},
					},
				},
			},
			expectErrors:  true,
			expectedCount: 1,
		},
		{
			name: "valid policy with Count action",
			policy: &appsv1beta1.PodFailurePolicy{
				Rules: []appsv1beta1.PodFailurePolicyRule{
					{
						Action: appsv1beta1.PodFailurePolicyActionCount,
						OnExitCodes: &appsv1beta1.PodFailurePolicyOnExitCodesRequirement{
							ContainerName: &containerName,
							Operator:      appsv1beta1.PodFailurePolicyOnExitCodesOpNotIn,
							Values:        []int32{0},
						},
					},
				},
			},
			expectErrors:  false,
			expectedCount: 0,
		},
		{
			name: "too many rules",
			policy: &appsv1beta1.PodFailurePolicy{
				Rules: func() []appsv1beta1.PodFailurePolicyRule {
					rules := make([]appsv1beta1.PodFailurePolicyRule, 21)
					for i := range rules {
						rules[i] = appsv1beta1.PodFailurePolicyRule{
							Action: appsv1beta1.PodFailurePolicyActionFailJob,
							OnExitCodes: &appsv1beta1.PodFailurePolicyOnExitCodesRequirement{
								Operator: appsv1beta1.PodFailurePolicyOnExitCodesOpIn,
								Values:   []int32{int32(i + 1)},
							},
						}
					}
					return rules
				}(),
			},
			expectErrors:  true,
			expectedCount: 1,
		},
		{
			name: "too many onPodConditions",
			policy: &appsv1beta1.PodFailurePolicy{
				Rules: []appsv1beta1.PodFailurePolicyRule{
					{
						Action: appsv1beta1.PodFailurePolicyActionIgnore,
						OnPodConditions: func() []appsv1beta1.PodFailurePolicyOnPodConditionsPattern {
							conditions := make([]appsv1beta1.PodFailurePolicyOnPodConditionsPattern, 21)
							for i := range conditions {
								conditions[i] = appsv1beta1.PodFailurePolicyOnPodConditionsPattern{
									Type:   v1.PodConditionType(fmt.Sprintf("Condition%d", i)),
									Status: v1.ConditionTrue,
								}
							}
							return conditions
						}(),
					},
				},
			},
			expectErrors:  true,
			expectedCount: 1,
		},
		{
			name: "too many exit code values",
			policy: &appsv1beta1.PodFailurePolicy{
				Rules: []appsv1beta1.PodFailurePolicyRule{
					{
						Action: appsv1beta1.PodFailurePolicyActionFailJob,
						OnExitCodes: &appsv1beta1.PodFailurePolicyOnExitCodesRequirement{
							Operator: appsv1beta1.PodFailurePolicyOnExitCodesOpIn,
							Values: func() []int32 {
								values := make([]int32, 256)
								for i := range values {
									values[i] = int32(i + 1)
								}
								return values
							}(),
						},
					},
				},
			},
			expectErrors:  true,
			expectedCount: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.policy == nil {
				// nil policy means no validation needed, it's optional
				return
			}
			errs := validatePodFailurePolicy(tt.policy, field.NewPath("spec").Child("podFailurePolicy"))
			if tt.expectErrors && len(errs) == 0 {
				t.Errorf("expected errors but got none")
			}
			if !tt.expectErrors && len(errs) > 0 {
				t.Errorf("expected no errors but got: %v", errs)
			}
			if tt.expectErrors && len(errs) != tt.expectedCount {
				t.Errorf("expected %d errors but got %d: %v", tt.expectedCount, len(errs), errs)
			}
		})
	}
}

func TestValidatePodReplacementPolicy(t *testing.T) {
	terminatingOrFailed := appsv1beta1.TerminatingOrFailed
	failed := appsv1beta1.Failed
	invalid := appsv1beta1.PodReplacementPolicy("Invalid")

	tests := []struct {
		name         string
		policy       *appsv1beta1.PodReplacementPolicy
		expectErrors bool
	}{
		{
			name:         "nil policy is valid",
			policy:       nil,
			expectErrors: false,
		},
		{
			name:         "TerminatingOrFailed is valid",
			policy:       &terminatingOrFailed,
			expectErrors: false,
		},
		{
			name:         "Failed is valid",
			policy:       &failed,
			expectErrors: false,
		},
		{
			name:         "invalid policy value",
			policy:       &invalid,
			expectErrors: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.policy == nil {
				// nil policy means no validation needed, it's optional
				return
			}
			errs := validatePodReplacementPolicy(tt.policy, field.NewPath("spec").Child("podReplacementPolicy"))
			if tt.expectErrors && len(errs) == 0 {
				t.Errorf("expected errors but got none")
			}
			if !tt.expectErrors && len(errs) > 0 {
				t.Errorf("expected no errors but got: %v", errs)
			}
		})
	}
}

func TestBroadcastJobWithPodReplacementPolicy(t *testing.T) {
	utilruntime.Must(apis.AddToScheme(scheme.Scheme))

	terminatingOrFailed := appsv1beta1.TerminatingOrFailed
	failed := appsv1beta1.Failed

	tests := []struct {
		name           string
		policy         *appsv1beta1.PodReplacementPolicy
		expectedResult bool
	}{
		{
			name:           "BroadcastJob with TerminatingOrFailed policy",
			policy:         &terminatingOrFailed,
			expectedResult: true,
		},
		{
			name:           "BroadcastJob with Failed policy",
			policy:         &failed,
			expectedResult: true,
		},
		{
			name:           "BroadcastJob without policy",
			policy:         nil,
			expectedResult: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bj := &appsv1beta1.BroadcastJob{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-bj-prp",
					Namespace: "default",
				},
				Spec: appsv1beta1.BroadcastJobSpec{
					Template: v1.PodTemplateSpec{
						Spec: v1.PodSpec{
							Containers: []v1.Container{
								{
									Name:  "test",
									Image: "nginx:latest",
								},
							},
							RestartPolicy: v1.RestartPolicyOnFailure,
						},
					},
					PodReplacementPolicy: tt.policy,
				},
			}

			data, err := json.Marshal(bj)
			if err != nil {
				t.Fatalf("failed to marshal BroadcastJob: %v", err)
			}

			decoder := admission.NewDecoder(scheme.Scheme)
			handler := BroadcastJobCreateUpdateHandler{
				Decoder: decoder,
			}

			request := admission.Request{
				AdmissionRequest: admissionv1.AdmissionRequest{
					Operation: admissionv1.Create,
					Resource: metav1.GroupVersionResource{
						Group:    appsv1beta1.GroupVersion.Group,
						Version:  appsv1beta1.GroupVersion.Version,
						Resource: "broadcastjobs",
					},
					Object: runtime.RawExtension{
						Raw: data,
					},
				},
			}

			response := handler.Handle(context.TODO(), request)

			if tt.expectedResult && !response.Allowed {
				t.Errorf("expected allowed but got rejected: %s", response.Result.Message)
			}
			if !tt.expectedResult && response.Allowed {
				t.Errorf("expected rejected but got allowed")
			}
		})
	}
}
