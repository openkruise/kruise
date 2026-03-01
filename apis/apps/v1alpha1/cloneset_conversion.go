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
	"strconv"

	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/conversion"

	appspub "github.com/openkruise/kruise/apis/apps/pub"
	"github.com/openkruise/kruise/apis/apps/v1beta1"
)

func (cs *CloneSet) ConvertTo(dst conversion.Hub) error {
	switch t := dst.(type) {
	case *v1beta1.CloneSet:
		csv1beta1 := dst.(*v1beta1.CloneSet)
		csv1beta1.ObjectMeta = cs.ObjectMeta

		// spec
		csv1beta1.Spec = v1beta1.CloneSetSpec{
			Replicas:                cs.Spec.Replicas,
			Selector:                cs.Spec.Selector,
			Template:                cs.Spec.Template,
			VolumeClaimTemplates:    cs.Spec.VolumeClaimTemplates,
			RevisionHistoryLimit:    cs.Spec.RevisionHistoryLimit,
			MinReadySeconds:         cs.Spec.MinReadySeconds,
			Lifecycle:               cs.Spec.Lifecycle,
			ProgressDeadlineSeconds: cs.Spec.ProgressDeadlineSeconds,
		}

		// Convert ScaleStrategy
		csv1beta1.Spec.ScaleStrategy = v1beta1.CloneSetScaleStrategy{
			PodsToDelete:   cs.Spec.ScaleStrategy.PodsToDelete,
			MaxUnavailable: cs.Spec.ScaleStrategy.MaxUnavailable,
			// Convert DisablePVCReuse (v1alpha1) to EnablePVCReuse (v1beta1) with inverted logic
			EnablePVCReuse: !cs.Spec.ScaleStrategy.DisablePVCReuse,
		}

		// Convert label to spec field for v1beta1
		// If the label is set to "true", set the spec field to true
		if cs.Labels != nil && cs.Labels[CloneSetScalingExcludePreparingDeleteKey] == "true" {
			csv1beta1.Spec.ScaleStrategy.ExcludePreparingDelete = true
		}

		// Convert UpdateStrategy
		// Map v1alpha1 Type to v1beta1 Type and PodUpdatePolicy
		var strategyType v1beta1.CloneSetUpdateStrategyType
		var podUpdatePolicy v1beta1.CloneSetPodUpdateStrategyType

		switch cs.Spec.UpdateStrategy.Type {
		case OnDeleteCloneSetUpdateStrategyType:
			strategyType = v1beta1.OnDeleteCloneSetUpdateStrategyType
			podUpdatePolicy = "" // not used for OnDelete
		case RecreateCloneSetUpdateStrategyType:
			strategyType = v1beta1.RollingUpdateCloneSetUpdateStrategyType
			podUpdatePolicy = v1beta1.RecreateCloneSetPodUpdateStrategyType
		case InPlaceIfPossibleCloneSetUpdateStrategyType:
			strategyType = v1beta1.RollingUpdateCloneSetUpdateStrategyType
			podUpdatePolicy = v1beta1.InPlaceIfPossibleCloneSetPodUpdateStrategyType
		case InPlaceOnlyCloneSetUpdateStrategyType:
			strategyType = v1beta1.RollingUpdateCloneSetUpdateStrategyType
			podUpdatePolicy = v1beta1.InPlaceOnlyCloneSetPodUpdateStrategyType
		default:
			// Default to RollingUpdate with ReCreate policy
			strategyType = v1beta1.RollingUpdateCloneSetUpdateStrategyType
			podUpdatePolicy = v1beta1.RecreateCloneSetPodUpdateStrategyType
		}

		csv1beta1.Spec.UpdateStrategy = v1beta1.CloneSetUpdateStrategy{
			Type: strategyType,
		}

		// Only set RollingUpdate if it's not OnDelete
		if strategyType != v1beta1.OnDeleteCloneSetUpdateStrategyType {
			// Convert InPlaceUpdateStrategy from v1alpha1 (copy existing + convert annotations)
			inPlaceStrategy := convertInPlaceUpdateStrategyToV1beta1(cs.Spec.UpdateStrategy.InPlaceUpdateStrategy, cs.Annotations)

			csv1beta1.Spec.UpdateStrategy.RollingUpdate = &v1beta1.RollingUpdateCloneSetStrategy{
				Partition:             cs.Spec.UpdateStrategy.Partition,
				MaxUnavailable:        cs.Spec.UpdateStrategy.MaxUnavailable,
				MaxSurge:              cs.Spec.UpdateStrategy.MaxSurge,
				PodUpdatePolicy:       podUpdatePolicy,
				Paused:                cs.Spec.UpdateStrategy.Paused,
				PriorityStrategy:      cs.Spec.UpdateStrategy.PriorityStrategy,
				ScatterStrategy:       convertUpdateScatterStrategyToV1beta1(cs.Spec.UpdateStrategy.ScatterStrategy),
				InPlaceUpdateStrategy: inPlaceStrategy,
			}
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
			Replicas:                csv1beta1.Spec.Replicas,
			Selector:                csv1beta1.Spec.Selector,
			Template:                csv1beta1.Spec.Template,
			VolumeClaimTemplates:    csv1beta1.Spec.VolumeClaimTemplates,
			RevisionHistoryLimit:    csv1beta1.Spec.RevisionHistoryLimit,
			MinReadySeconds:         csv1beta1.Spec.MinReadySeconds,
			Lifecycle:               csv1beta1.Spec.Lifecycle,
			ProgressDeadlineSeconds: csv1beta1.Spec.ProgressDeadlineSeconds,
		}

		// Convert ScaleStrategy
		cs.Spec.ScaleStrategy = CloneSetScaleStrategy{
			PodsToDelete:   csv1beta1.Spec.ScaleStrategy.PodsToDelete,
			MaxUnavailable: csv1beta1.Spec.ScaleStrategy.MaxUnavailable,
			// Convert EnablePVCReuse (v1beta1) to DisablePVCReuse (v1alpha1) with inverted logic
			DisablePVCReuse: !csv1beta1.Spec.ScaleStrategy.EnablePVCReuse,
			// Note: v1beta1's ExcludePreparingDelete field is not converted back to v1alpha1
			// because v1alpha1 uses label-based configuration only
		}

		// Convert UpdateStrategy
		// Map v1beta1 Type and PodUpdatePolicy back to v1alpha1 Type
		var updateStrategyType CloneSetUpdateStrategyType

		if csv1beta1.Spec.UpdateStrategy.Type == v1beta1.OnDeleteCloneSetUpdateStrategyType {
			updateStrategyType = OnDeleteCloneSetUpdateStrategyType
		} else {
			// RollingUpdate type, check PodUpdatePolicy
			if csv1beta1.Spec.UpdateStrategy.RollingUpdate != nil {
				switch csv1beta1.Spec.UpdateStrategy.RollingUpdate.PodUpdatePolicy {
				case v1beta1.RecreateCloneSetPodUpdateStrategyType, "":
					updateStrategyType = RecreateCloneSetUpdateStrategyType
				case v1beta1.InPlaceIfPossibleCloneSetPodUpdateStrategyType:
					updateStrategyType = InPlaceIfPossibleCloneSetUpdateStrategyType
				case v1beta1.InPlaceOnlyCloneSetPodUpdateStrategyType:
					updateStrategyType = InPlaceOnlyCloneSetUpdateStrategyType
				default:
					updateStrategyType = RecreateCloneSetUpdateStrategyType
				}
			} else {
				updateStrategyType = RecreateCloneSetUpdateStrategyType
			}
		}

		cs.Spec.UpdateStrategy = CloneSetUpdateStrategy{
			Type: updateStrategyType,
		}

		// Copy RollingUpdate fields if present
		if csv1beta1.Spec.UpdateStrategy.RollingUpdate != nil {
			cs.Spec.UpdateStrategy.Partition = csv1beta1.Spec.UpdateStrategy.RollingUpdate.Partition
			cs.Spec.UpdateStrategy.MaxUnavailable = csv1beta1.Spec.UpdateStrategy.RollingUpdate.MaxUnavailable
			cs.Spec.UpdateStrategy.MaxSurge = csv1beta1.Spec.UpdateStrategy.RollingUpdate.MaxSurge
			cs.Spec.UpdateStrategy.Paused = csv1beta1.Spec.UpdateStrategy.RollingUpdate.Paused
			cs.Spec.UpdateStrategy.PriorityStrategy = csv1beta1.Spec.UpdateStrategy.RollingUpdate.PriorityStrategy
			cs.Spec.UpdateStrategy.ScatterStrategy = convertUpdateScatterStrategyFromV1beta1(csv1beta1.Spec.UpdateStrategy.RollingUpdate.ScatterStrategy)

			// Convert InPlaceUpdateStrategy from v1beta1 to v1alpha1 (copy base + convert spec fields to annotations)
			cs.Spec.UpdateStrategy.InPlaceUpdateStrategy, cs.Annotations = convertInPlaceUpdateStrategyFromV1beta1(
				csv1beta1.Spec.UpdateStrategy.RollingUpdate.InPlaceUpdateStrategy,
				cs.Annotations,
			)
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
			LastUpdateTime:     condition.LastUpdateTime,
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
			LastUpdateTime:     condition.LastUpdateTime,
			LastTransitionTime: condition.LastTransitionTime,
			Reason:             condition.Reason,
			Message:            condition.Message,
		}
	}
	return dst
}

