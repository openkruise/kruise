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
	"strconv"
	admissionv1 "k8s.io/api/admission/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	metavalidation "k8s.io/apimachinery/pkg/apis/meta/v1/validation"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/validation/field"
	appsvalidation "k8s.io/kubernetes/pkg/apis/apps/validation"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	policyv1alpha1 "github.com/openkruise/kruise/apis/policy/v1alpha1"
	policyv1beta1 "github.com/openkruise/kruise/apis/policy/v1beta1"
	"github.com/openkruise/kruise/pkg/features"
	"github.com/openkruise/kruise/pkg/util"
	utilfeature "github.com/openkruise/kruise/pkg/util/feature"
)

// PodUnavailableBudgetCreateUpdateHandler handles PodUnavailableBudget
type PodUnavailableBudgetCreateUpdateHandler struct {
	Client  client.Client
	Decoder admission.Decoder
}

var _ admission.Handler = &PodUnavailableBudgetCreateUpdateHandler{}

// Handle handles admission requests.
func (h *PodUnavailableBudgetCreateUpdateHandler) Handle(ctx context.Context, req admission.Request) admission.Response {
	if !utilfeature.DefaultFeatureGate.Enabled(features.PodUnavailableBudgetDeleteGate) && !utilfeature.DefaultFeatureGate.Enabled(features.PodUnavailableBudgetUpdateGate) {
		return admission.Errored(http.StatusBadRequest, fmt.Errorf("feature PodUnavailableBudget is invalid, please open via feature-gate(%s, %s)",
			features.PodUnavailableBudgetDeleteGate, features.PodUnavailableBudgetUpdateGate))
	}

	switch req.AdmissionRequest.Resource.Version {
	case policyv1beta1.GroupVersion.Version:
		obj := &policyv1beta1.PodUnavailableBudget{}
		oldObj := &policyv1beta1.PodUnavailableBudget{}
		switch req.AdmissionRequest.Operation {
		case admissionv1.Create:
			if err := h.Decoder.Decode(req, obj); err != nil {
				return admission.Errored(http.StatusBadRequest, err)
			}
			if allErrs := h.validatingPodUnavailableBudgetFnV1beta1(obj, nil); len(allErrs) > 0 {
				return admission.Errored(http.StatusBadRequest, allErrs.ToAggregate())
			}
		case admissionv1.Update:
			if err := h.Decoder.Decode(req, obj); err != nil {
				return admission.Errored(http.StatusBadRequest, err)
			}
			if err := h.Decoder.DecodeRaw(req.AdmissionRequest.OldObject, oldObj); err != nil {
				return admission.Errored(http.StatusBadRequest, err)
			}
			if allErrs := h.validatingPodUnavailableBudgetFnV1beta1(obj, oldObj); len(allErrs) > 0 {
				return admission.Errored(http.StatusBadRequest, allErrs.ToAggregate())
			}
		}
		return admission.ValidationResponse(true, "")

	case policyv1alpha1.GroupVersion.Version:
		alphaObj := &policyv1alpha1.PodUnavailableBudget{}
		alphaOldObj := &policyv1alpha1.PodUnavailableBudget{}
		switch req.AdmissionRequest.Operation {
		case admissionv1.Create:
			if err := h.Decoder.Decode(req, alphaObj); err != nil {
				return admission.Errored(http.StatusBadRequest, err)
			}
			obj := &policyv1beta1.PodUnavailableBudget{}
			if err := alphaObj.ConvertTo(obj); err != nil {
				return admission.Errored(http.StatusBadRequest, fmt.Errorf("failed to convert v1alpha1 PUB to v1beta1: %w", err))
			}
			if allErrs := h.validatingPodUnavailableBudgetFnV1beta1(obj, nil); len(allErrs) > 0 {
				return admission.Errored(http.StatusBadRequest, allErrs.ToAggregate())
			}
		case admissionv1.Update:
			if err := h.Decoder.Decode(req, alphaObj); err != nil {
				return admission.Errored(http.StatusBadRequest, err)
			}
			if err := h.Decoder.DecodeRaw(req.AdmissionRequest.OldObject, alphaOldObj); err != nil {
				return admission.Errored(http.StatusBadRequest, err)
			}
			obj := &policyv1beta1.PodUnavailableBudget{}
			if err := alphaObj.ConvertTo(obj); err != nil {
				return admission.Errored(http.StatusBadRequest, fmt.Errorf("failed to convert v1alpha1 PUB to v1beta1: %w", err))
			}
			oldObj := &policyv1beta1.PodUnavailableBudget{}
			if err := alphaOldObj.ConvertTo(oldObj); err != nil {
				return admission.Errored(http.StatusBadRequest, fmt.Errorf("failed to convert old v1alpha1 PUB to v1beta1: %w", err))
			}
			if allErrs := h.validatingPodUnavailableBudgetFnV1beta1(obj, oldObj); len(allErrs) > 0 {
				return admission.Errored(http.StatusBadRequest, allErrs.ToAggregate())
			}
		}
		return admission.ValidationResponse(true, "")
	}

	return admission.Errored(http.StatusBadRequest, fmt.Errorf("unsupported version: %s", req.AdmissionRequest.Resource.Version))
}

