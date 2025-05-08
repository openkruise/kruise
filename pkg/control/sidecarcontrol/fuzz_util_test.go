package sidecarcontrol

import (
	"encoding/json"
	"testing"

	fuzz "github.com/AdaLogics/go-fuzz-headers"
	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	fuzzutils "github.com/openkruise/kruise/test/fuzz"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func FuzzPatchPodMetadata(f *testing.F) {
	f.Fuzz(func(t *testing.T, data []byte) {
		cf := fuzz.NewConsumer(data)
		metadata := &metav1.ObjectMeta{}
		if err := cf.GenerateStruct(metadata); err != nil {
			return
		}

		jsonPatch, err := cf.GetBool()
		if err != nil {
			return
		}

		patch, err := fuzzutils.GeneratePatchPodMetadata(cf, jsonPatch)
		if err != nil || patch == nil {
			return
		}

		// Make sure there is a probability that the same key exists.
		if exist, err := cf.GetBool(); exist && err == nil {
			for key := range patch.Annotations {
				if jsonPatch {
					m := make(map[string]string)
					if err := cf.FuzzMap(&m); err != nil {
						return
					}
					bytes, _ := json.Marshal(m)
					metadata.GetAnnotations()[key] = string(bytes)
				} else {
					val, err := cf.GetString()
					if err != nil {
						return
					}
					metadata.GetAnnotations()[key] = val
				}
			}
		}

		_, err = PatchPodMetadata(metadata, []appsv1alpha1.SidecarSetPatchPodMetadata{*patch})
		// Because function can capture panic error, so here to deal with the errors due to panic,
		// Meanwhile, the error of the failed Patch merge in JSON format needs to be ignored.
		if err != nil {
			if !jsonPatch && patch.PatchPolicy == appsv1alpha1.SidecarSetMergePatchJsonPatchPolicy {
				t.Logf("Ignore patch merge in JSON format failed: %s", err)
				return
			}
			// The panic error will be printed.
			t.Errorf("Panic: %s", err)
		}
	})
}
