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
	"github.com/openkruise/utils"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/client-go/tools/cache"
	"k8s.io/klog/v2"
)

// NewCommonControl new sidecarSet control
// indexer is controllerRevision indexer
// If you need GetSuitableRevisionSidecarSet function, you must set indexer, namespace parameters
// otherwise you don't need to set any parameters
func NewCommonControl(indexer cache.Indexer, namespace string) SidecarControl {
	c := &commonControl{}
	if indexer != nil {
		c.historyControl = NewHistoryControl(nil, indexer, namespace)
	}
	return c
}

type commonControl struct {
	historyControl HistoryControl
}

func (c *commonControl) UpgradeSidecarContainer(sidecarContainer *appsv1alpha1.SidecarContainer, pod *v1.Pod, sidecarSet *appsv1alpha1.SidecarSet) *v1.Container {
	var nameToUpgrade, otherContainer, oldImage string
	if IsHotUpgradeContainer(sidecarContainer) {
		nameToUpgrade, otherContainer = c.FindContainerToHotUpgrade(sidecarContainer, pod, sidecarSet)
		oldImage = utils.GetContainer(otherContainer, pod).Image
	} else {
		nameToUpgrade = sidecarContainer.Name
		oldImage = utils.GetContainer(nameToUpgrade, pod).Image
	}
	// community in-place upgrades are only allowed to update image
	if sidecarContainer.Image == oldImage {
		return nil
	}
	container := utils.GetContainer(nameToUpgrade, pod)
	container.Image = sidecarContainer.Image
	klog.V(3).Infof("upgrade pod(%s/%s) container(%s) Image from(%s) -> to(%s)", pod.Namespace, pod.Name, nameToUpgrade, oldImage, container.Image)
	return container
}

func (c *commonControl) NeedToInjectVolumeMount(volumeMount v1.VolumeMount) bool {
	return true
}

func (c *commonControl) NeedToInjectInUpdatedPod(pod, oldPod *v1.Pod, sidecarContainer *appsv1alpha1.SidecarContainer,
	injectedEnvs []v1.EnvVar, injectedMounts []v1.VolumeMount) (needInject bool, existSidecars []*appsv1alpha1.SidecarContainer, existVolumes []v1.Volume) {

	return false, nil, nil
}

func (c *commonControl) IsPodReady(pod *v1.Pod, sidecarSet *appsv1alpha1.SidecarSet) bool {
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
			klog.V(5).Infof("pod(%s/%s) sidecar empty container(%s) Image(%s) isn't Empty Image(%s)",
				pod.Namespace, pod.Name, container.Name, container.Image, emptyImage)
			return false
		}
	}

	// 1. pod.Status.Phase == v1.PodRunning
	// 2. pod.condition PodReady == true
	return utils.IsRunningAndReady(pod)
}

