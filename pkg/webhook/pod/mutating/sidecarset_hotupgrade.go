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

package mutating

import (
	"encoding/json"
	"fmt"

	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	"github.com/openkruise/kruise/pkg/control/sidecarcontrol"

	corev1 "k8s.io/api/core/v1"
)

func injectHotUpgradeContainers(hotUpgradeWorkInfo map[string]string, sidecarContainer *appsv1alpha1.SidecarContainer) (
	sidecarContainers []*appsv1alpha1.SidecarContainer, injectedAnnotations map[string]string) {

	injectedAnnotations = make(map[string]string)
	// container1 is current worked container
	// container2 is empty container, and don't work now
	container1, container2 := generateHotUpgradeContainers(sidecarContainer)
	sidecarContainers = append(sidecarContainers, container1)
	sidecarContainers = append(sidecarContainers, container2)
	//mark sidecarset.version in annotations
	// "1" indicates sidecar container is first injected into pod, and not upgrade process
	injectedAnnotations[sidecarcontrol.GetPodSidecarSetVersionAnnotation(container1.Name)] = "1"
	injectedAnnotations[sidecarcontrol.GetPodSidecarSetVersionAltAnnotation(container1.Name)] = "0"
	// "0" indicates sidecar container is hot upgrade empty container
	injectedAnnotations[sidecarcontrol.GetPodSidecarSetVersionAnnotation(container2.Name)] = "0"
	injectedAnnotations[sidecarcontrol.GetPodSidecarSetVersionAltAnnotation(container2.Name)] = "1"
	// used to mark which container is currently working, first is container1
	// format: map[container.name] = pod.spec.container[x].name
	hotUpgradeWorkInfo[sidecarContainer.Name] = container1.Name
	// store working HotUpgrade container in pod annotations
	by, _ := json.Marshal(hotUpgradeWorkInfo)
	injectedAnnotations[sidecarcontrol.SidecarSetWorkingHotUpgradeContainer] = string(by)

	return sidecarContainers, injectedAnnotations
}

func generateHotUpgradeContainers(container *appsv1alpha1.SidecarContainer) (*appsv1alpha1.SidecarContainer, *appsv1alpha1.SidecarContainer) {
	name1, name2 := sidecarcontrol.GetHotUpgradeContainerName(container.Name)
	container1, container2 := container.DeepCopy(), container.DeepCopy()
	container1.Name = name1
	container2.Name = name2
	// set the non-working hot upgrade container image to empty, first is container2
	container2.Container.Image = container.UpgradeStrategy.HotUpgradeEmptyImage
	// set sidecarset.version in container env
	setSidecarContainerVersionEnv(&container1.Container)
	setSidecarContainerVersionEnv(&container2.Container)
	return container1, container2
}

// use Sidecarset.ResourceVersion to mark sidecar container version in env(SIDECARSET_VERSION)
// env(SIDECARSET_VERSION) ValueFrom pod.metadata.annotations['sidecarset.kruise.io/{container.name}.version']
func setSidecarContainerVersionEnv(container *corev1.Container) {
	// inject SIDECARSET_VERSION
	container.Env = append(container.Env, corev1.EnvVar{
		Name: sidecarcontrol.SidecarSetVersionEnvKey,
		ValueFrom: &corev1.EnvVarSource{
			FieldRef: &corev1.ObjectFieldSelector{
				FieldPath: fmt.Sprintf("metadata.annotations['%s']", sidecarcontrol.GetPodSidecarSetVersionAnnotation(container.Name)),
			},
		},
	})
	// inject SIDECARSET_VERSION_ALT
	container.Env = append(container.Env, corev1.EnvVar{
		Name: sidecarcontrol.SidecarSetVersionAltEnvKey,
		ValueFrom: &corev1.EnvVarSource{
			FieldRef: &corev1.ObjectFieldSelector{
				FieldPath: fmt.Sprintf("metadata.annotations['%s']", sidecarcontrol.GetPodSidecarSetVersionAltAnnotation(container.Name)),
			},
		},
	})
}
