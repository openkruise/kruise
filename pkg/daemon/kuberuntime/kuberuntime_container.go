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
	"fmt"
	"sort"
	"strings"
	"time"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	runtimeapi "k8s.io/cri-api/pkg/apis/runtime/v1"
	"k8s.io/klog/v2"
	kubeletcontainer "k8s.io/kubernetes/pkg/kubelet/container"
	"k8s.io/kubernetes/pkg/kubelet/events"
	kubelettypes "k8s.io/kubernetes/pkg/kubelet/types"
	"k8s.io/kubernetes/pkg/kubelet/util/format"
)

// recordContainerEvent should be used by the runtime manager for all container related events.
// it has sanity checks to ensure that we do not write events that can abuse our masters.
// in particular, it ensures that a containerID never appears in an event message as that
// is prone to causing a lot of distinct events that do not count well.
// it replaces any reference to a containerID with the containerName which is stable, and is what users know.
func (m *genericRuntimeManager) recordContainerEvent(pod *v1.Pod, container *v1.Container, containerID, eventType, reason, message string, args ...interface{}) {
	ref, err := kubeletcontainer.GenerateContainerRef(pod, container)
	if err != nil {
		klog.ErrorS(err, "Can't make a ref to pod container", "pod", format.Pod(pod), "container", container.Name)
		return
	}
	eventMessage := message
	if len(args) > 0 {
		eventMessage = fmt.Sprintf(message, args...)
	}
	// this is a hack, but often the error from the runtime includes the containerID
	// which kills our ability to deduplicate events.  this protection makes a huge
	// difference in the number of unique events
	if containerID != "" {
		eventMessage = strings.Replace(eventMessage, containerID, container.Name, -1)
	}
	m.recorder.Event(ref, eventType, reason, eventMessage)
}

// getPodContainerStatuses gets all containers' statuses for the pod.
func (m *genericRuntimeManager) getPodContainerStatuses(uid types.UID, name, namespace string) ([]*kubeletcontainer.Status, error) {
	// Select all containers of the given pod.
	containers, err := m.runtimeService.ListContainers(context.TODO(), &runtimeapi.ContainerFilter{
		LabelSelector: map[string]string{kubelettypes.KubernetesPodUIDLabel: string(uid)},
	})
	if err != nil {
		return nil, fmt.Errorf("run ListContainers error: %v", err)
	}
	statuses := make([]*kubeletcontainer.Status, len(containers))
	for i, c := range containers {
		status, err := m.runtimeService.ContainerStatus(context.TODO(), c.Id, false)
		if err != nil {
			return nil, fmt.Errorf("run ContainerStatus for %s error: %v", c.Id, err)
		}
		cStatus := toKubeContainerStatus(status.Status, m.runtimeName)
		// TODO: Populate the termination message if needed.
		statuses[i] = cStatus
	}

	sort.Sort(containerStatusByCreated(statuses))
	return statuses, nil
}

func toKubeContainerStatus(status *runtimeapi.ContainerStatus, runtimeName string) *kubeletcontainer.Status {
	annotatedInfo := getContainerInfoFromAnnotations(status.Annotations)
	labeledInfo := getContainerInfoFromLabels(status.Labels)
	cStatus := &kubeletcontainer.Status{
		ID: kubeletcontainer.ContainerID{
			Type: runtimeName,
			ID:   status.Id,
		},
		Name:                 labeledInfo.ContainerName,
		Image:                status.Image.Image,
		ImageID:              status.ImageRef,
		Hash:                 annotatedInfo.Hash,
		HashWithoutResources: annotatedInfo.HashWithoutResources,
		RestartCount:         annotatedInfo.RestartCount,
		State:                toKubeContainerState(status.State),
		CreatedAt:            time.Unix(0, status.CreatedAt),
	}

	if status.State != runtimeapi.ContainerState_CONTAINER_CREATED {
		// If container is not in the created state, we have tried and
		// started the container. Set the StartedAt time.
		cStatus.StartedAt = time.Unix(0, status.StartedAt)
	}
	if status.State == runtimeapi.ContainerState_CONTAINER_EXITED {
		cStatus.Reason = status.Reason
		cStatus.Message = status.Message
		cStatus.ExitCode = int(status.ExitCode)
		cStatus.FinishedAt = time.Unix(0, status.FinishedAt)
	}
	return cStatus
}

// RunInContainer synchronously executes the command in the container, and returns the output.
func (m *genericRuntimeManager) RunInContainer(ctx context.Context, id kubeletcontainer.ContainerID, cmd []string, timeout time.Duration) ([]byte, error) {
	stdout, stderr, err := m.runtimeService.ExecSync(context.TODO(), id.ID, cmd, timeout)
	// NOTE(tallclair): This does not correctly interleave stdout & stderr, but should be sufficient
	// for logging purposes. A combined output option will need to be added to the ExecSyncRequest
	// if more precise output ordering is ever required.
	return append(stdout, stderr...), err
}

