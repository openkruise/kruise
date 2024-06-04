/*
Copyright 2020 The Kruise Authors.

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

package convertor

import (
	"strconv"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/util/validation"
	"k8s.io/kubernetes/pkg/apis/core"
	corev1 "k8s.io/kubernetes/pkg/apis/core/v1"

	"github.com/openkruise/kruise/apis/apps/defaults"
)

func ConvertPodTemplateSpec(template *v1.PodTemplateSpec) (*core.PodTemplateSpec, error) {
	coreTemplate := &core.PodTemplateSpec{}
	defaults.SetDefaultPodSpec(&template.Spec)
	if err := corev1.Convert_v1_PodTemplateSpec_To_core_PodTemplateSpec(template.DeepCopy(), coreTemplate, nil); err != nil {
		return nil, err
	}
	return coreTemplate, nil
}

func ConvertPod(pod *v1.Pod) (*core.Pod, error) {
	corePod := &core.Pod{}
	defaults.SetDefaultPodSpec(&pod.Spec)
	if err := corev1.Convert_v1_Pod_To_core_Pod(pod.DeepCopy(), corePod, nil); err != nil {
		return nil, err
	}
	return corePod, nil
}

func ConvertCoreVolumes(volumes []v1.Volume) ([]core.Volume, error) {
	coreVolumes := []core.Volume{}
	for _, volume := range volumes {
		coreVolume := core.Volume{}
		if err := corev1.Convert_v1_Volume_To_core_Volume(&volume, &coreVolume, nil); err != nil {
			return nil, err
		}
		coreVolumes = append(coreVolumes, coreVolume)
	}
	return coreVolumes, nil
}

func ConvertEphemeralContainer(ecs []v1.EphemeralContainer) ([]core.EphemeralContainer, error) {
	coreEphemeralContainers := []core.EphemeralContainer{}
	for _, ec := range ecs {
		coreEC := core.EphemeralContainer{}
		if err := corev1.Convert_v1_EphemeralContainer_To_core_EphemeralContainer(&ec, &coreEC, nil); err != nil {
			return nil, err
		}
		coreEphemeralContainers = append(coreEphemeralContainers, coreEC)
	}
	return coreEphemeralContainers, nil
}

func GetPercentValue(intOrStringValue intstr.IntOrString) (int, bool) {
	if intOrStringValue.Type != intstr.String {
		return 0, false
	}
	if len(validation.IsValidPercent(intOrStringValue.StrVal)) != 0 {
		return 0, false
	}
	value, _ := strconv.Atoi(intOrStringValue.StrVal[:len(intOrStringValue.StrVal)-1])
	return value, true
}

func GetIntOrPercentValue(intOrStringValue intstr.IntOrString) int {
	value, isPercent := GetPercentValue(intOrStringValue)
	if isPercent {
		return value
	}
	return intOrStringValue.IntValue()
}
