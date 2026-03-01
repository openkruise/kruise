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

	admissionv1 "k8s.io/api/admission/v1"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	"github.com/openkruise/kruise/pkg/util"
	"github.com/openkruise/kruise/pkg/webhook/util/deletionprotection"
)

// UnitedDeploymentCreateUpdateHandler handles UnitedDeployment
type UnitedDeploymentCreateUpdateHandler struct {
	// To use the client, you need to do the following:
	// - uncomment it
	// - import sigs.k8s.io/controller-runtime/pkg/client
	// - uncomment the InjectClient method at the bottom of this file.
	// Client  client.Client

	// Decoder decodes objects
	Decoder admission.Decoder
}

var _ admission.Handler = &UnitedDeploymentCreateUpdateHandler{}

// Handle handles admission requests.
func (h *UnitedDeploymentCreateUpdateHandler) Handle(ctx context.Context, req admission.Request) admission.Response {
	obj := &appsv1alpha1.UnitedDeployment{}
	oldObj := &appsv1alpha1.UnitedDeployment{}

	switch req.AdmissionRequest.Operation {
	case admissionv1.Create:
		if err := h.Decoder.Decode(req, obj); err != nil {
			return admission.Errored(http.StatusBadRequest, err)
		}
		if allErrs := validateUnitedDeployment(obj); len(allErrs) > 0 {
			return admission.Errored(http.StatusUnprocessableEntity, allErrs.ToAggregate())
		}
	case admissionv1.Update:
		if err := h.Decoder.Decode(req, obj); err != nil {
			return admission.Errored(http.StatusBadRequest, err)
		}
		if err := h.Decoder.DecodeRaw(req.AdmissionRequest.OldObject, oldObj); err != nil {
			return admission.Errored(http.StatusBadRequest, err)
		}

		validationErrorList := validateUnitedDeployment(obj)
		updateErrorList := ValidateUnitedDeploymentUpdate(obj, oldObj)
		if allErrs := append(validationErrorList, updateErrorList...); len(allErrs) > 0 {
			return admission.Errored(http.StatusUnprocessableEntity, allErrs.ToAggregate())
		}
	case admissionv1.Delete:
		if len(req.OldObject.Raw) == 0 {
			klog.InfoS("Skip to validate UnitedDeployment deletion for no old object, maybe because of Kubernetes version < 1.16", "namespace", req.Namespace, "name", req.Name)
			return admission.ValidationResponse(true, "")
		}
		if err := h.Decoder.DecodeRaw(req.AdmissionRequest.OldObject, oldObj); err != nil {
			return admission.Errored(http.StatusBadRequest, err)
		}
		if err := deletionprotection.ValidateWorkloadDeletion(oldObj, oldObj.Spec.Replicas); err != nil {
			deletionprotection.WorkloadDeletionProtectionMetrics.WithLabelValues(fmt.Sprintf("%s_%s_%s", req.Kind.Kind, oldObj.GetNamespace(), oldObj.GetName()), req.UserInfo.Username).Add(1)
			util.LoggerProtectionInfo(util.ProtectionEventDeletionProtection, req.Kind.Kind, oldObj.GetNamespace(), oldObj.GetName(), req.UserInfo.Username)
			return admission.Errored(http.StatusForbidden, err)
		}
	}

	return admission.ValidationResponse(true, "")
}
