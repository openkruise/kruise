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

func (src *AdvancedCronJob) ConvertTo(dstRaw conversion.Hub) error {
	switch t := dstRaw.(type) {
	case *v1beta1.AdvancedCronJob:
		dst := dstRaw.(*v1beta1.AdvancedCronJob)
		dst.ObjectMeta = src.ObjectMeta
		dst.Spec.Schedule = src.Spec.Schedule
		dst.Spec.TimeZone = src.Spec.TimeZone
		dst.Spec.StartingDeadlineSeconds = src.Spec.StartingDeadlineSeconds
		dst.Spec.ConcurrencyPolicy = v1beta1.ConcurrencyPolicy(src.Spec.ConcurrencyPolicy)
		dst.Spec.Paused = src.Spec.Paused
		dst.Spec.SuccessfulJobsHistoryLimit = src.Spec.SuccessfulJobsHistoryLimit
		dst.Spec.FailedJobsHistoryLimit = src.Spec.FailedJobsHistoryLimit
		dst.Spec.Template.JobTemplate = src.Spec.Template.JobTemplate
		if src.Spec.Template.BroadcastJobTemplate != nil {
			dst.Spec.Template.BroadcastJobTemplate = &v1beta1.BroadcastJobTemplateSpec{}
			dst.Spec.Template.BroadcastJobTemplate.ObjectMeta = src.Spec.Template.BroadcastJobTemplate.ObjectMeta
			dst.Spec.Template.BroadcastJobTemplate.Spec.Parallelism = src.Spec.Template.BroadcastJobTemplate.Spec.Parallelism
			dst.Spec.Template.BroadcastJobTemplate.Spec.Template = src.Spec.Template.BroadcastJobTemplate.Spec.Template
			dst.Spec.Template.BroadcastJobTemplate.Spec.CompletionPolicy.Type = v1beta1.CompletionPolicyType(src.Spec.Template.BroadcastJobTemplate.Spec.CompletionPolicy.Type)
			dst.Spec.Template.BroadcastJobTemplate.Spec.CompletionPolicy.ActiveDeadlineSeconds = src.Spec.Template.BroadcastJobTemplate.Spec.CompletionPolicy.ActiveDeadlineSeconds
			dst.Spec.Template.BroadcastJobTemplate.Spec.CompletionPolicy.TTLSecondsAfterFinished = src.Spec.Template.BroadcastJobTemplate.Spec.CompletionPolicy.TTLSecondsAfterFinished
			dst.Spec.Template.BroadcastJobTemplate.Spec.Paused = src.Spec.Template.BroadcastJobTemplate.Spec.Paused
			dst.Spec.Template.BroadcastJobTemplate.Spec.FailurePolicy.Type = v1beta1.FailurePolicyType(src.Spec.Template.BroadcastJobTemplate.Spec.FailurePolicy.Type)
			dst.Spec.Template.BroadcastJobTemplate.Spec.FailurePolicy.RestartLimit = src.Spec.Template.BroadcastJobTemplate.Spec.FailurePolicy.RestartLimit
		}
		dst.Status.Type = v1beta1.TemplateKind(src.Status.Type)
		dst.Status.Active = src.Status.Active
		dst.Status.LastScheduleTime = src.Status.LastScheduleTime
	default:
		return fmt.Errorf("unsupported type %v", t)
	}
	return nil
}

func (dst *AdvancedCronJob) ConvertFrom(srcRaw conversion.Hub) error {
	switch t := srcRaw.(type) {
	case *v1beta1.AdvancedCronJob:
		src := srcRaw.(*v1beta1.AdvancedCronJob)
		dst.ObjectMeta = src.ObjectMeta
		dst.Spec.Schedule = src.Spec.Schedule
		dst.Spec.TimeZone = src.Spec.TimeZone
		dst.Spec.StartingDeadlineSeconds = src.Spec.StartingDeadlineSeconds
		dst.Spec.ConcurrencyPolicy = ConcurrencyPolicy(src.Spec.ConcurrencyPolicy)
		dst.Spec.Paused = src.Spec.Paused
		dst.Spec.SuccessfulJobsHistoryLimit = src.Spec.SuccessfulJobsHistoryLimit
		dst.Spec.FailedJobsHistoryLimit = src.Spec.FailedJobsHistoryLimit
		dst.Spec.Template.JobTemplate = src.Spec.Template.JobTemplate
		if src.Spec.Template.BroadcastJobTemplate != nil {
			dst.Spec.Template.BroadcastJobTemplate = &BroadcastJobTemplateSpec{}
			dst.Spec.Template.BroadcastJobTemplate.ObjectMeta = src.Spec.Template.BroadcastJobTemplate.ObjectMeta
			dst.Spec.Template.BroadcastJobTemplate.Spec.Parallelism = src.Spec.Template.BroadcastJobTemplate.Spec.Parallelism
			dst.Spec.Template.BroadcastJobTemplate.Spec.Template = src.Spec.Template.BroadcastJobTemplate.Spec.Template
			dst.Spec.Template.BroadcastJobTemplate.Spec.CompletionPolicy.Type = CompletionPolicyType(src.Spec.Template.BroadcastJobTemplate.Spec.CompletionPolicy.Type)
			dst.Spec.Template.BroadcastJobTemplate.Spec.CompletionPolicy.ActiveDeadlineSeconds = src.Spec.Template.BroadcastJobTemplate.Spec.CompletionPolicy.ActiveDeadlineSeconds
			dst.Spec.Template.BroadcastJobTemplate.Spec.CompletionPolicy.TTLSecondsAfterFinished = src.Spec.Template.BroadcastJobTemplate.Spec.CompletionPolicy.TTLSecondsAfterFinished
			dst.Spec.Template.BroadcastJobTemplate.Spec.Paused = src.Spec.Template.BroadcastJobTemplate.Spec.Paused
			dst.Spec.Template.BroadcastJobTemplate.Spec.FailurePolicy.Type = FailurePolicyType(src.Spec.Template.BroadcastJobTemplate.Spec.FailurePolicy.Type)
			dst.Spec.Template.BroadcastJobTemplate.Spec.FailurePolicy.RestartLimit = src.Spec.Template.BroadcastJobTemplate.Spec.FailurePolicy.RestartLimit
		}
		dst.Status.Type = TemplateKind(src.Status.Type)
		dst.Status.Active = src.Status.Active
		dst.Status.LastScheduleTime = src.Status.LastScheduleTime
	default:
		return fmt.Errorf("unsupported type %v", t)
	}
	return nil
}
