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

func (src *EphemeralJob) ConvertTo(dstRaw conversion.Hub) error {
	switch t := dstRaw.(type) {
	case *v1beta1.EphemeralJob:
		dst := dstRaw.(*v1beta1.EphemeralJob)
		dst.ObjectMeta = src.ObjectMeta
		dst.Spec.Selector = src.Spec.Selector
		dst.Spec.Replicas = src.Spec.Replicas
		dst.Spec.Parallelism = src.Spec.Parallelism
		dst.Spec.Template.EphemeralContainers = src.Spec.Template.EphemeralContainers
		dst.Spec.Paused = src.Spec.Paused
		dst.Spec.ActiveDeadlineSeconds = src.Spec.ActiveDeadlineSeconds
		dst.Spec.TTLSecondsAfterFinished = src.Spec.TTLSecondsAfterFinished
		conditions := make([]v1beta1.EphemeralJobCondition, len(src.Status.Conditions))
		for conditions_idx, conditions_val := range src.Status.Conditions {
			conditions[conditions_idx].Type = v1beta1.EphemeralJobConditionType(conditions_val.Type)
			conditions[conditions_idx].Status = conditions_val.Status
			conditions[conditions_idx].LastProbeTime = conditions_val.LastProbeTime
			conditions[conditions_idx].LastTransitionTime = conditions_val.LastTransitionTime
			conditions[conditions_idx].Reason = conditions_val.Reason
			conditions[conditions_idx].Message = conditions_val.Message
		}
		dst.Status.Conditions = conditions
		dst.Status.StartTime = src.Status.StartTime
		dst.Status.CompletionTime = src.Status.CompletionTime
		dst.Status.Phase = v1beta1.EphemeralJobPhase(src.Status.Phase)
		dst.Status.Matches = src.Status.Matches
		dst.Status.Running = src.Status.Running
		dst.Status.Succeeded = src.Status.Succeeded
		dst.Status.Waiting = src.Status.Waiting
		dst.Status.Failed = src.Status.Failed
	default:
		return fmt.Errorf("unsupported type %v", t)
	}
	return nil
}

func (dst *EphemeralJob) ConvertFrom(srcRaw conversion.Hub) error {
	switch t := srcRaw.(type) {
	case *v1beta1.EphemeralJob:
		src := srcRaw.(*v1beta1.EphemeralJob)
		dst.ObjectMeta = src.ObjectMeta
		dst.Spec.Selector = src.Spec.Selector
		dst.Spec.Replicas = src.Spec.Replicas
		dst.Spec.Parallelism = src.Spec.Parallelism
		dst.Spec.Template.EphemeralContainers = src.Spec.Template.EphemeralContainers
		dst.Spec.Paused = src.Spec.Paused
		dst.Spec.ActiveDeadlineSeconds = src.Spec.ActiveDeadlineSeconds
		dst.Spec.TTLSecondsAfterFinished = src.Spec.TTLSecondsAfterFinished
		conditions := make([]EphemeralJobCondition, len(src.Status.Conditions))
		for conditions_idx, conditions_val := range src.Status.Conditions {
			conditions[conditions_idx].Type = EphemeralJobConditionType(conditions_val.Type)
			conditions[conditions_idx].Status = conditions_val.Status
			conditions[conditions_idx].LastProbeTime = conditions_val.LastProbeTime
			conditions[conditions_idx].LastTransitionTime = conditions_val.LastTransitionTime
			conditions[conditions_idx].Reason = conditions_val.Reason
			conditions[conditions_idx].Message = conditions_val.Message
		}
		dst.Status.Conditions = conditions
		dst.Status.StartTime = src.Status.StartTime
		dst.Status.CompletionTime = src.Status.CompletionTime
		dst.Status.Phase = EphemeralJobPhase(src.Status.Phase)
		dst.Status.Matches = src.Status.Matches
		dst.Status.Running = src.Status.Running
		dst.Status.Succeeded = src.Status.Succeeded
		dst.Status.Waiting = src.Status.Waiting
		dst.Status.Failed = src.Status.Failed
	default:
		return fmt.Errorf("unsupported type %v", t)
	}
	return nil
}
