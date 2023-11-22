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
	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/sets"
)

type SidecarControl interface {
	//*****common*****//
	// get sidecarset
	GetSidecarset() *appsv1alpha1.SidecarSet
	// when sidecarSet is not active, it will not perform injections and upgrades process.
	// You can re-implement the function IsActiveSidecarSet to indicate that this sidecarSet is no longer working by adding some sidecarSet flags,
	// for example: sidecarSet.Annotations[sidecarset.kruise.io/disabled] = "true"
	IsActiveSidecarSet() bool

	//*****inject portion*****//
	// whether need inject the volumeMount into container
	// when ShareVolumePolicy is enabled, the sidecar container will share the other container's VolumeMounts in the pod(don't contains the injected sidecar container).
	// You can reimplement the function NeedToInjectVolumeMount to filter out some of the volumes that don't need to be shared
	NeedToInjectVolumeMount(volumeMount v1.VolumeMount) bool
	// when update pod, judge whether inject sidecar container into pod
	// one can customize validation to allow sidecar addition after pod creation, and reimplement NeedToInjectInUpdatedPod to enable such injection in sidecarset
	NeedToInjectInUpdatedPod(pod, oldPod *v1.Pod, sidecarContainer *appsv1alpha1.SidecarContainer, injectedEnvs []v1.EnvVar,
		injectedMounts []v1.VolumeMount) (needInject bool, existSidecars []*appsv1alpha1.SidecarContainer, existVolumes []v1.Volume)
	// IsPodAvailabilityChanged check whether pod changed on updating trigger re-inject sidecar container
	// For update pod injection sidecar container scenario, this method can filter out many invalid update events, thus improving the overall webhook performance.
	IsPodAvailabilityChanged(pod, oldPod *v1.Pod) bool

	//*****upgrade portion*****//
	// IsPodStateConsistent indicates whether pod.spec and pod.status are consistent after updating the sidecar containers
	IsPodStateConsistent(pod *v1.Pod, sidecarContainers sets.String) bool
	// IsPodReady indicates whether pod is fully ready
	// 1. pod.Status.Phase == v1.PodRunning
	// 2. pod.condition PodReady == true
	// 3. whether empty sidecar container is HotUpgradeEmptyImage
	IsPodReady(pod *v1.Pod) bool
	// upgrade pod sidecar container to sidecarSet latest version
	// if container==nil means no change, no need to update, otherwise need to update
	UpgradeSidecarContainer(sidecarContainer *appsv1alpha1.SidecarContainer, pod *v1.Pod) *v1.Container
	// When upgrading the pod sidecar container, you need to record some in-place upgrade information in pod annotations,
	// which is needed by the sidecarset controller to determine whether the upgrade is completed.
	UpdatePodAnnotationsInUpgrade(changedContainers []string, pod *v1.Pod)
	// Is sidecarset can upgrade pods,
	// In Kubernetes native scenarios, only Container Image upgrades are allowed
	// When modifying other fields of the container, e.g. volumemounts, the sidecarSet will not depart to upgrade the sidecar container logic in-place,
	// and needs to be done by rebuilding the pod
	// consistent indicates pod.spec and pod.status is consistent,
	// when pod.spec.image is v2 and pod.status.image is v1, then it is inconsistent.
	IsSidecarSetUpgradable(pod *v1.Pod) (canUpgrade, consistent bool)
}

func New(cs *appsv1alpha1.SidecarSet) SidecarControl {
	return &commonControl{SidecarSet: cs}
}
