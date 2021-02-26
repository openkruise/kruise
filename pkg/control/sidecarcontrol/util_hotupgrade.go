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

	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/klog"
)

const (
	// record which hot upgrade container is working currently
	SidecarSetWorkingHotUpgradeContainer = "kruise.io/sidecarset-working-hotupgrade-container"

	// hotUpgrade container name suffix
	hotUpgradeNameSuffix1 = "-1"
	hotUpgradeNameSuffix2 = "-2"

	// sidecar container version in container env(SIDECARSET_VERSION)
	SidecarSetVersionEnvKey = "SIDECARSET_VERSION"
)

// return format: mesh-1, mesh-2
func GetHotUpgradeContainerName(name string) (string, string) {
	return name + hotUpgradeNameSuffix1, name + hotUpgradeNameSuffix2
}

// only used in hot upgrade container
// cName format: mesh-1, mesh-2
func GetSidecarContainerVersionAnnotationKey(cName string) string {
	return fmt.Sprintf("version.sidecarset.kruise.io/%s", cName)
}

// whether sidecar container update strategy is HotUpdate
func IsHotUpgradeContainer(sidecarContainer *appsv1alpha1.SidecarContainer) bool {
	return sidecarContainer.UpgradeStrategy.UpgradeType == appsv1alpha1.SidecarContainerHotUpgrade
}

// sidecarToUpgrade: sidecarSet.Spec.Container[x].name -> sidecar container in pod
// for example: mesh -> mesh-1, envoy -> envoy-2...
func RecordHotUpgradeInfoInAnnotations(sidecarToUpgrade map[string]string, pod *corev1.Pod) {
	hotUpgradeContainerInfos := GetHotUpgradeContainerInfos(pod)
	for sidecar, containerInPod := range sidecarToUpgrade {
		hotUpgradeContainerInfos[sidecar] = containerInPod
	}

	by, _ := json.Marshal(hotUpgradeContainerInfos)
	pod.Annotations[SidecarSetWorkingHotUpgradeContainer] = string(by)
}

// which hot upgrade sidecar container is working now
// format: sidecarset.spec.container[x].name -> pod.spec.container[x].name
// for example: mesh -> mesh-1, envoy -> envoy-2
func GetHotUpgradeContainerInfos(pod *corev1.Pod) map[string]string {
	hotUpgradeWorkContainer := make(map[string]string)
	currentStr, ok := pod.Annotations[SidecarSetWorkingHotUpgradeContainer]
	if !ok {
		klog.V(6).Infof("Pod(%s.%s) annotations(%s) Not Found", pod.Namespace, pod.Name, SidecarSetWorkingHotUpgradeContainer)
		return hotUpgradeWorkContainer
	}
	if err := json.Unmarshal([]byte(currentStr), &hotUpgradeWorkContainer); err != nil {
		klog.Errorf("Parse Pod(%s.%s) annotations(%s) Value(%s) failed: %s", pod.Namespace, pod.Name,
			SidecarSetWorkingHotUpgradeContainer, currentStr, err.Error())
		return hotUpgradeWorkContainer
	}
	return hotUpgradeWorkContainer
}

func GetEmptyHotUpgradeContainer(sidecarName string, pod *corev1.Pod) string {
	hotUpgradeWorkContainer := GetHotUpgradeContainerInfos(pod)
	name1, name2 := GetHotUpgradeContainerName(sidecarName)

	var notWorkContainer string
	if hotUpgradeWorkContainer[sidecarName] == name1 {
		notWorkContainer = name2
	} else {
		notWorkContainer = name1
	}

	return notWorkContainer
}

func GetSidecarSetEmptyContainers(sidecarSet *appsv1alpha1.SidecarSet, pod *corev1.Pod) sets.String {
	names := sets.NewString()
	for _, sidecarContainer := range sidecarSet.Spec.Containers {
		if IsHotUpgradeContainer(&sidecarContainer) {
			names.Insert(GetEmptyHotUpgradeContainer(sidecarContainer.Name, pod))
		}
	}
	return names
}
