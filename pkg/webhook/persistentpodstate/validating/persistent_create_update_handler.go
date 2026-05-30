/*
Copyright 2022 The Kruise Authors.

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

	admissionv1 "k8s.io/api/admission/v1"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	appsv1beta1 "github.com/openkruise/kruise/apis/apps/v1beta1"
	"github.com/openkruise/kruise/pkg/util"
	"github.com/openkruise/kruise/pkg/util/configuration"
)

// PersistentPodStateCreateUpdateHandler handles PersistentPodState
type PersistentPodStateCreateUpdateHandler struct {
	Client client.Client

	Decoder admission.Decoder
}

var _ admission.Handler = &PersistentPodStateCreateUpdateHandler{}

// Handle handles admission requests.
func (h *PersistentPodStateCreateUpdateHandler) Handle(ctx context.Context, req admission.Request) admission.Response {
	obj, err := h.decodeObject(req)
	if err != nil {
		return admission.Errored(http.StatusBadRequest, err)
	}

	var old *appsv1beta1.PersistentPodState
	if req.AdmissionRequest.Operation == admissionv1.Update {
		old, err = h.decodeOldObject(req)
		if err != nil {
			return admission.Errored(http.StatusBadRequest, err)
		}
	}

	allErrs := h.validatingPersistentPodStateFn(obj, old)
	if len(allErrs) != 0 {
		return admission.Errored(http.StatusBadRequest, allErrs.ToAggregate())
	}
	return admission.ValidationResponse(true, "")
}

func (h *PersistentPodStateCreateUpdateHandler) decodeObject(req admission.Request) (*appsv1beta1.PersistentPodState, error) {
	switch req.AdmissionRequest.Resource.Version {
	case appsv1beta1.GroupVersion.Version:
		obj := &appsv1beta1.PersistentPodState{}
		if err := h.Decoder.Decode(req, obj); err != nil {
			return nil, err
		}
		return obj, nil
	case appsv1alpha1.GroupVersion.Version:
		alpha := &appsv1alpha1.PersistentPodState{}
		if err := h.Decoder.Decode(req, alpha); err != nil {
			return nil, err
		}
		beta := &appsv1beta1.PersistentPodState{}
		if err := alpha.ConvertTo(beta); err != nil {
			return nil, fmt.Errorf("failed to convert v1alpha1->v1beta1: %v", err)
		}
		return beta, nil
	default:
		return nil, fmt.Errorf("unsupported version: %s", req.AdmissionRequest.Resource.Version)
	}
}

func (h *PersistentPodStateCreateUpdateHandler) decodeOldObject(req admission.Request) (*appsv1beta1.PersistentPodState, error) {
	switch req.AdmissionRequest.Resource.Version {
	case appsv1beta1.GroupVersion.Version:
		obj := &appsv1beta1.PersistentPodState{}
		if err := h.Decoder.DecodeRaw(req.AdmissionRequest.OldObject, obj); err != nil {
			return nil, err
		}
		return obj, nil
	case appsv1alpha1.GroupVersion.Version:
		alpha := &appsv1alpha1.PersistentPodState{}
		if err := h.Decoder.DecodeRaw(req.AdmissionRequest.OldObject, alpha); err != nil {
			return nil, err
		}
		beta := &appsv1beta1.PersistentPodState{}
		if err := alpha.ConvertTo(beta); err != nil {
			return nil, fmt.Errorf("failed to convert v1alpha1->v1beta1: %v", err)
		}
		return beta, nil
	default:
		return nil, fmt.Errorf("unsupported version: %s", req.AdmissionRequest.Resource.Version)
	}
}

func (h *PersistentPodStateCreateUpdateHandler) validatingPersistentPodStateFn(obj, old *appsv1beta1.PersistentPodState) field.ErrorList {
	allErrs := field.ErrorList{}
	whiteList, err := configuration.GetPPSWatchCustomWorkloadWhiteList(h.Client)
	if err != nil {
		allErrs = append(allErrs, field.InternalError(field.NewPath(""), fmt.Errorf("failed to get persistent pod state config white list, error: %v", err)))
		return allErrs
	}
	allErrs = append(allErrs, validatePersistentPodStateSpec(obj, field.NewPath("spec"), whiteList)...)
	if old != nil {
		allErrs = append(allErrs, validateUpdateObjImmutable(obj, old, field.NewPath("spec"))...)
	}
	ppsList := &appsv1beta1.PersistentPodStateList{}
	if err := h.Client.List(context.TODO(), ppsList, &client.ListOptions{Namespace: obj.Namespace}); err != nil {
		allErrs = append(allErrs, field.InternalError(field.NewPath(""), fmt.Errorf("query other PersistentPodState failed, err: %v", err)))
	} else {
		allErrs = append(allErrs, validatePerConflict(obj, ppsList.Items, field.NewPath("spec"))...)
	}
	return allErrs
}

func validateUpdateObjImmutable(obj, old *appsv1beta1.PersistentPodState, fldPath *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}
	if !reflect.DeepEqual(obj.Spec.TargetReference, old.Spec.TargetReference) {
		allErrs = append(allErrs, field.Forbidden(fldPath.Child("targetRef"), "targetRef is immutable"))
	}
	return allErrs
}

func validatePersistentPodStateSpec(obj *appsv1beta1.PersistentPodState, fldPath *field.Path, whiteList *configuration.CustomWorkloadWhiteList) field.ErrorList {
	spec := &obj.Spec
	allErrs := field.ErrorList{}
	if spec.TargetReference.APIVersion == "" || spec.TargetReference.Name == "" || spec.TargetReference.Kind == "" {
		allErrs = append(allErrs, field.Invalid(fldPath.Child("targetRef"), spec.TargetReference, "empty targetRef is not valid for PersistentPodState."))
	}

	apiVersion, kind := spec.TargetReference.APIVersion, spec.TargetReference.Kind
	if !whiteList.ValidateAPIVersionAndKind(apiVersion, kind) {
		allErrs = append(allErrs, field.Invalid(fldPath.Child("targetRef"), spec.TargetReference, "targetRef.Kind must be StatefulSet or in PPS_Watch_Custom_Workload_WhiteList"))
	}

	if spec.RequiredPersistentTopology == nil && len(spec.PreferredPersistentTopology) == 0 {
		allErrs = append(allErrs, field.Invalid(fldPath, spec, "TopologyConstraint and TopologyPreference cannot be empty at the same time"))
	}

	return allErrs
}

func validatePerConflict(pps *appsv1beta1.PersistentPodState, others []appsv1beta1.PersistentPodState, fldPath *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}

	for _, other := range others {
		if pps.Name == other.Name {
			continue
		}
		curRef := pps.Spec.TargetReference
		otherRef := other.Spec.TargetReference
		if util.IsReferenceEqualV1beta1(curRef, otherRef) {
			allErrs = append(allErrs, field.Invalid(fldPath.Child("targetRef"), pps.Spec.TargetReference, fmt.Sprintf(
				"targetRef is in conflict with other PersistentPodState %s", other.Name)))
			return allErrs
		}
	}
	return allErrs
}
