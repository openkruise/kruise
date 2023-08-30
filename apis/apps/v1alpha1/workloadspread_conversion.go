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

func (src *WorkloadSpread) ConvertTo(dstRaw conversion.Hub) error {
	switch t := dstRaw.(type) {
	case *v1beta1.WorkloadSpread:
		dst := dstRaw.(*v1beta1.WorkloadSpread)
		dst.ObjectMeta = src.ObjectMeta
		if src.Spec.TargetReference != nil {
			dst.Spec.TargetReference = &v1beta1.TargetReference{}
			dst.Spec.TargetReference.APIVersion = src.Spec.TargetReference.APIVersion
			dst.Spec.TargetReference.Kind = src.Spec.TargetReference.Kind
			dst.Spec.TargetReference.Name = src.Spec.TargetReference.Name
		}
		subsets := make([]v1beta1.WorkloadSpreadSubset, len(src.Spec.Subsets))
		for subsets_idx, subsets_val := range src.Spec.Subsets {
			subsets[subsets_idx].Name = subsets_val.Name
			subsets[subsets_idx].RequiredNodeSelectorTerm = subsets_val.RequiredNodeSelectorTerm
			subsets[subsets_idx].PreferredNodeSelectorTerms = subsets_val.PreferredNodeSelectorTerms
			subsets[subsets_idx].Tolerations = subsets_val.Tolerations
			subsets[subsets_idx].MaxReplicas = subsets_val.MaxReplicas
			subsets[subsets_idx].Patch = subsets_val.Patch
		}
		dst.Spec.Subsets = subsets
		dst.Spec.ScheduleStrategy.Type = v1beta1.WorkloadSpreadScheduleStrategyType(src.Spec.ScheduleStrategy.Type)
		if src.Spec.ScheduleStrategy.Adaptive != nil {
			dst.Spec.ScheduleStrategy.Adaptive = &v1beta1.AdaptiveWorkloadSpreadStrategy{}
			dst.Spec.ScheduleStrategy.Adaptive.DisableSimulationSchedule = src.Spec.ScheduleStrategy.Adaptive.DisableSimulationSchedule
			dst.Spec.ScheduleStrategy.Adaptive.RescheduleCriticalSeconds = src.Spec.ScheduleStrategy.Adaptive.RescheduleCriticalSeconds
		}
		dst.Status.ObservedGeneration = src.Status.ObservedGeneration
		subsetStatuses := make([]v1beta1.WorkloadSpreadSubsetStatus, len(src.Status.SubsetStatuses))
		for subsetStatuses_idx, subsetStatuses_val := range src.Status.SubsetStatuses {
			subsetStatuses[subsetStatuses_idx].Name = subsetStatuses_val.Name
			subsetStatuses[subsetStatuses_idx].Replicas = subsetStatuses_val.Replicas
			conditions := make([]v1beta1.WorkloadSpreadSubsetCondition, len(subsetStatuses_val.Conditions))
			for conditions_idx, conditions_val := range subsetStatuses_val.Conditions {
				conditions[conditions_idx].Type = v1beta1.WorkloadSpreadSubsetConditionType(conditions_val.Type)
				conditions[conditions_idx].Status = conditions_val.Status
				conditions[conditions_idx].LastTransitionTime = conditions_val.LastTransitionTime
				conditions[conditions_idx].Reason = conditions_val.Reason
				conditions[conditions_idx].Message = conditions_val.Message
			}
			subsetStatuses[subsetStatuses_idx].Conditions = conditions
			subsetStatuses[subsetStatuses_idx].MissingReplicas = subsetStatuses_val.MissingReplicas
			subsetStatuses[subsetStatuses_idx].CreatingPods = subsetStatuses_val.CreatingPods
			subsetStatuses[subsetStatuses_idx].DeletingPods = subsetStatuses_val.DeletingPods
		}
		dst.Status.SubsetStatuses = subsetStatuses
	default:
		return fmt.Errorf("unsupported type %v", t)
	}
	return nil
}

