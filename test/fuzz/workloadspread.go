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
	"math"
	"math/rand"

	fuzz "github.com/AdaLogics/go-fuzz-headers"
	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/utils/pointer"
)

var (
	SupportedKinds = []string{
		"CloneSet",
		"Deployment",
		"StatefulSet",
		"Job",
		"ReplicaSet",
		"TFJob",
	}
	SupportedKindsMap = map[string]string{
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
	isStructured, err := cf.GetBool()
	if err != nil {
		return err
	}

	if !isStructured {
		targetRef := &appsv1alpha1.TargetReference{}
		if err := cf.GenerateStruct(targetRef); err != nil {
			return err
		}
		ws.Spec.TargetReference = targetRef
		return nil
	}

	targetRef := &appsv1alpha1.TargetReference{}
	kindIndex, err := cf.GetInt()
	if err != nil {
		return err
	}
	kindIndex = int(math.Abs(float64(kindIndex))) % len(SupportedKinds)
	kind := SupportedKinds[kindIndex]
	targetRef.Kind = kind
	targetRef.APIVersion = SupportedKindsMap[kind]
	targetRef.Name = "valid-target"
	ws.Namespace = "default"

	ws.Spec.TargetReference = targetRef
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
	selector := &metav1.LabelSelector{}
	if err := GenerateLabelSelector(cf, selector); err != nil {
		return err
	}
	replicasPathList := make([]string, 0)
	if err := cf.CreateSlice(&replicasPathList); err != nil {
		return err
	}
	ws.Spec.TargetFilter = &appsv1alpha1.TargetFilter{
		Selector:         selector,
		ReplicasPathList: replicasPathList,
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
	rawExtension := &runtime.RawExtension{}
	if err := GeneratePatch(cf, rawExtension); err != nil {
		return err
	}
	subset.Patch = *rawExtension
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
	term := corev1.NodeSelectorTerm{}
	if err := GenerateNodeSelectorTerm(cf, &term); err != nil {
		return err
	}
	subset.RequiredNodeSelectorTerm = &term
	return nil
}

func GenerateWorkloadSpreadTolerations(cf *fuzz.ConsumeFuzzer, subset *appsv1alpha1.WorkloadSpreadSubset) error {
	tolerations := make([]corev1.Toleration, rand.Intn(2)+1)
	for i := range tolerations {
		if err := GenerateTolerations(cf, &tolerations[i]); err != nil {
			return err
		}
	}
	subset.Tolerations = tolerations
	return nil
}
