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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type UDSubsetFunc = func(cf *fuzz.ConsumeFuzzer, subset *appsv1alpha1.Subset) error

func GenerateUnitedDeploymentReplicas(cf *fuzz.ConsumeFuzzer, ud *appsv1alpha1.UnitedDeployment) error {
	if rep, err := cf.GetInt(); err == nil {
		r := int32(rep)
		ud.Spec.Replicas = &r
	} else {
		r := int32(5)
		ud.Spec.Replicas = &r
	}
	return nil
}

func GenerateUnitedDeploymentSubset(cf *fuzz.ConsumeFuzzer, ud *appsv1alpha1.UnitedDeployment, fns ...UDSubsetFunc) error {
	num, err := cf.GetInt()
	if err != nil {
		return err
	}

	if len(fns) == 0 {
		fns = []UDSubsetFunc{
			GenerateUnitedDeploymentSubsetName,
			GenerateUnitedDeploymentSubSetReplicas,
			GenerateUnitedDeploymentPatch,
			GenerateUnitedDeploymentNodeSelectorTerm,
			GenerateUnitedDeploymentTolerations,
		}
	}

	nSubsets := (num % 5) + 1
	subsets := make([]appsv1alpha1.Subset, nSubsets)

	for i := 0; i < nSubsets; i++ {
		for _, fn := range fns {
			if err := fn(cf, &subsets[i]); err != nil {
				return err
			}
		}
	}

	ud.Spec.Topology.Subsets = subsets
	return nil
}

func GenerateUnitedDeploymentScheduleStrategy(cf *fuzz.ConsumeFuzzer, ud *appsv1alpha1.UnitedDeployment) error {
	choice, err := cf.GetInt()
	if err != nil {
		return err
	}

	switch choice % 3 {
	case 0:
		ud.Spec.Topology.ScheduleStrategy.Type = appsv1alpha1.AdaptiveUnitedDeploymentScheduleStrategyType
	case 1:
		ud.Spec.Topology.ScheduleStrategy.Type = appsv1alpha1.FixedUnitedDeploymentScheduleStrategyType
	case 2:
		str, err := cf.GetString()
		if err != nil {
			return err
		}
		ud.Spec.Topology.ScheduleStrategy.Type = appsv1alpha1.UnitedDeploymentScheduleStrategyType(str)
	}
	return nil
}

func GenerateUnitedDeploymentSelector(cf *fuzz.ConsumeFuzzer, ud *appsv1alpha1.UnitedDeployment) error {
	if set, err := cf.GetBool(); set && err == nil {
		selector := &metav1.LabelSelector{}
		if nonEmpty, err := cf.GetBool(); nonEmpty && err == nil {
			labelsMap := make(map[string]string)
			if err := cf.FuzzMap(&labelsMap); err != nil {
				return err
			}
			selector.MatchLabels = labelsMap
		} else {
			selector.MatchLabels = map[string]string{}
			selector.MatchExpressions = []metav1.LabelSelectorRequirement{}
		}
		ud.Spec.Selector = selector
	}
	return nil
}

func GenerateUnitedDeploymentTemplate(cf *fuzz.ConsumeFuzzer, ud *appsv1alpha1.UnitedDeployment) error {
	var tmpl appsv1alpha1.SubsetTemplate

	choice, err := cf.GetInt()
	if err != nil {
		return err
	}

	switch choice % 5 {
	case 0:
		s := &appsv1alpha1.DeploymentTemplateSpec{}
		if err = cf.GenerateStruct(s); err != nil {
			return err
		}
		tmpl.DeploymentTemplate = s
	case 1:
		s := &appsv1alpha1.AdvancedStatefulSetTemplateSpec{}
		if err = cf.GenerateStruct(s); err != nil {
			return err
		}
		tmpl.AdvancedStatefulSetTemplate = s
	case 2:
		s := &appsv1alpha1.StatefulSetTemplateSpec{}
		if err = cf.GenerateStruct(s); err != nil {
			return err
		}
		tmpl.StatefulSetTemplate = s
	case 3:
		s := &appsv1alpha1.CloneSetTemplateSpec{}
		if err = cf.GenerateStruct(s); err != nil {
			return err
		}
		tmpl.CloneSetTemplate = s
	case 4:
		if err = cf.GenerateStruct(&tmpl); err != nil {
			return err
		}
	}

	ud.Spec.Template = tmpl
	return nil
}
func GenerateUnitedDeploymentUpdateStrategy(cf *fuzz.ConsumeFuzzer, ud *appsv1alpha1.UnitedDeployment) error {
	setParts, err := cf.GetBool()
	if err != nil || !setParts {
		return err
	}

	np, err := cf.GetInt()
	if err != nil {
		return err
	}
	numParts := np % 3
	partitions := make(map[string]int32)

	for j := 0; j < numParts; j++ {
		var key string

		useExisting, err := cf.GetBool()
		if err == nil && useExisting {
			if len(ud.Spec.Topology.Subsets) > 0 {
				key = ud.Spec.Topology.Subsets[j%len(ud.Spec.Topology.Subsets)].Name
			} else {
				key, err = cf.GetString()
				if err != nil {
					return err
				}
			}
		} else {
			key, err = cf.GetString()
			if err != nil {
				return err
			}
		}
		partitions[key] = int32(j)
	}

	ud.Spec.UpdateStrategy.ManualUpdate = &appsv1alpha1.ManualUpdate{
		Partitions: partitions,
	}
	return nil
}

func GenerateUnitedDeploymentSubsetName(cf *fuzz.ConsumeFuzzer, subset *appsv1alpha1.Subset) error {
	name, err := cf.GetString()
	if err != nil {
		return err
	}
	subset.Name = name
	return nil
}

func GenerateUnitedDeploymentSubSetReplicas(cf *fuzz.ConsumeFuzzer, subset *appsv1alpha1.Subset) error {
	if setMin, err := cf.GetBool(); setMin && err == nil {
		minVal, err := GenerateSubsetReplicas(cf)
		if err != nil {
			return err
		}
		subset.MinReplicas = &minVal
	}

	if setMax, err := cf.GetBool(); setMax && err == nil {
		maxVal, err := GenerateSubsetReplicas(cf)
		if err != nil {
			return err
		}
		subset.MaxReplicas = &maxVal
	}

	if setReplicas, err := cf.GetBool(); setReplicas && err == nil {
		replicas, err := GenerateSubsetReplicas(cf)
		if err != nil {
			return err
		}
		subset.Replicas = &replicas
	}

	return nil
}

func GenerateUnitedDeploymentPatch(cf *fuzz.ConsumeFuzzer, subset *appsv1alpha1.Subset) error {
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

func GenerateUnitedDeploymentNodeSelectorTerm(cf *fuzz.ConsumeFuzzer, subset *appsv1alpha1.Subset) error {
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
	subset.NodeSelectorTerm = term
	return nil
}

func GenerateUnitedDeploymentTolerations(cf *fuzz.ConsumeFuzzer, subset *appsv1alpha1.Subset) error {
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
