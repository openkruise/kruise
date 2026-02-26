/*
Copyright 2019 The Kruise Authors.

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

	v1 "k8s.io/api/core/v1"
	genericvalidation "k8s.io/apimachinery/pkg/api/validation"
	validationutil "k8s.io/apimachinery/pkg/util/validation"
	"k8s.io/apimachinery/pkg/util/validation/field"
	corevalidation "k8s.io/kubernetes/pkg/apis/core/validation"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	appsv1beta1 "github.com/openkruise/kruise/apis/apps/v1beta1"
	"github.com/openkruise/kruise/pkg/controller/broadcastjob"
	webhookutil "github.com/openkruise/kruise/pkg/webhook/util"
	"github.com/openkruise/kruise/pkg/webhook/util/convertor"
)

const (
	broadcastJobNameMaxLen = 63
)

var (
	validateBroadcastJobNameMsg   = "BroadcastJob name must consist of alphanumeric characters or '-'"
	validateBroadcastJobNameRegex = regexp.MustCompile(validBroadcastJobNameFmt)
	validBroadcastJobNameFmt      = `^[a-zA-Z0-9\-]+$`
)

// BroadcastJobCreateUpdateHandler handles BroadcastJob
type BroadcastJobCreateUpdateHandler struct {
	// To use the client, you need to do the following:
	// - uncomment it
	// - import sigs.k8s.io/controller-runtime/pkg/client
	// - uncomment the InjectClient method at the bottom of this file.
	// Client  client.Client

	// Decoder decodes objects
	Decoder admission.Decoder
}

func (h *BroadcastJobCreateUpdateHandler) validatingBroadcastJobFn(ctx context.Context, obj *appsv1beta1.BroadcastJob) (bool, string, error) {

	allErrs := validateBroadcastJob(obj)
	if len(allErrs) != 0 {
		return false, "", allErrs.ToAggregate()
	}
	return true, "allowed to be admitted", nil
}

func validateBroadcastJob(obj *appsv1beta1.BroadcastJob) field.ErrorList {
	allErrs := genericvalidation.ValidateObjectMeta(&obj.ObjectMeta, true, validateBroadcastJobName, field.NewPath("metadata"))
	allErrs = append(allErrs, validateBroadcastJobSpec(&obj.Spec, field.NewPath("spec"))...)
	return allErrs
}

func validateBroadcastJobSpec(spec *appsv1beta1.BroadcastJobSpec, fldPath *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}

	switch spec.CompletionPolicy.Type {
	case appsv1beta1.Always:

	case appsv1beta1.Never:
		if spec.CompletionPolicy.TTLSecondsAfterFinished != nil {
			allErrs = append(allErrs, field.Invalid(fldPath.Child("completionPolicy").Child("ttlSecondsAfterFinished"),
				spec.CompletionPolicy.TTLSecondsAfterFinished,
				"ttlSecondsAfterFinished can just work with Always CompletionPolicyType"))
		}
		if spec.CompletionPolicy.ActiveDeadlineSeconds != nil {
			allErrs = append(allErrs, field.Invalid(fldPath.Child("completionPolicy").Child("activeDeadlineSeconds"),
				spec.CompletionPolicy.ActiveDeadlineSeconds,
				"activeDeadlineSeconds can just work with Always CompletionPolicyType"))
		}
	default:
	}
	coreTemplate, err := convertor.ConvertPodTemplateSpec(&spec.Template)
	if err != nil {
		allErrs = append(allErrs, field.Invalid(fldPath.Root(), spec.Template, fmt.Sprintf("Convert_v1_PodTemplateSpec_To_core_PodTemplateSpec failed: %v", err)))
		return allErrs
	}
	if spec.Template.Spec.RestartPolicy != v1.RestartPolicyOnFailure &&
		spec.Template.Spec.RestartPolicy != v1.RestartPolicyNever {
		allErrs = append(allErrs, field.Invalid(fldPath.Child("template").Child("spec").Child("restartPolicy"),
			spec.Template.Spec.RestartPolicy,
			"pod restartPolicy can only be Never or OnFailure"))
	}
	if spec.Template.Labels != nil {
		if spec.Template.Labels[broadcastjob.JobNameLabelKey] != "" || spec.Template.Labels[broadcastjob.ControllerUIDLabelKey] != "" {
			allErrs = append(allErrs, field.Invalid(fldPath.Child("template").Child("metadata").Child("labels"),
				spec.Template.Labels,
				fmt.Sprintf("\"%s\" and \"%s\" are not allowed to preset in pod labels", broadcastjob.JobNameLabelKey, broadcastjob.ControllerUIDLabelKey)))
		}
	}
	allErrs = append(allErrs, corevalidation.ValidatePodTemplateSpec(coreTemplate, fldPath.Child("template"), webhookutil.DefaultPodValidationOptions)...)

	// Validate PodFailurePolicy if present
	if spec.PodFailurePolicy != nil {
		allErrs = append(allErrs, validatePodFailurePolicy(spec.PodFailurePolicy, fldPath.Child("podFailurePolicy"))...)
	}

	// Validate PodReplacementPolicy if present
	if spec.PodReplacementPolicy != nil {
		allErrs = append(allErrs, validatePodReplacementPolicy(spec.PodReplacementPolicy, fldPath.Child("podReplacementPolicy"))...)
	}

	return allErrs
}

func validatePodReplacementPolicy(policy *appsv1beta1.PodReplacementPolicy, fldPath *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}

	switch *policy {
	case appsv1beta1.TerminatingOrFailed, appsv1beta1.Failed:
		// valid values
	default:
		allErrs = append(allErrs, field.NotSupported(fldPath, *policy,
			[]string{string(appsv1beta1.TerminatingOrFailed), string(appsv1beta1.Failed)}))
	}

	return allErrs
}

func validatePodFailurePolicy(policy *appsv1beta1.PodFailurePolicy, fldPath *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}

	if len(policy.Rules) == 0 {
		allErrs = append(allErrs, field.Required(fldPath.Child("rules"), "must have at least one rule"))
		return allErrs
	}
	if len(policy.Rules) > 20 {
		allErrs = append(allErrs, field.TooMany(fldPath.Child("rules"), len(policy.Rules), 20))
	}

	for i, rule := range policy.Rules {
		rulePath := fldPath.Child("rules").Index(i)
		allErrs = append(allErrs, validatePodFailurePolicyRule(&rule, rulePath)...)
	}

	return allErrs
}

func validatePodFailurePolicyRule(rule *appsv1beta1.PodFailurePolicyRule, fldPath *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}

	// Validate action
	switch rule.Action {
	case appsv1beta1.PodFailurePolicyActionFailJob, appsv1beta1.PodFailurePolicyActionIgnore, appsv1beta1.PodFailurePolicyActionCount:
		// valid
	default:
		allErrs = append(allErrs, field.NotSupported(fldPath.Child("action"), rule.Action,
			[]string{string(appsv1beta1.PodFailurePolicyActionFailJob), string(appsv1beta1.PodFailurePolicyActionIgnore), string(appsv1beta1.PodFailurePolicyActionCount)}))
	}

	// Must have exactly one of onExitCodes or onPodConditions
	hasOnExitCodes := rule.OnExitCodes != nil
	hasOnPodConditions := len(rule.OnPodConditions) > 0

	if !hasOnExitCodes && !hasOnPodConditions {
		allErrs = append(allErrs, field.Required(fldPath, "must specify one of onExitCodes or onPodConditions"))
	}
	if hasOnExitCodes && hasOnPodConditions {
		allErrs = append(allErrs, field.Invalid(fldPath, rule, "cannot specify both onExitCodes and onPodConditions"))
	}

	// Validate onExitCodes if present
	if rule.OnExitCodes != nil {
		allErrs = append(allErrs, validatePodFailurePolicyOnExitCodes(rule.OnExitCodes, fldPath.Child("onExitCodes"))...)
	}

	// Validate onPodConditions if present
	if len(rule.OnPodConditions) > 0 {
		if len(rule.OnPodConditions) > 20 {
			allErrs = append(allErrs, field.TooMany(fldPath.Child("onPodConditions"), len(rule.OnPodConditions), 20))
		}
		for j, condition := range rule.OnPodConditions {
			conditionPath := fldPath.Child("onPodConditions").Index(j)
			if condition.Type == "" {
				allErrs = append(allErrs, field.Required(conditionPath.Child("type"), "must specify condition type"))
			}
			switch condition.Status {
			case v1.ConditionTrue, v1.ConditionFalse, v1.ConditionUnknown:
				// valid
			default:
				allErrs = append(allErrs, field.NotSupported(conditionPath.Child("status"), condition.Status,
					[]string{string(v1.ConditionTrue), string(v1.ConditionFalse), string(v1.ConditionUnknown)}))
			}
		}
	}

	return allErrs
}

func validatePodFailurePolicyOnExitCodes(req *appsv1beta1.PodFailurePolicyOnExitCodesRequirement, fldPath *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}

	// Validate operator
	switch req.Operator {
	case appsv1beta1.PodFailurePolicyOnExitCodesOpIn, appsv1beta1.PodFailurePolicyOnExitCodesOpNotIn:
		// valid
	default:
		allErrs = append(allErrs, field.NotSupported(fldPath.Child("operator"), req.Operator,
			[]string{string(appsv1beta1.PodFailurePolicyOnExitCodesOpIn), string(appsv1beta1.PodFailurePolicyOnExitCodesOpNotIn)}))
	}

	// Validate values
	if len(req.Values) == 0 {
		allErrs = append(allErrs, field.Required(fldPath.Child("values"), "must specify at least one exit code value"))
	}
	if len(req.Values) > 255 {
		allErrs = append(allErrs, field.TooMany(fldPath.Child("values"), len(req.Values), 255))
	}

	// Check for 0 in values when operator is In (exit code 0 means success)
	if req.Operator == appsv1beta1.PodFailurePolicyOnExitCodesOpIn {
		for i, val := range req.Values {
			if val == 0 {
				allErrs = append(allErrs, field.Invalid(fldPath.Child("values").Index(i), val,
					"exit code 0 cannot be used with 'In' operator as it indicates success"))
			}
		}
	}

	// Check for duplicates
	seen := make(map[int32]bool)
	for i, val := range req.Values {
		if seen[val] {
			allErrs = append(allErrs, field.Duplicate(fldPath.Child("values").Index(i), val))
		}
		seen[val] = true
	}

	return allErrs
}

func validateBroadcastJobName(name string, prefix bool) (allErrs []string) {
	if !validateBroadcastJobNameRegex.MatchString(name) {
		allErrs = append(allErrs, validationutil.RegexError(validateBroadcastJobNameMsg, validBroadcastJobNameFmt, "example-com"))
	}
	if len(name) > broadcastJobNameMaxLen {
		allErrs = append(allErrs, validationutil.MaxLenError(broadcastJobNameMaxLen))
	}
	return allErrs
}

var _ admission.Handler = &BroadcastJobCreateUpdateHandler{}

// Handle handles admission requests.
func (h *BroadcastJobCreateUpdateHandler) Handle(ctx context.Context, req admission.Request) admission.Response {
	obj := &appsv1beta1.BroadcastJob{}

	if err := h.decodeObject(req, obj); err != nil {
		return admission.Errored(http.StatusBadRequest, err)
	}

	allowed, reason, err := h.validatingBroadcastJobFn(ctx, obj)
	if err != nil {
		return admission.Errored(http.StatusInternalServerError, err)
	}
	return admission.ValidationResponse(allowed, reason)
}

func (h *BroadcastJobCreateUpdateHandler) decodeObject(req admission.Request, obj *appsv1beta1.BroadcastJob) error {
	switch req.AdmissionRequest.Resource.Version {
	case appsv1beta1.GroupVersion.Version:
		if err := h.Decoder.Decode(req, obj); err != nil {
			return err
		}
	case appsv1alpha1.GroupVersion.Version:
		objv1alpha1 := &appsv1alpha1.BroadcastJob{}
		if err := h.Decoder.Decode(req, objv1alpha1); err != nil {
			return err
		}
		if err := objv1alpha1.ConvertTo(obj); err != nil {
			return fmt.Errorf("failed to convert v1alpha1->v1beta1: %v", err)
		}
	}
	return nil
}

//var _ inject.Client = &BroadcastJobCreateUpdateHandler{}
//
//// InjectClient injects the client into the BroadcastJobCreateUpdateHandler
//func (h *BroadcastJobCreateUpdateHandler) InjectClient(c client.Client) error {
//	h.Client = c
//	return nil
//}
