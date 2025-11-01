/*
Copyright 2025 The Kruise Authors.

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

func (cs *CloneSet) ConvertTo(dst conversion.Hub) error {
	switch t := dst.(type) {
	case *v1beta1.CloneSet:
		csv1beta1 := dst.(*v1beta1.CloneSet)
		csv1beta1.ObjectMeta = cs.ObjectMeta

		// spec
		csv1beta1.Spec = v1beta1.CloneSetSpec{
			Replicas:             cs.Spec.Replicas,
			Selector:             cs.Spec.Selector,
			Template:             cs.Spec.Template,
			VolumeClaimTemplates: cs.Spec.VolumeClaimTemplates,
			RevisionHistoryLimit: cs.Spec.RevisionHistoryLimit,
			MinReadySeconds:      cs.Spec.MinReadySeconds,
			Lifecycle:            cs.Spec.Lifecycle,
		}

		// Convert ScaleStrategy
		csv1beta1.Spec.ScaleStrategy = v1beta1.CloneSetScaleStrategy{
			PodsToDelete:    cs.Spec.ScaleStrategy.PodsToDelete,
			MaxUnavailable:  cs.Spec.ScaleStrategy.MaxUnavailable,
			DisablePVCReuse: cs.Spec.ScaleStrategy.DisablePVCReuse,
			// Convert spec field to v1beta1
			ExcludePreparingDelete: cs.Spec.ScaleStrategy.ExcludePreparingDelete,
		}

		// Convert label to spec field for v1beta1
		// If the label is set to "true", set the spec field to true
		if cs.Labels != nil && cs.Labels[CloneSetScalingExcludePreparingDeleteKey] == "true" {
			csv1beta1.Spec.ScaleStrategy.ExcludePreparingDelete = true
		}

		// Convert UpdateStrategy
		csv1beta1.Spec.UpdateStrategy = v1beta1.CloneSetUpdateStrategy{
			Type:                  v1beta1.CloneSetUpdateStrategyType(cs.Spec.UpdateStrategy.Type),
			Partition:             cs.Spec.UpdateStrategy.Partition,
			MaxUnavailable:        cs.Spec.UpdateStrategy.MaxUnavailable,
			MaxSurge:              cs.Spec.UpdateStrategy.MaxSurge,
			Paused:                cs.Spec.UpdateStrategy.Paused,
			PriorityStrategy:      cs.Spec.UpdateStrategy.PriorityStrategy,
			ScatterStrategy:       convertUpdateScatterStrategyToV1beta1(cs.Spec.UpdateStrategy.ScatterStrategy),
			InPlaceUpdateStrategy: cs.Spec.UpdateStrategy.InPlaceUpdateStrategy,
		}

		// status
		csv1beta1.Status = v1beta1.CloneSetStatus{
			ObservedGeneration:       cs.Status.ObservedGeneration,
			Replicas:                 cs.Status.Replicas,
			ReadyReplicas:            cs.Status.ReadyReplicas,
			AvailableReplicas:        cs.Status.AvailableReplicas,
			UpdatedReplicas:          cs.Status.UpdatedReplicas,
			UpdatedReadyReplicas:     cs.Status.UpdatedReadyReplicas,
			UpdatedAvailableReplicas: cs.Status.UpdatedAvailableReplicas,
			ExpectedUpdatedReplicas:  cs.Status.ExpectedUpdatedReplicas,
			UpdateRevision:           cs.Status.UpdateRevision,
			CurrentRevision:          cs.Status.CurrentRevision,
			CollisionCount:           cs.Status.CollisionCount,
			Conditions:               convertCloneSetConditionsToV1beta1(cs.Status.Conditions),
			LabelSelector:            cs.Status.LabelSelector,
		}

		return nil

	default:
		return fmt.Errorf("unsupported type %v", t)
	}
}

func (cs *CloneSet) ConvertFrom(src conversion.Hub) error {
	switch t := src.(type) {
	case *v1beta1.CloneSet:
		csv1beta1 := src.(*v1beta1.CloneSet)
		cs.ObjectMeta = csv1beta1.ObjectMeta

		// spec
		cs.Spec = CloneSetSpec{
			Replicas:             csv1beta1.Spec.Replicas,
			Selector:             csv1beta1.Spec.Selector,
			Template:             csv1beta1.Spec.Template,
			VolumeClaimTemplates: csv1beta1.Spec.VolumeClaimTemplates,
			RevisionHistoryLimit: csv1beta1.Spec.RevisionHistoryLimit,
			MinReadySeconds:      csv1beta1.Spec.MinReadySeconds,
			Lifecycle:            csv1beta1.Spec.Lifecycle,
		}

		// Convert ScaleStrategy
		cs.Spec.ScaleStrategy = CloneSetScaleStrategy{
			PodsToDelete:           csv1beta1.Spec.ScaleStrategy.PodsToDelete,
			MaxUnavailable:         csv1beta1.Spec.ScaleStrategy.MaxUnavailable,
			DisablePVCReuse:        csv1beta1.Spec.ScaleStrategy.DisablePVCReuse,
			ExcludePreparingDelete: csv1beta1.Spec.ScaleStrategy.ExcludePreparingDelete,
		}

		// Convert UpdateStrategy
		cs.Spec.UpdateStrategy = CloneSetUpdateStrategy{
			Type:                  CloneSetUpdateStrategyType(csv1beta1.Spec.UpdateStrategy.Type),
			Partition:             csv1beta1.Spec.UpdateStrategy.Partition,
			MaxUnavailable:        csv1beta1.Spec.UpdateStrategy.MaxUnavailable,
			MaxSurge:              csv1beta1.Spec.UpdateStrategy.MaxSurge,
			Paused:                csv1beta1.Spec.UpdateStrategy.Paused,
			PriorityStrategy:      csv1beta1.Spec.UpdateStrategy.PriorityStrategy,
			ScatterStrategy:       convertUpdateScatterStrategyFromV1beta1(csv1beta1.Spec.UpdateStrategy.ScatterStrategy),
			InPlaceUpdateStrategy: csv1beta1.Spec.UpdateStrategy.InPlaceUpdateStrategy,
		}

		// status
		cs.Status = CloneSetStatus{
			ObservedGeneration:       csv1beta1.Status.ObservedGeneration,
			Replicas:                 csv1beta1.Status.Replicas,
			ReadyReplicas:            csv1beta1.Status.ReadyReplicas,
			AvailableReplicas:        csv1beta1.Status.AvailableReplicas,
			UpdatedReplicas:          csv1beta1.Status.UpdatedReplicas,
			UpdatedReadyReplicas:     csv1beta1.Status.UpdatedReadyReplicas,
			UpdatedAvailableReplicas: csv1beta1.Status.UpdatedAvailableReplicas,
			ExpectedUpdatedReplicas:  csv1beta1.Status.ExpectedUpdatedReplicas,
			UpdateRevision:           csv1beta1.Status.UpdateRevision,
			CurrentRevision:          csv1beta1.Status.CurrentRevision,
			CollisionCount:           csv1beta1.Status.CollisionCount,
			Conditions:               convertCloneSetConditionsFromV1beta1(csv1beta1.Status.Conditions),
			LabelSelector:            csv1beta1.Status.LabelSelector,
		}

		return nil

	default:
		return fmt.Errorf("unsupported type %v", t)
	}
}

func convertUpdateScatterStrategyToV1beta1(src UpdateScatterStrategy) v1beta1.UpdateScatterStrategy {
	if src == nil {
		return nil
	}
	dst := make(v1beta1.UpdateScatterStrategy, len(src))
	for i, term := range src {
		dst[i] = v1beta1.UpdateScatterTerm{
			Key:   term.Key,
			Value: term.Value,
		}
	}
	return dst
}

func convertUpdateScatterStrategyFromV1beta1(src v1beta1.UpdateScatterStrategy) UpdateScatterStrategy {
	if src == nil {
		return nil
	}
	dst := make(UpdateScatterStrategy, len(src))
	for i, term := range src {
		dst[i] = UpdateScatterTerm{
			Key:   term.Key,
			Value: term.Value,
		}
	}
	return dst
}

func convertCloneSetConditionsToV1beta1(src []CloneSetCondition) []v1beta1.CloneSetCondition {
	if src == nil {
		return nil
	}
	dst := make([]v1beta1.CloneSetCondition, len(src))
	for i, condition := range src {
		dst[i] = v1beta1.CloneSetCondition{
			Type:               v1beta1.CloneSetConditionType(condition.Type),
			Status:             condition.Status,
			LastTransitionTime: condition.LastTransitionTime,
			Reason:             condition.Reason,
			Message:            condition.Message,
		}
	}
	return dst
}

func convertCloneSetConditionsFromV1beta1(src []v1beta1.CloneSetCondition) []CloneSetCondition {
	if src == nil {
		return nil
	}
	dst := make([]CloneSetCondition, len(src))
	for i, condition := range src {
		dst[i] = CloneSetCondition{
			Type:               CloneSetConditionType(condition.Type),
			Status:             condition.Status,
			LastTransitionTime: condition.LastTransitionTime,
			Reason:             condition.Reason,
			Message:            condition.Message,
		}
	}
	return dst
}