func (h *PodUnavailableBudgetCreateUpdateHandler) validatingPodUnavailableBudgetFnV1beta1(obj, old *policyv1beta1.PodUnavailableBudget) field.ErrorList {
	allErrs := field.ErrorList{}
	if replicasValue, ok := obj.Annotations[policyv1beta1.PubProtectTotalReplicasAnnotation]; ok {
		if _, err := strconv.ParseInt(replicasValue, 10, 32); err != nil {
			allErrs = append(allErrs, field.Invalid(field.NewPath("metadata", "annotations").Key(policyv1beta1.PubProtectTotalReplicasAnnotation), replicasValue, "must be a valid int32"))
		} else if obj.Spec.ProtectTotalReplicas != nil && replicasValue != strconv.FormatInt(int64(*obj.Spec.ProtectTotalReplicas), 10) {
			allErrs = append(allErrs, field.Invalid(field.NewPath("metadata", "annotations").Key(policyv1beta1.PubProtectTotalReplicasAnnotation), replicasValue, "must match spec.protectTotalReplicas when both are set"))
		}
	}

	allErrs = append(allErrs, validatePodUnavailableBudgetSpecV1beta1(obj, field.NewPath("spec"))...)
	if old != nil {
		allErrs = append(allErrs, validateUpdatePubConflictV1beta1(obj, old, field.NewPath("spec"))...)
	}

	pubList := &policyv1beta1.PodUnavailableBudgetList{}
	if err := h.Client.List(context.TODO(), pubList, &client.ListOptions{Namespace: obj.Namespace}); err != nil {
		allErrs = append(allErrs, field.InternalError(field.NewPath(""), fmt.Errorf("query other podUnavailableBudget failed, err: %v", err)))
	} else {
		allErrs = append(allErrs, validatePubConflictV1beta1(obj, pubList.Items, field.NewPath("spec"))...)
	}
	return allErrs
}

func validateUpdatePubConflictV1beta1(obj, old *policyv1beta1.PodUnavailableBudget, fldPath *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}
	if !reflect.DeepEqual(obj.Spec.Selector, old.Spec.Selector) || !reflect.DeepEqual(obj.Spec.TargetReference, old.Spec.TargetReference) {
		allErrs = append(allErrs, field.Required(fldPath.Child("selector, targetRef"), "selector and targetRef cannot be modified"))
	}
	return allErrs
}

