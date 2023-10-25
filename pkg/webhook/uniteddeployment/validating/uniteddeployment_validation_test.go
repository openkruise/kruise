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
	"fmt"
	"strconv"
	"strings"
	"testing"

	apps "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"k8s.io/utils/pointer"

	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	appsv1beta1 "github.com/openkruise/kruise/apis/apps/v1beta1"
)

func TestValidateUnitedDeployment(t *testing.T) {
	validLabels := map[string]string{"a": "b"}
	validPodTemplate := corev1.PodTemplate{
		Template: corev1.PodTemplateSpec{
			ObjectMeta: metav1.ObjectMeta{
				Labels: validLabels,
			},
			Spec: corev1.PodSpec{
				RestartPolicy: corev1.RestartPolicyAlways,
				DNSPolicy:     corev1.DNSClusterFirst,
				Containers:    []corev1.Container{{Name: "abc", Image: "image", ImagePullPolicy: "IfNotPresent", TerminationMessagePolicy: "File"}},
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
							Replicas: &replicas1,
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
		{
			ObjectMeta: metav1.ObjectMeta{Name: "abc", Namespace: metav1.NamespaceDefault},
			Spec: appsv1alpha1.UnitedDeploymentSpec{
				Replicas: &val,
				Selector: &metav1.LabelSelector{MatchLabels: validLabels},
				Template: appsv1alpha1.SubsetTemplate{
					DeploymentTemplate: &appsv1alpha1.DeploymentTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Labels: validLabels,
						},
						Spec: apps.DeploymentSpec{
							Template: validPodTemplate.Template,
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
							Template: corev1.PodTemplateSpec{
								ObjectMeta: metav1.ObjectMeta{},
								Spec: corev1.PodSpec{
									RestartPolicy: corev1.RestartPolicyAlways,
									DNSPolicy:     corev1.DNSClusterFirst,
									Containers:    []corev1.Container{{Name: "abc", Image: "image", ImagePullPolicy: "IfNotPresent"}},
								},
							},
						},
					},
				},
			},
		},
		"deployment no pod template label": {
			ObjectMeta: metav1.ObjectMeta{Name: "abc", Namespace: metav1.NamespaceDefault},
			Spec: appsv1alpha1.UnitedDeploymentSpec{
				Replicas: &val,
				Selector: &metav1.LabelSelector{MatchLabels: validLabels},
				Template: appsv1alpha1.SubsetTemplate{
					DeploymentTemplate: &appsv1alpha1.DeploymentTemplateSpec{
						Spec: apps.DeploymentSpec{
							Template: corev1.PodTemplateSpec{
								ObjectMeta: metav1.ObjectMeta{},
								Spec: corev1.PodSpec{
									RestartPolicy: corev1.RestartPolicyAlways,
									DNSPolicy:     corev1.DNSClusterFirst,
									Containers:    []corev1.Container{{Name: "abc", Image: "image", ImagePullPolicy: "IfNotPresent"}},
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
		"invalid subset nodeSelectorTerm": {
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
							NodeSelectorTerm: corev1.NodeSelectorTerm{
								MatchExpressions: []corev1.NodeSelectorRequirement{
									{
										Key:      "key",
										Operator: corev1.NodeSelectorOpExists,
										Values:   []string{"unexpected"},
									},
								},
							},
						},
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
		"deployment subset replicas is too much": {
			ObjectMeta: metav1.ObjectMeta{Name: "abc", Namespace: metav1.NamespaceDefault},
			Spec: appsv1alpha1.UnitedDeploymentSpec{
				Replicas: &val,
				Selector: &metav1.LabelSelector{MatchLabels: validLabels},
				Template: appsv1alpha1.SubsetTemplate{
					DeploymentTemplate: &appsv1alpha1.DeploymentTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Labels: validLabels,
						},
						Spec: apps.DeploymentSpec{
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
		"subset replicas type is percent when spec replicas not set": {
			ObjectMeta: metav1.ObjectMeta{Name: "abc", Namespace: metav1.NamespaceDefault},
			Spec: appsv1alpha1.UnitedDeploymentSpec{
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
							Replicas: &replicas2,
						},
						{
							Name:     "subset2",
							Replicas: &replicas1,
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
				UpdateStrategy: appsv1alpha1.UnitedDeploymentUpdateStrategy{
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
		"duplicated templates": {
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
					AdvancedStatefulSetTemplate: &appsv1alpha1.AdvancedStatefulSetTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Labels: validLabels,
						},
						Spec: appsv1beta1.StatefulSetSpec{
							Template: validPodTemplate.Template,
						},
					},
				},
			},
		},
		"triple duplicated templates": {
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
					AdvancedStatefulSetTemplate: &appsv1alpha1.AdvancedStatefulSetTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Labels: validLabels,
						},
						Spec: appsv1beta1.StatefulSetSpec{
							Template: validPodTemplate.Template,
						},
					},
					DeploymentTemplate: &appsv1alpha1.DeploymentTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Labels: validLabels,
						},
						Spec: apps.DeploymentSpec{
							Template: validPodTemplate.Template,
						},
					},
				},
			},
		},
		"deployment duplicated templates": {
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
					DeploymentTemplate: &appsv1alpha1.DeploymentTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Labels: validLabels,
						},
						Spec: apps.DeploymentSpec{
							Template: validPodTemplate.Template,
						},
					},
				},
			},
		},
		"deployment no pod template termination policy": {
			ObjectMeta: metav1.ObjectMeta{Name: "abc", Namespace: metav1.NamespaceDefault},
			Spec: appsv1alpha1.UnitedDeploymentSpec{
				Replicas: &val,
				Selector: &metav1.LabelSelector{MatchLabels: validLabels},
				Template: appsv1alpha1.SubsetTemplate{
					DeploymentTemplate: &appsv1alpha1.DeploymentTemplateSpec{
						Spec: apps.DeploymentSpec{
							Template: corev1.PodTemplateSpec{
								ObjectMeta: metav1.ObjectMeta{
									Labels: validLabels,
								},
								Spec: corev1.PodSpec{
									RestartPolicy: corev1.RestartPolicyAlways,
									DNSPolicy:     corev1.DNSClusterFirst,
									Containers:    []corev1.Container{{Name: "abc", Image: "image", ImagePullPolicy: "IfNotPresent"}},
								},
							},
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
				if !strings.HasPrefix(field, "spec.template") &&
					field != "spec.selector" &&
					field != "spec.topology.subsets" &&
					field != "spec.topology.subsets[0]" &&
					field != "spec.topology.subsets[0].name" &&
					field != "spec.topology.subsets[0].replicas" &&
					field != "spec.updateStrategy.partitions" &&
					field != "spec.topology.subsets[0].nodeSelectorTerm.matchExpressions[0].values" {
					t.Errorf("%s: missing prefix for: %v", k, errs[i])
				}
			}
		})
	}
}

type UpdateCase struct {
	Old appsv1alpha1.UnitedDeployment
	New appsv1alpha1.UnitedDeployment
}

func TestValidateUnitedDeploymentUpdate(t *testing.T) {
	validLabels := map[string]string{"a": "b"}
	validPodTemplate := corev1.PodTemplate{
		Template: corev1.PodTemplateSpec{
			ObjectMeta: metav1.ObjectMeta{
				Labels: validLabels,
			},
			Spec: corev1.PodSpec{
				RestartPolicy: corev1.RestartPolicyAlways,
				DNSPolicy:     corev1.DNSClusterFirst,
				Containers:    []corev1.Container{{Name: "abc", Image: "image", ImagePullPolicy: "IfNotPresent"}},
			},
		},
	}

	var val int32 = 10
	successCases := []UpdateCase{
		{
			Old: appsv1alpha1.UnitedDeployment{
				ObjectMeta: metav1.ObjectMeta{Name: "abc", Namespace: metav1.NamespaceDefault, ResourceVersion: "1"},
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
								Name: "subset-a",
								NodeSelectorTerm: corev1.NodeSelectorTerm{
									MatchExpressions: []corev1.NodeSelectorRequirement{
										{
											Key:      "domain",
											Operator: corev1.NodeSelectorOpIn,
											Values:   []string{"a"},
										},
									},
								},
							},
						},
					},
				},
			},
			New: appsv1alpha1.UnitedDeployment{
				ObjectMeta: metav1.ObjectMeta{Name: "abc", Namespace: metav1.NamespaceDefault, ResourceVersion: "1"},
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
								Name: "subset-a",
								NodeSelectorTerm: corev1.NodeSelectorTerm{
									MatchExpressions: []corev1.NodeSelectorRequirement{
										{
											Key:      "domain",
											Operator: corev1.NodeSelectorOpIn,
											Values:   []string{"a"},
										},
									},
								},
							},
							{
								Name: "subset-b",
								NodeSelectorTerm: corev1.NodeSelectorTerm{
									MatchExpressions: []corev1.NodeSelectorRequirement{
										{
											Key:      "domain",
											Operator: corev1.NodeSelectorOpIn,
											Values:   []string{"b"},
										},
									},
								},
							},
						},
					},
				},
			},
		},
		{
			Old: appsv1alpha1.UnitedDeployment{
				ObjectMeta: metav1.ObjectMeta{Name: "abc", Namespace: metav1.NamespaceDefault, ResourceVersion: "1"},
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
								Name: "subset-a",
								NodeSelectorTerm: corev1.NodeSelectorTerm{
									MatchExpressions: []corev1.NodeSelectorRequirement{
										{
											Key:      "domain",
											Operator: corev1.NodeSelectorOpIn,
											Values:   []string{"a"},
										},
									},
								},
							},
							{
								Name: "subset-b",
								NodeSelectorTerm: corev1.NodeSelectorTerm{
									MatchExpressions: []corev1.NodeSelectorRequirement{
										{
											Key:      "domain",
											Operator: corev1.NodeSelectorOpIn,
											Values:   []string{"b"},
										},
									},
								},
							},
						},
					},
				},
			},
			New: appsv1alpha1.UnitedDeployment{
				ObjectMeta: metav1.ObjectMeta{Name: "abc", Namespace: metav1.NamespaceDefault, ResourceVersion: "1"},
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
								Name: "subset-a",
								NodeSelectorTerm: corev1.NodeSelectorTerm{
									MatchExpressions: []corev1.NodeSelectorRequirement{
										{
											Key:      "domain",
											Operator: corev1.NodeSelectorOpIn,
											Values:   []string{"a"},
										},
									},
								},
							},
						},
					},
				},
			},
		},
		{
			Old: appsv1alpha1.UnitedDeployment{
				ObjectMeta: metav1.ObjectMeta{Name: "abc", Namespace: metav1.NamespaceDefault, ResourceVersion: "1"},
				Spec: appsv1alpha1.UnitedDeploymentSpec{
					Replicas: &val,
					Selector: &metav1.LabelSelector{MatchLabels: validLabels},
					Template: appsv1alpha1.SubsetTemplate{
						DeploymentTemplate: &appsv1alpha1.DeploymentTemplateSpec{
							ObjectMeta: metav1.ObjectMeta{
								Labels: validLabels,
							},
							Spec: apps.DeploymentSpec{
								Template: validPodTemplate.Template,
							},
						},
					},
					Topology: appsv1alpha1.Topology{
						Subsets: []appsv1alpha1.Subset{
							{
								Name: "subset-a",
								NodeSelectorTerm: corev1.NodeSelectorTerm{
									MatchExpressions: []corev1.NodeSelectorRequirement{
										{
											Key:      "domain",
											Operator: corev1.NodeSelectorOpIn,
											Values:   []string{"a"},
										},
									},
								},
							},
						},
					},
				},
			},
			New: appsv1alpha1.UnitedDeployment{
				ObjectMeta: metav1.ObjectMeta{Name: "abc", Namespace: metav1.NamespaceDefault, ResourceVersion: "1"},
				Spec: appsv1alpha1.UnitedDeploymentSpec{
					Replicas: &val,
					Selector: &metav1.LabelSelector{MatchLabels: validLabels},
					Template: appsv1alpha1.SubsetTemplate{
						DeploymentTemplate: &appsv1alpha1.DeploymentTemplateSpec{
							ObjectMeta: metav1.ObjectMeta{
								Labels: validLabels,
							},
							Spec: apps.DeploymentSpec{
								Template: validPodTemplate.Template,
							},
						},
					},
					Topology: appsv1alpha1.Topology{
						Subsets: []appsv1alpha1.Subset{
							{
								Name: "subset-a",
								NodeSelectorTerm: corev1.NodeSelectorTerm{
									MatchExpressions: []corev1.NodeSelectorRequirement{
										{
											Key:      "domain",
											Operator: corev1.NodeSelectorOpIn,
											Values:   []string{"a"},
										},
									},
								},
							},
							{
								Name: "subset-b",
								NodeSelectorTerm: corev1.NodeSelectorTerm{
									MatchExpressions: []corev1.NodeSelectorRequirement{
										{
											Key:      "domain",
											Operator: corev1.NodeSelectorOpIn,
											Values:   []string{"b"},
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}

	for i, successCase := range successCases {
		t.Run("success case "+strconv.Itoa(i), func(t *testing.T) {
			setTestDefault(&successCase.Old)
			setTestDefault(&successCase.New)
			if errs := ValidateUnitedDeploymentUpdate(&successCase.Old, &successCase.New); len(errs) != 0 {
				t.Errorf("expected success: %v", errs)
			}
		})
	}

	errorCases := map[string]UpdateCase{
		"subset nodeSelector changed": {
			Old: appsv1alpha1.UnitedDeployment{
				ObjectMeta: metav1.ObjectMeta{Name: "abc", Namespace: metav1.NamespaceDefault, ResourceVersion: "1"},
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
								Name: "subset-a",
								NodeSelectorTerm: corev1.NodeSelectorTerm{
									MatchExpressions: []corev1.NodeSelectorRequirement{
										{
											Key:      "domain",
											Operator: corev1.NodeSelectorOpIn,
											Values:   []string{"a", "b"},
										},
									},
								},
							},
						},
					},
				},
			},
			New: appsv1alpha1.UnitedDeployment{
				ObjectMeta: metav1.ObjectMeta{Name: "abc", Namespace: metav1.NamespaceDefault, ResourceVersion: "1"},
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
								Name: "subset-a",
								NodeSelectorTerm: corev1.NodeSelectorTerm{
									MatchExpressions: []corev1.NodeSelectorRequirement{
										{
											Key:      "domain",
											Operator: corev1.NodeSelectorOpIn,
											Values:   []string{"a"},
										},
									},
								},
							},
							{
								Name: "subset-b",
								NodeSelectorTerm: corev1.NodeSelectorTerm{
									MatchExpressions: []corev1.NodeSelectorRequirement{
										{
											Key:      "domain",
											Operator: corev1.NodeSelectorOpIn,
											Values:   []string{"b"},
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}

	for k, v := range errorCases {
		t.Run(k, func(t *testing.T) {
			setTestDefault(&v.Old)
			setTestDefault(&v.New)
			errs := ValidateUnitedDeploymentUpdate(&v.Old, &v.New)
			if len(errs) == 0 {
				t.Errorf("expected failure for %s", k)
			}

			for i := range errs {
				field := errs[i].Field
				if !strings.HasPrefix(field, "spec.template.") &&
					field != "spec.selector" &&
					field != "spec.topology.subset" &&
					field != "spec.topology.subset.name" &&
					field != "spec.updateStrategy.partitions" &&
					field != "spec.topology.subsets[0].nodeSelectorTerm" {
					t.Errorf("%s: missing prefix for: %v", k, errs[i])
				}
			}
		})
	}
}

func Test(t *testing.T) {
	cases := []struct {
		name        string
		replicas    int32
		minReplicas []int32
		maxReplicas []int32
		errorHappen bool
	}{
		{
			name:     "sum_all_min_replicas == replicas",
			replicas: 10,
			minReplicas: []int32{
				2, 2, 2, 2, 2,
			},
			maxReplicas: []int32{
				5, 5, 5, 5, -1,
			},
			errorHappen: false,
		},
		{
			name:     "sum_all_min_replicas < replicas",
			replicas: 14,
			minReplicas: []int32{
				2, 2, 2, 2, 2,
			},
			maxReplicas: []int32{
				5, 5, 5, 5, -1,
			},
			errorHappen: false,
		},
		{
			name:     "sum_all_min_replicas > replicas",
			replicas: 5,
			minReplicas: []int32{
				2, 2, 2, 2, 2,
			},
			maxReplicas: []int32{
				5, 5, 5, 5, -1,
			},
			errorHappen: false,
		},
		{
			name:     "min_replicas > max_replicas",
			replicas: 5,
			minReplicas: []int32{
				2, 2, 2, 6, 2,
			},
			maxReplicas: []int32{
				5, 5, 5, 5, -1,
			},
			errorHappen: true,
		},
		{
			name:     "all_max_replicas != nil",
			replicas: 5,
			minReplicas: []int32{
				2, 2, 2, 2, 2,
			},
			maxReplicas: []int32{
				5, 5, 5, 5, 5,
			},
			errorHappen: true,
		},
	}

	for _, cs := range cases {
		t.Run(cs.name, func(t *testing.T) {
			ud := appsv1alpha1.UnitedDeployment{}
			ud.Spec.Replicas = pointer.Int32(cs.replicas)
			ud.Spec.Topology.Subsets = []appsv1alpha1.Subset{}
			for index := range cs.minReplicas {
				min := intstr.FromInt(int(cs.minReplicas[index]))
				var max *intstr.IntOrString
				if cs.maxReplicas[index] != -1 {
					m := intstr.FromInt(int(cs.maxReplicas[index]))
					max = &m
				}
				ud.Spec.Topology.Subsets = append(ud.Spec.Topology.Subsets, appsv1alpha1.Subset{
					Name:        fmt.Sprintf("subset-%d", index),
					MinReplicas: &min,
					MaxReplicas: max,
				})
			}
			errList := validateSubsetReplicas(&cs.replicas, ud.Spec.Topology.Subsets, field.NewPath("subset"))
			if len(errList) > 0 && !cs.errorHappen {
				t.Errorf("expected success, but got error: %v", errList)
			} else if len(errList) == 0 && cs.errorHappen {
				t.Errorf("expected error, but got success")
			}
		})
	}
}

func setTestDefault(obj *appsv1alpha1.UnitedDeployment) {
	if obj.Spec.RevisionHistoryLimit == nil {
		obj.Spec.RevisionHistoryLimit = new(int32)
		*obj.Spec.RevisionHistoryLimit = 10
	}
}
