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
	"fmt"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/conversion"

	"github.com/openkruise/kruise/apis/apps/v1beta1"
)

// ConvertTo converts this v1alpha1 WorkloadSpread to the Hub version (v1beta1).
func (src *WorkloadSpread) ConvertTo(dstRaw conversion.Hub) error {
	dst, ok := dstRaw.(*v1beta1.WorkloadSpread)
	if !ok {
		return fmt.Errorf("unsupported hub type %T", dstRaw)
	}

	dst.ObjectMeta = src.ObjectMeta

	// spec
	dst.Spec = v1beta1.WorkloadSpreadSpec{
		ScheduleStrategy: v1beta1.WorkloadSpreadScheduleStrategy{
			Type: v1beta1.WorkloadSpreadScheduleStrategyType(src.Spec.ScheduleStrategy.Type),
		},
	}
	if src.Spec.TargetReference != nil {
		dst.Spec.TargetReference = &v1beta1.TargetReference{
			APIVersion: src.Spec.TargetReference.APIVersion,
			Kind:       src.Spec.TargetReference.Kind,
			Name:       src.Spec.TargetReference.Name,
		}
	}
	if src.Spec.TargetFilter != nil {
		dst.Spec.TargetFilter = &v1beta1.TargetFilter{
			Selector:         src.Spec.TargetFilter.Selector,
			ReplicasPathList: src.Spec.TargetFilter.ReplicasPathList,
		}
	}
	if src.Spec.ScheduleStrategy.Adaptive != nil {
		dst.Spec.ScheduleStrategy.Adaptive = &v1beta1.AdaptiveWorkloadSpreadStrategy{
			DisableSimulationSchedule: src.Spec.ScheduleStrategy.Adaptive.DisableSimulationSchedule,
			RescheduleCriticalSeconds: src.Spec.ScheduleStrategy.Adaptive.RescheduleCriticalSeconds,
		}
	}
	if src.Spec.Subsets != nil {
		dst.Spec.Subsets = make([]v1beta1.WorkloadSpreadSubset, len(src.Spec.Subsets))
		for i, s := range src.Spec.Subsets {
			dst.Spec.Subsets[i] = v1beta1.WorkloadSpreadSubset{
				Name:        s.Name,
				Tolerations: s.Tolerations,
				MaxReplicas: s.MaxReplicas,
				Patch:       s.Patch,
				// field rename: RequiredNodeSelectorTerm → RequiredNodeSelector
				RequiredNodeSelector: s.RequiredNodeSelectorTerm,
				// field rename: PreferredNodeSelectorTerms → PreferredNodeSelector
				PreferredNodeSelector: s.PreferredNodeSelectorTerms,
			}
		}
	}

	// status
	dst.Status = v1beta1.WorkloadSpreadStatus{
		ObservedGeneration: src.Status.ObservedGeneration,
	}
	if src.Status.SubsetStatuses != nil {
		dst.Status.SubsetStatuses = convertSubsetStatusesToV1Beta1(src.Status.SubsetStatuses)
	}
	if src.Status.VersionedSubsetStatuses != nil {
		dst.Status.VersionedSubsetStatuses = make(map[string][]v1beta1.WorkloadSpreadSubsetStatus, len(src.Status.VersionedSubsetStatuses))
		for version, statuses := range src.Status.VersionedSubsetStatuses {
			dst.Status.VersionedSubsetStatuses[version] = convertSubsetStatusesToV1Beta1(statuses)
		}
	}

	return nil
}

