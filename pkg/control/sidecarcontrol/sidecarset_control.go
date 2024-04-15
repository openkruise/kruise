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

	"github.com/openkruise/kruise/apis/apps/pub"
	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	"github.com/openkruise/kruise/pkg/util"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/klog/v2"
)

type commonControl struct {
	*appsv1alpha1.SidecarSet
}

func (c *commonControl) GetSidecarset() *appsv1alpha1.SidecarSet {
	return c.SidecarSet
}

func (c *commonControl) IsActiveSidecarSet() bool {
	return true
}

func (c *commonControl) UpgradeSidecarContainer(sidecarContainer *appsv1alpha1.SidecarContainer, pod *v1.Pod) *v1.Container {
	var nameToUpgrade, otherContainer, oldImage string
	if IsHotUpgradeContainer(sidecarContainer) {
		nameToUpgrade, otherContainer = findContainerToHotUpgrade(sidecarContainer, pod, c)
		oldImage = util.GetContainer(otherContainer, pod).Image
	} else {
		nameToUpgrade = sidecarContainer.Name
		oldImage = util.GetContainer(nameToUpgrade, pod).Image
	}
	// community in-place upgrades are only allowed to update image
	if sidecarContainer.Image == oldImage {
		return nil
	}
	container := util.GetContainer(nameToUpgrade, pod)
	container.Image = sidecarContainer.Image
	klog.V(3).InfoS("Upgraded pod container image", "pod", klog.KObj(pod), "containerName", nameToUpgrade,
		"oldImage", oldImage, "newImage", container.Image)
	return container
}

func (c *commonControl) NeedToInjectVolumeMount(volumeMount v1.VolumeMount) bool {
	return true
}

func (c *commonControl) NeedToInjectInUpdatedPod(pod, oldPod *v1.Pod, sidecarContainer *appsv1alpha1.SidecarContainer,
	injectedEnvs []v1.EnvVar, injectedMounts []v1.VolumeMount) (needInject bool, existSidecars []*appsv1alpha1.SidecarContainer, existVolumes []v1.Volume) {

	return false, nil, nil
}

func (c *commonControl) IsPodReady(pod *v1.Pod) bool {
	sidecarSet := c.GetSidecarset()
	// check whether hot upgrade is complete
	// map[string]string: {empty container name}->{sidecarSet.spec.containers[x].upgradeStrategy.HotUpgradeEmptyImage}
	emptyContainers := map[string]string{}
	for _, sidecarContainer := range sidecarSet.Spec.Containers {
		if IsHotUpgradeContainer(&sidecarContainer) {
			_, emptyContainer := GetPodHotUpgradeContainers(sidecarContainer.Name, pod)
			emptyContainers[emptyContainer] = sidecarContainer.UpgradeStrategy.HotUpgradeEmptyImage
		}
	}
	for _, container := range pod.Spec.Containers {
		// If container is empty container, then its image must be empty image
		if emptyImage := emptyContainers[container.Name]; emptyImage != "" && container.Image != emptyImage {
			klog.V(5).InfoS("Pod sidecar empty container image wasn't empty image", "pod", klog.KObj(pod),
				"containerName", container.Name, "containerImage", container.Image, "emptyImage", emptyImage)
			return false
		}
	}

	// 1. pod.Status.Phase == v1.PodRunning
	// 2. pod.condition PodReady == true
	return util.IsRunningAndReady(pod)
}

func (c *commonControl) UpdatePodAnnotationsInUpgrade(changedContainers []string, pod *v1.Pod) {
	sidecarSet := c.GetSidecarset()
	// record the ImageID, before update pod sidecar container
	// if it is changed, indicates the update is complete.
	// format: sidecarset.name -> appsv1alpha1.InPlaceUpdateState
	sidecarUpdateStates := make(map[string]*pub.InPlaceUpdateState)
	if stateStr := pod.Annotations[SidecarsetInplaceUpdateStateKey]; len(stateStr) > 0 {
		if err := json.Unmarshal([]byte(stateStr), &sidecarUpdateStates); err != nil {
			klog.ErrorS(err, "Failed to parse pod annotations value", "pod", klog.KObj(pod),
				"annotation", SidecarsetInplaceUpdateStateKey, "value", stateStr)
		}
	}
	inPlaceUpdateState, ok := sidecarUpdateStates[sidecarSet.Name]
	if !ok {
		inPlaceUpdateState = &pub.InPlaceUpdateState{
			Revision:        GetSidecarSetRevision(sidecarSet),
			UpdateTimestamp: metav1.Now(),
		}
	}
	// format: container.name -> pod.status.containers[container.name].ImageID
	if inPlaceUpdateState.LastContainerStatuses == nil {
		inPlaceUpdateState.LastContainerStatuses = make(map[string]pub.InPlaceUpdateContainerStatus)
	}

	cStatus := make(map[string]string, len(pod.Status.ContainerStatuses))
	for i := range pod.Status.ContainerStatuses {
		c := &pod.Status.ContainerStatuses[i]
		cStatus[c.Name] = c.ImageID
	}
	for _, cName := range changedContainers {
		updateStatus := pub.InPlaceUpdateContainerStatus{
			ImageID: cStatus[cName],
		}
		// record status.ImageId before update pods in store
		inPlaceUpdateState.LastContainerStatuses[cName] = updateStatus
	}

	// record sidecar container status information in pod's annotations
	sidecarUpdateStates[sidecarSet.Name] = inPlaceUpdateState
	by, _ := json.Marshal(sidecarUpdateStates)
	pod.Annotations[SidecarsetInplaceUpdateStateKey] = string(by)
}