func (dst *WorkloadSpread) ConvertFrom(srcRaw conversion.Hub) error {
	switch t := srcRaw.(type) {
	case *v1beta1.WorkloadSpread:
		src := srcRaw.(*v1beta1.WorkloadSpread)
		dst.ObjectMeta = src.ObjectMeta
		if src.Spec.TargetReference != nil {
			dst.Spec.TargetReference = &TargetReference{}
			dst.Spec.TargetReference.APIVersion = src.Spec.TargetReference.APIVersion
			dst.Spec.TargetReference.Kind = src.Spec.TargetReference.Kind
			dst.Spec.TargetReference.Name = src.Spec.TargetReference.Name
		}
		subsets := make([]WorkloadSpreadSubset, len(src.Spec.Subsets))
		for subsets_idx, subsets_val := range src.Spec.Subsets {
			subsets[subsets_idx].Name = subsets_val.Name
			subsets[subsets_idx].RequiredNodeSelectorTerm = subsets_val.RequiredNodeSelectorTerm
			subsets[subsets_idx].PreferredNodeSelectorTerms = subsets_val.PreferredNodeSelectorTerms
			subsets[subsets_idx].Tolerations = subsets_val.Tolerations
			subsets[subsets_idx].MaxReplicas = subsets_val.MaxReplicas
			subsets[subsets_idx].Patch = subsets_val.Patch
		}
		dst.Spec.Subsets = subsets
		dst.Spec.ScheduleStrategy.Type = WorkloadSpreadScheduleStrategyType(src.Spec.ScheduleStrategy.Type)
		if src.Spec.ScheduleStrategy.Adaptive != nil {
			dst.Spec.ScheduleStrategy.Adaptive = &AdaptiveWorkloadSpreadStrategy{}
			dst.Spec.ScheduleStrategy.Adaptive.DisableSimulationSchedule = src.Spec.ScheduleStrategy.Adaptive.DisableSimulationSchedule
			dst.Spec.ScheduleStrategy.Adaptive.RescheduleCriticalSeconds = src.Spec.ScheduleStrategy.Adaptive.RescheduleCriticalSeconds
		}
		dst.Status.ObservedGeneration = src.Status.ObservedGeneration
		subsetStatuses := make([]WorkloadSpreadSubsetStatus, len(src.Status.SubsetStatuses))
		for subsetStatuses_idx, subsetStatuses_val := range src.Status.SubsetStatuses {
			subsetStatuses[subsetStatuses_idx].Name = subsetStatuses_val.Name
			subsetStatuses[subsetStatuses_idx].Replicas = subsetStatuses_val.Replicas
			conditions := make([]WorkloadSpreadSubsetCondition, len(subsetStatuses_val.Conditions))
			for conditions_idx, conditions_val := range subsetStatuses_val.Conditions {
				conditions[conditions_idx].Type = WorkloadSpreadSubsetConditionType(conditions_val.Type)
				conditions[conditions_idx].Status = conditions_val.Status
				conditions[conditions_idx].LastTransitionTime = conditions_val.LastTransitionTime
				conditions[conditions_idx].Reason = conditions_val.Reason
				conditions[conditions_idx].Message = conditions_val.Message
			}
			subsetStatuses[subsetStatuses_idx].Conditions = conditions
			subsetStatuses[subsetStatuses_idx].MissingReplicas = subsetStatuses_val.MissingReplicas
			subsetStatuses[subsetStatuses_idx].CreatingPods = subsetStatuses_val.CreatingPods
			subsetStatuses[subsetStatuses_idx].DeletingPods = subsetStatuses_val.DeletingPods
		}
		dst.Status.SubsetStatuses = subsetStatuses
	default:
		return fmt.Errorf("unsupported type %v", t)
	}
	return nil
}
