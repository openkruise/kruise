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

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/conversion"

	"github.com/openkruise/kruise/apis/apps/v1beta1"
)

const (
	// LabelMetadataName is the standard Kubernetes label for namespace name (since Kubernetes 1.21+)
	LabelMetadataName = "kubernetes.io/metadata.name"
)

func (scs *SidecarSet) ConvertTo(dst conversion.Hub) error {
	switch t := dst.(type) {
	case *v1beta1.SidecarSet:
		scsv1beta1 := dst.(*v1beta1.SidecarSet)
		scsv1beta1.ObjectMeta = scs.ObjectMeta

		// spec
		scsv1beta1.Spec = v1beta1.SidecarSetSpec{
			Selector:             scs.Spec.Selector,
			InitContainers:       convertSidecarContainersToV1Beta1(scs.Spec.InitContainers),
			Containers:           convertSidecarContainersToV1Beta1(scs.Spec.Containers),
			Volumes:              scs.Spec.Volumes,
			UpdateStrategy:       convertUpdateStrategyToV1Beta1(scs.Spec.UpdateStrategy),
			InjectionStrategy:    convertInjectionStrategyToV1Beta1(scs.Spec.InjectionStrategy),
			ImagePullSecrets:     scs.Spec.ImagePullSecrets,
			RevisionHistoryLimit: scs.Spec.RevisionHistoryLimit,
			PatchPodMetadata:     convertPatchPodMetadataToV1Beta1(scs.Spec.PatchPodMetadata),
		}

		// Convert namespace and namespaceSelector to NamespaceSelector
		// In v1alpha1, we have both Namespace and NamespaceSelector fields
		// In v1beta1, we only have NamespaceSelector field
		// Priority: Namespace > NamespaceSelector (to maintain backward compatibility with v1alpha1 behavior)
		if scs.Spec.Namespace != "" {
			// Convert specific namespace to a NamespaceSelector using kubernetes.io/metadata.name label
			scsv1beta1.Spec.NamespaceSelector = &metav1.LabelSelector{
				MatchLabels: map[string]string{
					LabelMetadataName: scs.Spec.Namespace,
				},
			}
		} else if scs.Spec.NamespaceSelector != nil {
			scsv1beta1.Spec.NamespaceSelector = scs.Spec.NamespaceSelector
		}

		// Convert customVersion from label to spec field
		if label, ok := scs.Labels[SidecarSetCustomVersionLabel]; ok {
			scsv1beta1.Spec.CustomVersion = label
			// Remove the label from metadata if it exists
			if scsv1beta1.Labels != nil {
				delete(scsv1beta1.Labels, SidecarSetCustomVersionLabel)
			}
		}

		// status
		scsv1beta1.Status = v1beta1.SidecarSetStatus{
			ObservedGeneration: scs.Status.ObservedGeneration,
			MatchedPods:        scs.Status.MatchedPods,
			UpdatedPods:        scs.Status.UpdatedPods,
			ReadyPods:          scs.Status.ReadyPods,
			UpdatedReadyPods:   scs.Status.UpdatedReadyPods,
			LatestRevision:     scs.Status.LatestRevision,
			CollisionCount:     scs.Status.CollisionCount,
		}

		return nil

	default:
		return fmt.Errorf("unsupported type %v", t)
	}
}

func (scs *SidecarSet) ConvertFrom(src conversion.Hub) error {
	switch t := src.(type) {
	case *v1beta1.SidecarSet:
		scsv1beta1 := src.(*v1beta1.SidecarSet)
		scs.ObjectMeta = scsv1beta1.ObjectMeta

		// spec
		scs.Spec = SidecarSetSpec{
			Selector:             scsv1beta1.Spec.Selector,
			InitContainers:       convertSidecarContainersToV1Alpha1(scsv1beta1.Spec.InitContainers),
			Containers:           convertSidecarContainersToV1Alpha1(scsv1beta1.Spec.Containers),
			Volumes:              scsv1beta1.Spec.Volumes,
			UpdateStrategy:       convertUpdateStrategyToV1Alpha1(scsv1beta1.Spec.UpdateStrategy),
			InjectionStrategy:    convertInjectionStrategyToV1Alpha1(scsv1beta1.Spec.InjectionStrategy),
			ImagePullSecrets:     scsv1beta1.Spec.ImagePullSecrets,
			RevisionHistoryLimit: scsv1beta1.Spec.RevisionHistoryLimit,
			PatchPodMetadata:     convertPatchPodMetadataToV1Alpha1(scsv1beta1.Spec.PatchPodMetadata),
		}

		// Convert NamespaceSelector back to namespaceSelector
		// Simply copy the NamespaceSelector from v1beta1, no special handling needed
		scs.Spec.NamespaceSelector = scsv1beta1.Spec.NamespaceSelector

		// Convert customVersion from spec field to label
		if scsv1beta1.Spec.CustomVersion != "" {
			if scs.Labels == nil {
				scs.Labels = make(map[string]string)
			}
			scs.Labels[SidecarSetCustomVersionLabel] = scsv1beta1.Spec.CustomVersion
		}

		// status
		scs.Status = SidecarSetStatus{
			ObservedGeneration: scsv1beta1.Status.ObservedGeneration,
			MatchedPods:        scsv1beta1.Status.MatchedPods,
			UpdatedPods:        scsv1beta1.Status.UpdatedPods,
			ReadyPods:          scsv1beta1.Status.ReadyPods,
			UpdatedReadyPods:   scsv1beta1.Status.UpdatedReadyPods,
			LatestRevision:     scsv1beta1.Status.LatestRevision,
			CollisionCount:     scsv1beta1.Status.CollisionCount,
		}

		return nil
	default:
		return fmt.Errorf("unsupported type %v", t)
	}
}

