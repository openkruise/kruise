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
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/appscode/jsonpatch"
	"github.com/google/go-cmp/cmp"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/kubernetes/pkg/apis/core/v1/helper/qos"
)

// interface for In-place workload vertical scaling
type VerticalUpdateInterface interface {
	// UpdateInplaceUpdateMetadata validates and applies the resource patch to the UpdateSpec.
	UpdateInplaceUpdateMetadata(op *jsonpatch.Operation, oldTemp *v1.PodTemplateSpec, updateSpec *UpdateSpec) error

	// IsPodQoSChanged check whether the pod qos has changed
	IsPodQoSChanged(oldTemp, newTemp *v1.PodTemplateSpec) bool

	// UpdateResource some or all containers of a pod can be updated at once within this interface.
	// container or pod level resources can be updated by this interface
	UpdateResource(pod *v1.Pod, expectedResources map[string]*v1.ResourceRequirements)
	// IsUpdateCompleted To determine whether the pod has been successfully vertical updated
	IsUpdateCompleted(pod *v1.Pod) (bool, error)
}

var verticalUpdateImpl VerticalUpdateInterface = nil

// To register vertical update operations,
// you can register different vertical update implementations here
func init() {
	// Now, we assume that there is a single standard per cluster, so register in init()
	// TODO(Abner-1): Perhaps we should dynamically select the verticalUpdateImpl based on the pod metadata being processed.
	// give us more suggestions if you need
	if verticalUpdateImpl == nil {
		verticalUpdateImpl = &NativeVerticalUpdate{}
	}
}

// NativeVerticalUpdate represents the vertical scaling of k8s standard
type NativeVerticalUpdate struct{}

var _ VerticalUpdateInterface = &NativeVerticalUpdate{}

func (v *NativeVerticalUpdate) UpdateInplaceUpdateMetadata(op *jsonpatch.Operation, oldTemp *v1.PodTemplateSpec, updateSpec *UpdateSpec) error {
	// for example: /spec/containers/0/resources/limits/cpu
	words := strings.Split(op.Path, "/")
	if len(words) != 7 {
		return fmt.Errorf("invalid resource path: %s", op.Path)
	}
	idx, err := strconv.Atoi(words[3])
	if err != nil || len(oldTemp.Spec.Containers) <= idx {
		return fmt.Errorf("invalid container index: %s", op.Path)
	}
	if op.Operation == "remove" || op.Operation == "add" {
		// Before k8s 1.32, we can not resize resources for a container with no limit or request
		// TODO(Abner-1) change it if 1.32 released and allowing this operation
		return errors.New("can not add or remove resources")
	}

	if op.Value == nil {
		return errors.New("json patch value is nil")
	}
	quantity, err := resource.ParseQuantity(op.Value.(string))
	if err != nil {
		return fmt.Errorf("parse quantity error: %v", err)
	}

	if !v.CanResourcesResizeInPlace(words[6]) {
		return fmt.Errorf("disallowed inplace update resource: %s", words[6])
	}

	cName := oldTemp.Spec.Containers[idx].Name
	if _, ok := updateSpec.ContainerResources[cName]; !ok {
		updateSpec.ContainerResources[cName] = v1.ResourceRequirements{
			Limits:   make(v1.ResourceList),
			Requests: make(v1.ResourceList),
		}
	}
	switch words[5] {
	case "limits":
		updateSpec.ContainerResources[cName].Limits[v1.ResourceName(words[6])] = quantity
	case "requests":
		updateSpec.ContainerResources[cName].Requests[v1.ResourceName(words[6])] = quantity
	}
	return nil
}

func (v *NativeVerticalUpdate) IsPodQoSChanged(oldTemp, newTemp *v1.PodTemplateSpec) bool {
	oldPod := &v1.Pod{
		Spec: oldTemp.Spec,
	}
	newPod := &v1.Pod{
		Spec: newTemp.Spec,
	}
	if qos.GetPodQOS(oldPod) != qos.GetPodQOS(newPod) {
		return true
	}
	return false
}

// updateContainerResource implements vertical updates by directly modifying the container's resources,
// conforming to the k8s community standard
func (v *NativeVerticalUpdate) updateContainerResource(container *v1.Container, newResource *v1.ResourceRequirements) {
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

// isContainerUpdateCompleted directly determines whether the current container is vertically updated by the spec and status of the container,
// which conforms to the k8s community standard
func (v *NativeVerticalUpdate) isContainerUpdateCompleted(container *v1.Container, containerStatus *v1.ContainerStatus) bool {
	if containerStatus == nil || containerStatus.Resources == nil || container == nil {
		return false
	}
	if !cmp.Equal(container.Resources.Limits, containerStatus.Resources.Limits) ||
		!cmp.Equal(container.Resources.Requests, containerStatus.Resources.Requests) {
		return false
	}
	return true
}

func (v *NativeVerticalUpdate) UpdateResource(pod *v1.Pod, expectedResources map[string]*v1.ResourceRequirements) {
	if len(expectedResources) == 0 {
		// pod level hook, ignore in native implementation
		return
	}
	for i := range pod.Spec.Containers {
		c := &pod.Spec.Containers[i]
		newResource, resourceExists := expectedResources[c.Name]
		if !resourceExists {
			continue
		}
		v.updateContainerResource(c, newResource)
	}
	return
}

func (v *NativeVerticalUpdate) IsUpdateCompleted(pod *v1.Pod) (bool, error) {
	containers := make(map[string]*v1.Container, len(pod.Spec.Containers))
	for i := range pod.Spec.Containers {
		c := &pod.Spec.Containers[i]
		containers[c.Name] = c
	}
	if len(pod.Status.ContainerStatuses) != len(containers) {
		return false, fmt.Errorf("some container status is not reported")
	}
	for _, cs := range pod.Status.ContainerStatuses {
		if !v.isContainerUpdateCompleted(containers[cs.Name], &cs) {
			return false, fmt.Errorf("container %s resources not changed", cs.Name)
		}
	}
	return true, nil
}

// only cpu and memory are allowed to be inplace updated
var allowedResizeResourceKey = map[string]bool{
	string(v1.ResourceCPU):    true,
	string(v1.ResourceMemory): true,
}

func (v *NativeVerticalUpdate) CanResourcesResizeInPlace(resourceKey string) bool {
	allowed, exist := allowedResizeResourceKey[resourceKey]
	return exist && allowed
}

// Internal implementation of vertical updates
// type VerticalUpdateInternal struct{}

// var _ VerticalUpdateInterface = &VerticalUpdateInternal{}
