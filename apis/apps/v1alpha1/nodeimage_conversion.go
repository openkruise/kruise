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

func (src *NodeImage) ConvertTo(dstRaw conversion.Hub) error {
	switch t := dstRaw.(type) {
	case *v1beta1.NodeImage:
		dst := dstRaw.(*v1beta1.NodeImage)
		dst.ObjectMeta = src.ObjectMeta
		images := make(map[string]v1beta1.ImageSpec, len(src.Spec.Images))
		for images_key, images_val := range src.Spec.Images {
			imageSpec := v1beta1.ImageSpec{}
			pullSecrets := make([]v1beta1.ReferenceObject, len(images_val.PullSecrets))
			for pullSecrets_idx, pullSecrets_val := range images_val.PullSecrets {
				pullSecrets[pullSecrets_idx].Namespace = pullSecrets_val.Namespace
				pullSecrets[pullSecrets_idx].Name = pullSecrets_val.Name
			}
			imageSpec.PullSecrets = pullSecrets
			tags := make([]v1beta1.ImageTagSpec, len(images_val.Tags))
			for tags_idx, tags_val := range images_val.Tags {
				tags[tags_idx].Tag = tags_val.Tag
				tags[tags_idx].CreatedAt = tags_val.CreatedAt
				if tags_val.PullPolicy != nil {
					tags[tags_idx].PullPolicy = &v1beta1.ImageTagPullPolicy{}
					tags[tags_idx].PullPolicy.TimeoutSeconds = tags_val.PullPolicy.TimeoutSeconds
					tags[tags_idx].PullPolicy.BackoffLimit = tags_val.PullPolicy.BackoffLimit
					tags[tags_idx].PullPolicy.TTLSecondsAfterFinished = tags_val.PullPolicy.TTLSecondsAfterFinished
					tags[tags_idx].PullPolicy.ActiveDeadlineSeconds = tags_val.PullPolicy.ActiveDeadlineSeconds
				}
				tags[tags_idx].OwnerReferences = tags_val.OwnerReferences
				tags[tags_idx].Version = tags_val.Version
			}
			imageSpec.Tags = tags
			if images_val.SandboxConfig != nil {
				imageSpec.SandboxConfig = &v1beta1.SandboxConfig{}
				imageSpec.SandboxConfig.Labels = images_val.SandboxConfig.Labels
				imageSpec.SandboxConfig.Annotations = images_val.SandboxConfig.Annotations
			}
			images[images_key] = imageSpec
		}
		dst.Spec.Images = images
		dst.Status.Desired = src.Status.Desired
		dst.Status.Succeeded = src.Status.Succeeded
		dst.Status.Failed = src.Status.Failed
		dst.Status.Pulling = src.Status.Pulling
		imageStatuses := make(map[string]v1beta1.ImageStatus, len(src.Status.ImageStatuses))
		for imageStatuses_key, imageStatuses_val := range src.Status.ImageStatuses {
			imageStatus := v1beta1.ImageStatus{}
			tags := make([]v1beta1.ImageTagStatus, len(imageStatuses_val.Tags))
			for tags_idx, tags_val := range imageStatuses_val.Tags {
				tags[tags_idx].Tag = tags_val.Tag
				tags[tags_idx].Phase = v1beta1.ImagePullPhase(tags_val.Phase)
				tags[tags_idx].Progress = tags_val.Progress
				tags[tags_idx].StartTime = tags_val.StartTime
				tags[tags_idx].CompletionTime = tags_val.CompletionTime
				tags[tags_idx].Version = tags_val.Version
				tags[tags_idx].ImageID = tags_val.ImageID
				tags[tags_idx].Message = tags_val.Message
			}
			imageStatus.Tags = tags
			imageStatuses[imageStatuses_key] = imageStatus
		}
		dst.Status.ImageStatuses = imageStatuses
		if src.Status.FirstSyncStatus != nil {
			dst.Status.FirstSyncStatus = &v1beta1.SyncStatus{}
			dst.Status.FirstSyncStatus.SyncAt = src.Status.FirstSyncStatus.SyncAt
			dst.Status.FirstSyncStatus.Status = v1beta1.SyncStatusPhase(src.Status.FirstSyncStatus.Status)
			dst.Status.FirstSyncStatus.Message = src.Status.FirstSyncStatus.Message
		}
	default:
		return fmt.Errorf("unsupported type %v", t)
	}
	return nil
}

