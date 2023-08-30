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

func (src *CloneSet) ConvertTo(dstRaw conversion.Hub) error {
	switch t := dstRaw.(type) {
	case *v1beta1.CloneSet:
		dst := dstRaw.(*v1beta1.CloneSet)
		dst.ObjectMeta = src.ObjectMeta
		dst.Spec.Replicas = src.Spec.Replicas
		dst.Spec.Selector = src.Spec.Selector
		dst.Spec.Template = src.Spec.Template
		dst.Spec.VolumeClaimTemplates = src.Spec.VolumeClaimTemplates
		dst.Spec.ScaleStrategy.PodsToDelete = src.Spec.ScaleStrategy.PodsToDelete
		dst.Spec.ScaleStrategy.MaxUnavailable = src.Spec.ScaleStrategy.MaxUnavailable
		dst.Spec.ScaleStrategy.DisablePVCReuse = src.Spec.ScaleStrategy.DisablePVCReuse
		dst.Spec.UpdateStrategy.Type = v1beta1.CloneSetUpdateStrategyType(src.Spec.UpdateStrategy.Type)
		dst.Spec.UpdateStrategy.Partition = src.Spec.UpdateStrategy.Partition
		dst.Spec.UpdateStrategy.MaxUnavailable = src.Spec.UpdateStrategy.MaxUnavailable
		dst.Spec.UpdateStrategy.MaxSurge = src.Spec.UpdateStrategy.MaxSurge
		dst.Spec.UpdateStrategy.Paused = src.Spec.UpdateStrategy.Paused
		dst.Spec.UpdateStrategy.PriorityStrategy = src.Spec.UpdateStrategy.PriorityStrategy
		scatterStrategy := make([]v1beta1.UpdateScatterTerm, len(src.Spec.UpdateStrategy.ScatterStrategy))
		for scatterStrategy_idx, scatterStrategy_val := range src.Spec.UpdateStrategy.ScatterStrategy {
			scatterStrategy[scatterStrategy_idx].Key = scatterStrategy_val.Key
			scatterStrategy[scatterStrategy_idx].Value = scatterStrategy_val.Value
		}
		dst.Spec.UpdateStrategy.ScatterStrategy = scatterStrategy
		dst.Spec.UpdateStrategy.InPlaceUpdateStrategy = src.Spec.UpdateStrategy.InPlaceUpdateStrategy
		dst.Spec.RevisionHistoryLimit = src.Spec.RevisionHistoryLimit
		dst.Spec.MinReadySeconds = src.Spec.MinReadySeconds
		dst.Spec.Lifecycle = src.Spec.Lifecycle
		dst.Status.ObservedGeneration = src.Status.ObservedGeneration
		dst.Status.Replicas = src.Status.Replicas
		dst.Status.ReadyReplicas = src.Status.ReadyReplicas
		dst.Status.AvailableReplicas = src.Status.AvailableReplicas
		dst.Status.UpdatedReplicas = src.Status.UpdatedReplicas
		dst.Status.UpdatedReadyReplicas = src.Status.UpdatedReadyReplicas
		dst.Status.UpdatedAvailableReplicas = src.Status.UpdatedAvailableReplicas
		dst.Status.ExpectedUpdatedReplicas = src.Status.ExpectedUpdatedReplicas
		dst.Status.UpdateRevision = src.Status.UpdateRevision
		dst.Status.CurrentRevision = src.Status.CurrentRevision
		dst.Status.CollisionCount = src.Status.CollisionCount
		conditions := make([]v1beta1.CloneSetCondition, len(src.Status.Conditions))
		for conditions_idx, conditions_val := range src.Status.Conditions {
			conditions[conditions_idx].Type = v1beta1.CloneSetConditionType(conditions_val.Type)
			conditions[conditions_idx].Status = conditions_val.Status
			conditions[conditions_idx].LastTransitionTime = conditions_val.LastTransitionTime
			conditions[conditions_idx].Reason = conditions_val.Reason
			conditions[conditions_idx].Message = conditions_val.Message
		}
		dst.Status.Conditions = conditions
		dst.Status.LabelSelector = src.Status.LabelSelector
	default:
		return fmt.Errorf("unsupported type %v", t)
	}
	return nil
}

