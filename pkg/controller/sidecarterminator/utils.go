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

package sidecarterminator

import (
	"strings"

	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/sets"
)

func isInterestingPod(pod *corev1.Pod) bool {
	if pod.DeletionTimestamp != nil ||
		pod.Status.Phase == corev1.PodPending ||
		pod.Spec.RestartPolicy == corev1.RestartPolicyAlways {
		return false
	}

	mainContainers, sidecarContainers := groupMainSidecarContainers(pod)
	if sidecarContainers.Len() == 0 || containersCompleted(pod, sidecarContainers) {
		return false
	}

	switch pod.Spec.RestartPolicy {
	case corev1.RestartPolicyNever:
		return containersCompleted(pod, mainContainers)
	case corev1.RestartPolicyOnFailure:
		return containersSucceeded(pod, mainContainers)
	}
	return false
}

func groupMainSidecarContainers(pod *corev1.Pod) (sets.Set[string], sets.Set[string]) {
	mainNames := sets.New[string]()
	sidecarNames := sets.New[string]()
	for i := range pod.Spec.Containers {
		container := pod.Spec.Containers[i]
		if isSidecarContainer(container) {
			sidecarNames.Insert(pod.Spec.Containers[i].Name)
		} else {
			mainNames.Insert(pod.Spec.Containers[i].Name)
		}
	}
	return mainNames, sidecarNames
}

func getSidecarContainerNames(pod *corev1.Pod, vk bool) (sets.Set[string], sets.Set[string], sets.Set[string]) {
	normalSidecarNames := sets.New[string]()
	ignoreExitCodeSidecarNames := sets.New[string]()
	inplaceUpdateSidecarNames := sets.New[string]()

	for i := range pod.Spec.Containers {
		container := pod.Spec.Containers[i]
		if vk && isInplaceUpdateSidecar(container) {
			inplaceUpdateSidecarNames.Insert(container.Name)
		} else if !vk && isIgnoreExitCodeSidecar(container) {
			ignoreExitCodeSidecarNames.Insert(container.Name)
		} else if !vk && isNormalSidecar(container) {
			normalSidecarNames.Insert(container.Name)
		}
	}

	return normalSidecarNames, ignoreExitCodeSidecarNames, inplaceUpdateSidecarNames
}

func isNormalSidecar(container corev1.Container) bool {
	for _, env := range container.Env {
		if env.Name == appsv1alpha1.KruiseTerminateSidecarEnv && strings.EqualFold(env.Value, "true") {
			return true
		}
	}
	return false
}

func isIgnoreExitCodeSidecar(container corev1.Container) bool {
	if !isNormalSidecar(container) {
		return false
	}
	for _, env := range container.Env {
		if env.Name == appsv1alpha1.KruiseIgnoreContainerExitCodeEnv && strings.EqualFold(env.Value, "true") {
			return true
		}
	}
	return false
}

func isInplaceUpdateSidecar(container corev1.Container) bool {
	for _, env := range container.Env {
		if env.Name == appsv1alpha1.KruiseTerminateSidecarWithImageEnv && env.Value != "" {
			return true
		}
	}
	return false
}

func isSidecarContainer(container corev1.Container) bool {
	return isNormalSidecar(container) || isInplaceUpdateSidecar(container)
}
