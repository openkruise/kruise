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

package v1alpha1

import (
	"fmt"

	"github.com/openkruise/kruise/apis/apps/v1beta1"
	"sigs.k8s.io/controller-runtime/pkg/conversion"
)

func (src *ContainerRecreateRequest) ConvertTo(dstRaw conversion.Hub) error {
	switch t := dstRaw.(type) {
	case *v1beta1.ContainerRecreateRequest:
		dst := dstRaw.(*v1beta1.ContainerRecreateRequest)
		dst.ObjectMeta = src.ObjectMeta
		dst.Spec.PodName = src.Spec.PodName
		containers := make([]v1beta1.ContainerRecreateRequestContainer, len(src.Spec.Containers))
		for containers_idx, containers_val := range src.Spec.Containers {
			containers[containers_idx].Name = containers_val.Name
			if containers_val.PreStop != nil {
				containers[containers_idx].PreStop = &v1beta1.ProbeHandler{}
				containers[containers_idx].PreStop.Exec = containers_val.PreStop.Exec
				containers[containers_idx].PreStop.HTTPGet = containers_val.PreStop.HTTPGet
				containers[containers_idx].PreStop.TCPSocket = containers_val.PreStop.TCPSocket
			}
			containers[containers_idx].Ports = containers_val.Ports
			if containers_val.StatusContext != nil {
				containers[containers_idx].StatusContext = &v1beta1.ContainerRecreateRequestContainerContext{}
				containers[containers_idx].StatusContext.ContainerID = containers_val.StatusContext.ContainerID
				containers[containers_idx].StatusContext.RestartCount = containers_val.StatusContext.RestartCount
			}
		}
		dst.Spec.Containers = containers
		if src.Spec.Strategy != nil {
			dst.Spec.Strategy = &v1beta1.ContainerRecreateRequestStrategy{}
			dst.Spec.Strategy.FailurePolicy = v1beta1.ContainerRecreateRequestFailurePolicyType(src.Spec.Strategy.FailurePolicy)
			dst.Spec.Strategy.OrderedRecreate = src.Spec.Strategy.OrderedRecreate
			dst.Spec.Strategy.ForceRecreate = src.Spec.Strategy.ForceRecreate
			dst.Spec.Strategy.TerminationGracePeriodSeconds = src.Spec.Strategy.TerminationGracePeriodSeconds
			dst.Spec.Strategy.UnreadyGracePeriodSeconds = src.Spec.Strategy.UnreadyGracePeriodSeconds
			dst.Spec.Strategy.MinStartedSeconds = src.Spec.Strategy.MinStartedSeconds
		}
		dst.Spec.ActiveDeadlineSeconds = src.Spec.ActiveDeadlineSeconds
		dst.Spec.TTLSecondsAfterFinished = src.Spec.TTLSecondsAfterFinished
		dst.Status.Phase = v1beta1.ContainerRecreateRequestPhase(src.Status.Phase)
		dst.Status.CompletionTime = src.Status.CompletionTime
		dst.Status.Message = src.Status.Message
		containerRecreateStates := make([]v1beta1.ContainerRecreateRequestContainerRecreateState, len(src.Status.ContainerRecreateStates))
		for containerRecreateStates_idx, containerRecreateStates_val := range src.Status.ContainerRecreateStates {
			containerRecreateStates[containerRecreateStates_idx].Name = containerRecreateStates_val.Name
			containerRecreateStates[containerRecreateStates_idx].Phase = v1beta1.ContainerRecreateRequestPhase(containerRecreateStates_val.Phase)
			containerRecreateStates[containerRecreateStates_idx].Message = containerRecreateStates_val.Message
			containerRecreateStates[containerRecreateStates_idx].IsKilled = containerRecreateStates_val.IsKilled
		}
		dst.Status.ContainerRecreateStates = containerRecreateStates
	default:
		return fmt.Errorf("unsupported type %v", t)
	}
	return nil
}

func (dst *ContainerRecreateRequest) ConvertFrom(srcRaw conversion.Hub) error {
	switch t := srcRaw.(type) {
	case *v1beta1.ContainerRecreateRequest:
		src := srcRaw.(*v1beta1.ContainerRecreateRequest)
		dst.ObjectMeta = src.ObjectMeta
		dst.Spec.PodName = src.Spec.PodName
		containers := make([]ContainerRecreateRequestContainer, len(src.Spec.Containers))
		for containers_idx, containers_val := range src.Spec.Containers {
			containers[containers_idx].Name = containers_val.Name
			if containers_val.PreStop != nil {
				containers[containers_idx].PreStop = &ProbeHandler{}
				containers[containers_idx].PreStop.Exec = containers_val.PreStop.Exec
				containers[containers_idx].PreStop.HTTPGet = containers_val.PreStop.HTTPGet
				containers[containers_idx].PreStop.TCPSocket = containers_val.PreStop.TCPSocket
			}
			containers[containers_idx].Ports = containers_val.Ports
			if containers_val.StatusContext != nil {
				containers[containers_idx].StatusContext = &ContainerRecreateRequestContainerContext{}
				containers[containers_idx].StatusContext.ContainerID = containers_val.StatusContext.ContainerID
				containers[containers_idx].StatusContext.RestartCount = containers_val.StatusContext.RestartCount
			}
		}
		dst.Spec.Containers = containers
		if src.Spec.Strategy != nil {
			dst.Spec.Strategy = &ContainerRecreateRequestStrategy{}
			dst.Spec.Strategy.FailurePolicy = ContainerRecreateRequestFailurePolicyType(src.Spec.Strategy.FailurePolicy)
			dst.Spec.Strategy.OrderedRecreate = src.Spec.Strategy.OrderedRecreate
			dst.Spec.Strategy.ForceRecreate = src.Spec.Strategy.ForceRecreate
			dst.Spec.Strategy.TerminationGracePeriodSeconds = src.Spec.Strategy.TerminationGracePeriodSeconds
			dst.Spec.Strategy.UnreadyGracePeriodSeconds = src.Spec.Strategy.UnreadyGracePeriodSeconds
			dst.Spec.Strategy.MinStartedSeconds = src.Spec.Strategy.MinStartedSeconds
		}
		dst.Spec.ActiveDeadlineSeconds = src.Spec.ActiveDeadlineSeconds
		dst.Spec.TTLSecondsAfterFinished = src.Spec.TTLSecondsAfterFinished
		dst.Status.Phase = ContainerRecreateRequestPhase(src.Status.Phase)
		dst.Status.CompletionTime = src.Status.CompletionTime
		dst.Status.Message = src.Status.Message
		containerRecreateStates := make([]ContainerRecreateRequestContainerRecreateState, len(src.Status.ContainerRecreateStates))
		for containerRecreateStates_idx, containerRecreateStates_val := range src.Status.ContainerRecreateStates {
			containerRecreateStates[containerRecreateStates_idx].Name = containerRecreateStates_val.Name
			containerRecreateStates[containerRecreateStates_idx].Phase = ContainerRecreateRequestPhase(containerRecreateStates_val.Phase)
			containerRecreateStates[containerRecreateStates_idx].Message = containerRecreateStates_val.Message
			containerRecreateStates[containerRecreateStates_idx].IsKilled = containerRecreateStates_val.IsKilled
		}
		dst.Status.ContainerRecreateStates = containerRecreateStates
	default:
		return fmt.Errorf("unsupported type %v", t)
	}
	return nil
}
