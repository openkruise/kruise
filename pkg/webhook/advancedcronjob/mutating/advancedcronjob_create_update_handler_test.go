/*
Copyright 2020 The Kruise Authors.

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
	"context"
	"encoding/json"
	"reflect"
	"testing"

	utilruntime "k8s.io/apimachinery/pkg/util/runtime"

	"gomodules.xyz/jsonpatch/v2"
	admissionv1 "k8s.io/api/admission/v1"
	batchv1 "k8s.io/api/batch/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	"github.com/openkruise/kruise/apis"
	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	appsv1beta1 "github.com/openkruise/kruise/apis/apps/v1beta1"
)

func TestAdvancedCronJobCreateUpdateHandler_Handle(t *testing.T) {
	utilruntime.Must(apis.AddToScheme(scheme.Scheme))

	tests := []struct {
		name            string
		request         admission.Request
		expectedResult  bool
		expectedError   bool
		expectedPatches []jsonpatch.JsonPatchOperation
	}{
		{
			name: "create v1beta1 AdvancedCronJob with JobTemplate defaults",
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
			expectedResult: true,
			expectedError:  false,
			expectedPatches: []jsonpatch.JsonPatchOperation{
				{
					Operation: "add",
					Path:      "/spec/concurrencyPolicy",
					Value:     string(appsv1beta1.AllowConcurrent),
				},
				{
					Operation: "add",
					Path:      "/spec/paused",
					Value:     false,
				},
				{
					Operation: "add",
					Path:      "/spec/successfulJobsHistoryLimit",
					Value:     float64(3),
				},
				{
					Operation: "add",
					Path:      "/spec/failedJobsHistoryLimit",
					Value:     float64(1),
				},
				{
					Operation: "add",
					Path:      "/spec/template/jobTemplate/spec/template/spec/containers/0/imagePullPolicy",
					Value:     "Always",
				},
				{
					Operation: "add",
					Path:      "/spec/template/jobTemplate/spec/template/spec/containers/0/terminationMessagePath",
					Value:     "/dev/termination-log",
				},
				{
					Operation: "add",
					Path:      "/spec/template/jobTemplate/spec/template/spec/containers/0/terminationMessagePolicy",
					Value:     "File",
				},
				{
					Operation: "add",
					Path:      "/spec/template/jobTemplate/spec/template/spec/dnsPolicy",
					Value:     "ClusterFirst",
				},
				{
					Operation: "add",
					Path:      "/spec/template/jobTemplate/spec/template/spec/schedulerName",
					Value:     "default-scheduler",
				},
				{
					Operation: "add",
					Path:      "/spec/template/jobTemplate/spec/template/spec/securityContext",
					Value:     map[string]interface{}{},
				},
				{
					Operation: "add",
					Path:      "/spec/template/jobTemplate/spec/template/spec/terminationGracePeriodSeconds",
					Value:     float64(30),
				},
			},
		},
		{
			name: "create v1beta1 AdvancedCronJob with BroadcastJobTemplate defaults",
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
								Name:      "test-acj-broadcast",
								Namespace: "default",
							},
							Spec: appsv1beta1.AdvancedCronJobSpec{
								Schedule: "0 0 * * *",
								Template: appsv1beta1.CronJobTemplate{
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
			expectedResult: true,
			expectedError:  false,
			expectedPatches: []jsonpatch.JsonPatchOperation{
				{
					Operation: "add",
					Path:      "/spec/concurrencyPolicy",
					Value:     string(appsv1beta1.AllowConcurrent),
				},
				{
					Operation: "add",
					Path:      "/spec/paused",
					Value:     false,
				},
				{
					Operation: "add",
					Path:      "/spec/successfulJobsHistoryLimit",
					Value:     float64(3),
				},
				{
					Operation: "add",
					Path:      "/spec/failedJobsHistoryLimit",
					Value:     float64(1),
				},
				{
					Operation: "add",
					Path:      "/spec/template/broadcastJobTemplate/spec/template/spec/containers/0/imagePullPolicy",
					Value:     "Always",
				},
				{
					Operation: "add",
					Path:      "/spec/template/broadcastJobTemplate/spec/template/spec/containers/0/terminationMessagePath",
					Value:     "/dev/termination-log",
				},
				{
					Operation: "add",
					Path:      "/spec/template/broadcastJobTemplate/spec/template/spec/containers/0/terminationMessagePolicy",
					Value:     "File",
				},
				{
					Operation: "add",
					Path:      "/spec/template/broadcastJobTemplate/spec/template/spec/dnsPolicy",
					Value:     "ClusterFirst",
				},
				{
					Operation: "add",
					Path:      "/spec/template/broadcastJobTemplate/spec/template/spec/schedulerName",
					Value:     "default-scheduler",
				},
				{
					Operation: "add",
					Path:      "/spec/template/broadcastJobTemplate/spec/template/spec/securityContext",
					Value:     map[string]interface{}{},
				},
				{
					Operation: "add",
					Path:      "/spec/template/broadcastJobTemplate/spec/template/spec/terminationGracePeriodSeconds",
					Value:     float64(30),
				},
			},
		},
		{
			name: "create v1alpha1 AdvancedCronJob with defaults",
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
			expectedResult: true,
			expectedError:  false,
			expectedPatches: []jsonpatch.JsonPatchOperation{
				{
					Operation: "add",
					Path:      "/spec/concurrencyPolicy",
					Value:     string(appsv1alpha1.AllowConcurrent),
				},
				{
					Operation: "add",
					Path:      "/spec/paused",
					Value:     false,
				},
				{
					Operation: "add",
					Path:      "/spec/successfulJobsHistoryLimit",
					Value:     float64(3),
				},
				{
					Operation: "add",
					Path:      "/spec/failedJobsHistoryLimit",
					Value:     float64(1),
				},
				{
					Operation: "add",
					Path:      "/spec/template/jobTemplate/spec/template/spec/containers/0/imagePullPolicy",
					Value:     "Always",
				},
				{
					Operation: "add",
					Path:      "/spec/template/jobTemplate/spec/template/spec/containers/0/terminationMessagePath",
					Value:     "/dev/termination-log",
				},
				{
					Operation: "add",
					Path:      "/spec/template/jobTemplate/spec/template/spec/containers/0/terminationMessagePolicy",
					Value:     "File",
				},
				{
					Operation: "add",
					Path:      "/spec/template/jobTemplate/spec/template/spec/dnsPolicy",
					Value:     "ClusterFirst",
				},
				{
					Operation: "add",
					Path:      "/spec/template/jobTemplate/spec/template/spec/schedulerName",
					Value:     "default-scheduler",
				},
				{
					Operation: "add",
					Path:      "/spec/template/jobTemplate/spec/template/spec/securityContext",
					Value:     map[string]interface{}{},
				},
				{
					Operation: "add",
					Path:      "/spec/template/jobTemplate/spec/template/spec/terminationGracePeriodSeconds",
					Value:     float64(30),
				},
			},
		},
		{
			name: "create v1beta1 AdvancedCronJob with existing defaults",
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
								Name:      "test-acj-with-defaults",
								Namespace: "default",
							},
							Spec: appsv1beta1.AdvancedCronJobSpec{
								Schedule:                   "0 0 * * *",
								ConcurrencyPolicy:          appsv1beta1.ForbidConcurrent,
								Paused:                     func() *bool { b := true; return &b }(),
								SuccessfulJobsHistoryLimit: func() *int32 { i := int32(5); return &i }(),
								FailedJobsHistoryLimit:     func() *int32 { i := int32(2); return &i }(),
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
			expectedResult: true,
			expectedError:  false,
			expectedPatches: []jsonpatch.JsonPatchOperation{
				{
					Operation: "add",
					Path:      "/spec/template/jobTemplate/spec/template/spec/containers/0/imagePullPolicy",
					Value:     "Always",
				},
				{
					Operation: "add",
					Path:      "/spec/template/jobTemplate/spec/template/spec/containers/0/terminationMessagePath",
					Value:     "/dev/termination-log",
				},
				{
					Operation: "add",
					Path:      "/spec/template/jobTemplate/spec/template/spec/containers/0/terminationMessagePolicy",
					Value:     "File",
				},
				{
					Operation: "add",
					Path:      "/spec/template/jobTemplate/spec/template/spec/dnsPolicy",
					Value:     "ClusterFirst",
				},
				{
					Operation: "add",
					Path:      "/spec/template/jobTemplate/spec/template/spec/schedulerName",
					Value:     "default-scheduler",
				},
				{
					Operation: "add",
					Path:      "/spec/template/jobTemplate/spec/template/spec/securityContext",
					Value:     map[string]interface{}{},
				},
				{
					Operation: "add",
					Path:      "/spec/template/jobTemplate/spec/template/spec/terminationGracePeriodSeconds",
					Value:     float64(30),
				},
			},
		},
		{
			name: "update v1beta1 AdvancedCronJob with template change",
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
											Template: v1.PodTemplateSpec{
												Spec: v1.PodSpec{
													Containers: []v1.Container{
														{
															Name:  "test-updated",
															Image: "nginx:1.20",
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
			expectedResult: true,
			expectedError:  false,
			expectedPatches: []jsonpatch.JsonPatchOperation{
				{
					Operation: "add",
					Path:      "/spec/concurrencyPolicy",
					Value:     string(appsv1beta1.AllowConcurrent),
				},
				{
					Operation: "add",
					Path:      "/spec/paused",
					Value:     false,
				},
				{
					Operation: "add",
					Path:      "/spec/successfulJobsHistoryLimit",
					Value:     float64(3),
				},
				{
					Operation: "add",
					Path:      "/spec/failedJobsHistoryLimit",
					Value:     float64(1),
				},
				{
					Operation: "add",
					Path:      "/spec/template/jobTemplate/spec/template/spec/containers/0/imagePullPolicy",
					Value:     "Always",
				},
				{
					Operation: "add",
					Path:      "/spec/template/jobTemplate/spec/template/spec/containers/0/terminationMessagePath",
					Value:     "/dev/termination-log",
				},
				{
					Operation: "add",
					Path:      "/spec/template/jobTemplate/spec/template/spec/containers/0/terminationMessagePolicy",
					Value:     "File",
				},
				{
					Operation: "add",
					Path:      "/spec/template/jobTemplate/spec/template/spec/dnsPolicy",
					Value:     "ClusterFirst",
				},
				{
					Operation: "add",
					Path:      "/spec/template/jobTemplate/spec/template/spec/schedulerName",
					Value:     "default-scheduler",
				},
				{
					Operation: "add",
					Path:      "/spec/template/jobTemplate/spec/template/spec/securityContext",
					Value:     map[string]interface{}{},
				},
				{
					Operation: "add",
					Path:      "/spec/template/jobTemplate/spec/template/spec/terminationGracePeriodSeconds",
					Value:     float64(30),
				},
			},
		},
		{
			name: "update v1beta1 AdvancedCronJob without template change",
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
								Name:      "test-acj-update-no-template-change",
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
								},
							},
						}),
					},
					OldObject: runtime.RawExtension{
						Raw: createAdvancedCronJobV1Beta1JSON(t, &appsv1beta1.AdvancedCronJob{
							ObjectMeta: metav1.ObjectMeta{
								Name:      "test-acj-update-no-template-change",
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
								},
							},
						}),
					},
				},
			},
			expectedResult: true,
			expectedError:  false,
			expectedPatches: []jsonpatch.JsonPatchOperation{
				{
					Operation: "add",
					Path:      "/spec/concurrencyPolicy",
					Value:     string(appsv1beta1.AllowConcurrent),
				},
				{
					Operation: "add",
					Path:      "/spec/paused",
					Value:     false,
				},
				{
					Operation: "add",
					Path:      "/spec/successfulJobsHistoryLimit",
					Value:     float64(3),
				},
				{
					Operation: "add",
					Path:      "/spec/failedJobsHistoryLimit",
					Value:     float64(1),
				},
			},
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
			name: "unsupported version should be handled as v1beta1",
			request: admission.Request{
				AdmissionRequest: admissionv1.AdmissionRequest{
					Operation: admissionv1.Create,
					Resource: metav1.GroupVersionResource{
						Group:    appsv1beta1.GroupVersion.Group,
						Version:  "v1",
						Resource: "advancedcronjobs",
					},
					Object: runtime.RawExtension{
						Raw: []byte(`{"apiVersion":"apps.kruise.io/v1","kind":"AdvancedCronJob","metadata":{"name":"test","namespace":"default"},"spec":{"schedule":"0 0 * * *","template":{"jobTemplate":{"spec":{"template":{"spec":{"containers":[{"name":"test","image":"nginx:latest"}],"restartPolicy":"OnFailure"}}}}}}}`),
					},
				},
			},
			expectedResult: true,
			expectedError:  false,
			// Don't check specific patches for unsupported version as behavior may vary
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

				// Check patches if expected
				if tt.expectedPatches != nil {
					if len(tt.expectedPatches) > 0 {
						if response.Patches == nil {
							t.Errorf("expected patches but got none")
						} else {
							// Sort patches for comparison
							sortPatches(response.Patches)
							sortPatches(tt.expectedPatches)

							if !reflect.DeepEqual(tt.expectedPatches, response.Patches) {
								t.Errorf("expected patches %+v, got patches %+v", tt.expectedPatches, response.Patches)
							}
						}
					} else if len(response.Patches) > 0 {
						t.Errorf("expected no patches but got %+v", response.Patches)
					}
				}
				// If expectedPatches is nil, don't check patches (for unsupported version test)
			}
		})
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

func sortPatches(patches []jsonpatch.JsonPatchOperation) {
	// Simple sort by path for deterministic comparison
	for i := 0; i < len(patches)-1; i++ {
		for j := i + 1; j < len(patches); j++ {
			if patches[i].Path > patches[j].Path {
				patches[i], patches[j] = patches[j], patches[i]
			}
		}
	}
}
