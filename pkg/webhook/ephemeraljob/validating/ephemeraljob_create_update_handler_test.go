package validating

import (
	"context"
	"fmt"
	"net/http"
	"reflect"
	"testing"

	admissionv1 "k8s.io/api/admission/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
)

var scheme = runtime.NewScheme()

func init() {
	scheme = runtime.NewScheme()
	_ = alpha1.AddToScheme(scheme)
}

func getFailedJSON() string {
	return `{
    "apiVersion": "apps.kruise.io/v1alpha1",
    "kind": "EphemeralJob",
    "metadata": {
        "creationTimestamp": "2024-05-09T08:29:50Z",
        "generation": 1,
        "name": "ephermeraljob-sample",
        "namespace": "test"
    },
    "spec": {
        "parallelism": 1,
        "replicas": 1,
        "selector": {
            "matchLabels": {
                "app": "test-2"
            }
        },
        "template": {
            "ephemeralContainers": {
                "ephemeralContainerCommon": {
                    "image": "busybox",
                    "imagePullPolicy": "IfNotPresent",
                    "name": "debugger",
                    "securityContext": {
                        "capabilities": {
                            "add": [
                                "SYS_ADMIN",
                                "NET_ADMIN"
                            ]
                        }
                    },
                    "terminationMessagePolicy": "File"
                },
                "targetContainerName": "test"
            }
        },
        "ttlSecondsAfterFinished": 1800
    }
}`
}

func getOKJSON(targetContainerName string) string {
	return fmt.Sprintf(`{
    "apiVersion": "apps.kruise.io/v1alpha1",
    "kind": "EphemeralJob",
    "metadata": {
        "name": "ephermeraljob-sample",
        "namespace": "test"
    },
    "spec": {
        "parallelism": 1,
        "replicas": 1,
        "selector": {
            "matchLabels": {
                "app": "test-2"
            }
        },
        "template": {
            "ephemeralContainers": [{
				"image": "busybox",
				"imagePullPolicy": "IfNotPresent",
				"name": "debugger",
				"securityContext": {
					"capabilities": {
						"add": [
							"SYS_ADMIN",
							"NET_ADMIN"
						]
					}
				},
				"terminationMessagePolicy": "File",
                "targetContainerName": "%v"
            }]
        },
        "ttlSecondsAfterFinished": 1800
    }
}`, targetContainerName)
}

func TestEphemeralJobCreateUpdateHandler_Handle(t *testing.T) {
	type args struct {
		req admission.Request
	}
	tests := []struct {
		name   string
		args   args
		wantOK bool
	}{
		{
			name:   "failed case",
			wantOK: false,
			args: args{
				req: admission.Request{
					AdmissionRequest: admissionv1.AdmissionRequest{
						Operation: admissionv1.Create,
						Object: runtime.RawExtension{
							Raw: []byte(getFailedJSON()),
						},
					},
				},
			},
		},
		{
			name:   "ok case with empty targetContainerName",
			wantOK: true,
			args: args{
				req: admission.Request{
					AdmissionRequest: admissionv1.AdmissionRequest{
						Operation: admissionv1.Create,
						Object: runtime.RawExtension{
							Raw: []byte(getOKJSON("")),
						},
					},
				},
			},
		},
		{
			name:   "ok case with targetContainerName",
			wantOK: true,
			args: args{
				req: admission.Request{
					AdmissionRequest: admissionv1.AdmissionRequest{
						Operation: admissionv1.Create,
						Object: runtime.RawExtension{
							Raw: []byte(getOKJSON("test")),
						},
					},
				},
			},
		},
	}

	d := admission.NewDecoder(scheme)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := &EphemeralJobCreateUpdateHandler{
				Decoder: d,
			}
			if got := h.Handle(context.TODO(), tt.args.req); !reflect.DeepEqual(got.Allowed, tt.wantOK) {
				t.Errorf("Handle() = %v, want %v", got.Result.Code == http.StatusOK, tt.wantOK)
			} else if !got.Allowed {
				t.Log(got.Result.Message)
			}
		})
	}
}
