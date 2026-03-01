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
	"context"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	criapi "k8s.io/cri-api/pkg/apis"
	kubeletcontainer "k8s.io/kubernetes/pkg/kubelet/container"
	kubeletlifecycle "k8s.io/kubernetes/pkg/kubelet/lifecycle"
	kubelettypes "k8s.io/kubernetes/pkg/kubelet/types"
)

const (
	minimumGracePeriodInSeconds = 2
)

type Runtime interface {
	// GetPodStatus retrieves the status of the pod, including the
	// information of all containers in the pod that are visible in Runtime.
	GetPodStatus(ctx context.Context, uid types.UID, name, namespace string) (*kubeletcontainer.PodStatus, error)
	// KillContainer kills a container through the following steps:
	// * Run the pre-stop lifecycle hooks (if applicable).
	// * Stop the container.
	KillContainer(pod *v1.Pod, containerID kubeletcontainer.ContainerID, containerName string, message string, gracePeriodOverride *int64) error
}

func NewGenericRuntime(
	runtimeName string,
	runtimeService criapi.RuntimeService,
	recorder record.EventRecorder,
	httpClient kubelettypes.HTTPDoer,
) Runtime {
	kubeRuntimeManager := &genericRuntimeManager{
		runtimeName:    runtimeName,
		runtimeService: runtimeService,
		recorder:       recorder,
	}
	kubeRuntimeManager.runner = kubeletlifecycle.NewHandlerRunner(httpClient, kubeRuntimeManager, kubeRuntimeManager, recorder)

	return kubeRuntimeManager
}

type genericRuntimeManager struct {
	runtimeName    string
	runtimeService criapi.RuntimeService
	recorder       record.EventRecorder

	// Runner of lifecycle events.
	runner kubeletcontainer.HandlerRunner
}

// GetPodStatus retrieves the status of the pod, including the
// information of all containers in the pod that are visible in Runtime.
func (m *genericRuntimeManager) GetPodStatus(ctx context.Context, uid types.UID, name, namespace string) (*kubeletcontainer.PodStatus, error) {
	// TODO: get Pod IPs from sandbox container if needed

	// Get statuses of all containers visible in the pod.
	containerStatuses, err := m.getPodContainerStatuses(uid, name, namespace)
	if err != nil {
		return nil, err
	}

	return &kubeletcontainer.PodStatus{
		ID:                uid,
		Name:              name,
		Namespace:         namespace,
		ContainerStatuses: containerStatuses,
		//IPs:               podIPs,
		//SandboxStatuses:   sandboxStatuses,
	}, nil
}
