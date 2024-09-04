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
	"fmt"
	"strconv"
	"strings"

	"github.com/appscode/jsonpatch"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"

	appspub "github.com/openkruise/kruise/apis/apps/pub"
)

// interface for In-place workload vertical scaling
type VerticalUpdateInterface interface {
	// ValidateResourcePatch validates and applies the resource patch to the UpdateSpec.
	ValidateResourcePatch(op *jsonpatch.Operation, oldTemp *v1.PodTemplateSpec, updateSpec *UpdateSpec) error

	// interface for container level vertical scaling api, such as k8s 1.27+

	// SyncContainerResource Get the expected resource values of the container and its current status
	SyncContainerResource(container *v1.ContainerStatus, state *appspub.InPlaceUpdateState)
	// UpdateContainerResource Pass in the container to be modified and the expected resource values.
	UpdateContainerResource(container *v1.Container, resource *v1.ResourceRequirements)
	// IsContainerUpdateCompleted To determine whether the container has been successfully vertical updated
	IsContainerUpdateCompleted(pod *v1.Pod, container *v1.Container, containerStatus *v1.ContainerStatus, lastContainerStatus appspub.InPlaceUpdateContainerStatus) bool

	// interface for pod level vertical scaling api

	// SyncPodResource Get the expected resource values of all containers in the pod and their current status
	SyncPodResource(pod *v1.Pod, state *appspub.InPlaceUpdateState)
	// UpdatePodResource All containers of a pod can be updated at once within this interface.
	UpdatePodResource(pod *v1.Pod)
	// IsPodUpdateCompleted To determine whether the pod has been successfully vertical updated
	IsPodUpdateCompleted(pod *v1.Pod) bool
}

var verticalUpdateOperator VerticalUpdateInterface = nil

// To register vertical update operations,
// you can register different vertical update implementations here
func init() {
	// Now, we assume that there is a single standard per cluster, so register in init()
	// TODO(Abner-1): Perhaps we should dynamically select the verticalUpdateOperator based on the pod metadata being processed.
	// give us more suggestions if you need
	if verticalUpdateOperator == nil {
		verticalUpdateOperator = &VerticalUpdate{}
	}
}

// VerticalUpdate represents the vertical scaling of k8s standard
type VerticalUpdate struct{}

var _ VerticalUpdateInterface = &VerticalUpdate{}

func (v *VerticalUpdate) ValidateResourcePatch(op *jsonpatch.Operation, oldTemp *v1.PodTemplateSpec, updateSpec *UpdateSpec) error {
	// for example: /spec/containers/0/resources/limits/cpu
	words := strings.Split(op.Path, "/")
	if len(words) != 7 {
		return fmt.Errorf("invalid resource path: %s", op.Path)
	}
	idx, err := strconv.Atoi(words[3])
	if err != nil || len(oldTemp.Spec.Containers) <= idx {
		return fmt.Errorf("invalid container index: %s", op.Path)
	}
	quantity, err := resource.ParseQuantity(op.Value.(string))
	if err != nil {
		return fmt.Errorf("parse quantity error: %v", err)
	}

	if !v.CanResourcesResizeInPlace(words[6]) {
		return fmt.Errorf("disallowed inplace update resouece: %s", words[6])
	}

	if _, ok := updateSpec.ContainerResources[oldTemp.Spec.Containers[idx].Name]; !ok {
		updateSpec.ContainerResources[oldTemp.Spec.Containers[idx].Name] = v1.ResourceRequirements{
			Limits:   make(v1.ResourceList),
			Requests: make(v1.ResourceList),
		}
	}
	switch words[5] {
	case "limits":
		updateSpec.ContainerResources[oldTemp.Spec.Containers[idx].Name].Limits[v1.ResourceName(words[6])] = quantity
	case "requests":
		updateSpec.ContainerResources[oldTemp.Spec.Containers[idx].Name].Requests[v1.ResourceName(words[6])] = quantity
	}
	return nil
}

// Get the resource status from the container and synchronize it to state
func (v *VerticalUpdate) SyncContainerResource(container *v1.ContainerStatus, state *appspub.InPlaceUpdateState) {
	if container == nil {
		return
	}

	if state.LastContainerStatuses == nil {
		state.LastContainerStatuses = make(map[string]appspub.InPlaceUpdateContainerStatus)
	}
	c := state.LastContainerStatuses[container.Name]
	if state.LastContainerStatuses[container.Name].Resources.Limits == nil {
		c.Resources.Limits = make(map[v1.ResourceName]resource.Quantity)
	}
	if state.LastContainerStatuses[container.Name].Resources.Requests == nil {
		c.Resources.Requests = make(map[v1.ResourceName]resource.Quantity)
	}

	if container.Resources != nil {
		for key, quantity := range container.Resources.Limits {
			if !v.CanResourcesResizeInPlace(string(key)) {
				continue
			}
			c.Resources.Limits[key] = quantity
		}
		for key, quantity := range container.Resources.Requests {
			if !v.CanResourcesResizeInPlace(string(key)) {
				continue
			}
			c.Resources.Requests[key] = quantity
		}
	}
	// Store container infos whether c is empty or not.
	// It will be used in IsContainerUpdateCompleted to check container resources is updated.
	// case: pending pod can also be resize resource
	state.LastContainerStatuses[container.Name] = c
}

