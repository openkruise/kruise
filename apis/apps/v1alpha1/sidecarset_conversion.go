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

func (src *SidecarSet) ConvertTo(dstRaw conversion.Hub) error {
	switch t := dstRaw.(type) {
	case *v1beta1.SidecarSet:
		dst := dstRaw.(*v1beta1.SidecarSet)
		dst.ObjectMeta = src.ObjectMeta
		dst.Spec.Selector = src.Spec.Selector
		dst.Spec.Namespace = src.Spec.Namespace
		dst.Spec.NamespaceSelector = src.Spec.NamespaceSelector
		initContainers := make([]v1beta1.SidecarContainer, len(src.Spec.InitContainers))
		for initContainers_idx, initContainers_val := range src.Spec.InitContainers {
			initContainers[initContainers_idx].Container = initContainers_val.Container
			initContainers[initContainers_idx].PodInjectPolicy = v1beta1.PodInjectPolicyType(initContainers_val.PodInjectPolicy)
			initContainers[initContainers_idx].UpgradeStrategy.UpgradeType = v1beta1.SidecarContainerUpgradeType(initContainers_val.UpgradeStrategy.UpgradeType)
			initContainers[initContainers_idx].UpgradeStrategy.HotUpgradeEmptyImage = initContainers_val.UpgradeStrategy.HotUpgradeEmptyImage
			initContainers[initContainers_idx].ShareVolumePolicy.Type = v1beta1.ShareVolumePolicyType(initContainers_val.ShareVolumePolicy.Type)
			transferEnv := make([]v1beta1.TransferEnvVar, len(initContainers_val.TransferEnv))
			for transferEnv_idx, transferEnv_val := range initContainers_val.TransferEnv {
				transferEnv[transferEnv_idx].SourceContainerName = transferEnv_val.SourceContainerName
				if transferEnv_val.SourceContainerNameFrom != nil {
					transferEnv[transferEnv_idx].SourceContainerNameFrom = &v1beta1.SourceContainerNameSource{}
					transferEnv[transferEnv_idx].SourceContainerNameFrom.FieldRef = transferEnv_val.SourceContainerNameFrom.FieldRef
				}
				transferEnv[transferEnv_idx].EnvName = transferEnv_val.EnvName
				transferEnv[transferEnv_idx].EnvNames = transferEnv_val.EnvNames
			}
			initContainers[initContainers_idx].TransferEnv = transferEnv
		}
		dst.Spec.InitContainers = initContainers
		containers := make([]v1beta1.SidecarContainer, len(src.Spec.Containers))
		for containers_idx, containers_val := range src.Spec.Containers {
			containers[containers_idx].Container = containers_val.Container
			containers[containers_idx].PodInjectPolicy = v1beta1.PodInjectPolicyType(containers_val.PodInjectPolicy)
			containers[containers_idx].UpgradeStrategy.UpgradeType = v1beta1.SidecarContainerUpgradeType(containers_val.UpgradeStrategy.UpgradeType)
			containers[containers_idx].UpgradeStrategy.HotUpgradeEmptyImage = containers_val.UpgradeStrategy.HotUpgradeEmptyImage
			containers[containers_idx].ShareVolumePolicy.Type = v1beta1.ShareVolumePolicyType(containers_val.ShareVolumePolicy.Type)
			transferEnv := make([]v1beta1.TransferEnvVar, len(containers_val.TransferEnv))
			for transferEnv_idx, transferEnv_val := range containers_val.TransferEnv {
				transferEnv[transferEnv_idx].SourceContainerName = transferEnv_val.SourceContainerName
				if transferEnv_val.SourceContainerNameFrom != nil {
					transferEnv[transferEnv_idx].SourceContainerNameFrom = &v1beta1.SourceContainerNameSource{}
					transferEnv[transferEnv_idx].SourceContainerNameFrom.FieldRef = transferEnv_val.SourceContainerNameFrom.FieldRef
				}
				transferEnv[transferEnv_idx].EnvName = transferEnv_val.EnvName
				transferEnv[transferEnv_idx].EnvNames = transferEnv_val.EnvNames
			}
			containers[containers_idx].TransferEnv = transferEnv
		}
		dst.Spec.Containers = containers
		dst.Spec.Volumes = src.Spec.Volumes
		dst.Spec.UpdateStrategy.Type = v1beta1.SidecarSetUpdateStrategyType(src.Spec.UpdateStrategy.Type)
		dst.Spec.UpdateStrategy.Paused = src.Spec.UpdateStrategy.Paused
		dst.Spec.UpdateStrategy.Selector = src.Spec.UpdateStrategy.Selector
		dst.Spec.UpdateStrategy.Partition = src.Spec.UpdateStrategy.Partition
		dst.Spec.UpdateStrategy.MaxUnavailable = src.Spec.UpdateStrategy.MaxUnavailable
		dst.Spec.UpdateStrategy.PriorityStrategy = src.Spec.UpdateStrategy.PriorityStrategy
		scatterStrategy := make([]v1beta1.UpdateScatterTerm, len(src.Spec.UpdateStrategy.ScatterStrategy))
		for scatterStrategy_idx, scatterStrategy_val := range src.Spec.UpdateStrategy.ScatterStrategy {
			scatterStrategy[scatterStrategy_idx].Key = scatterStrategy_val.Key
			scatterStrategy[scatterStrategy_idx].Value = scatterStrategy_val.Value
		}
		dst.Spec.UpdateStrategy.ScatterStrategy = scatterStrategy
		dst.Spec.InjectionStrategy.Paused = src.Spec.InjectionStrategy.Paused
		if src.Spec.InjectionStrategy.Revision != nil {
			dst.Spec.InjectionStrategy.Revision = &v1beta1.SidecarSetInjectRevision{}
			dst.Spec.InjectionStrategy.Revision.CustomVersion = src.Spec.InjectionStrategy.Revision.CustomVersion
			dst.Spec.InjectionStrategy.Revision.RevisionName = src.Spec.InjectionStrategy.Revision.RevisionName
			dst.Spec.InjectionStrategy.Revision.Policy = v1beta1.SidecarSetInjectRevisionPolicy(src.Spec.InjectionStrategy.Revision.Policy)
		}
		dst.Spec.ImagePullSecrets = src.Spec.ImagePullSecrets
		dst.Spec.RevisionHistoryLimit = src.Spec.RevisionHistoryLimit
		patchPodMetadata := make([]v1beta1.SidecarSetPatchPodMetadata, len(src.Spec.PatchPodMetadata))
		for patchPodMetadata_idx, patchPodMetadata_val := range src.Spec.PatchPodMetadata {
			patchPodMetadata[patchPodMetadata_idx].Annotations = patchPodMetadata_val.Annotations
			patchPodMetadata[patchPodMetadata_idx].PatchPolicy = v1beta1.SidecarSetPatchPolicyType(patchPodMetadata_val.PatchPolicy)
		}
		dst.Spec.PatchPodMetadata = patchPodMetadata
		dst.Status.ObservedGeneration = src.Status.ObservedGeneration
		dst.Status.MatchedPods = src.Status.MatchedPods
		dst.Status.UpdatedPods = src.Status.UpdatedPods
		dst.Status.ReadyPods = src.Status.ReadyPods
		dst.Status.UpdatedReadyPods = src.Status.UpdatedReadyPods
		dst.Status.LatestRevision = src.Status.LatestRevision
		dst.Status.CollisionCount = src.Status.CollisionCount
	default:
		return fmt.Errorf("unsupported type %v", t)
	}
	return nil
}

