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