// UpdateResource implements vertical updates by directly modifying the container's resources,
// conforming to the k8s community standard
func (v *VerticalUpdate) UpdateContainerResource(container *v1.Container, newResource *v1.ResourceRequirements) {
	if container == nil || newResource == nil {
		return
	}
	for key, quantity := range newResource.Limits {
		if !v.CanResourcesResizeInPlace(string(key)) {
			continue
		}
		container.Resources.Limits[key] = quantity
	}
	for key, quantity := range newResource.Requests {
		if !v.CanResourcesResizeInPlace(string(key)) {
			continue
		}
		container.Resources.Requests[key] = quantity
	}
}

// IsContainerUpdateCompleted directly determines whether the current container is vertically updated by the spec and status of the container,
// which conforms to the k8s community standard
//
// lstatus: lastContainerStatus record last resource：last container status => last container spec
// cspec: container spec
// cstatus: container status
//
//	lstatus   cspec  cstatus
//
// 1. exist   exist   exist  => compare cspec and cstatus (important)
// 2. exist   empty   nil    => change qos is forbidden
// 3. exist   empty   empty  => change qos is forbidden
// 4. exist   empty   exist  => change qos is forbidden
// 5. empty   empty   nil    => this container is not inplace-updated by kruise, ignore
// 6. empty   exist   exist  => this container is not inplace-updated by kruise, ignore
// 7. empty   exist   exist  => this container is not inplace-updated by kruise, ignore
func (v *VerticalUpdate) IsContainerUpdateCompleted(pod *v1.Pod, container *v1.Container, containerStatus *v1.ContainerStatus, lastContainerStatus appspub.InPlaceUpdateContainerStatus) bool {
	// case 5-7
	if lastContainerStatus.Resources.Limits == nil && lastContainerStatus.Resources.Requests == nil {
		return true
	}
	if lastContainerStatus.Resources.Limits != nil {
		// case 2-4 limits
		if container.Resources.Limits == nil || containerStatus == nil ||
			containerStatus.Resources == nil || containerStatus.Resources.Limits == nil {
			return false
		}
		// case 1
		for name, value := range container.Resources.Limits {
			// ignore resources key which can not inplace resize
			if !v.CanResourcesResizeInPlace(string(name)) {
				continue
			}
			if !value.Equal(containerStatus.Resources.Limits[name]) {
				return false
			}
		}
	}

	if lastContainerStatus.Resources.Requests != nil {
		// case 2-4 requests
		if container.Resources.Requests == nil || containerStatus == nil ||
			containerStatus.Resources == nil || containerStatus.Resources.Requests == nil {
			return false
		}
		// case 1
		for name, value := range container.Resources.Requests {
			if !v.CanResourcesResizeInPlace(string(name)) {
				continue
			}
			if !value.Equal(containerStatus.Resources.Requests[name]) {
				return false
			}
		}
	}
	return true
}

// Get the resource status from the pod and synchronize it to state
func (v *VerticalUpdate) SyncPodResource(pod *v1.Pod, state *appspub.InPlaceUpdateState) {
}

// For the community-standard vertical scale-down implementation,
// there is no need to do anything here because the container has already been updated in the UpdateContainerResource interface
func (v *VerticalUpdate) UpdatePodResource(pod *v1.Pod) {
	return
}

// IsUpdateCompleted directly determines whether the current pod is vertically updated by the spec and status of the container,
// which conforms to the k8s community standard
func (v *VerticalUpdate) IsPodUpdateCompleted(pod *v1.Pod) bool {
	return true
}

// only cpu and memory are allowed to be inplace updated
var allowedResizeResourceKey = map[string]bool{
	string(v1.ResourceCPU):    true,
	string(v1.ResourceMemory): true,
}

func (v *VerticalUpdate) CanResourcesResizeInPlace(resourceKey string) bool {
	allowed, exist := allowedResizeResourceKey[resourceKey]
	return exist && allowed
}

// Internal implementation of vertical updates
// type VerticalUpdateInternal struct{}

// var _ VerticalUpdateInterface = &VerticalUpdateInternal{}
