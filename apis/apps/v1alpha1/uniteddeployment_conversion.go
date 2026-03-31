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

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/conversion"

	"github.com/openkruise/kruise/apis/apps/v1beta1"
)

func (ud *UnitedDeployment) ConvertTo(dst conversion.Hub) error {
	switch t := dst.(type) {
	case *v1beta1.UnitedDeployment:
		return convertUnitedDeploymentToV1beta1(ud, t)
	default:
		return fmt.Errorf("unsupported type %v", t)
	}
}

func (ud *UnitedDeployment) ConvertFrom(src conversion.Hub) error {
	switch t := src.(type) {
	case *v1beta1.UnitedDeployment:
		return convertUnitedDeploymentFromV1beta1(t, ud)
	default:
		return fmt.Errorf("unsupported type %v", t)
	}
}

func convertUnitedDeploymentToV1beta1(src *UnitedDeployment, dst *v1beta1.UnitedDeployment) error {
	dst.TypeMeta = metav1.TypeMeta{
		APIVersion: v1beta1.GroupVersion.String(),
		Kind:       "UnitedDeployment",
	}
	dst.ObjectMeta = src.ObjectMeta
	dst.Spec = convertUnitedDeploymentSpecToV1beta1(src.Spec)
	dst.Status = convertUnitedDeploymentStatusToV1beta1(src.Status)

	for subset, replicas := range src.Status.SubsetReplicas {
		status := ensureV1beta1SubsetStatus(dst, subset)
		status.Replicas = replicas
	}
	if src.Status.UpdateStatus != nil {
		for subset, partition := range src.Status.UpdateStatus.CurrentPartitions {
			status := ensureV1beta1SubsetStatus(dst, subset)
			status.Partition = partition
		}
	}
	return nil
}

func convertUnitedDeploymentFromV1beta1(src *v1beta1.UnitedDeployment, dst *UnitedDeployment) error {
	dst.TypeMeta = metav1.TypeMeta{
		APIVersion: GroupVersion.String(),
		Kind:       "UnitedDeployment",
	}
	dst.ObjectMeta = src.ObjectMeta
	dst.Spec = convertUnitedDeploymentSpecFromV1beta1(src.Spec)
	dst.Status = convertUnitedDeploymentStatusFromV1beta1(src.Status)

	if len(src.Status.SubsetStatuses) == 0 {
		dst.Status.SubsetReplicas = nil
		return nil
	}

	dst.Status.SubsetReplicas = make(map[string]int32, len(src.Status.SubsetStatuses))
	for _, status := range src.Status.SubsetStatuses {
		if status.Name == "" {
			continue
		}
		dst.Status.SubsetReplicas[status.Name] = status.Replicas
	}
	if len(dst.Status.SubsetReplicas) == 0 {
		dst.Status.SubsetReplicas = nil
	}
	return nil
}

func ensureV1beta1SubsetStatus(ud *v1beta1.UnitedDeployment, subset string) *v1beta1.UnitedDeploymentSubsetStatus {
	if status := ud.Status.GetSubsetStatus(subset); status != nil {
		return status
	}

	ud.Status.SubsetStatuses = append(ud.Status.SubsetStatuses, v1beta1.UnitedDeploymentSubsetStatus{Name: subset})
	return &ud.Status.SubsetStatuses[len(ud.Status.SubsetStatuses)-1]
}

func convertUnitedDeploymentSpecToV1beta1(src UnitedDeploymentSpec) v1beta1.UnitedDeploymentSpec {
	return v1beta1.UnitedDeploymentSpec{
		Replicas:             src.Replicas,
		Selector:             src.Selector,
		Template:             convertSubsetTemplateToV1beta1(src.Template),
		Topology:             convertTopologyToV1beta1(src.Topology),
		UpdateStrategy:       convertUnitedDeploymentUpdateStrategyToV1beta1(src.UpdateStrategy),
		RevisionHistoryLimit: src.RevisionHistoryLimit,
	}
}

