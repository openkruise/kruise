package mutating

import (
	"context"
	"encoding/json"
	"reflect"
	"sort"
	"testing"

	"gomodules.xyz/jsonpatch/v2"
	admissionv1 "k8s.io/api/admission/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	"github.com/openkruise/kruise/apis"
	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	appsv1beta1 "github.com/openkruise/kruise/apis/apps/v1beta1"
)

func TestHandle(t *testing.T) {
	utilruntime.Must(apis.AddToScheme(scheme.Scheme))

	oldBroadcastJobStr := `{
    "metadata": {
        "creationTimestamp": null
    },
    "spec": {
        "parallelism": 100,
        "template": {
            "metadata": {
                "creationTimestamp": null
            },
            "spec": {
                "containers": null,
                "restartPolicy": "Always",
                "terminationGracePeriodSeconds": 30,
                "dnsPolicy": "ClusterFirst",
                "securityContext": {},
                "schedulerName": "fake-scheduler"
            }
        },
        "paused": false
    },
    "status": {
        "active": 0,
        "succeeded": 0,
        "failed": 0,
        "desired": 0,
        "phase": ""
    }
}`

	decoder := admission.NewDecoder(scheme.Scheme)
	handler := BroadcastJobCreateUpdateHandler{
		Decoder: decoder,
	}

	req := admission.Request{
		AdmissionRequest: admissionv1.AdmissionRequest{
			Resource: metav1.GroupVersionResource{
				Group:    appsv1beta1.GroupVersion.Group,
				Version:  appsv1beta1.GroupVersion.Version,
				Resource: "broadcastjobs",
			},
			Object: runtime.RawExtension{
				Raw: []byte(oldBroadcastJobStr),
			},
		},
	}
	resp := handler.Handle(context.TODO(), req)

	expectedPatches := []jsonpatch.JsonPatchOperation{
		{
			Operation: "remove",
			Path:      "/metadata/creationTimestamp",
		},
		{
			Operation: "add",
			Path:      "/spec/completionPolicy",
			Value:     map[string]interface{}{"type": string(appsv1beta1.Always)},
		},
		{
			Operation: "add",
			Path:      "/spec/failurePolicy",
			Value:     map[string]interface{}{"type": string(appsv1beta1.FailurePolicyTypeFailFast)},
		},
		{
			Operation: "remove",
			Path:      "/spec/paused",
		},
		{
			Operation: "remove",
			Path:      "/spec/template/metadata/creationTimestamp",
		},
	}
	// The response order is not deterministic
	sort.SliceStable(resp.Patches, func(i, j int) bool {
		return resp.Patches[i].Path < resp.Patches[j].Path
	})

	if !reflect.DeepEqual(expectedPatches, resp.Patches) {
		t.Fatalf("expected patches %+v, got patches %+v", expectedPatches, resp.Patches)
	}
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
			name: "create v1beta1 BroadcastJob with defaults",
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
			name: "create v1alpha1 BroadcastJob with defaults",
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
			name: "create v1beta1 BroadcastJob with existing completion policy",
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
								Name:      "test-bj-with-policy",
								Namespace: "default",
							},
							Spec: appsv1beta1.BroadcastJobSpec{
								CompletionPolicy: appsv1beta1.CompletionPolicy{
									Type: appsv1beta1.Never,
								},
								FailurePolicy: appsv1beta1.FailurePolicy{
									Type: appsv1beta1.FailurePolicyTypeContinue,
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
			expectedResult: true,
			expectedError:  false,
		},
		{
			name: "update v1beta1 BroadcastJob with template change",
			request: admission.Request{
				AdmissionRequest: admissionv1.AdmissionRequest{
					Operation: admissionv1.Update,
					Resource: metav1.GroupVersionResource{
						Group:    appsv1beta1.GroupVersion.Group,
						Version:  appsv1beta1.GroupVersion.Version,
						Resource: "broadcastjobs",
					},
					Object: runtime.RawExtension{
						Raw: createBroadcastJobV1Beta1JSON(t, &appsv1beta1.BroadcastJob{
							ObjectMeta: metav1.ObjectMeta{
								Name:      "test-bj-update",
								Namespace: "default",
							},
							Spec: appsv1beta1.BroadcastJobSpec{
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
						}),
					},
					OldObject: runtime.RawExtension{
						Raw: createBroadcastJobV1Beta1JSON(t, &appsv1beta1.BroadcastJob{
							ObjectMeta: metav1.ObjectMeta{
								Name:      "test-bj-update",
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
