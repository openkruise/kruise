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

func (src *ImagePullJob) ConvertTo(dstRaw conversion.Hub) error {
	switch t := dstRaw.(type) {
	case *v1beta1.ImagePullJob:
		dst := dstRaw.(*v1beta1.ImagePullJob)
		dst.ObjectMeta = src.ObjectMeta
		dst.Spec.Image = src.Spec.Image
		dst.Spec.ImagePullJobTemplate.PullSecrets = src.Spec.ImagePullJobTemplate.PullSecrets
		if src.Spec.ImagePullJobTemplate.Selector != nil {
			dst.Spec.ImagePullJobTemplate.Selector = &v1beta1.ImagePullJobNodeSelector{}
			dst.Spec.ImagePullJobTemplate.Selector.Names = src.Spec.ImagePullJobTemplate.Selector.Names
			dst.Spec.ImagePullJobTemplate.Selector.LabelSelector = src.Spec.ImagePullJobTemplate.Selector.LabelSelector
		}
		if src.Spec.ImagePullJobTemplate.PodSelector != nil {
			dst.Spec.ImagePullJobTemplate.PodSelector = &v1beta1.ImagePullJobPodSelector{}
			dst.Spec.ImagePullJobTemplate.PodSelector.LabelSelector = src.Spec.ImagePullJobTemplate.PodSelector.LabelSelector
		}
		dst.Spec.ImagePullJobTemplate.Parallelism = src.Spec.ImagePullJobTemplate.Parallelism
		if src.Spec.ImagePullJobTemplate.PullPolicy != nil {
			dst.Spec.ImagePullJobTemplate.PullPolicy = &v1beta1.PullPolicy{}
			dst.Spec.ImagePullJobTemplate.PullPolicy.TimeoutSeconds = src.Spec.ImagePullJobTemplate.PullPolicy.TimeoutSeconds
			dst.Spec.ImagePullJobTemplate.PullPolicy.BackoffLimit = src.Spec.ImagePullJobTemplate.PullPolicy.BackoffLimit
		}
		dst.Spec.ImagePullJobTemplate.CompletionPolicy.Type = v1beta1.CompletionPolicyType(src.Spec.ImagePullJobTemplate.CompletionPolicy.Type)
		dst.Spec.ImagePullJobTemplate.CompletionPolicy.ActiveDeadlineSeconds = src.Spec.ImagePullJobTemplate.CompletionPolicy.ActiveDeadlineSeconds
		dst.Spec.ImagePullJobTemplate.CompletionPolicy.TTLSecondsAfterFinished = src.Spec.ImagePullJobTemplate.CompletionPolicy.TTLSecondsAfterFinished
		if src.Spec.ImagePullJobTemplate.SandboxConfig != nil {
			dst.Spec.ImagePullJobTemplate.SandboxConfig = &v1beta1.SandboxConfig{}
			dst.Spec.ImagePullJobTemplate.SandboxConfig.Labels = src.Spec.ImagePullJobTemplate.SandboxConfig.Labels
			dst.Spec.ImagePullJobTemplate.SandboxConfig.Annotations = src.Spec.ImagePullJobTemplate.SandboxConfig.Annotations
		}
		dst.Status.StartTime = src.Status.StartTime
		dst.Status.CompletionTime = src.Status.CompletionTime
		dst.Status.Desired = src.Status.Desired
		dst.Status.Active = src.Status.Active
		dst.Status.Succeeded = src.Status.Succeeded
		dst.Status.Failed = src.Status.Failed
		dst.Status.Message = src.Status.Message
		dst.Status.FailedNodes = src.Status.FailedNodes
	default:
		return fmt.Errorf("unsupported type %v", t)
	}
	return nil
}

func (dst *ImagePullJob) ConvertFrom(srcRaw conversion.Hub) error {
	switch t := srcRaw.(type) {
	case *v1beta1.ImagePullJob:
		src := srcRaw.(*v1beta1.ImagePullJob)
		dst.ObjectMeta = src.ObjectMeta
		dst.Spec.Image = src.Spec.Image
		dst.Spec.ImagePullJobTemplate.PullSecrets = src.Spec.ImagePullJobTemplate.PullSecrets
		if src.Spec.ImagePullJobTemplate.Selector != nil {
			dst.Spec.ImagePullJobTemplate.Selector = &ImagePullJobNodeSelector{}
			dst.Spec.ImagePullJobTemplate.Selector.Names = src.Spec.ImagePullJobTemplate.Selector.Names
			dst.Spec.ImagePullJobTemplate.Selector.LabelSelector = src.Spec.ImagePullJobTemplate.Selector.LabelSelector
		}
		if src.Spec.ImagePullJobTemplate.PodSelector != nil {
			dst.Spec.ImagePullJobTemplate.PodSelector = &ImagePullJobPodSelector{}
			dst.Spec.ImagePullJobTemplate.PodSelector.LabelSelector = src.Spec.ImagePullJobTemplate.PodSelector.LabelSelector
		}
		dst.Spec.ImagePullJobTemplate.Parallelism = src.Spec.ImagePullJobTemplate.Parallelism
		if src.Spec.ImagePullJobTemplate.PullPolicy != nil {
			dst.Spec.ImagePullJobTemplate.PullPolicy = &PullPolicy{}
			dst.Spec.ImagePullJobTemplate.PullPolicy.TimeoutSeconds = src.Spec.ImagePullJobTemplate.PullPolicy.TimeoutSeconds
			dst.Spec.ImagePullJobTemplate.PullPolicy.BackoffLimit = src.Spec.ImagePullJobTemplate.PullPolicy.BackoffLimit
		}
		dst.Spec.ImagePullJobTemplate.CompletionPolicy.Type = CompletionPolicyType(src.Spec.ImagePullJobTemplate.CompletionPolicy.Type)
		dst.Spec.ImagePullJobTemplate.CompletionPolicy.ActiveDeadlineSeconds = src.Spec.ImagePullJobTemplate.CompletionPolicy.ActiveDeadlineSeconds
		dst.Spec.ImagePullJobTemplate.CompletionPolicy.TTLSecondsAfterFinished = src.Spec.ImagePullJobTemplate.CompletionPolicy.TTLSecondsAfterFinished
		if src.Spec.ImagePullJobTemplate.SandboxConfig != nil {
			dst.Spec.ImagePullJobTemplate.SandboxConfig = &SandboxConfig{}
			dst.Spec.ImagePullJobTemplate.SandboxConfig.Labels = src.Spec.ImagePullJobTemplate.SandboxConfig.Labels
			dst.Spec.ImagePullJobTemplate.SandboxConfig.Annotations = src.Spec.ImagePullJobTemplate.SandboxConfig.Annotations
		}
		dst.Status.StartTime = src.Status.StartTime
		dst.Status.CompletionTime = src.Status.CompletionTime
		dst.Status.Desired = src.Status.Desired
		dst.Status.Active = src.Status.Active
		dst.Status.Succeeded = src.Status.Succeeded
		dst.Status.Failed = src.Status.Failed
		dst.Status.Message = src.Status.Message
		dst.Status.FailedNodes = src.Status.FailedNodes
	default:
		return fmt.Errorf("unsupported type %v", t)
	}
	return nil
}