func convertUnitedDeploymentSpecFromV1beta1(src v1beta1.UnitedDeploymentSpec) UnitedDeploymentSpec {
	return UnitedDeploymentSpec{
		Replicas:             src.Replicas,
		Selector:             src.Selector,
		Template:             convertSubsetTemplateFromV1beta1(src.Template),
		Topology:             convertTopologyFromV1beta1(src.Topology),
		UpdateStrategy:       convertUnitedDeploymentUpdateStrategyFromV1beta1(src.UpdateStrategy),
		RevisionHistoryLimit: src.RevisionHistoryLimit,
	}
}

func convertSubsetTemplateToV1beta1(src SubsetTemplate) v1beta1.SubsetTemplate {
	return v1beta1.SubsetTemplate{
		StatefulSetTemplate:         convertStatefulSetTemplateSpecToV1beta1(src.StatefulSetTemplate),
		AdvancedStatefulSetTemplate: convertAdvancedStatefulSetTemplateSpecToV1beta1(src.AdvancedStatefulSetTemplate),
		CloneSetTemplate:            convertCloneSetTemplateSpecToV1beta1(src.CloneSetTemplate),
		DeploymentTemplate:          convertDeploymentTemplateSpecToV1beta1(src.DeploymentTemplate),
	}
}

func convertSubsetTemplateFromV1beta1(src v1beta1.SubsetTemplate) SubsetTemplate {
	return SubsetTemplate{
		StatefulSetTemplate:         convertStatefulSetTemplateSpecFromV1beta1(src.StatefulSetTemplate),
		AdvancedStatefulSetTemplate: convertAdvancedStatefulSetTemplateSpecFromV1beta1(src.AdvancedStatefulSetTemplate),
		CloneSetTemplate:            convertCloneSetTemplateSpecFromV1beta1(src.CloneSetTemplate),
		DeploymentTemplate:          convertDeploymentTemplateSpecFromV1beta1(src.DeploymentTemplate),
	}
}

func convertStatefulSetTemplateSpecToV1beta1(src *StatefulSetTemplateSpec) *v1beta1.StatefulSetTemplateSpec {
	if src == nil {
		return nil
	}
	return &v1beta1.StatefulSetTemplateSpec{
		ObjectMeta: src.ObjectMeta,
		Spec:       src.Spec,
	}
}

func convertStatefulSetTemplateSpecFromV1beta1(src *v1beta1.StatefulSetTemplateSpec) *StatefulSetTemplateSpec {
	if src == nil {
		return nil
	}
	return &StatefulSetTemplateSpec{
		ObjectMeta: src.ObjectMeta,
		Spec:       src.Spec,
	}
}

func convertAdvancedStatefulSetTemplateSpecToV1beta1(src *AdvancedStatefulSetTemplateSpec) *v1beta1.AdvancedStatefulSetTemplateSpec {
	if src == nil {
		return nil
	}
	return &v1beta1.AdvancedStatefulSetTemplateSpec{
		ObjectMeta: src.ObjectMeta,
		Spec:       src.Spec,
	}
}

func convertAdvancedStatefulSetTemplateSpecFromV1beta1(src *v1beta1.AdvancedStatefulSetTemplateSpec) *AdvancedStatefulSetTemplateSpec {
	if src == nil {
		return nil
	}
	return &AdvancedStatefulSetTemplateSpec{
		ObjectMeta: src.ObjectMeta,
		Spec:       src.Spec,
	}
}

func convertCloneSetTemplateSpecToV1beta1(src *CloneSetTemplateSpec) *v1beta1.CloneSetTemplateSpec {
	if src == nil {
		return nil
	}
	return &v1beta1.CloneSetTemplateSpec{
		ObjectMeta: src.ObjectMeta,
		Spec:       src.Spec,
	}
}

func convertCloneSetTemplateSpecFromV1beta1(src *v1beta1.CloneSetTemplateSpec) *CloneSetTemplateSpec {
	if src == nil {
		return nil
	}
	return &CloneSetTemplateSpec{
		ObjectMeta: src.ObjectMeta,
		Spec:       src.Spec,
	}
}

func convertDeploymentTemplateSpecToV1beta1(src *DeploymentTemplateSpec) *v1beta1.DeploymentTemplateSpec {
	if src == nil {
		return nil
	}
	return &v1beta1.DeploymentTemplateSpec{
		ObjectMeta: src.ObjectMeta,
		Spec:       src.Spec,
	}
}