func (dst *SidecarSet) ConvertFrom(srcRaw conversion.Hub) error {
	switch t := srcRaw.(type) {
	case *v1beta1.SidecarSet:
		src := srcRaw.(*v1beta1.SidecarSet)
		dst.ObjectMeta = src.ObjectMeta
		dst.Spec.Selector = src.Spec.Selector
		dst.Spec.Namespace = src.Spec.Namespace
		dst.Spec.NamespaceSelector = src.Spec.NamespaceSelector
		initContainers := make([]SidecarContainer, len(src.Spec.InitContainers))
		for initContainers_idx, initContainers_val := range src.Spec.InitContainers {
			initContainers[initContainers_idx].Container = initContainers_val.Container
			initContainers[initContainers_idx].PodInjectPolicy = PodInjectPolicyType(initContainers_val.PodInjectPolicy)
			initContainers[initContainers_idx].UpgradeStrategy.UpgradeType = SidecarContainerUpgradeType(initContainers_val.UpgradeStrategy.UpgradeType)
			initContainers[initContainers_idx].UpgradeStrategy.HotUpgradeEmptyImage = initContainers_val.UpgradeStrategy.HotUpgradeEmptyImage
			initContainers[initContainers_idx].ShareVolumePolicy.Type = ShareVolumePolicyType(initContainers_val.ShareVolumePolicy.Type)
			transferEnv := make([]TransferEnvVar, len(initContainers_val.TransferEnv))
			for transferEnv_idx, transferEnv_val := range initContainers_val.TransferEnv {
				transferEnv[transferEnv_idx].SourceContainerName = transferEnv_val.SourceContainerName
				if transferEnv_val.SourceContainerNameFrom != nil {
					transferEnv[transferEnv_idx].SourceContainerNameFrom = &SourceContainerNameSource{}
					transferEnv[transferEnv_idx].SourceContainerNameFrom.FieldRef = transferEnv_val.SourceContainerNameFrom.FieldRef
				}
				transferEnv[transferEnv_idx].EnvName = transferEnv_val.EnvName
				transferEnv[transferEnv_idx].EnvNames = transferEnv_val.EnvNames
			}
			initContainers[initContainers_idx].TransferEnv = transferEnv
		}
		dst.Spec.InitContainers = initContainers
		containers := make([]SidecarContainer, len(src.Spec.Containers))
		for containers_idx, containers_val := range src.Spec.Containers {
			containers[containers_idx].Container = containers_val.Container
			containers[containers_idx].PodInjectPolicy = PodInjectPolicyType(containers_val.PodInjectPolicy)
			containers[containers_idx].UpgradeStrategy.UpgradeType = SidecarContainerUpgradeType(containers_val.UpgradeStrategy.UpgradeType)
			containers[containers_idx].UpgradeStrategy.HotUpgradeEmptyImage = containers_val.UpgradeStrategy.HotUpgradeEmptyImage
			containers[containers_idx].ShareVolumePolicy.Type = ShareVolumePolicyType(containers_val.ShareVolumePolicy.Type)
			transferEnv := make([]TransferEnvVar, len(containers_val.TransferEnv))
			for transferEnv_idx, transferEnv_val := range containers_val.TransferEnv {
				transferEnv[transferEnv_idx].SourceContainerName = transferEnv_val.SourceContainerName
				if transferEnv_val.SourceContainerNameFrom != nil {
					transferEnv[transferEnv_idx].SourceContainerNameFrom = &SourceContainerNameSource{}
					transferEnv[transferEnv_idx].SourceContainerNameFrom.FieldRef = transferEnv_val.SourceContainerNameFrom.FieldRef
				}
				transferEnv[transferEnv_idx].EnvName = transferEnv_val.EnvName
				transferEnv[transferEnv_idx].EnvNames = transferEnv_val.EnvNames
			}
			containers[containers_idx].TransferEnv = transferEnv
		}
		dst.Spec.Containers = containers
		dst.Spec.Volumes = src.Spec.Volumes
		dst.Spec.UpdateStrategy.Type = SidecarSetUpdateStrategyType(src.Spec.UpdateStrategy.Type)
		dst.Spec.UpdateStrategy.Paused = src.Spec.UpdateStrategy.Paused
		dst.Spec.UpdateStrategy.Selector = src.Spec.UpdateStrategy.Selector
		dst.Spec.UpdateStrategy.Partition = src.Spec.UpdateStrategy.Partition
		dst.Spec.UpdateStrategy.MaxUnavailable = src.Spec.UpdateStrategy.MaxUnavailable
		dst.Spec.UpdateStrategy.PriorityStrategy = src.Spec.UpdateStrategy.PriorityStrategy
		scatterStrategy := make([]UpdateScatterTerm, len(src.Spec.UpdateStrategy.ScatterStrategy))
		for scatterStrategy_idx, scatterStrategy_val := range src.Spec.UpdateStrategy.ScatterStrategy {
			scatterStrategy[scatterStrategy_idx].Key = scatterStrategy_val.Key
			scatterStrategy[scatterStrategy_idx].Value = scatterStrategy_val.Value
		}
		dst.Spec.UpdateStrategy.ScatterStrategy = scatterStrategy
		dst.Spec.InjectionStrategy.Paused = src.Spec.InjectionStrategy.Paused
		if src.Spec.InjectionStrategy.Revision != nil {
			dst.Spec.InjectionStrategy.Revision = &SidecarSetInjectRevision{}
			dst.Spec.InjectionStrategy.Revision.CustomVersion = src.Spec.InjectionStrategy.Revision.CustomVersion
			dst.Spec.InjectionStrategy.Revision.RevisionName = src.Spec.InjectionStrategy.Revision.RevisionName
			dst.Spec.InjectionStrategy.Revision.Policy = SidecarSetInjectRevisionPolicy(src.Spec.InjectionStrategy.Revision.Policy)
		}
		dst.Spec.ImagePullSecrets = src.Spec.ImagePullSecrets
		dst.Spec.RevisionHistoryLimit = src.Spec.RevisionHistoryLimit
		patchPodMetadata := make([]SidecarSetPatchPodMetadata, len(src.Spec.PatchPodMetadata))
		for patchPodMetadata_idx, patchPodMetadata_val := range src.Spec.PatchPodMetadata {
			patchPodMetadata[patchPodMetadata_idx].Annotations = patchPodMetadata_val.Annotations
			patchPodMetadata[patchPodMetadata_idx].PatchPolicy = SidecarSetPatchPolicyType(patchPodMetadata_val.PatchPolicy)
		}
		dst.Spec.PatchPodMetadata = patchPodMetadata
		dst.Status.ObservedGeneration = src.Status.ObservedGeneration
		dst.Status.MatchedPods = src.Status.MatchedPods
		dst.Status.UpdatedPods = src.Status.UpdatedPods
		dst.Status.ReadyPods = src.Status.ReadyPods
		dst.Status.UpdatedReadyPods = src.Status.UpdatedReadyPods
		dst.Status.LatestRevision = src.Status.LatestRevision
		dst.Status.CollisionCount = src.Status.CollisionCount
	default:
		return fmt.Errorf("unsupported type %v", t)
	}
	return nil
}