func (dst *CloneSet) ConvertFrom(srcRaw conversion.Hub) error {
	switch t := srcRaw.(type) {
	case *v1beta1.CloneSet:
		src := srcRaw.(*v1beta1.CloneSet)
		dst.ObjectMeta = src.ObjectMeta
		dst.Spec.Replicas = src.Spec.Replicas
		dst.Spec.Selector = src.Spec.Selector
		dst.Spec.Template = src.Spec.Template
		dst.Spec.VolumeClaimTemplates = src.Spec.VolumeClaimTemplates
		dst.Spec.ScaleStrategy.PodsToDelete = src.Spec.ScaleStrategy.PodsToDelete
		dst.Spec.ScaleStrategy.MaxUnavailable = src.Spec.ScaleStrategy.MaxUnavailable
		dst.Spec.ScaleStrategy.DisablePVCReuse = src.Spec.ScaleStrategy.DisablePVCReuse
		dst.Spec.UpdateStrategy.Type = CloneSetUpdateStrategyType(src.Spec.UpdateStrategy.Type)
		dst.Spec.UpdateStrategy.Partition = src.Spec.UpdateStrategy.Partition
		dst.Spec.UpdateStrategy.MaxUnavailable = src.Spec.UpdateStrategy.MaxUnavailable
		dst.Spec.UpdateStrategy.MaxSurge = src.Spec.UpdateStrategy.MaxSurge
		dst.Spec.UpdateStrategy.Paused = src.Spec.UpdateStrategy.Paused
		dst.Spec.UpdateStrategy.PriorityStrategy = src.Spec.UpdateStrategy.PriorityStrategy
		scatterStrategy := make([]UpdateScatterTerm, len(src.Spec.UpdateStrategy.ScatterStrategy))
		for scatterStrategy_idx, scatterStrategy_val := range src.Spec.UpdateStrategy.ScatterStrategy {
			scatterStrategy[scatterStrategy_idx].Key = scatterStrategy_val.Key
			scatterStrategy[scatterStrategy_idx].Value = scatterStrategy_val.Value
		}
		dst.Spec.UpdateStrategy.ScatterStrategy = scatterStrategy
		dst.Spec.UpdateStrategy.InPlaceUpdateStrategy = src.Spec.UpdateStrategy.InPlaceUpdateStrategy
		dst.Spec.RevisionHistoryLimit = src.Spec.RevisionHistoryLimit
		dst.Spec.MinReadySeconds = src.Spec.MinReadySeconds
		dst.Spec.Lifecycle = src.Spec.Lifecycle
		dst.Status.ObservedGeneration = src.Status.ObservedGeneration
		dst.Status.Replicas = src.Status.Replicas
		dst.Status.ReadyReplicas = src.Status.ReadyReplicas
		dst.Status.AvailableReplicas = src.Status.AvailableReplicas
		dst.Status.UpdatedReplicas = src.Status.UpdatedReplicas
		dst.Status.UpdatedReadyReplicas = src.Status.UpdatedReadyReplicas
		dst.Status.UpdatedAvailableReplicas = src.Status.UpdatedAvailableReplicas
		dst.Status.ExpectedUpdatedReplicas = src.Status.ExpectedUpdatedReplicas
		dst.Status.UpdateRevision = src.Status.UpdateRevision
		dst.Status.CurrentRevision = src.Status.CurrentRevision
		dst.Status.CollisionCount = src.Status.CollisionCount
		conditions := make([]CloneSetCondition, len(src.Status.Conditions))
		for conditions_idx, conditions_val := range src.Status.Conditions {
			conditions[conditions_idx].Type = CloneSetConditionType(conditions_val.Type)
			conditions[conditions_idx].Status = conditions_val.Status
			conditions[conditions_idx].LastTransitionTime = conditions_val.LastTransitionTime
			conditions[conditions_idx].Reason = conditions_val.Reason
			conditions[conditions_idx].Message = conditions_val.Message
		}
		dst.Status.Conditions = conditions
		dst.Status.LabelSelector = src.Status.LabelSelector
	default:
		return fmt.Errorf("unsupported type %v", t)
	}
	return nil
}
