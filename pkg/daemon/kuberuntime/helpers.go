/*
Copyright 2021 The Kruise Authors.
Copyright 2016 The Kubernetes Authors.

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

package kuberuntime

import (
	runtimeapi "k8s.io/cri-api/pkg/apis/runtime/v1"
	kubeletcontainer "k8s.io/kubernetes/pkg/kubelet/container"
)

type containerStatusByCreated []*kubeletcontainer.Status

func (c containerStatusByCreated) Len() int           { return len(c) }
func (c containerStatusByCreated) Swap(i, j int)      { c[i], c[j] = c[j], c[i] }
func (c containerStatusByCreated) Less(i, j int) bool { return c[i].CreatedAt.After(c[j].CreatedAt) }

// toKubeContainerState converts runtimeapi.ContainerState to kubecontainer.ContainerState.
func toKubeContainerState(state runtimeapi.ContainerState) kubeletcontainer.State {
	switch state {
	case runtimeapi.ContainerState_CONTAINER_CREATED:
		return kubeletcontainer.ContainerStateCreated
	case runtimeapi.ContainerState_CONTAINER_RUNNING:
		return kubeletcontainer.ContainerStateRunning
	case runtimeapi.ContainerState_CONTAINER_EXITED:
		return kubeletcontainer.ContainerStateExited
	case runtimeapi.ContainerState_CONTAINER_UNKNOWN:
		return kubeletcontainer.ContainerStateUnknown
	}

	return kubeletcontainer.ContainerStateUnknown
}
