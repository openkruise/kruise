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

func (src *UnitedDeployment) ConvertTo(dstRaw conversion.Hub) error {
	switch t := dstRaw.(type) {
	case *v1beta1.UnitedDeployment:
		dst := dstRaw.(*v1beta1.UnitedDeployment)
		dst.ObjectMeta = src.ObjectMeta
		dst.Spec.Replicas = src.Spec.Replicas
		dst.Spec.Selector = src.Spec.Selector
		if src.Spec.Template.StatefulSetTemplate != nil {
			dst.Spec.Template.StatefulSetTemplate = &v1beta1.StatefulSetTemplateSpec{}
			dst.Spec.Template.StatefulSetTemplate.ObjectMeta = src.Spec.Template.StatefulSetTemplate.ObjectMeta
			dst.Spec.Template.StatefulSetTemplate.Spec = src.Spec.Template.StatefulSetTemplate.Spec
		}
		if src.Spec.Template.AdvancedStatefulSetTemplate != nil {
			dst.Spec.Template.AdvancedStatefulSetTemplate = &v1beta1.AdvancedStatefulSetTemplateSpec{}
			dst.Spec.Template.AdvancedStatefulSetTemplate.ObjectMeta = src.Spec.Template.AdvancedStatefulSetTemplate.ObjectMeta
			dst.Spec.Template.AdvancedStatefulSetTemplate.Spec = src.Spec.Template.AdvancedStatefulSetTemplate.Spec
		}
		if src.Spec.Template.CloneSetTemplate != nil {
			dst.Spec.Template.CloneSetTemplate = &v1beta1.CloneSetTemplateSpec{}
			dst.Spec.Template.CloneSetTemplate.ObjectMeta = src.Spec.Template.CloneSetTemplate.ObjectMeta
			dst.Spec.Template.CloneSetTemplate.Spec.Replicas = src.Spec.Template.CloneSetTemplate.Spec.Replicas
			dst.Spec.Template.CloneSetTemplate.Spec.Selector = src.Spec.Template.CloneSetTemplate.Spec.Selector
			dst.Spec.Template.CloneSetTemplate.Spec.Template = src.Spec.Template.CloneSetTemplate.Spec.Template
			dst.Spec.Template.CloneSetTemplate.Spec.VolumeClaimTemplates = src.Spec.Template.CloneSetTemplate.Spec.VolumeClaimTemplates
			dst.Spec.Template.CloneSetTemplate.Spec.ScaleStrategy.PodsToDelete = src.Spec.Template.CloneSetTemplate.Spec.ScaleStrategy.PodsToDelete
			dst.Spec.Template.CloneSetTemplate.Spec.ScaleStrategy.MaxUnavailable = src.Spec.Template.CloneSetTemplate.Spec.ScaleStrategy.MaxUnavailable
			dst.Spec.Template.CloneSetTemplate.Spec.ScaleStrategy.DisablePVCReuse = src.Spec.Template.CloneSetTemplate.Spec.ScaleStrategy.DisablePVCReuse
			dst.Spec.Template.CloneSetTemplate.Spec.UpdateStrategy.Type = v1beta1.CloneSetUpdateStrategyType(src.Spec.Template.CloneSetTemplate.Spec.UpdateStrategy.Type)
			dst.Spec.Template.CloneSetTemplate.Spec.UpdateStrategy.Partition = src.Spec.Template.CloneSetTemplate.Spec.UpdateStrategy.Partition
			dst.Spec.Template.CloneSetTemplate.Spec.UpdateStrategy.MaxUnavailable = src.Spec.Template.CloneSetTemplate.Spec.UpdateStrategy.MaxUnavailable
			dst.Spec.Template.CloneSetTemplate.Spec.UpdateStrategy.MaxSurge = src.Spec.Template.CloneSetTemplate.Spec.UpdateStrategy.MaxSurge
			dst.Spec.Template.CloneSetTemplate.Spec.UpdateStrategy.Paused = src.Spec.Template.CloneSetTemplate.Spec.UpdateStrategy.Paused
			dst.Spec.Template.CloneSetTemplate.Spec.UpdateStrategy.PriorityStrategy = src.Spec.Template.CloneSetTemplate.Spec.UpdateStrategy.PriorityStrategy
			scatterStrategy := make([]v1beta1.UpdateScatterTerm, len(src.Spec.Template.CloneSetTemplate.Spec.UpdateStrategy.ScatterStrategy))
			for scatterStrategy_idx, scatterStrategy_val := range src.Spec.Template.CloneSetTemplate.Spec.UpdateStrategy.ScatterStrategy {
				scatterStrategy[scatterStrategy_idx].Key = scatterStrategy_val.Key
				scatterStrategy[scatterStrategy_idx].Value = scatterStrategy_val.Value
			}
			dst.Spec.Template.CloneSetTemplate.Spec.UpdateStrategy.ScatterStrategy = scatterStrategy
			dst.Spec.Template.CloneSetTemplate.Spec.UpdateStrategy.InPlaceUpdateStrategy = src.Spec.Template.CloneSetTemplate.Spec.UpdateStrategy.InPlaceUpdateStrategy
			dst.Spec.Template.CloneSetTemplate.Spec.RevisionHistoryLimit = src.Spec.Template.CloneSetTemplate.Spec.RevisionHistoryLimit
			dst.Spec.Template.CloneSetTemplate.Spec.MinReadySeconds = src.Spec.Template.CloneSetTemplate.Spec.MinReadySeconds
			dst.Spec.Template.CloneSetTemplate.Spec.Lifecycle = src.Spec.Template.CloneSetTemplate.Spec.Lifecycle
		}
		if src.Spec.Template.DeploymentTemplate != nil {
			dst.Spec.Template.DeploymentTemplate = &v1beta1.DeploymentTemplateSpec{}
			dst.Spec.Template.DeploymentTemplate.ObjectMeta = src.Spec.Template.DeploymentTemplate.ObjectMeta
			dst.Spec.Template.DeploymentTemplate.Spec = src.Spec.Template.DeploymentTemplate.Spec
		}
		subsets := make([]v1beta1.Subset, len(src.Spec.Topology.Subsets))
		for subsets_idx, subsets_val := range src.Spec.Topology.Subsets {
			subsets[subsets_idx].Name = subsets_val.Name
			subsets[subsets_idx].NodeSelectorTerm = subsets_val.NodeSelectorTerm
			subsets[subsets_idx].Tolerations = subsets_val.Tolerations
			subsets[subsets_idx].Replicas = subsets_val.Replicas
			subsets[subsets_idx].Patch = subsets_val.Patch
		}
		dst.Spec.Topology.Subsets = subsets
		dst.Spec.UpdateStrategy.Type = v1beta1.UpdateStrategyType(src.Spec.UpdateStrategy.Type)
		if src.Spec.UpdateStrategy.ManualUpdate != nil {
			dst.Spec.UpdateStrategy.ManualUpdate = &v1beta1.ManualUpdate{}
			dst.Spec.UpdateStrategy.ManualUpdate.Partitions = src.Spec.UpdateStrategy.ManualUpdate.Partitions
		}
		dst.Spec.RevisionHistoryLimit = src.Spec.RevisionHistoryLimit
		dst.Status.ObservedGeneration = src.Status.ObservedGeneration
		dst.Status.ReadyReplicas = src.Status.ReadyReplicas
		dst.Status.Replicas = src.Status.Replicas
		dst.Status.UpdatedReplicas = src.Status.UpdatedReplicas
		dst.Status.UpdatedReadyReplicas = src.Status.UpdatedReadyReplicas
		dst.Status.CollisionCount = src.Status.CollisionCount
		dst.Status.CurrentRevision = src.Status.CurrentRevision
		dst.Status.SubsetReplicas = src.Status.SubsetReplicas
		conditions := make([]v1beta1.UnitedDeploymentCondition, len(src.Status.Conditions))
		for conditions_idx, conditions_val := range src.Status.Conditions {
			conditions[conditions_idx].Type = v1beta1.UnitedDeploymentConditionType(conditions_val.Type)
			conditions[conditions_idx].Status = conditions_val.Status
			conditions[conditions_idx].LastTransitionTime = conditions_val.LastTransitionTime
			conditions[conditions_idx].Reason = conditions_val.Reason
			conditions[conditions_idx].Message = conditions_val.Message
		}
		dst.Status.Conditions = conditions
		if src.Status.UpdateStatus != nil {
			dst.Status.UpdateStatus = &v1beta1.UpdateStatus{}
			dst.Status.UpdateStatus.UpdatedRevision = src.Status.UpdateStatus.UpdatedRevision
			dst.Status.UpdateStatus.CurrentPartitions = src.Status.UpdateStatus.CurrentPartitions
		}
		dst.Status.LabelSelector = src.Status.LabelSelector
	default:
		return fmt.Errorf("unsupported type %v", t)
	}
	return nil
}