func validatePodUnavailableBudgetSpecV1beta1(obj *policyv1beta1.PodUnavailableBudget, fldPath *field.Path) field.ErrorList {
	spec := &obj.Spec
	allErrs := field.ErrorList{}

	if spec.Selector == nil && spec.TargetReference == nil {
		allErrs = append(allErrs, field.Required(fldPath.Child("selector, targetRef"), "no selector or targetRef defined in PodUnavailableBudget"))
	} else if spec.Selector != nil && spec.TargetReference != nil {
		allErrs = append(allErrs, field.Required(fldPath.Child("selector, targetRef"), "selector and targetRef are mutually exclusive"))
	} else if spec.TargetReference != nil {
		if spec.TargetReference.APIVersion == "" || spec.TargetReference.Name == "" || spec.TargetReference.Kind == "" {
			allErrs = append(allErrs, field.Invalid(fldPath.Child("targetRef"), spec.TargetReference, "empty targetRef is not valid for PodUnavailableBudget"))
		}
		_, err := schema.ParseGroupVersion(spec.TargetReference.APIVersion)
		if err != nil {
			allErrs = append(allErrs, field.Invalid(fldPath.Child("targetRef"), spec.TargetReference, err.Error()))
		}
	} else {
		allErrs = append(allErrs, metavalidation.ValidateLabelSelector(spec.Selector, metavalidation.LabelSelectorValidationOptions{}, fldPath.Child("selector"))...)
		if len(spec.Selector.MatchLabels)+len(spec.Selector.MatchExpressions) == 0 {
			allErrs = append(allErrs, field.Invalid(fldPath.Child("selector"), spec.Selector, "empty selector is not valid for PodUnavailableBudget."))
		}
		_, err := metav1.LabelSelectorAsSelector(spec.Selector)
		if err != nil {
			allErrs = append(allErrs, field.Invalid(fldPath.Child("selector"), spec.Selector, ""))
		}
	}

	if spec.MaxUnavailable == nil && spec.MinAvailable == nil {
		allErrs = append(allErrs, field.Required(fldPath.Child("maxUnavailable, minAvailable"), "no maxUnavailable or minAvailable defined in PodUnavailableBudget"))
	} else if spec.MaxUnavailable != nil && spec.MinAvailable != nil {
		allErrs = append(allErrs, field.Required(fldPath.Child("maxUnavailable, minAvailable"), "maxUnavailable and minAvailable are mutually exclusive"))
	} else if spec.MaxUnavailable != nil {
		allErrs = append(allErrs, appsvalidation.ValidatePositiveIntOrPercent(*spec.MaxUnavailable, fldPath.Child("maxUnavailable"))...)
		allErrs = append(allErrs, appsvalidation.IsNotMoreThan100Percent(*spec.MaxUnavailable, fldPath.Child("maxUnavailable"))...)
	} else {
		allErrs = append(allErrs, appsvalidation.ValidatePositiveIntOrPercent(*spec.MinAvailable, fldPath.Child("minAvailable"))...)
		allErrs = append(allErrs, appsvalidation.IsNotMoreThan100Percent(*spec.MinAvailable, fldPath.Child("minAvailable"))...)
	}

	for i, operation := range spec.ProtectOperations {
		if operation != policyv1beta1.PubDeleteOperation && operation != policyv1beta1.PubUpdateOperation &&
			operation != policyv1beta1.PubEvictOperation && operation != policyv1beta1.PubResizeOperation {
			allErrs = append(allErrs, field.NotSupported(fldPath.Child("protectOperations").Index(i), operation, []string{
				string(policyv1beta1.PubDeleteOperation),
				string(policyv1beta1.PubUpdateOperation),
				string(policyv1beta1.PubEvictOperation),
				string(policyv1beta1.PubResizeOperation),
			}))
		}
	}

	if spec.IgnoredPodSelector != nil {
		allErrs = append(allErrs, metavalidation.ValidateLabelSelector(spec.IgnoredPodSelector, metavalidation.LabelSelectorValidationOptions{}, fldPath.Child("ignoredPodSelector"))...)
		if len(spec.IgnoredPodSelector.MatchLabels)+len(spec.IgnoredPodSelector.MatchExpressions) == 0 {
			allErrs = append(allErrs, field.Invalid(fldPath.Child("ignoredPodSelector"), spec.IgnoredPodSelector, "empty ignoredPodSelector is not valid for PodUnavailableBudget"))
		}
	}
	return allErrs
}

func validatePubConflictV1beta1(pub *policyv1beta1.PodUnavailableBudget, others []policyv1beta1.PodUnavailableBudget, fldPath *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}

	for _, other := range others {
		if pub.Name == other.Name {
			continue
		}
		if pub.Spec.TargetReference != nil && other.Spec.TargetReference != nil {
			curRef := pub.Spec.TargetReference
			otherRef := other.Spec.TargetReference
			curGv, _ := schema.ParseGroupVersion(curRef.APIVersion)
			otherGv, _ := schema.ParseGroupVersion(otherRef.APIVersion)
			if curGv.Group == otherGv.Group && curRef.Kind == otherRef.Kind && curRef.Name == otherRef.Name {
				allErrs = append(allErrs, field.Invalid(fldPath.Child("targetReference"), pub.Spec.TargetReference, fmt.Sprintf(
					"pub.spec.targetReference is in conflict with other PodUnavailableBudget %s", other.Name)))
				return allErrs
			}
		} else if pub.Spec.Selector != nil && other.Spec.Selector != nil {
			if util.IsSelectorLooseOverlap(pub.Spec.Selector, other.Spec.Selector) {
				allErrs = append(allErrs, field.Invalid(fldPath.Child("selector"), pub.Spec.Selector, fmt.Sprintf(
					"pub.spec.selector is in conflict with other PodUnavailableBudget %s", other.Name)))
				return allErrs
			}
		}
	}
	return allErrs
}

