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

	v1beta1 "github.com/openkruise/kruise/apis/apps/v1beta1"
)

func (ipj *ImagePullJob) ConvertTo(dst conversion.Hub) error {
	switch t := dst.(type) {
	case *v1beta1.ImagePullJob:
		v := dst.(*v1beta1.ImagePullJob)
		v.ObjectMeta = ipj.ObjectMeta

		v.Spec = v1beta1.ImagePullJobSpec{
			Image: ipj.Spec.Image,
			ImagePullJobTemplate: v1beta1.ImagePullJobTemplate{
				PullSecrets: ipj.Spec.PullSecrets,
				Selector:    convertNodeSelectorToV1Beta1(ipj.Spec.Selector),
				PodSelector: convertPodSelectorToV1Beta1(ipj.Spec.PodSelector),
				Parallelism: ipj.Spec.Parallelism,
				PullPolicy:  convertPullPolicyToV1Beta1(ipj.Spec.PullPolicy),
				CompletionPolicy: v1beta1.CompletionPolicy{
					Type:                    v1beta1.CompletionPolicyType(ipj.Spec.CompletionPolicy.Type),
					ActiveDeadlineSeconds:   ipj.Spec.CompletionPolicy.ActiveDeadlineSeconds,
					TTLSecondsAfterFinished: ipj.Spec.CompletionPolicy.TTLSecondsAfterFinished,
				},
				SandboxConfig:   convertSandboxConfigToV1Beta1(ipj.Spec.SandboxConfig),
				ImagePullPolicy: v1beta1.ImagePullPolicy(ipj.Spec.ImagePullPolicy),
			},
		}

		v.Status = v1beta1.ImagePullJobStatus{
			StartTime:      ipj.Status.StartTime,
			CompletionTime: ipj.Status.CompletionTime,
			Desired:        ipj.Status.Desired,
			Active:         ipj.Status.Active,
			Succeeded:      ipj.Status.Succeeded,
			Failed:         ipj.Status.Failed,
			Message:        ipj.Status.Message,
			FailedNodes:    ipj.Status.FailedNodes,
		}
		return nil
	default:
		return fmt.Errorf("unsupported type %T", t)
	}
}

func (ipj *ImagePullJob) ConvertFrom(src conversion.Hub) error {
	switch t := src.(type) {
	case *v1beta1.ImagePullJob:
		v := src.(*v1beta1.ImagePullJob)
		ipj.ObjectMeta = v.ObjectMeta

		ipj.Spec = ImagePullJobSpec{
			Image: v.Spec.Image,
			ImagePullJobTemplate: ImagePullJobTemplate{
				PullSecrets: v.Spec.PullSecrets,
				Selector:    convertNodeSelectorToV1Alpha1(v.Spec.Selector),
				PodSelector: convertPodSelectorToV1Alpha1(v.Spec.PodSelector),
				Parallelism: v.Spec.Parallelism,
				PullPolicy:  convertPullPolicyToV1Alpha1(v.Spec.PullPolicy),
				CompletionPolicy: CompletionPolicy{
					Type:                    CompletionPolicyType(v.Spec.CompletionPolicy.Type),
					ActiveDeadlineSeconds:   v.Spec.CompletionPolicy.ActiveDeadlineSeconds,
					TTLSecondsAfterFinished: v.Spec.CompletionPolicy.TTLSecondsAfterFinished,
				},
				SandboxConfig:   convertSandboxConfigToV1Alpha1(v.Spec.SandboxConfig),
				ImagePullPolicy: ImagePullPolicy(v.Spec.ImagePullPolicy),
			},
		}

		ipj.Status = ImagePullJobStatus{
			StartTime:      v.Status.StartTime,
			CompletionTime: v.Status.CompletionTime,
			Desired:        v.Status.Desired,
			Active:         v.Status.Active,
			Succeeded:      v.Status.Succeeded,
			Failed:         v.Status.Failed,
			Message:        v.Status.Message,
			FailedNodes:    v.Status.FailedNodes,
		}
		return nil
	default:
		return fmt.Errorf("unsupported type %T", t)
	}
}

func convertPodSelectorToV1Beta1(in *ImagePullJobPodSelector) *v1beta1.ImagePullJobPodSelector {
	if in == nil {
		return nil
	}
	out := &v1beta1.ImagePullJobPodSelector{}
	out.LabelSelector = in.LabelSelector
	return out
}

func convertPodSelectorToV1Alpha1(in *v1beta1.ImagePullJobPodSelector) *ImagePullJobPodSelector {
	if in == nil {
		return nil
	}
	out := &ImagePullJobPodSelector{}
	out.LabelSelector = in.LabelSelector
	return out
}

func convertNodeSelectorToV1Beta1(in *ImagePullJobNodeSelector) *v1beta1.ImagePullJobNodeSelector {
	if in == nil {
		return nil
	}
	out := &v1beta1.ImagePullJobNodeSelector{}
	out.Names = in.Names
	out.LabelSelector = in.LabelSelector
	return out
}

func convertNodeSelectorToV1Alpha1(in *v1beta1.ImagePullJobNodeSelector) *ImagePullJobNodeSelector {
	if in == nil {
		return nil
	}
	out := &ImagePullJobNodeSelector{}
	out.Names = in.Names
	out.LabelSelector = in.LabelSelector
	return out
}

func convertPullPolicyToV1Beta1(in *PullPolicy) *v1beta1.PullPolicy {
	if in == nil {
		return nil
	}
	return &v1beta1.PullPolicy{
		TimeoutSeconds: in.TimeoutSeconds,
		BackoffLimit:   in.BackoffLimit,
	}
}

func convertPullPolicyToV1Alpha1(in *v1beta1.PullPolicy) *PullPolicy {
	if in == nil {
		return nil
	}
	return &PullPolicy{
		TimeoutSeconds: in.TimeoutSeconds,
		BackoffLimit:   in.BackoffLimit,
	}
}

func convertSandboxConfigToV1Beta1(in *SandboxConfig) *v1beta1.SandboxConfig {
	if in == nil {
		return nil
	}
	out := &v1beta1.SandboxConfig{}
	if in.Labels != nil {
		out.Labels = make(map[string]string, len(in.Labels))
		for k, v := range in.Labels {
			out.Labels[k] = v
		}
	}
	if in.Annotations != nil {
		out.Annotations = make(map[string]string, len(in.Annotations))
		for k, v := range in.Annotations {
			out.Annotations[k] = v
		}
	}
	return out
}

func convertSandboxConfigToV1Alpha1(in *v1beta1.SandboxConfig) *SandboxConfig {
	if in == nil {
		return nil
	}
	out := &SandboxConfig{}
	if in.Labels != nil {
		out.Labels = make(map[string]string, len(in.Labels))
		for k, v := range in.Labels {
			out.Labels[k] = v
		}
	}
	if in.Annotations != nil {
		out.Annotations = make(map[string]string, len(in.Annotations))
		for k, v := range in.Annotations {
			out.Annotations[k] = v
		}
	}
	return out
}
