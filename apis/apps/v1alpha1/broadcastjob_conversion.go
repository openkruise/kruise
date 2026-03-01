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

	"sigs.k8s.io/controller-runtime/pkg/conversion"

	"github.com/openkruise/kruise/apis/apps/v1beta1"
)

func (bj *BroadcastJob) ConvertTo(dst conversion.Hub) error {
	switch t := dst.(type) {
	case *v1beta1.BroadcastJob:
		bjv1beta1 := dst.(*v1beta1.BroadcastJob)
		bjv1beta1.ObjectMeta = bj.ObjectMeta

		// spec
		bjv1beta1.Spec = v1beta1.BroadcastJobSpec{
			Parallelism: bj.Spec.Parallelism,
			Template:    bj.Spec.Template,
			CompletionPolicy: v1beta1.CompletionPolicy{
				Type:                    v1beta1.CompletionPolicyType(bj.Spec.CompletionPolicy.Type),
				ActiveDeadlineSeconds:   bj.Spec.CompletionPolicy.ActiveDeadlineSeconds,
				TTLSecondsAfterFinished: bj.Spec.CompletionPolicy.TTLSecondsAfterFinished,
			},
			Paused: bj.Spec.Paused,
			FailurePolicy: v1beta1.FailurePolicy{
				Type:         v1beta1.FailurePolicyType(bj.Spec.FailurePolicy.Type),
				RestartLimit: bj.Spec.FailurePolicy.RestartLimit,
			},
		}

		// status
		bjv1beta1.Status = v1beta1.BroadcastJobStatus{
			Conditions:     convertJobConditionsToV1Beta1(bj.Status.Conditions),
			StartTime:      bj.Status.StartTime,
			CompletionTime: bj.Status.CompletionTime,
			Active:         bj.Status.Active,
			Succeeded:      bj.Status.Succeeded,
			Failed:         bj.Status.Failed,
			Desired:        bj.Status.Desired,
			Phase:          v1beta1.BroadcastJobPhase(bj.Status.Phase),
		}

		return nil

	default:
		return fmt.Errorf("unsupported type %v", t)
	}
}

func (bj *BroadcastJob) ConvertFrom(src conversion.Hub) error {
	switch t := src.(type) {
	case *v1beta1.BroadcastJob:
		bjv1beta1 := src.(*v1beta1.BroadcastJob)
		bj.ObjectMeta = bjv1beta1.ObjectMeta

		// spec
		bj.Spec = BroadcastJobSpec{
			Parallelism: bjv1beta1.Spec.Parallelism,
			Template:    bjv1beta1.Spec.Template,
			CompletionPolicy: CompletionPolicy{
				Type:                    CompletionPolicyType(bjv1beta1.Spec.CompletionPolicy.Type),
				ActiveDeadlineSeconds:   bjv1beta1.Spec.CompletionPolicy.ActiveDeadlineSeconds,
				TTLSecondsAfterFinished: bjv1beta1.Spec.CompletionPolicy.TTLSecondsAfterFinished,
			},
			Paused: bjv1beta1.Spec.Paused,
			FailurePolicy: FailurePolicy{
				Type:         FailurePolicyType(bjv1beta1.Spec.FailurePolicy.Type),
				RestartLimit: bjv1beta1.Spec.FailurePolicy.RestartLimit,
			},
		}

		// status
		bj.Status = BroadcastJobStatus{
			Conditions:     convertJobConditionsToV1Alpha1(bjv1beta1.Status.Conditions),
			StartTime:      bjv1beta1.Status.StartTime,
			CompletionTime: bjv1beta1.Status.CompletionTime,
			Active:         bjv1beta1.Status.Active,
			Succeeded:      bjv1beta1.Status.Succeeded,
			Failed:         bjv1beta1.Status.Failed,
			Desired:        bjv1beta1.Status.Desired,
			Phase:          BroadcastJobPhase(bjv1beta1.Status.Phase),
		}

		return nil
	default:
		return fmt.Errorf("unsupported type %v", t)
	}
}

func convertJobConditionsToV1Beta1(conditions []JobCondition) []v1beta1.JobCondition {
	if conditions == nil {
		return nil
	}
	result := make([]v1beta1.JobCondition, len(conditions))
	for i, condition := range conditions {
		result[i] = v1beta1.JobCondition{
			Type:               v1beta1.JobConditionType(condition.Type),
			Status:             condition.Status,
			LastProbeTime:      condition.LastProbeTime,
			LastTransitionTime: condition.LastTransitionTime,
			Reason:             condition.Reason,
			Message:            condition.Message,
		}
	}
	return result
}

func convertJobConditionsToV1Alpha1(conditions []v1beta1.JobCondition) []JobCondition {
	if conditions == nil {
		return nil
	}
	result := make([]JobCondition, len(conditions))
	for i, condition := range conditions {
		result[i] = JobCondition{
			Type:               JobConditionType(condition.Type),
			Status:             condition.Status,
			LastProbeTime:      condition.LastProbeTime,
			LastTransitionTime: condition.LastTransitionTime,
			Reason:             condition.Reason,
			Message:            condition.Message,
		}
	}
	return result
}
