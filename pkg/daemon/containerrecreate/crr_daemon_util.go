/*
Copyright 2021 The Kruise Authors.

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

package containerrecreate

import (
	"encoding/json"
	"fmt"
	"time"

	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/klog/v2"
	kubeletcontainer "k8s.io/kubernetes/pkg/kubelet/container"
	utilpointer "k8s.io/utils/pointer"
)

const (
	// CRRPodNameIndex is the lookup name for the most comment index function, which is to index by the crr.spec.podName field.
	CRRPodNameIndex = "podName"
)

type crrListByPhaseAndCreated []*appsv1alpha1.ContainerRecreateRequest

func (c crrListByPhaseAndCreated) Len() int      { return len(c) }
func (c crrListByPhaseAndCreated) Swap(i, j int) { c[i], c[j] = c[j], c[i] }
func (c crrListByPhaseAndCreated) Less(i, j int) bool {
	if c[i].Status.Phase == c[j].Status.Phase {
		return c[i].CreationTimestamp.Before(&c[j].CreationTimestamp)
	}
	return c.phaseWeight(c[i].Status.Phase) < c.phaseWeight(c[j].Status.Phase)
}

func (c crrListByPhaseAndCreated) phaseWeight(phase appsv1alpha1.ContainerRecreateRequestPhase) int {
	switch phase {
	case appsv1alpha1.ContainerRecreateRequestCompleted:
		return 1
	case appsv1alpha1.ContainerRecreateRequestRecreating:
		return 2
	case appsv1alpha1.ContainerRecreateRequestPending:
		return 3
	}
	return 4
}

func getCurrentCRRContainersRecreateStates(
	crr *appsv1alpha1.ContainerRecreateRequest,
	podStatus *kubeletcontainer.PodStatus,
) []appsv1alpha1.ContainerRecreateRequestContainerRecreateState {

	var minStartedDuration time.Duration
	if crr.Spec.Strategy != nil {
		minStartedDuration = time.Duration(crr.Spec.Strategy.MinStartedSeconds) * time.Second
	}

	syncContainerStatuses := getCRRSyncContainerStatuses(crr)
	var statuses []appsv1alpha1.ContainerRecreateRequestContainerRecreateState

	for i := range crr.Spec.Containers {
		c := &crr.Spec.Containers[i]
		previousContainerRecreateState := getCRRContainerRecreateState(crr, c.Name)
		if previousContainerRecreateState != nil &&
			(previousContainerRecreateState.Phase == appsv1alpha1.ContainerRecreateRequestFailed ||
				previousContainerRecreateState.Phase == appsv1alpha1.ContainerRecreateRequestSucceeded) {
			statuses = append(statuses, *previousContainerRecreateState)
			continue
		}

		syncContainerStatus := syncContainerStatuses[c.Name]
		kubeContainerStatus := podStatus.FindContainerStatusByName(c.Name)

		var currentState appsv1alpha1.ContainerRecreateRequestContainerRecreateState

		if kubeContainerStatus == nil {
			// not found the real container
			currentState = appsv1alpha1.ContainerRecreateRequestContainerRecreateState{
				Name:     c.Name,
				Phase:    appsv1alpha1.ContainerRecreateRequestPending,
				IsKilled: getPreviousContainerKillState(previousContainerRecreateState),
				Message:  "not found container on Node",
			}

		} else if kubeContainerStatus.State == kubeletcontainer.ContainerStateExited {
			// for no-running state, we consider it will be recreated or restarted soon
			currentState = appsv1alpha1.ContainerRecreateRequestContainerRecreateState{
				Name:     c.Name,
				Phase:    appsv1alpha1.ContainerRecreateRequestRecreating,
				IsKilled: getPreviousContainerKillState(previousContainerRecreateState),
			}
		} else if crr.Spec.Strategy.ForceRecreate && (previousContainerRecreateState == nil || !previousContainerRecreateState.IsKilled) {
			// for forceKill scenarios, when the previous recreate state is empty or has not been killed, the current restart requirement will be set immediately
			currentState = appsv1alpha1.ContainerRecreateRequestContainerRecreateState{
				Name:  c.Name,
				Phase: appsv1alpha1.ContainerRecreateRequestPending,
			}
		} else if kubeContainerStatus.ID.String() != c.StatusContext.ContainerID ||
			kubeContainerStatus.RestartCount > int(c.StatusContext.RestartCount) ||
			kubeContainerStatus.StartedAt.After(crr.CreationTimestamp.Time) {
			// already recreated or restarted
			currentState = appsv1alpha1.ContainerRecreateRequestContainerRecreateState{
				Name:     c.Name,
				Phase:    appsv1alpha1.ContainerRecreateRequestRecreating,
				IsKilled: getPreviousContainerKillState(previousContainerRecreateState),
			}
			if syncContainerStatus != nil &&
				syncContainerStatus.ContainerID == kubeContainerStatus.ID.String() &&
				time.Since(kubeContainerStatus.StartedAt) > minStartedDuration &&
				syncContainerStatus.Ready {
				currentState.Phase = appsv1alpha1.ContainerRecreateRequestSucceeded
			}
		} else {
			currentState = appsv1alpha1.ContainerRecreateRequestContainerRecreateState{
				Name:     c.Name,
				Phase:    appsv1alpha1.ContainerRecreateRequestPending,
				IsKilled: getPreviousContainerKillState(previousContainerRecreateState),
			}
		}

		statuses = append(statuses, currentState)
	}

	return statuses
}

func getPreviousContainerKillState(previousContainerRecreateState *appsv1alpha1.ContainerRecreateRequestContainerRecreateState) bool {
	if previousContainerRecreateState == nil {
		return false
	}
	return previousContainerRecreateState.IsKilled
}

func getCRRContainerRecreateState(crr *appsv1alpha1.ContainerRecreateRequest, name string) *appsv1alpha1.ContainerRecreateRequestContainerRecreateState {
	for i := range crr.Status.ContainerRecreateStates {
		c := &crr.Status.ContainerRecreateStates[i]
		if c.Name == name {
			return c
		}
	}
	return nil
}

func getCRRSyncContainerStatuses(crr *appsv1alpha1.ContainerRecreateRequest) map[string]*appsv1alpha1.ContainerRecreateRequestSyncContainerStatus {
	str := crr.Annotations[appsv1alpha1.ContainerRecreateRequestSyncContainerStatusesKey]
	if str == "" {
		return nil
	}
	var syncContainerStatuses []appsv1alpha1.ContainerRecreateRequestSyncContainerStatus
	if err := json.Unmarshal([]byte(str), &syncContainerStatuses); err != nil {
		klog.ErrorS(err, "Failed to unmarshal CRR syncContainerStatuses", "namespace", crr.Namespace, "name", crr.Name, "rawString", str)
		return nil
	}

	statuses := make(map[string]*appsv1alpha1.ContainerRecreateRequestSyncContainerStatus, len(syncContainerStatuses))
	for i := range syncContainerStatuses {
		c := &syncContainerStatuses[i]
		statuses[c.Name] = c
	}
	return statuses
}

func convertCRRToPod(crr *appsv1alpha1.ContainerRecreateRequest) *v1.Pod {
	podName := crr.Spec.PodName
	podUID := types.UID(crr.Labels[appsv1alpha1.ContainerRecreateRequestPodUIDKey])

	pod := &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: crr.Namespace,
			Name:      podName,
			UID:       podUID,
		},
		Spec: v1.PodSpec{
			TerminationGracePeriodSeconds: crr.Spec.Strategy.TerminationGracePeriodSeconds,
		},
	}

	if pod.Spec.TerminationGracePeriodSeconds == nil {
		pod.Spec.TerminationGracePeriodSeconds = utilpointer.Int64Ptr(30)
	}

	for i := range crr.Spec.Containers {
		crrContainer := &crr.Spec.Containers[i]
		podContainer := v1.Container{
			Name:  crrContainer.Name,
			Ports: crrContainer.Ports,
		}
		if crrContainer.PreStop != nil {
			podContainer.Lifecycle = &v1.Lifecycle{PreStop: &v1.LifecycleHandler{
				Exec:      crrContainer.PreStop.Exec,
				HTTPGet:   crrContainer.PreStop.HTTPGet,
				TCPSocket: crrContainer.PreStop.TCPSocket,
			}}
		}
		pod.Spec.Containers = append(pod.Spec.Containers, podContainer)
	}

	return pod
}

// SpecPodNameIndexFunc is a default index function that indexes based on crr.spec.podName
func SpecPodNameIndexFunc(obj interface{}) ([]string, error) {
	crr, ok := obj.(*appsv1alpha1.ContainerRecreateRequest)
	if !ok {
		return []string{""}, fmt.Errorf("object cannot be convert to CRR")
	}
	return []string{crr.Spec.PodName}, nil
}
