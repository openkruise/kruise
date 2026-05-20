/*
Copyright 2026 The Kruise Authors.

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

package v1alpha1

import (
	"encoding/json"
	"fmt"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/conversion"

	"github.com/openkruise/kruise/apis/apps/v1beta1"
)

// ConvertTo converts this v1alpha1 ContainerRecreateRequest to the Hub version (v1beta1).
func (src *ContainerRecreateRequest) ConvertTo(dstRaw conversion.Hub) error {
	dst, ok := dstRaw.(*v1beta1.ContainerRecreateRequest)
	if !ok {
		return fmt.Errorf("unsupported hub type %T", dstRaw)
	}

	dst.ObjectMeta = src.ObjectMeta

	dst.Spec = v1beta1.ContainerRecreateRequestSpec{
		PodName:                 src.Spec.PodName,
		ActiveDeadlineSeconds:   src.Spec.ActiveDeadlineSeconds,
		TTLSecondsAfterFinished: src.Spec.TTLSecondsAfterFinished,
	}

	if src.Spec.Containers != nil {
		dst.Spec.Containers = make([]v1beta1.ContainerRecreateRequestContainer, len(src.Spec.Containers))
		for i, c := range src.Spec.Containers {
			dst.Spec.Containers[i] = v1beta1.ContainerRecreateRequestContainer{
				Name:  c.Name,
				Ports: c.Ports,
			}
			if c.PreStop != nil {
				dst.Spec.Containers[i].PreStop = &v1beta1.CRRProbeHandler{
					Exec:      c.PreStop.Exec,
					HTTPGet:   c.PreStop.HTTPGet,
					TCPSocket: c.PreStop.TCPSocket,
				}
			}
			if c.StatusContext != nil {
				dst.Spec.Containers[i].StatusContext = &v1beta1.ContainerRecreateRequestContainerContext{
					ContainerID:  c.StatusContext.ContainerID,
					RestartCount: c.StatusContext.RestartCount,
				}
			}
		}
	}

	if src.Spec.Strategy != nil {
		dst.Spec.Strategy = &v1beta1.ContainerRecreateRequestStrategy{
			FailurePolicy:                 v1beta1.ContainerRecreateRequestFailurePolicyType(src.Spec.Strategy.FailurePolicy),
			OrderedRecreate:               src.Spec.Strategy.OrderedRecreate,
			ForceRecreate:                 src.Spec.Strategy.ForceRecreate,
			TerminationGracePeriodSeconds: src.Spec.Strategy.TerminationGracePeriodSeconds,
			UnreadyGracePeriodSeconds:     src.Spec.Strategy.UnreadyGracePeriodSeconds,
			MinStartedSeconds:             src.Spec.Strategy.MinStartedSeconds,
		}
	}

	dst.Status = v1beta1.ContainerRecreateRequestStatus{
		Phase:          v1beta1.ContainerRecreateRequestPhase(src.Status.Phase),
		CompletionTime: src.Status.CompletionTime,
		Message:        src.Status.Message,
	}

	if src.Status.ContainerRecreateStates != nil {
		dst.Status.ContainerRecreateStates = make([]v1beta1.ContainerRecreateRequestContainerRecreateState, len(src.Status.ContainerRecreateStates))
		for i, s := range src.Status.ContainerRecreateStates {
			dst.Status.ContainerRecreateStates[i] = v1beta1.ContainerRecreateRequestContainerRecreateState{
				Name:     s.Name,
				Phase:    v1beta1.ContainerRecreateRequestPhase(s.Phase),
				Message:  s.Message,
				IsKilled: s.IsKilled,
			}
		}
	}

	// Promote annotation crr.apps.kruise.io/sync-container-statuses → status.containerStatusSnapshot
	if raw, ok := src.Annotations[ContainerRecreateRequestSyncContainerStatusesKey]; ok && raw != "" {
		var syncStatuses []ContainerRecreateRequestSyncContainerStatus
		if err := json.Unmarshal([]byte(raw), &syncStatuses); err == nil {
			dst.Status.ContainerStatusSnapshot = make([]v1beta1.ContainerRecreateRequestSyncContainerStatus, len(syncStatuses))
			for i, s := range syncStatuses {
				dst.Status.ContainerStatusSnapshot[i] = v1beta1.ContainerRecreateRequestSyncContainerStatus{
					Name:         s.Name,
					Ready:        s.Ready,
					RestartCount: s.RestartCount,
					ContainerID:  s.ContainerID,
				}
			}
		}
	}

	// Promote annotation crr.apps.kruise.io/unready-acquired → status.conditions[PodUnreadyAcquired]
	if timeStr, ok := src.Annotations[ContainerRecreateRequestUnreadyAcquiredKey]; ok && timeStr != "" {
		t, err := time.Parse(time.RFC3339, timeStr)
		if err == nil {
			dst.Status.Conditions = []metav1.Condition{
				{
					Type:               v1beta1.ContainerRecreateRequestPodUnreadyAcquiredType,
					Status:             metav1.ConditionTrue,
					LastTransitionTime: metav1.NewTime(t),
					Reason:             "UnreadyAcquired",
					Message:            "Pod has been forced to not-ready for unreadyGracePeriodSeconds drain",
				},
			}
		}
	}

	return nil
}

// ConvertFrom converts from the Hub version (v1beta1) back to this v1alpha1 ContainerRecreateRequest.
func (dst *ContainerRecreateRequest) ConvertFrom(srcRaw conversion.Hub) error {
	src, ok := srcRaw.(*v1beta1.ContainerRecreateRequest)
	if !ok {
		return fmt.Errorf("unsupported hub type %T", srcRaw)
	}

	dst.ObjectMeta = src.ObjectMeta

	dst.Spec = ContainerRecreateRequestSpec{
		PodName:                 src.Spec.PodName,
		ActiveDeadlineSeconds:   src.Spec.ActiveDeadlineSeconds,
		TTLSecondsAfterFinished: src.Spec.TTLSecondsAfterFinished,
	}

	if src.Spec.Containers != nil {
		dst.Spec.Containers = make([]ContainerRecreateRequestContainer, len(src.Spec.Containers))
		for i, c := range src.Spec.Containers {
			dst.Spec.Containers[i] = ContainerRecreateRequestContainer{
				Name:  c.Name,
				Ports: c.Ports,
			}
			if c.PreStop != nil {
				dst.Spec.Containers[i].PreStop = &ProbeHandler{
					Exec:      c.PreStop.Exec,
					HTTPGet:   c.PreStop.HTTPGet,
					TCPSocket: c.PreStop.TCPSocket,
				}
			}
			if c.StatusContext != nil {
				dst.Spec.Containers[i].StatusContext = &ContainerRecreateRequestContainerContext{
					ContainerID:  c.StatusContext.ContainerID,
					RestartCount: c.StatusContext.RestartCount,
				}
			}
		}
	}

	if src.Spec.Strategy != nil {
		dst.Spec.Strategy = &ContainerRecreateRequestStrategy{
			FailurePolicy:                 ContainerRecreateRequestFailurePolicyType(src.Spec.Strategy.FailurePolicy),
			OrderedRecreate:               src.Spec.Strategy.OrderedRecreate,
			ForceRecreate:                 src.Spec.Strategy.ForceRecreate,
			TerminationGracePeriodSeconds: src.Spec.Strategy.TerminationGracePeriodSeconds,
			UnreadyGracePeriodSeconds:     src.Spec.Strategy.UnreadyGracePeriodSeconds,
			MinStartedSeconds:             src.Spec.Strategy.MinStartedSeconds,
		}
	}

	dst.Status = ContainerRecreateRequestStatus{
		Phase:          ContainerRecreateRequestPhase(src.Status.Phase),
		CompletionTime: src.Status.CompletionTime,
		Message:        src.Status.Message,
	}

	if src.Status.ContainerRecreateStates != nil {
		dst.Status.ContainerRecreateStates = make([]ContainerRecreateRequestContainerRecreateState, len(src.Status.ContainerRecreateStates))
		for i, s := range src.Status.ContainerRecreateStates {
			dst.Status.ContainerRecreateStates[i] = ContainerRecreateRequestContainerRecreateState{
				Name:     s.Name,
				Phase:    ContainerRecreateRequestPhase(s.Phase),
				Message:  s.Message,
				IsKilled: s.IsKilled,
			}
		}
	}

	if src.Status.ContainerStatusSnapshot != nil {
		syncStatuses := make([]ContainerRecreateRequestSyncContainerStatus, len(src.Status.ContainerStatusSnapshot))
		for i, s := range src.Status.ContainerStatusSnapshot {
			syncStatuses[i] = ContainerRecreateRequestSyncContainerStatus{
				Name:         s.Name,
				Ready:        s.Ready,
				RestartCount: s.RestartCount,
				ContainerID:  s.ContainerID,
			}
		}
		if raw, err := json.Marshal(syncStatuses); err == nil {
			if dst.Annotations == nil {
				dst.Annotations = map[string]string{}
			}
			dst.Annotations[ContainerRecreateRequestSyncContainerStatusesKey] = string(raw)
		}
	}

	return nil
}