// convertInPlaceUpdateStrategyToV1beta1 converts InPlaceUpdateStrategy from v1alpha1 to v1beta1,
// reading image pre-download configuration from annotations and converting them to spec fields
func convertInPlaceUpdateStrategyToV1beta1(src *appspub.InPlaceUpdateStrategy, annotations map[string]string) *appspub.InPlaceUpdateStrategy {
	// Check if we have any image pre-download annotations
	hasAnnotations := annotations != nil && (annotations[v1beta1.ImagePreDownloadParallelismKey] != "" ||
		annotations[v1beta1.ImagePreDownloadTimeoutSecondsKey] != "" ||
		annotations[v1beta1.ImagePreDownloadMinUpdatedReadyPods] != "")

	// If no src and no annotations, return nil
	if src == nil && !hasAnnotations {
		return nil
	}

	// Initialize dst based on src
	var dst *appspub.InPlaceUpdateStrategy
	if src != nil {
		dst = &appspub.InPlaceUpdateStrategy{
			GracePeriodSeconds: src.GracePeriodSeconds,
		}
	} else {
		dst = &appspub.InPlaceUpdateStrategy{}
	}

	// Convert annotations to spec fields
	// Note: In v1alpha1, these fields should only exist in annotations, not in the spec.
	// The src.ImagePreDownload* fields should always be nil for v1alpha1.
	if annotations != nil {
		// Convert ImagePreDownloadParallelism from annotation
		if parallelismStr, ok := annotations[v1beta1.ImagePreDownloadParallelismKey]; ok && parallelismStr != "" {
			parallelism := intstr.Parse(parallelismStr)
			dst.ImagePreDownloadParallelism = &parallelism
		}

		// Convert ImagePreDownloadTimeoutSeconds from annotation
		if timeoutStr, ok := annotations[v1beta1.ImagePreDownloadTimeoutSecondsKey]; ok && timeoutStr != "" {
			if timeout, err := strconv.ParseInt(timeoutStr, 10, 32); err == nil {
				timeout32 := int32(timeout)
				dst.ImagePreDownloadTimeoutSeconds = &timeout32
			}
		}

		// Convert ImagePreDownloadMinUpdatedReadyPods from annotation
		if minPodsStr, ok := annotations[v1beta1.ImagePreDownloadMinUpdatedReadyPods]; ok && minPodsStr != "" {
			if minPods, err := strconv.ParseInt(minPodsStr, 10, 32); err == nil {
				minPods32 := int32(minPods)
				dst.ImagePreDownloadMinUpdatedReadyPods = &minPods32
			}
		}
	}

	return dst
}

