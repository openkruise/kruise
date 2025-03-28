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
	"encoding/json"
	"fmt"
	"testing"

	fuzz "github.com/AdaLogics/go-fuzz-headers"
	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

// FuzzNestedField tests the NestedField function's ability to handle
// various data structures and access paths through fuzzing.
// It generates random maps/slices and access paths to validate deep field access.
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

// FuzzIsPodSelected validates pod selection logic by generating random
// label selectors and pod labels to test matching/filtering behavior.
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

// FuzzHasPercentSubset verifies detection of percentage-based subset configurations
// in WorkloadSpread resources through randomized input generation.
func FuzzHasPercentSubset(f *testing.F) {
	f.Fuzz(func(t *testing.T, data []byte) {
		cf := fuzz.NewConsumer(data)

		ws := &appsv1alpha1.WorkloadSpread{}
		if err := cf.GenerateStruct(ws); err != nil {
			return
		}

		if len(ws.Spec.Subsets) == 0 {
			subset := appsv1alpha1.WorkloadSpreadSubset{}
			if err := cf.GenerateStruct(&subset); err != nil {
				return
			}
			ws.Spec.Subsets = append(ws.Spec.Subsets, subset)
		}

		for i := range ws.Spec.Subsets {
			if validPercent, err := cf.GetBool(); err == nil && validPercent {
				num, err := cf.GetInt()
				if err != nil {
					return
				}
				ws.Spec.Subsets[i].MaxReplicas = &intstr.IntOrString{
					Type:   intstr.String,
					StrVal: fmt.Sprintf("%d%%", num%1000), // Ensure valid percentage format
				}
			} else {
				maxReplicas := &intstr.IntOrString{}
				if err := cf.GenerateStruct(maxReplicas); err == nil {
					ws.Spec.Subsets[i].MaxReplicas = maxReplicas
				}
			}
		}

		_ = hasPercentSubset(ws)
	})
}

// FuzzInjectWorkloadSpreadIntoPod tests workload spread injection logic
// by generating random pod configurations and subset definitions.
func FuzzInjectWorkloadSpreadIntoPod(f *testing.F) {
	f.Fuzz(func(t *testing.T, data []byte) {
		cf := fuzz.NewConsumer(data)

		pod := &corev1.Pod{}
		if err := cf.GenerateStruct(pod); err != nil {
			return
		}

		subsetName, err := cf.GetString()
		if err != nil {
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

		if len(ws.Spec.Subsets) == 0 {
			subset := &appsv1alpha1.WorkloadSpreadSubset{}
			if err := cf.GenerateStruct(subset); err != nil {
				return
			}
			subset.Name = subsetName
			ws.Spec.Subsets = append(ws.Spec.Subsets, *subset)
		}

		// Configure patches for each subset
		for i := range ws.Spec.Subsets {
			if err := generatePatch(cf, &ws.Spec.Subsets[i]); err != nil {
				return
			}
		}

		_, _ = injectWorkloadSpreadIntoPod(ws, pod, subsetName, generatedUID)
	})
}

// generatePatch creates either valid labeled JSON patches or random byte payloads
// to test both successful merges and error handling scenarios.
func generatePatch(cf *fuzz.ConsumeFuzzer, subset *appsv1alpha1.WorkloadSpreadSubset) error {
	// 50% chance to generate structured label patch
	if isStructured, err := cf.GetBool(); isStructured && err == nil {
		labels := make(map[string]string)
		if err := cf.FuzzMap(&labels); err != nil {
			return err
		}

		patch := map[string]interface{}{
			"metadata": map[string]interface{}{"labels": labels},
		}

		raw, err := json.Marshal(patch)
		if err != nil {
			return err
		}
		subset.Patch.Raw = raw
		return nil
	}

	raw, err := cf.GetBytes()
	if err != nil {
		return err
	}
	subset.Patch.Raw = raw
	return nil
}
