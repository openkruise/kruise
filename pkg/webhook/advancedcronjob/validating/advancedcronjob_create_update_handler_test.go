/*
Copyright 2022 The Kruise Authors.

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

	admissionv1 "k8s.io/api/admission/v1"
	batchv1 "k8s.io/api/batch/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	"github.com/openkruise/kruise/apis"
	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	appsv1beta1 "github.com/openkruise/kruise/apis/apps/v1beta1"
)

func TestValidateCronJobSpec(t *testing.T) {
	validPodTemplateSpec := v1.PodTemplateSpec{
		Spec: v1.PodSpec{
			Containers: []v1.Container{
				{Name: "foo", Image: "foo:latest", TerminationMessagePolicy: v1.TerminationMessageReadFile, ImagePullPolicy: v1.PullIfNotPresent},
			},
			RestartPolicy:                 v1.RestartPolicyAlways,
			DNSPolicy:                     v1.DNSDefault,
			TerminationGracePeriodSeconds: &[]int64{v1.DefaultTerminationGracePeriodSeconds}[0],
		},
	}

	type testCase struct {
		acj       *appsv1beta1.AdvancedCronJobSpec
		expectErr bool
	}

	cases := map[string]testCase{
		"no validation because timeZone is nil": {
			acj: &appsv1beta1.AdvancedCronJobSpec{
				Schedule:          "0 * * * *",
				TimeZone:          nil,
				ConcurrencyPolicy: appsv1beta1.AllowConcurrent,
				Template: appsv1beta1.CronJobTemplate{
					JobTemplate: &batchv1.JobTemplateSpec{
						Spec: batchv1.JobSpec{
							Template: validPodTemplateSpec,
						},
					},
				},
			},
		},
		"check timeZone is valid": {
			acj: &appsv1beta1.AdvancedCronJobSpec{
				Schedule:          "0 * * * *",
				TimeZone:          pointer.String("America/New_York"),
				ConcurrencyPolicy: appsv1beta1.AllowConcurrent,
				Template: appsv1beta1.CronJobTemplate{
					JobTemplate: &batchv1.JobTemplateSpec{
						Spec: batchv1.JobSpec{
							Template: validPodTemplateSpec,
						},
					},
				},
			},
		},
		"check timeZone is invalid": {
			acj: &appsv1beta1.AdvancedCronJobSpec{
				Schedule:          "0 * * * *",
				TimeZone:          pointer.String("broken"),
				ConcurrencyPolicy: appsv1beta1.AllowConcurrent,
				Template: appsv1beta1.CronJobTemplate{
					JobTemplate: &batchv1.JobTemplateSpec{
						Spec: batchv1.JobSpec{
							Template: validPodTemplateSpec,
						},
					},
				},
			},
			expectErr: true,
		},
	}

	for k, v := range cases {
		errs := validateAdvancedCronJobSpec(v.acj, field.NewPath("spec"))
		if len(errs) > 0 && !v.expectErr {
			t.Errorf("unexpected error for %s: %v", k, errs)
		} else if len(errs) == 0 && v.expectErr {
			t.Errorf("expected error for %s but got nil", k)
		}
	}
}

func TestAdvancedCronJobCreateUpdateHandler_Handle(t *testing.T) {
	utilruntime.Must(apis.AddToScheme(scheme.Scheme))

	tests := []struct {
		name           string
		request        admission.Request
		expectedResult bool
		expectedError  bool
	}{
		{
			name: "create valid v1beta1 AdvancedCronJob",
			request: admission.Request{
				AdmissionRequest: admissionv1.AdmissionRequest{
					Operation: admissionv1.Create,
					Resource: metav1.GroupVersionResource{
						Group:    appsv1beta1.GroupVersion.Group,
						Version:  appsv1beta1.GroupVersion.Version,
						Resource: "advancedcronjobs",
					},
					Object: runtime.RawExtension{
						Raw: createAdvancedCronJobV1Beta1JSON(t, &appsv1beta1.AdvancedCronJob{
							ObjectMeta: metav1.ObjectMeta{
								Name:      "test-acj",
								Namespace: "default",
							},
							Spec: appsv1beta1.AdvancedCronJobSpec{
								Schedule: "0 0 * * *",
								Template: appsv1beta1.CronJobTemplate{
									JobTemplate: &batchv1.JobTemplateSpec{
										Spec: batchv1.JobSpec{
											Template: createValidPodTemplateSpec(),
										},
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
			name: "create invalid v1beta1 AdvancedCronJob with empty schedule",
			request: admission.Request{
				AdmissionRequest: admissionv1.AdmissionRequest{
					Operation: admissionv1.Create,
					Resource: metav1.GroupVersionResource{
						Group:    appsv1beta1.GroupVersion.Group,
						Version:  appsv1beta1.GroupVersion.Version,
						Resource: "advancedcronjobs",
					},
					Object: runtime.RawExtension{
						Raw: createAdvancedCronJobV1Beta1JSON(t, &appsv1beta1.AdvancedCronJob{
							ObjectMeta: metav1.ObjectMeta{
								Name:      "test-acj-invalid",
								Namespace: "default",
							},
							Spec: appsv1beta1.AdvancedCronJobSpec{
								Schedule: "",
								Template: appsv1beta1.CronJobTemplate{
									JobTemplate: &batchv1.JobTemplateSpec{
										Spec: batchv1.JobSpec{
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
			name: "create invalid v1beta1 AdvancedCronJob with invalid schedule",
			request: admission.Request{
				AdmissionRequest: admissionv1.AdmissionRequest{
					Operation: admissionv1.Create,
					Resource: metav1.GroupVersionResource{
						Group:    appsv1beta1.GroupVersion.Group,
						Version:  appsv1beta1.GroupVersion.Version,
						Resource: "advancedcronjobs",
					},
					Object: runtime.RawExtension{
						Raw: createAdvancedCronJobV1Beta1JSON(t, &appsv1beta1.AdvancedCronJob{
							ObjectMeta: metav1.ObjectMeta{
								Name:      "test-acj-invalid-schedule",
								Namespace: "default",
							},
							Spec: appsv1beta1.AdvancedCronJobSpec{
								Schedule: "invalid-cron",
								Template: appsv1beta1.CronJobTemplate{
									JobTemplate: &batchv1.JobTemplateSpec{
										Spec: batchv1.JobSpec{
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
			name: "create invalid v1beta1 AdvancedCronJob with no template",
			request: admission.Request{
				AdmissionRequest: admissionv1.AdmissionRequest{
					Operation: admissionv1.Create,
					Resource: metav1.GroupVersionResource{
						Group:    appsv1beta1.GroupVersion.Group,
						Version:  appsv1beta1.GroupVersion.Version,
						Resource: "advancedcronjobs",
					},
					Object: runtime.RawExtension{
						Raw: createAdvancedCronJobV1Beta1JSON(t, &appsv1beta1.AdvancedCronJob{
							ObjectMeta: metav1.ObjectMeta{
								Name:      "test-acj-no-template",
								Namespace: "default",
							},
							Spec: appsv1beta1.AdvancedCronJobSpec{
								Schedule: "0 0 * * *",
								Template: appsv1beta1.CronJobTemplate{},
							},
						}),
					},
				},
			},
			expectedResult: false,
			expectedError:  true,
		},
		{
			name: "create invalid v1beta1 AdvancedCronJob with both templates",
			request: admission.Request{
				AdmissionRequest: admissionv1.AdmissionRequest{
					Operation: admissionv1.Create,
					Resource: metav1.GroupVersionResource{
						Group:    appsv1beta1.GroupVersion.Group,
						Version:  appsv1beta1.GroupVersion.Version,
						Resource: "advancedcronjobs",
					},
					Object: runtime.RawExtension{
						Raw: createAdvancedCronJobV1Beta1JSON(t, &appsv1beta1.AdvancedCronJob{
							ObjectMeta: metav1.ObjectMeta{
								Name:      "test-acj-both-templates",
								Namespace: "default",
							},
							Spec: appsv1beta1.AdvancedCronJobSpec{
								Schedule: "0 0 * * *",
								Template: appsv1beta1.CronJobTemplate{
									JobTemplate: &batchv1.JobTemplateSpec{
										Spec: batchv1.JobSpec{
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
									},
									BroadcastJobTemplate: &appsv1beta1.BroadcastJobTemplateSpec{
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
			name: "create v1alpha1 AdvancedCronJob",
			request: admission.Request{
				AdmissionRequest: admissionv1.AdmissionRequest{
					Operation: admissionv1.Create,
					Resource: metav1.GroupVersionResource{
						Group:    appsv1alpha1.GroupVersion.Group,
						Version:  appsv1alpha1.GroupVersion.Version,
						Resource: "advancedcronjobs",
					},
					Object: runtime.RawExtension{
						Raw: createAdvancedCronJobV1Alpha1JSON(t, &appsv1alpha1.AdvancedCronJob{
							ObjectMeta: metav1.ObjectMeta{
								Name:      "test-acj-v1alpha1",
								Namespace: "default",
							},
							Spec: appsv1alpha1.AdvancedCronJobSpec{
								Schedule: "0 0 * * *",
								Template: appsv1alpha1.CronJobTemplate{
									BroadcastJobTemplate: &appsv1alpha1.BroadcastJobTemplateSpec{
										Spec: appsv1alpha1.BroadcastJobSpec{
											Template: createValidPodTemplateSpec(),
										},
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
			name: "update v1beta1 AdvancedCronJob with forbidden field change",
			request: admission.Request{
				AdmissionRequest: admissionv1.AdmissionRequest{
					Operation: admissionv1.Update,
					Resource: metav1.GroupVersionResource{
						Group:    appsv1beta1.GroupVersion.Group,
						Version:  appsv1beta1.GroupVersion.Version,
						Resource: "advancedcronjobs",
					},
					Object: runtime.RawExtension{
						Raw: createAdvancedCronJobV1Beta1JSON(t, &appsv1beta1.AdvancedCronJob{
							ObjectMeta: metav1.ObjectMeta{
								Name:      "test-acj-update",
								Namespace: "default",
							},
							Spec: appsv1beta1.AdvancedCronJobSpec{
								Schedule: "0 0 * * *",
								Template: appsv1beta1.CronJobTemplate{
									JobTemplate: &batchv1.JobTemplateSpec{
										Spec: batchv1.JobSpec{
											Template: createValidPodTemplateSpec(),
										},
									},
								},
							},
						}),
					},
					OldObject: runtime.RawExtension{
						Raw: createAdvancedCronJobV1Beta1JSON(t, &appsv1beta1.AdvancedCronJob{
							ObjectMeta: metav1.ObjectMeta{
								Name:      "test-acj-update",
								Namespace: "default",
							},
							Spec: appsv1beta1.AdvancedCronJobSpec{
								Schedule: "0 0 * * *",
								Template: appsv1beta1.CronJobTemplate{
									JobTemplate: &batchv1.JobTemplateSpec{
										Spec: batchv1.JobSpec{
											Template: createValidPodTemplateSpec(),
										},
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
			name: "create v1beta1 AdvancedCronJob ImageListPullJobTemplate",
			request: admission.Request{
				AdmissionRequest: admissionv1.AdmissionRequest{
					Operation: admissionv1.Create,
					Resource: metav1.GroupVersionResource{
						Group:    appsv1beta1.GroupVersion.Group,
						Version:  appsv1beta1.GroupVersion.Version,
						Resource: "advancedcronjobs",
					},
					Object: runtime.RawExtension{
						Raw: createAdvancedCronJobV1Beta1JSON(t, &appsv1beta1.AdvancedCronJob{
							ObjectMeta: metav1.ObjectMeta{
								Name:      "test-acj-v1beta1",
								Namespace: "default",
							},
							Spec: appsv1beta1.AdvancedCronJobSpec{
								Schedule: "0 0 * * *",
								Template: appsv1beta1.CronJobTemplate{
									ImageListPullJobTemplate: &appsv1beta1.ImageListPullJobTemplateSpec{
										Spec: appsv1beta1.ImageListPullJobSpec{
											Images: []string{
												"busybox:latest",
												"alpine:latest",
											},
											ImagePullJobTemplate: appsv1beta1.ImagePullJobTemplate{
												PullSecrets: nil,
												Selector: &appsv1beta1.ImagePullJobNodeSelector{
													Names: []string{
														"node1",
													},
												},
												PodSelector: nil,
												Parallelism: nil,
												PullPolicy:  nil,
												CompletionPolicy: appsv1beta1.CompletionPolicy{
													Type:                    appsv1beta1.Always,
													ActiveDeadlineSeconds:   int64Ptr(100),
													TTLSecondsAfterFinished: int32Ptr(100),
												},
												SandboxConfig:   nil,
												ImagePullPolicy: "",
											},
										},
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
			name: "create v1beta1 AdvancedCronJob ImageListPullJobTemplate conflict Selector",
			request: admission.Request{
				AdmissionRequest: admissionv1.AdmissionRequest{
					Operation: admissionv1.Create,
					Resource: metav1.GroupVersionResource{
						Group:    appsv1beta1.GroupVersion.Group,
						Version:  appsv1beta1.GroupVersion.Version,
						Resource: "advancedcronjobs",
					},
					Object: runtime.RawExtension{
						Raw: createAdvancedCronJobV1Beta1JSON(t, &appsv1beta1.AdvancedCronJob{
							ObjectMeta: metav1.ObjectMeta{
								Name:      "test-acj-v1beta1",
								Namespace: "default",
							},
							Spec: appsv1beta1.AdvancedCronJobSpec{
								Schedule: "0 0 * * *",
								Template: appsv1beta1.CronJobTemplate{
									ImageListPullJobTemplate: &appsv1beta1.ImageListPullJobTemplateSpec{
										Spec: appsv1beta1.ImageListPullJobSpec{
											Images: []string{
												"busybox:latest",
												"alpine:latest",
											},
											ImagePullJobTemplate: appsv1beta1.ImagePullJobTemplate{
												PullSecrets: nil,
												Selector: &appsv1beta1.ImagePullJobNodeSelector{
													Names: []string{
														"node1",
													},
													LabelSelector: metav1.LabelSelector{
														MatchLabels: map[string]string{
															"key": "value",
														},
													},
												},
												PodSelector: nil,
												Parallelism: nil,
												PullPolicy:  nil,
												CompletionPolicy: appsv1beta1.CompletionPolicy{
													Type:                    appsv1beta1.Always,
													ActiveDeadlineSeconds:   int64Ptr(100),
													TTLSecondsAfterFinished: int32Ptr(100),
												},
												SandboxConfig:   nil,
												ImagePullPolicy: "",
											},
										},
									},
								},
							},
						}),
					},
				},
			},
			expectedResult: true,
			expectedError:  true,
		},
		{
			name: "create v1beta1 AdvancedCronJob ImageListPullJobTemplate conflict Selector 2",
			request: admission.Request{
				AdmissionRequest: admissionv1.AdmissionRequest{
					Operation: admissionv1.Create,
					Resource: metav1.GroupVersionResource{
						Group:    appsv1beta1.GroupVersion.Group,
						Version:  appsv1beta1.GroupVersion.Version,
						Resource: "advancedcronjobs",
					},
					Object: runtime.RawExtension{
						Raw: createAdvancedCronJobV1Beta1JSON(t, &appsv1beta1.AdvancedCronJob{
							ObjectMeta: metav1.ObjectMeta{
								Name:      "test-acj-v1beta1",
								Namespace: "default",
							},
							Spec: appsv1beta1.AdvancedCronJobSpec{
								Schedule: "0 0 * * *",
								Template: appsv1beta1.CronJobTemplate{
									ImageListPullJobTemplate: &appsv1beta1.ImageListPullJobTemplateSpec{
										Spec: appsv1beta1.ImageListPullJobSpec{
											Images: []string{
												"busybox:latest",
												"alpine:latest",
											},
											ImagePullJobTemplate: appsv1beta1.ImagePullJobTemplate{
												PullSecrets: nil,
												Selector: &appsv1beta1.ImagePullJobNodeSelector{
													LabelSelector: metav1.LabelSelector{
														MatchLabels: map[string]string{
															"key": "value",
														},
														MatchExpressions: []metav1.LabelSelectorRequirement{
															{Key: "xxx"},
														},
													},
												},
												PodSelector: nil,
												Parallelism: nil,
												PullPolicy:  nil,
												CompletionPolicy: appsv1beta1.CompletionPolicy{
													Type:                    appsv1beta1.Always,
													ActiveDeadlineSeconds:   int64Ptr(100),
													TTLSecondsAfterFinished: int32Ptr(100),
												},
												SandboxConfig:   nil,
												ImagePullPolicy: "",
											},
										},
									},
								},
							},
						}),
					},
				},
			},
			expectedResult: true,
			expectedError:  true,
		},
		{
			name: "create v1beta1 AdvancedCronJob ImageListPullJobTemplate conflict PodSelector",
			request: admission.Request{
				AdmissionRequest: admissionv1.AdmissionRequest{
					Operation: admissionv1.Create,
					Resource: metav1.GroupVersionResource{
						Group:    appsv1beta1.GroupVersion.Group,
						Version:  appsv1beta1.GroupVersion.Version,
						Resource: "advancedcronjobs",
					},
					Object: runtime.RawExtension{
						Raw: createAdvancedCronJobV1Beta1JSON(t, &appsv1beta1.AdvancedCronJob{
							ObjectMeta: metav1.ObjectMeta{
								Name:      "test-acj-v1beta1",
								Namespace: "default",
							},
							Spec: appsv1beta1.AdvancedCronJobSpec{
								Schedule: "0 0 * * *",
								Template: appsv1beta1.CronJobTemplate{
									ImageListPullJobTemplate: &appsv1beta1.ImageListPullJobTemplateSpec{
										Spec: appsv1beta1.ImageListPullJobSpec{
											Images: []string{
												"busybox:latest",
												"alpine:latest",
											},
											ImagePullJobTemplate: appsv1beta1.ImagePullJobTemplate{
												PullSecrets: nil,
												Selector: &appsv1beta1.ImagePullJobNodeSelector{
													Names: []string{
														"node1",
													},
												},
												PodSelector: &appsv1beta1.ImagePullJobPodSelector{
													LabelSelector: metav1.LabelSelector{
														MatchLabels: map[string]string{
															"key": "value",
														},
													},
												},
												Parallelism: nil,
												PullPolicy:  nil,
												CompletionPolicy: appsv1beta1.CompletionPolicy{
													Type:                    appsv1beta1.Always,
													ActiveDeadlineSeconds:   int64Ptr(100),
													TTLSecondsAfterFinished: int32Ptr(100),
												},
												SandboxConfig:   nil,
												ImagePullPolicy: "",
											},
										},
									},
								},
							},
						}),
					},
				},
			},
			expectedResult: true,
			expectedError:  true,
		},
		{
			name: "create v1beta1 AdvancedCronJob ImageListPullJobTemplate conflict PodSelector 2",
			request: admission.Request{
				AdmissionRequest: admissionv1.AdmissionRequest{
					Operation: admissionv1.Create,
					Resource: metav1.GroupVersionResource{
						Group:    appsv1beta1.GroupVersion.Group,
						Version:  appsv1beta1.GroupVersion.Version,
						Resource: "advancedcronjobs",
					},
					Object: runtime.RawExtension{
						Raw: createAdvancedCronJobV1Beta1JSON(t, &appsv1beta1.AdvancedCronJob{
							ObjectMeta: metav1.ObjectMeta{
								Name:      "test-acj-v1beta1",
								Namespace: "default",
							},
							Spec: appsv1beta1.AdvancedCronJobSpec{
								Schedule: "0 0 * * *",
								Template: appsv1beta1.CronJobTemplate{
									ImageListPullJobTemplate: &appsv1beta1.ImageListPullJobTemplateSpec{
										Spec: appsv1beta1.ImageListPullJobSpec{
											Images: []string{
												"busybox:latest",
												"alpine:latest",
											},
											ImagePullJobTemplate: appsv1beta1.ImagePullJobTemplate{
												PullSecrets: nil,
												PodSelector: &appsv1beta1.ImagePullJobPodSelector{
													LabelSelector: metav1.LabelSelector{
														MatchLabels: map[string]string{
															"key": "value",
														},
														MatchExpressions: []metav1.LabelSelectorRequirement{
															{Key: "xxx"},
														},
													},
												},
												Parallelism: nil,
												PullPolicy:  nil,
												CompletionPolicy: appsv1beta1.CompletionPolicy{
													Type:                    appsv1beta1.Always,
													ActiveDeadlineSeconds:   int64Ptr(100),
													TTLSecondsAfterFinished: int32Ptr(100),
												},
												SandboxConfig:   nil,
												ImagePullPolicy: "",
											},
										},
									},
								},
							},
						}),
					},
				},
			},
			expectedResult: true,
			expectedError:  true,
		},
		{
			name: "invalid JSON should return error",
			request: admission.Request{
				AdmissionRequest: admissionv1.AdmissionRequest{
					Operation: admissionv1.Create,
					Resource: metav1.GroupVersionResource{
						Group:    appsv1beta1.GroupVersion.Group,
						Version:  appsv1beta1.GroupVersion.Version,
						Resource: "advancedcronjobs",
					},
					Object: runtime.RawExtension{
						Raw: []byte("invalid json"),
					},
				},
			},
			expectedResult: false,
			expectedError:  true,
		},
		{
			name: "update with invalid OldObject JSON should return error",
			request: admission.Request{
				AdmissionRequest: admissionv1.AdmissionRequest{
					Operation: admissionv1.Update,
					Resource: metav1.GroupVersionResource{
						Group:    appsv1beta1.GroupVersion.Group,
						Version:  appsv1beta1.GroupVersion.Version,
						Resource: "advancedcronjobs",
					},
					Object: runtime.RawExtension{
						Raw: createAdvancedCronJobV1Beta1JSON(t, &appsv1beta1.AdvancedCronJob{
							ObjectMeta: metav1.ObjectMeta{
								Name:      "test-acj-update",
								Namespace: "default",
							},
							Spec: appsv1beta1.AdvancedCronJobSpec{
								Schedule: "0 0 * * *",
								Template: appsv1beta1.CronJobTemplate{
									JobTemplate: &batchv1.JobTemplateSpec{
										Spec: batchv1.JobSpec{
											Template: createValidPodTemplateSpec(),
										},
									},
								},
							},
						}),
					},
					OldObject: runtime.RawExtension{
						Raw: []byte("invalid old object json"),
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
			handler := AdvancedCronJobCreateUpdateHandler{
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

func createValidPodTemplateSpec() v1.PodTemplateSpec {
	return v1.PodTemplateSpec{
		Spec: v1.PodSpec{
			Containers: []v1.Container{
				{
					Name:                     "test",
					Image:                    "nginx:latest",
					TerminationMessagePolicy: v1.TerminationMessageReadFile,
					ImagePullPolicy:          v1.PullIfNotPresent,
				},
			},
			RestartPolicy:                 v1.RestartPolicyOnFailure,
			DNSPolicy:                     v1.DNSDefault,
			TerminationGracePeriodSeconds: &[]int64{v1.DefaultTerminationGracePeriodSeconds}[0],
		},
	}
}

func createAdvancedCronJobV1Beta1JSON(t *testing.T, acj *appsv1beta1.AdvancedCronJob) []byte {
	data, err := json.Marshal(acj)
	if err != nil {
		t.Fatalf("failed to marshal AdvancedCronJob v1beta1: %v", err)
	}
	return data
}

func createAdvancedCronJobV1Alpha1JSON(t *testing.T, acj *appsv1alpha1.AdvancedCronJob) []byte {
	data, err := json.Marshal(acj)
	if err != nil {
		t.Fatalf("failed to marshal AdvancedCronJob v1alpha1: %v", err)
	}
	return data
}

func TestDecodeAdvancedCronJobFromRaw(t *testing.T) {
	utilruntime.Must(apis.AddToScheme(scheme.Scheme))
	decoder := admission.NewDecoder(scheme.Scheme)
	handler := AdvancedCronJobCreateUpdateHandler{
		Decoder: decoder,
	}

	tests := []struct {
		name        string
		raw         runtime.RawExtension
		version     string
		expectError bool
	}{
		{
			name: "decode valid v1beta1 AdvancedCronJob",
			raw: runtime.RawExtension{
				Raw: createAdvancedCronJobV1Beta1JSON(t, &appsv1beta1.AdvancedCronJob{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-acj",
						Namespace: "default",
					},
					Spec: appsv1beta1.AdvancedCronJobSpec{
						Schedule: "0 0 * * *",
						Template: appsv1beta1.CronJobTemplate{
							JobTemplate: &batchv1.JobTemplateSpec{
								Spec: batchv1.JobSpec{
									Template: createValidPodTemplateSpec(),
								},
							},
						},
					},
				}),
			},
			version:     appsv1beta1.GroupVersion.Version,
			expectError: false,
		},
		{
			name: "decode valid v1alpha1 AdvancedCronJob with conversion",
			raw: runtime.RawExtension{
				Raw: createAdvancedCronJobV1Alpha1JSON(t, &appsv1alpha1.AdvancedCronJob{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-acj-v1alpha1",
						Namespace: "default",
					},
					Spec: appsv1alpha1.AdvancedCronJobSpec{
						Schedule: "0 0 * * *",
						Template: appsv1alpha1.CronJobTemplate{
							BroadcastJobTemplate: &appsv1alpha1.BroadcastJobTemplateSpec{
								Spec: appsv1alpha1.BroadcastJobSpec{
									Template: createValidPodTemplateSpec(),
								},
							},
						},
					},
				}),
			},
			version:     appsv1alpha1.GroupVersion.Version,
			expectError: false,
		},
		{
			name: "decode invalid JSON",
			raw: runtime.RawExtension{
				Raw: []byte("invalid json"),
			},
			version:     appsv1beta1.GroupVersion.Version,
			expectError: true,
		},
		{
			name: "decode unsupported version",
			raw: runtime.RawExtension{
				Raw: createAdvancedCronJobV1Beta1JSON(t, &appsv1beta1.AdvancedCronJob{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-acj",
						Namespace: "default",
					},
					Spec: appsv1beta1.AdvancedCronJobSpec{
						Schedule: "0 0 * * *",
						Template: appsv1beta1.CronJobTemplate{
							JobTemplate: &batchv1.JobTemplateSpec{
								Spec: batchv1.JobSpec{
									Template: createValidPodTemplateSpec(),
								},
							},
						},
					},
				}),
			},
			version:     "v1",
			expectError: true,
		},
		{
			name: "decode v1alpha1 with invalid JSON in v1alpha1 format",
			raw: runtime.RawExtension{
				Raw: []byte(`{"invalid json for v1alpha1`),
			},
			version:     appsv1alpha1.GroupVersion.Version,
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			obj := &appsv1beta1.AdvancedCronJob{}
			err := handler.decodeAdvancedCronJobFromRaw(tt.raw, tt.version, obj)

			if tt.expectError {
				if err == nil {
					t.Errorf("expected error but got nil")
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
			}
		})
	}
}

// Helper functions
func int32Ptr(i int32) *int32 { return &i }
func int64Ptr(i int64) *int64 { return &i }
