package mutating

import (
	"context"
	"reflect"
	"sort"
	"testing"

	utilruntime "k8s.io/apimachinery/pkg/util/runtime"

	"gomodules.xyz/jsonpatch/v2"
	admissionv1 "k8s.io/api/admission/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	"github.com/openkruise/kruise/apis"
	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
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
			Object: runtime.RawExtension{
				Raw: []byte(oldBroadcastJobStr),
			},
		},
	}
	resp := handler.Handle(context.TODO(), req)

	expectedPatches := []jsonpatch.JsonPatchOperation{
		{
			Operation: "add",
			Path:      "/spec/completionPolicy",
			Value:     map[string]interface{}{"type": string(appsv1alpha1.Always)},
		},
		{
			Operation: "add",
			Path:      "/spec/failurePolicy",
			Value:     map[string]interface{}{"type": string(appsv1alpha1.FailurePolicyTypeFailFast)},
		},
		{
			Operation: "remove",
			Path:      "/spec/paused",
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
