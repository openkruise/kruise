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
	"math/rand"

	fuzz "github.com/AdaLogics/go-fuzz-headers"
	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
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

func GenerateUnitedDeploymentSelector(cf *fuzz.ConsumeFuzzer, ud *appsv1alpha1.UnitedDeployment) error {
	selector := &metav1.LabelSelector{}
	if err := GenerateLabelSelector(cf, selector); err != nil {
		return err
	}
	ud.Spec.Selector = selector
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
	rawExtension := &runtime.RawExtension{}
	if err := GeneratePatch(cf, rawExtension); err != nil {
		return err
	}
	subset.Patch = *rawExtension
	return nil
}

func GenerateUnitedDeploymentNodeSelectorTerm(cf *fuzz.ConsumeFuzzer, subset *appsv1alpha1.Subset) error {
	term := corev1.NodeSelectorTerm{}
	if err := GenerateNodeSelectorTerm(cf, &term); err != nil {
		return err
	}
	subset.NodeSelectorTerm = term
	return nil
}

func GenerateUnitedDeploymentTolerations(cf *fuzz.ConsumeFuzzer, subset *appsv1alpha1.Subset) error {
	tolerations := make([]corev1.Toleration, rand.Intn(2)+1)
	for i := range tolerations {
		if err := GenerateTolerations(cf, &tolerations[i]); err != nil {
			return err
		}
	}
	subset.Tolerations = tolerations
	return nil
}
