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
	"strings"

	policyv1alpha1 "github.com/openkruise/kruise/apis/policy/v1alpha1"
	"github.com/openkruise/kruise/pkg/features"
	"github.com/openkruise/kruise/pkg/util"
	utilfeature "github.com/openkruise/kruise/pkg/util/feature"

	admissionv1 "k8s.io/api/admission/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	metavalidation "k8s.io/apimachinery/pkg/apis/meta/v1/validation"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/validation/field"
	appsvalidation "k8s.io/kubernetes/pkg/apis/apps/validation"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// PodUnavailableBudgetCreateUpdateHandler handles PodUnavailableBudget
type PodUnavailableBudgetCreateUpdateHandler struct {
	// To use the client, you need to do the following:
	// - uncomment it
	// - import sigs.k8s.io/controller-runtime/pkg/client
	// - uncomment the InjectClient method at the bottom of this file.
	Client client.Client

	// Decoder decodes objects
	Decoder admission.Decoder
}

var _ admission.Handler = &PodUnavailableBudgetCreateUpdateHandler{}

// Handle handles admission requests.
func (h *PodUnavailableBudgetCreateUpdateHandler) Handle(ctx context.Context, req admission.Request) admission.Response {
	if !utilfeature.DefaultFeatureGate.Enabled(features.PodUnavailableBudgetDeleteGate) && !utilfeature.DefaultFeatureGate.Enabled(features.PodUnavailableBudgetUpdateGate) {
		return admission.Errored(http.StatusBadRequest, fmt.Errorf("feature PodUnavailableBudget is invalid, please open via feature-gate(%s, %s)",
			features.PodUnavailableBudgetDeleteGate, features.PodUnavailableBudgetUpdateGate))
	}

	obj := &policyv1alpha1.PodUnavailableBudget{}
	err := h.Decoder.Decode(req, obj)
	if err != nil {
		return admission.Errored(http.StatusBadRequest, err)
	}
	var old *policyv1alpha1.PodUnavailableBudget
	//when Operation is update, decode older object
	if req.AdmissionRequest.Operation == admissionv1.Update {
		old = new(policyv1alpha1.PodUnavailableBudget)
		if err := h.Decoder.Decode(
			admission.Request{AdmissionRequest: admissionv1.AdmissionRequest{Object: req.AdmissionRequest.OldObject}},
			old); err != nil {
			return admission.Errored(http.StatusBadRequest, err)
		}
	}
	allErrs := h.validatingPodUnavailableBudgetFn(obj, old)
	if len(allErrs) != 0 {
		return admission.Errored(http.StatusBadRequest, allErrs.ToAggregate())
	}
	return admission.ValidationResponse(true, "")
}

func (h *PodUnavailableBudgetCreateUpdateHandler) validatingPodUnavailableBudgetFn(obj, old *policyv1alpha1.PodUnavailableBudget) field.ErrorList {
	// validate pub.annotations
	allErrs := field.ErrorList{}
	if operationsValue, ok := obj.Annotations[policyv1alpha1.PubProtectOperationAnnotation]; ok {
		operations := strings.Split(operationsValue, ",")
		for _, operation := range operations {
			if operation != string(policyv1alpha1.PubUpdateOperation) && operation != string(policyv1alpha1.PubDeleteOperation) &&
				operation != string(policyv1alpha1.PubEvictOperation) {
				allErrs = append(allErrs, field.InternalError(field.NewPath("metadata"), fmt.Errorf("annotation[%s] is invalid", policyv1alpha1.PubProtectOperationAnnotation)))
			}
		}
	}
	if replicasValue, ok := obj.Annotations[policyv1alpha1.PubProtectTotalReplicasAnnotation]; ok {
		if _, err := strconv.ParseInt(replicasValue, 10, 32); err != nil {
			allErrs = append(allErrs, field.InternalError(field.NewPath("metadata"), fmt.Errorf("annotation[%s] is invalid", policyv1alpha1.PubProtectTotalReplicasAnnotation)))
		}
	}

	//validate Pub.Spec
	allErrs = append(allErrs, validatePodUnavailableBudgetSpec(obj, field.NewPath("spec"))...)
	// when operation is update, validating whether old and new pub conflict
	if old != nil {
		allErrs = append(allErrs, validateUpdatePubConflict(obj, old, field.NewPath("spec"))...)
	}
	//validate whether pub is in conflict with others
	pubList := &policyv1alpha1.PodUnavailableBudgetList{}
	if err := h.Client.List(context.TODO(), pubList, &client.ListOptions{Namespace: obj.Namespace}); err != nil {
		allErrs = append(allErrs, field.InternalError(field.NewPath(""), fmt.Errorf("query other podUnavailableBudget failed, err: %v", err)))
	} else {
		allErrs = append(allErrs, validatePubConflict(obj, pubList.Items, field.NewPath("spec"))...)
	}
	return allErrs
}

func validateUpdatePubConflict(obj, old *policyv1alpha1.PodUnavailableBudget, fldPath *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}
	// selector and targetRef can't be changed
	if !reflect.DeepEqual(obj.Spec.Selector, old.Spec.Selector) || !reflect.DeepEqual(obj.Spec.TargetReference, old.Spec.TargetReference) {
		allErrs = append(allErrs, field.Required(fldPath.Child("selector, targetRef"), "selector and targetRef cannot be modified"))
	}
	return allErrs
}

func validatePodUnavailableBudgetSpec(obj *policyv1alpha1.PodUnavailableBudget, fldPath *field.Path) field.ErrorList {
	spec := &obj.Spec
	allErrs := field.ErrorList{}

	if spec.Selector == nil && spec.TargetReference == nil {
		allErrs = append(allErrs, field.Required(fldPath.Child("selector, targetRef"), "no selector or targetRef defined in PodUnavailableBudget"))
	} else if spec.Selector != nil && spec.TargetReference != nil {
		allErrs = append(allErrs, field.Required(fldPath.Child("selector, targetRef"), "selector and targetRef are mutually exclusive"))
	} else if spec.TargetReference != nil {
		if spec.TargetReference.APIVersion == "" || spec.TargetReference.Name == "" || spec.TargetReference.Kind == "" {
			allErrs = append(allErrs, field.Invalid(fldPath.Child("TargetReference"), spec.TargetReference, "empty TargetReference is not valid for PodUnavailableBudget."))
		}
		_, err := schema.ParseGroupVersion(spec.TargetReference.APIVersion)
		if err != nil {
			allErrs = append(allErrs, field.Invalid(fldPath.Child("TargetReference"), spec.TargetReference, err.Error()))
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
	return allErrs
}

func validatePubConflict(pub *policyv1alpha1.PodUnavailableBudget, others []policyv1alpha1.PodUnavailableBudget, fldPath *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}

	for _, other := range others {
		if pub.Name == other.Name {
			continue
		}
		// pod cannot be controlled by multiple pubs
		if pub.Spec.TargetReference != nil && other.Spec.TargetReference != nil {
			curRef := pub.Spec.TargetReference
			otherRef := other.Spec.TargetReference
			// The previous has been verified, there is no possibility of error here
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
