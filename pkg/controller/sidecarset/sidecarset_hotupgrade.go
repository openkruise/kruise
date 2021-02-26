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
	"k8s.io/klog"
)

func (p *Processor) flipHotUpgradingContainers(control sidecarcontrol.SidecarControl, pods []*corev1.Pod) error {
	for _, pod := range pods {
		if err := p.flipPodSidecarContainer(control, pod); err != nil {
			p.recorder.Eventf(pod, corev1.EventTypeWarning, "ResetContainerFailed", fmt.Sprintf("reset sidecar container image empty failed: %s", err.Error()))
			return err
		}
		p.recorder.Eventf(pod, corev1.EventTypeNormal, "ResetContainerSucceed", fmt.Sprintf("reset sidecar container image empty successfully"))
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
			klog.Errorf("error getting updated pod(%s.%s) from client", podClone.Namespace, podClone.Name)
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
			klog.V(3).Infof("try to reset %v/%v/%v from %s to empty(%s)", pod.Namespace, pod.Name, containerNeedFlip.Name,
				containerNeedFlip.Image, sidecarContainer.UpgradeStrategy.HotUpgradeEmptyImage)
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

func updateHotUpgradeContainerInPod(sidecarContainer *appsv1alpha1.SidecarContainer, control sidecarcontrol.SidecarControl, pod *corev1.Pod) (changedContainer string) {
	sidecarSet := control.GetSidecarset()
	containerInPods := make(map[string]corev1.Container)
	for _, containerInPod := range pod.Spec.Containers {
		containerInPods[containerInPod.Name] = containerInPod
	}

	// the container will be upgraded in latest sidecarSet specification
	nameToUpgrade := findContainerToHotUpgrade(sidecarContainer, pod, control)
	containerToUpgrade := containerInPods[nameToUpgrade]
	newContainer := control.UpdateSidecarContainerToLatest(sidecarContainer.Container, containerToUpgrade)
	// new hot upgrade sidecar container specification
	afterContainerSpec := util.DumpJSON(newContainer)

	// older hot upgrade sidecar container specification
	var beforeContainerSpec string
	// older sidecar container
	var olderSidecar string
	name1, name2 := sidecarcontrol.GetHotUpgradeContainerName(sidecarContainer.Name)
	if nameToUpgrade == name1 {
		beforeContainerSpec = util.DumpJSON(containerInPods[name2])
		olderSidecar = name2
	} else {
		beforeContainerSpec = util.DumpJSON(containerInPods[name1])
		olderSidecar = name1
	}

	// sidecarToUpgrade: sidecarSet.Spec.Container[x].name -> sidecar container in pod
	// for example: mesh -> mesh-1, envoy -> envoy-2...
	sidecarToUpgrade := make(map[string]string)
	// pod.container definition changed, then update container spec in pod
	if beforeContainerSpec != afterContainerSpec {
		klog.V(3).Infof("try to update container %v/%v/%v, before: %v, after: %v",
			pod.Namespace, pod.Name, newContainer.Name, beforeContainerSpec, afterContainerSpec)
		updateContainerInPod(newContainer, pod)
		sidecarToUpgrade[sidecarContainer.Name] = newContainer.Name
		sidecarcontrol.RecordHotUpgradeInfoInAnnotations(sidecarToUpgrade, pod)
		// update sidecar container resource version in annotations
		pod.Annotations[sidecarcontrol.GetPodSidecarSetVersionAnnotation(newContainer.Name)] = sidecarSet.ResourceVersion
		pod.Annotations[sidecarcontrol.GetPodSidecarSetVersionAltAnnotation(newContainer.Name)] = pod.Annotations[sidecarcontrol.GetPodSidecarSetVersionAnnotation(olderSidecar)]
		pod.Annotations[sidecarcontrol.GetPodSidecarSetVersionAltAnnotation(olderSidecar)] = sidecarSet.ResourceVersion
		changedContainer = newContainer.Name
	}
	return
}

func findContainerToHotUpgrade(sidecarContainer *appsv1alpha1.SidecarContainer, pod *corev1.Pod, control sidecarcontrol.SidecarControl) string {
	containerInPods := make(map[string]corev1.Container)
	for _, containerInPod := range pod.Spec.Containers {
		containerInPods[containerInPod.Name] = containerInPod
	}
	name1, name2 := sidecarcontrol.GetHotUpgradeContainerName(sidecarContainer.Name)
	c1, c2 := containerInPods[name1], containerInPods[name2]

	// First, empty hot sidecar container will be upgraded with the latest sidecarSet specification
	if c1.Image == sidecarContainer.UpgradeStrategy.HotUpgradeEmptyImage {
		return c1.Name
	} else if c2.Image == sidecarContainer.UpgradeStrategy.HotUpgradeEmptyImage {
		return c2.Name
	}

	// Second, Not ready sidecar container will be upgraded
	c1Ready, c2Ready := isContainerConsistentAndReady(&c1, pod, control), isContainerConsistentAndReady(&c2, pod, control)
	klog.V(3).Infof("pod(%s.%s) container(%s) ready(%v) container(%s) ready(%v)", pod.Namespace, pod.Name, c1.Name, c1Ready, c2.Name, c2Ready)
	if c1Ready && !c2Ready {
		return c2.Name
	} else if !c1Ready && c2Ready {
		return c1.Name
	}

	// Third, the older sidecar container will be upgraded
	_, olderContainer := sidecarcontrol.GetPodHotUpgradeContainers(sidecarContainer.Name, pod)
	return olderContainer
}

func isContainerConsistentAndReady(c *corev1.Container, pod *corev1.Pod, control sidecarcontrol.SidecarControl) bool {
	for _, containerStatus := range pod.Status.ContainerStatuses {
		if containerStatus.Name == c.Name {
			return containerStatus.Ready && control.IsPodUpdatedConsistently(pod, sets.NewString(c.Name))
		}
	}
	return false
}
