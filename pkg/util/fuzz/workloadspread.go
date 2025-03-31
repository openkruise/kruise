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

package fuzz

import (
	"encoding/json"

	fuzz "github.com/AdaLogics/go-fuzz-headers"
	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/utils/pointer"
)

var (
	supportedKinds = map[string]string{
		"CloneSet":    "apps.kruise.io/v1alpha1",
		"Deployment":  "apps/v1",
		"StatefulSet": "apps/v1",
		"Job":         "batch/v1",
		"ReplicaSet":  "apps/v1",
		"TFJob":       "training.kubedl.io/v1",
	}
)

type WSSubsetFunc = func(cf *fuzz.ConsumeFuzzer, subset *appsv1alpha1.WorkloadSpreadSubset) error

func GenerateWorkloadSpreadTargetReference(cf *fuzz.ConsumeFuzzer, ws *appsv1alpha1.WorkloadSpread) error {
	useTargetRef, err := cf.GetBool()
	if err != nil {
		return err
	}

	if useTargetRef {
		targetRef := &appsv1alpha1.TargetReference{}
		if err := cf.GenerateStruct(targetRef); err != nil {
			return err
		}

		if makeValid, err := cf.GetBool(); makeValid && err == nil {
			var kind string
			if kindIndex, err := cf.GetInt(); err == nil {
				kind = []string{"CloneSet", "Deployment", "StatefulSet", "Job", "ReplicaSet", "TFJob"}[kindIndex%6]
			}
			targetRef.Kind = kind
			targetRef.APIVersion = supportedKinds[kind]
			targetRef.Name = "valid-target"
			ws.SetNamespace("default")
		}

		ws.Spec.TargetReference = targetRef
	}

	return nil
}

func GenerateWorkloadSpreadScheduleStrategy(cf *fuzz.ConsumeFuzzer, ws *appsv1alpha1.WorkloadSpread) error {
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
	return nil
}

func GenerateWorkloadSpreadTargetFilter(cf *fuzz.ConsumeFuzzer, ws *appsv1alpha1.WorkloadSpread) error {
	useFilter, err := cf.GetBool()
	if err != nil {
		return err
	}

	if useFilter {
		filter := &appsv1alpha1.TargetFilter{}
		if err := cf.GenerateStruct(filter); err != nil {
			return err
		}
		ws.Spec.TargetFilter = filter
	}
	return nil
}

func GenerateWorkloadSpreadSubset(cf *fuzz.ConsumeFuzzer, ws *appsv1alpha1.WorkloadSpread, fns ...WSSubsetFunc) error {
	num, err := cf.GetInt()
	if err != nil {
		return err
	}

	if len(fns) == 0 {
		fns = []WSSubsetFunc{
			GenerateWorkloadSpreadSubsetName,
			GenerateWorkloadSpreadSubsetPatch,
			GenerateWorkloadSpreadSubsetReplicas,
			GenerateWorkloadSpreadNodeSelectorTerm,
			GenerateWorkloadSpreadTolerations,
		}
	}

	nSubsets := (num % 5) + 1
	subsets := make([]appsv1alpha1.WorkloadSpreadSubset, nSubsets)

	for i := 0; i < nSubsets; i++ {
		for _, fn := range fns {
			if err := fn(cf, &subsets[i]); err != nil {
				return err
			}
		}
	}

	ws.Spec.Subsets = subsets
	return nil
}

func GenerateWorkloadSpreadSubsetName(cf *fuzz.ConsumeFuzzer, subset *appsv1alpha1.WorkloadSpreadSubset) error {
	name, err := cf.GetString()
	if err != nil {
		return err
	}
	subset.Name = name
	return nil
}

func GenerateWorkloadSpreadSubsetPatch(cf *fuzz.ConsumeFuzzer, subset *appsv1alpha1.WorkloadSpreadSubset) error {
	isStructured, err := cf.GetBool()
	if err != nil {
		return err
	}

	var raw []byte
	if isStructured {
		labels := make(map[string]string)
		if err := cf.FuzzMap(&labels); err != nil {
			return err
		}
		patch := map[string]interface{}{
			"metadata": map[string]interface{}{"labels": labels},
		}
		raw, err = json.Marshal(patch)
		if err != nil {
			return err
		}
	} else {
		raw, err = cf.GetBytes()
		if err != nil {
			return err
		}
	}
	subset.Patch.Raw = raw
	return nil
}

func GenerateWorkloadSpreadSubsetReplicas(cf *fuzz.ConsumeFuzzer, subset *appsv1alpha1.WorkloadSpreadSubset) error {
	maxReplicas, err := GenerateSubsetReplicas(cf)
	if err != nil {
		return err
	}
	subset.MaxReplicas = &maxReplicas
	return nil
}

func GenerateWorkloadSpreadNodeSelectorTerm(cf *fuzz.ConsumeFuzzer, subset *appsv1alpha1.WorkloadSpreadSubset) error {
	isStructured, err := cf.GetBool()
	if err != nil {
		return err
	}

	var term corev1.NodeSelectorTerm
	if isStructured {
		term = corev1.NodeSelectorTerm{
			MatchExpressions: []corev1.NodeSelectorRequirement{
				{
					Key:      "key",
					Operator: "In",
					Values:   []string{"value"},
				},
			},
		}
	} else {
		if err := cf.GenerateStruct(&term); err != nil {
			return err
		}
	}
	subset.RequiredNodeSelectorTerm = &term
	return nil
}

func GenerateWorkloadSpreadTolerations(cf *fuzz.ConsumeFuzzer, subset *appsv1alpha1.WorkloadSpreadSubset) error {
	isStructured, err := cf.GetBool()
	if err != nil {
		return err
	}

	var tolerations []corev1.Toleration
	if isStructured {
		tolerations = []corev1.Toleration{
			{
				Key:      "key",
				Operator: "In",
				Value:    "value",
			},
		}
	} else {
		toleration := corev1.Toleration{}
		if err := cf.GenerateStruct(&toleration); err != nil {
			return err
		}
		tolerations = []corev1.Toleration{toleration}
	}
	subset.Tolerations = tolerations
	return nil
}
