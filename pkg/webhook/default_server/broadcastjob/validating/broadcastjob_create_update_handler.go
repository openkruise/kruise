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

	appsv1alpha1 "github.com/openkruise/kruise/pkg/apis/apps/v1alpha1"
	v1 "k8s.io/api/core/v1"
	genericvalidation "k8s.io/apimachinery/pkg/api/validation"
	validationutil "k8s.io/apimachinery/pkg/util/validation"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"k8s.io/kubernetes/pkg/apis/core"
	corev1 "k8s.io/kubernetes/pkg/apis/core/v1"
	corevalidation "k8s.io/kubernetes/pkg/apis/core/validation"
	"sigs.k8s.io/controller-runtime/pkg/runtime/inject"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission/types"
)

func init() {
	webhookName := "validating-create-update-broadcastjob"
	if HandlerMap[webhookName] == nil {
		HandlerMap[webhookName] = []admission.Handler{}
	}
	HandlerMap[webhookName] = append(HandlerMap[webhookName], &BroadcastJobCreateUpdateHandler{})
}

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
	Decoder types.Decoder
}

func (h *BroadcastJobCreateUpdateHandler) validatingBroadcastJobFn(ctx context.Context, obj *appsv1alpha1.BroadcastJob) (bool, string, error) {

	allErrs := validateBroadcastJob(obj)
	if len(allErrs) != 0 {
		return false, "", allErrs.ToAggregate()
	}
	return true, "allowed to be admitted", nil
}

func validateBroadcastJob(obj *appsv1alpha1.BroadcastJob) field.ErrorList {
	allErrs := genericvalidation.ValidateObjectMeta(&obj.ObjectMeta, true, validateBroadcastJobName, field.NewPath("metadata"))
	allErrs = append(allErrs, validateBroadcastJobSpec(&obj.Spec, field.NewPath("spec"))...)
	return allErrs
}

func validateBroadcastJobSpec(spec *appsv1alpha1.BroadcastJobSpec, fldPath *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}

	switch spec.CompletionPolicy.Type {
	case appsv1alpha1.Always:

	case appsv1alpha1.Never:
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
		if spec.CompletionPolicy.BackoffLimit != nil {
			allErrs = append(allErrs, field.Invalid(fldPath.Child("completionPolicy").Child("backoffLimit"),
				spec.CompletionPolicy.BackoffLimit,
				"backoffLimit can just work with Always CompletionPolicyType"))
		}
	default:
	}
	coreTemplate, err := convertPodTemplateSpec(&spec.Template)
	if err != nil {
		allErrs = append(allErrs, field.Invalid(fldPath.Root(), spec.Template, fmt.Sprintf("Convert_v1_PodTemplateSpec_To_core_PodTemplateSpec failed: %v", err)))
		return allErrs
	}
	allErrs = append(allErrs, corevalidation.ValidatePodTemplateSpec(coreTemplate, fldPath.Child("template"))...)
	if spec.Template.Spec.RestartPolicy != v1.RestartPolicyOnFailure &&
		spec.Template.Spec.RestartPolicy != v1.RestartPolicyNever {
		allErrs = append(allErrs, field.Invalid(fldPath.Child("template").Child("spec").Child("restartPolicy"),
			spec.Template.Spec.RestartPolicy,
			"pod restartPolicy can only be Never or OnFailure"))
	}
	return allErrs
}

func convertPodTemplateSpec(template *v1.PodTemplateSpec) (*core.PodTemplateSpec, error) {
	coreTemplate := &core.PodTemplateSpec{}
	if err := corev1.Convert_v1_PodTemplateSpec_To_core_PodTemplateSpec(template.DeepCopy(), coreTemplate, nil); err != nil {
		return nil, err
	}
	return coreTemplate, nil
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
func (h *BroadcastJobCreateUpdateHandler) Handle(ctx context.Context, req types.Request) types.Response {
	obj := &appsv1alpha1.BroadcastJob{}

	err := h.Decoder.Decode(req, obj)
	if err != nil {
		return admission.ErrorResponse(http.StatusBadRequest, err)
	}

	allowed, reason, err := h.validatingBroadcastJobFn(ctx, obj)
	if err != nil {
		return admission.ErrorResponse(http.StatusInternalServerError, err)
	}
	return admission.ValidationResponse(allowed, reason)
}

//var _ inject.Client = &BroadcastJobCreateUpdateHandler{}
//
//// InjectClient injects the client into the BroadcastJobCreateUpdateHandler
//func (h *BroadcastJobCreateUpdateHandler) InjectClient(c client.Client) error {
//	h.Client = c
//	return nil
//}

var _ inject.Decoder = &BroadcastJobCreateUpdateHandler{}

// InjectDecoder injects the decoder into the BroadcastJobCreateUpdateHandler
func (h *BroadcastJobCreateUpdateHandler) InjectDecoder(d types.Decoder) error {
	h.Decoder = d
	return nil
}
