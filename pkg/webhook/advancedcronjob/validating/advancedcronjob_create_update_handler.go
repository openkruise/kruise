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

package validating

import (
	"context"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/robfig/cron/v3"
	admissionv1 "k8s.io/api/admission/v1"
	batchv1 "k8s.io/api/batch/v1"
	v1 "k8s.io/api/core/v1"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	genericvalidation "k8s.io/apimachinery/pkg/api/validation"
	validationutil "k8s.io/apimachinery/pkg/util/validation"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"k8s.io/kubernetes/pkg/apis/core"
	corev1 "k8s.io/kubernetes/pkg/apis/core/v1"
	apivalidation "k8s.io/kubernetes/pkg/apis/core/validation"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	webhookutil "github.com/openkruise/kruise/pkg/webhook/util"
)

const (
	AdvancedCronJobNameMaxLen      = 63
	validateAdvancedCronJobNameMsg = "AdvancedCronJob name must consist of alphanumeric characters or '-'"
	validAdvancedCronJobNameFmt    = `^[a-zA-Z0-9\-]+$`
)

var (
	validateAdvancedCronJobNameRegex = regexp.MustCompile(validAdvancedCronJobNameFmt)
)

// AdvancedCronJobCreateUpdateHandler handles AdvancedCronJob
type AdvancedCronJobCreateUpdateHandler struct {
	// Decoder decodes objects
	Decoder admission.Decoder
}

func (h *AdvancedCronJobCreateUpdateHandler) validateAdvancedCronJob(obj *appsv1alpha1.AdvancedCronJob) field.ErrorList {
	allErrs := genericvalidation.ValidateObjectMeta(&obj.ObjectMeta, true, validateAdvancedCronJobName, field.NewPath("metadata"))
	allErrs = append(allErrs, validateAdvancedCronJobSpec(&obj.Spec, field.NewPath("spec"))...)
	return allErrs
}

func validateAdvancedCronJobSpec(spec *appsv1alpha1.AdvancedCronJobSpec, fldPath *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}
	allErrs = append(allErrs, validateAdvancedCronJobSpecSchedule(spec, fldPath)...)
	allErrs = append(allErrs, validateAdvancedCronJobSpecTemplate(spec, fldPath)...)
	if spec.StartingDeadlineSeconds != nil {
		allErrs = append(allErrs, apivalidation.ValidateNonnegativeField(*spec.StartingDeadlineSeconds, fldPath.Child("startingDeadlineSeconds"))...)
	}
	if spec.SuccessfulJobsHistoryLimit != nil {
		allErrs = append(allErrs, apivalidation.ValidateNonnegativeField(int64(*spec.SuccessfulJobsHistoryLimit), fldPath.Child("successfulJobsHistoryLimit"))...)
	}
	if spec.FailedJobsHistoryLimit != nil {
		allErrs = append(allErrs, apivalidation.ValidateNonnegativeField(int64(*spec.FailedJobsHistoryLimit), fldPath.Child("failedJobsHistoryLimit"))...)
	}
	allErrs = append(allErrs, validateTimeZone(spec.TimeZone, fldPath.Child("timeZone"))...)
	return allErrs
}

func validateAdvancedCronJobSpecSchedule(spec *appsv1alpha1.AdvancedCronJobSpec, fldPath *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}
	if len(spec.Schedule) == 0 {
		allErrs = append(allErrs, field.Invalid(fldPath.Child("schedule"),
			spec.Schedule,
			"schedule cannot be empty, please provide valid cron schedule."))
	}

	if _, err := cron.ParseStandard(spec.Schedule); err != nil {
		allErrs = append(allErrs, field.Invalid(fldPath.Child("schedule"),
			spec.Schedule, err.Error()))
	}
	if strings.Contains(spec.Schedule, "TZ") && spec.TimeZone != nil {
		allErrs = append(allErrs, field.Invalid(fldPath.Child("schedule"),
			spec.Schedule, "cannot use both timeZone field and TZ or CRON_TZ in schedule"))
	}
	return allErrs
}

func validateTimeZone(timeZone *string, fldPath *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}
	if timeZone == nil {
		return allErrs
	}

	if len(*timeZone) == 0 {
		allErrs = append(allErrs, field.Invalid(fldPath, timeZone, "timeZone must be nil or non-empty string"))
		return allErrs
	}

	if strings.EqualFold(*timeZone, "Local") {
		allErrs = append(allErrs, field.Invalid(fldPath, timeZone, "timeZone must be an explicit time zone as defined in https://www.iana.org/time-zones"))
	}

	if _, err := time.LoadLocation(*timeZone); err != nil {
		allErrs = append(allErrs, field.Invalid(fldPath, timeZone, err.Error()))
	}

	return allErrs
}

func validateAdvancedCronJobSpecTemplate(spec *appsv1alpha1.AdvancedCronJobSpec, fldPath *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}
	templateCount := 0
	if spec.Template.JobTemplate != nil {
		templateCount++
		allErrs = append(allErrs, validateJobTemplateSpec(spec.Template.JobTemplate, fldPath)...)
	}

	if spec.Template.BroadcastJobTemplate != nil {
		templateCount++
		allErrs = append(allErrs, validateBroadcastJobTemplateSpec(spec.Template.BroadcastJobTemplate, fldPath)...)
	}

	if templateCount == 0 {
		allErrs = append(allErrs, field.Forbidden(fldPath.Child("template"),
			"spec must have one template, either JobTemplate or BroadcastJobTemplate should be provided"))
	} else if templateCount > 1 {
		allErrs = append(allErrs, field.Forbidden(fldPath.Child("template"),
			"spec can have only one template, either JobTemplate or BroadcastJobTemplate should be provided"))
	}
	return allErrs
}

