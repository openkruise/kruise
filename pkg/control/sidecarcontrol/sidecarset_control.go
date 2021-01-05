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

	"github.com/openkruise/kruise/apis/apps/pub"
	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	"github.com/openkruise/kruise/pkg/util"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/klog"
)

type commonControl struct {
	*appsv1alpha1.SidecarSet
}

func (c *commonControl) GetSidecarset() *appsv1alpha1.SidecarSet {
	return c.SidecarSet
}

func (c *commonControl) UpdateSidecarContainerToLatest(containerInSidecarSet, containerInPod v1.Container) v1.Container {
	containerInPod.Image = containerInSidecarSet.Image
	return containerInPod
}

func (c *commonControl) IsNeedInjectVolumeMount(volumeMount v1.VolumeMount) bool {
	return true
}

func (c *commonControl) NeedInjectOnUpdatedPod(pod, oldPod *v1.Pod, sidecarContainer *appsv1alpha1.SidecarContainer,
	injectedEnvs []v1.EnvVar, injectedMounts []v1.VolumeMount) (needInject bool, existSidecars []*appsv1alpha1.SidecarContainer, existVolumes []v1.Volume) {

	return false, nil, nil
}

func (c *commonControl) IsPodConsistentAndReady(pod *v1.Pod) bool {
	// If pod is consistent
	if !c.IsPodUpdatedConsistent(pod, nil) {
		return false
	}

	// 1. pod.Status.Phase == v1.PodRunning
	// 2. pod.condition PodReady == true
	return util.IsRunningAndReady(pod)
}

func (c *commonControl) UpdatePodAnnotationsInUpgrade(changedContainers []string, pod *v1.Pod) {

	sidecarSet := c.GetSidecarset()
	// 1. sidecar hash
	updatePodSidecarSetHash(pod, sidecarSet)

	// 3. record the ImageID, before update pod sidecar container
	// if it is changed, indicates the update is complete.
	//format: sidecarset.name -> appsv1alpha1.InPlaceUpdateState
	sidecarUpdateStates := make(map[string]*pub.InPlaceUpdateState)
	if stateStr, _ := pod.Annotations[SidecarsetInplaceUpdateStateKey]; len(stateStr) > 0 {
		if err := json.Unmarshal([]byte(stateStr), &sidecarUpdateStates); err != nil {
			klog.Errorf("parse pod(%s.%s) annotations[%s] value(%s) failed: %s",
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
func (c *commonControl) IsPodUpdatedConsistent(pod *v1.Pod, ignoreContainers sets.String) bool {
	sidecarset := c.GetSidecarset()
	sidecarContainers := GetSidecarContainersInPod(sidecarset)

	allDigestImage := true
	for _, container := range pod.Spec.Containers {
		// only check whether sidecar container is consistent
		if !sidecarContainers.Has(container.Name) {
			continue
		}
		if ignoreContainers.Has(container.Name) {
			continue
		}
		//whether image is digest format,
		//for example: docker.io/busybox@sha256:a9286defaba7b3a519d585ba0e37d0b2cbee74ebfe590960b0b1d6a5e97d1e1d
		if !util.IsImageDigest(container.Image) {
			allDigestImage = false
			continue
		}

		if !util.IsPodContainerDigestEqual(sets.NewString(container.Name), pod) {
			return false
		}
	}
	// If all spec.container[x].image is digest format, only check digest imageId
	if allDigestImage {
		return true
	}

	// check container InpalceUpdate status
	if err := isSidecarContainerUpdateCompleted(pod, sidecarset.Name, sidecarContainers); err != nil {
		klog.V(5).Infof(err.Error())
		return false
	}
	return true
}

// k8s only allow modify pod.spec.container[x].image,
// only when annotations[SidecarSetHashWithoutImageAnnotation] is the same, sidecarSet can upgrade pods
func (c *commonControl) IsSidecarSetCanUpgrade(pod *v1.Pod) bool {
	sidecarSet := c.GetSidecarset()
	return GetPodSidecarSetWithoutImageRevision(sidecarSet.Name, pod) == GetSidecarSetWithoutImageRevision(sidecarSet)
}

// isContainerInplaceUpdateCompleted checks whether imageID in container status has been changed since in-place update.
// If the imageID in containerStatuses has not been changed, we assume that kubelet has not updated containers in Pod.
func isSidecarContainerUpdateCompleted(pod *v1.Pod, sidecarSetName string, containers sets.String) error {
	if len(pod.Spec.Containers) != len(pod.Status.ContainerStatuses) {
		return fmt.Errorf("pod(%s.%s) is inconsistent in ContainerStatuses", pod.Namespace, pod.Name)
	}

	//format: sidecarset.name -> appsv1alpha1.InPlaceUpdateState
	sidecarUpdateStates := make(map[string]*pub.InPlaceUpdateState)
	// when the pod annotation not found, indicates the pod only injected sidecar container, and never inplace update
	// then always think it update complete
	if stateStr, ok := pod.Annotations[SidecarsetInplaceUpdateStateKey]; !ok {
		klog.V(5).Infof("pod(%s.%s) annotations[%s] not found", pod.Namespace, pod.Name, SidecarsetInplaceUpdateStateKey)
		return nil
		// this won't happen in practice, unless someone manually edit pod annotations
	} else if err := json.Unmarshal([]byte(stateStr), &sidecarUpdateStates); err != nil {
		return fmt.Errorf("parse pod(%s.%s) annotations[%s] value(%s) failed: %s",
			pod.Namespace, pod.Name, SidecarsetInplaceUpdateStateKey, stateStr, err.Error())
	}

	// when the sidecarset InPlaceUpdateState not found, indicates the pod only injected sidecar container, and never inplace update
	// then always think it update complete
	inPlaceUpdateState, ok := sidecarUpdateStates[sidecarSetName]
	if !ok {
		klog.V(5).Infof("parse pod(%s.%s) annotations[%s] sidecarset(%s) Not Found",
			pod.Namespace, pod.Name, SidecarsetInplaceUpdateStateKey, sidecarSetName)
		return nil
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
		if oldStatus, ok := inPlaceUpdateState.LastContainerStatuses[cs.Name]; ok {
			// we assume that users should not update workload template with new image
			// which actually has the same imageID as the old image
			if oldStatus.ImageID == cs.ImageID && containerImages[cs.Name] != cs.Image {
				return fmt.Errorf("container %s imageID not changed", cs.Name)
			}
		}
		// If sidecar container status.ImageID changed, or this oldStatus ImageID don't exist
		// indicate the sidecar container update is complete
	}

	return nil
}
