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
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	appsv1beta1 "github.com/openkruise/kruise/apis/apps/v1beta1"
	"github.com/openkruise/kruise/pkg/features"
	"github.com/openkruise/kruise/pkg/util"
	utilfeature "github.com/openkruise/kruise/pkg/util/feature"
	"github.com/openkruise/kruise/pkg/webhook/util/deletionprotection"
)

// StatefulSetCreateUpdateHandler handles StatefulSet
type StatefulSetCreateUpdateHandler struct {
	// To use the client, you need to do the following:
	// - uncomment it
	// - import sigs.k8s.io/controller-runtime/pkg/client
	// - uncomment the InjectClient method at the bottom of this file.
	Client client.Client

	// Decoder decodes objects
	Decoder admission.Decoder
}

var _ admission.Handler = &StatefulSetCreateUpdateHandler{}

// Handle handles admission requests.
func (h *StatefulSetCreateUpdateHandler) Handle(ctx context.Context, req admission.Request) admission.Response {
	obj := &appsv1beta1.StatefulSet{}
	oldObj := &appsv1beta1.StatefulSet{}

	switch req.AdmissionRequest.Operation {
	case admissionv1.Create:
		if err := h.decodeObject(req, obj); err != nil {
			return admission.Errored(http.StatusBadRequest, err)
		}
		if allErrs := validateStatefulSet(obj); len(allErrs) > 0 {
			return admission.Errored(http.StatusUnprocessableEntity, allErrs.ToAggregate())
		}
	case admissionv1.Update:
		if err := h.decodeObject(req, obj); err != nil {
			return admission.Errored(http.StatusBadRequest, err)
		}
		if err := h.decodeOldObject(req, oldObj); err != nil {
			return admission.Errored(http.StatusBadRequest, err)
		}

		validationErrorList := validateStatefulSet(obj)
		updateErrorList := ValidateStatefulSetUpdate(obj, oldObj)
		if allErrs := append(validationErrorList, updateErrorList...); len(allErrs) > 0 {
			return admission.Errored(http.StatusUnprocessableEntity, allErrs.ToAggregate())
		}
		if utilfeature.DefaultFeatureGate.Enabled(features.StatefulSetAutoResizePVCGate) {
			vctUpdateErr := ValidateVolumeClaimTemplateUpdate(h.Client, obj, oldObj)
			if len(vctUpdateErr) > 0 {
				return admission.Errored(http.StatusUnprocessableEntity, vctUpdateErr.ToAggregate())
			}
		}

		if obj.Spec.UpdateStrategy.RollingUpdate != nil &&
			obj.Spec.UpdateStrategy.RollingUpdate.PodUpdatePolicy == appsv1beta1.InPlaceOnlyPodUpdateStrategyType {
			if err := validateTemplateInPlaceOnly(&oldObj.Spec.Template, &obj.Spec.Template); err != nil {
				return admission.Errored(http.StatusUnprocessableEntity,
					fmt.Errorf("invalid template modified with InPlaceOnly strategy: %v, currently only image update is allowed for InPlaceOnly", err))
			}
		}

	case admissionv1.Delete:
		if len(req.OldObject.Raw) == 0 {
			klog.InfoS("Skip to validate StatefulSet deletion for no old object, maybe because of Kubernetes version < 1.16", "namespace", req.Namespace, "name", req.Name)
			return admission.ValidationResponse(true, "")
		}
		if err := h.decodeOldObject(req, oldObj); err != nil {
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

func (h *StatefulSetCreateUpdateHandler) decodeObject(req admission.Request, obj *appsv1beta1.StatefulSet) error {
	switch req.AdmissionRequest.Resource.Version {
	case appsv1beta1.GroupVersion.Version:
		if err := h.Decoder.Decode(req, obj); err != nil {
			return err
		}
	case appsv1alpha1.GroupVersion.Version:
		objv1alpha1 := &appsv1alpha1.StatefulSet{}
		if err := h.Decoder.Decode(req, objv1alpha1); err != nil {
			return err
		}
		if err := objv1alpha1.ConvertTo(obj); err != nil {
			return fmt.Errorf("failed to convert v1alpha1->v1beta1: %v", err)
		}
	}
	return nil
}

func (h *StatefulSetCreateUpdateHandler) decodeOldObject(req admission.Request, oldObj *appsv1beta1.StatefulSet) error {
	switch req.AdmissionRequest.Resource.Version {
	case appsv1beta1.GroupVersion.Version:
		if err := h.Decoder.DecodeRaw(req.AdmissionRequest.OldObject, oldObj); err != nil {
			return err
		}
	case appsv1alpha1.GroupVersion.Version:
		objv1alpha1 := &appsv1alpha1.StatefulSet{}
		if err := h.Decoder.DecodeRaw(req.AdmissionRequest.OldObject, objv1alpha1); err != nil {
			return err
		}
		if err := objv1alpha1.ConvertTo(oldObj); err != nil {
			return fmt.Errorf("failed to convert v1alpha1->v1beta1: %v", err)
		}
	}
	return nil
}

//var _ inject.Client = &StatefulSetCreateUpdateHandler{}
//
//// InjectClient injects the client into the StatefulSetCreateUpdateHandler
//func (h *StatefulSetCreateUpdateHandler) InjectClient(c client.Client) error {
//	h.Client = c
//	return nil
//}