func convertSidecarContainersToV1Beta1(containers []SidecarContainer) []v1beta1.SidecarContainer {
	if containers == nil {
		return nil
	}
	result := make([]v1beta1.SidecarContainer, len(containers))
	for i, container := range containers {
		result[i] = v1beta1.SidecarContainer{
			Container:               container.Container,
			PodInjectPolicy:         v1beta1.PodInjectPolicyType(container.PodInjectPolicy),
			UpgradeStrategy:         convertUpgradeStrategyToV1Beta1(container.UpgradeStrategy),
			ShareVolumePolicy:       convertShareVolumePolicyToV1Beta1(container.ShareVolumePolicy),
			ShareVolumeDevicePolicy: convertShareVolumePolicyPtrToV1Beta1(container.ShareVolumeDevicePolicy),
			TransferEnv:             convertTransferEnvVarsToV1Beta1(container.TransferEnv),
			ResourcesPolicy:         convertResourcesPolicyToV1Beta1(container.ResourcesPolicy),
		}
	}
	return result
}

func convertSidecarContainersToV1Alpha1(containers []v1beta1.SidecarContainer) []SidecarContainer {
	if containers == nil {
		return nil
	}
	result := make([]SidecarContainer, len(containers))
	for i, container := range containers {
		result[i] = SidecarContainer{
			Container:               container.Container,
			PodInjectPolicy:         PodInjectPolicyType(container.PodInjectPolicy),
			UpgradeStrategy:         convertUpgradeStrategyToV1Alpha1(container.UpgradeStrategy),
			ShareVolumePolicy:       convertShareVolumePolicyToV1Alpha1(container.ShareVolumePolicy),
			ShareVolumeDevicePolicy: convertShareVolumePolicyPtrToV1Alpha1(container.ShareVolumeDevicePolicy),
			TransferEnv:             convertTransferEnvVarsToV1Alpha1(container.TransferEnv),
			ResourcesPolicy:         convertResourcesPolicyToV1Alpha1(container.ResourcesPolicy),
		}
	}
	return result
}

func convertUpgradeStrategyToV1Beta1(strategy SidecarContainerUpgradeStrategy) v1beta1.SidecarContainerUpgradeStrategy {
	return v1beta1.SidecarContainerUpgradeStrategy{
		UpgradeType:          v1beta1.SidecarContainerUpgradeType(strategy.UpgradeType),
		HotUpgradeEmptyImage: strategy.HotUpgradeEmptyImage,
	}
}

func convertUpgradeStrategyToV1Alpha1(strategy v1beta1.SidecarContainerUpgradeStrategy) SidecarContainerUpgradeStrategy {
	return SidecarContainerUpgradeStrategy{
		UpgradeType:          SidecarContainerUpgradeType(strategy.UpgradeType),
		HotUpgradeEmptyImage: strategy.HotUpgradeEmptyImage,
	}
}

func convertShareVolumePolicyToV1Beta1(policy ShareVolumePolicy) v1beta1.ShareVolumePolicy {
	return v1beta1.ShareVolumePolicy{
		Type: v1beta1.ShareVolumePolicyType(policy.Type),
	}
}

func convertShareVolumePolicyToV1Alpha1(policy v1beta1.ShareVolumePolicy) ShareVolumePolicy {
	return ShareVolumePolicy{
		Type: ShareVolumePolicyType(policy.Type),
	}
}

