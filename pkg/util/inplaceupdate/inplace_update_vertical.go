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

package inplaceupdate

import (
	appspub "github.com/openkruise/kruise/apis/apps/pub"
	v1 "k8s.io/api/core/v1"
)

// For In-place workload vertical scaling
type VerticalUpdateInterface interface {
	// Get the expected resource values of the container and its current status
	SyncContainerResource(container *v1.ContainerStatus, state *appspub.InPlaceUpdateState)
	// Pass in the container to be modified and the expected resource values.
	UpdateContainerResource(container *v1.Container, resource *v1.ResourceRequirements)
	// Get the expected resource values of all containers in the pod and their current status
	SyncPodResource(pod *v1.Pod, state *appspub.InPlaceUpdateState)
	// All containers of a pod can be updated at once within this interface.
	UpdatePodResource(pod *v1.Pod)
	// To determine whether the container has been successfully vertical updated
	IsContainerUpdateCompleted(pod *v1.Pod, container *v1.Container, containerStatus *v1.ContainerStatus, lastContainerStatus appspub.InPlaceUpdateContainerStatus) bool
	// To determine whether the pod has been successfully vertical updated
	IsPodUpdateCompleted(pod *v1.Pod) bool
}

var verticalUpdateOperator VerticalUpdateInterface = nil

// To register vertical update operations,
// you can register different vertical update implementations here
func registerVerticalUpdate() {
	if verticalUpdateOperator == nil {
		verticalUpdateOperator = &VerticalUpdate{}
	}
}

// VerticalUpdate represents the vertical scaling of k8s standard
type VerticalUpdate struct{}

var _ VerticalUpdateInterface = &VerticalUpdate{}

// Get the resource status from the container and synchronize it to state
func (v *VerticalUpdate) SyncContainerResource(container *v1.ContainerStatus, state *appspub.InPlaceUpdateState) {
	// TODO(LavenderQAQ): Need to write the status synchronization module after api upgrade
}

// UpdateResource implements vertical updates by directly modifying the container's resources,
// conforming to the k8s community standard
func (v *VerticalUpdate) UpdateContainerResource(container *v1.Container, newResource *v1.ResourceRequirements) {
	for key, quantity := range newResource.Limits {
		container.Resources.Limits[key] = quantity
	}
	for key, quantity := range newResource.Requests {
		container.Resources.Requests[key] = quantity
	}
}

// Get the resource status from the pod and synchronize it to state
func (v *VerticalUpdate) SyncPodResource(pod *v1.Pod, state *appspub.InPlaceUpdateState) {
	// TODO(LavenderQAQ): Need to write the status synchronization module after api upgrade
}

// For the community-standard vertical scale-down implementation,
// there is no need to do anything here because the container has already been updated in the UpdateContainerResource interface
func (v *VerticalUpdate) UpdatePodResource(pod *v1.Pod) {
	return
}

// IsUpdateCompleted directly determines whether the current container is vertically updated by the spec and status of the container,
// which conforms to the k8s community standard
func (v *VerticalUpdate) IsContainerUpdateCompleted(pod *v1.Pod, container *v1.Container, containerStatus *v1.ContainerStatus, lastContainerStatus appspub.InPlaceUpdateContainerStatus) bool {
	return true
}

// IsUpdateCompleted directly determines whether the current pod is vertically updated by the spec and status of the container,
// which conforms to the k8s community standard
func (v *VerticalUpdate) IsPodUpdateCompleted(pod *v1.Pod) bool {
	return true
}

// Internal implementation of vertical updates
// type VerticalUpdateInternal struct{}

// var _ VerticalUpdateInterface = &VerticalUpdateInternal{}
