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

package sidecarcontrol

import (
	"encoding/json"
	"fmt"

	podutil "k8s.io/kubernetes/pkg/api/v1/pod"

	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/klog/v2"
)

const (
	// SidecarSetWorkingHotUpgradeContainer records which hot upgrade container is working currently
	SidecarSetWorkingHotUpgradeContainer = "kruise.io/sidecarset-working-hotupgrade-container"

	// hotUpgrade container name suffix
	hotUpgradeNameSuffix1 = "-1"
	hotUpgradeNameSuffix2 = "-2"

	// SidecarSetVersionEnvKey is sidecar container version in container env(SIDECARSET_VERSION)
	SidecarSetVersionEnvKey = "SIDECARSET_VERSION"
	// SidecarSetVersionAltEnvKey is container version env in the other sidecar container of the same hotupgrade sidecar(SIDECARSET_VERSION_ALT)
	SidecarSetVersionAltEnvKey = "SIDECARSET_VERSION_ALT"
)

// GetHotUpgradeContainerName returns format: mesh-1, mesh-2
func GetHotUpgradeContainerName(name string) (string, string) {
	return name + hotUpgradeNameSuffix1, name + hotUpgradeNameSuffix2
}

// GetPodSidecarSetVersionAnnotation is only used in hot upgrade container
// cName format: mesh-1, mesh-2
func GetPodSidecarSetVersionAnnotation(cName string) string {
	return fmt.Sprintf("version.sidecarset.kruise.io/%s", cName)
}

func GetPodSidecarSetVersionAltAnnotation(cName string) string {
	return fmt.Sprintf("versionalt.sidecarset.kruise.io/%s", cName)
}

// IsHotUpgradeContainer indicates whether sidecar container update strategy is HotUpdate
func IsHotUpgradeContainer(sidecarContainer *appsv1alpha1.SidecarContainer) bool {
	return sidecarContainer.UpgradeStrategy.UpgradeType == appsv1alpha1.SidecarContainerHotUpgrade
}

// GetPodHotUpgradeInfoInAnnotations checks which hot upgrade sidecar container is working now
// format: sidecarset.spec.container[x].name -> pod.spec.container[x].name
// for example: mesh -> mesh-1, envoy -> envoy-2
func GetPodHotUpgradeInfoInAnnotations(pod *corev1.Pod) map[string]string {
	hotUpgradeWorkContainer := make(map[string]string)
	currentStr, ok := pod.Annotations[SidecarSetWorkingHotUpgradeContainer]
	if !ok {
		klog.V(6).InfoS("Pod annotations was not found", "pod", klog.KObj(pod), "annotations", SidecarSetWorkingHotUpgradeContainer)
		return hotUpgradeWorkContainer
	}
	if err := json.Unmarshal([]byte(currentStr), &hotUpgradeWorkContainer); err != nil {
		klog.ErrorS(err, "Failed to parse pod annotations value failed", "pod", klog.KObj(pod),
			"annotations", SidecarSetWorkingHotUpgradeContainer, "value", currentStr)
		return hotUpgradeWorkContainer
	}
	return hotUpgradeWorkContainer
}

// GetPodHotUpgradeContainers return two hot upgrade sidecar containers
// workContainer: currently working sidecar container, record in pod annotations[kruise.io/sidecarset-working-hotupgrade-container]
// otherContainer:
//  1. empty container
//  2. when in hot upgrading process, the older sidecar container
func GetPodHotUpgradeContainers(sidecarName string, pod *corev1.Pod) (workContainer, otherContainer string) {
	hotUpgradeWorkContainer := GetPodHotUpgradeInfoInAnnotations(pod)
	name1, name2 := GetHotUpgradeContainerName(sidecarName)

	if hotUpgradeWorkContainer[sidecarName] == name1 {
		otherContainer = name2
		workContainer = name1
	} else {
		otherContainer = name1
		workContainer = name2
	}

	return
}

// para1: nameToUpgrade, para2: otherContainer
func findContainerToHotUpgrade(sidecarContainer *appsv1alpha1.SidecarContainer, pod *corev1.Pod, control SidecarControl) (string, string) {
	containerInPods := make(map[string]corev1.Container)
	for _, containerInPod := range pod.Spec.Containers {
		containerInPods[containerInPod.Name] = containerInPod
	}
	name1, name2 := GetHotUpgradeContainerName(sidecarContainer.Name)
	c1, c2 := containerInPods[name1], containerInPods[name2]

	// First, empty hot sidecar container will be upgraded with the latest sidecarSet specification
	if c1.Image == sidecarContainer.UpgradeStrategy.HotUpgradeEmptyImage {
		return c1.Name, c2.Name
	}
	if c2.Image == sidecarContainer.UpgradeStrategy.HotUpgradeEmptyImage {
		return c2.Name, c1.Name
	}

	// Second, Not ready sidecar container will be upgraded
	c1Ready := podutil.GetExistingContainerStatus(pod.Status.ContainerStatuses, c1.Name).Ready && control.IsPodStateConsistent(pod, sets.NewString(c1.Name))
	c2Ready := podutil.GetExistingContainerStatus(pod.Status.ContainerStatuses, c2.Name).Ready && control.IsPodStateConsistent(pod, sets.NewString(c2.Name))
	klog.V(3).InfoS("Pod container ready", "pod", klog.KObj(pod), "container1Name", c1.Name, "container1Ready",
		c1Ready, "container2Name", c2.Name, "container2Ready", c2Ready)
	if c1Ready && !c2Ready {
		return c2.Name, c1.Name
	}
	if !c1Ready && c2Ready {
		return c1.Name, c2.Name
	}

	// Third, the older sidecar container will be upgraded
	workContainer, olderContainer := GetPodHotUpgradeContainers(sidecarContainer.Name, pod)
	return olderContainer, workContainer
}