func convertShareVolumePolicyPtrToV1Beta1(policy *ShareVolumePolicy) *v1beta1.ShareVolumePolicy {
	if policy == nil {
		return nil
	}
	result := convertShareVolumePolicyToV1Beta1(*policy)
	return &result
}

func convertShareVolumePolicyPtrToV1Alpha1(policy *v1beta1.ShareVolumePolicy) *ShareVolumePolicy {
	if policy == nil {
		return nil
	}
	result := convertShareVolumePolicyToV1Alpha1(*policy)
	return &result
}

func convertTransferEnvVarsToV1Beta1(vars []TransferEnvVar) []v1beta1.TransferEnvVar {
	if vars == nil {
		return nil
	}
	result := make([]v1beta1.TransferEnvVar, len(vars))
	for i, v := range vars {
		result[i] = v1beta1.TransferEnvVar{
			SourceContainerName:     v.SourceContainerName,
			SourceContainerNameFrom: convertSourceContainerNameSourceToV1Beta1(v.SourceContainerNameFrom),
			EnvName:                 v.EnvName,
			EnvNames:                v.EnvNames,
		}
	}
	return result
}

func convertTransferEnvVarsToV1Alpha1(vars []v1beta1.TransferEnvVar) []TransferEnvVar {
	if vars == nil {
		return nil
	}
	result := make([]TransferEnvVar, len(vars))
	for i, v := range vars {
		result[i] = TransferEnvVar{
			SourceContainerName:     v.SourceContainerName,
			SourceContainerNameFrom: convertSourceContainerNameSourceToV1Alpha1(v.SourceContainerNameFrom),
			EnvName:                 v.EnvName,
			EnvNames:                v.EnvNames,
		}
	}
	return result
}

func convertSourceContainerNameSourceToV1Beta1(source *SourceContainerNameSource) *v1beta1.SourceContainerNameSource {
	if source == nil {
		return nil
	}
	return &v1beta1.SourceContainerNameSource{
		FieldRef: source.FieldRef,
	}
}

func convertSourceContainerNameSourceToV1Alpha1(source *v1beta1.SourceContainerNameSource) *SourceContainerNameSource {
	if source == nil {
		return nil
	}
	return &SourceContainerNameSource{
		FieldRef: source.FieldRef,
	}
}

func convertInjectionStrategyToV1Beta1(strategy SidecarSetInjectionStrategy) v1beta1.SidecarSetInjectionStrategy {
	return v1beta1.SidecarSetInjectionStrategy{
		Paused:   strategy.Paused,
		Revision: convertInjectRevisionToV1Beta1(strategy.Revision),
	}
}

func convertInjectionStrategyToV1Alpha1(strategy v1beta1.SidecarSetInjectionStrategy) SidecarSetInjectionStrategy {
	return SidecarSetInjectionStrategy{
		Paused:   strategy.Paused,
		Revision: convertInjectRevisionToV1Alpha1(strategy.Revision),
	}
}

func convertInjectRevisionToV1Beta1(revision *SidecarSetInjectRevision) *v1beta1.SidecarSetInjectRevision {
	if revision == nil {
		return nil
	}
	return &v1beta1.SidecarSetInjectRevision{
		CustomVersion: revision.CustomVersion,
		RevisionName:  revision.RevisionName,
		Policy:        v1beta1.SidecarSetInjectRevisionPolicy(revision.Policy),
	}
}

func convertInjectRevisionToV1Alpha1(revision *v1beta1.SidecarSetInjectRevision) *SidecarSetInjectRevision {
	if revision == nil {
		return nil
	}
	return &SidecarSetInjectRevision{
		CustomVersion: revision.CustomVersion,
		RevisionName:  revision.RevisionName,
		Policy:        SidecarSetInjectRevisionPolicy(revision.Policy),
	}
}

func convertUpdateStrategyToV1Beta1(strategy SidecarSetUpdateStrategy) v1beta1.SidecarSetUpdateStrategy {
	return v1beta1.SidecarSetUpdateStrategy{
		Type:             v1beta1.SidecarSetUpdateStrategyType(strategy.Type),
		Paused:           strategy.Paused,
		Selector:         strategy.Selector,
		Partition:        strategy.Partition,
		MaxUnavailable:   strategy.MaxUnavailable,
		PriorityStrategy: strategy.PriorityStrategy,
		ScatterStrategy:  convertScatterStrategyToV1Beta1(strategy.ScatterStrategy),
	}
}