// ConvertFrom converts from the Hub version (v1beta1) to this v1alpha1 WorkloadSpread.
func (dst *WorkloadSpread) ConvertFrom(srcRaw conversion.Hub) error {
	src, ok := srcRaw.(*v1beta1.WorkloadSpread)
	if !ok {
		return fmt.Errorf("unsupported hub type %T", srcRaw)
	}

	dst.ObjectMeta = src.ObjectMeta

	// spec
	dst.Spec = WorkloadSpreadSpec{
		ScheduleStrategy: WorkloadSpreadScheduleStrategy{
			Type: WorkloadSpreadScheduleStrategyType(src.Spec.ScheduleStrategy.Type),
		},
	}
	if src.Spec.TargetReference != nil {
		dst.Spec.TargetReference = &TargetReference{
			APIVersion: src.Spec.TargetReference.APIVersion,
			Kind:       src.Spec.TargetReference.Kind,
			Name:       src.Spec.TargetReference.Name,
		}
	}
	if src.Spec.TargetFilter != nil {
		dst.Spec.TargetFilter = &TargetFilter{
			Selector:         src.Spec.TargetFilter.Selector,
			ReplicasPathList: src.Spec.TargetFilter.ReplicasPathList,
		}
	}
	if src.Spec.ScheduleStrategy.Adaptive != nil {
		dst.Spec.ScheduleStrategy.Adaptive = &AdaptiveWorkloadSpreadStrategy{
			DisableSimulationSchedule: src.Spec.ScheduleStrategy.Adaptive.DisableSimulationSchedule,
			RescheduleCriticalSeconds: src.Spec.ScheduleStrategy.Adaptive.RescheduleCriticalSeconds,
		}
	}
	if src.Spec.Subsets != nil {
		dst.Spec.Subsets = make([]WorkloadSpreadSubset, len(src.Spec.Subsets))
		for i, s := range src.Spec.Subsets {
			dst.Spec.Subsets[i] = WorkloadSpreadSubset{
				Name:        s.Name,
				Tolerations: s.Tolerations,
				MaxReplicas: s.MaxReplicas,
				Patch:       s.Patch,
				// field rename (reverse): RequiredNodeSelector → RequiredNodeSelectorTerm
				RequiredNodeSelectorTerm: s.RequiredNodeSelector,
				// field rename (reverse): PreferredNodeSelector → PreferredNodeSelectorTerms
				PreferredNodeSelectorTerms: s.PreferredNodeSelector,
			}
		}
	}

	// status
	dst.Status = WorkloadSpreadStatus{
		ObservedGeneration: src.Status.ObservedGeneration,
	}
	if src.Status.SubsetStatuses != nil {
		dst.Status.SubsetStatuses = convertSubsetStatusesToV1Alpha1(src.Status.SubsetStatuses)
	}
	if src.Status.VersionedSubsetStatuses != nil {
		dst.Status.VersionedSubsetStatuses = make(map[string][]WorkloadSpreadSubsetStatus, len(src.Status.VersionedSubsetStatuses))
		for version, statuses := range src.Status.VersionedSubsetStatuses {
			dst.Status.VersionedSubsetStatuses[version] = convertSubsetStatusesToV1Alpha1(statuses)
		}
	}

	return nil
}

// convertSubsetStatusesToV1Beta1 converts a slice of v1alpha1 subset statuses to v1beta1,
// mapping WorkloadSpreadSubsetCondition → metav1.Condition.
func convertSubsetStatusesToV1Beta1(src []WorkloadSpreadSubsetStatus) []v1beta1.WorkloadSpreadSubsetStatus {
	dst := make([]v1beta1.WorkloadSpreadSubsetStatus, len(src))
	for i, ss := range src {
		dst[i] = v1beta1.WorkloadSpreadSubsetStatus{
			Name:            ss.Name,
			Replicas:        ss.Replicas,
			MissingReplicas: ss.MissingReplicas,
			CreatingPods:    ss.CreatingPods,
			DeletingPods:    ss.DeletingPods,
			Conditions:      convertConditionsToMetaV1(ss.Conditions),
		}
	}
	return dst
}

// convertSubsetStatusesToV1Alpha1 converts a slice of v1beta1 subset statuses to v1alpha1,
// mapping metav1.Condition → WorkloadSpreadSubsetCondition.
func convertSubsetStatusesToV1Alpha1(src []v1beta1.WorkloadSpreadSubsetStatus) []WorkloadSpreadSubsetStatus {
	dst := make([]WorkloadSpreadSubsetStatus, len(src))
	for i, ss := range src {
		dst[i] = WorkloadSpreadSubsetStatus{
			Name:            ss.Name,
			Replicas:        ss.Replicas,
			MissingReplicas: ss.MissingReplicas,
			CreatingPods:    ss.CreatingPods,
			DeletingPods:    ss.DeletingPods,
			Conditions:      convertConditionsToV1Alpha1(ss.Conditions),
		}
	}
	return dst
}

// convertConditionsToMetaV1 converts v1alpha1 WorkloadSpreadSubsetConditions to metav1.Conditions.
// metav1.Condition requires non-empty Reason; we use "Unknown" as a sentinel when the source has none.
func convertConditionsToMetaV1(src []WorkloadSpreadSubsetCondition) []metav1.Condition {
	if src == nil {
		return nil
	}
	dst := make([]metav1.Condition, len(src))
	for i, c := range src {
		reason := c.Reason
		if reason == "" {
			reason = "Unknown"
		}
		dst[i] = metav1.Condition{
			Type:               string(c.Type),
			Status:             metav1.ConditionStatus(c.Status),
			LastTransitionTime: c.LastTransitionTime,
			Reason:             reason,
			Message:            c.Message,
			// ObservedGeneration has no source in v1alpha1; zero is the safe default.
		}
	}
	return dst
}

// convertConditionsToV1Alpha1 converts metav1.Conditions to v1alpha1 WorkloadSpreadSubsetConditions.
// ObservedGeneration is dropped as v1alpha1 has no such field.
func convertConditionsToV1Alpha1(src []metav1.Condition) []WorkloadSpreadSubsetCondition {
	if src == nil {
		return nil
	}
	dst := make([]WorkloadSpreadSubsetCondition, len(src))
	for i, c := range src {
		dst[i] = WorkloadSpreadSubsetCondition{
			Type:               WorkloadSpreadSubsetConditionType(c.Type),
			Status:             corev1.ConditionStatus(c.Status),
			LastTransitionTime: c.LastTransitionTime,
			Reason:             c.Reason,
			Message:            c.Message,
		}
	}
	return dst
}
