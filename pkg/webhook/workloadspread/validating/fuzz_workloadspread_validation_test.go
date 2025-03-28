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

package validating

import (
	"encoding/json"
	"fmt"
	"testing"

	fuzz "github.com/AdaLogics/go-fuzz-headers"
	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/util/validation/field"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

var (
	fakeScheme = runtime.NewScheme()
	// supportedKinds defines valid workload types and their API versions
	supportedKinds = map[string]string{
		"CloneSet":    "apps.kruise.io/v1alpha1",
		"Deployment":  "apps/v1",
		"StatefulSet": "apps/v1",
		"Job":         "batch/v1",
		"ReplicaSet":  "apps/v1",
		"TFJob":       "training.kubedl.io/v1",
	}
)

func init() {
	_ = clientgoscheme.AddToScheme(fakeScheme)
	_ = appsv1alpha1.AddToScheme(fakeScheme)
}

// FuzzValidateWorkloadSpreadSpec tests validation logic with fuzzed inputs
// covering target references, subsets, scheduling strategies and filters
func FuzzValidateWorkloadSpreadSpec(f *testing.F) {
	f.Fuzz(func(t *testing.T, data []byte) {
		cf := fuzz.NewConsumer(data)

		ws := &appsv1alpha1.WorkloadSpread{}
		if err := cf.GenerateStruct(ws); err != nil {
			return
		}

		// Configure target reference
		if useTargetRef, err := cf.GetBool(); useTargetRef && err == nil {
			targetRef := &appsv1alpha1.TargetReference{}
			if err := cf.GenerateStruct(targetRef); err != nil {
				return
			}

			// Generate valid group/kind combinations for 50% of cases
			if makeValid, err := cf.GetBool(); makeValid && err == nil {
				var kind string
				if kindIndex, err := cf.GetInt(); err == nil {
					kind = []string{"CloneSet", "Deployment", "StatefulSet", "Job", "ReplicaSet", "TFJob"}[kindIndex%6]
				}
				targetRef.Kind = kind
				targetRef.APIVersion = supportedKinds[kind]
				targetRef.Name = "valid-target" // Ensure findable resource name
				ws.SetNamespace("default")      // Ensure findable resource namespace
			}
			ws.Spec.TargetReference = targetRef
		}

		// Generate subsets with controlled properties
		if subsetCount, err := cf.GetInt(); subsetCount > 0 && err == nil {
			ws.Spec.Subsets = make([]appsv1alpha1.WorkloadSpreadSubset, subsetCount%5)
			for i := range ws.Spec.Subsets {
				subset := &appsv1alpha1.WorkloadSpreadSubset{}
				if err := cf.GenerateStruct(subset); err != nil {
					return
				}
				ws.Spec.Subsets[i] = *subset

				// Ensure last subset has nil maxReplicas for adaptive strategy
				nilReplicas, err := cf.GetBool()
				if err != nil {
					return
				}
				if i == len(ws.Spec.Subsets)-1 && nilReplicas {
					ws.Spec.Subsets[i].MaxReplicas = nil
				}
			}
		}

		// Configure scheduling strategy
		if useAdaptive, err := cf.GetBool(); useAdaptive && err == nil {
			ws.Spec.ScheduleStrategy.Type = appsv1alpha1.AdaptiveWorkloadSpreadScheduleStrategyType
			if seconds, err := cf.GetInt(); err == nil {
				ws.Spec.ScheduleStrategy.Adaptive = &appsv1alpha1.AdaptiveWorkloadSpreadStrategy{
					RescheduleCriticalSeconds: pointer.Int32(int32(seconds%3000 - 1000)),
				}
			}
		} else {
			ws.Spec.ScheduleStrategy.Type = appsv1alpha1.FixedWorkloadSpreadScheduleStrategyType
			if invalid, err := cf.GetBool(); invalid && err == nil {
				if strategy, err := cf.GetString(); err == nil {
					ws.Spec.ScheduleStrategy.Type = appsv1alpha1.WorkloadSpreadScheduleStrategyType(strategy)
				}
			}
		}

		// Generate target filters for 50% of cases
		if useFilter, err := cf.GetBool(); useFilter && err == nil {
			filter := &appsv1alpha1.TargetFilter{}
			if err := cf.GenerateStruct(filter); err != nil {
				return
			}
			ws.Spec.TargetFilter = filter
		}

		// Generate white list for 50% of cases
		whiteList, err := cf.GetString()
		if err != nil {
			return
		}
		if validWhiteList, err := cf.GetBool(); validWhiteList && err == nil {
			whiteList = "{\"workloads\":[{\"Group\":\"test\",\"Kind\":\"TFJob\"}]}"
			if matched, err := cf.GetBool(); matched && err == nil {
				whiteList = "{\"workloads\":[{\"Group\":\"training.kubedl.io\",\"Kind\":\"TFJob\"}]}"
			}
		}

		// Create fake client with preconfigured resources
		fakeClient := fake.NewClientBuilder().
			WithScheme(fakeScheme).
			WithObjects(
				&appsv1alpha1.CloneSet{ObjectMeta: metav1.ObjectMeta{Name: "valid-target", Namespace: "default"}},
				&appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: "valid-target", Namespace: "default"}},
				&appsv1.StatefulSet{ObjectMeta: metav1.ObjectMeta{Name: "valid-target", Namespace: "default"}},
				&batchv1.Job{ObjectMeta: metav1.ObjectMeta{Name: "valid-target", Namespace: "default"}},
				&appsv1.ReplicaSet{ObjectMeta: metav1.ObjectMeta{Name: "valid-target", Namespace: "default"}},
				&corev1.ConfigMap{
					ObjectMeta: metav1.ObjectMeta{Name: "kruise-configuration", Namespace: "kruise-system"},
					Data: map[string]string{
						"WorkloadSpread_Watch_Custom_Workload_WhiteList": whiteList,
					},
				},
			).Build()

		handler := &WorkloadSpreadCreateUpdateHandler{Client: fakeClient}
		_ = validateWorkloadSpreadSpec(handler, ws, field.NewPath("spec"))
	})
}

