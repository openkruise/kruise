/*
Copyright 2020 The Kruise Authors.

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
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	genericvalidation "k8s.io/apimachinery/pkg/api/validation"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"k8s.io/klog/v2"
	apivalidation "k8s.io/kubernetes/pkg/apis/core/validation"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	appsv1beta1 "github.com/openkruise/kruise/apis/apps/v1beta1"
)

// ValidateDaemonSetName can be used to check whether the given daemon set name is valid.
// Prefix indicates this name will be used as part of generation, in which case
// trailing dashes are allowed.
var ValidateDaemonSetName = genericvalidation.NameIsDNSSubdomain

// DaemonSetCreateUpdateHandler handles DaemonSet
type DaemonSetCreateUpdateHandler struct {
	// Decoder decodes objects
	Decoder admission.Decoder
}

func (h *DaemonSetCreateUpdateHandler) validateDaemonSetUpdate(ds, oldDs *appsv1alpha1.DaemonSet) field.ErrorList {
	allErrs := apivalidation.ValidateObjectMetaUpdate(&ds.ObjectMeta, &oldDs.ObjectMeta, field.NewPath("metadata"))
	daemonset := ds.DeepCopy()
	daemonset.Spec.Template = oldDs.Spec.Template
	daemonset.Spec.UpdateStrategy = oldDs.Spec.UpdateStrategy
	daemonset.Spec.Lifecycle = oldDs.Spec.Lifecycle
	daemonset.Spec.BurstReplicas = oldDs.Spec.BurstReplicas
	daemonset.Spec.MinReadySeconds = oldDs.Spec.MinReadySeconds
	daemonset.Spec.RevisionHistoryLimit = oldDs.Spec.RevisionHistoryLimit

	if !apiequality.Semantic.DeepEqual(daemonset.Spec, oldDs.Spec) {
		allErrs = append(allErrs, field.Forbidden(field.NewPath("spec"), "updates to daemonset spec for fields other than 'BurstReplicas', 'template', 'lifecycle',  'updateStrategy', 'minReadySeconds', and 'revisionHistoryLimit' are forbidden"))
	}
	allErrs = append(allErrs, validateDaemonSetSpec(&ds.Spec, field.NewPath("spec"))...)
	return allErrs
}

func (h *DaemonSetCreateUpdateHandler) validateDaemonSetUpdateV1beta1(ds, oldDs *appsv1beta1.DaemonSet) field.ErrorList {
	allErrs := apivalidation.ValidateObjectMetaUpdate(&ds.ObjectMeta, &oldDs.ObjectMeta, field.NewPath("metadata"))
	daemonset := ds.DeepCopy()
	daemonset.Spec.Template = oldDs.Spec.Template
	daemonset.Spec.UpdateStrategy = oldDs.Spec.UpdateStrategy
	daemonset.Spec.Lifecycle = oldDs.Spec.Lifecycle
	daemonset.Spec.BurstReplicas = oldDs.Spec.BurstReplicas
	daemonset.Spec.MinReadySeconds = oldDs.Spec.MinReadySeconds
	daemonset.Spec.RevisionHistoryLimit = oldDs.Spec.RevisionHistoryLimit
	daemonset.Spec.ScaleStrategy = oldDs.Spec.ScaleStrategy

	if !apiequality.Semantic.DeepEqual(daemonset.Spec, oldDs.Spec) {
		allErrs = append(allErrs, field.Forbidden(field.NewPath("spec"), "updates to daemonset spec for fields other than 'BurstReplicas', 'template', 'lifecycle', 'scaleStrategy', 'updateStrategy', 'minReadySeconds', and 'revisionHistoryLimit' are forbidden"))
	}
	allErrs = append(allErrs, validateDaemonSetSpecV1beta1(&ds.Spec, field.NewPath("spec"))...)
	return allErrs
}

var _ admission.Handler = &DaemonSetCreateUpdateHandler{}

// Handle handles admission requests.
func (h *DaemonSetCreateUpdateHandler) Handle(ctx context.Context, req admission.Request) admission.Response {
	switch req.AdmissionRequest.Resource.Version {
	case appsv1beta1.GroupVersion.Version:
		obj := &appsv1beta1.DaemonSet{}
		oldObj := &appsv1beta1.DaemonSet{}
		switch req.AdmissionRequest.Operation {
		case admissionv1.Create:
			if err := h.Decoder.Decode(req, obj); err != nil {
				return admission.Errored(http.StatusBadRequest, err)
			}
			allowed, reason, err := validatingDaemonSetFnV1beta1(ctx, obj)
			if err != nil {
				klog.ErrorS(err, "validate daemonset failed", "namespace", obj.Namespace, "name", obj.Name, "operation", req.AdmissionRequest.Operation)
				return admission.Errored(http.StatusInternalServerError, err)
			}
			return admission.ValidationResponse(allowed, reason)

		case admissionv1.Update:
			if err := h.Decoder.Decode(req, obj); err != nil {
				return admission.Errored(http.StatusBadRequest, err)
			}
			if err := h.Decoder.DecodeRaw(req.AdmissionRequest.OldObject, oldObj); err != nil {
				return admission.Errored(http.StatusBadRequest, err)
			}

			if allErrs := h.validateDaemonSetUpdateV1beta1(obj, oldObj); len(allErrs) > 0 {
				return admission.Errored(http.StatusUnprocessableEntity, allErrs.ToAggregate())
			}
		}
		return admission.ValidationResponse(true, "")

	case appsv1alpha1.GroupVersion.Version:
		obj := &appsv1alpha1.DaemonSet{}
		oldObj := &appsv1alpha1.DaemonSet{}
		switch req.AdmissionRequest.Operation {
		case admissionv1.Create:
			if err := h.Decoder.Decode(req, obj); err != nil {
				return admission.Errored(http.StatusBadRequest, err)
			}
			allowed, reason, err := validatingDaemonSetFn(ctx, obj)
			if err != nil {
				klog.ErrorS(err, "validate daemonset failed", "namespace", obj.Namespace, "name", obj.Name, "operation", req.AdmissionRequest.Operation)
				return admission.Errored(http.StatusInternalServerError, err)
			}
			return admission.ValidationResponse(allowed, reason)

		case admissionv1.Update:
			if err := h.Decoder.Decode(req, obj); err != nil {
				return admission.Errored(http.StatusBadRequest, err)
			}
			if err := h.Decoder.DecodeRaw(req.AdmissionRequest.OldObject, oldObj); err != nil {
				return admission.Errored(http.StatusBadRequest, err)
			}

			if allErrs := h.validateDaemonSetUpdate(obj, oldObj); len(allErrs) > 0 {
				return admission.Errored(http.StatusUnprocessableEntity, allErrs.ToAggregate())
			}
		}
		return admission.ValidationResponse(true, "")
	}
	return admission.Errored(http.StatusBadRequest, fmt.Errorf("unsupported version: %s", req.AdmissionRequest.Resource.Version))
}