// KillContainer kills a container through the following steps:
// * Run the pre-stop lifecycle hooks (if applicable).
// * Stop the container.
func (m *genericRuntimeManager) KillContainer(pod *v1.Pod, containerID kubeletcontainer.ContainerID, containerName string, message string, gracePeriodOverride *int64) error {
	var containerSpec *v1.Container
	if pod != nil {
		if containerSpec = kubeletcontainer.GetContainerSpec(pod, containerName); containerSpec == nil {
			return fmt.Errorf("failed to get containerSpec %q(id=%q) in pod %q when killing container for reason %q",
				containerName, containerID.String(), format.Pod(pod), message)
		}
	} else {
		// Restore necessary information if one of the specs is nil.
		restoredPod, restoredContainer, err := m.restoreSpecsFromContainerLabels(containerID)
		if err != nil {
			return err
		}
		pod, containerSpec = restoredPod, restoredContainer
	}

	if len(message) == 0 {
		message = fmt.Sprintf("Stopping container %s", containerSpec.Name)
	}
	m.recordContainerEvent(pod, containerSpec, containerID.ID, v1.EventTypeNormal, events.KillingContainer, message)

	// From this point, pod and container must be non-nil.
	gracePeriod := int64(minimumGracePeriodInSeconds)
	switch {
	case pod.DeletionGracePeriodSeconds != nil:
		gracePeriod = *pod.DeletionGracePeriodSeconds
	case pod.Spec.TerminationGracePeriodSeconds != nil:
		gracePeriod = *pod.Spec.TerminationGracePeriodSeconds
	}

	// Run the pre-stop lifecycle hooks if applicable and if there is enough time to run it
	if containerSpec.Lifecycle != nil && containerSpec.Lifecycle.PreStop != nil && gracePeriod > 0 {
		gracePeriod = gracePeriod - m.executePreStopHook(pod, containerID, containerSpec, gracePeriod)
	}
	// always give containers a minimal shutdown window to avoid unnecessary SIGKILLs
	if gracePeriod < minimumGracePeriodInSeconds {
		gracePeriod = minimumGracePeriodInSeconds
	}

	if gracePeriodOverride != nil {
		gracePeriod = *gracePeriodOverride
		klog.V(3).InfoS("Killing container, but using grace period override", "containerID", containerID.String(), "gracePeriod", gracePeriod)
	}

	klog.V(2).InfoS("Killing container with grace period", "containerID", containerID.String(), "gracePeriod", gracePeriod)

	err := m.runtimeService.StopContainer(context.TODO(), containerID.ID, gracePeriod)
	if err != nil {
		klog.ErrorS(err, "Container termination failed with grace period", "containerID", containerID.String(), "gracePeriod", gracePeriod)
	} else {
		klog.V(3).InfoS("Container exited normally", "containerID", containerID.String())
	}

	return err
}

// restoreSpecsFromContainerLabels restores all information needed for killing a container. In some
// case we may not have pod and container spec when killing a container, e.g. pod is deleted during
// kubelet restart.
// To solve this problem, we've already written necessary information into container labels. Here we
// just need to retrieve them from container labels and restore the specs.
// TODO(random-liu): Add a node e2e test to test this behaviour.
// TODO(random-liu): Change the lifecycle handler to just accept information needed, so that we can
// just pass the needed function not create the fake object.
func (m *genericRuntimeManager) restoreSpecsFromContainerLabels(containerID kubeletcontainer.ContainerID) (*v1.Pod, *v1.Container, error) {
	var pod *v1.Pod
	var container *v1.Container
	s, err := m.runtimeService.ContainerStatus(context.TODO(), containerID.ID, false)
	if err != nil {
		return nil, nil, err
	}

	l := getContainerInfoFromLabels(s.Status.Labels)
	a := getContainerInfoFromAnnotations(s.Status.Annotations)
	// Notice that the followings are not full spec. The container killing code should not use
	// un-restored fields.
	pod = &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			UID:                        l.PodUID,
			Name:                       l.PodName,
			Namespace:                  l.PodNamespace,
			DeletionGracePeriodSeconds: a.PodDeletionGracePeriod,
		},
		Spec: v1.PodSpec{
			TerminationGracePeriodSeconds: a.PodTerminationGracePeriod,
		},
	}
	container = &v1.Container{
		Name:                   l.ContainerName,
		Ports:                  a.ContainerPorts,
		TerminationMessagePath: a.TerminationMessagePath,
	}
	if a.PreStopHandler != nil {
		container.Lifecycle = &v1.Lifecycle{
			PreStop: a.PreStopHandler,
		}
	}
	return pod, container, nil
}

// executePreStopHook runs the pre-stop lifecycle hooks if applicable and returns the duration it takes.
func (m *genericRuntimeManager) executePreStopHook(pod *v1.Pod, containerID kubeletcontainer.ContainerID, containerSpec *v1.Container, gracePeriod int64) int64 {
	klog.V(3).InfoS("Running preStop hook for container", "containerID", containerID.String())

	start := metav1.Now()
	done := make(chan struct{})
	go func() {
		defer close(done)
		defer utilruntime.HandleCrash()
		if msg, err := m.runner.Run(context.TODO(), containerID, pod, containerSpec, containerSpec.Lifecycle.PreStop); err != nil {
			klog.ErrorS(err, "preStop hook for container failed", "name", containerSpec.Name)
			m.recordContainerEvent(pod, containerSpec, containerID.ID, v1.EventTypeWarning, events.FailedPreStopHook, msg)
		}
	}()

	select {
	case <-time.After(time.Duration(gracePeriod) * time.Second):
		klog.V(2).InfoS("preStop hook for container did not complete in time", "containerID", containerID.String(), "gracePeriod", gracePeriod)
	case <-done:
		klog.V(3).InfoS("preStop hook for container completed", "containerID", containerID.String())
	}

	return int64(metav1.Now().Sub(start.Time).Seconds())
}
