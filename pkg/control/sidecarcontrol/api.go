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
	//common
	// get sidecarset
	GetSidecarset() *appsv1alpha1.SidecarSet

	// inject
	// whether need inject the volumeMount into container
	IsNeedInjectVolumeMount(volumeMount v1.VolumeMount) bool
	// when update pod, judge whether inject sidecar container into pod
	NeedInjectOnUpdatedPod(pod, oldPod *v1.Pod, sidecarContainer *appsv1alpha1.SidecarContainer, injectedEnvs []v1.EnvVar,
		injectedMounts []v1.VolumeMount) (needInject bool, existSidecars []*appsv1alpha1.SidecarContainer, existVolumes []v1.Volume)

	// update
	// pod whether consistent and ready
	IsPodConsistentAndReady(pod *v1.Pod) bool
	// update pod sidecar container to sidecarSet latest version
	UpdateSidecarContainerToLatest(containerInSidecarSet, containerInPod v1.Container) v1.Container
	// update pod information in upgrade
	UpdatePodAnnotationsInUpgrade(changedContainers []string, pod *v1.Pod)
	// IsPodUpdatedConsistent indicates whether pod.spec and pod.status are consistent after updating the sidecar containers
	IsPodUpdatedConsistent(pod *v1.Pod, ignoreContainers sets.String) bool
	// Is sidecarset can upgrade pods
	IsSidecarSetCanUpgrade(pod *v1.Pod) bool
}

func New(cs *appsv1alpha1.SidecarSet) SidecarControl {
	return &commonControl{SidecarSet: cs}
}
