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

func (o *ImageListPullJob) ConvertTo(dst conversion.Hub) error {
	switch t := dst.(type) {
	case *v1beta1.ImageListPullJob:
		v := dst.(*v1beta1.ImageListPullJob)
		v.ObjectMeta = o.ObjectMeta
		v.Spec = v1beta1.ImageListPullJobSpec{
			Images: o.Spec.Images,
			ImagePullJobTemplate: v1beta1.ImagePullJobTemplate{
				PullSecrets: o.Spec.PullSecrets,
				Selector:    convertNodeSelectorToV1Beta1(o.Spec.Selector),
				PodSelector: convertPodSelectorToV1Beta1(o.Spec.PodSelector),
				Parallelism: o.Spec.Parallelism,
				PullPolicy:  convertPullPolicyToV1Beta1(o.Spec.PullPolicy),
				CompletionPolicy: v1beta1.CompletionPolicy{
					Type:                    v1beta1.CompletionPolicyType(o.Spec.CompletionPolicy.Type),
					ActiveDeadlineSeconds:   o.Spec.CompletionPolicy.ActiveDeadlineSeconds,
					TTLSecondsAfterFinished: o.Spec.CompletionPolicy.TTLSecondsAfterFinished,
				},
				SandboxConfig:   convertSandboxConfigToV1Beta1(o.Spec.SandboxConfig),
				ImagePullPolicy: v1beta1.ImagePullPolicy(o.Spec.ImagePullPolicy),
			},
		}
		v.Status = v1beta1.ImageListPullJobStatus{
			StartTime:           o.Status.StartTime,
			CompletionTime:      o.Status.CompletionTime,
			Desired:             o.Status.Desired,
			Active:              o.Status.Active,
			Completed:           o.Status.Completed,
			Succeeded:           o.Status.Succeeded,
			FailedImageStatuses: convertFailedImageStatusesToV1Beta1(o.Status.FailedImageStatuses),
		}
		return nil
	default:
		return fmt.Errorf("unsupported type %T", t)
	}
}

func (o *ImageListPullJob) ConvertFrom(src conversion.Hub) error {
	switch t := src.(type) {
	case *v1beta1.ImageListPullJob:
		v := src.(*v1beta1.ImageListPullJob)
		o.ObjectMeta = v.ObjectMeta
		o.Spec = ImageListPullJobSpec{
			Images: v.Spec.Images,
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
		o.Status = ImageListPullJobStatus{
			StartTime:           v.Status.StartTime,
			CompletionTime:      v.Status.CompletionTime,
			Desired:             v.Status.Desired,
			Active:              v.Status.Active,
			Completed:           v.Status.Completed,
			Succeeded:           v.Status.Succeeded,
			FailedImageStatuses: convertFailedImageStatusesToV1Alpha1(v.Status.FailedImageStatuses),
		}
		return nil
	default:
		return fmt.Errorf("unsupported type %T", t)
	}
}

func convertFailedImageStatusesToV1Beta1(in []*FailedImageStatus) []*v1beta1.FailedImageStatus {
	if in == nil {
		return nil
	}
	out := make([]*v1beta1.FailedImageStatus, 0, len(in))
	for _, s := range in {
		if s == nil {
			continue
		}
		out = append(out, &v1beta1.FailedImageStatus{
			ImagePullJob: s.ImagePullJob,
			Name:         s.Name,
			Message:      s.Message,
		})
	}
	return out
}

func convertFailedImageStatusesToV1Alpha1(in []*v1beta1.FailedImageStatus) []*FailedImageStatus {
	if in == nil {
		return nil
	}
	out := make([]*FailedImageStatus, 0, len(in))
	for _, s := range in {
		if s == nil {
			continue
		}
		out = append(out, &FailedImageStatus{
			ImagePullJob: s.ImagePullJob,
			Name:         s.Name,
			Message:      s.Message,
		})
	}
	return out
}