// FuzzValidateWorkloadSpreadSubsets tests subset validation logic with various combinations
// of workload types, patches, and replica configurations.
func FuzzValidateWorkloadSpreadSubsets(f *testing.F) {
	f.Fuzz(func(t *testing.T, data []byte) {
		cf := fuzz.NewConsumer(data)

		ws := &appsv1alpha1.WorkloadSpread{}
		if err := cf.GenerateStruct(ws); err != nil {
			return
		}

		if len(ws.Spec.Subsets) == 0 {
			subset := appsv1alpha1.WorkloadSpreadSubset{}
			if err := cf.GenerateStruct(&subset); err == nil {
				ws.Spec.Subsets = append(ws.Spec.Subsets, subset)
			}
		}

		// Control duplicate subset names
		for i := range ws.Spec.Subsets {
			if sameName, err := cf.GetBool(); sameName && err == nil && i > 0 {
				ws.Spec.Subsets[i].Name = ws.Spec.Subsets[0].Name
			}
		}

		// Generate random workload type
		var workload client.Object
		switch workloadType, _ := cf.GetInt(); workloadType % 6 {
		case 0:
			workload = &appsv1alpha1.CloneSet{}
		case 1:
			workload = &appsv1.Deployment{}
		case 2:
			workload = &appsv1.ReplicaSet{}
		case 3:
			workload = &batchv1.Job{}
		case 4:
			workload = &appsv1.StatefulSet{}
		case 5:
			workload = &unstructured.Unstructured{}
		}
		if err := cf.GenerateStruct(workload); err != nil {
			return
		}

		for i := range ws.Spec.Subsets {
			if err := generatePatch(cf, &ws.Spec.Subsets[i]); err != nil {
				return
			}
			if err := generateMaxReplicas(cf, &ws.Spec.Subsets[i]); err != nil {
				return
			}
		}

		_ = validateWorkloadSpreadSubsets(ws, ws.Spec.Subsets, workload, field.NewPath("spec").Child("subsets"))
	})
}

