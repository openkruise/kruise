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

	admissionv1 "k8s.io/api/admission/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	appsv1beta1 "github.com/openkruise/kruise/apis/apps/v1beta1"
	"github.com/openkruise/kruise/pkg/features"
	utilfeature "github.com/openkruise/kruise/pkg/util/feature"
)

// WorkloadSpreadCreateUpdateHandler handles WorkloadSpread
type WorkloadSpreadCreateUpdateHandler struct {
	Client client.Client

	// Decoder decodes objects
	Decoder admission.Decoder
}

var _ admission.Handler = &WorkloadSpreadCreateUpdateHandler{}

// Handle handles admission requests.
func (h *WorkloadSpreadCreateUpdateHandler) Handle(ctx context.Context, req admission.Request) admission.Response {
	if !utilfeature.DefaultFeatureGate.Enabled(features.WorkloadSpread) {
		return admission.Errored(http.StatusForbidden, fmt.Errorf("feature-gate %s is not enabled", features.WorkloadSpread))
	}

	obj, err := h.decodeObject(req)
	if err != nil {
		return admission.Errored(http.StatusBadRequest, err)
	}

	switch req.AdmissionRequest.Operation {
	case admissionv1.Create:
		if allErrs := h.validatingWorkloadSpreadFn(obj); len(allErrs) > 0 {
			return admission.Errored(http.StatusBadRequest, allErrs.ToAggregate())
		}
	case admissionv1.Update:
		oldObj, err := h.decodeOldObject(req)
		if err != nil {
			return admission.Errored(http.StatusBadRequest, err)
		}

		validationErrorList := h.validatingWorkloadSpreadFn(obj)
		updateErrorList := validateWorkloadSpreadUpdate(obj, oldObj)
		if allErrs := append(validationErrorList, updateErrorList...); len(allErrs) > 0 {
			return admission.Errored(http.StatusBadRequest, allErrs.ToAggregate())
		}
	}

	return admission.ValidationResponse(true, "")
}

// decodeObject decodes both v1alpha1 and v1beta1 admission requests to *appsv1beta1.WorkloadSpread.
// For v1alpha1 requests the object is converted via the registered conversion functions.
func (h *WorkloadSpreadCreateUpdateHandler) decodeObject(req admission.Request) (*appsv1beta1.WorkloadSpread, error) {
	if req.AdmissionRequest.Resource.Version == appsv1beta1.GroupVersion.Version {
		obj := &appsv1beta1.WorkloadSpread{}
		if err := h.Decoder.Decode(req, obj); err != nil {
			return nil, err
		}
		return obj, nil
	}
	// v1alpha1 path: decode then convert to v1beta1
	alpha := &appsv1alpha1.WorkloadSpread{}
	if err := h.Decoder.Decode(req, alpha); err != nil {
		return nil, err
	}
	beta := &appsv1beta1.WorkloadSpread{}
	if err := alpha.ConvertTo(beta); err != nil {
		return nil, fmt.Errorf("failed to convert v1alpha1 WorkloadSpread to v1beta1: %v", err)
	}
	return beta, nil
}

// decodeOldObject decodes the OldObject from both v1alpha1 and v1beta1 requests.
func (h *WorkloadSpreadCreateUpdateHandler) decodeOldObject(req admission.Request) (*appsv1beta1.WorkloadSpread, error) {
	if req.AdmissionRequest.Resource.Version == appsv1beta1.GroupVersion.Version {
		obj := &appsv1beta1.WorkloadSpread{}
		if err := h.Decoder.DecodeRaw(req.AdmissionRequest.OldObject, obj); err != nil {
			return nil, err
		}
		return obj, nil
	}
	alpha := &appsv1alpha1.WorkloadSpread{}
	if err := h.Decoder.DecodeRaw(req.AdmissionRequest.OldObject, alpha); err != nil {
		return nil, err
	}
	beta := &appsv1beta1.WorkloadSpread{}
	if err := alpha.ConvertTo(beta); err != nil {
		return nil, fmt.Errorf("failed to convert old v1alpha1 WorkloadSpread to v1beta1: %v", err)
	}
	return beta, nil
}