func convertDeploymentTemplateSpecFromV1beta1(src *v1beta1.DeploymentTemplateSpec) *DeploymentTemplateSpec {
	if src == nil {
		return nil
	}
	return &DeploymentTemplateSpec{
		ObjectMeta: src.ObjectMeta,
		Spec:       src.Spec,
	}
}

func convertTopologyToV1beta1(src Topology) v1beta1.Topology {
	return v1beta1.Topology{
		Subsets:          convertSubsetsToV1beta1(src.Subsets),
		ScheduleStrategy: convertUnitedDeploymentScheduleStrategyToV1beta1(src.ScheduleStrategy),
	}
}

func convertTopologyFromV1beta1(src v1beta1.Topology) Topology {
	return Topology{
		Subsets:          convertSubsetsFromV1beta1(src.Subsets),
		ScheduleStrategy: convertUnitedDeploymentScheduleStrategyFromV1beta1(src.ScheduleStrategy),
	}
}

func convertSubsetsToV1beta1(src []Subset) []v1beta1.Subset {
	if src == nil {
		return nil
	}
	dst := make([]v1beta1.Subset, len(src))
	for i, subset := range src {
		dst[i] = v1beta1.Subset{
			Name:             subset.Name,
			NodeSelectorTerm: subset.NodeSelectorTerm,
			Tolerations:      subset.Tolerations,
			Replicas:         subset.Replicas,
			MinReplicas:      subset.MinReplicas,
			MaxReplicas:      subset.MaxReplicas,
			Patch:            subset.Patch,
		}
	}
	return dst
}

func convertSubsetsFromV1beta1(src []v1beta1.Subset) []Subset {
	if src == nil {
		return nil
	}
	dst := make([]Subset, len(src))
	for i, subset := range src {
		dst[i] = Subset{
			Name:             subset.Name,
			NodeSelectorTerm: subset.NodeSelectorTerm,
			Tolerations:      subset.Tolerations,
			Replicas:         subset.Replicas,
			MinReplicas:      subset.MinReplicas,
			MaxReplicas:      subset.MaxReplicas,
			Patch:            subset.Patch,
		}
	}
	return dst
}

func convertUnitedDeploymentUpdateStrategyToV1beta1(src UnitedDeploymentUpdateStrategy) v1beta1.UnitedDeploymentUpdateStrategy {
	return v1beta1.UnitedDeploymentUpdateStrategy{
		Type:         v1beta1.UpdateStrategyType(src.Type),
		ManualUpdate: convertManualUpdateToV1beta1(src.ManualUpdate),
	}
}

func convertUnitedDeploymentUpdateStrategyFromV1beta1(src v1beta1.UnitedDeploymentUpdateStrategy) UnitedDeploymentUpdateStrategy {
	return UnitedDeploymentUpdateStrategy{
		Type:         UpdateStrategyType(src.Type),
		ManualUpdate: convertManualUpdateFromV1beta1(src.ManualUpdate),
	}
}

func convertManualUpdateToV1beta1(src *ManualUpdate) *v1beta1.ManualUpdate {
	if src == nil {
		return nil
	}
	return &v1beta1.ManualUpdate{
		Partitions: cloneStringInt32Map(src.Partitions),
	}
}

func convertManualUpdateFromV1beta1(src *v1beta1.ManualUpdate) *ManualUpdate {
	if src == nil {
		return nil
	}
	return &ManualUpdate{
		Partitions: cloneStringInt32Map(src.Partitions),
	}
}

func convertUnitedDeploymentScheduleStrategyToV1beta1(src UnitedDeploymentScheduleStrategy) v1beta1.UnitedDeploymentScheduleStrategy {
	return v1beta1.UnitedDeploymentScheduleStrategy{
		Type:     v1beta1.UnitedDeploymentScheduleStrategyType(src.Type),
		Adaptive: convertAdaptiveUnitedDeploymentStrategyToV1beta1(src.Adaptive),
	}
}

