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
	fuzzutils "github.com/openkruise/kruise/pkg/util/fuzz"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func FuzzNestedField(f *testing.F) {
	f.Fuzz(func(t *testing.T, data []byte) {
		cf := fuzz.NewConsumer(data)

		paths := make([]string, 0)
		if err := cf.CreateSlice(&paths); err != nil {
			return
		}

		// Randomly choose between map or slice data structure
		useMap, err := cf.GetBool()
		if err != nil {
			return
		}

		if useMap {
			m := make(map[string]any) // Test with nested map structure
			if err := cf.FuzzMap(&m); err != nil {
				return
			}
			_, _, _ = NestedField[any](m, paths...)
		} else {
			s := make([]any, 0) // Test with nested slice structure
			if err := cf.CreateSlice(&s); err != nil {
				return
			}
			_, _, _ = NestedField[any](s, paths...)
		}
	})
}

func FuzzIsPodSelected(f *testing.F) {
	f.Fuzz(func(t *testing.T, data []byte) {
		cf := fuzz.NewConsumer(data)

		filter := &appsv1alpha1.TargetFilter{}
		if err := cf.GenerateStruct(filter); err != nil {
			return
		}

		// Ensure selector exists for valid test cases
		if filter.Selector == nil {
			selector := &metav1.LabelSelector{}
			if err := cf.GenerateStruct(selector); err != nil {
				return
			}
			filter.Selector = selector
		}

		// Generate random pod labels for testing
		labels := make(map[string]string)
		if err := cf.FuzzMap(&labels); err != nil {
			return
		}

		_, _ = IsPodSelected(filter, labels)
	})
}

func FuzzHasPercentSubset(f *testing.F) {
	f.Fuzz(func(t *testing.T, data []byte) {
		cf := fuzz.NewConsumer(data)

		ws := &appsv1alpha1.WorkloadSpread{}
		if err := cf.GenerateStruct(ws); err != nil {
			return
		}

		// Only generate subset replicas for valid test cases
		if err := fuzzutils.GenerateWorkloadSpreadSubset(cf, ws,
			fuzzutils.GenerateWorkloadSpreadSubsetReplicas,
		); err != nil {
			return
		}

		_ = hasPercentSubset(ws)
	})
}

func FuzzInjectWorkloadSpreadIntoPod(f *testing.F) {
	f.Fuzz(func(t *testing.T, data []byte) {
		cf := fuzz.NewConsumer(data)

		pod := &corev1.Pod{}
		if err := cf.GenerateStruct(pod); err != nil {
			return
		}

		generatedUID, err := cf.GetString()
		if err != nil {
			return
		}

		ws := &appsv1alpha1.WorkloadSpread{}
		if err := cf.GenerateStruct(ws); err != nil {
			return
		}

		// Generate subset configurations for WorkloadSpread.
		if err := fuzzutils.GenerateWorkloadSpreadSubset(cf, ws); err != nil {
			return
		}

		if len(ws.Spec.Subsets) == 0 {
			return
		}

		// Use the first subset's name.
		subsetName := ws.Spec.Subsets[0].Name

		_, _ = injectWorkloadSpreadIntoPod(ws, pod, subsetName, generatedUID)
	})
}
