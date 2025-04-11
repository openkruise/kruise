/*
Copyright 2021 The Kruise Authors.

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

package workloadspread

import (
	"strconv"
	"testing"

	fuzz "github.com/AdaLogics/go-fuzz-headers"
	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	fuzzutils "github.com/openkruise/kruise/test/fuzz"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

var (
	fakeScheme = runtime.NewScheme()
)

func init() {
	_ = clientgoscheme.AddToScheme(fakeScheme)
	_ = appsv1alpha1.AddToScheme(fakeScheme)
}

func FuzzPatchFavoriteSubsetMetadataToPod(f *testing.F) {
	f.Fuzz(func(t *testing.T, data []byte) {
		cf := fuzz.NewConsumer(data)

		pod := &corev1.Pod{}
		if err := cf.GenerateStruct(pod); err != nil {
			return
		}

		// Cleanup deletion timestamp when no finalizers exist
		if pod.GetDeletionTimestamp() != nil && len(pod.GetFinalizers()) == 0 {
			pod.SetDeletionTimestamp(nil)
		}

		ws := &appsv1alpha1.WorkloadSpread{}
		if err := cf.GenerateStruct(ws); err != nil {
			return
		}
		if ws.GetAnnotations() == nil {
			ws.Annotations = make(map[string]string)
		}

		ignore, err := cf.GetBool()
		if err != nil {
			return
		}
		ws.GetAnnotations()[IgnorePatchExistingPodsAnnotation] = strconv.FormatBool(ignore)

		subset := &appsv1alpha1.WorkloadSpreadSubset{}
		if err := cf.GenerateStruct(subset); err != nil {
			return
		}

		if err := fuzzutils.GenerateWorkloadSpreadSubsetPatch(cf, subset); err != nil {
			return
		}

		r := &ReconcileWorkloadSpread{
			Client: fake.NewClientBuilder().
				WithScheme(fakeScheme).
				WithObjects(pod.DeepCopy()).
				Build(),
		}

		_ = r.patchFavoriteSubsetMetadataToPod(pod, ws, subset)
	})
}

func FuzzPodPreferredScore(f *testing.F) {
	f.Fuzz(func(t *testing.T, data []byte) {
		cf := fuzz.NewConsumer(data)

		pod := &corev1.Pod{}
		if err := cf.GenerateStruct(pod); err != nil {
			return
		}

		subset := &appsv1alpha1.WorkloadSpreadSubset{}
		if err := cf.GenerateStruct(subset); err != nil {
			return
		}

		if err := fuzzutils.GenerateWorkloadSpreadSubsetPatch(cf, subset); err != nil {
			return
		}

		_ = podPreferredScore(subset, pod)
	})
}
