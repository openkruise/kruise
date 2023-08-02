/*
Copyright 2023 The Kruise Authors.

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
	"testing"

	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/util/validation/field"
)

var (
	ecDemo = v1.EphemeralContainer{
		EphemeralContainerCommon: v1.EphemeralContainerCommon{
			Name:    "ec-1",
			Image:   "busybox:1.32",
			Command: []string{"/bin/sh"},
			Args:    []string{"-c", "sleep 100d"},
			Env: []v1.EnvVar{
				{Name: "key_1", Value: "value_1"},
				{Name: "key_2", Value: "value_2"},
			},
			VolumeMounts: []v1.VolumeMount{
				{
					Name:      "vol-1",
					MountPath: "/home/logs",
				},
			},
		},
		TargetContainerName: "main",
	}
)

func TestValidateEphemeralJobSpec(t *testing.T) {
	sucessfulCases := map[string]func() *appsv1alpha1.EphemeralJobSpec{
		"normal case": func() *appsv1alpha1.EphemeralJobSpec {
			ec := ecDemo.DeepCopy()
			return &appsv1alpha1.EphemeralJobSpec{Template: appsv1alpha1.EphemeralContainerTemplateSpec{EphemeralContainers: []v1.EphemeralContainer{*ec}}}
		},
		"without comm": func() *appsv1alpha1.EphemeralJobSpec {
			ec := ecDemo.DeepCopy()
			ec.Command = nil
			return &appsv1alpha1.EphemeralJobSpec{Template: appsv1alpha1.EphemeralContainerTemplateSpec{EphemeralContainers: []v1.EphemeralContainer{*ec}}}
		},
		"without args": func() *appsv1alpha1.EphemeralJobSpec {
			ec := ecDemo.DeepCopy()
			ec.Args = nil
			return &appsv1alpha1.EphemeralJobSpec{Template: appsv1alpha1.EphemeralContainerTemplateSpec{EphemeralContainers: []v1.EphemeralContainer{*ec}}}
		},
		"without env": func() *appsv1alpha1.EphemeralJobSpec {
			ec := ecDemo.DeepCopy()
			ec.Env = nil
			return &appsv1alpha1.EphemeralJobSpec{Template: appsv1alpha1.EphemeralContainerTemplateSpec{EphemeralContainers: []v1.EphemeralContainer{*ec}}}
		},
		"without volumeMounts": func() *appsv1alpha1.EphemeralJobSpec {
			ec := ecDemo.DeepCopy()
			ec.VolumeMounts = nil
			return &appsv1alpha1.EphemeralJobSpec{Template: appsv1alpha1.EphemeralContainerTemplateSpec{EphemeralContainers: []v1.EphemeralContainer{*ec}}}
		},
		"without targetContainer": func() *appsv1alpha1.EphemeralJobSpec {
			ec := ecDemo.DeepCopy()
			ec.TargetContainerName = ""
			return &appsv1alpha1.EphemeralJobSpec{Template: appsv1alpha1.EphemeralContainerTemplateSpec{EphemeralContainers: []v1.EphemeralContainer{*ec}}}
		},
	}

	for name, cs := range sucessfulCases {
		t.Run(name, func(t *testing.T) {
			allErrs := validateEphemeralJobSpec(cs(), field.NewPath("spec"))
			if len(allErrs) != 0 {
				t.Fatalf("got unexpected error: %v", allErrs.ToAggregate())
			}
		})
	}

	failedCases := map[string]func() *appsv1alpha1.EphemeralJobSpec{
		"without image": func() *appsv1alpha1.EphemeralJobSpec {
			ec := ecDemo.DeepCopy()
			ec.Image = ""
			return &appsv1alpha1.EphemeralJobSpec{Template: appsv1alpha1.EphemeralContainerTemplateSpec{EphemeralContainers: []v1.EphemeralContainer{*ec}}}
		},
		"without name": func() *appsv1alpha1.EphemeralJobSpec {
			ec := ecDemo.DeepCopy()
			ec.Name = ""
			return &appsv1alpha1.EphemeralJobSpec{Template: appsv1alpha1.EphemeralContainerTemplateSpec{EphemeralContainers: []v1.EphemeralContainer{*ec}}}
		},
		"with ports": func() *appsv1alpha1.EphemeralJobSpec {
			ec := ecDemo.DeepCopy()
			ec.Ports = []v1.ContainerPort{{Name: "web", ContainerPort: 80, Protocol: v1.ProtocolTCP}}
			return &appsv1alpha1.EphemeralJobSpec{Template: appsv1alpha1.EphemeralContainerTemplateSpec{EphemeralContainers: []v1.EphemeralContainer{*ec}}}
		},
		"with resources": func() *appsv1alpha1.EphemeralJobSpec {
			ec := ecDemo.DeepCopy()
			ec.Resources = v1.ResourceRequirements{Limits: v1.ResourceList{"cpu": *resource.NewQuantity(100000, resource.DecimalSI)}}
			return &appsv1alpha1.EphemeralJobSpec{Template: appsv1alpha1.EphemeralContainerTemplateSpec{EphemeralContainers: []v1.EphemeralContainer{*ec}}}
		},
		"with subPath": func() *appsv1alpha1.EphemeralJobSpec {
			ec := ecDemo.DeepCopy()
			ec.VolumeMounts[0].SubPath = "sub_path"
			return &appsv1alpha1.EphemeralJobSpec{Template: appsv1alpha1.EphemeralContainerTemplateSpec{EphemeralContainers: []v1.EphemeralContainer{*ec}}}
		},
		"with probe": func() *appsv1alpha1.EphemeralJobSpec {
			ec := ecDemo.DeepCopy()
			ec.ReadinessProbe = &v1.Probe{
				Handler: v1.Handler{
					Exec: &v1.ExecAction{
						Command: []string{"/bin/sh"},
					},
				},
				InitialDelaySeconds: 10,
				FailureThreshold:    3,
				SuccessThreshold:    1,
				TimeoutSeconds:      10,
				PeriodSeconds:       10,
			}
			return &appsv1alpha1.EphemeralJobSpec{Template: appsv1alpha1.EphemeralContainerTemplateSpec{EphemeralContainers: []v1.EphemeralContainer{*ec}}}
		},
	}

	for name, cs := range failedCases {
		t.Run(name, func(t *testing.T) {
			allErrs := validateEphemeralJobSpec(cs(), field.NewPath("spec"))
			if len(allErrs) != 1 {
				t.Fatalf("got unexpected error: %v", allErrs.ToAggregate())
			}
		})
	}
}
