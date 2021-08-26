/*
Copyright 2021 The Kruise Authors.

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
	"reflect"

	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	"github.com/openkruise/kruise/pkg/util"

	admissionv1 "k8s.io/api/admission/v1"
	"k8s.io/apimachinery/pkg/api/validation"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	metav1validation "k8s.io/apimachinery/pkg/apis/meta/v1/validation"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/runtime/inject"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

const (
	LimitedPreferencesNumber = 31
)

// PodMarkerCreateUpdateHandler handles PodMarker
type PodMarkerCreateUpdateHandler struct {
	Client client.Client

	// Decoder decodes objects
	Decoder *admission.Decoder
}

func validatePodMarkerCreate(marker *appsv1alpha1.PodMarker) field.ErrorList {
	// 1. validate metadata
	allErrs := validation.ValidateObjectMeta(&marker.ObjectMeta, true, validation.NameIsDNSSubdomain, field.NewPath("metadata"))
	// 2. validate spec
	return append(allErrs, validatePodMarkerSpec(&marker.Spec, field.NewPath("spec"))...)
}

func validatePodMarkerUpdate(marker, oldMarker *appsv1alpha1.PodMarker) (allErrs field.ErrorList) {
	oldMarker.Spec.Strategy = marker.Spec.Strategy
	if !reflect.DeepEqual(marker.Spec, oldMarker.Spec) {
		allErrs = append(allErrs, field.Invalid(field.NewPath("spec"), marker.Spec, "can not change PodMarker spec except strategy"))
	}
	return append(allErrs, validatePodMarkerStrategy(&marker.Spec.Strategy, field.NewPath("spec.Strategy"))...)
}

func validatePodMarkerSpec(spec *appsv1alpha1.PodMarkerSpec, fldPath *field.Path) field.ErrorList {
	// 1. validate Strategy
	allErrs := validatePodMarkerStrategy(&spec.Strategy, fldPath.Child("strategy"))
	// 2. validate MatchRequirements
	allErrs = append(allErrs, validatePodMarkerMatchRequirements(spec, fldPath.Child("matchRequirements"))...)
	// 3. validate MatchPreferences
	allErrs = append(allErrs, validatePodMarkerMatchPreferences(spec.MatchPreferences, fldPath.Child("matchPreferences"))...)
	// 4. validate MarkItems
	return append(allErrs, validatePodMarkerMarkIdentities(&spec.MarkItems, fldPath.Child("markIdentities"))...)
}

func validatePodMarkerStrategy(strategy *appsv1alpha1.PodMarkerStrategy, fldPath *field.Path) (allErrs field.ErrorList) {
	// 1.ConflictPolicy must be "", "Ignore", or "Overwrite"
	switch strategy.ConflictPolicy {
	case "", appsv1alpha1.PodMarkerConflictOverwrite, appsv1alpha1.PodMarkerConflictIgnore:
	default:
		allErrs = append(allErrs, field.Invalid(fldPath.Child("conflictPolicy"), strategy.ConflictPolicy,
			fmt.Sprintf("unsupported ConflictPolicy type: %s, ConflictPolicy should be Ignore or Overwrite", strategy.ConflictPolicy)))
	}
	// 2. Replicas must be nil, non-negative int, or a percent in [0%, 100%]
	if strategy.Replicas != nil {
		replicas, err := intstr.GetValueFromIntOrPercent(strategy.Replicas, 100, true)
		if err != nil {
			allErrs = append(allErrs, field.InternalError(fldPath.Child("replicas"), err))
		} else if replicas < 0 || replicas > 100 {
			allErrs = append(allErrs, field.Invalid(fldPath.Child("replicas"), *strategy.Replicas,
				"replicas must be a non-negative integer, or a percentage satisfying 0% <= replicas <= 100%"))
		}
	}
	return
}

func validatePodMarkerMatchRequirements(spec *appsv1alpha1.PodMarkerSpec, fldPath *field.Path) (allErrs field.ErrorList) {
	requirement := &spec.MatchRequirements
	if requirement.PodSelector == nil && requirement.NodeSelector == nil {
		return
	}

	// 1. validate podSelector
	_, err := v1.LabelSelectorAsSelector(requirement.PodSelector)
	if err != nil {
		allErrs = append(allErrs, field.Invalid(fldPath.Child("matchPodSelector"), requirement.PodSelector, fmt.Sprintf("invalid matchPodSelector, err: %v", err)))
	} else if util.IsDefinitelyNeverOverlapped(requirement.PodSelector, &v1.LabelSelector{MatchLabels: spec.MarkItems.Labels}) {
		// in cases of podSelector.MatchLabels = {"a":"b"}, markItems.Labels = {"a":"c"}
		allErrs = append(allErrs, field.Invalid(fldPath.Child("matchPodSelector"), *requirement.PodSelector,
			"matchPodSelector fights with spec.markItems, spec.markItems will make podSelector invalid after marking"))
	}
	// 2. validate nodeSelector
	_, err = v1.LabelSelectorAsSelector(requirement.NodeSelector)
	if err != nil {
		allErrs = append(allErrs, field.Invalid(fldPath.Child("matchNodeSelector"), requirement.NodeSelector, fmt.Sprintf("invalid matchNodeSelector, err: %v", err)))
	}

	return
}

func validatePodMarkerMatchPreferences(preferences []appsv1alpha1.PodMarkerPreference, fldPath *field.Path) (allErrs field.ErrorList) {
	if len(preferences) > LimitedPreferencesNumber {
		allErrs = append(allErrs, field.Invalid(fldPath, preferences,
			fmt.Sprintf("over the limitation of the number of MatchPreferences, limitation: %d, yours: %d", LimitedPreferencesNumber, len(preferences))))
	}
	for i := range preferences {
		preference := &preferences[i]
		// 1. validate podSelector
		if _, err := v1.LabelSelectorAsSelector(preference.PodSelector); err != nil {
			allErrs = append(allErrs, field.Invalid(fldPath.Child("matchPreferences").Index(i), *preference.PodSelector, fmt.Sprintf("invalid preferred podSelector, err: %v", err)))
		}
		// 2. validate nodeSelector
		if _, err := v1.LabelSelectorAsSelector(preference.NodeSelector); err != nil {
			allErrs = append(allErrs, field.Invalid(fldPath.Child("matchPreferences").Index(i), *preference.PodSelector, fmt.Sprintf("invalid preferred NodeSelector, err: %v", err)))
		}
	}
	return
}

func validatePodMarkerMarkIdentities(markItems *appsv1alpha1.PodMarkerMarkItems, fldPath *field.Path) (allErrs field.ErrorList) {
	// markItems cannot be empty
	if len(markItems.Labels)+len(markItems.Annotations) == 0 {
		return append(allErrs, field.Invalid(fldPath, *markItems, "markItems cannot be empty"))
	}
	// 1. validate the labels users want to mark
	allErrs = metav1validation.ValidateLabels(markItems.Labels, fldPath.Child("labels"))

	// 2. validate the annotations users want to mark
	return append(allErrs, validation.ValidateAnnotations(markItems.Annotations, fldPath.Child("annotations"))...)
}

var _ admission.Handler = &PodMarkerCreateUpdateHandler{}

// Handle handles admission requests.
func (h *PodMarkerCreateUpdateHandler) Handle(ctx context.Context, req admission.Request) admission.Response {
	marker := &appsv1alpha1.PodMarker{}
	if err := h.Decoder.Decode(req, marker); err != nil {
		return admission.Errored(http.StatusBadRequest, err)
	}

	var errList field.ErrorList
	switch req.AdmissionRequest.Operation {
	case admissionv1.Create:
		errList = validatePodMarkerCreate(marker)
	case admissionv1.Update:
		oldMarker := &appsv1alpha1.PodMarker{}
		if err := h.Decoder.DecodeRaw(req.OldObject, oldMarker); err != nil {
			return admission.Errored(http.StatusBadRequest, err)
		}
		errList = validatePodMarkerUpdate(marker, oldMarker)
	}

	if len(errList) != 0 {
		klog.Errorf("All errors about podMarker validation: %v", errList)
		return admission.Errored(http.StatusUnprocessableEntity, errList.ToAggregate())
	}
	return admission.ValidationResponse(true, "")
}

var _ inject.Client = &PodMarkerCreateUpdateHandler{}

// InjectClient injects the client into the PodMarkerCreateUpdateHandler
func (h *PodMarkerCreateUpdateHandler) InjectClient(c client.Client) error {
	h.Client = c
	return nil
}

var _ admission.DecoderInjector = &PodMarkerCreateUpdateHandler{}

// InjectDecoder injects the decoder into the PodMarkerCreateUpdateHandler
func (h *PodMarkerCreateUpdateHandler) InjectDecoder(d *admission.Decoder) error {
	h.Decoder = d
	return nil
}