func convertUnitedDeploymentScheduleStrategyFromV1beta1(src v1beta1.UnitedDeploymentScheduleStrategy) UnitedDeploymentScheduleStrategy {
	return UnitedDeploymentScheduleStrategy{
		Type:     UnitedDeploymentScheduleStrategyType(src.Type),
		Adaptive: convertAdaptiveUnitedDeploymentStrategyFromV1beta1(src.Adaptive),
	}
}

func convertAdaptiveUnitedDeploymentStrategyToV1beta1(src *AdaptiveUnitedDeploymentStrategy) *v1beta1.AdaptiveUnitedDeploymentStrategy {
	if src == nil {
		return nil
	}
	return &v1beta1.AdaptiveUnitedDeploymentStrategy{
		RescheduleCriticalSeconds: src.RescheduleCriticalSeconds,
		UnschedulableDuration:     src.UnschedulableDuration,
		ReserveUnschedulablePods:  src.ReserveUnschedulablePods,
	}
}

func convertAdaptiveUnitedDeploymentStrategyFromV1beta1(src *v1beta1.AdaptiveUnitedDeploymentStrategy) *AdaptiveUnitedDeploymentStrategy {
	if src == nil {
		return nil
	}
	return &AdaptiveUnitedDeploymentStrategy{
		RescheduleCriticalSeconds: src.RescheduleCriticalSeconds,
		UnschedulableDuration:     src.UnschedulableDuration,
		ReserveUnschedulablePods:  src.ReserveUnschedulablePods,
	}
}

func convertUnitedDeploymentStatusToV1beta1(src UnitedDeploymentStatus) v1beta1.UnitedDeploymentStatus {
	return v1beta1.UnitedDeploymentStatus{
		ObservedGeneration:   src.ObservedGeneration,
		ReadyReplicas:        src.ReadyReplicas,
		Replicas:             src.Replicas,
		UpdatedReplicas:      src.UpdatedReplicas,
		ReservedPods:         src.ReservedPods,
		UpdatedReadyReplicas: src.UpdatedReadyReplicas,
		CollisionCount:       src.CollisionCount,
		CurrentRevision:      src.CurrentRevision,
		SubsetStatuses:       convertUnitedDeploymentSubsetStatusesToV1beta1(src.SubsetStatuses),
		Conditions:           convertUnitedDeploymentConditionsToV1beta1(src.Conditions),
		UpdateStatus:         convertUpdateStatusToV1beta1(src.UpdateStatus),
		LabelSelector:        src.LabelSelector,
	}
}

func convertUnitedDeploymentStatusFromV1beta1(src v1beta1.UnitedDeploymentStatus) UnitedDeploymentStatus {
	return UnitedDeploymentStatus{
		ObservedGeneration:   src.ObservedGeneration,
		ReadyReplicas:        src.ReadyReplicas,
		Replicas:             src.Replicas,
		UpdatedReplicas:      src.UpdatedReplicas,
		ReservedPods:         src.ReservedPods,
		UpdatedReadyReplicas: src.UpdatedReadyReplicas,
		CollisionCount:       src.CollisionCount,
		CurrentRevision:      src.CurrentRevision,
		SubsetStatuses:       convertUnitedDeploymentSubsetStatusesFromV1beta1(src.SubsetStatuses),
		Conditions:           convertUnitedDeploymentConditionsFromV1beta1(src.Conditions),
		UpdateStatus:         convertUpdateStatusFromV1beta1(src.UpdateStatus),
		LabelSelector:        src.LabelSelector,
	}
}

func convertUnitedDeploymentConditionsToV1beta1(src []UnitedDeploymentCondition) []v1beta1.UnitedDeploymentCondition {
	if src == nil {
		return nil
	}
	dst := make([]v1beta1.UnitedDeploymentCondition, len(src))
	for i, condition := range src {
		dst[i] = v1beta1.UnitedDeploymentCondition{
			Type:               v1beta1.UnitedDeploymentConditionType(condition.Type),
			Status:             condition.Status,
			LastTransitionTime: condition.LastTransitionTime,
			Reason:             condition.Reason,
			Message:            condition.Message,
		}
	}
	return dst
}