// only check sidecar container is consistent
func (c *commonControl) IsPodStateConsistent(pod *v1.Pod, sidecarContainers sets.String) bool {
	if len(pod.Spec.Containers) != len(pod.Status.ContainerStatuses) {
		return false
	}

	sidecarset := c.GetSidecarset()
	if sidecarContainers.Len() == 0 {
		sidecarContainers = GetSidecarContainersInPod(sidecarset)
	}

	allDigestImage := true
	cImageIDs := util.GetPodContainerImageIDs(pod)
	for _, container := range pod.Spec.Containers {
		// only check whether sidecar container is consistent
		if !sidecarContainers.Has(container.Name) {
			continue
		}

		// whether image is digest format,
		// for example: docker.io/busybox@sha256:a9286defaba7b3a519d585ba0e37d0b2cbee74ebfe590960b0b1d6a5e97d1e1d
		if !util.IsImageDigest(container.Image) {
			allDigestImage = false
			break
		}

		imageID, ok := cImageIDs[container.Name]
		if !ok {
			return false
		}
		if !util.IsContainerImageEqual(container.Image, imageID) {
			return false
		}
	}
	// If all spec.container[x].image is digest format, only check digest imageId
	if allDigestImage {
		return true
	}

	// check container InplaceUpdate status
	return IsSidecarContainerUpdateCompleted(pod, sets.NewString(sidecarset.Name), sidecarContainers)
}

func (c *commonControl) IsSidecarSetUpgradable(pod *v1.Pod) (canUpgrade, consistent bool) {
	sidecarSet := c.GetSidecarset()
	// k8s only allow modify pod.spec.container[x].image,
	// only when annotations[SidecarSetHashWithoutImageAnnotation] is the same, sidecarSet can upgrade pods
	if GetPodSidecarSetWithoutImageRevision(sidecarSet.Name, pod) != GetSidecarSetWithoutImageRevision(sidecarSet) {
		return false, false
	}

	// cStatus: container.name -> containerStatus.Ready
	cStatus := map[string]bool{}
	for _, status := range pod.Status.ContainerStatuses {
		cStatus[status.Name] = status.Ready
	}
	sidecarContainerList := GetSidecarContainersInPod(sidecarSet)
	for _, sidecar := range sidecarContainerList.List() {
		// when containerStatus.Ready == true and container non-consistent,
		// indicates that sidecar container is in the process of being upgraded
		// wait for the last upgrade to complete before performing this upgrade
		if cStatus[sidecar] && !c.IsPodStateConsistent(pod, sets.NewString(sidecar)) {
			return true, false
		}
	}

	return true, true
}

func (c *commonControl) IsPodAvailabilityChanged(pod, oldPod *v1.Pod) bool {
	return false
}

// isContainerInplaceUpdateCompleted checks whether imageID in container status has been changed since in-place update.
// If the imageID in containerStatuses has not been changed, we assume that kubelet has not updated containers in Pod.
func IsSidecarContainerUpdateCompleted(pod *v1.Pod, sidecarSets, containers sets.String) bool {
	// format: sidecarset.name -> appsv1alpha1.InPlaceUpdateState
	sidecarUpdateStates := make(map[string]*pub.InPlaceUpdateState)
	// when the pod annotation not found, indicates the pod only injected sidecar container, and never inplace update
	// then always think it update complete
	if stateStr, ok := pod.Annotations[SidecarsetInplaceUpdateStateKey]; !ok {
		return true
		// this won't happen in practice, unless someone manually edit pod annotations
	} else if err := json.Unmarshal([]byte(stateStr), &sidecarUpdateStates); err != nil {
		klog.V(5).InfoS("Failed to parse pod annotations value", "pod", klog.KObj(pod),
			"annotation", SidecarsetInplaceUpdateStateKey, "value", stateStr, "error", err)
		return false
	}

	// The container imageId recorded before the in-place sidecar upgrade
	// when the container imageId not found, indicates the pod only injected the sidecar container,
	// and never in-place update sidecar, then always think it update complete
	lastContainerStatus := make(map[string]pub.InPlaceUpdateContainerStatus)
	for _, sidecarSetName := range sidecarSets.List() {
		if inPlaceUpdateState, ok := sidecarUpdateStates[sidecarSetName]; ok {
			for name, status := range inPlaceUpdateState.LastContainerStatuses {
				lastContainerStatus[name] = status
			}
		}
	}

	containerImages := make(map[string]string, len(pod.Spec.Containers))
	for i := range pod.Spec.Containers {
		c := &pod.Spec.Containers[i]
		containerImages[c.Name] = c.Image
	}

	for _, cs := range pod.Status.ContainerStatuses {
		// only check containers set
		if !containers.Has(cs.Name) {
			continue
		}
		if oldStatus, ok := lastContainerStatus[cs.Name]; ok {
			// we assume that users should not update workload template with new image
			// which actually has the same imageID as the old image
			if oldStatus.ImageID == cs.ImageID && containerImages[cs.Name] != cs.Image {
				klog.V(5).InfoS("Pod container status imageID not changed, then inconsistent",
					"pod", klog.KObj(pod), "containerStatusName", cs.Name)
				return false
			}
		}
		// If sidecar container status.ImageID changed, or this oldStatus ImageID don't exist
		// indicate the sidecar container update is complete
	}

	return true
}