func validateJobTemplateSpec(jobSpec *batchv1.JobTemplateSpec, fldPath *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}
	coreTemplate, err := convertPodTemplateSpec(&jobSpec.Spec.Template)
	if err != nil {
		allErrs = append(allErrs, field.Invalid(fldPath.Root(), jobSpec.Spec.Template, fmt.Sprintf("Convert_v1_PodTemplateSpec_To_core_PodTemplateSpec failed: %v", err)))
		return allErrs
	}
	return append(allErrs, apivalidation.ValidatePodTemplateSpec(coreTemplate, fldPath.Child("template"), webhookutil.DefaultPodValidationOptions)...)
}

func validateBroadcastJobTemplateSpec(brJobSpec *appsv1alpha1.BroadcastJobTemplateSpec, fldPath *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}
	coreTemplate, err := convertPodTemplateSpec(&brJobSpec.Spec.Template)
	if err != nil {
		allErrs = append(allErrs, field.Invalid(fldPath.Root(), brJobSpec.Spec.Template, fmt.Sprintf("Convert_v1_PodTemplateSpec_To_core_PodTemplateSpec failed: %v", err)))
		return allErrs
	}
	return append(allErrs, apivalidation.ValidatePodTemplateSpec(coreTemplate, fldPath.Child("template"), webhookutil.DefaultPodValidationOptions)...)
}

func convertPodTemplateSpec(template *v1.PodTemplateSpec) (*core.PodTemplateSpec, error) {
	coreTemplate := &core.PodTemplateSpec{}
	if err := corev1.Convert_v1_PodTemplateSpec_To_core_PodTemplateSpec(template.DeepCopy(), coreTemplate, nil); err != nil {
		return nil, err
	}
	return coreTemplate, nil
}

func validateAdvancedCronJobName(name string, prefix bool) (allErrs []string) {
	if !validateAdvancedCronJobNameRegex.MatchString(name) {
		allErrs = append(allErrs, validationutil.RegexError(validateAdvancedCronJobNameMsg, validAdvancedCronJobNameFmt, "example-com"))
	}
	if len(name) > AdvancedCronJobNameMaxLen {
		allErrs = append(allErrs, validationutil.MaxLenError(AdvancedCronJobNameMaxLen))
	}
	return allErrs
}

func (h *AdvancedCronJobCreateUpdateHandler) validateAdvancedCronJobUpdate(obj, oldObj *appsv1alpha1.AdvancedCronJob) field.ErrorList {
	allErrs := apivalidation.ValidateObjectMetaUpdate(&obj.ObjectMeta, &oldObj.ObjectMeta, field.NewPath("metadata"))
	allErrs = append(allErrs, validateAdvancedCronJobSpec(&obj.Spec, field.NewPath("spec"))...)

	advanceCronJob := obj.DeepCopy()
	advanceCronJob.Spec.Schedule = oldObj.Spec.Schedule
	advanceCronJob.Spec.ConcurrencyPolicy = oldObj.Spec.ConcurrencyPolicy
	advanceCronJob.Spec.SuccessfulJobsHistoryLimit = oldObj.Spec.SuccessfulJobsHistoryLimit
	advanceCronJob.Spec.FailedJobsHistoryLimit = oldObj.Spec.FailedJobsHistoryLimit
	advanceCronJob.Spec.StartingDeadlineSeconds = oldObj.Spec.StartingDeadlineSeconds
	advanceCronJob.Spec.Paused = oldObj.Spec.Paused
	advanceCronJob.Spec.TimeZone = oldObj.Spec.TimeZone
	if !apiequality.Semantic.DeepEqual(advanceCronJob.Spec, oldObj.Spec) {
		allErrs = append(allErrs, field.Forbidden(field.NewPath("spec"), "updates to advancedcronjob spec for fields other than 'schedule', 'concurrencyPolicy', 'successfulJobsHistoryLimit', 'failedJobsHistoryLimit', 'startingDeadlineSeconds', 'timeZone' and 'paused' are forbidden"))
	}
	return allErrs
}

var _ admission.Handler = &AdvancedCronJobCreateUpdateHandler{}

// Handle handles admission requests.
func (h *AdvancedCronJobCreateUpdateHandler) Handle(ctx context.Context, req admission.Request) admission.Response {
	obj := &appsv1alpha1.AdvancedCronJob{}

	err := h.Decoder.Decode(req, obj)
	if err != nil {
		return admission.Errored(http.StatusBadRequest, err)
	}
	switch req.AdmissionRequest.Operation {
	case admissionv1.Create:
		if allErrs := h.validateAdvancedCronJob(obj); len(allErrs) > 0 {
			return admission.Errored(http.StatusUnprocessableEntity, allErrs.ToAggregate())
		}
	case admissionv1.Update:
		oldObj := &appsv1alpha1.AdvancedCronJob{}
		if err := h.Decoder.DecodeRaw(req.AdmissionRequest.OldObject, oldObj); err != nil {
			return admission.Errored(http.StatusBadRequest, err)
		}

		if allErrs := h.validateAdvancedCronJobUpdate(obj, oldObj); len(allErrs) > 0 {
			return admission.Errored(http.StatusUnprocessableEntity, allErrs.ToAggregate())
		}
	}

	return admission.ValidationResponse(true, "")
}