func (c *commonControl) UpdatePodAnnotationsInUpgrade(changedContainers []string, pod *v1.Pod, sidecarSet *appsv1alpha1.SidecarSet) {
	// record the ImageID, before update pod sidecar container
	// if it is changed, indicates the update is complete.
	//format: sidecarset.name -> appsv1alpha1.InPlaceUpdateState
	sidecarUpdateStates := make(map[string]*pub.InPlaceUpdateState)
	if stateStr, _ := pod.Annotations[SidecarsetInplaceUpdateStateKey]; len(stateStr) > 0 {
		if err := json.Unmarshal([]byte(stateStr), &sidecarUpdateStates); err != nil {
			klog.Errorf("parse pod(%s/%s) annotations[%s] value(%s) failed: %s",
				pod.Namespace, pod.Name, SidecarsetInplaceUpdateStateKey, stateStr, err.Error())
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
		//record status.ImageId before update pods in store
		inPlaceUpdateState.LastContainerStatuses[cName] = updateStatus
	}

	//record sidecar container status information in pod's annotations
	sidecarUpdateStates[sidecarSet.Name] = inPlaceUpdateState
	by, _ := json.Marshal(sidecarUpdateStates)
	pod.Annotations[SidecarsetInplaceUpdateStateKey] = string(by)
	return
}

// only check sidecar container is consistent
func (c *commonControl) IsPodStateConsistent(pod *v1.Pod, sidecarSet *appsv1alpha1.SidecarSet, sidecarContainers sets.String) bool {
	if len(pod.Spec.Containers) != len(pod.Status.ContainerStatuses) {
		return false
	}
	if sidecarContainers.Len() == 0 {
		sidecarContainers = GetSidecarContainersInPod(sidecarSet)
	}

	allDigestImage := true
	cImageIDs := utils.GetPodContainerImageIDs(pod)
	for _, container := range pod.Spec.Containers {
		// only check whether sidecar container is consistent
		if !sidecarContainers.Has(container.Name) {
			continue
		}

		//whether image is digest format,
		//for example: docker.io/busybox@sha256:a9286defaba7b3a519d585ba0e37d0b2cbee74ebfe590960b0b1d6a5e97d1e1d
		if !utils.IsImageDigest(container.Image) {
			allDigestImage = false
			break
		}

		imageID, ok := cImageIDs[container.Name]
		if !ok {
			return false
		}
		if !utils.IsContainerImageEqual(container.Image, imageID) {
			return false
		}
	}
	// If all spec.container[x].image is digest format, only check digest imageId
	if allDigestImage {
		return true
	}

	// check container InpalceUpdate status
	return IsSidecarContainerUpdateCompleted(pod, sets.NewString(sidecarSet.Name), sidecarContainers)
}

// k8s only allow modify pod.spec.container[x].image,
// only when annotations[SidecarSetHashWithoutImageAnnotation] is the same, sidecarSet can upgrade pods
func (c *commonControl) IsSidecarSetUpgradable(pod *v1.Pod, sidecarSet *appsv1alpha1.SidecarSet) bool {
	if GetPodSidecarSetWithoutImageRevision(sidecarSet.Name, pod) != GetSidecarSetWithoutImageRevision(sidecarSet) {
		return false
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
		if cStatus[sidecar] && !c.IsPodStateConsistent(pod, sidecarSet, sets.NewString(sidecar)) {
			return false
		}
	}

	return true
}

func (c *commonControl) IsPodAvailabilityChanged(pod, oldPod *v1.Pod) bool {
	return false
}

// para1: nameToUpgrade, para2: otherContainer
func (c *commonControl) FindContainerToHotUpgrade(sidecarContainer *appsv1alpha1.SidecarContainer, pod *v1.Pod, sidecarSet *appsv1alpha1.SidecarSet) (string, string) {
	containerInPods := make(map[string]v1.Container)
	for _, containerInPod := range pod.Spec.Containers {
		containerInPods[containerInPod.Name] = containerInPod
	}
	name1, name2 := GetHotUpgradeContainerName(sidecarContainer.Name)
	c1, c2 := containerInPods[name1], containerInPods[name2]

	// First, empty hot sidecar container will be upgraded with the latest sidecarSet specification
	if c1.Image == sidecarContainer.UpgradeStrategy.HotUpgradeEmptyImage {
		return c1.Name, c2.Name
	} else if c2.Image == sidecarContainer.UpgradeStrategy.HotUpgradeEmptyImage {
		return c2.Name, c1.Name
	}

	// Second, Not ready sidecar container will be upgraded
	c1Ready := utils.GetContainerStatus(c1.Name, pod).Ready && c.IsPodStateConsistent(pod, sidecarSet, sets.NewString(c1.Name))
	c2Ready := utils.GetContainerStatus(c2.Name, pod).Ready && c.IsPodStateConsistent(pod, sidecarSet, sets.NewString(c2.Name))
	klog.V(3).Infof("pod(%s/%s) container(%s) ready(%v) container(%s) ready(%v)", pod.Namespace, pod.Name, c1.Name, c1Ready, c2.Name, c2Ready)
	if c1Ready && !c2Ready {
		return c2.Name, c1.Name
	} else if !c1Ready && c2Ready {
		return c1.Name, c2.Name
	}

	// Third, the older sidecar container will be upgraded
	workContianer, olderContainer := GetPodHotUpgradeContainers(sidecarContainer.Name, pod)
	return olderContainer, workContianer
}

// isContainerInplaceUpdateCompleted checks whether imageID in container status has been changed since in-place update.
// If the imageID in containerStatuses has not been changed, we assume that kubelet has not updated containers in Pod.
func IsSidecarContainerUpdateCompleted(pod *v1.Pod, sidecarSets, containers sets.String) bool {
	//format: sidecarset.name -> appsv1alpha1.InPlaceUpdateState
	sidecarUpdateStates := make(map[string]*pub.InPlaceUpdateState)
	// when the pod annotation not found, indicates the pod only injected sidecar container, and never inplace update
	// then always think it update complete
	if stateStr, ok := pod.Annotations[SidecarsetInplaceUpdateStateKey]; !ok {
		return true
		// this won't happen in practice, unless someone manually edit pod annotations
	} else if err := json.Unmarshal([]byte(stateStr), &sidecarUpdateStates); err != nil {
		klog.V(5).Infof("parse pod(%s/%s) annotations[%s] value(%s) failed: %s",
			pod.Namespace, pod.Name, SidecarsetInplaceUpdateStateKey, stateStr, err.Error())
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
				klog.V(5).Infof("pod(%s/%s) container %s status imageID not changed, then inconsistent", pod.Namespace, pod.Name, cs.Name)
				return false
			}
		}
		// If sidecar container status.ImageID changed, or this oldStatus ImageID don't exist
		// indicate the sidecar container update is complete
	}

	return true
}

func (c *commonControl) GetSuitableRevisionSidecarSet(sidecarSet *appsv1alpha1.SidecarSet, oldPod, newPod *v1.Pod) (*appsv1alpha1.SidecarSet, error) {
	// If historyControl is nil, then don't support get suitable revision sidecarSet, and return current obj.
	if c.historyControl == nil {
		return sidecarSet.DeepCopy(), nil
	}
	return c.historyControl.GetSuitableRevisionSidecarSet(sidecarSet, oldPod, newPod)
}
