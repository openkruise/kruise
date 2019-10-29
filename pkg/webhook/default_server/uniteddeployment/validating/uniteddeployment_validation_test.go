/*
Copyright 2019 The Kruise Authors.

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
	"strconv"
	"strings"
	"testing"

	appsv1alpha1 "github.com/openkruise/kruise/pkg/apis/apps/v1alpha1"
	apps "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

func TestValidateUnitedDeployment(t *testing.T) {
	validLabels := map[string]string{"a": "b"}
	validPodTemplate := v1.PodTemplate{
		Template: v1.PodTemplateSpec{
			ObjectMeta: metav1.ObjectMeta{
				Labels: validLabels,
			},
			Spec: v1.PodSpec{
				RestartPolicy: v1.RestartPolicyAlways,
				DNSPolicy:     v1.DNSClusterFirst,
				Containers:    []v1.Container{{Name: "abc", Image: "image", ImagePullPolicy: "IfNotPresent"}},
			},
		},
	}

	var val int32 = 10
	replicas1 := intstr.FromInt(1)
	replicas2 := intstr.FromString("90%")
	replicas3 := intstr.FromString("71%")
	replicas4 := intstr.FromString("29%")
	successCases := []appsv1alpha1.UnitedDeployment{
		{
			ObjectMeta: metav1.ObjectMeta{Name: "abc", Namespace: metav1.NamespaceDefault},
			Spec: appsv1alpha1.UnitedDeploymentSpec{
				Replicas: &val,
				Selector: &metav1.LabelSelector{MatchLabels: validLabels},
				Template: appsv1alpha1.SubsetTemplate{
					StatefulSetTemplate: &appsv1alpha1.StatefulSetTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Labels: validLabels,
						},
						Spec: apps.StatefulSetSpec{
							Template: validPodTemplate.Template,
						},
					},
				},
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{Name: "abc", Namespace: metav1.NamespaceDefault},
			Spec: appsv1alpha1.UnitedDeploymentSpec{
				Replicas: &val,
				Selector: &metav1.LabelSelector{MatchLabels: validLabels},
				Template: appsv1alpha1.SubsetTemplate{
					StatefulSetTemplate: &appsv1alpha1.StatefulSetTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Labels: validLabels,
						},
						Spec: apps.StatefulSetSpec{
							Template: validPodTemplate.Template,
						},
					},
				},
				Topology: appsv1alpha1.Topology{
					Subsets: []appsv1alpha1.Subset{
						{
							Name: "subset",
						},
					},
				},
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{Name: "abc", Namespace: metav1.NamespaceDefault},
			Spec: appsv1alpha1.UnitedDeploymentSpec{
				Replicas: &val,
				Selector: &metav1.LabelSelector{MatchLabels: validLabels},
				Template: appsv1alpha1.SubsetTemplate{
					StatefulSetTemplate: &appsv1alpha1.StatefulSetTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Labels: validLabels,
						},
						Spec: apps.StatefulSetSpec{
							Template: validPodTemplate.Template,
						},
					},
				},
				Topology: appsv1alpha1.Topology{
					Subsets: []appsv1alpha1.Subset{
						{
							Name:     "subset1",
							Replicas: &replicas1,
						},
						{
							Name: "subset2",
						},
					},
				},
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{Name: "abc", Namespace: metav1.NamespaceDefault},
			Spec: appsv1alpha1.UnitedDeploymentSpec{
				Replicas: &val,
				Selector: &metav1.LabelSelector{MatchLabels: validLabels},
				Template: appsv1alpha1.SubsetTemplate{
					StatefulSetTemplate: &appsv1alpha1.StatefulSetTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Labels: validLabels,
						},
						Spec: apps.StatefulSetSpec{
							Template: validPodTemplate.Template,
						},
					},
				},
				Topology: appsv1alpha1.Topology{
					Subsets: []appsv1alpha1.Subset{
						{
							Name:     "subset1",
							Replicas: &replicas1,
						},
						{
							Name:     "subset2",
							Replicas: &replicas2,
						},
					},
				},
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{Name: "abc", Namespace: metav1.NamespaceDefault},
			Spec: appsv1alpha1.UnitedDeploymentSpec{
				Replicas: &val,
				Selector: &metav1.LabelSelector{MatchLabels: validLabels},
				Template: appsv1alpha1.SubsetTemplate{
					StatefulSetTemplate: &appsv1alpha1.StatefulSetTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Labels: validLabels,
						},
						Spec: apps.StatefulSetSpec{
							Template: validPodTemplate.Template,
						},
					},
				},
				Topology: appsv1alpha1.Topology{
					Subsets: []appsv1alpha1.Subset{
						{
							Name:     "subset1",
							Replicas: &replicas3,
						},
						{
							Name:     "subset2",
							Replicas: &replicas4,
						},
					},
				},
			},
		},
	}

	for i, successCase := range successCases {
		t.Run("success case "+strconv.Itoa(i), func(t *testing.T) {
			setTestDefault(&successCase)
			if errs := validateUnitedDeployment(&successCase); len(errs) != 0 {
				t.Errorf("expected success: %v", errs)
			}
		})
	}

	errorCases := map[string]appsv1alpha1.UnitedDeployment{
		"no pod template label": {
			ObjectMeta: metav1.ObjectMeta{Name: "abc", Namespace: metav1.NamespaceDefault},
			Spec: appsv1alpha1.UnitedDeploymentSpec{
				Replicas: &val,
				Selector: &metav1.LabelSelector{MatchLabels: validLabels},
				Template: appsv1alpha1.SubsetTemplate{
					StatefulSetTemplate: &appsv1alpha1.StatefulSetTemplateSpec{
						Spec: apps.StatefulSetSpec{
							Template: v1.PodTemplateSpec{
								ObjectMeta: metav1.ObjectMeta{},
								Spec: v1.PodSpec{
									RestartPolicy: v1.RestartPolicyAlways,
									DNSPolicy:     v1.DNSClusterFirst,
									Containers:    []v1.Container{{Name: "abc", Image: "image", ImagePullPolicy: "IfNotPresent"}},
								},
							},
						},
					},
				},
			},
		},
		"no subset template": {
			ObjectMeta: metav1.ObjectMeta{Name: "abc", Namespace: metav1.NamespaceDefault},
			Spec: appsv1alpha1.UnitedDeploymentSpec{
				Replicas: &val,
				Selector: &metav1.LabelSelector{MatchLabels: validLabels},
				Template: appsv1alpha1.SubsetTemplate{},
			},
		},
		"no subset name": {
			ObjectMeta: metav1.ObjectMeta{Name: "abc", Namespace: metav1.NamespaceDefault},
			Spec: appsv1alpha1.UnitedDeploymentSpec{
				Replicas: &val,
				Selector: &metav1.LabelSelector{MatchLabels: validLabels},
				Template: appsv1alpha1.SubsetTemplate{
					StatefulSetTemplate: &appsv1alpha1.StatefulSetTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Labels: validLabels,
						},
						Spec: apps.StatefulSetSpec{
							Template: validPodTemplate.Template,
						},
					},
				},
				Topology: appsv1alpha1.Topology{
					Subsets: []appsv1alpha1.Subset{
						{},
					},
				},
			},
		},
		"subset replicas is not enough": {
			ObjectMeta: metav1.ObjectMeta{Name: "abc", Namespace: metav1.NamespaceDefault},
			Spec: appsv1alpha1.UnitedDeploymentSpec{
				Replicas: &val,
				Selector: &metav1.LabelSelector{MatchLabels: validLabels},
				Template: appsv1alpha1.SubsetTemplate{
					StatefulSetTemplate: &appsv1alpha1.StatefulSetTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Labels: validLabels,
						},
						Spec: apps.StatefulSetSpec{
							Template: validPodTemplate.Template,
						},
					},
				},
				Topology: appsv1alpha1.Topology{
					Subsets: []appsv1alpha1.Subset{
						{
							Name:     "subset1",
							Replicas: &replicas1,
						},
					},
				},
			},
		},
		"subset replicas is too small": {
			ObjectMeta: metav1.ObjectMeta{Name: "abc", Namespace: metav1.NamespaceDefault},
			Spec: appsv1alpha1.UnitedDeploymentSpec{
				Replicas: &val,
				Selector: &metav1.LabelSelector{MatchLabels: validLabels},
				Template: appsv1alpha1.SubsetTemplate{
					StatefulSetTemplate: &appsv1alpha1.StatefulSetTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Labels: validLabels,
						},
						Spec: apps.StatefulSetSpec{
							Template: validPodTemplate.Template,
						},
					},
				},
				Topology: appsv1alpha1.Topology{
					Subsets: []appsv1alpha1.Subset{
						{
							Name:     "subset1",
							Replicas: &replicas1,
						},
						{
							Name:     "subset2",
							Replicas: &replicas3,
						},
					},
				},
			},
		},
		"subset replicas is too much": {
			ObjectMeta: metav1.ObjectMeta{Name: "abc", Namespace: metav1.NamespaceDefault},
			Spec: appsv1alpha1.UnitedDeploymentSpec{
				Replicas: &val,
				Selector: &metav1.LabelSelector{MatchLabels: validLabels},
				Template: appsv1alpha1.SubsetTemplate{
					StatefulSetTemplate: &appsv1alpha1.StatefulSetTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Labels: validLabels,
						},
						Spec: apps.StatefulSetSpec{
							Template: validPodTemplate.Template,
						},
					},
				},
				Topology: appsv1alpha1.Topology{
					Subsets: []appsv1alpha1.Subset{
						{
							Name:     "subset1",
							Replicas: &replicas3,
						},
						{
							Name:     "subset2",
							Replicas: &replicas2,
						},
					},
				},
			},
		},
		"partition not exist": {
			ObjectMeta: metav1.ObjectMeta{Name: "abc", Namespace: metav1.NamespaceDefault},
			Spec: appsv1alpha1.UnitedDeploymentSpec{
				Replicas: &val,
				Selector: &metav1.LabelSelector{MatchLabels: validLabels},
				Template: appsv1alpha1.SubsetTemplate{
					StatefulSetTemplate: &appsv1alpha1.StatefulSetTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Labels: validLabels,
						},
						Spec: apps.StatefulSetSpec{
							Template: validPodTemplate.Template,
						},
					},
				},
				Strategy: appsv1alpha1.UnitedDeploymentUpdateStrategy{
					ManualUpdate: &appsv1alpha1.ManualUpdate{
						Partitions: map[string]int32{
							"notExist": 1,
						},
					},
				},
				Topology: appsv1alpha1.Topology{
					Subsets: []appsv1alpha1.Subset{
						{
							Name:     "subset1",
							Replicas: &replicas3,
						},
						{
							Name:     "subset2",
							Replicas: &replicas2,
						},
					},
				},
			},
		},
	}

	for k, v := range errorCases {
		t.Run(k, func(t *testing.T) {
			setTestDefault(&v)
			errs := validateUnitedDeployment(&v)
			if len(errs) == 0 {
				t.Errorf("expected failure for %s", k)
			}

			for i := range errs {
				field := errs[i].Field
				if !strings.HasPrefix(field, "spec.template.") &&
					field != "spec.selector" &&
					field != "spec.topology.subset" &&
					field != "spec.topology.subset.name" &&
					field != "spec.strategy.partitions" {
					t.Errorf("%s: missing prefix for: %v", k, errs[i])
				}
			}
		})
	}
}

func setTestDefault(obj *appsv1alpha1.UnitedDeployment) {
	if obj.Spec.Replicas == nil {
		obj.Spec.Replicas = new(int32)
		*obj.Spec.Replicas = 0
	}
	if obj.Spec.RevisionHistoryLimit == nil {
		obj.Spec.RevisionHistoryLimit = new(int32)
		*obj.Spec.RevisionHistoryLimit = 0
	}
}
