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
	"testing"

	fuzz "github.com/AdaLogics/go-fuzz-headers"
	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	fuzzutils "github.com/openkruise/kruise/test/fuzz"
	corev1 "k8s.io/api/core/v1"
)

func FuzzNestedField(f *testing.F) {
	f.Fuzz(func(t *testing.T, data []byte) {
		cf := fuzz.NewConsumer(data)

		paths := make([]string, 0)
		if err := cf.CreateSlice(&paths); err != nil {
			return
		}

		useMap, err := cf.GetBool()
		if err != nil {
			return
		}

		if useMap {
			m := make(map[string]any)
			if err := cf.FuzzMap(&m); err != nil {
				return
			}
			_, _, _ = NestedField[any](m, paths...)
		} else {
			s := make([]any, 0)
			if err := cf.CreateSlice(&s); err != nil {
				return
			}
			_, _, _ = NestedField[any](s, paths...)
		}
	})
}

func FuzzInjectWorkloadSpreadIntoPod(f *testing.F) {
	f.Fuzz(func(t *testing.T, data []byte) {
		cf := fuzz.NewConsumer(data)

		pod := &corev1.Pod{}
		if err := cf.GenerateStruct(pod); err != nil {
			return
		}

		ws := &appsv1alpha1.WorkloadSpread{}
		if err := cf.GenerateStruct(ws); err != nil {
			return
		}

		generatedUID, err := cf.GetString()
		if err != nil {
			return
		}

		if err := fuzzutils.GenerateWorkloadSpreadSubset(cf, ws); err != nil {
			return
		}

		if len(ws.Spec.Subsets) == 0 {
			return
		}

		_, _ = injectWorkloadSpreadIntoPod(ws, pod, ws.Spec.Subsets[0].Name, generatedUID)
	})
}
