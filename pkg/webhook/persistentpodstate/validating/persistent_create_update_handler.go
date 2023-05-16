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

	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	"github.com/openkruise/kruise/pkg/util/configuration"
	admissionv1 "k8s.io/api/admission/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/runtime/inject"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// PersistentPodStateCreateUpdateHandler handles PersistentPodState
type PersistentPodStateCreateUpdateHandler struct {
	// To use the client, you need to do the following:
	// - uncomment it
	// - import sigs.k8s.io/controller-runtime/pkg/client
	// - uncomment the InjectClient method at the bottom of this file.
	Client client.Client

	// Decoder decodes objects
	Decoder *admission.Decoder
}

var _ admission.Handler = &PersistentPodStateCreateUpdateHandler{}

// Handle handles admission requests.
func (h *PersistentPodStateCreateUpdateHandler) Handle(ctx context.Context, req admission.Request) admission.Response {
	obj := &appsv1alpha1.PersistentPodState{}
	err := h.Decoder.Decode(req, obj)
	if err != nil {
		return admission.Errored(http.StatusBadRequest, err)
	}
	var old *appsv1alpha1.PersistentPodState
	//when Operation is update, decode older object
	if req.AdmissionRequest.Operation == admissionv1.Update {
		old = new(appsv1alpha1.PersistentPodState)
		if err := h.Decoder.Decode(
			admission.Request{AdmissionRequest: admissionv1.AdmissionRequest{Object: req.AdmissionRequest.OldObject}},
			old); err != nil {
			return admission.Errored(http.StatusBadRequest, err)
		}
	}
	allErrs := h.validatingPersistentPodStateFn(obj, old)
	if len(allErrs) != 0 {
		return admission.Errored(http.StatusBadRequest, allErrs.ToAggregate())
	}
	return admission.ValidationResponse(true, "")
}

func (h *PersistentPodStateCreateUpdateHandler) validatingPersistentPodStateFn(obj, old *appsv1alpha1.PersistentPodState) field.ErrorList {
	//validate pps.Spec
	allErrs := field.ErrorList{}
	whiteList, err := configuration.GetPPSWatchCustomWorkloadWhiteList(h.Client)
	if err != nil {
		allErrs = append(allErrs, field.InternalError(field.NewPath(""), fmt.Errorf("failed to get persistent pod state config white list, error: %v", err)))
		return allErrs
	}
	errs := validatePersistentPodStateSpec(obj, field.NewPath("spec"), whiteList)
	if len(errs) != 0 {
		allErrs = append(allErrs, errs...)
	}
	// when operation is update, validating whether old and new pps conflict
	if old != nil {
		allErrs = append(allErrs, validateUpdateObjImmutable(obj, old, field.NewPath("spec"))...)
	}
	//validate whether pps is in conflict with others
	ppsList := &appsv1alpha1.PersistentPodStateList{}
	if err := h.Client.List(context.TODO(), ppsList, &client.ListOptions{Namespace: obj.Namespace}); err != nil {
		allErrs = append(allErrs, field.InternalError(field.NewPath(""), fmt.Errorf("query other PersistentPodState failed, err: %v", err)))
	} else {
		allErrs = append(allErrs, validatePerConflict(obj, ppsList.Items, field.NewPath("spec"))...)
	}
	return allErrs
}

func validateUpdateObjImmutable(obj, old *appsv1alpha1.PersistentPodState, fldPath *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}
	// targetRef can't be changed
	if !reflect.DeepEqual(obj.Spec.TargetReference, old.Spec.TargetReference) {
		allErrs = append(allErrs, field.Required(fldPath.Child("targetRef"), "targetRef cannot be modified"))
	}
	return allErrs
}

func validatePersistentPodStateSpec(obj *appsv1alpha1.PersistentPodState, fldPath *field.Path, whiteList *configuration.CustomWorkloadWhiteList) field.ErrorList {
	spec := &obj.Spec
	allErrs := field.ErrorList{}
	// targetRef
	if spec.TargetReference.APIVersion == "" || spec.TargetReference.Name == "" || spec.TargetReference.Kind == "" {
		allErrs = append(allErrs, field.Invalid(fldPath.Child("TargetReference"), spec.TargetReference, "empty TargetReference is not valid for PersistentPodState."))
	}

	apiVersion, kind := spec.TargetReference.APIVersion, spec.TargetReference.Kind
	if !whiteList.ValidateAPIVersionAndKind(apiVersion, kind) {
		allErrs = append(allErrs, field.Invalid(fldPath.Child("TargetReference"), spec.TargetReference, "TargetReference.Kind must be StatefulSet or in PPS_Watch_Custom_Workload_WhiteList"))
	}

	if spec.RequiredPersistentTopology == nil && len(spec.PreferredPersistentTopology) == 0 {
		allErrs = append(allErrs, field.Invalid(fldPath, spec, "TopologyConstraint and TopologyPreference cannot be empty at the same time"))
	}

	return allErrs
}

func validatePerConflict(pps *appsv1alpha1.PersistentPodState, others []appsv1alpha1.PersistentPodState, fldPath *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}

	for _, other := range others {
		if pps.Name == other.Name {
			continue
		}
		// pod cannot be controlled by multiple ppss
		curRef := pps.Spec.TargetReference
		otherRef := other.Spec.TargetReference
		// The previous has been verified, there is no possibility of error here
		curGv, _ := schema.ParseGroupVersion(curRef.APIVersion)
		otherGv, _ := schema.ParseGroupVersion(otherRef.APIVersion)
		if curGv.Group == otherGv.Group && curRef.Kind == otherRef.Kind && curRef.Name == otherRef.Name {
			allErrs = append(allErrs, field.Invalid(fldPath.Child("targetReference"), pps.Spec.TargetReference, fmt.Sprintf(
				"targetReference is in conflict with other PersistentPodState %s", other.Name)))
			return allErrs
		}
	}
	return allErrs
}

var _ inject.Client = &PersistentPodStateCreateUpdateHandler{}

// InjectClient injects the client into the PersistentPodStateCreateUpdateHandler
func (h *PersistentPodStateCreateUpdateHandler) InjectClient(c client.Client) error {
	h.Client = c
	return nil
}

var _ admission.DecoderInjector = &PersistentPodStateCreateUpdateHandler{}

// InjectDecoder injects the decoder into the PersistentPodStateCreateUpdateHandler
func (h *PersistentPodStateCreateUpdateHandler) InjectDecoder(d *admission.Decoder) error {
	h.Decoder = d
	return nil
}