func convertUnitedDeploymentConditionsFromV1beta1(src []v1beta1.UnitedDeploymentCondition) []UnitedDeploymentCondition {
	if src == nil {
		return nil
	}
	dst := make([]UnitedDeploymentCondition, len(src))
	for i, condition := range src {
		dst[i] = UnitedDeploymentCondition{
			Type:               UnitedDeploymentConditionType(condition.Type),
			Status:             condition.Status,
			LastTransitionTime: condition.LastTransitionTime,
			Reason:             condition.Reason,
			Message:            condition.Message,
		}
	}
	return dst
}

func convertUpdateStatusToV1beta1(src *UpdateStatus) *v1beta1.UpdateStatus {
	if src == nil {
		return nil
	}
	return &v1beta1.UpdateStatus{
		UpdatedRevision:   src.UpdatedRevision,
		CurrentPartitions: cloneStringInt32Map(src.CurrentPartitions),
	}
}

func convertUpdateStatusFromV1beta1(src *v1beta1.UpdateStatus) *UpdateStatus {
	if src == nil {
		return nil
	}
	return &UpdateStatus{
		UpdatedRevision:   src.UpdatedRevision,
		CurrentPartitions: cloneStringInt32Map(src.CurrentPartitions),
	}
}

func convertUnitedDeploymentSubsetStatusesToV1beta1(src []UnitedDeploymentSubsetStatus) []v1beta1.UnitedDeploymentSubsetStatus {
	if src == nil {
		return nil
	}
	dst := make([]v1beta1.UnitedDeploymentSubsetStatus, len(src))
	for i, status := range src {
		dst[i] = v1beta1.UnitedDeploymentSubsetStatus{
			Name:          status.Name,
			Replicas:      status.Replicas,
			ReadyReplicas: status.ReadyReplicas,
			Partition:     status.Partition,
			ReservedPods:  status.ReservedPods,
			Conditions:    convertUnitedDeploymentSubsetConditionsToV1beta1(status.Conditions),
		}
	}
	return dst
}

func convertUnitedDeploymentSubsetStatusesFromV1beta1(src []v1beta1.UnitedDeploymentSubsetStatus) []UnitedDeploymentSubsetStatus {
	if src == nil {
		return nil
	}
	dst := make([]UnitedDeploymentSubsetStatus, len(src))
	for i, status := range src {
		dst[i] = UnitedDeploymentSubsetStatus{
			Name:          status.Name,
			Replicas:      status.Replicas,
			ReadyReplicas: status.ReadyReplicas,
			Partition:     status.Partition,
			ReservedPods:  status.ReservedPods,
			Conditions:    convertUnitedDeploymentSubsetConditionsFromV1beta1(status.Conditions),
		}
	}
	return dst
}

func convertUnitedDeploymentSubsetConditionsToV1beta1(src []UnitedDeploymentSubsetCondition) []v1beta1.UnitedDeploymentSubsetCondition {
	if src == nil {
		return nil
	}
	dst := make([]v1beta1.UnitedDeploymentSubsetCondition, len(src))
	for i, condition := range src {
		dst[i] = v1beta1.UnitedDeploymentSubsetCondition{
			Type:               v1beta1.UnitedDeploymentSubsetConditionType(condition.Type),
			Status:             condition.Status,
			LastTransitionTime: condition.LastTransitionTime,
			Reason:             condition.Reason,
			Message:            condition.Message,
		}
	}
	return dst
}

func convertUnitedDeploymentSubsetConditionsFromV1beta1(src []v1beta1.UnitedDeploymentSubsetCondition) []UnitedDeploymentSubsetCondition {
	if src == nil {
		return nil
	}
	dst := make([]UnitedDeploymentSubsetCondition, len(src))
	for i, condition := range src {
		dst[i] = UnitedDeploymentSubsetCondition{
			Type:               UnitedDeploymentSubsetConditionType(condition.Type),
			Status:             condition.Status,
			LastTransitionTime: condition.LastTransitionTime,
			Reason:             condition.Reason,
			Message:            condition.Message,
		}
	}
	return dst
}

func cloneStringInt32Map(src map[string]int32) map[string]int32 {
	if src == nil {
		return nil
	}
	dst := make(map[string]int32, len(src))
	for key, value := range src {
		dst[key] = value
	}
	return dst
}