// convertInPlaceUpdateStrategyFromV1beta1 converts InPlaceUpdateStrategy from v1beta1 to v1alpha1,
// writing image pre-download configuration from spec fields back to annotations
func convertInPlaceUpdateStrategyFromV1beta1(src *appspub.InPlaceUpdateStrategy, annotations map[string]string) (*appspub.InPlaceUpdateStrategy, map[string]string) {
	if src == nil {
		return nil, annotations
	}

	// Check if src has any meaningful content beyond zero values
	hasContent := src.GracePeriodSeconds != 0 ||
		src.ImagePreDownloadParallelism != nil ||
		src.ImagePreDownloadTimeoutSeconds != nil ||
		src.ImagePreDownloadMinUpdatedReadyPods != nil

	// If src is effectively empty, return nil
	if !hasContent {
		return nil, annotations
	}

	// Create a copy with only base fields for v1alpha1
	dst := &appspub.InPlaceUpdateStrategy{
		GracePeriodSeconds: src.GracePeriodSeconds,
		// Note: v1alpha1 doesn't have the three image pre-download fields in spec
	}

	// Initialize annotations map if needed
	if annotations == nil {
		annotations = make(map[string]string)
	}

	// Convert spec fields to annotations
	if src.ImagePreDownloadParallelism != nil {
		annotations[v1beta1.ImagePreDownloadParallelismKey] = src.ImagePreDownloadParallelism.String()
	}

	if src.ImagePreDownloadTimeoutSeconds != nil {
		annotations[v1beta1.ImagePreDownloadTimeoutSecondsKey] = strconv.FormatInt(int64(*src.ImagePreDownloadTimeoutSeconds), 10)
	}

	if src.ImagePreDownloadMinUpdatedReadyPods != nil {
		annotations[v1beta1.ImagePreDownloadMinUpdatedReadyPods] = strconv.FormatInt(int64(*src.ImagePreDownloadMinUpdatedReadyPods), 10)
	}

	return dst, annotations
}