func (dst *UnitedDeployment) ConvertFrom(srcRaw conversion.Hub) error {
	switch t := srcRaw.(type) {
	case *v1beta1.UnitedDeployment:
		src := srcRaw.(*v1beta1.UnitedDeployment)
		dst.ObjectMeta = src.ObjectMeta
		dst.Spec.Replicas = src.Spec.Replicas
		dst.Spec.Selector = src.Spec.Selector
		if src.Spec.Template.StatefulSetTemplate != nil {
			dst.Spec.Template.StatefulSetTemplate = &StatefulSetTemplateSpec{}
			dst.Spec.Template.StatefulSetTemplate.ObjectMeta = src.Spec.Template.StatefulSetTemplate.ObjectMeta
			dst.Spec.Template.StatefulSetTemplate.Spec = src.Spec.Template.StatefulSetTemplate.Spec
		}
		if src.Spec.Template.AdvancedStatefulSetTemplate != nil {
			dst.Spec.Template.AdvancedStatefulSetTemplate = &AdvancedStatefulSetTemplateSpec{}
			dst.Spec.Template.AdvancedStatefulSetTemplate.ObjectMeta = src.Spec.Template.AdvancedStatefulSetTemplate.ObjectMeta
			dst.Spec.Template.AdvancedStatefulSetTemplate.Spec = src.Spec.Template.AdvancedStatefulSetTemplate.Spec
		}
		if src.Spec.Template.CloneSetTemplate != nil {
			dst.Spec.Template.CloneSetTemplate = &CloneSetTemplateSpec{}
			dst.Spec.Template.CloneSetTemplate.ObjectMeta = src.Spec.Template.CloneSetTemplate.ObjectMeta
			dst.Spec.Template.CloneSetTemplate.Spec.Replicas = src.Spec.Template.CloneSetTemplate.Spec.Replicas
			dst.Spec.Template.CloneSetTemplate.Spec.Selector = src.Spec.Template.CloneSetTemplate.Spec.Selector
			dst.Spec.Template.CloneSetTemplate.Spec.Template = src.Spec.Template.CloneSetTemplate.Spec.Template
			dst.Spec.Template.CloneSetTemplate.Spec.VolumeClaimTemplates = src.Spec.Template.CloneSetTemplate.Spec.VolumeClaimTemplates
			dst.Spec.Template.CloneSetTemplate.Spec.ScaleStrategy.PodsToDelete = src.Spec.Template.CloneSetTemplate.Spec.ScaleStrategy.PodsToDelete
			dst.Spec.Template.CloneSetTemplate.Spec.ScaleStrategy.MaxUnavailable = src.Spec.Template.CloneSetTemplate.Spec.ScaleStrategy.MaxUnavailable
			dst.Spec.Template.CloneSetTemplate.Spec.ScaleStrategy.DisablePVCReuse = src.Spec.Template.CloneSetTemplate.Spec.ScaleStrategy.DisablePVCReuse
			dst.Spec.Template.CloneSetTemplate.Spec.UpdateStrategy.Type = CloneSetUpdateStrategyType(src.Spec.Template.CloneSetTemplate.Spec.UpdateStrategy.Type)
			dst.Spec.Template.CloneSetTemplate.Spec.UpdateStrategy.Partition = src.Spec.Template.CloneSetTemplate.Spec.UpdateStrategy.Partition
			dst.Spec.Template.CloneSetTemplate.Spec.UpdateStrategy.MaxUnavailable = src.Spec.Template.CloneSetTemplate.Spec.UpdateStrategy.MaxUnavailable
			dst.Spec.Template.CloneSetTemplate.Spec.UpdateStrategy.MaxSurge = src.Spec.Template.CloneSetTemplate.Spec.UpdateStrategy.MaxSurge
			dst.Spec.Template.CloneSetTemplate.Spec.UpdateStrategy.Paused = src.Spec.Template.CloneSetTemplate.Spec.UpdateStrategy.Paused
			dst.Spec.Template.CloneSetTemplate.Spec.UpdateStrategy.PriorityStrategy = src.Spec.Template.CloneSetTemplate.Spec.UpdateStrategy.PriorityStrategy
			scatterStrategy := make([]UpdateScatterTerm, len(src.Spec.Template.CloneSetTemplate.Spec.UpdateStrategy.ScatterStrategy))
			for scatterStrategy_idx, scatterStrategy_val := range src.Spec.Template.CloneSetTemplate.Spec.UpdateStrategy.ScatterStrategy {
				scatterStrategy[scatterStrategy_idx].Key = scatterStrategy_val.Key
				scatterStrategy[scatterStrategy_idx].Value = scatterStrategy_val.Value
			}
			dst.Spec.Template.CloneSetTemplate.Spec.UpdateStrategy.ScatterStrategy = scatterStrategy
			dst.Spec.Template.CloneSetTemplate.Spec.UpdateStrategy.InPlaceUpdateStrategy = src.Spec.Template.CloneSetTemplate.Spec.UpdateStrategy.InPlaceUpdateStrategy
			dst.Spec.Template.CloneSetTemplate.Spec.RevisionHistoryLimit = src.Spec.Template.CloneSetTemplate.Spec.RevisionHistoryLimit
			dst.Spec.Template.CloneSetTemplate.Spec.MinReadySeconds = src.Spec.Template.CloneSetTemplate.Spec.MinReadySeconds
			dst.Spec.Template.CloneSetTemplate.Spec.Lifecycle = src.Spec.Template.CloneSetTemplate.Spec.Lifecycle
		}
		if src.Spec.Template.DeploymentTemplate != nil {
			dst.Spec.Template.DeploymentTemplate = &DeploymentTemplateSpec{}
			dst.Spec.Template.DeploymentTemplate.ObjectMeta = src.Spec.Template.DeploymentTemplate.ObjectMeta
			dst.Spec.Template.DeploymentTemplate.Spec = src.Spec.Template.DeploymentTemplate.Spec
		}
		subsets := make([]Subset, len(src.Spec.Topology.Subsets))
		for subsets_idx, subsets_val := range src.Spec.Topology.Subsets {
			subsets[subsets_idx].Name = subsets_val.Name
			subsets[subsets_idx].NodeSelectorTerm = subsets_val.NodeSelectorTerm
			subsets[subsets_idx].Tolerations = subsets_val.Tolerations
			subsets[subsets_idx].Replicas = subsets_val.Replicas
			subsets[subsets_idx].Patch = subsets_val.Patch
		}
		dst.Spec.Topology.Subsets = subsets
		dst.Spec.UpdateStrategy.Type = UpdateStrategyType(src.Spec.UpdateStrategy.Type)
		if src.Spec.UpdateStrategy.ManualUpdate != nil {
			dst.Spec.UpdateStrategy.ManualUpdate = &ManualUpdate{}
			dst.Spec.UpdateStrategy.ManualUpdate.Partitions = src.Spec.UpdateStrategy.ManualUpdate.Partitions
		}
		dst.Spec.RevisionHistoryLimit = src.Spec.RevisionHistoryLimit
		dst.Status.ObservedGeneration = src.Status.ObservedGeneration
		dst.Status.ReadyReplicas = src.Status.ReadyReplicas
		dst.Status.Replicas = src.Status.Replicas
		dst.Status.UpdatedReplicas = src.Status.UpdatedReplicas
		dst.Status.UpdatedReadyReplicas = src.Status.UpdatedReadyReplicas
		dst.Status.CollisionCount = src.Status.CollisionCount
		dst.Status.CurrentRevision = src.Status.CurrentRevision
		dst.Status.SubsetReplicas = src.Status.SubsetReplicas
		conditions := make([]UnitedDeploymentCondition, len(src.Status.Conditions))
		for conditions_idx, conditions_val := range src.Status.Conditions {
			conditions[conditions_idx].Type = UnitedDeploymentConditionType(conditions_val.Type)
			conditions[conditions_idx].Status = conditions_val.Status
			conditions[conditions_idx].LastTransitionTime = conditions_val.LastTransitionTime
			conditions[conditions_idx].Reason = conditions_val.Reason
			conditions[conditions_idx].Message = conditions_val.Message
		}
		dst.Status.Conditions = conditions
		if src.Status.UpdateStatus != nil {
			dst.Status.UpdateStatus = &UpdateStatus{}
			dst.Status.UpdateStatus.UpdatedRevision = src.Status.UpdateStatus.UpdatedRevision
			dst.Status.UpdateStatus.CurrentPartitions = src.Status.UpdateStatus.CurrentPartitions
		}
		dst.Status.LabelSelector = src.Status.LabelSelector
	default:
		return fmt.Errorf("unsupported type %v", t)
	}
	return nil
}