func convertUpdateStrategyToV1Alpha1(strategy v1beta1.SidecarSetUpdateStrategy) SidecarSetUpdateStrategy {
	return SidecarSetUpdateStrategy{
		Type:             SidecarSetUpdateStrategyType(strategy.Type),
		Paused:           strategy.Paused,
		Selector:         strategy.Selector,
		Partition:        strategy.Partition,
		MaxUnavailable:   strategy.MaxUnavailable,
		PriorityStrategy: strategy.PriorityStrategy,
		ScatterStrategy:  convertScatterStrategyToV1Alpha1(strategy.ScatterStrategy),
	}
}

func convertScatterStrategyToV1Beta1(strategy UpdateScatterStrategy) v1beta1.UpdateScatterStrategy {
	if strategy == nil {
		return nil
	}
	result := make(v1beta1.UpdateScatterStrategy, len(strategy))
	for i, term := range strategy {
		result[i] = v1beta1.UpdateScatterTerm{
			Key:   term.Key,
			Value: term.Value,
		}
	}
	return result
}

func convertScatterStrategyToV1Alpha1(strategy v1beta1.UpdateScatterStrategy) UpdateScatterStrategy {
	if strategy == nil {
		return nil
	}
	result := make(UpdateScatterStrategy, len(strategy))
	for i, term := range strategy {
		result[i] = UpdateScatterTerm{
			Key:   term.Key,
			Value: term.Value,
		}
	}
	return result
}

func convertPatchPodMetadataToV1Beta1(metadata []SidecarSetPatchPodMetadata) []v1beta1.SidecarSetPatchPodMetadata {
	if metadata == nil {
		return nil
	}
	result := make([]v1beta1.SidecarSetPatchPodMetadata, len(metadata))
	for i, m := range metadata {
		result[i] = v1beta1.SidecarSetPatchPodMetadata{
			Annotations: m.Annotations,
			PatchPolicy: v1beta1.SidecarSetPatchPolicyType(m.PatchPolicy),
		}
	}
	return result
}

func convertPatchPodMetadataToV1Alpha1(metadata []v1beta1.SidecarSetPatchPodMetadata) []SidecarSetPatchPodMetadata {
	if metadata == nil {
		return nil
	}
	result := make([]SidecarSetPatchPodMetadata, len(metadata))
	for i, m := range metadata {
		result[i] = SidecarSetPatchPodMetadata{
			Annotations: m.Annotations,
			PatchPolicy: SidecarSetPatchPolicyType(m.PatchPolicy),
		}
	}
	return result
}

func convertResourcesPolicyToV1Beta1(policy *ResourcesPolicy) *v1beta1.ResourcesPolicy {
	if policy == nil {
		return nil
	}
	return &v1beta1.ResourcesPolicy{
		TargetContainerMode:       v1beta1.TargetContainerModeType(policy.TargetContainerMode),
		TargetContainersNameRegex: policy.TargetContainersNameRegex,
		ResourceExpr:              convertResourceExprToV1Beta1(policy.ResourceExpr),
	}
}

func convertResourcesPolicyToV1Alpha1(policy *v1beta1.ResourcesPolicy) *ResourcesPolicy {
	if policy == nil {
		return nil
	}
	return &ResourcesPolicy{
		TargetContainerMode:       TargetContainerModeType(policy.TargetContainerMode),
		TargetContainersNameRegex: policy.TargetContainersNameRegex,
		ResourceExpr:              convertResourceExprToV1Alpha1(policy.ResourceExpr),
	}
}

func convertResourceExprToV1Beta1(expr ResourceExpr) v1beta1.ResourceExpr {
	result := v1beta1.ResourceExpr{}
	if expr.Limits != nil {
		result.Limits = &v1beta1.ResourceExprLimits{
			CPU:    expr.Limits.CPU,
			Memory: expr.Limits.Memory,
		}
	}
	if expr.Requests != nil {
		result.Requests = &v1beta1.ResourceExprRequests{
			CPU:    expr.Requests.CPU,
			Memory: expr.Requests.Memory,
		}
	}
	return result
}

func convertResourceExprToV1Alpha1(expr v1beta1.ResourceExpr) ResourceExpr {
	result := ResourceExpr{}
	if expr.Limits != nil {
		result.Limits = &ResourceExprLimits{
			CPU:    expr.Limits.CPU,
			Memory: expr.Limits.Memory,
		}
	}
	if expr.Requests != nil {
		result.Requests = &ResourceExprRequests{
			CPU:    expr.Requests.CPU,
			Memory: expr.Requests.Memory,
		}
	}
	return result
}