func (dst *NodeImage) ConvertFrom(srcRaw conversion.Hub) error {
	switch t := srcRaw.(type) {
	case *v1beta1.NodeImage:
		src := srcRaw.(*v1beta1.NodeImage)
		dst.ObjectMeta = src.ObjectMeta
		images := make(map[string]ImageSpec, len(src.Spec.Images))
		for images_key, images_val := range src.Spec.Images {
			imageSpec := ImageSpec{}
			pullSecrets := make([]ReferenceObject, len(images_val.PullSecrets))
			for pullSecrets_idx, pullSecrets_val := range images_val.PullSecrets {
				pullSecrets[pullSecrets_idx].Namespace = pullSecrets_val.Namespace
				pullSecrets[pullSecrets_idx].Name = pullSecrets_val.Name
			}
			imageSpec.PullSecrets = pullSecrets
			tags := make([]ImageTagSpec, len(images_val.Tags))
			for tags_idx, tags_val := range images_val.Tags {
				tags[tags_idx].Tag = tags_val.Tag
				tags[tags_idx].CreatedAt = tags_val.CreatedAt
				if tags_val.PullPolicy != nil {
					tags[tags_idx].PullPolicy = &ImageTagPullPolicy{}
					tags[tags_idx].PullPolicy.TimeoutSeconds = tags_val.PullPolicy.TimeoutSeconds
					tags[tags_idx].PullPolicy.BackoffLimit = tags_val.PullPolicy.BackoffLimit
					tags[tags_idx].PullPolicy.TTLSecondsAfterFinished = tags_val.PullPolicy.TTLSecondsAfterFinished
					tags[tags_idx].PullPolicy.ActiveDeadlineSeconds = tags_val.PullPolicy.ActiveDeadlineSeconds
				}
				tags[tags_idx].OwnerReferences = tags_val.OwnerReferences
				tags[tags_idx].Version = tags_val.Version
			}
			imageSpec.Tags = tags
			if images_val.SandboxConfig != nil {
				imageSpec.SandboxConfig = &SandboxConfig{}
				imageSpec.SandboxConfig.Labels = images_val.SandboxConfig.Labels
				imageSpec.SandboxConfig.Annotations = images_val.SandboxConfig.Annotations
			}
			images[images_key] = imageSpec
		}
		dst.Spec.Images = images
		dst.Status.Desired = src.Status.Desired
		dst.Status.Succeeded = src.Status.Succeeded
		dst.Status.Failed = src.Status.Failed
		dst.Status.Pulling = src.Status.Pulling
		imageStatuses := make(map[string]ImageStatus, len(src.Status.ImageStatuses))
		for imageStatuses_key, imageStatuses_val := range src.Status.ImageStatuses {
			imageStatus := ImageStatus{}
			tags := make([]ImageTagStatus, len(imageStatuses_val.Tags))
			for tags_idx, tags_val := range imageStatuses_val.Tags {
				tags[tags_idx].Tag = tags_val.Tag
				tags[tags_idx].Phase = ImagePullPhase(tags_val.Phase)
				tags[tags_idx].Progress = tags_val.Progress
				tags[tags_idx].StartTime = tags_val.StartTime
				tags[tags_idx].CompletionTime = tags_val.CompletionTime
				tags[tags_idx].Version = tags_val.Version
				tags[tags_idx].ImageID = tags_val.ImageID
				tags[tags_idx].Message = tags_val.Message
			}
			imageStatus.Tags = tags
			imageStatuses[imageStatuses_key] = imageStatus
		}
		dst.Status.ImageStatuses = imageStatuses
		if src.Status.FirstSyncStatus != nil {
			dst.Status.FirstSyncStatus = &SyncStatus{}
			dst.Status.FirstSyncStatus.SyncAt = src.Status.FirstSyncStatus.SyncAt
			dst.Status.FirstSyncStatus.Status = SyncStatusPhase(src.Status.FirstSyncStatus.Status)
			dst.Status.FirstSyncStatus.Message = src.Status.FirstSyncStatus.Message
		}
	default:
		return fmt.Errorf("unsupported type %v", t)
	}
	return nil
}
