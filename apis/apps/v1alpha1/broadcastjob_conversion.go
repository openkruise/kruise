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

func (src *BroadcastJob) ConvertTo(dstRaw conversion.Hub) error {
	switch t := dstRaw.(type) {
	case *v1beta1.BroadcastJob:
		dst := dstRaw.(*v1beta1.BroadcastJob)
		dst.ObjectMeta = src.ObjectMeta
		dst.Spec.Parallelism = src.Spec.Parallelism
		dst.Spec.Template = src.Spec.Template
		dst.Spec.CompletionPolicy.Type = v1beta1.CompletionPolicyType(src.Spec.CompletionPolicy.Type)
		dst.Spec.CompletionPolicy.ActiveDeadlineSeconds = src.Spec.CompletionPolicy.ActiveDeadlineSeconds
		dst.Spec.CompletionPolicy.TTLSecondsAfterFinished = src.Spec.CompletionPolicy.TTLSecondsAfterFinished
		dst.Spec.Paused = src.Spec.Paused
		dst.Spec.FailurePolicy.Type = v1beta1.FailurePolicyType(src.Spec.FailurePolicy.Type)
		dst.Spec.FailurePolicy.RestartLimit = src.Spec.FailurePolicy.RestartLimit
		conditions := make([]v1beta1.JobCondition, len(src.Status.Conditions))
		for conditions_idx, conditions_val := range src.Status.Conditions {
			conditions[conditions_idx].Type = v1beta1.JobConditionType(conditions_val.Type)
			conditions[conditions_idx].Status = conditions_val.Status
			conditions[conditions_idx].LastProbeTime = conditions_val.LastProbeTime
			conditions[conditions_idx].LastTransitionTime = conditions_val.LastTransitionTime
			conditions[conditions_idx].Reason = conditions_val.Reason
			conditions[conditions_idx].Message = conditions_val.Message
		}
		dst.Status.Conditions = conditions
		dst.Status.StartTime = src.Status.StartTime
		dst.Status.CompletionTime = src.Status.CompletionTime
		dst.Status.Active = src.Status.Active
		dst.Status.Succeeded = src.Status.Succeeded
		dst.Status.Failed = src.Status.Failed
		dst.Status.Desired = src.Status.Desired
		dst.Status.Phase = v1beta1.BroadcastJobPhase(src.Status.Phase)
	default:
		return fmt.Errorf("unsupported type %v", t)
	}
	return nil
}

func (dst *BroadcastJob) ConvertFrom(srcRaw conversion.Hub) error {
	switch t := srcRaw.(type) {
	case *v1beta1.BroadcastJob:
		src := srcRaw.(*v1beta1.BroadcastJob)
		dst.ObjectMeta = src.ObjectMeta
		dst.Spec.Parallelism = src.Spec.Parallelism
		dst.Spec.Template = src.Spec.Template
		dst.Spec.CompletionPolicy.Type = CompletionPolicyType(src.Spec.CompletionPolicy.Type)
		dst.Spec.CompletionPolicy.ActiveDeadlineSeconds = src.Spec.CompletionPolicy.ActiveDeadlineSeconds
		dst.Spec.CompletionPolicy.TTLSecondsAfterFinished = src.Spec.CompletionPolicy.TTLSecondsAfterFinished
		dst.Spec.Paused = src.Spec.Paused
		dst.Spec.FailurePolicy.Type = FailurePolicyType(src.Spec.FailurePolicy.Type)
		dst.Spec.FailurePolicy.RestartLimit = src.Spec.FailurePolicy.RestartLimit
		conditions := make([]JobCondition, len(src.Status.Conditions))
		for conditions_idx, conditions_val := range src.Status.Conditions {
			conditions[conditions_idx].Type = JobConditionType(conditions_val.Type)
			conditions[conditions_idx].Status = conditions_val.Status
			conditions[conditions_idx].LastProbeTime = conditions_val.LastProbeTime
			conditions[conditions_idx].LastTransitionTime = conditions_val.LastTransitionTime
			conditions[conditions_idx].Reason = conditions_val.Reason
			conditions[conditions_idx].Message = conditions_val.Message
		}
		dst.Status.Conditions = conditions
		dst.Status.StartTime = src.Status.StartTime
		dst.Status.CompletionTime = src.Status.CompletionTime
		dst.Status.Active = src.Status.Active
		dst.Status.Succeeded = src.Status.Succeeded
		dst.Status.Failed = src.Status.Failed
		dst.Status.Desired = src.Status.Desired
		dst.Status.Phase = BroadcastJobPhase(src.Status.Phase)
	default:
		return fmt.Errorf("unsupported type %v", t)
	}
	return nil
}
