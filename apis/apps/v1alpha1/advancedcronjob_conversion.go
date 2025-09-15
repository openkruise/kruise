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

func (acj *AdvancedCronJob) ConvertTo(dst conversion.Hub) error {
	switch t := dst.(type) {
	case *v1beta1.AdvancedCronJob:
		acjv1beta1 := dst.(*v1beta1.AdvancedCronJob)
		acjv1beta1.ObjectMeta = acj.ObjectMeta

		// spec
		acjv1beta1.Spec = v1beta1.AdvancedCronJobSpec{
			Schedule:                   acj.Spec.Schedule,
			TimeZone:                   acj.Spec.TimeZone,
			StartingDeadlineSeconds:    acj.Spec.StartingDeadlineSeconds,
			ConcurrencyPolicy:          v1beta1.ConcurrencyPolicy(acj.Spec.ConcurrencyPolicy),
			Paused:                     acj.Spec.Paused,
			SuccessfulJobsHistoryLimit: acj.Spec.SuccessfulJobsHistoryLimit,
			FailedJobsHistoryLimit:     acj.Spec.FailedJobsHistoryLimit,
			Template: v1beta1.CronJobTemplate{
				JobTemplate:          acj.Spec.Template.JobTemplate,
				BroadcastJobTemplate: convertBroadcastJobTemplateToV1Beta1(acj.Spec.Template.BroadcastJobTemplate),
			},
		}

		// status
		acjv1beta1.Status = v1beta1.AdvancedCronJobStatus{
			Type:             v1beta1.TemplateKind(acj.Status.Type),
			Active:           acj.Status.Active,
			LastScheduleTime: acj.Status.LastScheduleTime,
		}

		return nil

	default:
		return fmt.Errorf("unsupported type %v", t)
	}
}

func (acj *AdvancedCronJob) ConvertFrom(src conversion.Hub) error {
	switch t := src.(type) {
	case *v1beta1.AdvancedCronJob:
		acjv1beta1 := src.(*v1beta1.AdvancedCronJob)
		acj.ObjectMeta = acjv1beta1.ObjectMeta

		// spec
		acj.Spec = AdvancedCronJobSpec{
			Schedule:                   acjv1beta1.Spec.Schedule,
			TimeZone:                   acjv1beta1.Spec.TimeZone,
			StartingDeadlineSeconds:    acjv1beta1.Spec.StartingDeadlineSeconds,
			ConcurrencyPolicy:          ConcurrencyPolicy(acjv1beta1.Spec.ConcurrencyPolicy),
			Paused:                     acjv1beta1.Spec.Paused,
			SuccessfulJobsHistoryLimit: acjv1beta1.Spec.SuccessfulJobsHistoryLimit,
			FailedJobsHistoryLimit:     acjv1beta1.Spec.FailedJobsHistoryLimit,
			Template: CronJobTemplate{
				JobTemplate:          acjv1beta1.Spec.Template.JobTemplate,
				BroadcastJobTemplate: convertBroadcastJobTemplateToV1Alpha1(acjv1beta1.Spec.Template.BroadcastJobTemplate),
			},
		}

		// status
		acj.Status = AdvancedCronJobStatus{
			Type:             TemplateKind(acjv1beta1.Status.Type),
			Active:           acjv1beta1.Status.Active,
			LastScheduleTime: acjv1beta1.Status.LastScheduleTime,
		}

		return nil
	default:
		return fmt.Errorf("unsupported type %v", t)
	}
}

func convertBroadcastJobTemplateToV1Beta1(template *BroadcastJobTemplateSpec) *v1beta1.BroadcastJobTemplateSpec {
	if template == nil {
		return nil
	}
	return &v1beta1.BroadcastJobTemplateSpec{
		ObjectMeta: template.ObjectMeta,
		Spec:       convertBroadcastJobSpecToV1Beta1(template.Spec),
	}
}

func convertBroadcastJobTemplateToV1Alpha1(template *v1beta1.BroadcastJobTemplateSpec) *BroadcastJobTemplateSpec {
	if template == nil {
		return nil
	}
	return &BroadcastJobTemplateSpec{
		ObjectMeta: template.ObjectMeta,
		Spec:       convertBroadcastJobSpecToV1Alpha1(template.Spec),
	}
}

func convertBroadcastJobSpecToV1Beta1(spec BroadcastJobSpec) v1beta1.BroadcastJobSpec {
	return v1beta1.BroadcastJobSpec{
		Parallelism: spec.Parallelism,
		Template:    spec.Template,
		CompletionPolicy: v1beta1.CompletionPolicy{
			Type:                    v1beta1.CompletionPolicyType(spec.CompletionPolicy.Type),
			ActiveDeadlineSeconds:   spec.CompletionPolicy.ActiveDeadlineSeconds,
			TTLSecondsAfterFinished: spec.CompletionPolicy.TTLSecondsAfterFinished,
		},
		Paused: spec.Paused,
		FailurePolicy: v1beta1.FailurePolicy{
			Type:         v1beta1.FailurePolicyType(spec.FailurePolicy.Type),
			RestartLimit: spec.FailurePolicy.RestartLimit,
		},
	}
}

func convertBroadcastJobSpecToV1Alpha1(spec v1beta1.BroadcastJobSpec) BroadcastJobSpec {
	return BroadcastJobSpec{
		Parallelism: spec.Parallelism,
		Template:    spec.Template,
		CompletionPolicy: CompletionPolicy{
			Type:                    CompletionPolicyType(spec.CompletionPolicy.Type),
			ActiveDeadlineSeconds:   spec.CompletionPolicy.ActiveDeadlineSeconds,
			TTLSecondsAfterFinished: spec.CompletionPolicy.TTLSecondsAfterFinished,
		},
		Paused: spec.Paused,
		FailurePolicy: FailurePolicy{
			Type:         FailurePolicyType(spec.FailurePolicy.Type),
			RestartLimit: spec.FailurePolicy.RestartLimit,
		},
	}
}