// FuzzValidateWorkloadSpreadConflict tests conflict detection between multiple WorkloadSpread instances
// by generating controlled combinations of target references.
func FuzzValidateWorkloadSpreadConflict(f *testing.F) {
	f.Fuzz(func(t *testing.T, data []byte) {
		cf := fuzz.NewConsumer(data)

		ws := &appsv1alpha1.WorkloadSpread{}
		if err := cf.GenerateStruct(ws); err != nil {
			return
		}

		others := make([]appsv1alpha1.WorkloadSpread, 0)
		if numOthers, err := cf.GetInt(); err == nil {
			for i := 0; i < numOthers%5; i++ {
				other := appsv1alpha1.WorkloadSpread{}
				if err := cf.GenerateStruct(&other); err != nil {
					continue
				}

				// Control name conflicts
				if sameName, err := cf.GetBool(); sameName && err == nil {
					other.Name = ws.Name
				}

				// Create target reference conflicts
				if ws.Spec.TargetReference != nil {
					if conflict, err := cf.GetBool(); conflict && err == nil {
						other.Spec.TargetReference = &appsv1alpha1.TargetReference{
							APIVersion: ws.Spec.TargetReference.APIVersion,
							Kind:       ws.Spec.TargetReference.Kind,
							Name:       ws.Spec.TargetReference.Name,
						}
					}
				}
				others = append(others, other)
			}
		}

		_ = validateWorkloadSpreadConflict(ws, others, field.NewPath("spec"))
	})
}

// FuzzValidateWorkloadSpreadTargetRefUpdate tests validation rules for target reference updates
// by generating controlled field modifications.
func FuzzValidateWorkloadSpreadTargetRefUpdate(f *testing.F) {
	f.Fuzz(func(t *testing.T, data []byte) {
		cf := fuzz.NewConsumer(data)

		targetRef := &appsv1alpha1.TargetReference{}
		if err := cf.GenerateStruct(targetRef); err != nil {
			return
		}

		oldTargetRef := &appsv1alpha1.TargetReference{}
		if err := cf.GenerateStruct(oldTargetRef); err != nil {
			return
		}

		// Control group version consistency
		if sameGroup, err := cf.GetBool(); sameGroup && err == nil {
			if group, err := cf.GetString(); err == nil {
				targetRef.APIVersion, oldTargetRef.APIVersion = group+"/v1", group+"/v1"
			}
		}

		// Control kind consistency
		if sameKind, err := cf.GetBool(); sameKind && err == nil {
			if kind, err := cf.GetString(); err == nil {
				targetRef.Kind, oldTargetRef.Kind = kind, kind
			}
		}

		// Control name consistency
		if sameName, err := cf.GetBool(); sameName && err == nil {
			if name, err := cf.GetString(); err == nil {
				targetRef.Name, oldTargetRef.Name = name, name
			}
		}

		_ = validateWorkloadSpreadTargetRefUpdate(targetRef, oldTargetRef, field.NewPath("spec"))
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

// generateMaxReplicas creates valid percentage-based or integer values for replica counts,
// with random fallback to ensure edge case coverage.
func generateMaxReplicas(cf *fuzz.ConsumeFuzzer, subset *appsv1alpha1.WorkloadSpreadSubset) error {
	subset.MaxReplicas = &intstr.IntOrString{}

	// 50% probability to generate valid format
	if useCustom, err := cf.GetBool(); useCustom && err == nil {
		// 50% probability between percentage and integer
		if usePercent, err := cf.GetBool(); usePercent && err == nil {
			num, err := cf.GetInt()
			if err != nil {
				return err
			}
			subset.MaxReplicas = &intstr.IntOrString{
				Type:   intstr.String,
				StrVal: fmt.Sprintf("%d%%", num%1000), // Ensure valid percentage format
			}
		} else {
			num, err := cf.GetInt()
			if err != nil {
				return err
			}
			subset.MaxReplicas = &intstr.IntOrString{
				Type:   intstr.Int,
				IntVal: int32(num),
			}
		}
		return nil
	}
	maxReplicas := &intstr.IntOrString{}
	if err := cf.GenerateStruct(maxReplicas); err != nil {
		return err
	}
	subset.MaxReplicas = maxReplicas
	return nil
}
