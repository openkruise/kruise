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

	genericvalidation "k8s.io/apimachinery/pkg/api/validation"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	metavalidation "k8s.io/apimachinery/pkg/apis/meta/v1/validation"
	validationutil "k8s.io/apimachinery/pkg/util/validation"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"k8s.io/kubernetes/pkg/apis/core"
	corev1 "k8s.io/kubernetes/pkg/apis/core/v1"
	corevalidation "k8s.io/kubernetes/pkg/apis/core/validation"

	"sigs.k8s.io/controller-runtime/pkg/runtime/inject"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission/types"

	appsv1alpha1 "github.com/openkruise/kruise/pkg/apis/apps/v1alpha1"
)

func init() {
	webhookName := "validating-create-update-sidecarset"
	if HandlerMap[webhookName] == nil {
		HandlerMap[webhookName] = []admission.Handler{}
	}
	HandlerMap[webhookName] = append(HandlerMap[webhookName], &SidecarSetCreateUpdateHandler{})
}

const (
	sidecarSetNameMaxLen = 63
)

var (
	validateSidecarSetNameMsg   = "sidecarset name must consist of alphanumeric characters or '-'"
	validateSidecarSetNameRegex = regexp.MustCompile(validSidecarSetNameFmt)
	validSidecarSetNameFmt      = `^[a-zA-Z0-9\-]+$`
)

// SidecarSetCreateUpdateHandler handles SidecarSet
type SidecarSetCreateUpdateHandler struct {
	// To use the client, you need to do the following:
	// - uncomment it
	// - import sigs.k8s.io/controller-runtime/pkg/client
	// - uncomment the InjectClient method at the bottom of this file.
	// Client  client.Client

	// Decoder decodes objects
	Decoder types.Decoder
}

func (h *SidecarSetCreateUpdateHandler) validatingSidecarSetFn(ctx context.Context, obj *appsv1alpha1.SidecarSet) (bool, string, error) {
	allErrs := validateSidecarSet(obj)
	if len(allErrs) != 0 {
		return false, "", allErrs.ToAggregate()
	}
	return true, "allowed to be admitted", nil
}

func validateSidecarSet(obj *appsv1alpha1.SidecarSet) field.ErrorList {
	allErrs := genericvalidation.ValidateObjectMeta(&obj.ObjectMeta, false, validateSidecarSetName, field.NewPath("metadata"))
	allErrs = append(allErrs, validateSidecarSetSpec(obj, field.NewPath("spec"))...)
	return allErrs
}

func validateSidecarSetName(name string, prefix bool) (allErrs []string) {
	if !validateSidecarSetNameRegex.MatchString(name) {
		allErrs = append(allErrs, validationutil.RegexError(validateSidecarSetNameMsg, validSidecarSetNameFmt, "example-com"))
	}
	if len(name) > sidecarSetNameMaxLen {
		allErrs = append(allErrs, validationutil.MaxLenError(sidecarSetNameMaxLen))
	}
	return allErrs
}

func validateSidecarSetSpec(obj *appsv1alpha1.SidecarSet, fldPath *field.Path) field.ErrorList {
	spec := &obj.Spec
	allErrs := field.ErrorList{}

	if spec.Selector == nil {
		allErrs = append(allErrs, field.Required(fldPath.Child("selector"), "no selector defined for sidecarset"))
	} else {
		allErrs = append(allErrs, metavalidation.ValidateLabelSelector(spec.Selector, fldPath.Child("selector"))...)
		if len(spec.Selector.MatchLabels)+len(spec.Selector.MatchExpressions) == 0 {
			allErrs = append(allErrs, field.Invalid(fldPath.Child("selector"), spec.Selector, "empty selector is not valid for sidecarset."))
		}
	}

	allErrs = append(allErrs, validateContainersForSidecarSet(spec.Containers, nil, fldPath.Child("containers"))...)

	return allErrs
}

func validateContainersForSidecarSet(
	containers []appsv1alpha1.SidecarContainer, volumes map[string]core.VolumeSource, fldPath *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}

	coreContainers := []core.Container{}
	for _, container := range containers {
		coreContainer := core.Container{}
		if err := corev1.Convert_v1_Container_To_core_Container(&container.Container, &coreContainer, nil); err != nil {
			allErrs = append(allErrs, field.Invalid(fldPath.Root(), container.Container, fmt.Sprintf("Convert_v1_Container_To_core_Container failed: %v", err)))
			return allErrs
		}
		coreContainers = append(coreContainers, coreContainer)
	}

	// use fakePod to reuse unexported 'validateContainers' function
	fakePod := &core.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"},
		Spec: core.PodSpec{
			DNSPolicy:     core.DNSClusterFirst,
			RestartPolicy: core.RestartPolicyAlways,
			Containers:    coreContainers,
		},
	}
	allErrs = append(allErrs, corevalidation.ValidatePod(fakePod)...)

	return allErrs
}

var _ admission.Handler = &SidecarSetCreateUpdateHandler{}

// Handle handles admission requests.
func (h *SidecarSetCreateUpdateHandler) Handle(ctx context.Context, req types.Request) types.Response {
	obj := &appsv1alpha1.SidecarSet{}

	err := h.Decoder.Decode(req, obj)
	if err != nil {
		return admission.ErrorResponse(http.StatusBadRequest, err)
	}

	allowed, reason, err := h.validatingSidecarSetFn(ctx, obj)
	if err != nil {
		return admission.ErrorResponse(http.StatusInternalServerError, err)
	}
	return admission.ValidationResponse(allowed, reason)
}

//var _ inject.Client = &SidecarSetCreateUpdateHandler{}
//
//// InjectClient injects the client into the SidecarSetCreateUpdateHandler
//func (h *SidecarSetCreateUpdateHandler) InjectClient(c client.Client) error {
//	h.Client = c
//	return nil
//}

var _ inject.Decoder = &SidecarSetCreateUpdateHandler{}

// InjectDecoder injects the decoder into the SidecarSetCreateUpdateHandler
func (h *SidecarSetCreateUpdateHandler) InjectDecoder(d types.Decoder) error {
	h.Decoder = d
	return nil
}
