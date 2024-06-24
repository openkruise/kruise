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

package sidecarset

import (
	"context"
	"fmt"

	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	"github.com/openkruise/kruise/pkg/control/sidecarcontrol"
	"github.com/openkruise/kruise/pkg/util"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/client-go/util/retry"
	"k8s.io/klog/v2"
)

func (p *Processor) flipHotUpgradingContainers(control sidecarcontrol.SidecarControl, pods []*corev1.Pod) error {
	for _, pod := range pods {
		if err := p.flipPodSidecarContainer(control, pod); err != nil {
			p.recorder.Eventf(pod, corev1.EventTypeWarning, "ResetContainerFailed", fmt.Sprintf("reset sidecar container image empty failed: %s", err.Error()))
			return err
		}
		p.recorder.Eventf(pod, corev1.EventTypeNormal, "ResetContainerSucceed", "reset sidecar container image empty successfully")
	}
	return nil
}

func (p *Processor) flipPodSidecarContainer(control sidecarcontrol.SidecarControl, pod *corev1.Pod) error {
	podClone := pod.DeepCopy()
	err := retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		// sidecar container hot upgrade already complete, and flip container
		flipPodSidecarContainerDo(control, podClone)
		// update pod in store
		updateErr := p.Client.Update(context.TODO(), podClone)
		if updateErr == nil {
			return nil
		}

		key := types.NamespacedName{
			Namespace: podClone.Namespace,
			Name:      podClone.Name,
		}
		if err := p.Client.Get(context.TODO(), key, podClone); err != nil {
			klog.ErrorS(err, "Failed to get updated pod from client", "pod", klog.KObj(podClone))
		}
		return updateErr
	})

	return err
}

func flipPodSidecarContainerDo(control sidecarcontrol.SidecarControl, pod *corev1.Pod) {
	sidecarSet := control.GetSidecarset()
	containersInPod := make(map[string]*corev1.Container)
	for i := range pod.Spec.Containers {
		container := &pod.Spec.Containers[i]
		containersInPod[container.Name] = container
	}

	var changedContainer []string
	for _, sidecarContainer := range sidecarSet.Spec.Containers {
		if sidecarcontrol.IsHotUpgradeContainer(&sidecarContainer) {
			workContainer, emptyContainer := sidecarcontrol.GetPodHotUpgradeContainers(sidecarContainer.Name, pod)
			if containersInPod[emptyContainer].Image == sidecarContainer.UpgradeStrategy.HotUpgradeEmptyImage {
				continue
			}
			// flip the empty sidecar container image
			containerNeedFlip := containersInPod[emptyContainer]
			klog.V(3).InfoS("Tried to reset container's image to empty", "pod", klog.KObj(pod), "containerName", containerNeedFlip.Name,
				"imageName", containerNeedFlip.Image, "hotUpgradeEmptyImageName", sidecarContainer.UpgradeStrategy.HotUpgradeEmptyImage)
			containerNeedFlip.Image = sidecarContainer.UpgradeStrategy.HotUpgradeEmptyImage
			changedContainer = append(changedContainer, containerNeedFlip.Name)
			// update pod sidecarSet version annotations
			pod.Annotations[sidecarcontrol.GetPodSidecarSetVersionAnnotation(containerNeedFlip.Name)] = "0"
			pod.Annotations[sidecarcontrol.GetPodSidecarSetVersionAltAnnotation(workContainer)] = "0"
		}
	}
	// record the updated container status, to determine if the update is complete
	control.UpdatePodAnnotationsInUpgrade(changedContainer, pod)
}

func isSidecarSetHasHotUpgradeContainer(sidecarSet *appsv1alpha1.SidecarSet) bool {
	for _, sidecarContainer := range sidecarSet.Spec.Containers {
		if sidecarcontrol.IsHotUpgradeContainer(&sidecarContainer) {
			return true
		}
	}
	return false
}

func isHotUpgradingReady(sidecarSet *appsv1alpha1.SidecarSet, pod *corev1.Pod) bool {
	if util.IsRunningAndReady(pod) {
		return true
	}

	emptyContainers := sets.NewString()
	for _, sidecarContainer := range sidecarSet.Spec.Containers {
		if sidecarcontrol.IsHotUpgradeContainer(&sidecarContainer) {
			_, emptyContainer := sidecarcontrol.GetPodHotUpgradeContainers(sidecarContainer.Name, pod)
			emptyContainers.Insert(emptyContainer)
		}
	}

	for _, containerStatus := range pod.Status.ContainerStatuses {
		// ignore empty sidecar container status
		if emptyContainers.Has(containerStatus.Name) {
			continue
		}
		// if container is not ready, then return false
		if !containerStatus.Ready {
			return false
		}
	}
	// all containers with exception of empty sidecar containers are ready, then return true
	return true
}

// If none of the hot upgrade container has HotUpgradeEmptyImage,
// then Pod is in hotUpgrading and return true
func isPodSidecarInHotUpgrading(sidecarSet *appsv1alpha1.SidecarSet, pod *corev1.Pod) bool {
	containerImage := make(map[string]string)
	for _, container := range pod.Spec.Containers {
		containerImage[container.Name] = container.Image
	}

	for _, sidecar := range sidecarSet.Spec.Containers {
		if sidecarcontrol.IsHotUpgradeContainer(&sidecar) {
			_, emptyContainer := sidecarcontrol.GetPodHotUpgradeContainers(sidecar.Name, pod)
			if containerImage[emptyContainer] != sidecar.UpgradeStrategy.HotUpgradeEmptyImage {
				return true
			}
		}
	}
	return false
}
